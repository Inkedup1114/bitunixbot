# ADR-009: Circuit Breaker Pattern for Risk Management

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires robust risk management to protect against:
- **Market Volatility**: Extreme price movements and flash crashes
- **System Failures**: Exchange outages, network issues, and internal errors
- **Abnormal Conditions**: Unusual trading volumes, order book imbalances
- **Model Degradation**: ML model failures or poor performance
- **Operational Risks**: Configuration errors and deployment issues

Risk management requirements:
- **Automatic Protection**: Immediate response to dangerous conditions
- **Configurable Thresholds**: Adjustable limits for different market conditions
- **Graceful Degradation**: Continue safe operations when possible
- **Quick Recovery**: Automatic resumption when conditions normalize
- **Comprehensive Monitoring**: Real-time visibility into risk status
- **Audit Trail**: Complete record of risk events and decisions

Risk management patterns considered:
- **Static Limits**: Fixed thresholds that never change
- **Circuit Breaker**: Dynamic protection that adapts to conditions
- **Kill Switch**: Manual emergency stop mechanism
- **Gradual Degradation**: Progressive reduction of trading activity
- **Hybrid Approach**: Combination of multiple risk management techniques

## Decision

We chose the **Circuit Breaker Pattern** as the primary risk management mechanism, enhanced with multiple detection methods and automatic recovery.

### Circuit Breaker Architecture:
1. **Volatility Monitoring**: VWAP standard deviation tracking
2. **Order Book Analysis**: Bid-ask spread and imbalance detection
3. **Volume Monitoring**: Unusual trading volume detection
4. **Error Rate Tracking**: System and exchange error monitoring
5. **Automatic Recovery**: Configurable cooldown and recovery logic

### Key Features:
- **Multi-dimensional Monitoring**: Multiple independent risk factors
- **Configurable Thresholds**: Environment-specific risk limits
- **Automatic Recovery**: Self-healing when conditions improve
- **Comprehensive Logging**: Detailed audit trail of all risk events

## Consequences

### Positive:
- **Automatic Protection**: Immediate response to dangerous market conditions
- **Reduced Losses**: Prevention of trading during adverse conditions
- **System Resilience**: Graceful handling of exchange and system failures
- **Operational Safety**: Protection against configuration and deployment errors
- **Regulatory Compliance**: Demonstrates responsible risk management practices
- **Peace of Mind**: Automated protection allows focus on strategy development

### Negative:
- **Missed Opportunities**: May prevent trading during profitable but volatile periods
- **False Positives**: Overly sensitive thresholds may trigger unnecessary stops
- **Complexity**: Additional monitoring and configuration requirements
- **Recovery Delays**: Time required for automatic recovery may miss opportunities

### Mitigations:
- **Tunable Thresholds**: Careful calibration based on historical data
- **Multiple Conditions**: Require multiple factors to trigger circuit breaker
- **Quick Recovery**: Short cooldown periods for rapid resumption
- **Manual Override**: Emergency controls for exceptional circumstances

## Implementation Details

### 1. Circuit Breaker Core

#### Circuit Breaker State Machine:
```go
// internal/exec/circuit_breaker.go
type CircuitBreakerState int

const (
    CircuitClosed CircuitBreakerState = iota  // Normal operation
    CircuitOpen                               // Trading blocked
    CircuitHalfOpen                          // Testing recovery
)

type CircuitBreaker struct {
    state           CircuitBreakerState
    config          CircuitBreakerConfig
    lastFailureTime time.Time
    consecutiveFailures int
    mu              sync.RWMutex
    
    // Monitoring components
    volatilityMonitor *VolatilityMonitor
    volumeMonitor     *VolumeMonitor
    errorMonitor      *ErrorMonitor
    imbalanceMonitor  *ImbalanceMonitor
}

func (cb *CircuitBreaker) CanTrade(symbol string) bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    
    switch cb.state {
    case CircuitClosed:
        return true
    case CircuitOpen:
        // Check if cooldown period has passed
        if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
            cb.setState(CircuitHalfOpen)
            return true
        }
        return false
    case CircuitHalfOpen:
        // Allow limited trading to test recovery
        return true
    default:
        return false
    }
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if cb.state == CircuitHalfOpen {
        cb.setState(CircuitClosed)
        cb.consecutiveFailures = 0
    }
}

func (cb *CircuitBreaker) RecordFailure(reason string) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    cb.consecutiveFailures++
    cb.lastFailureTime = time.Now()
    
    if cb.consecutiveFailures >= cb.config.FailureThreshold {
        cb.setState(CircuitOpen)
        log.Warn().
            Str("reason", reason).
            Int("consecutive_failures", cb.consecutiveFailures).
            Msg("Circuit breaker opened")
    }
}
```

