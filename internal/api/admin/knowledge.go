package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerKnowledge mounts /admin/knowledge* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerKnowledge(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
