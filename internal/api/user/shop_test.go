package user_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment/gateways/epay"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type shopTestPlan struct {
	ID        uint64           `json:"id"`
	Name      string           `json:"name"`
	Prices    map[string]int64 `json:"prices"`
	SoldOut   bool             `json:"sold_out"`
	IsCurrent bool             `json:"is_current"`
	GroupID   *uint64          `json:"group_id"`
	Capacity  *int             `json:"capacity_limit"`
}

func shopFixture(t *testing.T) (*app.App, http.Handler) {
	t.Helper()
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com", "app_name": "GG Bond"}))
	return a, testutil.NewRouterWith(t, a, user.Register, guest.Register)
}

func shopCreate[T any](t *testing.T, a *app.App, v *T) *T {
	t.Helper()
	if err := a.DB.Create(v).Error; err != nil {
		t.Fatal(err)
	}
	return v
}

func TestShopPlansAndPaymentMethods(t *testing.T) {
	a, r := shopFixture(t)
	one := 1
	open := shopCreate(t, a, &model.Plan{Name: "Open", Show: true, Sell: true, Renew: true, Prices: model.JSONObject[int64]{"monthly": 1000}})
	hidden := shopCreate(t, a, &model.Plan{Name: "Hidden", Show: false, Sell: true, Renew: true, Prices: model.JSONObject[int64]{"monthly": 500}})
	full := shopCreate(t, a, &model.Plan{Name: "Full", Show: true, Sell: true, CapacityLimit: &one, Prices: model.JSONObject[int64]{"monthly": 100}})
	future := time.Now().Add(time.Hour)
	testutil.CreateUser(t, a, "holder@example.com", usersvc.CreateOptions{PlanID: &full.ID, ExpiredAt: &future})
	u := testutil.CreateUser(t, a, "shopper@example.com", usersvc.CreateOptions{PlanID: &hidden.ID, ExpiredAt: &future})
	tok := testutil.Login(t, a, u)

	plans, total := testutil.DecodeList[shopTestPlan](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/plans", nil, tok))
	if total != 3 {
		t.Fatalf("plans = %+v", plans)
	}
	seen := map[uint64]shopTestPlan{}
	for _, p := range plans {
		seen[p.ID] = p
		if p.GroupID != nil || p.Capacity != nil {
			t.Fatalf("internal fields leaked: %+v", p)
		}
	}
	if !seen[hidden.ID].IsCurrent || !seen[full.ID].SoldOut || seen[open.ID].SoldOut {
		t.Fatalf("flags = %+v", seen)
	}
	got := testutil.Decode[shopTestPlan](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/plans/"+strconv.FormatUint(hidden.ID, 10), nil, tok))
	if got.Name != "Hidden" || got.Prices["monthly"] != 500 {
		t.Fatalf("plan = %+v", got)
	}
	other := testutil.Login(t, a, testutil.CreateUser(t, a, "other@example.com"))
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/plans/"+strconv.FormatUint(hidden.ID, 10), nil, other), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/plans", nil, ""), http.StatusUnauthorized)

	// Guest list: show && sell with capacity left, no internal fields.
	gplans, _ := testutil.DecodeList[shopTestPlan](t, testutil.Do(t, r, http.MethodGet, "/api/v1/guest/plans", nil, ""))
	if len(gplans) != 1 || gplans[0].ID != open.ID {
		t.Fatalf("guest plans = %+v", gplans)
	}
	if body := testutil.Do(t, r, http.MethodGet, "/api/v1/guest/plans", nil, "").Body.String(); strings.Contains(body, "capacity_limit") || strings.Contains(body, "group_id") {
		t.Fatalf("guest body leaks internals: %s", body)
	}

	shopCreate(t, a, &model.Payment{UUID: strings.Repeat("a", 32), Gateway: epay.Name, Name: "Alipay", Icon: "alipay.svg", Enabled: true, Sort: 2,
		Config: model.JSONObject[string]{"key": "secret-key"}, HandlingFeeFixed: 5})
	shopCreate(t, a, &model.Payment{UUID: strings.Repeat("b", 32), Gateway: epay.Name, Name: "Off", Enabled: false})
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/payment-methods", nil, tok)
	type method struct {
		ID               uint64 `json:"id"`
		Name             string `json:"name"`
		Icon             string `json:"icon"`
		HandlingFeeFixed int64  `json:"handling_fee_fixed"`
	}
	methods, _ := testutil.DecodeList[method](t, rec)
	if len(methods) != 1 || methods[0].Name != "Alipay" || methods[0].HandlingFeeFixed != 5 || strings.Contains(rec.Body.String(), "secret-key") {
		t.Fatalf("methods = %s", rec.Body.String())
	}
}

type shopTestOrder struct {
	TradeNo        string `json:"trade_no"`
	Status         int    `json:"status"`
	Type           int    `json:"type"`
	TotalAmount    int64  `json:"total_amount"`
	DiscountAmount int64  `json:"discount_amount"`
	HandlingAmount int64  `json:"handling_amount"`
	Plan           *struct {
		Name string `json:"name"`
	} `json:"plan"`
	Payment *struct {
		Name string `json:"name"`
	} `json:"payment"`
}

