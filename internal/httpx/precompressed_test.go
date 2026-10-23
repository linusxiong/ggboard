package httpx_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// GzipBytes produces what the middleware sends for the same body, so a cached encoding is interchangeable with it.
func TestGzipBytesMatchesMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	body := []byte(`{"users":[` + strings.Repeat(`{"id":1,"uuid":"3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f"},`, 200) + `{}]}`)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.GET("/data", func(c *gin.Context) { c.Data(http.StatusOK, "application/json; charset=utf-8", body) })
	})
	rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/data", nil, "", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("middleware did not compress: %v", rec.Header())
	}
	gz, err := httpx.GzipBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gz, rec.Body.Bytes()) {
		t.Fatalf("GzipBytes (%d bytes) differs from the middleware's encoding (%d bytes)", len(gz), rec.Body.Len())
	}
	if !bytes.Equal(gunzip(t, gz), body) {
		t.Fatal("GzipBytes does not decompress to its input")
	}
}

// perf 2026-10-23 (docs/perf/README.md, finding 1): a cached body is sent with its cached encoding, with exactly the
// headers the middleware would set, and every other response is c.Data's.
func TestDataPrecompressed(t *testing.T) {
	a := testutil.NewTestApp(t)
	body := []byte(`{"list":"` + strings.Repeat("abcdefgh", 512) + `"}`)
	small := []byte(`{"ok":true}`)
	// A valid gzip stream that the middleware would never produce (header name, best compression): seeing these
	// exact bytes proves the response passed through instead of being compressed again.
	var marked bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&marked, gzip.BestCompression)
	zw.Name = "precompressed"
	_, _ = zw.Write(body)
	_ = zw.Close()

	var calls atomic.Int32
	cached := func() []byte { calls.Add(1); return marked.Bytes() }
	route := func(path string, b []byte, gz func() []byte) func(r gin.IRouter) {
		return func(r gin.IRouter) {
			r.GET(path, func(c *gin.Context) {
				c.Header("ETag", `"tag"`)
				httpx.DataPrecompressed(c, http.StatusOK, "application/json; charset=utf-8", b, gz)
			})
		}
	}
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		route("/cached", body, cached)(r)
		r.GET("/reference", func(c *gin.Context) { // what the handlers did before: c.Data through the middleware
			c.Header("ETag", `"tag"`)
			c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		})
		route("/small", small, cached)(r)
		route("/failed", body, func() []byte { return nil })(r)
	})
	gz := map[string]string{"Accept-Encoding": "gzip, br"}

	// Gzip client: the cached bytes, the middleware's headers.
	ref := testutil.DoWithHeaders(t, r, http.MethodGet, "/reference", nil, "", gz)
	rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/cached", nil, "", gz)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), marked.Bytes()) || calls.Load() != 1 {
		t.Fatalf("cached: %d, %d bytes (want the %d cached ones), %d calls", rec.Code, rec.Body.Len(), marked.Len(), calls.Load())
	}
	if !bytes.Equal(gunzip(t, rec.Body.Bytes()), body) {
		t.Fatal("cached encoding does not decompress to the body")
	}
	for _, h := range []string{"Content-Encoding", "Vary", "ETag", "Content-Type"} {
		if got, want := rec.Header().Values(h), ref.Header().Values(h); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s = %q, the middleware sends %q", h, got, want)
		}
	}
	// The one difference: the encoded length is known, so it is sent (the middleware streams, hence chunked).
	if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(marked.Len()) || ref.Header().Get("Content-Length") != "" {
		t.Errorf("Content-Length = %q (want %d), middleware %q", got, marked.Len(), ref.Header().Get("Content-Length"))
	}
	if rec.Header().Get("ETag") != `W/"tag"` || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzip headers = %v", rec.Header())
	}

	// Identity clients (no header, q=0) get c.Data's response: same headers, the plain body, no encoding computed.
	for _, hdr := range []map[string]string{nil, {"Accept-Encoding": "gzip;q=0"}, {"Accept-Encoding": "br"}} {
		ref = testutil.DoWithHeaders(t, r, http.MethodGet, "/reference", nil, "", hdr)
		rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/cached", nil, "", hdr)
		if !bytes.Equal(rec.Body.Bytes(), body) || !bytes.Equal(ref.Body.Bytes(), body) {
			t.Fatalf("%v: identity body changed", hdr)
		}
		for k, v := range ref.Header() {
			if k != "X-Request-Id" && strings.Join(rec.Header()[k], ",") != strings.Join(v, ",") {
				t.Errorf("%v: %s = %q, c.Data sends %q", hdr, k, rec.Header()[k], v)
			}
		}
		if len(rec.Header()) != len(ref.Header()) || rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("ETag") != `"tag"` {
			t.Fatalf("%v: identity headers = %v, want %v", hdr, rec.Header(), ref.Header())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("identity responses must not ask for the encoding (%d calls)", calls.Load())
	}

	// Below the middleware's 1 KiB threshold nothing is compressed, as before.
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/small", nil, "", gz)
	if !bytes.Equal(rec.Body.Bytes(), small) || rec.Header().Get("Content-Encoding") != "" || calls.Load() != 1 {
		t.Fatalf("small: %v %q (%d calls)", rec.Header(), rec.Body.String(), calls.Load())
	}

	// Without a cached encoding (gzipped returns nil) the middleware compresses as usual.
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/failed", nil, "", gz)
	ref = testutil.DoWithHeaders(t, r, http.MethodGet, "/reference", nil, "", gz)
	if !bytes.Equal(rec.Body.Bytes(), ref.Body.Bytes()) || rec.Header().Get("ETag") != `W/"tag"` || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("failed: %v (%d bytes), the middleware sends %v (%d bytes)", rec.Header(), rec.Body.Len(), ref.Header(), ref.Body.Len())
	}
}
