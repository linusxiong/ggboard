# Payments (pluggable gateways)

Package `internal/payment`. Order money flow lives in `internal/service/order`; gateways only create a payment and verify callbacks.

## Interface
```go
package payment

type Field struct {
    Key, Label, Description string
    Type     string   // text | password | number | select | textarea
    Options  []string // suggestions for select (free text is still accepted when Options is used as hints)
    Default  string
    Required bool
}

type PayRequest struct {
    TradeNo     string
    TotalAmount int64     // cents actually charged = order.total_amount + handling_amount
    Currency    string    // settings.currency, e.g. CNY/USD
    Subject     string    // "<app_name> - <plan name>" (EPay name, Alipay subject, Stripe product name, BTCPay itemDesc, Coinbase description)
    ExpiresAt   time.Time // order.created_at + order_timeout_minutes; gateways that support expiry pass it through
    UserID      uint64
    UserEmail   string
    NotifyURL   string // https://host/api/v1/guest/payment/notify/<gateway>/<uuid>
    ReturnURL   string // https://host/api/v1/guest/payment/return/<trade_no> (302 → SPA; gateway query params never reach the SPA)
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
    // Notify verifies the callback.
    //   (res, resp, nil)  verified payment: caller marks the order paid (idempotent) and then writes resp.
    //   (nil, resp, nil)  verified but not a payment (expired/pending/partial events): caller writes resp and does nothing else.
    //   (nil, nil, err)   verification failed: caller writes 400 text/plain "fail" and logs at WARN.
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
3. Gateway calls `ANY /guest/payment/notify/:gateway/:uuid` → look up payment by uuid, check enabled and gateway match
   (also accept Xboard class names as `:gateway` aliases: `EPay`, `EpusdtPay`, `AlipayF2F`, `StripeALLInOne`, `Coinbase`, `BTCPay`) → `Notify` →
   caller checks: the order's `payment_id` equals the resolved payment; when `NotifyResult.Amount != 0` it equals the expected charge in cents
   (else do NOT mark paid, audit-log ERROR); an already-paid order with a different `CallbackNo` is logged WARN (possible double payment) but still answered success;
   a verified payment for an order already cancelled by `order_timeout` is NOT dropped: store `callback_no`, log ERROR, email admin →
   `orderService.Paid(tradeNo, callbackNo)` (transaction, idempotent) → `Fulfil`.
4. `Fulfil`: set plan/group/transfer/expiry per period (monthly=+1 month … onetime=no expiry; reset_traffic=u=d=0), speed/device limits from plan,
   compute `next_reset_at`, handle surplus orders (status 4), refund_amount → balance, commission (status 1 pending settle),
   `order.status=3`, send email "order paid".
5. `order_timeout` job cancels stale pending orders; `cancel` returns balance_amount to balance.

## Built-in gateways
| name | label | config fields | pay | notify |
|---|---|---|---|---|
| `epay` | 易支付 (EPay) / GM Pay v2 compatible | url, pid, key, type(free text; suggestions alipay/wxpay/qqpay; CNY-only warning) | GET `<url>/submit.php?...&sign=md5(ksorted&key)&sign_type=MD5` → `url` | verify md5 over params (excluding sign/sign_type); `trade_status=TRADE_SUCCESS`; respond `success` |
| `epusdt` | Epusdt / BEpusdt (legacy protocol) | api_url, api_token, currency_type(free text, default usdt.trc20) | POST `/api/v1/order/create-transaction` JSON `{amount, order_id, redirect_url, notify_url, trade_type}` + `signature` (md5 of ksorted `k=v&` + token) → `payment_url` | POST JSON `{trade_id, order_id, amount, actual_amount, token, block_transaction_id, signature, status}`; verify signature, status==2; respond `ok` |
| `alipay_f2f` | 支付宝当面付 | app_id, private_key, alipay_public_key, (optional) sandbox | `alipay.trade.precreate` (RSA2) → `qrcode` | form POST; verify RSA2 sign; `trade_status` in (TRADE_SUCCESS, TRADE_FINISHED); respond `success` |
| `stripe` | Stripe Checkout | secret_key, webhook_secret, currency, exchange_rate(number, needed when currency ≠ site currency), payment_methods(free text list; maps to `AllowedPaymentMethodTypes`) | `stripe-go/v87` Checkout Session (`mode=payment`, `client_reference_id=trade_no`, `expires_at`) → `url` | webhook `checkout.session.completed` + `checkout.session.async_payment_succeeded`, `Stripe-Signature` verify with `WithIgnoreAPIVersionMismatch()`; respond 200 JSON |
| `coinbase` | Coinbase Business Checkouts (USDC; Commerce closed 2026-03-31) | api_key_name, api_key_secret, webhook_secret, exchange_rate | Checkouts API (JWT auth) → hosted checkout `url` | webhook HMAC verify per docs/PAYMENTS_GATEWAYS.md §6; respond 200 |
| `btcpay` | BTCPay Server | server_url, store_id, api_key, webhook_secret | Greenfield `POST /api/v1/stores/{store}/invoices` (`metadata.orderId=trade_no`, `expirationMinutes`) → `checkoutLink` | `BTCPay-Sig` HMAC-SHA256; only `InvoiceSettled` means paid (other events: GET invoice, require `status=="Settled"`); respond 200 |

**The exact wire protocols, signing algorithms and test vectors are in `docs/PAYMENTS_GATEWAYS.md` — implement from there.**
All HTTP calls use `net/http` with 15s timeouts; Stripe uses `stripe-go/v87`. Alipay F2F is CNY-only (reject other site currencies). Amounts: CNY/USD etc. converted from cents to decimal strings with 2 places.
Each gateway has unit tests with recorded request/response fixtures (signature verification tests with known vectors).
