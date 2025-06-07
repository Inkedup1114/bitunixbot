package ml

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MLManager is a comprehensive manager for all ML capabilities
type MLManager struct {
	mu                 sync.RWMutex
	enabled            bool
	featureImportance  *FeatureImportance
	driftDetector      *DriftDetector
	performanceMonitor *PerformanceMonitor
	abTestManager      *ABTestManager
	onlineLearner      *OnlineLearner
	predictor          PredictorInterface
	modelManager       *ModelManager
	featureNames       []string
	activeExperimentID string
}

// MLManagerConfig contains configuration for all ML components
type MLManagerConfig struct {
	Enabled            bool                     `yaml:"enabled"`
	FeatureNames       []string                 `yaml:"feature_names"`
	FeatureImportance  FeatureImportanceConfig  `yaml:"feature_importance"`
	DriftDetection     DriftDetectionConfig     `yaml:"drift_detection"`
	PerformanceMonitor PerformanceMonitorConfig `yaml:"performance_monitor"`
	ABTesting          ABTestConfig             `yaml:"ab_testing"`
	OnlineLearning     OnlineLearningConfig     `yaml:"online_learning"`
	ModelsDirectory    string                   `yaml:"models_directory"`
}

// MLInsights contains comprehensive insights from all ML components
type MLInsights struct {
	FeatureImportance      map[string]*FeatureStats      `json:"feature_importance"`
	DriftStatus            map[string]float64            `json:"drift_status"`
	DriftAlerts            []DriftAlert                  `json:"drift_alerts"`
	PerformanceMetrics     *PerformanceMetrics           `json:"performance_metrics"`
	PerformanceDegradation []PerformanceDegradationAlert `json:"performance_degradation"`
	OnlineLearningMetrics  OnlineLearningMetrics         `json:"online_learning_metrics"`
	ActiveExperiment       *ABExperiment                 `json:"active_experiment,omitempty"`
	ABTestResults          *ABTestResult                 `json:"ab_test_results,omitempty"`
	Recommendations        []string                      `json:"recommendations"`
	SystemHealth           string                        `json:"system_health"`
	LastUpdated            time.Time                     `json:"last_updated"`
}

// NewMLManager creates a new comprehensive ML manager
func NewMLManager(config MLManagerConfig, predictor PredictorInterface) (*MLManager, error) {
	if !config.Enabled {
		return &MLManager{enabled: false}, nil
	}

	manager := &MLManager{
		enabled:      true,
		featureNames: config.FeatureNames,
		predictor:    predictor,
	}

	var err error

	// Initialize model manager
	if config.ModelsDirectory != "" {
		manager.modelManager, err = NewModelManager(config.ModelsDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to create model manager: %w", err)
		}
	}

	// Initialize feature importance tracking
	if config.FeatureImportance.Enabled {
		config.FeatureImportance.FeatureNames = config.FeatureNames
		manager.featureImportance = NewFeatureImportance(config.FeatureImportance)
	}

	// Initialize drift detection
	if config.DriftDetection.Enabled {
		config.DriftDetection.FeatureNames = config.FeatureNames
		manager.driftDetector = NewDriftDetector(config.DriftDetection)
	}

	// Initialize performance monitoring
	if config.PerformanceMonitor.Enabled {
		manager.performanceMonitor = NewPerformanceMonitor(config.PerformanceMonitor)
	}

	// Initialize A/B testing
	if config.ABTesting.Enabled {
		manager.abTestManager = NewABTestManager(config.ABTesting)
	}

	// Initialize online learning
	if config.OnlineLearning.Enabled && len(config.FeatureNames) > 0 {
		manager.onlineLearner = NewOnlineLearner(config.OnlineLearning, len(config.FeatureNames))
	}

	log.Info().
		Bool("feature_importance", config.FeatureImportance.Enabled).
		Bool("drift_detection", config.DriftDetection.Enabled).
		Bool("performance_monitor", config.PerformanceMonitor.Enabled).
		Bool("ab_testing", config.ABTesting.Enabled).
		Bool("online_learning", config.OnlineLearning.Enabled).
		Int("feature_count", len(config.FeatureNames)).
		Msg("ML Manager initialized with enhanced capabilities")

	return manager, nil
}

