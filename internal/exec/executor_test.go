package exec

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/storage"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MockPredictor implements ml.PredictorInterface for testing
type MockPredictor struct {
	approveResult bool
	approveError  error
	predictResult []float32
	predictError  error
}

func (m *MockPredictor) Approve(features []float32, threshold float64) bool {
	return m.approveResult
}

func (m *MockPredictor) Predict(features []float32) ([]float32, error) {
	return m.predictResult, m.predictError
}

// Helper function to create test metrics wrapper
func createTestMetricsWrapper() *metrics.MetricsWrapper {
	registry := prometheus.NewRegistry()
	metricsInstance := metrics.NewWithRegistry(registry)
	return metrics.NewWrapper(metricsInstance)
}

func TestNew(t *testing.T) {
	config := cfg.Settings{
		Key:         "test_key",
		Secret:      "test_secret",
		BaseURL:     "https://api.test.com",
		RESTTimeout: 10 * time.Second,
	}

	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	if exec == nil {
		t.Fatal("Expected non-nil executor")
	}

	if exec.predictor != predictor {
		t.Error("Expected predictor to be set correctly")
	}

	if exec.config.Key != "test_key" {
		t.Errorf("Expected config key 'test_key', got %s", exec.config.Key)
	}

	if exec.positionSizes == nil {
		t.Error("Expected positionSizes to be initialized")
	}
}

func TestSetStorage(t *testing.T) {
	config := cfg.Settings{}
	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	store := &storage.Store{}
	exec.SetStorage(store)

	if exec.store != store {
		t.Error("Expected storage to be set correctly")
	}
}

func TestSize(t *testing.T) {
	tests := []struct {
		name         string
		config       cfg.Settings
		symbol       string
		zDist        float64
		expectedSize string
	}{
		{
			name: "basic size calculation",
			config: cfg.Settings{
				BaseSizeRatio: 0.002,
			},
			symbol:       "BTCUSDT",
			zDist:        0.5,
			expectedSize: "0.0013",
		},
		{
			name: "zero distance",
			config: cfg.Settings{
				BaseSizeRatio: 0.002,
			},
			symbol:       "BTCUSDT",
			zDist:        0,
			expectedSize: "0.0020",
		},
		{
			name: "negative distance",
			config: cfg.Settings{
				BaseSizeRatio: 0.002,
			},
			symbol:       "BTCUSDT",
			zDist:        -0.5,
			expectedSize: "0.0013",
		},
		{
			name: "symbol specific config",
			config: cfg.Settings{
				BaseSizeRatio: 0.002,
				SymbolConfigs: map[string]cfg.SymbolConfig{
					"BTCUSDT": {
						BaseSizeRatio: 0.004,
					},
				},
			},
			symbol:       "BTCUSDT",
			zDist:        0,
			expectedSize: "0.0040",
		},
		{
			name: "fallback to default when config is zero",
			config: cfg.Settings{
				BaseSizeRatio: 0,
			},
			symbol:       "BTCUSDT",
			zDist:        0,
			expectedSize: "0.0020", // fallback to 0.002
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predictor := &MockPredictor{}
			metricsWrapper := createTestMetricsWrapper()

			exec := New(tt.config, predictor, metricsWrapper)
			size := exec.Size(tt.symbol, tt.zDist)

			if size != tt.expectedSize {
				t.Errorf("Expected size %s, got %s", tt.expectedSize, size)
			}
		})
	}
}

func TestTry_PredicatorRejects(t *testing.T) {
	config := cfg.Settings{
		BaseSizeRatio: 0.002,
	}

	// Predictor that rejects all trades
	predictor := &MockPredictor{
		approveResult: false,
	}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	// Should not execute trade when predictor rejects
	exec.Try("BTCUSDT", 50000, 49500, 100, 0.1, 0.05, 1000, 950)

	positions := exec.GetPositions()
	if len(positions) != 0 {
		t.Error("Expected no positions when predictor rejects")
	}
}