### 2. Volatility Monitoring

#### VWAP-based Volatility Detection:
```go
// internal/exec/volatility_monitor.go
type VolatilityMonitor struct {
    config    VolatilityConfig
    vwapCalc  *features.VWAPCalculator
    mu        sync.RWMutex
}

func (vm *VolatilityMonitor) CheckVolatility(symbol string) (bool, float64) {
    vm.mu.RLock()
    defer vm.mu.RUnlock()
    
    vwap, stdDev, err := vm.vwapCalc.CalcWithMetrics()
    if err != nil {
        return false, 0
    }
    
    // Calculate volatility as percentage of VWAP
    volatility := (stdDev / vwap) * 100
    
    isExcessive := volatility > vm.config.MaxVolatilityPercent
    
    if isExcessive {
        log.Warn().
            Str("symbol", symbol).
            Float64("volatility_percent", volatility).
            Float64("threshold", vm.config.MaxVolatilityPercent).
            Msg("Excessive volatility detected")
    }
    
    return isExcessive, volatility
}
```

### 3. Order Book Imbalance Detection

#### Bid-Ask Imbalance Monitoring:
```go
// internal/exec/imbalance_monitor.go
type ImbalanceMonitor struct {
    config ImbalanceConfig
    mu     sync.RWMutex
}

func (im *ImbalanceMonitor) CheckImbalance(depth *bitunix.Depth) (bool, float64) {
    im.mu.RLock()
    defer im.mu.RUnlock()
    
    if len(depth.Bids) == 0 || len(depth.Asks) == 0 {
        return true, 1.0 // Complete imbalance
    }
    
    // Calculate total bid and ask volumes
    totalBidVolume := 0.0
    totalAskVolume := 0.0
    
    for _, bid := range depth.Bids {
        totalBidVolume += bid.Quantity
    }
    
    for _, ask := range depth.Asks {
        totalAskVolume += ask.Quantity
    }
    
    // Calculate imbalance ratio
    totalVolume := totalBidVolume + totalAskVolume
    if totalVolume == 0 {
        return true, 1.0
    }
    
    imbalance := math.Abs(totalBidVolume-totalAskVolume) / totalVolume
    isExcessive := imbalance > im.config.MaxImbalanceRatio
    
    if isExcessive {
        log.Warn().
            Float64("bid_volume", totalBidVolume).
            Float64("ask_volume", totalAskVolume).
            Float64("imbalance_ratio", imbalance).
            Float64("threshold", im.config.MaxImbalanceRatio).
            Msg("Excessive order book imbalance detected")
    }
    
    return isExcessive, imbalance
}
```

### 4. Volume Monitoring

#### Trading Volume Anomaly Detection:
```go
// internal/exec/volume_monitor.go
type VolumeMonitor struct {
    config        VolumeConfig
    recentVolumes map[string]*VolumeTracker
    mu            sync.RWMutex
}

type VolumeTracker struct {
    volumes   []float64
    window    time.Duration
    timestamps []time.Time
}

func (vm *VolumeMonitor) CheckVolume(symbol string, volume float64) (bool, float64) {
    vm.mu.Lock()
    defer vm.mu.Unlock()
    
    tracker, exists := vm.recentVolumes[symbol]
    if !exists {
        tracker = &VolumeTracker{
            window: vm.config.VolumeWindow,
        }
        vm.recentVolumes[symbol] = tracker
    }
    
    // Add current volume
    now := time.Now()
    tracker.volumes = append(tracker.volumes, volume)
    tracker.timestamps = append(tracker.timestamps, now)
    
    // Remove old volumes outside the window
    cutoff := now.Add(-tracker.window)
    for i := 0; i < len(tracker.timestamps); i++ {
        if tracker.timestamps[i].After(cutoff) {
            tracker.volumes = tracker.volumes[i:]
            tracker.timestamps = tracker.timestamps[i:]
            break
        }
    }
    
    // Calculate average volume
    if len(tracker.volumes) < vm.config.MinSamples {
        return false, 0 // Not enough data
    }
    
    avgVolume := 0.0
    for _, v := range tracker.volumes {
        avgVolume += v
    }
    avgVolume /= float64(len(tracker.volumes))
    
    // Check if current volume is excessive
    volumeRatio := volume / avgVolume
    isExcessive := volumeRatio > vm.config.MaxVolumeMultiplier
    
    if isExcessive {
        log.Warn().
            Str("symbol", symbol).
            Float64("current_volume", volume).
            Float64("average_volume", avgVolume).
            Float64("volume_ratio", volumeRatio).
            Float64("threshold", vm.config.MaxVolumeMultiplier).
            Msg("Excessive trading volume detected")
    }
    
    return isExcessive, volumeRatio
}
```

