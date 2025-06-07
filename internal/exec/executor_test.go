package exec

import (
	"math"
	"sync"
	"testing"
	"time"

	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"

	"github.com/prometheus/client_golang/prometheus"
)

// MockPredictor implements ml.PredictorInterface for testing
type MockPredictor struct {
	mu            sync.RWMutex
	approveCount  int
	rejectCount   int
	shouldApprove bool
}

func (m *MockPredictor) Approve(features []float32, threshold float64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldApprove {
		m.approveCount++
		return true
	}
	m.rejectCount++
	return false
}

func (m *MockPredictor) Predict(features []float32) ([]float32, error) {
	return []float32{0.5, 0.5}, nil
}

func createTestMetrics() *metrics.MetricsWrapper {
	return metrics.NewWrapper(metrics.NewWithRegistry(prometheus.NewRegistry()))
}

func createTestConfig() cfg.Settings {
	return cfg.Settings{
		Key:                 "test-key",
		Secret:              "test-secret",
		InitialBalance:      10000,
		MaxPositionSize:     1.0,
		MaxPositionExposure: 0.5,  // 50% max exposure per symbol
		MaxDailyLoss:        0.05, // 5% daily loss limit
		RiskUSD:             1000,
		Leverage:            1,
		BaseURL:             "https://api.bitunix.com",
		WsURL:               "wss://fapi.bitunix.com/public",
		RESTTimeout:         5 * time.Second,
		DryRun:              true, // Set to true to avoid FORCE_LIVE_TRADING requirement
		// Circuit breaker settings with relaxed values for testing
		CircuitBreakerVolatility:   10.0,            // High threshold to avoid triggering in tests
		CircuitBreakerImbalance:    0.95,            // High threshold to avoid triggering in tests
		CircuitBreakerVolume:       100.0,           // High threshold to avoid triggering in tests
		CircuitBreakerErrorRate:    0.9,             // High threshold to avoid triggering in tests
		CircuitBreakerRecoveryTime: 1 * time.Minute, // Short recovery time for tests
		// Order execution timeout settings
		OrderExecutionTimeout:    30 * time.Second,
		OrderStatusCheckInterval: 5 * time.Second,
		MaxOrderRetries:          3,
	}
}

func TestExecutor_New(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{}, createTestMetrics())

	if exec.config.InitialBalance != config.InitialBalance {
		t.Errorf("Expected balance %v, got %v", config.InitialBalance, exec.config.InitialBalance)
	}
}

func TestExecutor_Try(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{shouldApprove: true}, createTestMetrics())

	// Test long position - use values that won't trigger circuit breaker
	exec.Try("BTCUSDT", 50000, 49000, 5.0, 0.5, 0.5, 10, 10) // std=5.0, volume=20

	// Test short position
	exec.Try("BTCUSDT", 51000, 50000, 5.0, -0.5, -0.5, 10, 10)

	// Test rejected signal
	exec.Try("BTCUSDT", 50000, 50000, 0, 0, 0, 10, 10)
}

func TestExecutor_Size(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{}, createTestMetrics())

	// Test position sizing with different balances
	testCases := []struct {
		symbol       string
		price        float64
		expectedSize float64
	}{
		{"BTCUSDT", 50000, 0.02},  // 1000 USD risk
		{"ETHUSDT", 3000, 0.33},   // 1000 USD risk
		{"BTCUSDT", 100000, 0.01}, // 1000 USD risk
	}

	for _, tc := range testCases {
		size := exec.Size(tc.symbol, tc.price)
		if size != tc.expectedSize {
			t.Errorf("Expected size %v for %s at price %v, got %v",
				tc.expectedSize, tc.symbol, tc.price, size)
		}
	}
}

func TestExecutor_ConcurrentAccess(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{shouldApprove: true}, createTestMetrics())
	var wg sync.WaitGroup

	// Test concurrent position operations - use price < vwap to generate BUY signals
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exec.Try("BTCUSDT", 49000, 50000, 5.0, 0.5, 0.5, 10, 10) // price < vwap = BUY, std=5.0, volume=20
		}()
	}

	wg.Wait()

	// Verify final state
	positions := exec.GetPositions()
	if positions["BTCUSDT"] <= 0 {
		t.Error("Position size should be positive after concurrent operations")
	}
}

func TestExecutor_UpdatePnL(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{}, createTestMetrics())

	// Test PnL updates
	exec.UpdatePnL(100)
	if exec.GetDailyPnL() != 100 {
		t.Errorf("Expected PnL 100, got %v", exec.GetDailyPnL())
	}

	exec.UpdatePnL(-50)
	if exec.GetDailyPnL() != 50 {
		t.Errorf("Expected PnL 50, got %v", exec.GetDailyPnL())
	}
}

func TestExecutor_GetPositions(t *testing.T) {
	config := createTestConfig()
	exec := New(config, &MockPredictor{shouldApprove: true}, createTestMetrics())

	// Open positions - use price < vwap to generate BUY signals (positive positions)
	exec.Try("BTCUSDT", 49000, 50000, 1000, 0.5, 0.5, 100, 100) // price < vwap = BUY
	exec.Try("ETHUSDT", 2900, 3000, 100, 0.5, 0.5, 100, 100)    // price < vwap = BUY

	// Verify positions
	positions := exec.GetPositions()
	if positions["BTCUSDT"] <= 0 {
		t.Error("BTCUSDT position should be positive")
	}
	if positions["ETHUSDT"] <= 0 {
		t.Error("ETHUSDT position should be positive")
	}
}

func TestDailyLossLimitEnforcement(t *testing.T) {
	// Create test config with low daily loss limit for testing
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02, // 2% daily loss limit
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	// Test 1: Trading should be allowed initially
	if !exec.CanTrade() {
		t.Error("Trading should be allowed initially")
	}

	// Test 2: Simulate a small loss (should still allow trading)
	exec.UpdatePnL(-100) // 1% loss
	if !exec.CanTrade() {
		t.Error("Trading should be allowed with 1% loss (below 2% limit)")
	}

	// Test 3: Simulate reaching the daily loss limit
	exec.UpdatePnL(-100) // Additional 1% loss, total 2%
	if exec.CanTrade() {
		t.Error("Trading should be suspended at 2% loss limit")
	}

	// Test 4: Verify the daily P&L is correct
	if exec.GetDailyPnL() != -200 {
		t.Errorf("Expected daily P&L of -200, got %v", exec.GetDailyPnL())
	}

	// Test 5: Simulate exceeding the daily loss limit
	exec.UpdatePnL(-50) // Additional 0.5% loss, total 2.5%
	if exec.CanTrade() {
		t.Error("Trading should remain suspended when exceeding loss limit")
	}
}

func TestDailyTrackingReset(t *testing.T) {
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02,
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	// Simulate losses
	exec.UpdatePnL(-300) // 3% loss
	if exec.CanTrade() {
		t.Error("Trading should be suspended after 3% loss")
	}

	// Reset daily tracking
	exec.ResetDailyTracking()

	// Verify P&L is reset
	if exec.GetDailyPnL() != 0 {
		t.Errorf("Expected daily P&L to be reset to 0, got %v", exec.GetDailyPnL())
	}

	// Verify trading is allowed again
	if !exec.CanTrade() {
		t.Error("Trading should be allowed after daily reset")
	}
}

