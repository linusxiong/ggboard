package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerShop mounts GET /user/plans, GET /user/plans/:id, GET /user/payment-methods, POST /user/coupons/check, /user/orders*.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerShop(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
