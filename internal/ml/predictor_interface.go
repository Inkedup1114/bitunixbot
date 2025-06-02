package ml

// PredictorInterface defines the interface for ML predictors
type PredictorInterface interface {
	Approve(features []float32, threshold float64) bool
	Predict(features []float32) ([]float32, error)
}
