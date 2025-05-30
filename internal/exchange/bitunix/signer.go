package bitunix

import (
	"crypto/sha256"
	"encoding/hex"
)

func Sign(secret, nonce, apiKey, ts string) string {
	h1 := sha256.Sum256([]byte(nonce + ts + apiKey))
	h2 := sha256.Sum256([]byte(hex.EncodeToString(h1[:]) + secret))
	return hex.EncodeToString(h2[:])
}
