package ml

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// FeatureImportance tracks the importance of different features in predictions
type FeatureImportance struct {
	mu             sync.RWMutex
	featureNames   []string
	importanceData map[string]*FeatureStats
	baselineScore  float64
	enabled        bool
	savePath       string
}

// FeatureStats contains statistics for a single feature
type FeatureStats struct {
	Name                  string    `json:"name"`
	ImportanceScore       float64   `json:"importance_score"`
	UsageCount            int64     `json:"usage_count"`
	AverageValue          float64   `json:"average_value"`
	StandardDeviation     float64   `json:"standard_deviation"`
	MinValue              float64   `json:"min_value"`
	MaxValue              float64   `json:"max_value"`
	LastUpdated           time.Time `json:"last_updated"`
	PermutationScore      float64   `json:"permutation_score"`
	CorrelationWithTarget float64   `json:"correlation_with_target"`
}

// FeatureImportanceConfig configures feature importance tracking
type FeatureImportanceConfig struct {
	Enabled            bool          `yaml:"enabled"`
	FeatureNames       []string      `yaml:"feature_names"`
	SavePath           string        `yaml:"save_path"`
	UpdateInterval     time.Duration `yaml:"update_interval"`
	PermutationSamples int           `yaml:"permutation_samples"`
}

// NewFeatureImportance creates a new feature importance tracker
func NewFeatureImportance(config FeatureImportanceConfig) *FeatureImportance {
	fi := &FeatureImportance{
		featureNames:   config.FeatureNames,
		importanceData: make(map[string]*FeatureStats),
		enabled:        config.Enabled,
		savePath:       config.SavePath,
	}

	// Initialize feature stats
	for _, name := range config.FeatureNames {
		fi.importanceData[name] = &FeatureStats{
			Name:        name,
			MinValue:    math.Inf(1),
			MaxValue:    math.Inf(-1),
			LastUpdated: time.Now(),
		}
	}

	// Load existing data if available
	if config.SavePath != "" {
		if err := fi.Load(); err != nil {
			log.Warn().Err(err).Msg("Failed to load feature importance data")
		}
	}

	return fi
}

// UpdateFeatureStats updates statistics for features after a prediction
func (fi *FeatureImportance) UpdateFeatureStats(features []float32, predictionScore float64, actualOutcome float64) {
	if !fi.enabled {
		return
	}

	fi.mu.Lock()
	defer fi.mu.Unlock()

	for i, value := range features {
		if i >= len(fi.featureNames) {
			break
		}

		name := fi.featureNames[i]
		stats := fi.importanceData[name]

		// Update basic statistics
		stats.UsageCount++

		// Update running average (using exponential moving average)
		alpha := 0.1 // Decay factor
		if stats.UsageCount == 1 {
			stats.AverageValue = float64(value)
		} else {
			stats.AverageValue = alpha*float64(value) + (1-alpha)*stats.AverageValue
		}

		// Update min/max
		if float64(value) < stats.MinValue {
			stats.MinValue = float64(value)
		}
		if float64(value) > stats.MaxValue {
			stats.MaxValue = float64(value)
		}

		// Update standard deviation (simplified running calculation)
		if stats.UsageCount > 1 {
			variance := alpha*math.Pow(float64(value)-stats.AverageValue, 2) + (1-alpha)*math.Pow(stats.StandardDeviation, 2)
			stats.StandardDeviation = math.Sqrt(variance)
		}

		// Update correlation with target (simplified)
		if !math.IsNaN(actualOutcome) {
			stats.CorrelationWithTarget = alpha*float64(value)*actualOutcome + (1-alpha)*stats.CorrelationWithTarget
		}

		stats.LastUpdated = time.Now()
	}
}

