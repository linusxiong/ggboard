package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
)

// LoginTokenTTL is the lifetime of one-time login tokens.
const LoginTokenTTL = 5 * time.Minute

const loginTokenPrefix = "auth:login_token:" //nolint:gosec // KV key prefix, not a credential

// IssueLoginToken creates a random one-time token (admin "login as") that ConsumeLoginToken exchanges for
// the account snapshot within 5 minutes. It is kept in a.KV (never evicted early).
func IssueLoginToken(a *app.App, u *model.User) string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	tok := hex.EncodeToString(b[:])
	a.KV.Set(loginTokenPrefix+tok, Claims{Email: u.Email, UUID: u.UUID, Ver: u.PasswordVersion,
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatUint(u.ID, 10)}}, LoginTokenTTL)
	return tok
}

// ConsumeLoginToken returns the account binding and invalidates token. Callers must check BoundTo against
// the current user before issuing a session; never replace the binding with the user's latest version.
func ConsumeLoginToken(a *app.App, token string) (*Claims, bool) {
	if token == "" {
		return nil, false
	}
	v, ok := a.KV.Take(loginTokenPrefix + token)
	if !ok {
		return nil, false
	}
	claims, ok := v.(Claims)
	return &claims, ok
}
