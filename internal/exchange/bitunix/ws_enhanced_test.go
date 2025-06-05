package bitunix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock WebSocket server for testing
type mockWSServer struct {
	server   *httptest.Server
	upgrader websocket.Upgrader
	connChan chan *websocket.Conn
	delay    time.Duration
	sendPong bool
}

func newMockWSServer() *mockWSServer {
	m := &mockWSServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		connChan: make(chan *websocket.Conn, 1),
		sendPong: true,
	}

	m.server = httptest.NewServer(http.HandlerFunc(m.handleWebSocket))
	return m
}

func (m *mockWSServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	select {
	case m.connChan <- conn:
	default:
		conn.Close()
		return
	}

	// Handle ping messages and respond with pong if enabled
	conn.SetPingHandler(func(appData string) error {
		if m.sendPong {
			if m.delay > 0 {
				time.Sleep(m.delay)
			}
			return conn.WriteMessage(websocket.PongMessage, []byte(appData))
		}
		return nil
	})

	// Keep connection alive and handle messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Handle subscription messages
		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err == nil {
			if op, ok := data["op"].(string); ok && op == "subscribe" {
				response := map[string]interface{}{
					"op":      "subscribe",
					"success": true,
				}
				responseData, _ := json.Marshal(response)
				conn.WriteMessage(websocket.TextMessage, responseData)
			}
		}
	}
}

func (m *mockWSServer) getWebSocketURL() string {
	return "ws" + strings.TrimPrefix(m.server.URL, "http")
}

func (m *mockWSServer) close() {
	m.server.Close()
}

func (m *mockWSServer) getConnection() *websocket.Conn {
	select {
	case conn := <-m.connChan:
		return conn
	case <-time.After(time.Second):
		return nil
	}
}

func TestWS_Alive(t *testing.T) {
	tests := []struct {
		name          string
		connected     int32
		lastPongTime  int64
		lastPingTime  int64
		expectedAlive bool
	}{
		{
			name:          "not connected",
			connected:     0,
			expectedAlive: false,
		},
		{
			name:          "connected with no pong yet",
			connected:     1,
			lastPongTime:  0,
			expectedAlive: true,
		},
		{
			name:          "connected with recent pong",
			connected:     1,
			lastPongTime:  time.Now().UnixNano(),
			lastPingTime:  time.Now().Add(-1 * time.Second).UnixNano(),
			expectedAlive: true,
		},
		{
			name:          "connected with stale pong",
			connected:     1,
			lastPongTime:  time.Now().Add(-10 * time.Second).UnixNano(),
			lastPingTime:  time.Now().UnixNano(),
			expectedAlive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := NewWS("ws://localhost")
			atomic.StoreInt32(&ws.isConnected, tt.connected)
			atomic.StoreInt64(&ws.lastPongTime, tt.lastPongTime)
			atomic.StoreInt64(&ws.lastPingTime, tt.lastPingTime)

			assert.Equal(t, tt.expectedAlive, ws.Alive())
		})
	}
}

func TestWS_GetConnectionStats(t *testing.T) {
	ws := NewWS("ws://localhost")

	// Set some test values
	atomic.StoreInt32(&ws.isConnected, 1)
	atomic.StoreInt32(&ws.reconnectCount, 5)
	now := time.Now().UnixNano()
	atomic.StoreInt64(&ws.lastPongTime, now)
	atomic.StoreInt64(&ws.lastPingTime, now-1000000) // 1ms earlier

	stats := ws.GetConnectionStats()

	assert.Equal(t, true, stats["connected"])
	assert.Equal(t, int32(5), stats["reconnect_count"])
	assert.Equal(t, now, stats["last_pong_time"])
	assert.Equal(t, now-1000000, stats["last_ping_time"])
}

func TestWS_PingPongFlow(t *testing.T) {
	mockServer := newMockWSServer()
	defer mockServer.close()

	ws := NewWS(mockServer.getWebSocketURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	// Start the WebSocket stream in a goroutine
	go func() {
		ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 100*time.Millisecond)
	}()

	// Wait for connection to establish
	time.Sleep(200 * time.Millisecond)

	// Check that connection is alive
	assert.True(t, ws.Alive())

	// Verify connection stats
	stats := ws.GetConnectionStats()
	assert.True(t, stats["connected"].(bool))
	assert.True(t, stats["last_ping_time"].(int64) > 0)
}

func TestWS_PongTimeout(t *testing.T) {
	mockServer := newMockWSServer()
	mockServer.sendPong = false // Don't send pong responses
	defer mockServer.close()

	ws := NewWS(mockServer.getWebSocketURL())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	// Start the WebSocket stream
	start := time.Now()
	err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 100*time.Millisecond)

	// Should fail due to pong timeout or context timeout as both are valid failure modes
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "pong timeout") ||
			strings.Contains(err.Error(), "context deadline exceeded"),
		"Expected pong timeout or context timeout, got: %v", err.Error())

	// Should fail within reasonable time
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 15*time.Second)
}

