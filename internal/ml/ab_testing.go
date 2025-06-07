package ml

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ABTestManager manages A/B testing for multiple ML models
type ABTestManager struct {
	mu          sync.RWMutex
	enabled     bool
	experiments map[string]*ABExperiment
	savePath    string
	rng         *rand.Rand
}

// ABExperiment represents a single A/B test experiment
type ABExperiment struct {
	ID              string                        `json:"id"`
	Name            string                        `json:"name"`
	Description     string                        `json:"description"`
	StartTime       time.Time                     `json:"start_time"`
	EndTime         time.Time                     `json:"end_time"`
	Status          ExperimentStatus              `json:"status"`
	TrafficSplit    map[string]float64            `json:"traffic_split"`
	Models          map[string]ModelConfig        `json:"models"`
	Metrics         map[string]*ExperimentMetrics `json:"metrics"`
	Criteria        SuccessCriteria               `json:"criteria"`
	StatisticalTest StatisticalTestConfig         `json:"statistical_test"`
	LastUpdated     time.Time                     `json:"last_updated"`
}

// ModelConfig contains configuration for a model variant in A/B test
type ModelConfig struct {
	Name        string `json:"name"`
	ModelPath   string `json:"model_path"`
	Version     string `json:"version"`
	IsControl   bool   `json:"is_control"`
	Description string `json:"description"`
}

// ExperimentMetrics tracks metrics for a model variant
type ExperimentMetrics struct {
	SampleCount    int64     `json:"sample_count"`
	ConversionRate float64   `json:"conversion_rate"`
	AverageValue   float64   `json:"average_value"`
	Accuracy       float64   `json:"accuracy"`
	Precision      float64   `json:"precision"`
	Recall         float64   `json:"recall"`
	F1Score        float64   `json:"f1_score"`
	LatencyP50     float64   `json:"latency_p50"`
	LatencyP95     float64   `json:"latency_p95"`
	ErrorRate      float64   `json:"error_rate"`
	LastUpdated    time.Time `json:"last_updated"`
	ConversionsSum int64     `json:"conversions_sum"`
	ValuesSum      float64   `json:"values_sum"`
	LatencyValues  []float64 `json:"latency_values"`
	AccuracyValues []float64 `json:"accuracy_values"`
}

// ExperimentStatus represents the status of an experiment
type ExperimentStatus string

const (
	StatusDraft     ExperimentStatus = "draft"
	StatusRunning   ExperimentStatus = "running"
	StatusPaused    ExperimentStatus = "paused"
	StatusCompleted ExperimentStatus = "completed"
	StatusAborted   ExperimentStatus = "aborted"
)

// SuccessCriteria defines what constitutes success for the experiment
type SuccessCriteria struct {
	PrimaryMetric     string  `json:"primary_metric"`
	MinimumEffect     float64 `json:"minimum_effect"`
	PowerLevel        float64 `json:"power_level"`
	SignificanceLevel float64 `json:"significance_level"`
	MinSampleSize     int64   `json:"min_sample_size"`
	MaxDurationDays   int     `json:"max_duration_days"`
}

// StatisticalTestConfig configures statistical testing
type StatisticalTestConfig struct {
	TestType            string  `json:"test_type"`
	ConfidenceLevel     float64 `json:"confidence_level"`
	PowerLevel          float64 `json:"power_level"`
	MinDetectableEffect float64 `json:"min_detectable_effect"`
}

// ABTestResult represents the result of A/B test analysis
type ABTestResult struct {
	ExperimentID     string                   `json:"experiment_id"`
	Winner           string                   `json:"winner"`
	ConfidenceLevel  float64                  `json:"confidence_level"`
	PValue           float64                  `json:"p_value"`
	EffectSize       float64                  `json:"effect_size"`
	StatisticalPower float64                  `json:"statistical_power"`
	IsSignificant    bool                     `json:"is_significant"`
	Recommendation   string                   `json:"recommendation"`
	VariantResults   map[string]VariantResult `json:"variant_results"`
	Timestamp        time.Time                `json:"timestamp"`
}

