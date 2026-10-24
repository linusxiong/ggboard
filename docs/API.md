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
  `password_version` changes (password change or reset, subscription reset, admin password change or `reset-secret`) or the
  account's email no longer matches the token's `email` claim (admin email edit, or the id now belongs to another account
  after a deletion, `import xboard --force` or a restore); bans and role changes apply immediately because the user is reloaded (30 s cache, dropped on every admin change).

| status | `code` | when |
| --- | --- | --- |
| 400 | `validation_failed` | binding/validation failure, domain rule violation (details in `errors`) |
| 401 | `unauthorized` | missing/invalid/expired JWT, stale `password_version` or `email`, wrong credentials |
| 401 | `mfa_invalid`, `mfa_expired` | second login step (§Passport) |
| 402 | `payment_failed` | the payment gateway failed during checkout |
| 403 | `forbidden` | banned account, admin/staff only, feature switched off, invalid subscription or node token |
| 403 | `mfa_setup_required` | `/admin/*` while `admin_mfa_required` is on and the caller has neither TOTP nor a passkey |
| 404 | `not_found` | unknown route or resource (also a non-numeric or zero `:id`) |
| 406 | `not_acceptable` | client subscription in a format that cannot be age-encrypted while the user's key is on and `subscribe_age_strict` is set |
| 409 | `conflict` | state conflict or duplicate |
| 413 | `payload_too_large` | body over 1 MiB (8 MiB for node report endpoints) |
| 429 | `rate_limited` | always with `Retry-After: <seconds>` |
| 500 | `internal` | unexpected error (logged with the request id, never detailed to the client) |
| 502 | `telegram_unavailable` | the Telegram Bot API could not be reached or refused `getMe` (§Telegram) |

Rate limits (in-memory token buckets): every `/passport/*` route 30/min per IP, plus `POST /passport/email/send-code`
10/h per IP and 60/h per IPv6 /48; `POST /user/coupons/check`, `POST /user/orders` and `POST /user/orders/:trade_no/checkout` share 30/min per
user (burst 10); every `/user/security/*` route 60/min per user; rule-profile writes and the preview 60/min per user (burst
30); age-key writes and `rotate-token` 30/min per user (burst 10); `POST /user/telegram/bind-code` 10/min per user (burst
5); `POST /user/tickets` 30/h per user (burst 10) and `POST /user/tickets/:id/reply` 120/h per user (burst 30); the Telegram
webhook 600/min per IP; subscriptions 120/h per token (burst 30), unknown tokens 60/h per IP (burst 30).
Account rules add per-email limits (code cooldown, password lockout), per-IP registrations and the second-factor failure
counter.

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
| POST | `/guest/telegram/webhook/:secret` | Telegram `Update` | the bot webhook (§Telegram); 403 unless `:secret` equals `telegram_webhook_secret` and a bot token is set |

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
| POST | `/passport/email/send-code` | `{email*, captcha?}` | `true` (60 s cooldown per email → 429; 10/h per IP, 60/h per IPv6 /48; mail queue above its anonymous high-water mark → 429, `Retry-After: 60`) |

