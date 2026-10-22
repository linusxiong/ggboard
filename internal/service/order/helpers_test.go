package ordersvc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// fakeGateway verifies notifications by a shared "X-Fake-Secret" header equal to cfg["secret"]; the JSON body
// {"trade_no","callback_no","amount","event"} carries the result (event "pending" → ack without a result).
type fakeGateway struct {
	mu      sync.Mutex
	lastReq payment.PayRequest
	payErr  error
}

const fakeName = "fake_ordersvc"

var fake = &fakeGateway{}

func init() { payment.Register(fake) }

func (*fakeGateway) Name() string  { return fakeName }
func (*fakeGateway) Label() string { return "Fake" }
func (*fakeGateway) Fields() []payment.Field {
	return []payment.Field{{Key: "secret", Label: "Secret", Type: payment.FieldPassword, Required: true}}
}

func (f *fakeGateway) Pay(_ context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastReq = req
	if f.payErr != nil {
		return nil, f.payErr
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: "https://pay.example.com/" + req.TradeNo + "?s=" + cfg["secret"]}, nil
}

func (f *fakeGateway) last() payment.PayRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

func (*fakeGateway) Notify(_ context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if r.Header.Get("X-Fake-Secret") != cfg["secret"] {
		return nil, nil, errors.New("fake: bad secret")
	}
	b, _ := io.ReadAll(r.Body)
	var body struct {
		TradeNo    string `json:"trade_no"`
		CallbackNo string `json:"callback_no"`
		Amount     int64  `json:"amount"`
		Event      string `json:"event"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, nil, err
	}
	ok := &payment.NotifyResponse{Status: 200, Body: "fake-ok", ContentType: "text/plain"}
	if body.Event == "pending" {
		return nil, ok, nil
	}
	return &payment.NotifyResult{TradeNo: body.TradeNo, CallbackNo: body.CallbackNo, Amount: body.Amount}, ok, nil
}

type fixture struct {
	t   *testing.T
	a   *app.App
	svc *Service
	now time.Time
}

func newFixture(t *testing.T, settings ...map[string]any) *fixture {
	t.Helper()
	opts := []testutil.Option{testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com", "app_name": "GG Bond"})}
	for _, s := range settings {
		opts = append(opts, testutil.WithSettings(s))
	}
	a := testutil.NewTestApp(t, opts...)
	f := &fixture{t: t, a: a, now: time.Now().Truncate(time.Second)}
	f.svc = New(a)
	f.svc.Now = func() time.Time { return f.now }
	return f
}

func (f *fixture) plan(name string, prices map[string]int64, mut ...func(*model.Plan)) *model.Plan {
	f.t.Helper()
	p := &model.Plan{GroupID: 7, Name: name, TransferEnable: 100, Prices: prices, Show: true, Sell: true, Renew: true}
	for _, m := range mut {
		m(p)
	}
	if err := f.a.DB.Create(p).Error; err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) user(email string, opts ...usersvc.CreateOptions) *model.User {
	f.t.Helper()
	return testutil.CreateUser(f.t, f.a, email, opts...)
}

func (f *fixture) reload(u *model.User) *model.User {
	f.t.Helper()
	var got model.User
	if err := f.a.DB.First(&got, u.ID).Error; err != nil {
		f.t.Fatal(err)
	}
	return &got
}

func (f *fixture) order(tradeNo string) *model.Order {
	f.t.Helper()
	var o model.Order
	if err := f.a.DB.Where("trade_no = ?", tradeNo).First(&o).Error; err != nil {
		f.t.Fatal(err)
	}
	return &o
}

func (f *fixture) payment(enabled bool, mut ...func(*model.Payment)) *model.Payment {
	f.t.Helper()
	p := &model.Payment{UUID: strings.Repeat("ab", 16), Gateway: fakeName, Name: "Fake Pay", Config: model.JSONObject[string]{"secret": "s3cret"}, Enabled: enabled}
	for _, m := range mut {
		m(p)
	}
	if err := f.a.DB.Create(p).Error; err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) setUser(u *model.User, cols map[string]any) {
	f.t.Helper()
	if err := f.a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(cols).Error; err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) notify(p *model.Payment, gatewayParam, secret, body string) *payment.NotifyResponse {
	f.t.Helper()
	r := httptest.NewRequest(http.MethodPost, NotifyPath+gatewayParam+"/"+p.UUID, strings.NewReader(body))
	r.Header.Set("X-Fake-Secret", secret)
	return f.svc.HandleNotify(context.Background(), gatewayParam, p.UUID, r)
}

func status(err error) int {
	if err == nil {
		return 0
	}
	return httpx.AsError(err).Status
}

func fieldErrs(err error) map[string][]string { return httpx.AsError(err).Errors }

func ptr[T any](v T) *T { return &v }
