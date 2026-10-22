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
`payment.Register(gw Gateway)` in `init()`; `payment.Get(name)`; `payment.List()`. The built-ins live in
`internal/payment/gateways/<name>` and are linked by `internal/payment/gateways/all.go` (blank-imported by
`cmd/ggboard/plugins_payment.go`); shared helpers are in `internal/payment/gateways/gwutil`.

Optional extensions (duck-typed, no change to the interface):
- `ValidateConfig(cfg map[string]string, siteCurrency string) error` — called by `POST/PATCH /admin/payments` after the
  required `Fields()` are checked (key parsing, CNY-only Alipay, Stripe currency/rate, Coinbase rate for non-USD sites).
- Errors implementing `UserMessage() string` (e.g. `gwutil.UserError`, Stripe `amount_too_small`) are shown to the user
  as the 402 message; every other `Pay` error becomes 402 "payment gateway error" (cause logged).
- The payment uuid is the last path segment of `PayRequest.NotifyURL` and of the notify request path
  (`gwutil.PaymentUUID`): Stripe and Coinbase put it in the checkout metadata and ack (without paying) events whose
  `payment_uuid` differs, so one provider account can serve several sites/payments.

## Order lifecycle (`internal/service/order`, package `ordersvc`)
1. `POST /user/orders` → order pending (status 0), all inside one transaction with the user row locked:
   - one open order per user (status 0/1) → 409 `conflict` with `errors.trade_no = [<pending trade_no>]`;
   - shop rules: the plan must have `prices[period]` (400 `period`); non-holders need `show && sell` (hidden plans 404) and
     free capacity (`capacity_limit` vs active holders, buyer excluded → 409); holders need `renew`, and hidden/legacy
     (`sell=false`) plans only while the subscription is active; `reset_traffic` only for an active holder;
   - type: 4 reset_traffic; 3 upgrade (another plan while active; needs `plan_change_enable`, else 403); 2 renew (same
     plan, active or one-time); 1 new;
   - pricing (`ComputeQuote`): user `discount` % and coupon (fixed cents or %) both on the plan price, summed and capped
     at the price → `discount_amount`; upgrade credit `surplus_amount` when `surplus_enable` (`ComputeSurplus`: one-time
     plans by unused traffic share of the last one-time order; periodic plans by remaining time share of the current
     plan's orders since its last new/upgrade order, capped at the user's `expired_at`), surplus above the price →
     `refund_amount` (credited on fulfilment); then the balance pays as much as possible → `balance_amount` (deducted
     now); the rest is `total_amount`. Percentages round half up.
   - coupons: `limit_use` is the remaining count (Xboard): one use is taken at creation and given back on cancel (Xboard never gives it back; changed so that
     timed-out orders do not burn uses);
     `limit_use_with_user` counts the user's paid orders with the coupon; dates, plans and periods are enforced.
   - `invite_user_id` = the user's inviter when `total_amount > 0`.
   - `trade_no` = `YYYYMMDDHHMMSS` (server time) + 6 random digits. `total_amount == 0` → paid and fulfilled immediately
     with `callback_no` `free` (nothing to pay) or `balance`.
2. `POST /user/orders/:trade_no/checkout {payment_id}` → paid/free orders answer `{type:"paid", data:<trade_no>}`;
   otherwise the payment must be enabled and its gateway registered (400 `payment_id`), `handling_amount` =
   `round(total × percent / 100) + fixed`, `payment_id`/`handling_amount` are stored, and `gateway.Pay` gets
   `TotalAmount = total + handling`, `Subject = "<app_name> - <plan name>"`, `ExpiresAt = created_at + order_timeout_minutes`,
   `NotifyURL = <notify_domain | app_url | request origin>/api/v1/guest/payment/notify/<gateway>/<uuid>`,
   `ReturnURL = <app_url | request origin>/api/v1/guest/payment/return/<trade_no>` (302 → `/orders/<trade_no>`).
   Response `{type, data}`; the SPA redirects (url), renders a QR (qrcode) and polls `status`.
3. Gateway calls `ANY /guest/payment/notify/:gateway/:uuid` → look up payment by uuid, check enabled and gateway match
   (also accept Xboard class names as `:gateway` aliases, case-insensitive: `EPay`, `EpusdtPay`, `AlipayF2F`,
   `StripeALLInOne` (and the older `StripeAlipay`/`StripeWepay`/`StripeCredit`/`StripeCheckout`), `Coinbase`, `BTCPay`) →
   `Notify` → caller checks (`ordersvc.HandleNotify`):
   - unknown/disabled payment, gateway mismatch, verification failure → 400 `fail` (WARN);
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
4. `Fulfil` (same transaction): onetime → plan/group/traffic, `u=d=0`, `expired_at=NULL`; reset_traffic → `u=d=0`;
   periods → plan/group/traffic and `expired_at` = max(now, expired_at) + 1/3/6/12/24/36 calendar months (server time
   zone; upgrades start from now); usage resets for new purchases, upgrades and one-time → periodic switches, renewals keep
   it; speed/device limits from the plan; `next_reset_at` (`usersvc.ComputeNextResetAt`); surplus orders → status 4;
   `refund_amount` → balance; commission computed (`commission_status = 0`). After commit: `auth.InvalidateUser`,
   `usersvc.InvalidateNodeUsers()`, email `order_paid`.
