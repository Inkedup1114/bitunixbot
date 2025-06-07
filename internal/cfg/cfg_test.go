package cfg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantErr  bool
		validate func(t *testing.T, settings Settings)
	}{
		{
			name: "valid config with required fields",
			envVars: map[string]string{
				"BITUNIX_API_KEY":    "test_key",
				"BITUNIX_SECRET_KEY": "test_secret",
				"FORCE_LIVE_TRADING": "true",
			},
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				if settings.Key != "test_key" {
					t.Errorf("expected Key to be 'test_key', got %s", settings.Key)
				}
				if settings.Secret != "test_secret" {
					t.Errorf("expected Secret to be 'test_secret', got %s", settings.Secret)
				}
				// Test defaults
				if len(settings.Symbols) != 1 || settings.Symbols[0] != "BTCUSDT" {
					t.Errorf("expected default symbols [BTCUSDT], got %v", settings.Symbols)
				}
				if settings.BaseURL != "https://api.bitunix.com" {
					t.Errorf("expected default BaseURL, got %s", settings.BaseURL)
				}
				if settings.VWAPWindow != 30*time.Second {
					t.Errorf("expected default VWAPWindow 30s, got %v", settings.VWAPWindow)
				}
				if settings.BaseSizeRatio != 0.002 {
					t.Errorf("expected default BaseSizeRatio 0.002, got %f", settings.BaseSizeRatio)
				}
			},
		},
		{
			name: "custom symbols and settings",
			envVars: map[string]string{
				"BITUNIX_API_KEY":    "test_key",
				"BITUNIX_SECRET_KEY": "test_secret",
				"SYMBOLS":            "BTCUSDT,ETHUSDT,ADAUSDT",
				"BASE_SIZE_RATIO":    "0.005",
				"VWAP_WINDOW":        "60s",
				"DRY_RUN":            "true",
				"METRICS_PORT":       "9090",
				"MAX_POSITION_SIZE":  "0.02",
				"MAX_PRICE_DISTANCE": "2.5",
				"FORCE_LIVE_TRADING": "true",
			},
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				expectedSymbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}
				if len(settings.Symbols) != len(expectedSymbols) {
					t.Errorf("expected %d symbols, got %d", len(expectedSymbols), len(settings.Symbols))
				}
				for i, symbol := range expectedSymbols {
					if i >= len(settings.Symbols) || settings.Symbols[i] != symbol {
						t.Errorf("expected symbol %s at index %d, got %v", symbol, i, settings.Symbols)
					}
				}
				if settings.BaseSizeRatio != 0.005 {
					t.Errorf("expected BaseSizeRatio 0.005, got %f", settings.BaseSizeRatio)
				}
				if settings.VWAPWindow != 60*time.Second {
					t.Errorf("expected VWAPWindow 60s, got %v", settings.VWAPWindow)
				}
				if !settings.DryRun {
					t.Error("expected DryRun to be true")
				}
				if settings.MetricsPort != 9090 {
					t.Errorf("expected MetricsPort 9090, got %d", settings.MetricsPort)
				}
				if settings.MaxPositionSize != 0.02 {
					t.Errorf("expected MaxPositionSize 0.02, got %f", settings.MaxPositionSize)
				}
				if settings.MaxPriceDistance != 2.5 {
					t.Errorf("expected MaxPriceDistance 2.5, got %f", settings.MaxPriceDistance)
				}
			},
		},
		{
			name: "missing API key",
			envVars: map[string]string{
				"BITUNIX_SECRET_KEY": "test_secret",
			},
			wantErr: true,
		},
		{
			name: "missing secret key",
			envVars: map[string]string{
				"BITUNIX_API_KEY": "test_key",
			},
			wantErr: true,
		},
		{
			name:    "missing both keys",
			envVars: map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all environment variables first
			clearTestEnv(t)

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			settings, err := loadFromEnv()

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, settings)
			}
		})
	}
}

