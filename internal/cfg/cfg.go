package cfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Settings struct {
	Key, Secret      string
	Symbols          []string
	BaseURL          string
	WsURL            string
	Ping             time.Duration
	DataPath         string
	ModelPath        string
	VWAPWindow       time.Duration
	VWAPSize         int
	TickSize         int
	BaseSizeRatio    float64
	ProbThreshold    float64
	DryRun           bool
	MaxDailyLoss     float64
	MetricsPort      int
	MaxPositionSize  float64
	MaxPriceDistance float64
	SymbolConfigs    map[string]SymbolConfig
	RESTTimeout      time.Duration
}

type SymbolConfig struct {
	BaseSizeRatio    float64 `yaml:"baseSizeRatio"`
	MaxPositionSize  float64 `yaml:"maxPositionSize"`
	MaxPriceDistance float64 `yaml:"maxPriceDistance"`
}

type ConfigFile struct {
	API struct {
		Key     string `yaml:"key"`
		Secret  string `yaml:"secret"`
		BaseURL string `yaml:"baseURL"`
		WsURL   string `yaml:"wsURL"`
	} `yaml:"api"`

	Trading struct {
		Symbols          []string `yaml:"symbols"`
		BaseSizeRatio    float64  `yaml:"baseSizeRatio"`
		MaxPositionSize  float64  `yaml:"maxPositionSize"`
		MaxDailyLoss     float64  `yaml:"maxDailyLoss"`
		MaxPriceDistance float64  `yaml:"maxPriceDistance"`
		DryRun           bool     `yaml:"dryRun"`
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
	} `yaml:"system"`
}

func Load() (Settings, error) {
	// Try to load from YAML file first
	if configPath := os.Getenv("CONFIG_FILE"); configPath != "" {
		return loadFromYAML(configPath)
	}

	// Fallback to environment variables
	return loadFromEnv()
}

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

	// Override with environment variables if they exist
	key := getEnvOrDefault("BITUNIX_API_KEY", config.API.Key)
	secret := getEnvOrDefault("BITUNIX_SECRET_KEY", config.API.Secret)

	if key == "" || secret == "" {
		return Settings{}, fmt.Errorf("API key and secret are required")
	}

	settings := Settings{
		Key:              key,
		Secret:           secret,
		Symbols:          getSymbolsFromEnvOrConfig(config.Trading.Symbols),
		BaseURL:          getEnvOrDefault("BASE_URL", config.API.BaseURL),
		WsURL:            getEnvOrDefault("WS_URL", config.API.WsURL),
		Ping:             ping,
		DataPath:         getEnvOrDefault("DATA_PATH", config.System.DataPath),
		ModelPath:        getEnvOrDefault("MODEL_PATH", config.ML.ModelPath),
		VWAPWindow:       vwapWindow,
		VWAPSize:         getIntFromEnvOrConfig("VWAP_SIZE", config.Features.VWAPSize),
		TickSize:         getIntFromEnvOrConfig("TICK_SIZE", config.Features.TickSize),
		BaseSizeRatio:    getFloatFromEnvOrConfig("BASE_SIZE_RATIO", config.Trading.BaseSizeRatio),
		ProbThreshold:    getFloatFromEnvOrConfig("PROB_THRESHOLD", config.ML.ProbThreshold),
		DryRun:           getBoolFromEnvOrConfig("DRY_RUN", config.Trading.DryRun),
		MaxDailyLoss:     getFloatFromEnvOrConfig("MAX_DAILY_LOSS", config.Trading.MaxDailyLoss),
		MetricsPort:      getIntFromEnvOrConfig("METRICS_PORT", config.System.MetricsPort),
		MaxPositionSize:  getFloatFromEnvOrConfig("MAX_POSITION_SIZE", config.Trading.MaxPositionSize),
		MaxPriceDistance: getFloatFromEnvOrConfig("MAX_PRICE_DISTANCE", config.Trading.MaxPriceDistance),
		SymbolConfigs:    config.SymbolConfig,
		RESTTimeout:      restTimeout,
	}

	// Validate configuration
	if err := validateSettings(&settings); err != nil {
		return Settings{}, fmt.Errorf("configuration validation failed: %w", err)
	}

	return settings, nil
}

func loadFromEnv() (Settings, error) {
	key, err := getEnvRequired("BITUNIX_API_KEY")
	if err != nil {
		return Settings{}, err
	}

	secret, err := getEnvRequired("BITUNIX_SECRET_KEY")
	if err != nil {
		return Settings{}, err
	}

	settings := Settings{
		Key:              key,
		Secret:           secret,
		Symbols:          splitOrDefault(os.Getenv("SYMBOLS"), []string{"BTCUSDT"}),
		BaseURL:          getEnvOrDefault("BASE_URL", "https://api.bitunix.com"),
		WsURL:            getEnvOrDefault("WS_URL", "wss://fapi.bitunix.com/public"),
		Ping:             getDurationOrDefault("PING_INTERVAL", 15*time.Second),
		DataPath:         os.Getenv("DATA_PATH"), // optional
		ModelPath:        getEnvOrDefault("MODEL_PATH", "model.onnx"),
		VWAPWindow:       getDurationOrDefault("VWAP_WINDOW", 30*time.Second),
		VWAPSize:         getIntOrDefault("VWAP_SIZE", 600),
		TickSize:         getIntOrDefault("TICK_SIZE", 50),
		BaseSizeRatio:    getFloatOrDefault("BASE_SIZE_RATIO", 0.002),
		ProbThreshold:    getFloatOrDefault("PROB_THRESHOLD", 0.65),
		DryRun:           getBoolOrDefault("DRY_RUN", false),
		MaxDailyLoss:     getFloatOrDefault("MAX_DAILY_LOSS", 0.05), // 5%
		MetricsPort:      getIntOrDefault("METRICS_PORT", 8080),
		MaxPositionSize:  getFloatOrDefault("MAX_POSITION_SIZE", 0.01), // 1% max position
		MaxPriceDistance: getFloatOrDefault("MAX_PRICE_DISTANCE", 3.0), // 3 std devs
		SymbolConfigs:    make(map[string]SymbolConfig),
		RESTTimeout:      getDurationOrDefault("REST_TIMEOUT", 5*time.Second),
	}

	// Validate configuration
	if err := validateSettings(&settings); err != nil {
		return Settings{}, fmt.Errorf("configuration validation failed: %w", err)
	}

	return settings, nil
}