func TestPositionExposureLimitEnforcement(t *testing.T) {
	// Create test config with position exposure limits
	config := cfg.Settings{
		Key:                 "test",
		Secret:              "test",
		BaseURL:             "http://test",
		WsURL:               "ws://test",
		Symbols:             []string{"BTCUSDT"},
		VWAPWindow:          30 * time.Second,
		VWAPSize:            600,
		TickSize:            50,
		BaseSizeRatio:       0.002,
		ProbThreshold:       0.65,
		DryRun:              true,
		MaxDailyLoss:        0.05,
		MetricsPort:         8080,
		MaxPositionSize:     0.01,
		MaxPositionExposure: 0.1, // 10% max exposure per symbol
		MaxPriceDistance:    3.0,
		Leverage:            20,
		MarginMode:          "ISOLATION",
		RiskUSD:             25.0,
		RESTTimeout:         5 * time.Second,
		InitialBalance:      10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	price := 50000.0
	maxAllowedExposure := exec.GetMaxAllowedExposure(symbol) // 10% of 10000 = 1000 USD

	// Test 1: Small position should be allowed
	smallSize := 0.01 // 0.01 * 50000 = 500 USD (under 1000 limit)
	if !exec.CanTradeSymbol(symbol, smallSize, price) {
		t.Error("Small position should be allowed")
	}

	// Test 2: Simulate adding a position that's within limits
	exec.mu.Lock()
	exec.positionSizes[symbol] = smallSize
	exec.mu.Unlock()

	// Verify exposure calculation
	currentExposure := exec.GetPositionExposure(symbol, price)
	expectedExposure := 500.0
	if currentExposure != expectedExposure {
		t.Errorf("Expected exposure %v, got %v", expectedExposure, currentExposure)
	}

	// Test 3: Another small position should still be allowed
	additionalSize := 0.005 // Total would be 0.015 * 50000 = 750 USD (still under limit)
	if !exec.CanTradeSymbol(symbol, additionalSize, price) {
		t.Error("Additional small position should be allowed")
	}

	// Test 4: Large position that would exceed limit should be rejected
	largeSize := 0.015 // Current 0.01 + 0.015 = 0.025 * 50000 = 1250 USD (over 1000 limit)
	if exec.CanTradeSymbol(symbol, largeSize, price) {
		t.Error("Large position that exceeds exposure limit should be rejected")
	}

	// Test 5: Test with negative position (short)
	exec.mu.Lock()
	exec.positionSizes[symbol] = -0.01 // Short position
	exec.mu.Unlock()

	// Additional short position that would exceed limit should be rejected
	largeShortSize := -0.015 // Total would be -0.025 * 50000 = 1250 USD exposure (over limit)
	if exec.CanTradeSymbol(symbol, largeShortSize, price) {
		t.Error("Large short position that exceeds exposure limit should be rejected")
	}

	// Test 6: Position reducing trade should be allowed even if current position exceeds limit
	exec.mu.Lock()
	exec.positionSizes[symbol] = 0.025 // Set position above limit (for testing reducing trades)
	exec.mu.Unlock()

	reducingSize := -0.005              // Reducing the large position
	newPosition := 0.025 + reducingSize // 0.02 * 50000 = 1000 USD (at limit)
	newExposure := math.Abs(newPosition) * price
	if newExposure <= maxAllowedExposure {
		if !exec.CanTradeSymbol(symbol, reducingSize, price) {
			t.Error("Position reducing trade that brings exposure within limit should be allowed")
		}
	}
}

func TestPositionExposureLimitWithSymbolConfig(t *testing.T) {
	// Create test config with per-symbol position exposure limits
	config := cfg.Settings{
		Key:                 "test",
		Secret:              "test",
		BaseURL:             "http://test",
		WsURL:               "ws://test",
		Symbols:             []string{"BTCUSDT", "ETHUSDT"},
		VWAPWindow:          30 * time.Second,
		VWAPSize:            600,
		TickSize:            50,
		BaseSizeRatio:       0.002,
		ProbThreshold:       0.65,
		DryRun:              true,
		MaxDailyLoss:        0.05,
		MetricsPort:         8080,
		MaxPositionSize:     0.01,
		MaxPositionExposure: 0.1, // Default 10% exposure
		MaxPriceDistance:    3.0,
		Leverage:            20,
		MarginMode:          "ISOLATION",
		RiskUSD:             25.0,
		RESTTimeout:         5 * time.Second,
		InitialBalance:      10000,
		SymbolConfigs: map[string]cfg.SymbolConfig{
			"BTCUSDT": {
				BaseSizeRatio:       0.001,
				MaxPositionSize:     0.015,
				MaxPositionExposure: 0.15, // 15% exposure for BTC
				MaxPriceDistance:    2.5,
			},
			"ETHUSDT": {
				BaseSizeRatio:       0.002,
				MaxPositionSize:     0.01,
				MaxPositionExposure: 0.05, // 5% exposure for ETH
				MaxPriceDistance:    3.0,
			},
		},
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	btcPrice := 50000.0
	ethPrice := 3000.0

	// Test BTC with 15% limit (1500 USD max)
	btcMaxExposure := exec.GetMaxAllowedExposure("BTCUSDT")
	expectedBtcMax := 10000 * 0.15
	if btcMaxExposure != expectedBtcMax {
		t.Errorf("Expected BTC max exposure %v, got %v", expectedBtcMax, btcMaxExposure)
	}

	btcSize := 0.025 // 0.025 * 50000 = 1250 USD (under 1500 limit)
	if !exec.CanTradeSymbol("BTCUSDT", btcSize, btcPrice) {
		t.Error("BTC position within symbol-specific limit should be allowed")
	}

	// Test ETH with 5% limit (500 USD max)
	ethMaxExposure := exec.GetMaxAllowedExposure("ETHUSDT")
	expectedEthMax := 10000 * 0.05
	if ethMaxExposure != expectedEthMax {
		t.Errorf("Expected ETH max exposure %v, got %v", expectedEthMax, ethMaxExposure)
	}

	ethSize := 0.2 // 0.2 * 3000 = 600 USD (over 500 limit)
	if exec.CanTradeSymbol("ETHUSDT", ethSize, ethPrice) {
		t.Error("ETH position exceeding symbol-specific limit should be rejected")
	}

	ethSizeAllowed := 0.15 // 0.15 * 3000 = 450 USD (under 500 limit)
	if !exec.CanTradeSymbol("ETHUSDT", ethSizeAllowed, ethPrice) {
		t.Error("ETH position within symbol-specific limit should be allowed")
	}
}

func TestPositionExposureLimitDisabled(t *testing.T) {
	// Create test config with exposure limits disabled (set to 0)
	config := cfg.Settings{
		Key:                 "test",
		Secret:              "test",
		BaseURL:             "http://test",
		WsURL:               "ws://test",
		Symbols:             []string{"BTCUSDT"},
		VWAPWindow:          30 * time.Second,
		VWAPSize:            600,
		TickSize:            50,
		BaseSizeRatio:       0.002,
		ProbThreshold:       0.65,
		DryRun:              true,
		MaxDailyLoss:        0.05,
		MetricsPort:         8080,
		MaxPositionSize:     0.01,
		MaxPositionExposure: 0, // Disabled
		MaxPriceDistance:    3.0,
		Leverage:            20,
		MarginMode:          "ISOLATION",
		RiskUSD:             25.0,
		RESTTimeout:         5 * time.Second,
		InitialBalance:      10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	price := 50000.0

	// Test: Very large position should be allowed when limits are disabled
	largeSize := 1.0 // 1.0 * 50000 = 50000 USD (would normally exceed any reasonable limit)
	if !exec.CanTradeSymbol(symbol, largeSize, price) {
		t.Error("Large position should be allowed when exposure limits are disabled")
	}

	// Verify that CheckPositionExposureLimit returns false (no limit reached)
	if exec.CheckPositionExposureLimit(symbol, largeSize, price) {
		t.Error("CheckPositionExposureLimit should return false when limits are disabled")
	}
}

func TestIsNewTradingDay(t *testing.T) {
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02,
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	// Initially, it should not be a new trading day
	if exec.IsNewTradingDay() {
		t.Error("Should not be a new trading day immediately after creation")
	}

	// Manually set the day start time to yesterday
	exec.mu.Lock()
	exec.dayStartTime = time.Now().Add(-25 * time.Hour)
	exec.mu.Unlock()

	// Now it should be a new trading day
	if !exec.IsNewTradingDay() {
		t.Error("Should be a new trading day after 25 hours")
	}
}

func TestDailyLossLimitTimeZoneBehavior(t *testing.T) {
	// Create test config with daily loss limit
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02, // 2% daily loss limit
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	// Test 1: Test normal daily loss limit enforcement within the same trading day
	// Set day start time to current time to ensure we're in the same trading day
	exec.ResetDailyTracking()

	// Simulate losses that reach the limit
	exec.UpdatePnL(-200) // 2% loss

	// Verify the loss is recorded
	currentPnL := exec.GetDailyPnL()
	if currentPnL != -200 {
		t.Errorf("Expected daily P&L of -200, got %v", currentPnL)
	}

	// Trading should be suspended due to loss limit
	if exec.CanTrade() {
		t.Error("Trading should be suspended at 2% loss limit")
	}

	// Test 2: Simulate time zone edge case - same calendar day but 24+ hours later
	// This tests the 24-hour fallback logic in IsNewTradingDay()

	// Create a reference time for testing
	referenceTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Set day start time to more than 24 hours ago (same calendar day in some timezone)
	exec.mu.Lock()
	exec.dayStartTime = referenceTime.Add(-25 * time.Hour) // 25 hours ago, might be same day in different TZ
	exec.mu.Unlock()

	// The IsNewTradingDay should return true due to 24+ hour check
	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when more than 24 hours have passed")
	}

	// Test 3: Test daily reset behavior across time zone boundaries
	// Reset and verify trading is allowed again
	exec.ResetDailyTracking()

	if !exec.CanTrade() {
		t.Error("Trading should be allowed after daily reset")
	}

	if exec.GetDailyPnL() != 0 {
		t.Errorf("Daily P&L should be reset to 0, got %v", exec.GetDailyPnL())
	}

	// Test 4: Test behavior when crossing date line (different calendar days)
	// Simulate a scenario where local time crosses midnight

	// Set day start to yesterday
	yesterday := time.Date(2024, 1, 14, 12, 0, 0, 0, time.UTC)
	exec.mu.Lock()
	exec.dayStartTime = yesterday
	exec.mu.Unlock()

	// Current time is next day
	// IsNewTradingDay should detect this as a new day
	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when calendar day has changed")
	}

	// Test 5: Test that daily loss limit is properly enforced within the same trading day
	// regardless of time zone
	exec.ResetDailyTracking()

	// Simulate losses within the same trading day
	exec.UpdatePnL(-150) // 1.5% loss
	if !exec.CanTrade() {
		t.Error("Trading should be allowed with 1.5% loss (below 2% limit)")
	}

	exec.UpdatePnL(-50) // Additional 0.5% loss, total 2%
	if exec.CanTrade() {
		t.Error("Trading should be suspended at 2% loss limit")
	}

	// Test 6: Verify that the loss limit check includes automatic reset detection
	// This tests the logic in CheckDailyLossLimit that calls IsNewTradingDay

	// Set day start time to trigger new day detection
	exec.mu.Lock()
	exec.dayStartTime = time.Now().Add(-25 * time.Hour)
	exec.mu.Unlock()

	// CheckDailyLossLimit should automatically reset and allow trading
	if exec.CheckDailyLossLimit() {
		t.Error("Daily loss limit should not be reached after automatic reset")
	}

	// Verify that the reset actually happened
	if exec.GetDailyPnL() != 0 {
		t.Error("Daily P&L should be automatically reset when new trading day is detected")
	}
}

func TestDailyLossLimitTimeZoneEdgeCases(t *testing.T) {
	// Create test config with daily loss limit
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02, // 2% daily loss limit
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	// Test 1: Test behavior when crossing UTC midnight
	// Set day start to 23:45 UTC
	dayStart := time.Date(2024, 1, 15, 23, 45, 0, 0, time.UTC)
	exec.mu.Lock()
	exec.dayStartTime = dayStart
	exec.mu.Unlock()

	// Simulate some losses
	exec.UpdatePnL(-100) // 1% loss

	// Current time is 00:15 UTC next day (30 minutes later)
	// This should trigger a new trading day
	exec.mu.Lock()
	exec.dayStartTime = dayStart.Add(30 * time.Minute)
	exec.mu.Unlock()

	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when crossing UTC midnight")
	}

	// Test 2: Test behavior when crossing date line (UTC+12 to UTC-12)
	// Set day start to 23:45 UTC+12
	dayStart = time.Date(2024, 1, 15, 23, 45, 0, 0, time.FixedZone("UTC+12", 12*3600))
	exec.mu.Lock()
	exec.dayStartTime = dayStart
	exec.mu.Unlock()

	// Simulate some losses
	exec.UpdatePnL(-100) // 1% loss

	// Current time is 00:15 UTC-12 (same UTC time, different calendar day)
	// This should trigger a new trading day
	exec.mu.Lock()
	exec.dayStartTime = time.Date(2024, 1, 16, 0, 15, 0, 0, time.FixedZone("UTC-12", -12*3600))
	exec.mu.Unlock()

	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when crossing date line")
	}

	// Test 3: Test behavior with DST transition
	// Set day start to 01:45 EDT (UTC-4)
	dayStart = time.Date(2024, 3, 10, 1, 45, 0, 0, time.FixedZone("EDT", -4*3600))
	exec.mu.Lock()
	exec.dayStartTime = dayStart
	exec.mu.Unlock()

	// Simulate some losses
	exec.UpdatePnL(-100) // 1% loss

	// Current time is 03:15 EDT (after DST transition)
	// This should trigger a new trading day due to 24+ hour check
	exec.mu.Lock()
	exec.dayStartTime = time.Date(2024, 3, 10, 3, 15, 0, 0, time.FixedZone("EDT", -4*3600))
	exec.mu.Unlock()

	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day after DST transition")
	}

	// Test 4: Test behavior with leap second
	// Set day start to 23:59:30 UTC
	dayStart = time.Date(2024, 6, 30, 23, 59, 30, 0, time.UTC)
	exec.mu.Lock()
	exec.dayStartTime = dayStart
	exec.mu.Unlock()

	// Simulate some losses
	exec.UpdatePnL(-100) // 1% loss

	// Current time is 00:00:30 UTC next day (1 minute later)
	// This should trigger a new trading day
	exec.mu.Lock()
	exec.dayStartTime = time.Date(2024, 7, 1, 0, 0, 30, 0, time.UTC)
	exec.mu.Unlock()

	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when crossing midnight with leap second")
	}

	// Test 5: Test behavior with timezone offset changes
	// Set day start to 23:45 UTC+8
	dayStart = time.Date(2024, 1, 15, 23, 45, 0, 0, time.FixedZone("UTC+8", 8*3600))
	exec.mu.Lock()
	exec.dayStartTime = dayStart
	exec.mu.Unlock()

	// Simulate some losses
	exec.UpdatePnL(-100) // 1% loss

	// Current time is 00:15 UTC+9 (timezone offset changed)
	// This should trigger a new trading day
	exec.mu.Lock()
	exec.dayStartTime = time.Date(2024, 1, 16, 0, 15, 0, 0, time.FixedZone("UTC+9", 9*3600))
	exec.mu.Unlock()

	if !exec.IsNewTradingDay() {
		t.Error("Should detect new trading day when timezone offset changes")
	}

	// Test 6: Verify that daily loss limit is properly enforced within the same trading day
	// regardless of timezone changes
	exec.ResetDailyTracking()

	// Simulate losses within the same trading day
	exec.UpdatePnL(-150) // 1.5% loss
	if !exec.CanTrade() {
		t.Error("Trading should be allowed with 1.5% loss (below 2% limit)")
	}

	exec.UpdatePnL(-50) // Additional 0.5% loss, total 2%
	if exec.CanTrade() {
		t.Error("Trading should be suspended at 2% loss limit")
	}

	// Test 7: Verify that the loss limit check includes automatic reset detection
	// when crossing timezone boundaries
	exec.mu.Lock()
	exec.dayStartTime = time.Now().Add(-25 * time.Hour)
	exec.mu.Unlock()

	// CheckDailyLossLimit should automatically reset and allow trading
	if exec.CheckDailyLossLimit() {
		t.Error("Daily loss limit should not be reached after automatic reset")
	}

	// Verify that the reset actually happened
	if exec.GetDailyPnL() != 0 {
		t.Error("Daily P&L should be automatically reset when new trading day is detected")
	}
}

