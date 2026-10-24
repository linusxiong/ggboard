# Payments (pluggable gateways)

Package `internal/payment`. Order money flow lives in `internal/service/order` (package `ordersvc`); gateways only
create a payment and verify callbacks. Wire protocols, signatures and test vectors: [`PAYMENTS_GATEWAYS.md`](PAYMENTS_GATEWAYS.md).

## Interface

```go
package payment // internal/payment/gateway.go

type Field struct {
    Key         string   `json:"key"`
    Label       string   `json:"label"`
    Description string   `json:"description,omitempty"`
    Type        string   `json:"type"`              // text | password | number | select | textarea
    Options     []string `json:"options,omitempty"` // suggestions (free text is still accepted)
    Default     string   `json:"default,omitempty"`
    Required    bool     `json:"required"`
}

type PayRequest struct {
    TradeNo     string
    TotalAmount int64     // cents actually charged = order.total_amount + handling_amount
    Currency    string    // settings.currency, e.g. CNY/USD
    Subject     string    // "<app_name> - <plan name>"
    ExpiresAt   time.Time // order.created_at + order_timeout_minutes
    UserID      uint64
    UserEmail   string
    NotifyURL   string // <base>/api/v1/guest/payment/notify/<gateway>/<uuid>
    ReturnURL   string // <base>/api/v1/guest/payment/return/<trade_no> (302 → /orders/<trade_no>)
    ClientIP    string
}

type PayResult struct {
    Type string `json:"type"` // "url" (redirect) | "qrcode" (render a QR of Data) | "html" (auto-submitting form)
    Data string `json:"data"`
}

type NotifyResult struct {
    TradeNo    string // our order no
    CallbackNo string // gateway transaction id
    Amount     int64  // cents if the gateway reports it, else 0
}

type NotifyResponse struct {
    Status      int
    Body        string
    ContentType string
}

type Gateway interface {
    Name() string    // registry id, e.g. "epay"
    Label() string   // human name
    Fields() []Field // admin form schema
    Pay(ctx context.Context, cfg map[string]string, req PayRequest) (*PayResult, error)
    // Notify verifies the callback:
    //   (res, resp, nil)  verified payment: the caller marks the order paid (idempotent) and then writes resp.
    //   (nil, resp, nil)  verified but not a payment (expired/pending/partial events): the caller only writes resp.
    //   (nil, nil, err)   verification failed: the caller writes 400 text/plain "fail" and logs at WARN.
    Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*NotifyResult, *NotifyResponse, error)
}
```

`payment.Register(gw)` from `init()` (panics on duplicates), `payment.Get(name)`, `payment.List()` / `payment.Names()`
(sorted by name), `payment.FailResponse()` (the 400 `fail` answer). The built-ins live in
`internal/payment/gateways/<name>` and are linked by `internal/payment/gateways/all.go`, which
`cmd/ggboard/plugins_payment.go` blank-imports; shared helpers (HTTP client, signing, amounts) are in
`internal/payment/gateways/gwutil`. `payment.FormatAmount` / `ParseAmount` convert cents ⇄ `"12.34"`.

Optional extensions (duck-typed, no change to the interface):

- `ValidateConfig(cfg map[string]string, siteCurrency string) error` — called by `POST`/`PATCH /admin/payments` after the
  required `Fields()` are checked; its error becomes a 400 on `config` (base-URL checks, key parsing, CNY-only Alipay,
  Stripe currency/rate/method, Coinbase rate for non-USD sites).
- Errors implementing `UserMessage() string` (e.g. `gwutil.UserError`, Stripe `amount_too_small`) are shown to the user
  as the 402 message; every other `Pay` error becomes 402 "payment gateway error" (cause logged at WARN).
- The payment uuid is the last path segment of `PayRequest.NotifyURL` and of the notify request path
  (`gwutil.PaymentUUID`): Stripe and Coinbase put it in the checkout metadata and ack (without paying) events whose
  `payment_uuid` differs, so one provider account can serve several sites/payments.

