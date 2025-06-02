package main

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/exec"
	"bitunix-bot/internal/features"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
	"bitunix-bot/internal/storage"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

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
	trades := make(chan bitunix.Trade, 1024)
	depths := make(chan bitunix.Depth, 1024)
	errors := make(chan error, 100)

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
	vwapMap := make(map[string]*features.VWAP)
	ticksMap := make(map[string]*features.TickImb)
	lastPriceMap := sync.Map{} // Thread-safe map for lastPrice

	for _, s := range c.Symbols {
		vwapMap[s] = features.NewVWAP(c.VWAPWindow, c.VWAPSize)
		ticksMap[s] = features.NewTickImb(c.TickSize)
		lastPriceMap.Store(s, 0.0)
	}

	// Initialize production ML predictor
	mlConfig := ml.PredictorConfig{
		ModelPath:         c.ModelPath,
		FallbackThreshold: c.ProbThreshold,
		MaxRetries:        3,
		RetryDelay:        time.Second,
		CacheSize:         1000,
		CacheTTL:          5 * time.Minute,
		EnableProfiling:   os.Getenv("ML_PROFILING") == "true",
		EnableValidation:  true,
		MinConfidence:     0.5,
	}

	// Create executor with appropriate predictor
	var exe *exec.Exec
	prod, err := ml.NewProductionPredictor(mlConfig, mw)
	if err != nil {
		log.Warn().Err(err).Msg("ML model unavailable, using fallback")
		// Create basic predictor as fallback
		basicPred, _ := ml.NewWithMetrics(c.ModelPath, mw, 5*time.Second)
		exe = exec.New(c, basicPred, mw)
	} else {
		// Explicitly cast to interface to ensure compatibility
		var predictor ml.PredictorInterface = prod
		exe = exec.New(c, predictor, mw)

		// Start ML model server if enabled (only for ProductionPredictor)
		if mlPort := os.Getenv("ML_SERVER_PORT"); mlPort != "" {
			port, _ := strconv.Atoi(mlPort)
			if port > 0 {
				mlServer := ml.NewModelServer(prod, port)
				go func() {
					if err := mlServer.Start(); err != nil && err != http.ErrServerClosed {
						log.Error().Err(err).Msg("ML server failed")
					}
				}()
				defer mlServer.Shutdown(context.Background())
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

	// Depth handler goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case d := <-depths:
				// Thread-safe price access
				priceVal, ok := lastPriceMap.Load(d.Symbol)
				if !ok {
					continue
				}
				price, ok := priceVal.(float64)
				if !ok {
					continue
				}
				if price == 0 {
					continue // Skip if no price data yet
				}

				vwap, std := vwapMap[d.Symbol].CalcWithMetrics(mw)
				if std == 0 {
					continue
				}

				// Increment VWAP calculation metric
				m.VWAPCalculations.Inc()

				tickRatio := ticksMap[d.Symbol].Ratio()
				depthRatio := features.DepthImbWithMetrics(d.BidVol, d.AskVol, mw)
				exe.Try(d.Symbol, price, vwap, std, tickRatio, depthRatio, d.BidVol, d.AskVol)

				// Store depth data if storage is available
				if store != nil {
					store.StoreDepth(d)
				}

				// Update metrics
				m.DepthsReceived.Inc()
			}
		}
	}()

	// Trade handler goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-trades:
				vwapMap[t.Symbol].Add(t.Price, t.Qty)

				// Thread-safe price update
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

				// Store trade data if storage is available
				if store != nil {
					store.StoreTrade(t)
				}

				// Update metrics
				m.TradesReceived.Inc()
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
