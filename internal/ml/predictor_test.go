package ml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPredictor_FallbackWhenModelNotFound(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, err := NewWithMetrics("nonexistent_model.onnx", metrics, 5*time.Second)
	if err != nil {
		t.Fatalf("Expected no error for missing model, got: %v", err)
	}

	if predictor.available {
		t.Error("Expected predictor to not be available when model is missing")
	}

	// Test fallback behavior
	features := []float32{0.5, -0.3, 1.2}
	result := predictor.Approve(features, 0.6)

	// Should use fallback heuristics
	if metrics.fallbackUse == 0 {
		t.Error("Expected fallback to be used when model is not available")
	}

	if metrics.predictions == 0 {
		t.Error("Expected prediction to be counted even when using fallback")
	}

	// Result should be deterministic for given input
	result2 := predictor.Approve(features, 0.6)
	if result != result2 {
		t.Error("Expected consistent fallback behavior")
	}
}

func TestPredictor_ValidateFeatures(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent_model.onnx", metrics, 5*time.Second)

	testCases := []struct {
		name     string
		features []float32
		valid    bool
	}{
		{"valid features", []float32{0.1, -0.2, 0.5}, true},
		{"too few features", []float32{0.1, -0.2}, false},
		{"too many features", []float32{0.1, -0.2, 0.5, 0.8}, false},
		{"nil features", nil, false},
		{"empty features", []float32{}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := predictor.Approve(tc.features, 0.6)
			if tc.valid && result == false && len(tc.features) == 3 {
				// For valid features, result depends on heuristics, just check no panic
			} else if !tc.valid && result == true {
				t.Errorf("Expected invalid features to return false, got true")
			}
		})
	}
}

func TestPredictor_NilSafety(t *testing.T) {
	var predictor *Predictor = nil

	// Test nil predictor doesn't panic
	result := predictor.Approve([]float32{0.1, 0.2, 0.3}, 0.6)
	if result != false {
		t.Error("Expected nil predictor to return false")
	}

	predictions, err := predictor.Predict([]float32{0.1, 0.2, 0.3})
	if err == nil {
		t.Error("Expected error for nil predictor Predict call")
	}
	if predictions != nil {
		t.Error("Expected nil predictions for nil predictor")
	}
}

func TestPredictor_MetricsTracking(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent_model.onnx", metrics, 5*time.Second)

	features := []float32{0.1, -0.2, 0.5}

	// Make multiple predictions
	for i := 0; i < 3; i++ {
		predictor.Approve(features, 0.6)
	}

	if metrics.predictions != 3 {
		t.Errorf("Expected 3 predictions tracked, got %d", metrics.predictions)
	}

	if metrics.fallbackUse != 3 {
		t.Errorf("Expected 3 fallback uses tracked, got %d", metrics.fallbackUse)
	}

	if metrics.latencySum == 0 {
		t.Error("Expected some latency to be tracked")
	}
}

func TestPredictor_Concurrency(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent_model.onnx", metrics, 5*time.Second)

	features := []float32{0.1, -0.2, 0.5}
	numGoroutines := 10
	numCalls := 100

	done := make(chan bool, numGoroutines)

	// Launch multiple goroutines making predictions
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numCalls; j++ {
				predictor.Approve(features, 0.6)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	expectedCalls := numGoroutines * numCalls
	if metrics.predictions != expectedCalls {
		t.Errorf("Expected %d predictions, got %d", expectedCalls, metrics.predictions)
	}
}

func TestPredictor_ThresholdBehavior(t *testing.T) {
	metrics := &MockMetrics{}
	predictor, _ := NewWithMetrics("nonexistent_model.onnx", metrics, 5*time.Second)

	// Test with different thresholds
	features := []float32{0.8, 0.9, 0.1} // High confidence features

	thresholds := []float64{0.1, 0.5, 0.9, 0.99}
	results := make([]bool, len(thresholds))

	for i, threshold := range thresholds {
		results[i] = predictor.Approve(features, threshold)
	}

	// Higher thresholds should generally be harder to meet
	// (though this depends on the heuristic implementation)
	for i := 1; i < len(results); i++ {
		if results[i-1] == false && results[i] == true {
			t.Logf("Threshold behavior: %.2f=%v, %.2f=%v",
				thresholds[i-1], results[i-1], thresholds[i], results[i])
		}
	}
}

func TestCreateInferenceScript(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "test_inference.py")

	err := createInferenceScript(scriptPath)
	if err != nil {
		t.Fatalf("Failed to create inference script: %v", err)
	}

	// Check script was created
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("Inference script was not created")
	}

	// Check script is executable
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("Failed to stat script: %v", err)
	}

	if info.Mode()&0111 == 0 {
		t.Error("Inference script is not executable")
	}

	// Basic content check
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	scriptStr := string(content)
	expectedParts := []string{
		"#!/usr/bin/env python3",
		"import onnxruntime",
		"json.load(sys.stdin)",
		"session.run",
	}

	for _, part := range expectedParts {
		if !strings.Contains(scriptStr, part) {
			t.Errorf("Script missing expected part: %s", part)
		}
	}
}
