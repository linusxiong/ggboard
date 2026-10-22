// Package testutil builds isolated Apps, routers, users and HTTP requests for tests.
//
//	a := testutil.NewTestApp(t)
//	r := testutil.NewRouterWith(t, a, user.Register)
//	u := testutil.CreateUser(t, a, "alice@example.com")
//	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/me", nil, testutil.Login(t, a, u))
//	me := testutil.Decode[model.User](t, rec)
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Password is the default password of users created by CreateUser.
const Password = "password123"

// JWTSecret is the JWT secret of test apps.
const JWTSecret = "test-secret-0123456789abcdef0123456789abcdef"

var fastHashOnce sync.Once

// FastHashing switches argon2id to cheap parameters for the whole test binary (NewTestApp calls it).
func FastHashing() {
	fastHashOnce.Do(func() {
		auth.SetParams(auth.Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	})
}

type options struct {
	cfg      []func(*config.Config)
	settings map[string]any
}

// Option customizes NewTestApp.
type Option func(*options)

// WithConfig mutates the config before the App is built.
func WithConfig(fn func(*config.Config)) Option {
	return func(o *options) { o.cfg = append(o.cfg, fn) }
}

// WithSettings persists settings after the App is built.
func WithSettings(values map[string]any) Option {
	return func(o *options) {
		if o.settings == nil {
			o.settings = map[string]any{}
		}
		for k, v := range values {
			o.settings[k] = v
		}
	}
}

var mailboxes sync.Map // *app.App -> *mail.Memory

// NewTestApp returns a migrated App on a private SQLite database in t.TempDir() (WAL, so tests behave like
// production: concurrent readers, busy_timeout for writers), default settings, a started mail queue that
// delivers to an in-memory mailbox (see Mails) and no scheduler. Everything is closed by t.Cleanup.
// Set GGBOARD_TEST_LOG=1 to print logs.
func NewTestApp(t testing.TB, opts ...Option) *app.App {
	t.Helper()
	FastHashing()
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	dir := t.TempDir()
	cfg := &config.Config{
		Server:   config.ServerConfig{Addr: "127.0.0.1:0"},
		Database: config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(dir, "test.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(OFF)&_pragma=foreign_keys(1)"},
		JWT:      config.JWTConfig{Secret: JWTSecret, TTL: time.Hour},
		Log:      config.LogConfig{Level: "debug", Format: "text"},
		DataDir:  dir,
	}
	for _, fn := range o.cfg {
		fn(cfg)
	}
	ring := logx.NewRing(0)
	w := io.Discard
	if os.Getenv("GGBOARD_TEST_LOG") != "" {
		w = os.Stderr
	}
	log, err := logx.New(cfg.Log.Level, cfg.Log.Format, w, ring)
	if err != nil {
		t.Fatalf("testutil: logger: %v", err)
	}
	box := &mail.Memory{}
	a, err := app.New(cfg,
		app.WithLogger(log, ring),
		app.WithVersion("test"),
		app.WithMailer(box),
		app.WithMailQueueOptions(mail.QueueOptions{Retries: 2, Backoff: func(int) time.Duration { return time.Millisecond }}),
	)
	if err != nil {
		t.Fatalf("testutil: new app: %v", err)
	}
	mailboxes.Store(a, box)
	t.Cleanup(func() {
		mailboxes.Delete(a)
		if err := a.Close(context.Background()); err != nil {
			t.Errorf("testutil: close app: %v", err)
		}
	})
	if len(o.settings) > 0 {
		if err := a.Settings.Set(o.settings); err != nil {
			t.Fatalf("testutil: settings: %v", err)
		}
	}
	a.Mailer.Start()
	return a
}

// Mailbox returns the in-memory mailbox of a test App.
func Mailbox(t testing.TB, a *app.App) *mail.Memory {
	t.Helper()
	v, ok := mailboxes.Load(a)
	if !ok {
		t.Fatal("testutil: app was not created by NewTestApp")
	}
	return v.(*mail.Memory)
}

// Mails waits for the mail queue to drain and returns every delivered message.
func Mails(t testing.TB, a *app.App) []mail.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Mailer.Wait(ctx); err != nil {
		t.Fatalf("testutil: mail queue did not drain: %v", err)
	}
	return Mailbox(t, a).Messages()
}