5. `order_timeout` job (every minute) cancels pending orders older than `order_timeout_minutes`, except orders checked
   out (payment_id set) within the last `ordersvc.CheckoutGrace` (31 min: Stripe sessions live ≥ 30 min, so a payment
   page opened just before the deadline would otherwise turn into "paid after cancel"); checkout refuses (409) orders
   past `created_at + order_timeout_minutes`. Cancel (user, admin or job) returns `balance_amount` to the balance and
   the coupon use. Admin `POST /admin/orders/:id/paid` fulfils a pending
   order with `callback_no = "manual"`.

## Commissions
- Computed when an order is paid, on `total_amount` (gateway money; balance and handling fees excluded) when the order
  has `invite_user_id`. Eligibility by the inviter's `commission_type`: 0 system → `commission_first_time_enable` decides
  between "the invitee's first paid order only" and "every order"; 1 period → every order; 2 one-time → first only.
  Rate = inviter `commission_rate`, or `invite_commission` when NULL or 0 (Xboard treats 0 as unset).
- `commission_settle` job (hourly, `ordersvc.SettleCommissions`; tests use `a.Scheduler.RunNow`): with
  `commission_auto_check_enable`, completed orders whose commission has been pending for 72 h (Xboard's refund window,
  `ordersvc.CommissionHold`) are approved (0 → 1); every approved order (1, automatic or admin `PATCH
  /admin/orders/:id {commission_status:1}`) is paid out: with `commission_distribution_enable` the commission is split
  l1/l2/l3 % up the invite chain, otherwise the direct inviter gets 100 %; shares go to `commission_balance` (or
  `balance` when `withdraw_close_enable`), one `commission_logs` row each, order → 2. Admins may set 0, 1 or 3
  (invalid); 2 is only set by settlement and a settled commission cannot change (409).

## Built-in gateways
| name | label | config fields | pay | notify |
|---|---|---|---|---|
| `epay` | 易支付 (EPay) / GM Pay v2 compatible | url, pid, key, type(free text; suggestions alipay/wxpay/qqpay; CNY-only warning) | GET `<url>/submit.php?...&sign=md5(ksorted&key)&sign_type=MD5` → `url` | verify md5 over params (excluding sign/sign_type); `trade_status=TRADE_SUCCESS`; respond `success` |
| `epusdt` | Epusdt / BEpusdt (legacy protocol) | api_url, api_token, currency_type(free text, default usdt.trc20) | POST `/api/v1/order/create-transaction` JSON `{amount, order_id, redirect_url, notify_url, trade_type}` + `signature` (md5 of ksorted `k=v&` + token) → `payment_url` | POST JSON `{trade_id, order_id, amount, actual_amount, token, block_transaction_id, signature, status}`; verify signature, status==2; respond `ok` |
| `alipay_f2f` | 支付宝当面付 | app_id, private_key, alipay_public_key, (optional) sandbox | `alipay.trade.precreate` (RSA2) → `qrcode` | form POST; verify RSA2 sign; `trade_status` in (TRADE_SUCCESS, TRADE_FINISHED); respond `success` |
| `stripe` | Stripe Checkout | secret_key, webhook_secret, currency, exchange_rate(number, needed when currency ≠ site currency), payment_methods(select `auto`/`card`/`alipay`/`wechat_pay`; non-auto maps to `AllowedPaymentMethodTypes`) | `stripe-go/v87` Checkout Session (`mode=payment`, `client_reference_id=trade_no`, `expires_at`) → `url` | webhook `checkout.session.completed` + `checkout.session.async_payment_succeeded`, `Stripe-Signature` verify with `WithIgnoreAPIVersionMismatch()`; respond 200 JSON |
| `coinbase` | Coinbase Business Checkouts (USDC; Commerce closed 2026-03-31) | api_key_name, api_key_secret, webhook_secret, exchange_rate | Checkouts API (JWT auth) → hosted checkout `url` | webhook HMAC verify per docs/PAYMENTS_GATEWAYS.md §6; respond 200 |
| `btcpay` | BTCPay Server | server_url, store_id, api_key, webhook_secret | Greenfield `POST /api/v1/stores/{store}/invoices` (`metadata.orderId=trade_no`, `expirationMinutes`) → `checkoutLink` | `BTCPay-Sig` HMAC-SHA256; only `InvoiceSettled` means paid (other events: GET invoice, require `status=="Settled"`); respond 200 |

**The exact wire protocols, signing algorithms and test vectors are in `docs/PAYMENTS_GATEWAYS.md` — implement from there.**
All HTTP calls use `net/http` with 15s timeouts; Stripe uses `stripe-go/v87`. Alipay F2F is CNY-only (reject other site currencies). Amounts: CNY/USD etc. converted from cents to decimal strings with 2 places.
Each gateway has unit tests with recorded request/response fixtures (signature verification tests with known vectors).
