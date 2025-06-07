# ADR-006: Prometheus for Metrics and Monitoring

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires comprehensive monitoring and observability for:
- **Trading Performance**: P&L tracking, win rates, position metrics
- **System Health**: CPU, memory, network, and application performance
- **Risk Management**: Exposure limits, circuit breaker status, daily loss tracking
- **ML Performance**: Prediction accuracy, latency, model drift detection
- **Exchange Connectivity**: WebSocket uptime, REST API latency, error rates
- **Operational Metrics**: Order execution times, feature calculation performance

Monitoring requirements:
- **Real-time Metrics**: Live dashboards for trading operations
- **Historical Analysis**: Long-term performance and trend analysis
- **Alerting**: Automated alerts for critical issues and thresholds
- **Multi-dimensional**: Metrics with labels for filtering and aggregation
- **Industry Standard**: Compatible with existing monitoring infrastructure
- **Low Overhead**: Minimal impact on trading performance

Monitoring solutions considered:
- **Prometheus**: Industry standard, excellent Go support, pull-based
- **InfluxDB**: Time-series optimized, but requires additional infrastructure
- **DataDog**: SaaS solution, but vendor lock-in and cost concerns
- **Custom Logging**: Simple but limited analysis capabilities
- **StatsD**: Push-based, but requires additional aggregation layer
- **OpenTelemetry**: Comprehensive but complex for simple use cases

## Decision

We chose **Prometheus** as the primary metrics collection and monitoring solution.

### Architecture:
- **Metrics Collection**: Prometheus client library in Go application
- **Metrics Exposure**: HTTP endpoint for Prometheus scraping
- **Storage**: Prometheus server for metrics storage and querying
- **Visualization**: Grafana dashboards for real-time monitoring
- **Alerting**: Prometheus Alertmanager for threshold-based alerts

### Key Factors:

1. **Industry Standard**: Widely adopted in the Go and DevOps communities
2. **Pull-based Model**: Prometheus scrapes metrics, reducing application complexity
3. **Excellent Go Support**: Native client library with minimal overhead
4. **Multi-dimensional**: Rich label support for detailed analysis
5. **Query Language**: PromQL for powerful metric analysis and alerting
6. **Ecosystem**: Extensive integrations with Grafana, Kubernetes, and cloud platforms
7. **Reliability**: Battle-tested in production environments

## Consequences

### Positive:
- **Rich Metrics**: Comprehensive metrics with labels and dimensions
- **Real-time Monitoring**: Live dashboards and alerting capabilities
- **Performance Analysis**: Historical data for optimization and debugging
- **Standard Tooling**: Compatible with existing monitoring infrastructure
- **Low Overhead**: Efficient metrics collection with minimal performance impact
- **Flexible Querying**: PromQL enables complex analysis and alerting rules
- **Community Support**: Large ecosystem and extensive documentation

### Negative:
- **Storage Requirements**: Metrics data requires disk space over time
- **Learning Curve**: PromQL and Prometheus concepts require training
- **Infrastructure Dependency**: Requires Prometheus server deployment
- **Cardinality Concerns**: High-cardinality metrics can impact performance

### Mitigations:
- **Retention Policies**: Configure appropriate data retention periods
- **Metric Design**: Careful label design to avoid cardinality explosion
- **Sampling**: Use histograms and summaries for high-frequency metrics
- **Documentation**: Provide team training and metric documentation

## Implementation Details

### Metrics Categories:

#### 1. Trading Metrics:
```go
var (
    // Order execution metrics
    ordersTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "bitunix_orders_total",
            Help: "Total number of orders placed",
        },
        []string{"symbol", "side", "status"},
    )
    
    // P&L tracking
    pnlGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "bitunix_pnl_usd",
            Help: "Current profit and loss in USD",
        },
        []string{"symbol", "strategy"},
    )
    
    // Position exposure
    positionExposure = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "bitunix_position_exposure_ratio",
            Help: "Current position exposure as ratio of account balance",
        },
        []string{"symbol"},
    )
)
```

