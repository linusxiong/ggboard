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

// Failed node-token checks allowed per client address (30, refilled at 60 per hour); further failures get 429.
// The token is always compared first and the right one is never refused: an address is not a principal (backends
// share it with anonymous clients behind NAT, a proxy without server.trusted_proxies, the Docker bridge or a shared
// IPv6 /64), so refusing it would let anyone stop node sync. Online guessing is defeated by the token's length
// (server_token is at least 16 random characters); the limiter only slows probing down.
const (
	authFailRate  = 60
	authFailBurst = 30
)

// Register mounts the package's routes on the root router r (paths include httpx.APIPrefix).
//
// soga joins webapi_url naively ("/soga//api/v1/node", docs/NODE_PROTOCOLS.md §1.1): httpx.NewEngine enables gin's
// RemoveExtraSlash for that, with trailing-slash redirects off (they echoed the raw "//host" path as Location).
func Register(r gin.IRouter, a *app.App) {
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

// tokenCheck is the first, node-independent check: the key must be server_token or some node's own token
// (docs/NODE_PROTOCOLS.md §1.1). The token is compared first (the right token is never limited, whatever its
// address did); a failed comparison is charged to the per-IP failure bucket and limited is true once that bucket
// is empty (429, like the subscription miss bucket in internal/api/client/subscribe.go).
func (h *handler) tokenCheck(c *gin.Context, given string) (ok, limited bool, err error) {
	if h.tokenOK(given) {
		return true, false, nil
	}
	if given != "" {
		idx, err := h.svc.NodeTokens(c.Request.Context())
		if err != nil {
			return false, false, err
		}
		if idx.Has(given) {
			return true, false, nil
		}
	}
	return false, h.miss(c), nil
}

// miss charges a failed check to the client's address; true once its bucket is empty.
func (h *handler) miss(c *gin.Context) bool { return !h.fails.Allow(httpx.KeyByIP(c)) }

// tokenFits is the second check, once the node is loaded: a node with its own token (a child: its parent's) accepts
// only that token, any other node only server_token. A key that passed tokenCheck as another node's token is
// refused here, so one node's key never reads another node's users or reports for them.
func (h *handler) tokenFits(r *nodesvc.Resolved, given string) bool {
	if want := r.P.Token; want != "" {
		return subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
	}
	return h.tokenOK(given)
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
// (docs/NODE_PROTOCOLS.md §6.6: substring compare accepts quoted, bare and W/ forms). A gzip response sends the
// body's cached encoding (Body.Gzip) instead of compressing it again.
func writeBody(c *gin.Context, b *nodesvc.Body) {
	c.Header("ETag", `"`+b.ETag+`"`)
	if inm := c.GetHeader("If-None-Match"); inm != "" && strings.Contains(inm, b.ETag) {
		c.Status(http.StatusNotModified)
		return
	}
	httpx.DataPrecompressed(c, http.StatusOK, "application/json; charset=utf-8", b.JSON, b.Gzip)
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
