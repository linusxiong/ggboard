package httpx_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type item struct {
	Name string `json:"name" binding:"required,min=2"`
	Qty  int    `json:"qty" binding:"gte=1,lte=10"`
}

type createReq struct {
	Email    string  `json:"email" binding:"required,email"`
	Password string  `json:"password" binding:"required,min=8"`
	Period   string  `json:"period" binding:"omitempty,oneof=monthly yearly"`
	Items    []item  `json:"items" binding:"omitempty,dive"`
	Ignored  string  `json:"-"`
	Amount   float64 `json:"amount"`
}

type listQuery struct {
	Status *int   `form:"status" binding:"omitempty,oneof=0 1"`
	Email  string `form:"email"`
}

func engine(t *testing.T, a *app.App, routes func(r gin.IRouter, a *app.App)) *gin.Engine {
	return testutil.NewRouterWith(t, a, routes)
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{httpx.BadRequest("x"), 400, httpx.CodeValidation},
		{httpx.Unauthorized("x"), 401, httpx.CodeUnauthorized},
		{httpx.PaymentFailed("x"), 402, httpx.CodePaymentFailed},
		{httpx.Forbidden("x"), 403, httpx.CodeForbidden},
		{httpx.NotFound("x"), 404, httpx.CodeNotFound},
		{httpx.Conflict("x"), 409, httpx.CodeConflict},
		{httpx.PayloadTooLarge("x"), 413, httpx.CodePayloadTooLarge},
		{httpx.RateLimited("x"), 429, httpx.CodeRateLimited},
		{fmt.Errorf("wrapped: %w", gorm.ErrRecordNotFound), 404, httpx.CodeNotFound},
		{gorm.ErrDuplicatedKey, 409, httpx.CodeConflict},
		{&http.MaxBytesError{Limit: 5}, 413, httpx.CodePayloadTooLarge},
		{settings.FieldErrors{"app_name": {"must be a string"}}, 400, httpx.CodeValidation},
		{errors.New("db exploded"), 500, httpx.CodeInternal},
	}
	for _, tc := range cases {
		e := httpx.AsError(tc.err)
		if e.Status != tc.status || e.Code != tc.code {
			t.Errorf("%v -> %d %s, want %d %s", tc.err, e.Status, e.Code, tc.status, tc.code)
		}
	}
	if e := httpx.AsError(settings.FieldErrors{"k": {"bad"}}); e.Errors["k"][0] != "bad" {
		t.Fatal("field errors must be preserved")
	}
	if e := httpx.Internal(errors.New("secret detail")); strings.Contains(e.Message, "secret") || !errors.Is(e, e.Unwrap()) {
		t.Fatal("internal errors must not leak causes")
	}
	if e := httpx.FieldError("code", "invalid coupon"); e.Errors["code"][0] != "invalid coupon" {
		t.Fatal("FieldError")
	}
	if httpx.AsError(nil) != nil {
		t.Fatal("nil")
	}
}

func TestBindValidation(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.POST("/x", func(c *gin.Context) {
			var req createReq
			if err := httpx.Bind(c, &req); err != nil {
				httpx.Fail(c, err)
				return
			}
			httpx.OK(c, req)
		})
		r.GET("/q", func(c *gin.Context) {
			var q listQuery
			if err := httpx.Bind(c, &q); err != nil {
				httpx.Fail(c, err)
				return
			}
			httpx.OK(c, q)
		})
	})

	rec := testutil.Do(t, r, http.MethodPost, "/x", nil, "")
	e := testutil.DecodeError(t, rec)
	if e.Status != 400 || e.Code != httpx.CodeValidation || e.Errors["email"][0] != "is required" || e.Errors["password"][0] != "is required" {
		t.Fatalf("empty body = %+v", e)
	}

	rec = testutil.Do(t, r, http.MethodPost, "/x", map[string]any{
		"email": "nope", "password": "short", "period": "weekly",
		"items": []map[string]any{{"name": "a", "qty": 0}},
	}, "")
	e = testutil.DecodeError(t, rec)
	want := map[string]string{
		"email":         "must be a valid email address",
		"password":      "must be at least 8 characters",
		"period":        "must be one of: monthly, yearly",
		"items[0].name": "must be at least 2 characters",
		"items[0].qty":  "must be greater than or equal to 1",
	}
	for f, msg := range want {
		if len(e.Errors[f]) == 0 || e.Errors[f][0] != msg {
			t.Errorf("%s = %v, want %q", f, e.Errors[f], msg)
		}
	}

	rec = testutil.Do(t, r, http.MethodPost, "/x", `{"email": 5}`, "")
	if e = testutil.DecodeError(t, rec); e.Status != 400 || e.Errors["email"][0] != "must be a string" {
		t.Fatalf("type error = %+v", e)
	}
	rec = testutil.Do(t, r, http.MethodPost, "/x", `{"email":`, "")
	if e = testutil.DecodeError(t, rec); e.Status != 400 || e.Message != "invalid JSON body" {
		t.Fatalf("syntax error = %+v", e)
	}

	ok := testutil.Decode[createReq](t, testutil.Do(t, r, http.MethodPost, "/x", map[string]any{"email": "a@example.com", "password": "password123"}, ""))
	if ok.Email != "a@example.com" {
		t.Fatal("valid body")
	}

	rec = testutil.Do(t, r, http.MethodGet, "/q?status=5", nil, "")
	if e = testutil.DecodeError(t, rec); e.Errors["status"][0] != "must be one of: 0, 1" {
		t.Fatalf("query validation = %+v", e)
	}
	rec = testutil.Do(t, r, http.MethodGet, "/q?status=abc", nil, "")
	testutil.ExpectStatus(t, rec, 400)
	q := testutil.Decode[listQuery](t, testutil.Do(t, r, http.MethodGet, "/q?status=1&email=x", nil, ""))
	if q.Status == nil || *q.Status != 1 || q.Email != "x" {
		t.Fatalf("query = %+v", q)
	}
	if err := httpx.Validate(&item{Name: "ab", Qty: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestBodyLimit(t *testing.T) {
	a := testutil.NewTestApp(t)
	echo := func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, len(b))
	}
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.POST("/small", echo)
		r.POST("/node", httpx.LargeBody(httpx.NodeBodyLimit), echo)
	})
	big := bytes.Repeat([]byte("a"), int(httpx.DefaultBodyLimit)+1)
	rec := testutil.Do(t, r, http.MethodPost, "/small", big, "")
	if e := testutil.DecodeError(t, rec); e.Status != 413 || e.Code != httpx.CodePayloadTooLarge {
		t.Fatalf("small = %+v", e)
	}
	if n := testutil.Decode[int](t, testutil.Do(t, r, http.MethodPost, "/node", big, "")); n != len(big) {
		t.Fatalf("node body = %d", n)
	}
}

