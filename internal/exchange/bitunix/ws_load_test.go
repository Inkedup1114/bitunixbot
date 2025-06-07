package bitunix

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// TestReconnectionUnderLoad tests the WebSocket reconnection behavior under high load
func TestReconnectionUnderLoad(t *testing.T) {
	// Set up logging for the test
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Create a test server that simulates connection issues under load
	var (
		messageCount     int32
		connectionCount  int32
		disconnectCount  int32
		reconnectSuccess int32
		serverReady      = make(chan struct{})
		serverClosed     = make(chan struct{})
		mu               sync.Mutex
		activeConns      = make(map[*websocket.Conn]bool)
		forceDisconnect  = make(chan struct{}, 10) // Channel to trigger forced disconnections
	)

	// Create an upgrader with check origin disabled for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate server overload by rejecting connections when too many messages are being processed
		if atomic.LoadInt32(&messageCount) > 1000 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Upgrade the connection to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Failed to upgrade connection: %v", err)
			return
		}

		// Track active connections
		mu.Lock()
		activeConns[conn] = true
		mu.Unlock()

		// Increment connection count
		atomic.AddInt32(&connectionCount, 1)

		// Handle the WebSocket connection
		go func() {
			defer func() {
				mu.Lock()
				delete(activeConns, conn)
				mu.Unlock()
				conn.Close()
			}()

			// Read messages from the client
			for {
				messageType, message, err := conn.ReadMessage()
				if err != nil {
					break
				}

				// Increment message count
				count := atomic.AddInt32(&messageCount, 1)

				// Simulate server overload by closing connections when too many messages are received
				if count > 5000 && count%100 == 0 {
					atomic.AddInt32(&disconnectCount, 1)
					break
				}

				// Echo the message back
				if err := conn.WriteMessage(messageType, message); err != nil {
					break
				}

				// Simulate subscription response for subscribe messages
				if string(message) == `{"op":"subscribe"}` {
					resp := `{"op":"subscribe","success":true}`
					if err := conn.WriteMessage(websocket.TextMessage, []byte(resp)); err != nil {
						break
					}
				}

				// Simulate trade and depth messages
				if count%10 == 0 {
					tradeMsg := `{"ch":"trade","symbol":"BTCUSDT","seq":` + fmt.Sprintf("%d", count) + `,"data":[{"p":"50000.0","v":"1.0","t":"2025-06-05T12:00:00Z"}]}`
					if err := conn.WriteMessage(websocket.TextMessage, []byte(tradeMsg)); err != nil {
						break
					}
				}

				if count%15 == 0 {
					depthMsg := `{"ch":"depth_books","symbol":"BTCUSDT","seq":` + fmt.Sprintf("%d", count) + `,"data":{"a":[["50100.0","1.5"]],"b":[["49900.0","2.0"]],"ts":"2025-06-05T12:00:00Z"}}`
					if err := conn.WriteMessage(websocket.TextMessage, []byte(depthMsg)); err != nil {
						break
					}
				}
			}
		}()
	}))
	defer server.Close()

	// Signal that the server is ready
	close(serverReady)

	// Create channels for the WebSocket client
	trades := make(chan Trade, 1000)
	depth := make(chan Depth, 1000)
	errors := make(chan error, 100)

	// Create a WebSocket client
	ws := NewWS("ws" + server.URL[4:])

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Start a goroutine to consume messages from the channels
	go func() {
		for {
			select {
			case <-trades:
				// Process trade
			case <-depth:
				// Process depth
			case err := <-errors:
				if err != nil {
					// Check if it's a reconnection error
					if fmt.Sprintf("%v", err) != "ws reconnect: <nil>" {
						t.Logf("Error: %v", err)
					} else {
						atomic.AddInt32(&reconnectSuccess, 1)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start the WebSocket stream
	go func() {
		defer close(serverClosed)
		err := ws.Stream(ctx, []string{"BTCUSDT"}, trades, depth, errors, 50*time.Millisecond) // Faster ping for quicker reconnection
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Stream error: %v", err)
		}
	}()

	// Wait for the server to be ready
	<-serverReady

	// Wait for the WebSocket connection to be established
	time.Sleep(500 * time.Millisecond)

	// Force disconnections at regular intervals
	for i := 0; i < 3; i++ {
		// Wait a bit
		time.Sleep(1 * time.Second)

		// Force disconnect all connections
		t.Logf("Forcing disconnection #%d", i+1)
		mu.Lock()
		for conn := range activeConns {
			select {
			case forceDisconnect <- struct{}{}:
			default:
			}
			conn.Close()
		}
		mu.Unlock()

		// Wait for reconnection to happen
		time.Sleep(1 * time.Second)
	}

	// Generate some load
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create a separate connection for load generation
			loadConn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
			if err != nil {
				t.Logf("Load generator %d failed to connect: %v", id, err)
				return
			}
			defer func() {
				if loadConn != nil {
					loadConn.Close()
				}
			}()

			// Send messages in a loop
			for j := 0; j < 100; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					if loadConn == nil {
						// Try to reconnect if connection is nil
						loadConn, _, err = websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
						if err != nil {
							time.Sleep(5 * time.Millisecond)
							continue
						}
					}

					msg := fmt.Sprintf(`{"id":%d,"seq":%d,"data":"load_test"}`, id, j)
					if err := loadConn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
						// Connection might have been closed, try to reconnect
						if loadConn != nil {
							loadConn.Close()
						}
						loadConn = nil
						continue
					}
					time.Sleep(5 * time.Millisecond) // Small delay to prevent overwhelming the server
				}
			}
		}(i)
	}

	// Wait for load generation to complete
	wg.Wait()

	// Wait a bit more for any reconnections to complete
	time.Sleep(1 * time.Second)

	// Wait for the server to close
	<-serverClosed

	// Force a reconnection by closing all active connections
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	for conn := range activeConns {
		conn.Close()
	}
	mu.Unlock()

	// Wait a bit for reconnection to happen
	time.Sleep(1 * time.Second)

	// Force another reconnection
	mu.Lock()
	for conn := range activeConns {
		conn.Close()
	}
	mu.Unlock()

	// Wait a bit for reconnection to happen
	time.Sleep(1 * time.Second)

	// Log test results
	reconnectCount := atomic.LoadInt32(&ws.reconnectCount)
	t.Logf("Test completed with:")
	t.Logf("- Total messages processed: %d", atomic.LoadInt32(&messageCount))
	t.Logf("- Total connections: %d", atomic.LoadInt32(&connectionCount))
	t.Logf("- Disconnections: %d", atomic.LoadInt32(&disconnectCount))
	t.Logf("- Successful reconnections: %d", atomic.LoadInt32(&reconnectSuccess))
	t.Logf("- WS reconnect count: %d", reconnectCount)
	t.Logf("- Memory stats: %+v", ws.GetConnectionStats())

	// Verify that reconnections happened
	if reconnectCount > 0 {
		t.Logf("Successfully detected %d reconnections", reconnectCount)
	} else {
		// If the reconnect count is still 0, let's check the errors channel for reconnection messages
		hasReconnectErrors := false
		for i := 0; i < 10; i++ {
			select {
			case err := <-errors:
				if strings.Contains(err.Error(), "ws reconnect") {
					hasReconnectErrors = true
					t.Logf("Found reconnection error: %v", err)
				}
			default:
				// No more errors
				break
			}
		}

		if !hasReconnectErrors {
			t.Errorf("No successful reconnections detected in WS reconnect count or error messages")
		} else {
			t.Logf("Reconnections detected in error messages")
		}
	}

	// Verify that the WebSocket client handled the load
	if atomic.LoadInt32(&messageCount) < 1000 {
		t.Errorf("Not enough messages processed: %d", atomic.LoadInt32(&messageCount))
	}
}

