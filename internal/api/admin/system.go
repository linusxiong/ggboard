package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerSystem mounts /admin/system/* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerSystem(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
