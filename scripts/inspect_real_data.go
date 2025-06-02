package main

import (
	"bitunix-bot/internal/storage"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
)

func main() {
	var dataPath = flag.String("data", "./data", "Data directory path")
	flag.Parse()

	fmt.Printf("Inspecting data in: %s\n", *dataPath)

	// Open storage
	store, err := storage.New(*dataPath)
	if err != nil {
		log.Fatalf("Failed to open storage: %v", err)
	}
	defer store.Close()

	// Check what data we have for common symbols
	symbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT", "BNBUSDT"}

	// Get data for the last 30 days
	end := time.Now()
	start := end.AddDate(0, 0, -30)

	fmt.Printf("\nLooking for data between %s and %s\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
	fmt.Println(strings.Repeat("=", 60))

	totalTrades := 0
	totalDepths := 0

	for _, symbol := range symbols {
		trades, err := store.GetTrades(symbol, start, end)
		if err != nil {
			fmt.Printf("Error getting trades for %s: %v\n", symbol, err)
			continue
		}

		depths, err := store.GetDepths(symbol, start, end)
		if err != nil {
			fmt.Printf("Error getting depths for %s: %v\n", symbol, err)
			continue
		}

		fmt.Printf("📊 %s: %d trades, %d depth records\n", symbol, len(trades), len(depths))

		if len(trades) > 0 {
			fmt.Printf("   First trade: %s (Price: %.2f)\n", trades[0].Ts.Format("2006-01-02 15:04:05"), trades[0].Price)
			fmt.Printf("   Last trade:  %s (Price: %.2f)\n", trades[len(trades)-1].Ts.Format("2006-01-02 15:04:05"), trades[len(trades)-1].Price)
		}

		totalTrades += len(trades)
		totalDepths += len(depths)

		fmt.Println()
	}

	fmt.Printf("📈 TOTAL: %d trades, %d depth records across all symbols\n", totalTrades, totalDepths)

	if totalTrades > 0 {
		fmt.Println("✅ Database contains real data - ready for backtesting!")
	} else {
		fmt.Println("⚠️  No data found - you may need to collect historical data first")
	}
}