// TestMemoryLeakDetection tests the memory leak detection functionality
func TestMemoryLeakDetection(t *testing.T) {
	// Create a WebSocket client with memory monitoring
	ws := NewWS("ws://example.com")

	// Get initial memory stats
	initialStats := ws.GetConnectionStats()

	// Simulate message processing and object pool usage
	for i := 0; i < 1000; i++ {
		// Simulate trade processing
		trade := tradePool.Get().(*Trade)
		ws.memStats.TrackTradePoolGet()

		// Do something with the trade
		trade.Symbol = "BTCUSDT"
		trade.Price = 50000.0
		trade.Qty = 1.0
		trade.Ts = time.Now()
		trade.Seq = int64(i)

		// Return the trade to the pool
		tradePool.Put(trade)
		ws.memStats.TrackTradePoolPut()

		// Simulate depth processing
		depth := depthPool.Get().(*Depth)
		ws.memStats.TrackDepthPoolGet()

		// Do something with the depth
		depth.Symbol = "BTCUSDT"
		depth.BidVol = 2.0
		depth.AskVol = 1.5
		depth.LastPrice = 50000.0
		depth.Ts = time.Now()
		depth.Seq = int64(i)

		// Return the depth to the pool
		depthPool.Put(depth)
		ws.memStats.TrackDepthPoolPut()

		// Simulate message processing
		ws.memStats.TrackMessageProcessed(100)
	}

	// Get final memory stats
	finalStats := ws.GetConnectionStats()

	// Verify that the memory stats are being tracked
	t.Logf("Initial stats: %+v", initialStats)
	t.Logf("Final stats: %+v", finalStats)

	// Check for memory leaks in object pools
	tradePoolBalance, ok := finalStats["trade_pool_balance"].(int64)
	if !ok {
		t.Errorf("trade_pool_balance not found in stats")
	} else if tradePoolBalance != 0 {
		t.Errorf("Trade pool leak detected: %d", tradePoolBalance)
	}

	depthPoolBalance, ok := finalStats["depth_pool_balance"].(int64)
	if !ok {
		t.Errorf("depth_pool_balance not found in stats")
	} else if depthPoolBalance != 0 {
		t.Errorf("Depth pool leak detected: %d", depthPoolBalance)
	}

	// Verify that message processing is being tracked
	messagesProcessed, ok := finalStats["messages_processed"].(int64)
	if !ok {
		t.Errorf("messages_processed not found in stats")
	} else if messagesProcessed != 1000 {
		t.Errorf("Expected 1000 messages processed, got %d", messagesProcessed)
	}
}
