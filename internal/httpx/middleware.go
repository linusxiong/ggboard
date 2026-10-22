package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const (
	ctxRequestID = "httpx.request_id"
	ctxLogger    = "httpx.logger"
	ctxBody      = "httpx.body"

	// HeaderRequestID carries the request id in requests and responses.
	HeaderRequestID = "X-Request-ID"

	// DefaultBodyLimit applies to every request; NodeBodyLimit is for node push endpoints (use LargeBody).
	DefaultBodyLimit int64 = 1 << 20
	NodeBodyLimit    int64 = 8 << 20
)

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// RequestID assigns X-Request-ID (reusing a sane incoming one) and a request-scoped logger.
func RequestID(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID(id) {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		c.Set(ctxRequestID, id)
		c.Set(ctxLogger, log.With("request_id", id))
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// Recovery converts panics into a JSON 500 and logs the stack.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, rec any) {
		Logger(c).Error("panic recovered", "panic", rec, "path", c.Request.URL.Path, "stack", string(debug.Stack()))
		Fail(c, NewError(http.StatusInternalServerError, CodeInternal, "internal server error"))
	})
}

// AccessLog logs one line per request (path only: query strings may carry tokens).
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		// Log the route pattern (e.g. /s/:token) so secrets carried in path params never reach the log;
		// unmatched requests (404) have no pattern and log their literal path.
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		Logger(c).Log(c.Request.Context(), level, "http",
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency", time.Since(start).String(),
			"ip", c.ClientIP(),
			"bytes", c.Writer.Size(),
		)
	}
}

// SecurityHeaders sets conservative headers on every response (the SPA handler adds its CSP).
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		c.Next()
	}
}

// CORS returns a CORS middleware for origins ("*" = any), or nil when origins is empty (same-origin only).
func CORS(origins []string) gin.HandlerFunc {
	if len(origins) == 0 {
		return nil
	}
	cfg := cors.Config{
		AllowMethods:  []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{"Authorization", "Content-Type", HeaderRequestID, "Accept-Language"},
		ExposeHeaders: []string{HeaderRequestID, "Retry-After", "Content-Disposition"},
		MaxAge:        12 * time.Hour,
	}
	for _, o := range origins {
		if o == "*" {
			cfg.AllowAllOrigins = true
		}
	}
	if !cfg.AllowAllOrigins {
		cfg.AllowOrigins = origins
	}
	return cors.New(cfg)
}

// limitedBody applies http.MaxBytesReader lazily so the limit can still be changed (SetBodyLimit) by
// middleware that runs before the body is read.
type limitedBody struct {
	rc    io.ReadCloser
	w     http.ResponseWriter
	limit int64
	r     io.ReadCloser
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.r == nil {
		b.r = http.MaxBytesReader(b.w, b.rc, b.limit)
	}
	return b.r.Read(p)
}

func (b *limitedBody) Close() error { return b.rc.Close() }

// BodyLimit caps request bodies at n bytes (413 payload_too_large when exceeded).
func BodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && c.Request.Body != http.NoBody {
			lb := &limitedBody{rc: c.Request.Body, w: c.Writer, limit: n}
			c.Request.Body = lb
			c.Set(ctxBody, lb)
		}
		c.Next()
	}
}

// SetBodyLimit changes the body limit of the current request (effective only before the body is read).
func SetBodyLimit(c *gin.Context, n int64) {
	if v, ok := c.Get(ctxBody); ok {
		if lb, ok := v.(*limitedBody); ok && lb.r == nil {
			lb.limit = n
		}
	}
}

// LargeBody is middleware raising the body limit to n for a route group (node push endpoints use
// LargeBody(NodeBodyLimit)).
func LargeBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		SetBodyLimit(c, n)
		c.Next()
	}
}
