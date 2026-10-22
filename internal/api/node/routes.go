// Package node implements the Node APIs (docs/NODE_PROTOCOLS.md): XrayR/V2bX UniProxy under
// /api/v1/server/UniProxy/* and soga-v1 under both /api/v1/* and /soga/api/v1/*.
package node

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// SogaPrefix is the dedicated soga mount (recommended webapi_url=https://<panel>/soga/).
const SogaPrefix = "/soga" + httpx.APIPrefix

const ctxResolved = "node.resolved"

type handler struct {
	a     *app.App
	svc   *nodesvc.Service
	fails *httpx.Limiter // failed token checks per client IP (IPv6 per /64)
}

// Failed node-token checks allowed per client address: backends never fail (they all share server_token), so
// 30 failures (refilled at 60 per hour) only ever stop online guessing of the token.
const (
	authFailRate  = 60
	authFailBurst = 30
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
func Register(r gin.IRouter, a *app.App) {
	if e, ok := r.(*gin.Engine); ok {
		// docs/NODE_PROTOCOLS.md §1.1: tolerate soga's naive joining of webapi_url ("/soga//api/v1/node").
		e.RemoveExtraSlash = true
	}
	h := &handler{a: a, svc: nodesvc.New(a), fails: httpx.NewLimiter(httpx.Per(authFailRate, time.Hour), authFailBurst)}
	big := httpx.LargeBody(httpx.NodeBodyLimit)

	uni := r.Group(httpx.APIPrefix+"/server/UniProxy", h.uniAuth)
	uni.GET("/config", h.uniConfig)
	uni.GET("/user", h.uniUsers)
	uni.POST("/push", big, h.uniPush)
	uni.POST("/alive", big, h.uniAlive)
	uni.GET("/alivelist", h.uniAliveList)
	uni.POST("/status", big, h.uniStatus)

	for _, prefix := range []string{httpx.APIPrefix, SogaPrefix} {
		g := r.Group(prefix, h.sogaAuth)
		g.GET("/node", h.sogaNode)
		g.GET("/users", h.sogaUsers)
		g.GET("/audit_rules", h.sogaAuditRules)
		g.GET("/white_list", h.sogaWhiteList)
		g.POST("/traffic", big, h.sogaTraffic)
		g.POST("/alive_ip", big, h.sogaAlive)
		g.POST("/audit_log", big, h.sogaAuditLog)
		g.POST("/status", big, h.sogaStatus)
	}
}

// tokenOK compares the presented key with server_token in constant time; an unset token rejects everything.
func (h *handler) tokenOK(given string) bool {
	want := h.a.Settings.String("server_token")
	return want != "" && subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
}

// tokenCheck is tokenOK behind the per-IP failure limiter: limited is true (and the token is not even compared)
// once the client address used up its failures; every failed comparison is charged.
func (h *handler) tokenCheck(c *gin.Context, given string) (ok, limited bool) {
	key := httpx.KeyByIP(c)
	if h.fails.Exhausted(key) {
		return false, true
	}
	if h.tokenOK(given) {
		return true, false
	}
	h.fails.Allow(key)
	return false, false
}

// parseNodeID parses a node id (decimal uint64 > 0).
func parseNodeID(s string) (uint64, bool) { return nodesvc.ParseUserID(s) }

// touch records the authenticated request on P (LastCheckAt) and the backend's User-Agent as its version.
func (h *handler) touch(c *gin.Context, r *nodesvc.Resolved) {
	h.a.Online.Touch(r.P.ID)
	if ua := strings.TrimSpace(c.GetHeader("User-Agent")); ua != "" {
		if len(ua) > 64 {
			ua = ua[:64]
		}
		h.a.Online.SetVersion(r.P.ID, ua)
	}
	c.Set(ctxResolved, r)
}

func resolved(c *gin.Context) *nodesvc.Resolved {
	r, _ := c.MustGet(ctxResolved).(*nodesvc.Resolved)
	return r
}

// writeBody sends a cached body with its ETag, or 304 when If-None-Match contains the tag
// (docs/NODE_PROTOCOLS.md §6.6: substring compare accepts quoted, bare and W/ forms).
func writeBody(c *gin.Context, b *nodesvc.Body) {
	c.Header("ETag", `"`+b.ETag+`"`)
	if inm := c.GetHeader("If-None-Match"); inm != "" && strings.Contains(inm, b.ETag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", b.JSON)
}

// writeJSON sends v (an Object keeps its key order) without an ETag.
func writeJSON(c *gin.Context, v any) {
	b, err := nodesvc.NewBody(v)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", b.JSON)
}

// readBody reads the (size-limited) request body.
func readBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	return io.ReadAll(c.Request.Body)
}

func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
