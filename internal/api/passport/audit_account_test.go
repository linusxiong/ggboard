package passport_test

// Security audit 2026-10-22, NEEDS-VALIDATION §1 and §3:
//   - §1: with email_verify on, POST /register answered 409 for a registered address before checking the email
//     code, so anyone could tell registered addresses from new ones.
//   - §3: with email_verify off, anyone could register someone else's address; when the mailbox owner later
//     reset the password, the registrant's passkey, TOTP, subscription token, node uuid and Telegram binding
//     all survived.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/api/passport"
	userapi "github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/service/adminuser"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// auditSetup mounts the passport, user, client (subscription) and guest (Telegram webhook) routes with app_url
// set for passkeys and the TOTP clock fixed.
func auditSetup(t *testing.T, settings map[string]any) (*app.App, *gin.Engine, *time.Time) {
	t.Helper()
	values := map[string]any{"app_url": rpOrigin + "/"}
	for k, v := range settings {
		values[k] = v
	}
	a := testutil.NewTestApp(t, testutil.WithSettings(values))
	now := mfatest.FixClock(t, &mfasvc.TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	return a, testutil.NewRouterWith(t, a, passport.Register, userapi.Register, client.Register, guest.Register), now
}

// auditSendCode requests a verification code for email (lifting the per-email cooldown first) and returns it.
func auditSendCode(t *testing.T, a *app.App, r http.Handler, email string) string {
	t.Helper()
	a.KV.Delete("account:code_cooldown:" + email)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": email}, ""), 200)
	return lastCode(t, a, email)
}

type auditResponse struct {
	code int
	body string
}

func auditPost(t *testing.T, r http.Handler, path string, body any, token string) auditResponse {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, path, body, token)
	return auditResponse{code: rec.Code, body: rec.Body.String()}
}

func TestAuditRegisterDoesNotRevealRegisteredEmail(t *testing.T) {
	a, r, _ := auditSetup(t, map[string]any{"email_verify": true})
	known := testutil.CreateUser(t, a, "known@example.com")
	const unknown = "unknown@example.com"
	reg := func(email, code string) auditResponse {
		return auditPost(t, r, base+"/register", map[string]any{"email": email, "password": "password123", "email_code": code}, "")
	}

	// No code was ever sent, then a wrong guess at a sent code: byte-identical 400 on email_code either way.
	for _, code := range []string{"000000", "123456"} {
		k, u := reg(known.Email, code), reg(unknown, code)
		if k.code != http.StatusBadRequest || k != u || !strings.Contains(k.body, `"email_code"`) {
			t.Fatalf("no code sent: known = %+v, unknown = %+v", k, u)
		}
	}
	// send-code answers the same for both (internal/service/account SendCode; read-only check).
	if k, u := auditPost(t, r, base+"/email/send-code", map[string]any{"email": known.Email}, ""),
		auditPost(t, r, base+"/email/send-code", map[string]any{"email": unknown}, ""); k.code != 200 || k != u {
		t.Fatalf("send-code: known = %+v, unknown = %+v", k, u)
	}
	knownCode, unknownCode := lastCode(t, a, known.Email), lastCode(t, a, unknown)
	k, u := reg(known.Email, wrongCode(knownCode)), reg(unknown, wrongCode(unknownCode))
	if k.code != http.StatusBadRequest || k != u || !strings.Contains(k.body, `"email_code"`) {
		t.Fatalf("wrong code: known = %+v, unknown = %+v", k, u)
	}
	for _, email := range []string{known.Email, unknown} {
		expectError(t, r, "/register", map[string]any{"email": email, "password": "password123"}, 400, "email_code")
	}

	// Only the mailbox owner (holding the right code) learns that the address is taken.
	expectError(t, r, "/register", map[string]any{"email": known.Email, "password": "password123", "email_code": knownCode}, 409, "")
	if got := loadUser(t, a, known.ID); got.PasswordHash != known.PasswordHash {
		t.Fatal("a conflicting registration changed the existing account")
	}
	res := register(t, r, map[string]any{"email": unknown, "password": "password123", "email_code": unknownCode})
	if got := loadUser(t, a, res.User.ID); got.EmailVerifiedAt == nil {
		t.Fatal("a registration with a valid email code must stamp email_verified_at")
	}
}

// With email_verify on, the invite code is checked before the email code is consumed, so a typo in the invite
// code does not burn the email code.
func TestAuditRegisterInviteCheckedBeforeEmailCode(t *testing.T) {
	a, r, _ := auditSetup(t, map[string]any{"email_verify": true, "invite_force": true})
	inviter := testutil.CreateUser(t, a, "inviter@example.com")
	if err := a.DB.Create(&model.InviteCode{UserID: inviter.ID, Code: "AUDIT0001"}).Error; err != nil {
		t.Fatal(err)
	}
	const email = "invitee@example.com"
	code := auditSendCode(t, a, r, email)
	expectError(t, r, "/register", map[string]any{"email": email, "password": "password123", "email_code": code, "invite_code": "TYPO"}, 400, "invite_code")
	res := register(t, r, map[string]any{"email": email, "password": "password123", "email_code": code, "invite_code": "AUDIT0001"})
	if res.User.Email != email {
		t.Fatalf("register = %+v", res.User)
	}
}

