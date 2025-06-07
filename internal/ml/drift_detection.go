package ml

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DriftDetector detects when the input data distribution changes from training time
type DriftDetector struct {
	mu               sync.RWMutex
	enabled          bool
	featureNames     []string
	baselineStats    map[string]*FeatureDistribution
	currentStats     map[string]*FeatureDistribution
	driftThresholds  map[string]float64
	alertThreshold   float64
	windowSize       int
	savePath         string
	lastAlertTime    time.Time
	alertCooldown    time.Duration
	detectionMethods []DriftDetectionMethod
}

// FeatureDistribution contains distribution statistics for a feature
type FeatureDistribution struct {
	Mean          float64   `json:"mean"`
	StandardDev   float64   `json:"standard_dev"`
	Min           float64   `json:"min"`
	Max           float64   `json:"max"`
	Percentiles   []float64 `json:"percentiles"` // 25th, 50th, 75th percentiles
	SampleCount   int64     `json:"sample_count"`
	LastUpdated   time.Time `json:"last_updated"`
	RecentSamples []float64 `json:"recent_samples"`
	KSStatistic   float64   `json:"ks_statistic"` // Kolmogorov-Smirnov test statistic
	PSIScore      float64   `json:"psi_score"`    // Population Stability Index
}

// DriftDetectionMethod represents different methods for detecting drift
type DriftDetectionMethod string

const (
	KolmogorovSmirnovTest    DriftDetectionMethod = "kolmogorov_smirnov"
	PopulationStabilityIndex DriftDetectionMethod = "population_stability_index"
	StatisticalMoments       DriftDetectionMethod = "statistical_moments"
	ChiSquareTest            DriftDetectionMethod = "chi_square"
)

// DriftAlert represents a drift detection alert
type DriftAlert struct {
	Timestamp      time.Time            `json:"timestamp"`
	FeatureName    string               `json:"feature_name"`
	Method         DriftDetectionMethod `json:"method"`
	DriftScore     float64              `json:"drift_score"`
	Threshold      float64              `json:"threshold"`
	Severity       string               `json:"severity"`
	Description    string               `json:"description"`
	Recommendation string               `json:"recommendation"`
}

// DriftDetectionConfig configures drift detection
type DriftDetectionConfig struct {
	Enabled          bool                   `yaml:"enabled"`
	FeatureNames     []string               `yaml:"feature_names"`
	SavePath         string                 `yaml:"save_path"`
	AlertThreshold   float64                `yaml:"alert_threshold"`
	WindowSize       int                    `yaml:"window_size"`
	AlertCooldown    time.Duration          `yaml:"alert_cooldown"`
	DetectionMethods []DriftDetectionMethod `yaml:"detection_methods"`
	DriftThresholds  map[string]float64     `yaml:"drift_thresholds"`
}

// NewDriftDetector creates a new drift detector
func NewDriftDetector(config DriftDetectionConfig) *DriftDetector {
	dd := &DriftDetector{
		enabled:          config.Enabled,
		featureNames:     config.FeatureNames,
		baselineStats:    make(map[string]*FeatureDistribution),
		currentStats:     make(map[string]*FeatureDistribution),
		driftThresholds:  config.DriftThresholds,
		alertThreshold:   config.AlertThreshold,
		windowSize:       config.WindowSize,
		savePath:         config.SavePath,
		alertCooldown:    config.AlertCooldown,
		detectionMethods: config.DetectionMethods,
	}

	// Set default values
	if dd.alertThreshold == 0 {
		dd.alertThreshold = 0.1
	}
	if dd.windowSize == 0 {
		dd.windowSize = 1000
	}
	if dd.alertCooldown == 0 {
		dd.alertCooldown = 1 * time.Hour
	}

	// Initialize feature distributions
	for _, name := range config.FeatureNames {
		dd.baselineStats[name] = &FeatureDistribution{
			RecentSamples: make([]float64, 0, dd.windowSize),
			Percentiles:   make([]float64, 3), // 25th, 50th, 75th
			Min:           math.Inf(1),
			Max:           math.Inf(-1),
			LastUpdated:   time.Now(),
		}
		dd.currentStats[name] = &FeatureDistribution{
			RecentSamples: make([]float64, 0, dd.windowSize),
			Percentiles:   make([]float64, 3),
			Min:           math.Inf(1),
			Max:           math.Inf(-1),
			LastUpdated:   time.Now(),
		}
	}

	// Load existing baseline if available
	if config.SavePath != "" {
		if err := dd.LoadBaseline(); err != nil {
			log.Warn().Err(err).Msg("Failed to load drift detection baseline")
		}
	}

	return dd
}

