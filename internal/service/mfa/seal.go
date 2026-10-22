package mfasvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// SealString and OpenString expose the TOTP-secret scheme (crypto.go) to other packages that keep a secret the
// owner must be able to read again (for example the age identity of encrypted subscriptions): AES-256-GCM with
// a key derived from jwtSecret by HKDF-SHA256 under info (one label per use, so keys never repeat across
// features), aad bound as additional data (bind the row owner so ciphertexts cannot be moved between rows).
// Stored form: "v1." + base64url(nonce || ciphertext). Rotating jwt.secret makes existing values unreadable.

// SealString encrypts plain.
func SealString(jwtSecret, info string, aad []byte, plain string) (string, error) {
	key, err := deriveKeyInfo(jwtSecret, info)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", fmt.Errorf("mfa: cipher: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("mfa: nonce: %w", err)
	}
	return secretPrefix + base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), aad)), nil
}

// OpenString decrypts a value produced by SealString with the same jwtSecret, info and aad.
func OpenString(jwtSecret, info string, aad []byte, enc string) (string, error) {
	key, err := deriveKeyInfo(jwtSecret, info)
	if err != nil {
		return "", err
	}
	raw, ok := strings.CutPrefix(enc, secretPrefix)
	if !ok {
		return "", errSecretFormat
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", errSecretFormat
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", fmt.Errorf("mfa: cipher: %w", err)
	}
	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return "", errSecretFormat
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], aad)
	if err != nil {
		return "", fmt.Errorf("mfa: open sealed value: %w", err)
	}
	return string(plain), nil
}

func deriveKeyInfo(jwtSecret, info string) ([]byte, error) {
	if jwtSecret == "" {
		return nil, errors.New("mfa: empty jwt secret")
	}
	if info == "" || info == secretKeyInfo {
		return nil, errors.New("mfa: SealString needs its own HKDF info label")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(jwtSecret), nil, []byte(info)), key); err != nil {
		return nil, fmt.Errorf("mfa: derive key: %w", err)
	}
	return key, nil
}
