package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"bitunix-bot/internal/storage"

	bolt "go.etcd.io/bbolt"
)

// FeatureRecord represents a single feature record for ML training
type FeatureRecord struct {
	Timestamp  int64   `json:"timestamp"`
	Symbol     string  `json:"symbol"`
	TickRatio  float64 `json:"tick_ratio"`
	DepthRatio float64 `json:"depth_ratio"`
	PriceDist  float64 `json:"price_dist"`
	Price      float64 `json:"price"`
	VWAP       float64 `json:"vwap"`
	StdDev     float64 `json:"std_dev"`
	BidVol     float64 `json:"bid_vol"`
	AskVol     float64 `json:"ask_vol"`
}

func main() {
	var (
		dbPath     = flag.String("db", "data/features.db", "Path to BoltDB database")
		outputPath = flag.String("output", "scripts/training_data.json", "Output JSON file path")
		symbol     = flag.String("symbol", "", "Symbol to export (empty for all)")
		days       = flag.Int("days", 30, "Number of days to export (0 for all)")
		bucket     = flag.String("bucket", "features", "BoltDB bucket name")
	)
	flag.Parse()

	log.Printf("Exporting data from %s to %s", *dbPath, *outputPath)
	if *symbol != "" {
		log.Printf("Filtering by symbol: %s", *symbol)
	}
	if *days > 0 {
		log.Printf("Exporting last %d days", *days)
	}

	// Open BoltDB
	db, err := bolt.Open(*dbPath, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var records []FeatureRecord
	cutoffTime := int64(0)
	if *days > 0 {
		cutoffTime = time.Now().AddDate(0, 0, -*days).Unix()
	}

	// Read data from BoltDB
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(*bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", *bucket)
		}

		c := b.Cursor()
		count := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Parse the stored feature data
			var feature storage.FeatureRecord
			if err := json.Unmarshal(v, &feature); err != nil {
				log.Printf("Failed to unmarshal feature: %v", err)
				continue
			}

			// Apply time filter
			if cutoffTime > 0 && feature.Timestamp.Unix() < cutoffTime {
				continue
			}

			// Apply symbol filter
			if *symbol != "" && feature.Symbol != *symbol {
				continue
			}

			// Convert to ML training format
			record := FeatureRecord{
				Timestamp:  feature.Timestamp.Unix(),
				Symbol:     feature.Symbol,
				TickRatio:  feature.TickRatio,
				DepthRatio: feature.DepthRatio,
				PriceDist:  feature.PriceDist,
				Price:      feature.Price,
				VWAP:       feature.VWAP,
				StdDev:     feature.StdDev,
				BidVol:     feature.BidVol,
				AskVol:     feature.AskVol,
			}

			// Skip records with invalid data
			if isValidRecord(record) {
				records = append(records, record)
				count++
			}
		}

		log.Printf("Exported %d valid records", count)
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to read from database: %v", err)
	}

	if len(records) == 0 {
		log.Println("Warning: No records found matching criteria")
	}

	// Write to JSON file (newline-delimited format for Python compatibility)
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	encoder := json.NewEncoder(outputFile)

	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			log.Fatalf("Failed to write JSON record: %v", err)
		}
	}

	log.Printf("Successfully exported %d records to %s", len(records), *outputPath)

	// Print some statistics
	if len(records) > 0 {
		start := time.Unix(records[0].Timestamp, 0)
		end := time.Unix(records[len(records)-1].Timestamp, 0)
		log.Printf("Time range: %v to %v", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))

		// Count by symbol
		symbolCounts := make(map[string]int)
		for _, record := range records {
			symbolCounts[record.Symbol]++
		}

		log.Println("Records by symbol:")
		for sym, count := range symbolCounts {
			log.Printf("  %s: %d", sym, count)
		}
	}
}

// isValidRecord checks if a feature record has valid data for ML training
func isValidRecord(record FeatureRecord) bool {
	// Check for NaN or infinite values
	if !isFinite(record.TickRatio) || !isFinite(record.DepthRatio) || !isFinite(record.PriceDist) {
		return false
	}
	if !isFinite(record.Price) || !isFinite(record.VWAP) || !isFinite(record.StdDev) {
		return false
	}

	// Check for reasonable ranges (adjust these based on your data)
	if record.TickRatio < -10 || record.TickRatio > 10 {
		return false
	}
	if record.DepthRatio < -10 || record.DepthRatio > 10 {
		return false
	}
	if record.PriceDist < -1 || record.PriceDist > 1 {
		return false
	}
	if record.Price <= 0 || record.StdDev <= 0 {
		return false
	}

	return true
}

// isFinite checks if a float64 is finite (not NaN or Inf)
func isFinite(f float64) bool {
	return !isNaN(f) && !isInf(f)
}

// isNaN checks if a float64 is NaN
func isNaN(f float64) bool {
	return f != f
}

// isInf checks if a float64 is infinite
func isInf(f float64) bool {
	return f > 1.7976931348623157e+308 || f < -1.7976931348623157e+308
}
