package guest

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
)

// registerPayment mounts ANY /guest/payment/notify/:gateway/:uuid and GET /guest/payment/return/:trade_no.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPayment(g *gin.RouterGroup, a *app.App) {
	g.Any("/payment/notify/:gateway/:uuid", func(c *gin.Context) {
		resp := ordersvc.New(a).HandleNotify(c.Request.Context(), c.Param("gateway"), c.Param("uuid"), c.Request)
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
