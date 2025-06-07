package bitunix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	defaultBufferSize = 1000            // Default buffer size for channels
	maxConnections    = 10              // Maximum concurrent WebSocket connections
	workerPoolSize    = 5               // Number of worker goroutines for message processing
	messagePoolSize   = 1000            // Size of message object pool
	pongTimeout       = 5 * time.Second // Pong timeout threshold for connection health
)

// Object pools for frequently created objects to reduce garbage collection pressure
var (
	tradePool = sync.Pool{
		New: func() interface{} {
			return &Trade{}
		},
	}
	depthPool = sync.Pool{
		New: func() interface{} {
			return &Depth{}
		},
	}
	messagePool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 4096)
		},
	}
)

// Trade represents a trade execution from the WebSocket stream.
// It contains price, quantity, timestamp, and sequence information
// for real-time trade processing and analysis.
type Trade struct {
	Symbol string    // Trading symbol
	Price  float64   // Execution price
	Qty    float64   // Execution quantity
	Ts     time.Time // Execution timestamp
	Seq    int64     // Sequence number for ordering
}

// Depth represents order book depth information from the WebSocket stream.
// It contains bid/ask volume aggregates and last price for market analysis
// and order book imbalance calculations.
type Depth struct {
	Symbol         string    // Trading symbol
	BidVol, AskVol float64   // Total bid and ask volumes
	LastPrice      float64   // Last traded price
	Ts             time.Time // Timestamp of depth update
	Seq            int64     // Sequence number for ordering
}

// Lock-free sequence tracking
type sequenceTracker struct {
	sequences unsafe.Pointer // *map[string]int64
}

func newSequenceTracker() *sequenceTracker {
	m := make(map[string]int64)
	return &sequenceTracker{
		sequences: unsafe.Pointer(&m),
	}
}

func (st *sequenceTracker) get(symbol string) int64 {
	m := *(*map[string]int64)(atomic.LoadPointer(&st.sequences))
	return m[symbol]
}

func (st *sequenceTracker) set(symbol string, seq int64) {
	for {
		old := atomic.LoadPointer(&st.sequences)
		m := *(*map[string]int64)(old)
		newM := make(map[string]int64, len(m)+1)
		for k, v := range m {
			newM[k] = v
		}
		newM[symbol] = seq
		if atomic.CompareAndSwapPointer(&st.sequences, old, unsafe.Pointer(&newM)) {
			return
		}
	}
}

type WS struct {
	url            string
	connPool       chan *websocket.Conn
	workerPool     chan struct{}
	seqTracker     *sequenceTracker
	reconnectCount int32

	// Connection status tracking
	isConnected  int32 // atomic bool (0 = false, 1 = true)
	lastPongTime int64 // atomic unix timestamp
	lastPingTime int64 // atomic unix timestamp

	// Memory monitoring
	memStats *MemoryStats
}

func NewWS(u string) *WS {
	ws := &WS{
		url:        u,
		connPool:   make(chan *websocket.Conn, maxConnections),
		workerPool: make(chan struct{}, workerPoolSize),
		seqTracker: newSequenceTracker(),
		memStats:   NewMemoryStats(),
	}

	// Start memory monitoring
	ws.memStats.StartMonitoring()

	return ws
}

// Alive returns true if the WebSocket connection is healthy
func (w *WS) Alive() bool {
	if atomic.LoadInt32(&w.isConnected) == 0 {
		return false
	}

	lastPong := atomic.LoadInt64(&w.lastPongTime)
	lastPing := atomic.LoadInt64(&w.lastPingTime)

	// If we never received a pong, use current time as baseline
	if lastPong == 0 {
		return true
	}

	// Check if pong response is within acceptable timeframe
	pongTime := time.Unix(0, lastPong)
	pingTime := time.Unix(0, lastPing)

	// If we sent a ping and haven't received pong within timeout, connection might be dead
	if !pingTime.IsZero() && time.Since(pongTime) > pongTimeout {
		return false
	}

	return true
}

// GetConnectionStats returns connection statistics
func (w *WS) GetConnectionStats() map[string]interface{} {
	stats := map[string]interface{}{
		"connected":       atomic.LoadInt32(&w.isConnected) == 1,
		"reconnect_count": atomic.LoadInt32(&w.reconnectCount),
		"last_pong_time":  atomic.LoadInt64(&w.lastPongTime),
		"last_ping_time":  atomic.LoadInt64(&w.lastPingTime),
	}

	// Add memory stats if available
	if w.memStats != nil {
		memStats := w.memStats.GetStats()
		for k, v := range memStats {
			stats[k] = v
		}
	}

	return stats
}

