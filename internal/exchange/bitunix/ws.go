package bitunix

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type Trade struct {
	Symbol string
	Price  float64
	Qty    float64
	Ts     time.Time
}

type Depth struct {
	Symbol         string
	BidVol, AskVol float64
	LastPrice      float64
	Ts             time.Time
}

type WS struct{ url string }

func NewWS(u string) WS { return WS{u} }

func (w WS) Stream(ctx context.Context, symbols []string, trades chan<- Trade, depth chan<- Depth, errors chan<- error, ping time.Duration) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := w.streamOnce(ctx, symbols, trades, depth, errors, ping); err != nil {
				log.Warn().Err(err).Dur("backoff", backoff).Msg("WebSocket connection failed, reconnecting with exponential backoff...")
				select {
				case errors <- fmt.Errorf("ws reconnect: %w", err):
				default:
				}

				// Exponential backoff for reconnection
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}

				// Double the backoff, up to maxBackoff
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			// Reset backoff on successful connection
			backoff = time.Second
		}
	}
}

func (w WS) streamOnce(ctx context.Context, symbols []string, trades chan<- Trade, depth chan<- Depth, errors chan<- error, ping time.Duration) error {
	log.Info().Str("url", w.url).Int("symbols_count", len(symbols)).Msg("Establishing WebSocket connection")

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.url, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer func() {
		conn.Close()
		log.Debug().Msg("WebSocket connection closed")
	}()

	// Set connection timeouts and limits
	conn.SetReadLimit(512 * 1024) // 512KB max message size
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Set close handler
	conn.SetCloseHandler(func(code int, text string) error {
		log.Warn().Int("code", code).Str("text", text).Msg("WebSocket connection closed by server")
		return fmt.Errorf("connection closed: %d %s", code, text)
	})

	// Set pong handler for keep-alive
	conn.SetPongHandler(func(appData string) error {
		log.Debug().Msg("Received pong from server")
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// subscription payload
	var args []map[string]string
	for _, s := range symbols {
		args = append(args, map[string]string{"symbol": s, "ch": "trade"})
		args = append(args, map[string]string{"symbol": s, "ch": "depth_books"})
	}

	log.Info().Interface("symbols", symbols).Msg("Subscribing to WebSocket channels")
	if err = conn.WriteJSON(map[string]any{"op": "subscribe", "args": args}); err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	// keep-alive ping ticker
	pingTicker := time.NewTicker(ping)
	defer pingTicker.Stop()

	// Send initial ping and set up periodic pings
	if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
		return fmt.Errorf("initial ping failed: %w", err)
	}
	log.Debug().Dur("interval", ping).Msg("Started WebSocket ping ticker")

	// Connection health monitoring
	lastDataReceived := time.Now()
	healthCheckTicker := time.NewTicker(30 * time.Second)
	defer healthCheckTicker.Stop()

	// read loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pingTicker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				select {
				case errors <- fmt.Errorf("ping failed: %w", err):
				default:
				}
				return err
			}
			log.Debug().Msg("Sent ping to server")
		case <-healthCheckTicker.C:
			// Check if we've received data recently
			if time.Since(lastDataReceived) > 60*time.Second {
				log.Warn().Time("last_data", lastDataReceived).Msg("No data received for 60 seconds, connection may be stale")
				return fmt.Errorf("connection appears stale - no data for %v", time.Since(lastDataReceived))
			}
		default:
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Info().Msg("WebSocket connection closed normally")
					return err
				}
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Error().Err(err).Msg("WebSocket connection closed unexpectedly")
				}
				return fmt.Errorf("read message failed: %w", err)
			}

			lastDataReceived = time.Now()

			var raw map[string]any
			if err := json.Unmarshal(msg, &raw); err != nil {
				log.Debug().Err(err).Str("message", string(msg)).Msg("failed to parse message")
				continue
			}

			// Log successful subscription confirmations
			if op, ok := raw["op"].(string); ok && op == "subscribe" {
				if success, ok := raw["success"].(bool); ok && success {
					log.Info().Interface("response", raw).Msg("Successfully subscribed to WebSocket channels")
				} else {
					log.Warn().Interface("response", raw).Msg("Subscription may have failed")
				}
				continue
			}

			// Handle data messages
			switch raw["ch"] {
			case "trade":
				if err := parseTrade(raw, trades); err != nil {
					log.Debug().Err(err).Interface("raw_data", raw).Msg("Failed to parse trade")
					select {
					case errors <- fmt.Errorf("parse trade: %w", err):
					default:
					}
				}
			case "depth_books":
				if err := parseDepth(raw, depth); err != nil {
					log.Debug().Err(err).Interface("raw_data", raw).Msg("Failed to parse depth")
					select {
					case errors <- fmt.Errorf("parse depth: %w", err):
					default:
					}
				}
			default:
				// Log unknown message types for debugging
				if ch, ok := raw["ch"].(string); ok && ch != "" {
					log.Debug().Str("channel", ch).Interface("data", raw).Msg("Received unknown channel message")
				}
			}
		}
	}
}

func parseTrade(m map[string]any, out chan<- Trade) error {
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

	trade := Trade{
		Symbol: symbol,
		Price:  price,
		Qty:    qty,
		Ts:     timestamp,
	}

	select {
	case out <- trade:
		log.Debug().
			Str("symbol", symbol).
			Float64("price", price).
			Float64("qty", qty).
			Msg("Trade processed successfully")
	default:
		log.Warn().Str("symbol", symbol).Msg("trade channel full, dropping message")
	}

	return nil
}

func parseDepth(m map[string]any, out chan<- Depth) error {
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

	depth := Depth{
		Symbol:    symbol,
		BidVol:    bidVol,
		AskVol:    askVol,
		LastPrice: midPrice,
		Ts:        timestamp,
	}

	select {
	case out <- depth:
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
