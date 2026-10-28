package ordersvc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/stripe/stripe-go/v87"

	"github.com/linusxiong/ggboard/internal/model"
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/coinbase"
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/stripe"
)

func TestNotifyValidatesProviderQuote(t *testing.T) {
	for _, tc := range []struct {
		name, gateway, site, configured, rate, currency string
		amount                                          any
		status                                          int
		paid                                            bool
	}{
		{"stripe default currency", "stripe", "CNY", "", "", "cny", 1234, 200, true},
		{"stripe explicit currency", "stripe", "CNY", "cny", "", "cny", 1234, 200, true},
		{"stripe FX", "stripe", "CNY", "usd", "0.14", "usd", 173, 200, true},
		{"stripe FX underpaid", "stripe", "CNY", "usd", "0.14", "usd", 1, 200, false},
		{"stripe FX site amount", "stripe", "CNY", "usd", "0.14", "usd", 1234, 200, false},
		{"stripe FX wrong currency", "stripe", "CNY", "usd", "0.14", "eur", 173, 200, false},
		{"stripe default wrong currency", "stripe", "CNY", "", "", "usd", 1234, 200, false},
		{"stripe missing currency", "stripe", "CNY", "", "", "", 1234, 400, false},
		{"stripe zero", "stripe", "CNY", "usd", "0.14", "usd", 0, 400, false},
		{"stripe missing amount", "stripe", "CNY", "usd", "0.14", "usd", nil, 400, false},
		{"stripe negative", "stripe", "CNY", "", "", "cny", -1, 400, false},
		{"stripe zero decimal", "stripe", "CNY", "jpy", "20.5", "jpy", 253, 200, true},
		{"stripe zero decimal underpaid", "stripe", "CNY", "jpy", "20.5", "jpy", 252, 200, false},
		{"coinbase dollars", "coinbase", "USD", "", "", "USDC", "12.34", 200, true},
		{"coinbase FX", "coinbase", "CNY", "", "0.14", "USDC", "1.73", 200, true},
		{"coinbase FX underpaid", "coinbase", "CNY", "", "0.14", "USDC", "0.01", 200, false},
		{"coinbase FX site amount", "coinbase", "CNY", "", "0.14", "USDC", "12.34", 200, false},
		{"coinbase wrong currency", "coinbase", "CNY", "", "0.14", "USD", "1.73", 400, false},
		{"coinbase missing currency", "coinbase", "CNY", "", "0.14", "", "1.73", 400, false},
		{"coinbase zero", "coinbase", "CNY", "", "0.14", "USDC", "0", 400, false},
		{"coinbase missing amount", "coinbase", "CNY", "", "0.14", "USDC", nil, 400, false},
		{"coinbase malformed amount", "coinbase", "CNY", "", "0.14", "USDC", "bad", 400, false},
		{"coinbase negative", "coinbase", "USD", "", "", "USDC", "-1", 400, false},
		{"coinbase invalid rate", "coinbase", "CNY", "", "", "USDC", "12.34", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]any{"currency": tc.site})
			u := f.user("buyer@example.com")
			plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
			p := f.payment(true, func(p *model.Payment) {
				p.Gateway = tc.gateway
				p.Config = model.JSONObject[string]{"webhook_secret": "fixture-webhook-secret", "currency": tc.configured, "exchange_rate": tc.rate}
			})
			o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
			if err != nil {
				t.Fatal(err)
			}
			// The charge includes handling; the order's plan price alone must never be compared.
			if err := f.a.DB.Model(o).Updates(map[string]any{"payment_id": p.ID, "handling_amount": 234}).Error; err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"id": "fixture-checkout", "currency": tc.currency,
				"metadata": map[string]string{"trade_no": o.TradeNo, "payment_uuid": p.UUID}}
			var body map[string]any
			if tc.gateway == "stripe" {
				payload["client_reference_id"], payload["payment_status"] = o.TradeNo, "paid"
				if tc.amount != nil {
					payload["amount_total"] = tc.amount
				}
				body = map[string]any{"id": "fixture-event", "object": "event", "type": "checkout.session.completed", "data": map[string]any{"object": payload}}
			} else {
				payload["eventType"], payload["status"] = "checkout.payment.success", "COMPLETED"
				if tc.amount != nil {
					payload["amount"] = tc.amount
				}
				body = payload
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, NotifyPath+p.Gateway+"/"+p.UUID, strings.NewReader(string(raw)))
			ts := time.Now()
			if tc.gateway == "stripe" {
				sig := sdk.ComputeSignature(ts, raw, p.Config["webhook_secret"])
				r.Header.Set("Stripe-Signature", "t="+strconv.FormatInt(ts.Unix(), 10)+",v1="+hex.EncodeToString(sig))
			} else {
				stamp := strconv.FormatInt(ts.Unix(), 10)
				mac := hmac.New(sha256.New, []byte(p.Config["webhook_secret"]))
				mac.Write([]byte(stamp + "." + string(raw)))
				r.Header.Set("X-Webhook-Signature", "t="+stamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
			}
			resp := f.svc.HandleNotify(ctx, p.Gateway, p.UUID, r)
			if resp.Status != tc.status {
				t.Fatalf("status = %d, want %d", resp.Status, tc.status)
			}
			stored, user := f.order(o.TradeNo), f.reload(u)
			if (stored.Status == model.OrderStatusCompleted) != tc.paid || (user.PlanID != nil) != tc.paid ||
				(user.GroupID != nil) != tc.paid || (user.TransferEnable > 0) != tc.paid {
				t.Fatalf("paid = %v, status = %d, plan = %v, group = %v, traffic = %d", tc.paid, stored.Status, user.PlanID, user.GroupID, user.TransferEnable)
			}
		})
	}
}
