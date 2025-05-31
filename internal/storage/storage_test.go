package storage

import (
	"bitunix-bot/internal/exchange/bitunix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tempDir := t.TempDir()

	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Error("Store database is nil")
	}

	// Check if database file was created
	dbPath := filepath.Join(tempDir, "bitunix-data.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Try to create store in non-existent directory without permissions
	invalidPath := "/root/nonexistent/path"

	_, err := New(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

func TestStore_Close(t *testing.T) {
	tempDir := t.TempDir()

	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Error closing store: %v", err)
	}

	// Test closing already closed store
	err = store.Close()
	if err != nil {
		t.Errorf("Error closing already closed store: %v", err)
	}
}

func TestStore_CloseNilDB(t *testing.T) {
	store := &Store{db: nil}
	err := store.Close()
	if err != nil {
		t.Errorf("Expected no error for nil db, got: %v", err)
	}
}

func TestStoreTrade(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	trade := bitunix.Trade{
		Symbol: "BTCUSDT",
		Price:  50000.00,
		Qty:    0.001,
		Ts:     time.Now(),
	}

	err = store.StoreTrade(trade)
	if err != nil {
		t.Errorf("Failed to store trade: %v", err)
	}
}

func TestStoreDepth(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	depth := bitunix.Depth{
		Symbol:    "BTCUSDT",
		BidVol:    1.5, // Total bid volume
		AskVol:    1.1, // Total ask volume
		LastPrice: 50000.0,
		Ts:        time.Now(),
	}

	err = store.StoreDepth(depth)
	if err != nil {
		t.Errorf("Failed to store depth: %v", err)
	}
}

func TestGetTrades(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	trades := []bitunix.Trade{
		{
			Symbol: "BTCUSDT",
			Price:  50000.00,
			Qty:    0.001,
			Ts:     now,
		},
		{
			Symbol: "BTCUSDT",
			Price:  50010.00,
			Qty:    0.002,
			Ts:     now.Add(time.Second),
		},
		{
			Symbol: "ETHUSDT",
			Price:  3000.00,
			Qty:    0.1,
			Ts:     now.Add(2 * time.Second),
		},
		{
			Symbol: "BTCUSDT",
			Price:  49990.00,
			Qty:    0.003,
			Ts:     now.Add(10 * time.Second), // Outside range
		},
	}

	// Store all trades
	for _, trade := range trades {
		err = store.StoreTrade(trade)
		if err != nil {
			t.Fatalf("Failed to store trade: %v", err)
		}
	}

	// Retrieve trades for BTCUSDT within 5 seconds
	start := now.Add(-time.Second)
	end := now.Add(5 * time.Second)
	retrievedTrades, err := store.GetTrades("BTCUSDT", start, end)
	if err != nil {
		t.Fatalf("Failed to get trades: %v", err)
	}

	// Should get only the first 2 BTCUSDT trades
	expectedCount := 2
	if len(retrievedTrades) != expectedCount {
		t.Errorf("Expected %d trades, got %d", expectedCount, len(retrievedTrades))
	}

	// Check first trade
	if len(retrievedTrades) > 0 {
		if retrievedTrades[0].Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", retrievedTrades[0].Symbol)
		}
		if retrievedTrades[0].Price != 50000.00 {
			t.Errorf("Expected price 50000.00, got %f", retrievedTrades[0].Price)
		}
	}
}

func TestGetTrades_EmptyResult(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	start := now.Add(-time.Hour)
	end := now.Add(-30 * time.Minute)

	trades, err := store.GetTrades("BTCUSDT", start, end)
	if err != nil {
		t.Fatalf("Failed to get trades: %v", err)
	}

	if len(trades) != 0 {
		t.Errorf("Expected empty result, got %d trades", len(trades))
	}
}

