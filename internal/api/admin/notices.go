package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerNotices mounts /admin/notices* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerNotices(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
