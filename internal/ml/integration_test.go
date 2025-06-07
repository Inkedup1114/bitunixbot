package ml

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPredictor_IntegrationWithSampleModel tests the full pipeline with a mock ONNX model
func TestPredictor_IntegrationWithSampleModel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for our test model
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "test_model.onnx")

	// Create a minimal mock ONNX model file (just for file existence)
	mockModelContent := []byte("mock onnx model content")
	err := os.WriteFile(modelPath, mockModelContent, 0o644)
	if err != nil {
		t.Fatalf("Failed to create mock model file: %v", err)
	}

	metrics := &MockMetrics{}
	predictor, err := NewWithMetrics(modelPath, metrics, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to create predictor: %v", err)
	}

	// The predictor should fall back to heuristics since we don't have a real ONNX model
	// but it should handle the case gracefully
	features := []float32{0.1, -0.2, 0.5}

	// Test prediction
	result := predictor.Approve(features, 0.6)
	t.Logf("Prediction result: %v", result)

	// Check metrics were updated
	if metrics.predictions == 0 {
		t.Error("Expected at least one prediction to be tracked")
	}

	// Test concurrent access
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			features := []float32{float32(id) * 0.1, -0.2, 0.5}
			predictor.Approve(features, 0.6)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if metrics.predictions < 6 { // 1 + 5 from concurrent calls
		t.Errorf("Expected at least 6 predictions, got %d", metrics.predictions)
	}
}

// TestPredictor_ErrorHandling tests various error conditions
func TestPredictor_ErrorHandling(t *testing.T) {
	metrics := &MockMetrics{}

	// Test with non-existent directory
	invalidPath := "/nonexistent/path/model.onnx"
	predictor, err := NewWithMetrics(invalidPath, metrics, 1*time.Second)
	if err != nil {
		t.Fatalf("Expected no error for invalid path, got: %v", err)
	}

	// Should fall back to heuristics
	if predictor.available {
		t.Error("Expected predictor to not be available for invalid path")
	}

	// Test predictions still work with fallback
	features := []float32{0.1, -0.2, 0.5}
	_ = predictor.Approve(features, 0.6)

	if metrics.fallbackUse == 0 {
		t.Error("Expected fallback to be used")
	}

	// Test with very short timeout
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "test_model.onnx")
	if err := os.WriteFile(modelPath, []byte("mock"), 0o644); err != nil {
		t.Fatalf("Failed to write mock model file: %v", err)
	}

	shortTimeoutPredictor, _ := NewWithMetrics(modelPath, metrics, 1*time.Millisecond)

	// This should likely timeout or fail, but shouldn't panic
	shortTimeoutPredictor.Approve(features, 0.6)
}

// TestPredictor_HealthCheck tests the health checking functionality
func TestPredictor_HealthCheck(t *testing.T) {
	metrics := &MockMetrics{}
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "test_model.onnx")
	if err := os.WriteFile(modelPath, []byte("mock"), 0o644); err != nil {
		t.Fatalf("Failed to write mock model file: %v", err)
	}

	predictor, _ := NewWithMetrics(modelPath, metrics, 5*time.Second)

	// Test health check doesn't run too frequently
	err1 := predictor.healthCheck()
	err2 := predictor.healthCheck() // Should be skipped due to timing

	if err1 == nil && err2 != nil {
		t.Error("Health check behavior inconsistent")
	}

	// Force health check by resetting timestamp
	predictor.healthChecked = time.Time{}
	err3 := predictor.healthCheck()

	// Should attempt health check again
	t.Logf("Health check results: %v, %v, %v", err1, err2, err3)
}

// TestMetricsIntegration tests that all metrics are properly tracked
func TestMetricsIntegration(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent.onnx", metrics, 5*time.Second)

	// Test various operations
	features := []float32{0.1, -0.2, 0.5}

	// Multiple predictions with different thresholds
	thresholds := []float64{0.5, 0.6, 0.7, 0.8}
	for _, threshold := range thresholds {
		predictor.Approve(features, threshold)
	}

	// Check all expected metrics were updated
	expectedPredictions := len(thresholds)
	if metrics.predictions != expectedPredictions {
		t.Errorf("Expected %d predictions, got %d", expectedPredictions, metrics.predictions)
	}

	if metrics.fallbackUse != expectedPredictions {
		t.Errorf("Expected %d fallback uses, got %d", expectedPredictions, metrics.fallbackUse)
	}

	if metrics.latencySum == 0 {
		t.Error("Expected some latency to be recorded")
	}

	// Test Predict method
	predictions, err := predictor.Predict(features)
	if err != nil {
		t.Errorf("Predict failed: %v", err)
	}
	if len(predictions) != 2 {
		t.Errorf("Expected 2 predictions, got %d", len(predictions))
	}
}

// TestConfigurationValidation tests that the predictor handles various configurations
func TestConfigurationValidation(t *testing.T) {
	metrics := &MockMetrics{}

	testCases := []struct {
		name    string
		timeout time.Duration
		valid   bool
	}{
		{"very short timeout", 1 * time.Microsecond, true}, // Should not fail, just perform poorly
		{"reasonable timeout", 5 * time.Second, true},
		{"long timeout", 60 * time.Second, true},
		{"zero timeout", 0, true}, // Should be handled gracefully
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			predictor, err := NewWithMetrics("nonexistent.onnx", metrics, tc.timeout)

			if tc.valid && err != nil {
				t.Errorf("Expected valid configuration to succeed, got error: %v", err)
			}

			if predictor == nil {
				t.Error("Expected predictor to be created even with unusual timeout")
			}

			// Test that it can still make predictions
			features := []float32{0.1, -0.2, 0.5}
			result := predictor.Approve(features, 0.6)
			t.Logf("Timeout %v: prediction result %v", tc.timeout, result)
		})
	}
}

// BenchmarkPredictor_Approve benchmarks the prediction performance
func BenchmarkPredictor_Approve(b *testing.B) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent.onnx", metrics, 5*time.Second)

	// Pre-allocate features to avoid allocation in hot loop
	features := []float32{0.1, -0.2, 0.5}
	threshold := 0.6

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		predictor.Approve(features, threshold)
	}
}

// BenchmarkPredictor_Predict benchmarks the Predict method
func BenchmarkPredictor_Predict(b *testing.B) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent.onnx", metrics, 5*time.Second)

	// Pre-allocate features to avoid allocation in hot loop
	features := []float32{0.1, -0.2, 0.5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = predictor.Predict(features)
	}
}
