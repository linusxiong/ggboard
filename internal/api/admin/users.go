package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerUsers mounts /admin/users* (GET list/detail allowed for staff, everything else admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerUsers(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
