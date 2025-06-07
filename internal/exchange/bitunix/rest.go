// Package bitunix provides REST and WebSocket client implementations for the Bitunix exchange.
// It includes order placement, market data retrieval, WebSocket streaming, and order tracking
// with timeout handling and metrics integration.
//
// The package provides both basic REST operations and advanced features like order tracking,
// connection pooling, and performance optimizations for high-frequency trading.
package bitunix

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client provides REST API access to the Bitunix exchange.
// It includes HTTP connection pooling, retry mechanisms, and optional
// order tracking with timeout handling for reliable order execution.
type Client struct {
	key, secret, base string        // API credentials and base URL
	rest              *resty.Client // HTTP client with optimizations
	orderTracker      *OrderTracker // Optional order tracking for timeouts
}

// NewREST creates a new REST client with optimized HTTP transport settings.
// It configures connection pooling, timeouts, and retry mechanisms for
// reliable API communication. Returns a client ready for trading operations.
func NewREST(key, secret, base string, timeout time.Duration) *Client {
	// Configure HTTP transport with connection pooling optimizations
	transport := &http.Transport{
		MaxIdleConns:        100,              // Max idle connections in total
		MaxIdleConnsPerHost: 10,               // Max idle connections per host
		IdleConnTimeout:     90 * time.Second, // Idle connection timeout
		DisableCompression:  false,            // Enable compression for bandwidth efficiency
		ForceAttemptHTTP2:   true,             // Use HTTP/2 if available for multiplexing
	}

	r := resty.New()
	r.SetTransport(transport)

	if timeout > 0 {
		r.SetTimeout(timeout)
	} else {
		r.SetTimeout(5 * time.Second) // default fallback
	}

	// Additional performance optimizations
	r.SetRetryCount(3)                     // Retry failed requests
	r.SetRetryWaitTime(1 * time.Second)    // Wait time between retries
	r.SetRetryMaxWaitTime(5 * time.Second) // Max wait time for retries
	r.EnableTrace()                        // Enable request tracing for performance monitoring

	client := &Client{
		key:          key,
		secret:       secret,
		base:         base,
		rest:         r,
		orderTracker: nil, // Will be initialized separately if needed
	}

	// Note: Order tracker will be initialized separately when needed
	// This allows backward compatibility

	return client
}

// NewRESTWithOrderTracking creates a REST client with order tracking enabled.
// Order tracking provides timeout handling, retry logic, and execution monitoring
// for reliable order placement in volatile market conditions.
func NewRESTWithOrderTracking(key, secret, base string, timeout, executionTimeout, statusCheckInterval time.Duration, maxRetries int) *Client {
	client := NewREST(key, secret, base, timeout)
	client.orderTracker = NewOrderTracker(client, executionTimeout, statusCheckInterval, maxRetries)
	return client
}

// NewRESTWithOrderTrackingAndMetrics creates a REST client with order tracking and metrics enabled.
// This is the most feature-complete client, providing order tracking, timeout handling,
// and comprehensive metrics collection for production trading systems.
func NewRESTWithOrderTrackingAndMetrics(key, secret, base string, timeout, executionTimeout, statusCheckInterval time.Duration, maxRetries int, metrics MetricsInterface) *Client {
	client := NewREST(key, secret, base, timeout)
	client.orderTracker = NewOrderTracker(client, executionTimeout, statusCheckInterval, maxRetries)
	if metrics != nil {
		client.orderTracker.SetMetrics(metrics)
	}
	return client
}

// GetOrderTracker returns the order tracker if available.
// Returns nil if the client was created without order tracking enabled.
func (c *Client) GetOrderTracker() *OrderTracker {
	return c.orderTracker
}

// PlaceWithTimeout places an order with timeout tracking if available.
// Falls back to regular order placement if no order tracker is configured.
// Provides automatic timeout handling and retry logic for reliable execution.
func (c *Client) PlaceWithTimeout(o OrderReq) error {
	if c.orderTracker == nil {
		// Fallback to regular placement if no tracker
		return c.Place(o)
	}

	return c.orderTracker.PlaceOrderWithTimeout(o)
}

// Close closes the client and stops any background processes.
// This should be called when the client is no longer needed to ensure
// proper cleanup of order tracking and other resources.
func (c *Client) Close() {
	if c.orderTracker != nil {
		c.orderTracker.Stop()
	}
}

