package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	_ "github.com/linusxiong/ggboard/internal/payment/gateways" // register the built-in gateways
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type commerceEnv struct {
	a     *app.App
	r     http.Handler
	token string
}

func newCommerceEnv(t *testing.T) *commerceEnv {
	t.Helper()
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	r := testutil.NewRouterWith(t, a, admin.Register)
	return &commerceEnv{a: a, r: r, token: testutil.Login(t, a, testutil.CreateAdmin(t, a, "root@example.com"))}
}

func (e *commerceEnv) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return testutil.Do(t, e.r, method, "/api/v1/admin"+path, body, e.token)
}

func (e *commerceEnv) group(t *testing.T) uint64 {
	t.Helper()
	g := &model.ServerGroup{Name: "Default"}
	if err := e.a.DB.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func commercePath(prefix string, id uint64) string { return prefix + "/" + strconv.FormatUint(id, 10) }

type commercePlan struct {
	ID            uint64           `json:"id"`
	Name          string           `json:"name"`
	GroupID       uint64           `json:"group_id"`
	SpeedLimit    *int             `json:"speed_limit"`
	CapacityLimit *int             `json:"capacity_limit"`
	Prices        map[string]int64 `json:"prices"`
	Show          bool             `json:"show"`
	Sell          bool             `json:"sell"`
	Renew         bool             `json:"renew"`
	Sort          int              `json:"sort"`
	UserCount     int64            `json:"user_count"`
}

func TestAdminPlans(t *testing.T) {
	e := newCommerceEnv(t)
	gid := e.group(t)
	staff := testutil.Login(t, e.a, testutil.CreateStaff(t, e.a, "staff@example.com"))
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/plans", nil, staff), http.StatusForbidden)

	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/plans", map[string]any{"name": "X"}), http.StatusBadRequest)
	rec := e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Bad", "group_id": gid, "prices": map[string]any{"weekly": 1, "monthly": -1}})
	if er := testutil.DecodeError(t, rec); er.Status != 400 || er.Errors["prices.weekly"] == nil || er.Errors["prices.monthly"] == nil {
		t.Fatalf("price validation = %+v", er)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/plans", map[string]any{"name": "NoGroup", "group_id": gid + 100}), http.StatusBadRequest)
	rec = e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Pro", "group_id": gid, "transfer_enable": 100, "speed_limit": 50,
		"capacity_limit": 10, "prices": map[string]any{"monthly": 1000}, "show": true})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	pro := testutil.Decode[commercePlan](t, rec)
	if !pro.Show || !pro.Sell || !pro.Renew || pro.Prices["monthly"] != 1000 || *pro.SpeedLimit != 50 {
		t.Fatalf("created = %+v", pro)
	}
	basic := testutil.Decode[commercePlan](t, e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Basic", "group_id": gid}))

	// PATCH keeps absent fields, clears explicit nulls and can push changes to holders.
	future := time.Now().Add(time.Hour)
	holder := testutil.CreateUser(t, e.a, "holder@example.com", usersvc.CreateOptions{PlanID: &pro.ID, ExpiredAt: &future, GroupID: &gid})
	rec = e.do(t, http.MethodPatch, commercePath("/plans", pro.ID), map[string]any{"speed_limit": nil, "transfer_enable": 200, "force_update": true})
	p := testutil.Decode[commercePlan](t, rec)
	if p.SpeedLimit != nil || p.CapacityLimit == nil || *p.CapacityLimit != 10 || p.Name != "Pro" || p.UserCount != 1 {
		t.Fatalf("patched = %+v", p)
	}
	var hu model.User
	e.a.DB.First(&hu, holder.ID)
	if hu.TransferEnable != 200*model.BytesPerGB || hu.SpeedLimit != nil {
		t.Fatalf("force update = %+v", hu)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/plans", pro.ID), map[string]any{"name": ""}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/plans", 9999), map[string]any{"name": "x"}), http.StatusNotFound)

	// Sort and list (with user_count).
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/plans/sort", map[string]any{"ids": []uint64{basic.ID, pro.ID}}), http.StatusOK)
	plans, total := testutil.DecodeList[commercePlan](t, e.do(t, http.MethodGet, "/plans", nil))
	if total != 2 || plans[0].ID != basic.ID || plans[1].UserCount != 1 {
		t.Fatalf("plans = %+v", plans)
	}
	if got := testutil.Decode[commercePlan](t, e.do(t, http.MethodGet, commercePath("/plans", pro.ID), nil)); got.UserCount != 1 || got.Sort != 1 {
		t.Fatalf("get = %+v", got)
	}

	// Delete: 409 while users hold it.
	testutil.ExpectStatus(t, e.do(t, http.MethodDelete, commercePath("/plans", pro.ID), nil), http.StatusConflict)
	testutil.ExpectStatus(t, e.do(t, http.MethodDelete, commercePath("/plans", basic.ID), nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(t, http.MethodGet, commercePath("/plans", basic.ID), nil), http.StatusNotFound)
}