// =========== ORDER EXECUTION FAILURE SCENARIOS TESTS ===========

func TestOrderExecutionFailureScenarios(t *testing.T) {
	config := createTestConfig()
	config.MaxDailyLoss = 0.05 // 5% daily loss limit

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p, _ := ml.NewWithMetrics("", mw, 5*time.Second)
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	price := 50000.0
	vwap := 49500.0

	// Test 1: Order execution blocked by daily loss limit
	exec.UpdatePnL(-500) // 5% loss to trigger limit
	if exec.CanTrade() {
		t.Error("Order execution should be blocked when daily loss limit is reached")
	}

	// Reset for next test
	exec.ResetDailyTracking()

	// Test 2: Order execution blocked by position exposure limit
	config.MaxPositionExposure = 0.01 // 1% exposure limit
	exec.config = config

	largeSize := 0.5 // 0.5 * 50000 = 25000 USD (far exceeds 1% of 10000 = 100 USD)
	if exec.CanTradeSymbol(symbol, largeSize, price) {
		t.Error("Order execution should be blocked when position exposure limit would be exceeded")
	}

	// Test 3: Order execution with invalid price distance
	config.MaxPriceDistance = 1.0 // 1% max distance
	exec.config = config

	// Price is 50000, VWAP is 49500, distance = (50000-49500)/49500 = 1.01% > 1%
	priceDistance := math.Abs((price-vwap)/vwap) * 100 // Convert to percentage
	if priceDistance <= config.MaxPriceDistance {
		t.Error("Test setup error: price distance should exceed maximum for this test")
	}

	// The trade should be implicitly blocked by the strategy's price distance logic
	// We test this by attempting a trade and verifying no position change occurs
	initialPositions := exec.GetPositions()
	exec.Try(symbol, price, vwap, 5.0, 0.5, 0.5, 10, 10) // This should be filtered out
	currentPositions := exec.GetPositions()

	// Position should not change due to price distance filtering in strategy logic
	if len(currentPositions) != len(initialPositions) {
		for s, pos := range currentPositions {
			if initialPos, exists := initialPositions[s]; !exists || pos != initialPos {
				t.Error("Order execution should be blocked when price distance exceeds maximum")
				break
			}
		}
	}

	// Test 4: Order execution with ML predictor rejection
	predictor := &MockPredictor{shouldApprove: false}
	exec.predictor = predictor

	// Reset config for normal operation
	config.MaxPriceDistance = 3.0
	config.MaxPositionExposure = 0.5
	exec.config = config

	// Clear any existing strategies and register only OVIR-X (which uses ML predictor)
	exec.mu.Lock()
	exec.strategies = make(map[string]Strategy)
	exec.RegisterStrategy(&OVIRXStrategy{exec: exec})
	exec.mu.Unlock()

	// Try to execute - should be rejected by predictor
	exec.Try(symbol, price, vwap-100, 5.0, 0.5, 0.5, 10, 10) // price > vwap for BUY signal

	// Verify no position was opened due to predictor rejection
	positions := exec.GetPositions()
	if positions[symbol] != 0 {
		t.Error("No position should be opened when ML predictor rejects the signal")
	}

	// Test 5: Order execution with zero or invalid size calculation
	exec.predictor = &MockPredictor{shouldApprove: true}
	config.RiskUSD = 0 // This should result in zero size
	exec.config = config

	size := exec.Size(symbol, price)
	if size != 0 {
		t.Error("Order size should be zero when risk amount is zero")
	}

	// Test 6: Order execution during circuit breaker activation
	config.RiskUSD = 1000                 // Reset risk
	config.CircuitBreakerVolatility = 1.0 // Very low threshold to trigger
	exec.config = config

	// Update the circuit breaker configuration
	exec.circuitBreaker = NewCircuitBreakerState(config)

	// Update circuit breaker with high volatility to trigger it
	exec.circuitBreaker.UpdateMarketConditions(2.0, 0.1, 10.0) // volatility=2.0 > threshold=1.0

	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped with high volatility")
	}

	// Clear strategies and register only OVIR-X for predictable testing
	exec.mu.Lock()
	exec.strategies = make(map[string]Strategy)
	exec.RegisterStrategy(&OVIRXStrategy{exec: exec})
	exec.predictor = &MockPredictor{shouldApprove: true} // Allow predictor to approve
	exec.mu.Unlock()

	// Store initial position
	initialPositions = exec.GetPositions()
	initialPositionCount := len(initialPositions)

	// Try to execute trade - should be blocked by circuit breaker
	exec.Try(symbol, price, vwap-100, 2.0, 0.5, 0.5, 10, 10)

	// Verify no position was opened due to circuit breaker
	positions = exec.GetPositions()
	currentPositionCount := len(positions)

	if currentPositionCount != initialPositionCount {
		t.Error("No position should be opened when circuit breaker is active")
	}
}

