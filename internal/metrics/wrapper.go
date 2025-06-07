package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Interfaces for metrics to avoid circular imports
type MetricsCounter interface {
	Inc()
}

type MetricsGauge interface {
	Set(float64)
	Add(float64)
}

type MetricsHistogram interface {
	Observe(float64)
}

// Legacy interfaces for compatibility
type (
	Counter   = MetricsCounter
	Gauge     = MetricsGauge
	Histogram = MetricsHistogram
)

// MetricsWrapper provides a simple interface for executor to use metrics
type MetricsWrapper struct {
	m *Metrics
}

func NewWrapper(m *Metrics) *MetricsWrapper {
	return &MetricsWrapper{m: m}
}

func (w *MetricsWrapper) OrdersTotal() MetricsCounter {
	return &CounterWrapper{w.m.OrdersTotal}
}

func (w *MetricsWrapper) PnLTotal() MetricsGauge {
	return &GaugeWrapper{w.m.PnLTotal}
}

func (w *MetricsWrapper) ONNXLatency() MetricsHistogram {
	return &HistogramWrapper{w.m.ONNXLatency}
}

func (w *MetricsWrapper) UpdatePositions(positions map[string]float64) {
	w.m.UpdatePositions(positions)
}

// ML metrics methods
func (w *MetricsWrapper) MLPredictionsInc() {
	w.m.MLPredictions.Inc()
}

func (w *MetricsWrapper) MLFailuresInc() {
	w.m.MLFailures.Inc()
}

func (w *MetricsWrapper) MLLatencyObserve(v float64) {
	w.m.MLLatency.Observe(v)
}

func (w *MetricsWrapper) MLModelAgeSet(v float64) {
	w.m.MLModelAge.Set(v)
}

func (w *MetricsWrapper) FeatureErrorsInc() {
	w.m.FeatureErrors.Inc()
}

// Enhanced ML metrics methods
func (w *MetricsWrapper) MLAccuracyObserve(v float64) {
	w.m.MLAccuracy.Observe(v)
}

func (w *MetricsWrapper) MLPredictionScoresObserve(v float64) {
	w.m.MLPredictionScores.Observe(v)
}

func (w *MetricsWrapper) MLTimeoutsInc() {
	w.m.MLTimeouts.Inc()
}

func (w *MetricsWrapper) MLFallbackUseInc() {
	w.m.MLFallbackUse.Inc()
}

// Order execution timeout metrics methods
func (w *MetricsWrapper) OrderTimeoutsInc() {
	w.m.OrderTimeouts.Inc()
}

func (w *MetricsWrapper) OrderRetriesInc() {
	w.m.OrderRetries.Inc()
}

func (w *MetricsWrapper) OrderExecutionDurationObserve(v float64) {
	w.m.OrderExecutionDuration.Observe(v)
}

// Feature calculation duration tracking
func (w *MetricsWrapper) FeatureCalcDuration(duration time.Duration) {
	// Convert duration to seconds and observe it as ML latency
	w.m.MLLatency.Observe(duration.Seconds())
}

// Feature sample count tracking
func (w *MetricsWrapper) FeatureSampleCount(count int) {
	// Could add a specific metric for sample counts if needed
	// For now, we'll track it as a gauge
	w.m.VWAPCalculations.Inc()
}

// GetErrorRate exposes the error rate calculation method
func (w *MetricsWrapper) GetErrorRate() float64 {
	return w.m.GetErrorRate()
}

type CounterWrapper struct {
	c prometheus.Counter
}

func (cw *CounterWrapper) Inc() {
	cw.c.Inc()
}

type GaugeWrapper struct {
	g prometheus.Gauge
}

func (gw *GaugeWrapper) Set(v float64) {
	gw.g.Set(v)
}

func (gw *GaugeWrapper) Add(v float64) {
	gw.g.Add(v)
}

type HistogramWrapper struct {
	h prometheus.Histogram
}

func (hw *HistogramWrapper) Observe(v float64) {
	hw.h.Observe(v)
}
