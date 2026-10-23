package httpx

import (
	"bytes"
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
// files), partial content, 1xx/204/304 responses or HEAD requests. Handlers that cache a body can cache its
// encoding too and send it with DataPrecompressed.
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

// compresses reports whether the middleware gzips a response with this status and headers whose first n body bytes
// are known (final: n is the whole body).
func compresses(status int, h http.Header, n int, final bool) bool {
	return n > 0 &&
		!(final && n < gzipMinSize) &&
		status >= 200 && status != http.StatusNoContent && status != http.StatusPartialContent && status != http.StatusNotModified &&
		h.Get("Content-Encoding") == "" && h.Get("Content-Range") == "" &&
		compressible(h.Get("Content-Type"))
}

// varies adds Vary: Accept-Encoding to every compressible response the middleware could have encoded (before
// Content-Encoding is set), whether or not this one is.
func varies(h http.Header) {
	if compressible(h.Get("Content-Type")) && h.Get("Content-Encoding") == "" {
		h.Add("Vary", "Accept-Encoding")
	}
}

// markGzip sets the headers of a gzip-encoded response; a strong ETag becomes weak (RFC 9110: the encoded bytes
// differ from the identity representation's).
func markGzip(h http.Header) {
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	if et := h.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		h.Set("ETag", "W/"+et)
	}
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
	compress := compresses(status, h, len(g.buf), final)
	varies(h)
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
	markGzip(h)
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

// GzipBytes returns the gzip encoding of b, made the way the Gzip middleware makes it (same level, same stream), for
// a body that is cached together with its encoding and sent with DataPrecompressed.
func GzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	_, err := zw.Write(b)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	zw.Reset(nil)
	gzipPool.Put(zw)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(buf.Bytes()), nil // drop the buffer's spare capacity: the result is kept in a cache
}

// DataPrecompressed sends body like c.Data. When the Gzip middleware would compress this response (the client accepts
// gzip, the request is not HEAD, the body is compressible and at least 1 KiB, nothing set Content-Encoding), it sends
// gzipped() instead, with the headers the middleware sets (Content-Encoding: gzip, Vary: Accept-Encoding, a weak
// ETag) plus the encoded Content-Length (the middleware streams, so it cannot), and a cached body is not compressed
// again for every response. gzipped returns the gzip encoding of body (GzipBytes), usually cached next to it; nil
// makes the middleware compress as usual. Every other response, the identity one included, is exactly c.Data's.
func DataPrecompressed(c *gin.Context, status int, contentType string, body []byte, gzipped func() []byte) {
	if gw, ok := c.Writer.(*gzipWriter); ok && !gw.decided && len(gw.buf) == 0 {
		h := gw.Header()
		if h.Get("Content-Type") == "" { // c.Data keeps a Content-Type set earlier
			h.Set("Content-Type", contentType)
		}
		if compresses(status, h, len(body), true) {
			if gz := gzipped(); gz != nil {
				varies(h)
				markGzip(h)
				gw.decided = true // already encoded: the writer passes status and bytes through
				c.Data(status, contentType, gz)
				return
			}
		}
	}
	c.Data(status, contentType, body)
}
