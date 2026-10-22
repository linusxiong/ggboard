package httpx

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// gzipMinSize is the smallest body worth compressing.
const gzipMinSize = 1024

var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return w
}}

// Gzip compresses compressible responses (JSON, text, JS, CSS, SVG, XML, YAML) of at least 1 KiB when the
// client accepts gzip. It never touches responses that already set Content-Encoding (precompressed static
// files), partial content, 1xx/204/304 responses or HEAD requests.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead || c.GetHeader("Upgrade") != "" || !acceptsEncoding(c.Request, "gzip") {
			c.Next()
			return
		}
		orig := c.Writer
		gw := &gzipWriter{ResponseWriter: orig}
		c.Writer = gw
		defer func() {
			gw.finish()
			c.Writer = orig
		}()
		c.Next()
	}
}

// acceptsEncoding reports whether the Accept-Encoding header allows enc (q > 0).
func acceptsEncoding(r *http.Request, enc string) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), enc) {
			continue
		}
		params = strings.ReplaceAll(params, " ", "")
		return !(strings.HasPrefix(params, "q=0") && strings.Trim(strings.TrimPrefix(params, "q="), "0.") == "")
	}
	return false
}

// AcceptsEncoding is exported for the static file server.
func AcceptsEncoding(r *http.Request, enc string) bool { return acceptsEncoding(r, enc) }

func compressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mt = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	switch {
	case strings.HasPrefix(mt, "text/") && mt != "text/event-stream":
		return true
	case mt == "application/json", mt == "application/javascript", mt == "application/xml",
		mt == "image/svg+xml", mt == "application/manifest+json", mt == "application/yaml",
		mt == "application/x-yaml", strings.HasSuffix(mt, "+json"), strings.HasSuffix(mt, "+xml"):
		return true
	}
	return false
}

type gzipWriter struct {
	gin.ResponseWriter
	status  int
	buf     []byte
	decided bool
	gz      *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.decided {
		g.ResponseWriter.WriteHeader(code)
		return
	}
	g.status = code
}

func (g *gzipWriter) Status() int {
	if !g.decided && g.status != 0 {
		return g.status
	}
	return g.ResponseWriter.Status()
}

func (g *gzipWriter) Written() bool {
	return len(g.buf) > 0 || g.ResponseWriter.Written()
}

func (g *gzipWriter) WriteHeaderNow() {
	if !g.decided {
		_ = g.decide(true)
	}
	g.ResponseWriter.WriteHeaderNow()
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.decided {
		g.buf = append(g.buf, p...)
		if len(g.buf) >= gzipMinSize {
			if err := g.decide(false); err != nil {
				return 0, err
			}
		}
		return len(p), nil
	}
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipWriter) WriteString(s string) (int, error) { return g.Write([]byte(s)) }

func (g *gzipWriter) Flush() {
	if !g.decided {
		_ = g.decide(false)
	}
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	g.ResponseWriter.Flush()
}

// decide picks compression based on headers and the buffered prefix, then writes the header and buffer.
func (g *gzipWriter) decide(final bool) error {
	g.decided = true
	h := g.Header()
	status := g.status
	if status == 0 {
		status = http.StatusOK
	}
	if h.Get("Content-Type") == "" && len(g.buf) > 0 {
		h.Set("Content-Type", http.DetectContentType(g.buf))
	}
	compress := len(g.buf) > 0 &&
		!(final && len(g.buf) < gzipMinSize) &&
		status >= 200 && status != http.StatusNoContent && status != http.StatusPartialContent && status != http.StatusNotModified &&
		h.Get("Content-Encoding") == "" && h.Get("Content-Range") == "" &&
		compressible(h.Get("Content-Type"))
	if compressible(h.Get("Content-Type")) && h.Get("Content-Encoding") == "" {
		h.Add("Vary", "Accept-Encoding")
	}
	if !compress {
		if g.status != 0 || len(g.buf) > 0 {
			g.ResponseWriter.WriteHeader(status)
		}
		buf := g.buf
		g.buf = nil
		if len(buf) > 0 {
			_, err := g.ResponseWriter.Write(buf)
			return err
		}
		return nil
	}
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	if et := h.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		h.Set("ETag", "W/"+et)
	}
	g.ResponseWriter.WriteHeader(status)
	g.gz = gzipPool.Get().(*gzip.Writer)
	g.gz.Reset(g.ResponseWriter)
	buf := g.buf
	g.buf = nil
	_, err := g.gz.Write(buf)
	return err
}

func (g *gzipWriter) finish() {
	if !g.decided && (len(g.buf) > 0 || g.status != 0) {
		_ = g.decide(true)
	}
	if g.gz != nil {
		_ = g.gz.Close()
		g.gz.Reset(nil)
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}
