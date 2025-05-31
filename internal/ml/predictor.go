package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MetricsInterface defines metrics methods needed by the predictor
type MetricsInterface interface {
	MLPredictionsInc()
	MLFailuresInc()
	MLLatencyObserve(float64)
	MLModelAgeSet(float64)
	MLAccuracyObserve(float64)
	MLPredictionScoresObserve(float64)
	MLTimeoutsInc()
	MLFallbackUseInc()
}

type Predictor struct {
	available     bool
	threshold     float64
	modelPath     string
	pythonPath    string
	scriptPath    string
	mu            sync.RWMutex
	lastUsed      time.Time
	healthChecked time.Time
	timeout       time.Duration
	modelCreated  time.Time
	metrics       MetricsInterface
}

type PredictionRequest struct {
	Features []float32 `json:"features"`
}

type PredictionResponse struct {
	Probabilities []float64 `json:"probabilities"`
	Prediction    int       `json:"prediction"`
	Error         string    `json:"error,omitempty"`
}

func New(path string) (*Predictor, error) {
	return NewWithMetrics(path, nil, 5*time.Second)
}

func NewWithMetrics(path string, metrics MetricsInterface, timeout time.Duration) (*Predictor, error) {
	// Get model file info for age tracking
	var modelCreated time.Time
	if info, err := os.Stat(path); err == nil {
		modelCreated = info.ModTime()
	} else if !os.IsNotExist(err) {
		log.Warn().Err(err).Str("model_path", path).Msg("Failed to get model file info")
	}

	// Check if model file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warn().Str("model_path", path).Msg("ONNX model not found, using fallback heuristics")
		return &Predictor{
			available:    false,
			threshold:    0.65,
			modelPath:    path,
			timeout:      timeout,
			modelCreated: modelCreated,
			metrics:      metrics,
		}, nil
	}

	// Find Python executable
	pythonPath, err := findPython()
	if err != nil {
		log.Warn().Err(err).Msg("Python not found, using fallback heuristics")
		return &Predictor{
			available:    false,
			threshold:    0.65,
			modelPath:    path,
			timeout:      timeout,
			modelCreated: modelCreated,
			metrics:      metrics,
		}, nil
	}

	// Create inference script path
	scriptPath := filepath.Join(filepath.Dir(path), "onnx_inference.py")
	if err := createInferenceScript(scriptPath); err != nil {
		log.Warn().Err(err).Msg("Failed to create inference script, using fallback")
		return &Predictor{
			available:    false,
			threshold:    0.65,
			modelPath:    path,
			timeout:      timeout,
			modelCreated: modelCreated,
			metrics:      metrics,
		}, nil
	}

	p := &Predictor{
		available:    true,
		threshold:    0.65,
		modelPath:    path,
		pythonPath:   pythonPath,
		scriptPath:   scriptPath,
		timeout:      timeout,
		modelCreated: modelCreated,
		metrics:      metrics,
	}

	// Test the model
	if err := p.healthCheck(); err != nil {
		log.Warn().Err(err).Msg("Model health check failed, using fallback")
		p.available = false
	} else {
		log.Info().Str("model_path", path).Msg("ONNX model loaded successfully")
	}

	// Update model age metric if metrics available
	if p.metrics != nil && !modelCreated.IsZero() {
		age := time.Since(modelCreated).Seconds()
		p.metrics.MLModelAgeSet(age)
	}

	return p, nil
}

