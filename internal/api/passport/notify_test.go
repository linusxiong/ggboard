package passport_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Every administrator/staff sign-in emits admin_login from the three success paths; members never do.
func TestAdminLoginNotifications(t *testing.T) {
	a, r, now := setupMFA(t)
	rec := notifytest.Attach(t, a)
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15"
	login := func(path string, body any) {
		t.Helper()
		res := testutil.Decode[passport.AuthResult](t, testutil.DoWithHeaders(t, r, http.MethodPost, base+path, body, "", map[string]string{"User-Agent": ua}))
		if res.Token == "" {
			t.Fatalf("%s: no token", path)
		}
	}
	member := testutil.CreateUser(t, a, "member@example.com")
	admin := testutil.CreateAdmin(t, a, "root@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")

	login("/login", map[string]any{"email": member.Email, "password": testutil.Password})
	rec.None(t, notify.TypeAdminLogin)

	login("/login", map[string]any{"email": staff.Email, "password": testutil.Password})
	fs := notifytest.Fields(rec.Last(t, notify.TypeAdminLogin))
	if fs["email"] != staff.Email || fs["role"] != "客服" || fs["method"] != "密码" || fs["ip"] == "" || fs["time"] == "" {
		t.Fatalf("staff password login = %v", fs)
	}
	if len([]rune(fs["ua"])) != notify.MaxUALen {
		t.Fatalf("ua = %q", fs["ua"])
	}

	// Password + TOTP, then password + backup code.
	secret, backups := enableTOTP(t, a, admin, now)
	login("/login/mfa", map[string]any{"mfa_token": mfaToken(t, r, admin.Email), "code": mfatest.TOTP(t, secret, *now)})
	fs = notifytest.Fields(rec.Last(t, notify.TypeAdminLogin))
	if fs["email"] != admin.Email || fs["role"] != "管理员" || fs["method"] != "密码 + 两步验证" {
		t.Fatalf("totp login = %v", fs)
	}
	*now = now.Add(30 * time.Second)
	login("/login/mfa", map[string]any{"mfa_token": mfaToken(t, r, admin.Email), "code": backups[0]})
	if fs = notifytest.Fields(rec.Last(t, notify.TypeAdminLogin)); fs["method"] != "密码 + 备用码" {
		t.Fatalf("backup login = %v", fs)
	}

	// Passkey.
	key := enrollPasskey(t, a, admin)
	cer := beginLogin(t, r, map[string]any{})
	login("/passkey/login/finish", map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Assert(t, cer.Options)})
	if fs = notifytest.Fields(rec.Last(t, notify.TypeAdminLogin)); fs["method"] != "通行密钥" || fs["email"] != admin.Email {
		t.Fatalf("passkey login = %v", fs)
	}

	// The password step of a TOTP account is not a sign-in yet.
	before := len(rec.Events(t, notify.TypeAdminLogin))
	mfaToken(t, r, admin.Email)
	if n := len(rec.Events(t, notify.TypeAdminLogin)); n != before || n != 4 {
		t.Fatalf("admin_login events = %d (before %d)", n, before)
	}
}
