package static_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/static"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func dist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                  {Data: []byte("<!doctype html><title>GG</title>")},
		"index.html.br":               {Data: []byte("BR-INDEX")},
		"assets/index-AbCd1234.js":    {Data: []byte("console.log(1)")},
		"assets/index-AbCd1234.js.br": {Data: []byte("BR-JS")},
		"assets/index-AbCd1234.js.gz": {Data: []byte("GZ-JS")},
		"assets/style-Zz9_8765.css":   {Data: []byte("body{}")},
		"favicon.svg":                 {Data: []byte("<svg/>")},
		"apple-touch-icon.png":        {Data: []byte("png")},
		"robots.txt":                  {Data: []byte("User-agent: *")},
		".gitkeep":                    {Data: nil},
	}
}

func router(t *testing.T, fsys fstest.MapFS) *gin.Engine {
	t.Helper()
	a := testutil.NewTestApp(t)
	h, err := static.NewHandler(fsys)
	if err != nil {
		t.Fatal(err)
	}
	e := testutil.NewRouterWith(t, a, func(r gin.IRouter, _ *app.App) {})
	e.NoRoute(h.Serve)
	return e
}

func get(t *testing.T, e *gin.Engine, path, accept string, extra ...string) *httpResult {
	t.Helper()
	h := map[string]string{}
	if accept != "" {
		h["Accept-Encoding"] = accept
	}
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	rec := testutil.DoWithHeaders(t, e, http.MethodGet, path, nil, "", h)
	return &httpResult{code: rec.Code, h: rec.Header(), body: rec.Body.String()}
}

type httpResult struct {
	code int
	h    http.Header
	body string
}

func TestHashedAssetsArePrecompressedAndImmutable(t *testing.T) {
	e := router(t, dist())
	r := get(t, e, "/assets/index-AbCd1234.js", "gzip, deflate, br")
	if r.code != 200 || r.body != "BR-JS" || r.h.Get("Content-Encoding") != "br" || r.h.Get("Cache-Control") != static.CacheImmutable {
		t.Fatalf("br = %d %q %v", r.code, r.body, r.h)
	}
	if r.h.Get("Content-Type") != "text/javascript; charset=utf-8" || !strings.Contains(r.h.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("headers = %v", r.h)
	}
	if r = get(t, e, "/assets/index-AbCd1234.js", "gzip"); r.body != "GZ-JS" || r.h.Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip = %q %v", r.body, r.h)
	}
	r = get(t, e, "/assets/index-AbCd1234.js", "")
	if r.body != "console.log(1)" || r.h.Get("Content-Encoding") != "" {
		t.Fatalf("identity = %q %v", r.body, r.h)
	}
	etag := r.h.Get("ETag")
	if r = get(t, e, "/assets/index-AbCd1234.js", "", "If-None-Match", etag); r.code != http.StatusNotModified {
		t.Fatalf("conditional = %d", r.code)
	}
	if r = get(t, e, "/assets/style-Zz9_8765.css", ""); r.h.Get("Cache-Control") != static.CacheImmutable || !strings.HasPrefix(r.h.Get("Content-Type"), "text/css") {
		t.Fatalf("css = %v", r.h)
	}
	if r = get(t, e, "/favicon.svg", ""); r.h.Get("Cache-Control") != static.CacheNoCache || r.h.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("favicon = %v", r.h)
	}
	if r = get(t, e, "/apple-touch-icon.png", ""); r.h.Get("Cache-Control") != static.CacheNoCache {
		t.Fatalf("public files must not be immutable: %v", r.h)
	}
}

func TestIndexAndSPAFallback(t *testing.T) {
	e := router(t, dist())
	for _, p := range []string{"/", "/index.html", "/orders/2026102100000001", "/admin/users/5"} {
		r := get(t, e, p, "")
		if r.code != 200 || !strings.Contains(r.body, "<title>GG</title>") {
			t.Fatalf("%s = %d %q", p, r.code, r.body)
		}
		if r.h.Get("Cache-Control") != static.CacheNoCache || r.h.Get("Content-Security-Policy") != static.CSP || r.h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s headers = %v", p, r.h)
		}
	}
	if r := get(t, e, "/orders/1", "br"); r.body != "BR-INDEX" || r.h.Get("Content-Encoding") != "br" {
		t.Fatalf("precompressed index = %q", r.body)
	}
	if r := get(t, e, "/assets/missing-12345678.js", ""); r.code != 404 || strings.Contains(r.body, "<title>") {
		t.Fatalf("missing asset = %d", r.code)
	}
	if r := get(t, e, "/missing.png", ""); r.code != 404 {
		t.Fatalf("missing file = %d", r.code)
	}
	for _, p := range []string{"/api/v1/nope", "/api", "/soga/api/v1/node"} {
		r := get(t, e, p, "")
		if r.code != 404 || !strings.Contains(r.body, `"code":"not_found"`) {
			t.Fatalf("%s = %d %s", p, r.code, r.body)
		}
	}
	rec := testutil.Do(t, e, http.MethodPost, "/orders/1", nil, "")
	testutil.ExpectStatus(t, rec, 404)
	rec = testutil.Do(t, e, http.MethodHead, "/", nil, "")
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD = %d len %d", rec.Code, rec.Body.Len())
	}
}

func TestMissingIndexShowsBuildHint(t *testing.T) {
	e := router(t, fstest.MapFS{".gitkeep": {}})
	r := get(t, e, "/dashboard", "")
	if r.code != 200 || !strings.Contains(r.body, "Frontend not built") || r.h.Get("Cache-Control") != "no-store" {
		t.Fatalf("not built = %d %q", r.code, r.body)
	}
}

func TestUncompressedHTMLIsGzippedByMiddleware(t *testing.T) {
	big := "<!doctype html>" + strings.Repeat("<p>hello</p>", 200)
	e := router(t, fstest.MapFS{"index.html": {Data: []byte(big)}})
	r := get(t, e, "/", "gzip")
	if r.h.Get("Content-Encoding") != "gzip" || r.h.Get("Content-Length") != "" || !strings.HasPrefix(r.h.Get("ETag"), "W/") {
		t.Fatalf("gzip middleware on html = %v", r.h)
	}
}

func TestMountWithFrontendDir(t *testing.T) {
	dir := t.TempDir()
	a := testutil.NewTestApp(t, testutil.WithConfig(func(c *configAlias) { c.Frontend.Dir = dir }))
	e := testutil.NewRouterWith(t, a)
	if err := static.Mount(e, a); err != nil {
		t.Fatal(err)
	}
	if r := get(t, e, "/", ""); !strings.Contains(r.body, "Frontend not built") {
		t.Fatal("empty frontend dir must show the build hint")
	}
	b := testutil.NewTestApp(t, testutil.WithConfig(func(c *configAlias) { c.Frontend.Dir = dir + "/missing" }))
	if err := static.Mount(testutil.NewRouterWith(t, b), b); err == nil {
		t.Fatal("missing frontend dir must fail")
	}
	c := testutil.NewTestApp(t)
	if err := static.Mount(testutil.NewRouterWith(t, c), c); err != nil {
		t.Fatalf("embedded dist: %v", err)
	}
}
