# Payment gateways: wire protocols

This is the implementation reference for the six built-in gateways in `internal/payment/gateways/<name>/`. It
builds on the interface, order lifecycle and gateway table in [`PAYMENTS.md`](PAYMENTS.md). Everything here was
checked on **2026-10-21** against upstream source code or official docs; sources are listed per section. All
MD5/HMAC/SHA-256 vectors were computed with `python3 -I` (stdlib `hashlib`/`hmac`). Three of them reproduce the
upstream projects' own published examples, which confirms the algorithms.

**Common test inputs** (used by every vector unless stated otherwise):

- trade_no `202610211234567a8b9c0d1e2f3a4b5c` (32 chars); amount `1234` cents → `12.34`.
- payment uuid `0123456789abcdef0123456789abcdef`.
- NotifyURL `https://panel.example.com/api/v1/guest/payment/notify/<gateway>/0123456789abcdef0123456789abcdef`.
- ReturnURL `https://panel.example.com/orders/202610211234567a8b9c0d1e2f3a4b5c` (a fixture value; production
  return URLs are `<base>/api/v1/guest/payment/return/<trade_no>`).
- Timestamp `1792584000` = 2026-10-21T12:00:00Z.

---

## 0. Design decisions (folded into `PAYMENTS.md`)

The original contract had gaps; these decisions are implemented and kept here as the rationale.

1. **Coinbase:** Commerce was merged into **Coinbase Business**; after **2026-03-31** the Commerce dashboard closed
   and charges can't be created. `coinbase` implements the **Coinbase Business Checkouts API** (§6, settles in USDC;
   US and Singapore merchants). The Xboard importer imports Xboard "Coinbase" payments **disabled**, because their
   keys no longer work.
