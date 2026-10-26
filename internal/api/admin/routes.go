// Package admin implements the Admin API (admin/staff JWT): /api/v1/admin/*.
// The group is mounted with auth.RequireStaff; every area that is admin-only must add auth.RequireAdmin(a)
// on its own sub-group (staff may only use tickets, notices, knowledge and read users).
package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix+"/admin", auth.RequireStaff(a), mfasvc.AdminGate(a))
	registerStats(g, a)     // admin/stats.go     (BE-E1)
	registerUsers(g, a)     // admin/users.go     (BE-E1)
	registerSettings(g, a)  // admin/settings.go  (BE-E1)
	registerSystem(g, a)    // admin/system.go    (BE-E1)
	registerPlans(g, a)     // admin/plans.go     (BE-C)
	registerOrders(g, a)    // admin/orders.go    (BE-C)
	registerCoupons(g, a)   // admin/coupons.go   (BE-C)
	registerPayments(g, a)  // admin/payments.go  (BE-C)
	registerGroups(g, a)    // admin/groups.go    (BE-B)
	registerNodes(g, a)     // admin/nodes.go     (BE-B)
	registerRoutes(g, a)    // admin/noderoutes.go (BE-B)
	registerTickets(g, a)   // admin/tickets.go   (BE-E2)
	registerNotices(g, a)   // admin/notices.go   (BE-E2)
	registerKnowledge(g, a) // admin/knowledge.go (BE-E2)
	registerRulesets(g, a)  // admin/rulesets.go  (BE-D)

	registerMailTemplates(g, a) // admin/mailtemplates.go (BE-MT)
	registerGeoIP(g, a)         // admin/geoip.go (IP locations)
}
