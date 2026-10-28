package ordersvc

import (
	"strconv"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestGatewayMatches(t *testing.T) {
	for param, gw := range map[string]string{
		"epay": "epay", "EPay": "epay", "EpusdtPay": "epusdt", "AlipayF2F": "alipay_f2f", "alipay_f2f": "alipay_f2f",
		"StripeALLInOne": "stripe", "StripeCheckout": "stripe", "Coinbase": "coinbase", "BTCPay": "btcpay",
	} {
		if !GatewayMatches(param, gw) {
			t.Errorf("%s should match %s", param, gw)
		}
	}
	if GatewayMatches("EPay", "stripe") || GatewayMatches("", "epay") {
		t.Fatal("mismatch accepted")
	}
}

func TestNotifyCallerChecks(t *testing.T) {
	f := newFixture(t)
	admin := testutil.CreateAdmin(t, f.a, "root@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	pay := f.payment(true)
	other := f.payment(true, func(p *model.Payment) { p.UUID = strings.Repeat("ef", 16); p.Name = "Other" })
	disabled := f.payment(false, func(p *model.Payment) { p.UUID = strings.Repeat("01", 16) })
	u := f.user("n@example.com")
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); err != nil {
		t.Fatal(err)
	}
	body := func(tradeNo, cb string, amount int) string {
		return `{"trade_no":"` + tradeNo + `","callback_no":"` + cb + `","amount":` + strconv.Itoa(amount) + `}`
	}
	adminMails := func() int {
		n := 0
		for _, m := range testutil.Mails(t, f.a) {
			if m.To == admin.Email && m.Template == mail.TplNotify {
				n++
			}
		}
		return n
	}

	// Verification-type failures → 400 fail.
	if resp := f.notify(pay, fakeName, "wrong", body(o.TradeNo, "x", 1000)); resp.Status != 400 || resp.Body != "fail" {
		t.Fatalf("bad signature = %+v", resp)
	}
	if resp := f.notify(pay, "epay", "s3cret", body(o.TradeNo, "x", 1000)); resp.Status != 400 {
		t.Fatalf("gateway mismatch = %+v", resp)
	}
	if resp := f.notify(disabled, fakeName, "s3cret", body(o.TradeNo, "x", 1000)); resp.Status != 400 {
		t.Fatalf("disabled = %+v", resp)
	}
	unknown := &model.Payment{UUID: strings.Repeat("99", 16)}
	if resp := f.notify(unknown, fakeName, "s3cret", body(o.TradeNo, "x", 1000)); resp.Status != 400 {
		t.Fatalf("unknown uuid = %+v", resp)
	}
	// Amount mismatch: acknowledged, not paid, admin alerted.
	if resp := f.notify(pay, fakeName, "s3cret", body(o.TradeNo, "x", 1)); resp.Status != 200 || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("amount mismatch = %+v", resp)
	}
	if adminMails() != 1 {
		t.Fatal("amount mismatch must alert the admin")
	}
	// Paid through another payment than the one checked out with.
	if resp := f.notify(other, fakeName, "s3cret", body(o.TradeNo, "x", 1000)); resp.Status != 200 || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("payment mismatch = %+v", resp)
	}
	if adminMails() != 2 {
		t.Fatal("payment mismatch must alert the admin")
	}
	// Unknown order: acknowledged + alert.
	if resp := f.notify(pay, fakeName, "s3cret", body("20990101000000000000", "x", 1000)); resp.Status != 200 || adminMails() != 3 {
		t.Fatalf("unknown order = %+v", resp)
	}
	// Cancelled by the timeout before the payment arrived: callback stored, not fulfilled, alert.
	if _, err := f.svc.Cancel(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if resp := f.notify(pay, "fake_ordersvc", "s3cret", body(o.TradeNo, "late-1", 1000)); resp.Status != 200 {
		t.Fatalf("cancelled = %+v", resp)
	}
	got := f.order(o.TradeNo)
	if got.Status != model.OrderStatusCancelled || got.CallbackNo != "late-1" || adminMails() != 4 {
		t.Fatalf("cancelled order = %+v", got)
	}
	if f.reload(u).PlanID != nil {
		t.Fatal("cancelled order must not be fulfilled")
	}
}