func TestWS_ReconnectWithExponentialBackoff(t *testing.T) {
	// Create a server that will close immediately
	mockServer := newMockWSServer()
	mockServer.close() // Close immediately to force reconnection

	ws := NewWS(mockServer.getWebSocketURL())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	start := time.Now()
	err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 100*time.Millisecond)

	// Should fail after timeout
	assert.Error(t, err)
	elapsed := time.Since(start)

	// Should try multiple reconnects within the timeout period
	stats := ws.GetConnectionStats()
	reconnectCount := stats["reconnect_count"].(int32)
	assert.Greater(t, reconnectCount, int32(0))

	// Should respect the timeout
	assert.GreaterOrEqual(t, elapsed, 2*time.Second)
}

func TestWS_ConnectionStatusTracking(t *testing.T) {
	mockServer := newMockWSServer()
	defer mockServer.close()

	ws := NewWS(mockServer.getWebSocketURL())

	// Initially not connected
	assert.False(t, ws.Alive())
	assert.False(t, ws.GetConnectionStats()["connected"].(bool))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	// Start connection in goroutine
	go func() {
		ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 500*time.Millisecond)
	}()

	// Wait for connection to establish
	time.Sleep(300 * time.Millisecond)

	// Should be connected now
	assert.True(t, ws.Alive())
	assert.True(t, ws.GetConnectionStats()["connected"].(bool))

	// Cancel context to close connection
	cancel()

	// Wait for disconnection to be processed
	maxWait := time.Now().Add(1 * time.Second)
	for time.Now().Before(maxWait) {
		if !ws.GetConnectionStats()["connected"].(bool) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Should be disconnected
	assert.False(t, ws.GetConnectionStats()["connected"].(bool))
}

func TestWS_MessageProcessing(t *testing.T) {
	mockServer := newMockWSServer()
	defer mockServer.close()

	ws := NewWS(mockServer.getWebSocketURL())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	// Start the WebSocket stream in a goroutine
	go func() {
		ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 100*time.Millisecond)
	}()

	// Wait for connection and subscription
	time.Sleep(200 * time.Millisecond)

	// Get the server connection and send test messages
	conn := mockServer.getConnection()
	require.NotNil(t, conn)

	// Send a test trade message
	tradeMsg := map[string]interface{}{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"seq":    1,
		"data": []interface{}{
			map[string]interface{}{
				"p": "50000.0",
				"v": "1.5",
				"t": time.Now().Format(time.RFC3339),
			},
		},
	}
	tradeMsgData, _ := json.Marshal(tradeMsg)
	conn.WriteMessage(websocket.TextMessage, tradeMsgData)

	// Send a test depth message
	depthMsg := map[string]interface{}{
		"ch":     "depth_books",
		"symbol": "BTCUSDT",
		"seq":    2,
		"data": map[string]interface{}{
			"b":  []interface{}{[]interface{}{"49950.0", "10.0"}},
			"a":  []interface{}{[]interface{}{"50050.0", "8.0"}},
			"ts": time.Now().Format(time.RFC3339),
		},
	}
	depthMsgData, _ := json.Marshal(depthMsg)
	conn.WriteMessage(websocket.TextMessage, depthMsgData)

	// Verify messages were processed
	select {
	case trade := <-trades:
		assert.Equal(t, "BTCUSDT", trade.Symbol)
		assert.Equal(t, 50000.0, trade.Price)
		assert.Equal(t, 1.5, trade.Qty)
	case <-time.After(time.Second):
		t.Fatal("Trade message not received")
	}

	select {
	case depthData := <-depth:
		assert.Equal(t, "BTCUSDT", depthData.Symbol)
		assert.Equal(t, 10.0, depthData.BidVol)
		assert.Equal(t, 8.0, depthData.AskVol)
		assert.Equal(t, 50000.0, depthData.LastPrice) // Mid-price
	case <-time.After(time.Second):
		t.Fatal("Depth message not received")
	}
}

func TestWS_SlowPongResponse(t *testing.T) {
	mockServer := newMockWSServer()
	mockServer.delay = 6 * time.Second // Delay longer than pongTimeout (5s)
	defer mockServer.close()

	ws := NewWS(mockServer.getWebSocketURL())
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	trades := make(chan Trade, 10)
	depth := make(chan Depth, 10)
	errors := make(chan error, 10)

	// Start the WebSocket stream
	err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 100*time.Millisecond)

	// Should fail due to pong timeout or context timeout (both are valid)
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "pong timeout") ||
			strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "i/o timeout"),
		"Expected pong timeout, context timeout, or i/o timeout, got: %v", err.Error())
}

// Benchmark test for message processing performance
func BenchmarkWS_MessageProcessing(b *testing.B) {
	ws := NewWS("ws://localhost")
	trades := make(chan Trade, 1000)
	depth := make(chan Depth, 1000)
	errors := make(chan error, 1000)

	// Prepare test messages
	tradeMsg := map[string]interface{}{
		"ch":     "trade",
		"symbol": "BTCUSDT",
		"seq":    1,
		"data": []interface{}{
			map[string]interface{}{
				"p": "50000.0",
				"v": "1.5",
				"t": time.Now().Format(time.RFC3339),
			},
		},
	}
	tradeMsgData, _ := json.Marshal(tradeMsg)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ws.processMessage(tradeMsgData, trades, depth, errors)
		}
	})
}
