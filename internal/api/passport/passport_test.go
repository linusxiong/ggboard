package passport_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const base = "/api/v1/passport"

func setup(t *testing.T, settings map[string]any) (*app.App, *gin.Engine) {
	t.Helper()
	var opts []testutil.Option
	if settings != nil {
		opts = append(opts, testutil.WithSettings(settings))
	}
	a := testutil.NewTestApp(t, opts...)
	return a, testutil.NewRouterWith(t, a, passport.Register)
}

func register(t *testing.T, r http.Handler, body map[string]any) passport.AuthResult {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, base+"/register", body, "")
	return testutil.Decode[passport.AuthResult](t, rec)
}

func expectError(t *testing.T, r http.Handler, path string, body any, status int, field string) {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, base+path, body, "")
	e := testutil.DecodeError(t, rec)
	if e.Status != status {
		t.Fatalf("%s: status = %d, want %d (%s)", path, e.Status, status, rec.Body.String())
	}
	if field != "" && len(e.Errors[field]) == 0 {
		t.Fatalf("%s: expected error on %q, got %+v", path, field, e)
	}
}

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

// lastCode returns the code of the last verification mail sent to email.
func lastCode(t *testing.T, a *app.App, email string) string {
	t.Helper()
	var code string
	for _, m := range testutil.Mails(t, a) {
		if m.To == email && m.Template == mail.TplVerifyCode {
			match := codeRe.FindStringSubmatch(m.Text)
			if match == nil {
				t.Fatalf("no code in mail %q", m.Text)
			}
			code = match[1]
		}
	}
	if code == "" {
		t.Fatalf("no verification mail for %s", email)
	}
	return code
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func loadUser(t *testing.T, a *app.App, id uint64) *model.User {
	t.Helper()
	u, err := usersvc.New(a).FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRegisterHappyPath(t *testing.T) {
	a, r := setup(t, map[string]any{"default_remind_expire": false, "default_remind_traffic": true})
	res := register(t, r, map[string]any{"email": "New.User@Example.com", "password": "s3cret-pass"})
	if res.Token == "" || res.User.Email != "new.user@example.com" || res.User.ID == 0 {
		t.Fatalf("register = %+v", res)
	}
	if res.User.Plan != nil || res.User.PlanID != nil || res.User.RemindExpire || !res.User.RemindTraffic {
		t.Fatalf("defaults = %+v", res.User)
	}
	if res.User.LastLoginAt == nil || !strings.HasPrefix(res.User.AvatarURL, "https://www.gravatar.com/avatar/") {
		t.Fatalf("view = %+v", res.User)
	}
	claims, err := auth.Parse(a, res.Token)
	if err != nil || claims.Email != "new.user@example.com" {
		t.Fatalf("token claims = %+v, %v", claims, err)
	}
	u := loadUser(t, a, res.User.ID)
	if u.LastLoginIP == "" || u.InviteUserID != nil || u.Token == "" {
		t.Fatalf("stored = %+v", u)
	}
	// Duplicate (case-insensitive) is a conflict.
	expectError(t, r, "/register", map[string]any{"email": "NEW.user@example.com", "password": "s3cret-pass"}, 409, "")
	// Validation.
	expectError(t, r, "/register", map[string]any{"email": "nope", "password": "short"}, 400, "password")
	expectError(t, r, "/register", map[string]any{"email": "nope", "password": "long-enough"}, 400, "email")
}

func TestRegisterStopAndWhitelist(t *testing.T) {
	_, r := setup(t, map[string]any{"stop_register": true})
	expectError(t, r, "/register", map[string]any{"email": "a@example.com", "password": "password123"}, 403, "")

	_, r = setup(t, map[string]any{"email_whitelist_enable": true, "email_whitelist_suffix": []string{"Example.com", "@example.org"}})
	expectError(t, r, "/register", map[string]any{"email": "a@example.net", "password": "password123"}, 400, "email")
	expectError(t, r, "/register", map[string]any{"email": "a@sub.example.com", "password": "password123"}, 400, "email")
	register(t, r, map[string]any{"email": "a@EXAMPLE.com", "password": "password123"})
	register(t, r, map[string]any{"email": "b@example.org", "password": "password123"})
}

func TestRegisterEmailVerify(t *testing.T) {
	a, r := setup(t, map[string]any{"email_verify": true})
	email := "verify@example.com"
	expectError(t, r, "/register", map[string]any{"email": email, "password": "password123"}, 400, "email_code")
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": email}, ""), 200)
	code := lastCode(t, a, email)
	expectError(t, r, "/register", map[string]any{"email": email, "password": "password123", "email_code": wrongCode(code)}, 400, "email_code")
	res := register(t, r, map[string]any{"email": email, "password": "password123", "email_code": code})
	if res.User.Email != email {
		t.Fatalf("user = %+v", res.User)
	}
	// Codes are single use, also for the other purpose.
	expectError(t, r, "/forgot", map[string]any{"email": email, "email_code": code, "password": "another-pass"}, 400, "email_code")
}

func TestEmailCodeAttemptsExhausted(t *testing.T) {
	a, r := setup(t, map[string]any{"email_verify": true})
	email := "brute@example.com"
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": email}, ""), 200)
	code := lastCode(t, a, email)
	for range account.MaxCodeAttempts {
		expectError(t, r, "/register", map[string]any{"email": email, "password": "password123", "email_code": wrongCode(code)}, 400, "email_code")
	}
	expectError(t, r, "/register", map[string]any{"email": email, "password": "password123", "email_code": code}, 400, "email_code")
}

func TestRegisterInvite(t *testing.T) {
	a, r := setup(t, map[string]any{"invite_force": true, "invite_never_expire": false})
	inviter := testutil.CreateUser(t, a, "inviter@example.com")
	ic := model.InviteCode{UserID: inviter.ID, Code: "INVITE0001"}
	if err := a.DB.Create(&ic).Error; err != nil {
		t.Fatal(err)
	}
	expectError(t, r, "/register", map[string]any{"email": "x@example.com", "password": "password123"}, 400, "invite_code")
	expectError(t, r, "/register", map[string]any{"email": "x@example.com", "password": "password123", "invite_code": "WRONG"}, 400, "invite_code")
	res := register(t, r, map[string]any{"email": "x@example.com", "password": "password123", "invite_code": "INVITE0001"})
	if u := loadUser(t, a, res.User.ID); u.InviteUserID == nil || *u.InviteUserID != inviter.ID {
		t.Fatalf("invite_user_id = %v", u.InviteUserID)
	}
	var got model.InviteCode
	if err := a.DB.First(&got, ic.ID).Error; err != nil || !got.Status {
		t.Fatalf("invite code not consumed: %+v %v", got, err)
	}
	// Consumed codes cannot be reused.
	expectError(t, r, "/register", map[string]any{"email": "y@example.com", "password": "password123", "invite_code": "INVITE0001"}, 400, "invite_code")
}

func TestRegisterInviteNeverExpireAndOptional(t *testing.T) {
	a, r := setup(t, nil) // invite_never_expire defaults to true, invite_force to false
	inviter := testutil.CreateUser(t, a, "inviter@example.com")
	if err := a.DB.Create(&model.InviteCode{UserID: inviter.ID, Code: "KEEPME"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"one@example.com", "two@example.com"} {
		res := register(t, r, map[string]any{"email": email, "password": "password123", "invite_code": "KEEPME"})
		if u := loadUser(t, a, res.User.ID); u.InviteUserID == nil || *u.InviteUserID != inviter.ID {
			t.Fatalf("%s invite_user_id = %v", email, u.InviteUserID)
		}
	}
	var ic model.InviteCode
	if err := a.DB.Where("code = ?", "KEEPME").First(&ic).Error; err != nil || ic.Status {
		t.Fatalf("never-expire code consumed: %+v %v", ic, err)
	}
	// An unknown code is ignored when invites are optional.
	res := register(t, r, map[string]any{"email": "three@example.com", "password": "password123", "invite_code": "NOPE"})
	if u := loadUser(t, a, res.User.ID); u.InviteUserID != nil {
		t.Fatalf("invite_user_id = %v", *u.InviteUserID)
	}
}

func TestRegisterTrialPlan(t *testing.T) {
	a, r := setup(t, nil)
	method := model.ResetMonthly
	plan := model.Plan{GroupID: 7, Name: "Trial", TransferEnable: 10, SpeedLimit: testutil.Ptr(100), DeviceLimit: testutil.Ptr(2), ConnLimit: testutil.Ptr(16),
		ResetTrafficMethod: &method}
	if err := a.DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Settings.Set(map[string]any{"try_out_plan_id": int(plan.ID), "try_out_hour": 48}); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	res := register(t, r, map[string]any{"email": "trial@example.com", "password": "password123"})
	u := loadUser(t, a, res.User.ID)
	if u.PlanID == nil || *u.PlanID != plan.ID || u.GroupID == nil || *u.GroupID != 7 {
		t.Fatalf("plan/group = %v/%v", u.PlanID, u.GroupID)
	}
	if u.TransferEnable != 10*model.BytesPerGB || *u.SpeedLimit != 100 || *u.DeviceLimit != 2 || u.ConnLimit == nil || *u.ConnLimit != 16 {
		t.Fatalf("limits = %d %v %v %v", u.TransferEnable, u.SpeedLimit, u.DeviceLimit, u.ConnLimit)
	}
	want := before.Add(48 * time.Hour)
	if u.ExpiredAt == nil || u.ExpiredAt.Before(want.Add(-time.Second)) || u.ExpiredAt.After(want.Add(time.Minute)) {
		t.Fatalf("expired_at = %v, want ~%v", u.ExpiredAt, want)
	}
	if u.NextResetAt == nil || !u.NextResetAt.After(before) {
		t.Fatalf("next_reset_at = %v", u.NextResetAt)
	}
	if res.User.Plan == nil || res.User.Plan.Name != "Trial" {
		t.Fatalf("view plan = %+v", res.User.Plan)
	}
	// A missing trial plan does not block registration.
	if err := a.Settings.Set(map[string]any{"try_out_plan_id": 999}); err != nil {
		t.Fatal(err)
	}
	if res := register(t, r, map[string]any{"email": "trial2@example.com", "password": "password123"}); res.User.PlanID != nil {
		t.Fatalf("plan = %v", res.User.PlanID)
	}
}

func TestRegisterLimitByIP(t *testing.T) {
	_, r := setup(t, map[string]any{"register_limit_by_ip_enable": true, "register_limit_count": 2, "register_limit_expire": 30})
	register(t, r, map[string]any{"email": "ip1@example.com", "password": "password123"})
	// Failed registrations do not count.
	expectError(t, r, "/register", map[string]any{"email": "ip1@example.com", "password": "password123"}, 409, "")
	register(t, r, map[string]any{"email": "ip2@example.com", "password": "password123"})
	rec := testutil.Do(t, r, http.MethodPost, base+"/register", map[string]any{"email": "ip3@example.com", "password": "password123"}, "")
	if e := testutil.DecodeError(t, rec); e.Status != 429 || e.Code != "rate_limited" || !strings.Contains(e.Message, "minutes") {
		t.Fatalf("third = %+v", e)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
}

func TestLoginLockoutAndBanned(t *testing.T) {
	a, r := setup(t, map[string]any{"password_limit_count": 3, "password_limit_expire": 15})
	u := testutil.CreateUser(t, a, "alice@example.com")
	res := testutil.Decode[passport.AuthResult](t, testutil.Do(t, r, http.MethodPost, base+"/login",
		map[string]any{"email": "ALICE@example.com", "password": testutil.Password}, ""))
	if res.Token == "" || res.User.ID != u.ID || res.User.LastLoginAt == nil {
		t.Fatalf("login = %+v", res)
	}
	if got := loadUser(t, a, u.ID); got.LastLoginAt == nil || got.LastLoginIP == "" {
		t.Fatalf("last login not recorded: %+v", got)
	}
	for range 3 {
		expectError(t, r, "/login", map[string]any{"email": u.Email, "password": "wrong-password"}, 401, "")
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": u.Email, "password": testutil.Password}, "")
	e := testutil.DecodeError(t, rec)
	if e.Status != 429 || !strings.Contains(e.Message, "too many failed password attempts") || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("locked = %+v %v", e, rec.Header())
	}
	// Other emails are unaffected; unknown emails are 401.
	expectError(t, r, "/login", map[string]any{"email": "ghost@example.com", "password": "whatever1"}, 401, "")

	b := testutil.CreateUser(t, a, "banned@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", b.ID).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	expectError(t, r, "/login", map[string]any{"email": b.Email, "password": testutil.Password}, 403, "")
}

func TestLoginLockoutDisabled(t *testing.T) {
	a, r := setup(t, map[string]any{"password_limit_enable": false, "password_limit_count": 1})
	u := testutil.CreateUser(t, a, "alice@example.com")
	for range 3 {
		expectError(t, r, "/login", map[string]any{"email": u.Email, "password": "wrong-password"}, 401, "")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": u.Email, "password": testutil.Password}, ""), 200)
}

func TestForgot(t *testing.T) {
	a, r := setup(t, map[string]any{"password_limit_count": 2})
	u := testutil.CreateUser(t, a, "forgot@example.com")
	for range 2 {
		expectError(t, r, "/login", map[string]any{"email": u.Email, "password": "wrong-password"}, 401, "")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": u.Email}, ""), 200)
	code := lastCode(t, a, u.Email)
	expectError(t, r, "/forgot", map[string]any{"email": u.Email, "email_code": wrongCode(code), "password": "brand-new-pass"}, 400, "email_code")
	expectError(t, r, "/forgot", map[string]any{"email": u.Email, "email_code": code, "password": "short"}, 400, "password")
	if got := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/forgot",
		map[string]any{"email": u.Email, "email_code": code, "password": "brand-new-pass"}, "")); !got {
		t.Fatal("forgot = false")
	}
	after := loadUser(t, a, u.ID)
	if after.PasswordVersion != u.PasswordVersion+1 || !auth.VerifyPassword(after.PasswordHash, "brand-new-pass") {
		t.Fatalf("password not reset: version %d", after.PasswordVersion)
	}
	// The reset cleared the lockout: logging in with the new password works.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": u.Email, "password": "brand-new-pass"}, ""), 200)
	expectError(t, r, "/login", map[string]any{"email": u.Email, "password": testutil.Password}, 401, "")

	// A valid code for an unregistered address names the problem (only the mailbox owner can get here).
	ghost := "ghost@example.com"
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": ghost}, ""), 200)
	expectError(t, r, "/forgot", map[string]any{"email": ghost, "email_code": lastCode(t, a, ghost), "password": "brand-new-pass"}, 400, "email")
}

func TestSendCodeCooldownAndMail(t *testing.T) {
	a, r := setup(t, map[string]any{"app_name": "Example Panel"})
	testutil.CreateUser(t, a, "known@example.com")
	// Known and unknown emails get the same response.
	for _, email := range []string{"known@example.com", "unknown@example.com"} {
		if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": email}, "")); !ok {
			t.Fatalf("%s: send-code = false", email)
		}
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": "Known@Example.com"}, "")
	e := testutil.DecodeError(t, rec)
	if e.Status != 429 || !strings.Contains(e.Message, "seconds") {
		t.Fatalf("cooldown = %+v", e)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q", ra)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 2 {
		t.Fatalf("mails = %d", len(mails))
	}
	for _, m := range mails {
		if m.Template != mail.TplVerifyCode || !strings.Contains(m.Subject, "Example Panel") || !codeRe.MatchString(m.Text) {
			t.Fatalf("mail = %+v", m)
		}
	}
	expectError(t, r, "/email/send-code", map[string]any{"email": "not-an-email"}, 400, "email")
}

func TestSendCodeIPLimit(t *testing.T) {
	_, r := setup(t, nil)
	for i := range 10 {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code",
			map[string]any{"email": "user" + string(rune('a'+i)) + "@example.com"}, ""), 200)
	}
	expectError(t, r, "/email/send-code", map[string]any{"email": "another@example.com"}, 429, "")
}

func TestLoginToken(t *testing.T) {
	a, r := setup(t, nil)
	u := testutil.CreateUser(t, a, "target@example.com")
	tok := auth.IssueLoginToken(a, u.ID)
	res := testutil.Decode[passport.AuthResult](t, testutil.Do(t, r, http.MethodPost, base+"/login/token", map[string]any{"token": tok}, ""))
	if res.User.ID != u.ID || res.Token == "" {
		t.Fatalf("login/token = %+v", res)
	}
	expectError(t, r, "/login/token", map[string]any{"token": tok}, 401, "")
	expectError(t, r, "/login/token", map[string]any{}, 400, "token")

	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	expectError(t, r, "/login/token", map[string]any{"token": auth.IssueLoginToken(a, u.ID)}, 403, "")
}

type fakeCaptcha struct{}

func (fakeCaptcha) Verify(_ context.Context, token, _ string) error {
	if token != "human" {
		return captcha.ErrFailed
	}
	return nil
}

func TestCaptcha(t *testing.T) {
	orig := account.CaptchaVerifier
	account.CaptchaVerifier = func(*app.App) captcha.Verifier { return fakeCaptcha{} }
	t.Cleanup(func() { account.CaptchaVerifier = orig })

	a, r := setup(t, map[string]any{"captcha_provider": "turnstile", "captcha_site_key": "site", "captcha_secret": "secret"})
	u := testutil.CreateUser(t, a, "human@example.com")
	expectError(t, r, "/login", map[string]any{"email": u.Email, "password": testutil.Password}, 400, "captcha")
	expectError(t, r, "/login", map[string]any{"email": u.Email, "password": testutil.Password, "captcha": "bot"}, 400, "captcha")
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/login",
		map[string]any{"email": u.Email, "password": testutil.Password, "captcha": "human"}, ""), 200)

	expectError(t, r, "/register", map[string]any{"email": "new@example.com", "password": "password123"}, 400, "captcha")
	register(t, r, map[string]any{"email": "new@example.com", "password": "password123", "captcha": "human"})

	expectError(t, r, "/email/send-code", map[string]any{"email": "new@example.com"}, 400, "captcha")
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code",
		map[string]any{"email": "new@example.com", "captcha": "human"}, ""), 200)
}
