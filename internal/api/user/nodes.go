package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerNodes mounts GET /user/nodes.
// Owner: BE-B nodes. Implement in this file; do not add routes elsewhere.
func registerNodes(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
