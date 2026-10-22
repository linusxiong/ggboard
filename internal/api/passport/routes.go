// Package passport implements the Passport API (registration, login, password reset): /api/v1/passport/*.
package passport

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix + "/passport")
	g.GET("/_health", func(c *gin.Context) {
		httpx.OK(c, gin.H{"package": "passport", "version": a.Version})
	})
}
