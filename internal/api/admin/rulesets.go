package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerRulesets mounts /admin/rulesets (admin only).
// Owner: BE-D subscription. Implement in this file; do not add routes elsewhere.
func registerRulesets(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
