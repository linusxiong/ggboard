// Package node implements the Node APIs (XrayR/V2bX UniProxy under /api/v1/server/*, soga-v1 under /api/v1/* and /soga/api/v1/*).
package node

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix + "/server")
	g.GET("/_health", func(c *gin.Context) {
		httpx.OK(c, gin.H{"package": "node", "version": a.Version})
	})
}
