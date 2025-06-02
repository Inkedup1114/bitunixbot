package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewWrapper_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	if wrapper == nil {
		t.Fatal("NewWrapper returned nil")
	}
	if wrapper.m != metrics {
		t.Error("Wrapper does not contain the expected metrics instance")
	}
}

func TestMetricsWrapper_CounterOperations_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	ordersCounter := wrapper.OrdersTotal()
	if ordersCounter == nil {
		t.Fatal("OrdersTotal returned nil counter")
	}

	// Initial value should be 0
	initialValue := testutil.ToFloat64(metrics.OrdersTotal)
	if initialValue != 0 {
		t.Errorf("Expected counter initial value 0, got %f", initialValue)
	}

	// Increment and check
	ordersCounter.Inc()
	newValue := testutil.ToFloat64(metrics.OrdersTotal)
	if newValue != 1 {
		t.Errorf("Expected counter value 1 after increment, got %f", newValue)
	}
}

func TestMetricsWrapper_GaugeOperations_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	pnlGauge := wrapper.PnLTotal()
	if pnlGauge == nil {
		t.Fatal("PnLTotal returned nil gauge")
	}

	// Set initial value
	pnlGauge.Set(100.5)
	value := testutil.ToFloat64(metrics.PnLTotal)
	if value != 100.5 {
		t.Errorf("Expected gauge value 100.5, got %f", value)
	}

	// Add to the value
	pnlGauge.Add(50.0)
	newValue := testutil.ToFloat64(metrics.PnLTotal)
	if newValue != 150.5 {
		t.Errorf("Expected gauge value 150.5 after add, got %f", newValue)
	}
}

func TestMetricsWrapper_HistogramOperations_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	onnxHistogram := wrapper.ONNXLatency()
	if onnxHistogram == nil {
		t.Fatal("ONNXLatency returned nil histogram")
	}

	// Observe some values - this verifies the histogram is working
	onnxHistogram.Observe(0.1)
	onnxHistogram.Observe(0.2)
	onnxHistogram.Observe(0.3)

	// Histogram is functioning if we can observe values without panic
	t.Log("Histogram operations completed successfully")
}

func TestMetricsWrapper_MLMethods_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	// Test ML prediction methods
	wrapper.MLPredictionsInc()
	predCount := testutil.ToFloat64(metrics.MLPredictions)
	if predCount != 1 {
		t.Errorf("Expected ML predictions count 1, got %f", predCount)
	}

	wrapper.MLFailuresInc()
	failCount := testutil.ToFloat64(metrics.MLFailures)
	if failCount != 1 {
		t.Errorf("Expected ML failures count 1, got %f", failCount)
	}

	// Test histogram - just verify it doesn't panic
	wrapper.MLLatencyObserve(0.5)
	t.Log("ML latency observation completed successfully")

	wrapper.MLModelAgeSet(3600)
	modelAge := testutil.ToFloat64(metrics.MLModelAge)
	if modelAge != 3600 {
		t.Errorf("Expected ML model age 3600, got %f", modelAge)
	}

	wrapper.FeatureErrorsInc()
	featureErrCount := testutil.ToFloat64(metrics.FeatureErrors)
	if featureErrCount != 1 {
		t.Errorf("Expected feature errors count 1, got %f", featureErrCount)
	}
}

func TestMetricsWrapper_EnhancedMLMethods_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	// Test enhanced ML metrics - histograms just verify they don't panic
	wrapper.MLAccuracyObserve(0.85)
	t.Log("ML accuracy observation completed successfully")

	wrapper.MLPredictionScoresObserve(0.92)
	t.Log("ML prediction scores observation completed successfully")

	wrapper.MLTimeoutsInc()
	timeoutCount := testutil.ToFloat64(metrics.MLTimeouts)
	if timeoutCount != 1 {
		t.Errorf("Expected ML timeouts count 1, got %f", timeoutCount)
	}

	wrapper.MLFallbackUseInc()
	fallbackCount := testutil.ToFloat64(metrics.MLFallbackUse)
	if fallbackCount != 1 {
		t.Errorf("Expected ML fallback use count 1, got %f", fallbackCount)
	}
}

func TestMetricsWrapper_UpdatePositions_Clean(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	positions := map[string]float64{
		"BTCUSDT": 0.5,
		"ETHUSDT": 1.2,
	}

	// Test updating positions (this should not panic)
	wrapper.UpdatePositions(positions)

	// Check that active positions gauge was updated
	activePos := testutil.ToFloat64(metrics.ActivePositions)
	if activePos != 2 {
		t.Errorf("Expected active positions 2, got %f", activePos)
	}
}