2. **Stripe:** module **`github.com/stripe/stripe-go/v87`** (v87.0.0, API `2026-09-30.endive`, pulls in
   `github.com/shopspring/decimal`). v87 removed `PaymentMethodTypes`; `AllowedPaymentMethodTypes` filters the methods
   enabled in the Dashboard. No `publishable_key` (hosted Checkout doesn't need it).
   `checkout.session.async_payment_succeeded` is handled. A `currency` different from the site currency needs an
   explicit `exchange_rate` (Xboard fetched a public FX JSON at checkout time; we don't).
3. **BTCPay:** `InvoicePaymentSettled` fires for each settled *payment*, partial ones included, so no event alone
   means "paid": the invoice is fetched and must be `status == "Settled"` (§7).
4. **Epusdt:** `assimon/epusdt` now redirects to `GMWalletApp/epusdt` ("GM Pay"), whose **v2.0.0 (2026-07-23)**
   removed `POST /api/v1/order/create-transaction`. `epusdt` implements the legacy protocol, still served by
   epusdt ≤ v1 and the maintained fork **BEpusdt** (`v03413/BEpusdt` v1.24.3). GM Pay v2 users configure the
   **`epay`** gateway against GM Pay's EPay-compatible endpoint (§2.5). `currency_type` (wire name `trade_type`) is
   free text defaulting to `usdt.trc20`.
5. **EPay `type`** is free text with `alipay`/`wxpay`/`qqpay` as suggestions; clones and GM Pay use values such as
   `usdt.tron`.
6. **Verified callbacks that aren't payments** (Stripe `checkout.session.expired`, BTCPay `InvoiceCreated`, BEpusdt
   status 1/3, Alipay `WAIT_BUYER_PAY`): `Notify` returns `(nil, resp, nil)` and the caller only writes `resp`;
   verification failures return `(nil, nil, err)` → `400 text/plain fail` + WARN.
7. **`PayRequest`** carries `TotalAmount = total_amount + handling_amount` (what is charged), `Subject` (EPay `name`,
   Alipay `subject`, Stripe product name, BTCPay `itemDesc`, Coinbase `description`) and `ExpiresAt` (Stripe
   `expires_at`, Alipay `timeout_express`, BTCPay `expirationMinutes`, Coinbase `expiresAt`).
8. **Caller checks** (`ordersvc.HandleNotify`): the order's `payment_id` must equal the payment resolved from
   `:uuid`; a non-zero `NotifyResult.Amount` must equal the expected charge; an already-paid order with a different
   `CallbackNo` is a possible double payment (WARN); a verified payment for a cancelled order is stored and alerted,
   never dropped (Xboard drops it).
9. `Field` has `Default` and the `textarea` type. Alipay F2F is CNY-only (config rejected otherwise); EPay settles in
   CNY (field help says so).
10. **`ReturnURL`** is `/api/v1/guest/payment/return/<trade_no>`, which 302s to the SPA, because EPay appends its
    signed result query string to `return_url` and those parameters must never reach the SPA URL.

---

## 1. Common rules

**Notify URL.** The format is `<base>/api/v1/guest/payment/notify/<gateway>/<uuid>`.

- `<gateway>` is the registry name: `epay`, `epusdt`, `alipay_f2f`, `stripe`, `coinbase` or `btcpay`.
- `<uuid>` is `payments.uuid` (32 hex).
- `<base>` is `payments.notify_domain` (scheme+host, no trailing `/`), else `app_url`, else the request origin.
- The route is `ANY` because EPay notifies with GET.
- Stripe, Coinbase and BTCPay need the URL pasted into the provider's webhook settings and require HTTPS.
  Every admin payment response (`GET`/`POST`/`PATCH /admin/payments…`) carries it as `notify_url`.

**Reading callbacks.**

- Bodies are capped at 1 MiB (`gwutil.ReadBody`; form bodies go through `http.MaxBytesReader`) and read **once**.
  HMACs are verified over those exact bytes before parsing; JSON is never re-marshalled before verifying.
- Form gateways (EPay, Alipay) use `gwutil.Values`: GET uses `r.URL.Query()`; POST uses `r.ParseForm()` and then
  `r.PostForm`, falling back to the query when it's empty. Take the first value of each key. Signatures cover the
  **decoded** values.
- Header lookup is case-insensitive: Xboard's "IS Btcpay-Sig, API doc is WRONG" comment is a PHP
  `getallheaders` quirk.
- Signatures are compared with `gwutil.EqualHex` (`hmac.Equal` after lowercasing and trimming the hex).

**Amounts.**

- Site amounts are int64 **cents**: the major unit × 100, whatever the currency.
- Outgoing: `payment.FormatAmount` (1234 → `12.34`, 5 → `0.05`). Never use float64.
- Incoming decimal strings (`12.3`, `12.34`, `100.0000`, `10`): `gwutil.ParseCents` (exact decimal, rounded half-up
  to 2 places).
- `Pay` rejects `TotalAmount <= 0`; the caller fulfils zero-amount orders itself.
- FX (Stripe/Coinbase `exchange_rate`): `minor = roundHalfUp(cents × rate)` with `shopspring/decimal`
  (`gwutil.ConvertCents`); an empty rate means 1.

**HTTP client and TLS.**

- One shared `gwutil.DefaultClient` (`&http.Client{Timeout: 15 * time.Second}`) and `http.NewRequestWithContext`;
  the order service bounds each `Pay` call with a 30 s context.
- The default Transport keeps HTTP/2, `HTTPS_PROXY` support and **TLS verification on**. **Never** set
  `InsecureSkipVerify`; Xboard's Epusdt plugin disables peer verification, so don't port that.
- Trim spaces and the trailing `/` from base URLs. Require `https://`; for self-hosted EPay/Epusdt accept
  `http://` with a WARN.
- Every request sends `User-Agent: ggboard (+https://github.com/linusxiong/ggboard)`; response bodies are read
  through `io.LimitReader(…, 1<<20)`.
- On non-2xx, log the status and ≤ 512 B of the body; the SPA only sees "payment gateway error".

**Logging (slog).** Log gateway, payment id, trade_no, gateway txn id, event type, verification result, HTTP
status and latency. **Never** log config values (keys, tokens, private keys, webhook secrets), `Authorization`
headers or JWTs, EPay/Epusdt sign strings (they end with the key), or raw Alipay notify bodies (buyer ids).

**Idempotency.**

- Gateways retry a notify until they get their success answer: an exact body for EPay, Epusdt and Alipay; any
  2xx for Stripe, BTCPay and Coinbase.
- `ordersvc.Service.Paid` is idempotent, and an already-paid order still gets the success response.
- `CallbackNo` → `orders.callback_no`.

| gateway | `PayResult.Type` | `CallbackNo` | success response (also used to ack non-payment events) |
| --- | --- | --- | --- |
| epay | `url` | `trade_no` param | `200 text/plain success` |
| epusdt | `url` | `trade_id` | `200 text/plain ok` |
| alipay_f2f | `qrcode` | `trade_no` param | `200 text/plain success` |
| stripe | `url` | PaymentIntent id (fallback: session id) | `200 application/json {"received":true}` |
| coinbase | `url` | checkout `id` | `200 text/plain ok` |
| btcpay | `url` | `invoiceId` | `200 text/plain ok` |

---

## 2. EPay (易支付): `epay`

Sources: 彩虹易支付 (Caihong EPay) mirror `github.com/itxve/epay` (`submit.php`, `mapi.php`, `includes/lib/Payment.php`
`makeSign`/`verifySign`, `includes/functions.php` `creat_callback`/`do_notify`); Xboard `EPay.php`,
`plugins-core/Epay/Plugin.php`; GM Pay v2.0.0 `wiki/API.md`.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `url` | 网关地址 | text | ✓ | Base URL, e.g. `https://pay.example.com`. No `/submit.php`, no trailing `/`. |
| `pid` | 商户 ID | text | ✓ | Merchant id (numeric on Caihong). |
| `key` | 商户密钥 | password | ✓ | MD5 key. |
| `type` | 支付方式 | text | – | `alipay`/`wxpay`/`qqpay` or a provider value. Empty lets the provider's cashier ask. |

### 2.1 Signature (MD5)

1. Take every parameter except `sign` and `sign_type`, dropping empty values.
2. Sort the keys ascending (byte order).
3. Join the pairs as `k=v` with `&`, using **raw values** (not URL-encoded).
4. `sign = lowercase hex(md5(joined + key))`. No `&` goes before the key.

This is Caihong `makeSign`. Xboard's `stripslashes(urldecode(http_build_query()))` yields the same string for
normal values.

### 2.2 Pay

Return `PayResult{url}` = `<url>/submit.php?` + `url.Values.Encode()`. Encoding happens after signing. Caihong
also accepts the same fields as a POST form.

| param | value |
| --- | --- |
| `pid` | cfg.pid |
| `type` | cfg.type; omit when empty |
| `out_trade_no` | trade_no (`mapi.php` requires `^[A-Za-z0-9._\-\|]+$`) |
| `notify_url` / `return_url` | NotifyURL / ReturnURL |
| `name` | Subject (or trade_no) |
| `money` | `12.34`; must match `^[0-9.]+$` and be > 0 |
| `sign` / `sign_type` | §2.1 / `MD5` |

- Optional `sitename` / `param` must then be signed too; Caihong echoes `param` back in the notify.
- **Gotcha:** Caihong rejects a resubmit of an `out_trade_no` for **10 days** in two cases: when the order is
  already paid, and when `money`, `name`, `notify_url`, `return_url` or `param` changed ("支付参数有变化"). Keep
  them deterministic per order. Switching to a method with a different handling fee hits this error; the user
  must create a new order.

**Vector** (key `epay_test_key_123`, pid `1001`, type `alipay`):

```text
money=12.34&name=202610211234567a8b9c0d1e2f3a4b5c&notify_url=https://panel.example.com/api/v1/guest/payment/notify/epay/0123456789abcdef0123456789abcdef&out_trade_no=202610211234567a8b9c0d1e2f3a4b5c&pid=1001&return_url=https://panel.example.com/orders/202610211234567a8b9c0d1e2f3a4b5c&type=alipay
md5(<above> + "epay_test_key_123") = 6964057e967af1328f0ec76ebbcb50e5
```

Upstream check (GM Pay docs): the string
`money=100&name=VIP&notify_url=https://merchant.example/notify&out_trade_no=ORD202605230001&pid=1000&return_url=https://merchant.example/return&type=alipay`
plus `epusdt_secret_key` gives `b865b0acbb2b01554c35a1bd33351452` ✔.

### 2.3 Notify

Caihong (`creat_callback`) sends a **GET**:
`notify_url?pid=…&trade_no=…&out_trade_no=…&type=…&name=…&money=…&trade_status=TRADE_SUCCESS[&param=…]&sign=…&sign_type=MD5`.
Some clones POST a form instead, so accept both.

- `money` is a PHP float (`12.3`, `10`).
- `name` may be the literal `product`, depending on a site option.

Steps:

1. Recompute §2.1 over **all received** params, extras included, and compare in constant time.
2. Require `pid == cfg.pid`.
3. Require `trade_status == "TRADE_SUCCESS"`; anything else is a no-op ack.
4. Return `TradeNo=out_trade_no`, `CallbackNo=trade_no`, `Amount=cents(money)`.

Response: `200 success`. Caihong's `do_notify` accepts any body containing `success`/`SUCCESS`/`Success`;
otherwise it retries. The browser is also redirected to `return_url` with the same signed query; never treat that
redirect as payment.

**Vector**: params `pid=1001 trade_no=2026102112000012345 out_trade_no=<trade_no> type=alipay name=<trade_no>
money=12.34 trade_status=TRADE_SUCCESS sign_type=MD5 sign=…`:

```text
money=12.34&name=202610211234567a8b9c0d1e2f3a4b5c&out_trade_no=202610211234567a8b9c0d1e2f3a4b5c&pid=1001&trade_no=2026102112000012345&trade_status=TRADE_SUCCESS&type=alipay
md5(<above> + "epay_test_key_123") = 33c9590808c14643afc49d9b1d12ed8a
```

### 2.4 `mapi.php` (API mode; reference only, not implemented)

- Request: POST a form to `<url>/mapi.php` with the §2.2 fields plus `clientip` (required; `PayRequest.ClientIP`)
  and `device` (`pc` default, `mobile`, `qq`, `wechat`, `alipay`). `type` is required in this mode.
- Response JSON: `{"code":1,"msg":"…","trade_no":"…","payurl":"…"}`, where `payurl` may be replaced by `qrcode`
  (→ `qrcode`) or `urlscheme` (→ `url`).
- `code != 1` → error with `msg`.

### 2.5 GM Pay v2 through `epay`

Config:

- `url = https://<gmpay>/payments/epay/v1/order/create-transaction`; we append `/submit.php`.
- `pid` = the GM Pay API-key PID (numeric); `key` = its `secret_key`.
- `type` = a `token.network` selector such as `usdt.tron`; empty or `alipay` makes the GM Pay cashier ask.

Fiat defaults to GM Pay's `epay.default_currency` (`cny`). GM Pay notifies with a GET of EPay parameters
(`money=100.0000`) signed with MD5, and accepts `ok`/`success`.

---

## 3. Epusdt: `epusdt`

Sources: BEpusdt v1.24.3 (`docs/api/api.md`, `docs/notify/readme.md`, `docs/trade-type.md`, `app/utils/utils.go`
`EpusdtSign`, `app/handler/epusdt/epusdt.go`, `app/task/notify/notify.go`); GM Pay v2.0.0 (`wiki/API.md`,
`src/util/sign/sign.go`); Xboard `EpusdtPay.php`.

Server compatibility:

- Legacy epusdt (≤ v1): USDT-TRC20 only, `amount` always in CNY.
- BEpusdt: adds `trade_type` and `fiat`.
- GM Pay ≥ v2: incompatible; use §2.5.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `api_url` | API 地址 | text | ✓ | Base URL, e.g. `https://usdt.example.com`. |
| `api_token` | API Token | password | ✓ | BEpusdt: 系统管理 → 基本设置 → API 设置 → 对接令牌. |
| `currency_type` | 交易类型 | text (default `usdt.trc20`) | – | Sent as `trade_type`: `usdt.trc20`, `usdc.trc20`, `tron.trx`, `usdt.erc20`, `usdt.polygon`, `usdt.bep20`, `usdt.solana`, `usdt.ton`, `usdt.arbitrum`, `usdc.base`, … Legacy epusdt ignores it. |

### 3.1 Signature (MD5)

1. Take every parameter except `signature`, dropping `null` and `""`.
2. Sort the keys and join as `k=v` with `&`.
3. Append the token, with no `&`.
4. Take the lowercase hex md5.

Servers re-format numbers from float64. Legacy epusdt and GM Pay use `strconv.FormatFloat(f,'f',-1,64)`. BEpusdt
uses `fmt.Sprintf("%v")` (`%g`), which gives `1e+06` for 1 000 000. The two agree for 0.0001 ≤ x < 1e6 (checked
with Go 1.27).

### 3.2 Pay

`POST <api_url>/api/v1/order/create-transaction`, `Content-Type: application/json`:

```json
{"amount":12.34,"order_id":"<trade_no>","notify_url":"<NotifyURL>","redirect_url":"<ReturnURL>","trade_type":"usdt.trc20","signature":"…"}
```

- **`amount`** is a JSON number. Take the decimal, trim trailing zeros and then a trailing `.` (`12.30` → `12.3`,
  `100.00` → `100`), emit it via `json.Number`, and use the same text in the sign string.
  - Reject amounts ≥ 1 000 000: the BEpusdt formatting diverges there.
  - Never send `0`: BEpusdt treats it as "exclusive mode", which accepts *any* amount.
- **`fiat`**: sent only when the site currency isn't CNY, as the uppercased site currency (`USD`, `EUR`, …;
  BEpusdt only, legacy epusdt treats the amount as CNY regardless).
