package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogPathRedactsUnmatchedSecrets(t *testing.T) {
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.GET("/s/:token", func(c *gin.Context) { c.String(http.StatusOK, LogPath(c)) })
	r.NoRoute(func(c *gin.Context) { c.String(http.StatusNotFound, LogPath(c)) })
	for _, tc := range []struct{ path, want string }{
		{"/s/fake-secret", "/s/:token"},
		{"/s/fake-secret/", "/[unmatched]"},
		{"/api/v1/guest/telegram/webhook/fake-secret/", "/[unmatched]"},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Body.String() != tc.want {
			t.Fatalf("%q logged as %q, want %q", tc.path, rec.Body.String(), tc.want)
		}
	}
}