// Zero-copy message parsing
func parseMessage(msg []byte) (map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(msg, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
	return raw, nil
}

func (w *WS) Stream(ctx context.Context, symbols []string, trades chan<- Trade, depth chan<- Depth, errors chan<- error, ping time.Duration) error {
	// Ensure channels are buffered
	if cap(trades) == 0 {
		trades = make(chan Trade, defaultBufferSize)
	}
	if cap(depth) == 0 {
		depth = make(chan Depth, defaultBufferSize)
	}
	if cap(errors) == 0 {
		errors = make(chan error, defaultBufferSize)
	}

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&w.isConnected, 0)
			return ctx.Err()
		default:
			if err := w.streamOnce(ctx, symbols, trades, depth, errors, ping); err != nil {
				atomic.StoreInt32(&w.isConnected, 0)
				log.Warn().Err(err).Dur("backoff", backoff).Msg("WebSocket connection failed, reconnecting with exponential backoff...")
				select {
				case errors <- fmt.Errorf("ws reconnect: %w", err):
				default:
				}

				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					atomic.StoreInt32(&w.isConnected, 0)
					return ctx.Err()
				}

				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				atomic.AddInt32(&w.reconnectCount, 1)
				continue
			}
			backoff = time.Second
			atomic.StoreInt32(&w.reconnectCount, 0)
		}
	}
}

func (w *WS) streamOnce(ctx context.Context, symbols []string, trades chan<- Trade, depth chan<- Depth, errors chan<- error, ping time.Duration) error {
	// Normalize URL by removing trailing slash
	url := strings.TrimRight(w.url, "/")
	log.Info().Str("url", url).Int("symbols_count", len(symbols)).Msg("Establishing WebSocket connection")

	// Track connection in memory stats
	if w.memStats != nil {
		w.memStats.TrackConnectionActive()
	}

	// Get connection from pool or create new
	var conn *websocket.Conn
	select {
	case conn = <-w.connPool:
		// Reuse existing connection
	default:
		var err error
		var resp *http.Response
		conn, resp, err = websocket.DefaultDialer.DialContext(ctx, url, nil)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err != nil {
			return fmt.Errorf("dial failed: %w", err)
		}
	}

	defer func() {
		atomic.StoreInt32(&w.isConnected, 0)
		// Track connection closure in memory stats
		if w.memStats != nil {
			w.memStats.TrackConnectionClosed()
		}
		// Return connection to pool
		select {
		case w.connPool <- conn:
		default:
			conn.Close()
		}
	}()

	// Set connection parameters
	conn.SetReadLimit(512 * 1024)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Pong wait channel for tracking pong responses
	pongWait := make(chan struct{}, 1)

	// Set handlers
	conn.SetCloseHandler(func(code int, text string) error {
		log.Warn().Int("code", code).Str("text", text).Msg("WebSocket connection closed by server")
		atomic.StoreInt32(&w.isConnected, 0)
		return fmt.Errorf("connection closed: %d %s", code, text)
	})

	conn.SetPongHandler(func(appData string) error {
		atomic.StoreInt64(&w.lastPongTime, time.Now().UnixNano())
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Signal that we received a pong
		select {
		case pongWait <- struct{}{}:
		default:
		}

		log.Debug().Msg("Received pong from server")
		return nil
	})

	// Subscribe to channels
	var args []map[string]string
	for _, s := range symbols {
		args = append(args, map[string]string{"symbol": s, "ch": "trade"})
		args = append(args, map[string]string{"symbol": s, "ch": "depth_books"})
	}

	if err := conn.WriteJSON(map[string]any{"op": "subscribe", "args": args}); err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	// Setup ping ticker
	pingTicker := time.NewTicker(ping)
	defer pingTicker.Stop()

	// Send initial ping
	atomic.StoreInt64(&w.lastPingTime, time.Now().UnixNano())
	if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
		return fmt.Errorf("initial ping failed: %w", err)
	}

	// Health check ticker
	healthCheckTicker := time.NewTicker(30 * time.Second)
	defer healthCheckTicker.Stop()

	// Pong timeout checker
	pongTimeoutTicker := time.NewTicker(pongTimeout)
	defer pongTimeoutTicker.Stop()

	lastDataReceived := time.Now()
	atomic.StoreInt32(&w.isConnected, 1)

	// Message processing loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-pingTicker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			atomic.StoreInt64(&w.lastPingTime, time.Now().UnixNano())
			if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				select {
				case errors <- fmt.Errorf("ping failed: %w", err):
				default:
				}
				return err
			}
			log.Debug().Msg("Sent ping to server")

		case <-pongWait:
			// Pong received, reset timeout ticker
			pongTimeoutTicker.Reset(pongTimeout)

		case <-pongTimeoutTicker.C:
			// Check if we're waiting for a pong response
			lastPing := atomic.LoadInt64(&w.lastPingTime)
			lastPong := atomic.LoadInt64(&w.lastPongTime)

			if lastPing > lastPong {
				log.Warn().
					Time("last_ping", time.Unix(0, lastPing)).
					Time("last_pong", time.Unix(0, lastPong)).
					Msg("No pong response within timeout, connection may be stale")
				return fmt.Errorf("pong timeout: no response within %v", pongTimeout)
			}

		case <-healthCheckTicker.C:
			if time.Since(lastDataReceived) > 60*time.Second {
				return fmt.Errorf("connection appears stale - no data for %v", time.Since(lastDataReceived))
			}

		default:
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					return err
				}
				return fmt.Errorf("read message failed: %w", err)
			}

			lastDataReceived = time.Now()

			// Get buffer from pool
			buf := messagePool.Get().([]byte)
			buf = buf[:0]
			buf = append(buf, msg...)

			// Track message pool usage
			if w.memStats != nil {
				w.memStats.TrackMessagePoolGet()
			}

			// Process message in worker pool
			select {
			case w.workerPool <- struct{}{}:
				go func() {
					if w.memStats != nil {
						w.memStats.TrackWorkerActive()
					}

					defer func() {
						<-w.workerPool
						if w.memStats != nil {
							w.memStats.TrackWorkerInactive()
							w.memStats.TrackMessagePoolPut()
						}
						messagePool.Put(buf)
					}()

					w.processMessage(buf, trades, depth, errors)
				}()
			default:
				// If worker pool is full, process in current goroutine
				w.processMessage(buf, trades, depth, errors)
				if w.memStats != nil {
					w.memStats.TrackMessagePoolPut()
				}
				messagePool.Put(buf)
			}
		}
	}
}

