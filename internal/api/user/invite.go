package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerInvite mounts GET /user/invite, POST /user/invite/codes, GET /user/invite/commissions, POST /user/commission/transfer, POST /user/commission/withdraw.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerInvite(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