// features = [tickRatio, depthRatio, priceDist]
func (p *Predictor) Approve(f []float32, threshold float64) bool {
	if p == nil {
		return false
	}

	start := time.Now()
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		// Track latency if metrics available
		if p.metrics != nil {
			p.metrics.MLLatencyObserve(time.Since(start).Seconds())
		}
	}()

	if !p.available {
		// Simple heuristic fallback when no ML model is available
		// This is a basic momentum + mean reversion strategy
		if len(f) == 3 {
			tickRatio := f[0]  // momentum signal
			depthRatio := f[1] // order book imbalance
			priceDist := f[2]  // price distance from VWAP in std devs

			// Simple rules: trade when there's momentum, good depth signal,
			// and price isn't too far from VWAP
			confidence := (abs(tickRatio) + abs(depthRatio)) / 2.0
			result := confidence > float32(threshold-0.5) && abs(priceDist) < 2.0

			// Track prediction (even heuristic) and fallback usage
			if p.metrics != nil {
				p.metrics.MLPredictionsInc()
				p.metrics.MLFallbackUseInc()
			}

			return result
		}
		return false
	}

	// Use ONNX model for prediction
	predictions, err := p.predictInternal(f)
	if err != nil {
		log.Error().Err(err).Msg("ONNX prediction failed, falling back to heuristics")
		// Track failure
		if p.metrics != nil {
			p.metrics.MLFailuresInc()
		}
		// Fall back to heuristic if ONNX fails
		p.available = false
		return p.Approve(f, threshold)
	}

	// Track successful prediction and score distribution
	if p.metrics != nil {
		p.metrics.MLPredictionsInc()
		// Track the confidence score for monitoring
		if len(predictions) >= 2 {
			p.metrics.MLPredictionScoresObserve(predictions[1])
		}
	}

	p.lastUsed = time.Now()

	// predictions[1] is the probability of reversal (positive signal)
	if len(predictions) >= 2 {
		revertProb := predictions[1]
		return revertProb > threshold
	}

	return false
}

// Predict returns prediction scores for the given features
func (p *Predictor) Predict(features []float32) ([]float32, error) {
	if p == nil {
		return nil, fmt.Errorf("predictor is nil")
	}

	p.mu.RLock()
	available := p.available
	p.mu.RUnlock()

	if !available {
		// Return dummy prediction when no model is available
		return []float32{0.8, 0.2}, nil // [no_signal, reversal]
	}

	predictions, err := p.predictInternal(features)
	if err != nil {
		return nil, err
	}

	// Convert to float32
	result := make([]float32, len(predictions))
	for i, p := range predictions {
		result[i] = float32(p)
	}

	return result, nil
}

func (p *Predictor) predictInternal(features []float32) ([]float64, error) {
	if len(features) != 3 {
		return nil, fmt.Errorf("expected 3 features, got %d", len(features))
	}

	// Validate feature values for NaN/Inf
	for i, f := range features {
		if f != f { // Check for NaN
			return nil, fmt.Errorf("feature %d is NaN", i)
		}
		if f > 1e10 || f < -1e10 { // Check for extreme values
			return nil, fmt.Errorf("feature %d has extreme value: %f", i, f)
		}
	}

	// Create prediction request
	req := PredictionRequest{Features: features}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Error().Err(err).Interface("features", features).Msg("Failed to marshal prediction request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Set timeout (use configured timeout)
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	// Run Python inference with timeout
	cmd := exec.CommandContext(ctx, p.pythonPath, p.scriptPath, p.modelPath)
	cmd.Stdin = strings.NewReader(string(reqJSON))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Enhanced error logging with context
		log.Error().
			Err(err).
			Str("python_path", p.pythonPath).
			Str("script_path", p.scriptPath).
			Str("model_path", p.modelPath).
			Str("stderr", stderr.String()).
			Str("stdout", stdout.String()).
			Interface("features", features).
			Dur("timeout", p.timeout).
			Time("last_health_check", p.healthChecked).
			Bool("context_cancelled", ctx.Err() != nil).
			Msg("Python inference execution failed")

		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			// Mark model as potentially unhealthy and track timeout
			p.healthChecked = time.Time{} // Force next health check
			if p.metrics != nil {
				p.metrics.MLTimeoutsInc()
			}
			return nil, fmt.Errorf("prediction timeout after %v (model may be overloaded)", p.timeout)
		}

		// Check for specific error types
		if strings.Contains(stderr.String(), "onnxruntime not installed") {
			return nil, fmt.Errorf("ONNX runtime dependency missing: %w", err)
		}
		if strings.Contains(stderr.String(), "No such file or directory") {
			return nil, fmt.Errorf("model file not accessible: %w", err)
		}
		if strings.Contains(stderr.String(), "Permission denied") {
			return nil, fmt.Errorf("permission denied accessing model files: %w", err)
		}

		return nil, fmt.Errorf("python inference failed: %w, stderr: %s", err, stderr.String())
	}

	// Parse response
	var resp PredictionResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		log.Error().
			Err(err).
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Interface("features", features).
			Msg("Failed to parse prediction response")
		return nil, fmt.Errorf("failed to parse response: %w, stdout: %s", err, stdout.String())
	}

	if resp.Error != "" {
		log.Error().
			Str("python_error", resp.Error).
			Interface("features", features).
			Msg("Python inference returned error")
		return nil, fmt.Errorf("python inference error: %s", resp.Error)
	}

	// Validate prediction results
	if len(resp.Probabilities) != 2 {
		log.Error().
			Int("prob_count", len(resp.Probabilities)).
			Interface("probabilities", resp.Probabilities).
			Interface("features", features).
			Msg("Invalid prediction response - expected 2 probabilities")
		return nil, fmt.Errorf("expected 2 probabilities, got %d", len(resp.Probabilities))
	}

	// Check for valid probability values
	for i, prob := range resp.Probabilities {
		if prob < 0 || prob > 1 || prob != prob { // Check bounds and NaN
			log.Error().
				Int("prob_index", i).
				Float64("prob_value", prob).
				Interface("features", features).
				Msg("Invalid probability value in prediction")
			return nil, fmt.Errorf("invalid probability %d: %f", i, prob)
		}
	}

	log.Debug().
		Interface("features", features).
		Interface("probabilities", resp.Probabilities).
		Int("prediction", resp.Prediction).
		Msg("Prediction successful")

	return resp.Probabilities, nil
}

