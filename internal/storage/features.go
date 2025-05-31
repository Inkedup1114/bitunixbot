package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

const (
	featuresBucket = "features"
	pricesBucket   = "prices"
)

// FeatureRecord represents a single feature observation
type FeatureRecord struct {
	Symbol     string    `json:"symbol"`
	Timestamp  time.Time `json:"timestamp"`
	TickRatio  float64   `json:"tick_ratio"`
	DepthRatio float64   `json:"depth_ratio"`
	PriceDist  float64   `json:"price_dist"`
	Price      float64   `json:"price"`
	VWAP       float64   `json:"vwap"`
	StdDev     float64   `json:"std_dev"`
	BidVol     float64   `json:"bid_vol"`
	AskVol     float64   `json:"ask_vol"`
}

// PriceRecord represents price data for labeling
type PriceRecord struct {
	Symbol    string    `json:"symbol"`
	Timestamp time.Time `json:"timestamp"`
	Price     float64   `json:"price"`
	VWAP      float64   `json:"vwap"`
	StdDev    float64   `json:"std_dev"`
}

// StoreFeatures stores a feature record for ML training
func (s *Store) StoreFeatures(record FeatureRecord) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(featuresBucket))
		if err != nil {
			return fmt.Errorf("create features bucket: %w", err)
		}

		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal feature record: %w", err)
		}

		key := fmt.Sprintf("%s_%d", record.Symbol, record.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

// StorePrice stores price data for labeling
func (s *Store) StorePrice(record PriceRecord) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(pricesBucket))
		if err != nil {
			return fmt.Errorf("create prices bucket: %w", err)
		}

		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal price record: %w", err)
		}

		key := fmt.Sprintf("%s_%d", record.Symbol, record.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

// ExportFeaturesToCSV exports all features to CSV format for training
func (s *Store) ExportFeaturesToCSV(filename string) error {
	// This will be implemented to export data for Python training
	// For now, we'll create the structure that the Python script expects
	return nil
}

// GetFeaturesInRange returns features within a time range
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
			if err := json.Unmarshal(v, &feature); err != nil {
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
