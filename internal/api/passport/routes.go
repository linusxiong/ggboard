// Package passport implements the Passport API (registration, login with optional second factor, passkey
// login, password reset): /api/v1/passport/*, plus the password-manager well-known URLs
// /.well-known/change-password and /.well-known/passkey-endpoints.
package passport

import (
	"net/netip"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Rate limits (per client IP). Per-email limits (code cooldown, password lockout, registrations per IP
// from settings) are enforced by internal/service/account.
const (
	passportPerMinute = 30 // every passport route
	sendCodePerHour   = 10 // POST /email/send-code, per IPv4 address or IPv6 /64
	// sendCodeSitePerHour is the POST /email/send-code budget of a whole IPv6 /48: one holder (a tunnel broker
	// hands out free /48s) would otherwise get 65 536 per-/64 budgets.
	sendCodeSitePerHour = 60
)

// keyByIPv6Site keys IPv6 clients by their /48 and skips IPv4 clients ("" is never limited).
func keyByIPv6Site(c *gin.Context) string {
	addr, err := netip.ParseAddr(c.ClientIP())
	if err != nil || addr.Unmap().Is4() {
		return ""
	}
	p, err := addr.WithZone("").Prefix(48)
	if err != nil {
		return ""
	}
	return p.String()
}

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix+"/passport",
		httpx.RateLimit("passport", httpx.Per(passportPerMinute, time.Minute), passportPerMinute, httpx.KeyByIP))
	g.POST("/register", register(a))
	g.POST("/login", login(a))
	g.POST("/login/token", loginToken(a))
	g.POST("/forgot", forgot(a))
	g.POST("/email/send-code",
		httpx.RateLimit("passport_send_code", httpx.Per(sendCodePerHour, time.Hour), sendCodePerHour, httpx.KeyByIP),
		httpx.RateLimit("passport_send_code_v6_48", httpx.Per(sendCodeSitePerHour, time.Hour), sendCodeSitePerHour, keyByIPv6Site),
		sendCode(a))

	// Second factor and passkeys (BE-A2): passport/mfa.go, passport/passkey.go.
	g.POST("/login/mfa", loginMFA(a))
	g.POST("/passkey/login/begin", passkeyLoginBegin(a))
	g.POST("/passkey/login/finish", passkeyLoginFinish(a))

	// Password-manager well-known URLs (root paths, outside the SPA fallback).
	r.GET("/.well-known/change-password", changePasswordURL(a))
	r.GET("/.well-known/passkey-endpoints", passkeyEndpoints(a))
}
