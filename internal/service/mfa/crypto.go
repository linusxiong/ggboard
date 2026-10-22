package mfasvc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// TOTP secrets are encrypted at rest with AES-256-GCM. The key is derived from the JWT secret with HKDF-SHA256
// (so rotating jwt.secret also invalidates enrolled authenticators); the user id is bound as additional data so
// a ciphertext cannot be moved to another user's row. Stored form: "v1." + base64url(nonce || ciphertext).
const (
	secretKeyInfo = "ggboard/mfa/totp-secret/v1" //nolint:gosec // HKDF info label, not a credential
	secretPrefix  = "v1."
)

var errSecretFormat = errors.New("mfa: malformed encrypted secret")

// deriveKey returns the 32-byte AES key for TOTP secrets derived from jwtSecret.
func deriveKey(jwtSecret string) ([]byte, error) {
	if jwtSecret == "" {
		return nil, errors.New("mfa: empty jwt secret")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(jwtSecret), nil, []byte(secretKeyInfo)), key); err != nil {
		return nil, fmt.Errorf("mfa: derive key: %w", err)
	}
	return key, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func secretAAD(userID uint64) []byte { return []byte("ggboard:totp:" + strconv.FormatUint(userID, 10)) }

// encryptSecret seals plain for userID.
func encryptSecret(key []byte, userID uint64, plain string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", fmt.Errorf("mfa: cipher: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("mfa: nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), secretAAD(userID))
	return secretPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// decryptSecret opens a value produced by encryptSecret for the same userID and key.
func decryptSecret(key []byte, userID uint64, enc string) (string, error) {
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
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], secretAAD(userID))
	if err != nil {
		return "", fmt.Errorf("mfa: decrypt secret: %w", err)
	}
	return string(plain), nil
}

func (s *Service) secretKey() ([]byte, error) { return deriveKey(s.a.Cfg.JWT.Secret) }