// UpdateBaseline updates the baseline distribution with training data
func (dd *DriftDetector) UpdateBaseline(features [][]float32) error {
	if !dd.enabled {
		return nil
	}

	dd.mu.Lock()
	defer dd.mu.Unlock()

	for _, featureSet := range features {
		for i, value := range featureSet {
			if i >= len(dd.featureNames) {
				break
			}

			name := dd.featureNames[i]
			dist := dd.baselineStats[name]

			// Update distribution statistics
			dd.updateDistribution(dist, float64(value))
		}
	}

	// Calculate percentiles for baseline
	for _, dist := range dd.baselineStats {
		dd.calculatePercentiles(dist)
	}

	// Save baseline
	if dd.savePath != "" {
		return dd.SaveBaseline()
	}

	return nil
}

// UpdateCurrent updates the current distribution with new data
func (dd *DriftDetector) UpdateCurrent(features []float32) {
	if !dd.enabled {
		return
	}

	dd.mu.Lock()
	defer dd.mu.Unlock()

	for i, value := range features {
		if i >= len(dd.featureNames) {
			break
		}

		name := dd.featureNames[i]
		dist := dd.currentStats[name]

		// Update distribution statistics
		dd.updateDistribution(dist, float64(value))
	}
}

// DetectDrift checks for drift and returns alerts if detected
func (dd *DriftDetector) DetectDrift() []DriftAlert {
	if !dd.enabled {
		return nil
	}

	// Check cooldown period
	if time.Since(dd.lastAlertTime) < dd.alertCooldown {
		return nil
	}

	dd.mu.RLock()
	defer dd.mu.RUnlock()

	var alerts []DriftAlert

	for _, featureName := range dd.featureNames {
		baseline := dd.baselineStats[featureName]
		current := dd.currentStats[featureName]

		// Skip if not enough samples
		if current.SampleCount < 30 || baseline.SampleCount < 30 {
			continue
		}

		// Run different detection methods
		for _, method := range dd.detectionMethods {
			alert := dd.runDetectionMethod(featureName, baseline, current, method)
			if alert != nil {
				alerts = append(alerts, *alert)
			}
		}
	}

	if len(alerts) > 0 {
		dd.lastAlertTime = time.Now()
	}

	return alerts
}

// runDetectionMethod runs a specific drift detection method
func (dd *DriftDetector) runDetectionMethod(featureName string, baseline, current *FeatureDistribution, method DriftDetectionMethod) *DriftAlert {
	var driftScore float64
	var description string

	switch method {
	case KolmogorovSmirnovTest:
		driftScore = dd.kolmogorovSmirnovTest(baseline.RecentSamples, current.RecentSamples)
		description = "Distribution shape change detected using Kolmogorov-Smirnov test"

	case PopulationStabilityIndex:
		driftScore = dd.populationStabilityIndex(baseline, current)
		description = "Population distribution shift detected using PSI"

	case StatisticalMoments:
		driftScore = dd.statisticalMomentsTest(baseline, current)
		description = "Statistical properties (mean, std dev) have changed significantly"

	case ChiSquareTest:
		driftScore = dd.chiSquareTest(baseline.RecentSamples, current.RecentSamples)
		description = "Distribution comparison shows significant change using Chi-Square test"

	default:
		return nil
	}

	// Check if drift score exceeds threshold
	threshold := dd.alertThreshold
	if featureThreshold, exists := dd.driftThresholds[featureName]; exists {
		threshold = featureThreshold
	}

	if driftScore > threshold {
		severity := "medium"
		if driftScore > threshold*2 {
			severity = "high"
		}
		if driftScore > threshold*3 {
			severity = "critical"
		}

		return &DriftAlert{
			Timestamp:      time.Now(),
			FeatureName:    featureName,
			Method:         method,
			DriftScore:     driftScore,
			Threshold:      threshold,
			Severity:       severity,
			Description:    description,
			Recommendation: dd.getRecommendation(severity, featureName),
		}
	}

	return nil
}

