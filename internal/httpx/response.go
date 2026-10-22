package httpx

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIPrefix is the base path of the JSON API.
const APIPrefix = "/api/v1"

// OK writes 200 {"data": data}. Use OK(c, true) for empty successes.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// Created writes 201 {"data": data}.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

// List writes 200 {"data": items, "total": total}; a nil slice is rendered as [].
func List[T any](c *gin.Context, items []T, total int64) {
	if items == nil {
		items = []T{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

// Fail aborts the request with err rendered by AsError. 5xx errors are logged with their cause.
func Fail(c *gin.Context, err error) {
	e := AsError(err)
	if e == nil {
		e = Internal(nil)
	}
	if e.Status >= http.StatusInternalServerError {
		Logger(c).Error("request failed", "path", c.Request.URL.Path, "err", err)
	}
	if c.Writer.Written() {
		c.Abort()
		return
	}
	c.AbortWithStatusJSON(e.Status, e)
}

// Logger returns the request-scoped logger (with request_id) or slog.Default().
func Logger(c *gin.Context) *slog.Logger {
	if v, ok := c.Get(ctxLogger); ok {
		if l, ok := v.(*slog.Logger); ok {
			return l
		}
	}
	return slog.Default()
}

// RequestIDFrom returns the request id assigned by the RequestID middleware.
func RequestIDFrom(c *gin.Context) string { return c.GetString(ctxRequestID) }
