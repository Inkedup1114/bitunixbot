package main

import (
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/storage"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"
)

func main() {
	var (
		dataPath   = flag.String("data", "data", "Data directory path")
		symbol     = flag.String("symbol", "BTCUSDT", "Symbol to generate data for")
		days       = flag.Int("days", 7, "Number of days of data to generate")
		startPrice = flag.Float64("start-price", 50000, "Starting price")
	)
	flag.Parse()

	fmt.Printf("Generating sample data for %s...\n", *symbol)
	fmt.Printf("  Days: %d\n", *days)
	fmt.Printf("  Start Price: $%.2f\n", *startPrice)
	fmt.Printf("  Data Path: %s\n", *dataPath)

	// Create storage
	store, err := storage.New(*dataPath)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Generate realistic market data
	startTime := time.Now().AddDate(0, 0, -*days)
	endTime := time.Now()

	err = generateMarketData(store, *symbol, *startPrice, startTime, endTime)
	if err != nil {
		log.Fatalf("Failed to generate data: %v", err)
	}

	fmt.Printf("✓ Generated sample market data for %s\n", *symbol)
}

func generateMarketData(store *storage.Store, symbol string, startPrice float64, startTime, endTime time.Time) error {
	rand.Seed(time.Now().UnixNano())

	currentPrice := startPrice
	currentTime := startTime

	// Market simulation parameters
	volatility := 0.02      // 2% volatility
	trendStrength := 0.0001 // Small trend component
	meanReversionStrength := 0.05
	minuteInterval := time.Minute

	tradeCount := 0
	depthCount := 0

	for currentTime.Before(endTime) {
		// Simulate price movement with mean reversion and trend
		// Use geometric Brownian motion with mean reversion
		dt := 1.0 / (365 * 24 * 60) // 1 minute in years

		// Random walk component
		dW := rand.NormFloat64() * math.Sqrt(dt)

		// Mean reversion to a slowly changing trend
		trendPrice := startPrice * (1 + trendStrength*currentTime.Sub(startTime).Hours())
		meanReversion := meanReversionStrength * (trendPrice - currentPrice) * dt

		// Price change
		dPrice := currentPrice * (meanReversion + volatility*dW)
		currentPrice += dPrice

		// Ensure price doesn't go negative
		if currentPrice < 100 {
			currentPrice = 100
		}

		// Generate trade data (every minute)
		volume := 0.001 + rand.Float64()*0.01 // Random volume 0.001-0.011

		trade := bitunix.Trade{
			Symbol: symbol,
			Price:  currentPrice,
			Qty:    volume,
			Ts:     currentTime,
		}

		err := store.StoreTrade(trade)
		if err != nil {
			return fmt.Errorf("failed to store trade: %w", err)
		}
		tradeCount++

		// Generate depth data (every 30 seconds)
		if tradeCount%2 == 0 {
			// Simulate order book with some realistic properties
			spread := currentPrice * 0.0001 // 0.01% spread
			_ = currentPrice - spread/2     // bidPrice (for reference)
			_ = currentPrice + spread/2     // askPrice (for reference)

			// Volume tends to be higher near the current price
			baseVolume := 1.0 + rand.Float64()*5.0
			bidVolume := baseVolume * (0.8 + rand.Float64()*0.4)
			askVolume := baseVolume * (0.8 + rand.Float64()*0.4)

			// Add some imbalance occasionally
			if rand.Float64() < 0.2 {
				if rand.Float64() < 0.5 {
					bidVolume *= 2 // Bid heavy
				} else {
					askVolume *= 2 // Ask heavy
				}
			}

			depth := bitunix.Depth{
				Symbol:    symbol,
				BidVol:    bidVolume,
				AskVol:    askVolume,
				LastPrice: currentPrice,
				Ts:        currentTime,
			}

			err := store.StoreDepth(depth)
			if err != nil {
				return fmt.Errorf("failed to store depth: %w", err)
			}
			depthCount++
		}

		// Move to next minute
		currentTime = currentTime.Add(minuteInterval)
	}

	fmt.Printf("  Generated %d trades and %d depth updates\n", tradeCount, depthCount)
	fmt.Printf("  Final price: $%.2f\n", currentPrice)

	return nil
}
