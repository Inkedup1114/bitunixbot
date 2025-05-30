package ml

import (
	"os"
)

type Predictor struct {
	available bool
	threshold float64
}

func New(path string) (*Predictor, error) {
	// Check if model file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Predictor{available: false, threshold: 0.65}, nil
	}

	// TODO: Replace with actual ONNX runtime when a working package is available
	// For now, return a stub that always permits trading
	return &Predictor{available: false, threshold: 0.65}, nil
}

// features = [tickRatio, depthRatio, priceDist]
func (p *Predictor) Approve(f []float32, threshold float64) bool {
	if p == nil || !p.available {
		// Simple heuristic fallback when no ML model is available
		// This is a basic momentum + mean reversion strategy
		if len(f) >= 3 {
			tickRatio := f[0]  // momentum signal
			depthRatio := f[1] // order book imbalance
			priceDist := f[2]  // price distance from VWAP in std devs

			// Simple rules: trade when there's momentum, good depth signal,
			// and price isn't too far from VWAP
			// Use the provided threshold in a meaningful way
			confidence := (abs(tickRatio) + abs(depthRatio)) / 2.0
			return confidence > float32(threshold-0.5) && abs(priceDist) < 2.0
		}
		return false
	}

	// TODO: Implement actual ONNX inference here
	return true
}

// Predict returns prediction scores for the given features
func (p *Predictor) Predict(features []float32) ([]float32, error) {
	if p == nil || !p.available {
		// Return dummy prediction when no model is available
		// In practice, this would be ONNX inference
		return []float32{0.1, 0.05, 0.0}, nil
	}

	// TODO: Implement actual ONNX inference here
	return []float32{0.1, 0.05, 0.0}, nil
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
