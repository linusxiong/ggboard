package passport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const (
	rpID     = "panel.example.com"
	rpOrigin = "https://panel.example.com"
)

var bg = context.Background()

func setupMFA(t *testing.T) (*app.App, *gin.Engine, *time.Time) {
	t.Helper()
	a, r := setup(t, map[string]any{"app_url": rpOrigin + "/"})
	now := mfatest.FixClock(t, &mfasvc.TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	return a, r, now
}

// enableTOTP turns TOTP on for u and returns the secret and backup codes; the clock moves to the next step.
func enableTOTP(t *testing.T, a *app.App, u *model.User, now *time.Time) (string, []string) {
	t.Helper()
	s := mfasvc.New(a)
	st, err := s.SetupTOTP(bg, u)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.EnableTOTP(bg, u, mfatest.TOTP(t, st.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(30 * time.Second)
	return st.Secret, codes
}

// enrollPasskey registers a software passkey for u through the service.
func enrollPasskey(t *testing.T, a *app.App, u *model.User) *mfatest.Authenticator {
	t.Helper()
	s := mfasvc.New(a)
	cer, err := s.BeginRegistration(bg, u, "")
	if err != nil {
		t.Fatal(err)
	}
	opts, err := json.Marshal(cer.Options)
	if err != nil {
		t.Fatal(err)
	}
	key := mfatest.New(t, rpID, rpOrigin)
	if _, err := s.FinishRegistration(bg, u, cer.ChallengeID, key.Register(t, opts), ""); err != nil {
		t.Fatal(err)
	}
	return key
}

func passwordLogin(t *testing.T, r http.Handler, email string) map[string]json.RawMessage {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": email, "password": testutil.Password}, "")
	return testutil.Decode[map[string]json.RawMessage](t, rec)
}

func mfaToken(t *testing.T, r http.Handler, email string) string {
	t.Helper()
	body := passwordLogin(t, r, email)
	var ch mfasvc.LoginChallenge
	raw, _ := json.Marshal(body)
	if err := json.Unmarshal(raw, &ch); err != nil || !ch.MFARequired || ch.MFAToken == "" {
		t.Fatalf("challenge = %s", raw)
	}
	return ch.MFAToken
}

func expectCode(t *testing.T, r http.Handler, path string, body any, status int, code string) {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, base+path, body, "")
	e := testutil.DecodeError(t, rec)
	if e.Status != status || (code != "" && e.Code != code) {
		t.Fatalf("%s: got %d %q, want %d %q (%s)", path, e.Status, e.Code, status, code, rec.Body.String())
	}
}

func TestLoginBranchesIntoMFA(t *testing.T) {
	a, r, now := setupMFA(t)
	u := testutil.CreateUser(t, a, "alice@example.com")

	// No TOTP: unchanged {token, user}.
	if body := passwordLogin(t, r, u.Email); body["token"] == nil || body["user"] == nil || body["mfa_required"] != nil {
		t.Fatalf("plain login = %v", body)
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("last_login_at", nil).Error; err != nil {
		t.Fatal(err)
	}

	secret, _ := enableTOTP(t, a, u, now)
	body := passwordLogin(t, r, u.Email)
	if body["token"] != nil || body["user"] != nil || string(body["mfa_required"]) != "true" ||
		string(body["methods"]) != `["totp"]` || string(body["has_passkey"]) != "false" {
		t.Fatalf("mfa login = %v", body)
	}
	var tok string
	_ = json.Unmarshal(body["mfa_token"], &tok)
	if loadUser(t, a, u.ID).LastLoginAt != nil {
		t.Fatal("the password step must not record the login")
	}

	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok}, 400, "validation_failed")
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": "000000"}, 401, mfasvc.CodeInvalid)
	rec := testutil.Do(t, r, http.MethodPost, base+"/login/mfa",
		map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, "")
	res := testutil.Decode[passport.AuthResult](t, rec)
	if res.Token == "" || res.User.ID != u.ID || res.User.LastLoginAt == nil {
		t.Fatalf("mfa result = %+v", res)
	}
	if claims, err := auth.Parse(a, res.Token); err != nil || claims.Subject == "" {
		t.Fatalf("session token: %v", err)
	}
	if got := loadUser(t, a, u.ID); got.LastLoginAt == nil || got.LastLoginIP == "" {
		t.Fatal("second step must record the login")
	}
	// Single use.
	*now = now.Add(30 * time.Second)
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, 401, mfasvc.CodeExpired)
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": "unknown", "code": "123456"}, 401, mfasvc.CodeExpired)

	// A passkey shows up in has_passkey; wrong password still 401 before any MFA step.
	enrollPasskey(t, a, u)
	if body := passwordLogin(t, r, u.Email); string(body["has_passkey"]) != "true" {
		t.Fatalf("has_passkey = %v", body)
	}
	expectCode(t, r, "/login", map[string]any{"email": u.Email, "password": "wrong-password"}, 401, "")
}

