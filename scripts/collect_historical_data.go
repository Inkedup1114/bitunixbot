package main

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/storage"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
)

func main() {
	var (
		symbols    = flag.String("symbols", "", "Comma-separated symbols to collect")
		days       = flag.Int("days", 30, "Number of days to collect")
		interval   = flag.String("interval", "1h", "Kline interval (1m, 5m, 15m, 1h, 4h, 1d)")
		startDate  = flag.String("start", "", "Start date (YYYY-MM-DD)")
		endDate    = flag.String("end", "", "End date (YYYY-MM-DD)")
		dataPath   = flag.String("data", "", "Data directory path")
	)
	flag.Parse()

	// Load configuration
	config, err := cfg.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Use symbols from config if not provided
	symbolList := config.Symbols
	if *symbols != "" {
		symbolList = parseSymbols(*symbols)
	}

	// Use data path from config if not provided
	dataDir := config.DataPath
	if *dataPath != "" {
		dataDir = *dataPath
	}

	if len(symbolList) == 0 {
		log.Fatal("No symbols specified")
	}

	// Parse dates
	var startTime, endTime time.Time
	if *startDate != "" {
		startTime, err = time.Parse("2006-01-02", *startDate)
		if err != nil {
			log.Fatalf("Invalid start date format: %v", err)
		}
	} else {
		startTime = time.Now().AddDate(0, 0, -*days)
	}

	if *endDate != "" {
		endTime, err = time.Parse("2006-01-02", *endDate)
		if err != nil {
			log.Fatalf("Invalid end date format: %v", err)
		}
	} else {
		endTime = time.Now()
	}

	// Convert interval to kline interval
	var klineInterval bitunix.KlineInterval
	switch *interval {
	case "1m":
		klineInterval = bitunix.Interval1m
	case "5m":
		klineInterval = bitunix.Interval5m
	case "15m":
		klineInterval = bitunix.Interval15m
	case "1h":
		klineInterval = bitunix.Interval1h
	case "4h":
		klineInterval = bitunix.Interval4h
	case "1d":
		klineInterval = bitunix.Interval1d
	default:
		log.Fatalf("Invalid interval: %s", *interval)
	}

	fmt.Printf("Collecting historical data:\n")
	fmt.Printf("  Symbols: %v\n", symbolList)
	fmt.Printf("  Interval: %s\n", *interval)
	fmt.Printf("  Start: %s\n", startTime.Format("2006-01-02"))
	fmt.Printf("  End: %s\n", endTime.Format("2006-01-02"))
	fmt.Printf("  Data Path: %s\n", dataDir)

	// Create storage
	store, err := storage.New(dataDir)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create Bitunix client
	client := bitunix.NewREST(config.Key, config.Secret, config.BaseURL, config.RESTTimeout)

	// Collect data for each symbol
	for _, symbol := range symbolList {
		err := collectSymbolData(client, store, symbol, klineInterval, startTime, endTime)
		if err != nil {
			log.Printf("Failed to collect data for %s: %v", symbol, err)
			continue
		}
		fmt.Printf("✓ Collected data for %s\n", symbol)
	}

	fmt.Println("Historical data collection completed!")
}

func collectSymbolData(client *bitunix.Client, store *storage.Store, symbol string, interval bitunix.KlineInterval, startTime, endTime time.Time) error {
	const maxLimit = 1000 // Bitunix API limit
	
	fmt.Printf("Collecting data for %s...\n", symbol)
	
	current := startTime
	totalKlines := 0
	
	for current.Before(endTime) {
		// Calculate the end time for this batch
		batchEnd := current.Add(time.Duration(maxLimit) * getIntervalDuration(interval))
		if batchEnd.After(endTime) {
			batchEnd = endTime
		}
		
		// Fetch klines
		startMs := current.UnixMilli()
		endMs := batchEnd.UnixMilli()
		
		klines, err := client.GetKlines(symbol, interval, startMs, endMs, maxLimit)
		if err != nil {
			return fmt.Errorf("failed to fetch klines: %w", err)
		}
		
		if len(klines) == 0 {
			fmt.Printf("  No more data available for %s at %s\n", symbol, current.Format("2006-01-02 15:04:05"))
			break
		}
		
		// Convert klines to trades and store them
		for _, kline := range klines {
			trade := bitunix.Trade{
				Symbol: symbol,
				Price:  kline.Close, // Use close price as trade price
				Qty:    kline.Volume,
				Ts:     time.UnixMilli(kline.CloseTime),
			}
			
			err := store.StoreTrade(trade)
			if err != nil {
				return fmt.Errorf("failed to store trade: %w", err)
			}
			
			// Also create a synthetic depth entry
			depth := bitunix.Depth{
				Symbol:    symbol,
				BidVol:    kline.Volume * 0.5, // Synthetic bid volume
				AskVol:    kline.Volume * 0.5, // Synthetic ask volume
				LastPrice: kline.Close,
				Ts:        time.UnixMilli(kline.CloseTime),
			}
			
			err = store.StoreDepth(depth)
			if err != nil {
				return fmt.Errorf("failed to store depth: %w", err)
			}
		}
		
		totalKlines += len(klines)
		fmt.Printf("  Collected %d klines (total: %d)\n", len(klines), totalKlines)
		
		// Move to next batch
		current = time.UnixMilli(klines[len(klines)-1].CloseTime).Add(getIntervalDuration(interval))
		
		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}
	
	fmt.Printf("  Total %d data points collected for %s\n", totalKlines, symbol)
	return nil
}

func getIntervalDuration(interval bitunix.KlineInterval) time.Duration {
	switch interval {
	case bitunix.Interval1m:
		return time.Minute
	case bitunix.Interval5m:
		return 5 * time.Minute
	case bitunix.Interval15m:
		return 15 * time.Minute
	case bitunix.Interval1h:
		return time.Hour
	case bitunix.Interval4h:
		return 4 * time.Hour
	case bitunix.Interval1d:
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

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
