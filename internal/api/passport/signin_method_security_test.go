package passport_test

import (
	"net/http"
	"strconv"
	"testing"

	adminapi "github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/api/passport"
	userapi "github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Audit 2026-11-25: with admin_mfa_required on, an administrator whose only second factor is a passkey got a
// full admin session from the password alone (the gate asked whether a factor existed, not how the session
// signed in).
func TestAdminMFAGateNeedsASecondFactorSession(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": rpOrigin + "/", "admin_mfa_required": true}))
	r := testutil.NewRouterWith(t, a, passport.Register, userapi.Register, adminapi.Register)
	admin := testutil.CreateAdmin(t, a, "root@example.com")
	key := enrollPasskey(t, a, admin)
	const area = "/api/v1/admin/users"

	body := passwordLogin(t, r, admin.Email)
	if body["mfa_required"] != nil || body["token"] == nil {
		t.Fatalf("password login of a passkey-only admin = %v", body)
	}
	pwd := testutil.Decode[passport.AuthResult](t, testutil.Do(t, r, http.MethodPost, base+"/login",
		map[string]any{"email": admin.Email, "password": testutil.Password}, "")).Token
	if e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodGet, area, nil, pwd)); e.Status != 403 || e.Code != mfasvc.CodeReauth {
		t.Fatalf("admin area with a password session = %+v", e)
	}
	// The user area stays usable, so the admin can still reach /user/security.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/security", nil, pwd), 200)

	cer := beginLogin(t, r, map[string]any{})
	res := testutil.Decode[passport.AuthResult](t, testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)}, ""))
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, area, nil, res.Token), 200)

	// A session re-issued by a subscription reset keeps the method of the one it replaces.
	reset := testutil.Decode[userapi.SubscriptionReset](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/subscription/reset", nil, res.Token))
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, area, nil, reset.AuthToken), 200)
	reset = testutil.Decode[userapi.SubscriptionReset](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/subscription/reset", nil,
		testutil.Login(t, a, loadUser(t, a, admin.ID))))
	if e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodGet, area, nil, reset.AuthToken)); e.Code != mfasvc.CodeReauth {
		t.Fatalf("re-issued password session = %+v", e)
	}
}

// Audit 2026-11-25: user verification is only "preferred", so a security key without a PIN signed in to
// accounts that otherwise need a password and a TOTP code, and to administrators.
func TestPasskeyLoginWithoutUserVerification(t *testing.T) {
	a, r, now := setupMFA(t)
	cases := []struct {
		name string
		user *model.User
		want int
	}{
		{"plain user", testutil.CreateUser(t, a, "plain@example.com"), 200},
		{"admin", testutil.CreateAdmin(t, a, "boss@example.com"), 401},
		{"staff", testutil.CreateStaff(t, a, "helper@example.com"), 401},
		{"totp user", testutil.CreateUser(t, a, "careful@example.com"), 401},
	}
	enableTOTP(t, a, cases[3].user, now)
	for _, tc := range cases {
		key := enrollPasskey(t, a, loadUser(t, a, tc.user.ID))
		key.NoUserVerification = true
		cer := beginLogin(t, r, map[string]any{})
		rec := testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
			map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)}, "")
		if rec.Code != tc.want {
			t.Fatalf("%s without user verification = %d %s", tc.name, rec.Code, rec.Body.String())
		}
		key.NoUserVerification = false
		cer = beginLogin(t, r, map[string]any{})
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/passkey/login/finish",
			map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)}, ""), 200)
	}
}

// Audit 2026-11-25: a password reset of an account whose email was never verified is an ownership change that
// removes every second factor (usersvc.ReclaimAccount). Self-registered accounts promoted to admin or staff
// stayed unverified, so reading the admin's mailbox was enough to strip TOTP.
func TestPasswordResetNeverReclaimsPrivilegedAccounts(t *testing.T) {
	for _, role := range []string{"is_admin", "is_staff"} {
		t.Run(role, func(t *testing.T) {
			a, r, now := auditSetup(t, nil) // email_verify off: the registration proves nothing
			email := role + "@example.com"
			res := register(t, r, map[string]any{"email": email, "password": testutil.Password})
			// Promoted the way an older release (or a direct SQL update) left it: still unverified.
			if err := a.DB.Model(&model.User{}).Where("id = ?", res.User.ID).Update(role, true).Error; err != nil {
				t.Fatal(err)
			}
			u := loadUser(t, a, res.User.ID)
			if u.EmailVerifiedAt != nil {
				t.Fatal("fixture must be unverified")
			}
			enableTOTP(t, a, u, now)
			code := auditSendCode(t, a, r, email)
			if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/forgot",
				map[string]any{"email": email, "email_code": code, "password": "a-new-password"}, "")); !ok {
				t.Fatal("forgot = false")
			}
			after := loadUser(t, a, u.ID)
			if after.EmailVerifiedAt == nil || after.UUID != u.UUID || after.Token != u.Token {
				t.Fatalf("a privileged account was reclaimed: %+v", after)
			}
			if n := auditCountRows(t, a, &model.UserMFA{}, u.ID); n != 1 {
				t.Fatalf("user_mfa rows = %d, TOTP must survive", n)
			}
			rec := testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": email, "password": "a-new-password"}, "")
			if body := testutil.Decode[map[string]any](t, rec); body["mfa_required"] != true {
				t.Fatalf("login after reset = %v", body)
			}
		})
	}
}

func TestPromotionMarksTheEmailVerified(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, adminapi.Register)
	root := testutil.CreateAdmin(t, a, "root@example.com")
	for _, field := range []string{"is_admin", "is_staff"} {
		res := register(t, testutil.NewRouterWith(t, a, passport.Register), map[string]any{"email": field + "@example.com", "password": testutil.Password})
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, "/api/v1/admin/users/"+strconv.FormatUint(res.User.ID, 10),
			map[string]any{field: true}, testutil.Login(t, a, root)), 200)
		if loadUser(t, a, res.User.ID).EmailVerifiedAt == nil {
			t.Fatalf("%s: promotion must mark the email verified", field)
		}
	}
}

// Audit 2026-11-25: wrong passwords at the sign-in form (anyone can type them) locked the email, and the same
// counter blocked the owner's own security changes made from a signed-in session.
func TestSignInLockoutDoesNotBlockTheOwnersSecurityChanges(t *testing.T) {
	a, r, _ := setupMFA(t)
	userRouter := testutil.NewRouterWith(t, a, userapi.Register)
	u := testutil.CreateUser(t, a, "owner@example.com")
	tok := testutil.Login(t, a, u)
	for range a.Settings.Int("password_limit_count") + 1 {
		testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": u.Email, "password": "guess-guess"}, "")
	}
	if rec := testutil.Do(t, r, http.MethodPost, base+"/login", map[string]any{"email": u.Email, "password": testutil.Password}, ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sign-in after the lockout = %d", rec.Code)
	}
	testutil.ExpectStatus(t, testutil.Do(t, userRouter, http.MethodPost, "/api/v1/user/security/totp/setup",
		map[string]any{"password": testutil.Password}, tok), 200)

	// A signed-in session still gets only password_limit_count guesses of its own.
	other := testutil.CreateUser(t, a, "other@example.com")
	otherTok := testutil.Login(t, a, other)
	last := 0
	for range a.Settings.Int("password_limit_count") + 1 {
		last = testutil.Do(t, userRouter, http.MethodPost, "/api/v1/user/security/totp/setup",
			map[string]any{"password": "guess-guess"}, otherTok).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("password confirmation after too many guesses = %d", last)
	}
}