// kolmogorovSmirnovTest implements a simplified KS test
func (dd *DriftDetector) kolmogorovSmirnovTest(baseline, current []float64) float64 {
	if len(baseline) == 0 || len(current) == 0 {
		return 0
	}

	// Sort both samples
	baselineSorted := make([]float64, len(baseline))
	currentSorted := make([]float64, len(current))
	copy(baselineSorted, baseline)
	copy(currentSorted, current)

	// Simple bubble sort (for small datasets)
	dd.sortSlice(baselineSorted)
	dd.sortSlice(currentSorted)

	// Calculate empirical CDFs and find maximum difference
	maxDiff := 0.0
	i, j := 0, 0

	for i < len(baselineSorted) && j < len(currentSorted) {
		cdfBaseline := float64(i+1) / float64(len(baselineSorted))
		cdfCurrent := float64(j+1) / float64(len(currentSorted))

		diff := math.Abs(cdfBaseline - cdfCurrent)
		if diff > maxDiff {
			maxDiff = diff
		}

		if baselineSorted[i] <= currentSorted[j] {
			i++
		} else {
			j++
		}
	}

	return maxDiff
}

// populationStabilityIndex calculates PSI between two distributions
func (dd *DriftDetector) populationStabilityIndex(baseline, current *FeatureDistribution) float64 {
	// Simplified PSI calculation using binning
	numBins := 10
	minVal := math.Min(baseline.Min, current.Min)
	maxVal := math.Max(baseline.Max, current.Max)

	if maxVal == minVal {
		return 0
	}

	binWidth := (maxVal - minVal) / float64(numBins)

	baselineBins := make([]int, numBins)
	currentBins := make([]int, numBins)

	// Count samples in each bin for baseline
	for _, sample := range baseline.RecentSamples {
		bin := int((sample - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		if bin < 0 {
			bin = 0
		}
		baselineBins[bin]++
	}

	// Count samples in each bin for current
	for _, sample := range current.RecentSamples {
		bin := int((sample - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		if bin < 0 {
			bin = 0
		}
		currentBins[bin]++
	}

	// Calculate PSI
	psi := 0.0
	baselineTotal := float64(len(baseline.RecentSamples))
	currentTotal := float64(len(current.RecentSamples))

	for i := 0; i < numBins; i++ {
		baselinePercent := float64(baselineBins[i]) / baselineTotal
		currentPercent := float64(currentBins[i]) / currentTotal

		// Avoid division by zero
		if baselinePercent > 0 && currentPercent > 0 {
			psi += (currentPercent - baselinePercent) * math.Log(currentPercent/baselinePercent)
		}
	}

	return math.Abs(psi)
}

// statisticalMomentsTest compares statistical moments (mean, std dev)
func (dd *DriftDetector) statisticalMomentsTest(baseline, current *FeatureDistribution) float64 {
	// Normalized difference in means
	meanDiff := math.Abs(baseline.Mean - current.Mean)
	meanNormalized := meanDiff / (1 + math.Abs(baseline.Mean))

	// Normalized difference in standard deviations
	stdDiff := math.Abs(baseline.StandardDev - current.StandardDev)
	stdNormalized := stdDiff / (1 + baseline.StandardDev)

	// Combined score
	return (meanNormalized + stdNormalized) / 2
}

// chiSquareTest implements a simplified chi-square test
func (dd *DriftDetector) chiSquareTest(baseline, current []float64) float64 {
	if len(baseline) == 0 || len(current) == 0 {
		return 0
	}

	// Create bins and calculate frequencies
	numBins := 10
	allSamples := append(baseline, current...)
	minVal, maxVal := dd.findMinMax(allSamples)

	if maxVal == minVal {
		return 0
	}

	binWidth := (maxVal - minVal) / float64(numBins)

	baselineBins := make([]float64, numBins)
	currentBins := make([]float64, numBins)

	// Count baseline frequencies
	for _, sample := range baseline {
		bin := int((sample - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		if bin < 0 {
			bin = 0
		}
		baselineBins[bin]++
	}

	// Count current frequencies
	for _, sample := range current {
		bin := int((sample - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		if bin < 0 {
			bin = 0
		}
		currentBins[bin]++
	}

	// Calculate chi-square statistic
	chiSquare := 0.0
	baselineTotal := float64(len(baseline))
	currentTotal := float64(len(current))

	for i := 0; i < numBins; i++ {
		expected := (baselineBins[i] / baselineTotal) * currentTotal
		if expected > 0 {
			chiSquare += math.Pow(currentBins[i]-expected, 2) / expected
		}
	}

	// Normalize by degrees of freedom
	return chiSquare / float64(numBins-1)
}

// Helper functions
func (dd *DriftDetector) updateDistribution(dist *FeatureDistribution, value float64) {
	// Update basic statistics
	if dist.SampleCount == 0 {
		dist.Mean = value
		dist.StandardDev = 0
	} else {
		// Update running mean and standard deviation
		n := float64(dist.SampleCount)
		newMean := (dist.Mean*n + value) / (n + 1)
		newVariance := ((n-1)*math.Pow(dist.StandardDev, 2) + math.Pow(value-newMean, 2)) / n
		dist.Mean = newMean
		dist.StandardDev = math.Sqrt(newVariance)
	}

	dist.SampleCount++

	// Update min/max
	if value < dist.Min {
		dist.Min = value
	}
	if value > dist.Max {
		dist.Max = value
	}

	// Add to recent samples (sliding window)
	if len(dist.RecentSamples) >= dd.windowSize {
		// Remove oldest sample
		dist.RecentSamples = dist.RecentSamples[1:]
	}
	dist.RecentSamples = append(dist.RecentSamples, value)

	dist.LastUpdated = time.Now()
}

func (dd *DriftDetector) calculatePercentiles(dist *FeatureDistribution) {
	if len(dist.RecentSamples) == 0 {
		return
	}

	sorted := make([]float64, len(dist.RecentSamples))
	copy(sorted, dist.RecentSamples)
	dd.sortSlice(sorted)

	n := len(sorted)
	dist.Percentiles[0] = sorted[n/4]   // 25th percentile
	dist.Percentiles[1] = sorted[n/2]   // 50th percentile (median)
	dist.Percentiles[2] = sorted[3*n/4] // 75th percentile
}

func (dd *DriftDetector) sortSlice(slice []float64) {
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

func (dd *DriftDetector) findMinMax(slice []float64) (float64, float64) {
	if len(slice) == 0 {
		return 0, 0
	}

	min, max := slice[0], slice[0]
	for _, val := range slice {
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	return min, max
}

func (dd *DriftDetector) getRecommendation(severity, featureName string) string {
	switch severity {
	case "critical":
		return fmt.Sprintf("CRITICAL: Feature '%s' shows severe drift. Consider immediate model retraining or feature engineering.", featureName)
	case "high":
		return fmt.Sprintf("HIGH: Feature '%s' shows significant drift. Schedule model retraining within 24-48 hours.", featureName)
	case "medium":
		return fmt.Sprintf("MEDIUM: Feature '%s' shows moderate drift. Monitor closely and consider retraining if trend continues.", featureName)
	default:
		return fmt.Sprintf("Feature '%s' shows some drift. Continue monitoring.", featureName)
	}
}

// SaveBaseline saves the baseline distribution to disk
func (dd *DriftDetector) SaveBaseline() error {
	if dd.savePath == "" {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dd.savePath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(dd.baselineStats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(dd.savePath, data, 0o600)
}

// LoadBaseline loads the baseline distribution from disk
func (dd *DriftDetector) LoadBaseline() error {
	if dd.savePath == "" {
		return nil
	}

	data, err := os.ReadFile(dd.savePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dd.mu.Lock()
	defer dd.mu.Unlock()

	return json.Unmarshal(data, &dd.baselineStats)
}

// GetDriftStatus returns the current drift status for all features
func (dd *DriftDetector) GetDriftStatus() map[string]float64 {
	if !dd.enabled {
		return nil
	}

	dd.mu.RLock()
	defer dd.mu.RUnlock()

	status := make(map[string]float64)

	for _, featureName := range dd.featureNames {
		baseline := dd.baselineStats[featureName]
		current := dd.currentStats[featureName]

		if current.SampleCount < 30 || baseline.SampleCount < 30 {
			status[featureName] = 0
			continue
		}

		// Use PSI as default drift metric
		driftScore := dd.populationStabilityIndex(baseline, current)
		status[featureName] = driftScore
	}

	return status
}

// IsEnabled returns whether drift detection is enabled
func (dd *DriftDetector) IsEnabled() bool {
	return dd.enabled
}

// Reset resets all current statistics (keeps baseline)
func (dd *DriftDetector) Reset() {
	if !dd.enabled {
		return
	}

	dd.mu.Lock()
	defer dd.mu.Unlock()

	for _, name := range dd.featureNames {
		dd.currentStats[name] = &FeatureDistribution{
			RecentSamples: make([]float64, 0, dd.windowSize),
			Percentiles:   make([]float64, 3),
			Min:           math.Inf(1),
			Max:           math.Inf(-1),
			LastUpdated:   time.Now(),
		}
	}
}
