package subscription

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/curve25519"
)

var (
	x25519Cache sync.Map
)

// DeriveX25519PublicKey derives the raw URL-safe Base64 public key from a Base64 private key.
func DeriveX25519PublicKey(privateKeyBase64 string) (string, error) {
	if val, ok := x25519Cache.Load(privateKeyBase64); ok {
		return val.(string), nil
	}

	privKey := strings.TrimSpace(privateKeyBase64)
	var privBytes []byte
	var err error

	// Try standard base64 and URL base64
	privBytes, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(privKey, "="))
	if err != nil {
		privBytes, err = base64.StdEncoding.DecodeString(privKey)
	}
	if err != nil {
		privBytes, err = base64.URLEncoding.DecodeString(privKey)
	}
	if err != nil {
		return "", fmt.Errorf("failed to decode private key base64: %w", err)
	}

	if len(privBytes) != 32 {
		return "", fmt.Errorf("invalid x25519 private key length: %d (expected 32)", len(privBytes))
	}

	pubBytes, err := curve25519.X25519(privBytes, curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("failed to compute curve25519: %w", err)
	}

	// Node.js crypto JWK export uses unpadded URL-safe base64 (RFC 7515)
	pubKey := base64.RawURLEncoding.EncodeToString(pubBytes)
	x25519Cache.Store(privateKeyBase64, pubKey)
	return pubKey, nil
}
