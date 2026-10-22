// Package static serves the embedded single-page app (web/dist copied to internal/static/dist by
// `make web`): precompressed .br/.gz siblings, immutable caching for hashed assets, no-cache HTML with a CSP,
// prerendered public routes (`/login` → login/index.html, written by web/scripts/prerender.mjs) and SPA
// fallback (spa.html — the plain shell — when present, else index.html) for every other non-API path.
package static

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/httpx"
)

//go:embed all:dist
var embedded embed.FS

// CSP is the Content-Security-Policy sent with HTML documents when no captcha provider is enabled.
const CSP = "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; " +
	"connect-src 'self'; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// captchaSources are the origins each captcha widget (web/src/components/Captcha.tsx) loads scripts, frames,
// XHRs and styles from, per the providers' CSP guides. reCAPTCHA is loaded from www.recaptcha.net (reachable in
// mainland China) and may still frame or fetch from www.google.com.
var captchaSources = map[string]struct{ script, frame, connect, style string }{
	captcha.Turnstile: {
		script: "https://challenges.cloudflare.com",
		frame:  "https://challenges.cloudflare.com",
	},
	captcha.ReCaptcha: {
		script: "https://www.recaptcha.net/recaptcha/ https://www.google.com/recaptcha/ https://www.gstatic.com/recaptcha/",
		frame:  "https://www.recaptcha.net/recaptcha/ https://www.google.com/recaptcha/ https://recaptcha.google.com/recaptcha/",
	},
	captcha.HCaptcha: {
		script:  "https://hcaptcha.com https://*.hcaptcha.com",
		frame:   "https://hcaptcha.com https://*.hcaptcha.com",
		connect: "https://hcaptcha.com https://*.hcaptcha.com",
		style:   "https://hcaptcha.com https://*.hcaptcha.com",
	},
}

// CSPFor returns the HTML Content-Security-Policy allowing the given captcha provider's widget ("none" or an
// unknown provider: CSP). Only the enabled provider is allowed, never all of them.
func CSPFor(provider string) string {
	src, ok := captchaSources[provider]
	if !ok {
		return CSP
	}
	join := func(base, extra string) string {
		if extra == "" {
			return base
		}
		return base + " " + extra
	}
	return "default-src 'self'; img-src 'self' data: https:; style-src " + join("'self' 'unsafe-inline'", src.style) +
		"; script-src " + join("'self'", src.script) + "; frame-src " + join("'self'", src.frame) +
		"; connect-src " + join("'self'", src.connect) +
		"; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
}

// Cache-Control values.
const (
	CacheImmutable = "public, max-age=31536000, immutable"
	CacheNoCache   = "no-cache"
)

type file struct {
	name        string
	contentType string
	etag        string
	cache       string
	html        bool
	br, gz      string // ETags of the precompressed siblings ("" if absent)
}

// Handler serves files from an fs.FS.
type Handler struct {
	fsys  fs.FS
	files map[string]*file
	csp   func() string // policy for HTML documents (CSP when nil)
}

// SetCSP makes HTML documents carry the policy returned by fn (evaluated per request, so settings changes
// apply at once).
func (h *Handler) SetCSP(fn func() string) { h.csp = fn }

func (h *Handler) policy() string {
	if h.csp == nil {
		return CSP
	}
	return h.csp()
}

// FS returns the frontend file system: cfg.Frontend.Dir when set, else the embedded dist directory.
func FS(a *app.App) (fs.FS, error) {
	if dir := a.Cfg.Frontend.Dir; dir != "" {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("static: frontend.dir %q is not a directory", dir)
		}
		return os.DirFS(dir), nil
	}
	return fs.Sub(embedded, "dist")
}

// Mount installs the SPA handler as the engine's NoRoute handler (call after registering all routes).
func Mount(e *gin.Engine, a *app.App) error {
	fsys, err := FS(a)
	if err != nil {
		return err
	}
	h, err := NewHandler(fsys)
	if err != nil {
		return err
	}
	h.SetCSP(func() string {
		if !captcha.Enabled(a.Settings) {
			return CSP
		}
		return CSPFor(a.Settings.String("captcha_provider"))
	})
	e.NoRoute(h.Serve)
	return nil
}

// ImmutablePrefix is the directory Vite writes content-hashed files to (build.assetsDir); only files under it
// are cached as immutable. Files copied from web/public keep their names and are revalidated.
const ImmutablePrefix = "assets/"

// ShellFile is the plain SPA shell (no prerendered markup) that web/scripts/prerender.mjs keeps next to the
// prerendered pages; index.html then holds the landing page snapshot, so deep links must not get it.
const ShellFile = "spa.html"