func TestGetDepths(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	depths := []bitunix.Depth{
		{
			Symbol:    "BTCUSDT",
			BidVol:    0.5,
			AskVol:    0.3,
			LastPrice: 50000.0,
			Ts:        now,
		},
		{
			Symbol:    "BTCUSDT",
			BidVol:    0.7,
			AskVol:    0.4,
			LastPrice: 50025.0,
			Ts:        now.Add(time.Second),
		},
	}

	// Store depths
	for _, depth := range depths {
		err = store.StoreDepth(depth)
		if err != nil {
			t.Fatalf("Failed to store depth: %v", err)
		}
	}

	// Retrieve depths
	start := now.Add(-time.Second)
	end := now.Add(5 * time.Second)
	retrievedDepths, err := store.GetDepths("BTCUSDT", start, end)
	if err != nil {
		t.Fatalf("Failed to get depths: %v", err)
	}

	expectedCount := 2
	if len(retrievedDepths) != expectedCount {
		t.Errorf("Expected %d depths, got %d", expectedCount, len(retrievedDepths))
	}

	// Check first depth
	if len(retrievedDepths) > 0 {
		if retrievedDepths[0].Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", retrievedDepths[0].Symbol)
		}
		if retrievedDepths[0].BidVol != 0.5 {
			t.Errorf("Expected bid volume 0.5, got %f", retrievedDepths[0].BidVol)
		}
	}
}

func TestStoreFeatures(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	feature := FeatureRecord{
		Symbol:     "BTCUSDT",
		Timestamp:  time.Now(),
		TickRatio:  0.5,
		DepthRatio: -0.2,
		PriceDist:  1.5,
		Price:      50000.0,
		VWAP:       49950.0,
		StdDev:     100.0,
		BidVol:     1000.0,
		AskVol:     800.0,
	}

	err = store.StoreFeatures(feature)
	if err != nil {
		t.Errorf("Failed to store features: %v", err)
	}
}

func TestStorePrice(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	price := PriceRecord{
		Symbol:    "BTCUSDT",
		Timestamp: time.Now(),
		Price:     50000.0,
		VWAP:      49950.0,
		StdDev:    100.0,
	}

	err = store.StorePrice(price)
	if err != nil {
		t.Errorf("Failed to store price: %v", err)
	}
}

func TestGetFeaturesInRange(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	features := []FeatureRecord{
		{
			Symbol:     "BTCUSDT",
			Timestamp:  now,
			TickRatio:  0.5,
			DepthRatio: -0.2,
			PriceDist:  1.5,
			Price:      50000.0,
		},
		{
			Symbol:     "BTCUSDT",
			Timestamp:  now.Add(time.Second),
			TickRatio:  0.3,
			DepthRatio: 0.1,
			PriceDist:  -0.8,
			Price:      49950.0,
		},
		{
			Symbol:     "ETHUSDT",
			Timestamp:  now.Add(2 * time.Second),
			TickRatio:  -0.1,
			DepthRatio: 0.4,
			PriceDist:  0.2,
			Price:      3000.0,
		},
		{
			Symbol:     "BTCUSDT",
			Timestamp:  now.Add(10 * time.Second), // Outside range
			TickRatio:  0.7,
			DepthRatio: -0.5,
			PriceDist:  2.1,
			Price:      51000.0,
		},
	}

	// Store features
	for _, feature := range features {
		err = store.StoreFeatures(feature)
		if err != nil {
			t.Fatalf("Failed to store feature: %v", err)
		}
	}

	// Retrieve features for BTCUSDT within range
	start := now.Add(-time.Second)
	end := now.Add(5 * time.Second)
	retrievedFeatures, err := store.GetFeaturesInRange("BTCUSDT", start, end)
	if err != nil {
		t.Fatalf("Failed to get features: %v", err)
	}

	// Should get only the first 2 BTCUSDT features
	expectedCount := 2
	if len(retrievedFeatures) != expectedCount {
		t.Errorf("Expected %d features, got %d", expectedCount, len(retrievedFeatures))
	}

	// Check first feature
	if len(retrievedFeatures) > 0 {
		if retrievedFeatures[0].Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", retrievedFeatures[0].Symbol)
		}
		if retrievedFeatures[0].TickRatio != 0.5 {
			t.Errorf("Expected tick ratio 0.5, got %f", retrievedFeatures[0].TickRatio)
		}
	}
}

func TestGetFeaturesInRange_NoBucket(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	// Try to get features when no features bucket exists
	features, err := store.GetFeaturesInRange("BTCUSDT", start, end)
	if err != nil {
		t.Fatalf("Failed to get features: %v", err)
	}

	if len(features) != 0 {
		t.Errorf("Expected empty result when no bucket exists, got %d features", len(features))
	}
}

