package user_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

var ctx = context.Background()

func TestNextResetAt(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	at := func(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, loc) }
	ptr := func(t time.Time) *time.Time { return &t }
	cases := []struct {
		name   string
		method model.ResetMethod
		exp    *time.Time
		now    time.Time
		want   *time.Time
	}{
		{"no expiry", model.ResetMonthly, nil, at(2026, 1, 1, 0, 0), nil},
		{"never", model.ResetNever, ptr(at(2027, 1, 15, 0, 0)), at(2026, 1, 1, 0, 0), nil},
		{"unknown method", model.ResetMethod(9), ptr(at(2027, 1, 15, 0, 0)), at(2026, 1, 1, 0, 0), nil},
		{"first of month", model.ResetFirstDayOfMonth, ptr(at(2027, 1, 15, 0, 0)), at(2026, 1, 31, 23, 59), ptr(at(2026, 2, 1, 0, 0))},
		{"first of month december", model.ResetFirstDayOfMonth, ptr(at(2027, 1, 15, 0, 0)), at(2026, 12, 1, 0, 0), ptr(at(2027, 1, 1, 0, 0))},
		{"monthly later this month", model.ResetMonthly, ptr(at(2027, 3, 15, 10, 30)), at(2026, 4, 2, 0, 0), ptr(at(2026, 4, 15, 10, 30))},
		{"monthly passed this month", model.ResetMonthly, ptr(at(2027, 3, 15, 10, 30)), at(2026, 4, 15, 10, 30), ptr(at(2026, 5, 15, 10, 30))},
		{"monthly 31st clamps in feb", model.ResetMonthly, ptr(at(2027, 1, 31, 8, 0)), at(2026, 2, 3, 0, 0), ptr(at(2026, 2, 28, 8, 0))},
		{"monthly 31st clamps in leap feb", model.ResetMonthly, ptr(at(2029, 1, 31, 8, 0)), at(2028, 2, 3, 0, 0), ptr(at(2028, 2, 29, 8, 0))},
		{"monthly 31st after feb", model.ResetMonthly, ptr(at(2027, 1, 31, 8, 0)), at(2026, 2, 28, 9, 0), ptr(at(2026, 3, 31, 8, 0))},
		{"monthly 31st into april clamps", model.ResetMonthly, ptr(at(2027, 1, 31, 8, 0)), at(2026, 3, 31, 9, 0), ptr(at(2026, 4, 30, 8, 0))},
		{"monthly december rollover", model.ResetMonthly, ptr(at(2027, 6, 10, 0, 0)), at(2026, 12, 20, 0, 0), ptr(at(2027, 1, 10, 0, 0))},
		{"first of year", model.ResetFirstDayOfYear, ptr(at(2030, 1, 1, 0, 0)), at(2026, 6, 1, 0, 0), ptr(at(2027, 1, 1, 0, 0))},
		{"yearly later this year", model.ResetYearly, ptr(at(2030, 9, 1, 12, 0)), at(2026, 6, 1, 0, 0), ptr(at(2026, 9, 1, 12, 0))},
		{"yearly passed", model.ResetYearly, ptr(at(2030, 3, 1, 12, 0)), at(2026, 6, 1, 0, 0), ptr(at(2027, 3, 1, 12, 0))},
		{"yearly feb 29 clamps", model.ResetYearly, ptr(at(2028, 2, 29, 0, 0)), at(2026, 3, 1, 0, 0), ptr(at(2027, 2, 28, 0, 0))},
		{"expiry in other zone", model.ResetMonthly, ptr(time.Date(2027, 3, 14, 20, 0, 0, 0, time.UTC)), at(2026, 4, 1, 0, 0), ptr(at(2026, 4, 15, 4, 0))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := usersvc.NextResetAt(tc.method, tc.exp, tc.now)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("got %v, want nil", got)
			case tc.want != nil && (got == nil || !got.Equal(*tc.want)):
				t.Fatalf("got %v, want %v", got, tc.want)
			case got != nil && !got.After(tc.now):
				t.Fatalf("reset %v must be after now %v", got, tc.now)
			}
		})
	}
}