- **`redirect_url`** is required by BEpusdt.
- **`order_id`** must be ≤ 32 chars. BEpusdt *rebuilds* a pending order with the same `order_id` (new
  address/amount, timer reset), so re-checkout is safe.
- Extra parameters (`name`, `timeout` in seconds ≥ 120) are allowed but must be signed.

The response is **HTTP 200 even on errors**:

```json
{"status_code":200,"message":"success","data":{"trade_id":"…","order_id":"<trade_no>","amount":"12.34","actual_amount":"1.7","token":"T…(address)","expiration_time":1200,"payment_url":"https://usdt.example.com/pay/checkout-counter/…"},"request_id":""}
```

- `status_code != 200` → error with `message`.
- Field types (string vs number) vary between servers. Decode only `payment_url` and `trade_id`, as strings.
- → `PayResult{url, payment_url}`.

**Vector** (token `epusdt_test_token`):

```text
amount=12.34&notify_url=https://panel.example.com/api/v1/guest/payment/notify/epusdt/0123456789abcdef0123456789abcdef&order_id=202610211234567a8b9c0d1e2f3a4b5c&redirect_url=https://panel.example.com/orders/202610211234567a8b9c0d1e2f3a4b5c&trade_type=usdt.trc20
md5(<above> + "epusdt_test_token") = 965b836ba79aeace6e2863b8447de372
```