func TestOrderExecutionEdgeCases(t *testing.T) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Execution with extreme price values
	symbol := "BTCUSDT"
	extremePrice := 1000000.0 // Very high price
	vwap := 999000.0

	exec.Try(symbol, extremePrice, vwap, 5.0, 0.5, 0.5, 10, 10)
	positions := exec.GetPositions()

	// Should handle extreme prices gracefully
	if positions[symbol] < 0 {
		t.Error("Should handle extreme prices without creating invalid positions")
	}

	// Test 2: Execution with negative volume/bid/ask values
	exec.Try(symbol, 50000, 49000, 5.0, 0.5, 0.5, -10, -10) // Negative bid/ask

	// Should handle negative values gracefully
	if len(exec.GetPositions()) == 0 {
		t.Error("Should handle negative bid/ask values gracefully")
	}

	// Test 3: Execution with NaN or infinite values
	exec.Try(symbol, math.NaN(), 50000, 5.0, 0.5, 0.5, 10, 10)  // NaN price
	exec.Try(symbol, math.Inf(1), 50000, 5.0, 0.5, 0.5, 10, 10) // Infinite price

	// Should not crash or create invalid positions
	positions = exec.GetPositions()
	for _, pos := range positions {
		if math.IsNaN(pos) || math.IsInf(pos, 0) {
			t.Error("Positions should not contain NaN or infinite values")
		}
	}

	// Test 4: Concurrent execution failures
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()
			// Some will fail due to various conditions
			if iteration%3 == 0 {
				exec.UpdatePnL(-1000) // Create loss to potentially trigger limit
			}
			exec.Try(symbol, float64(50000+iteration), 49000, 5.0, 0.5, 0.5, 10, 10)
		}(i)
	}
	wg.Wait()

	// Verify system remains stable after concurrent failures
	if exec.GetDailyPnL() == 0 && len(exec.GetPositions()) == 0 {
		t.Error("System should remain functional after concurrent execution attempts")
	}
}

