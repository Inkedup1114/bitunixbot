package bitunix

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

func (cl *Client) ChangeLeverage(symbol string, leverage int) error {
	payload := map[string]interface{}{
		"symbol":   symbol,
		"leverage": leverage,
	}
	resp, err := cl.doRequest("POST", "/api/v1/futures/account/change_leverage", payload)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to change leverage")
		return err
	}
	if respErr := respHasError(resp); respErr != nil {
		log.Warn().Err(respErr).Msg("Non-fatal error changing leverage")
		return respErr
	}
	return nil
}

func (cl *Client) ChangeMarginMode(sym, mode string) error {
	payload := map[string]string{
		"symbol":     sym,
		"marginMode": mode,
	}
	resp, err := cl.doRequest("POST", "/api/v1/futures/account/change_margin_mode", payload)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to change margin mode")
		return err
	}
	if respErr := respHasError(resp); respErr != nil {
		log.Warn().Err(respErr).Msg("Non-fatal error changing margin mode")
		return respErr
	}
	return nil
}

func respHasError(resp *Response) error {
	if resp.Code != 0 {
		return fmt.Errorf("API error: %s", resp.Msg)
	}
	return nil
}
