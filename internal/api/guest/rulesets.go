package guest

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// registerRulesets mounts GET /guest/rulesets/catalog (the public rule-set catalog, docs/SUBSCRIPTION.md).
// Owner: BE-D subscription.
func registerRulesets(g *gin.RouterGroup, a *app.App) {
	g.GET("/rulesets/catalog", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=300")
		httpx.OK(c, rulesets.PublicView(rulesets.Current(a.Settings), a.Settings))
	})
}
