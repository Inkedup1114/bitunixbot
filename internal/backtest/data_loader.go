package backtest

import (
	"bitunix-bot/internal/storage"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// DataPoint represents a single data point in the backtest
type DataPoint struct {
	Type      string // "trade" or "depth"
	Symbol    string
	Timestamp time.Time
	Price     float64
	Volume    float64
	BidVolume float64
	AskVolume float64
}

// DataLoader handles loading and serving historical data
type DataLoader struct {
	data      []DataPoint
	index     int
	StartTime time.Time
	EndTime   time.Time
}

// NewDataLoader creates a new data loader
func NewDataLoader() *DataLoader {
	return &DataLoader{
		data:  make([]DataPoint, 0),
		index: 0,
	}
}

// LoadFromBoltDB loads data from BoltDB storage
func (dl *DataLoader) LoadFromBoltDB(store *storage.Store, symbols []string, startTime, endTime time.Time) error {
	log.Info().
		Time("start", startTime).
		Time("end", endTime).
		Strs("symbols", symbols).
		Msg("Loading data from BoltDB")

	for _, symbol := range symbols {
		// Load trades
		trades, err := store.GetTrades(symbol, startTime, endTime)
		if err != nil {
			return fmt.Errorf("failed to load trades for %s: %w", symbol, err)
		}

		for _, trade := range trades {
			dl.data = append(dl.data, DataPoint{
				Type:      "trade",
				Symbol:    symbol,
				Timestamp: trade.Ts,
				Price:     trade.Price,
				Volume:    trade.Qty,
			})
		}

		// Load depth data
		depths, err := store.GetDepths(symbol, startTime, endTime)
		if err != nil {
			return fmt.Errorf("failed to load depths for %s: %w", symbol, err)
		}

		for _, depth := range depths {
			dl.data = append(dl.data, DataPoint{
				Type:      "depth",
				Symbol:    symbol,
				Timestamp: depth.Ts,
				Price:     depth.LastPrice,
				BidVolume: depth.BidVol,
				AskVolume: depth.AskVol,
			})
		}
	}

	// Sort data by timestamp
	sort.Slice(dl.data, func(i, j int) bool {
		return dl.data[i].Timestamp.Before(dl.data[j].Timestamp)
	})

	if len(dl.data) > 0 {
		dl.StartTime = dl.data[0].Timestamp
		dl.EndTime = dl.data[len(dl.data)-1].Timestamp
	}

	log.Info().
		Int("total_points", len(dl.data)).
		Time("data_start", dl.StartTime).
		Time("data_end", dl.EndTime).
		Msg("Data loaded successfully")

	return nil
}

// LoadFromCSV loads data from CSV files
func (dl *DataLoader) LoadFromCSV(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Map header indices
	indices := make(map[string]int)
	for i, col := range header {
		indices[col] = i
	}

	// Read data
	for {
		record, err := reader.Read()
		if err != nil {
			break // EOF or error
		}

		// Parse timestamp
		timestamp, err := time.Parse("2006-01-02 15:04:05", record[indices["timestamp"]])
		if err != nil {
			continue
		}

		// Parse price
		price, err := strconv.ParseFloat(record[indices["price"]], 64)
		if err != nil {
			continue
		}

		// Parse volume
		volume := 0.0
		if idx, ok := indices["volume"]; ok {
			volume, _ = strconv.ParseFloat(record[idx], 64)
		}

		dataType := "trade"
		if typeIdx, ok := indices["type"]; ok {
			dataType = record[typeIdx]
		}

		point := DataPoint{
			Type:      dataType,
			Symbol:    record[indices["symbol"]],
			Timestamp: timestamp,
			Price:     price,
			Volume:    volume,
		}

		// Parse bid/ask volumes for depth data
		if dataType == "depth" {
			if idx, ok := indices["bid_volume"]; ok {
				point.BidVolume, _ = strconv.ParseFloat(record[idx], 64)
			}
			if idx, ok := indices["ask_volume"]; ok {
				point.AskVolume, _ = strconv.ParseFloat(record[idx], 64)
			}
		}

		dl.data = append(dl.data, point)
	}

	// Sort data by timestamp
	sort.Slice(dl.data, func(i, j int) bool {
		return dl.data[i].Timestamp.Before(dl.data[j].Timestamp)
	})

	if len(dl.data) > 0 {
		dl.StartTime = dl.data[0].Timestamp
		dl.EndTime = dl.data[len(dl.data)-1].Timestamp
	}

	log.Info().
		Str("file", filePath).
		Int("total_points", len(dl.data)).
		Msg("CSV data loaded successfully")

	return nil
}

// LoadFromJSON loads data from JSON files (exported from export_data.go)
func (dl *DataLoader) LoadFromJSON(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open JSON file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	for decoder.More() {
		var record storage.FeatureRecord
		if err := decoder.Decode(&record); err != nil {
			continue
		}

		// Create trade data point
		dl.data = append(dl.data, DataPoint{
			Type:      "trade",
			Symbol:    record.Symbol,
			Timestamp: record.Timestamp,
			Price:     record.Price,
			Volume:    1.0, // Default volume if not available
		})

		// Create depth data point
		if record.BidVol > 0 || record.AskVol > 0 {
			dl.data = append(dl.data, DataPoint{
				Type:      "depth",
				Symbol:    record.Symbol,
				Timestamp: record.Timestamp,
				Price:     record.Price,
				BidVolume: record.BidVol,
				AskVolume: record.AskVol,
			})
		}
	}

	// Sort data by timestamp
	sort.Slice(dl.data, func(i, j int) bool {
		return dl.data[i].Timestamp.Before(dl.data[j].Timestamp)
	})

	if len(dl.data) > 0 {
		dl.StartTime = dl.data[0].Timestamp
		dl.EndTime = dl.data[len(dl.data)-1].Timestamp
	}

	log.Info().
		Str("file", filePath).
		Int("total_points", len(dl.data)).
		Msg("JSON data loaded successfully")

	return nil
}

// Reset resets the data loader to the beginning
func (dl *DataLoader) Reset() {
	dl.index = 0
}

// HasNext returns true if there's more data to process
func (dl *DataLoader) HasNext() bool {
	return dl.index < len(dl.data)
}

// Next returns the next data point
func (dl *DataLoader) Next() DataPoint {
	if dl.index >= len(dl.data) {
		return DataPoint{}
	}

	point := dl.data[dl.index]
	dl.index++
	return point
}

// GetDataCount returns the total number of data points
func (dl *DataLoader) GetDataCount() int {
	return len(dl.data)
}

// GetProgress returns the current progress as a percentage
func (dl *DataLoader) GetProgress() float64 {
	if len(dl.data) == 0 {
		return 100.0
	}
	return float64(dl.index) / float64(len(dl.data)) * 100.0
}
