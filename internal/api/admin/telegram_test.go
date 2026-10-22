package admin_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminTelegramEndpointsAreAdminOnly(t *testing.T) {
	e := newOpsEnv(t)
	for _, path := range []string{"/api/v1/admin/settings/telegram/webhook", "/api/v1/admin/settings/telegram/test"} {
		e.expect(t, http.MethodPost, path, nil, e.staffTok, http.StatusForbidden)
		e.expect(t, http.MethodPost, path, nil, e.userTok, http.StatusForbidden)
		e.expect(t, http.MethodPost, path, nil, "", http.StatusUnauthorized)
	}
}

func TestAdminTelegramWebhook(t *testing.T) {
	e := newOpsEnv(t)
	path := "/api/v1/admin/settings/telegram/webhook"
	rec := testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusBadRequest || err.Errors[telegram.KeyToken] == nil || err.Errors["app_url"] == nil {
		t.Fatalf("incomplete settings = %+v", err)
	}

	srv := telegramtest.Enable(t, e.a, map[string]any{"app_url": "https://panel.example.com/"})
	secret := e.a.Settings.String(telegram.KeyWebhookSecret)
	res := testutil.Decode[telegram.WebhookResult](t, testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok))
	wantURL := "https://panel.example.com/api/v1/guest/telegram/webhook/" + secret
	if !res.OK || res.Description != "Webhook was set" || res.URL != wantURL {
		t.Fatalf("result = %+v", res)
	}
	calls := srv.Calls("setWebhook")
	if len(calls) != 1 || calls[0].Params["url"] != wantURL || calls[0].Params["secret_token"] != secret ||
		!strings.Contains(string(telegramJSON(t, calls[0].Params["allowed_updates"])), `"message"`) {
		t.Fatalf("setWebhook = %+v", calls)
	}

	// Telegram's refusal is passed through.
	srv.Respond("setWebhook", func(telegramtest.Call) (int, string) {
		return telegramtest.Fail(http.StatusBadRequest, "Bad Request: bad webhook: HTTPS url must be provided for webhook")
	})
	res = testutil.Decode[telegram.WebhookResult](t, testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok))
	if res.OK || res.ErrorCode != http.StatusBadRequest || !strings.Contains(res.Description, "HTTPS url") {
		t.Fatalf("refusal = %+v", res)
	}

	// A secret that cannot be used in a URL path.
	if err := e.a.Settings.Set(map[string]any{telegram.KeyWebhookSecret: "has/slash/and?query"}); err != nil {
		t.Fatal(err)
	}
	if err := testutil.DecodeError(t, testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok)); err.Errors[telegram.KeyWebhookSecret] == nil {
		t.Fatalf("bad secret = %+v", err)
	}
	if err := e.a.Settings.Set(map[string]any{telegram.KeyWebhookSecret: nil}); err != nil { // null regenerates
		t.Fatal(err)
	}

	// Unreachable Bot API: 502 without the token.
	telegram.SetAPIBase(e.a, "http://127.0.0.1:1")
	rec = testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusBadGateway || err.Code != telegram.CodeUnavailable {
		t.Fatalf("unreachable = %+v", err)
	}
	if strings.Contains(rec.Body.String(), telegramtest.Token) {
		t.Fatal("token leaked")
	}
}

