package bitunix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Mock WebSocket server for testing
func createMockWSServer(behavior string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		switch behavior {
		case "normal":
			// Simulate normal operation
			handleNormalBehavior(conn)
		case "close_immediately":
			// Close connection immediately
			conn.Close()
		case "invalid_json":
			// Send invalid JSON
			conn.WriteMessage(websocket.TextMessage, []byte(`{"invalid": json}`))
		case "ping_timeout":
			// Don't respond to pings
			handlePingTimeout(conn)
		case "partial_data":
			// Send partial or malformed data
			handlePartialData(conn)
		}
	}))
}

func handleNormalBehavior(conn *websocket.Conn) {
	// Read subscription message
	_, _, err := conn.ReadMessage()
	if err != nil {
		return
	}

	// Send trade data
	tradeMsg := map[string]any{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"data": []map[string]any{
			{
				"p": "50000.0",
				"v": "0.1",
				"t": "2025-06-01T18:44:52Z",
			},
		},
	}
	conn.WriteJSON(tradeMsg)

	// Send depth data
	depthMsg := map[string]any{
		"ch":     "depth_books",
		"symbol": "BTCUSDT",
		"data": map[string]any{
			"b": [][]any{
				{"49950.0", "1.5"},
			},
			"a": [][]any{
				{"50050.0", "2.0"},
			},
		},
	}
	conn.WriteJSON(depthMsg)

	// Keep connection alive for a short time
	time.Sleep(100 * time.Millisecond)
}

func handlePingTimeout(conn *websocket.Conn) {
	// Read subscription but don't respond to pings
	_, _, err := conn.ReadMessage()
	if err != nil {
		return
	}

	// Wait for ping timeout
	time.Sleep(200 * time.Millisecond)
}

func handlePartialData(conn *websocket.Conn) {
	// Read subscription message
	_, _, err := conn.ReadMessage()
	if err != nil {
		return
	}

	// Send malformed trade data
	malformedTrade := map[string]any{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"data":   "invalid_data", // Should be array
	}
	conn.WriteJSON(malformedTrade)

	// Send malformed depth data
	malformedDepth := map[string]any{
		"ch":     "depth_books",
		"symbol": "BTCUSDT",
		"data": map[string]any{
			"b": "invalid", // Should be array
		},
	}
	conn.WriteJSON(malformedDepth)
}

func TestNewWS(t *testing.T) {
	url := "wss://test.example.com"
	ws := NewWS(url)

	if ws.url != url {
		t.Errorf("Expected URL %s, got %s", url, ws.url)
	}
}

func TestWSStream_Normal(t *testing.T) {
	server := createMockWSServer("normal")
	defer server.Close()

	// Convert HTTP URL to WS URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws := NewWS(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	go func() {
		err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 50*time.Millisecond)
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("Unexpected error: %v", err)
		}
	}()

	// Wait for data
	select {
	case trade := <-trades:
		if trade.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", trade.Symbol)
		}
		if trade.Price != 50000.0 {
			t.Errorf("Expected price 50000.0, got %f", trade.Price)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for trade data")
	}

	select {
	case d := <-depth:
		if d.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", d.Symbol)
		}
		if d.BidVol != 1.5 {
			t.Errorf("Expected bid volume 1.5, got %f", d.BidVol)
		}
		if d.AskVol != 2.0 {
			t.Errorf("Expected ask volume 2.0, got %f", d.AskVol)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for depth data")
	}
}

func TestWSStream_Reconnection(t *testing.T) {
	server := createMockWSServer("close_immediately")
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws := NewWS(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 50) // Larger buffer

	go func() {
		err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 10*time.Millisecond) // Faster reconnects
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("Stream ended with: %v", err)
		}
	}()

	// Should receive reconnection errors
	errorCount := 0
	timeout := time.After(4 * time.Second)

	for {
		select {
		case err := <-errors:
			if strings.Contains(err.Error(), "ws reconnect") {
				errorCount++
				t.Logf("Received reconnection error %d: %v", errorCount, err)
				if errorCount >= 2 {
					t.Logf("Successfully received %d reconnection errors", errorCount)
					return
				}
			}
		case <-timeout:
			if errorCount < 1 { // Accept at least 1 reconnection error as success
				t.Errorf("Expected at least 1 reconnection error, got %d", errorCount)
			} else {
				t.Logf("Received %d reconnection error(s), test passed", errorCount)
			}
			return
		}
	}
}

func TestWSStream_InvalidData(t *testing.T) {
	server := createMockWSServer("partial_data")
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws := NewWS(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	go func() {
		err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 50*time.Millisecond)
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("Stream ended with: %v", err)
		}
	}()

	// Should receive parsing errors
	errorCount := 0
	timeout := time.After(500 * time.Millisecond)

	for errorCount < 2 { // Expect errors for both trade and depth parsing
		select {
		case err := <-errors:
			if strings.Contains(err.Error(), "parse") {
				errorCount++
				t.Logf("Received parsing error %d: %v", errorCount, err)
			}
		case <-timeout:
			break
		}
	}

	if errorCount == 0 {
		t.Error("Expected parsing errors but got none")
	}
}

