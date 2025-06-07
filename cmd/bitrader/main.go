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
	"bitunix-bot/internal/common"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/exec"
	"bitunix-bot/internal/features"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
	"bitunix-bot/internal/security"
	"bitunix-bot/internal/storage"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// SecurityManagerAdapter adapts security.SecurityManager to exec.SecurityManager interface
type SecurityManagerAdapter struct {
	sm *security.SecurityManager
}

func (sma *SecurityManagerAdapter) LogTradingAction(eventType, userIP, userAgent string, tradingData exec.TradingAuditData, success bool, errorMsg string) {
	// Convert exec.TradingAuditData to security.TradingAuditData
	securityData := security.TradingAuditData{
		Symbol:    tradingData.Symbol,
		Side:      tradingData.Side,
		Quantity:  tradingData.Quantity,
		Price:     tradingData.Price,
		OrderType: tradingData.OrderType,
		OrderID:   tradingData.OrderID,
		Balance:   tradingData.Balance,
		PnL:       tradingData.PnL,
	}

	if sma.sm != nil {
		sma.sm.LogTradingAction(eventType, userIP, userAgent, securityData, success, errorMsg)
	}
}

func main() {
	c, err := cfg.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load failed")
	}

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize security manager
	securityConfig := security.LoadSecurityConfig()
	securityManager, err := security.NewSecurityManager(securityConfig)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize security manager, continuing with basic security")
		securityManager = nil
	}

	// Initialize components
	m := metrics.New()
	mw := metrics.NewWrapper(m)
	store := initializeStorage(c)
	if store != nil {
		defer store.Close()
	}

	// Create communication channels
	trades := make(chan bitunix.Trade, 64)
	depths := make(chan bitunix.Depth, 64)
	errors := make(chan error, 32)

	// Start metrics server with security
	startMetricsServer(ctx, c, cancel, securityManager)

	// Start WebSocket
	ws := bitunix.NewWS(c.WsURL)
	startWebSocketHandler(ctx, ws, c, trades, depths, errors)

	// Initialize feature buffers and executor
	vwapMap, ticksMap, lastPriceMap := initializeFeatureBuffers(c)
	exe := initializeExecutor(c, mw, securityManager)

	// Set security manager in executor for audit logging
	if securityManager != nil && exe != nil {
		adapter := &SecurityManagerAdapter{sm: securityManager}
		exe.SetSecurityManager(adapter)
	}

	// Start background goroutines
	var wg sync.WaitGroup
	startErrorHandler(ctx, &wg, errors, m)
	startDepthHandler(ctx, &wg, depths, vwapMap, ticksMap, lastPriceMap, exe, store, mw, securityManager)
	startTradeHandler(ctx, &wg, trades, vwapMap, ticksMap, lastPriceMap, store, mw)

	// Close security manager on shutdown
	defer func() {
		if securityManager != nil {
			if auditLogger := securityManager.GetAuditLogger(); auditLogger != nil {
				auditLogger.Close()
			}
		}
	}()

	// Wait for shutdown signal
	waitForShutdown(ctx, cancel, &wg)
}

// initializeStorage initializes storage if DATA_PATH is configured
func initializeStorage(c cfg.Settings) *storage.Store {
	if c.DataPath != "" {
		store, err := storage.New(c.DataPath)
		if err != nil {
			log.Warn().Err(err).Msg("storage initialization failed, continuing without persistence")
			return nil
		}
		return store
	}
	return nil
}

// startMetricsServer starts the Prometheus metrics HTTP server
func startMetricsServer(ctx context.Context, c cfg.Settings, cancel context.CancelFunc, securityManager *security.SecurityManager) {
	go func() {
		mux := http.NewServeMux()

		// Add health endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		// Add metrics endpoint
		mux.Handle("/metrics", promhttp.Handler())

		// Create handler with security middleware if available
		var handler http.Handler = mux
		if securityManager != nil {
			// Apply security middleware in order: IP whitelist -> Rate limit -> Signature verification
			handler = securityManager.IPWhitelistMiddleware(handler)
			handler = securityManager.RateLimitMiddleware(handler)
			handler = securityManager.APISignatureMiddleware(handler)

			log.Info().Msg("Security middleware enabled for metrics server")
		}

		server := &http.Server{
			Addr:              fmt.Sprintf(":%d", c.MetricsPort),
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		go func() {
			<-ctx.Done()
			if err := server.Shutdown(context.Background()); err != nil {
				log.Error().Err(err).Msg("failed to shutdown metrics server")
			}
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("metrics server failed")
		}
	}()
}

// startWebSocketHandler starts the WebSocket connection handler
func startWebSocketHandler(ctx context.Context, ws *bitunix.WS, c cfg.Settings, trades chan bitunix.Trade, depths chan bitunix.Depth, errors chan error) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ws.Stream(ctx, c.Symbols, trades, depths, errors, c.Ping); err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("WebSocket stream ended")
			errors <- err
		}
	}()
}

// initializeFeatureBuffers creates and initializes feature calculation buffers
func initializeFeatureBuffers(c cfg.Settings) (map[string]*features.FastVWAP, map[string]*features.TickImb, *sync.Map) {
	vwapMap := make(map[string]*features.FastVWAP)
	ticksMap := make(map[string]*features.TickImb)
	var lastPriceMap sync.Map

	for _, s := range c.Symbols {
		vwapMap[s] = features.NewFastVWAP(c.VWAPWindow, c.VWAPSize)
		ticksMap[s] = features.NewTickImb(c.TickSize)
		lastPriceMap.Store(s, 0.0)
	}

	return vwapMap, ticksMap, &lastPriceMap
}