// auditTelegramUpdate is a private-chat message from chat to the bot.
func auditTelegramUpdate(chat int64, text string) map[string]any {
	return map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 10,
			"from":       map[string]any{"id": chat, "is_bot": false, "first_name": "R", "username": "registrant"},
			"chat":       map[string]any{"id": chat, "type": "private"},
			"text":       text,
		},
	}
}

// auditTelegramStart sends /start <code> through the webhook and returns the bot's reply text.
func auditTelegramStart(t *testing.T, a *app.App, r http.Handler, chat int64, code string) string {
	t.Helper()
	rec := testutil.Do(t, r, http.MethodPost, "/api/v1/guest/telegram/webhook/"+a.Settings.String(telegram.KeyWebhookSecret),
		auditTelegramUpdate(chat, "/start "+code), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook = %d %s", rec.Code, rec.Body)
	}
	var rep telegram.Reply // the webhook answers with a bare sendMessage call, no {"data"} envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("webhook body %q: %v", rec.Body, err)
	}
	return rep.Text
}

func auditCountRows(t *testing.T, a *app.App, m any, userID uint64) int64 {
	t.Helper()
	var n int64
	if err := a.DB.Model(m).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAuditUnverifiedAccountIsReclaimedOnPasswordReset(t *testing.T) {
	a, r, now := auditSetup(t, nil) // email_verify defaults to off
	telegramtest.Enable(t, a)
	const victim = "victim@example.com"
	const chat = int64(9001)

	// The registrant signs up with the victim's address and sets up every credential they can.
	res := register(t, r, map[string]any{"email": victim, "password": "registrant-pass"})
	session := res.Token
	if u := loadUser(t, a, res.User.ID); u.EmailVerifiedAt != nil {
		t.Fatal("a registration without an email code must not be marked verified")
	}
	sub := testutil.Decode[userapi.Subscription](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription", nil, session))
	cer := testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/security/passkeys/begin", map[string]any{"name": "registrant"}, session))
	key := mfatest.New(t, rpID, rpOrigin)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/security/passkeys/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Register(t, cer.Options)}, session), 200)
	enableTOTP(t, a, loadUser(t, a, res.User.ID), now)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/subscription/age", map[string]any{}, session), 200)
	bindCode := testutil.Decode[telegram.BindCode](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, session))
	if reply := auditTelegramStart(t, a, r, chat, bindCode.Code); !strings.HasPrefix(reply, "绑定成功") {
		t.Fatalf("bind reply = %q", reply)
	}
	pending := testutil.Decode[telegram.BindCode](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, session))
	before := loadUser(t, a, res.User.ID)
	if before.TelegramID == nil || *before.TelegramID != chat || sub.Token != before.Token ||
		auditCountRows(t, a, &model.Passkey{}, before.ID) != 1 || auditCountRows(t, a, &model.UserMFA{}, before.ID) != 1 ||
		auditCountRows(t, a, &model.SubscriptionKey{}, before.ID) != 1 {
		t.Fatalf("registrant setup incomplete: %+v", before)
	}
	if on, err := mfasvc.New(a).Configured(bg, before.ID); err != nil || !on { // primes the AdminGate cache
		t.Fatalf("configured = %v %v", on, err)
	}
	if k, err := guard.LoadKey(bg, a, before.ID); err != nil || k == nil { // primes the key cache
		t.Fatalf("age key = %v %v", k, err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/client/subscribe?token="+before.Token, nil, ""), 200)

	// The mailbox owner resets the password.
	code := auditSendCode(t, a, r, victim)
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/forgot",
		map[string]any{"email": victim, "email_code": code, "password": "owner-password"}, "")); !ok {
		t.Fatal("forgot = false")
	}

	after := loadUser(t, a, before.ID)
	if after.Token == before.Token || after.UUID == before.UUID || after.TelegramID != nil || after.TelegramUsername != nil ||
		after.EmailVerifiedAt == nil || after.PasswordVersion != before.PasswordVersion+1 ||
		!auth.VerifyPassword(after.PasswordHash, "owner-password") {
		t.Fatalf("after reclaim = %+v", after)
	}
	if n := auditCountRows(t, a, &model.Passkey{}, after.ID); n != 0 {
		t.Fatalf("passkeys left = %d", n)
	}
	if n := auditCountRows(t, a, &model.UserMFA{}, after.ID); n != 0 {
		t.Fatalf("user_mfa rows left = %d", n)
	}
	if n := auditCountRows(t, a, &model.SubscriptionKey{}, after.ID); n != 0 {
		t.Fatalf("age keys left = %d", n)
	}
	if k, err := guard.LoadKey(bg, a, after.ID); err != nil || k != nil {
		t.Fatalf("age key after reclaim = %v %v (stale cache?)", k, err)
	}
	if on, err := mfasvc.New(a).Configured(bg, after.ID); err != nil || on {
		t.Fatalf("configured after reclaim = %v %v (stale cache?)", on, err)
	}

	// None of the registrant's credentials work any more.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/me", nil, session), http.StatusUnauthorized)
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/client/subscribe?token="+before.Token, nil, ""); rec.Code == http.StatusOK {
		t.Fatalf("old subscription token = %d", rec.Code)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/client/subscribe?token="+after.Token, nil, ""), 200)
	login := beginLogin(t, r, map[string]any{})
	if res := finishLogin(t, r, login, key.Assert(t, login.Options)); res.code != http.StatusUnauthorized {
		t.Fatalf("registrant passkey login = %+v", res)
	}
	if reply := auditTelegramStart(t, a, r, chat, pending.Code); !strings.Contains(reply, "绑定码无效") {
		t.Fatalf("pending bind code after reclaim = %q", reply)
	}
	if got := loadUser(t, a, after.ID); got.TelegramID != nil {
		t.Fatalf("telegram_id rebound = %v", *got.TelegramID)
	}
	// The registrant's TOTP no longer guards the account: the owner signs in with the password alone.
	login2 := testutil.Decode[passport.AuthResult](t, testutil.Do(t, r, http.MethodPost, base+"/login",
		map[string]any{"email": victim, "password": "owner-password"}, ""))
	if login2.Token == "" || login2.User.ID != after.ID {
		t.Fatalf("owner login = %+v", login2)
	}
	audited := false
	for _, e := range a.Logs.Entries(slog.LevelInfo, 0) {
		if strings.HasPrefix(e.Msg, "account: unverified account reclaimed") && e.Attrs["passkeys_removed"] != nil {
			audited = true
		}
	}
	if !audited {
		t.Fatal("no audit log for the reclaim")
	}

	// The account is verified now: a second reset is an ordinary one and keeps the owner's own passkey.
	enrollPasskey(t, a, after)
	code = auditSendCode(t, a, r, victim)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/forgot",
		map[string]any{"email": victim, "email_code": code, "password": "owner-password-2"}, ""), 200)
	again := loadUser(t, a, after.ID)
	if again.Token != after.Token || again.UUID != after.UUID || auditCountRows(t, a, &model.Passkey{}, after.ID) != 1 ||
		!again.EmailVerifiedAt.Equal(*after.EmailVerifiedAt) {
		t.Fatalf("second reset of a verified account = %+v", again)
	}
}

