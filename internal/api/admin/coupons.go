package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerCoupons mounts /admin/coupons* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerCoupons(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