func TestLoginMFABackupCodesAndExpiry(t *testing.T) {
	a, r, now := setupMFA(t)
	u := testutil.CreateUser(t, a, "bob@example.com")
	secret, codes := enableTOTP(t, a, u, now)

	tok := mfaToken(t, r, u.Email)
	rec := testutil.Do(t, r, http.MethodPost, base+"/login/mfa", map[string]any{"mfa_token": tok, "code": strings.ToUpper(codes[0])}, "")
	if res := testutil.Decode[passport.AuthResult](t, rec); res.Token == "" {
		t.Fatal("backup code login failed")
	}
	tok = mfaToken(t, r, u.Email)
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": codes[0]}, 401, mfasvc.CodeInvalid)
	ov, err := mfasvc.New(a).Overview(bg, u.ID)
	if err != nil || ov.BackupCodesRemaining != 9 {
		t.Fatalf("remaining = %d %v", ov.BackupCodesRemaining, err)
	}

	// The token expires (KV entry of internal/service/mfa: "mfa:token:<token>").
	v, ok := a.KV.Get("mfa:token:" + tok)
	if !ok {
		t.Fatal("token not in KV")
	}
	a.KV.Set("mfa:token:"+tok, v, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, 401, mfasvc.CodeExpired)

	// A password change revokes outstanding tokens.
	tok = mfaToken(t, r, u.Email)
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", 5).Error; err != nil {
		t.Fatal(err)
	}
	auth.InvalidateUser(a, u.ID)
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, 401, mfasvc.CodeExpired)
}

func TestLoginMFAAttemptLimitAndBan(t *testing.T) {
	a, r, now := setupMFA(t)
	u := testutil.CreateUser(t, a, "carol@example.com")
	secret, _ := enableTOTP(t, a, u, now)

	tok := mfaToken(t, r, u.Email)
	for range mfasvc.MaxTokenAttempts - 1 {
		expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": "000000"}, 401, mfasvc.CodeInvalid)
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/login/mfa", map[string]any{"mfa_token": tok, "code": "000000"}, "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("5th wrong code = %d %s", rec.Code, rec.Body.String())
	}
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, 401, mfasvc.CodeExpired)

	tok = mfaToken(t, r, u.Email)
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	expectCode(t, r, "/login/mfa", map[string]any{"mfa_token": tok, "code": mfatest.TOTP(t, secret, *now)}, 403, "")
}

type ceremony struct {
	ChallengeID string          `json:"challenge_id"`
	Options     json.RawMessage `json:"options"`
}

func beginLogin(t *testing.T, r http.Handler, body any) ceremony {
	t.Helper()
	return testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, base+"/passkey/login/begin", body, ""))
}

func finishLogin(t *testing.T, r http.Handler, cer ceremony, credential json.RawMessage) *httpResult {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": credential}, "")
	return &httpResult{code: rec.Code, body: rec.Body.String()}
}

type httpResult struct {
	code int
	body string
}

