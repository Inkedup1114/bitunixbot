package bitunix

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseTradeWithActualFormat(t *testing.T) {
	// Test with actual Bitunix API format
	rawMessage := `{"ch":"trade","data":[{"p":"104350","s":"buy","t":"2025-06-01T18:44:52Z","v":"1.1536"}],"symbol":"BTCUSDT","ts":1748803492958}`

	var raw map[string]any
	if err := json.Unmarshal([]byte(rawMessage), &raw); err != nil {
		t.Fatalf("Failed to unmarshal test message: %v", err)
	}

	trades := make(chan Trade, 1)
	if err := parseTrade(raw, trades, 0); err != nil {
		t.Fatalf("parseTrade failed: %v", err)
	}

	select {
	case trade := <-trades:
		if trade.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", trade.Symbol)
		}
		if trade.Price != 104350.0 {
			t.Errorf("Expected price 104350.0, got %f", trade.Price)
		}
		if trade.Qty != 1.1536 {
			t.Errorf("Expected qty 1.1536, got %f", trade.Qty)
		}
		expectedTime, _ := time.Parse(time.RFC3339, "2025-06-01T18:44:52Z")
		if !trade.Ts.Equal(expectedTime) {
			t.Errorf("Expected timestamp %v, got %v", expectedTime, trade.Ts)
		}
	default:
		t.Fatal("No trade received")
	}
}

func TestParseDepthWithActualFormat(t *testing.T) {
	// Test with actual Bitunix API format
	rawMessage := `{"ch":"depth_books","data":{"a":[["104350","1.6317"]],"b":[["104349.9","0.9674"]]},"symbol":"BTCUSDT","ts":1748803493217}`

	var raw map[string]any
	if err := json.Unmarshal([]byte(rawMessage), &raw); err != nil {
		t.Fatalf("Failed to unmarshal test message: %v", err)
	}

	depths := make(chan Depth, 1)
	if err := parseDepth(raw, depths, 0); err != nil {
		t.Fatalf("parseDepth failed: %v", err)
	}

	select {
	case depth := <-depths:
		if depth.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", depth.Symbol)
		}
		// Bid volume from b[0][1] = "0.9674"
		if depth.BidVol != 0.9674 {
			t.Errorf("Expected bid volume 0.9674, got %f", depth.BidVol)
		}
		// Ask volume from a[0][1] = "1.6317"
		if depth.AskVol != 1.6317 {
			t.Errorf("Expected ask volume 1.6317, got %f", depth.AskVol)
		}
		// Mid-price should be (104349.9 + 104350) / 2 = 104349.95
		expectedMidPrice := (104349.9 + 104350.0) / 2
		if depth.LastPrice != expectedMidPrice {
			t.Errorf("Expected mid-price %f, got %f", expectedMidPrice, depth.LastPrice)
		}
	default:
		t.Fatal("No depth received")
	}
}