// Predict makes a prediction using the appropriate model (considering A/B tests)
func (ml *MLManager) Predict(userID string, features []float32) (float32, error) {
	if !ml.enabled {
		return 0, fmt.Errorf("ML manager is disabled")
	}

	ml.mu.RLock()
	activeExperiment := ml.activeExperimentID
	ml.mu.RUnlock()

	var prediction float32
	var err error
	var modelUsed string

	start := time.Now()

	// Check if we have an active A/B test
	if activeExperiment != "" && ml.abTestManager != nil && ml.abTestManager.IsEnabled() {
		variant, variantErr := ml.abTestManager.GetVariantForUser(activeExperiment, userID)
		if variantErr == nil {
			// Use A/B test variant (would need to implement variant-specific predictors)
			predictions, predictErr := ml.predictor.Predict(features)
			if predictErr == nil && len(predictions) > 1 {
				prediction = predictions[1] // Use second element for binary classification probability
			} else if predictErr == nil && len(predictions) > 0 {
				prediction = predictions[0]
			}
			err = predictErr
			modelUsed = variant
		} else {
			// Fallback to main predictor
			predictions, predictErr := ml.predictor.Predict(features)
			if predictErr == nil && len(predictions) > 1 {
				prediction = predictions[1] // Use second element for binary classification probability
			} else if predictErr == nil && len(predictions) > 0 {
				prediction = predictions[0]
			}
			err = predictErr
			modelUsed = "main"
		}
	} else {
		// Check if online learning is available and should be used
		if ml.onlineLearner != nil && ml.onlineLearner.IsEnabled() {
			// Use online learning model
			featuresFloat64 := make([]float64, len(features))
			for i, f := range features {
				featuresFloat64[i] = float64(f)
			}
			predictionFloat64, onlineErr := ml.onlineLearner.PredictWithCurrentModel(featuresFloat64)
			if onlineErr == nil {
				prediction = float32(predictionFloat64)
				modelUsed = "online"
			} else {
				// Fallback to main predictor
				predictions, predictErr := ml.predictor.Predict(features)
				if predictErr == nil && len(predictions) > 1 {
					prediction = predictions[1] // Use second element for binary classification probability
				} else if predictErr == nil && len(predictions) > 0 {
					prediction = predictions[0]
				}
				err = predictErr
				modelUsed = "main"
			}
		} else {
			// Use main predictor
			predictions, predictErr := ml.predictor.Predict(features)
			if predictErr == nil && len(predictions) > 1 {
				prediction = predictions[1] // Use second element for binary classification probability
			} else if predictErr == nil && len(predictions) > 0 {
				prediction = predictions[0]
			}
			err = predictErr
			modelUsed = "main"
		}
	}

	latency := time.Since(start)

	// Track prediction in all relevant components
	go ml.trackPrediction(features, prediction, latency, err != nil, modelUsed, userID)

	if err != nil {
		return 0, err
	}

	return prediction, nil
}

// trackPrediction tracks the prediction across all ML components
func (ml *MLManager) trackPrediction(features []float32, prediction float32, latency time.Duration, hadError bool, modelUsed, userID string) {
	// Update drift detection
	if ml.driftDetector != nil && ml.driftDetector.IsEnabled() {
		ml.driftDetector.UpdateCurrent(features)
	}

	// Update performance monitoring (simplified - would need actual outcome for accuracy)
	if ml.performanceMonitor != nil && ml.performanceMonitor.IsEnabled() {
		// For demonstration, assuming we don't have the actual outcome yet
		ml.performanceMonitor.UpdateFromPrediction(prediction > 0.5, true, latency, hadError)
	}

	// Record A/B test event if applicable
	if ml.activeExperimentID != "" && ml.abTestManager != nil && ml.abTestManager.IsEnabled() {
		if variant, err := ml.abTestManager.GetVariantForUser(ml.activeExperimentID, userID); err == nil {
			// For demonstration, assuming conversion and value (would need real business metrics)
			converted := prediction > 0.5
			value := float64(prediction) * 100 // Example value calculation
			accuracy := 0.85                   // Example accuracy (would need real feedback)

			ml.abTestManager.RecordEvent(ml.activeExperimentID, variant, converted, value, latency, accuracy)
		}
	}
}

// UpdateWithFeedback updates the ML system with feedback (actual outcomes)
func (ml *MLManager) UpdateWithFeedback(features []float32, prediction float32, actualOutcome float64) error {
	if !ml.enabled {
		return nil
	}

	// Update feature importance
	if ml.featureImportance != nil && ml.featureImportance.IsEnabled() {
		ml.featureImportance.UpdateFeatureStats(features, float64(prediction), actualOutcome)
	}

	// Update online learning
	if ml.onlineLearner != nil && ml.onlineLearner.IsEnabled() {
		featuresFloat64 := make([]float64, len(features))
		for i, f := range features {
			featuresFloat64[i] = float64(f)
		}
		return ml.onlineLearner.AddTrainingSample(featuresFloat64, actualOutcome, 1.0)
	}

	return nil
}

