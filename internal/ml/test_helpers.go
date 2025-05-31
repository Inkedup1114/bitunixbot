package ml

// MockMetrics implements MetricsInterface for testing
type MockMetrics struct {
	predictions      int
	failures         int
	latencySum       float64
	accuracySum      float64
	timeouts         int
	fallbackUse      int
	modelAge         float64
	predictionScores []float64
}

func (m *MockMetrics) MLPredictionsInc()           { m.predictions++ }
func (m *MockMetrics) MLFailuresInc()              { m.failures++ }
func (m *MockMetrics) MLLatencyObserve(v float64)  { m.latencySum += v }
func (m *MockMetrics) MLModelAgeSet(v float64)     { m.modelAge = v }
func (m *MockMetrics) MLAccuracyObserve(v float64) { m.accuracySum += v }
func (m *MockMetrics) MLPredictionScoresObserve(v float64) {
	m.predictionScores = append(m.predictionScores, v)
}
func (m *MockMetrics) MLTimeoutsInc()    { m.timeouts++ }
func (m *MockMetrics) MLFallbackUseInc() { m.fallbackUse++ }
