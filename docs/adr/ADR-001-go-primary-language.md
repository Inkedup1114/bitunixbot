# ADR-001: Go as Primary Language

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires a programming language that can handle:
- High-frequency trading operations with low latency
- Concurrent processing of market data streams
- Real-time order execution and risk management
- Integration with external APIs and WebSocket connections
- Deployment as a single, lightweight binary
- Strong type safety for financial calculations
- Excellent performance under load

Several language options were considered:
- **Go**: Strong concurrency, single binary deployment, excellent performance
- **Rust**: Maximum performance, memory safety, but steeper learning curve
- **Java**: Mature ecosystem, but JVM overhead and complex deployment
- **Python**: Rich ML ecosystem, but performance limitations for HFT
- **C++**: Maximum performance, but complex memory management and deployment

## Decision

We chose **Go (Golang) version 1.22+** as the primary programming language for the Bitunix Trading Bot.

### Key Factors:

1. **Concurrency Model**: Go's goroutines and channels provide excellent support for handling multiple market data streams, order processing, and risk management concurrently
2. **Performance**: Near C++ performance with much simpler development and deployment
3. **Single Binary Deployment**: Go compiles to a single static binary (~15MB), simplifying deployment across environments
4. **Type Safety**: Strong static typing prevents common financial calculation errors
5. **Standard Library**: Excellent built-in support for HTTP clients, JSON processing, and cryptographic operations
6. **Ecosystem**: Rich ecosystem with libraries for REST APIs, WebSockets, databases, and monitoring
7. **Maintainability**: Simple syntax and excellent tooling reduce development and maintenance overhead

## Consequences

### Positive:
- **Fast Development**: Simple syntax and excellent tooling accelerate development
- **High Performance**: Excellent performance for concurrent operations and low-latency trading
- **Easy Deployment**: Single binary deployment simplifies operations across environments
- **Memory Efficiency**: Garbage collector optimized for low-latency applications
- **Strong Concurrency**: Built-in support for concurrent market data processing
- **Cross-Platform**: Easy compilation for Linux, Windows, and macOS
- **Rich Ecosystem**: Extensive libraries for financial applications

### Negative:
- **ML Limitations**: Limited machine learning ecosystem compared to Python
- **Generics**: Limited generics support (improved in Go 1.18+)
- **Dependency Management**: Go modules learning curve for team members

### Mitigations:
- **ML Integration**: Use Python bridge for ONNX runtime and ML operations (see ADR-003)
- **Team Training**: Provide Go training and establish coding standards
- **Tooling**: Leverage excellent Go tooling (gofmt, golint, go vet) for code quality

## Implementation Notes

- Use Go 1.22+ for improved performance and language features
- Follow Go best practices for project structure (`cmd/`, `internal/`, `pkg/`)
- Leverage Go's built-in testing framework for comprehensive test coverage
- Use Go modules for dependency management
- Implement proper error handling with Go's explicit error returns
- Utilize Go's built-in profiling tools for performance optimization

## Related ADRs
- ADR-003: ONNX Runtime with Python Bridge for ML
- ADR-004: Microservices Architecture with Internal Packages
