package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/exec"
	"bitunix-bot/internal/features"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
	"bitunix-bot/internal/storage"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func main() {
	c, err := cfg.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load failed")
	}

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize metrics
	m := metrics.New()
	mw := metrics.NewWrapper(m)

	// Initialize storage if DATA_PATH is set
	var store *storage.Store
	if c.DataPath != "" {
		store, err = storage.New(c.DataPath)
		if err != nil {
			log.Warn().Err(err).Msg("storage initialization failed, continuing without persistence")
		} else {
			defer store.Close()
		}
	}

	// Channels (buffered to prevent blocking)
	trades := make(chan bitunix.Trade, 64) // Reduced buffer size for lower latency
	depths := make(chan bitunix.Depth, 64) // Reduced buffer size for lower latency
	errors := make(chan error, 32)         // Reduced buffer size for lower latency

	// Start metrics server
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", c.MetricsPort),
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			server.Shutdown(context.Background())
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("metrics server failed")
		}
	}()

	// Start WebSocket with context and error handling
	ws := bitunix.NewWS(c.WsURL)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ws.Stream(ctx, c.Symbols, trades, depths, errors, c.Ping); err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("WebSocket stream ended")
			errors <- err
		}
	}()

	// Feature buffers per symbol with thread-safe access
	vwapMap := make(map[string]*features.FastVWAP) // Using optimized FastVWAP
	ticksMap := make(map[string]*features.TickImb)
	lastPriceMap := sync.Map{} // Thread-safe map for lastPrice

	for _, s := range c.Symbols {
		vwapMap[s] = features.NewFastVWAP(c.VWAPWindow, c.VWAPSize)
		ticksMap[s] = features.NewTickImb(c.TickSize)
		lastPriceMap.Store(s, 0.0)
	}

	// Initialize production ML predictor
	mlConfig := ml.FastPredictorConfig{
		ModelPath:   c.ModelPath,
		BatchSize:   32,
		Timeout:     5 * time.Millisecond,
		CacheSize:   1000,
		CacheTTL:    5 * time.Minute,
		EnableCache: true,
	}

	// Create executor with appropriate predictor
	var exe *exec.Exec
	prod, err := ml.NewFastPredictor(mlConfig, mw)
	if err != nil {
		log.Warn().Err(err).Msg("ML model unavailable, using fallback")
		// Create basic predictor as fallback
		basicPred, _ := ml.NewWithMetrics(c.ModelPath, mw, 5*time.Second)
		exe = exec.New(c, basicPred, mw)
	} else {
		// Explicitly cast to interface to ensure compatibility
		var predictor ml.PredictorInterface = prod
		exe = exec.New(c, predictor, mw)

		// Start ML model server if enabled
		if mlPort := os.Getenv("ML_SERVER_PORT"); mlPort != "" {
			port, _ := strconv.Atoi(mlPort)
			if port > 0 {
				// Create a production predictor for the model server
				prodConfig := ml.PredictorConfig{
					ModelPath:         c.ModelPath,
					FallbackThreshold: 0.65,
					MaxRetries:        3,
					RetryDelay:        100 * time.Millisecond,
					CacheSize:         1000,
					CacheTTL:          5 * time.Minute,
					EnableProfiling:   true,
					EnableValidation:  true,
					MinConfidence:     0.6,
				}
				prodPredictor, err := ml.NewProductionPredictor(prodConfig, mw)
				if err != nil {
					log.Error().Err(err).Msg("Failed to create production predictor for model server")
				} else {
					// Create a new model server with the production predictor
					mlServer := ml.NewModelServer(prodPredictor, port)
					go func() {
						if err := mlServer.Start(); err != nil && err != http.ErrServerClosed {
							log.Error().Err(err).Msg("ML server failed")
						}
					}()
					defer mlServer.Shutdown(context.Background())
				}
			}
		}
	}

	// Error handler goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errors:
				log.Error().Err(err).Msg("background error")
				m.WSReconnects.Inc()
				m.ErrorsTotal.Inc()
			}
		}
	}()

	// Depth handler goroutine with batch processing
	wg.Add(1)
	go func() {
		defer wg.Done()
		batchSize := 10
		depthBatch := make([]bitunix.Depth, 0, batchSize)
		ticker := time.NewTicker(1 * time.Millisecond) // Process every 1ms
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case d := <-depths:
				depthBatch = append(depthBatch, d)
				if len(depthBatch) >= batchSize {
					processDepthBatch(depthBatch, vwapMap, ticksMap, lastPriceMap, exe, store, mw)
					depthBatch = depthBatch[:0]
				}
			case <-ticker.C:
				if len(depthBatch) > 0 {
					processDepthBatch(depthBatch, vwapMap, ticksMap, lastPriceMap, exe, store, mw)
					depthBatch = depthBatch[:0]
				}
			}
		}
	}()

	// Trade handler goroutine with batch processing
	wg.Add(1)
	go func() {
		defer wg.Done()
		batchSize := 20
		tradeBatch := make([]bitunix.Trade, 0, batchSize)
		ticker := time.NewTicker(1 * time.Millisecond) // Process every 1ms
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case t := <-trades:
				tradeBatch = append(tradeBatch, t)
				if len(tradeBatch) >= batchSize {
					processTradeBatch(tradeBatch, vwapMap, ticksMap, lastPriceMap, store, mw)
					tradeBatch = tradeBatch[:0]
				}
			case <-ticker.C:
				if len(tradeBatch) > 0 {
					processTradeBatch(tradeBatch, vwapMap, ticksMap, lastPriceMap, store, mw)
					tradeBatch = tradeBatch[:0]
				}
			}
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Info().Msg("shutdown signal received")
	case <-ctx.Done():
		log.Info().Msg("context cancelled")
	}

	log.Info().Msg("shutting down gracefully...")
	cancel() // Cancel context to stop all goroutines

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("all goroutines stopped")
	case <-time.After(10 * time.Second):
		log.Warn().Msg("shutdown timeout, forcing exit")
	}
}