// GetComprehensiveInsights returns insights from all ML components
func (ml *MLManager) GetComprehensiveInsights() *MLInsights {
	if !ml.enabled {
		return &MLInsights{
			SystemHealth: "disabled",
			LastUpdated:  time.Now(),
		}
	}

	insights := &MLInsights{
		LastUpdated:     time.Now(),
		Recommendations: make([]string, 0),
	}

	// Feature importance insights
	if ml.featureImportance != nil && ml.featureImportance.IsEnabled() {
		insights.FeatureImportance = ml.featureImportance.GetFeatureImportance()
		topFeatures := ml.featureImportance.GetTopFeatures(3)
		if len(topFeatures) > 0 {
			insights.Recommendations = append(insights.Recommendations,
				fmt.Sprintf("Top important features: %v. Consider focusing feature engineering on these.", topFeatures))
		}
	}

	// Drift detection insights
	if ml.driftDetector != nil && ml.driftDetector.IsEnabled() {
		insights.DriftStatus = ml.driftDetector.GetDriftStatus()
		insights.DriftAlerts = ml.driftDetector.DetectDrift()

		if len(insights.DriftAlerts) > 0 {
			severeCounts := 0
			for _, alert := range insights.DriftAlerts {
				if alert.Severity == "critical" || alert.Severity == "high" {
					severeCounts++
				}
			}
			if severeCounts > 0 {
				insights.Recommendations = append(insights.Recommendations,
					fmt.Sprintf("URGENT: %d severe drift alerts detected. Consider model retraining.", severeCounts))
			}
		}
	}

	// Performance monitoring insights
	if ml.performanceMonitor != nil && ml.performanceMonitor.IsEnabled() {
		insights.PerformanceMetrics = ml.performanceMonitor.GetCurrentMetrics()
		insights.PerformanceDegradation = ml.performanceMonitor.CheckDegradation()

		if len(insights.PerformanceDegradation) > 0 {
			criticalCount := 0
			for _, alert := range insights.PerformanceDegradation {
				if alert.Severity == "critical" {
					criticalCount++
				}
			}
			if criticalCount > 0 {
				insights.Recommendations = append(insights.Recommendations,
					fmt.Sprintf("CRITICAL: %d performance degradation alerts. Immediate action required.", criticalCount))
			}
		}
	}

	// A/B testing insights
	if ml.abTestManager != nil && ml.abTestManager.IsEnabled() && ml.activeExperimentID != "" {
		if experiment, err := ml.abTestManager.GetExperiment(ml.activeExperimentID); err == nil {
			insights.ActiveExperiment = experiment

			// Get A/B test results if experiment has enough data
			if result, err := ml.abTestManager.AnalyzeExperiment(ml.activeExperimentID); err == nil {
				insights.ABTestResults = result
				if result.IsSignificant {
					insights.Recommendations = append(insights.Recommendations,
						fmt.Sprintf("A/B test shows significant results: %s", result.Recommendation))
				}
			}
		}
	}

	// Online learning insights
	if ml.onlineLearner != nil && ml.onlineLearner.IsEnabled() {
		insights.OnlineLearningMetrics = ml.onlineLearner.GetMetrics()

		if insights.OnlineLearningMetrics.TotalSamples > 1000 {
			insights.Recommendations = append(insights.Recommendations,
				"Online learning model has processed significant data. Consider evaluating for production use.")
		}
	}

	// Determine system health
	insights.SystemHealth = ml.assessSystemHealth(insights)

	return insights
}

// assessSystemHealth determines overall ML system health
func (ml *MLManager) assessSystemHealth(insights *MLInsights) string {
	criticalIssues := 0
	warnings := 0

	// Check drift alerts
	for _, alert := range insights.DriftAlerts {
		if alert.Severity == "critical" {
			criticalIssues++
		} else if alert.Severity == "high" {
			warnings++
		}
	}

	// Check performance degradation
	for _, alert := range insights.PerformanceDegradation {
		if alert.Severity == "critical" {
			criticalIssues++
		} else if alert.Severity == "high" {
			warnings++
		}
	}

	// Determine overall health
	if criticalIssues > 0 {
		return "critical"
	} else if warnings > 2 {
		return "degraded"
	} else if warnings > 0 {
		return "warning"
	}
	return "healthy"
}

