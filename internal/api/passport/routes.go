// Package passport implements the Passport API (registration, login with optional second factor, passkey
// login, password reset): /api/v1/passport/*, plus the password-manager well-known URLs
// /.well-known/change-password and /.well-known/passkey-endpoints.
package passport

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Rate limits (per client IP). Per-email limits (code cooldown, password lockout, registrations per IP
// from settings) are enforced by internal/service/account.
const (
	passportPerMinute = 30 // every passport route
	sendCodePerHour   = 10 // POST /email/send-code
)

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
		sendCode(a))

	// Second factor and passkeys (BE-A2): passport/mfa.go, passport/passkey.go.
	g.POST("/login/mfa", loginMFA(a))
	g.POST("/passkey/login/begin", passkeyLoginBegin(a))
	g.POST("/passkey/login/finish", passkeyLoginFinish(a))

	// Password-manager well-known URLs (root paths, outside the SPA fallback).
	r.GET("/.well-known/change-password", changePasswordURL(a))
	r.GET("/.well-known/passkey-endpoints", passkeyEndpoints(a))
}