func TestPagination(t *testing.T) {
	a := testutil.NewTestApp(t)
	for i := range 25 {
		testutil.CreateUser(t, a, fmt.Sprintf("u%02d@example.com", i))
	}
	r := engine(t, a, func(r gin.IRouter, a *app.App) {
		r.GET("/users", func(c *gin.Context) {
			items, total, err := httpx.Paginate[model.User](c, a.DB.Where("email LIKE ?", "u%"), map[string]string{"id": "id", "email": "email"}, "id ASC")
			if err != nil {
				httpx.Fail(c, err)
				return
			}
			httpx.List(c, items, total)
		})
		r.GET("/page", func(c *gin.Context) {
			p, s, o := httpx.ParsePage(c)
			httpx.OK(c, []int{p, s, o})
		})
		r.GET("/sort", func(c *gin.Context) {
			httpx.OK(c, httpx.ParseSort(c, map[string]string{"created_at": "users.created_at", "id": "users.id"}))
		})
	})
	users, total := testutil.DecodeList[model.User](t, testutil.Do(t, r, http.MethodGet, "/users?page=2&page_size=10&sort=-id", nil, ""))
	if total != 25 || len(users) != 10 || users[0].Email != "u14@example.com" {
		t.Fatalf("page 2 = %d items, total %d, first %s", len(users), total, users[0].Email)
	}
	users, _ = testutil.DecodeList[model.User](t, testutil.Do(t, r, http.MethodGet, "/users?page=9", nil, ""))
	if users == nil || len(users) != 0 {
		t.Fatal("out-of-range page must be an empty list")
	}
	if got := testutil.Decode[[]int](t, testutil.Do(t, r, http.MethodGet, "/page?page=-1&page_size=1000", nil, "")); got[0] != 1 || got[1] != httpx.MaxPageSize || got[2] != 0 {
		t.Fatalf("ParsePage = %v", got)
	}
	if got := testutil.Decode[[]int](t, testutil.Do(t, r, http.MethodGet, "/page?page=3", nil, "")); got[1] != 20 || got[2] != 40 {
		t.Fatalf("ParsePage defaults = %v", got)
	}
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/sort?sort=-created_at,id,password%3Bdrop,id", nil, "")); got != "users.created_at DESC, users.id ASC" {
		t.Fatalf("ParseSort = %q", got)
	}
}

func TestRateLimit(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.GET("/limited", httpx.RateLimit("test", httpx.Per(2, time.Hour), 2, httpx.KeyByIP), func(c *gin.Context) { httpx.OK(c, true) })
		r.GET("/bytoken", httpx.RateLimit("token", httpx.Per(1, time.Hour), 1, httpx.KeyByQuery("token")), func(c *gin.Context) { httpx.OK(c, true) })
	})
	for range 2 {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/limited", nil, ""), 200)
	}
	rec := testutil.Do(t, r, http.MethodGet, "/limited", nil, "")
	if e := testutil.DecodeError(t, rec); e.Status != 429 || e.Code != httpx.CodeRateLimited || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("limited = %+v %v", e, rec.Header())
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/bytoken?token=a", nil, ""), 200)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/bytoken?token=b", nil, ""), 200)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/bytoken?token=a", nil, ""), 429)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/bytoken", nil, ""), 200) // empty key skips

	l := httpx.NewLimiter(httpx.Per(1, time.Minute), 1)
	if !l.Allow("k") || l.Allow("k") || l.Keys() != 1 || l.RetryAfter("k") <= 0 {
		t.Fatal("limiter")
	}
}

