package guest

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
)

// Payment notifications that fail verification (unknown payment, wrong gateway, bad signature) allowed per client
// address (IPv6 per /64): 120, refilled at 60 per minute. Once they are used up the address gets 429 before any
// lookup. Verified callbacks are never charged, so a gateway's own retries are not limited; only a gateway sharing
// its address with a prober (behind a proxy missing from server.trusted_proxies) can see a 429, which it retries.
const (
	NotifyMissRate  = 60
	NotifyMissBurst = 120
)

// registerPayment mounts ANY /guest/payment/notify/:gateway/:uuid and GET /guest/payment/return/:trade_no.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPayment(g *gin.RouterGroup, a *app.App) {
	misses := httpx.NewLimiter(httpx.Per(NotifyMissRate, time.Minute), NotifyMissBurst)
	g.Any("/payment/notify/:gateway/:uuid", func(c *gin.Context) {
		key := httpx.KeyByIP(c)
		if misses.Exhausted(key) {
			misses.Deny(c, "payment_notify_miss", key)
			return
		}
		resp, verified := ordersvc.New(a).HandleNotifyVerified(c.Request.Context(), c.Param("gateway"), c.Param("uuid"), c.Request)
		if !verified && resp.Status < http.StatusInternalServerError {
			misses.Allow(key)
		}
		ct := resp.ContentType
		if ct == "" {
			ct = "text/plain; charset=utf-8"
		}
		c.Data(resp.Status, ct, []byte(resp.Body))
	})
	// The gateway sends the buyer here (with its own query parameters, which are dropped); the SPA order page
	// polls the order status.
	g.GET("/payment/return/:trade_no", func(c *gin.Context) {
		tradeNo := strings.TrimSpace(c.Param("trade_no"))
		c.Redirect(http.StatusFound, ordersvc.New(a).OrderPageURL(url.PathEscape(tradeNo), ""))
	})
}