func TestResetDaysAndServiceMethods(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		next time.Time
		want int
	}{{now.Add(time.Hour), 1}, {now.Add(24 * time.Hour), 1}, {now.Add(25 * time.Hour), 2}, {now.Add(-time.Hour), 0}} {
		if got := usersvc.ResetDays(&tc.next, now); *got != tc.want {
			t.Errorf("ResetDays(%v) = %d, want %d", tc.next.Sub(now), *got, tc.want)
		}
	}
	if usersvc.ResetDays(nil, now) != nil {
		t.Fatal("nil next")
	}

	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"reset_traffic_method": 3}))
	svc := usersvc.New(a)
	exp := now.AddDate(1, 0, 0)
	u := &model.User{ExpiredAt: &exp}
	if got := svc.ComputeNextResetAt(u, &model.Plan{}, now); got == nil || got.Month() != time.January || got.Year() != 2027 {
		t.Fatalf("plan following settings = %v", got)
	}
	monthly := model.ResetMonthly
	if got := svc.ComputeNextResetAt(u, &model.Plan{ResetTrafficMethod: &monthly}, now); got == nil || got.Day() != 1 || got.Month() != time.February {
		t.Fatalf("plan override = %v", got)
	}
	if svc.ComputeNextResetAt(u, nil, now) != nil {
		t.Fatal("no plan -> nil")
	}
	next := time.Now().Add(49 * time.Hour)
	if d := svc.ResetDay(&model.User{NextResetAt: &next}); d == nil || *d != 3 {
		t.Fatalf("ResetDay = %v", d)
	}
}

func TestSubscribeURL(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := usersvc.New(a)
	u := &model.User{Token: "abc123"}
	if got := svc.SubscribeURL(u); got != "/api/v1/client/subscribe?token=abc123" {
		t.Fatalf("no base = %s", got)
	}
	if err := a.Settings.Set(map[string]any{"app_url": "https://panel.example.com/"}); err != nil {
		t.Fatal(err)
	}
	if got := svc.SubscribeURL(u); got != "https://panel.example.com/api/v1/client/subscribe?token=abc123" {
		t.Fatalf("app_url = %s", got)
	}
	if err := a.Settings.Set(map[string]any{"subscribe_url": []string{"https://s1.example.com", "https://s2.example.com"}}); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 50 {
		got, err := url.Parse(svc.SubscribeURL(u))
		if err != nil || got.Query().Get("token") != "abc123" {
			t.Fatalf("subscribe url = %v", got)
		}
		seen[got.Host] = true
	}
	if !seen["s1.example.com"] || !seen["s2.example.com"] {
		t.Fatalf("random pick hosts = %v", seen)
	}
}

func TestCreateFindAuthenticate(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"default_remind_traffic": false}))
	svc := usersvc.New(a)
	gid := uint64(3)
	u, err := svc.Create(ctx, "  Alice@Example.COM ", "password123", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "alice@example.com" || len(u.Token) != 32 || len(u.UUID) != 36 || !u.RemindExpire || u.RemindTraffic {
		t.Fatalf("created = %+v", u)
	}
	if _, err := svc.Create(ctx, "ALICE@example.com", "password123", usersvc.CreateOptions{}); !errors.Is(err, usersvc.ErrEmailTaken) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := svc.Create(ctx, " ", "password123", usersvc.CreateOptions{}); err == nil {
		t.Fatal("empty email")
	}
	for _, find := range []func() (*model.User, error){
		func() (*model.User, error) { return svc.FindByEmail(ctx, "ALICE@example.com") },
		func() (*model.User, error) { return svc.FindByToken(ctx, u.Token) },
		func() (*model.User, error) { return svc.FindByID(ctx, u.ID) },
	} {
		if got, err := find(); err != nil || got.ID != u.ID {
			t.Fatalf("find = %v %v", got, err)
		}
	}
	if _, err := svc.FindByToken(ctx, ""); !errors.Is(err, usersvc.ErrNotFound) {
		t.Fatal("empty token")
	}
	if _, err := svc.FindByEmail(ctx, "nobody@example.com"); !errors.Is(err, usersvc.ErrNotFound) {
		t.Fatal("not found")
	}

	if got, err := svc.Authenticate(ctx, "alice@example.com", "password123"); err != nil || got.ID != u.ID {
		t.Fatalf("auth = %v", err)
	}
	if _, err := svc.Authenticate(ctx, "alice@example.com", "nope"); !errors.Is(err, usersvc.ErrInvalidCredentials) {
		t.Fatal("wrong password")
	}
	if _, err := svc.Authenticate(ctx, "ghost@example.com", "password123"); !errors.Is(err, usersvc.ErrInvalidCredentials) {
		t.Fatal("unknown email must look like a wrong password")
	}
	if err := svc.RecordLogin(ctx, u, "203.0.113.9"); err != nil || u.LastLoginIP != "203.0.113.9" {
		t.Fatal("RecordLogin")
	}

	a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true)
	if got, err := svc.Authenticate(ctx, "alice@example.com", "password123"); !errors.Is(err, usersvc.ErrBanned) || got == nil {
		t.Fatalf("banned = %v", err)
	}
}

