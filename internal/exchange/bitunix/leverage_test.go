package bitunix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// NewTestClient creates a test client with a mock server
func NewTestClient() *Client {
	return NewREST("test-key", "test-secret", "http://localhost:8080", 5*time.Second)
}

// NewTestClientWithServer creates a test client with a custom mock server
func NewTestClientWithServer(server *httptest.Server) *Client {
	return NewREST("test-key", "test-secret", server.URL, 5*time.Second)
}

func TestChangeLeverage(t *testing.T) {
	tests := []struct {
		name        string
		symbol      string
		leverage    int
		serverResp  interface{}
		expectError bool
		expectLog   string
	}{
		{
			name:     "success",
			symbol:   "BTCUSDT",
			leverage: 20,
			serverResp: map[string]interface{}{
				"code": 0,
				"msg":  "success",
				"data": map[string]interface{}{
					"symbol":   "BTCUSDT",
					"leverage": 20,
				},
			},
			expectError: false,
		},
		{
			name:     "already_set_34002",
			symbol:   "BTCUSDT",
			leverage: 20,
			serverResp: map[string]interface{}{
				"code": 34002,
				"msg":  "Leverage already set to this value",
			},
			expectError: false,
			expectLog:   "Non-fatal error: leverage already set to requested value",
		},
		{
			name:     "margin_conflict_10007",
			symbol:   "BTCUSDT",
			leverage: 20,
			serverResp: map[string]interface{}{
				"code": 10007,
				"msg":  "Margin mode conflict",
			},
			expectError: false,
			expectLog:   "Non-fatal error: margin mode conflict",
		},
		{
			name:     "other_error",
			symbol:   "BTCUSDT",
			leverage: 20,
			serverResp: map[string]interface{}{
				"code": 1001,
				"msg":  "Invalid parameter",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/futures/account/change_leverage" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.serverResp)
			}))
			defer server.Close()

			client := NewTestClientWithServer(server)
			err := client.ChangeLeverage(tt.symbol, tt.leverage)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestChangeMarginMode(t *testing.T) {
	tests := []struct {
		name            string
		symbol          string
		marginMode      string
		serverResp      interface{}
		expectError     bool
		expectLog       string
		checkMarginCoin bool
	}{
		{
			name:       "success_isolation",
			symbol:     "BTCUSDT",
			marginMode: "ISOLATION",
			serverResp: map[string]interface{}{
				"code": 0,
				"msg":  "success",
				"data": map[string]interface{}{
					"symbol":     "BTCUSDT",
					"marginMode": "ISOLATION",
				},
			},
			expectError:     false,
			checkMarginCoin: true,
		},
		{
			name:       "success_cross",
			symbol:     "BTCUSDT",
			marginMode: "CROSS",
			serverResp: map[string]interface{}{
				"code": 0,
				"msg":  "success",
				"data": map[string]interface{}{
					"symbol":     "BTCUSDT",
					"marginMode": "CROSS",
				},
			},
			expectError:     false,
			checkMarginCoin: false,
		},
		{
			name:       "already_set_34002",
			symbol:     "BTCUSDT",
			marginMode: "ISOLATION",
			serverResp: map[string]interface{}{
				"code": 34002,
				"msg":  "Margin mode already set to this value",
			},
			expectError: false,
			expectLog:   "Non-fatal error: margin mode already set to requested value",
		},
		{
			name:       "leverage_conflict_10007",
			symbol:     "BTCUSDT",
			marginMode: "ISOLATION",
			serverResp: map[string]interface{}{
				"code": 10007,
				"msg":  "Leverage/margin mode conflict",
			},
			expectError: false,
			expectLog:   "Non-fatal error: leverage/margin mode conflict",
		},
		{
			name:       "other_error",
			symbol:     "BTCUSDT",
			marginMode: "ISOLATION",
			serverResp: map[string]interface{}{
				"code": 1001,
				"msg":  "Invalid parameter",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/futures/account/change_margin_mode" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				// Check if marginCoin is included for ISOLATION mode
				if tt.checkMarginCoin {
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if marginCoin, ok := body["marginCoin"]; !ok || marginCoin != "USDT" {
						t.Errorf("expected marginCoin=USDT for ISOLATION mode, got %v", marginCoin)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.serverResp)
			}))
			defer server.Close()

			client := NewTestClientWithServer(server)
			err := client.ChangeMarginMode(tt.symbol, tt.marginMode)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
