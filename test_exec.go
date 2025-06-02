package main

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exec"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
)

func main() {
	// This should compile if the signature is correct
	var c cfg.Settings
	var p *ml.Predictor
	var m *metrics.MetricsWrapper

	_ = exec.New(c, p, m)
}
