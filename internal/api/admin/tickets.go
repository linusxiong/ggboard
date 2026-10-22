package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// registerTickets mounts /admin/tickets* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerTickets(g *gin.RouterGroup, a *app.App) {
	_, _ = g, a
}