Admin API: `GET /admin/payments/gateways` returns `{name, label, fields}` per registered gateway (the admin form is
generated from it and pre-fills `Field.Default`); every admin payment response carries `gateway_label` and the
`notify_url` to register with the provider.

## Order lifecycle (`internal/service/order`)

1. `POST /user/orders {plan_id, period, coupon_code?}` → **201** `{trade_no, status}`, all inside one transaction with
   the user row locked (`POST /user/orders`, `/checkout` and `/coupons/check` share a per-user rate limit → 429):
   - one open order per user (status 0/1) → 409 `conflict` with `errors.trade_no = [<pending trade_no>]`;
   - shop rules: the period must be known and the plan must have `prices[period]` (400 `period`); a hidden plan
     (`show=false`) is 404 unless the buyer holds it; non-holders need `show && sell` (400 `plan_id`) and free
     capacity (`capacity_limit` vs active holders, buyer excluded → 409 "sold out"); holders need `renew`, and
     hidden/legacy (`sell=false`) plans only while the subscription is active; `reset_traffic` only for an active
     holder;
   - type: 4 reset_traffic; 3 upgrade (another plan while active; needs `plan_change_enable`, else 403); 2 renew (same
     plan, active or one-time); 1 new;
   - pricing (`ComputeQuote`): user `discount` % and coupon (fixed cents or %) both on the plan price, summed and capped
     at the price → `discount_amount`; upgrade credit `surplus_amount` when `surplus_enable` (`ComputeSurplus`: one-time
     plans by unused traffic share of the last one-time order; periodic plans by remaining time share of the current
     plan's orders since its last new/upgrade order, capped at the user's `expired_at`), surplus above the price →
     `refund_amount` (credited on fulfilment); then the balance pays as much as possible → `balance_amount` (deducted
     now); the rest is `total_amount`. Percentages round half up.
   - coupons: `limit_use` is the *remaining* count (Xboard): one use is taken at creation and given back on cancel
     (Xboard never gives it back; changed so that timed-out orders do not burn uses); `limit_use_with_user` counts the
     user's paid orders with the coupon; dates, plans and periods are enforced.
   - `invite_user_id` = the user's inviter when `total_amount > 0`.
   - `trade_no` = `YYYYMMDDHHMMSS` (server time) + 6 random digits. `total_amount == 0` → paid and fulfilled immediately
     with `callback_no` `free` (nothing to pay) or `balance`.
2. `POST /user/orders/:trade_no/checkout {payment_id}` → paid/free orders answer `{type:"paid", data:<trade_no>}`, a
   cancelled order 409; otherwise `payment_id` is required and the payment must be enabled with a registered gateway
   (400 `payment_id`). `handling_amount = round(total × percent / 100 + fixed)` and `payment_id` are stored
   (409 if the order stopped being pending), and `gateway.Pay` (30 s timeout) gets `TotalAmount = total + handling`,
   `Subject = "<app_name> - <plan name>"`, `ExpiresAt = created_at + order_timeout_minutes`,
   `NotifyURL = <notify_domain | app_url | request origin>/api/v1/guest/payment/notify/<gateway>/<uuid>`,
   `ReturnURL = <app_url | request origin>/api/v1/guest/payment/return/<trade_no>`. Set `app_url`: the request-origin
   fallback uses the `Host` header. Response `{type, data}`; the SPA redirects (`url`), renders a QR (`qrcode`) or the
   form (`html`), and polls `GET /user/orders/:trade_no/status`.
