package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type opsFailingMailer struct{}

func (opsFailingMailer) Send(context.Context, mail.Message) error {
	return errors.New("smtp: connection refused by mail.example.com")
}

func TestAdminSettingsGetPut(t *testing.T) {
	e := newOpsEnv(t)
	get := func() map[string]json.RawMessage {
		t.Helper()
		return testutil.Decode[map[string]json.RawMessage](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/settings", nil, e.adminTok))
	}
	out := get()
	var site map[string]any
	var server map[string]any
	var groups []settings.Group
	if err := json.Unmarshal(out["site"], &site); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out["server"], &server); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out["groups"], &groups); err != nil {
		t.Fatal(err)
	}
	if site["app_name"] != "GG Bond" || server["server_token"] == "" || len(groups) != len(settings.Groups()) || groups[0].Name != settings.GroupSite {
		t.Fatalf("settings = site %v server %v groups %d", site, server, len(groups))
	}
	if strings.Contains(string(out["subscribe"]), "rulesets_custom") {
		t.Fatal("hidden key exposed")
	}

	// Saving a server_* key drops the cached node user lists.
	gid := uint64(1)
	u := testutil.CreateUser(t, e.a, "node-user@example.com", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 1 << 30})
	svc := usersvc.New(e.a)
	if list, _ := svc.AvailableUsersForGroups(context.Background(), []uint64{gid}); len(list) != 1 {
		t.Fatalf("available = %v", list)
	}
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"app_name": "Pig Board", "invite_commission": 15, "server_pull_interval": 30, "email_whitelist_suffix": "example.com, example.org",
	}, e.adminTok, http.StatusOK)
	if list, _ := svc.AvailableUsersForGroups(context.Background(), []uint64{gid}); len(list) != 0 {
		t.Fatalf("node users not invalidated: %v", list)
	}
	if e.a.Settings.String("app_name") != "Pig Board" || e.a.Settings.Int("invite_commission") != 15 ||
		strings.Join(e.a.Settings.Strings("email_whitelist_suffix"), ",") != "example.com,example.org" {
		t.Fatal("settings not saved")
	}
	if err := json.Unmarshal(get()["site"], &site); err != nil || site["app_name"] != "Pig Board" {
		t.Fatalf("GET after PUT = %v, %v", site, err)
	}
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", map[string]any{"app_name": nil}, e.adminTok, http.StatusOK)
	if e.a.Settings.String("app_name") != "GG Bond" {
		t.Fatal("null did not reset app_name")
	}
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", map[string]any{}, e.adminTok, http.StatusOK)

	rec := testutil.Do(t, e.r, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"unknown_key": 1, "invite_commission": 150, "stop_register": "maybe", "captcha_provider": "other", "app_name": "Never saved",
	}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	he := testutil.DecodeError(t, rec)
	for _, k := range []string{"unknown_key", "invite_commission", "stop_register", "captcha_provider"} {
		if len(he.Errors[k]) == 0 {
			t.Errorf("no error for %s: %v", k, he.Errors)
		}
	}
	if e.a.Settings.String("app_name") != "GG Bond" {
		t.Fatal("invalid request saved values")
	}
	rec = testutil.Do(t, e.r, http.MethodPut, "/api/v1/admin/settings", map[string]any{"rulesets_custom": map[string]any{}}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if he := testutil.DecodeError(t, rec); len(he.Errors["rulesets_custom"]) == 0 {
		t.Fatalf("hidden key accepted: %v", he)
	}
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", `[1, 2]`, e.adminTok, http.StatusBadRequest)
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", `{"app_name": `, e.adminTok, http.StatusBadRequest)
}

func TestAdminSettingsMailTest(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_name": "Test Panel", "default_locale": "en"}))
	e.expect(t, http.MethodPost, "/api/v1/admin/settings/mail/test", map[string]any{"email": "ops@example.com"}, e.adminTok, http.StatusOK)
	msgs := testutil.Mailbox(t, e.a).Messages()
	if len(msgs) != 1 || msgs[0].To != "ops@example.com" || msgs[0].Template != mail.TplNotify || !strings.Contains(msgs[0].Subject, "Test Panel") {
		t.Fatalf("mails = %+v", msgs)
	}
	var logged int64
	if err := e.a.DB.Model(&model.MailLog{}).Where("email = ? AND error IS NULL", "ops@example.com").Count(&logged).Error; err != nil || logged != 1 {
		t.Fatalf("mail_logs = %d, %v", logged, err)
	}

	e.a.Mailer.SetMailer(opsFailingMailer{})
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/settings/mail/test", map[string]any{"email": "ops@example.com"}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if he := testutil.DecodeError(t, rec); !strings.Contains(he.Message, "connection refused") {
		t.Fatalf("error = %+v", he)
	}
	e.expect(t, http.MethodPost, "/api/v1/admin/settings/mail/test", map[string]any{"email": "not-an-email"}, e.adminTok, http.StatusBadRequest)
	e.expect(t, http.MethodPost, "/api/v1/admin/settings/mail/test", nil, e.adminTok, http.StatusBadRequest)
}