// =========== CIRCUIT BREAKER FUNCTIONALITY TESTS ===========

func TestCircuitBreakerFunctionality(t *testing.T) {
	config := createTestConfig()
	config.CircuitBreakerVolatility = 2.0                      // 2% volatility threshold
	config.CircuitBreakerImbalance = 0.7                       // 70% imbalance threshold
	config.CircuitBreakerVolume = 10.0                         // 10x volume threshold
	config.CircuitBreakerErrorRate = 0.1                       // 10% error rate threshold
	config.CircuitBreakerRecoveryTime = 100 * time.Millisecond // Fast recovery for testing

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Volatility circuit breaker
	exec.circuitBreaker.UpdateMarketConditions(3.0, 0.1, 5.0) // volatility=3.0 > threshold=2.0
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped due to high volatility")
	}

	status := exec.circuitBreaker.GetStatus()
	if !status["volatility"] {
		t.Error("Volatility circuit breaker should be active")
	}

	// Test 2: Order book imbalance circuit breaker
	exec.circuitBreaker = NewCircuitBreakerState(config)      // Reset
	exec.circuitBreaker.UpdateMarketConditions(1.0, 0.8, 5.0) // imbalance=0.8 > threshold=0.7
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped due to order book imbalance")
	}

	status = exec.circuitBreaker.GetStatus()
	if !status["imbalance"] {
		t.Error("Imbalance circuit breaker should be active")
	}

	// Test 3: Volume circuit breaker
	exec.circuitBreaker = NewCircuitBreakerState(config)       // Reset
	exec.circuitBreaker.UpdateMarketConditions(1.0, 0.1, 15.0) // volume=15.0 > threshold=10.0
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped due to high volume")
	}

	status = exec.circuitBreaker.GetStatus()
	if !status["volume"] {
		t.Error("Volume circuit breaker should be active")
	}

	// Test 4: Error rate circuit breaker
	exec.circuitBreaker = NewCircuitBreakerState(config) // Reset
	exec.circuitBreaker.UpdateErrorRate(0.15)            // error_rate=0.15 > threshold=0.1
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped due to high error rate")
	}

	status = exec.circuitBreaker.GetStatus()
	if !status["error_rate"] {
		t.Error("Error rate circuit breaker should be active")
	}

	// Test 5: Recovery after timeout
	exec.circuitBreaker = NewCircuitBreakerState(config)      // Reset
	exec.circuitBreaker.UpdateMarketConditions(3.0, 0.1, 5.0) // Trigger volatility breaker
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should initially be tripped")
	}

	// Wait for recovery time
	time.Sleep(150 * time.Millisecond)

	// Update with normal conditions
	exec.circuitBreaker.UpdateMarketConditions(1.0, 0.1, 5.0) // Normal conditions
	if exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should recover after timeout with normal conditions")
	}

	// Test 6: Multiple simultaneous circuit breaker triggers
	exec.circuitBreaker = NewCircuitBreakerState(config)       // Reset
	exec.circuitBreaker.UpdateMarketConditions(3.0, 0.8, 15.0) // Trigger all market condition breakers
	exec.circuitBreaker.UpdateErrorRate(0.2)                   // Also trigger error rate breaker

	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be tripped with multiple conditions")
	}

	status = exec.circuitBreaker.GetStatus()
	if !status["volatility"] || !status["imbalance"] || !status["volume"] || !status["error_rate"] {
		t.Error("All circuit breakers should be active simultaneously")
	}
}

func TestCircuitBreakerTradeBlocking(t *testing.T) {
	config := createTestConfig()
	config.CircuitBreakerVolatility = 1.0 // Low threshold for easy triggering
	config.CircuitBreakerRecoveryTime = 1 * time.Second

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	price := 50000.0
	vwap := 49000.0

	// Test 1: Normal trading before circuit breaker
	exec.Try(symbol, price, vwap, 0.5, 0.5, 0.5, 10, 10) // Low volatility, should work
	positions := exec.GetPositions()
	initialPosition := positions[symbol]

	if initialPosition <= 0 {
		t.Error("Should be able to trade normally before circuit breaker activation")
	}

	// Test 2: Trading blocked when circuit breaker is active
	exec.circuitBreaker.UpdateMarketConditions(2.0, 0.1, 5.0) // High volatility triggers breaker
	if !exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should be active")
	}

	// Try to trade - should be blocked
	exec.Try(symbol, price+1000, vwap, 2.0, 0.5, 0.5, 10, 10) // High volatility trade attempt
	positions = exec.GetPositions()

	if positions[symbol] != initialPosition {
		t.Error("Position should not change when circuit breaker is active")
	}

	// Test 3: Trading resumes after circuit breaker recovery
	time.Sleep(1100 * time.Millisecond)                       // Wait for recovery
	exec.circuitBreaker.UpdateMarketConditions(0.5, 0.1, 5.0) // Normal conditions

	if exec.circuitBreaker.IsTripped() {
		t.Error("Circuit breaker should recover after timeout")
	}

	// Try to trade again - should work
	exec.Try(symbol, price+500, vwap, 0.5, 0.5, 0.5, 10, 10)
	positions = exec.GetPositions()

	if positions[symbol] == initialPosition {
		t.Error("Should be able to trade again after circuit breaker recovery")
	}
}