// StartABTest starts a new A/B test experiment
func (ml *MLManager) StartABTest(experimentConfig ABExperiment) error {
	if !ml.enabled || ml.abTestManager == nil || !ml.abTestManager.IsEnabled() {
		return fmt.Errorf("A/B testing is not enabled")
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	// Create and start the experiment
	_, err := ml.abTestManager.CreateExperiment(
		experimentConfig.ID,
		experimentConfig.Name,
		experimentConfig.Description,
		experimentConfig.Models,
		experimentConfig.TrafficSplit,
		experimentConfig.Criteria,
	)
	if err != nil {
		return err
	}

	err = ml.abTestManager.StartExperiment(experimentConfig.ID)
	if err != nil {
		return err
	}

	ml.activeExperimentID = experimentConfig.ID

	log.Info().
		Str("experiment_id", experimentConfig.ID).
		Str("name", experimentConfig.Name).
		Msg("A/B test experiment started")

	return nil
}

// CompleteABTest completes the active A/B test
func (ml *MLManager) CompleteABTest() (*ABTestResult, error) {
	if !ml.enabled || ml.abTestManager == nil || !ml.abTestManager.IsEnabled() {
		return nil, fmt.Errorf("A/B testing is not enabled")
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	if ml.activeExperimentID == "" {
		return nil, fmt.Errorf("no active A/B test experiment")
	}

	// Analyze results
	result, err := ml.abTestManager.AnalyzeExperiment(ml.activeExperimentID)
	if err != nil {
		return nil, err
	}

	// Complete the experiment
	err = ml.abTestManager.CompleteExperiment(ml.activeExperimentID)
	if err != nil {
		return nil, err
	}

	ml.activeExperimentID = ""

	log.Info().
		Str("experiment_id", result.ExperimentID).
		Str("winner", result.Winner).
		Bool("significant", result.IsSignificant).
		Float64("effect_size", result.EffectSize).
		Msg("A/B test experiment completed")

	return result, nil
}

// SetBaseline sets baseline metrics for monitoring (typically from training data)
func (ml *MLManager) SetBaseline(trainingFeatures [][]float32, trainingTargets []float64, performanceMetrics PerformanceMetrics) error {
	if !ml.enabled {
		return nil
	}

	// Set drift detection baseline
	if ml.driftDetector != nil && ml.driftDetector.IsEnabled() {
		if err := ml.driftDetector.UpdateBaseline(trainingFeatures); err != nil {
			log.Warn().Err(err).Msg("Failed to set drift detection baseline")
		}
	}

	// Set performance monitoring baseline
	if ml.performanceMonitor != nil && ml.performanceMonitor.IsEnabled() {
		ml.performanceMonitor.SetBaseline(performanceMetrics)
	}

	// Calculate feature importance baseline
	if ml.featureImportance != nil && ml.featureImportance.IsEnabled() && ml.predictor != nil {
		testFeatures := make([][]float32, len(trainingFeatures))
		copy(testFeatures, trainingFeatures)

		if err := ml.featureImportance.CalculatePermutationImportance(ml.predictor, testFeatures, trainingTargets); err != nil {
			log.Warn().Err(err).Msg("Failed to calculate baseline feature importance")
		}
	}

	log.Info().
		Int("training_samples", len(trainingFeatures)).
		Float64("baseline_accuracy", performanceMetrics.Accuracy).
		Msg("ML baseline metrics set successfully")

	return nil
}

// IsEnabled returns whether the ML manager is enabled
func (ml *MLManager) IsEnabled() bool {
	return ml.enabled
}

// GetModelManager returns the model manager
func (ml *MLManager) GetModelManager() *ModelManager {
	return ml.modelManager
}

// Shutdown gracefully shuts down all ML components
func (ml *MLManager) Shutdown() error {
	if !ml.enabled {
		return nil
	}

	log.Info().Msg("Shutting down ML Manager")

	// Save all component states
	var errors []error

	if ml.featureImportance != nil && ml.featureImportance.IsEnabled() {
		if err := ml.featureImportance.Save(); err != nil {
			errors = append(errors, fmt.Errorf("failed to save feature importance: %w", err))
		}
	}

	if ml.driftDetector != nil && ml.driftDetector.IsEnabled() {
		if err := ml.driftDetector.SaveBaseline(); err != nil {
			errors = append(errors, fmt.Errorf("failed to save drift baseline: %w", err))
		}
	}

	if ml.performanceMonitor != nil && ml.performanceMonitor.IsEnabled() {
		if err := ml.performanceMonitor.Save(); err != nil {
			errors = append(errors, fmt.Errorf("failed to save performance monitor: %w", err))
		}
	}

	if ml.onlineLearner != nil && ml.onlineLearner.IsEnabled() {
		if err := ml.onlineLearner.SaveModel(); err != nil {
			errors = append(errors, fmt.Errorf("failed to save online learning model: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("multiple shutdown errors: %v", errors)
	}

	log.Info().Msg("ML Manager shutdown complete")
	return nil
}
