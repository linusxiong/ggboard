package mfasvc

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/testutil"
)

var ctx = context.Background()

func status(err error) int {
	if err == nil {
		return 0
	}
	return httpx.AsError(err).Status
}

func code(err error) string {
	if err == nil {
		return ""
	}
	return httpx.AsError(err).Code
}

// enable sets up TOTP for u at the fixed clock and returns the secret and backup codes.
func enable(t *testing.T, a *app.App, u *model.User, now *time.Time) (string, []string) {
	t.Helper()
	s := New(a)
	setup, err := s.SetupTOTP(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.EnableTOTP(ctx, u, mfatest.TOTP(t, setup.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(30 * time.Second) // the next code is a fresh step
	return setup.Secret, codes
}

func TestSettingsKeys(t *testing.T) {
	for k, want := range map[string]any{"passkey_rp_id": "", "admin_mfa_required": false, "mfa_issuer": ""} {
		d, ok := settings.Lookup(k)
		if !ok || d.Group != settings.GroupSecurity || settings.Defaults[k] != want {
			t.Errorf("%s: def %+v default %v", k, d, settings.Defaults[k])
		}
	}
	if d, ok := settings.Lookup("passkey_origins"); !ok || d.Type != settings.TypeStrings || d.Group != settings.GroupSecurity {
		t.Fatalf("passkey_origins = %+v", d)
	}
	a := testutil.NewTestApp(t)
	if got := a.Settings.AllGrouped()["security"]; len(got) != 4 {
		t.Fatalf("security group = %v", got)
	}
}

func TestSecretEncryptionRoundTrip(t *testing.T) {
	key, err := deriveKey(testutil.JWTSecret)
	if err != nil || len(key) != 32 {
		t.Fatalf("key = %x, %v", key, err)
	}
	if again, _ := deriveKey(testutil.JWTSecret); string(again) != string(key) {
		t.Fatal("HKDF must be deterministic")
	}
	if other, _ := deriveKey("another-secret"); string(other) == string(key) {
		t.Fatal("different secrets must give different keys")
	}
	enc, err := encryptSecret(key, 7, "JBSWY3DPEHPK3PXP")
	if err != nil || !strings.HasPrefix(enc, "v1.") || strings.Contains(enc, "JBSWY3DPEHPK3PXP") {
		t.Fatalf("enc = %q, %v", enc, err)
	}
	enc2, _ := encryptSecret(key, 7, "JBSWY3DPEHPK3PXP")
	if enc == enc2 {
		t.Fatal("nonces must be random")
	}
	if plain, err := decryptSecret(key, 7, enc); err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("plain = %q, %v", plain, err)
	}
	if _, err := decryptSecret(key, 8, enc); err == nil {
		t.Fatal("ciphertext must be bound to the user id")
	}
	other, _ := deriveKey("another-secret")
	if _, err := decryptSecret(other, 7, enc); err == nil {
		t.Fatal("wrong key must fail")
	}
	for _, bad := range []string{"", "v1.", "v1.!!!", "v2." + enc[3:], "v1.AAAA"} {
		if _, err := decryptSecret(key, 7, bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
	if _, err := deriveKey(""); err == nil {
		t.Fatal("empty jwt secret must fail")
	}
}

func TestBackupCodeFormat(t *testing.T) {
	codes, hashes := newBackupCodes()
	if len(codes) != BackupCodeCount || len(hashes) != BackupCodeCount {
		t.Fatalf("codes = %v", codes)
	}
	re := regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{5}-[0-9a-hjkmnp-tv-z]{5}$`)
	seen := map[string]bool{}
	for i, c := range codes {
		if !re.MatchString(c) || seen[c] {
			t.Fatalf("code %q", c)
		}
		seen[c] = true
		n, ok := normalizeBackup(" " + strings.ToUpper(c) + " ")
		if !ok || hashBackup(n) != hashes[i] {
			t.Fatalf("normalize %q = %q %v", c, n, ok)
		}
	}
	for _, bad := range []string{"", "12345", "abcde-fghij-k", "iiiii-ooooo", "123456"} {
		if _, ok := normalizeBackup(bad); ok {
			t.Errorf("%q must not be a backup code", bad)
		}
	}
	for in, want := range map[string]bool{"123456": true, " 123 456 ": true, "12345a": false, "1234567": false} {
		if _, ok := normalizeTOTP(in); ok != want {
			t.Errorf("normalizeTOTP(%q) = %v", in, ok)
		}
	}
}

func TestRelyingParty(t *testing.T) {
	a := testutil.NewTestApp(t)
	if PasskeyEnabled(a.Settings) {
		t.Fatal("no app_url: passkeys must be disabled")
	}
	if _, _, err := New(a).webAuthn(); status(err) != http.StatusBadRequest {
		t.Fatalf("webAuthn without RP = %v", err)
	}
	cases := []struct {
		set     map[string]any
		id      string
		origins []string
	}{
		{map[string]any{"app_url": "https://Panel.Example.com:8443/app/"}, "panel.example.com", []string{"https://panel.example.com:8443"}},
		{map[string]any{"app_url": "panel.example.com"}, "panel.example.com", []string{"https://panel.example.com"}},
		{map[string]any{"app_url": "https://panel.example.com", "passkey_rp_id": "example.com",
			"passkey_origins": []string{"https://www.example.com/", "https://panel.example.com"}},
			"example.com", []string{"https://panel.example.com", "https://www.example.com"}},
		{map[string]any{"app_url": "", "passkey_rp_id": "example.com"}, "example.com", []string{"https://example.com"}},
		{map[string]any{"app_url": "http://localhost:5173", "passkey_rp_id": ""}, "localhost", []string{"http://localhost:5173"}},
	}
	for _, tc := range cases {
		if err := a.Settings.Set(map[string]any{"app_url": nil, "passkey_rp_id": nil, "passkey_origins": nil}); err != nil {
			t.Fatal(err)
		}
		if err := a.Settings.Set(tc.set); err != nil {
			t.Fatal(err)
		}
		rp, ok := RPFromSettings(a.Settings)
		if !ok || rp.ID != tc.id || !slices.Equal(rp.Origins, tc.origins) || rp.Name != "GG Bond" || !PasskeyEnabled(a.Settings) {
			t.Errorf("%v: rp = %+v %v", tc.set, rp, ok)
		}
		if _, _, err := New(a).webAuthn(); err != nil {
			t.Errorf("%v: webauthn: %v", tc.set, err)
		}
	}
	if err := a.Settings.Set(map[string]any{"app_url": "https://panel.example.com/base/", "passkey_rp_id": nil}); err != nil {
		t.Fatal(err)
	}
	if AppBaseURL(a.Settings) != "https://panel.example.com/base" {
		t.Fatalf("base = %q", AppBaseURL(a.Settings))
	}
	if err := a.Settings.Set(map[string]any{"app_url": "ftp://example.com"}); err != nil {
		t.Fatal(err)
	}
	if PasskeyEnabled(a.Settings) || AppBaseURL(a.Settings) != "" {
		t.Fatal("non-http app_url must not enable passkeys")
	}
	// An IP address is never a valid RP id (go-webauthn rejects it): passkeys are off, not a 500.
	for _, set := range []map[string]any{
		{"app_url": "http://127.0.0.1:18090", "passkey_rp_id": nil},
		{"app_url": "http://[::1]:8080", "passkey_rp_id": nil},
		{"app_url": "https://panel.example.com", "passkey_rp_id": "192.0.2.1"},
	} {
		if err := a.Settings.Set(set); err != nil {
			t.Fatal(err)
		}
		if PasskeyEnabled(a.Settings) {
			t.Errorf("%v: IP rp id must disable passkeys", set)
		}
		if _, _, err := New(a).webAuthn(); status(err) != http.StatusBadRequest {
			t.Errorf("%v: webauthn err = %v, want 400", set, err)
		}
	}
}

func TestTOTPLifecycle(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"mfa_issuer": "Example Panel"}))
	now := mfatest.FixClock(t, &TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	u := testutil.CreateUser(t, a, "alice@example.com")
	s := New(a)

	setup, err := s.SetupTOTP(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.OTPAuthURL, "otpauth://totp/Example%20Panel:alice@example.com?") ||
		!strings.Contains(setup.OTPAuthURL, "secret="+setup.Secret) || len(setup.Secret) < 32 {
		t.Fatalf("setup = %+v", setup)
	}
	// Wrong codes are field errors; the pending secret survives until MaxSetupAttempts.
	if _, err := s.EnableTOTP(ctx, u, "abc"); status(err) != http.StatusBadRequest {
		t.Fatalf("malformed = %v", err)
	}
	codes, err := s.EnableTOTP(ctx, u, mfatest.TOTP(t, setup.Secret, now.Add(-30*time.Second))) // skew -1
	if err != nil || len(codes) != BackupCodeCount {
		t.Fatalf("enable = %v %v", codes, err)
	}
	if _, err := s.SetupTOTP(ctx, u); status(err) != http.StatusConflict {
		t.Fatalf("setup when enabled = %v", err)
	}
	var row model.UserMFA
	if err := a.DB.First(&row, "user_id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !row.TOTPEnabled || row.TOTPEnabledAt == nil || strings.Contains(row.TOTPSecret, setup.Secret) || len(row.BackupCodes) != 10 {
		t.Fatalf("row = %+v", row)
	}
	for _, h := range row.BackupCodes {
		if slices.Contains(codes, h) {
			t.Fatal("backup codes must be stored hashed")
		}
	}

	// Replay: the step used to enable (now-30s) and older steps are rejected; current and +1 are accepted once.
	ok, err := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, now.Add(-30*time.Second)))
	if err != nil || ok {
		t.Fatalf("replayed step accepted: %v %v", ok, err)
	}
	if ok, _ := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, *now)); !ok {
		t.Fatal("current step rejected")
	}
	if ok, _ := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, *now)); ok {
		t.Fatal("same code accepted twice")
	}
	if ok, _ := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, now.Add(30*time.Second))); !ok {
		t.Fatal("step +1 rejected")
	}
	*now = now.Add(10 * time.Minute)
	if ok, _ := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, now.Add(-60*time.Second))); ok {
		t.Fatal("step -2 accepted")
	}
	if ok, _ := s.verifyCode(ctx, &row, mfatest.TOTP(t, setup.Secret, now.Add(60*time.Second))); ok {
		t.Fatal("step +2 accepted")
	}

	// Backup codes are single use and accepted in any case / without the dash.
	bc := strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))
	if ok, err := s.verifyCode(ctx, &row, bc); err != nil || !ok {
		t.Fatalf("backup code: %v %v", ok, err)
	}
	if ok, _ := s.verifyCode(ctx, &row, codes[0]); ok {
		t.Fatal("backup code reused")
	}
	ov, err := s.Overview(ctx, u.ID)
	if err != nil || !ov.TOTPEnabled || ov.BackupCodesRemaining != 9 || ov.TOTPEnabledAt == nil || ov.Passkeys == nil {
		t.Fatalf("overview = %+v %v", ov, err)
	}

	// Regenerate replaces every code.
	*now = now.Add(time.Minute)
	fresh, err := s.RegenerateBackupCodes(ctx, u, mfatest.TOTP(t, setup.Secret, *now))
	if err != nil || len(fresh) != 10 || slices.Contains(fresh, codes[1]) {
		t.Fatalf("regenerate = %v %v", fresh, err)
	}
	if err := a.DB.First(&row, "user_id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.verifyCode(ctx, &row, codes[1]); ok {
		t.Fatal("old backup code survived regeneration")
	}

	// Disable needs the password and a code.
	*now = now.Add(time.Minute)
	if err := s.DisableTOTP(ctx, u, "wrong-password", mfatest.TOTP(t, setup.Secret, *now)); status(err) != http.StatusBadRequest ||
		len(httpx.AsError(err).Errors["password"]) == 0 {
		t.Fatalf("wrong password = %v", err)
	}
	if err := s.DisableTOTP(ctx, u, testutil.Password, "000000"); status(err) != http.StatusBadRequest ||
		len(httpx.AsError(err).Errors["code"]) == 0 {
		t.Fatalf("wrong code = %v", err)
	}
	if err := s.DisableTOTP(ctx, u, testutil.Password, fresh[3]); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.TOTPEnabled(ctx, u.ID); on {
		t.Fatal("still enabled")
	}
	if err := a.DB.First(&row, "user_id = ?", u.ID).Error; err != nil || row.TOTPSecret != "" || len(row.BackupCodes) != 0 {
		t.Fatalf("row after disable = %+v %v", row, err)
	}
	if err := s.DisableTOTP(ctx, u, testutil.Password, "123456"); status(err) != http.StatusConflict {
		t.Fatalf("disable twice = %v", err)
	}
	if _, err := s.RegenerateBackupCodes(ctx, u, "123456"); status(err) != http.StatusConflict {
		t.Fatalf("regenerate when disabled = %v", err)
	}
}

func TestEnableAttemptsAndPending(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "bob@example.com")
	s := New(a)
	if _, err := s.EnableTOTP(ctx, u, "123456"); status(err) != http.StatusBadRequest {
		t.Fatalf("enable without setup = %v", err)
	}
	if _, err := s.SetupTOTP(ctx, u); err != nil {
		t.Fatal(err)
	}
	for range MaxSetupAttempts {
		if _, err := s.EnableTOTP(ctx, u, "12345x"); status(err) != http.StatusBadRequest {
			t.Fatalf("wrong = %v", err)
		}
	}
	if _, ok := a.KV.Get(idKey(kvSetup, u.ID)); ok {
		t.Fatal("pending secret must be discarded after MaxSetupAttempts")
	}
}

func TestUserFailureLimit(t *testing.T) {
	a := testutil.NewTestApp(t)
	now := mfatest.FixClock(t, &TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	u := testutil.CreateUser(t, a, "carol@example.com")
	enable(t, a, u, now)
	s := New(a)
	for range UserFailureLimit {
		if _, err := s.RegenerateBackupCodes(ctx, u, "00000-00000"); status(err) != http.StatusBadRequest {
			t.Fatalf("wrong = %v", err)
		}
	}
	_, err := s.RegenerateBackupCodes(ctx, u, "00000-00000")
	var re *account.RetryError
	if status(err) != http.StatusTooManyRequests || !errors.As(err, &re) || re.RetryAfterSeconds() < 1 {
		t.Fatalf("locked = %v", err)
	}
}

func TestLoginToken(t *testing.T) {
	a := testutil.NewTestApp(t)
	now := mfatest.FixClock(t, &TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	u := testutil.CreateUser(t, a, "dave@example.com")
	s := New(a)
	if ch, err := s.Challenge(ctx, u); err != nil || ch != nil {
		t.Fatalf("challenge without totp = %+v %v", ch, err)
	}
	secret, _ := enable(t, a, u, now)
	ch, err := s.Challenge(ctx, u)
	if err != nil || ch == nil || !ch.MFARequired || len(ch.MFAToken) != 64 || !slices.Equal(ch.Methods, []string{"totp"}) || ch.HasPasskey {
		t.Fatalf("challenge = %+v %v", ch, err)
	}
	// Expired token.
	v, _ := a.KV.Get(kvToken + ch.MFAToken)
	p := v.(pendingLogin)
	p.Expires = time.Now().Add(-time.Second)
	a.KV.Set(kvToken+ch.MFAToken, p, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, err := s.CompleteLogin(ctx, ch.MFAToken, mfatest.TOTP(t, secret, *now)); code(err) != CodeExpired {
		t.Fatalf("expired = %v", err)
	}
	// Token bound to password_version.
	ch, _ = s.Challenge(ctx, u)
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteLogin(ctx, ch.MFAToken, mfatest.TOTP(t, secret, *now)); code(err) != CodeExpired {
		t.Fatalf("stale version = %v", err)
	}
	if _, err := s.CompleteLogin(ctx, "", "123456"); code(err) != CodeExpired {
		t.Fatalf("empty token = %v", err)
	}
}

func TestAdminGate(t *testing.T) {
	a := testutil.NewTestApp(t)
	now := mfatest.FixClock(t, &TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	r := testutil.NewRouterWith(t, a, func(r gin.IRouter, a *app.App) {
		g := r.Group("/api/v1/admin", auth.RequireStaff(a), AdminGate(a))
		g.GET("/ping", func(c *gin.Context) { httpx.OK(c, true) })
	})
	admin := testutil.CreateAdmin(t, a, "root@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	user := testutil.CreateUser(t, a, "user@example.com")
	ping := func(u *model.User) int {
		return testutil.Do(t, r, http.MethodGet, "/api/v1/admin/ping", nil, testutil.Login(t, a, u)).Code
	}
	if ping(admin) != 200 || ping(staff) != 200 || ping(user) != 403 {
		t.Fatal("gate must be open while admin_mfa_required is off")
	}
	if err := a.Settings.Set(map[string]any{"admin_mfa_required": true}); err != nil {
		t.Fatal(err)
	}
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/admin/ping", nil, testutil.Login(t, a, admin))
	if e := testutil.DecodeError(t, rec); e.Status != 403 || e.Code != CodeSetupRequired {
		t.Fatalf("admin without mfa = %+v", e)
	}
	if ping(staff) != 403 {
		t.Fatal("staff without mfa must be blocked")
	}
	// TOTP opens the gate immediately (cache dropped on change).
	enable(t, a, admin, now)
	if ping(admin) != 200 {
		t.Fatal("admin with totp must pass")
	}
	// A passkey counts too.
	if err := a.DB.Create(&model.Passkey{UserID: staff.ID, RPID: "example.com", CredentialHash: "h1", CredentialID: "x",
		Credential: "{}", Name: "Key"}).Error; err != nil {
		t.Fatal(err)
	}
	New(a).changed(staff.ID)
	if ping(staff) != 200 {
		t.Fatal("staff with passkey must pass")
	}
	// Without an authenticated user the gate refuses (misconfigured mount).
	bare := testutil.NewRouterWith(t, a, func(r gin.IRouter, a *app.App) {
		r.GET("/bare", AdminGate(a), func(c *gin.Context) { httpx.OK(c, true) })
	})
	if testutil.Do(t, bare, http.MethodGet, "/bare", nil, "").Code != 401 {
		t.Fatal("gate without auth must be 401")
	}
}