func TestPasskeyDiscoverableLogin(t *testing.T) {
	a, r, now := setupMFA(t)
	u := testutil.CreateUser(t, a, "dave@example.com")
	enableTOTP(t, a, u, now) // passkey login never needs the second step
	key := enrollPasskey(t, a, u)

	cer := beginLogin(t, r, map[string]any{})
	var opts map[string]any
	if err := json.Unmarshal(cer.Options, &opts); err != nil {
		t.Fatal(err)
	}
	if cer.ChallengeID == "" || opts["challenge"] == "" || opts["rpId"] != rpID || opts["userVerification"] != "preferred" ||
		opts["timeout"] != float64(120000) || opts["allowCredentials"] != nil {
		t.Fatalf("discoverable options = %s", cer.Options)
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)}, "")
	res := testutil.Decode[passport.AuthResult](t, rec)
	if res.Token == "" || res.User.ID != u.ID || res.User.LastLoginAt == nil {
		t.Fatalf("passkey login = %+v", res)
	}
	var pk model.Passkey
	if err := a.DB.Where("user_id = ?", u.ID).First(&pk).Error; err != nil || pk.SignCount != 1 || pk.LastUsedAt == nil {
		t.Fatalf("passkey after login = %+v %v", pk, err)
	}

	// The challenge is single use.
	if res := finishLogin(t, r, cer, key.Assert(t, cer.Options)); res.code != 401 {
		t.Fatalf("reused challenge = %+v", res)
	}
	key.SignCount = 1 // the failed attempt did not reach the server's counter

	// Sign counter regression (cloned authenticator) is rejected.
	cer = beginLogin(t, r, nil)
	key.SignCount = 0 // next assertion carries 1 == stored 1
	if res := finishLogin(t, r, cer, key.Assert(t, cer.Options)); res.code != 401 {
		t.Fatalf("counter regression = %+v", res)
	}
	key.SignCount = 5
	cer = beginLogin(t, r, nil)
	if res := finishLogin(t, r, cer, key.Assert(t, cer.Options)); res.code != 200 {
		t.Fatalf("counter advance = %+v", res)
	}

	// Wrong origin, unknown credential and malformed bodies fail.
	evil := *key
	evil.Origin = "https://evil.example.org"
	cer = beginLogin(t, r, nil)
	if res := finishLogin(t, r, cer, evil.Assert(t, cer.Options)); res.code != 401 {
		t.Fatalf("wrong origin = %+v", res)
	}
	stranger := mfatest.New(t, rpID, rpOrigin)
	stranger.UserHandle = key.UserHandle
	cer = beginLogin(t, r, nil)
	if res := finishLogin(t, r, cer, stranger.Assert(t, cer.Options)); res.code != 401 {
		t.Fatalf("unknown credential = %+v", res)
	}
	cer = beginLogin(t, r, nil)
	if res := finishLogin(t, r, cer, json.RawMessage(`{"id":"x"}`)); res.code != 400 {
		t.Fatalf("malformed = %+v", res)
	}
	expectCode(t, r, "/passkey/login/finish", map[string]any{"challenge_id": cer.ChallengeID}, 400, "validation_failed")

	// Banned users are refused after verification.
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	cer = beginLogin(t, r, nil)
	if res := finishLogin(t, r, cer, key.Assert(t, cer.Options)); res.code != 403 {
		t.Fatalf("banned = %+v", res)
	}
}

