package exec

import (
	"testing"

	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
)

func TestCompilation(t *testing.T) {
	// This should compile if the signature is correct
	var c cfg.Settings
	var p *ml.Predictor
	var m *metrics.MetricsWrapper

	_ = New(c, p, m)
}
