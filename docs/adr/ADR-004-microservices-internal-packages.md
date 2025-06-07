# ADR-004: Microservices Architecture with Internal Packages

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires a well-structured architecture that:
- Separates concerns between different functional areas
- Enables independent testing and development of components
- Provides clear interfaces between modules
- Supports future scaling and feature additions
- Maintains code organization and readability
- Follows Go best practices for project structure

Key functional areas identified:
- **Exchange Communication**: REST API and WebSocket clients
- **Feature Calculation**: Technical indicators and market analysis
- **ML Prediction**: Machine learning inference and model management
- **Order Execution**: Trading logic and risk management
- **Data Storage**: Persistent storage and data access
- **Metrics**: Monitoring and observability
- **Configuration**: Settings and environment management
- **Security**: Authentication, authorization, and audit logging

Architecture options considered:
- **Monolithic**: Single large package with all functionality
- **Microservices**: Separate services communicating over network
- **Modular Monolith**: Single binary with well-defined internal modules
- **Plugin Architecture**: Dynamic loading of components

## Decision

We chose a **Modular Monolith with Internal Packages** architecture, following Go's standard project layout.

### Project Structure:
```
bitunix-bot/
├── cmd/                    # Application entry points
│   ├── bitrader/          # Main trading application
│   ├── backtest/          # Backtesting tool
│   └── scripts/           # Utility scripts
├── internal/              # Private application packages
│   ├── exchange/          # Exchange API clients
│   │   └── bitunix/       # Bitunix-specific implementation
│   ├── features/          # Technical indicators
│   ├── ml/                # Machine learning components
│   ├── exec/              # Order execution engine
│   ├── storage/           # Data persistence
│   ├── metrics/           # Monitoring and metrics
│   ├── cfg/               # Configuration management
│   ├── security/          # Security components
│   ├── dashboard/         # Risk dashboard
│   └── common/            # Shared constants and utilities
├── scripts/               # External scripts (Python ML pipeline)
├── deploy/                # Deployment configurations
└── docs/                  # Documentation
```

### Key Principles:

1. **Single Binary**: Deploy as one executable for operational simplicity
2. **Clear Boundaries**: Each package has a specific responsibility
3. **Interface-Driven**: Use interfaces for testability and flexibility
4. **Dependency Injection**: Pass dependencies explicitly for better testing
5. **Internal Packages**: Use Go's `internal/` to prevent external imports

## Consequences

### Positive:
- **Operational Simplicity**: Single binary deployment and management
- **Clear Separation**: Well-defined package boundaries and responsibilities
- **Testability**: Each package can be tested independently
- **Performance**: No network overhead between components
- **Consistency**: Shared data structures and error handling
- **Development Speed**: Easy to understand and navigate codebase
- **Go Idiomatic**: Follows Go community best practices

### Negative:
- **Scaling Limitations**: Cannot scale individual components independently
- **Deployment Coupling**: All components deploy together
- **Resource Sharing**: All components share the same process resources
- **Technology Constraints**: All components must use the same language/runtime

### Mitigations:
- **Interface Design**: Use interfaces to enable future service extraction
- **Resource Management**: Implement proper resource pooling and limits
- **Monitoring**: Comprehensive metrics to identify bottlenecks
- **Modular Design**: Keep packages loosely coupled for future extraction

## Package Responsibilities

### cmd/bitrader
- Application entry point and initialization
- Dependency wiring and configuration loading
- Graceful shutdown handling
- Signal processing and lifecycle management

### internal/exchange
- Exchange API communication (REST and WebSocket)
- Order placement and status tracking
- Market data streaming and parsing
- Connection management and retry logic

### internal/features
- Technical indicator calculation (VWAP, imbalances)
- Real-time feature extraction from market data
- Time-series data management
- Mathematical computations for trading signals

### internal/ml
- Machine learning model management
- Prediction inference and caching
- Model health monitoring and fallback
- A/B testing and performance tracking

### internal/exec
- Trading strategy execution
- Risk management and position tracking
- Order sizing and execution logic
- Circuit breaker implementation

### internal/storage
- Data persistence using BoltDB
- Time-series data storage and retrieval
- Database connection management
- Data migration and backup

### internal/metrics
- Prometheus metrics collection
- Performance monitoring
- Health checks and alerting
- Custom business metrics

### internal/cfg
- Configuration loading and validation
- Environment variable processing
- Settings management and defaults
- Configuration hot-reloading

### internal/security
- API authentication and authorization
- Rate limiting and IP whitelisting
- Audit logging and encryption
- Security middleware and validation

## Interface Design

### Key Interfaces:
```go
// ML Predictor interface
type PredictorInterface interface {
    Predict(features []float32) ([]float32, error)
    PredictWithContext(ctx context.Context, features []float32) (float32, error)
    Health() error
}

// Storage interface
type StorageInterface interface {
    StoreTrade(trade *bitunix.Trade) error
    GetTradesInRange(symbol string, start, end time.Time) ([]*bitunix.Trade, error)
    Close() error
}

// Exchange client interface
type ExchangeInterface interface {
    PlaceOrder(order *OrderRequest) (*OrderResponse, error)
    GetBalance() (*BalanceResponse, error)
    Subscribe(symbols []string) (<-chan *MarketData, error)
}
```

## Dependency Flow

```
cmd/bitrader
    ├── internal/cfg (configuration)
    ├── internal/metrics (monitoring)
    ├── internal/storage (persistence)
    ├── internal/exchange (market data)
    ├── internal/features (indicators)
    ├── internal/ml (predictions)
    ├── internal/exec (trading)
    └── internal/security (protection)
```

## Testing Strategy

- **Unit Tests**: Each package tested independently with mocks
- **Integration Tests**: Test package interactions with real dependencies
- **End-to-End Tests**: Full system tests with external services
- **Benchmark Tests**: Performance testing for critical paths

## Future Evolution

This architecture supports future evolution to microservices:
1. **Extract ML Service**: Move ML components to separate service
2. **Extract Storage Service**: Create dedicated data service
3. **Extract Exchange Service**: Separate exchange communication
4. **API Gateway**: Add API gateway for service coordination

## Related ADRs
- ADR-001: Go as Primary Language
- ADR-002: BoltDB for Embedded Storage
- ADR-003: ONNX Runtime with Python Bridge for ML
