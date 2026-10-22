package guest

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerPayment mounts ANY /guest/payment/notify/:gateway/:uuid and GET /guest/payment/return/:trade_no.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPayment(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
