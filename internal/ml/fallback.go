package ml

import (
	"math"
	"sync"
)

// FallbackPredictor implements a simple heuristic-based fallback when ML is unavailable
type FallbackPredictor struct {
	mu              sync.RWMutex
	lastPredictions map[string]float64
	windowSize      int
	threshold       float64
}

// NewFallbackPredictor creates a new fallback predictor
func NewFallbackPredictor(windowSize int, threshold float64) *FallbackPredictor {
	return &FallbackPredictor{
		lastPredictions: make(map[string]float64),
		windowSize:      windowSize,
		threshold:       threshold,
	}
}

// Predict implements a simple mean reversion strategy
func (p *FallbackPredictor) Predict(features []float32) ([]float32, error) {
	if len(features) < 3 {
		return []float32{1.0, 0.0}, nil // Default to no signal
	}

	// Extract features
	tickRatio := float64(features[0])
	depthRatio := float64(features[1])
	priceDist := float64(features[2])

	// Simple mean reversion strategy
	score := p.calculateScore(tickRatio, depthRatio, priceDist)

	// Convert to probabilities
	prob := sigmoid(score)
	return []float32{1.0 - float32(prob), float32(prob)}, nil
}

// calculateScore implements a simple scoring function
func (p *FallbackPredictor) calculateScore(tickRatio, depthRatio, priceDist float64) float64 {
	// Weight the features
	tickWeight := 0.4
	depthWeight := 0.3
	priceWeight := 0.3

	// Normalize features
	tickScore := math.Tanh(tickRatio)
	depthScore := math.Tanh(depthRatio)
	priceScore := -math.Tanh(priceDist) // Negative because we want mean reversion

	// Combine scores
	score := tickWeight*tickScore + depthWeight*depthScore + priceWeight*priceScore

	// Apply threshold
	if math.Abs(score) < p.threshold {
		score = 0
	}

	return score
}

// sigmoid converts a score to a probability
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// UpdateMetrics updates the predictor's internal metrics
func (p *FallbackPredictor) UpdateMetrics(key string, score float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastPredictions[key] = score
}

// GetMetrics returns the current prediction metrics
func (p *FallbackPredictor) GetMetrics() map[string]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metrics := make(map[string]float64)
	for k, v := range p.lastPredictions {
		metrics[k] = v
	}
	return metrics
}

// Reset clears all stored metrics
func (p *FallbackPredictor) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastPredictions = make(map[string]float64)
}
