package cfg

import (
	"testing"
	"time"
)

// createValidSettings creates a valid Settings struct for testing
func createValidSettings() *Settings {
	return &Settings{
		Key:                        "valid_key",
		Secret:                     "valid_secret",
		Symbols:                    []string{"BTCUSDT", "ETHUSDT"},
		BaseURL:                    "https://api.bitunix.com",
		WsURL:                      "wss://fapi.bitunix.com/public",
		Ping:                       30 * time.Second,
		VWAPWindow:                 30 * time.Second,
		RESTTimeout:                10 * time.Second,
		VWAPSize:                   500,
		TickSize:                   50,
		MetricsPort:                9090,
		BaseSizeRatio:              0.002,
		ProbThreshold:              0.65,
		MaxDailyLoss:               0.05,
		MaxPositionSize:            0.01,
		MaxPositionExposure:        0.1,
		MaxPriceDistance:           2.5,
		SymbolConfigs:              make(map[string]SymbolConfig),
		DryRun:                     true, // Set to true to avoid FORCE_LIVE_TRADING requirement
		CircuitBreakerVolatility:   2.0,
		CircuitBreakerImbalance:    0.8,
		CircuitBreakerVolume:       5.0,
		CircuitBreakerErrorRate:    0.2,
		CircuitBreakerRecoveryTime: 5 * time.Minute,
	}
}

func TestValidateSettings_ValidConfig(t *testing.T) {
	settings := createValidSettings()

	err := validateSettings(settings)
	if err != nil {
		t.Errorf("Expected valid config to pass, got error: %v", err)
	}
}

func TestValidateSettings_MissingAPIKey(t *testing.T) {
	settings := &Settings{
		Secret:           "valid_secret",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for missing API key")
	}
	if err != nil && err.Error() != "API key and secret are required" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestValidateSettings_MissingSecret(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for missing secret")
	}
}

func TestValidateSettings_EmptySymbols(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Secret:           "valid_secret",
		Symbols:          []string{},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for empty symbols")
	}
}

func TestValidateSettings_EmptyBaseURL(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Secret:           "valid_secret",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for empty base URL")
	}
}

func TestValidateSettings_EmptyWsURL(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Secret:           "valid_secret",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for empty WebSocket URL")
	}
}