3. The gateway calls `ANY /guest/payment/notify/:gateway/:uuid` → look up the payment by uuid, check the gateway match
   (Xboard class names are accepted as `:gateway` aliases, case-insensitive: `EPay`, `EpusdtPay`, `AlipayF2F`,
   `StripeALLInOne` (and the older `StripeAlipay`/`StripeWepay`/`StripeCredit`/`StripeCheckout`), `Coinbase`, `BTCPay`),
   that it is enabled and its gateway registered → `Notify` → `ordersvc.HandleNotify`:
   - unknown/disabled payment, gateway mismatch, unregistered gateway, verification failure → 400 `fail` (WARN/ERROR);
   - unknown order → acknowledged, ERROR + admin alert;
   - the order's `payment_id` must equal the resolved payment, and when `NotifyResult.Amount != 0` it must equal
     `total_amount + handling_amount`; otherwise the order is NOT paid, ERROR + admin alert, acknowledged;
   - an already-paid order with a different `CallbackNo` is logged WARN (possible double payment) and acknowledged;
   - a verified payment for a cancelled order is NOT dropped: `callback_no` stored, ERROR, admin alerted (once per
     gateway transaction: replays of the same notification are acknowledged without a new alert);
   - gateways that always report the amount (EPay, Epusdt, Alipay) treat a zero or negative signed amount as a
     verification failure (`gwutil.ParsePaidCents`), since `Amount == 0` means "not reported" here;
   - otherwise `Paid(tradeNo, callbackNo)` (conditional `status 0 → 3` update + fulfilment in one transaction, idempotent);
     database errors answer 500 so the gateway retries. The gateway's `NotifyResponse` is written verbatim.
   Admin alerts use the `notify` mail template and go to the first administrator (lowest id).
   `GET /guest/payment/return/:trade_no` drops the gateway's query string and 302s to `<app_url>/orders/<trade_no>`.
4. `fulfil` (same transaction): onetime → plan/group/traffic, `u=d=0`, `expired_at=NULL`; reset_traffic → `u=d=0`;
   periods → plan/group/traffic and `expired_at` = max(now, expired_at) + 1/3/6/12/24/36 calendar months (server time
   zone; upgrades start from now); usage resets for new purchases, upgrades and one-time → periodic switches, renewals
   keep it; speed/device/connection limits (`speed_limit`, `device_limit`, `conn_limit`) from the plan; `next_reset_at` (`usersvc.ComputeNextResetAt`); surplus orders → status 4;
   `refund_amount` → balance; commission computed (`commission_status = 0`). After commit: `auth.InvalidateUser`,
   `usersvc.InvalidateNodeUsers()`, email `order_paid`.
5. `order_timeout` job (every minute) cancels pending orders older than `order_timeout_minutes`, except orders checked
   out (payment_id set) within the last `ordersvc.CheckoutGrace` (31 min: Stripe sessions live ≥ 30 min, so a payment
   page opened just before the deadline would otherwise turn into "paid after cancel"); checkout refuses (409) orders
   past `created_at + order_timeout_minutes`. Cancel (user, admin or job) returns `balance_amount` to the balance and
   the coupon use; only pending orders can be cancelled (409). Admin `POST /admin/orders/:id/paid` fulfils a pending
   order with `callback_no = "manual"`; `POST /admin/orders` assigns a pending order without discounts, coupon, surplus or balance.

## Commissions

- Computed when an order is paid, on `total_amount` (gateway money; balance and handling fees excluded) when the order
  has `invite_user_id`. Eligibility by the inviter's `commission_type`: 0 system → `commission_first_time_enable` decides
  between "the invitee's first paid order only" and "every order"; 1 period → every order; 2 one-time → first only.
  Rate = inviter `commission_rate`, or `invite_commission` when NULL or 0 (Xboard treats 0 as unset), capped at 100.
- `commission_settle` job (hourly, `ordersvc.SettleCommissions`; tests use `a.Scheduler.RunNow`): with
  `commission_auto_check_enable`, completed orders whose commission has been pending for 72 h (Xboard's refund window,
  `ordersvc.CommissionHold`) are approved (0 → 1); every approved order (1, automatic or admin
  `PATCH /admin/orders/:id {commission_status:1}`) is paid out: with `commission_distribution_enable` the commission is
  split l1/l2/l3 % up the invite chain, otherwise the direct inviter gets 100 %; shares go to `commission_balance` (or
  `balance` when `withdraw_close_enable`), one `commission_logs` row each, order → 2. Admins may set 0, 1 or 3
  (invalid); 2 is only set by settlement and a settled commission cannot change (409).

