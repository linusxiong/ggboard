package httpx_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Security audit 2026-11-25 §4: API responses carried no Cache-Control, including the login response with its JWT
// and admin settings with secrets, so a shared or browser cache was free to keep them. Every response now defaults
// to no-store; a handler that sets its own value (a public catalog, a static file) keeps it, ETag/304 included.
func TestReviewResponsesDefaultToNoStore(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.POST("/token", func(c *gin.Context) { httpx.OK(c, gin.H{"token": "jwt"}) })
		r.GET("/list", func(c *gin.Context) { httpx.List(c, []int{1}, 1) })
		r.GET("/fail", func(c *gin.Context) { httpx.Fail(c, httpx.Forbidden("no")) })
		r.GET("/public", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=300")
			c.Header("ETag", `"v1"`)
			if c.GetHeader("If-None-Match") == `"v1"` {
				c.Status(http.StatusNotModified)
				return
			}
			httpx.OK(c, true)
		})
	})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/token"}, {http.MethodGet, "/list"}, {http.MethodGet, "/fail"}, {http.MethodGet, "/missing"},
	} {
		if got := testutil.Do(t, r, tc.method, tc.path, nil, "").Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s Cache-Control = %q, want no-store", tc.method, tc.path, got)
		}
	}
	rec := testutil.Do(t, r, http.MethodGet, "/public", nil, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("own value = %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/public", nil, "", map[string]string{"If-None-Match": `"v1"`})
	if rec.Code != http.StatusNotModified || rec.Header().Get("Cache-Control") != "public, max-age=300" || rec.Header().Get("ETag") != `"v1"` {
		t.Fatalf("304 = %d %v", rec.Code, rec.Header())
	}
}