// NewHandler indexes fsys (content types, ETags, precompressed siblings).
func NewHandler(fsys fs.FS) (*Handler, error) {
	h := &Handler{fsys: fsys, files: map[string]*file{}}
	etags := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path.Base(p) == ".gitkeep" {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		etags[p] = `"` + hex.EncodeToString(sum[:12]) + `"`
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("static: index: %w", err)
	}
	for p, etag := range etags {
		if strings.HasSuffix(p, ".br") || strings.HasSuffix(p, ".gz") {
			if _, orig := etags[p[:len(p)-3]]; orig {
				continue // served as a sibling only
			}
		}
		f := &file{name: p, contentType: contentType(p), etag: etag, html: strings.HasSuffix(p, ".html")}
		switch {
		case f.html || p == "sw.js":
			f.cache = CacheNoCache
		case strings.HasPrefix(p, ImmutablePrefix):
			f.cache = CacheImmutable
		default:
			f.cache = CacheNoCache
		}
		if e, ok := etags[p+".br"]; ok {
			f.br = e
		}
		if e, ok := etags[p+".gz"]; ok {
			f.gz = e
		}
		h.files[p] = f
	}
	return h, nil
}

var types = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".json": "application/json", ".map": "application/json",
	".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".avif": "image/avif", ".ico": "image/x-icon", ".woff2": "font/woff2", ".woff": "font/woff",
	".txt": "text/plain; charset=utf-8", ".xml": "application/xml", ".webmanifest": "application/manifest+json",
	".wasm": "application/wasm",
}

func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := types[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func isAPIPath(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/soga/")
}

// Serve handles one request: API paths get a JSON 404, existing files are served, a directory with a
// prerendered index.html is served from it (`/login` → login/index.html), missing asset-like paths get 404
// and everything else falls back to the SPA shell (spa.html, else index.html).
func (h *Handler) Serve(c *gin.Context) {
	p := c.Request.URL.Path
	if isAPIPath(p) || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
		httpx.Fail(c, httpx.NotFound("route not found"))
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name == "" {
		name = "index.html"
	}
	f, ok := h.files[name]
	if !ok {
		// Missing assets and well-known URLs (RFC 8615: webauthn, apple-app-site-association, acme-challenge...)
		// are real 404s: the SPA page would be a 200 that clients parse as the resource.
		if strings.HasPrefix(name, ImmutablePrefix) || strings.HasPrefix(name, ".well-known/") ||
			(path.Ext(name) != "" && path.Ext(name) != ".html") {
			c.Header("Cache-Control", CacheNoCache)
			c.String(http.StatusNotFound, "404 not found")
			return
		}
		if f, ok = h.files[name+"/index.html"]; !ok {
			if f, ok = h.files[ShellFile]; !ok {
				if f, ok = h.files["index.html"]; !ok {
					notBuilt(c)
					return
				}
			}
		}
	}
	h.serveFile(c, f)
}

func (h *Handler) serveFile(c *gin.Context, f *file) {
	w, r := c.Writer, c.Request
	name, etag, enc := f.name, f.etag, ""
	switch {
	case f.br != "" && httpx.AcceptsEncoding(r, "br"):
		name, etag, enc = f.name+".br", f.br, "br"
	case f.gz != "" && httpx.AcceptsEncoding(r, "gzip"):
		name, etag, enc = f.name+".gz", f.gz, "gzip"
	}
	hd := w.Header()
	if f.br != "" || f.gz != "" {
		hd.Add("Vary", "Accept-Encoding")
	}
	hd.Set("Content-Type", f.contentType)
	hd.Set("Cache-Control", f.cache)
	hd.Set("ETag", etag)
	if enc != "" {
		hd.Set("Content-Encoding", enc)
	}
	if f.html {
		hd.Set("Content-Security-Policy", h.policy())
		hd.Set("X-Frame-Options", "DENY")
	}
	rs, err := h.open(name)
	if err != nil {
		httpx.Fail(c, httpx.Internal(err))
		return
	}
	defer rs.Close()
	http.ServeContent(w, r, "", time.Time{}, rs)
}

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func (h *Handler) open(name string) (readSeekCloser, error) {
	fl, err := h.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	if rsc, ok := fl.(readSeekCloser); ok {
		return rsc, nil
	}
	defer fl.Close()
	b, err := io.ReadAll(fl)
	if err != nil {
		return nil, err
	}
	return nopCloser{bytes.NewReader(b)}, nil
}

const notBuiltPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GG Bond</title></head>` +
	`<body style="margin:0;display:flex;min-height:100vh;align-items:center;justify-content:center;background:#F2F2F7;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',sans-serif;color:#1C1C1E">` +
	`<main style="max-width:420px;padding:32px;background:#fff;border-radius:18px;text-align:center"><h1 style="font-size:22px;margin:0 0 12px">Frontend not built</h1>` +
	`<p style="margin:0;color:#3C3C43;line-height:1.5">The API is running. Build the web app with <code>make web</code> and rebuild the binary.</p></main></body></html>`

func notBuilt(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Security-Policy", CSP)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(notBuiltPage))
}
