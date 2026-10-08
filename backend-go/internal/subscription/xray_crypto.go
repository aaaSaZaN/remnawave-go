package subscription

import (
	"crypto/mlkem"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/curve25519"
)

var (
	x25519Cache          sync.Map
	vlessEncryptionCache sync.Map
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

// DeriveVlessEncryption derives client VLESS encryption string from server decryption string (e.g. ML-KEM-768).
// Format: <proto>.<mode>.<rtt>[.<padding>].<public_key_1>[.<public_key_2>...]
func DeriveVlessEncryption(decryption string) (string, error) {
	if decryption == "" || decryption == "none" {
		return "none", nil
	}
	if val, ok := vlessEncryptionCache.Load(decryption); ok {
		return val.(string), nil
	}

	parts := strings.Split(decryption, ".")
	if len(parts) < 4 {
		return "none", fmt.Errorf("invalid decryption string format: %s", decryption)
	}

	proto := parts[0]
	mode := parts[1]
	ticketLifetime := parts[2]
	rest := parts[3:]

	var padding []string
	var keys []string
	foundKey := false

	for _, item := range rest {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			if !foundKey {
				padding = append(padding, item)
			}
			continue
		}

		rawKey := strings.TrimRight(trimmed, "=")
		b, err := base64.RawURLEncoding.DecodeString(rawKey)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(rawKey)
		}
		if err != nil {
			b, err = base64.StdEncoding.DecodeString(trimmed)
		}

		if err == nil && (len(b) == 32 || len(b) == 64) {
			foundKey = true
			keys = append(keys, trimmed)
		} else if foundKey {
			return "none", fmt.Errorf("invalid key length after keys started: %d", len(b))
		} else {
			padding = append(padding, item)
		}
	}

	if len(keys) == 0 {
		return "none", fmt.Errorf("no valid keys found in decryption string")
	}

	var pubKeys []string
	for _, k := range keys {
		rawKey := strings.TrimRight(strings.TrimSpace(k), "=")
		b, err := base64.RawURLEncoding.DecodeString(rawKey)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(rawKey)
		}
		if err != nil {
			b, err = base64.StdEncoding.DecodeString(k)
		}
		if err != nil {
			return "none", err
		}

		if len(b) == 32 {
			pub, err := curve25519.X25519(b, curve25519.Basepoint)
			if err != nil {
				return "none", err
			}
			pubKeys = append(pubKeys, base64.RawURLEncoding.EncodeToString(pub))
		} else if len(b) == 64 {
			dk, err := mlkem.NewDecapsulationKey768(b)
			if err != nil {
				return "none", err
			}
			pub := dk.EncapsulationKey().Bytes()
			pubKeys = append(pubKeys, base64.RawURLEncoding.EncodeToString(pub))
		}
	}

	pRTT := "0rtt"
	if ticketLifetime == "0s" {
		pRTT = "1rtt"
	}

	resParts := []string{proto, mode, pRTT}
	if len(padding) > 0 {
		resParts = append(resParts, strings.Join(padding, "."))
	}
	resParts = append(resParts, pubKeys...)

	result := strings.Join(resParts, ".")
	vlessEncryptionCache.Store(decryption, result)
	return result, nil
}