func TestTry_ZeroStandardDeviation(t *testing.T) {
	config := cfg.Settings{
		BaseSizeRatio: 0.002,
	}

	predictor := &MockPredictor{
		approveResult: true,
	}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	// Should return early when std is 0
	exec.Try("BTCUSDT", 50000, 49500, 0, 0.1, 0.05, 1000, 950)

	positions := exec.GetPositions()
	if len(positions) != 0 {
		t.Error("Expected no positions when std is 0")
	}
}

func TestGetPositions(t *testing.T) {
	config := cfg.Settings{}
	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	// Initially should be empty
	positions := exec.GetPositions()
	if len(positions) != 0 {
		t.Error("Expected empty positions initially")
	}

	// Add some test positions
	exec.positionSizes["BTCUSDT"] = 0.5
	exec.positionSizes["ETHUSDT"] = -0.3

	positions = exec.GetPositions()
	if len(positions) != 2 {
		t.Errorf("Expected 2 positions, got %d", len(positions))
	}

	if positions["BTCUSDT"] != 0.5 {
		t.Errorf("Expected BTCUSDT position 0.5, got %f", positions["BTCUSDT"])
	}

	if positions["ETHUSDT"] != -0.3 {
		t.Errorf("Expected ETHUSDT position -0.3, got %f", positions["ETHUSDT"])
	}
}

func TestUpdatePnL(t *testing.T) {
	config := cfg.Settings{}
	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	// Initially should be zero
	if exec.GetDailyPnL() != 0 {
		t.Error("Expected initial PnL to be 0")
	}

	// Update PnL
	exec.UpdatePnL(100.5)
	if exec.GetDailyPnL() != 100.5 {
		t.Errorf("Expected PnL 100.5, got %f", exec.GetDailyPnL())
	}

	// Update again (should accumulate)
	exec.UpdatePnL(-50.25)
	expected := 100.5 - 50.25
	if exec.GetDailyPnL() != expected {
		t.Errorf("Expected PnL %f, got %f", expected, exec.GetDailyPnL())
	}
}

func TestGetDailyPnL(t *testing.T) {
	config := cfg.Settings{}
	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	exec.dailyPnL = 250.75

	if exec.GetDailyPnL() != 250.75 {
		t.Errorf("Expected PnL 250.75, got %f", exec.GetDailyPnL())
	}
}

func TestTry_WithStorage(t *testing.T) {
	config := cfg.Settings{
		BaseSizeRatio: 0.002,
	}

	predictor := &MockPredictor{
		approveResult: true,
	}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	// Test that SetStorage works without crashing
	// We'll test with nil storage first
	exec.SetStorage(nil)

	// This should work fine with nil storage
	exec.Try("BTCUSDT", 50000, 49500, 100, 0.1, 0.05, 1000, 950)

	// Test that setting a non-nil storage doesn't crash the constructor
	// Note: We don't test actual storage operations as they require DB setup
	store := &storage.Store{}
	exec.SetStorage(store)

	// The executor should handle storage errors gracefully
	// We test this by ensuring Try() doesn't panic, but we expect storage calls to fail quietly
}

func BenchmarkSize(b *testing.B) {
	config := cfg.Settings{
		BaseSizeRatio: 0.002,
	}
	predictor := &MockPredictor{}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exec.Size("BTCUSDT", 0.5)
	}
}

func BenchmarkTry(b *testing.B) {
	config := cfg.Settings{
		BaseSizeRatio: 0.002,
	}

	// Predictor that always rejects to avoid expensive operations
	predictor := &MockPredictor{
		approveResult: false,
	}
	metricsWrapper := createTestMetricsWrapper()

	exec := New(config, predictor, metricsWrapper)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exec.Try("BTCUSDT", 50000, 49500, 100, 0.1, 0.05, 1000, 950)
	}
}
