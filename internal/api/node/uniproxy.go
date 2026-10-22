package node

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/httpx"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// uniAuth authenticates XrayR/V2bX requests: query token, node_id, node_type (docs/NODE_PROTOCOLS.md §1.2).
func (h *handler) uniAuth(c *gin.Context) {
	if !h.tokenOK(c.Query("token")) {
		httpx.Fail(c, httpx.Forbidden("invalid token"))
		return
	}
	id, ok := parseNodeID(c.Query("node_id"))
	if !ok {
		httpx.Fail(c, httpx.BadRequest("node_id must be a positive integer"))
		return
	}
	typ, hy2, err := nodesvc.UniProxyType(c.Query("node_type"))
	if err != nil {
		httpx.Fail(c, httpx.BadRequest(err.Error()))
		return
	}
	r, err := h.svc.Resolve(c.Request.Context(), id)
	if errors.Is(err, nodesvc.ErrNotFound) {
		httpx.Fail(c, httpx.BadRequest("server is not exist"))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := nodesvc.CheckUniProxy(r.Node, typ, hy2); err != nil {
		httpx.Fail(c, httpx.BadRequest(err.Error()))
		return
	}
	h.touch(c, r)
	c.Next()
}

// GET /server/UniProxy/config
func (h *handler) uniConfig(c *gin.Context) {
	b, err := h.svc.UniProxyConfig(c.Request.Context(), resolved(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	writeBody(c, b)
}

// GET /server/UniProxy/user (X-Response-Format: msgpack is ignored: JSON only).
func (h *handler) uniUsers(c *gin.Context) {
	b, err := h.svc.UniProxyUsers(c.Request.Context(), resolved(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	writeBody(c, b)
}

// uniRead reads the body; size errors become 413 and read errors 400 with msg.
func uniRead(c *gin.Context, msg string) ([]byte, bool) {
	body, err := readBody(c)
	if err != nil {
		if isTooLarge(err) {
			httpx.Fail(c, err)
		} else {
			httpx.Fail(c, httpx.BadRequest(msg))
		}
		return nil, false
	}
	return body, true
}

var dataTrue = nodesvc.Object{{Key: "data", Value: true}}

// POST /server/UniProxy/push {"<uid>":[up,down]}
func (h *handler) uniPush(c *gin.Context) {
	const msg = "invalid data format"
	body, ok := uniRead(c, msg)
	if !ok {
		return
	}
	entries, err := nodesvc.ParseUniProxyPush(body)
	if err != nil {
		httpx.Fail(c, httpx.BadRequest(msg))
		return
	}
	h.svc.RecordTraffic(resolved(c), entries)
	writeJSON(c, dataTrue)
}

// POST /server/UniProxy/alive {"<uid>":["ip",...]}
func (h *handler) uniAlive(c *gin.Context) {
	const msg = "invalid online data"
	body, ok := uniRead(c, msg)
	if !ok {
		return
	}
	alive, err := nodesvc.ParseUniProxyAlive(body)
	if err != nil {
		httpx.Fail(c, httpx.BadRequest(msg))
		return
	}
	h.svc.RecordAlive(resolved(c), alive)
	writeJSON(c, dataTrue)
}

// GET /server/UniProxy/alivelist → {"alive":{"<uid>":count}} (no ETag).
func (h *handler) uniAliveList(c *gin.Context) {
	alive, err := h.svc.AliveList(c.Request.Context(), resolved(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	writeJSON(c, nodesvc.Object{{Key: "alive", Value: alive}})
}

// POST /server/UniProxy/status {"cpu","mem","swap","disk"}
func (h *handler) uniStatus(c *gin.Context) {
	body, ok := uniRead(c, "invalid status data")
	if !ok {
		return
	}
	load, err := nodesvc.ParseStatus(body, true)
	if err != nil {
		e := httpx.BadRequest("invalid status data")
		var se *nodesvc.StatusError
		if errors.As(err, &se) {
			e = e.WithField(se.Field, "is missing or invalid")
		}
		httpx.Fail(c, e)
		return
	}
	h.svc.RecordLoad(resolved(c), load)
	writeJSON(c, nodesvc.Object{{Key: "data", Value: true}, {Key: "code", Value: 0}, {Key: "message", Value: "success"}})
}