Upstream check (BEpusdt docs): the string
`amount=42&notify_url=http://example.com/notify&order_id=20220201030210321&redirect_url=http://example.com/redirect`
plus `epusdt_password_xasddawqe` gives `1cd4b52df5587cfb1968b0c0c6e156cd` ✔.

### 3.3 Notify

`POST application/json`:

```json
{"trade_id":"202610211200001234","order_id":"<trade_no>","amount":12.34,"actual_amount":"1.7","token":"TTestTronAddress0000000000000000001","block_transaction_id":"9f2c4e1a","signature":"…","status":2}
```

Here `token` is the receiving address, and `actual_amount` is a string on BEpusdt but a number on legacy servers.

Steps:

1. Decode with `json.NewDecoder(…).UseNumber()` into `map[string]any`; reject nested objects or arrays.
2. Canonicalise values: string as-is; `json.Number` as its literal text; bool as `true`/`false`; skip `null`
   and `""`.
3. Verify per §3.1 in constant time. On a mismatch, recompute **once** with numbers formatted as
   `fmt.Sprint(float64)` (BEpusdt `%v`), and accept either result.
4. `status`: `2` → paid. `1` (BEpusdt "waiting", sent every minute) and `3` (expired) → no-op ack.
5. Return `TradeNo=order_id`, `CallbackNo=trade_id`, `Amount=cents(amount)` (the fiat amount we sent). Log
   `block_transaction_id`.

Response: `200 ok`.

- Legacy epusdt needs the exact body `ok`; GM Pay accepts `ok`/`success`.
- BEpusdt v1.24.3 only checks for HTTP 200, and retries status 2 up to 10× with 2ⁿ-minute backoff.

**Vector** (the JSON above, token `epusdt_test_token`):

```text
actual_amount=1.7&amount=12.34&block_transaction_id=9f2c4e1a&order_id=202610211234567a8b9c0d1e2f3a4b5c&status=2&token=TTestTronAddress0000000000000000001&trade_id=202610211200001234
md5(<above> + "epusdt_test_token") = b1b1ae9d8ce1af4ad5a94812d665299f
```

---

## 4. Alipay Face-to-Face (支付宝当面付): `alipay_f2f`

Sources: `alipay/alipay-sdk-java-all` master (`AlipaySignature.java` `getSignContent`/`getSignCheckContentV1`,
`AlipayTradePrecreateModel.java`, `AlipayTradePrecreateResponse.java`); Xboard `AlipayF2f/library/AlipayF2F.php`.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `app_id` | APPID | text | ✓ | Open Platform app id; the app must have 当面付 signed. |
| `private_key` | 应用私钥 | textarea | ✓ | RSA ≥ 2048 app private key: PKCS#1 or PKCS#8, PEM or bare base64. |
| `alipay_public_key` | 支付宝公钥 | textarea | ✓ | The **Alipay** public key (not 应用公钥): PEM or bare base64 X.509 SPKI. |
| `sandbox` | 沙箱 | select `0`/`1` | – | `1` → sandbox gateway. |

- Gateways:
  - production: `https://openapi.alipay.com/gateway.do`;
  - sandbox: `https://openapi-sandbox.dl.alipaydev.com/gateway.do` (older guides show `openapi.alipaydev.com`).
- Only **公钥模式** (public-key mode) is supported. 公钥证书模式 (`app_cert_sn`/`alipay_root_cert_sn`) is not;
  state that in the field help.
- The OpenAPI v3 REST style (`/v3/alipay/trade/precreate`) is not used.
- CNY only.

**Key parsing (Go).**

1. Strip `\r` and whitespace at both ends.
2. If the text contains `-----BEGIN`, `pem.Decode` it and pick the parser by block type:
   - `RSA PRIVATE KEY` → `x509.ParsePKCS1PrivateKey`;
   - `PRIVATE KEY` → `x509.ParsePKCS8PrivateKey` (the result must be `*rsa.PrivateKey`);
   - `PUBLIC KEY` → `x509.ParsePKIXPublicKey`;
   - `RSA PUBLIC KEY` → `x509.ParsePKCS1PublicKey`.
3. Otherwise remove all whitespace, base64-decode (`StdEncoding`) and try PKCS#8 then PKCS#1 for the private key,
   PKIX then PKCS#1 for the public key.

Alipay's key tool emits bare base64: PKCS#1 private keys start `MIIEo`/`MIIEp`, PKCS#8 keys start `MIIEv`, and
public keys start `MIIBIjAN`. Parse at config save so the admin sees errors, and reject keys under 2048 bits.

### 4.1 Pay (`alipay.trade.precreate`)

`POST <gateway>`, `Content-Type: application/x-www-form-urlencoded;charset=utf-8`, with these fields:

| field | value |
| --- | --- |
| `app_id` | cfg.app_id |
| `method` | `alipay.trade.precreate` |
| `format` / `charset` / `sign_type` / `version` | `JSON` / `utf-8` / `RSA2` / `1.0` |
| `timestamp` | Now in **UTC+8**, `2006-01-02 15:04:05`. Use `time.FixedZone("CST", 8*3600)`: a server in UTC would cause skew errors. |
| `notify_url` | NotifyURL |
| `biz_content` | JSON string `{"out_trade_no":"<≤64>","total_amount":"12.34","subject":"<≤256>","timeout_express":"30m"}`. `timeout_express` is derived from `ExpiresAt` (relative minutes, clamped to `1m`–`15d`). |
| `sign` | see below |

**Sign.**

1. Take all params except `sign`. This **includes `sign_type`**.
2. Drop empty values and sort the keys.
3. Join as `k=v` with `&`, using raw values; `biz_content` goes in verbatim.
4. `rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sha256(str))`, then base64 `StdEncoding`.

Marshal `biz_content` once and use the identical bytes for signing and for the form.

Response:

```json
{"alipay_trade_precreate_response":{"code":"10000","msg":"Success","out_trade_no":"<trade_no>","qr_code":"https://qr.alipay.com/bax0…"},"sign":"…"}
```

- `code != "10000"` → error with `sub_code`/`sub_msg` (e.g. `ACQ.TRADE_HAS_SUCCESS`, `ACQ.TRADE_HAS_CLOSE`).
  Check `code`, not `msg` as Xboard does.
