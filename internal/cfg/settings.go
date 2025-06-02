package cfg

import (
	"os"
	"strconv"
)

type Settings struct {
	// ...existing fields...
	Leverage   int
	MarginMode string
	RiskUSD    float64
}

func LoadSettings() (*Settings, error) {
	// ...existing code...
	leverage := getEnvAsInt("LEVERAGE", 20)
	marginMode := getEnv("MARGIN_MODE", "ISOLATION")
	riskUSD := getEnvAsFloat("RISK_USD", 25.0)

	return &Settings{
		// ...existing fields...
		Leverage:   leverage,
		MarginMode: marginMode,
		RiskUSD:    riskUSD,
	}, nil
}

// Helper functions (if not already present)
func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvAsInt(name string, defaultVal int) int {
	if v, err := strconv.Atoi(getEnv(name, "")); err == nil {
		return v
	}
	return defaultVal
}

func getEnvAsFloat(name string, defaultVal float64) float64 {
	if v, err := strconv.ParseFloat(getEnv(name, ""), 64); err == nil {
		return v
	}
	return defaultVal
}
