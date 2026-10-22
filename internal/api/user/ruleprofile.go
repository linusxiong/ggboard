package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerRuleProfile mounts GET/PUT /user/rule-profile, GET /user/rulesets/catalog, GET /user/subscription/preview.
// Owner: BE-D subscription. Implement in this file; do not add routes elsewhere.
func registerRuleProfile(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
