// Package cfg provides configuration management for the Bitunix trading bot.
// It supports loading configuration from both YAML files and environment variables,
// with environment variables taking precedence over YAML settings.
//
// The package handles validation of all configuration parameters and provides
// sensible defaults for optional settings. It supports both live trading and
// dry-run modes with appropriate safety checks.
package cfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"bitunix-bot/internal/common"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Settings contains all configuration parameters for the trading bot.
// It includes API credentials, trading parameters, feature calculation settings,
// ML model configuration, and system settings.
type Settings struct {
	// API Configuration
	Key    string // Bitunix API key for authentication
	Secret string // Bitunix API secret for request signing

	// Trading Configuration
	Symbols             []string // List of trading symbols (e.g., ["BTCUSDT", "ETHUSDT"])
	BaseSizeRatio       float64  // Base position size as ratio of account balance
	MaxPositionSize     float64  // Maximum position size per trade
	MaxPositionExposure float64  // Maximum total position exposure per symbol (as fraction of balance)
	MaxDailyLoss        float64  // Maximum daily loss limit (as fraction of balance)
	MaxPriceDistance    float64  // Maximum allowed price distance for trades
	DryRun              bool     // Whether to run in dry-run mode (no actual trades)

	// Exchange Configuration
	BaseURL string        // Base URL for REST API endpoints
	WsURL   string        // WebSocket URL for real-time data
	Ping    time.Duration // Ping interval for WebSocket connections

	// Feature Calculation Settings
	VWAPWindow time.Duration // Time window for VWAP calculations
	VWAPSize   int           // Number of samples for VWAP calculations
	TickSize   int           // Number of ticks for imbalance calculations

	// ML Model Configuration
	DataPath      string  // Path to data storage directory
	ModelPath     string  // Path to ONNX model file
	ProbThreshold float64 // Probability threshold for ML predictions

	// System Configuration
	MetricsPort    int           // Port for Prometheus metrics server
	RESTTimeout    time.Duration // Timeout for REST API requests
	InitialBalance float64       // Initial account balance for trading

	// Advanced Trading Configuration
	Leverage      int                     // Trading leverage multiplier
	MarginMode    string                  // Margin mode (e.g., "ISOLATED", "CROSS")
	RiskUSD       float64                 // Risk amount in USD
	SymbolConfigs map[string]SymbolConfig // Per-symbol configuration overrides

	// Circuit breaker settings
	CircuitBreakerVolatility   float64       // Volatility threshold for circuit breaker
	CircuitBreakerImbalance    float64       // Order book imbalance threshold for circuit breaker
	CircuitBreakerVolume       float64       // Volume threshold for circuit breaker
	CircuitBreakerErrorRate    float64       // Error rate threshold for circuit breaker
	CircuitBreakerRecoveryTime time.Duration // Recovery time after circuit breaker activation

	// Order execution timeout settings
	OrderExecutionTimeout    time.Duration // Maximum time to wait for order execution
	OrderStatusCheckInterval time.Duration // Interval for checking order status
	MaxOrderRetries          int           // Maximum number of order retry attempts

	// Maximum drawdown protection settings
	MaxDrawdownProtection float64 // Maximum drawdown threshold (as fraction of peak balance)
}

// SymbolConfig contains per-symbol configuration overrides.
// These settings allow different trading parameters for specific symbols,
// overriding the global configuration values.
type SymbolConfig struct {
	BaseSizeRatio       float64 `yaml:"baseSizeRatio"`       // Base position size ratio for this symbol
	MaxPositionSize     float64 `yaml:"maxPositionSize"`     // Maximum position size for this symbol
	MaxPositionExposure float64 `yaml:"maxPositionExposure"` // Maximum position exposure for this symbol
	MaxPriceDistance    float64 `yaml:"maxPriceDistance"`    // Maximum price distance for this symbol
}

