package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the trading bot
type Metrics struct {
	OrdersTotal        prometheus.Counter
	WSReconnects       prometheus.Counter
	PnLTotal           prometheus.Gauge
	ONNXLatency        prometheus.Histogram
	ActivePositions    prometheus.Gauge
	TradesReceived     prometheus.Counter
	DepthsReceived     prometheus.Counter
	ErrorsTotal        prometheus.Counter
	VWAPCalculations   prometheus.Counter
	MLPredictions      prometheus.Counter
	MLFailures         prometheus.Counter
	MLModelAge         prometheus.Gauge
	MLLatency          prometheus.Histogram
	FeatureErrors      prometheus.Counter
	MLAccuracy         prometheus.Histogram
	MLPredictionScores prometheus.Histogram
	MLTimeouts         prometheus.Counter
	MLFallbackUse      prometheus.Counter
}

// New creates and registers all Prometheus metrics
func New() *Metrics {
	return NewWithRegistry(prometheus.DefaultRegisterer)
}

// NewWithRegistry creates metrics with a custom registry (useful for testing)
func NewWithRegistry(registerer prometheus.Registerer) *Metrics {
	factory := promauto.With(registerer)
	return &Metrics{
		OrdersTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "orders_total",
			Help: "Total number of orders placed",
		}),
		WSReconnects: factory.NewCounter(prometheus.CounterOpts{
			Name: "ws_reconnects_total",
			Help: "Total number of WebSocket reconnections",
		}),
		PnLTotal: factory.NewGauge(prometheus.GaugeOpts{
			Name: "pnl_total",
			Help: "Current total profit and loss",
		}),
		ONNXLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "onnx_latency_seconds",
			Help:    "ONNX model inference latency in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		ActivePositions: factory.NewGauge(prometheus.GaugeOpts{
			Name: "active_positions",
			Help: "Number of active positions",
		}),
		TradesReceived: factory.NewCounter(prometheus.CounterOpts{
			Name: "trades_received_total",
			Help: "Total number of trade messages received",
		}),
		DepthsReceived: factory.NewCounter(prometheus.CounterOpts{
			Name: "depths_received_total",
			Help: "Total number of depth messages received",
		}),
		ErrorsTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors encountered",
		}),
		VWAPCalculations: factory.NewCounter(prometheus.CounterOpts{
			Name: "vwap_calculations_total",
			Help: "Total number of VWAP calculations performed",
		}),
		MLPredictions: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_predictions_total",
			Help: "Total number of ML predictions made",
		}),
		MLFailures: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_failures_total",
			Help: "Total number of ML prediction failures",
		}),
		MLModelAge: factory.NewGauge(prometheus.GaugeOpts{
			Name: "ml_model_age_seconds",
			Help: "Age of the current ML model in seconds",
		}),
		MLLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "ml_latency_seconds",
			Help:    "ML prediction latency in seconds (end-to-end)",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		}),
		FeatureErrors: factory.NewCounter(prometheus.CounterOpts{
			Name: "feature_errors_total",
			Help: "Total number of feature calculation errors",
		}),
		MLAccuracy: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "ml_accuracy",
			Help:    "ML model prediction accuracy (when ground truth is available)",
			Buckets: []float64{0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 1.0},
		}),
		MLPredictionScores: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "ml_prediction_scores",
			Help:    "Distribution of ML prediction confidence scores",
			Buckets: []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		}),
		MLTimeouts: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_timeouts_total",
			Help: "Total number of ML prediction timeouts",
		}),
		MLFallbackUse: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_fallback_use_total",
			Help: "Total number of times fallback heuristics were used",
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
