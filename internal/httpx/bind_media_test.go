package httpx_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type mediaReq struct {
	Email string `json:"email" form:"email" xml:"email" yaml:"email" toml:"email" binding:"required"`
}

// countingBody records how many bytes the handler read from the request body.
type countingBody struct {
	r io.ReadCloser
	n *int
}

func (b countingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	*b.n += n
	return n, err
}

func (b countingBody) Close() error { return b.r.Close() }

// Security audit 2026-11-25 §1: Bind passed every Content-Type other than JSON to gin's ShouldBind, which picks
// XML, YAML, TOML, msgpack, BSON or protobuf by Content-Type. goccy/go-yaml expands aliases without limit, so a
// ~700-byte alias bomb to any anonymous passport route grew the heap by gigabytes. Only JSON bodies are decoded
// now; anything else is a 415 answered before the body is read. The bodies below are tiny and decode fine with
// the matching gin binding (old code: 200 for YAML/TOML/XML/msgpack/form, 400 for the rest), never a bomb.
func TestBindAcceptsOnlyJSONBodies(t *testing.T) {
	a := testutil.NewTestApp(t)
	var (
		read  int
		bound mediaReq
	)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.Use(func(c *gin.Context) {
			if c.Request.Body != nil {
				c.Request.Body = countingBody{r: c.Request.Body, n: &read}
			}
			c.Next()
		})
		h := func(c *gin.Context) {
			var req mediaReq
			err := httpx.Bind(c, &req)
			bound = req
			if err != nil {
				httpx.Fail(c, err)
				return
			}
			httpx.OK(c, req)
		}
		r.POST("/x", h)
		r.PUT("/x", h)
		r.PATCH("/x", h)
	})
	do := func(method, ct, body string) *httptest.ResponseRecorder {
		read, bound = 0, mediaReq{}
		req := httptest.NewRequest(method, "/x", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	const email = "a@example.com"
	for _, tc := range []struct{ ct, body string }{
		{"application/x-yaml", "email: " + email},
		{"application/yaml", "email: " + email},
		{"application/x-yaml; charset=utf-8", "email: " + email},
		{"Application/X-YAML", "email: " + email},
		{"application/x-yaml", "email: &a [*a]"},
		{"application/toml", `email = "` + email + `"`},
		{"application/x-msgpack", "\x81\xa5email\xad" + email},
		{"application/msgpack", "\x81\xa5email\xad" + email},
		{"application/bson", "\x05\x00\x00\x00\x00"},
		{"application/xml", "<mediaReq><email>" + email + "</email></mediaReq>"},
		{"text/xml", "<mediaReq><email>" + email + "</email></mediaReq>"},
		{"application/x-protobuf", "\x0a\x0d" + email},
		{"application/x-www-form-urlencoded", "email=" + email},
		{"multipart/form-data; boundary=b", "--b\r\nContent-Disposition: form-data; name=\"email\"\r\n\r\n" + email + "\r\n--b--\r\n"},
		{"text/plain", email},
		{"application/+json", `{"email":"` + email + `"}`},
		{"text/json", `{"email":"` + email + `"}`},
	} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
			rec := do(method, tc.ct, tc.body)
			e := testutil.DecodeError(t, rec)
			if e.Status != http.StatusUnsupportedMediaType || e.Code != httpx.CodeUnsupportedMediaType {
				t.Errorf("%s %q = %d %+v, want 415 %s", method, tc.ct, rec.Code, e, httpx.CodeUnsupportedMediaType)
			}
			if read != 0 || bound != (mediaReq{}) {
				t.Errorf("%s %q: read %d body bytes, bound %+v; want the body untouched", method, tc.ct, read, bound)
			}
		}
	}

	for _, ct := range []string{"", "application/json", "application/json; charset=utf-8", "Application/JSON", "application/merge-patch+json"} {
		rec := do(http.MethodPost, ct, `{"email":"`+email+`"}`)
		if got := testutil.Decode[mediaReq](t, rec); got.Email != email {
			t.Errorf("JSON with Content-Type %q = %d %s", ct, rec.Code, rec.Body.String())
		}
	}
	// An empty body is still validated (GET query binding: TestBindValidation).
	if e := testutil.DecodeError(t, do(http.MethodPost, "", "")); e.Status != http.StatusBadRequest || e.Errors["email"][0] != "is required" {
		t.Fatalf("empty body = %+v", e)
	}
	if e := testutil.DecodeError(t, do(http.MethodPost, "application/json", `{"email":`)); e.Status != http.StatusBadRequest {
		t.Fatalf("broken JSON = %+v", e)
	}
}