func TestGzipAndHeaders(t *testing.T) {
	a := testutil.NewTestApp(t)
	big := strings.Repeat("x", 4096)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.GET("/big", func(c *gin.Context) { c.Header("ETag", `"abc"`); httpx.OK(c, big) })
		r.GET("/small", func(c *gin.Context) { httpx.OK(c, true) })
		r.GET("/pre", func(c *gin.Context) {
			c.Header("Content-Encoding", "br")
			c.Data(200, "text/javascript", bytes.Repeat([]byte{1}, 4096))
		})
		r.GET("/png", func(c *gin.Context) { c.Data(200, "image/png", bytes.Repeat([]byte{1}, 4096)) })
		r.GET("/nocontent", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		r.GET("/panic", func(c *gin.Context) { panic("boom") })
	})
	gz := map[string]string{"Accept-Encoding": "gzip, br"}

	rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/big", nil, "", gz)
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" || rec.Header().Get("ETag") != `W/"abc"` {
		t.Fatalf("big headers = %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !bytes.Contains(plain, []byte(big)) {
		t.Fatal("gzip body mismatch")
	}

	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/big", nil, "", map[string]string{"Accept-Encoding": "gzip;q=0"})
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("q=0 must disable gzip")
	}
	for _, p := range []string{"/small", "/png"} {
		rec = testutil.DoWithHeaders(t, r, http.MethodGet, p, nil, "", gz)
		if rec.Header().Get("Content-Encoding") != "" || rec.Code != 200 {
			t.Fatalf("%s must not be compressed: %v", p, rec.Header())
		}
	}
	if testutil.Decode[bool](t, testutil.DoWithHeaders(t, r, http.MethodGet, "/small", nil, "", gz)) != true {
		t.Fatal("small body")
	}
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/pre", nil, "", gz)
	if rec.Header().Get("Content-Encoding") != "br" || rec.Body.Len() != 4096 {
		t.Fatal("pre-encoded responses must pass through")
	}
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/nocontent", nil, "", gz)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("204 = %d", rec.Code)
	}

	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/panic", nil, "", map[string]string{"X-Request-ID": "req-123"})
	if e := testutil.DecodeError(t, rec); e.Status != 500 || e.Code != httpx.CodeInternal {
		t.Fatalf("panic = %+v", e)
	}
	if rec.Header().Get("X-Request-ID") != "req-123" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("headers = %v", rec.Header())
	}
	// Security audit 2026-10-22 (h08.u5): without COOP a cross-origin opener could count the login page's frames.
	for _, p := range []string{"/panic", "/small", "/missing"} {
		if got := testutil.Do(t, r, http.MethodGet, p, nil, "").Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
			t.Fatalf("%s Cross-Origin-Opener-Policy = %q", p, got)
		}
	}
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/panic", nil, "", map[string]string{"X-Request-ID": "bad id with spaces"})
	if id := rec.Header().Get("X-Request-ID"); id == "" || strings.Contains(id, " ") {
		t.Fatalf("request id = %q", id)
	}
	if e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodGet, "/missing", nil, "")); e.Status != 404 || e.Code != httpx.CodeNotFound {
		t.Fatalf("404 = %+v", e)
	}
}

func TestCORS(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) { r.GET("/x", func(c *gin.Context) { httpx.OK(c, true) }) })
	rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/x", nil, "", map[string]string{"Origin": "https://other.example.com"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("same-origin by default")
	}
}

func TestCORSConfigured(t *testing.T) {
	a := testutil.NewTestApp(t)
	a.Cfg.Server.CORSOrigins = []string{"https://admin.example.com"}
	r := engine(t, a, func(r gin.IRouter, _ *app.App) { r.GET("/x", func(c *gin.Context) { httpx.OK(c, true) }) })
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://admin.example.com" {
		t.Fatalf("preflight = %d %v", rec.Code, rec.Header())
	}
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/x", nil, "", map[string]string{"Origin": "https://evil.example.com"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unlisted origin must not be allowed")
	}
}

func TestParamID(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := engine(t, a, func(r gin.IRouter, _ *app.App) {
		r.GET("/items/:id", func(c *gin.Context) {
			id, err := httpx.ParamID(c, "id")
			if err != nil {
				httpx.Fail(c, err)
				return
			}
			httpx.OK(c, id)
		})
	})
	if testutil.Decode[uint64](t, testutil.Do(t, r, http.MethodGet, "/items/42", nil, "")) != 42 {
		t.Fatal("ParamID")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/items/abc", nil, ""), 404)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/items/0", nil, ""), 404)
}
