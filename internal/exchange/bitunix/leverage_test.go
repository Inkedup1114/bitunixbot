package bitunix

import (
	"testing"
)

func TestChangeLeverage(t *testing.T) {
	client := NewTestClient()
	err := client.ChangeLeverage("BTCUSDT", 20)
	if err != nil {
		t.Errorf("ChangeLeverage failed: %v", err)
	}
}

func TestChangeMarginMode(t *testing.T) {
	client := NewTestClient()
	err := client.ChangeMarginMode("BTCUSDT", "ISOLATION")
	if err != nil {
		t.Errorf("ChangeMarginMode failed: %v", err)
	}
}
