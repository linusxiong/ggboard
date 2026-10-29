package node

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/httpx"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// sogaFail aborts with soga's error shape: an int code equal to the HTTP status (docs/NODE_PROTOCOLS.md §1.3).
func sogaFail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, nodesvc.Object{{Key: "code", Value: status}, {Key: "message", Value: msg}})
}

// sogaError renders an unexpected error (logged) as a soga error.
func sogaError(c *gin.Context, err error) {
	e := httpx.AsError(err)
	if e.Status >= http.StatusInternalServerError {
		httpx.Logger(c).Error("node request failed", "path", c.Request.URL.Path, "err", err)
	}
	sogaFail(c, e.Status, e.Message)
}

// sogaAuth authenticates soga requests: headers API-KEY, NODE-ID, NODE-TYPE (docs/NODE_PROTOCOLS.md §1.3), with the
// same two token checks as uniAuth.
func (h *handler) sogaAuth(c *gin.Context) {
	key := c.GetHeader("API-KEY")
	invalid := func() {
		if h.miss(c) {
			sogaFail(c, http.StatusTooManyRequests, "too many invalid API-KEY attempts, retry later")
		} else {
			sogaFail(c, http.StatusForbidden, "invalid API-KEY")
		}
	}
	if ok, limited, err := h.tokenCheck(c, key); err != nil {
		sogaError(c, err)
		return
	} else if limited {
		sogaFail(c, http.StatusTooManyRequests, "too many invalid API-KEY attempts, retry later")
		return
	} else if !ok {
		sogaFail(c, http.StatusForbidden, "invalid API-KEY")
		return
	}
	id, ok := parseNodeID(c.GetHeader("NODE-ID"))
	if !ok {
		sogaFail(c, http.StatusBadRequest, "NODE-ID must be a positive integer")
		return
	}
	typ, hy2, err := nodesvc.SogaType(c.GetHeader("NODE-TYPE"))
	if err != nil {
		sogaFail(c, http.StatusBadRequest, err.Error())
		return
	}
	r, err := h.svc.Resolve(c.Request.Context(), id)
	if errors.Is(err, nodesvc.ErrNotFound) {
		if !h.tokenOK(key) { // a node's own key does not learn which other node ids exist
			invalid()
			return
		}
		sogaFail(c, http.StatusBadRequest, "node not found")
		return
	}
	if err != nil {
		sogaError(c, err)
		return
	}
	if !h.tokenFits(r, key) {
		invalid()
		return
	}
	if err := nodesvc.CheckSoga(r.Node, typ, hy2); err != nil {
		sogaFail(c, http.StatusBadRequest, err.Error())
		return
	}
	h.touch(c, r)
	c.Next()
}

func (h *handler) sogaBody(c *gin.Context, b *nodesvc.Body, err error) {
	if err != nil {
		sogaError(c, err)
		return
	}
	writeBody(c, b)
}

// GET /node
func (h *handler) sogaNode(c *gin.Context) {
	b, err := h.svc.SogaNode(c.Request.Context(), resolved(c))
	h.sogaBody(c, b, err)
}

// GET /users
func (h *handler) sogaUsers(c *gin.Context) {
	b, err := h.svc.SogaUsers(c.Request.Context(), resolved(c))
	h.sogaBody(c, b, err)
}

// GET /audit_rules
func (h *handler) sogaAuditRules(c *gin.Context) {
	b, err := h.svc.AuditRules(c.Request.Context(), resolved(c))
	h.sogaBody(c, b, err)
}

// GET /white_list
func (h *handler) sogaWhiteList(c *gin.Context) {
	b, err := h.svc.WhiteList()
	h.sogaBody(c, b, err)
}

var sogaOK = nodesvc.Object{{Key: "code", Value: 0}, {Key: "message", Value: ""}}

// sogaRead reads the body; size errors become 413, everything else 400.
func sogaRead(c *gin.Context) ([]byte, bool) {
	body, err := readBody(c)
	if err != nil {
		if isTooLarge(err) {
			sogaFail(c, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			sogaFail(c, http.StatusBadRequest, "invalid request body")
		}
		return nil, false
	}
	return body, true
}

// POST /traffic [{"id","u","d"}]
func (h *handler) sogaTraffic(c *gin.Context) {
	body, ok := sogaRead(c)
	if !ok {
		return
	}
	entries, err := nodesvc.ParseSogaTraffic(body)
	if err != nil {
		sogaFail(c, http.StatusBadRequest, "invalid traffic data")
		return
	}
	if err := h.svc.RecordTraffic(c.Request.Context(), resolved(c), entries); err != nil {
		sogaError(c, err)
		return
	}
	writeJSON(c, sogaOK)
}

// POST /alive_ip [{"id","ips"}]
func (h *handler) sogaAlive(c *gin.Context) {
	body, ok := sogaRead(c)
	if !ok {
		return
	}
	alive, err := nodesvc.ParseSogaAlive(body)
	if err != nil {
		sogaFail(c, http.StatusBadRequest, "invalid alive_ip data")
		return
	}
	if err := h.svc.RecordAlive(c.Request.Context(), resolved(c), alive); err != nil {
		sogaError(c, err)
		return
	}
	writeJSON(c, sogaOK)
}

// POST /audit_log [{"user_id","audit_id"}]
func (h *handler) sogaAuditLog(c *gin.Context) {
	body, ok := sogaRead(c)
	if !ok {
		return
	}
	entries, err := nodesvc.ParseAuditLog(body)
	if err != nil {
		sogaFail(c, http.StatusBadRequest, "invalid audit_log data")
		return
	}
	if err := h.svc.RecordAudit(c.Request.Context(), resolved(c), entries); err != nil {
		sogaError(c, err)
		return
	}
	writeJSON(c, sogaOK)
}

// POST /status {"cpu","mem","swap","disk","uptime"}
func (h *handler) sogaStatus(c *gin.Context) {
	body, ok := sogaRead(c)
	if !ok {
		return
	}
	load, err := nodesvc.ParseStatus(body, false)
	if err != nil {
		sogaFail(c, http.StatusBadRequest, err.Error())
		return
	}
	h.svc.RecordLoad(resolved(c), load)
	writeJSON(c, sogaOK)
}
