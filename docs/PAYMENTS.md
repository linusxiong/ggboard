# Payments (pluggable gateways)

Package `internal/payment`. Order money flow lives in `internal/service/order`; gateways only create a payment and verify callbacks.

## Interface
```go
package payment

type Field struct {
    Key, Label, Description string
    Type    string   // text | password | number | select | textarea
    Options []string // for select
    Required bool
}

type PayRequest struct {
    TradeNo     string
    TotalAmount int64  // cents
    Currency    string // settings.currency, e.g. CNY/USD
    UserID      uint64
    UserEmail   string
    NotifyURL   string // https://host/api/v1/guest/payment/notify/<gateway>/<uuid>
    ReturnURL   string // https://host/orders/<trade_no>
    ClientIP    string
}

type PayResult struct {
    Type string // "url" (redirect) | "qrcode" (render QR of Data) | "html"
    Data string
}

type NotifyResult struct {
    TradeNo    string // our order no
    CallbackNo string // gateway transaction id
    Amount     int64  // cents if gateway reports it, else 0
}

type Gateway interface {
    Name() string            // registry id, e.g. "epay"
    Label() string           // human name
    Fields() []Field         // admin form schema
    Pay(ctx context.Context, cfg map[string]string, req PayRequest) (*PayResult, error)
    // Notify verifies the callback. On success it returns the matched order and the HTTP response the gateway expects
    // (status code + body + content type); the caller marks the order paid (idempotent) and then writes the response.
    Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*NotifyResult, *NotifyResponse, error)
}
type NotifyResponse struct{ Status int; Body string; ContentType string }
```
`payment.Register(gw Gateway)` in `init()`; `payment.Get(name)`; `payment.List()`.

## Order lifecycle
1. `POST /user/orders` → order pending (status 0). Pricing: plan price for period → user `discount` → coupon → `surplus` (upgrade credit,
   when `surplus_enable`) → balance (if the user has balance it is applied automatically; remaining `total_amount`). `total_amount == 0` → fulfil immediately.
2. `POST /user/orders/:trade_no/checkout {payment_id}` → compute `handling_amount` (fixed + percent) → `gateway.Pay`.
   Response `{type, data}`; the SPA redirects (url) or renders a QR (qrcode) and polls `status`.
3. Gateway calls `ANY /guest/payment/notify/:gateway/:uuid` → look up payment by uuid, check enabled and gateway match → `Notify` →
   `orderService.Paid(tradeNo, callbackNo)` (transaction, idempotent: already paid → still success response) → `Fulfil`.
4. `Fulfil`: set plan/group/transfer/expiry per period (monthly=+1 month … onetime=no expiry; reset_traffic=u=d=0), speed/device limits from plan,
   compute `next_reset_at`, handle surplus orders (status 4), refund_amount → balance, commission (status 1 pending settle),
   `order.status=3`, send email "order paid".
5. `order_timeout` job cancels stale pending orders; `cancel` returns balance_amount to balance.

## Built-in gateways
| name | label | config fields | pay | notify |
|---|---|---|---|---|
| `epay` | 易支付 (EPay) | url, pid, key, type(select: alipay/wxpay/qqpay/"") | GET `<url>/submit.php?...&sign=md5(ksorted&key)&sign_type=MD5` → `url` | verify md5 over params (excluding sign/sign_type); `trade_status=TRADE_SUCCESS`; respond `success` |
| `epusdt` | Epusdt (USDT) | api_url, api_token, currency_type(select: usdt.trc20/usdt.polygon/tron.trx) | POST `/api/v1/order/create-transaction` JSON `{amount, order_id, redirect_url, notify_url, trade_type}` + `signature` (md5 of ksorted `k=v&` + token) → `payment_url` | POST JSON `{trade_id, order_id, amount, actual_amount, token, block_transaction_id, signature, status}`; verify signature, status==2; respond `ok` |
| `alipay_f2f` | 支付宝当面付 | app_id, private_key, alipay_public_key, (optional) sandbox | `alipay.trade.precreate` (RSA2) → `qrcode` | form POST; verify RSA2 sign; `trade_status` in (TRADE_SUCCESS, TRADE_FINISHED); respond `success` |
| `stripe` | Stripe Checkout | secret_key, publishable_key, webhook_secret, currency, payment_methods(select: card/alipay/wechat_pay/auto) | Checkout Session (`mode=payment`, `client_reference_id=trade_no`) → `url` | webhook `checkout.session.completed` with `Stripe-Signature` verify; respond 200 JSON |
| `coinbase_commerce` | Coinbase Commerce | api_key, webhook_secret | POST `/charges` (`pricing_type=fixed_price`, metadata trade_no) → `hosted_url` | `X-CC-Webhook-Signature` HMAC-SHA256; event `charge:confirmed|charge:resolved`; respond 200 |
| `btcpay` | BTCPay Server | server_url, store_id, api_key, webhook_secret | Greenfield `POST /api/v1/stores/{store}/invoices` (`metadata.orderId=trade_no`) → `checkoutLink` | `BTCPay-Sig` HMAC-SHA256; `InvoiceSettled` (and `InvoicePaymentSettled` when `afterExpiration=false`); respond 200 |

All HTTP calls use `net/http` with 15s timeouts; Stripe uses `stripe-go`. Amounts: CNY/USD etc. converted from cents to decimal strings with 2 places.
Each gateway has unit tests with recorded request/response fixtures (signature verification tests with known vectors).