func TestParseTrade_Valid(t *testing.T) {
	tradeData := map[string]any{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"data": []any{
			map[string]any{
				"p": "50000.0",
				"v": "0.1",
			},
		},
	}

	trades := make(chan Trade, 1)
	err := parseTrade(tradeData, trades, 0)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	select {
	case trade := <-trades:
		if trade.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", trade.Symbol)
		}
		if trade.Price != 50000.0 {
			t.Errorf("Expected price 50000.0, got %f", trade.Price)
		}
		if trade.Qty != 0.1 {
			t.Errorf("Expected quantity 0.1, got %f", trade.Qty)
		}
	default:
		t.Error("Expected trade to be sent to channel")
	}
}

func TestParseTrade_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{
			name: "missing symbol",
			data: map[string]any{
				"ch": "trade",
				"data": []any{
					map[string]any{
						"p": "50000.0",
						"v": "0.1",
					},
				},
			},
		},
		{
			name: "invalid data format",
			data: map[string]any{
				"ch":     "trade",
				"symbol": "BTCUSDT",
				"data":   "invalid",
			},
		},
		{
			name: "empty data array",
			data: map[string]any{
				"ch":     "trade",
				"symbol": "BTCUSDT",
				"data":   []any{},
			},
		},
		{
			name: "invalid price",
			data: map[string]any{
				"ch":     "trade",
				"symbol": "BTCUSDT",
				"data": []any{
					map[string]any{
						"p": "invalid",
						"v": "0.1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trades := make(chan Trade, 1)
			err := parseTrade(tt.data, trades, 0)

			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestParseDepth_Valid(t *testing.T) {
	depthData := map[string]any{
		"ch":     "depth_books",
		"symbol": "BTCUSDT",
		"data": map[string]any{
			"b": []any{
				[]any{"49950.0", "1.5"},
			},
			"a": []any{
				[]any{"50050.0", "2.0"},
			},
		},
	}

	depths := make(chan Depth, 1)
	err := parseDepth(depthData, depths, 0)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	select {
	case depth := <-depths:
		if depth.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", depth.Symbol)
		}
		if depth.BidVol != 1.5 {
			t.Errorf("Expected bid volume 1.5, got %f", depth.BidVol)
		}
		if depth.AskVol != 2.0 {
			t.Errorf("Expected ask volume 2.0, got %f", depth.AskVol)
		}
		// Last price should be mid-price: (49950 + 50050) / 2 = 50000
		if depth.LastPrice != 50000.0 {
			t.Errorf("Expected last price 50000.0, got %f", depth.LastPrice)
		}
	default:
		t.Error("Expected depth to be sent to channel")
	}
}

func TestParseDepth_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{
			name: "missing symbol",
			data: map[string]any{
				"ch": "depth_books",
				"data": map[string]any{
					"b": []any{[]any{"49950.0", "1.5"}},
					"a": []any{[]any{"50050.0", "2.0"}},
				},
			},
		},
		{
			name: "invalid data format",
			data: map[string]any{
				"ch":     "depth_books",
				"symbol": "BTCUSDT",
				"data":   "invalid",
			},
		},
		{
			name: "empty bids",
			data: map[string]any{
				"ch":     "depth_books",
				"symbol": "BTCUSDT",
				"data": map[string]any{
					"b": []any{},
					"a": []any{[]any{"50050.0", "2.0"}},
				},
			},
		},
		{
			name: "invalid bid format",
			data: map[string]any{
				"ch":     "depth_books",
				"symbol": "BTCUSDT",
				"data": map[string]any{
					"b": []any{[]any{"invalid"}},
					"a": []any{[]any{"50050.0", "2.0"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depths := make(chan Depth, 1)
			err := parseDepth(tt.data, depths, 0)

			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    float64
		wantErr bool
	}{
		{"valid string", "123.45", 123.45, false},
		{"integer string", "100", 100.0, false},
		{"zero", "0", 0.0, false},
		{"negative", "-50.25", -50.25, false},
		{"integer input", 123, 123.0, false},
		{"float64 input", 123.45, 123.45, false},
		{"invalid number", "abc", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("toFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("toFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkParseTrade(b *testing.B) {
	tradeData := map[string]any{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"data": []any{
			map[string]any{
				"p": "50000.0",
				"v": "0.1",
			},
		},
	}

	trades := make(chan Trade, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parseTrade(tradeData, trades, 0)
		// Drain channel to prevent blocking
		select {
		case <-trades:
		default:
		}
	}
}

func BenchmarkParseDepth(b *testing.B) {
	depthData := map[string]any{
		"ch":     "depth_books",
		"symbol": "BTCUSDT",
		"data": map[string]any{
			"b": []any{
				[]any{"49950.0", "1.5"},
			},
			"a": []any{
				[]any{"50050.0", "2.0"},
			},
		},
	}

	depths := make(chan Depth, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parseDepth(depthData, depths, 0)
		// Drain channel to prevent blocking
		select {
		case <-depths:
		default:
		}
	}
}
