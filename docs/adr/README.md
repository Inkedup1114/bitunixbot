# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Bitunix Trading Bot project. ADRs document the significant architectural decisions made during the development of the system.

## What are ADRs?

Architecture Decision Records (ADRs) are short text documents that capture important architectural decisions made along with their context and consequences. They help teams understand why certain decisions were made and provide historical context for future development.

## ADR Format

Each ADR follows this structure:
- **Title**: A short descriptive title
- **Status**: Proposed, Accepted, Deprecated, or Superseded
- **Context**: The situation that led to this decision
- **Decision**: The architectural decision made
- **Consequences**: The positive and negative outcomes of this decision

## Index of ADRs

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [ADR-001](ADR-001-go-primary-language.md) | Go as Primary Language | Accepted | 2025-01-31 |
| [ADR-002](ADR-002-boltdb-embedded-storage.md) | BoltDB for Embedded Storage | Accepted | 2025-01-31 |
| [ADR-003](ADR-003-onnx-python-bridge-ml.md) | ONNX Runtime with Python Bridge for ML | Accepted | 2025-01-31 |
| [ADR-004](ADR-004-microservices-internal-packages.md) | Microservices Architecture with Internal Packages | Accepted | 2025-01-31 |
| [ADR-005](ADR-005-websocket-rest-hybrid.md) | WebSocket + REST API Hybrid Communication | Accepted | 2025-01-31 |
| [ADR-006](ADR-006-prometheus-metrics.md) | Prometheus for Metrics and Monitoring | Accepted | 2025-01-31 |
| [ADR-007](ADR-007-multi-environment-deployment.md) | Multi-Environment Deployment Strategy | Accepted | 2025-01-31 |
| [ADR-008](ADR-008-security-first-design.md) | Security-First Design with Multiple Layers | Accepted | 2025-01-31 |
| [ADR-009](ADR-009-circuit-breaker-risk-management.md) | Circuit Breaker Pattern for Risk Management | Accepted | 2025-01-31 |
| [ADR-010](ADR-010-configuration-driven-strategies.md) | Configuration-Driven Trading Strategies | Accepted | 2025-01-31 |

## Guidelines for New ADRs

When creating new ADRs:

1. **Use the next sequential number** (ADR-011, ADR-012, etc.)
2. **Follow the established format** for consistency
3. **Be concise but comprehensive** - include enough context for future developers
4. **Update this README** to include the new ADR in the index
5. **Consider the long-term implications** of the decision

## ADR Lifecycle

- **Proposed**: The ADR is under discussion
- **Accepted**: The decision has been made and is being implemented
- **Deprecated**: The decision is no longer recommended but may still be in use
- **Superseded**: The decision has been replaced by a newer ADR

## References

- [Architecture Decision Records (ADRs) by Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
- [ADR GitHub Organization](https://adr.github.io/)
