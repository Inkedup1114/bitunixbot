// Package storage provides persistent data storage for the Bitunix trading bot.
// It uses BoltDB as the underlying storage engine to store trading data including
// trades, market depth, and feature records for machine learning.
//
// The package provides thread-safe operations for storing and retrieving
// time-series data with efficient range queries and automatic bucket management.
package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"bitunix-bot/internal/exchange/bitunix"

	"go.etcd.io/bbolt"
)

const (
	tradesBucket = "trades" // Bucket name for storing trade records
	depthsBucket = "depths" // Bucket name for storing market depth records
)

// Store provides persistent storage for trading data using BoltDB.
// It manages multiple buckets for different data types and provides
// efficient time-range queries for historical data analysis.
type Store struct {
	db *bbolt.DB // BoltDB database instance
}

// New creates a new storage instance with the specified data path.
// It initializes the BoltDB database and creates necessary buckets.
// Returns an error if the database cannot be opened or buckets cannot be created.
func New(dataPath string) (*Store, error) {
	dbPath := filepath.Join(dataPath, "bitunix-data.db")

	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(tradesBucket)); err != nil {
			return fmt.Errorf("create trades bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(depthsBucket)); err != nil {
			return fmt.Errorf("create depths bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the database connection gracefully.
// It should be called when the storage is no longer needed to ensure
// proper cleanup of database resources.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// StoreTrade stores a trade record in the trades bucket.
// The trade is stored with a key format of "symbol_timestamp" for efficient
// time-range queries. Returns an error if the trade cannot be serialized or stored.
func (s *Store) StoreTrade(trade bitunix.Trade) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(tradesBucket))

		data, err := json.Marshal(trade)
		if err != nil {
			return fmt.Errorf("marshal trade: %w", err)
		}

		key := fmt.Sprintf("%s_%d", trade.Symbol, trade.Ts.UnixNano())
		return b.Put([]byte(key), data)
	})
}

// StoreDepth stores a market depth record in the depths bucket.
// The depth record is stored with a key format of "symbol_timestamp" for efficient
// time-range queries. Returns an error if the depth cannot be serialized or stored.
func (s *Store) StoreDepth(depth bitunix.Depth) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(depthsBucket))

		data, err := json.Marshal(depth)
		if err != nil {
			return fmt.Errorf("marshal depth: %w", err)
		}

		key := fmt.Sprintf("%s_%d", depth.Symbol, depth.Ts.UnixNano())
		return b.Put([]byte(key), data)
	})
}

// getRecordsInRange is a generic function to retrieve records from a bucket within a time range.
// It uses BoltDB cursors for efficient range scanning and applies the provided unmarshal function
// to deserialize each record. Returns a slice of records or an error if the query fails.
func (s *Store) getRecordsInRange(bucketName, symbol string, start, end time.Time, unmarshalFunc func([]byte) (interface{}, error)) ([]interface{}, error) {
	var records []interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		c := b.Cursor()

		prefix := []byte(symbol + "_")
		startKey := []byte(fmt.Sprintf("%s_%d", symbol, start.UnixNano()))
		endKey := []byte(fmt.Sprintf("%s_%d", symbol, end.UnixNano()))

		for k, v := c.Seek(startKey); k != nil && compareKeys(k, endKey) <= 0; k, v = c.Next() {
			if !hasPrefix(k, prefix) {
				continue
			}

			record, err := unmarshalFunc(v)
			if err != nil {
				continue // Skip malformed records
			}
			records = append(records, record)
		}

		return nil
	})

	return records, err
}

// GetTrades retrieves trade records for a specific symbol within a time range.
// Returns a slice of Trade structs ordered by timestamp, or an error if the query fails.
// The time range is inclusive of both start and end times.
func (s *Store) GetTrades(symbol string, start, end time.Time) ([]bitunix.Trade, error) {
	records, err := s.getRecordsInRange(tradesBucket, symbol, start, end, func(data []byte) (interface{}, error) {
		var trade bitunix.Trade
		err := json.Unmarshal(data, &trade)
		return trade, err
	})
	if err != nil {
		return nil, err
	}

	trades := make([]bitunix.Trade, len(records))
	for i, record := range records {
		trades[i] = record.(bitunix.Trade)
	}
	return trades, nil
}

// GetDepths retrieves market depth records for a specific symbol within a time range.
// Returns a slice of Depth structs ordered by timestamp, or an error if the query fails.
// The time range is inclusive of both start and end times.
func (s *Store) GetDepths(symbol string, start, end time.Time) ([]bitunix.Depth, error) {
	records, err := s.getRecordsInRange(depthsBucket, symbol, start, end, func(data []byte) (interface{}, error) {
		var depth bitunix.Depth
		err := json.Unmarshal(data, &depth)
		return depth, err
	})
	if err != nil {
		return nil, err
	}

	depths := make([]bitunix.Depth, len(records))
	for i, record := range records {
		depths[i] = record.(bitunix.Depth)
	}
	return depths, nil
}

func hasPrefix(data, prefix []byte) bool {
	return bytes.HasPrefix(data, prefix)
}

func compareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}
