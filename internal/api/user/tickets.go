package user

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerTickets mounts /user/tickets*.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerTickets(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
