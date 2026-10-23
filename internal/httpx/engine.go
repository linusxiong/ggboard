package httpx

import (
	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
)

// RegisterFunc mounts routes on the root router. Every API package exports one named Register.
type RegisterFunc func(r gin.IRouter, a *app.App)

// NewEngine returns a gin engine (release mode) with the standard middleware chain and JSON 404/405
// handlers, but without routes:
//
//	request id -> access log -> security headers -> CORS (if configured) -> gzip -> recovery -> body limit
func NewEngine(a *app.App) *gin.Engine {
	setup()
	e := gin.New()
	e.HandleMethodNotAllowed = false
	e.ContextWithFallback = true
	// Route on the cleaned path so "/soga//api/v1/node" (soga joins webapi_url naively, docs/NODE_PROTOCOLS.md §1.1)
	// and "https://panel.example.com//api/v1/..." still match.
	e.RemoveExtraSlash = true
	// Never redirect to a "fixed" path: gin builds the Location from the raw request path, so with RemoveExtraSlash
	// "GET //evil.example%2f..%2fhealthz/" was answered with "301 Location: //evil.example/../healthz" (an open
	// redirect). Unmatched paths go to NoRoute (JSON 404, or the SPA) instead.
	e.RedirectTrailingSlash = false
	e.RedirectFixedPath = false
	if err := e.SetTrustedProxies(a.Cfg.Server.TrustedProxies); err != nil {
		a.Log.Error("invalid server.trusted_proxies; trusting none", "err", err)
		_ = e.SetTrustedProxies(nil)
	}
	e.Use(RequestID(a.Log), AccessLog(), SecurityHeaders(), trustedPeer(parsePrefixes(a.Cfg.Server.TrustedProxies)))
	if mw := CORS(a.Cfg.Server.CORSOrigins); mw != nil {
		e.Use(mw)
	}
	e.Use(Gzip(), Recovery(), BodyLimit(DefaultBodyLimit))
	e.NoRoute(func(c *gin.Context) { Fail(c, NotFound("route not found")) })
	return e
}