type commerceOrder struct {
	ID                uint64 `json:"id"`
	TradeNo           string `json:"trade_no"`
	Status            int    `json:"status"`
	CommissionStatus  int    `json:"commission_status"`
	CommissionBalance int64  `json:"commission_balance"`
	TotalAmount       int64  `json:"total_amount"`
	CallbackNo        string `json:"callback_no"`
	User              *struct {
		Email string `json:"email"`
	} `json:"user"`
	CommissionLogs []any `json:"commission_logs"`
}

func TestAdminOrders(t *testing.T) {
	e := newCommerceEnv(t)
	plan := &model.Plan{Name: "Pro", Show: true, Sell: true, Renew: true, TransferEnable: 10, Prices: model.JSONObject[int64]{"monthly": 1000}}
	e.a.DB.Create(plan)
	inviter := testutil.CreateUser(t, e.a, "inv@example.com")
	u := testutil.CreateUser(t, e.a, "cust@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID})

	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/orders", map[string]any{"plan_id": plan.ID, "period": "monthly"}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/orders", map[string]any{"email": "ghost@example.com", "plan_id": plan.ID, "period": "monthly"}), http.StatusBadRequest)
	rec := e.do(t, http.MethodPost, "/orders", map[string]any{"email": "cust@example.com", "plan_id": plan.ID, "period": "monthly", "total_amount": 800})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	o := testutil.Decode[commerceOrder](t, rec)
	if o.Status != 0 || o.TotalAmount != 800 || o.User == nil || o.User.Email != "cust@example.com" {
		t.Fatalf("assigned = %+v", o)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/orders", map[string]any{"user_id": u.ID, "plan_id": plan.ID, "period": "monthly"}), http.StatusConflict)

	// List filters.
	for q, want := range map[string]int64{
		"":                          1,
		"?email=CUST@example.com":   1,
		"?email=nobody@example.com": 0,
		"?status=0":                 1,
		"?status=3":                 0,
		"?trade_no=" + o.TradeNo:    1,
		"?plan_id=" + strconv.FormatUint(plan.ID, 10):                      1,
		"?date_from=2000-01-01&date_to=" + time.Now().Format("2006-01-02"): 1,
		"?date_to=2000-01-01": 0,
	} {
		list, total := testutil.DecodeList[commerceOrder](t, e.do(t, http.MethodGet, "/orders"+q, nil))
		if total != want || (want == 1 && list[0].User.Email != "cust@example.com") {
			t.Fatalf("filter %q: total %d, want %d", q, total, want)
		}
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodGet, "/orders?date_from=yesterday", nil), http.StatusBadRequest)

	// Mark paid → fulfilled, commission computed; paying twice conflicts.
	paid := testutil.Decode[commerceOrder](t, e.do(t, http.MethodPost, commercePath("/orders", o.ID)+"/paid", nil))
	if paid.Status != 3 || paid.CallbackNo != "manual" || paid.CommissionBalance != 80 || paid.CommissionLogs == nil {
		t.Fatalf("paid = %+v", paid)
	}
	var cu model.User
	e.a.DB.First(&cu, u.ID)
	if cu.PlanID == nil || *cu.PlanID != plan.ID {
		t.Fatal("manual paid must fulfil")
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, commercePath("/orders", o.ID)+"/paid", nil), http.StatusConflict)
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, commercePath("/orders", o.ID)+"/cancel", nil), http.StatusConflict)

	// Commission status.
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/orders", o.ID), map[string]any{}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/orders", o.ID), map[string]any{"commission_status": 2}), http.StatusBadRequest)
	if got := testutil.Decode[commerceOrder](t, e.do(t, http.MethodPatch, commercePath("/orders", o.ID), map[string]any{"commission_status": 3})); got.CommissionStatus != 3 {
		t.Fatalf("commission = %+v", got)
	}

	// Cancel a fresh assignment; detail of a missing order is 404.
	o2 := testutil.Decode[commerceOrder](t, e.do(t, http.MethodPost, "/orders", map[string]any{"user_id": u.ID, "plan_id": plan.ID, "period": "monthly"}))
	if c := testutil.Decode[commerceOrder](t, e.do(t, http.MethodPost, commercePath("/orders", o2.ID)+"/cancel", nil)); c.Status != 2 {
		t.Fatalf("cancelled = %+v", c)
	}
	if d := testutil.Decode[commerceOrder](t, e.do(t, http.MethodGet, commercePath("/orders", o2.ID), nil)); d.TradeNo != o2.TradeNo {
		t.Fatalf("detail = %+v", d)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodGet, "/orders/9999", nil), http.StatusNotFound)
}