// ConfigFile represents the structure of the YAML configuration file.
// It provides a hierarchical organization of configuration parameters
// that can be loaded from a YAML file and converted to Settings.
type ConfigFile struct {
	API struct {
		Key     string `yaml:"key"`
		Secret  string `yaml:"secret"`
		BaseURL string `yaml:"baseURL"`
		WsURL   string `yaml:"wsURL"`
	} `yaml:"api"`

	Trading struct {
		Symbols               []string `yaml:"symbols"`
		BaseSizeRatio         float64  `yaml:"baseSizeRatio"`
		MaxPositionSize       float64  `yaml:"maxPositionSize"`
		MaxPositionExposure   float64  `yaml:"maxPositionExposure"`
		MaxDailyLoss          float64  `yaml:"maxDailyLoss"`
		MaxDrawdownProtection float64  `yaml:"maxDrawdownProtection"`
		MaxPriceDistance      float64  `yaml:"maxPriceDistance"`
		DryRun                bool     `yaml:"dryRun"`
	} `yaml:"trading"`

	SymbolConfig map[string]SymbolConfig `yaml:"symbolConfig"`

	Features struct {
		VWAPWindow string `yaml:"vwapWindow"`
		VWAPSize   int    `yaml:"vwapSize"`
		TickSize   int    `yaml:"tickSize"`
	} `yaml:"features"`

	ML struct {
		ModelPath     string  `yaml:"modelPath"`
		ProbThreshold float64 `yaml:"probThreshold"`
	} `yaml:"ml"`

	System struct {
		DataPath     string `yaml:"dataPath"`
		PingInterval string `yaml:"pingInterval"`
		MetricsPort  int    `yaml:"metricsPort"`
		RESTTimeout  string `yaml:"restTimeout"`

		// Order execution timeout settings
		OrderExecutionTimeout    string `yaml:"orderExecutionTimeout"`
		OrderStatusCheckInterval string `yaml:"orderStatusCheckInterval"`
		MaxOrderRetries          int    `yaml:"maxOrderRetries"`
	} `yaml:"system"`

	CircuitBreaker struct {
		Volatility   float64 `yaml:"volatility"`
		Imbalance    float64 `yaml:"imbalance"`
		Volume       float64 `yaml:"volume"`
		ErrorRate    float64 `yaml:"errorRate"`
		RecoveryTime string  `yaml:"recoveryTime"`
	} `yaml:"circuitBreaker"`
}

// Load loads configuration from either a YAML file or environment variables.
// It first checks for a CONFIG_FILE environment variable to load from YAML,
// otherwise falls back to loading from environment variables.
// Returns a validated Settings struct or an error if configuration is invalid.
func Load() (Settings, error) {
	// Load .env file if it exists (ignore errors as it's optional)
	_ = godotenv.Load()

	// Try to load from YAML file first
	if configPath := os.Getenv("CONFIG_FILE"); configPath != "" {
		return loadFromYAML(configPath)
	}

	// Fallback to environment variables
	return loadFromEnv()
}

