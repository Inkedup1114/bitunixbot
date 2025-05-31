package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewWrapper(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	if wrapper == nil {
		t.Fatal("NewWrapper returned nil")
	}
	if wrapper.m != metrics {
		t.Error("Wrapper does not contain correct metrics instance")
	}
}

func TestMetricsWrapper_CounterOperations(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewWithRegistry(registry)
	wrapper := NewWrapper(metrics)

	// Test OrdersTotal counter
	ordersCounter := wrapper.OrdersTotal()
	if ordersCounter == nil {
		t.Fatal("OrdersTotal returned nil counter")
	}

	// Initial value should be 0
	initialValue := testutil.ToFloat64(metrics.OrdersTotal)
	if initialValue != 0 {
		t.Errorf("Expected initial counter value 0, got %f", initialValue)
	}

	// Increment counter
	ordersCounter.Inc()
	newValue := testutil.ToFloat64(metrics.OrdersTotal)
	if newValue != 1 {
		t.Errorf("Expected counter value 1 after increment, got %f", newValue)
	}

	// Increment again
	ordersCounter.Inc()
	finalValue := testutil.ToFloat64(metrics.OrdersTotal)
	if finalValue != 2 {
		t.Errorf("Expected counter value 2 after second increment, got %f", finalValue)
	}
}

func TestMetricsWrapper_GaugeOperations(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	pnlGauge := wrapper.PnLTotal()
	if pnlGauge == nil {
		t.Fatal("PnLTotal returned nil gauge")
	}

	// Test Set operation
	pnlGauge.Set(123.45)
	value := testutil.ToFloat64(metrics.PnLTotal)
	if value != 123.45 {
		t.Errorf("Expected gauge value 123.45, got %f", value)
	}

	// Test Add operation
	pnlGauge.Add(10.55)
	newValue := testutil.ToFloat64(metrics.PnLTotal)
	expected := 123.45 + 10.55
	if newValue != expected {
		t.Errorf("Expected gauge value %f after add, got %f", expected, newValue)
	}

	// Test negative add
	pnlGauge.Add(-20.0)
	finalValue := testutil.ToFloat64(metrics.PnLTotal)
	expected = 123.45 + 10.55 - 20.0
	if finalValue != expected {
		t.Errorf("Expected gauge value %f after negative add, got %f", expected, finalValue)
	}
}

func TestMetricsWrapper_HistogramOperations(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	latencyHist := wrapper.ONNXLatency()
	if latencyHist == nil {
		t.Fatal("ONNXLatency returned nil histogram")
	}

	// Observe some values
	testValues := []float64{0.001, 0.005, 0.01, 0.05, 0.1}
	for _, value := range testValues {
		latencyHist.Observe(value)
	}

	// Check that observations were recorded
	count := testutil.ToFloat64(metrics.ONNXLatency)
	if count != float64(len(testValues)) {
		t.Errorf("Expected %d observations, got %f", len(testValues), count)
	}
}

func TestMetricsWrapper_UpdatePositions(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	positions := map[string]float64{
		"BTCUSDT": 0.5,
		"ETHUSDT": -0.3,
		"ADAUSDT": 0.0,
	}

	// This should not panic
	wrapper.UpdatePositions(positions)

	// Check active positions count
	activeCount := testutil.ToFloat64(metrics.ActivePositions)
	expected := 2.0 // Only non-zero positions
	if activeCount != expected {
		t.Errorf("Expected %f active positions, got %f", expected, activeCount)
	}
}

func TestMetricsWrapper_MLMethods(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	// Test ML counter methods
	wrapper.MLPredictionsInc()
	predictions := testutil.ToFloat64(metrics.MLPredictions)
	if predictions != 1 {
		t.Errorf("Expected 1 ML prediction, got %f", predictions)
	}

	wrapper.MLFailuresInc()
	failures := testutil.ToFloat64(metrics.MLFailures)
	if failures != 1 {
		t.Errorf("Expected 1 ML failure, got %f", failures)
	}

	wrapper.MLTimeoutsInc()
	timeouts := testutil.ToFloat64(metrics.MLTimeouts)
	if timeouts != 1 {
		t.Errorf("Expected 1 ML timeout, got %f", timeouts)
	}

	wrapper.MLFallbackUseInc()
	fallbacks := testutil.ToFloat64(metrics.MLFallbackUse)
	if fallbacks != 1 {
		t.Errorf("Expected 1 ML fallback use, got %f", fallbacks)
	}

	wrapper.FeatureErrorsInc()
	featureErrors := testutil.ToFloat64(metrics.FeatureErrors)
	if featureErrors != 1 {
		t.Errorf("Expected 1 feature error, got %f", featureErrors)
	}

	// Test ML gauge methods
	wrapper.MLModelAgeSet(3600.0) // 1 hour in seconds
	modelAge := testutil.ToFloat64(metrics.MLModelAge)
	if modelAge != 3600.0 {
		t.Errorf("Expected model age 3600.0, got %f", modelAge)
	}

	// Test ML histogram methods
	wrapper.MLLatencyObserve(0.25)
	wrapper.MLAccuracyObserve(0.85)
	wrapper.MLPredictionScoresObserve(0.75)

	// These should not panic and should record observations
	// Exact values are hard to test without accessing histogram internals
}