func (w *WS) processMessage(msg []byte, trades chan<- Trade, depth chan<- Depth, errors chan<- error) {
	// Track message processing
	if w.memStats != nil {
		w.memStats.TrackMessageProcessed(len(msg))
	}
	raw, err := parseMessage(msg)
	if err != nil {
		log.Debug().Err(err).Str("message", string(msg)).Msg("failed to parse message")
		return
	}

	// Handle subscription confirmations
	if op, ok := raw["op"].(string); ok && op == "subscribe" {
		if success, ok := raw["success"].(bool); ok && success {
			log.Info().Interface("response", raw).Msg("Successfully subscribed to WebSocket channels")
		}
		return
	}

	// Get sequence number
	seq, _ := raw["seq"].(float64)
	seqNum := int64(seq)

	// Check for sequence gaps
	symbol, _ := raw["symbol"].(string)
	lastSeq := w.seqTracker.get(symbol)

	if seqNum > 0 && lastSeq > 0 && seqNum != lastSeq+1 {
		log.Warn().
			Str("symbol", symbol).
			Int64("expected", lastSeq+1).
			Int64("received", seqNum).
			Msg("Sequence gap detected")
	}

	// Update last sequence
	w.seqTracker.set(symbol, seqNum)

	// Process data messages
	switch raw["ch"] {
	case "trade":
		if err := w.parseTrade(raw, trades, seqNum); err != nil {
			log.Debug().Err(err).Interface("raw_data", raw).Msg("Failed to parse trade")
			select {
			case errors <- fmt.Errorf("parse trade: %w", err):
			default:
			}
		}
	case "depth_books":
		if err := w.parseDepth(raw, depth, seqNum); err != nil {
			log.Debug().Err(err).Interface("raw_data", raw).Msg("Failed to parse depth")
			select {
			case errors <- fmt.Errorf("parse depth: %w", err):
			default:
			}
		}
	}
}

func (w *WS) parseTrade(m map[string]any, out chan<- Trade, seqNum int64) error {
	data, ok := m["data"].([]any)
	if !ok || len(data) == 0 {
		return fmt.Errorf("invalid trade data format")
	}

	d, ok := data[0].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid trade data item format")
	}

	symbol, ok := m["symbol"].(string)
	if !ok {
		return fmt.Errorf("missing symbol in trade")
	}

	// Validate symbol format
	if len(symbol) < 3 || len(symbol) > 20 {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}

	price, err := toFloat(d["p"])
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	// Validate price is positive
	if price <= 0 {
		return fmt.Errorf("invalid price value: %f", price)
	}

	qty, err := toFloat(d["v"])
	if err != nil {
		return fmt.Errorf("invalid volume: %w", err)
	}

	// Validate quantity is positive
	if qty <= 0 {
		return fmt.Errorf("invalid quantity value: %f", qty)
	}

	// Parse timestamp if available
	var timestamp time.Time
	if ts, ok := d["t"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = parsed
		}
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Get trade object from pool
	trade := tradePool.Get().(*Trade)
	if w.memStats != nil {
		w.memStats.TrackTradePoolGet()
	}
	trade.Symbol = symbol
	trade.Price = price
	trade.Qty = qty
	trade.Ts = timestamp
	trade.Seq = seqNum

	select {
	case out <- *trade:
		log.Debug().
			Str("symbol", symbol).
			Float64("price", price).
			Float64("qty", qty).
			Msg("Trade processed successfully")
	default:
		log.Warn().Str("symbol", symbol).Msg("trade channel full, dropping message")
		if w.memStats != nil {
			w.memStats.TrackMessageDropped()
		}
	}

	// Return trade object to pool
	tradePool.Put(trade)
	if w.memStats != nil {
		w.memStats.TrackTradePoolPut()
	}

	return nil
}

