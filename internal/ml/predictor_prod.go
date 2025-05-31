package ml

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// ModelMetadata contains information about the loaded model
type ModelMetadata struct {
	Version       string    `json:"version"`
	TrainedAt     time.Time `json:"trained_at"`
	Features      []string  `json:"features"`
	Accuracy      float64   `json:"accuracy"`
	InputShape    []int64   `json:"input_shape"`
	OutputShape   []int64   `json:"output_shape"`
	TrainingRows  int       `json:"training_rows"`
	ValidationAcc float64   `json:"validation_accuracy"`
}

// PredictorConfig contains configuration for the predictor
type PredictorConfig struct {
	ModelPath         string
	FallbackThreshold float64
	MaxRetries        int
	RetryDelay        time.Duration
	CacheSize         int
	CacheTTL          time.Duration
	EnableProfiling   bool
	EnableValidation  bool
	MinConfidence     float64
}

// PredictionCache caches recent predictions to reduce computation
type PredictionCache struct {
	mu      sync.RWMutex
	cache   map[string]*CachedPrediction
	maxSize int
	ttl     time.Duration
}

type CachedPrediction struct {
	Score     float32
	Timestamp time.Time
}

// ModelValidator validates model predictions against expected ranges
type ModelValidator struct {
	minScore         float32
	maxScore         float32
	featureRanges    map[int][2]float32
	anomalyThreshold float64
}

// ProductionPredictor extends the basic Predictor with production features
type ProductionPredictor struct {
	*Predictor
	config       PredictorConfig
	cache        *PredictionCache
	validator    *ModelValidator
	metadata     *ModelMetadata
	healthStatus atomic.Value // stores *HealthStatus
	perfStats    *PerformanceStats
}

type HealthStatus struct {
	Healthy         bool      `json:"healthy"`
	LastCheck       time.Time `json:"last_check"`
	ModelLoaded     bool      `json:"model_loaded"`
	AverageLatency  float64   `json:"average_latency_ms"`
	PredictionCount int64     `json:"prediction_count"`
	ErrorRate       float64   `json:"error_rate"`
	CacheHitRate    float64   `json:"cache_hit_rate"`
	LastError       string    `json:"last_error,omitempty"`
	ModelVersion    string    `json:"model_version"`
	UptimeSeconds   float64   `json:"uptime_seconds"`
}

type PerformanceStats struct {
	mu               sync.RWMutex
	predictions      int64
	errors           int64
	cacheHits        int64
	cacheMisses      int64
	totalLatency     time.Duration
	startTime        time.Time
	latencyHistogram []time.Duration
}

// NewProductionPredictor creates a production-ready predictor
func NewProductionPredictor(config PredictorConfig, metrics MetricsInterface) (*ProductionPredictor, error) {
	// Create base predictor
	predictor, err := NewWithMetrics(config.ModelPath, metrics, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create base predictor: %w", err)
	}

	// Load model metadata
	metadata, err := loadModelMetadata(config.ModelPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load model metadata, using defaults")
		metadata = &ModelMetadata{
			Version:   "unknown",
			TrainedAt: time.Now(),
			Features:  []string{"tick_ratio", "depth_ratio", "price_distance"},
		}
	}

	pp := &ProductionPredictor{
		Predictor: predictor,
		config:    config,
		cache: &PredictionCache{
			cache:   make(map[string]*CachedPrediction),
			maxSize: config.CacheSize,
			ttl:     config.CacheTTL,
		},
		validator: &ModelValidator{
			minScore:         0.0,
			maxScore:         1.0,
			featureRanges:    getDefaultFeatureRanges(),
			anomalyThreshold: 0.95,
		},
		metadata: metadata,
		perfStats: &PerformanceStats{
			startTime:        time.Now(),
			latencyHistogram: make([]time.Duration, 0, 1000),
		},
	}

	// Initialize health status
	pp.updateHealthStatus()

	// Start background health checker
	go pp.backgroundHealthChecker()

	// Start cache cleaner
	go pp.backgroundCacheCleaner()

	return pp, nil
}

// PredictWithContext performs prediction with context and validation
func (pp *ProductionPredictor) PredictWithContext(ctx context.Context, features []float32) (float32, error) {
	start := time.Now()
	defer func() {
		pp.recordLatency(time.Since(start))
	}()

	// Check context
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// Validate input
	if err := pp.validateInput(features); err != nil {
		pp.recordError(err)
		return 0, fmt.Errorf("input validation failed: %w", err)
	}

	// Check cache
	cacheKey := pp.getCacheKey(features)
	if cached := pp.getFromCache(cacheKey); cached != nil {
		pp.recordCacheHit()
		return cached.Score, nil
	}
	pp.recordCacheMiss()

	// Perform prediction
	scores, err := pp.Predictor.Predict(features)
	if err != nil {
		pp.recordError(err)
		return 0, err
	}
	if len(scores) == 0 {
		err := fmt.Errorf("empty prediction result")
		pp.recordError(err)
		return 0, err
	}
	// By convention scores[1] is the reversal probability when available.
	var score float32
	if len(scores) >= 2 {
		score = scores[1]
	} else {
		score = scores[0]
	}

	// Validate output
	if err := pp.validateOutput(score); err != nil {
		pp.recordError(err)
		return 0, fmt.Errorf("output validation failed: %w", err)
	}

	// Cache result
	pp.putInCache(cacheKey, score)

	pp.recordPrediction()
	return score, nil
}

