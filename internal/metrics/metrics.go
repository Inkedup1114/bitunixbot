package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the trading bot
type Metrics struct {
	OrdersTotal      prometheus.Counter
	WSReconnects     prometheus.Counter
	PnLTotal         prometheus.Gauge
	ONNXLatency      prometheus.Histogram
	ActivePositions  prometheus.Gauge
	TradesReceived   prometheus.Counter
	DepthsReceived   prometheus.Counter
	ErrorsTotal      prometheus.Counter
	VWAPCalculations prometheus.Counter
}

// New creates and registers all Prometheus metrics
func New() *Metrics {
	return &Metrics{
		OrdersTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "orders_total",
			Help: "Total number of orders placed",
		}),
		WSReconnects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ws_reconnects_total",
			Help: "Total number of WebSocket reconnections",
		}),
		PnLTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "pnl_total",
			Help: "Current total profit and loss",
		}),
		ONNXLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "onnx_latency_seconds",
			Help:    "ONNX model inference latency in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		ActivePositions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "active_positions",
			Help: "Number of active positions",
		}),
		TradesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "trades_received_total",
			Help: "Total number of trade messages received",
		}),
		DepthsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "depths_received_total",
			Help: "Total number of depth messages received",
		}),
		ErrorsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors encountered",
		}),
		VWAPCalculations: promauto.NewCounter(prometheus.CounterOpts{
			Name: "vwap_calculations_total",
			Help: "Total number of VWAP calculations performed",
		}),
	}
}

// UpdatePositions updates the active positions metric
func (m *Metrics) UpdatePositions(positions map[string]float64) {
	count := 0
	for _, pos := range positions {
		if pos != 0 {
			count++
		}
	}
	m.ActivePositions.Set(float64(count))
}
