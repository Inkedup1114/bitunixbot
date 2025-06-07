package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"bitunix-bot/internal/backtest"
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/ml"
	"bitunix-bot/internal/storage"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// MockMetrics implements ml.MetricsInterface for backtest
type MockMetrics struct{}

func (m *MockMetrics) MLPredictionsInc()                   {}
func (m *MockMetrics) MLFailuresInc()                      {}
func (m *MockMetrics) MLLatencyObserve(v float64)          {}
func (m *MockMetrics) MLModelAgeSet(v float64)             {}
func (m *MockMetrics) MLAccuracyObserve(v float64)         {}
func (m *MockMetrics) MLPredictionScoresObserve(v float64) {}
func (m *MockMetrics) MLTimeoutsInc()                      {}
func (m *MockMetrics) MLFallbackUseInc()                   {}

func main() {
	// Parse command line arguments
	var (
		dataPath   = flag.String("data", "data", "Path to data directory")
		modelPath  = flag.String("model", "models/model.onnx", "Path to ONNX model")
		outputPath = flag.String("output", "", "Output directory for results")
		logLevel   = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		symbols    = flag.String("symbols", "", "Comma-separated symbols to test (overrides config)")
		startDate  = flag.String("start", "", "Start date (YYYY-MM-DD)")
		endDate    = flag.String("end", "", "End date (YYYY-MM-DD)")
		dataFormat = flag.String("format", "auto", "Data format: auto, csv, json, boltdb")
	)
	flag.Parse()

	// Setup logging
	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Print configuration
	fmt.Println("=== Backtest Configuration ===")
	fmt.Printf("Config File: %s/config.yaml\n", os.Getenv("PWD"))
	fmt.Printf("Data Path: %s\n", *dataPath)
	fmt.Printf("Model Path: %s\n", *modelPath)
	fmt.Printf("Output Directory: %s\n", *outputPath)
	fmt.Printf("Log Level: %s\n", *logLevel)
	fmt.Println("==============================")

	fmt.Println("Starting backtest...")

	// Load configuration
	config, err := cfg.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Override config with command line arguments
	if *modelPath != "" {
		config.ModelPath = *modelPath
	}
	if *symbols != "" {
		config.Symbols = parseSymbols(*symbols)
	}

	// Parse dates
	var startTime, endTime time.Time
	if *startDate != "" {
		startTime, err = time.Parse("2006-01-02", *startDate)
		if err != nil {
			log.Fatal().Err(err).Msg("Invalid start date format")
		}
	} else {
		startTime = time.Now().AddDate(0, -1, 0) // Default: 1 month ago
	}

	if *endDate != "" {
		endTime, err = time.Parse("2006-01-02", *endDate)
		if err != nil {
			log.Fatal().Err(err).Msg("Invalid end date format")
		}
	} else {
		endTime = time.Now() // Default: now
	}

	// Create data loader
	loader := backtest.NewDataLoader()

	// Load data based on format
	if *dataPath == "" {
		// Use default data path from config
		*dataPath = config.DataPath
	}

	switch *dataFormat {
	case "csv":
		err = loader.LoadFromCSV(*dataPath)
	case "json":
		err = loader.LoadFromJSON(*dataPath)
	case "boltdb":
		store, err := storage.New(*dataPath)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to open BoltDB")
		}
		defer store.Close()
		err = loader.LoadFromBoltDB(store, config.Symbols, startTime, endTime)
	case "auto":
		// Auto-detect format based on file extension or path
		err = autoLoadData(loader, *dataPath, config.Symbols, startTime, endTime)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to auto-load data")
		}
	default:
		log.Fatal().Str("format", *dataFormat).Msg("Unknown data format")
	}

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load data")
	}

	// Create ML predictor
	// For backtest, we don't need real metrics, so use a simple mock
	mockMetrics := &MockMetrics{}
	predictor, err := ml.NewWithMetrics(config.ModelPath, mockMetrics, 5*time.Second)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load ML model, using fallback")
	}

	// Create and run backtest engine
	engine := backtest.NewEngine(&config, predictor, loader)

	log.Info().Msg("Starting backtest...")
	if err := engine.Run(); err != nil {
		log.Fatal().Err(err).Msg("Backtest failed")
	}

	// Get results
	results := engine.GetResults()

	// Generate reports
	reporter := backtest.NewReporter(results, *outputPath)
	if err := reporter.GenerateReport(); err != nil {
		log.Error().Err(err).Msg("Failed to generate reports")
	}

	// Print summary to console
	reporter.PrintSummary()

	log.Info().
		Str("output", *outputPath).
		Msg("Backtest completed successfully")
}

// autoLoadData attempts to automatically detect and load data
func autoLoadData(loader *backtest.DataLoader, path string, symbols []string, start, end time.Time) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		// Assume it's a BoltDB directory
		store, err := storage.New(path)
		if err != nil {
			return fmt.Errorf("failed to open BoltDB: %w", err)
		}
		defer store.Close()
		return loader.LoadFromBoltDB(store, symbols, start, end)
	}

	// Check file extension
	switch {
	case endsWith(path, ".csv"):
		return loader.LoadFromCSV(path)
	case endsWith(path, ".json"):
		return loader.LoadFromJSON(path)
	default:
		return fmt.Errorf("cannot determine file format for: %s", path)
	}
}

// parseSymbols parses comma-separated symbols
func parseSymbols(symbols string) []string {
	var result []string
	for _, s := range strings.Split(symbols, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// endsWith checks if string ends with suffix
func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
