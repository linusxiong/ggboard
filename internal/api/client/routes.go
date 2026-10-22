// Package client implements the Client subscription API (token): GET /api/v1/client/subscribe?token= and the
// short link GET /s/:token (docs/API.md "Client subscription", docs/SUBSCRIPTION.md).
package client

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"

	// Register every subscription format.
	_ "github.com/linusxiong/ggboard/internal/subscription/formats"
)

// Rate limit per subscription token, shared by both routes: clients refresh every few hours, so 120 requests
// per hour with a burst of 30 leaves room for many devices while stopping abuse of a leaked link.
const (
	SubscribeRate  = 120
	SubscribeBurst = 30
)

// Unknown tokens are limited per client IP (IPv6 per /64): each costs a database lookup, and keying the token
// bucket by attacker-chosen strings would let anyone grow the limiter without bound. 60 misses per hour (burst
// 30) is far above what a client with a reset link produces.
const (
	MissRate  = 60
	MissBurst = 30
)

// limits are the rate limiters shared by both subscription routes.
type limits struct {
	token *httpx.Limiter // per existing token
	miss  *httpx.Limiter // per IP, charged for unknown or malformed tokens
}

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	lim := &limits{
		token: httpx.NewLimiter(httpx.Per(SubscribeRate, time.Hour), SubscribeBurst),
		miss:  httpx.NewLimiter(httpx.Per(MissRate, time.Hour), MissBurst),
	}

	g := r.Group(httpx.APIPrefix + "/client")
	fromQuery := subscribe(a, lim, func(c *gin.Context) string { return c.Query("token") }, false)
	g.GET("/subscribe", fromQuery)
	g.HEAD("/subscribe", fromQuery)

	fromParam := subscribe(a, lim, func(c *gin.Context) string { return c.Param("token") }, true)
	r.GET("/s/:token", fromParam)
	r.HEAD("/s/:token", fromParam)
}