// loadFromYAML loads configuration from a YAML file at the specified path.
// It parses the YAML file, converts duration strings to time.Duration,
// applies environment variable overrides, and validates the final configuration.
func loadFromYAML(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config ConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Settings{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Parse durations
	vwapWindow, err := time.ParseDuration(config.Features.VWAPWindow)
	if err != nil {
		vwapWindow = 30 * time.Second
	}

	ping, err := time.ParseDuration(config.System.PingInterval)
	if err != nil {
		ping = 15 * time.Second
	}

	restTimeout, err := time.ParseDuration(config.System.RESTTimeout)
	if err != nil {
		restTimeout = 5 * time.Second
	}

	// Parse circuit breaker recovery time
	circuitBreakerRecoveryTime := 5 * time.Minute
	if config.CircuitBreaker.RecoveryTime != "" {
		if parsed, err := time.ParseDuration(config.CircuitBreaker.RecoveryTime); err == nil {
			circuitBreakerRecoveryTime = parsed
		}
	}

	// Parse order execution timeout settings
	orderExecutionTimeout := 30 * time.Second
	if config.System.OrderExecutionTimeout != "" {
		if parsed, err := time.ParseDuration(config.System.OrderExecutionTimeout); err == nil {
			orderExecutionTimeout = parsed
		}
	}

	orderStatusCheckInterval := 5 * time.Second
	if config.System.OrderStatusCheckInterval != "" {
		if parsed, err := time.ParseDuration(config.System.OrderStatusCheckInterval); err == nil {
			orderStatusCheckInterval = parsed
		}
	}

	maxOrderRetries := 3
	if config.System.MaxOrderRetries > 0 {
		maxOrderRetries = config.System.MaxOrderRetries
	}

	// Override with environment variables if they exist
	key := getEnvOrDefault(common.EnvBitunixAPIKey, config.API.Key)
	secret := getEnvOrDefault(common.EnvBitunixSecretKey, config.API.Secret)

	if key == "" || secret == "" {
		return Settings{}, fmt.Errorf(common.ErrMsgAPIKeyRequired)
	}

	settings := Settings{
		Key:                   key,
		Secret:                secret,
		Symbols:               getSymbolsFromEnvOrConfig(config.Trading.Symbols),
		BaseURL:               getEnvOrDefault(common.EnvBaseURL, config.API.BaseURL),
		WsURL:                 getEnvOrDefault(common.EnvWsURL, config.API.WsURL),
		Ping:                  ping,
		DataPath:              getEnvOrDefault(common.EnvDataPath, config.System.DataPath),
		ModelPath:             getEnvOrDefault(common.EnvModelPath, common.DefaultModelPath),
		VWAPWindow:            vwapWindow,
		VWAPSize:              getIntFromEnvOrConfig(common.EnvVWAPSize, config.Features.VWAPSize),
		TickSize:              getIntFromEnvOrConfig(common.EnvTickSize, config.Features.TickSize),
		BaseSizeRatio:         getFloatFromEnvOrConfig(common.EnvBaseSizeRatio, config.Trading.BaseSizeRatio),
		ProbThreshold:         getFloatFromEnvOrConfig(common.EnvProbThreshold, config.ML.ProbThreshold),
		DryRun:                getBoolFromEnvOrConfig(common.EnvDryRun, config.Trading.DryRun),
		MaxDailyLoss:          getFloatFromEnvOrConfig(common.EnvMaxDailyLoss, config.Trading.MaxDailyLoss),
		MetricsPort:           getIntFromEnvOrConfig(common.EnvMetricsPort, config.System.MetricsPort),
		MaxPositionSize:       getFloatFromEnvOrConfigWithDefault(common.EnvMaxPositionSize, config.Trading.MaxPositionSize, common.DefaultMaxPositionSize),
		MaxPositionExposure:   getFloatFromEnvOrConfigWithDefault(common.EnvMaxPositionExposure, config.Trading.MaxPositionExposure, common.DefaultMaxPositionExposure),
		MaxDrawdownProtection: getFloatFromEnvOrConfigWithDefault(common.EnvMaxDrawdownProtection, config.Trading.MaxDrawdownProtection, common.DefaultMaxDrawdownProtection),
		MaxPriceDistance:      getFloatFromEnvOrConfigWithDefault(common.EnvMaxPriceDistance, config.Trading.MaxPriceDistance, common.DefaultMaxPriceDistance),
		Leverage:              getIntOrDefault(common.EnvLeverage, common.DefaultLeverage),
		MarginMode:            getEnvOrDefault(common.EnvMarginMode, common.DefaultMarginMode),
		RiskUSD:               getFloatOrDefault(common.EnvRiskUSD, common.DefaultRiskUSD),
		SymbolConfigs:         config.SymbolConfig,
		RESTTimeout:           restTimeout,
		// Circuit breaker settings with defaults from YAML or environment
		CircuitBreakerVolatility:   getFloatFromEnvOrConfigWithDefault(common.EnvCircuitBreakerVolatility, config.CircuitBreaker.Volatility, common.DefaultCircuitBreakerVolatility),
		CircuitBreakerImbalance:    getFloatFromEnvOrConfigWithDefault(common.EnvCircuitBreakerImbalance, config.CircuitBreaker.Imbalance, common.DefaultCircuitBreakerImbalance),
		CircuitBreakerVolume:       getFloatFromEnvOrConfigWithDefault(common.EnvCircuitBreakerVolume, config.CircuitBreaker.Volume, common.DefaultCircuitBreakerVolume),
		CircuitBreakerErrorRate:    getFloatFromEnvOrConfigWithDefault(common.EnvCircuitBreakerErrorRate, config.CircuitBreaker.ErrorRate, common.DefaultCircuitBreakerErrorRate),
		CircuitBreakerRecoveryTime: getDurationOrDefault(common.EnvCircuitBreakerRecovery, circuitBreakerRecoveryTime),
		// Order execution timeout settings
		OrderExecutionTimeout:    getDurationOrDefault(common.EnvOrderExecutionTimeout, orderExecutionTimeout),
		OrderStatusCheckInterval: getDurationOrDefault(common.EnvOrderStatusCheckInterval, orderStatusCheckInterval),
		MaxOrderRetries:          getIntFromEnvOrConfig(common.EnvMaxOrderRetries, maxOrderRetries),
	}

	// Validate configuration
	if err := validateSettings(&settings); err != nil {
		return Settings{}, fmt.Errorf("configuration validation failed: %w", err)
	}

	return settings, nil
}

// loadFromEnv loads configuration entirely from environment variables.
// It uses default values for any missing optional parameters and validates
// the final configuration before returning.
func loadFromEnv() (Settings, error) {
	key, err := getEnvRequired(common.EnvBitunixAPIKey)
	if err != nil {
		return Settings{}, err
	}

	secret, err := getEnvRequired(common.EnvBitunixSecretKey)
	if err != nil {
		return Settings{}, err
	}

	settings := Settings{
		Key:                   key,
		Secret:                secret,
		Symbols:               splitOrDefault(os.Getenv(common.EnvSymbols), []string{common.BTCUSDTSymbol}),
		BaseURL:               getEnvOrDefault(common.EnvBaseURL, common.DefaultBaseURL),
		WsURL:                 getEnvOrDefault(common.EnvWsURL, common.DefaultWsURL),
		Ping:                  getDurationOrDefault(common.EnvPingInterval, 15*time.Second),
		DataPath:              os.Getenv(common.EnvDataPath), // optional
		ModelPath:             getEnvOrDefault(common.EnvModelPath, common.DefaultModelPath),
		VWAPWindow:            getDurationOrDefault(common.EnvVWAPWindow, 30*time.Second),
		VWAPSize:              getIntOrDefault(common.EnvVWAPSize, common.DefaultVWAPSize),
		TickSize:              getIntOrDefault(common.EnvTickSize, common.DefaultTickSize),
		BaseSizeRatio:         getFloatOrDefault(common.EnvBaseSizeRatio, common.DefaultBaseSizeRatio),
		ProbThreshold:         getFloatOrDefault(common.EnvProbThreshold, common.DefaultProbThreshold),
		DryRun:                getBoolOrDefault(common.EnvDryRun, false),
		MaxDailyLoss:          getFloatOrDefault(common.EnvMaxDailyLoss, common.DefaultMaxDailyLoss),
		MetricsPort:           getIntOrDefault(common.EnvMetricsPort, common.DefaultMetricsPort),
		MaxPositionSize:       getFloatOrDefault(common.EnvMaxPositionSize, common.DefaultMaxPositionSize),
		MaxPositionExposure:   getFloatOrDefault(common.EnvMaxPositionExposure, common.DefaultMaxPositionExposure),
		MaxDrawdownProtection: getFloatOrDefault(common.EnvMaxDrawdownProtection, common.DefaultMaxDrawdownProtection),
		MaxPriceDistance:      getFloatOrDefault(common.EnvMaxPriceDistance, common.DefaultMaxPriceDistance),
		Leverage:              getIntOrDefault(common.EnvLeverage, common.DefaultLeverage),
		MarginMode:            getEnvOrDefault(common.EnvMarginMode, common.DefaultMarginMode),
		RiskUSD:               getFloatOrDefault(common.EnvRiskUSD, common.DefaultRiskUSD),
		SymbolConfigs:         make(map[string]SymbolConfig),
		RESTTimeout:           getDurationOrDefault(common.EnvRESTTimeout, 5*time.Second),
		// Circuit breaker settings with defaults
		CircuitBreakerVolatility:   getFloatOrDefault(common.EnvCircuitBreakerVolatility, common.DefaultCircuitBreakerVolatility),
		CircuitBreakerImbalance:    getFloatOrDefault(common.EnvCircuitBreakerImbalance, common.DefaultCircuitBreakerImbalance),
		CircuitBreakerVolume:       getFloatOrDefault(common.EnvCircuitBreakerVolume, common.DefaultCircuitBreakerVolume),
		CircuitBreakerErrorRate:    getFloatOrDefault(common.EnvCircuitBreakerErrorRate, common.DefaultCircuitBreakerErrorRate),
		CircuitBreakerRecoveryTime: getDurationOrDefault(common.EnvCircuitBreakerRecovery, 5*time.Minute),
		// Order execution timeout settings
		OrderExecutionTimeout:    getDurationOrDefault(common.EnvOrderExecutionTimeout, 30*time.Second),
		OrderStatusCheckInterval: getDurationOrDefault(common.EnvOrderStatusCheckInterval, 5*time.Second),
		MaxOrderRetries:          getIntOrDefault(common.EnvMaxOrderRetries, 3),
	}

	// Validate configuration
	if err := validateSettings(&settings); err != nil {
		return Settings{}, fmt.Errorf("configuration validation failed: %w", err)
	}

	return settings, nil
}

// GetSymbolConfig returns configuration for a specific symbol, with fallback to global config.
// If a symbol-specific configuration exists, it returns that; otherwise, it returns
// a SymbolConfig populated with global configuration values.
func (s *Settings) GetSymbolConfig(symbol string) SymbolConfig {
	if config, exists := s.SymbolConfigs[symbol]; exists {
		return config
	}

	// Return default config
	return SymbolConfig{
		BaseSizeRatio:       s.BaseSizeRatio,
		MaxPositionSize:     s.MaxPositionSize,
		MaxPositionExposure: s.MaxPositionExposure,
		MaxPriceDistance:    s.MaxPriceDistance,
	}
}

func getEnvRequired(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is missing", key)
	}
	return v, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func getIntOrDefault(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

func getFloatOrDefault(key string, defaultValue float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getBoolOrDefault(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultValue
}

func splitOrDefault(v string, def []string) []string {
	if v == "" {
		return def
	}
	return strings.Split(v, ",")
}

func getSymbolsFromEnvOrConfig(configSymbols []string) []string {
	if env := os.Getenv(common.EnvSymbols); env != "" {
		return strings.Split(env, ",")
	}
	if len(configSymbols) > 0 {
		return configSymbols
	}
	return []string{common.BTCUSDTSymbol}
}

func getIntFromEnvOrConfig(key string, configValue int) int {
	if env := os.Getenv(key); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			return val
		}
	}
	if configValue != 0 {
		return configValue
	}
	return getIntOrDefault(key, 0)
}

func getFloatFromEnvOrConfig(key string, configValue float64) float64 {
	if env := os.Getenv(key); env != "" {
		if val, err := strconv.ParseFloat(env, 64); err == nil {
			return val
		}
	}
	if configValue != 0 {
		return configValue
	}
	return getFloatOrDefault(key, 0)
}

func getBoolFromEnvOrConfig(key string, configValue bool) bool {
	if env := os.Getenv(key); env != "" {
		if val, err := strconv.ParseBool(env); err == nil {
			return val
		}
	}
	return configValue
}

func getFloatFromEnvOrConfigWithDefault(key string, configValue, defaultValue float64) float64 {
	if env := os.Getenv(key); env != "" {
		if val, err := strconv.ParseFloat(env, 64); err == nil {
			return val
		}
	}
	if configValue != 0 {
		return configValue
	}
	return defaultValue
}

// validateSettings performs comprehensive validation of configuration values
func validateSettings(s *Settings) error {
	if err := validateCredentials(s); err != nil {
		return err
	}

	if err := validateURLs(s); err != nil {
		return err
	}

	if err := validateTradingParameters(s); err != nil {
		return err
	}

	if err := validateLiveTradingRestrictions(s); err != nil {
		return err
	}

	if err := validateMLParameters(s); err != nil {
		return err
	}

	if err := validateSystemParameters(s); err != nil {
		return err
	}

	if err := validateSymbolConfigs(s); err != nil {
		return err
	}

	if err := validateCircuitBreakerSettings(s); err != nil {
		return err
	}

	if err := validateOrderExecutionSettings(s); err != nil {
		return err
	}

	return nil
}

// validateCredentials validates API credentials
func validateCredentials(s *Settings) error {
	if s.Key == "" || s.Secret == "" {
		return fmt.Errorf(common.ErrMsgAPIKeyRequired)
	}
	return nil
}

// validateURLs validates required URL configurations
func validateURLs(s *Settings) error {
	if s.BaseURL == "" {
		return fmt.Errorf(common.ErrMsgBaseURLRequired)
	}
	if s.WsURL == "" {
		return fmt.Errorf(common.ErrMsgWsURLRequired)
	}
	return nil
}

// validateTradingParameters validates core trading parameters
func validateTradingParameters(s *Settings) error {
	if len(s.Symbols) == 0 {
		return fmt.Errorf(common.ErrMsgSymbolRequired)
	}
	if s.BaseSizeRatio <= 0 || s.BaseSizeRatio > common.MaxBaseSizeRatio {
		return fmt.Errorf("baseSizeRatio must be between 0 and %g", common.MaxBaseSizeRatio)
	}
	if s.MaxPositionSize <= 0 || s.MaxPositionSize > common.MaxPositionSizeLimit {
		return fmt.Errorf("maxPositionSize must be between 0 and %g", common.MaxPositionSizeLimit)
	}
	if s.MaxPositionExposure <= 0 || s.MaxPositionExposure > common.MaxPositionSizeLimit {
		return fmt.Errorf("maxPositionExposure must be between 0 and %g", common.MaxPositionSizeLimit)
	}
	if s.MaxDailyLoss <= 0 || s.MaxDailyLoss > common.MaxDailyLossLimit {
		return fmt.Errorf("maxDailyLoss must be between 0 and %g", common.MaxDailyLossLimit)
	}
	if s.MaxDrawdownProtection <= 0 || s.MaxDrawdownProtection > common.MaxDailyLossLimit {
		return fmt.Errorf("maxDrawdownProtection must be between 0 and %g", common.MaxDailyLossLimit)
	}
	if s.MaxPriceDistance <= 0 {
		return fmt.Errorf("maxPriceDistance must be positive")
	}
	return nil
}

// validateLiveTradingRestrictions validates additional restrictions for live trading
func validateLiveTradingRestrictions(s *Settings) error {
	if !s.DryRun {
		// Check for explicit environment variable override
		if os.Getenv(common.EnvForceLiveTrading) != "true" {
			return fmt.Errorf(common.ErrMsgForceLiveTradingRequired)
		}

		// Additional validation for live trading
		if s.MaxPositionSize > common.MaxPositionSizeLive {
			return fmt.Errorf("maxPositionSize too high for live trading (max %g%%)", common.MaxPositionSizeLive*100)
		}
		if s.MaxDailyLoss > common.MaxDailyLossLive {
			return fmt.Errorf("maxDailyLoss too high for live trading (max %g%%)", common.MaxDailyLossLive*100)
		}
	}
	return nil
}

// validateMLParameters validates machine learning related parameters
func validateMLParameters(s *Settings) error {
	if s.ProbThreshold < common.MinProbThreshold || s.ProbThreshold > common.MaxProbThreshold {
		return fmt.Errorf("probThreshold must be between %g and %g", common.MinProbThreshold, common.MaxProbThreshold)
	}
	return nil
}

// validateSystemParameters validates system-level parameters
func validateSystemParameters(s *Settings) error {
	if s.Ping < 1*time.Second || s.Ping > 5*time.Minute {
		return fmt.Errorf("pingInterval must be between 1s and 5m")
	}
	if s.VWAPWindow < 1*time.Second || s.VWAPWindow > 1*time.Hour {
		return fmt.Errorf("vwapWindow must be between 1s and 1h")
	}
	if s.RESTTimeout < 1*time.Second || s.RESTTimeout > 1*time.Minute {
		return fmt.Errorf("restTimeout must be between 1s and 1m")
	}
	if s.MetricsPort < common.MinMetricsPort || s.MetricsPort > common.MaxMetricsPort {
		return fmt.Errorf("metricsPort must be between %d and %d", common.MinMetricsPort, common.MaxMetricsPort)
	}
	if s.VWAPSize < common.MinVWAPSize || s.VWAPSize > common.MaxVWAPSize {
		return fmt.Errorf("vwapSize must be between %d and %d", common.MinVWAPSize, common.MaxVWAPSize)
	}
	return nil
}

// validateSymbolConfigs validates per-symbol configuration overrides
func validateSymbolConfigs(s *Settings) error {
	for symbol, sc := range s.SymbolConfigs {
		if sc.BaseSizeRatio <= 0 || sc.BaseSizeRatio > common.MaxBaseSizeRatio {
			return fmt.Errorf("symbol %s: baseSizeRatio must be between 0 and %g", symbol, common.MaxBaseSizeRatio)
		}
		if sc.MaxPositionSize <= 0 || sc.MaxPositionSize > common.MaxPositionSizeLimit {
			return fmt.Errorf("symbol %s: maxPositionSize must be between 0 and %g", symbol, common.MaxPositionSizeLimit)
		}
		if sc.MaxPositionExposure <= 0 || sc.MaxPositionExposure > common.MaxPositionSizeLimit {
			return fmt.Errorf("symbol %s: maxPositionExposure must be between 0 and %g", symbol, common.MaxPositionSizeLimit)
		}
		if sc.MaxPriceDistance <= 0 {
			return fmt.Errorf("symbol %s: maxPriceDistance must be positive", symbol)
		}
	}
	return nil
}

// validateCircuitBreakerSettings validates circuit breaker configuration
func validateCircuitBreakerSettings(s *Settings) error {
	if s.CircuitBreakerVolatility <= 0 {
		return fmt.Errorf("circuitBreakerVolatility must be positive")
	}
	if s.CircuitBreakerImbalance <= 0 || s.CircuitBreakerImbalance > 1 {
		return fmt.Errorf("circuitBreakerImbalance must be between 0 and 1")
	}
	if s.CircuitBreakerVolume <= 0 {
		return fmt.Errorf("circuitBreakerVolume must be positive")
	}
	if s.CircuitBreakerErrorRate <= 0 || s.CircuitBreakerErrorRate > 1 {
		return fmt.Errorf("circuitBreakerErrorRate must be between 0 and 1")
	}
	if s.CircuitBreakerRecoveryTime < 1*time.Minute || s.CircuitBreakerRecoveryTime > 24*time.Hour {
		return fmt.Errorf("circuitBreakerRecoveryTime must be between 1m and 24h")
	}
	return nil
}

// validateOrderExecutionSettings validates order execution timeout settings
func validateOrderExecutionSettings(s *Settings) error {
	if s.OrderExecutionTimeout < 10*time.Second || s.OrderExecutionTimeout > 5*time.Minute {
		return fmt.Errorf("orderExecutionTimeout must be between 10s and 5m")
	}
	if s.OrderStatusCheckInterval < 1*time.Second || s.OrderStatusCheckInterval > 30*time.Second {
		return fmt.Errorf("orderStatusCheckInterval must be between 1s and 30s")
	}
	if s.MaxOrderRetries < 1 || s.MaxOrderRetries > 10 {
		return fmt.Errorf("maxOrderRetries must be between 1 and 10")
	}
	return nil
}