func TestAdminTelegramTest(t *testing.T) {
	e := newOpsEnv(t)
	path := "/api/v1/admin/settings/telegram/test"
	srv := telegramtest.Enable(t, e.a)
	if err := testutil.DecodeError(t, testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok)); err.Errors[telegram.KeyAdminChats] == nil {
		t.Fatalf("no chats = %+v", err)
	}
	if err := e.a.Settings.Set(map[string]any{telegram.KeyAdminChats: "111, 222"}); err != nil {
		t.Fatal(err)
	}
	srv.Respond("sendMessage", func(c telegramtest.Call) (int, string) {
		if c.ChatID() == "222" {
			return telegramtest.Fail(http.StatusBadRequest, "Bad Request: chat not found")
		}
		return telegramtest.OK(`{"message_id":1}`)
	})
	rep := testutil.Decode[telegram.TestReport](t, testutil.Do(t, e.r, http.MethodPost, path, nil, e.adminTok))
	if rep.Sent != 1 || rep.Failed != 1 || len(rep.Results) != 2 || !rep.Results[0].OK || rep.Results[0].ChatID != "111" ||
		rep.Results[1].OK || !strings.Contains(rep.Results[1].Error, "chat not found") {
		t.Fatalf("report = %+v", rep)
	}
	calls := srv.Calls("sendMessage")
	if len(calls) != 2 || !strings.Contains(calls[0].Text(), "<b>测试消息</b>") {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestAdminSystemStatusNotify(t *testing.T) {
	e := newOpsEnv(t)
	type status struct {
		Notify *notify.QueueStats `json:"notify"`
	}
	st := testutil.Decode[status](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/status", nil, e.adminTok))
	if st.Notify == nil || st.Notify.Drivers == nil || len(st.Notify.Drivers) != 0 {
		t.Fatalf("notify = %+v", st.Notify)
	}
	rec := notifytest.Attach(t, e.a)
	notify.Emit(e.a, notify.Test(e.a))
	rec.Deliveries(t)
	st = testutil.Decode[status](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/system/status", nil, e.adminTok))
	if st.Notify.Sent != 1 || st.Notify.Pending != 0 || st.Notify.Failed != 0 || len(st.Notify.Drivers) != 1 || st.Notify.Drivers[0] != notifytest.DriverName {
		t.Fatalf("notify = %+v", st.Notify)
	}
}

func TestAdminSettingsTelegramGroup(t *testing.T) {
	e := newOpsEnv(t)
	out := testutil.Decode[map[string]json.RawMessage](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/settings", nil, e.adminTok))
	var tg map[string]any
	if err := json.Unmarshal(out["telegram"], &tg); err != nil {
		t.Fatal(err)
	}
	if s, _ := tg[telegram.KeyWebhookSecret].(string); len(s) != 32 || tg[telegram.KeyToken] != "" || tg[telegram.KeyUserNotify] != true {
		t.Fatalf("telegram group = %v", tg)
	}
	rec := testutil.Do(t, e.r, http.MethodPut, "/api/v1/admin/settings", map[string]any{telegram.KeyEvents: []string{"order_paid", "bogus"}}, e.adminTok)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusBadRequest || err.Errors[telegram.KeyEvents] == nil {
		t.Fatalf("unknown event = %+v", err)
	}
	e.expect(t, http.MethodPut, "/api/v1/admin/settings", map[string]any{telegram.KeyEvents: []string{"order_new", "order_cancel"}}, e.adminTok, http.StatusOK)
	if got := e.a.Settings.Strings(telegram.KeyEvents); strings.Join(got, ",") != "order_new,order_cancel" {
		t.Fatalf("events = %v", got)
	}
}

// Cancelling from the admin API is reported as an administrator cancellation.
func TestAdminOrderCancelNotifies(t *testing.T) {
	e := newOpsEnv(t)
	rec := notifytest.Attach(t, e.a)
	plan := &model.Plan{Name: "Pro", Show: true, Sell: true, Renew: true, Prices: model.JSONObject[int64]{"monthly": 1000}}
	if err := e.a.DB.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	o, err := ordersvc.New(e.a).Create(t.Context(), e.user.ID, ordersvc.CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(t, http.MethodPost, "/api/v1/admin/orders/"+strconv.FormatUint(o.ID, 10)+"/cancel", nil, e.adminTok, http.StatusOK)
	fs := notifytest.Fields(rec.Last(t, notify.TypeOrderCancel))
	if fs["reason"] != "管理员取消" || fs["trade_no"] != o.TradeNo || fs["email"] != e.user.Email {
		t.Fatalf("order_cancel = %v", fs)
	}
}

func telegramJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