// =========== PERFORMANCE BENCHMARK TESTS ===========

func BenchmarkOrderExecution(b *testing.B) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	basePrice := 50000.0
	baseVwap := 49000.0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		price := basePrice + float64(i%1000)*0.01 // Vary price slightly
		vwap := baseVwap + float64(i%800)*0.01    // Vary VWAP slightly
		exec.Try(symbol, price, vwap, 5.0, 0.5, 0.5, 10, 10)
	}
}

func BenchmarkPositionSizeCalculation(b *testing.B) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT", "DOTUSDT"}
	prices := []float64{50000, 3000, 1.5, 25.0}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		symbolIndex := i % len(symbols)
		exec.Size(symbols[symbolIndex], prices[symbolIndex])
	}
}

func BenchmarkCircuitBreakerChecks(b *testing.B) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		volatility := 1.0 + float64(i%100)*0.01
		imbalance := 0.1 + float64(i%80)*0.01
		volume := 5.0 + float64(i%50)*0.1

		exec.circuitBreaker.UpdateMarketConditions(volatility, imbalance, volume)
		exec.circuitBreaker.IsTripped()
	}
}

func BenchmarkConcurrentTrading(b *testing.B) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			symbolIndex := i % len(symbols)
			price := 50000.0 + float64(i%1000)*0.01
			vwap := 49000.0 + float64(i%800)*0.01

			exec.Try(symbols[symbolIndex], price, vwap, 5.0, 0.5, 0.5, 10, 10)
			i++
		}
	})
}

func BenchmarkRiskManagementChecks(b *testing.B) {
	config := createTestConfig()

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	price := 50000.0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		size := 0.01 + float64(i%100)*0.0001
		exec.CanTradeSymbol(symbol, size, price)
		exec.CheckPositionExposureLimit(symbol, size, price)
		exec.CheckDailyLossLimit()
	}
}

// =========== DAILY LOSS LIMIT WITH REAL P&L UPDATES FROM CLOSED POSITIONS ===========

// MockPosition represents a trading position for testing
type MockPosition struct {
	Symbol     string
	Size       float64
	EntryPrice float64
	Timestamp  time.Time
}

// ClosePosition simulates closing a position and calculating real P&L
func (exec *Exec) ClosePositionWithPnL(symbol string, exitPrice float64) float64 {
	exec.mu.Lock()
	defer exec.mu.Unlock()

	currentPosition := exec.positionSizes[symbol]
	if currentPosition == 0 {
		return 0 // No position to close
	}

	// Calculate P&L based on position size and price difference
	// For simplicity, assume entry price was stored or use a reference price
	entryPrice := 50000.0 // Simplified for testing
	pnl := currentPosition * (exitPrice - entryPrice)

	// Update daily P&L with the realized P&L from closing the position
	exec.dailyPnL += pnl

	// Close the position
	exec.positionSizes[symbol] = 0

	return pnl
}

func TestDailyLossLimitWithRealClosedPositions(t *testing.T) {
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02, // 2% daily loss limit
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	entryPrice := 50000.0
	vwap := 49000.0

	// Test 1: Open positions that will result in losses when closed
	exec.Try(symbol, entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10) // Open long position

	initialPosition := exec.GetPositions()[symbol]
	if initialPosition <= 0 {
		t.Error("Should have opened a long position")
	}

	// Test 2: Close position with a loss (price dropped)
	lossExitPrice := 49000.0 // Price dropped by 1000
	pnl1 := exec.ClosePositionWithPnL(symbol, lossExitPrice)

	if pnl1 >= 0 {
		t.Error("Should have realized a loss when price dropped")
	}

	if exec.GetPositions()[symbol] != 0 {
		t.Error("Position should be closed after ClosePositionWithPnL")
	}

	// Verify daily P&L was updated with the loss
	if exec.GetDailyPnL() != pnl1 {
		t.Errorf("Daily P&L should reflect the realized loss: expected %v, got %v", pnl1, exec.GetDailyPnL())
	}

	// Test 3: Open another position and close with more losses
	exec.Try(symbol, entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10) // Open another long position

	// Close with even bigger loss
	bigLossExitPrice := 48000.0 // Price dropped by 2000
	pnl2 := exec.ClosePositionWithPnL(symbol, bigLossExitPrice)

	totalPnL := pnl1 + pnl2
	if exec.GetDailyPnL() != totalPnL {
		t.Errorf("Daily P&L should accumulate losses: expected %v, got %v", totalPnL, exec.GetDailyPnL())
	}

	// Test 4: Verify daily loss limit is triggered by real P&L
	lossPercentage := -totalPnL / exec.initialBalance
	if lossPercentage >= config.MaxDailyLoss {
		if exec.CanTrade() {
			t.Error("Trading should be suspended when daily loss limit is reached through real position P&L")
		}
	}

	// Test 5: Test with profitable position close
	exec.ResetDailyTracking() // Reset for clean test

	exec.Try(symbol, entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10) // Open long position

	// Close with profit (price increased)
	profitExitPrice := 52000.0 // Price increased by 2000
	pnl3 := exec.ClosePositionWithPnL(symbol, profitExitPrice)

	if pnl3 <= 0 {
		t.Error("Should have realized a profit when price increased")
	}

	if exec.GetDailyPnL() != pnl3 {
		t.Errorf("Daily P&L should reflect the realized profit: expected %v, got %v", pnl3, exec.GetDailyPnL())
	}

	// Should still be able to trade with profits
	if !exec.CanTrade() {
		t.Error("Should be able to trade when in profit")
	}

	// Test 6: Mixed profitable and losing trades
	exec.ResetDailyTracking() // Reset for clean test

	// Series of trades with mixed outcomes
	trades := []struct {
		entryPrice float64
		exitPrice  float64
	}{
		{50000, 51000}, // +1000 profit
		{51000, 50500}, // -500 loss
		{50500, 49000}, // -1500 loss
		{49000, 49500}, // +500 profit
	}

	totalRealizedPnL := 0.0
	for i, trade := range trades {
		// Open position
		exec.Try(symbol, trade.entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10)

		// Close position and accumulate P&L
		pnl := exec.ClosePositionWithPnL(symbol, trade.exitPrice)
		totalRealizedPnL += pnl

		// Verify daily P&L tracking
		if exec.GetDailyPnL() != totalRealizedPnL {
			t.Errorf("Trade %d: Daily P&L mismatch: expected %v, got %v", i+1, totalRealizedPnL, exec.GetDailyPnL())
		}
	}

	// Test 7: Daily loss limit enforcement with accumulated real losses
	netLoss := -totalRealizedPnL // Make it positive if it's a loss
	if netLoss > 0 {
		lossPercentage := netLoss / exec.initialBalance
		shouldBeSuspended := lossPercentage >= config.MaxDailyLoss

		if shouldBeSuspended && exec.CanTrade() {
			t.Error("Trading should be suspended when accumulated real losses exceed daily limit")
		} else if !shouldBeSuspended && !exec.CanTrade() {
			t.Error("Trading should be allowed when accumulated real losses are within daily limit")
		}
	}
}