type commerceCoupon struct {
	ID        uint64 `json:"id"`
	Code      string `json:"code"`
	Value     int64  `json:"value"`
	Show      bool   `json:"show"`
	LimitUse  *int   `json:"limit_use"`
	UsedCount int64  `json:"used_count"`
}

func TestAdminCoupons(t *testing.T) {
	e := newCommerceEnv(t)
	plan := &model.Plan{Name: "Pro", Show: true, Sell: true, Renew: true, Prices: model.JSONObject[int64]{"monthly": 1000}}
	e.a.DB.Create(plan)
	start, end := time.Now().Add(-time.Hour).Format(time.RFC3339), time.Now().Add(24*time.Hour).Format(time.RFC3339)
	base := map[string]any{"name": "Promo", "type": 2, "value": 20, "started_at": start, "ended_at": end}
	with := func(kv ...any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	for name, body := range map[string]map[string]any{
		"percent > 100": with("value", 150),
		"ended first":   with("ended_at", start, "started_at", end),
		"bad period":    with("limit_periods", []string{"weekly"}),
		"bad plan":      with("limit_plan_ids", []uint64{9999}),
		"bad type":      with("type", 3),
		"missing dates": {"name": "x", "type": 1, "value": 1},
		"space in code": with("code", "A B"),
	} {
		if rec := e.do(t, http.MethodPost, "/coupons", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := e.do(t, http.MethodPost, "/coupons", with("code", "SPRING", "limit_use", 3, "limit_plan_ids", []uint64{plan.ID}, "limit_periods", []string{"monthly"}))
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	single := testutil.Decode[[]commerceCoupon](t, rec)
	if len(single) != 1 || single[0].Code != "SPRING" || !single[0].Show || *single[0].LimitUse != 3 {
		t.Fatalf("single = %+v", single)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/coupons", with("code", "SPRING")), http.StatusConflict)
	batch := testutil.Decode[[]commerceCoupon](t, e.do(t, http.MethodPost, "/coupons", with("count", 5, "code", "IGNORED")))
	codes := map[string]bool{}
	for _, c := range batch {
		codes[c.Code] = true
		if len(c.Code) != 8 || c.Code == "IGNORED" {
			t.Fatalf("generated code %q", c.Code)
		}
	}
	if len(batch) != 5 || len(codes) != 5 {
		t.Fatalf("batch = %+v", batch)
	}

	// used_count counts non-cancelled orders with the coupon.
	u := testutil.CreateUser(t, e.a, "c@example.com")
	cid := single[0].ID
	e.a.DB.Create(&model.Order{TradeNo: "T1", UserID: u.ID, PlanID: plan.ID, Period: "monthly", Type: 1, CouponID: &cid, Status: model.OrderStatusCompleted})
	e.a.DB.Create(&model.Order{TradeNo: "T2", UserID: u.ID, PlanID: plan.ID, Period: "monthly", Type: 1, CouponID: &cid, Status: model.OrderStatusCancelled})
	list, total := testutil.DecodeList[commerceCoupon](t, e.do(t, http.MethodGet, "/coupons?code=SPRING", nil))
	if total != 1 || list[0].UsedCount != 1 {
		t.Fatalf("list = %+v", list)
	}
	if _, total := testutil.DecodeList[commerceCoupon](t, e.do(t, http.MethodGet, "/coupons", nil)); total != 6 {
		t.Fatalf("total = %d", total)
	}

	// PATCH: clear limit_use with null, rename; duplicate code conflicts.
	got := testutil.Decode[commerceCoupon](t, e.do(t, http.MethodPatch, commercePath("/coupons", cid), map[string]any{"limit_use": nil, "value": 30}))
	if got.LimitUse != nil || got.Value != 30 || got.Code != "SPRING" || got.UsedCount != 1 {
		t.Fatalf("patched = %+v", got)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/coupons", cid), map[string]any{"code": batch[0].Code}), http.StatusConflict)
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/coupons", cid), map[string]any{"count": 2}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(t, http.MethodDelete, commercePath("/coupons", cid), nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(t, http.MethodGet, commercePath("/coupons", cid), nil), http.StatusNotFound)
}

type commercePayment struct {
	ID           uint64            `json:"id"`
	UUID         string            `json:"uuid"`
	Gateway      string            `json:"gateway"`
	GatewayLabel string            `json:"gateway_label"`
	Name         string            `json:"name"`
	Config       map[string]string `json:"config"`
	Enabled      bool              `json:"enabled"`
	NotifyURL    string            `json:"notify_url"`
	Sort         int               `json:"sort"`
}

func TestAdminPayments(t *testing.T) {
	e := newCommerceEnv(t)
	type field struct {
		Key      string `json:"key"`
		Type     string `json:"type"`
		Required bool   `json:"required"`
	}
	type gateway struct {
		Name   string  `json:"name"`
		Label  string  `json:"label"`
		Fields []field `json:"fields"`
	}
	gws, total := testutil.DecodeList[gateway](t, e.do(t, http.MethodGet, "/payments/gateways", nil))
	names := []string{}
	for _, g := range gws {
		names = append(names, g.Name)
		if len(g.Fields) == 0 || g.Label == "" {
			t.Fatalf("gateway %+v", g)
		}
	}
	if total != 6 || strings.Join(names, ",") != "alipay_f2f,btcpay,coinbase,epay,epusdt,stripe" {
		t.Fatalf("gateways = %v", names)
	}

	for name, body := range map[string]map[string]any{
		"unknown gateway":   {"gateway": "paypal", "name": "x", "config": map[string]string{}},
		"missing config":    {"gateway": "epay", "name": "x"},
		"required keys":     {"gateway": "epay", "name": "x", "config": map[string]string{"url": "https://pay.example.com"}},
		"bad url":           {"gateway": "epay", "name": "x", "config": map[string]string{"url": "nope", "pid": "1", "key": "k"}},
		"stripe needs rate": {"gateway": "stripe", "name": "x", "config": map[string]string{"secret_key": "sk", "webhook_secret": "wh", "currency": "usd"}},
		"bad notify":        {"gateway": "epay", "name": "x", "notify_domain": "cb.example.com", "config": map[string]string{"url": "https://p.example.com", "pid": "1", "key": "k"}},
	} {
		if rec := e.do(t, http.MethodPost, "/payments", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := e.do(t, http.MethodPost, "/payments", map[string]any{"gateway": "epay", "name": "Alipay", "enabled": true, "handling_fee_percent": 1.5,
		"config": map[string]string{"url": "https://pay.example.com/", "pid": "1001", "key": " secret "}})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	p := testutil.Decode[commercePayment](t, rec)
	if len(p.UUID) != 32 || !p.Enabled || p.Config["key"] != "secret" || p.GatewayLabel == "" ||
		p.NotifyURL != "https://panel.example.com/api/v1/guest/payment/notify/epay/"+p.UUID {
		t.Fatalf("created = %+v", p)
	}
	p2 := testutil.Decode[commercePayment](t, e.do(t, http.MethodPost, "/payments", map[string]any{"gateway": "epusdt", "name": "USDT",
		"notify_domain": "https://cb.example.com/", "config": map[string]string{"api_url": "https://usdt.example.com", "api_token": "t"}}))
	if p2.Enabled || p2.NotifyURL != "https://cb.example.com/api/v1/guest/payment/notify/epusdt/"+p2.UUID {
		t.Fatalf("second = %+v", p2)
	}

	got := testutil.Decode[commercePayment](t, e.do(t, http.MethodPatch, commercePath("/payments", p.ID), map[string]any{"enabled": false, "name": "Alipay 2"}))
	if got.Enabled || got.Name != "Alipay 2" || got.Config["pid"] != "1001" {
		t.Fatalf("patched = %+v", got)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, commercePath("/payments", p.ID), map[string]any{"config": map[string]string{"url": "https://x.example.com"}}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/payments/sort", map[string]any{"ids": []uint64{p2.ID, p.ID}}), http.StatusOK)
	list, _ := testutil.DecodeList[commercePayment](t, e.do(t, http.MethodGet, "/payments", nil))
	if len(list) != 2 || list[0].ID != p2.ID || list[1].Config["key"] != "secret" {
		t.Fatalf("list = %+v", list)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodDelete, commercePath("/payments", p2.ID), nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(t, http.MethodGet, commercePath("/payments", p2.ID), nil), http.StatusNotFound)
}
