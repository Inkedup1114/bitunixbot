package metrics

import "github.com/prometheus/client_golang/prometheus"

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
type Counter = MetricsCounter
type Gauge = MetricsGauge
type Histogram = MetricsHistogram

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
