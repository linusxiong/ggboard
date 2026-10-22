package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerAccount mounts GET/PATCH /user/me, POST /user/password, GET /user/stats, GET /user/subscription, POST /user/subscription/reset, GET /user/traffic-logs, POST /user/logout.
// Owner: BE-A auth/account. Implement in this file; do not add routes elsewhere.
func registerAccount(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