func (w *WS) parseDepth(m map[string]any, out chan<- Depth, seqNum int64) error {
	data, ok := m["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid depth data format")
	}

	symbol, ok := m["symbol"].(string)
	if !ok {
		return fmt.Errorf("missing symbol in depth")
	}

	// Validate symbol format
	if len(symbol) < 3 || len(symbol) > 20 {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}

	bids, ok := data["b"].([]any)
	if !ok || len(bids) == 0 {
		return fmt.Errorf("invalid bids format or empty bids")
	}

	asks, ok := data["a"].([]any)
	if !ok || len(asks) == 0 {
		return fmt.Errorf("invalid asks format or empty asks")
	}

	bid, ok := bids[0].([]any)
	if !ok || len(bid) < 2 {
		return fmt.Errorf("invalid bid entry")
	}

	ask, ok := asks[0].([]any)
	if !ok || len(ask) < 2 {
		return fmt.Errorf("invalid ask entry")
	}

	bidVol, err := toFloat(bid[1])
	if err != nil {
		return fmt.Errorf("invalid bid volume: %w", err)
	}

	askVol, err := toFloat(ask[1])
	if err != nil {
		return fmt.Errorf("invalid ask volume: %w", err)
	}

	bidPrice, err := toFloat(bid[0])
	if err != nil {
		return fmt.Errorf("invalid bid price: %w", err)
	}

	askPrice, err := toFloat(ask[0])
	if err != nil {
		return fmt.Errorf("invalid ask price: %w", err)
	}

	// Validate price and volume values
	if bidPrice <= 0 || askPrice <= 0 {
		return fmt.Errorf("invalid prices: bid=%f, ask=%f", bidPrice, askPrice)
	}

	if bidVol <= 0 || askVol <= 0 {
		return fmt.Errorf("invalid volumes: bid_vol=%f, ask_vol=%f", bidVol, askVol)
	}

	// Validate that ask price is higher than bid price
	if askPrice <= bidPrice {
		return fmt.Errorf("invalid spread: bid=%f >= ask=%f", bidPrice, askPrice)
	}

	// Use mid-price instead of bid price to avoid systematic bias
	midPrice := (bidPrice + askPrice) / 2

	// Parse timestamp if available
	var timestamp time.Time
	if ts, ok := data["ts"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = parsed
		}
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Get depth object from pool
	depth := depthPool.Get().(*Depth)
	if w.memStats != nil {
		w.memStats.TrackDepthPoolGet()
	}
	depth.Symbol = symbol
	depth.BidVol = bidVol
	depth.AskVol = askVol
	depth.LastPrice = midPrice
	depth.Ts = timestamp
	depth.Seq = seqNum

	select {
	case out <- *depth:
		log.Debug().
			Str("symbol", symbol).
			Float64("bid_price", bidPrice).
			Float64("ask_price", askPrice).
			Float64("mid_price", midPrice).
			Float64("bid_vol", bidVol).
			Float64("ask_vol", askVol).
			Msg("Depth processed successfully")
	default:
		log.Warn().Str("symbol", symbol).Msg("depth channel full, dropping message")
		if w.memStats != nil {
			w.memStats.TrackMessageDropped()
		}
	}

	// Return depth object to pool
	depthPool.Put(depth)
	if w.memStats != nil {
		w.memStats.TrackDepthPoolPut()
	}

	return nil
}

func toFloat(v any) (float64, error) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return 0, fmt.Errorf("empty string")
		}
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse string '%s' as float: %w", val, err)
		}
		// Check for NaN and Inf
		if f != f { // NaN check
			return 0, fmt.Errorf("parsed value is NaN")
		}
		if f == f+1 { // Inf check
			return 0, fmt.Errorf("parsed value is infinite")
		}
		return f, nil
	case float64:
		// Check for NaN and Inf
		if val != val { // NaN check
			return 0, fmt.Errorf("value is NaN")
		}
		if val == val+1 { // Inf check
			return 0, fmt.Errorf("value is infinite")
		}
		return val, nil
	case float32:
		f := float64(val)
		// Check for NaN and Inf
		if f != f { // NaN check
			return 0, fmt.Errorf("value is NaN")
		}
		if f == f+1 { // Inf check
			return 0, fmt.Errorf("value is infinite")
		}
		return f, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case int32:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("value type %T is not convertible to float", v)
	}
}
