package ml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
)

// ModelVersion represents a versioned ML model
type ModelVersion struct {
	Version   string       `json:"version"`
	Path      string       `json:"path"`
	CreatedAt time.Time    `json:"created_at"`
	Metrics   ModelMetrics `json:"metrics"`
	IsActive  bool         `json:"is_active"`
}

// ModelMetrics contains performance metrics for a model
type ModelMetrics struct {
	AUCScore        float64 `json:"auc_score"`
	F1Score         float64 `json:"f1_score"`
	Precision       float64 `json:"precision"`
	Recall          float64 `json:"recall"`
	ApprovalRate    float64 `json:"approval_rate"`
	TrainingSamples int     `json:"training_samples"`
}

// ModelManager handles model versioning and rollback
type ModelManager struct {
	modelsDir    string
	versionsFile string
	versions     []ModelVersion
	currentModel *ModelVersion
}

// NewModelManager creates a new model manager
func NewModelManager(modelsDir string) (*ModelManager, error) {
	versionsFile := filepath.Join(modelsDir, "model_versions.json")

	mm := &ModelManager{
		modelsDir:    modelsDir,
		versionsFile: versionsFile,
		versions:     make([]ModelVersion, 0),
	}

	// Load existing versions if available
	if err := mm.loadVersions(); err != nil {
		log.Warn().Err(err).Msg("Failed to load model versions, starting fresh")
	}

	return mm, nil
}

// AddVersion adds a new model version
func (mm *ModelManager) AddVersion(modelPath string, metrics ModelMetrics) error {
	version := ModelVersion{
		Version:   time.Now().Format("20060102-150405"),
		Path:      modelPath,
		CreatedAt: time.Now(),
		Metrics:   metrics,
		IsActive:  false,
	}

	// Add to versions list
	mm.versions = append(mm.versions, version)

	// Sort versions by creation time
	sort.Slice(mm.versions, func(i, j int) bool {
		return mm.versions[i].CreatedAt.After(mm.versions[j].CreatedAt)
	})

	// Save versions
	return mm.saveVersions()
}

// ActivateVersion activates a specific model version
func (mm *ModelManager) ActivateVersion(version string) error {
	found := false
	for i := range mm.versions {
		if mm.versions[i].Version == version {
			mm.versions[i].IsActive = true
			mm.currentModel = &mm.versions[i]
			found = true
		} else {
			mm.versions[i].IsActive = false
		}
	}

	if !found {
		return fmt.Errorf("version %s not found", version)
	}

	return mm.saveVersions()
}

// Rollback rolls back to the previous version
func (mm *ModelManager) Rollback() error {
	if len(mm.versions) < 2 {
		return fmt.Errorf("no previous version available for rollback")
	}

	// Find current version
	currentIdx := -1
	for i, v := range mm.versions {
		if v.IsActive {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return fmt.Errorf("no active version found")
	}

	// Activate previous version
	if currentIdx+1 < len(mm.versions) {
		return mm.ActivateVersion(mm.versions[currentIdx+1].Version)
	}

	return fmt.Errorf("no previous version available")
}

// GetCurrentVersion returns the currently active version
func (mm *ModelManager) GetCurrentVersion() *ModelVersion {
	return mm.currentModel
}

// ListVersions returns all model versions
func (mm *ModelManager) ListVersions() []ModelVersion {
	return mm.versions
}

// loadVersions loads model versions from file
func (mm *ModelManager) loadVersions() error {
	data, err := os.ReadFile(mm.versionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &mm.versions); err != nil {
		return err
	}

	// Find current model
	for i := range mm.versions {
		if mm.versions[i].IsActive {
			mm.currentModel = &mm.versions[i]
			break
		}
	}

	return nil
}

// saveVersions saves model versions to file
func (mm *ModelManager) saveVersions() error {
	data, err := json.MarshalIndent(mm.versions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mm.versionsFile, data, 0o600)
}