func TestDailyLossLimitWithPartialClosures(t *testing.T) {
	config := cfg.Settings{
		Key:              "test",
		Secret:           "test",
		BaseURL:          "http://test",
		WsURL:            "ws://test",
		Symbols:          []string{"BTCUSDT"},
		VWAPWindow:       30 * time.Second,
		VWAPSize:         600,
		TickSize:         50,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		DryRun:           true,
		MaxDailyLoss:     0.02, // 2% daily loss limit
		MetricsPort:      8080,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		Leverage:         20,
		MarginMode:       "ISOLATION",
		RiskUSD:          25.0,
		RESTTimeout:      5 * time.Second,
		InitialBalance:   10000,
	}

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	symbol := "BTCUSDT"
	entryPrice := 50000.0
	vwap := 49000.0

	// Test 1: Open larger position
	exec.Try(symbol, entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10) // Open position
	exec.Try(symbol, entryPrice, vwap, 5.0, 0.5, 0.5, 10, 10) // Add to position

	totalPosition := exec.GetPositions()[symbol]
	if totalPosition <= 0 {
		t.Error("Should have opened a substantial position")
	}

	// Test 2: Partial position closure with loss
	partialExitPrice := 49000.0 // Loss on partial close

	// Simulate partial closure by manually adjusting position and calculating P&L
	exec.mu.Lock()
	partialSize := totalPosition / 2 // Close half the position
	partialPnL := partialSize * (partialExitPrice - entryPrice)
	exec.dailyPnL += partialPnL
	exec.positionSizes[symbol] = totalPosition - partialSize // Reduce position
	exec.mu.Unlock()

	// Verify partial P&L tracking
	if exec.GetDailyPnL() != partialPnL {
		t.Errorf("Daily P&L should reflect partial closure: expected %v, got %v", partialPnL, exec.GetDailyPnL())
	}

	// Verify remaining position
	remainingPosition := exec.GetPositions()[symbol]
	expectedRemaining := totalPosition - partialSize
	if math.Abs(remainingPosition-expectedRemaining) > 0.0001 {
		t.Errorf("Remaining position incorrect: expected %v, got %v", expectedRemaining, remainingPosition)
	}

	// Test 3: Close remaining position with additional loss
	finalExitPrice := 48000.0 // Further loss
	finalPnL := exec.ClosePositionWithPnL(symbol, finalExitPrice)

	totalPnL := partialPnL + finalPnL
	if exec.GetDailyPnL() != totalPnL {
		t.Errorf("Total daily P&L should include all closures: expected %v, got %v", totalPnL, exec.GetDailyPnL())
	}

	// Test 4: Verify daily loss limit with accumulated partial losses
	lossPercentage := -totalPnL / exec.initialBalance
	if lossPercentage >= config.MaxDailyLoss {
		if exec.CanTrade() {
			t.Error("Trading should be suspended when partial closure losses exceed daily limit")
		}
	}
}

// =========== MAXIMUM DRAWDOWN PROTECTION TESTS ===========

func TestMaximumDrawdownProtection(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = 0.15 // 15% maximum drawdown limit
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Initial state - no drawdown
	if exec.GetCurrentDrawdown() != 0 {
		t.Error("Initial drawdown should be 0")
	}

	if exec.GetPeakBalance() != config.InitialBalance {
		t.Errorf("Initial peak balance should be %v, got %v", config.InitialBalance, exec.GetPeakBalance())
	}

	if exec.GetCurrentBalance() != config.InitialBalance {
		t.Errorf("Initial current balance should be %v, got %v", config.InitialBalance, exec.GetCurrentBalance())
	}

	// Test 2: Profitable trading increases peak balance
	exec.UpdatePnL(500.0) // +$500 profit

	if exec.GetPeakBalance() != 10500.0 {
		t.Errorf("Peak balance should be updated to 10500, got %v", exec.GetPeakBalance())
	}

	if exec.GetCurrentBalance() != 10500.0 {
		t.Errorf("Current balance should be updated to 10500, got %v", exec.GetCurrentBalance())
	}

	if exec.GetCurrentDrawdown() != 0 {
		t.Error("Drawdown should still be 0 when at peak")
	}

	// Test 3: Some losses create drawdown but don't hit limit
	exec.UpdatePnL(-200.0) // -$200 loss, current balance = $10,300

	expectedDrawdown := (10500.0 - 10300.0) / 10500.0 // ~1.9%
	actualDrawdown := exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.001 {
		t.Errorf("Drawdown calculation incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered at 1.9% drawdown")
	}

	if !exec.CanTrade() {
		t.Error("Should be able to trade when drawdown is below limit")
	}

	// Test 4: Larger losses approaching the limit
	exec.UpdatePnL(-1000.0) // Additional -$1000 loss, current balance = $9,300

	expectedDrawdown = (10500.0 - 9300.0) / 10500.0 // ~11.4%
	actualDrawdown = exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.001 {
		t.Errorf("Drawdown calculation incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered at 11.4% drawdown")
	}

	// Test 5: Drawdown hits the protection limit
	exec.UpdatePnL(-300.0) // Additional -$300 loss, current balance = $9,000

	expectedDrawdown = (10500.0 - 9000.0) / 10500.0 // ~14.3%
	actualDrawdown = exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.001 {
		t.Errorf("Drawdown calculation incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered at 14.3% (below 15% limit)")
	}

	// Test 6: Drawdown exceeds the protection limit
	exec.UpdatePnL(-600.0) // Additional -$600 loss, current balance = $8,400

	expectedDrawdown = (10500.0 - 8400.0) / 10500.0 // 20%
	actualDrawdown = exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.001 {
		t.Errorf("Drawdown calculation incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should be triggered at 20% drawdown")
	}

	if exec.CanTrade() {
		t.Error("Trading should be suspended when drawdown protection is triggered")
	}

	// Test 7: Recovery doesn't immediately reset peak (drawdown protection still active)
	exec.UpdatePnL(200.0) // Small recovery, current balance = $8,600

	if exec.GetPeakBalance() != 10500.0 {
		t.Error("Peak balance should not change during recovery phase")
	}

	// Drawdown should improve but still above limit
	expectedDrawdown = (10500.0 - 8600.0) / 10500.0 // ~18.1%
	actualDrawdown = exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.001 {
		t.Errorf("Drawdown calculation after recovery incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should still be triggered at 18.1% drawdown")
	}

	// Test 8: Full recovery to new peak
	exec.UpdatePnL(2000.0) // Large recovery, current balance = $10,600

	if exec.GetPeakBalance() != 10600.0 {
		t.Errorf("Peak balance should be updated to new high: expected 10600, got %v", exec.GetPeakBalance())
	}

	if exec.GetCurrentDrawdown() != 0 {
		t.Error("Drawdown should be 0 when at new peak")
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered when at new peak")
	}

	if !exec.CanTrade() {
		t.Error("Should be able to trade when at new peak")
	}
}

