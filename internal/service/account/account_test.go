package account_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestEmailAllowed(t *testing.T) {
	cases := []struct {
		email string
		list  []string
		want  bool
	}{
		{"a@example.com", []string{"example.com"}, true},
		{"A@EXAMPLE.COM", []string{" Example.com "}, true},
		{"a@example.com", []string{"@example.com"}, true},
		{"a@sub.example.com", []string{"example.com"}, false},
		{"a@example.net", []string{"example.com", "example.org"}, false},
		{"no-at-sign", []string{"example.com"}, false},
		{"a@example.com", nil, false},
	}
	for _, c := range cases {
		if got := account.EmailAllowed(c.email, c.list); got != c.want {
			t.Errorf("EmailAllowed(%q, %v) = %v", c.email, c.list, got)
		}
	}
	if account.EmailDomain("x@y@example.com") != "example.com" || account.EmailDomain("none") != "" {
		t.Fatal("EmailDomain")
	}
}

func TestNewCode(t *testing.T) {
	re := regexp.MustCompile(`^\d{6}$`)
	seen := map[string]bool{}
	for range 200 {
		c := account.NewCode()
		if !re.MatchString(c) {
			t.Fatalf("code %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 150 {
		t.Fatalf("codes are not random: %d distinct of 200", len(seen))
	}
}

func TestConsumeCode(t *testing.T) {
	a := testutil.NewTestApp(t)
	svc := account.New(a)
	if err := svc.SendCode("Code@Example.com"); err != nil {
		t.Fatal(err)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 1 || mails[0].To != "code@example.com" {
		t.Fatalf("mails = %+v", mails)
	}
	code := regexp.MustCompile(`\d{6}`).FindString(mails[0].Text)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if e := httpx.AsError(svc.ConsumeCode(account.PurposeForgot, "code@example.com", "")); e.Status != 400 || len(e.Errors["email_code"]) == 0 {
		t.Fatalf("empty = %+v", e)
	}
	if err := svc.ConsumeCode(account.PurposeForgot, "code@example.com", wrong); err == nil {
		t.Fatal("wrong code accepted")
	}
	// The right code still works after a wrong guess, for either purpose, and only once.
	if err := svc.ConsumeCode(account.PurposeForgot, "CODE@example.com", code); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConsumeCode(account.PurposeRegister, "code@example.com", code); err == nil {
		t.Fatal("code reused")
	}
	if err := svc.ConsumeCode(account.PurposeRegister, "nobody@example.com", code); err == nil {
		t.Fatal("code for another email accepted")
	}

	// A second code is subject to the cooldown.
	err := svc.SendCode("code@example.com")
	var re *account.RetryError
	if !errors.As(err, &re) || re.RetryAfterSeconds() < 1 || re.RetryAfterSeconds() > 60 || httpx.AsError(err).Status != http.StatusTooManyRequests {
		t.Fatalf("cooldown = %v", err)
	}
}

func TestViews(t *testing.T) {
	a := testutil.NewTestApp(t)
	plan := &model.Plan{GroupID: 1, Name: "Gold"}
	if err := a.DB.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	inviter := testutil.CreateUser(t, a, "inviter@example.com")
	u := testutil.CreateUser(t, a, "view@example.com", usersvc.CreateOptions{PlanID: &plan.ID, InviteUserID: &inviter.ID, Remarks: "note"})

	var loaded model.User
	if err := a.DB.Preload("Plan").First(&loaded, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	v := account.ToUserView(&loaded, nil)
	if v.Plan == nil || v.Plan.Name != "Gold" || v.CreatedAt.Location() != time.UTC {
		t.Fatalf("view = %+v", v)
	}
	if v2, err := account.New(a).View(context.Background(), u); err != nil || v2.Plan == nil || v2.Plan.ID != plan.ID {
		t.Fatalf("View = %+v %v", v2, err)
	}

	b, err := json.Marshal(account.ToAdminUserView(&loaded, nil))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "email", "plan", "avatar_url", "token", "remarks", "invite_user_id", "last_login_ip", "telegram_id", "commission_type"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("admin view lacks %q: %s", k, b)
		}
	}
	for _, k := range []string{"password_hash", "password_version", "password"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("admin view exposes %q", k)
		}
	}
	if account.RefOf(nil) != nil {
		t.Fatal("RefOf(nil)")
	}
	if p, err := account.LoadPlan(context.Background(), a.DB, testutil.Ptr(uint64(12345))); err != nil || p != nil {
		t.Fatalf("missing plan = %v %v", p, err)
	}
}