// OrderReq represents an order request to the Bitunix exchange.
// It contains all necessary fields for placing different types of orders
// including market orders, stop-loss, and take-profit orders.
type OrderReq struct {
	Symbol    string `json:"symbol"`              // Trading symbol (e.g., "BTCUSDT")
	Side      string `json:"side"`                // Order side: "BUY" or "SELL"
	TradeSide string `json:"tradeSide"`           // Trade side: "OPEN" or "CLOSE"
	Qty       string `json:"qty"`                 // Order quantity
	OrderType string `json:"orderType"`           // Order type: "MARKET", "STOP_LOSS", "TAKE_PROFIT"
	StopPrice string `json:"stopPrice,omitempty"` // Stop price for stop-loss/take-profit orders
}

type orderResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (c *Client) Place(o OrderReq) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := ts // simple

	sign := Sign(c.secret, nonce, c.key, ts)
	path := "/api/v1/futures/trade/place_order"

	resp := &orderResp{}
	_, err := c.rest.R().
		SetHeader("api-key", c.key).
		SetHeader("nonce", nonce).
		SetHeader("timestamp", ts).
		SetHeader("sign", sign).
		SetBody(o).
		SetResult(resp).
		Post(c.base + path)
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("bitunix: %d %s", resp.Code, resp.Msg)
	}
	return nil
}

// KlineInterval represents kline/candlestick intervals
type KlineInterval string

const (
	Interval1m  KlineInterval = "1m"
	Interval5m  KlineInterval = "5m"
	Interval15m KlineInterval = "15m"
	Interval1h  KlineInterval = "1h"
	Interval4h  KlineInterval = "4h"
	Interval1d  KlineInterval = "1d"
)

// Kline represents a candlestick data point
type Kline struct {
	OpenTime  int64   `json:"openTime"`
	Open      float64 `json:"open,string"`
	High      float64 `json:"high,string"`
	Low       float64 `json:"low,string"`
	Close     float64 `json:"close,string"`
	Volume    float64 `json:"volume,string"`
	CloseTime int64   `json:"closeTime"`
}

// GetKlines fetches historical kline data
func (c *Client) GetKlines(symbol string, interval KlineInterval, startTime, endTime int64, limit int) ([]Kline, error) {
	path := "/api/v1/market/klines"

	params := map[string]string{
		"symbol":   symbol,
		"interval": string(interval),
		"limit":    strconv.Itoa(limit),
	}

	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}

	var klines []Kline
	resp, err := c.rest.R().
		SetQueryParams(params).
		SetResult(&klines).
		Get(c.base + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode(), resp.String())
	}

	return klines, nil
}

// GetTrades fetches recent trades
func (c *Client) GetTrades(symbol string, limit int) ([]Trade, error) {
	path := "/api/v1/market/trades"

	params := map[string]string{
		"symbol": symbol,
		"limit":  strconv.Itoa(limit),
	}

	var trades []Trade
	resp, err := c.rest.R().
		SetQueryParams(params).
		SetResult(&trades).
		Get(c.base + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode())
	}

	return trades, nil
}

// GetDepth fetches order book depth
func (c *Client) GetDepth(symbol string, limit int) (*Depth, error) {
	path := "/api/v1/market/depth"

	params := map[string]string{
		"symbol": symbol,
		"limit":  strconv.Itoa(limit),
	}

	var depthResp struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}

	resp, err := c.rest.R().
		SetQueryParams(params).
		SetResult(&depthResp).
		Get(c.base + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode())
	}

	// Convert to Depth struct
	bidVol := 0.0
	askVol := 0.0
	lastPrice := 0.0

	// Sum up bid volumes
	for _, bid := range depthResp.Bids {
		if len(bid) >= 2 {
			vol, _ := strconv.ParseFloat(bid[1], 64)
			bidVol += vol
			if lastPrice == 0 && len(bid) > 0 {
				lastPrice, _ = strconv.ParseFloat(bid[0], 64)
			}
		}
	}

	// Sum up ask volumes
	for _, ask := range depthResp.Asks {
		if len(ask) >= 2 {
			vol, _ := strconv.ParseFloat(ask[1], 64)
			askVol += vol
		}
	}

	return &Depth{
		Symbol:    symbol,
		BidVol:    bidVol,
		AskVol:    askVol,
		LastPrice: lastPrice,
		Ts:        time.Now(),
	}, nil
}