// GetSymbolConfig returns configuration for a specific symbol, with fallback to global config
func (s *Settings) GetSymbolConfig(symbol string) SymbolConfig {
	if config, exists := s.SymbolConfigs[symbol]; exists {
		return config
	}

	// Return default config
	return SymbolConfig{
		BaseSizeRatio:    s.BaseSizeRatio,
		MaxPositionSize:  s.MaxPositionSize,
		MaxPriceDistance: s.MaxPriceDistance,
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
	if env := os.Getenv("SYMBOLS"); env != "" {
		return strings.Split(env, ",")
	}
	if len(configSymbols) > 0 {
		return configSymbols
	}
	return []string{"BTCUSDT"}
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

// validateSettings performs comprehensive validation of configuration values
func validateSettings(settings *Settings) error {
	// Validate API credentials
	if settings.Key == "" || settings.Secret == "" {
		return fmt.Errorf("API key and secret are required")
	}

	// Validate symbols
	if len(settings.Symbols) == 0 {
		return fmt.Errorf("at least one trading symbol must be specified")
	}

	// Validate URLs
	if settings.BaseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}
	if settings.WsURL == "" {
		return fmt.Errorf("WebSocket URL cannot be empty")
	}

	// Validate time durations
	if settings.Ping < time.Second || settings.Ping > 5*time.Minute {
		return fmt.Errorf("ping interval must be between 1s and 5m, got %v", settings.Ping)
	}
	if settings.VWAPWindow < time.Second || settings.VWAPWindow > time.Hour {
		return fmt.Errorf("VWAP window must be between 1s and 1h, got %v", settings.VWAPWindow)
	}
	if settings.RESTTimeout < time.Second || settings.RESTTimeout > time.Minute {
		return fmt.Errorf("REST timeout must be between 1s and 1m, got %v", settings.RESTTimeout)
	}

	// Validate integer values
	if settings.VWAPSize <= 0 || settings.VWAPSize > 10000 {
		return fmt.Errorf("VWAP size must be between 1 and 10000, got %d", settings.VWAPSize)
	}
	if settings.TickSize <= 0 || settings.TickSize > 1000 {
		return fmt.Errorf("tick size must be between 1 and 1000, got %d", settings.TickSize)
	}
	if settings.MetricsPort < 1024 || settings.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1024 and 65535, got %d", settings.MetricsPort)
	}

	// Validate float values - trading parameters
	if settings.BaseSizeRatio <= 0 || settings.BaseSizeRatio > 0.1 {
		return fmt.Errorf("base size ratio must be between 0 and 0.1 (10%%), got %f", settings.BaseSizeRatio)
	}
	if settings.ProbThreshold < 0.5 || settings.ProbThreshold > 0.99 {
		return fmt.Errorf("probability threshold must be between 0.5 and 0.99, got %f", settings.ProbThreshold)
	}
	if settings.MaxDailyLoss <= 0 || settings.MaxDailyLoss > 0.5 {
		return fmt.Errorf("max daily loss must be between 0 and 0.5 (50%%), got %f", settings.MaxDailyLoss)
	}
	if settings.MaxPositionSize <= 0 || settings.MaxPositionSize > 0.2 {
		return fmt.Errorf("max position size must be between 0 and 0.2 (20%%), got %f", settings.MaxPositionSize)
	}
	if settings.MaxPriceDistance <= 0 || settings.MaxPriceDistance > 10.0 {
		return fmt.Errorf("max price distance must be between 0 and 10 standard deviations, got %f", settings.MaxPriceDistance)
	}

	// Validate symbol-specific configs
	for symbol, config := range settings.SymbolConfigs {
		if config.BaseSizeRatio <= 0 || config.BaseSizeRatio > 0.1 {
			return fmt.Errorf("symbol %s: base size ratio must be between 0 and 0.1, got %f", symbol, config.BaseSizeRatio)
		}
		if config.MaxPositionSize <= 0 || config.MaxPositionSize > 0.2 {
			return fmt.Errorf("symbol %s: max position size must be between 0 and 0.2, got %f", symbol, config.MaxPositionSize)
		}
		if config.MaxPriceDistance <= 0 || config.MaxPriceDistance > 10.0 {
			return fmt.Errorf("symbol %s: max price distance must be between 0 and 10, got %f", symbol, config.MaxPriceDistance)
		}
	}

	return nil
}
