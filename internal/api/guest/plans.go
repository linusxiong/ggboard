package guest

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
)

// registerPlans mounts GET /guest/plans.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPlans(g *gin.RouterGroup, a *app.App) {
	g.GET("/plans", func(c *gin.Context) {
		// Plans with show && sell that still have capacity; capacity figures are never exposed.
		plans, err := plansvc.ListForGuest(c.Request.Context(), a.DB, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, plans, int64(len(plans)))
	})
}
