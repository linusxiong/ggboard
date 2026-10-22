package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
)

// LoginTokenTTL is the lifetime of one-time login tokens.
const LoginTokenTTL = 5 * time.Minute

const loginTokenPrefix = "auth:login_token:" //nolint:gosec // KV key prefix, not a credential

// IssueLoginToken creates a random one-time token (admin "login as") that ConsumeLoginToken exchanges for
// userID within 5 minutes. It is kept in a.KV (never evicted early).
func IssueLoginToken(a *app.App, userID uint64) string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	tok := hex.EncodeToString(b[:])
	a.KV.Set(loginTokenPrefix+tok, userID, LoginTokenTTL)
	return tok
}

// ConsumeLoginToken returns the user id of token and invalidates it.
func ConsumeLoginToken(a *app.App, token string) (uint64, bool) {
	if token == "" {
		return 0, false
	}
	v, ok := a.KV.Take(loginTokenPrefix + token)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok
}
