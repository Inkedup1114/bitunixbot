package bitunix

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// ChangeLeverage updates the leverage for a symbol
func (c *Client) ChangeLeverage(symbol string, leverage int) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := ts

	sign := Sign(c.secret, nonce, c.key, ts)
	path := "/api/v1/futures/account/change_leverage"

	body := map[string]interface{}{
		"symbol":   symbol,
		"leverage": leverage,
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Symbol   string `json:"symbol"`
			Leverage int    `json:"leverage"`
		} `json:"data"`
	}

	_, err := c.rest.R().
		SetHeader("api-key", c.key).
		SetHeader("nonce", nonce).
		SetHeader("timestamp", ts).
		SetHeader("sign", sign).
		SetBody(body).
		SetResult(&resp).
		Post(c.base + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	// Handle non-fatal error codes with logging
	if resp.Code == 34002 {
		log.Warn().
			Str("symbol", symbol).
			Int("leverage", leverage).
			Int("code", resp.Code).
			Str("message", resp.Msg).
			Msg("Non-fatal error: leverage already set to requested value")
		return nil
	}

	if resp.Code == 10007 {
		log.Warn().
			Str("symbol", symbol).
			Int("leverage", leverage).
			Int("code", resp.Code).
			Str("message", resp.Msg).
			Msg("Non-fatal error: margin mode conflict")
		return nil
	}

	if resp.Code != 0 {
		return fmt.Errorf("bitunix: %d %s", resp.Code, resp.Msg)
	}

	log.Info().
		Str("symbol", symbol).
		Int("leverage", leverage).
		Msg("Leverage changed successfully")

	return nil
}

// ChangeMarginMode updates the margin mode for a symbol
func (c *Client) ChangeMarginMode(symbol string, marginMode string) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := ts

	sign := Sign(c.secret, nonce, c.key, ts)
	path := "/api/v1/futures/account/change_margin_mode"

	body := map[string]interface{}{
		"symbol":     symbol,
		"marginMode": marginMode,
	}

	// When setting ISOLATION mode, include marginCoin as USDT
	if marginMode == "ISOLATION" {
		body["marginCoin"] = "USDT"
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Symbol     string `json:"symbol"`
			MarginMode string `json:"marginMode"`
		} `json:"data"`
	}

	_, err := c.rest.R().
		SetHeader("api-key", c.key).
		SetHeader("nonce", nonce).
		SetHeader("timestamp", ts).
		SetHeader("sign", sign).
		SetBody(body).
		SetResult(&resp).
		Post(c.base + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	// Handle non-fatal error codes with logging
	if resp.Code == 34002 {
		log.Warn().
			Str("symbol", symbol).
			Str("marginMode", marginMode).
			Int("code", resp.Code).
			Str("message", resp.Msg).
			Msg("Non-fatal error: margin mode already set to requested value")
		return nil
	}

	if resp.Code == 10007 {
		log.Warn().
			Str("symbol", symbol).
			Str("marginMode", marginMode).
			Int("code", resp.Code).
			Str("message", resp.Msg).
			Msg("Non-fatal error: leverage/margin mode conflict")
		return nil
	}

	if resp.Code != 0 {
		return fmt.Errorf("bitunix: %d %s", resp.Code, resp.Msg)
	}

	log.Info().
		Str("symbol", symbol).
		Str("marginMode", marginMode).
		Msg("Margin mode changed successfully")

	return nil
}
