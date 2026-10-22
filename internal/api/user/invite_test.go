package user_test

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func inviteSeedOrder(t *testing.T, a *app.App, buyer, inviter uint64, status model.OrderStatus, cs model.CommissionStatus, commission int64) {
	t.Helper()
	o := model.Order{
		TradeNo: fmt.Sprintf("T%d%d%d%d", buyer, status, cs, time.Now().UnixNano()), UserID: buyer, PlanID: 1, InviteUserID: &inviter,
		Type: model.OrderTypeNew, Period: model.PeriodMonthly, TotalAmount: commission * 10, Status: status,
		CommissionStatus: cs, CommissionBalance: commission,
	}
	if err := a.DB.Create(&o).Error; err != nil {
		t.Fatal(err)
	}
}

func inviteSeedLog(t *testing.T, a *app.App, inviter, buyer uint64, tradeNo string, order, get int64, at time.Time) {
	t.Helper()
	l := model.CommissionLog{InviteUserID: inviter, UserID: buyer, TradeNo: tradeNo, OrderAmount: order, GetAmount: get, CreatedAt: at}
	if err := a.DB.Create(&l).Error; err != nil {
		t.Fatal(err)
	}
}

func TestInviteOverviewStats(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	alice := testutil.CreateUser(t, a, "alice@example.com")
	other := testutil.CreateUser(t, a, "other@example.com")
	tok := testutil.Login(t, a, alice)

	ov := testutil.Decode[invitesvc.Overview](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite", nil, tok))
	if ov.Codes == nil || len(ov.Codes) != 0 || ov.Stat != (invitesvc.Stat{CommissionRate: 10, InviteGenLimit: 5}) {
		t.Fatalf("empty overview = %+v", ov)
	}

	b1 := testutil.CreateUser(t, a, "b1@example.com", usersvc.CreateOptions{InviteUserID: &alice.ID})
	b2 := testutil.CreateUser(t, a, "b2@example.com", usersvc.CreateOptions{InviteUserID: &alice.ID})
	testutil.CreateUser(t, a, "b3@example.com", usersvc.CreateOptions{InviteUserID: &other.ID})
	inviteSeedOrder(t, a, b1.ID, alice.ID, model.OrderStatusCompleted, model.CommissionStatusPending, 1000)
	inviteSeedOrder(t, a, b2.ID, alice.ID, model.OrderStatusCompleted, model.CommissionStatusProcessing, 500)
	inviteSeedOrder(t, a, b1.ID, alice.ID, model.OrderStatusCompleted, model.CommissionStatusValid, 700)    // already paid
	inviteSeedOrder(t, a, b1.ID, alice.ID, model.OrderStatusCompleted, model.CommissionStatusInvalid, 900)  // rejected
	inviteSeedOrder(t, a, b2.ID, alice.ID, model.OrderStatusPending, model.CommissionStatusPending, 300)    // unpaid order
	inviteSeedOrder(t, a, b2.ID, alice.ID, model.OrderStatusCancelled, model.CommissionStatusPending, 300)  // cancelled
	inviteSeedOrder(t, a, b1.ID, other.ID, model.OrderStatusCompleted, model.CommissionStatusPending, 4000) // other inviter
	now := time.Now()
	inviteSeedLog(t, a, alice.ID, b1.ID, "TRADE-1", 10000, 300, now.Add(-2*time.Hour))
	inviteSeedLog(t, a, alice.ID, b2.ID, "TRADE-2", 5000, 200, now.Add(-time.Hour))
	inviteSeedLog(t, a, alice.ID, b2.ID, "TRADE-0", 5000, 0, now)
	inviteSeedLog(t, a, other.ID, b1.ID, "TRADE-X", 9000, 999, now)

	ov = testutil.Decode[invitesvc.Overview](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite", nil, tok))
	want := invitesvc.Stat{RegisterCount: 2, CommissionPending: 1500, CommissionPaid: 500, CommissionRate: 10, InviteGenLimit: 5}
	if ov.Stat != want {
		t.Fatalf("stat = %+v, want %+v", ov.Stat, want)
	}

	// Per-user rate override and the first-level share of distributed commissions.
	if err := a.DB.Model(&model.User{}).Where("id = ?", alice.ID).Update("commission_rate", 25).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Settings.Set(map[string]any{"commission_distribution_enable": true, "commission_distribution_l1": 50, "invite_gen_limit": 3}); err != nil {
		t.Fatal(err)
	}
	ov = testutil.Decode[invitesvc.Overview](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite", nil, tok))
	if ov.Stat.CommissionRate != 25 || ov.Stat.CommissionPending != 750 || ov.Stat.InviteGenLimit != 3 {
		t.Fatalf("stat with overrides = %+v", ov.Stat)
	}

	// Commission history: get_amount > 0, newest first, paginated.
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite/commissions?page_size=1", nil, tok)
	logs, total := testutil.DecodeList[invitesvc.Commission](t, rec)
	if total != 2 || len(logs) != 1 || logs[0].TradeNo != "TRADE-2" || logs[0].OrderAmount != 5000 || logs[0].GetAmount != 200 {
		t.Fatalf("commissions page 1 = %s", rec.Body.String())
	}
	logs, _ = testutil.DecodeList[invitesvc.Commission](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite/commissions?page=2&page_size=1", nil, tok))
	if len(logs) != 1 || logs[0].TradeNo != "TRADE-1" {
		t.Fatalf("commissions page 2 = %+v", logs)
	}
}

func inviteReloadUser(t *testing.T, a *app.App, id uint64) *model.User {
	t.Helper()
	var u model.User
	if err := a.DB.First(&u, id).Error; err != nil {
		t.Fatal(err)
	}
	return &u
}

func TestInviteCodes(t *testing.T) {
	a, r := ticketTestEnv(t, map[string]any{"invite_gen_limit": 2})
	alice := testutil.CreateUser(t, a, "alice@example.com")
	tok := testutil.Login(t, a, alice)
	alnum := regexp.MustCompile(`^[A-Za-z0-9]{32}$`)
	var codes []string
	for range 2 {
		rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/invite/codes", nil, tok)
		testutil.ExpectStatus(t, rec, http.StatusCreated)
		c := testutil.Decode[map[string]string](t, rec)["code"]
		if !alnum.MatchString(c) {
			t.Fatalf("code %q", c)
		}
		codes = append(codes, c)
	}
	if codes[0] == codes[1] {
		t.Fatal("codes must be random")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/invite/codes", nil, tok), http.StatusBadRequest)

	// A consumed code no longer counts towards the limit and is no longer listed.
	if err := a.DB.Model(&model.InviteCode{}).Where("code = ?", codes[0]).Updates(map[string]any{"status": true, "pv": 3}).Error; err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/invite/codes", nil, tok), http.StatusCreated)
	if err := a.DB.Model(&model.InviteCode{}).Where("code = ?", codes[1]).Update("pv", 7).Error; err != nil {
		t.Fatal(err)
	}
	ov := testutil.Decode[invitesvc.Overview](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite", nil, tok))
	if len(ov.Codes) != 2 || ov.Codes[1].Code != codes[1] || ov.Codes[1].PV != 7 || ov.Codes[0].CreatedAt.IsZero() {
		t.Fatalf("codes = %+v", ov.Codes)
	}
	for _, c := range ov.Codes {
		if c.Code == codes[0] {
			t.Fatal("consumed code listed")
		}
	}
	// Another user's codes are separate.
	bob := testutil.Login(t, a, testutil.CreateUser(t, a, "bob@example.com"))
	if ov := testutil.Decode[invitesvc.Overview](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/invite", nil, bob)); len(ov.Codes) != 0 {
		t.Fatalf("bob codes = %+v", ov.Codes)
	}
	// invite_gen_limit = 0 disables generation.
	if err := a.Settings.Set(map[string]any{"invite_gen_limit": 0}); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/invite/codes", nil, bob), http.StatusBadRequest)
}

func TestCommissionTransfer(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	plan := model.Plan{GroupID: 1, Name: "Pro", Prices: model.JSONObject[int64]{"monthly": 1000}}
	if err := a.DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	alice := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{PlanID: &plan.ID, Balance: 50})
	if err := a.DB.Model(&model.User{}).Where("id = ?", alice.ID).Update("commission_balance", 1000).Error; err != nil {
		t.Fatal(err)
	}
	tok := testutil.Login(t, a, alice)
	path := "/api/v1/user/commission/transfer"

	for _, body := range []any{nil, map[string]any{"amount": 0}, map[string]any{"amount": -5}, map[string]any{"amount": "10"}} {
		rec := testutil.Do(t, r, http.MethodPost, path, body, tok)
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if e := testutil.DecodeError(t, rec); len(e.Errors["amount"]) == 0 {
			t.Fatalf("body %v: %+v", body, e)
		}
	}
	rec := testutil.Do(t, r, http.MethodPost, path, map[string]any{"amount": 1001}, tok)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)

	rec = testutil.Do(t, r, http.MethodPost, path, map[string]any{"amount": 400}, tok)
	type view struct {
		ID                uint64 `json:"id"`
		Email             string `json:"email"`
		Balance           int64  `json:"balance"`
		CommissionBalance int64  `json:"commission_balance"`
		Plan              *struct {
			ID   uint64 `json:"id"`
			Name string `json:"name"`
		} `json:"plan"`
		AvatarURL string `json:"avatar_url"`
	}
	v := testutil.Decode[view](t, rec)
	if v.ID != alice.ID || v.Email != alice.Email || v.Balance != 450 || v.CommissionBalance != 600 ||
		v.Plan == nil || v.Plan.Name != "Pro" || !strings.HasPrefix(v.AvatarURL, "https://www.gravatar.com/avatar/") {
		t.Fatalf("view = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("view leaks secrets: %s", rec.Body.String())
	}
	u := inviteReloadUser(t, a, alice.ID)
	if u.Balance != 450 || u.CommissionBalance != 600 {
		t.Fatalf("db = %d/%d", u.Balance, u.CommissionBalance)
	}
	// Exactly the remaining commission is allowed, then nothing is left.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path, map[string]any{"amount": 600}, tok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path, map[string]any{"amount": 1}, tok), http.StatusBadRequest)
	if u := inviteReloadUser(t, a, alice.ID); u.Balance != 1050 || u.CommissionBalance != 0 {
		t.Fatalf("db = %d/%d", u.Balance, u.CommissionBalance)
	}
}

func TestCommissionWithdraw(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	alice := testutil.CreateUser(t, a, "alice@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", alice.ID).Update("commission_balance", 600).Error; err != nil {
		t.Fatal(err)
	}
	tok := testutil.Login(t, a, alice)
	path := "/api/v1/user/commission/withdraw"
	body := map[string]any{"method": "USDT", "account": "T-wallet-example"}

	// Default minimum is 100 (currency units) = 10000 cents.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path, body, tok), http.StatusBadRequest)
	if err := a.Settings.Set(map[string]any{"commission_withdraw_limit": 5}); err != nil {
		t.Fatal(err)
	}
	for field, b := range map[string]map[string]any{
		"method":  {"method": "PayPal", "account": "x"},
		"account": {"method": "USDT", "account": "  "},
	} {
		rec := testutil.Do(t, r, http.MethodPost, path, b, tok)
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if e := testutil.DecodeError(t, rec); len(e.Errors[field]) == 0 {
			t.Fatalf("%s: %+v", field, e)
		}
	}

	rec := testutil.Do(t, r, http.MethodPost, path, body, tok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	id := testutil.Decode[map[string]uint64](t, rec)["ticket_id"]
	var tk model.Ticket
	if err := a.DB.Preload("Messages").First(&tk, id).Error; err != nil {
		t.Fatal(err)
	}
	if tk.UserID != alice.ID || tk.Level != model.TicketLevelHigh || tk.Subject != "[提现申请] 本工单由系统发出" || len(tk.Messages) != 1 {
		t.Fatalf("ticket = %+v", tk)
	}
	msg := tk.Messages[0].Message
	for _, s := range []string{"提现方式：USDT", "提现账号：T-wallet-example", "提现金额：6.00 CNY"} {
		if !strings.Contains(msg, s) {
			t.Fatalf("message %q lacks %q", msg, s)
		}
	}
	if u := inviteReloadUser(t, a, alice.ID); u.CommissionBalance != 600 {
		t.Fatalf("commission must not be deducted: %d", u.CommissionBalance)
	}
	// The withdrawal ticket is the user's open ticket.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path, body, tok), http.StatusConflict)

	// English site locale; closing the request allows a new one.
	if err := a.DB.Model(&model.Ticket{}).Where("id = ?", id).Update("status", model.TicketStatusClosed).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Settings.Set(map[string]any{"default_locale": "en", "currency": "USD"}); err != nil {
		t.Fatal(err)
	}
	rec = testutil.Do(t, r, http.MethodPost, path, map[string]any{"method": "Alipay", "account": "pay@example.com"}, tok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	var en model.Ticket
	if err := a.DB.Preload("Messages").First(&en, testutil.Decode[map[string]uint64](t, rec)["ticket_id"]).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(en.Subject, "[Withdrawal Request]") || !strings.Contains(en.Messages[0].Message, "Withdrawal amount: 6.00 USD") {
		t.Fatalf("english ticket = %+v", en)
	}

	// Disabled withdrawals.
	if err := a.Settings.Set(map[string]any{"withdraw_close_enable": true}); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path, body, tok), http.StatusForbidden)
}
