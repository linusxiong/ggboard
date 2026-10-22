package admin_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: deleting a user left its passkeys and user_mfa row behind; passkey login resolves the owner
// through those rows, so the credential signed in as whoever got the id next.
func TestReviewDeleteUserRemovesSecondFactors(t *testing.T) {
	e := newOpsEnv(t)
	victim := testutil.CreateUser(t, e.a, "mfa-victim@example.com")
	for _, row := range []any{
		&model.UserMFA{UserID: victim.ID, TOTPEnabled: true},
		&model.Passkey{UserID: victim.ID, RPID: "panel.example.com", CredentialHash: "h1", CredentialID: "c1", Credential: "{}", Name: "Laptop"},
	} {
		if err := e.a.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	e.expect(t, http.MethodDelete, usersPath(victim.ID, ""), nil, e.adminTok, http.StatusOK)
	for _, m := range []any{&model.UserMFA{}, &model.Passkey{}} {
		var n int64
		if err := e.a.DB.Model(m).Where("user_id = ?", victim.ID).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("%T rows left = %d (%v)", m, n, err)
		}
	}
}

// Review 2026-10-22: "login as" skipped the target's second factor for administrators and staff too.
func TestReviewLoginTokenRefusesPrivilegedTargets(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	other := testutil.CreateAdmin(t, e.a, "second-admin@example.com")
	e.expect(t, http.MethodPost, usersPath(other.ID, "/login-token"), nil, e.adminTok, http.StatusConflict)
	e.expect(t, http.MethodPost, usersPath(e.staff.ID, "/login-token"), nil, e.adminTok, http.StatusConflict)

	// A member promoted after the token was issued cannot use it.
	rec := testutil.Do(t, e.r, http.MethodPost, usersPath(e.user.ID, "/login-token"), nil, e.adminTok)
	out := testutil.Decode[struct {
		URL string `json:"url"`
	}](t, rec)
	tok := out.URL[strings.Index(out.URL, "token=")+len("token="):]
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", e.user.ID).Update("is_staff", true).Error; err != nil {
		t.Fatal(err)
	}
	auth.InvalidateUser(e.a, e.user.ID)
	pr := testutil.NewRouterWith(t, e.a, passport.Register)
	if rec := testutil.Do(t, pr, http.MethodPost, "/api/v1/passport/login/token", map[string]any{"token": tok}, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("promoted target login = %d %s", rec.Code, rec.Body.String())
	}
}

// Review 2026-10-22: host, tags and protocol settings reached the line-based subscription formats (Surge, Loon,
// Quantumult X) raw, so a newline injected whole lines or sections into every user's config.
func TestReviewNodeRejectsLineBreaks(t *testing.T) {
	e := newNodesAdminEnv(t)
	g := e.group("g")
	host := nodesAdminVless("n", []uint64{g})
	host["host"] = "vl.example.com\n[Rule]"
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", host)); !nodesAdminHas(f, "host") {
		t.Fatalf("host fields %v", f)
	}
	ps := nodesAdminVless("n", []uint64{g})
	ps["protocol_settings"].(map[string]any)["network_settings"] = map[string]any{"path": "/ws\nFINAL,DIRECT"}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", ps)); !nodesAdminHas(f, "protocol_settings.network_settings.path") {
		t.Fatalf("settings fields %v", f)
	}
	tags := nodesAdminVless("n", []uint64{g})
	tags["tags"] = []string{"HK x"}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", tags)); !nodesAdminHas(f, "tags") {
		t.Fatalf("tag fields %v", f)
	}
}

// Review 2026-10-22: the coupon name filter passed % and _ through as wildcards and was case-sensitive on
// PostgreSQL.
func TestReviewCouponNameFilter(t *testing.T) {
	e := newOpsEnv(t)
	now := time.Now()
	for _, c := range []model.Coupon{
		{Code: "SUMMER", Name: "Summer Sale", Type: model.CouponTypePercent, Value: 10, StartedAt: now, EndedAt: now.Add(time.Hour)},
		{Code: "HUNDRED", Name: "100% off", Type: model.CouponTypePercent, Value: 100, StartedAt: now, EndedAt: now.Add(time.Hour)},
	} {
		if err := e.a.DB.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	for q, want := range map[string]int{"summer": 1, "%25": 1, "_": 0, "SALE": 1} {
		rec := testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/coupons?name="+q, nil, e.adminTok)
		if items, _ := testutil.DecodeList[map[string]any](t, rec); len(items) != want {
			t.Errorf("name=%s: %d coupons, want %d", q, len(items), want)
		}
	}
}

// Review 2026-10-22: notice contents and knowledge bodies had no size cap (the body limit is 1 MiB) while MySQL
// TEXT holds 65535 bytes: strict mode failed with a 500, non-strict mode truncated silently.
func TestReviewContentSizeCaps(t *testing.T) {
	e := newOpsEnv(t)
	big := strings.Repeat("字", 22000) // 66000 bytes, 22000 runes
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/knowledge",
		map[string]any{"language": "zh-CN", "category": "c", "title": "t", "body": big}, e.adminTok)
	if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest || len(er.Errors["body"]) == 0 {
		t.Fatalf("knowledge body: %d %+v", er.Status, er.Errors)
	}
	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/notices", map[string]any{"title": "t", "content": big}, e.adminTok)
	if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest || len(er.Errors["content"]) == 0 {
		t.Fatalf("notice content: %d %+v", er.Status, er.Errors)
	}
	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/notices", map[string]any{"title": "t", "content": "c", "sort": 1 << 40}, e.adminTok)
	if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest {
		t.Fatalf("notice sort: %d %+v", er.Status, er.Errors)
	}
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/knowledge",
		map[string]any{"language": "zh-CN", "category": "c", "title": "t", "body": strings.Repeat("字", 21000)}, e.adminTok), http.StatusCreated)
}

// Review 2026-10-22: PATCH /admin/coupons/:id saved the whole row, writing back the limit_use loaded before a
// concurrent order decremented it (granting an extra use).
func TestReviewCouponPatchKeepsConcurrentUse(t *testing.T) {
	e := newOpsEnv(t)
	now := time.Now()
	five := 5
	cp := model.Coupon{Code: "RACE", Name: "Race", Type: model.CouponTypePercent, Value: 10, LimitUse: &five, StartedAt: now, EndedAt: now.Add(time.Hour)}
	if err := e.a.DB.Create(&cp).Error; err != nil {
		t.Fatal(err)
	}
	fired := false
	if err := e.a.DB.Callback().Update().Before("gorm:update").Register("review:concurrent_use", func(db *gorm.DB) {
		if fired || db.Statement.Table != "coupons" {
			return
		}
		fired = true // an order takes one use between the handler's load and its save
		db.Session(&gorm.Session{NewDB: true}).Exec("UPDATE coupons SET limit_use = limit_use - 1 WHERE id = ?", cp.ID)
	}); err != nil {
		t.Fatal(err)
	}
	rec := testutil.Do(t, e.r, http.MethodPatch, "/api/v1/admin/coupons/"+strconv.FormatUint(cp.ID, 10), map[string]any{"name": "Renamed"}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	var got model.Coupon
	if err := e.a.DB.First(&got, cp.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !fired || got.Name != "Renamed" || got.LimitUse == nil || *got.LimitUse != 4 {
		t.Fatalf("fired=%v coupon = %+v limit_use=%v", fired, got, got.LimitUse)
	}
}
