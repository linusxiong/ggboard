package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerStats mounts /admin/stats/* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerStats(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