// ApproveWithContext is the context-aware version of Approve
func (pp *ProductionPredictor) ApproveWithContext(ctx context.Context, features []float32, threshold float64) bool {
	score, err := pp.PredictWithContext(ctx, features)
	if err != nil {
		log.Warn().Err(err).Msg("prediction failed, using fallback")
		return pp.fallbackHeuristic(features, threshold)
	}
	return float64(score) > threshold
}

// validateInput checks if input features are within expected ranges
func (pp *ProductionPredictor) validateInput(features []float32) error {
	if len(features) != len(pp.metadata.Features) {
		return fmt.Errorf("expected %d features, got %d", len(pp.metadata.Features), len(features))
	}

	for i, val := range features {
		if ranges, ok := pp.validator.featureRanges[i]; ok {
			if val < ranges[0] || val > ranges[1] {
				return fmt.Errorf("feature %d value %.4f outside range [%.4f, %.4f]",
					i, val, ranges[0], ranges[1])
			}
		}
	}

	return nil
}

// validateOutput checks if output is within expected range
func (pp *ProductionPredictor) validateOutput(score float32) error {
	if score < pp.validator.minScore || score > pp.validator.maxScore {
		return fmt.Errorf("score %.4f outside valid range [%.4f, %.4f]",
			score, pp.validator.minScore, pp.validator.maxScore)
	}
	return nil
}

// getCacheKey generates a cache key from features
func (pp *ProductionPredictor) getCacheKey(features []float32) string {
	// Simple key generation - in production, use a proper hash
	return fmt.Sprintf("%.4f_%.4f_%.4f", features[0], features[1], features[2])
}

// getFromCache retrieves a cached prediction
func (pp *ProductionPredictor) getFromCache(key string) *CachedPrediction {
	pp.cache.mu.RLock()
	defer pp.cache.mu.RUnlock()

	if cached, ok := pp.cache.cache[key]; ok {
		if time.Since(cached.Timestamp) < pp.cache.ttl {
			return cached
		}
	}
	return nil
}

// putInCache stores a prediction in cache
func (pp *ProductionPredictor) putInCache(key string, score float32) {
	pp.cache.mu.Lock()
	defer pp.cache.mu.Unlock()

	// Simple LRU eviction
	if len(pp.cache.cache) >= pp.cache.maxSize {
		// Remove oldest entry
		var oldestKey string
		var oldestTime time.Time
		for k, v := range pp.cache.cache {
			if oldestTime.IsZero() || v.Timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.Timestamp
			}
		}
		delete(pp.cache.cache, oldestKey)
	}

	pp.cache.cache[key] = &CachedPrediction{
		Score:     score,
		Timestamp: time.Now(),
	}
}

// backgroundCacheCleaner periodically removes expired cache entries
func (pp *ProductionPredictor) backgroundCacheCleaner() {
	ticker := time.NewTicker(pp.cache.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		pp.cleanCache()
	}
}

func (pp *ProductionPredictor) cleanCache() {
	pp.cache.mu.Lock()
	defer pp.cache.mu.Unlock()

	now := time.Now()
	for key, cached := range pp.cache.cache {
		if now.Sub(cached.Timestamp) > pp.cache.ttl {
			delete(pp.cache.cache, key)
		}
	}
}

// backgroundHealthChecker periodically updates health status
func (pp *ProductionPredictor) backgroundHealthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pp.updateHealthStatus()
	}
}

func (pp *ProductionPredictor) updateHealthStatus() {
	pp.perfStats.mu.RLock()
	predictions := pp.perfStats.predictions
	errors := pp.perfStats.errors
	cacheHits := pp.perfStats.cacheHits
	cacheMisses := pp.perfStats.cacheMisses
	totalLatency := pp.perfStats.totalLatency
	uptime := time.Since(pp.perfStats.startTime)
	pp.perfStats.mu.RUnlock()

	var avgLatency float64
	if predictions > 0 {
		avgLatency = float64(totalLatency.Milliseconds()) / float64(predictions)
	}

	var errorRate float64
	if predictions > 0 {
		errorRate = float64(errors) / float64(predictions)
	}

	var cacheHitRate float64
	totalCacheAccess := cacheHits + cacheMisses
	if totalCacheAccess > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalCacheAccess)
	}

	modelLoaded := pp.Predictor != nil && pp.Predictor.available
	status := &HealthStatus{
		Healthy:         modelLoaded && errorRate < 0.1,
		LastCheck:       time.Now(),
		ModelLoaded:     modelLoaded,
		AverageLatency:  avgLatency,
		PredictionCount: predictions,
		ErrorRate:       errorRate,
		CacheHitRate:    cacheHitRate,
		ModelVersion:    pp.metadata.Version,
		UptimeSeconds:   uptime.Seconds(),
	}

	pp.healthStatus.Store(status)
}

