package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerSettings mounts /admin/settings* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerSettings(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
