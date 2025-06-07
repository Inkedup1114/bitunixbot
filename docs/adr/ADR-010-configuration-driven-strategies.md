# ADR-010: Configuration-Driven Trading Strategies

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires flexible trading strategy management to support:
- **Multiple Strategies**: OVIR-X, Mean Reversion, and future strategy additions
- **Dynamic Configuration**: Runtime parameter adjustments without code changes
- **Symbol-Specific Settings**: Different parameters for different trading pairs
- **Environment Variations**: Different settings for development, staging, and production
- **Risk Management**: Configurable limits and safety parameters
- **A/B Testing**: Ability to test different strategy configurations
- **Operational Flexibility**: Quick strategy adjustments based on market conditions

Strategy management approaches considered:
- **Hard-coded Strategies**: Simple but inflexible and requires redeployment
- **Plugin Architecture**: Flexible but complex with dynamic loading
- **Configuration-Driven**: Balance of flexibility and simplicity
- **Database-Driven**: Dynamic but adds infrastructure complexity
- **Hybrid Approach**: Configuration for parameters, code for logic

## Decision

We chose a **Configuration-Driven Trading Strategy** approach with YAML-based configuration and runtime parameter management.

### Architecture:
1. **Strategy Interface**: Common interface for all trading strategies
2. **Configuration Schema**: Structured YAML configuration for all parameters
3. **Runtime Updates**: Hot-reloading of configuration without restart
4. **Symbol-Specific Overrides**: Per-symbol parameter customization
5. **Environment Profiles**: Environment-specific configuration profiles
6. **Validation Framework**: Comprehensive parameter validation and constraints

### Key Principles:
- **Separation of Concerns**: Strategy logic separate from parameters
- **Type Safety**: Strong typing for all configuration parameters
- **Validation**: Comprehensive validation of all parameters
- **Flexibility**: Easy parameter adjustments without code changes
- **Auditability**: Complete audit trail of configuration changes

## Consequences

### Positive:
- **Operational Flexibility**: Quick parameter adjustments for market conditions
- **A/B Testing**: Easy testing of different parameter combinations
- **Environment Management**: Clean separation between environments
- **Risk Management**: Centralized configuration of all risk parameters
- **Deployment Safety**: Parameter changes without code deployment
- **Strategy Development**: Rapid iteration on strategy parameters
- **Monitoring**: Clear visibility into active strategy configurations

### Negative:
- **Configuration Complexity**: Large configuration files can become unwieldy
- **Validation Overhead**: Need comprehensive validation for all parameters
- **Runtime Errors**: Invalid configurations may cause runtime failures
- **Documentation Burden**: Need to maintain configuration documentation

### Mitigations:
- **Schema Validation**: JSON Schema validation for configuration files
- **Default Values**: Sensible defaults for all optional parameters
- **Configuration Testing**: Automated testing of configuration validity
- **Documentation**: Auto-generated documentation from configuration schema

## Implementation Details

### 1. Strategy Interface

#### Common Strategy Interface:
```go
// internal/exec/strategy.go
type Strategy interface {
    Name() string
    Execute(symbol string, features []float32, prediction float32) (*TradeDecision, error)
    Configure(config StrategyConfig) error
    Validate() error
    GetMetrics() StrategyMetrics
}

type TradeDecision struct {
    Action    TradeAction // BUY, SELL, HOLD
    Size      float64     // Position size
    Price     float64     // Target price (optional)
    Reason    string      // Decision rationale
    Confidence float64    // Decision confidence (0-1)
}

type StrategyConfig struct {
    Name       string                 `yaml:"name"`
    Enabled    bool                   `yaml:"enabled"`
    Parameters map[string]interface{} `yaml:"parameters"`
    RiskLimits RiskLimits            `yaml:"riskLimits"`
}

type RiskLimits struct {
    MaxPositionSize     float64 `yaml:"maxPositionSize"`
    MaxDailyLoss        float64 `yaml:"maxDailyLoss"`
    MaxPriceDistance    float64 `yaml:"maxPriceDistance"`
    MinConfidenceLevel  float64 `yaml:"minConfidenceLevel"`
}
```

### 2. Configuration Schema

#### Comprehensive Configuration Structure:
```yaml
# config.yaml
# Trading Strategy Configuration
strategies:
  ovirx:
    enabled: true
    parameters:
      probThreshold: 0.65
      baseSizeRatio: 0.002
      maxPriceDistance: 3.0
      minVolumeThreshold: 1000.0
      cooldownPeriod: "30s"
    riskLimits:
      maxPositionSize: 0.01
      maxDailyLoss: 0.05
      maxPriceDistance: 3.0
      minConfidenceLevel: 0.6
      
  meanReversion:
    enabled: false
    parameters:
      reversionThreshold: 2.0
      holdingPeriod: "5m"
      exitThreshold: 0.5
      volumeFilter: true
    riskLimits:
      maxPositionSize: 0.005
      maxDailyLoss: 0.03
      maxPriceDistance: 2.0
      minConfidenceLevel: 0.7

# Symbol-specific configuration overrides
symbolConfig:
  BTCUSDT:
    strategies:
      ovirx:
        parameters:
          baseSizeRatio: 0.001
          probThreshold: 0.7
        riskLimits:
          maxPositionSize: 0.015
  ETHUSDT:
    strategies:
      ovirx:
        parameters:
          baseSizeRatio: 0.002
          probThreshold: 0.65
        riskLimits:
          maxPositionSize: 0.01

# Environment-specific profiles
profiles:
  development:
    strategies:
      ovirx:
        parameters:
          baseSizeRatio: 0.0001  # Smaller sizes for testing
        riskLimits:
          maxDailyLoss: 0.01     # Lower risk for development
  production:
    strategies:
      ovirx:
        parameters:
          baseSizeRatio: 0.002
        riskLimits:
          maxDailyLoss: 0.05
```

