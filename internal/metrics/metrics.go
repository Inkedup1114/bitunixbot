// Package metrics provides Prometheus metrics collection for the Bitunix trading bot.
// It defines and manages all performance, trading, and system metrics that are
// exposed via the Prometheus metrics endpoint for monitoring and alerting.
//
// The package includes metrics for order execution, ML predictions, WebSocket
// connections, feature calculations, and general system health.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the trading bot.
// It provides counters, gauges, and histograms for comprehensive monitoring
// of trading operations, ML predictions, and system performance.
type Metrics struct {
	// Trading metrics
	OrdersTotal            prometheus.Counter   // Total number of orders placed
	PnLTotal               prometheus.Gauge     // Current total profit and loss
	ActivePositions        prometheus.Gauge     // Number of active positions
	OrderTimeouts          prometheus.Counter   // Number of order execution timeouts
	OrderRetries           prometheus.Counter   // Number of order placement retries
	OrderExecutionDuration prometheus.Histogram // Duration of order execution attempts

	// WebSocket and data metrics
	WSReconnects   prometheus.Counter // Total number of WebSocket reconnections
	TradesReceived prometheus.Counter // Total number of trade messages received
	DepthsReceived prometheus.Counter // Total number of depth messages received

	// ML and prediction metrics
	MLPredictions      prometheus.Counter   // Total number of ML predictions made
	MLFailures         prometheus.Counter   // Total number of ML prediction failures
	MLModelAge         prometheus.Gauge     // Age of the current ML model in seconds
	MLLatency          prometheus.Histogram // ML prediction latency in seconds
	MLAccuracy         prometheus.Histogram // ML model prediction accuracy
	MLPredictionScores prometheus.Histogram // Distribution of ML prediction confidence scores
	MLTimeouts         prometheus.Counter   // Total number of ML prediction timeouts
	MLFallbackUse      prometheus.Counter   // Total number of times ML fallback was used
	ONNXLatency        prometheus.Histogram // ONNX model inference latency

	// Feature calculation metrics
	VWAPCalculations prometheus.Counter // Total number of VWAP calculations performed
	FeatureErrors    prometheus.Counter // Total number of feature calculation errors

	// System metrics
	ErrorsTotal prometheus.Counter // Total number of errors encountered
}

// New creates and registers all Prometheus metrics using the default registry.
// This is the standard way to create metrics for production use.
func New() *Metrics {
	return NewWithRegistry(prometheus.DefaultRegisterer)
}

// NewWithRegistry creates metrics with a custom registry (useful for testing).
// This allows for isolated metric collection in tests without affecting
// the global Prometheus registry.
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
			Buckets: prometheus.LinearBuckets(0, 0.1, 11),
		}),
		MLTimeouts: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_timeouts_total",
			Help: "Total number of ML prediction timeouts",
		}),
		MLFallbackUse: factory.NewCounter(prometheus.CounterOpts{
			Name: "ml_fallback_use_total",
			Help: "Total number of times ML fallback was used",
		}),
		OrderTimeouts: factory.NewCounter(prometheus.CounterOpts{
			Name: "order_timeouts_total",
			Help: "Total number of order execution timeouts",
		}),
		OrderRetries: factory.NewCounter(prometheus.CounterOpts{
			Name: "order_retries_total",
			Help: "Total number of order placement retries",
		}),
		OrderExecutionDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "order_execution_duration_seconds",
			Help:    "Duration of order execution attempts in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		}),
	}
}

// UpdatePositions updates the active positions metric based on current position sizes.
// It counts the number of non-zero positions across all symbols and updates the gauge.
func (m *Metrics) UpdatePositions(positions map[string]float64) {
	count := 0
	for _, pos := range positions {
		if pos != 0 {
			count++
		}
	}
	m.ActivePositions.Set(float64(count))
}

// GetErrorRate calculates the current error rate based on total operations and errors.
// Returns the ratio of errors to total operations, or 0 if no operations have been recorded.
// This is useful for circuit breaker implementations and system health monitoring.
func (m *Metrics) GetErrorRate() float64 {
	// Get total operations and errors using Prometheus metric types
	var totalOps, totalErrors float64

	// Get metric values from registry
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}

	for _, mf := range metricFamilies {
		switch *mf.Name {
		case "orders_total":
			for _, m := range mf.Metric {
				totalOps = *m.Counter.Value
			}
		case "errors_total":
			for _, m := range mf.Metric {
				totalErrors = *m.Counter.Value
			}
		}
	}

	// Avoid division by zero
	if totalOps == 0 {
		return 0
	}

	// Calculate error rate
	return totalErrors / totalOps
}

// RegisterMetrics registers all metrics with the Prometheus registry.
// This method is deprecated - use New() or NewWithRegistry() instead,
// which automatically register metrics during creation.
func (m *Metrics) RegisterMetrics() {
	prometheus.MustRegister(m.OrdersTotal)
	prometheus.MustRegister(m.WSReconnects)
	prometheus.MustRegister(m.PnLTotal)
	prometheus.MustRegister(m.ONNXLatency)
	prometheus.MustRegister(m.ActivePositions)
	prometheus.MustRegister(m.TradesReceived)
	prometheus.MustRegister(m.DepthsReceived)
	prometheus.MustRegister(m.ErrorsTotal)
	prometheus.MustRegister(m.VWAPCalculations)
	prometheus.MustRegister(m.MLPredictions)
	prometheus.MustRegister(m.MLFailures)
	prometheus.MustRegister(m.MLModelAge)
	prometheus.MustRegister(m.MLLatency)
	prometheus.MustRegister(m.FeatureErrors)
	prometheus.MustRegister(m.MLAccuracy)
	prometheus.MustRegister(m.MLPredictionScores)
	prometheus.MustRegister(m.MLTimeouts)
	prometheus.MustRegister(m.MLFallbackUse)
	prometheus.MustRegister(m.OrderTimeouts)
	prometheus.MustRegister(m.OrderRetries)
	prometheus.MustRegister(m.OrderExecutionDuration)
}
