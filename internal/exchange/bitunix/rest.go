package bitunix

import (
	"fmt"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	key, secret, base string
	rest              *resty.Client
}

func NewREST(key, secret, base string, timeout time.Duration) *Client {
	r := resty.New()
	if timeout > 0 {
		r.SetTimeout(timeout)
	} else {
		r.SetTimeout(5 * time.Second) // default fallback
	}
	return &Client{key, secret, base, r}
}

type OrderReq struct {
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`      // BUY or SELL
	TradeSide string `json:"tradeSide"` // OPEN or CLOSE
	Qty       string `json:"qty"`
	OrderType string `json:"orderType"` // MARKET
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