// CalculatePermutationImportance calculates feature importance using permutation method
func (fi *FeatureImportance) CalculatePermutationImportance(predictor PredictorInterface, testFeatures [][]float32, testTargets []float64) error {
	if !fi.enabled || len(testFeatures) == 0 {
		return nil
	}

	// Calculate baseline score
	totalCorrect := 0
	for i, features := range testFeatures {
		prediction := predictor.Approve(features, 0.5)
		actual := testTargets[i] > 0.5
		if prediction == actual {
			totalCorrect++
		}
	}
	fi.baselineScore = float64(totalCorrect) / float64(len(testFeatures))

	fi.mu.Lock()
	defer fi.mu.Unlock()

	// Calculate importance for each feature
	for featureIdx, featureName := range fi.featureNames {
		if featureIdx >= len(testFeatures[0]) {
			continue
		}

		// Permute this feature and measure performance drop
		permutedFeatures := make([][]float32, len(testFeatures))
		for i := range testFeatures {
			permutedFeatures[i] = make([]float32, len(testFeatures[i]))
			copy(permutedFeatures[i], testFeatures[i])
		}

		// Randomly shuffle the values of this feature across samples
		for i := range permutedFeatures {
			// Simple permutation: swap with a random other sample
			j := i
			if len(permutedFeatures) > 1 {
				j = (i + 1) % len(permutedFeatures)
			}
			permutedFeatures[i][featureIdx] = testFeatures[j][featureIdx]
		}

		// Calculate score with permuted feature
		permutedCorrect := 0
		for i, features := range permutedFeatures {
			prediction := predictor.Approve(features, 0.5)
			actual := testTargets[i] > 0.5
			if prediction == actual {
				permutedCorrect++
			}
		}
		permutedScore := float64(permutedCorrect) / float64(len(testFeatures))

		// Importance is the drop in performance
		importance := fi.baselineScore - permutedScore
		fi.importanceData[featureName].PermutationScore = importance
		fi.importanceData[featureName].ImportanceScore = math.Max(0, importance) // Non-negative importance
	}

	return nil
}

// GetFeatureImportance returns the current feature importance rankings
func (fi *FeatureImportance) GetFeatureImportance() map[string]*FeatureStats {
	if !fi.enabled {
		return nil
	}

	fi.mu.RLock()
	defer fi.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*FeatureStats)
	for name, stats := range fi.importanceData {
		statsCopy := *stats
		result[name] = &statsCopy
	}

	return result
}

// GetTopFeatures returns the top N most important features
func (fi *FeatureImportance) GetTopFeatures(n int) []string {
	if !fi.enabled {
		return nil
	}

	fi.mu.RLock()
	defer fi.mu.RUnlock()

	// Create a slice of feature names sorted by importance
	type featureScore struct {
		name  string
		score float64
	}

	features := make([]featureScore, 0, len(fi.importanceData))
	for name, stats := range fi.importanceData {
		features = append(features, featureScore{
			name:  name,
			score: stats.ImportanceScore,
		})
	}

	// Sort by importance score (descending)
	for i := 0; i < len(features)-1; i++ {
		for j := i + 1; j < len(features); j++ {
			if features[i].score < features[j].score {
				features[i], features[j] = features[j], features[i]
			}
		}
	}

	// Return top N
	if n > len(features) {
		n = len(features)
	}

	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = features[i].name
	}

	return result
}

// Save saves the feature importance data to disk
func (fi *FeatureImportance) Save() error {
	if !fi.enabled || fi.savePath == "" {
		return nil
	}

	fi.mu.RLock()
	defer fi.mu.RUnlock()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(fi.savePath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(fi.importanceData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(fi.savePath, data, 0o600)
}

// Load loads feature importance data from disk
func (fi *FeatureImportance) Load() error {
	if !fi.enabled || fi.savePath == "" {
		return nil
	}

	data, err := os.ReadFile(fi.savePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, start fresh
		}
		return err
	}

	fi.mu.Lock()
	defer fi.mu.Unlock()

	return json.Unmarshal(data, &fi.importanceData)
}

// GetFeatureCorrelations returns correlation matrix between features
func (fi *FeatureImportance) GetFeatureCorrelations() map[string]map[string]float64 {
	if !fi.enabled {
		return nil
	}

	fi.mu.RLock()
	defer fi.mu.RUnlock()

	correlations := make(map[string]map[string]float64)

	for _, name1 := range fi.featureNames {
		correlations[name1] = make(map[string]float64)
		for _, name2 := range fi.featureNames {
			if name1 == name2 {
				correlations[name1][name2] = 1.0
			} else {
				// Simplified correlation calculation
				// In a real implementation, you'd track pairwise correlations
				stats1 := fi.importanceData[name1]
				stats2 := fi.importanceData[name2]

				// Simple correlation approximation based on importance scores
				correlation := math.Abs(stats1.ImportanceScore - stats2.ImportanceScore)
				if correlation > 1.0 {
					correlation = 1.0 / correlation
				}
				correlations[name1][name2] = correlation
			}
		}
	}

	return correlations
}

// IsEnabled returns whether feature importance tracking is enabled
func (fi *FeatureImportance) IsEnabled() bool {
	return fi.enabled
}

// Reset resets all feature importance data
func (fi *FeatureImportance) Reset() {
	if !fi.enabled {
		return
	}

	fi.mu.Lock()
	defer fi.mu.Unlock()

	for name := range fi.importanceData {
		fi.importanceData[name] = &FeatureStats{
			Name:        name,
			MinValue:    math.Inf(1),
			MaxValue:    math.Inf(-1),
			LastUpdated: time.Now(),
		}
	}

	fi.baselineScore = 0
}
