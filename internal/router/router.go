// Package router assembles the HTTP engine: middleware (httpx.NewEngine), every API package and the
// embedded SPA as the fallback handler.
package router

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/api/node"
	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/static"
)

// Packages lists the API packages in registration order. Each Register receives the root router and mounts
// its own paths under httpx.APIPrefix (client and node also mount /s/:token and /soga/api/v1).
var Packages = []httpx.RegisterFunc{
	guest.Register,
	passport.Register,
	user.Register,
	admin.Register,
	client.Register,
	node.Register,
}

// New returns the fully wired engine.
func New(a *app.App) (*gin.Engine, error) {
	e := httpx.NewEngine(a)
	e.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok", "version": a.Version, "uptime": time.Since(a.StartedAt).Round(time.Second).String()})
	})
	for _, register := range Packages {
		register(e, a)
	}
	if err := static.Mount(e, a); err != nil {
		return nil, err
	}
	return e, nil
}
