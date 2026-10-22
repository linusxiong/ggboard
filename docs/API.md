# HTTP API contract

Base path `/api/v1` (exceptions: `/healthz`, `/.well-known/*`, the short subscription link `/s/:token` and the soga mount
`/soga/api/v1/*`). JSON everywhere. Envelope and error format: `docs/ARCHITECTURE.md` §HTTP conventions (summary below).
`🔓` public, `🔑` user JWT, `🛡` admin JWT (`is_admin`), `🧑‍💼` staff (`is_admin || is_staff`), `🤖` node token.

## Conventions

- Success: `{"data": …}` with 200 (201 where a row marks it). Lists: `{"data": [...], "total": N}`.
  **Paginated** lists accept `page` (1-based), `page_size` (default 20, max 200) and `sort` (`field` or `-field`, comma
  separated; only the fields listed for the endpoint, others are ignored). **Unpaginated** lists return every row with
  `total` = row count. A few endpoints return a bare array in `data` without `total` ("array" below).
- Errors: `{"code": "...", "message": "...", "errors": {"<json field path>": ["..."]}}` (`errors` only for field errors).
- Authentication: `Authorization: Bearer <JWT>` (from `/passport/*`, lifetime `jwt.ttl`). A JWT stops working when
  `password_version` changes (password change or reset, subscription reset, admin password change or `reset-secret`); bans and
  role changes apply immediately because the user is reloaded (30 s cache, dropped on every admin change).

| status | `code` | when |
| --- | --- | --- |
| 400 | `validation_failed` | binding/validation failure, domain rule violation (details in `errors`) |
| 401 | `unauthorized` | missing/invalid/expired JWT, stale `password_version`, wrong credentials |
| 401 | `mfa_invalid`, `mfa_expired` | second login step (§Passport) |
| 402 | `payment_failed` | the payment gateway failed during checkout |
| 403 | `forbidden` | banned account, admin/staff only, feature switched off, invalid subscription or node token |
| 403 | `mfa_setup_required` | `/admin/*` while `admin_mfa_required` is on and the caller has neither TOTP nor a passkey |
| 404 | `not_found` | unknown route or resource (also a non-numeric or zero `:id`) |
| 409 | `conflict` | state conflict or duplicate |
| 413 | `payload_too_large` | body over 1 MiB (8 MiB for node report endpoints) |
| 429 | `rate_limited` | always with `Retry-After: <seconds>` |
| 500 | `internal` | unexpected error (logged with the request id, never detailed to the client) |

Rate limits (in-memory token buckets): every `/passport/*` route 30/min per IP, plus `POST /passport/email/send-code`
10/h per IP; `POST /user/coupons/check`, `POST /user/orders` and `POST /user/orders/:trade_no/checkout` share 30/min per
user (burst 10); every `/user/security/*` route 60/min per user; subscriptions 120/h per token (burst 30). Account rules
add per-email limits (code cooldown, password lockout), per-IP registrations and the second-factor failure counter.

`GET /healthz` 🔓 → `{"data":{"status":"ok","version":"…","uptime":"1h2m3s"}}`.

Shared object shapes:

```jsonc
User (self view) {
  "id":1,"email":"a@example.com","uuid":"...","plan_id":2,"plan":{"id":2,"name":"Pro"},"group_id":1,
  "transfer_enable":107374182400,"u":0,"d":0,"expired_at":"2027-01-01T00:00:00Z"|null,"next_reset_at":"..."|null,
  "speed_limit":null,"device_limit":null,"balance":0,"commission_balance":0,"commission_rate":null,"discount":null,
  "remind_expire":true,"remind_traffic":true,"is_admin":false,"is_staff":false,"banned":false,
  "avatar_url":"https://www.gravatar.com/avatar/<md5>?s=200&d=mp","created_at":"...","last_login_at":"..."|null
}
Plan (admin) {"id","group_id","name","content","tags","transfer_enable"(GB),"speed_limit","device_limit","capacity_limit",
      "prices":{"monthly":1000,...}  /* period -> cents; absent period = not sold */,
      "reset_traffic_method"(null = setting),"show","sell","renew","sort","created_at","updated_at"}
      // admin endpoints add "user_count" (holders whose subscription has not expired)
PublicPlan {"id","name","content","tags","transfer_enable"(GB),"speed_limit","device_limit","prices","reset_traffic_method",
      "show","sell","renew","sort","sold_out"(capacity full; never for the holder),"is_current"(user's plan),"created_at","updated_at"}
      // no group_id / capacity_limit
Node (admin) {"id","type","name","group_ids","route_ids","parent_id","host","port"(string),"server_port","tags","rate","show","sort",
      "protocol_settings":{...},"parent":{"id","name"}|null,
      "status":{"state":"online|idle|offline","online":3,"last_check_at":"..."|null,"last_push_at":"..."|null,
                "load":{"cpu","mem","swap","disk","uptime"}|null,"version":"<backend User-Agent>"},
      "created_at","updated_at"}   // a child's status is its parent's (docs/NODE_PROTOCOLS.md §5.3)
Route {"id","remarks","match":["..."],"action":"block|dns","action_value","created_at","updated_at"}
Order {"id","trade_no","user_id","plan_id","plan":{"id","name"}|null,"type"(1 new,2 renew,3 upgrade,4 reset_traffic),"period",
      "status"(0 pending,1 processing,2 cancelled,3 completed,4 discounted),"commission_status"(0 pending,1 processing,2 valid,3 invalid),
      "total_amount","handling_amount","discount_amount","surplus_amount","refund_amount","balance_amount",
      "payment_id","payment":{"id","name"}|null,"callback_no","paid_at","created_at","updated_at"}
AdminOrder = Order + {"user":{"id","email"}|null,"invite_user_id","coupon_id","commission_balance","surplus_order_ids":[...]}
Ticket {"id","user_id","subject","level"(0..2),"status"(0 open,1 closed),"reply_status"(0 awaiting staff,1 replied),
        "last_reply_user_id","created_at","updated_at",
        "messages":[{"id","user_id","is_me","message","created_at"}] /* detail responses only, oldest first */,
        "user":{"id","email"} /* staff responses only */}
        // is_me: user API = written by the owner; admin API = written by staff (anyone but the owner), as in Xboard
Passkey {"id":1,"name":"Laptop","created_at":"…","last_used_at":null,"backup_eligible":true,"transports":["internal","hybrid"]}
```