#### 2. System Metrics:
```go
var (
    // Application performance
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "bitunix_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "endpoint", "status"},
    )
    
    // Memory usage
    memoryUsage = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "bitunix_memory_usage_bytes",
            Help: "Memory usage in bytes",
        },
        []string{"type"},
    )
)
```

#### 3. ML Metrics:
```go
var (
    // Prediction performance
    mlPredictionDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "bitunix_ml_prediction_duration_seconds",
            Help:    "ML prediction duration in seconds",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
        },
    )
    
    // Model accuracy
    mlAccuracy = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "bitunix_ml_accuracy_ratio",
            Help: "ML model accuracy ratio",
        },
        []string{"model_version", "time_window"},
    )
)
```

### Metrics Wrapper:
```go
// internal/metrics/metrics.go
type MetricsWrapper struct {
    registry *prometheus.Registry
    
    // Trading metrics
    OrdersTotal      *prometheus.CounterVec
    PnLGauge         *prometheus.GaugeVec
    PositionExposure *prometheus.GaugeVec
    
    // System metrics
    RequestDuration *prometheus.HistogramVec
    MemoryUsage     *prometheus.GaugeVec
    
    // ML metrics
    MLPredictionDuration *prometheus.Histogram
    MLAccuracy          *prometheus.GaugeVec
}

func NewMetricsWrapper() *MetricsWrapper {
    registry := prometheus.NewRegistry()
    
    wrapper := &MetricsWrapper{
        registry:             registry,
        OrdersTotal:          ordersTotal,
        PnLGauge:            pnlGauge,
        PositionExposure:    positionExposure,
        RequestDuration:     requestDuration,
        MemoryUsage:         memoryUsage,
        MLPredictionDuration: mlPredictionDuration,
        MLAccuracy:          mlAccuracy,
    }
    
    // Register all metrics
    registry.MustRegister(
        ordersTotal,
        pnlGauge,
        positionExposure,
        requestDuration,
        memoryUsage,
        mlPredictionDuration,
        mlAccuracy,
    )
    
    return wrapper
}

func (m *MetricsWrapper) Handler() http.Handler {
    return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
```

### HTTP Metrics Endpoint:
```go
// cmd/bitrader/main.go
func startMetricsServer(metrics *metrics.MetricsWrapper, port int) {
    mux := http.NewServeMux()
    mux.Handle("/metrics", metrics.Handler())
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })
    
    server := &http.Server{
        Addr:    fmt.Sprintf(":%d", port),
        Handler: mux,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
    }
    
    log.Info().Int("port", port).Msg("Starting metrics server")
    if err := server.ListenAndServe(); err != nil {
        log.Error().Err(err).Msg("Metrics server failed")
    }
}
```

### Prometheus Configuration:
```yaml
# deploy/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "alert_rules.yml"

scrape_configs:
  - job_name: 'bitunix-bot'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 5s
    metrics_path: /metrics

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
```

### Key Metrics Dashboard:

#### Trading Dashboard:
- Real-time P&L by symbol and strategy
- Order execution success rates and latency
- Position exposure and risk limits
- Daily loss tracking and circuit breaker status

#### System Dashboard:
- Application performance and resource usage
- WebSocket connection status and message rates
- REST API latency and error rates
- Database performance and storage usage

#### ML Dashboard:
- Prediction accuracy and drift detection
- Model performance and A/B test results
- Feature importance and correlation analysis
- Inference latency and cache hit rates

## Alerting Rules

### Critical Alerts:
- Daily loss limit exceeded
- Circuit breaker activated
- WebSocket connection lost
- Order execution failures
- ML model accuracy degradation

### Warning Alerts:
- High memory usage
- Increased API latency
- Position exposure approaching limits
- Model prediction latency spikes

## Related ADRs
- ADR-004: Microservices Architecture with Internal Packages
- ADR-005: WebSocket + REST API Hybrid Communication
- ADR-009: Circuit Breaker Pattern for Risk Management
