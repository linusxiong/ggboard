package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerContent mounts GET /user/notices, GET /user/knowledge, GET /user/knowledge/:id.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerContent(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