func TestMetricsWrapper_MultipleIncrement(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	// Increment ML predictions multiple times
	numIncrements := 10
	for i := 0; i < numIncrements; i++ {
		wrapper.MLPredictionsInc()
	}

	predictions := testutil.ToFloat64(metrics.MLPredictions)
	if predictions != float64(numIncrements) {
		t.Errorf("Expected %d ML predictions, got %f", numIncrements, predictions)
	}
}

func TestCounterWrapper_DirectUsage(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter for unit tests",
	})

	wrapper := &CounterWrapper{c: counter}

	// Test increment
	wrapper.Inc()
	value := testutil.ToFloat64(counter)
	if value != 1 {
		t.Errorf("Expected counter value 1, got %f", value)
	}
}

func TestGaugeWrapper_DirectUsage(t *testing.T) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge",
		Help: "Test gauge for unit tests",
	})

	wrapper := &GaugeWrapper{g: gauge}

	// Test set
	wrapper.Set(42.0)
	value := testutil.ToFloat64(gauge)
	if value != 42.0 {
		t.Errorf("Expected gauge value 42.0, got %f", value)
	}

	// Test add
	wrapper.Add(8.0)
	newValue := testutil.ToFloat64(gauge)
	if newValue != 50.0 {
		t.Errorf("Expected gauge value 50.0 after add, got %f", newValue)
	}
}

func TestHistogramWrapper_DirectUsage(t *testing.T) {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_histogram",
		Help:    "Test histogram for unit tests",
		Buckets: prometheus.DefBuckets,
	})

	wrapper := &HistogramWrapper{h: histogram}

	// Test observe
	wrapper.Observe(0.5)
	// Note: Hard to test exact histogram values without diving into internals
	// The main test is that it doesn't panic
}

func TestMetricsWrapper_ConcurrentAccess(t *testing.T) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	// Test concurrent access to metrics
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				wrapper.MLPredictionsInc()
				wrapper.MLLatencyObserve(0.01)
				wrapper.FeatureErrorsInc()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check final counts
	predictions := testutil.ToFloat64(metrics.MLPredictions)
	featureErrors := testutil.ToFloat64(metrics.FeatureErrors)

	expected := 1000.0 // 10 goroutines * 100 increments
	if predictions != expected {
		t.Errorf("Expected %f predictions after concurrent access, got %f", expected, predictions)
	}
	if featureErrors != expected {
		t.Errorf("Expected %f feature errors after concurrent access, got %f", expected, featureErrors)
	}
}

func TestMetricsWrapper_NilGuard(t *testing.T) {
	// Test that wrapper methods don't panic with nil metrics
	// (though this shouldn't happen in practice)
	wrapper := &MetricsWrapper{m: nil}

	// These should panic as expected since we're dereferencing nil
	// In practice, NewWrapper ensures m is never nil
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when accessing nil metrics")
		}
	}()

	wrapper.MLPredictionsInc()
}

func BenchmarkMetricsWrapper_MLPredictionsInc(b *testing.B) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.MLPredictionsInc()
	}
}

func BenchmarkMetricsWrapper_MLLatencyObserve(b *testing.B) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.MLLatencyObserve(0.01)
	}
}

func BenchmarkMetricsWrapper_UpdatePositions(b *testing.B) {
	metrics := New()
	wrapper := NewWrapper(metrics)

	// Pre-allocate positions map to avoid allocation in hot loop
	positions := map[string]float64{
		"BTCUSDT": 0.5,
		"ETHUSDT": -0.3,
		"ADAUSDT": 0.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.UpdatePositions(positions)
	}
}