- → `PayResult{qrcode, qr_code}`.
- When the response carries `sign`, it is verified over the raw `alipay_trade_precreate_response` bytes with the
  Alipay public key (`json.RawMessage`); an empty `qr_code` is an error.

**Vector.** RSA PKCS#1 v1.5 is deterministic for a given key. The test generates a key with `rsa.GenerateKey`,
asserts this exact string and its digest, then round-trips sign/verify:

```text
app_id=2021000000000000&biz_content={"out_trade_no":"202610211234567a8b9c0d1e2f3a4b5c","total_amount":"12.34","subject":"GG Bond 202610211234567a8b9c0d1e2f3a4b5c"}&charset=utf-8&format=JSON&method=alipay.trade.precreate&notify_url=https://panel.example.com/api/v1/guest/payment/notify/alipay_f2f/0123456789abcdef0123456789abcdef&sign_type=RSA2&timestamp=2026-10-21 20:00:00&version=1.0
sha256 = 2ac5f7bd2bb0fc82971f6507637b67dc494441d3e15d74b50e767469342ac3ab
```

### 4.2 Notify

`POST application/x-www-form-urlencoded; charset=utf-8`. Fields include:

- `notify_time`, `notify_type=trade_status_sync`, `notify_id`, `app_id`, `auth_app_id`, `charset`, `version`,
  `sign_type=RSA2`, `sign`;
- `trade_no` (Alipay txn), `out_trade_no`, `buyer_id`, `seller_id`, `trade_status`;
- `total_amount`, `receipt_amount`, `invoice_amount`, `buyer_pay_amount`, `point_amount`;
- `gmt_create`, `gmt_payment`, `subject`, `fund_bill_list` (a JSON string).

Verification follows SDK `rsaCheckV1`:

1. `r.ParseForm()` and build a first-value map from `r.PostForm`. A `+` in `sign` arrives as `%2B` and decodes
   correctly.
2. Remove `sign` **and `sign_type`**. Drop empty values, sort, and join as `k=v` with `&`.
3. Base64-decode `sign`, then `rsa.VerifyPKCS1v15(pub, crypto.SHA256, sha256(str), sig)`.
4. Require `app_id == cfg.app_id`.
5. `trade_status`: `TRADE_SUCCESS`/`TRADE_FINISHED` → paid. `TRADE_FINISHED` can come much later than
   `TRADE_SUCCESS`, so rely on idempotency. `WAIT_BUYER_PAY`/`TRADE_CLOSED` → no-op ack.
6. Return `TradeNo=out_trade_no`, `CallbackNo=trade_no`, `Amount=cents(total_amount)`.

Response: `200 text/plain`, exactly `success` (7 chars). Anything else makes Alipay retry about 8 times within
25 h, at intervals of 4m, 10m, 10m, 1h, 2h, 6h and 15h.

**Vector.** `sign=XXX` and `sign_type=RSA2` were removed; the empty `seller_email=` was dropped:

```text
app_id=2021000000000000&auth_app_id=2021000000000000&buyer_id=2088000000000000&buyer_pay_amount=12.34&charset=utf-8&fund_bill_list=[{"amount":"12.34","fundChannel":"ALIPAYACCOUNT"}]&gmt_create=2026-10-21 20:00:05&gmt_payment=2026-10-21 20:00:10&invoice_amount=12.34&notify_id=2026102100222200012345678901234567&notify_time=2026-10-21 20:00:11&notify_type=trade_status_sync&out_trade_no=202610211234567a8b9c0d1e2f3a4b5c&point_amount=0.00&receipt_amount=12.34&seller_id=2088000000000001&subject=GG Bond 202610211234567a8b9c0d1e2f3a4b5c&total_amount=12.34&trade_no=2026102122001400001234567890&trade_status=TRADE_SUCCESS&version=1.0
sha256 = c4f73092a9a24c5300711a639602e1b66c0b925e015ddf44d30399473baa78ef
```

---

## 5. Stripe Checkout: `stripe`

Sources: stripe-go **v87.0.0** source (`checkout_session.go`, `checkout_session_service.go`, `webhooks.go`,
`stripe.go`, `api_version.go`, `CHANGELOG.md`); Go module proxy (v86.4.2 2026-09-09, v87.0.0 2026-10-01, only
v87.1.0 pre-releases after it); docs.stripe.com *Supported currencies*, *WeChat Pay with Checkout*; Xboard fork
`StripeALLInOne.php` (pins `2026-04-22.dahlia`).

Install with `go get github.com/stripe/stripe-go/v87@v87.0.0`. After that, `stripe.APIVersion ==
"2026-09-30.endive"`.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `secret_key` | Secret key | password | ✓ | `sk_live_…`/`sk_test_…`, or a restricted `rk_…` with Checkout Sessions write. |
| `webhook_secret` | Webhook signing secret | password | ✓ | `whsec_…` of the endpoint that targets the notify URL. |
| `currency` | Currency | text | – | ISO 4217, lowercased. Defaults to the site currency. |
| `exchange_rate` | Exchange rate | number | – | Units of `currency` per site-currency unit. Required iff the currencies differ. |
| `payment_methods` | Payment methods | select | – | `auto` (default: methods enabled in the Dashboard), `card`, `alipay`, `wechat_pay`. |

**Alipay and WeChat Pay must be enabled** in Dashboard → Settings → Payment methods, and the account country must
support them. WeChat Pay supports `aud cad cny eur gbp hkd jpy sgd usd dkk nok sek chf` and one-time payments
only.

**Amount rules.**

- `minor = round(cents × rate)`.
- Zero-decimal currencies (`bif clp djf gnf jpy kmf krw mga pyg rwf vnd vuv xaf xof xpf`) use
  `round(cents × rate / 100)`.
- ISK and UGX are two-decimal in the API but must be whole units, so require `minor % 100 == 0`.
- Reject three-decimal currencies (`bhd jod kwd omr tnd`) at config save.
- Minimums apply to the settlement-currency equivalent (USD 0.50, EUR 0.50, GBP 0.30, HKD 4.00, SGD 0.50,
  JPY 50, …). Map error code `amount_too_small` to a user-facing message.
- Non-card methods allow at most 8 minor-unit digits.

### 5.1 Pay