### 5. Error Rate Monitoring

#### System Error Tracking:
```go
// internal/exec/error_monitor.go
type ErrorMonitor struct {
    config      ErrorConfig
    errorCounts map[string]*ErrorTracker
    mu          sync.RWMutex
}

type ErrorTracker struct {
    errors     []time.Time
    window     time.Duration
}

func (em *ErrorMonitor) RecordError(errorType string) {
    em.mu.Lock()
    defer em.mu.Unlock()
    
    tracker, exists := em.errorCounts[errorType]
    if !exists {
        tracker = &ErrorTracker{
            window: em.config.ErrorWindow,
        }
        em.errorCounts[errorType] = tracker
    }
    
    now := time.Now()
    tracker.errors = append(tracker.errors, now)
    
    // Remove old errors outside the window
    cutoff := now.Add(-tracker.window)
    for i := 0; i < len(tracker.errors); i++ {
        if tracker.errors[i].After(cutoff) {
            tracker.errors = tracker.errors[i:]
            break
        }
    }
}

func (em *ErrorMonitor) CheckErrorRate(errorType string) (bool, float64) {
    em.mu.RLock()
    defer em.mu.RUnlock()
    
    tracker, exists := em.errorCounts[errorType]
    if !exists {
        return false, 0
    }
    
    errorRate := float64(len(tracker.errors)) / em.config.ErrorWindow.Seconds()
    isExcessive := errorRate > em.config.MaxErrorRate
    
    if isExcessive {
        log.Warn().
            Str("error_type", errorType).
            Float64("error_rate", errorRate).
            Float64("threshold", em.config.MaxErrorRate).
            Msg("Excessive error rate detected")
    }
    
    return isExcessive, errorRate
}
```

### 6. Integration with Trading Engine

#### Circuit Breaker Integration:
```go
// internal/exec/executor.go
func (e *Exec) ExecuteStrategy(symbol string, strategy Strategy) error {
    // Check circuit breaker before trading
    if !e.circuitBreaker.CanTrade(symbol) {
        return fmt.Errorf("trading blocked by circuit breaker for symbol %s", symbol)
    }
    
    // Execute trading strategy
    err := strategy.Execute(symbol)
    
    // Record result with circuit breaker
    if err != nil {
        e.circuitBreaker.RecordFailure(fmt.Sprintf("strategy execution failed: %v", err))
        return err
    }
    
    e.circuitBreaker.RecordSuccess()
    return nil
}
```

## Configuration

### Circuit Breaker Settings:
```yaml
# config.yaml
circuitBreaker:
  enabled: true
  failureThreshold: 3
  recoveryTimeout: "5m"
  
  volatility:
    enabled: true
    maxVolatilityPercent: 5.0
    
  imbalance:
    enabled: true
    maxImbalanceRatio: 0.8
    
  volume:
    enabled: true
    volumeWindow: "10m"
    maxVolumeMultiplier: 5.0
    minSamples: 10
    
  errorRate:
    enabled: true
    errorWindow: "5m"
    maxErrorRate: 0.1  # 0.1 errors per second
```

## Monitoring and Alerting

### Circuit Breaker Metrics:
- Circuit breaker state changes
- Trigger frequency by condition type
- Recovery time and success rate
- False positive rate analysis

### Risk Dashboard:
- Real-time circuit breaker status
- Current risk factor levels
- Historical trigger patterns
- Recovery performance metrics

## Related ADRs
- ADR-006: Prometheus for Metrics and Monitoring
- ADR-008: Security-First Design with Multiple Layers
- ADR-010: Configuration-Driven Trading Strategies
