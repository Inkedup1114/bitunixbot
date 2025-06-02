package ml

import "sync"

// MockMetrics implements MetricsInterface for testing
type MockMetrics struct {
	mu               sync.Mutex
	predictions      int
	failures         int
	latencySum       float64
	accuracySum      float64
	timeouts         int
	fallbackUse      int
	modelAge         float64
	predictionScores []float64
}

func (m *MockMetrics) MLPredictionsInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.predictions++
}

func (m *MockMetrics) MLFailuresInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures++
}

func (m *MockMetrics) MLLatencyObserve(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencySum += v
}

func (m *MockMetrics) MLModelAgeSet(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelAge = v
}

func (m *MockMetrics) MLAccuracyObserve(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accuracySum += v
}

func (m *MockMetrics) MLPredictionScoresObserve(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.predictionScores = append(m.predictionScores, v)
}

func (m *MockMetrics) MLTimeoutsInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timeouts++
}

func (m *MockMetrics) MLFallbackUseInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallbackUse++
}