```go
sc := stripe.NewClient(cfg["secret_key"], stripe.WithBackends(stripe.NewBackendsWithConfig(
    &stripe.BackendConfig{HTTPClient: httpClient, LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}}))) // §1 client; MaxNetworkRetries defaults to 2
p := &stripe.CheckoutSessionCreateParams{
    Mode:              stripe.String("payment"),
    ClientReferenceID: stripe.String(req.TradeNo), // ≤ 200 chars
    SuccessURL:        stripe.String(req.ReturnURL),
    CancelURL:         stripe.String(req.ReturnURL),
    ExpiresAt:         stripe.Int64(exp.Unix()), // 30 min .. 24 h after creation; default 24 h
    Metadata:          map[string]string{"trade_no": req.TradeNo, "payment_uuid": uuid},
    PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{Metadata: map[string]string{"trade_no": req.TradeNo}},
    LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
        Quantity: stripe.Int64(1),
        PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
            Currency: stripe.String(cur), UnitAmount: stripe.Int64(minor),
            ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: stripe.String(subject)}}}},
}
if pm != "auto" { p.AllowedPaymentMethodTypes = stripe.StringSlice([]string{pm}) } // PaymentMethodTypes was removed in v87
if pm == "wechat_pay" || pm == "auto" {
    p.PaymentMethodOptions = &stripe.CheckoutSessionCreatePaymentMethodOptionsParams{
        WeChatPay: &stripe.CheckoutSessionCreatePaymentMethodOptionsWeChatPayParams{Client: stripe.String("web")}}
}
s, err := sc.V1CheckoutSessions.Create(ctx, p) // → PayResult{Type: "url", Data: s.URL}
```

- `exp = clamp(req.ExpiresAt, now+31m, now+24h)` (Stripe accepts 30 min – 24 h).
- Stripe documents `payment_method_options[wechat_pay][client]=web` as required for WeChat Pay in Checkout. With
  `auto` on an account where WeChat Pay is *disabled* Stripe may reject the option, so a 400 mentioning
  `wechat_pay` is retried once without it.
- **Idempotency:** stripe-go adds an `Idempotency-Key` automatically to every POST and reuses it across its own
  network retries (`stripe.go`). Don't set a custom key: `ExpiresAt` changes between clicks, and reusing a key
  with different params fails with `idempotency_error`. Each re-click creates a new session; stale sessions expire
  on their own.
- Errors are `*stripe.Error` (`HTTPStatusCode`, `Code`, `Msg`); `amount_too_small` (and a converted amount ≤ 0 or a
  non-whole ISK/UGX amount) becomes a `gwutil.UserError` shown to the buyer.

### 5.2 Notify (webhook)

Subscribe the endpoint to `checkout.session.completed`, `checkout.session.async_payment_succeeded`,
`checkout.session.async_payment_failed` and `checkout.session.expired`. Deliveries are `POST application/json`
with a `Stripe-Signature: t=<unix>,v1=<hex>[,v1=…]` header; there are several `v1` values during secret rotation.

```go
ev, err := stripe.ConstructEvent(raw, r.Header.Get("Stripe-Signature"), cfg["webhook_secret"],
    stripe.WithIgnoreAPIVersionMismatch()) // WebhookDefaultTolerance = 300 s
```

- **What `ConstructEvent` checks:**
  - `t` is no older than 300 s;
  - some `v1` equals HMAC-SHA256(key = the **whole** secret string including `whsec_`, msg = `"<t>." + raw`);
  - the secret is not empty.
- **Version mismatch:** by default it also *rejects* events from another API release train than the SDK's
  `endive`; the endpoint's version is fixed when it's created. Ignore the mismatch and decode only stable fields
  from `ev.Data.Raw`:

  ```go
  type sess struct {
      ID                string            `json:"id"`
      ClientReferenceID string            `json:"client_reference_id"`
      PaymentStatus     string            `json:"payment_status"` // paid | unpaid | no_payment_required
      AmountTotal       int64             `json:"amount_total"`
      Currency          string            `json:"currency"`
      Metadata          map[string]string `json:"metadata"`
      PaymentIntent     json.RawMessage   `json:"payment_intent"` // "pi_…" or an expanded object with "id"
  }
  ```

Rules:

1. Require `metadata.payment_uuid == uuid`; otherwise no-op ack. The endpoint receives every subscribed event of
   the *account*, so other sites or payments must not trigger non-2xx retries.
2. `completed` with `payment_status=="paid"`, or `async_payment_succeeded` → **paid**.
3. `completed` with `unpaid` → ack and wait for the async event. `async_payment_failed`, `expired` and anything
   else → ack.
4. Return `TradeNo=client_reference_id` and `CallbackNo` = the PaymentIntent id (falling back to the session id).
   `Amount` is `amount_total` converted back to cents when the currency equals the site currency and the rate
   is 1; otherwise 0.

Response: `200 application/json {"received":true}`. In live mode Stripe retries non-2xx with backoff for up to
3 days.

**Vector** (`whsec_test_secret`). Tests use `stripe.WithIgnoreTolerance()` for this fixed `t`, or build a fresh
header with `webhook.GenerateTestSignedPayload`:

```text
payload: {"id":"evt_test_1","object":"event","type":"checkout.session.completed"}
signed:  1792584000.{"id":"evt_test_1","object":"event","type":"checkout.session.completed"}
Stripe-Signature: t=1792584000,v1=ff6756256066e28f04e7e24189356b8c4c5d7068404ca18564d9bbb9ab59bc1d
```

---

## 6. Coinbase Business Checkouts: `coinbase`

Sources: docs.cdp.coinbase.com Coinbase Business (*Migrate from Commerce* overview and API & schema mapping,
*Checkouts webhooks*, *x402 integration*), CDP *API key authentication*, Hook0 *verifying webhook signatures*,
Coinbase Help Center *Transitioning from Coinbase Commerce to Coinbase Business* (via search excerpts; direct
fetch returns 403).

Settlement is in **USDC** on Base (`network` defaults to `base`), and a Coinbase Business account is required.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `api_key_name` | CDP API key name | text | ✓ | `organizations/{org_id}/apiKeys/{key_id}` (JWT `kid` and `sub`). |
| `api_key_secret` | CDP API key secret | textarea | ✓ | ECDSA P-256 PEM (`-----BEGIN EC PRIVATE KEY-----`; literal `\n` accepted). Create the key as **ECDSA**: Ed25519 is undocumented here, and Coinbase App APIs reject it. |
| `webhook_secret` | Webhook secret | password | ✓ | The `secret` returned when creating the webhook subscription (the sample shows `metadata.secret`). |
| `exchange_rate` | Exchange rate | number | – | USDC per site-currency unit. Empty means `1`, which is only accepted for a USD/USDC/USDT site: other site currencies need an explicit rate ≠ 1 (checked on save and again before every checkout). |