### 3. OVIR-X Strategy Implementation

#### Configuration-Driven OVIR-X Strategy:
```go
// internal/exec/ovirx_strategy.go
type OVIRXStrategy struct {
    config OVIRXConfig
    mu     sync.RWMutex
}

type OVIRXConfig struct {
    ProbThreshold      float64       `yaml:"probThreshold" validate:"min=0,max=1"`
    BaseSizeRatio      float64       `yaml:"baseSizeRatio" validate:"min=0,max=1"`
    MaxPriceDistance   float64       `yaml:"maxPriceDistance" validate:"min=0"`
    MinVolumeThreshold float64       `yaml:"minVolumeThreshold" validate:"min=0"`
    CooldownPeriod     time.Duration `yaml:"cooldownPeriod"`
    RiskLimits         RiskLimits    `yaml:"riskLimits"`
}

func (s *OVIRXStrategy) Configure(config StrategyConfig) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Parse strategy-specific configuration
    var ovirxConfig OVIRXConfig
    if err := mapstructure.Decode(config.Parameters, &ovirxConfig); err != nil {
        return fmt.Errorf("failed to decode OVIR-X config: %w", err)
    }
    
    // Apply risk limits
    ovirxConfig.RiskLimits = config.RiskLimits
    
    // Validate configuration
    if err := s.validateConfig(ovirxConfig); err != nil {
        return fmt.Errorf("invalid OVIR-X config: %w", err)
    }
    
    s.config = ovirxConfig
    return nil
}

func (s *OVIRXStrategy) Execute(symbol string, features []float32, prediction float32) (*TradeDecision, error) {
    s.mu.RLock()
    config := s.config
    s.mu.RUnlock()
    
    // Check ML prediction threshold
    if prediction < config.ProbThreshold {
        return &TradeDecision{
            Action: HOLD,
            Reason: fmt.Sprintf("ML prediction %.3f below threshold %.3f", prediction, config.ProbThreshold),
        }, nil
    }
    
    // Extract features
    vwap := float64(features[0])
    imbalance := float64(features[1])
    volume := float64(features[2])
    
    // Check volume threshold
    if volume < config.MinVolumeThreshold {
        return &TradeDecision{
            Action: HOLD,
            Reason: fmt.Sprintf("Volume %.0f below threshold %.0f", volume, config.MinVolumeThreshold),
        }, nil
    }
    
    // Determine trade direction based on imbalance
    var action TradeAction
    if imbalance > 0 {
        action = BUY
    } else {
        action = SELL
    }
    
    // Calculate position size
    size := config.BaseSizeRatio
    
    // Apply risk limits
    if size > config.RiskLimits.MaxPositionSize {
        size = config.RiskLimits.MaxPositionSize
    }
    
    return &TradeDecision{
        Action:     action,
        Size:       size,
        Reason:     fmt.Sprintf("OVIR-X signal: prediction=%.3f, imbalance=%.3f", prediction, imbalance),
        Confidence: float64(prediction),
    }, nil
}
```

### 4. Configuration Management

#### Configuration Loader with Validation:
```go
// internal/cfg/strategy_config.go
type StrategyConfigManager struct {
    configPath    string
    strategies    map[string]Strategy
    symbolConfigs map[string]SymbolConfig
    mu            sync.RWMutex
    validator     *validator.Validate
}

func NewStrategyConfigManager(configPath string) *StrategyConfigManager {
    return &StrategyConfigManager{
        configPath: configPath,
        strategies: make(map[string]Strategy),
        symbolConfigs: make(map[string]SymbolConfig),
        validator:  validator.New(),
    }
}

func (scm *StrategyConfigManager) LoadConfiguration() error {
    scm.mu.Lock()
    defer scm.mu.Unlock()
    
    // Read configuration file
    data, err := os.ReadFile(scm.configPath)
    if err != nil {
        return fmt.Errorf("failed to read config file: %w", err)
    }
    
    var config Config
    if err := yaml.Unmarshal(data, &config); err != nil {
        return fmt.Errorf("failed to parse config: %w", err)
    }
    
    // Validate configuration
    if err := scm.validator.Struct(config); err != nil {
        return fmt.Errorf("config validation failed: %w", err)
    }
    
    // Apply environment profile
    if profile, exists := config.Profiles[os.Getenv("ENVIRONMENT")]; exists {
        config = scm.mergeProfile(config, profile)
    }
    
    // Configure strategies
    for name, strategyConfig := range config.Strategies {
        if strategy, exists := scm.strategies[name]; exists {
            if err := strategy.Configure(strategyConfig); err != nil {
                return fmt.Errorf("failed to configure strategy %s: %w", name, err)
            }
        }
    }
    
    // Store symbol-specific configurations
    scm.symbolConfigs = config.SymbolConfig
    
    return nil
}

func (scm *StrategyConfigManager) GetStrategyConfig(strategyName, symbol string) (StrategyConfig, error) {
    scm.mu.RLock()
    defer scm.mu.RUnlock()
    
    // Start with base strategy configuration
    baseConfig, exists := scm.strategies[strategyName]
    if !exists {
        return StrategyConfig{}, fmt.Errorf("strategy %s not found", strategyName)
    }
    
    // Apply symbol-specific overrides
    if symbolConfig, exists := scm.symbolConfigs[symbol]; exists {
        if strategyOverride, exists := symbolConfig.Strategies[strategyName]; exists {
            return scm.mergeConfigs(baseConfig, strategyOverride), nil
        }
    }
    
    return baseConfig, nil
}
```