func TestShopOrderFlowWithEPay(t *testing.T) {
	a, r := shopFixture(t)
	plan := shopCreate(t, a, &model.Plan{Name: "Pro", GroupID: 3, TransferEnable: 50, Show: true, Sell: true, Renew: true,
		Prices: model.JSONObject[int64]{"monthly": 1000, "yearly": 9000}})
	shopCreate(t, a, &model.Coupon{Code: "TENOFF", Name: "Ten", Type: model.CouponTypePercent, Value: 10, Show: true,
		StartedAt: time.Now().Add(-time.Hour), EndedAt: time.Now().Add(time.Hour)})
	pay := shopCreate(t, a, &model.Payment{UUID: strings.Repeat("c", 32), Gateway: epay.Name, Name: "EPay", Enabled: true,
		Config: model.JSONObject[string]{"url": "https://pay.example.com", "pid": "1001", "key": "epay_test_key_123", "type": "alipay"}})
	u := testutil.CreateUser(t, a, "flow@example.com")
	tok := testutil.Login(t, a, u)

	// Coupon check.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/coupons/check", map[string]any{"code": "NOPE", "plan_id": plan.ID, "period": "monthly"}, tok), http.StatusBadRequest)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/coupons/check", map[string]any{"code": "TENOFF"}, tok), http.StatusBadRequest)
	type coupon struct {
		Code  string `json:"code"`
		Type  int    `json:"type"`
		Value int64  `json:"value"`
	}
	if c := testutil.Decode[coupon](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/coupons/check", map[string]any{"code": "TENOFF", "plan_id": plan.ID, "period": "monthly"}, tok)); c.Value != 10 || c.Type != 2 {
		t.Fatalf("coupon = %+v", c)
	}

	// Create.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{"plan_id": plan.ID}, tok), http.StatusBadRequest)
	rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{"plan_id": plan.ID, "period": "monthly", "coupon_code": "TENOFF"}, tok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	created := testutil.Decode[shopTestOrder](t, rec)
	if created.TradeNo == "" || created.Status != 0 {
		t.Fatalf("created = %+v", created)
	}
	// Second order → 409 with the pending trade_no.
	rec = testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{"plan_id": plan.ID, "period": "yearly"}, tok)
	e := testutil.DecodeError(t, rec)
	if e.Status != http.StatusConflict || e.Code != "conflict" || len(e.Errors["trade_no"]) != 1 || e.Errors["trade_no"][0] != created.TradeNo {
		t.Fatalf("pending conflict = %+v", e)
	}

	// Detail, list, status.
	o := testutil.Decode[shopTestOrder](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/orders/"+created.TradeNo, nil, tok))
	if o.TotalAmount != 900 || o.DiscountAmount != 100 || o.Plan == nil || o.Plan.Name != "Pro" {
		t.Fatalf("order = %+v", o)
	}
	list, total := testutil.DecodeList[shopTestOrder](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/orders?status=0", nil, tok))
	if total != 1 || list[0].TradeNo != created.TradeNo {
		t.Fatalf("list = %+v", list)
	}
	other := testutil.Login(t, a, testutil.CreateUser(t, a, "spy@example.com"))
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/orders/"+created.TradeNo, nil, other), http.StatusNotFound)

	// Checkout through EPay.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+created.TradeNo+"/checkout", map[string]any{}, tok), http.StatusBadRequest)
	type payResult struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	res := testutil.Decode[payResult](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+created.TradeNo+"/checkout", map[string]any{"payment_id": pay.ID}, tok))
	if res.Type != "url" || !strings.HasPrefix(res.Data, "https://pay.example.com/submit.php?") {
		t.Fatalf("checkout = %+v", res)
	}
	q, _ := url.ParseQuery(strings.SplitN(res.Data, "?", 2)[1])
	if q.Get("money") != "9.00" || q.Get("out_trade_no") != created.TradeNo || q.Get("name") != "GG Bond - Pro" ||
		q.Get("notify_url") != "https://panel.example.com/api/v1/guest/payment/notify/epay/"+pay.UUID ||
		q.Get("return_url") != "https://panel.example.com/api/v1/guest/payment/return/"+created.TradeNo {
		t.Fatalf("epay query = %v", q)
	}

	// EPay notifies with GET, using the Xboard class name in the path.
	params := map[string]string{"pid": "1001", "trade_no": "EP123", "out_trade_no": created.TradeNo, "type": "alipay", "name": "GG Bond - Pro", "money": "9.00", "trade_status": "TRADE_SUCCESS"}
	nq := url.Values{}
	for k, v := range params {
		nq.Set(k, v)
	}
	nq.Set("sign", epay.Sign(params, "epay_test_key_123"))
	nq.Set("sign_type", "MD5")
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/guest/payment/notify/EPay/"+pay.UUID+"?"+nq.Encode(), nil, "")
	if rec.Code != 200 || rec.Body.String() != "success" {
		t.Fatalf("notify = %d %s", rec.Code, rec.Body.String())
	}
	// A forged notify is rejected.
	nq.Set("money", "0.01")
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/guest/payment/notify/epay/"+pay.UUID+"?"+nq.Encode(), nil, "")
	if rec.Code != 400 || rec.Body.String() != "fail" {
		t.Fatalf("forged notify = %d %s", rec.Code, rec.Body.String())
	}
	type st struct {
		Status int `json:"status"`
	}
	if s := testutil.Decode[st](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/orders/"+created.TradeNo+"/status", nil, tok)); s.Status != int(model.OrderStatusCompleted) {
		t.Fatalf("status = %+v", s)
	}
	var fresh model.User
	a.DB.First(&fresh, u.ID)
	if fresh.PlanID == nil || *fresh.PlanID != plan.ID || fresh.TransferEnable != 50*model.BytesPerGB || fresh.ExpiredAt == nil {
		t.Fatalf("user = %+v", fresh)
	}
	if paid := testutil.Decode[payResult](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+created.TradeNo+"/checkout", map[string]any{"payment_id": pay.ID}, tok)); paid.Type != ordersvc.PayTypePaid {
		t.Fatalf("checkout paid = %+v", paid)
	}
	// Cancelling a paid order fails; return URL redirects to the SPA.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+created.TradeNo+"/cancel", nil, tok), http.StatusConflict)
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/guest/payment/return/"+created.TradeNo+"?pid=1&sign=x", nil, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://panel.example.com/orders/"+created.TradeNo {
		t.Fatalf("return = %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// New order then cancel.
	o2 := testutil.Decode[shopTestOrder](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{"plan_id": plan.ID, "period": "yearly"}, tok))
	if o2.Status != 0 {
		t.Fatalf("renew = %+v", o2)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+o2.TradeNo+"/cancel", nil, tok), http.StatusOK)
	if s := testutil.Decode[st](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/orders/"+o2.TradeNo+"/status", nil, tok)); s.Status != int(model.OrderStatusCancelled) {
		t.Fatalf("cancelled status = %+v", s)
	}
}