**Auth.** Build a fresh JWT per request with `github.com/golang-jwt/jwt/v5` (already in the stack).

- Header: `{"alg":"ES256","kid":"<api_key_name>","nonce":"<hex of 16 random bytes>","typ":"JWT"}`.
- Claims: `{"iss":"cdp","sub":"<api_key_name>","nbf":now,"exp":now+120,"uri":"POST business.coinbase.com/api/v1/checkouts"}`.
  `uri` is `METHOD host/path`, without the scheme.
- Sign with `jwt.NewWithClaims(jwt.SigningMethodES256, claims)`, set `Header["kid"]` and `Header["nonce"]`, then
  `SignedString(ecKey)`. Get the key from `pem.Decode` → `x509.ParseECPrivateKey` (SEC1) or
  `ParsePKCS8PrivateKey`.

### 6.1 Pay

`POST https://business.coinbase.com/api/v1/checkouts` with headers `Authorization: Bearer <jwt>`,
`Content-Type: application/json` and `X-Idempotency-Key: <uuid>`:

```json
{"amount":"12.34","currency":"USDC","description":"<subject>","metadata":{"trade_no":"<trade_no>","payment_uuid":"<uuid>"},
 "successRedirectUrl":"<ReturnURL>","failRedirectUrl":"<ReturnURL>","expiresAt":"2026-10-21T12:30:00Z"}
```

- `amount = decimal(roundHalfUp(cents × exchange_rate))`.
- Idempotency key: `uuid.NewSHA1(ns, payment_uuid|trade_no|amount|expiresAt)`. A retry with the same key returns
  the same checkout, so a deterministic `expiresAt` (§0 item 7) makes re-clicks reuse it.
- Response: `{"id":"<24 hex>","url":"https://…","status":"ACTIVE","amount","currency","network","address","expiresAt",…}`
  → `PayResult{url, url}`. Non-2xx → error.
- Status flow: `ACTIVE` → `PROCESSING` → `COMPLETED` | `FAILED` | `EXPIRED` | `DEACTIVATED`. After a refund:
  `PARTIALLY_REFUNDED`/`REFUNDED`.
- Polling fallback: `GET https://business.coinbase.com/api/v1/checkouts/{id}`, with JWT `uri` set to that
  method and path.

### 6.2 Notify

The admin creates the subscription with the CDP CLI `cdp data webhooks subscriptions create` (or the CDP API):

- `eventTypes`: `checkout.payment.success`, `checkout.payment.failed`, `checkout.payment.expired`,
  `checkout.refund.success`, `checkout.refund.failed`;
- `target.url` = the notify URL; `target.method` = `POST`.

Deliveries are `POST application/json` with an `X-Webhook-Signature` header. It falls back to the deprecated
`X-Hook0-Signature`, which carries the same value. Format: `t=<unix>,h=<space-separated header names>,v1=<hex>`.

Verification steps:

1. Split on `,`, then on the first `=`. Require `t` and `v1`, and `|now − t| ≤ 300 s`.
2. Build the signed string:
   - with `h`: `t + "." + h + "." + join(values, ".") + "." + rawBody`, where `values[i] = r.Header.Get(name_i)`
     in `h` order (a missing header contributes `""`);
   - without `h`: `t + "." + rawBody`.
3. Compare `hex(HMAC-SHA256(webhook_secret, signed))` with `v1` in constant time.

Payload: `{"id":"<checkout id>","eventType":"…","status":"COMPLETED","amount":"12.34","currency":"USDC","network":"base","metadata":{…},"transactionHash":"0x…","settlement":{"totalAmount","feeAmount","netAmount","currency"},…}`

Rules:

1. Require `metadata.payment_uuid == uuid`; otherwise ack.
2. `eventType=="checkout.payment.success"` with `status=="COMPLETED"` → **paid**.
3. `failed`, `expired` and `checkout.refund.*` → ack; log refunds for the admin.
4. Return `TradeNo=metadata.trade_no`, `CallbackNo=id`, and log `transactionHash`. `Amount=cents(amount)` only
   when `exchange_rate` is empty or 1 (then USDC cents equal site cents); otherwise 0.

Deliveries can repeat. Response: `200 ok`.

**Vector** (`cb_test_secret`, request header `Content-Type: application/json`):

```text
body:   {"id":"6750a1b2c3d4e5f6a7b8c9d0","eventType":"checkout.payment.success","status":"COMPLETED","amount":"12.34","currency":"USDC","metadata":{"trade_no":"202610211234567a8b9c0d1e2f3a4b5c"}}
signed: 1792584000.content-type.application/json.<body>
X-Webhook-Signature: t=1792584000,h=content-type,v1=e8f80395b5b260133c6ea4eacc96c8434ecf5127dd3dbbca33c24833b6c32512
without h (signed = "1792584000." + body): v1=55a059c3184d57fdbdcf555e2d54966a679a3e49f5d1bac2aa6cdae74fdd438c
```

### 6.3 Legacy Commerce Charge API (closed; reference only, do not implement)

- Create: `POST https://api.commerce.coinbase.com/charges` with `X-CC-Api-Key` and `X-CC-Version: 2018-03-22`.
  Body: `{name, description, pricing_type:"fixed_price", local_price:{amount,currency}, metadata, redirect_url,
  cancel_url}`. Response: `data.hosted_url`, `data.id`, `data.code`.
- Webhook: `X-CC-Webhook-Signature` = hex HMAC-SHA256(secret, raw body). The type is in `event.type`
  (`charge:created|pending|confirmed|failed|delayed|resolved`), with `event.data.metadata` and `event.data.code`.
  Paid = `charge:confirmed` or `charge:resolved`.
- The Xboard plugin has two bugs: it sends a form-encoded body, and it fulfils on **every** event, including
  `charge:created`.

---

## 7. BTCPay Server Greenfield: `btcpay`

Sources: `btcpayserver/btcpayserver` v2.4.5, 2026-10-06 (`swagger.template.invoices.json`,
`swagger.template.webhooks.json`, `GreenfieldInvoiceController.cs`, `Plugins/Webhooks/WebhookSender.cs`); Xboard
`BTCPay.php`.