// initializeExecutor creates the trading executor with ML predictor
func initializeExecutor(c cfg.Settings, mw *metrics.MetricsWrapper, securityManager *security.SecurityManager) *exec.Exec {
	mlConfig := ml.PredictorConfig{
		ModelPath:         c.ModelPath,
		FallbackThreshold: 0.65,
		MaxRetries:        3,
		RetryDelay:        100 * time.Millisecond,
		CacheSize:         1000,
		CacheTTL:          5 * time.Minute,
		EnableProfiling:   false,
		EnableValidation:  true,
		MinConfidence:     0.6,
	}

	prod, err := ml.NewProductionPredictor(mlConfig, mw)
	if err != nil {
		log.Warn().Err(err).Msg("ML model unavailable, using fallback")
		basicPred, err := ml.NewWithMetrics(c.ModelPath, mw, 5*time.Second)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create basic predictor, using nil predictor")
			return exec.New(c, nil, mw)
		}
		return exec.New(c, basicPred, mw)
	}

	var predictor ml.PredictorInterface = prod
	exe := exec.New(c, predictor, mw)

	// Start ML model server if enabled
	startMLModelServer(c, mw)

	return exe
}

// startMLModelServer starts the ML model server if ML_SERVER_PORT is configured
func startMLModelServer(c cfg.Settings, mw *metrics.MetricsWrapper) {
	if mlPort := os.Getenv(common.EnvMLServerPort); mlPort != "" {
		port, err := strconv.Atoi(mlPort)
		if err != nil {
			log.Error().Err(err).Msg("Invalid ML_SERVER_PORT value")
			return
		}
		if port > 0 {
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
				mlServer := ml.NewModelServer(prodPredictor, port)
				go func() {
					if err := mlServer.Start(); err != nil && err != http.ErrServerClosed {
						log.Error().Err(err).Msg("ML server failed")
					}
				}()
			}
		}
	}
}

// startErrorHandler starts the background error handling goroutine
func startErrorHandler(ctx context.Context, wg *sync.WaitGroup, errors chan error, m *metrics.Metrics) {
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
}

// startDepthHandler starts the depth data processing goroutine with batch processing
func startDepthHandler(ctx context.Context, wg *sync.WaitGroup, depths chan bitunix.Depth,
	vwapMap map[string]*features.FastVWAP, ticksMap map[string]*features.TickImb,
	lastPriceMap *sync.Map, exe *exec.Exec, store *storage.Store, mw *metrics.MetricsWrapper,
	securityManager *security.SecurityManager,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		batchSize := 10
		depthBatch := make([]bitunix.Depth, 0, batchSize)
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case d := <-depths:
				depthBatch = append(depthBatch, d)
				if len(depthBatch) >= batchSize {
					processDepthBatch(depthBatch, vwapMap, ticksMap, lastPriceMap, exe, store, mw, securityManager)
					depthBatch = depthBatch[:0]
				}
			case <-ticker.C:
				if len(depthBatch) > 0 {
					processDepthBatch(depthBatch, vwapMap, ticksMap, lastPriceMap, exe, store, mw, securityManager)
					depthBatch = depthBatch[:0]
				}
			}
		}
	}()
}

// startTradeHandler starts the trade data processing goroutine with batch processing
func startTradeHandler(ctx context.Context, wg *sync.WaitGroup, trades chan bitunix.Trade,
	vwapMap map[string]*features.FastVWAP, ticksMap map[string]*features.TickImb,
	lastPriceMap *sync.Map, store *storage.Store, mw *metrics.MetricsWrapper,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		batchSize := 20
		tradeBatch := make([]bitunix.Trade, 0, batchSize)
		ticker := time.NewTicker(1 * time.Millisecond)
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
}

// waitForShutdown waits for shutdown signals and handles graceful shutdown
func waitForShutdown(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Info().Msg("shutdown signal received")
	case <-ctx.Done():
		log.Info().Msg("context canceled")
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
	ticksMap map[string]*features.TickImb, lastPriceMap *sync.Map,
	exe *exec.Exec, store *storage.Store, m *metrics.MetricsWrapper,
	securityManager *security.SecurityManager,
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

		// Try to execute trade and log attempt
		exe.Try(d.Symbol, price, vwap, std, tickRatio, depthRatio, d.BidVol, d.AskVol)

		// Log trading analysis for audit
		if securityManager != nil {
			securityManager.LogAuditEvent(security.AuditEvent{
				EventType: "trading_analysis",
				Method:    "DEPTH_ANALYSIS",
				Path:      "/internal/trading",
				Success:   true,
				Data: map[string]interface{}{
					"symbol":      d.Symbol,
					"price":       price,
					"vwap":        vwap,
					"std":         std,
					"tick_ratio":  tickRatio,
					"depth_ratio": depthRatio,
					"bid_vol":     d.BidVol,
					"ask_vol":     d.AskVol,
				},
			})
		}

		if store != nil {
			store.StoreDepth(d)
		}

		m.FeatureSampleCount(1)
	}
}

func processTradeBatch(batch []bitunix.Trade, vwapMap map[string]*features.FastVWAP,
	ticksMap map[string]*features.TickImb, lastPriceMap *sync.Map,
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