Rules: `stop_register` → 403; `email_whitelist_enable` (exact domain match, else 400 on `email`); `invite_force` requires a valid invite code (400 on `invite_code`);
a valid code sets `invite_user_id` and is consumed (`status=true`) when `invite_never_expire=false` (unknown codes are ignored unless forced);
`try_out_plan_id` grants trial on register (plan/group/limits, `transfer_enable` = GB×1024³, `expired_at` = now + `try_out_hour` h, `next_reset_at`);
password limit lockout (`password_limit_*`, per email, also counts wrong `old_password` on `/user/password` and wrong passwords on `/user/security/*`) → 429; register-by-ip limit → 429; duplicate email → 409. With `email_verify` on, the invite code and then the
email code are checked **before** the duplicate check, so without a valid code a registered and a new address get the same 400 on
`email_code` (only the mailbox owner can see the 409); such registrations stamp `users.email_verified_at`.
Login: wrong credentials 401, banned 403. Every 429 carries `Retry-After` (seconds). All passport routes: 30 requests/min per IP
(IPv6 clients are keyed by their /64, for every per-IP limit). Lockout counters are reserved before the password is checked, so
concurrent requests cannot exceed `password_limit_count`.
Email codes: 6 digits, valid 10 min, single use, discarded after 5 wrong guesses; 10 wrong codes per email per hour (whatever the
code) → 429 until the hour ends; one `send-code` serves both register and forgot (the response is `true`
whether or not the email is registered); nothing is sent, with the same `true`, when no account has the email and `stop_register`
or the email whitelist would refuse it, or when the global budget of verification-code mails (300/h, burst 60, all clients
together; `account.CodeBudgetPerHour` / `CodeBudgetBurst`) is spent (WARN log, at most one a minute). Code mails may fill at most
75 % of the mail queue (`mail.AnonymousSharePercent`), so transactional mail always has room; 400 on `email_code` when missing or invalid. Captcha (when `captcha_provider != none`) is checked on
register, login and send-code: 400 on `captcha`. `/forgot` with a valid code for an unregistered email → 400 on `email`.
`/forgot` on an account whose email was never verified (`email_verified_at` null: registered while `email_verify` was off, so
possibly by someone else with the owner's address) is an ownership change: besides the new password it issues a new uuid and
subscription token, deletes every passkey and the TOTP / backup codes / WebAuthn handle, clears the Telegram binding (and any
pending bind code) and stamps `email_verified_at`. Verified accounts (email code at registration, a previous reset, admin-created,
imported, or existing before the upgrade) keep all of that; only the password and `password_version` change.

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
| GET | `/user/subscription` | — | `{subscribe_url, token, plan, transfer_enable, u, d, expired_at, next_reset_at, reset_day, device_limit, speed_limit, profiles}` (`plan` is `{id,name}` or null; `subscribe_url` = a random entry of `subscribe_url`, else `app_url`, + `/api/v1/client/subscribe?token=`; `profiles` = `[{slug, name, is_default, subscribe_url}]`, the default profile's link is `subscribe_url`, the others append `&profile=<slug>`; a user without stored profiles gets the virtual `default` / `默认`) |
| POST | `/user/subscription/reset` | — | `{subscribe_url, token, auth_token}` (new uuid + token; the reset bumps `password_version`, so `auth_token` is the caller's new session JWT) |
| POST | `/user/subscription/rotate-token` | — | `{subscribe_url, token}` (new subscription token only: uuid, node credentials and sessions are kept; the old link stops working at once) |
| GET | `/user/subscription/preview` | `client?` (format or client name, default `clash-meta`) + every query parameter of client subscribe | `text/plain` body of the generated config (base64 formats v2ray/shadowrocket/ssr shown decoded), headers `X-Subscription-Format`, `Cache-Control: no-store`; unknown client → 400 on `client` |
| GET | `/user/traffic-logs` | `page`, `page_size` | paginated `[{record_at:"YYYY-MM-DD", node_id, node_name, rate, u, d}]` newest first (u/d already × rate); `node_name` is null for unknown/deleted nodes |
| GET | `/user/nodes` | — | array `[{id, name, type, tags, rate, host?, port?, is_online, sort}]`: `show=true` nodes of the user's group by `sort`, `id`; `host`/`port` (string) only when `show_info_to_user`; `is_online` = state online or idle; `[]` when the user is expired, out of traffic or has no group, like Xboard |
| POST | `/user/logout` | — | `true` (client discards the JWT; server no-op) |

### Rule profile

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/rule-profile` | — | `ProfileView` of the default profile: `RuleProfile` (stored, else the default template) + `customized`, `editable`, `effective`, `id`, `slug`, `name`, `is_default` (see `docs/SUBSCRIPTION.md`) |
| PUT | `/user/rule-profile` | `RuleProfile` (omitted parts filled from its template; omitted `nodes` keeps the stored node operations; empty body → 400) | same view as GET; 400 field errors (limits in `docs/SUBSCRIPTION.md`, node operations under `nodes.*`); 60 requests/min per user (shared by every rule-profile write and the preview); 403 when `allow_user_rule_profile=false` |
| GET | `/user/rule-profiles` | — | array `[{id, slug, name, is_default, customized, updated_at}]` by id (creates the `default` row for a user without profiles) |
| POST | `/user/rule-profiles` | `{name* (1..64), slug? (^[a-z0-9][a-z0-9_-]{0,31}$, derived from the name when omitted), copy_from? (slug)}` | 201 summary (`{id, slug, name, is_default, customized, updated_at}`); 400 on `name`/`slug`/`copy_from` (unknown); 409 duplicate slug (`errors.slug`) or more than 10 profiles; 403 when customization is disabled |
| GET | `/user/rule-profiles/:slug` | — | `ProfileView` of that profile; 404 unknown |
| PUT | `/user/rule-profiles/:slug` | `RuleProfile` | same rules as `PUT /user/rule-profile`, for that profile; 404 unknown |
| PATCH | `/user/rule-profiles/:slug` | `{name?, slug?}` | summary; 409 duplicate slug; 404 unknown (the web app renames only; a changed slug changes that profile's link) |
| DELETE | `/user/rule-profiles/:slug` | — | `true`; 409 for the default profile; 404 unknown (its `?profile=` link then serves the default profile) |
| POST | `/user/rule-profiles/:slug/default` | — | summary of the new default profile |
| GET | `/user/rulesets/catalog` | — | same body as the guest catalog |

Every profile write (`PUT`, `POST`, `PATCH`, `DELETE`, `…/default`) answers 403 while `allow_user_rule_profile=false`; writes and
the preview share one limiter (60 requests/min per user, burst 30).
`NodeOps` (the `nodes` object of a `RuleProfile`): `{include:[re2], exclude:[re2], types:[type], regions:[code], rename:[{match, replace}], prefix, suffix, emoji: "keep"/"add"/"strip", sort: "default"/"name"/"region", dedupe, limit}`; semantics,
the region keyword table and limits in `docs/SUBSCRIPTION.md` "Node operations".

### Subscription protection

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/subscription/age` | — | `AgeKey` (`public_key`, `private_key`, `created_at` null when the user has no key) |
| POST | `/user/subscription/age` | `{regenerate?, enabled?}` | `AgeKey` with the new key pair (X25519); a new key is disabled unless `enabled: true`; `regenerate` keeps the current switch unless `enabled` is given; 409 when a key exists and `regenerate` is not true |
| PATCH | `/user/subscription/age` | `{enabled*}` | `AgeKey`; 404 without a key |
| DELETE | `/user/subscription/age` | — | `true` (encryption off; no error without a key) |
| GET | `/user/subscription/access-log` | — | array `[{ip, client, format, at}]`, newest 50 first; `ip` masked (`203.0.113.*`, `2001:db8:1:2:*`) |

`AgeKey` = `{enabled, public_key ("age1…"), private_key ("AGE-SECRET-KEY-1…"; owner only, null when it cannot be decrypted after a jwt.secret rotation), created_at, formats (["clash-meta"]: formats served encrypted), optional (subscribe_age_optional), strict (subscribe_age_strict)}`.
`POST`/`PATCH`/`DELETE` here and `rotate-token` share 30 requests/min per user. See `docs/SUBSCRIPTION.md`
"Encrypted subscriptions (age)" and "Access log and leak detection".

### Shop

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/plans` | — | `[PublicPlan]` unpaginated: `show=true` plans plus the user's current plan even when hidden, by `sort`, `id`, with `sold_out`/`is_current` |
| GET | `/user/plans/:id` | — | `PublicPlan` (404 for hidden plans the user does not hold) |
| GET | `/user/payment-methods` | — | unpaginated `[{id, name, icon, gateway, handling_fee_fixed, handling_fee_percent}]` (enabled payments, by `sort`; `icon` exactly as stored, see [payment icons](#payment-icons)) |
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

### Telegram binding

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/user/telegram` | — | `{enabled, bound, username?, bot_username?, discuss_link}` |
| POST | `/user/telegram/bind-code` | — | `{code, bot_username, deep_link}` (10/min per user, burst 5) |
| DELETE | `/user/telegram` | — | `true` (idempotent) |

- `enabled`: a bot token is configured. `bound`: `users.telegram_id` is set. `username`: the Telegram `@username` (without `@`)
  stored at binding time, omitted when unknown (e.g. imported from Xboard). `bot_username`: from `getMe`, cached 1 h;
  omitted while the bot is off or Telegram is unreachable (after a failed `getMe` the page does not wait again for 1 min).
  `discuss_link` = `telegram_discuss_link` (`""` when unset).
- `bind-code`: `code` = 24 alphanumeric characters, single use, valid 10 min; issuing a new code revokes the previous one.
  `deep_link` = `https://t.me/<bot_username>?start=<code>`; opening it and tapping **Start** sends `/start <code>`.
  403 `forbidden` when no bot token is set; 502 `telegram_unavailable` when `getMe` fails.
- `DELETE` clears `telegram_id` and `telegram_username` and revokes a pending code.

Bound users receive their own `order_paid`, `ticket_reply` and the expiry/traffic reminders while `telegram_user_notify` is on
(§Telegram).

```json
{"data":{"enabled":true,"bound":true,"username":"alice_tg","bot_username":"ggbond_bot","discuss_link":"https://t.me/example_group"}}
{"data":{"code":"<24 chars>","bot_username":"ggbond_bot","deep_link":"https://t.me/ggbond_bot?start=<24 chars>"}}
```

## Client subscription 🔓 (token)

`GET|HEAD /api/v1/client/subscribe?token=<token>[&flag=clash][&types=vmess,vless][&filter=HK,JP][&emoji=0][&list=1][&profile=<slug or template>][&source=blackmatrix7][&cats=openai,telegram][&final=Proxy][&mirror=1][&age=0]`

- Detects the format from `flag` or `User-Agent` (see `docs/SUBSCRIPTION.md`); `types`/`filter` narrow the nodes, `emoji=0` strips
  flag emoji, `list=1` returns proxy lines only (surge, surfboard, loon, quantumultx). `profile` selects one of the user's rule
  profiles by slug (a catalog template id instead overrides the default profile's template; anything else = the default
  profile, never an error); `source`/`cats`/`final`/`mirror` override the selected profile for this request.
- Age encryption: when the user's age key is enabled, `clash-meta` bodies are armored age (`text/plain; charset=utf-8`,
  `X-Subscription-Encryption: age`, ETag `<sha1>-age-<key tag>`); `age=0` forces plaintext while `subscribe_age_optional`;
  other formats get plaintext, or 406 `not_acceptable` with `subscribe_age_strict` (rules in `docs/SUBSCRIPTION.md`).
- Every served request (200/304) is written to the subscription access log (batched, never blocking).
- Headers: `Subscription-Userinfo` (`upload=…; download=…; total=…; expire=<unix or 0>`), `Profile-Update-Interval`
  (`subscribe_update_interval` h), `Content-Disposition` (`app_name`, RFC 5987), `Profile-Web-Page-Url` (`app_url`, when set),
  `ETag`, `Cache-Control: no-store`. `If-None-Match` naming the current ETag exactly → 304.
- Unknown/malformed token (allowed: `[A-Za-z0-9_-]`, ≤ 64 chars) → 403 (no detail). Banned/expired/out-of-traffic users → 200 with
  info-only entries when `show_info_to_server_enable` (also prepended for active users), else a config without proxies.
  Invalid overrides → 400. Rate limit 120/h per token (burst 30) → 429; unknown or malformed tokens are limited per IP
  (IPv6 per /64: 60 misses/h, burst 30 → 429).
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
banned, not expired and `u + d < transfer_enable`; `online_users` = distinct users online on any node (docs/NODE_PROTOCOLS.md §5.1); `pending_tickets` = open and awaiting
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
| GET | `/admin/users/:id/subscription/age` | — | `AgeKey` without the private key (`private_key` always null) |
| POST | `/admin/users/:id/subscription/age/reset` | — | `AgeKey` without the private key: a new key pair, the enabled switch kept (the user reads the new private key in the user center); 404 when the user has no key |
| GET | `/admin/users/:id/subscription/access-log` | — | array `[{ip, ip_bucket, token_tag, client, format, at}]`, newest 100 first, unmasked |
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
- The three `subscription/*` routes are admin only (staff get 403).
- `DELETE` → 409 for yourself or the last admin. Cascade: orders, tickets + messages, stat_users, invite_codes, commission_logs earned
  as inviter, rule_profiles, passkeys, user_mfa, subscription_keys, subscription_access; users and orders that reference the user as inviter get `invite_user_id = null`.
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
| GET | `/admin/nodes/:id/online` | — | array `[{user_id, email, ips[]}]`: the users online on P, by `user_id`; their number is P's `status.online` (docs/NODE_PROTOCOLS.md §5.1). `ips` is `[]` when P reported none |
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
| POST | `/admin/payments` | `{gateway*, name*, icon?, config*, notify_domain?, handling_fee_fixed?, handling_fee_percent? (0..100), enabled? (default false), sort?}` | 201 `Payment + gateway_label + notify_url` (uuid generated; unknown gateway → 400 on `gateway`; missing required fields → `errors["config.<key>"]`; gateway rules → `errors.config`; `notify_domain` must start with `http(s)://`; bad `icon` → `errors.icon`, see [payment icons](#payment-icons)) |
| GET | `/admin/payments/:id` | — | `Payment + gateway_label + notify_url` |
| PATCH | `/admin/payments/:id` | any create field (`config` replaced as a whole) | `Payment + gateway_label + notify_url` (`icon` is validated only when the request sets it, so imported legacy values never block other edits) |
| DELETE | `/admin/payments/:id` | — | `true` |
| POST | `/admin/payments/sort` | `{ids}` | `true` (0-based position) |

`notify_url` = `<notify_domain or app_url or the request origin>/api/v1/guest/payment/notify/<gateway>/<uuid>` — register it with
the provider.

#### Payment icons

`Payment.icon` is a plain string (≤ 255, trimmed). Accepted values (`internal/api/admin/payments.go` → `validPaymentIcon`):

| value | meaning |
| --- | --- |
| `""` | no icon (the web app draws a neutral card glyph) |
| `preset:<id>` (`<id>` = `[a-z0-9_-]{1,32}`) | built-in mark from the web app's registry (`web/src/components/icons/payments`): `alipay wechat stripe card paypal crypto btc usdt eth bank qrcode wallet epay epusdt btcpay coinbase`; any well-formed id is accepted, ids the web app does not know render the fallback glyph |
| `http(s)://host/…`, `/path`, `//host/…`, `data:image/…` | an image (square SVG/PNG recommended); no whitespace |
| a single emoji / symbol | ≤ 16 code points, no ASCII letters, whitespace or control characters (`💳`, `¥`, `🇨🇳`) |

Anything else → 400 with `errors.icon`. The admin console stores a picked preset as `preset:<id>`; new payments start with the
gateway's default (`epay` → `epay`, or `alipay` / `wechat` / `usdt` from its `type`; `epusdt` → `usdt`, or `crypto` when
`currency_type` is not a USDT variant; `stripe` → `stripe`, or
`card` / `alipay` / `wechat` from `payment_methods`; `alipay_f2f` → `alipay`; `coinbase` → `coinbase`; `btcpay` → `btcpay`).

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
| POST | `/admin/knowledge` | `{language* (≤5), category*, title*, body*, show? (default true), sort?}` | 201 `Knowledge`; 403 (`errors.body`) when a staff body contains `{{subscribeUrl}}`, `{{urlEncodeSubscribeUrl}}` or `{{safeBase64SubscribeUrl}}` (admin only: they expand to each reader's token) |
| GET | `/admin/knowledge/:id` | — | `Knowledge` (raw body) |
| PATCH | `/admin/knowledge/:id` | any POST field | `Knowledge`; staff: 403 like POST unless `body` equals the stored body (then it is left alone) |
| DELETE | `/admin/knowledge/:id` | — | `true` |
| POST | `/admin/knowledge/sort` | `{ids}` | `true` (1-based position) |

`Notice` = `{id, title, content, img_url, tags, show, sort, created_at, updated_at}`;
`Knowledge` = `{id, language, category, title, body, sort, show, created_at, updated_at}`.

### Settings, rule sets & system

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/settings` | — | `{"<group>": {"<key>": value}, …, "groups": [{name, keys:[{key, group, type, default, secret?, options?, min?, max?, min_len?}]}]}`: every visible key by group (secrets included) + UI metadata in display order (`site`, `register`, `subscribe`, `order`, `invite`, `server`, `email`, `frontend`, `app`, `security`, `telegram`) |
| PUT | `/admin/settings` | `{"<key>": value, ...}` (`null` resets a key to its default) | `true`; unknown keys, invalid values and hidden keys (`rulesets_custom`, use `/admin/rulesets`) → 400 per key, nothing saved |
| POST | `/admin/settings/mail/test` | `{email*}` | `true`; sent synchronously (template `notify`) with the current mail settings; delivery error → 400 with the driver's message and `errors.email` |
| POST | `/admin/settings/telegram/webhook` | — | `{ok, description, error_code?, url}`: Telegram's answer to `setWebhook` with `url` = `<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>` (§Telegram) |
| POST | `/admin/settings/telegram/test` | — | `{sent, failed, results:[{chat_id, ok, error?}]}`: one test message per `telegram_admin_chat_ids` entry, sent synchronously (§Telegram) |
| GET | `/admin/rulesets` | — | `{builtin: catalog, custom: {...}, effective: merged catalog}` |
| PUT | `/admin/rulesets` | `{custom: {sources?, categories?, templates?, mirrors?}}` or `{custom: null}` (reset) | `true` (merged over builtin by id and validated; 400 on `custom` or per field) |
| GET | `/admin/system/status` | — | `{version, go_version, uptime (s), db_driver, started_at, scheduler:[{name, spec, running, last_run, last_duration?, last_error, next_run, runs, failures}], mail:{pending, sent, failed, driver}, mail_queue (= mail.pending), notify:{pending, sent, failed, drivers}, cache:{items, max_items, hits, misses, hit_ratio}, online:{nodes, users}, goroutines, memory:{alloc, sys}}` |
| GET | `/admin/system/logs` | `level?` (`debug` default, `info`, `warn`/`warning`, `error` = minimum level), `limit?` (1..1000, default 200) | array `[{time, level, msg, attrs}]` newest first (in-memory ring buffer, last 1000 records) |

### Mail templates 🛡

Every built-in e-mail (`verify_code`, `remind_expire`, `remind_traffic`, `order_paid`, `ticket_reply`, `notify`) can be
replaced per locale (`zh-CN`, `en`) by an admin-written **subject** and **Markdown body**. Placeholders are written exactly like
the knowledge base's (`{{siteName}}`): `{{name}}`, no spaces, case-sensitive. A custom body is rendered inside the standard
layout (logo / site name header, white card, footer): only the card's content is edited. Admin only (staff: 403).

| method | path | req | res |
| --- | --- | --- | --- |
| GET | `/admin/mail-templates` | — | array `[{name, variables:[{name, required}], locales:{"zh-CN":{custom, updated_at}, "en":{custom, updated_at}}}]` in built-in order; `updated_at` is `null` while the built-in template is in use |
| GET | `/admin/mail-templates/:name` | `locale*` (query) | `MailTemplate` |
| PUT | `/admin/mail-templates/:name` | `{locale*, subject*, body*}` | `MailTemplate` (creates or replaces the override; 400 per field, see below) |
| DELETE | `/admin/mail-templates/:name` | `locale*` (query) | `true`: back to the built-in template (idempotent) |
| POST | `/admin/mail-templates/:name/preview` | `{locale*, subject*, body*}` | `{subject, html, text, unknown:[…], missing:[…]}`: the draft rendered with the template's sample data (`mail.SampleData`: example.com links, fake order numbers) and the current site name / `app_url` in `locale`; nothing is saved or sent. Lenient: unknown placeholders stay literal and are listed in `unknown` as written (`"{{foo}}"`, subject first, no duplicates), required ones absent from the body in `missing` (`"{{code}}"`); both are `[]` when empty. An empty or multi-line subject (collapsed) and an empty body are rendered too; only `locale` and the size limits give a 400 |
| POST | `/admin/mail-templates/:name/test` | `{locale*, subject*, body*, email*}` | `true`: validates like PUT, renders with the sample data and sends synchronously with the current mail settings (template `<name>`); delivery error → 400 with the driver's message and `errors.email` |

`MailTemplate` = `{name, locale, custom, subject, body, updated_at, default:{subject, body}}`. While `custom` is false,
`subject`/`body` equal `default` (the built-in template as Markdown, the editor's starting point) and `updated_at` is `null`.
Unknown `:name` → 404 (before any other check). The saved `subject` is trimmed; the body is stored as sent. Every write
reloads the mail queue's copy at once and logs an audit line (admin id, name, locale; never the body).

```jsonc
// GET /admin/mail-templates → data[0]
{"name":"verify_code","variables":[{"name":"code","required":true},{"name":"minutes","required":false},
  {"name":"siteName","required":false},{"name":"siteUrl","required":false},{"name":"year","required":false}],
 "locales":{"en":{"custom":false,"updated_at":null},"zh-CN":{"custom":true,"updated_at":"2026-10-23T08:00:00Z"}}}
// GET /admin/mail-templates/verify_code?locale=zh-CN
{"name":"verify_code","locale":"zh-CN","custom":true,"subject":"【{{siteName}}】验证码 {{code}}","body":"# 验证\n\n**{{code}}**",
 "updated_at":"2026-10-23T08:00:00Z","default":{"subject":"【{{siteName}}】邮箱验证码","body":"# 邮箱验证码\n\n…"}}
```

Validation (PUT, test; one 400 with every field error): `locale` must be `zh-CN` or `en` exactly (`errors.locale`: `is required`,
`must be zh-CN or en`); `subject` trimmed, 1–255 characters, single line (`errors.subject`: `is required`, `must be at most 255
characters`, `must be a single line`); `body` 1–65535 bytes, not blank (`errors.body`: `is required`, `must be at most 65535
bytes`); neither may contain NUL characters (`must not contain NUL characters`: PostgreSQL cannot store them); a placeholder that is not in the template's variable list → 400 on the field that contains it (`unknown
placeholders: {{foo}}, {{ code }}`); near misses such as `{{ code }}` or `{{Code}}` count as unknown (they would stay literal);
a `required` variable missing from `body` → 400 `errors.body` (`missing required placeholders: {{code}}`; the subject does not
count). A field over its size limit is not checked further. The test mail also checks `email` (`errors.email`).

Variables (`variables` lists the template's own first, then the common ones). Common to every template: `siteName`,
`siteUrl` (`app_url`, may be empty), `year`. Per template (★ = required in the body):

| template | variables | notes |
| --- | --- | --- |
| `verify_code` | `code` ★, `minutes` | `minutes` defaults to 10 like the built-in mail |
| `remind_expire` | `expiredAt` ★, `url` | `url` = renew page (falls back to `siteUrl`) |
| `remind_traffic` | `percent` ★, `used`, `total`, `url` | `percent` is a bare number (`86`) |
| `order_paid` | `tradeNo` ★, `amount` ★, `planName`, `period`, `url` | `amount` includes the currency symbol; `url` = the order page |
| `ticket_reply` | `ticketSubject` ★, `message`, `url` | `message` is the staff reply (multi-line) |
| `notify` | `content` ★, `title`, `url` | `title` is the caller's title, else the caller's subject (= the built-in e-mail subject); `content` is multi-line. The default subject is `{{title}}`, so alerts and bulk mail keep their own subject |

Every `url` falls back to `siteUrl` when the caller passes none.

Rendering: the body is Markdown (GFM: tables, strikethrough, task lists, bare URLs become links; raw HTML allowed; a single
line break is kept as `<br>`) → HTML with the layout's inline styles; placeholder values are HTML-escaped and never read as
Markdown, HTML or placeholders, `message` and `content` keep their line breaks (`<br>`); `content` given as trusted HTML by the
panel itself (`ContentHTML`) is inserted as is (alone in a paragraph it becomes a `<div>`). Write placeholders inside raw HTML
attributes quoted (`<a href="{{url}}">`). Two conventions follow the built-in mails: a paragraph that is nothing but a link
(`[查看订单]({{url}})`) is rendered as the layout's button, and a link whose address is empty once substituted (no `app_url`)
is dropped (a button disappears, an inline link keeps its text). The plain-text part is derived from the same Markdown with raw
values (blocks separated by blank lines, links as `text (url)` or just the url when the text is the url, list items `- ` /
`1. `, raw HTML without tags with `<br>` as a line break, images as their alt text) under the usual site-name header and `--`
footer. The subject gets raw values and is collapsed to one line; an empty result falls back to the built-in subject. The
defaults reproduce the built-in mails (same subjects; the verification code box and the traffic bar are raw HTML blocks).
Built-in templates (no override) render exactly as before.

### Settings keys

Source of truth: `internal/settings/defs.go`. Types: `string`, `int` (inclusive range), `bool`, `strings` (JSON array, or a
comma/newline separated string; elements must be among the options when a key lists them), `json`. Secrets are returned to
admins only and masked in logs; `server_token` and `telegram_webhook_secret` are generated (32 chars) on first boot when empty.

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
| subscribe | `allow_user_rule_profile` | bool | `true` | users may customize their rule profiles |
| subscribe | `subscribe_age_optional` | bool | `true` | `age=0` returns plaintext although the user's age key is on |
| subscribe | `subscribe_age_strict` | bool | `false` | formats without age support get 406 instead of plaintext while the key is on |
| subscribe | `subscribe_leak_max_ips` | int 0..1000 | `0` | distinct IPs (IPv6 per /64) per token within 24 h before a leak alert (0 = off) |
| subscribe | `subscribe_leak_action` | string | `"notify"` | `notify` (log + mail the user, `subscription.LeakHook`) or `rotate` (also rotate the token and mail the new link) |
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
| server | `server_token` | string (secret, ≥ 16) | generated | node API key; at least 16 characters when set by an admin; `null` generates a new token immediately (see the note below) |
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
| email | `email_template` | string | `"default"` | legacy, never read (the Xboard importer drops it); e-mails are customised per template via `/admin/mail-templates`. Still listed by `GET /admin/settings` (not a hidden key); the admin UI does not show it |
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
| telegram | `telegram_bot_token` | string (secret) | `""` | BotFather token; empty = Telegram off |
| telegram | `telegram_admin_chat_ids` | strings | `[]` | chats receiving administrator events (numeric ids, `-100…` groups/channels or `@channel`) |
| telegram | `telegram_events` | strings | `["order_paid","admin_login","ticket_new","withdraw","subscription_leak"]` | administrator events; each element one of `order_new`, `order_paid`, `order_cancel`, `admin_login`, `ticket_new`, `ticket_reply`, `withdraw`, `subscription_leak` (anything else → 400) |
| telegram | `telegram_user_notify` | bool | `true` | bound users get their own `order_paid`, `ticket_reply`, expiry and traffic reminders |
| telegram | `telegram_discuss_link` | string | `""` | community link shown to users (`/user/telegram`, bot help) |
| telegram | `telegram_webhook_secret` | string (secret, ≥ 16) | generated | last path segment of the webhook URL and Telegram `secret_token`; generated (32 chars) on first boot; `null` regenerates it |

Note: `PUT /admin/settings {"server_token": null}` (likewise `telegram_webhook_secret`) stores a fresh random 32-character value
at once, as on first boot. `""` empties the token instead: every node request is then rejected with 403 until a new token is
saved or the next start generates one. After rotating the token, update every backend.

## Telegram

Bot driver of the notification system (`internal/notify/telegram`, docs/ARCHITECTURE.md §Notifications). Active only while
`telegram_bot_token` is set. Bot API calls: `sendMessage` (`parse_mode: HTML`, every event value HTML-escaped, link previews
off), `getMe` (cached 1 h) and `setWebhook`, each with a 10 s timeout; a 429 answer is retried after its `retry_after` (capped at
60 s). The token and the webhook secret never appear in logs or API errors.

Webhook `POST /api/v1/guest/telegram/webhook/:secret`:

- `:secret` (and `X-Telegram-Bot-Api-Secret-Token` when Telegram sends it) must equal `telegram_webhook_secret` (constant-time
  compare) and a bot token must be set, else 403. Access logs record the route pattern, never the secret. 600 requests/min per IP.
- Handles `message` updates with text. Other updates, messages from bots and undecodable bodies get an empty 200, so Telegram does
  not redeliver them.
- The reply is the webhook response itself, a method call that Telegram performs:
  `{"method":"sendMessage","chat_id":…,"text":"…","parse_mode":"HTML","link_preview_options":{"is_disabled":true}}`.
- Commands (`/cmd@<bot>` is accepted; commands addressed to another bot are ignored once the bot's username is known):

| command | chats | effect |
| --- | --- | --- |
| `/start <code>` | private | binds the account that issued `code`: `users.telegram_id` = chat id, `telegram_username` = the sender's `@username`; another account bound to the same chat is unbound first; replies 绑定成功（al***@example.com）… in `default_locale`. Invalid, used or expired codes get a short error; in groups the bot asks for a private chat |
| `/unbind` | private | unbinds every account bound to this chat; in groups the bot asks for a private chat |
| `/id` | any | replies `Chat ID: <code>…</code>` (administrators use it to fill `telegram_admin_chat_ids`) |
| `/help` | any | short help, plus `telegram_discuss_link` |
| `/start` (no code), other text and unknown commands | private | the same help; groups get no reply |

Administrator events (`telegram_events`) go to every chat in `telegram_admin_chat_ids`; a retry after a partial failure resends
only to the chats that failed. User events go to the bound chat while `telegram_user_notify` is on; a chat that is also an
administrator chat subscribed to the same event gets it once. 4xx answers other than 429 (chat not found, bot blocked) are not
retried.

| event | to | fields |
| --- | --- | --- |
| `order_new` | admins | email, plan, period, amount, trade_no (user purchases; admin-assigned orders are not reported) |
| `order_paid` | admins + buyer | email, plan, period, amount (total + handling + balance), trade_no, gateway (payment name), callback_no |
| `order_cancel` | admins | email, plan, period, amount, trade_no, reason (timeout job or administrator; a user cancelling their own order is silent) |
| `admin_login` | admins | email, role (admin or staff), method (password, password + TOTP, password + backup code, passkey), ip, ua (≤ 80 chars), time |
| `ticket_new` | admins | subject, level, user; body = the first message (≤ 200 chars) |
| `ticket_reply` | ticket owner, admins when subscribed | subject; body = the staff reply (≤ 200 chars); sent for every staff reply |
| `withdraw` | admins | user, amount, method, account (masked `abc***xyz`) |
| `subscription_leak` | admins | user, distinct (number of client IPs); `notify.EmitSubscriptionLeak` has the shape of `subscription.LeakHook` |
| `remind_expire`, `remind_traffic` | bound user | sent by `expire_remind` / `traffic_remind` alongside the mail |

`POST /admin/settings/telegram/webhook`: 400 with `errors` on `telegram_bot_token` (unset), `app_url` (unset) or
`telegram_webhook_secret` (not 16–256 characters of `A-Za-z0-9_-`); 502 `telegram_unavailable` when Telegram cannot be reached.
Telegram's refusals come back as `200 {"data":{"ok":false,"description":"…","error_code":400,"url":"…"}}`. The call registers
`allowed_updates: ["message"]` and `secret_token` = the webhook secret. Telegram requires an HTTPS `app_url` (port 443, 80, 88 or 8443).

`POST /admin/settings/telegram/test`: 400 on `telegram_bot_token` or `telegram_admin_chat_ids` when unset; otherwise 200 with one
result per chat, e.g.
`{"data":{"sent":1,"failed":1,"results":[{"chat_id":"123","ok":true},{"chat_id":"-100456","ok":false,"error":"telegram sendMessage: 400 Bad Request: chat not found"}]}}`.