| key | label | type | req | description |
| --- | --- | --- | --- | --- |
| `server_url` | Server URL | text | ✓ | e.g. `https://btcpay.example.com`. A trailing `/` is optional (Xboard required one). |
| `store_id` | Store ID | text | ✓ | Store → Settings → General. |
| `api_key` | API key | password | ✓ | Account → API keys, with `btcpay.store.cancreateinvoice` + `btcpay.store.canviewinvoices` (scope with `:<storeId>`). |
| `webhook_secret` | Webhook secret | password | ✓ | Store → Settings → Webhooks, payload URL = the notify URL. |

### 7.1 Pay

`POST <server_url>/api/v1/stores/<store_id>/invoices` with `Authorization: token <api_key>` and
`Content-Type: application/json`:

```json
{"amount":"12.34","currency":"CNY","metadata":{"orderId":"<trade_no>","itemDesc":"<subject>"},
 "checkout":{"redirectURL":"<ReturnURL>","redirectAutomatically":true,"expirationMinutes":30}}
```

- `amount` is a decimal **string**.
- `currency` is the site currency; the store's rate source must support it.
- `metadata.orderId` is indexed, and it also fills the `{OrderId}` placeholder.
- `expirationMinutes = max(1, ceil(minutes until ExpiresAt))` (the store default is 15).
- Don't send Xboard's `jsonResponse`; it isn't a Greenfield field.

Response `200`: `{"id","storeId","amount","currency","status":"New","checkoutLink","expirationTime","metadata",…}`
→ `PayResult{url, checkoutLink}`. Errors:

- `400` → `[{"path","message"}]`;
- `401`/`403` → `{"code","message"}` (missing permission);
- `404` → unknown store.

### 7.2 Notify

Deliveries are `POST application/json` with `BTCPay-Sig: sha256=<hex>`, an HMAC-SHA256 keyed with the UTF-8
webhook secret over the **raw body bytes**. Don't `trim()` the body first, as Xboard does. The body is
`{deliveryId, webhookId, originalDeliveryId, isRedelivery, type, timestamp, storeId, invoiceId, metadata, …}`
plus type-specific fields:

| `type` | extra fields | action |
| --- | --- | --- |
| `InvoiceSettled` | `manuallyMarked`, `overPaid` | **paid**, after the confirmation GET |
| `InvoiceProcessing` | `overPaid` | Paid in full, awaiting confirmations: GET, usually ack. |
| `InvoiceReceivedPayment`, `InvoicePaymentSettled` | `afterExpiration`, `paymentMethodId`, `payment` | GET; paid only if `Settled`. Log `afterExpiration=true`. |
| `InvoiceExpired` | `partiallyPaid` | ack; log `partiallyPaid=true` for the admin |
| `InvoiceInvalid`, `InvoiceCreated`, other | – | ack |

Steps:

1. Check `storeId == cfg.store_id`; on a mismatch ack with a WARN, since it means a misconfiguration. Only the four
   payment events above are confirmed; `InvoiceExpired` and everything else are acked without a GET.
2. Confirm with `GET <server_url>/api/v1/stores/<store_id>/invoices/<invoiceId>`, same auth (`/api/v1/invoices/<id>`
   also works on 2.x).
   - Require `status == "Settled"`. `additionalStatus` may be `None`, `PaidOver` or `Marked` (settled manually by
     an admin).
   - A late full payment ends up `Expired`/`PaidLate` and never fires `InvoiceSettled`: don't auto-fulfil it; WARN
     the admin instead.
3. Return `TradeNo = metadata.orderId` (from the GET, falling back to the body), `CallbackNo = invoiceId` and
   `Amount = cents(invoice.amount)` (invoices are created in the site currency).

"Settled" timing follows the store's speed policy (confirmations); Lightning settles instantly. Xboard checks no
event type at all, so "send everything" fulfils on `InvoiceCreated`. Don't port that.

Response: `200 ok`; any 2xx counts. With automatic redelivery on, BTCPay retries after 10 s, 1 min, then 10 min
×6. Redeliveries carry `isRedelivery:true` and the same body.

**Vector** (`btcpay_test_secret`):

```text
body: {"deliveryId":"Dlv1","webhookId":"Wh1","originalDeliveryId":"Dlv1","isRedelivery":false,"type":"InvoiceSettled","timestamp":1792584000,"storeId":"Store1","invoiceId":"Inv1","metadata":{"orderId":"202610211234567a8b9c0d1e2f3a4b5c"},"manuallyMarked":false,"overPaid":false}
BTCPay-Sig: sha256=f1a2d817b438106a73e851ffee748de61af0666fb1038809ca63cbdc4d7fea17
```

---

## 8. Minimum test set per gateway

- **Signatures:** the exact sign string and digest from the vectors; one flipped byte → reject; wrong key;
  missing signature; uppercase hex → accept.
- **`Pay`** (against `httptest.NewTLSServer` fixtures): success; provider error (Epusdt HTTP 200 with
  `status_code` 400, Alipay `code` 40004, BTCPay 403, Stripe `amount_too_small`); context timeout.
- **`Notify`:** paid; every no-op event → `(nil, resp, nil)`; bad signature → `err`; stale timestamp (Stripe,
  Coinbase); foreign `payment_uuid`/`storeId`/`app_id`/`pid` → ack without a result.
- **Alipay:** generate an RSA-2048 key in the test; cover PKCS#1/PKCS#8 × PEM/bare-base64 parsing.

## 9. Implementation notes (ggboard, 2026-10-22)

- **Epusdt** sends only the vector's fields (+ `fiat` for non-CNY sites); `name`/`timeout` are not sent because legacy
  epusdt verifies a signature over its own fixed field set.
- **Stripe** `payment_methods=auto` sends `payment_method_options[wechat_pay][client]=web` and retries once without it
  when Stripe answers 400 about `wechat_pay`. `NotifyResult.Amount` is reported only for a 1:1 rate in a two-decimal
  currency (otherwise 0, so the caller skips the amount check). `customer_email` is not sent.
- **Coinbase** reports `Amount` only when `exchange_rate` is empty or 1.
- **BTCPay** reports `Amount = cents(invoice.amount)` (invoices are created in the site currency); `server_url` must
  be https.
- **EPay/Epusdt** accept `http://` base URLs with a WARN; every other gateway requires https.
- Gateway structs expose test hooks (`Client`, `Endpoint`/`BaseURL`/`BackendURL`, `Now`, Stripe `Tolerance`); the
  registered instances use the defaults.