## Guest 🔓 (`/guest`)

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/guest/config` | — | `{app_name, app_description, app_url, logo, tos_url, currency, currency_symbol, register_enabled, email_verify, email_whitelist_suffix[], invite_force, captcha:{provider, site_key}, downloads:{windows:{url,version}, macos, android, ios}, locales:["zh-CN","en"], passkey_enabled, allow_user_rule_profile}` — `register_enabled` = `!stop_register`; `email_whitelist_suffix` is `[]` unless `email_whitelist_enable`; `captcha.provider` is one of `none`, `turnstile`, `recaptcha`, `hcaptcha` (the secret is never returned); `passkey_enabled` is true when a WebAuthn RP id can be derived (`passkey_rp_id` or the host of `app_url`) |
| GET | `/guest/plans` | — | `[PublicPlan]` unpaginated: `show && sell` plans with capacity left, by `sort`, `id` (`sold_out`/`is_current` always false) |
| GET | `/guest/rulesets/catalog` | — | public catalog `{sources, categories, templates, mirrors, clients, defaults, editable}` (no URL templates; `Cache-Control: public, max-age=300`; see `docs/SUBSCRIPTION.md`) |
| ANY | `/guest/payment/notify/:gateway/:uuid` | gateway-specific | status, content type and body chosen by the gateway plugin (`docs/PAYMENTS.md`) |
| GET | `/guest/payment/return/:trade_no` | — | 302 → `<app_url>/orders/<trade_no>` (gateway query parameters dropped; a relative `/orders/<trade_no>` when `app_url` is empty) |

## Passport 🔓 (`/passport`) — rate limited

| method | path | req | res |
| --- | --- | --- | --- |
| POST | `/passport/register` | `{email*, password* (8..128), invite_code?, email_code? (required when email_verify), captcha?}` | `{token, user}` |
| POST | `/passport/login` | `{email*, password*, captcha?}` | `{token, user}`; when the account has TOTP enabled: `{mfa_required:true, mfa_token, methods:["totp"], has_passkey}` (no session token, see below) |
| POST | `/passport/login/mfa` | `{mfa_token*, code*}` (`code`: 6-digit TOTP or backup code `xxxxx-xxxxx`) | `{token, user}` |
| POST | `/passport/login/token` | `{token*}` one-time login token (from admin "login as", valid 5 min) | `{token, user}` (no second factor asked) |
| POST | `/passport/passkey/login/begin` | `{email?}` | `{challenge_id, options}` (`options` = `PublicKeyCredentialRequestOptionsJSON`) |
| POST | `/passport/passkey/login/finish` | `{challenge_id*, credential*}` (`credential` = `AuthenticationResponseJSON`) | `{token, user}` |
| POST | `/passport/forgot` | `{email*, email_code*, password* (8..128)}` | `true` |
| POST | `/passport/email/send-code` | `{email*, captcha?}` | `true` (60 s cooldown per email → 429; 10/h per IP) |

Rules: `stop_register` → 403; `email_whitelist_enable` (exact domain match, else 400 on `email`); `invite_force` requires a valid invite code (400 on `invite_code`);
a valid code sets `invite_user_id` and is consumed (`status=true`) when `invite_never_expire=false` (unknown codes are ignored unless forced);
`try_out_plan_id` grants trial on register (plan/group/limits, `transfer_enable` = GB×1024³, `expired_at` = now + `try_out_hour` h, `next_reset_at`);
password limit lockout (`password_limit_*`, per email, also counts wrong `old_password` on `/user/password` and wrong passwords on `/user/security/*`) → 429; register-by-ip limit → 429; duplicate email → 409.
Login: wrong credentials 401, banned 403. Every 429 carries `Retry-After` (seconds). All passport routes: 30 requests/min per IP
(IPv6 clients are keyed by their /64, for every per-IP limit). Lockout counters are reserved before the password is checked, so
concurrent requests cannot exceed `password_limit_count`.
Email codes: 6 digits, valid 10 min, single use, discarded after 5 wrong guesses; 10 wrong codes per email per hour (whatever the
code) → 429 until the hour ends; one `send-code` serves both register and forgot (the response is `true`
whether or not the email is registered); 400 on `email_code` when missing or invalid. Captcha (when `captcha_provider != none`) is checked on
register, login and send-code: 400 on `captcha`. `/forgot` with a valid code for an unregistered email → 400 on `email`.

Second factor (TOTP) — `internal/service/mfa`:

- `/login` with TOTP enabled checks the password (lockout, captcha, ban as usual) and returns
  `{"data":{"mfa_required":true,"mfa_token":"<64 hex>","methods":["totp"],"has_passkey":false}}`. `last_login_*` is recorded only
  after the second step. `methods` is always `["totp"]`; backup codes are accepted by the same endpoint.
- `mfa_token`: opaque, valid 5 min, single use, bound to the user id and `password_version` (a password change revokes it).
- `/login/mfa`: TOTP accepts the current step ±1 (30 s steps) and never the same or an older step twice; a backup code is consumed.
  Wrong code → 401 `mfa_invalid`; the 5th wrong code for a token revokes it → 429 (`Retry-After`); unknown, used or expired token
  (or TOTP disabled meanwhile) → 401 `mfa_expired`; banned → 403. Independently, 20 wrong second-factor codes per user per hour
  (login, disable, backup-codes) → 429 with `Retry-After` until the window ends.

Passkeys (WebAuthn, go-webauthn) — never need a second step:

- RP id = `passkey_rp_id` or the host of `app_url` (no port); allowed origins = the `app_url` origin + `passkey_origins`
  (`https://<rp id>` when neither gives one); RP name = `app_name`. Without an RP id every passkey endpoint returns 400.
- `begin` without `email` → discoverable (usernameless) login: `options` has no `allowCredentials`. With `email` the user's passkeys are
  listed in `allowCredentials`; an unknown email (or an account without passkeys) gets the discoverable options with a fresh random
  challenge, so the response never reveals whether the account exists.
- The challenge lives 2 min, is single use (consumed by the first `finish`, even a failed one) and is bound to the RP id.
- `finish`: unknown/expired challenge → 401; malformed `credential` → 400 on `credential`; failed verification (signature, origin,
  RP id hash, foreign credential, sign counter not increasing — a possible clone) → 401; banned → 403. On success `sign_count`,
  `backup_state` and `last_used_at` are updated and `last_login_*` recorded.
- Options JSON (`options`, pass as `optionsJSON` to `@simplewebauthn/browser` `startAuthentication`):
  `{"challenge":"<b64url>","timeout":120000,"rpId":"panel.example.com","allowCredentials":[{"type":"public-key","id":"<b64url>","transports":["internal","hybrid"]}],"userVerification":"preferred"}`
  (`allowCredentials` omitted for discoverable logins).
- `credential` is the object returned by `startAuthentication`:
  `{"id":"<b64url>","rawId":"<b64url>","type":"public-key","response":{"clientDataJSON":"<b64url>","authenticatorData":"<b64url>","signature":"<b64url>","userHandle":"<b64url>"},"clientExtensionResults":{},"authenticatorAttachment":"platform"}`.

Password-manager well-known URLs 🔓 (root paths, not under `/api/v1`; explicit routes, so never served by the SPA fallback):

- `GET /.well-known/change-password` → 302 to `<app_url>/profile?tab=security` (the request's scheme and host when `app_url`
  is empty), `Cache-Control: no-store`.
- `GET /.well-known/passkey-endpoints` → `{"enroll":"<base>/profile?tab=security","manage":"<base>/profile?tab=security"}` (no
  envelope), `Content-Type: application/json`, `Cache-Control: public, max-age=3600`.

## User 🔑 (`/user`)

Every route requires a valid JWT (401) of a user who is not banned (403).

### Account

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/me` | — | `User` |
| PATCH | `/user/me` | `{remind_expire?, remind_traffic?}` | `User` |
| POST | `/user/password` | `{old_password*, new_password* (8..128)}` | `true`; bumps `password_version`, so every session (the caller's too) must sign in again; wrong `old_password` → 400 on `old_password` (counts towards the password lockout, 429) |
| GET | `/user/stats` | — | `{pending_orders, open_tickets, notices_count, invite_count, commission_pending}` (`commission_pending` as in `/user/invite` `stat`) |
| GET | `/user/subscription` | — | `{subscribe_url, token, plan, transfer_enable, u, d, expired_at, next_reset_at, reset_day, device_limit, speed_limit}` (`plan` is `{id,name}` or null; `subscribe_url` = a random entry of `subscribe_url`, else `app_url`, + `/api/v1/client/subscribe?token=`) |
| POST | `/user/subscription/reset` | — | `{subscribe_url, token, auth_token}` (new uuid + token; the reset bumps `password_version`, so `auth_token` is the caller's new session JWT) |
| GET | `/user/subscription/preview` | `client?` (format or client name, default `clash-meta`) + every query parameter of client subscribe | `text/plain` body of the generated config (base64 formats v2ray/shadowrocket/ssr shown decoded), headers `X-Subscription-Format`, `Cache-Control: no-store`; unknown client → 400 on `client` |
| GET | `/user/traffic-logs` | `page`, `page_size` | paginated `[{record_at:"YYYY-MM-DD", node_id, node_name, rate, u, d}]` newest first (u/d already × rate); `node_name` is null for unknown/deleted nodes |
| GET | `/user/nodes` | — | array `[{id, name, type, tags, rate, host?, port?, is_online, sort}]`: `show=true` nodes of the user's group by `sort`, `id`; `host`/`port` (string) only when `show_info_to_user`; `is_online` = state online or idle; `[]` when the user is expired, out of traffic or has no group, like Xboard |
| POST | `/user/logout` | — | `true` (client discards the JWT; server no-op) |

### Rule profile

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/rule-profile` | — | `RuleProfile` (stored, else the default template) + `customized`, `editable`, `effective` (see `docs/SUBSCRIPTION.md`) |
| PUT | `/user/rule-profile` | `RuleProfile` (omitted parts filled from its template; empty body → 400) | same view as GET; 400 field errors (limits in `docs/SUBSCRIPTION.md`); 60 requests/min per user (also for preview); 403 when `allow_user_rule_profile=false` |
| GET | `/user/rulesets/catalog` | — | same body as the guest catalog |

### Shop

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/plans` | — | `[PublicPlan]` unpaginated: `show=true` plans plus the user's current plan even when hidden, by `sort`, `id`, with `sold_out`/`is_current` |
| GET | `/user/plans/:id` | — | `PublicPlan` (404 for hidden plans the user does not hold) |
| GET | `/user/payment-methods` | — | unpaginated `[{id, name, icon, gateway, handling_fee_fixed, handling_fee_percent}]` (enabled payments, by `sort`) |
| POST | `/user/coupons/check` | `{code*, plan_id*, period*}` | `{id, code, name, type, value, limit_plan_ids, limit_periods, ended_at}` or 400 on `code`, `plan_id` or `period` |
| GET | `/user/orders` | `page`, `page_size`, `status?` (0..4), `sort` (`id created_at`) | paginated `[Order]` (default newest first) |
| POST | `/user/orders` | `{plan_id*, period*, coupon_code?}` | 201 `{trade_no, status}` (status 3 when nothing was left to pay: free or covered by balance). 409 when an unpaid order exists (`errors.trade_no` = its trade_no), the plan is sold out (`errors.plan_id`) or the balance changed meanwhile; 404 unknown or hidden plan; 403 upgrade while `plan_change_enable=false`; 400 on `period`, `plan_id` (not for sale, renew disabled, expired legacy plan, reset without an active subscription) or `coupon_code` |
| GET | `/user/orders/:trade_no` | — | `Order` (404 for other users' orders) |
| POST | `/user/orders/:trade_no/checkout` | `{payment_id?}` (omit when nothing is left to pay) | `{type, data}`: `type` is `url` (redirect to `data`), `qrcode` (render `data`), `html` (render the auto-submitting form) or `paid` (`data` = trade_no; already paid, or fulfilled now because the total is 0). 400 on `payment_id` (missing, unknown or disabled); 402 `payment_failed` when the gateway fails; 409 cancelled or no longer pending; 409 when the order is past `created_at + order_timeout_minutes` |
| GET | `/user/orders/:trade_no/status` | — | `{status}` (poll while paying) |
| POST | `/user/orders/:trade_no/cancel` | — | `true` (409 unless pending; balance and coupon use restored) |

### Invite & commission

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/invite` | — | `{codes:[{code, pv, created_at}], stat:{register_count, commission_pending, commission_paid, commission_rate, invite_gen_limit}}` |
| POST | `/user/invite/codes` | — | 201 `{code}` (32 alphanumeric chars; 400 when the user already holds `invite_gen_limit` unused codes) |
| GET | `/user/invite/commissions` | `page`, `page_size` | paginated `[{trade_no, order_amount, get_amount, created_at}]` |
| POST | `/user/commission/transfer` | `{amount*}` (cents, > 0) | `User` (commission → balance, atomic; 400 on `amount` when it exceeds `commission_balance`) |
| POST | `/user/commission/withdraw` | `{method*, account*}` | 201 `{ticket_id}` (creates a ticket; see rules) |

Invite rules (Xboard): `codes` lists the user's unused codes (`status=false`, newest first; consumed codes can no longer be
used, whatever `invite_never_expire` says now). `register_count` = users with `invite_user_id = me`; `commission_paid` =
Σ `commission_logs.get_amount` with `invite_user_id = me`; `commission_pending` = Σ `orders.commission_balance` with
`invite_user_id = me`, a paid order status (1 processing, 3 completed or 4 discounted) and `commission_status` 0 or 1
(× `commission_distribution_l1` % when `commission_distribution_enable`); `commission_rate` = `users.commission_rate` when not
null, else `invite_commission` (the commission itself treats a rate of 0 as unset). Commission history skips rows with `get_amount = 0`, newest first. Withdraw: 403 when
`withdraw_close_enable`; 400 on `method` unless listed in `commission_withdraw_method`; 400 unless `commission_balance > 0`
and `commission_balance ≥ commission_withdraw_limit × 100`; 409 while the user has an open ticket (the request **is** a
ticket). It opens a level-2 ticket (subject/message in `default_locale`: method, account, current commission balance +
`currency`); nothing is deducted, staff settle it manually.

### Support & content

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/tickets` | `page`, `page_size`, `status?` (0 or 1), `sort` (`id created_at updated_at`) | paginated `[Ticket]` (own, no messages, default latest activity first) |
| POST | `/user/tickets` | `{subject* (≤255), level* (0..2), message* (≤10000)}` | 201 `Ticket` with its first message (409 while the user has another **open** ticket, Xboard rule) |
| GET | `/user/tickets/:id` | — | `Ticket` with messages (404 for other users' tickets) |
| POST | `/user/tickets/:id/reply` | `{message*}` | `Ticket` (400 when closed; → `reply_status=0`) |
| POST | `/user/tickets/:id/close` | — | `true` (idempotent) |
| GET | `/user/notices` | `page`, `page_size` | paginated `[{id, title, content, img_url, tags, created_at}]` (show=true, `sort` ASC then newest first) |
| GET | `/user/knowledge` | `language?` (default `default_locale`), `keyword?` | `{"<category>":[{id, title, updated_at}]}` (show=true, `sort` ASC, `id` ASC) |
| GET | `/user/knowledge/:id` | — | `{id, title, body, category, language, updated_at}` (404 when hidden); `body` with `{{siteName}}`, `{{subscribeUrl}}`, `{{urlEncodeSubscribeUrl}}`, `{{safeBase64SubscribeUrl}}` replaced |

Knowledge rules: `language=zh-CN` matches exactly, a bare primary tag (`en`) also matches regional tags (`en-US`, imported
from Xboard). `keyword` is a case-insensitive substring of title or body (`%`/`_` literal). Users without a valid plan
(`plan_id` null or not `IsAvailable`: banned, expired or traffic used up) get every `<!--access start-->…<!--access end-->`
block replaced by `<div class="knowledge-no-access">…</div>` (text in the article's language); placeholders are replaced
after that, `{{urlEncodeSubscribeUrl}}` = query escaping, `{{safeBase64SubscribeUrl}}` = URL-safe base64 without padding.

### Security (2FA / passkeys)

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/security` | — | `{totp_enabled, totp_enabled_at, backup_codes_remaining, passkeys:[Passkey]}` (`totp_enabled_at` null when off) |
| POST | `/user/security/totp/setup` | — | `{secret, otpauth_url}` (pending secret kept 10 min; issuer = `mfa_issuer` or `app_name`, account = email; 409 when already enabled) |
| POST | `/user/security/totp/enable` | `{code*}` | `{backup_codes:[10 × "xxxxx-xxxxx"]}` (shown once; wrong code → 400 on `code`, the pending secret is discarded after 5; no pending setup → 400 on `code`; 409 when already enabled) |
| POST | `/user/security/totp/disable` | `{password*, code*}` | `true` (`code` = TOTP or backup code; deletes secret and backup codes; wrong password → 400 on `password` (counts towards the password lockout), wrong code → 400 on `code`; 409 when not enabled) |
| POST | `/user/security/backup-codes` | `{code*}` | `{backup_codes}` (regenerates all 10; `code` = TOTP or backup code; 409 when TOTP is off) |
| POST | `/user/security/passkeys/begin` | `{name?}` (max 64) | `{challenge_id, options}` (`options` = `PublicKeyCredentialCreationOptionsJSON`) |
| POST | `/user/security/passkeys/finish` | `{challenge_id*, credential*, name?}` (`credential` = `RegistrationResponseJSON`) | 200 `Passkey` (bad/expired/foreign challenge → 400 on `challenge_id`; failed attestation → 400 on `credential`; already registered → 409) |
| PATCH | `/user/security/passkeys/:id` | `{name*}` (max 64) | `Passkey` (404 when not yours) |
| DELETE | `/user/security/passkeys/:id` | JSON body `{password*}` | `true` (wrong password → 400 on `password`, counts towards the lockout; 404 when not yours) |

Rules: enabling/disabling TOTP and adding/removing passkeys keep every session valid (`password_version` is not bumped). Backup codes
are 10 lower-case Crockford base32 characters shown as `xxxxx-xxxxx` (input is case-insensitive, the dash optional) and are stored as
SHA-256 hashes; the TOTP secret is stored AES-256-GCM encrypted with a key derived (HKDF-SHA256) from `jwt.secret`, so rotating the JWT
secret invalidates enrolled authenticators. Every `/user/security/*` route: 60 requests/min per user.
Passkey registration: resident key required (`residentKey:"required"`, `requireResidentKey:true`), user verification preferred,
attestation `none`, `credProps` requested, existing passkeys in `excludeCredentials`, user handle = 16 random bytes per user (stable
across registrations), timeout 120 s, challenge valid 2 min and single use. `name` defaults to "Synced passkey"/"Device passkey"
(platform authenticator, backup eligible or not), "Phone passkey" (hybrid), "Security key" (usb/nfc/ble) or "Passkey".
Options JSON (pass as `optionsJSON` to `startRegistration`):
`{"rp":{"name":"GG Bond","id":"panel.example.com"},"user":{"name":"a@example.com","displayName":"a@example.com","id":"<b64url 16 bytes>"},"challenge":"<b64url>","pubKeyCredParams":[{"type":"public-key","alg":-7},…],"timeout":120000,"excludeCredentials":[{"type":"public-key","id":"<b64url>","transports":["internal"]}],"authenticatorSelection":{"requireResidentKey":true,"residentKey":"required","userVerification":"preferred"},"attestation":"none","extensions":{"credProps":true}}`.
`credential` is the object returned by `startRegistration`:
`{"id","rawId","type":"public-key","response":{"clientDataJSON","attestationObject","transports":[…],"publicKeyAlgorithm","publicKey","authenticatorData"},"clientExtensionResults":{},"authenticatorAttachment":"platform"}`
(only `id`, `rawId`, `type`, `response.clientDataJSON`, `response.attestationObject` are required; `transports` is stored).
Admin enforcement: with `admin_mfa_required` on, `/admin/*` answers 403 `mfa_setup_required` to admins/staff without TOTP and without
a passkey (`mfasvc.AdminGate`, mounted right after `RequireStaff`); `/user/security` stays reachable to enrol.

## Client subscription 🔓 (token)

`GET|HEAD /api/v1/client/subscribe?token=<token>[&flag=clash][&types=vmess,vless][&filter=HK,JP][&emoji=0][&list=1][&profile=standard][&source=blackmatrix7][&cats=openai,telegram][&final=Proxy][&mirror=1]`

- Detects the format from `flag` or `User-Agent` (see `docs/SUBSCRIPTION.md`); `types`/`filter` narrow the nodes, `emoji=0` strips
  flag emoji, `list=1` returns proxy lines only (surge, surfboard, loon, quantumultx), `profile`/`source`/`cats`/`final`/`mirror`
  override the user's rule profile for this request.
- Headers: `Subscription-Userinfo` (`upload=…; download=…; total=…; expire=<unix or 0>`), `Profile-Update-Interval`
  (`subscribe_update_interval` h), `Content-Disposition` (`app_name`, RFC 5987), `Profile-Web-Page-Url` (`app_url`, when set),
  `ETag`, `Cache-Control: no-store`. `If-None-Match` with the current ETag → 304.
- Unknown/malformed token (allowed: `[A-Za-z0-9_-]`, ≤ 64 chars) → 403 (no detail). Banned/expired/out-of-traffic users → 200 with
  info-only entries when `show_info_to_server_enable` (also prepended for active users), else a config without proxies.
  Invalid overrides → 400. Rate limit 120/h per token (burst 30) → 429; unknown tokens are limited per IP (30 misses → 429).
- Also mounted at `GET|HEAD /s/:token` (short link) — same handler and rate-limit bucket; access logs record the route pattern,
  never the token.

## Node APIs 🤖 — see `docs/NODE_PROTOCOLS.md`

- XrayR/V2bX (query `token`, `node_id`, `node_type`): `GET /api/v1/server/UniProxy/config`, `GET …/user`, `POST …/push`,
  `POST …/alive`, `GET …/alivelist`, `POST …/status`.
- soga-v1 (headers `API-KEY`, `NODE-ID`, `NODE-TYPE`): `GET /node`, `GET /users`, `GET /audit_rules`, `GET /white_list`,
  `POST /traffic`, `POST /alive_ip`, `POST /audit_log`, `POST /status`. Mounted under both `/api/v1/...` and
  `/soga/api/v1/...` (recommended `webapi_url=https://host/soga/`; double slashes are tolerated).
- Wrong token → 403; bad node id/type or unknown node → 400. `config`/`user` (UniProxy) and every soga GET send an `ETag` and
  answer 304 to a matching `If-None-Match`. Report bodies may be up to 8 MiB.

## Admin 🛡 (`/admin`)

The group requires staff (`RequireStaff`, then the MFA gate); every area below is admin only except users (read),
tickets, notices and knowledge (🧑‍💼).

### Stats

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/stats/overview` | — | `{today:{income, paid_orders, registers, commission}, month:{income, paid_orders, registers}, totals:{users, active_users, online_users, pending_tickets, pending_orders, pending_commission}, nodes:{total, online}}` |
| GET | `/admin/stats/series` | `metric*` (`income`, `orders`, `registers` or `traffic`), `range?` (`7d`, `30d` default, `90d`) | array `[{date, value}]` |
| GET | `/admin/stats/nodes/ranking` | `date?` (YYYY-MM-DD, default today) | array `[{node_id, name, u, d, total}]` busiest first, every node with traffic that day |
| GET | `/admin/stats/users/ranking` | `date?`, `limit?` (1..500, default 20) | array `[{user_id, email, u, d, total}]` busiest first |

Stats rules (`internal/service/stat`): days are calendar days in the server time zone. "Paid" orders are `status ∈ {1 processing,
3 completed, 4 discounted}` bucketed by `paid_at` (a discounted order was paid and later credited to an upgrade, so it stays
revenue); `income` = Σ `total_amount`, `today.commission` = Σ `commission_balance` of today's paid orders. `active_users` = not
banned, not expired and `u + d < transfer_enable`; `online_users` = users with an alive IP; `pending_tickets` = open and awaiting
staff; `pending_orders` = status 0; `pending_commission` = paid orders with an inviter, `commission_balance > 0` and
`commission_status = 0`; `nodes.online` counts nodes in state online or idle (a child reports its parent, docs/NODE_PROTOCOLS.md §5.1).
`series` returns `range` points, oldest first, today last; past days read the `stats` row written by `stats_daily`
(`orders` = `paid_count`) and fall back to live queries, today is always live, `traffic` = Σ `stat_nodes` u+d. Deleted
nodes/users keep their ranking rows with an empty `name`/`email`.

### Users

| method | path | req | res |
| --- | --- | --- | --- |
| GET 🧑‍💼 | `/admin/users` | filters `id`, `email`, `uuid`, `token`, `plan_id`, `group_id`, `invite_user_id`, `banned`, `is_admin`, `is_staff`, `expired` (`active`, `expired` or `never`), `remarks`; `sort` (see rules) | paginated `[AdminUser]` |
| GET 🧑‍💼 | `/admin/users/:id` | — | `AdminUser` + `stats:{orders_total, commission_total}` |
| POST | `/admin/users` | `{email*, password* (8..128), plan_id?, group_id?, transfer_enable? (GB), expired_at?, is_admin?, is_staff?, speed_limit?, device_limit?, balance?, remarks?, invite_user_id?}` | 201 `AdminUser` |
| PATCH | `/admin/users/:id` | any editable field (see rules) | `AdminUser` |
| DELETE | `/admin/users/:id` | — | `true` |
| POST | `/admin/users/:id/reset-secret` | — | `AdminUser` (new uuid/token; bumps `password_version`, ending the user's sessions) |
| POST | `/admin/users/:id/login-token` | — | `{url}` one-time login URL |
| POST | `/admin/users/:id/traffic-reset` | — | `AdminUser` (u = d = 0) |
| POST | `/admin/users/generate` | `{email_prefix?, email_suffix*, count* (1..500), plan_id?, expired_at?, password? (8..128)}` | `{count, csv}` |
| POST | `/admin/users/export` | same filters as the list, in the JSON body | `text/csv` attachment |
| POST | `/admin/users/mail` | `{<filters>, subject* (≤255), content*}` | `{queued}` |

Users rules (`internal/service/adminuser`): only `GET /admin/users` and `GET /admin/users/:id` are open to staff, and staff get `uuid`
and `token` blanked. `AdminUser` = self view + `token`, `commission_type`, `remarks`, `invite_user_id`, `invite_user_email`,
`last_login_ip`, `telegram_id`, `updated_at` (never the password). Filters: `email`/`remarks` are case-insensitive substrings; `plan_id`, `group_id`,
`invite_user_id` = `0` select null; `expired` partitions users into `active` (expires in the future), `expired` (expired) and `never`
(no expiry). `sort` accepts `id email expired_at created_at u d transfer_enable balance last_login_at` (default `-id`).

- `POST /admin/users` → 201. `plan_id` fills `group_id`, `transfer_enable`, `speed_limit`, `device_limit` when omitted;
  `next_reset_at` follows the plan; duplicate email → 409; unknown plan/group/inviter → 400 field error.
- `PATCH` fields: `email password banned u d transfer_enable(bytes) expired_at plan_id group_id invite_user_id is_admin is_staff
  speed_limit device_limit balance commission_balance commission_type commission_rate discount remarks remind_expire remind_traffic
  telegram_id`; `null` clears nullable fields. Setting `plan_id` without `group_id` also moves the user to the plan's group;
  `next_reset_at` is recomputed when `plan_id` or `expired_at` is present. 409: banning or demoting yourself, demoting the last admin,
  duplicate email. Sessions are invalidated immediately (password, ban, roles).
- `DELETE` → 409 for yourself or the last admin. Cascade: orders, tickets + messages, stat_users, invite_codes, commission_logs earned
  as inviter, rule_profiles, passkeys, user_mfa; users and orders that reference the user as inviter get `invite_user_id = null`.
- `login-token` → `{url: app_url + "/login?token=<one-time, 5 min>"}`; the SPA posts the token to `/passport/login/token`.
  Administrators and staff cannot be impersonated: 409 for such a target (the token skips the second factor), and a token
  whose user was promoted after it was issued is refused with 401.
- `generate`: emails are `<prefix><8 random [a-z0-9]>@<suffix>` (exactly `<prefix>@<suffix>` when `count` is 1 and a prefix is
  given). The whole batch shares `password`, or one random 12-character password when omitted (one argon2id hash per batch);
  `csv` = `email,password` rows. Taken single email → 409.
- `export`: `text/csv` attachment `users-<timestamp>.csv` with `email,plan,expired_at,transfer_enable,u,d,balance,created_at`
  (RFC3339 UTC, bytes, cents; cells starting with `= + - @` are prefixed with `'`).
- `mail`: plain-text `content` (blank lines = paragraphs) through the `notify` template to every matched user, ignoring `remind_*`;
  recipients that do not fit the mail queue are queued in the background. `{queued}` = number of recipients.

### Plans / groups / nodes / routes

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/plans` | — | unpaginated `[Plan + user_count]` by `sort`, `id` |
| POST | `/admin/plans` | Plan fields (`group_id*`, `name*`; defaults `show=false`, `sell=true`, `renew=true`; `prices` keys must be periods, values ≥ 0) | 201 `Plan + user_count` |
| GET | `/admin/plans/:id` | — | `Plan + user_count` |
| PATCH | `/admin/plans/:id` | any Plan field (`null` clears `speed_limit`, `device_limit`, `capacity_limit`, `reset_traffic_method`) + `force_update` (push group, traffic and limits to every holder) | `Plan + user_count` |
| DELETE | `/admin/plans/:id` | — | `true` (409 while any user holds the plan or it has pending/processing orders) |
| POST | `/admin/plans/sort` | `{ids:[...]}` (1..1000) | `true` (`sort` = 0-based position) |
| GET | `/admin/groups` | — | unpaginated `[{id, name, user_count, plan_count, node_count, created_at, updated_at}]` |
| POST | `/admin/groups` | `{name*}` | 201 group with counts |
| PATCH | `/admin/groups/:id` | `{name*}` | group with counts |
| DELETE | `/admin/groups/:id` | — | `true` (409 while users, plans or nodes reference it) |
| GET | `/admin/nodes` | — | unpaginated `[Node]` (with status) by `sort`, `id` |
| POST | `/admin/nodes` | Node fields (`type*`, `name*`, `host*`, `port*` as a number or a string `"443"`, `"20000-30000"`, `"443,8443"`; `server_port*` unless a child inherits its parent's; `rate` default 1) | 201 `Node` |
| GET | `/admin/nodes/:id` | — | `Node` |
| PATCH | `/admin/nodes/:id` | any subset of Node fields (`parent_id: null` detaches; `protocol_settings` is replaced, not merged) | `Node` |
| DELETE | `/admin/nodes/:id` | — | `true` (409 while the node has children; stats are kept) |
| POST | `/admin/nodes/:id/copy` | — | 201 `Node` (`name + " (copy)"`, `show=false`) |
| POST | `/admin/nodes/sort` | `{ids}` | `true` (`sort` = 0-based position in `ids`) |
| GET | `/admin/nodes/:id/stats` | `range?` (`7d` default or `30d`) | array `[{date, u, d}]` one point per day, oldest first, zero-filled (raw bytes of the parent P) |
| GET | `/admin/nodes/:id/online` | — | array `[{user_id, email, ips[]}]` (alive on P) |
| GET | `/admin/nodes/:id/audit` | — | array `[{at, node_id, user_id, audit_id, route_id, rule}]`: soga audit hits of P, newest first (in memory, last 1000 overall) |
| GET | `/admin/routes` | — | unpaginated `[Route]` by `id` |
| POST | `/admin/routes` | `{remarks?, match* (1..999 entries), action* (block or dns), action_value}` | 201 `Route` (block: `match` joined by a pipe must compile as a Go regexp; dns: `action_value` required unless `match[0]=="main"`) |
| PATCH | `/admin/routes/:id` | any POST field | `Route` |
| DELETE | `/admin/routes/:id` | — | `true` (409 while a node references it) |

Node validation (400 per field): `type` ∈ vmess, vless, shadowsocks, shadowsocksr, trojan, hysteria, anytls, mieru, tuic;
`group_ids`/`route_ids` must exist; a parent must exist, have the same type and not be a child itself; a node with children
cannot get a parent or change its type; `protocol_settings` is validated per type for parents only (`docs/NODE_PROTOCOLS.md`).

### Orders / coupons / payments

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/orders` | `trade_no`, `user_id`, `email`, `status`, `commission_status`, `plan_id`, `payment_id`, `invite_user_id`, `type`, `date_from`, `date_to` (YYYY-MM-DD with `date_to` inclusive, or RFC 3339; on `created_at`); `sort` (`id created_at paid_at total_amount status`) | paginated `[AdminOrder]` (default newest first) |
| GET | `/admin/orders/:id` | — | `AdminOrderDetail` = `AdminOrder` + `commission_logs`, `surplus_orders` |
| POST | `/admin/orders` | `{user_id* or email*, plan_id*, period*, total_amount?}` | 201 `AdminOrder` (assigned, pending; no discount/coupon/surplus/balance; 409 if the user has an unpaid order; 400 when the plan has no price for `period` and `total_amount` is omitted) |
| POST | `/admin/orders/:id/paid` | — | `AdminOrderDetail` (pending only, else 409; `callback_no="manual"` → fulfil) |
| POST | `/admin/orders/:id/cancel` | — | `AdminOrderDetail` (pending only, else 409; balance and coupon use restored) |
| PATCH | `/admin/orders/:id` | `{commission_status*}` (0, 1 or 3) | `AdminOrderDetail` (2 is set by `commission_settle` → 400; an already paid-out commission → 409) |
| GET | `/admin/coupons` | `code?` (exact), `name?` (substring); `sort` (`id created_at started_at ended_at value`) | paginated `[Coupon + used_count]` (`used_count` = non-cancelled orders) |
| POST | `/admin/coupons` | `{name*, code?, type* (1 amount in cents, 2 percent ≤ 100), value* (> 0), count? (1..500; > 1 = batch of generated 8-char codes), limit_use? (remaining uses), limit_use_with_user?, limit_plan_ids?, limit_periods?, started_at*, ended_at* (after started_at), show? (default true)}` | 201 `[Coupon + used_count]` (409 duplicate code) |
| GET | `/admin/coupons/:id` | — | `Coupon + used_count` |
| PATCH | `/admin/coupons/:id` | any create field except `count` (`null` clears `limit_use`, `limit_use_with_user`) | `Coupon + used_count` (409 duplicate code) |
| DELETE | `/admin/coupons/:id` | — | `true` |
| GET | `/admin/payments` | — | unpaginated `[Payment + gateway_label + notify_url]` by `sort` (`config` included) |
| GET | `/admin/payments/gateways` | — | unpaginated `[{name, label, fields:[{key, label, description?, type, options?, default?, required}]}]` (`type` ∈ text, password, number, select, textarea) |
| POST | `/admin/payments` | `{gateway*, name*, icon?, config*, notify_domain?, handling_fee_fixed?, handling_fee_percent? (0..100), enabled? (default false), sort?}` | 201 `Payment + gateway_label + notify_url` (uuid generated; unknown gateway → 400 on `gateway`; missing required fields → `errors["config.<key>"]`; gateway rules → `errors.config`; `notify_domain` must start with `http(s)://`) |
| GET | `/admin/payments/:id` | — | `Payment + gateway_label + notify_url` |
| PATCH | `/admin/payments/:id` | any create field (`config` replaced as a whole) | `Payment + gateway_label + notify_url` |
| DELETE | `/admin/payments/:id` | — | `true` |
| POST | `/admin/payments/sort` | `{ids}` | `true` (0-based position) |

`notify_url` = `<notify_domain or app_url or the request origin>/api/v1/guest/payment/notify/<gateway>/<uuid>` — register it with
the provider.

### Support & content 🧑‍💼

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/tickets` | `status?`, `reply_status?`, `user_id?`, `email?` (exact), `page`, `page_size`, `sort` (`id created_at updated_at level status reply_status`) | paginated `[Ticket + user]` (default latest activity first) |
| GET | `/admin/tickets/:id` | — | `Ticket` with messages + user |
| POST | `/admin/tickets/:id/reply` | `{message*}` | `Ticket` (reopens a closed ticket, `reply_status=1`; e-mails the owner `ticket_reply`, at most once per ticket per 30 min — an owner reply re-arms it) |
| POST | `/admin/tickets/:id/close` | — | `true` |
| GET | `/admin/notices` | — | unpaginated `[Notice]` all (hidden included), display order |
| POST | `/admin/notices` | `{title*, content*, img_url? (http(s) or ""), tags? (≤20 × ≤32 chars), show? (default true), sort?}` | 201 `Notice` |
| PATCH | `/admin/notices/:id` | any POST field | `Notice` |
| DELETE | `/admin/notices/:id` | — | `true` |
| POST | `/admin/notices/sort` | `{ids}` (unique, all existing) | `true` (`sort` = position, 1-based) |
| GET | `/admin/knowledge` | `language?`, `category?`, `keyword?` | unpaginated `[{id, language, category, title, show, sort, created_at, updated_at}]` (no body), display order |
| POST | `/admin/knowledge` | `{language* (≤5), category*, title*, body*, show? (default true), sort?}` | 201 `Knowledge` |
| GET | `/admin/knowledge/:id` | — | `Knowledge` (raw body) |
| PATCH | `/admin/knowledge/:id` | any POST field | `Knowledge` |
| DELETE | `/admin/knowledge/:id` | — | `true` |
| POST | `/admin/knowledge/sort` | `{ids}` | `true` (1-based position) |

`Notice` = `{id, title, content, img_url, tags, show, sort, created_at, updated_at}`;
`Knowledge` = `{id, language, category, title, body, sort, show, created_at, updated_at}`.

### Settings, rule sets & system

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/settings` | — | `{"<group>": {"<key>": value}, …, "groups": [{name, keys:[{key, group, type, default, secret?, options?, min?, max?}]}]}`: every visible key by group (secrets included) + UI metadata in display order |
| PUT | `/admin/settings` | `{"<key>": value, ...}` (`null` resets a key to its default) | `true`; unknown keys, invalid values and hidden keys (`rulesets_custom`, use `/admin/rulesets`) → 400 per key, nothing saved |
| POST | `/admin/settings/mail/test` | `{email*}` | `true`; sent synchronously (template `notify`) with the current mail settings; delivery error → 400 with the driver's message and `errors.email` |
| GET | `/admin/rulesets` | — | `{builtin: catalog, custom: {...}, effective: merged catalog}` |
| PUT | `/admin/rulesets` | `{custom: {sources?, categories?, templates?, mirrors?}}` or `{custom: null}` (reset) | `true` (merged over builtin by id and validated; 400 on `custom` or per field) |
| GET | `/admin/system/status` | — | `{version, go_version, uptime (s), db_driver, started_at, scheduler:[{name, spec, running, last_run, last_duration?, last_error, next_run, runs, failures}], mail:{pending, sent, failed, driver}, mail_queue (= mail.pending), cache:{items, max_items, hits, misses, hit_ratio}, online:{nodes, users}, goroutines, memory:{alloc, sys}}` |
| GET | `/admin/system/logs` | `level?` (`debug` default, `info`, `warn`/`warning`, `error` = minimum level), `limit?` (1..1000, default 200) | array `[{time, level, msg, attrs}]` newest first (in-memory ring buffer, last 1000 records) |

### Settings keys

Source of truth: `internal/settings/defs.go`. Types: `string`, `int` (inclusive range), `bool`, `strings` (JSON array, or a
comma/newline separated string), `json`. Secrets are returned to admins only and masked in logs; `server_token` is generated
(32 chars) on first boot when empty.

| group | key | type | default | notes |
| --- | --- | --- | --- | --- |
| site | `app_name` | string | `"GG Bond"` | |
| site | `app_description` | string | `""` | |
| site | `app_url` | string | `""` | public base URL: links, passkey RP id, payment return/notify fallback |
| site | `logo` | string | `""` | |
| site | `tos_url` | string | `""` | |
| site | `currency` | string | `"CNY"` | |
| site | `currency_symbol` | string | `"¥"` | |
| site | `default_locale` | string | `"zh-CN"` | `zh-CN` or `en` |
| site | `show_info_to_user` | bool | `true` | host/port in `/user/nodes` |
| register | `stop_register` | bool | `false` | |
| register | `email_verify` | bool | `false` | |
| register | `email_whitelist_enable` | bool | `false` | |
| register | `email_whitelist_suffix` | strings | `[]` | exact domains |
| register | `invite_force` | bool | `false` | |
| register | `register_limit_by_ip_enable` | bool | `false` | |
| register | `register_limit_count` | int 1.. | `3` | registrations per IP |
| register | `register_limit_expire` | int 1.. | `60` | window, minutes |
| register | `password_limit_enable` | bool | `true` | |
| register | `password_limit_count` | int 1.. | `5` | failed passwords per email |
| register | `password_limit_expire` | int 1.. | `60` | lockout window, minutes |
| register | `try_out_plan_id` | int 0.. | `0` | trial plan on register (0 = off) |
| register | `try_out_hour` | int 0.. | `0` | trial length, hours |
| register | `captcha_provider` | string | `"none"` | `none`, `turnstile`, `recaptcha`, `hcaptcha` |
| register | `captcha_site_key` | string | `""` | |
| register | `captcha_secret` | string (secret) | `""` | |
| subscribe | `subscribe_url` | strings | `[]` | alternative subscription hosts (one picked at random; empty → `app_url`) |
| subscribe | `plan_change_enable` | bool | `true` | allow upgrades to another plan |
| subscribe | `reset_traffic_method` | int 0..4 | `0` | 0 first day of month, 1 monthly, 2 never, 3 first day of year, 4 yearly (plans may override) |
| subscribe | `surplus_enable` | bool | `true` | credit the unused value on upgrades |
| subscribe | `show_info_to_server_enable` | bool | `true` | info entries (traffic, expiry) in subscriptions |
| subscribe | `show_protocol_to_server_enable` | bool | `false` | prefix node names with the protocol |
| subscribe | `default_remind_expire` | bool | `true` | default for new users |
| subscribe | `default_remind_traffic` | bool | `true` | default for new users |
| subscribe | `ruleset_default_template` | string | `"standard"` | catalog template id |
| subscribe | `ruleset_default_source` | string | `"blackmatrix7"` | catalog source id |
| subscribe | `ruleset_mirror` | string | `""` | GitHub mirror prefix, e.g. `https://ghproxy.net/` (empty = direct) |
| subscribe | `subscribe_update_interval` | int 1..720 | `24` | hours (`Profile-Update-Interval`) |
| subscribe | `allow_user_rule_profile` | bool | `true` | users may customize their rule profile |
| subscribe | `rulesets_custom` | json (hidden) | `{}` | only via `/admin/rulesets` |
| order | `order_timeout_minutes` | int 1..10080 | `30` | pending orders are cancelled after this |
| invite | `invite_commission` | int 0..100 | `10` | percent |
| invite | `invite_gen_limit` | int 0.. | `5` | unused codes per user |
| invite | `invite_never_expire` | bool | `true` | false = a code is consumed by its first use |
| invite | `commission_first_time_enable` | bool | `true` | inviters with `commission_type` 0: commission on the invitee's first paid order only (off = every order) |
| invite | `commission_auto_check_enable` | bool | `true` | `commission_settle` approves pending commissions of completed orders after 72 h |
| invite | `commission_withdraw_limit` | int 0.. | `100` | minimum withdrawal, currency units |
| invite | `commission_withdraw_method` | strings | `["USDT","Alipay"]` | |
| invite | `withdraw_close_enable` | bool | `false` | no withdrawals; settled commission goes to `balance` instead |
| invite | `commission_distribution_enable` | bool | `false` | split each commission across up to three upline levels (l1/l2/l3 %) |
| invite | `commission_distribution_l1` | int 0..100 | `0` | percent |
| invite | `commission_distribution_l2` | int 0..100 | `0` | percent |
| invite | `commission_distribution_l3` | int 0..100 | `0` | percent |
| server | `server_token` | string (secret) | generated | node API key; see note below at least 16 chars when set by an admin; `null` regenerates a new token immediately |
| server | `server_pull_interval` | int 10..86400 | `60` | seconds |
| server | `server_push_interval` | int 10..86400 | `60` | seconds |
| server | `server_white_list` | strings | `[]` | soga `white_list` |
| server | `device_limit_mode` | int 0..1 | `0` | 0 strict (per IP), 1 relaxed (IPv4 /24, IPv6 /64) |
| email | `email_driver` | string | `"log"` | `log` or `smtp` |
| email | `email_host` | string | `""` | |
| email | `email_port` | int 1..65535 | `465` | |
| email | `email_encryption` | string | `"ssl"` | `ssl`, `tls` or `none` |
| email | `email_username` | string | `""` | |
| email | `email_password` | string (secret) | `""` | |
| email | `email_from_address` | string | `""` | |
| email | `email_from_name` | string | `""` | |
| email | `email_template` | string | `"default"` | stored and imported, not read by the mailer yet |
| email | `remind_mail_enable` | bool | `true` | expiry/traffic reminder mails |
| frontend | `frontend_accent` | string | `"#007AFF"` | editable, not exposed by `/guest/config` yet |
| frontend | `frontend_background_url` | string | `""` | editable, not exposed by `/guest/config` yet |
| frontend | `frontend_customer_service_url` | string | `""` | editable, not exposed by `/guest/config` yet |
| frontend | `frontend_footer_html` | string | `""` | editable, not exposed by `/guest/config` yet |
| app | `windows_download_url`, `windows_version` | string | `""` | `downloads.windows` in `/guest/config` |
| app | `macos_download_url`, `macos_version` | string | `""` | `downloads.macos` |
| app | `android_download_url`, `android_version` | string | `""` | `downloads.android` |
| app | `ios_download_url`, `ios_version` | string | `""` | `downloads.ios` |
| security | `passkey_rp_id` | string | `""` | empty → host of `app_url` |
| security | `passkey_origins` | strings | `[]` | extra allowed origins (the `app_url` origin is always allowed) |
| security | `admin_mfa_required` | bool | `false` | admins/staff need TOTP or a passkey for `/admin/*` |
| security | `mfa_issuer` | string | `""` | TOTP issuer; empty → `app_name` |

Note: `PUT /admin/settings {"server_token": null}` (or `""`) empties the token; every node request is then rejected with 403
until a new token is saved or the next start generates one. Rotate it by saving a new random value and updating every backend.