func (p *Predictor) healthCheck() error {
	now := time.Now()
	if now.Sub(p.healthChecked) < 5*time.Minute {
		return nil // Don't check too frequently
	}

	// Test with dummy features
	testFeatures := []float32{0.1, -0.2, 0.5}
	_, err := p.predictInternal(testFeatures)
	if err == nil {
		p.healthChecked = now
	}
	return err
}

func findPython() (string, error) {
	// Try common Python executable names
	candidates := []string{"python3", "python", "python3.9", "python3.10", "python3.11"}

	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			// Verify it's Python 3
			cmd := exec.Command(path, "--version")
			output, err := cmd.Output()
			if err == nil && strings.Contains(string(output), "Python 3") {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable Python 3 executable found")
}

func createInferenceScript(scriptPath string) error {
	script := `#!/usr/bin/env python3
"""
ONNX Inference Script for Bitunix Trading Bot
Reads JSON from stdin, returns prediction JSON to stdout
"""
import sys
import json
import numpy as np

try:
    import onnxruntime as ort
except ImportError:
    print(json.dumps({"error": "onnxruntime not installed"}))
    sys.exit(1)

def main():
    if len(sys.argv) != 2:
        print(json.dumps({"error": "Usage: python onnx_inference.py <model_path>"}))
        sys.exit(1)
    
    model_path = sys.argv[1]
    
    try:
        # Read input from stdin
        request = json.load(sys.stdin)
        features = np.array([request["features"]], dtype=np.float32)
        
        # Load ONNX model
        session = ort.InferenceSession(model_path)
        input_name = session.get_inputs()[0].name
        
        # Run inference
        result = session.run(None, {input_name: features})
        
        # Format response
        if len(result) >= 2:
            probabilities = result[1][0].tolist()  # Get probability scores
            prediction = int(result[0][0])         # Get class prediction
        else:
            probabilities = result[0][0].tolist()
            prediction = int(np.argmax(probabilities))
        
        response = {
            "probabilities": probabilities,
            "prediction": prediction
        }
        
        print(json.dumps(response))
        
    except Exception as e:
        error_response = {"error": str(e)}
        print(json.dumps(error_response))
        sys.exit(1)

if __name__ == "__main__":
    main()
`

	return os.WriteFile(scriptPath, []byte(script), 0755)
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