func TestShopCapacityConflict(t *testing.T) {
	a, r := shopFixture(t)
	one := 1
	plan := shopCreate(t, a, &model.Plan{Name: "Tiny", Show: true, Sell: true, Renew: true, CapacityLimit: &one, Prices: model.JSONObject[int64]{"monthly": 100}})
	future := time.Now().Add(time.Hour)
	testutil.CreateUser(t, a, "first@example.com", usersvc.CreateOptions{PlanID: &plan.ID, ExpiredAt: &future})
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "second@example.com"))
	rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{"plan_id": plan.ID, "period": "monthly"}, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusConflict || e.Code != "conflict" {
		t.Fatalf("capacity = %+v", e)
	}
}

// A hidden plan must answer a coupon check exactly like an unknown id: the period check used to tell "exists but
// not for this period" apart from "plan not found", which enumerated hidden plans and their periods.
func TestShopCouponCheckHidesHiddenPlans(t *testing.T) {
	a, r := shopFixture(t)
	hidden := shopCreate(t, a, &model.Plan{Name: "Hidden", Show: false, Sell: true, Renew: true, Prices: model.JSONObject[int64]{"monthly": 500}})
	shopCreate(t, a, &model.Coupon{Code: "TENOFF", Name: "Ten", Type: model.CouponTypePercent, Value: 10, Show: true,
		StartedAt: time.Now().Add(-time.Hour), EndedAt: time.Now().Add(time.Hour)})
	future := time.Now().Add(time.Hour)
	shopper := testutil.Login(t, a, testutil.CreateUser(t, a, "shopper@example.com"))
	holder := testutil.Login(t, a, testutil.CreateUser(t, a, "holder@example.com", usersvc.CreateOptions{PlanID: &hidden.ID, ExpiredAt: &future}))

	check := func(tok string, planID uint64, period string) (int, string) {
		rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/coupons/check", map[string]any{"code": "TENOFF", "plan_id": planID, "period": period}, tok)
		return rec.Code, rec.Body.String()
	}
	missingCode, missingBody := check(shopper, hidden.ID+1000, "yearly")
	if missingCode != http.StatusBadRequest || !strings.Contains(missingBody, "plan not found") {
		t.Fatalf("unknown plan: %d %s", missingCode, missingBody)
	}
	for _, period := range []string{"yearly", "monthly"} {
		if code, body := check(shopper, hidden.ID, period); code != missingCode || body != missingBody {
			t.Fatalf("hidden plan (%s): %d %s, want the unknown-plan answer %d %s", period, code, body, missingCode, missingBody)
		}
	}
	// The holder of the hidden plan still sees it (renewals keep working).
	if code, body := check(holder, hidden.ID, "monthly"); code != http.StatusOK {
		t.Fatalf("holder: %d %s", code, body)
	}
	if code, body := check(holder, hidden.ID, "yearly"); code != http.StatusBadRequest || !strings.Contains(body, "period") {
		t.Fatalf("holder, missing period: %d %s", code, body)
	}
}