func TestLoadFromYAML(t *testing.T) {
	tests := []struct {
		name         string
		yamlContent  string
		envOverrides map[string]string
		wantErr      bool
		validate     func(t *testing.T, settings Settings)
	}{
		{
			name: "valid YAML config",
			yamlContent: `
api:
  key: "yaml_key"
  secret: "yaml_secret"
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"

trading:
  symbols:
    - "BTCUSDT"
    - "ETHUSDT"
  baseSizeRatio: 0.001
  maxPositionSize: 0.015
  maxDailyLoss: 0.03
  maxPriceDistance: 2.0
  dryRun: true

features:
  vwapWindow: "45s"
  vwapSize: 800
  tickSize: 100

ml:
  modelPath: "custom.onnx"
  probThreshold: 0.7

system:
  dataPath: "/custom/data"
  pingInterval: "20s"
  metricsPort: 9090
  restTimeout: "10s"
`,
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				if settings.Key != "yaml_key" {
					t.Errorf("expected Key 'yaml_key', got %s", settings.Key)
				}
				if settings.Secret != "yaml_secret" {
					t.Errorf("expected Secret 'yaml_secret', got %s", settings.Secret)
				}
				if settings.BaseSizeRatio != 0.001 {
					t.Errorf("expected BaseSizeRatio 0.001, got %f", settings.BaseSizeRatio)
				}
				if settings.VWAPWindow != 45*time.Second {
					t.Errorf("expected VWAPWindow 45s, got %v", settings.VWAPWindow)
				}
				if settings.VWAPSize != 800 {
					t.Errorf("expected VWAPSize 800, got %d", settings.VWAPSize)
				}
				if !settings.DryRun {
					t.Error("expected DryRun to be true")
				}
				if settings.MetricsPort != 9090 {
					t.Errorf("expected MetricsPort 9090, got %d", settings.MetricsPort)
				}
				if settings.RESTTimeout != 10*time.Second {
					t.Errorf("expected RESTTimeout 10s, got %v", settings.RESTTimeout)
				}
			},
		},
		{
			name: "YAML with env overrides",
			yamlContent: `
api:
  key: "yaml_key"
  secret: "yaml_secret"
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"
trading:
  symbols: ["BTCUSDT"]
  baseSizeRatio: 0.001
  maxDailyLoss: 0.05
  maxPositionSize: 0.01
  maxPriceDistance: 2.5
features:
  vwapWindow: "30s"
  vwapSize: 500
  tickSize: 50
ml:
  probThreshold: 0.65
system:
  metricsPort: 9090
  pingInterval: "30s"
  restTimeout: "10s"
`,
			envOverrides: map[string]string{
				"BITUNIX_API_KEY": "env_key",
				"BASE_SIZE_RATIO": "0.005",
			},
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				if settings.Key != "env_key" {
					t.Errorf("expected env override Key 'env_key', got %s", settings.Key)
				}
				if settings.Secret != "yaml_secret" {
					t.Errorf("expected YAML Secret 'yaml_secret', got %s", settings.Secret)
				}
				if settings.BaseSizeRatio != 0.005 {
					t.Errorf("expected env override BaseSizeRatio 0.005, got %f", settings.BaseSizeRatio)
				}
			},
		},
		{
			name: "YAML missing required keys",
			yamlContent: `
trading:
  symbols: ["BTCUSDT"]
`,
			wantErr: true,
		},
		{
			name:        "invalid YAML",
			yamlContent: `invalid: yaml: content: [`,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearTestEnv(t)

			// Set environment overrides
			for key, value := range tt.envOverrides {
				t.Setenv(key, value)
			}

			// Create temporary YAML file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.yamlContent), 0o644)
			if err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			settings, err := loadFromYAML(configPath)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, settings)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		configFile  string
		yamlContent string
		envVars     map[string]string
		wantErr     bool
		validate    func(t *testing.T, settings Settings)
	}{
		{
			name: "load from env when no config file",
			envVars: map[string]string{
				"BITUNIX_API_KEY":    "env_key",
				"BITUNIX_SECRET_KEY": "env_secret",
			},
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				if settings.Key != "env_key" {
					t.Errorf("expected Key 'env_key', got %s", settings.Key)
				}
			},
		},
		{
			name:       "load from YAML when config file specified",
			configFile: "config.yaml",
			yamlContent: `
api:
  key: "yaml_key"
  secret: "yaml_secret"
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"
trading:
  symbols: ["BTCUSDT"]
  baseSizeRatio: 0.002
  maxDailyLoss: 0.05
  maxPositionSize: 0.01
  maxPriceDistance: 2.5
features:
  vwapWindow: "30s"
  vwapSize: 500
  tickSize: 50
ml:
  probThreshold: 0.65
system:
  metricsPort: 9090
  pingInterval: "30s"
  restTimeout: "10s"
`,
			wantErr: false,
			validate: func(t *testing.T, settings Settings) {
				if settings.Key != "yaml_key" {
					t.Errorf("expected Key 'yaml_key', got %s", settings.Key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearTestEnv(t)

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create config file if specified
			if tt.configFile != "" && tt.yamlContent != "" {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, tt.configFile)
				err := os.WriteFile(configPath, []byte(tt.yamlContent), 0o644)
				if err != nil {
					t.Fatalf("failed to write test config file: %v", err)
				}
				t.Setenv("CONFIG_FILE", configPath)
			}

			settings, err := Load()

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, settings)
			}
		})
	}
}