func TestHasPrefix(t *testing.T) {
	testCases := []struct {
		data     []byte
		prefix   []byte
		expected bool
	}{
		{[]byte("BTCUSDT_123456"), []byte("BTCUSDT_"), true},
		{[]byte("ETHUSDT_789012"), []byte("BTCUSDT_"), false},
		{[]byte("BTC"), []byte("BTCUSDT_"), false},
		{[]byte(""), []byte("BTCUSDT_"), false},
		{[]byte("BTCUSDT_123456"), []byte(""), true},
	}

	for _, tc := range testCases {
		result := hasPrefix(tc.data, tc.prefix)
		if result != tc.expected {
			t.Errorf("hasPrefix(%q, %q) = %v, expected %v", tc.data, tc.prefix, result, tc.expected)
		}
	}
}

func TestCompareKeys(t *testing.T) {
	testCases := []struct {
		a        []byte
		b        []byte
		expected int
	}{
		{[]byte("BTCUSDT_123456"), []byte("BTCUSDT_123456"), 0},
		{[]byte("BTCUSDT_123456"), []byte("BTCUSDT_123457"), -1},
		{[]byte("BTCUSDT_123457"), []byte("BTCUSDT_123456"), 1},
		{[]byte("BTCUSDT_"), []byte("ETHUSDT_"), -1},
		{[]byte("ETHUSDT_"), []byte("BTCUSDT_"), 1},
	}

	for _, tc := range testCases {
		result := compareKeys(tc.a, tc.b)
		if (result < 0 && tc.expected >= 0) || (result > 0 && tc.expected <= 0) || (result == 0 && tc.expected != 0) {
			t.Errorf("compareKeys(%q, %q) = %v, expected %v", tc.a, tc.b, result, tc.expected)
		}
	}
}

func TestExportFeaturesToCSV(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// This is currently a no-op, just test it doesn't panic
	err = store.ExportFeaturesToCSV("test.csv")
	if err != nil {
		t.Errorf("ExportFeaturesToCSV failed: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test concurrent reads and writes
	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(id int) {
			now := time.Now()
			for j := 0; j < 10; j++ {
				trade := bitunix.Trade{
					Symbol: "BTCUSDT",
					Price:  50000.00,
					Qty:    0.001,
					Ts:     now.Add(time.Duration(j) * time.Millisecond),
				}
				store.StoreTrade(trade)

				feature := FeatureRecord{
					Symbol:    "BTCUSDT",
					Timestamp: now.Add(time.Duration(j) * time.Millisecond),
					TickRatio: 0.5,
					Price:     50000.0,
				}
				store.StoreFeatures(feature)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			now := time.Now()
			for j := 0; j < 10; j++ {
				start := now.Add(-time.Second)
				end := now.Add(time.Second)
				store.GetTrades("BTCUSDT", start, end)
				store.GetFeaturesInRange("BTCUSDT", start, end)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkStoreTrade(b *testing.B) {
	tempDir := b.TempDir()
	store, err := New(tempDir)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-allocate timestamps to avoid allocation in hot loop
	baseTime := time.Now()
	trades := make([]bitunix.Trade, b.N)
	for i := 0; i < b.N; i++ {
		trades[i] = bitunix.Trade{
			Symbol: "BTCUSDT",
			Price:  50000.00,
			Qty:    0.001,
			Ts:     baseTime.Add(time.Duration(i) * time.Nanosecond),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.StoreTrade(trades[i])
	}
}

func BenchmarkStoreFeatures(b *testing.B) {
	tempDir := b.TempDir()
	store, err := New(tempDir)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-allocate features to avoid allocation in hot loop
	baseTime := time.Now()
	features := make([]FeatureRecord, b.N)
	for i := 0; i < b.N; i++ {
		features[i] = FeatureRecord{
			Symbol:     "BTCUSDT",
			Timestamp:  baseTime.Add(time.Duration(i) * time.Nanosecond),
			TickRatio:  0.5,
			DepthRatio: -0.2,
			PriceDist:  1.5,
			Price:      50000.0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.StoreFeatures(features[i])
	}
}