// NewRouterWith returns an engine with the standard middleware and only the given route packages.
func NewRouterWith(t testing.TB, a *app.App, fns ...httpx.RegisterFunc) *gin.Engine {
	t.Helper()
	e := httpx.NewEngine(a)
	for _, fn := range fns {
		fn(e, a)
	}
	return e
}

// CreateUser creates a user with Password. Optional opts[0] sets plan, group, traffic, expiry, roles...
func CreateUser(t testing.TB, a *app.App, email string, opts ...usersvc.CreateOptions) *model.User {
	t.Helper()
	var o usersvc.CreateOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	u, err := usersvc.New(a).Create(context.Background(), email, Password, o)
	if err != nil {
		t.Fatalf("testutil: create user %s: %v", email, err)
	}
	return u
}

// CreateAdmin creates an administrator.
func CreateAdmin(t testing.TB, a *app.App, email string) *model.User {
	t.Helper()
	return CreateUser(t, a, email, usersvc.CreateOptions{IsAdmin: true})
}

// CreateStaff creates a staff member (not admin).
func CreateStaff(t testing.TB, a *app.App, email string) *model.User {
	t.Helper()
	return CreateUser(t, a, email, usersvc.CreateOptions{IsStaff: true})
}

// Login returns a session JWT for u.
func Login(t testing.TB, a *app.App, u *model.User) string {
	t.Helper()
	tok, err := auth.Issue(a, u)
	if err != nil {
		t.Fatalf("testutil: issue token: %v", err)
	}
	return tok
}

// Do performs a request against h. body may be nil, a string/[]byte (sent as-is with JSON content type),
// url.Values (form) or any value (JSON-encoded). token, when set, is sent as a Bearer token.
func Do(t testing.TB, h http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	return DoWithHeaders(t, h, method, path, body, token, nil)
}

// DoWithHeaders is Do with extra request headers.
func DoWithHeaders(t testing.TB, h http.Handler, method, path string, body any, token string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var (
		r  io.Reader
		ct string
	)
	switch b := body.(type) {
	case nil:
	case string:
		r, ct = strings.NewReader(b), "application/json"
	case []byte:
		r, ct = bytes.NewReader(b), "application/json"
	case url.Values:
		r, ct = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("testutil: encode body: %v", err)
		}
		r, ct = bytes.NewReader(buf), "application/json"
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, r)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Decode asserts a 2xx response and decodes {"data": T}.
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	decode(t, rec, &env)
	return env.Data
}

// DecodeList asserts a 2xx response and decodes {"data": []T, "total": N}.
func DecodeList[T any](t testing.TB, rec *httptest.ResponseRecorder) ([]T, int64) {
	t.Helper()
	var env struct {
		Data  []T   `json:"data"`
		Total int64 `json:"total"`
	}
	decode(t, rec, &env)
	return env.Data, env.Total
}

func decode(t testing.TB, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if rec.Code < 200 || rec.Code > 299 {
		t.Fatalf("testutil: status %d, body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("testutil: decode %q: %v", rec.Body.String(), err)
	}
}

// DecodeError decodes an error response ({"code","message","errors"}) and fills Status.
func DecodeError(t testing.TB, rec *httptest.ResponseRecorder) httpx.Error {
	t.Helper()
	var e httpx.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("testutil: decode error %q: %v", rec.Body.String(), err)
	}
	e.Status = rec.Code
	return e
}

// ExpectStatus fails the test unless rec has status code want.
func ExpectStatus(t testing.TB, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

// Ptr returns a pointer to v (handy for optional model fields).
func Ptr[T any](v T) *T { return &v }

// MustJSON marshals v or fails the test.
func MustJSON(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("testutil: %v", fmt.Errorf("marshal: %w", err))
	}
	return string(b)
}
