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
	port      int
}

// NewModelServer creates a new HTTP server for model serving
func NewModelServer(predictor *ProductionPredictor, port int) *ModelServer {
	ms := &ModelServer{
		predictor: predictor,
		port:      port,
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
	mux := http.NewServeMux()
	mux.HandleFunc("/predict", ms.handlePredict)
	mux.HandleFunc("/health", ms.handleHealth)

	ms.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", ms.port),
		Handler: mux,
	}

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
		resp := PredictionResponse{
			Error: fmt.Sprintf("prediction failed: %v", err),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Convert score to probabilities format (binary classification)
	prob0 := float64(1.0 - score)
	prob1 := float64(score)

	resp := PredictionResponse{
		Probabilities: []float64{prob0, prob1},
		Prediction:    0,
	}

	// Set prediction based on score
	if score > 0.5 {
		resp.Prediction = 1
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
