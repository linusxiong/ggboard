// Package client implements the Client subscription API (token): /api/v1/client/* and /s/:token.
package client

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix + "/client")
	g.GET("/_health", func(c *gin.Context) {
		httpx.OK(c, gin.H{"package": "client", "version": a.Version})
	})
}
