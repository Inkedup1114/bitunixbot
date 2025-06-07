package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

const (
	featuresBucket = "features" // Bucket name for storing feature records for ML training
	pricesBucket   = "prices"   // Bucket name for storing price records for labeling
)

// FeatureRecord represents a single feature observation for machine learning.
// It contains all the calculated technical indicators and market features
// that are used as inputs to the ML prediction model.
type FeatureRecord struct {
	Symbol            string    `json:"symbol"`             // Trading symbol (e.g., "BTCUSDT")
	Timestamp         time.Time `json:"timestamp"`          // Time when features were calculated
	TickRatio         float64   `json:"tick_ratio"`         // Tick imbalance ratio
	DepthRatio        float64   `json:"depth_ratio"`        // Order book depth imbalance ratio
	PriceDist         float64   `json:"price_dist"`         // Price distance from VWAP
	Price             float64   `json:"price"`              // Current market price
	VWAP              float64   `json:"vwap"`               // Volume Weighted Average Price
	StdDev            float64   `json:"std_dev"`            // Standard deviation of VWAP
	BidVol            float64   `json:"bid_vol"`            // Total bid volume
	AskVol            float64   `json:"ask_vol"`            // Total ask volume
	RollingVolatility float64   `json:"rolling_volatility"` // Rolling price volatility
	BidAskSpread      float64   `json:"bid_ask_spread"`     // Bid-ask spread
	VolumeSpike       float64   `json:"volume_spike"`       // Volume spike indicator
}

// PriceRecord represents price data for labeling ML training data.
// It contains the price information needed to generate labels for
// supervised learning algorithms.
type PriceRecord struct {
	Symbol    string    `json:"symbol"`    // Trading symbol
	Timestamp time.Time `json:"timestamp"` // Time of price observation
	Price     float64   `json:"price"`     // Market price at timestamp
	VWAP      float64   `json:"vwap"`      // VWAP at timestamp
	StdDev    float64   `json:"std_dev"`   // Standard deviation at timestamp
}

// storeRecord is a generic function to store records with symbol and timestamp.
// It creates the bucket if it doesn't exist and stores the record with a key
// format of "symbol_timestamp" for efficient time-range queries.
func (s *Store) storeRecord(bucketName string, symbol string, timestamp time.Time, record interface{}, recordType string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("create %s bucket: %w", recordType, err)
		}

		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal %s record: %w", recordType, err)
		}

		key := fmt.Sprintf("%s_%d", symbol, timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

// StoreFeatures stores a feature record for ML training.
// The record is stored in the features bucket with a timestamp-based key
// for efficient retrieval during model training and backtesting.
func (s *Store) StoreFeatures(record FeatureRecord) error {
	return s.storeRecord(featuresBucket, record.Symbol, record.Timestamp, record, "feature")
}

// StorePrice stores price data for labeling ML training data.
// The record is stored in the prices bucket and is used to generate
// labels for supervised learning algorithms.
func (s *Store) StorePrice(record PriceRecord) error {
	return s.storeRecord(pricesBucket, record.Symbol, record.Timestamp, record, "price")
}

// ExportFeaturesToCSV exports all features to CSV format for training.
// This method is a placeholder for future implementation that will export
// feature data in a format suitable for Python ML training scripts.
func (s *Store) ExportFeaturesToCSV(filename string) error {
	// This will be implemented to export data for Python training
	// For now, we'll create the structure that the Python script expects
	return nil
}

// GetFeaturesInRange returns feature records for a symbol within a time range.
// Returns a slice of FeatureRecord structs ordered by timestamp, or an error
// if the query fails. The time range is exclusive of the end time.
func (s *Store) GetFeaturesInRange(symbol string, start, end time.Time) ([]FeatureRecord, error) {
	var features []FeatureRecord

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(featuresBucket))
		if b == nil {
			return nil
		}

		c := b.Cursor()
		prefix := []byte(symbol + "_")

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var feature FeatureRecord
			if unmarshalErr := json.Unmarshal(v, &feature); unmarshalErr != nil {
				continue
			}

			if feature.Timestamp.After(start) && feature.Timestamp.Before(end) {
				features = append(features, feature)
			}
		}
		return nil
	})

	return features, err
}