func TestGetSymbolConfig(t *testing.T) {
	settings := Settings{
		BaseSizeRatio:    0.002,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 3.0,
		SymbolConfigs: map[string]SymbolConfig{
			"BTCUSDT": {
				BaseSizeRatio:    0.001,
				MaxPositionSize:  0.015,
				MaxPriceDistance: 2.5,
			},
		},
	}

	t.Run("symbol with specific config", func(t *testing.T) {
		config := settings.GetSymbolConfig("BTCUSDT")
		if config.BaseSizeRatio != 0.001 {
			t.Errorf("expected BaseSizeRatio 0.001, got %f", config.BaseSizeRatio)
		}
		if config.MaxPositionSize != 0.015 {
			t.Errorf("expected MaxPositionSize 0.015, got %f", config.MaxPositionSize)
		}
		if config.MaxPriceDistance != 2.5 {
			t.Errorf("expected MaxPriceDistance 2.5, got %f", config.MaxPriceDistance)
		}
	})

	t.Run("symbol with default config", func(t *testing.T) {
		config := settings.GetSymbolConfig("ETHUSDT")
		if config.BaseSizeRatio != 0.002 {
			t.Errorf("expected default BaseSizeRatio 0.002, got %f", config.BaseSizeRatio)
		}
		if config.MaxPositionSize != 0.01 {
			t.Errorf("expected default MaxPositionSize 0.01, got %f", config.MaxPositionSize)
		}
		if config.MaxPriceDistance != 3.0 {
			t.Errorf("expected default MaxPriceDistance 3.0, got %f", config.MaxPriceDistance)
		}
	})
}

// clearTestEnv clears potentially conflicting environment variables
func clearTestEnv(t *testing.T) {
	envVars := []string{
		"BITUNIX_API_KEY", "BITUNIX_SECRET_KEY", "SYMBOLS", "BASE_URL", "WS_URL",
		"PING_INTERVAL", "DATA_PATH", "MODEL_PATH", "VWAP_WINDOW", "VWAP_SIZE",
		"TICK_SIZE", "BASE_SIZE_RATIO", "PROB_THRESHOLD", "DRY_RUN", "MAX_DAILY_LOSS",
		"METRICS_PORT", "MAX_POSITION_SIZE", "MAX_PRICE_DISTANCE", "REST_TIMEOUT",
		"CONFIG_FILE",
	}

	for _, env := range envVars {
		if val := os.Getenv(env); val != "" {
			t.Setenv(env, "")
		}
	}
}
