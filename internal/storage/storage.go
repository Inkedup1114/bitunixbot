package storage

import (
	"bitunix-bot/internal/exchange/bitunix"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"go.etcd.io/bbolt"
)

const (
	tradesBucket = "trades"
	depthsBucket = "depths"
)

// Store provides persistent storage for trading data using BoltDB
type Store struct {
	db *bbolt.DB
}

// New creates a new storage instance
func New(dataPath string) (*Store, error) {
	dbPath := filepath.Join(dataPath, "bitunix-data.db")

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
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

// Close closes the database connection
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// StoreTrade stores a trade record
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

// StoreDepth stores a depth record
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

// GetTrades retrieves trades for a symbol within a time range
func (s *Store) GetTrades(symbol string, start, end time.Time) ([]bitunix.Trade, error) {
	var trades []bitunix.Trade

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(tradesBucket))
		c := b.Cursor()

		prefix := []byte(symbol + "_")
		startKey := []byte(fmt.Sprintf("%s_%d", symbol, start.UnixNano()))
		endKey := []byte(fmt.Sprintf("%s_%d", symbol, end.UnixNano()))

		for k, v := c.Seek(startKey); k != nil && compareKeys(k, endKey) <= 0; k, v = c.Next() {
			if !hasPrefix(k, prefix) {
				continue
			}

			var trade bitunix.Trade
			if err := json.Unmarshal(v, &trade); err != nil {
				continue // Skip malformed records
			}
			trades = append(trades, trade)
		}

		return nil
	})

	return trades, err
}

// GetDepths retrieves depth records for a symbol within a time range
func (s *Store) GetDepths(symbol string, start, end time.Time) ([]bitunix.Depth, error) {
	var depths []bitunix.Depth

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(depthsBucket))
		c := b.Cursor()

		prefix := []byte(symbol + "_")
		startKey := []byte(fmt.Sprintf("%s_%d", symbol, start.UnixNano()))
		endKey := []byte(fmt.Sprintf("%s_%d", symbol, end.UnixNano()))

		for k, v := c.Seek(startKey); k != nil && compareKeys(k, endKey) <= 0; k, v = c.Next() {
			if !hasPrefix(k, prefix) {
				continue
			}

			var depth bitunix.Depth
			if err := json.Unmarshal(v, &depth); err != nil {
				continue // Skip malformed records
			}
			depths = append(depths, depth)
		}

		return nil
	})

	return depths, err
}

func hasPrefix(data, prefix []byte) bool {
	return bytes.HasPrefix(data, prefix)
}

func compareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}
