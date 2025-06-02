package main

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/storage"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// KlineData represents historical kline/candlestick data
type KlineData struct {
	OpenTime  int64   `json:"openTime"`
	Open      float64 `json:"open,string"`
	High      float64 `json:"high,string"`
	Low       float64 `json:"low,string"`
	Close     float64 `json:"close,string"`
	Volume    float64 `json:"volume,string"`
	CloseTime int64   `json:"closeTime"`
}

// HistoricalDataCollector collects historical data from Bitunix
type HistoricalDataCollector struct {
	client *resty.Client
	store  *storage.Store
	config *cfg.Settings
}

func NewHistoricalDataCollector(config *cfg.Settings, store *storage.Store) *HistoricalDataCollector {
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(5 * time.Second)

	return &HistoricalDataCollector{
		client: client,
		store:  store,
		config: config,
	}
}

// FetchKlines fetches historical kline data
func (h *HistoricalDataCollector) FetchKlines(symbol string, interval string, startTime, endTime time.Time) ([]KlineData, error) {
	// Bitunix API endpoint for klines (you may need to adjust based on actual API)
	url := fmt.Sprintf("%s/api/v1/market/klines", h.config.BaseURL)

	params := map[string]string{
		"symbol":    symbol,
		"interval":  interval,
		"startTime": fmt.Sprintf("%d", startTime.UnixMilli()),
		"endTime":   fmt.Sprintf("%d", endTime.UnixMilli()),
		"limit":     "1000",
	}

	resp, err := h.client.R().
		SetQueryParams(params).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch klines: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), resp.String())
	}

	var klines []KlineData
	if err := json.Unmarshal(resp.Body(), &klines); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return klines, nil
}

// ConvertAndStore converts kline data to trades and stores in BoltDB
func (h *HistoricalDataCollector) ConvertAndStore(symbol string, klines []KlineData) error {
	for _, kline := range klines {
		// Create synthetic trade data from klines
		// Using OHLC data to simulate trades
		timestamp := time.Unix(0, kline.OpenTime*int64(time.Millisecond))

		// Store open price as a trade
		trade := bitunix.Trade{
			Symbol: symbol,
			Price:  kline.Open,
			Qty:    kline.Volume / 4, // Distribute volume across OHLC
			Ts:     timestamp,
		}
		if err := h.store.StoreTrade(trade); err != nil {
			return fmt.Errorf("failed to store trade: %w", err)
		}

		// Store high, low, close as additional trades with timestamps
		// This gives more data points for VWAP calculation
		highTime := timestamp.Add(15 * time.Minute)
		trade.Price = kline.High
		trade.Ts = highTime
		h.store.StoreTrade(trade)

		lowTime := timestamp.Add(30 * time.Minute)
		trade.Price = kline.Low
		trade.Ts = lowTime
		h.store.StoreTrade(trade)

		closeTime := time.Unix(0, kline.CloseTime*int64(time.Millisecond))
		trade.Price = kline.Close
		trade.Ts = closeTime
		h.store.StoreTrade(trade)

		// Create synthetic depth data
		// This is approximated based on price movements
		depth := bitunix.Depth{
			Symbol:    symbol,
			BidVol:    kline.Volume * 0.45, // Approximate bid volume
			AskVol:    kline.Volume * 0.45, // Approximate ask volume
			LastPrice: kline.Close,
			Ts:        closeTime,
		}
		if err := h.store.StoreDepth(depth); err != nil {
			return fmt.Errorf("failed to store depth: %w", err)
		}
	}

	return nil
}

// CollectHistoricalData collects data for a date range
func (h *HistoricalDataCollector) CollectHistoricalData(symbol string, startDate, endDate time.Time, interval string) error {
	log.Printf("Collecting data for %s from %s to %s", symbol, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// Bitunix might have a limit on how much data can be fetched at once
	// So we'll fetch in chunks
	chunkSize := 7 * 24 * time.Hour // 7 days at a time
	current := startDate

	totalKlines := 0

	for current.Before(endDate) {
		chunkEnd := current.Add(chunkSize)
		if chunkEnd.After(endDate) {
			chunkEnd = endDate
		}

		log.Printf("Fetching chunk: %s to %s", current.Format("2006-01-02 15:04"), chunkEnd.Format("2006-01-02 15:04"))

		klines, err := h.FetchKlines(symbol, interval, current, chunkEnd)
		if err != nil {
			log.Printf("Error fetching klines: %v", err)
			// Continue with next chunk instead of failing completely
			current = chunkEnd
			continue
		}

		if len(klines) == 0 {
			log.Printf("No data returned for this period")
			current = chunkEnd
			continue
		}

		if err := h.ConvertAndStore(symbol, klines); err != nil {
			return fmt.Errorf("failed to store data: %w", err)
		}

		totalKlines += len(klines)
		log.Printf("Stored %d klines for this chunk", len(klines))

		// Move to next chunk
		current = chunkEnd

		// Rate limiting - be nice to the API
		time.Sleep(1 * time.Second)
	}

	log.Printf("Total klines collected for %s: %d", symbol, totalKlines)
	return nil
}

func main() {
	// Command line flags
	var (
		symbols   = flag.String("symbols", "", "Comma-separated symbols to collect (e.g., BTCUSDT,ETHUSDT)")
		days      = flag.Int("days", 30, "Number of days of historical data to collect")
		interval  = flag.String("interval", "1h", "Kline interval (1m, 5m, 15m, 1h, 4h, 1d)")
		startDate = flag.String("start", "", "Start date (YYYY-MM-DD), defaults to N days ago")
		endDate   = flag.String("end", "", "End date (YYYY-MM-DD), defaults to now")
		dataPath  = flag.String("data", "data", "Path to data directory")
	)
	flag.Parse()

	// Load configuration
	config, err := cfg.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Parse symbols
	symbolList := config.Symbols
	if *symbols != "" {
		symbolList = parseSymbols(*symbols)
	}

	if len(symbolList) == 0 {
		log.Fatal("No symbols specified")
	}

	// Parse dates
	var start, end time.Time
	if *startDate != "" {
		start, err = time.Parse("2006-01-02", *startDate)
		if err != nil {
			log.Fatalf("Invalid start date: %v", err)
		}
	} else {
		start = time.Now().AddDate(0, 0, -*days)
	}

	if *endDate != "" {
		end, err = time.Parse("2006-01-02", *endDate)
		if err != nil {
			log.Fatalf("Invalid end date: %v", err)
		}
	} else {
		end = time.Now()
	}

	// Initialize storage
	store, err := storage.New(*dataPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Create collector
	collector := NewHistoricalDataCollector(&config, store)

	// Collect data for each symbol
	for _, symbol := range symbolList {
		if err := collector.CollectHistoricalData(symbol, start, end, *interval); err != nil {
			log.Printf("Error collecting data for %s: %v", symbol, err)
			continue
		}
	}

	log.Println("Historical data collection completed")
}

func parseSymbols(symbols string) []string {
	var result []string
	for _, s := range splitString(symbols, ",") {
		s = trimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func splitString(s string, sep string) []string {
	// Simple string split implementation
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func trimSpace(s string) string {
	// Simple trim implementation
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}
