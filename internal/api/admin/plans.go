package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerPlans mounts /admin/plans* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPlans(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
