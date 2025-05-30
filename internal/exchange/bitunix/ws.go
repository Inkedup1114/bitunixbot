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
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := w.streamOnce(ctx, symbols, trades, depth, errors, ping); err != nil {
				log.Warn().Err(err).Msg("WebSocket connection failed, reconnecting...")
				select {
				case errors <- fmt.Errorf("ws reconnect: %w", err):
				default:
				}
				// Exponential backoff for reconnection
				time.Sleep(5 * time.Second)
				continue
			}
		}
	}
}

func (w WS) streamOnce(ctx context.Context, symbols []string, trades chan<- Trade, depth chan<- Depth, errors chan<- error, ping time.Duration) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.url, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Set close handler
	conn.SetCloseHandler(func(code int, text string) error {
		return fmt.Errorf("connection closed: %d %s", code, text)
	})

	// Set pong handler for keep-alive
	conn.SetPongHandler(func(string) error {
		return nil
	})

	// subscription payload
	var args []map[string]string
	for _, s := range symbols {
		args = append(args, map[string]string{"symbol": s, "ch": "trade"})
		args = append(args, map[string]string{"symbol": s, "ch": "depth_books"})
	}
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

	// read loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				select {
				case errors <- fmt.Errorf("ping failed: %w", err):
				default:
				}
				return err
			}
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read message failed: %w", err)
			}

			var raw map[string]any
			if err := json.Unmarshal(msg, &raw); err != nil {
				log.Debug().Err(err).Msg("failed to parse message")
				continue
			}

			switch raw["ch"] {
			case "trade":
				if err := parseTrade(raw, trades); err != nil {
					select {
					case errors <- fmt.Errorf("parse trade: %w", err):
					default:
					}
				}
			case "depth_books":
				if err := parseDepth(raw, depth); err != nil {
					select {
					case errors <- fmt.Errorf("parse depth: %w", err):
					default:
					}
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

	price, err := toFloat(d["price"])
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	qty, err := toFloat(d["size"])
	if err != nil {
		return fmt.Errorf("invalid size: %w", err)
	}

	trade := Trade{
		Symbol: symbol,
		Price:  price,
		Qty:    qty,
		Ts:     time.Now(),
	}

	select {
	case out <- trade:
	default:
		log.Warn().Msg("trade channel full, dropping message")
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

	bids, ok := data["bids"].([]any)
	if !ok || len(bids) == 0 {
		return fmt.Errorf("invalid bids format")
	}

	asks, ok := data["asks"].([]any)
	if !ok || len(asks) == 0 {
		return fmt.Errorf("invalid asks format")
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

	// Use mid-price instead of bid price to avoid systematic bias
	midPrice := (bidPrice + askPrice) / 2

	depth := Depth{
		Symbol:    symbol,
		BidVol:    bidVol,
		AskVol:    askVol,
		LastPrice: midPrice,
		Ts:        time.Now(),
	}

	select {
	case out <- depth:
	default:
		log.Warn().Msg("depth channel full, dropping message")
	}

	return nil
}

func toFloat(v any) (float64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("value is not a string")
	}
	return strconv.ParseFloat(s, 64)
}