// GetHealthStatus returns the current health status
func (pp *ProductionPredictor) GetHealthStatus() *HealthStatus {
	status := pp.healthStatus.Load()
	if status == nil {
		return &HealthStatus{Healthy: false}
	}
	return status.(*HealthStatus)
}

// Performance tracking methods
func (pp *ProductionPredictor) recordLatency(d time.Duration) {
	pp.perfStats.mu.Lock()
	defer pp.perfStats.mu.Unlock()

	pp.perfStats.totalLatency += d
	pp.perfStats.latencyHistogram = append(pp.perfStats.latencyHistogram, d)

	// Keep only last 1000 samples
	if len(pp.perfStats.latencyHistogram) > 1000 {
		pp.perfStats.latencyHistogram = pp.perfStats.latencyHistogram[1:]
	}
}

func (pp *ProductionPredictor) recordPrediction() {
	pp.perfStats.mu.Lock()
	pp.perfStats.predictions++
	pp.perfStats.mu.Unlock()
}

func (pp *ProductionPredictor) recordError(err error) {
	pp.perfStats.mu.Lock()
	pp.perfStats.errors++
	pp.perfStats.mu.Unlock()

	// Update health status with error
	if status := pp.GetHealthStatus(); status != nil {
		status.LastError = err.Error()
		pp.healthStatus.Store(status)
	}
}

func (pp *ProductionPredictor) recordCacheHit() {
	pp.perfStats.mu.Lock()
	pp.perfStats.cacheHits++
	pp.perfStats.mu.Unlock()
}

func (pp *ProductionPredictor) recordCacheMiss() {
	pp.perfStats.mu.Lock()
	pp.perfStats.cacheMisses++
	pp.perfStats.mu.Unlock()
}

// GetPerformanceMetrics returns detailed performance metrics
func (pp *ProductionPredictor) GetPerformanceMetrics() map[string]interface{} {
	pp.perfStats.mu.RLock()
	defer pp.perfStats.mu.RUnlock()

	// Calculate percentiles
	var p50, p95, p99 time.Duration
	if len(pp.perfStats.latencyHistogram) > 0 {
		sorted := make([]time.Duration, len(pp.perfStats.latencyHistogram))
		copy(sorted, pp.perfStats.latencyHistogram)
		// Simple percentile calculation (should use proper algorithm in production)
		p50 = sorted[len(sorted)/2]
		p95 = sorted[int(float64(len(sorted))*0.95)]
		p99 = sorted[int(float64(len(sorted))*0.99)]
	}

	return map[string]interface{}{
		"predictions_total": pp.perfStats.predictions,
		"errors_total":      pp.perfStats.errors,
		"cache_hits":        pp.perfStats.cacheHits,
		"cache_misses":      pp.perfStats.cacheMisses,
		"latency_p50_ms":    p50.Milliseconds(),
		"latency_p95_ms":    p95.Milliseconds(),
		"latency_p99_ms":    p99.Milliseconds(),
		"uptime_hours":      time.Since(pp.perfStats.startTime).Hours(),
	}
}

// Helper functions
func loadModelMetadata(modelPath string) (*ModelMetadata, error) {
	dir := filepath.Dir(modelPath)
	primary := filepath.Join(dir, "model_metadata.json")

	if md, err := decodeMetadata(primary); err == nil {
		return md, nil
	}

	// Fallback: pick the newest metadata file by timestamp suffix
	pattern := filepath.Join(dir, "model_metadata_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("no metadata files found: %w", err)
	}
	sort.Strings(matches)                          // chronological order
	return decodeMetadata(matches[len(matches)-1]) // newest
}

// Helper to keep original decoding logic in one place
func decodeMetadata(path string) (*ModelMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var md ModelMetadata
	if err := json.NewDecoder(file).Decode(&md); err != nil {
		return nil, err
	}
	return &md, nil
}

func getDefaultFeatureRanges() map[int][2]float32 {
	return map[int][2]float32{
		0: {-1.0, 1.0}, // tick_ratio
		1: {-1.0, 1.0}, // depth_ratio
		2: {-5.0, 5.0}, // price_distance (z-score)
	}
}

// fallbackHeuristic provides a simple rule-based decision when ML fails
func (pp *ProductionPredictor) fallbackHeuristic(features []float32, threshold float64) bool {
	if len(features) < 3 {
		return false
	}

	tickRatio := features[0]
	depthRatio := features[1]
	priceDist := features[2]

	// Conservative heuristic
	score := float64(0.5)

	// Adjust based on features
	if tickRatio > 0.3 {
		score += 0.2
	} else if tickRatio < -0.3 {
		score -= 0.2
	}

	if depthRatio > 0.2 {
		score += 0.15
	} else if depthRatio < -0.2 {
		score -= 0.15
	}

	// Price distance is most important
	if priceDist > 1.5 && priceDist < 3.0 {
		score += 0.3
	} else if priceDist < -1.5 && priceDist > -3.0 {
		score += 0.3
	} else if priceDist > 3.0 || priceDist < -3.0 {
		score -= 0.4 // Too far from mean
	}

	return score > threshold
}