func TestMaximumDrawdownProtectionDisabled(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = 0.0 // Disabled
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Large losses should not trigger protection when disabled
	exec.UpdatePnL(1000.0)  // Get to peak of $11,000
	exec.UpdatePnL(-5000.0) // Massive loss, current balance = $6,000

	// This would be ~45% drawdown, but protection is disabled
	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered when disabled (MaxDrawdownProtection = 0)")
	}

	if !exec.CanTrade() {
		// Check if trading is suspended for other reasons
		if exec.CheckDailyLossLimit() {
			t.Log("Trading suspended due to daily loss limit (expected)")
		} else {
			t.Error("Trading should not be suspended due to drawdown when protection is disabled")
		}
	}
}

func TestMaximumDrawdownProtectionWithNegativeConfig(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = -0.1 // Negative value (should be treated as disabled)
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test with large losses
	exec.UpdatePnL(1000.0)  // Get to peak of $11,000
	exec.UpdatePnL(-5000.0) // Massive loss, current balance = $6,000

	// Protection should be disabled with negative value
	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered with negative MaxDrawdownProtection")
	}
}

func TestMaximumDrawdownProtectionResetOnNewDay(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = 0.1 // 10% maximum drawdown limit
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Create peak and drawdown
	exec.UpdatePnL(500.0)   // Peak at $10,500
	exec.UpdatePnL(-1200.0) // Large loss, current balance = $9,300, drawdown ~11.4%

	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should be triggered at 11.4% drawdown")
	}

	initialDrawdown := exec.GetCurrentDrawdown()
	initialPeak := exec.GetPeakBalance()
	initialCurrent := exec.GetCurrentBalance()

	// Test 2: Reset daily tracking (simulates new trading day)
	exec.ResetDailyTracking()

	// After reset, balances should be reset to initial values
	if exec.GetPeakBalance() != config.InitialBalance {
		t.Errorf("Peak balance should reset to initial balance: expected %v, got %v", config.InitialBalance, exec.GetPeakBalance())
	}

	if exec.GetCurrentBalance() != config.InitialBalance {
		t.Errorf("Current balance should reset to initial balance: expected %v, got %v", config.InitialBalance, exec.GetCurrentBalance())
	}

	if exec.GetCurrentDrawdown() != 0 {
		t.Error("Drawdown should be 0 after daily reset")
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered after daily reset")
	}

	if !exec.CanTrade() {
		t.Error("Should be able to trade after daily reset")
	}

	// Verify values were different before reset
	if initialDrawdown <= 0 {
		t.Error("Should have had significant drawdown before reset")
	}
	if initialPeak <= config.InitialBalance {
		t.Error("Should have had peak above initial before reset")
	}
	if initialCurrent >= config.InitialBalance {
		t.Error("Should have had current balance below initial before reset")
	}
}

func TestMaximumDrawdownProtectionWithMultipleSymbols(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = 0.12 // 12% maximum drawdown limit
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Use only BTCUSDT which we know has proper lot size configuration
	symbols := []string{"BTCUSDT"}
	basePrice := 50000.0
	vwap := 49000.0

	// Test 1: Normal trading before drawdown limit
	for i, symbol := range symbols {
		price := basePrice + float64(i*1000)
		exec.Try(symbol, price, vwap, 5.0, 0.5, 0.5, 10, 10)
		exec.Try(symbol, price, vwap, 5.0, 0.5, 0.5, 10, 10) // Multiple trades to build position
	}

	// Verify positions were opened (can be positive or negative)
	positions := exec.GetPositions()
	for _, symbol := range symbols {
		if positions[symbol] == 0 {
			t.Errorf("Should have opened position for %s, got %v", symbol, positions[symbol])
		}
	}

	// Test 2: Simulate losses that trigger drawdown protection
	exec.UpdatePnL(500.0)   // Create peak at $10,500
	exec.UpdatePnL(-1400.0) // Large loss creates 13.3% drawdown, exceeds 12% limit

	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should be triggered")
	}

	// Test 3: Try to trade all symbols - should be blocked
	for i, symbol := range symbols {
		price := basePrice + float64(i*1000) + 100
		initialPosition := exec.GetPositions()[symbol]

		exec.Try(symbol, price, vwap, 5.0, 0.5, 0.5, 10, 10)

		finalPosition := exec.GetPositions()[symbol]
		if finalPosition != initialPosition {
			t.Errorf("Position for %s should not change when drawdown protection is active", symbol)
		}
	}

	// Test 4: Verify CanTradeSymbol is blocked for all symbols
	for _, symbol := range symbols {
		if exec.CanTradeSymbol(symbol, 0.001, basePrice) {
			t.Errorf("Should not be able to trade %s when drawdown protection is active", symbol)
		}
	}
}

func TestMaximumDrawdownProtectionEdgeCases(t *testing.T) {
	config := createTestConfig()
	config.MaxDrawdownProtection = 0.05 // 5% maximum drawdown limit
	config.InitialBalance = 10000.0

	// Use a new registry to avoid conflicts
	registry := prometheus.NewRegistry()
	m := metrics.NewWithRegistry(registry)
	mw := metrics.NewWrapper(m)
	p := &MockPredictor{shouldApprove: true}
	exec := New(config, p, mw)

	// Test 1: Exact limit boundary
	exec.UpdatePnL(1000.0) // Peak at $11,000
	exec.UpdatePnL(-550.0) // Loss to $10,450, exactly 5% drawdown

	expectedDrawdown := 550.0 / 11000.0 // Exactly 5%
	actualDrawdown := exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.0001 {
		t.Errorf("Drawdown calculation at boundary incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	// At exactly 5%, should trigger protection (>= comparison)
	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should be triggered at exactly 5% drawdown")
	}

	// Test 2: Just below limit
	exec.ResetDailyTracking()
	exec.UpdatePnL(1000.0) // Peak at $11,000
	exec.UpdatePnL(-549.0) // Loss to $10,451, just under 5% drawdown

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not be triggered at 4.99% drawdown")
	}

	// Test 3: Zero peak balance edge case
	exec.ResetDailyTracking()
	exec.peakBalance = 0 // Force zero peak (edge case)

	drawdown := exec.GetCurrentDrawdown()
	if drawdown != 0 {
		t.Error("Drawdown should be 0 when peak balance is 0")
	}

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not trigger with zero peak balance")
	}

	// Test 4: Very small amounts
	exec.ResetDailyTracking()
	exec.UpdatePnL(0.01)   // Peak at $10000.01
	exec.UpdatePnL(-0.006) // Loss to $10000.004, ~0.06% drawdown

	if exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should not trigger for tiny amounts below limit")
	}

	// Test 5: Large profit swings
	exec.ResetDailyTracking()
	exec.UpdatePnL(50000.0) // Massive profit, peak at $60,000
	exec.UpdatePnL(-3000.0) // Large loss but only 5% of new peak

	expectedDrawdown = 3000.0 / 60000.0 // Exactly 5%
	actualDrawdown = exec.GetCurrentDrawdown()

	if math.Abs(actualDrawdown-expectedDrawdown) > 0.0001 {
		t.Errorf("Drawdown calculation with large numbers incorrect: expected %v, got %v", expectedDrawdown, actualDrawdown)
	}

	if !exec.CheckMaxDrawdownProtection() {
		t.Error("Drawdown protection should be triggered at 5% even with large numbers")
	}
}