// Helper functions for batch processing
func processDepthBatch(batch []bitunix.Depth, vwapMap map[string]*features.FastVWAP,
	ticksMap map[string]*features.TickImb, lastPriceMap sync.Map,
	exe *exec.Exec, store *storage.Store, m *metrics.MetricsWrapper,
) {
	for _, d := range batch {
		priceVal, ok := lastPriceMap.Load(d.Symbol)
		if !ok {
			continue
		}
		price, ok := priceVal.(float64)
		if !ok || price == 0 {
			continue
		}

		vwap, std := vwapMap[d.Symbol].CalcWithMetrics(m)
		if std == 0 {
			continue
		}

		tickRatio := ticksMap[d.Symbol].Ratio()
		depthRatio := features.DepthImbWithMetrics(d.BidVol, d.AskVol, m)
		exe.Try(d.Symbol, price, vwap, std, tickRatio, depthRatio, d.BidVol, d.AskVol)

		if store != nil {
			store.StoreDepth(d)
		}

		m.FeatureSampleCount(1)
	}
}

func processTradeBatch(batch []bitunix.Trade, vwapMap map[string]*features.FastVWAP,
	ticksMap map[string]*features.TickImb, lastPriceMap sync.Map,
	store *storage.Store, m *metrics.MetricsWrapper,
) {
	for _, t := range batch {
		vwapMap[t.Symbol].Add(t.Price, t.Qty)

		priceVal, _ := lastPriceMap.Load(t.Symbol)
		var oldPrice float64
		if priceVal != nil {
			if p, ok := priceVal.(float64); ok {
				oldPrice = p
			}
		}

		sign := int8(0)
		if t.Price > oldPrice {
			sign = 1
		} else if t.Price < oldPrice {
			sign = -1
		}
		ticksMap[t.Symbol].Add(sign)
		lastPriceMap.Store(t.Symbol, t.Price)

		if store != nil {
			store.StoreTrade(t)
		}

		m.FeatureSampleCount(1)
	}
}