## Built-in gateways

| name | label | config fields | pay | notify |
| --- | --- | --- | --- | --- |
| `epay` | 易支付 (EPay); also GM Pay v2's EPay endpoint | url, pid, key, type (free text; suggestions alipay/wxpay/qqpay) | GET `<url>/submit.php?…&sign=md5(sorted k=v + key)&sign_type=MD5` → `url` | GET or form POST; verify MD5 (excluding sign/sign_type), `pid`; `trade_status=TRADE_SUCCESS`; respond `success` |
| `epusdt` | Epusdt / BEpusdt (legacy protocol) | api_url, api_token, currency_type (free text, default `usdt.trc20`) | POST `/api/v1/order/create-transaction` JSON `{amount, order_id, notify_url, redirect_url, trade_type, fiat?, signature}` (MD5 of sorted `k=v` + token) → `payment_url` | POST JSON; verify signature, `status==2`; respond `ok` |
| `alipay_f2f` | 支付宝当面付 (Alipay F2F) | app_id, private_key, alipay_public_key, sandbox (`0`/`1`) | `alipay.trade.precreate` (RSA2) → `qrcode` | form POST; verify RSA2 sign, `app_id`; `trade_status` in (TRADE_SUCCESS, TRADE_FINISHED); respond `success` |
| `stripe` | Stripe Checkout | secret_key, webhook_secret, currency, exchange_rate (needed when currency ≠ site currency), payment_methods (`auto`/`card`/`alipay`/`wechat_pay`) | `stripe-go/v87` Checkout Session (`mode=payment`, `client_reference_id=trade_no`, `expires_at`) → `url` | `checkout.session.completed` (paid) + `checkout.session.async_payment_succeeded`, `Stripe-Signature` verified with `WithIgnoreAPIVersionMismatch()`; respond `200 {"received":true}` |
| `coinbase` | Coinbase Business (USDC); Commerce closed 2026-03-31 | api_key_name, api_key_secret, webhook_secret, exchange_rate | Checkouts API `POST /api/v1/checkouts` (CDP ES256 JWT) → hosted `url` | `X-Webhook-Signature` HMAC-SHA256; `checkout.payment.success` + `COMPLETED`; respond `200 ok` |
| `btcpay` | BTCPay Server | server_url, store_id, api_key, webhook_secret | Greenfield `POST /api/v1/stores/{store}/invoices` (`metadata.orderId=trade_no`, `ggboardPayment`/`ggboardBinding`, `expirationMinutes`) → `checkoutLink` | `BTCPay-Sig` HMAC-SHA256; `InvoiceSettled`/`Processing`/`ReceivedPayment`/`PaymentSettled` → GET the invoice, paid only when `status=="Settled"` and its `ggboardBinding` (HMAC over payment uuid, trade_no, amount, currency) matches; respond `200 ok` |

All gateway HTTP calls use one shared `net/http` client (15 s timeout, TLS verification on); Stripe goes through
`stripe-go/v87`. Alipay F2F is CNY-only (config rejected for other site currencies). EPay/Epusdt accept `http://` base
URLs with a WARN; BTCPay requires `https://`. Each gateway has unit tests with `httptest` fixtures and the signature
vectors of `PAYMENTS_GATEWAYS.md`.

## Display icons

A payment's `icon` is display-only and gateway-independent: `""`, `preset:<id>` (16 original marks in
`web/src/components/icons/payments`, e.g. `preset:alipay`, `preset:wechat`, `preset:usdt`), an image URL or one emoji.
The admin console preselects the gateway's default when a payment is created (EPay by its `type`, Epusdt by
`currency_type`, Stripe by `payment_methods`). The accepted values and the default mapping are in
[API.md › Payment icons](API.md#payment-icons); a new gateway needs no icon code (unknown presets render a neutral card glyph).
