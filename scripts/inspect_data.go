package main

import (
	"bitunix-bot/internal/storage"
	"flag"
	"fmt"
	"log"
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

	// Improved data inspection logic
	fmt.Println("\nInspecting recent feature records:")
	records, err := store.GetRecentFeatures("BTCUSDT", 100)
	if err != nil {
		log.Fatalf("Failed to fetch recent features: %v", err)
	}

	for _, r := range records {
		fmt.Printf("Timestamp: %v, TickRatio: %.3f, DepthRatio: %.3f, PriceDist: %.3f\n",
			r.Timestamp, r.TickRatio, r.DepthRatio, r.PriceDist)
	}

	// Check what data we have
	fmt.Println("\nDatabase inspection:")

	// Try to get some sample trades
	fmt.Println("Checking for trades data...")

	// We'll have to look at the storage implementation to see how to query it
	fmt.Println("Storage opened successfully!")
	fmt.Println("To see the actual data, we need to query specific buckets.")
	fmt.Println("Available methods depend on the storage interface.")
}