func TestPasskeyEmailLogin(t *testing.T) {
	a, r, _ := setupMFA(t)
	u := testutil.CreateUser(t, a, "erin@example.com")
	other := testutil.CreateUser(t, a, "frank@example.com")
	key := enrollPasskey(t, a, u)
	otherKey := enrollPasskey(t, a, other)

	cer := beginLogin(t, r, map[string]any{"email": "Erin@Example.com"})
	var opts struct {
		Challenge        string `json:"challenge"`
		AllowCredentials []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"allowCredentials"`
	}
	if err := json.Unmarshal(cer.Options, &opts); err != nil {
		t.Fatal(err)
	}
	if len(opts.AllowCredentials) != 1 || opts.AllowCredentials[0].ID != key.ID() || opts.AllowCredentials[0].Type != "public-key" {
		t.Fatalf("email options = %s", cer.Options)
	}
	// Another user's passkey cannot answer a challenge scoped to this email.
	if res := finishLogin(t, r, cer, otherKey.Assert(t, cer.Options)); res.code != 401 {
		t.Fatalf("foreign passkey = %+v", res)
	}
	cer = beginLogin(t, r, map[string]any{"email": u.Email})
	rec := testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)}, "")
	if res := testutil.Decode[passport.AuthResult](t, rec); res.User.ID != u.ID {
		t.Fatalf("email login = %+v", res)
	}

	// Unknown emails (and users without passkeys) get discoverable options: nothing reveals the account.
	for _, email := range []string{"nobody@example.com", testutil.CreateUser(t, a, "gina@example.com").Email} {
		cer := beginLogin(t, r, map[string]any{"email": email})
		var o map[string]any
		_ = json.Unmarshal(cer.Options, &o)
		if o["allowCredentials"] != nil || o["challenge"] == "" || cer.ChallengeID == "" {
			t.Fatalf("%s options = %s", email, cer.Options)
		}
	}
	expectCode(t, r, "/passkey/login/begin", map[string]any{"email": "not-an-email"}, 400, "validation_failed")
}

func TestPasskeyLoginDisabled(t *testing.T) {
	_, r := setup(t, nil)
	expectCode(t, r, "/passkey/login/begin", map[string]any{}, 400, "")
	expectCode(t, r, "/passkey/login/finish", map[string]any{"challenge_id": "nope", "credential": map[string]any{}}, 401, "")
}

func TestWellKnownURLs(t *testing.T) {
	_, r := setup(t, map[string]any{"app_url": "https://panel.example.com/"})
	rec := testutil.Do(t, r, http.MethodGet, "/.well-known/change-password", nil, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://panel.example.com/profile?tab=security" {
		t.Fatalf("change-password = %d %v", rec.Code, rec.Header())
	}
	rec = testutil.Do(t, r, http.MethodGet, "/.well-known/passkey-endpoints", nil, "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" ||
		rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("passkey-endpoints = %d %v", rec.Code, rec.Header())
	}
	var ep passport.PasskeyEndpoints
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil ||
		ep.Enroll != "https://panel.example.com/profile?tab=security" || ep.Manage != ep.Enroll {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// Without app_url the request origin is used; X-Forwarded-Proto only counts from a trusted proxy
	// (review 2026-10-22: anyone could claim https).
	_, r = setup(t, nil)
	rec = testutil.DoWithHeaders(t, r, http.MethodGet, "/.well-known/change-password", nil, "",
		map[string]string{"X-Forwarded-Proto": "https"})
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || loc != "http://example.com/profile?tab=security" {
		t.Fatalf("change-password without app_url, untrusted proxy = %d %q", rec.Code, loc)
	}
	ta := testutil.NewTestApp(t)
	ta.Cfg.Server.TrustedProxies = []string{"192.0.2.0/24"} // httptest's RemoteAddr is 192.0.2.1
	tr := testutil.NewRouterWith(t, ta, passport.Register)
	rec = testutil.DoWithHeaders(t, tr, http.MethodGet, "/.well-known/change-password", nil, "",
		map[string]string{"X-Forwarded-Proto": "https"})
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || loc != "https://example.com/profile?tab=security" {
		t.Fatalf("change-password without app_url, trusted proxy = %d %q", rec.Code, loc)
	}
	rec = testutil.Do(t, r, http.MethodGet, "/.well-known/passkey-endpoints", nil, "")
	if !strings.Contains(rec.Body.String(), `"enroll":"http://example.com/profile?tab=security"`) {
		t.Fatalf("passkey-endpoints without app_url = %s", rec.Body.String())
	}
}
