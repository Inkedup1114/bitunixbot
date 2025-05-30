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
