// Package user implements the authenticated user API: /api/v1/user/*.
package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix+"/user", auth.RequireUser(a))
	registerAccount(g, a)     // user/account.go      (BE-A)
	registerRuleProfile(g, a) // user/ruleprofile.go  (BE-D)
	registerNodes(g, a)       // user/nodes.go        (BE-B)
	registerShop(g, a)        // user/shop.go         (BE-C)
	registerInvite(g, a)      // user/invite.go       (BE-E2)
	registerTickets(g, a)     // user/tickets.go      (BE-E2)
	registerContent(g, a)     // user/content.go      (BE-E2)
	registerSecurity(g, a)    // user/security.go     (BE-A2)
}