func TestValidateSettings_InvalidPingInterval(t *testing.T) {
	testCases := []struct {
		name    string
		ping    time.Duration
		wantErr bool
	}{
		{"too short", 500 * time.Millisecond, true},
		{"minimum valid", 1 * time.Second, false},
		{"normal", 30 * time.Second, false},
		{"maximum valid", 5 * time.Minute, false},
		{"too long", 10 * time.Minute, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := createValidSettings()
			settings.Ping = tc.ping

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid ping interval")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid ping interval, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidVWAPWindow(t *testing.T) {
	testCases := []struct {
		name       string
		vwapWindow time.Duration
		wantErr    bool
	}{
		{"too short", 500 * time.Millisecond, true},
		{"minimum valid", 1 * time.Second, false},
		{"normal", 30 * time.Second, false},
		{"maximum valid", 1 * time.Hour, false},
		{"too long", 2 * time.Hour, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       tc.vwapWindow,
				RESTTimeout:      10 * time.Second,
				VWAPSize:         500,
				TickSize:         50,
				MetricsPort:      9090,
				BaseSizeRatio:    0.002,
				ProbThreshold:    0.65,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid VWAP window")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid VWAP window, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidRESTTimeout(t *testing.T) {
	testCases := []struct {
		name        string
		restTimeout time.Duration
		wantErr     bool
	}{
		{"too short", 500 * time.Millisecond, true},
		{"minimum valid", 1 * time.Second, false},
		{"normal", 10 * time.Second, false},
		{"maximum valid", 1 * time.Minute, false},
		{"too long", 2 * time.Minute, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       30 * time.Second,
				RESTTimeout:      tc.restTimeout,
				VWAPSize:         500,
				TickSize:         50,
				MetricsPort:      9090,
				BaseSizeRatio:    0.002,
				ProbThreshold:    0.65,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid REST timeout")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid REST timeout, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidVWAPSize(t *testing.T) {
	testCases := []struct {
		name     string
		vwapSize int
		wantErr  bool
	}{
		{"zero", 0, true},
		{"negative", -1, true},
		{"too small", 9, true},
		{"minimum valid", 10, false},
		{"normal", 500, false},
		{"maximum valid", 10000, false},
		{"too large", 10001, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       30 * time.Second,
				RESTTimeout:      10 * time.Second,
				VWAPSize:         tc.vwapSize,
				TickSize:         50,
				MetricsPort:      9090,
				BaseSizeRatio:    0.002,
				ProbThreshold:    0.65,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid VWAP size")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid VWAP size, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidMetricsPort(t *testing.T) {
	testCases := []struct {
		name        string
		metricsPort int
		wantErr     bool
	}{
		{"too low", 1023, true},
		{"minimum valid", 1024, false},
		{"normal", 9090, false},
		{"maximum valid", 65535, false},
		{"too high", 65536, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       30 * time.Second,
				RESTTimeout:      10 * time.Second,
				VWAPSize:         500,
				TickSize:         50,
				MetricsPort:      tc.metricsPort,
				BaseSizeRatio:    0.002,
				ProbThreshold:    0.65,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid metrics port")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid metrics port, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidBaseSizeRatio(t *testing.T) {
	testCases := []struct {
		name          string
		baseSizeRatio float64
		wantErr       bool
	}{
		{"zero", 0.0, true},
		{"negative", -0.001, true},
		{"minimum valid", 0.0001, false},
		{"normal", 0.002, false},
		{"maximum valid", 0.1, false},
		{"too large", 0.101, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       30 * time.Second,
				RESTTimeout:      10 * time.Second,
				VWAPSize:         500,
				TickSize:         50,
				MetricsPort:      9090,
				BaseSizeRatio:    tc.baseSizeRatio,
				ProbThreshold:    0.65,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid base size ratio")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid base size ratio, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_InvalidProbThreshold(t *testing.T) {
	testCases := []struct {
		name          string
		probThreshold float64
		wantErr       bool
	}{
		{"too low", 0.49, true},
		{"minimum valid", 0.5, false},
		{"normal", 0.65, false},
		{"maximum valid", 0.99, false},
		{"too high", 1.0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &Settings{
				Key:              "valid_key",
				Secret:           "valid_secret",
				Symbols:          []string{"BTCUSDT"},
				BaseURL:          "https://api.bitunix.com",
				WsURL:            "wss://fapi.bitunix.com/public",
				Ping:             30 * time.Second,
				VWAPWindow:       30 * time.Second,
				RESTTimeout:      10 * time.Second,
				VWAPSize:         500,
				TickSize:         50,
				MetricsPort:      9090,
				BaseSizeRatio:    0.002,
				ProbThreshold:    tc.probThreshold,
				MaxDailyLoss:     0.05,
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			}

			err := validateSettings(settings)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid probability threshold")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for valid probability threshold, got: %v", err)
			}
		})
	}
}

func TestValidateSettings_SymbolConfigs(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Secret:           "valid_secret",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
		SymbolConfigs: map[string]SymbolConfig{
			"BTCUSDT": {
				BaseSizeRatio:    0.15, // Too large
				MaxPositionSize:  0.01,
				MaxPriceDistance: 2.5,
			},
		},
	}

	err := validateSettings(settings)
	if err == nil {
		t.Error("Expected error for invalid symbol config")
	}
}

func TestValidateSettings_ValidSymbolConfigs(t *testing.T) {
	settings := &Settings{
		Key:              "valid_key",
		Secret:           "valid_secret",
		Symbols:          []string{"BTCUSDT"},
		BaseURL:          "https://api.bitunix.com",
		WsURL:            "wss://fapi.bitunix.com/public",
		Ping:             30 * time.Second,
		VWAPWindow:       30 * time.Second,
		RESTTimeout:      10 * time.Second,
		VWAPSize:         500,
		TickSize:         50,
		MetricsPort:      9090,
		BaseSizeRatio:    0.002,
		ProbThreshold:    0.65,
		MaxDailyLoss:     0.05,
		MaxPositionSize:  0.01,
		MaxPriceDistance: 2.5,
		SymbolConfigs: map[string]SymbolConfig{
			"BTCUSDT": {
				BaseSizeRatio:    0.001,
				MaxPositionSize:  0.015,
				MaxPriceDistance: 2.0,
			},
			"ETHUSDT": {
				BaseSizeRatio:    0.003,
				MaxPositionSize:  0.02,
				MaxPriceDistance: 3.0,
			},
		},
	}

	err := validateSettings(settings)
	if err != nil {
		t.Errorf("Expected valid symbol configs to pass, got error: %v", err)
	}
}