func TestAuditVerifiedAccountKeepsCredentialsOnPasswordReset(t *testing.T) {
	a, r, now := auditSetup(t, map[string]any{"email_verify": true})
	const email = "owner@example.com"
	res := register(t, r, map[string]any{"email": email, "password": "password123", "email_code": auditSendCode(t, a, r, email)})
	u := loadUser(t, a, res.User.ID)
	if u.EmailVerifiedAt == nil {
		t.Fatal("email_verified_at not stamped")
	}
	key := enrollPasskey(t, a, u)
	enableTOTP(t, a, u, now)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/subscription/age", map[string]any{}, res.Token), 200)
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("telegram_id", 4242).Error; err != nil {
		t.Fatal(err)
	}
	before := loadUser(t, a, u.ID)

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/forgot",
		map[string]any{"email": email, "email_code": auditSendCode(t, a, r, email), "password": "brand-new-pass"}, ""), 200)

	after := loadUser(t, a, u.ID)
	if after.Token != before.Token || after.UUID != before.UUID || after.TelegramID == nil || *after.TelegramID != 4242 ||
		after.PasswordVersion != before.PasswordVersion+1 || !after.EmailVerifiedAt.Equal(*before.EmailVerifiedAt) {
		t.Fatalf("after reset = %+v", after)
	}
	if auditCountRows(t, a, &model.Passkey{}, u.ID) != 1 || auditCountRows(t, a, &model.UserMFA{}, u.ID) != 1 ||
		auditCountRows(t, a, &model.SubscriptionKey{}, u.ID) != 1 {
		t.Fatal("a verified account's second factors must survive a password reset")
	}
	login := beginLogin(t, r, map[string]any{})
	if res := finishLogin(t, r, login, key.Assert(t, login.Options)); res.code != http.StatusOK {
		t.Fatalf("passkey login after reset = %+v", res)
	}
}

// Accounts an administrator creates (one by one or generated in bulk) count as verified.
func TestAuditAdminCreatedUsersAreVerified(t *testing.T) {
	a, _, _ := auditSetup(t, nil)
	svc := adminuser.New(a)
	u, err := svc.Create(bg, adminuser.CreateInput{Email: "made@example.com", Password: "password123"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := loadUser(t, a, u.ID); got.EmailVerifiedAt == nil || u.EmailVerifiedAt == nil {
		t.Fatalf("admin-created email_verified_at = %v", got.EmailVerifiedAt)
	}
	if _, err := svc.Generate(bg, adminuser.GenerateInput{EmailSuffix: "example.com", Count: 3}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var unverified int64
	if err := a.DB.Model(&model.User{}).Where("email_verified_at IS NULL").Count(&unverified).Error; err != nil || unverified != 0 {
		t.Fatalf("generated users without email_verified_at = %d %v", unverified, err)
	}
}
