// Package ml provides machine learning prediction capabilities for the trading bot.
// It includes interfaces for ML predictors, ONNX model integration, feature importance
// tracking, drift detection, and comprehensive ML pipeline management.
//
// The package supports both production ONNX models and fallback heuristics,
// with extensive monitoring and performance tracking capabilities.
package ml

// PredictorInterface defines the interface for ML predictors used in trading decisions.
// Implementations should provide both prediction and approval methods for trading signals.
type PredictorInterface interface {
	// Approve determines whether to approve a trading signal based on features and threshold.
	// Returns true if the prediction confidence exceeds the threshold.
	Approve(features []float32, threshold float64) bool

	// Predict generates raw prediction scores from input features.
	// Returns prediction probabilities or scores, or an error if prediction fails.
	Predict(features []float32) ([]float32, error)
}
