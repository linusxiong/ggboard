// Package guest implements the public API: /api/v1/guest/*.
package guest

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix + "/guest")
	registerConfig(g, a)   // guest/config.go   (BE-A)
	registerPlans(g, a)    // guest/plans.go    (BE-C)
	registerPayment(g, a)  // guest/payment.go  (BE-C)
	registerRulesets(g, a) // guest/rulesets.go (BE-D)
}