func TestLegacyHashesUpgradeWithoutVersionBump(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := usersvc.New(a)
	bc, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	hashes := map[string]string{
		"bcrypt@example.com": "$2y$" + string(bc)[4:],
		"md5@example.com":    "xb$md5$$482c811da5d5b4bc6d497ffa98491e38",
	}
	for email, hash := range hashes {
		u, err := svc.Create(ctx, email, "", usersvc.CreateOptions{PasswordHash: hash})
		if err != nil {
			t.Fatal(err)
		}
		got, err := svc.Authenticate(ctx, email, "password123")
		if err != nil {
			t.Fatalf("%s: %v", email, err)
		}
		var stored model.User
		a.DB.First(&stored, u.ID)
		if !strings.HasPrefix(stored.PasswordHash, "$argon2id$") || got.PasswordHash != stored.PasswordHash {
			t.Fatalf("%s not upgraded: %s", email, stored.PasswordHash)
		}
		if stored.PasswordVersion != 0 {
			t.Fatalf("%s: rehash must not bump password_version", email)
		}
		if _, err := svc.Authenticate(ctx, email, "password123"); err != nil {
			t.Fatalf("%s: login after upgrade: %v", email, err)
		}
	}
}

func TestSetPasswordAndResetSecurity(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := usersvc.New(a)
	u := testutil.CreateUser(t, a, "bob@example.com")
	oldUUID, oldToken := u.UUID, u.Token
	if err := svc.SetPassword(ctx, u, "another-pass"); err != nil {
		t.Fatal(err)
	}
	if u.PasswordVersion != 1 || !auth.VerifyPassword(u.PasswordHash, "another-pass") {
		t.Fatal("SetPassword")
	}
	if _, err := svc.Authenticate(ctx, u.Email, "another-pass"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetSecurity(ctx, u); err != nil {
		t.Fatal(err)
	}
	var stored model.User
	a.DB.First(&stored, u.ID)
	if stored.UUID == oldUUID || stored.Token == oldToken || stored.PasswordVersion != 2 || stored.UUID != u.UUID {
		t.Fatalf("reset security = %+v", stored)
	}
	ghost := &model.User{ID: 9999}
	if err := svc.SetPassword(ctx, ghost, "x-password"); !errors.Is(err, usersvc.ErrNotFound) {
		t.Fatal("missing user")
	}
	if err := svc.ResetSecurity(ctx, ghost); !errors.Is(err, usersvc.ErrNotFound) {
		t.Fatal("missing user")
	}
}

func TestSecurityWritesRejectStaleUserSnapshot(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := usersvc.New(a)
	u := testutil.CreateUser(t, a, "stale-security@example.com")
	stale := *u
	oldUUID, oldToken := u.UUID, u.Token

	if err := svc.SetPassword(ctx, u, "new-password-1"); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"set password":   func() error { return svc.SetPassword(ctx, &stale, "attacker-password") },
		"reset security": func() error { return svc.ResetSecurity(ctx, &stale) },
		"rotate token":   func() error { return svc.RotateToken(ctx, &stale) },
	} {
		if err := write(); !errors.Is(err, usersvc.ErrNotFound) {
			t.Fatalf("%s with stale snapshot = %v, want ErrNotFound", name, err)
		}
	}

	var stored model.User
	if err := a.DB.First(&stored, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordVersion != 1 || stored.UUID != oldUUID || stored.Token != oldToken ||
		!auth.VerifyPassword(stored.PasswordHash, "new-password-1") {
		t.Fatalf("stale security write changed account: %+v", stored)
	}

	sameVersion := stored
	replacementUUID := uuid.NewString()
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", replacementUUID).Error; err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"set password":   func() error { return svc.SetPassword(ctx, &sameVersion, "attacker-password") },
		"reset security": func() error { return svc.ResetSecurity(ctx, &sameVersion) },
		"rotate token":   func() error { return svc.RotateToken(ctx, &sameVersion) },
	} {
		if err := write(); !errors.Is(err, usersvc.ErrNotFound) {
			t.Fatalf("%s after account replacement = %v, want ErrNotFound", name, err)
		}
	}
	if err := a.DB.First(&stored, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UUID != replacementUUID || stored.Token != oldToken || !auth.VerifyPassword(stored.PasswordHash, "new-password-1") {
		t.Fatalf("replaced account changed by stale snapshot: %+v", stored)
	}
}

func TestAvailableUsersForGroups(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := usersvc.New(a)
	g1, g2, g3 := uint64(1), uint64(2), uint64(3)
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	speed, devices, conns := 100, 3, 64
	mk := func(email string, o usersvc.CreateOptions) *model.User {
		if o.TransferEnable == 0 {
			o.TransferEnable = 1 << 30
		}
		return testutil.CreateUser(t, a, email, o)
	}
	ok1 := mk("ok1@example.com", usersvc.CreateOptions{GroupID: &g1, ExpiredAt: &future, SpeedLimit: &speed, DeviceLimit: &devices, ConnLimit: &conns})
	ok2 := mk("ok2@example.com", usersvc.CreateOptions{GroupID: &g2})
	mk("other-group@example.com", usersvc.CreateOptions{GroupID: &g3})
	mk("no-group@example.com", usersvc.CreateOptions{})
	mk("expired@example.com", usersvc.CreateOptions{GroupID: &g1, ExpiredAt: &past})
	banned := mk("banned@example.com", usersvc.CreateOptions{GroupID: &g1})
	a.DB.Model(&model.User{}).Where("id = ?", banned.ID).Update("banned", true)
	used := mk("used@example.com", usersvc.CreateOptions{GroupID: &g2})
	a.DB.Model(&model.User{}).Where("id = ?", used.ID).Updates(map[string]any{"u": 1 << 29, "d": 1 << 29})

	users, err := svc.AvailableUsersForGroups(ctx, []uint64{g2, g1, g1})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].ID != ok1.ID || users[1].ID != ok2.ID {
		t.Fatalf("users = %+v", users)
	}
	if users[0].UUID != ok1.UUID || users[0].SpeedLimit != 100 || users[0].DeviceLimit != 3 || users[0].ConnLimit != 64 ||
		users[1].SpeedLimit != 0 || users[1].ConnLimit != 0 {
		t.Fatalf("limits = %+v", users)
	}
	// The JSON is the UniProxy element: conn_limit only reaches soga (tcp_limit).
	if b, err := json.Marshal(users[0]); err != nil || string(b) != fmt.Sprintf(`{"id":%d,"uuid":"%s","speed_limit":100,"device_limit":3}`, ok1.ID, ok1.UUID) {
		t.Fatalf("UniProxy element = %s %v", b, err)
	}

	// Cached for the same group set, regardless of order.
	late := mk("late@example.com", usersvc.CreateOptions{GroupID: &g1})
	a.Cache.Set(a.Cache.Key(usersvc.NodeUsersNamespace, "1,2"), []usersvc.NodeUser{{ID: 1}}, time.Minute)
	cached, _ := svc.AvailableUsersForGroups(ctx, []uint64{1, 2})
	if len(cached) != 1 {
		t.Fatalf("expected cached result, got %d", len(cached))
	}
	svc.InvalidateNodeUsers()
	fresh, _ := svc.AvailableUsersForGroups(ctx, []uint64{1, 2})
	if len(fresh) != 3 || fresh[2].ID != late.ID {
		t.Fatalf("after invalidation = %+v", fresh)
	}
	if empty, err := svc.AvailableUsersForGroups(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatal("no groups")
	}
}
