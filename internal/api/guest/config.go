package guest

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerConfig mounts GET /guest/config.
// Owner: BE-A auth/account. Implement in this file; do not add routes elsewhere.
func registerConfig(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
