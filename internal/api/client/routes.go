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

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	lim := httpx.NewLimiter(httpx.Per(SubscribeRate, time.Hour), SubscribeBurst)
	byQuery := httpx.Limit("subscribe", lim, httpx.KeyByQuery("token"))
	byParam := httpx.Limit("subscribe", lim, func(c *gin.Context) string { return c.Param("token") })

	g := r.Group(httpx.APIPrefix + "/client")
	fromQuery := subscribe(a, func(c *gin.Context) string { return c.Query("token") }, false)
	g.GET("/subscribe", byQuery, fromQuery)
	g.HEAD("/subscribe", byQuery, fromQuery)

	fromParam := subscribe(a, func(c *gin.Context) string { return c.Param("token") }, true)
	r.GET("/s/:token", byParam, fromParam)
	r.HEAD("/s/:token", byParam, fromParam)
}