// VariantResult contains results for a single variant
type VariantResult struct {
	SampleSize         int64      `json:"sample_size"`
	ConversionRate     float64    `json:"conversion_rate"`
	ConfidenceInterval [2]float64 `json:"confidence_interval"`
	StandardError      float64    `json:"standard_error"`
}

// ABTestConfig configures the A/B testing framework
type ABTestConfig struct {
	Enabled  bool   `yaml:"enabled"`
	SavePath string `yaml:"save_path"`
	Seed     int64  `yaml:"seed"`
}

// NewABTestManager creates a new A/B test manager
func NewABTestManager(config ABTestConfig) *ABTestManager {
	seed := config.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	manager := &ABTestManager{
		enabled:     config.Enabled,
		experiments: make(map[string]*ABExperiment),
		savePath:    config.SavePath,
		rng:         rand.New(rand.NewSource(seed)),
	}

	// Load existing experiments if available
	if config.SavePath != "" {
		if err := manager.Load(); err != nil {
			log.Warn().Err(err).Msg("Failed to load A/B test data")
		}
	}

	return manager
}

// CreateExperiment creates a new A/B test experiment
func (ab *ABTestManager) CreateExperiment(
	id, name, description string,
	models map[string]ModelConfig,
	trafficSplit map[string]float64,
	criteria SuccessCriteria,
) (*ABExperiment, error) {
	if !ab.enabled {
		return nil, fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	// Validate traffic split
	totalTraffic := 0.0
	for _, split := range trafficSplit {
		totalTraffic += split
	}
	if math.Abs(totalTraffic-1.0) > 0.001 {
		return nil, fmt.Errorf("traffic split must sum to 1.0, got %.3f", totalTraffic)
	}

	// Validate that all models in traffic split exist
	for variant := range trafficSplit {
		if _, exists := models[variant]; !exists {
			return nil, fmt.Errorf("variant %s in traffic split not found in models", variant)
		}
	}

	experiment := &ABExperiment{
		ID:           id,
		Name:         name,
		Description:  description,
		StartTime:    time.Now(),
		Status:       StatusDraft,
		TrafficSplit: trafficSplit,
		Models:       models,
		Metrics:      make(map[string]*ExperimentMetrics),
		Criteria:     criteria,
		StatisticalTest: StatisticalTestConfig{
			TestType:            "t_test",
			ConfidenceLevel:     0.95,
			PowerLevel:          0.8,
			MinDetectableEffect: criteria.MinimumEffect,
		},
		LastUpdated: time.Now(),
	}

	// Initialize metrics for each variant
	for variant := range models {
		experiment.Metrics[variant] = &ExperimentMetrics{
			LastUpdated:    time.Now(),
			LatencyValues:  make([]float64, 0, 1000),
			AccuracyValues: make([]float64, 0, 1000),
		}
	}

	ab.experiments[id] = experiment

	log.Info().
		Str("experiment_id", id).
		Str("name", name).
		Int("variants", len(models)).
		Msg("A/B test experiment created")

	return experiment, ab.save()
}

// StartExperiment starts an experiment
func (ab *ABTestManager) StartExperiment(experimentID string) error {
	if !ab.enabled {
		return fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment %s not found", experimentID)
	}

	if experiment.Status != StatusDraft && experiment.Status != StatusPaused {
		return fmt.Errorf("can only start draft or paused experiments, current status: %s", experiment.Status)
	}

	experiment.Status = StatusRunning
	experiment.StartTime = time.Now()
	experiment.LastUpdated = time.Now()

	log.Info().
		Str("experiment_id", experimentID).
		Str("name", experiment.Name).
		Msg("A/B test experiment started")

	return ab.save()
}

// GetVariantForUser determines which model variant to use for a given user
func (ab *ABTestManager) GetVariantForUser(experimentID, userID string) (string, error) {
	if !ab.enabled {
		return "", fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return "", fmt.Errorf("experiment %s not found", experimentID)
	}

	if experiment.Status != StatusRunning {
		return "", fmt.Errorf("experiment %s is not running (status: %s)", experimentID, experiment.Status)
	}

	// Use hash of user ID to ensure consistent assignment
	hash := ab.hashUser(userID, experimentID)

	// Map hash to variant based on traffic split
	cumulative := 0.0
	for variant, split := range experiment.TrafficSplit {
		cumulative += split
		if hash <= cumulative {
			return variant, nil
		}
	}

	// Fallback to first variant (shouldn't happen with proper traffic split)
	for variant := range experiment.TrafficSplit {
		return variant, nil
	}

	return "", fmt.Errorf("no variant found for user %s", userID)
}

// RecordEvent records an event for a specific variant in an experiment
func (ab *ABTestManager) RecordEvent(experimentID, variant string, converted bool, value float64, latency time.Duration, accuracy float64) error {
	if !ab.enabled {
		return nil
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment %s not found", experimentID)
	}

	if experiment.Status != StatusRunning {
		return nil // Silently ignore events for non-running experiments
	}

	metrics, exists := experiment.Metrics[variant]
	if !exists {
		return fmt.Errorf("variant %s not found in experiment %s", variant, experimentID)
	}

	// Update metrics
	metrics.SampleCount++

	if converted {
		metrics.ConversionsSum++
	}
	metrics.ConversionRate = float64(metrics.ConversionsSum) / float64(metrics.SampleCount)

	metrics.ValuesSum += value
	metrics.AverageValue = metrics.ValuesSum / float64(metrics.SampleCount)

	// Update accuracy with exponential moving average
	alpha := 0.1
	if metrics.SampleCount == 1 {
		metrics.Accuracy = accuracy
	} else {
		metrics.Accuracy = alpha*accuracy + (1-alpha)*metrics.Accuracy
	}

	// Track latency (keep only recent values to calculate percentiles)
	latencyMs := float64(latency.Milliseconds())
	if len(metrics.LatencyValues) >= 1000 {
		metrics.LatencyValues = metrics.LatencyValues[1:]
	}
	metrics.LatencyValues = append(metrics.LatencyValues, latencyMs)

	// Calculate latency percentiles
	if len(metrics.LatencyValues) > 0 {
		sorted := make([]float64, len(metrics.LatencyValues))
		copy(sorted, metrics.LatencyValues)
		ab.sortFloat64Slice(sorted)

		n := len(sorted)
		metrics.LatencyP50 = sorted[n/2]
		metrics.LatencyP95 = sorted[int(float64(n)*0.95)]
	}

	metrics.LastUpdated = time.Now()
	experiment.LastUpdated = time.Now()

	return nil
}

// AnalyzeExperiment performs statistical analysis of an experiment
func (ab *ABTestManager) AnalyzeExperiment(experimentID string) (*ABTestResult, error) {
	if !ab.enabled {
		return nil, fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return nil, fmt.Errorf("experiment %s not found", experimentID)
	}

	// Find control variant
	var controlVariant string
	for variant, model := range experiment.Models {
		if model.IsControl {
			controlVariant = variant
			break
		}
	}
	if controlVariant == "" {
		return nil, fmt.Errorf("no control variant found in experiment")
	}

	controlMetrics := experiment.Metrics[controlVariant]

	result := &ABTestResult{
		ExperimentID:    experimentID,
		ConfidenceLevel: experiment.StatisticalTest.ConfidenceLevel,
		VariantResults:  make(map[string]VariantResult),
		Timestamp:       time.Now(),
	}

	// Analyze each variant against control
	maxEffectSize := 0.0
	var bestVariant string
	maxPValue := 1.0

	for variant, metrics := range experiment.Metrics {
		if variant == controlVariant {
			continue
		}

		// Perform statistical test (simplified t-test)
		pValue, effectSize := ab.performTTest(controlMetrics, metrics)

		result.VariantResults[variant] = VariantResult{
			SampleSize:         metrics.SampleCount,
			ConversionRate:     metrics.ConversionRate,
			ConfidenceInterval: ab.calculateConfidenceInterval(metrics),
			StandardError:      ab.calculateStandardError(metrics),
		}

		if effectSize > maxEffectSize {
			maxEffectSize = effectSize
			bestVariant = variant
			maxPValue = pValue
		}
	}

	// Add control to results
	result.VariantResults[controlVariant] = VariantResult{
		SampleSize:         controlMetrics.SampleCount,
		ConversionRate:     controlMetrics.ConversionRate,
		ConfidenceInterval: ab.calculateConfidenceInterval(controlMetrics),
		StandardError:      ab.calculateStandardError(controlMetrics),
	}

	// Determine winner and significance
	alpha := 1.0 - experiment.StatisticalTest.ConfidenceLevel
	result.IsSignificant = maxPValue < alpha
	result.PValue = maxPValue
	result.EffectSize = maxEffectSize

	if result.IsSignificant && maxEffectSize > experiment.Criteria.MinimumEffect {
		result.Winner = bestVariant
		result.Recommendation = fmt.Sprintf("Implement variant %s (%.2f%% improvement)", bestVariant, maxEffectSize*100)
	} else if result.IsSignificant {
		result.Winner = bestVariant
		result.Recommendation = fmt.Sprintf("Variant %s shows statistical significance but effect size (%.2f%%) is below minimum threshold (%.2f%%)",
			bestVariant, maxEffectSize*100, experiment.Criteria.MinimumEffect*100)
	} else {
		result.Winner = controlVariant
		result.Recommendation = "No significant difference found. Keep current model (control)."
	}

	// Calculate statistical power (simplified)
	result.StatisticalPower = ab.calculateStatisticalPower(controlMetrics, experiment.Metrics[bestVariant])

	return result, nil
}

// CompleteExperiment marks an experiment as completed
func (ab *ABTestManager) CompleteExperiment(experimentID string) error {
	if !ab.enabled {
		return fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment %s not found", experimentID)
	}

	experiment.Status = StatusCompleted
	experiment.EndTime = time.Now()
	experiment.LastUpdated = time.Now()

	log.Info().
		Str("experiment_id", experimentID).
		Str("name", experiment.Name).
		Msg("A/B test experiment completed")

	return ab.save()
}

// ListExperiments returns all experiments
func (ab *ABTestManager) ListExperiments() map[string]*ABExperiment {
	if !ab.enabled {
		return nil
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	// Return copies to avoid race conditions
	result := make(map[string]*ABExperiment)
	for id, experiment := range ab.experiments {
		exp := *experiment
		result[id] = &exp
	}

	return result
}

// GetExperiment returns a specific experiment
func (ab *ABTestManager) GetExperiment(experimentID string) (*ABExperiment, error) {
	if !ab.enabled {
		return nil, fmt.Errorf("A/B testing is disabled")
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	experiment, exists := ab.experiments[experimentID]
	if !exists {
		return nil, fmt.Errorf("experiment %s not found", experimentID)
	}

	// Return a copy
	exp := *experiment
	return &exp, nil
}

// Helper methods
func (ab *ABTestManager) hashUser(userID, experimentID string) float64 {
	hasher := md5.New()
	hasher.Write([]byte(userID + experimentID))
	hashBytes := hasher.Sum(nil)
	hashString := hex.EncodeToString(hashBytes)

	// Convert first 8 characters of hex to float between 0 and 1
	var hashInt uint64
	for i := 0; i < 8 && i < len(hashString); i++ {
		hashInt = hashInt*16 + uint64(ab.hexCharToInt(hashString[i]))
	}

	return float64(hashInt) / float64(0xFFFFFFFF)
}

func (ab *ABTestManager) hexCharToInt(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c - 'a' + 10)
	}
	if c >= 'A' && c <= 'F' {
		return int(c - 'A' + 10)
	}
	return 0
}

func (ab *ABTestManager) performTTest(control, treatment *ExperimentMetrics) (pValue, effectSize float64) {
	// Simplified two-sample t-test for conversion rates
	if control.SampleCount == 0 || treatment.SampleCount == 0 {
		return 1.0, 0.0
	}

	p1 := control.ConversionRate
	p2 := treatment.ConversionRate
	n1 := float64(control.SampleCount)
	n2 := float64(treatment.SampleCount)

	// Effect size (difference in conversion rates)
	effectSize = p2 - p1

	// Pooled standard error
	pooledP := (p1*n1 + p2*n2) / (n1 + n2)
	se := math.Sqrt(pooledP * (1 - pooledP) * (1/n1 + 1/n2))

	if se == 0 {
		return 1.0, effectSize
	}

	// t-statistic
	t := effectSize / se

	// Simplified p-value calculation (approximate)
	// For a more accurate implementation, use a t-distribution lookup table
	absT := math.Abs(t)
	if absT > 2.58 {
		pValue = 0.01
	} else if absT > 1.96 {
		pValue = 0.05
	} else if absT > 1.65 {
		pValue = 0.1
	} else {
		pValue = 0.2
	}

	return pValue, effectSize
}

func (ab *ABTestManager) calculateConfidenceInterval(metrics *ExperimentMetrics) [2]float64 {
	if metrics.SampleCount == 0 {
		return [2]float64{0, 0}
	}

	p := metrics.ConversionRate
	n := float64(metrics.SampleCount)

	// 95% confidence interval for proportion
	z := 1.96 // 95% confidence level
	se := math.Sqrt(p * (1 - p) / n)
	margin := z * se

	return [2]float64{
		math.Max(0, p-margin),
		math.Min(1, p+margin),
	}
}

func (ab *ABTestManager) calculateStandardError(metrics *ExperimentMetrics) float64 {
	if metrics.SampleCount == 0 {
		return 0
	}

	p := metrics.ConversionRate
	n := float64(metrics.SampleCount)

	return math.Sqrt(p * (1 - p) / n)
}

func (ab *ABTestManager) calculateStatisticalPower(control, treatment *ExperimentMetrics) float64 {
	// Simplified power calculation
	// In practice, this would involve more complex statistical calculations

	if control.SampleCount < 100 || treatment.SampleCount < 100 {
		return 0.5 // Low power with small sample sizes
	}

	effectSize := math.Abs(treatment.ConversionRate - control.ConversionRate)

	if effectSize > 0.1 {
		return 0.9 // High power for large effect sizes
	} else if effectSize > 0.05 {
		return 0.8
	} else if effectSize > 0.02 {
		return 0.6
	}

	return 0.4 // Low power for small effect sizes
}

func (ab *ABTestManager) sortFloat64Slice(slice []float64) {
	// Simple bubble sort for small slices
	n := len(slice)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if slice[j] > slice[j+1] {
				slice[j], slice[j+1] = slice[j+1], slice[j]
			}
		}
	}
}

// Save saves all experiments to disk
func (ab *ABTestManager) save() error {
	if ab.savePath == "" {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(ab.savePath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ab.experiments, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ab.savePath, data, 0o600)
}

// Load loads experiments from disk
func (ab *ABTestManager) Load() error {
	if ab.savePath == "" {
		return nil
	}

	data, err := os.ReadFile(ab.savePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	return json.Unmarshal(data, &ab.experiments)
}

// IsEnabled returns whether A/B testing is enabled
func (ab *ABTestManager) IsEnabled() bool {
	return ab.enabled
}
