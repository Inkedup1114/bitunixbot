package ml

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// ModelServer provides HTTP API for model predictions
type ModelServer struct {
	predictor *ProductionPredictor
	server    *http.Server
}

// PredictionRequest represents the incoming prediction request
type PredictionRequest struct {
	Features  []float32 `json:"features"`
	RequestID string    `json:"request_id,omitempty"`
}

// PredictionResponse represents the prediction result
type PredictionResponse struct {
	Score        float32                `json:"score"`
	Approved     bool                   `json:"approved"`
	Threshold    float64                `json:"threshold"`
	RequestID    string                 `json:"request_id,omitempty"`
	ModelVersion string                 `json:"model_version"`
	Latency      float64                `json:"latency_ms"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// NewModelServer creates a new HTTP server for model serving
func NewModelServer(predictor *ProductionPredictor, port int) *ModelServer {
	ms := &ModelServer{
		predictor: predictor,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/predict", ms.handlePredict)
	mux.HandleFunc("/health", ms.handleHealth)
	mux.HandleFunc("/metrics", ms.handleMetrics)
	mux.HandleFunc("/model/info", ms.handleModelInfo)

	ms.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return ms
}

// Start begins serving HTTP requests
func (ms *ModelServer) Start() error {
	log.Info().Str("addr", ms.server.Addr).Msg("starting model server")
	return ms.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (ms *ModelServer) Shutdown(ctx context.Context) error {
	return ms.server.Shutdown(ctx)
}

func (ms *ModelServer) handlePredict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	var req PredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if len(req.Features) == 0 {
		http.Error(w, "features cannot be empty", http.StatusBadRequest)
		return
	}

	// Perform prediction
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	score, err := ms.predictor.PredictWithContext(ctx, req.Features)
	if err != nil {
		log.Error().Err(err).Msg("prediction failed")
		http.Error(w, fmt.Sprintf("prediction failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Get threshold from config
	threshold := ms.predictor.config.FallbackThreshold
	approved := float64(score) > threshold

	resp := PredictionResponse{
		Score:        score,
		Approved:     approved,
		Threshold:    threshold,
		RequestID:    req.RequestID,
		ModelVersion: ms.predictor.metadata.Version,
		Latency:      float64(time.Since(start).Milliseconds()),
		Timestamp:    time.Now(),
		Metadata: map[string]interface{}{
			"features_received": len(req.Features),
			"cache_enabled":     ms.predictor.config.CacheSize > 0,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (ms *ModelServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := ms.predictor.GetHealthStatus()

	status := http.StatusOK
	if !health.Healthy {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

func (ms *ModelServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ms.predictor.GetPerformanceMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (ms *ModelServer) handleModelInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":        ms.predictor.metadata.Version,
		"trained_at":     ms.predictor.metadata.TrainedAt,
		"features":       ms.predictor.metadata.Features,
		"accuracy":       ms.predictor.metadata.Accuracy,
		"validation_acc": ms.predictor.metadata.ValidationAcc,
		"training_rows":  ms.predictor.metadata.TrainingRows,
		"input_shape":    ms.predictor.metadata.InputShape,
		"output_shape":   ms.predictor.metadata.OutputShape,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