### 5. Hot Configuration Reloading

#### Runtime Configuration Updates:
```go
// internal/cfg/hot_reload.go
type ConfigWatcher struct {
    configManager *StrategyConfigManager
    watcher       *fsnotify.Watcher
    done          chan bool
}

func NewConfigWatcher(configManager *StrategyConfigManager) (*ConfigWatcher, error) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }
    
    return &ConfigWatcher{
        configManager: configManager,
        watcher:       watcher,
        done:          make(chan bool),
    }, nil
}

func (cw *ConfigWatcher) Start() error {
    if err := cw.watcher.Add(cw.configManager.configPath); err != nil {
        return err
    }
    
    go cw.watchLoop()
    return nil
}

func (cw *ConfigWatcher) watchLoop() {
    for {
        select {
        case event := <-cw.watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                log.Info().Str("file", event.Name).Msg("Configuration file changed, reloading...")
                
                if err := cw.configManager.LoadConfiguration(); err != nil {
                    log.Error().Err(err).Msg("Failed to reload configuration")
                } else {
                    log.Info().Msg("Configuration reloaded successfully")
                }
            }
        case err := <-cw.watcher.Errors:
            log.Error().Err(err).Msg("Configuration watcher error")
        case <-cw.done:
            return
        }
    }
}
```

### 6. Configuration Validation

#### Comprehensive Parameter Validation:
```go
// internal/cfg/validation.go
func (scm *StrategyConfigManager) validateStrategyConfig(config StrategyConfig) error {
    // Validate required fields
    if config.Name == "" {
        return fmt.Errorf("strategy name is required")
    }
    
    // Validate risk limits
    if err := scm.validateRiskLimits(config.RiskLimits); err != nil {
        return fmt.Errorf("invalid risk limits: %w", err)
    }
    
    // Strategy-specific validation
    switch config.Name {
    case "ovirx":
        return scm.validateOVIRXConfig(config.Parameters)
    case "meanReversion":
        return scm.validateMeanReversionConfig(config.Parameters)
    default:
        return fmt.Errorf("unknown strategy: %s", config.Name)
    }
}

func (scm *StrategyConfigManager) validateRiskLimits(limits RiskLimits) error {
    if limits.MaxPositionSize <= 0 || limits.MaxPositionSize > 1 {
        return fmt.Errorf("maxPositionSize must be between 0 and 1")
    }
    
    if limits.MaxDailyLoss <= 0 || limits.MaxDailyLoss > 1 {
        return fmt.Errorf("maxDailyLoss must be between 0 and 1")
    }
    
    if limits.MaxPriceDistance <= 0 {
        return fmt.Errorf("maxPriceDistance must be positive")
    }
    
    if limits.MinConfidenceLevel < 0 || limits.MinConfidenceLevel > 1 {
        return fmt.Errorf("minConfidenceLevel must be between 0 and 1")
    }
    
    return nil
}
```

## Configuration Documentation

### Auto-generated Schema Documentation:
```go
// Generate JSON schema from Go structs
func GenerateConfigSchema() {
    schema := jsonschema.Reflect(&Config{})
    schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
    os.WriteFile("docs/config-schema.json", schemaJSON, 0644)
}
```

### Configuration Examples:
- Development configuration with safe defaults
- Staging configuration for testing
- Production configuration with optimized parameters
- Symbol-specific configuration examples

## Monitoring and Metrics

### Configuration Metrics:
- Configuration reload frequency and success rate
- Active strategy configurations by symbol
- Parameter change audit trail
- Configuration validation errors

### Strategy Performance by Configuration:
- P&L by strategy configuration
- Win rate by parameter combination
- Risk metrics by configuration profile

## Related ADRs
- ADR-004: Microservices Architecture with Internal Packages
- ADR-008: Security-First Design with Multiple Layers
- ADR-009: Circuit Breaker Pattern for Risk Management
