# HTTP API contract

Base path `/api/v1`. JSON everywhere. Envelope and error format: `docs/ARCHITECTURE.md` §HTTP conventions.
`🔓` public, `🔑` user JWT, `🛡` admin JWT (`is_admin`), `🧑‍💼` staff (`is_admin || is_staff`), `🤖` node token.
Lists accept `page`, `page_size`, `sort` and return `{"data": [...], "total": N}`.

Shared object shapes:
```jsonc
User (self view) {
  "id":1,"email":"a@example.com","uuid":"...","plan_id":2,"plan":{"id":2,"name":"Pro"},"group_id":1,
  "transfer_enable":107374182400,"u":0,"d":0,"expired_at":"2027-01-01T00:00:00Z"|null,"next_reset_at":"...",
  "speed_limit":null,"device_limit":null,"balance":0,"commission_balance":0,"commission_rate":null,"discount":null,
  "remind_expire":true,"remind_traffic":true,"is_admin":false,"is_staff":false,"banned":false,
  "avatar_url":"https://www.gravatar.com/avatar/<md5>?s=200&d=mp","created_at":"...","last_login_at":"..."
}
Plan {"id","group_id","name","content","tags","transfer_enable"(GB),"speed_limit","device_limit","capacity_limit",
      "prices":{"monthly":1000,...},"reset_traffic_method","show","sell","renew","sort","created_at","updated_at"}
Node (admin) {"id","type","name","group_ids","route_ids","parent_id","host","port","server_port","tags","rate","show","sort",
      "protocol_settings":{...},"parent":{"id","name"}|null,
      "status":{"state":"online|idle|offline","online":3,"last_check_at":"...","last_push_at":"...","load":{cpu,mem,swap,disk,uptime}|null,"version":"<backend User-Agent>"},
      "created_at","updated_at"}   // a child's status is its parent's (docs/NODE_PROTOCOLS.md §5.3)

      "protocol_settings":{...},"status":{"online":3,"last_check_at":"...","last_push_at":"...","load":{...}|null},"created_at","updated_at"}
PublicPlan {"id","name","content","tags","transfer_enable"(GB),"speed_limit","device_limit","prices","reset_traffic_method","show","sell","renew","sort",
      "sold_out"(capacity full; never for the holder),"is_current"(user's plan),"created_at","updated_at"}
Order {"id","trade_no","user_id","plan_id","plan":{...},"type","period","status","commission_status","total_amount","handling_amount",
      "discount_amount","surplus_amount","refund_amount","balance_amount","payment_id","payment":{"id","name"},"callback_no","paid_at","created_at","updated_at"}
Ticket {"id","user_id","subject","level","status","reply_status","last_reply_user_id","created_at","updated_at",
        "messages":[{"id","user_id","is_me","message","created_at"}] /* detail responses only, oldest first */,
        "user":{"id","email"} /* staff responses only */}
        // is_me: user API = written by the owner; admin API = written by staff (anyone but the owner), as in Xboard
```

## Guest 🔓 (`/guest`)
| method | path | req | res |
|---|---|---|---|
| GET | `/guest/config` | — | `{app_name, app_description, app_url, logo, tos_url, currency, currency_symbol, register_enabled, email_verify, email_whitelist_suffix[], invite_force, captcha:{provider:"none|turnstile|recaptcha|hcaptcha", site_key}, downloads:{windows:{url,version},macos:{...},android:{...},ios:{...}}, locales:["zh-CN","en"], passkey_enabled}` — `email_whitelist_suffix` is `[]` unless `email_whitelist_enable`; the captcha secret is never returned; `passkey_enabled` is true when a WebAuthn RP id can be derived (`app_url` or `passkey_rp_id` set) |
| GET | `/guest/plans` | — | `[Plan]` where show=true & sell=true (prices only; no capacity leak) |

| GET | `/guest/config` | — | `{app_name, app_description, app_url, logo, tos_url, currency, currency_symbol, register_enabled, email_verify, email_whitelist_suffix[], invite_force, captcha:{provider:"none|turnstile|recaptcha|hcaptcha", site_key}, downloads:{windows:{url,version},macos:{...},android:{...},ios:{...}}, locales:["zh-CN","en"]}` |
| GET | `/guest/plans` | — | `[PublicPlan]` where show=true & sell=true and capacity left, by `sort` (no `group_id`/`capacity_limit`) |
| ANY | `/guest/payment/notify/:gateway/:uuid` | gateway-specific | gateway-specific body (plugin returns it) |
| GET | `/guest/payment/return/:trade_no` | — | 302 → `<app_url>/orders/:trade_no` (gateway query parameters dropped) |
| GET | `/guest/rulesets/catalog` | — | catalog (see `docs/SUBSCRIPTION.md`) |

| GET | `/guest/payment/return/:trade_no` | — | 302 → `/orders/:trade_no` |
| GET | `/guest/rulesets/catalog` | — | public catalog `{sources, categories, templates, mirrors, clients, defaults, editable}` (see `docs/SUBSCRIPTION.md`; no URL templates) |

## Passport 🔓 (`/passport`) — rate limited
| method | path | req | res |
|---|---|---|---|
| POST | `/passport/register` | `{email*, password*(min 8), invite_code?, email_code?, captcha?}` | `{token, user}` |
| POST | `/passport/login` | `{email*, password*, captcha?}` | `{token, user}`; when the account has TOTP enabled: `{mfa_required:true, mfa_token, methods:["totp"], has_passkey}` (no session token, see below) |
| POST | `/passport/forgot` | `{email*, email_code*, password*}` | `true` |
| POST | `/passport/email/send-code` | `{email*, captcha?}` | `true` (60s cooldown per email, 10/h per IP) |
| POST | `/passport/login/token` | `{token*}` one-time login token (from admin "login as") | `{token, user}` |
| POST | `/passport/login/mfa` | `{mfa_token*, code*}` (`code`: 6-digit TOTP or backup code `xxxxx-xxxxx`) | `{token, user}` |
| POST | `/passport/passkey/login/begin` | `{email?}` | `{challenge_id, options}` (`options` = `PublicKeyCredentialRequestOptionsJSON`) |
| POST | `/passport/passkey/login/finish` | `{challenge_id*, credential*}` (`credential` = `AuthenticationResponseJSON`) | `{token, user}` |
Rules: `stop_register` → 403; `email_whitelist_enable` (exact domain match, else 400 on `email`); `invite_force` requires valid invite code (400 on `invite_code`);
a valid code sets `invite_user_id` and is consumed (`status=true`) when `invite_never_expire=false` (unknown codes are ignored unless forced);
`try_out_plan_id` grants trial on register (plan/group/limits, `transfer_enable` = GB×1024³, `expired_at` = now + `try_out_hour` h, `next_reset_at`);
password limit lockout (`password_limit_*`, per email, also counts wrong `old_password` on `/user/password`) → 429; register-by-ip limit → 429; duplicate email → 409.
Login: wrong credentials 401, banned 403. Every 429 carries `Retry-After` (seconds). All passport routes: 30 requests/min per IP.
Email codes: 6 digits, valid 10 min, single use, discarded after 5 wrong guesses; one `send-code` serves both register and forgot (the response is `true`
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
- `/login/token` (admin "login as") does not ask for a second factor.

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

Password-manager well-known URLs (root paths, not under `/api/v1`; explicit routes, so never served by the SPA fallback):
- `GET /.well-known/change-password` → 302 to `<app_url>/profile?tab=security` (request origin when `app_url` is empty).
- `GET /.well-known/passkey-endpoints` → `{"enroll":"<base>/profile?tab=security","manage":"<base>/profile?tab=security"}`,
  `Content-Type: application/json`, `Cache-Control: public, max-age=3600`.

## User 🔑 (`/user`)
### Account
| method | path | req | res |
|---|---|---|---|
| GET | `/user/me` | — | `User` |
| PATCH | `/user/me` | `{remind_expire?, remind_traffic?}` | `User` |
| POST | `/user/password` | `{old_password*, new_password*(min 8)}` | `true` (bumps password_version; wrong `old_password` → 400) |
| GET | `/user/stats` | — | `{pending_orders, open_tickets, notices_count, invite_count, commission_pending}` (`commission_pending` = Σ `commission_balance` of paid orders — status processing/completed/discounted — invited by me with `commission_status` pending/processing) |
| GET | `/user/subscription` | — | `{subscribe_url, token, plan, transfer_enable, u, d, expired_at, next_reset_at, reset_day, device_limit, speed_limit}` (`plan` is `{id,name}` or null) |
| POST | `/user/subscription/reset` | — | `{subscribe_url, token, auth_token}` (new uuid + token; the reset bumps password_version, so `auth_token` is the caller's new session JWT) |
| GET | `/user/subscription/preview?client=clash&...` | same query as client subscribe | `text/plain` body of generated config |
| GET | `/user/traffic-logs` | `page`, `page_size` | `[{record_at:"YYYY-MM-DD", node_id, node_name, rate, u, d}]` newest first; `node_name` is null for unknown/deleted nodes |

| POST | `/user/password` | `{old_password*, new_password*}` | `true` (bumps password_version) |
| GET | `/user/stats` | — | `{pending_orders, open_tickets, notices_count, invite_count, commission_pending}` |
| GET | `/user/subscription` | — | `{subscribe_url, token, plan, transfer_enable, u, d, expired_at, next_reset_at, reset_day, device_limit, speed_limit}` |
| POST | `/user/subscription/reset` | — | `{subscribe_url, token}` (new uuid + token) |
| GET | `/user/subscription/preview?client=clash-meta&...` | same query as client subscribe (`client` = format or client name, default `clash-meta`) | `text/plain` body of generated config (base64 formats decoded), header `X-Subscription-Format` |
| GET | `/user/traffic-logs` | `page`, `page_size` | `[{record_at, node_id, node_name, rate, u, d}]` |
| GET | `/user/nodes` | — | `[{id,name,type,tags,rate,host,port,is_online,sort}]` (host/port only when `show_info_to_user`=true; default true) |

| GET | `/user/traffic-logs` | `page`, `page_size` | `[{record_at, node_id, node_name, rate, u, d}]` |
| GET | `/user/nodes` | — | `[{id,name,type,tags,rate,host,port,is_online,sort}]` (show=true nodes of the user's group, by sort, id; host/port only when `show_info_to_user`=true, default true; `[]` when the user is banned, expired or out of traffic, like Xboard) |
| POST | `/user/logout` | — | `true` (client discards; server no-op) |
### Rule profile
| GET | `/user/rule-profile` | — | `RuleProfile` (defaults if none) + `customized`, `editable`, `effective` (see `docs/SUBSCRIPTION.md`) |
| PUT | `/user/rule-profile` | `RuleProfile` (omitted parts filled from its template) | same view as GET; 400 field errors; 403 when `allow_user_rule_profile=false` |
| GET | `/user/rulesets/catalog` | — | same as guest catalog |
### Shop
| GET | `/user/plans` | — | `[PublicPlan]` (show=true; includes current plan even if hidden, with `sell/renew` flags) |
| GET | `/user/plans/:id` | — | `PublicPlan` (404 for hidden plans the user does not hold) |
| GET | `/user/payment-methods` | — | `[{id,name,icon,gateway,handling_fee_fixed,handling_fee_percent}]` (enabled, by `sort`) |
| POST | `/user/coupons/check` | `{code*, plan_id*, period*}` | `{id,code,name,type,value,limit_plan_ids,limit_periods,ended_at}` or 400 (`errors.code`) |
| GET | `/user/orders` | `page`,`page_size`,`status?` | `[Order]` (newest first) |
| POST | `/user/orders` | `{plan_id*, period*, coupon_code?}` | 201 `{trade_no, status}` (status 3 when nothing was left to pay). 409 if a pending order exists (`errors.trade_no=[pending]`) or the plan is sold out; 403 plan change disabled; 400 `period`/`plan_id`/`coupon_code` |
| GET | `/user/orders/:trade_no` | — | `Order` |
| POST | `/user/orders/:trade_no/checkout` | `{payment_id?}` (omit when total 0 / balance covers) | `{type:"url"|"qrcode"|"html"|"paid", data:"..."}` (402 `payment_failed` when the gateway fails; 409 cancelled) |
| GET | `/user/orders/:trade_no/status` | — | `{status}` (poll while paying) |
| POST | `/user/orders/:trade_no/cancel` | — | `true` (409 unless pending; balance and coupon use restored) |
### Invite & commission
| GET | `/user/invite` | — | `{codes:[{code,pv,created_at}], stat:{register_count, commission_pending, commission_paid, commission_rate, invite_gen_limit}}` |
| POST | `/user/invite/codes` | — | 201 `{code}` (32 alphanumeric chars; 400 when the user already holds `invite_gen_limit` unused codes) |
| GET | `/user/invite/commissions` | `page`, `page_size` | `[{trade_no, order_amount, get_amount, created_at}]` |
| POST | `/user/commission/transfer` | `{amount*}` (cents, > 0) | `User` (commission → balance, atomic; 400 `errors.amount` when it exceeds `commission_balance`) |
| POST | `/user/commission/withdraw` | `{method*, account*}` | 201 `{ticket_id}` (creates ticket; respects `commission_withdraw_limit`, `withdraw_close_enable`) |
Invite rules (Xboard): `codes` lists the user's unused codes (`status=false`, newest first; consumed codes can no longer be
used, whatever `invite_never_expire` says now). `register_count` = users with `invite_user_id = me`; `commission_paid` =
Σ `commission_logs.get_amount`; `commission_pending` = Σ `orders.commission_balance` with `invite_user_id = me`, `status = 3`
and `commission_status ∈ {0,1}` (× `commission_distribution_l1`% when `commission_distribution_enable`);
`commission_rate` = `users.commission_rate` ?? `invite_commission`. Commission history skips rows with `get_amount = 0`,
newest first. Withdraw: 403 when `withdraw_close_enable`; 400 `errors.method` unless listed in `commission_withdraw_method`;
400 unless `0 < commission_balance` and `commission_balance ≥ commission_withdraw_limit × 100`; 409 while the user has an
open ticket (the request **is** a ticket). It opens a level-2 ticket (subject/message in `default_locale`: method, account,
current commission balance + `currency`); nothing is deducted, staff settle it manually.
### Support & content
| GET | `/user/tickets` | `page`,`page_size`,`status?` | `[Ticket]` (own, no messages, latest activity first) |
| POST | `/user/tickets` | `{subject* (≤255), level* (0..2), message* (≤10000)}` | 201 `Ticket` (409 while the user has another **open** ticket, Xboard rule) |
| GET | `/user/tickets/:id` | — | `Ticket` with messages (404 for other users' tickets) |
| POST | `/user/tickets/:id/reply` | `{message*}` | `Ticket` (400 when closed; → `reply_status=0`) |
| POST | `/user/tickets/:id/close` | — | `true` (idempotent) |
| GET | `/user/notices` | `page`,`page_size` | `[{id,title,content,img_url,tags,created_at}]` (show=true, `sort` ASC then newest first) |
| GET | `/user/knowledge` | `language?` (default `default_locale`), `keyword?` | `{"<category>":[{id,title,updated_at}]}` (show=true, `sort` ASC) |
| GET | `/user/knowledge/:id` | — | `{id,title,body,category,language,updated_at}` body with `{{siteName}} {{subscribeUrl}} {{urlEncodeSubscribeUrl}} {{safeBase64SubscribeUrl}}` replaced |
Knowledge rules: `language=zh-CN` matches exactly, a bare primary tag (`en`) also matches regional tags (`en-US`, imported
from Xboard). `keyword` is a case-insensitive substring of title or body (`%`/`_` literal). Users without a valid plan
(`plan_id` null or not `IsAvailable`: banned, expired or traffic used up) get every `<!--access start-->…<!--access end-->`
block replaced by `<div class="knowledge-no-access">…</div>` (text in the article's language); placeholders are replaced
after that, `{{urlEncodeSubscribeUrl}}` = query escaping, `{{safeBase64SubscribeUrl}}` = URL-safe base64 without padding.

### Security (2FA / passkeys)
| method | path | req | res |
|---|---|---|---|
| GET | `/user/security` | — | `{totp_enabled, totp_enabled_at, backup_codes_remaining, passkeys:[Passkey]}` (`totp_enabled_at` null when off) |
| POST | `/user/security/totp/setup` | — | `{secret, otpauth_url}` (pending secret kept 10 min; issuer = `mfa_issuer` or `app_name`, account = email; 409 when already enabled) |
| POST | `/user/security/totp/enable` | `{code*}` | `{backup_codes:[10 × "xxxxx-xxxxx"]}` (shown once; wrong code → 400 on `code`, the pending secret is discarded after 5; no pending setup → 400 on `code`; 409 when already enabled) |
| POST | `/user/security/totp/disable` | `{password*, code*}` | `true` (`code` = TOTP or backup code; deletes secret and backup codes; wrong password → 400 on `password` (counts towards the password lockout), wrong code → 400 on `code`; 409 when not enabled) |
| POST | `/user/security/backup-codes` | `{code*}` | `{backup_codes}` (regenerates all 10; `code` = TOTP or backup code; 409 when TOTP is off) |
| POST | `/user/security/passkeys/begin` | `{name?}` (max 64) | `{challenge_id, options}` (`options` = `PublicKeyCredentialCreationOptionsJSON`) |
| POST | `/user/security/passkeys/finish` | `{challenge_id*, credential*, name?}` (`credential` = `RegistrationResponseJSON`) | `Passkey` (bad/expired/foreign challenge → 400 on `challenge_id`; failed attestation → 400 on `credential`; already registered → 409) |
| PATCH | `/user/security/passkeys/:id` | `{name*}` (max 64) | `Passkey` (404 when not yours) |
| DELETE | `/user/security/passkeys/:id` | JSON body `{password*}` | `true` (wrong password → 400 on `password`; 404 when not yours) |

`Passkey` = `{"id":1,"name":"Laptop","created_at":"…","last_used_at":null,"backup_eligible":true,"transports":["internal","hybrid"]}`.
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
a passkey (`mfasvc.AdminGate`, mounted after `RequireStaff`/`RequireAdmin`); `/user/security` stays reachable to enrol.

## Client subscription 🔓 (token)
`GET|HEAD /api/v1/client/subscribe?token=<token>[&flag=clash][&types=vmess,vless][&filter=HK,JP][&emoji=0][&list=1][&profile=standard][&source=blackmatrix7][&cats=openai,telegram][&final=Proxy][&mirror=1]`
- Detects client from `flag` or `User-Agent` (see `docs/SUBSCRIPTION.md`). Headers: `subscription-userinfo`, `profile-update-interval`, `content-disposition`, `profile-web-page-url`, `ETag`.
- Unknown/malformed token → 403 (no detail). Banned/expired/out-of-traffic users → 200 with info-only nodes when `show_info_to_server_enable`, else a config without proxies. Invalid overrides → 400. Rate limit 120/h per token (burst 30) → 429.
- Also mounted at `GET|HEAD /s/:token` (short link) — same handler and rate-limit bucket; the token is redacted from access logs.

## Node APIs 🤖 — see `docs/NODE_PROTOCOLS.md`
- XrayR/V2bX: `GET /server/UniProxy/config`, `GET /server/UniProxy/user`, `POST /server/UniProxy/push`, `POST /server/UniProxy/alive`, `GET /server/UniProxy/alivelist` (query `token`, `node_id`, `node_type`).
- soga-v1 (header `API-KEY`, `NODE-ID`, `NODE-TYPE`): `GET /node`, `GET /users`, `GET /audit_rules`, `GET /white_list`, `POST /traffic`, `POST /alive_ip`, `POST /audit_log`, `POST /status`.
  Mounted under both `/api/v1/...` and `/soga/api/v1/...` (recommended `webapi_url=https://host/soga/`).

## Admin 🛡 (`/admin`)
### Stats
| GET | `/admin/stats/overview` | — | `{today:{income,paid_orders,registers,commission}, month:{income,paid_orders,registers}, totals:{users,active_users,online_users,pending_tickets,pending_orders,pending_commission}, nodes:{total,online}}` |
| GET | `/admin/stats/series?metric=income|orders|registers|traffic&range=7d|30d|90d` | — | `[{date, value}]` |
| GET | `/admin/stats/nodes/ranking?date=YYYY-MM-DD` | — | `[{node_id,name,u,d,total}]` |
| GET | `/admin/stats/users/ranking?date=&limit=` | — | `[{user_id,email,u,d,total}]` |

Stats rules (`internal/service/stat`): days are calendar days in the server time zone. "Paid" orders are `status ∈ {1 processing,
3 completed, 4 discounted}` bucketed by `paid_at` (a discounted order was paid and later credited to an upgrade, so it stays
revenue); `income` = Σ `total_amount`, `today.commission` = Σ `commission_balance` of today's paid orders. `active_users` = not
banned, not expired and `u + d < transfer_enable`; `online_users` = users with an alive IP; `pending_tickets` = open and awaiting
staff; `pending_orders` = status 0; `pending_commission` = paid orders with an inviter, `commission_balance > 0` and
`commission_status = 0`; `nodes.online` counts nodes in state ≥ 1 (a child reports its parent, docs/NODE_PROTOCOLS.md §5.1).
`series` returns `range` points (default `30d`), oldest first, today last; past days read the `stats` row written by `stats_daily`
(`orders` = `paid_count`) and fall back to live queries, today is always live, `traffic` = Σ `stat_nodes` u+d. Rankings default to
today, `limit` 1..500 (default 20); deleted nodes/users keep their rows with an empty `name`/`email`.
### Users
| GET | `/admin/users` | filters: `email`,`plan_id`,`group_id`,`banned`,`is_admin`,`is_staff`,`invite_user_id`,`expired` (`active|expired|never`), `id`, `uuid`, `token`, `remarks` | `[User(admin view: + remarks, invite_user_id, last_login_ip, telegram_id, password never)]` |
| POST | `/admin/users` | `{email*, password*, plan_id?, group_id?, transfer_enable(GB)?, expired_at?, is_admin?, is_staff?, speed_limit?, device_limit?, balance?, remarks?}` | `User` |
| GET | `/admin/users/:id` | — | `User` + `stats:{orders_total, commission_total}` |
| PATCH | `/admin/users/:id` | any editable field incl. `password`, `banned`, `u`, `d`, `invite_user_id` | `User` |
| DELETE | `/admin/users/:id` | — | `true` (cascades orders/tickets/stat_users) |
| POST | `/admin/users/:id/reset-secret` | — | `User` (new uuid/token) |
| POST | `/admin/users/:id/login-token` | — | `{url}` one-time login URL |
| POST | `/admin/users/generate` | `{email_prefix?, email_suffix*, count* (1..500), plan_id?, expired_at?, password?}` | `{count, csv}` |
| POST | `/admin/users/export` | same filters as list | `text/csv` |
| POST | `/admin/users/mail` | `{filters…, subject*, content*}` | `{queued}` |
| POST | `/admin/users/:id/traffic-reset` | — | `User` |

Users rules (`internal/service/adminuser`): only `GET /admin/users` and `GET /admin/users/:id` are open to staff, and staff get `uuid`
and `token` blanked. Admin view = self view + `token`, `commission_type`, `remarks`, `invite_user_id`, `invite_user_email`,
`last_login_ip`, `telegram_id`, `updated_at`. Filters: `email`/`remarks` are case-insensitive substrings; `plan_id`, `group_id`,
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
  as inviter, rule_profiles; users and orders that reference the user as inviter get `invite_user_id = null`.
- `login-token` → `{url: app_url + "/login?token=<one-time, 5 min>"}`; the SPA posts the token to `/passport/login/token`.
- `generate`: emails are `<prefix><8 random [a-z0-9]>@<suffix>` (exactly `<prefix>@<suffix>` when `count` is 1 and a prefix is
  given). The whole batch shares `password`, or one random 12-character password when omitted (one argon2id hash per batch);
  `csv` = `email,password` rows. Taken single email → 409.
- `export`: filters in the JSON body; `text/csv` attachment `email,plan,expired_at,transfer_enable,u,d,balance,created_at`
  (RFC3339 UTC, bytes, cents; cells starting with `= + - @` are prefixed with `'`).
- `mail`: plain-text `content` (blank lines = paragraphs) through the `notify` template to every matched user, ignoring `remind_*`;
  recipients that do not fit the mail queue are queued in the background. `{queued}` = number of recipients.
### Plans / groups / nodes / routes
| GET,POST | `/admin/plans` | Plan fields (`group_id*`, `name*`; defaults show=false, sell=true, renew=true) | `[Plan + user_count]` (active holders) / 201 `Plan` |
| GET,PATCH,DELETE | `/admin/plans/:id` | PATCH: any Plan field (null clears nullable limits) + `force_update` (push group/traffic/limits to holders) | `Plan + user_count`; DELETE 409 while users hold the plan or it has unpaid orders |
| POST | `/admin/plans/sort` | `{ids:[...]}` | `true` |
| GET,POST | `/admin/groups` | `{name}` | `[{id,name,user_count,plan_count,node_count,created_at,updated_at}]` |
| PATCH,DELETE | `/admin/groups/:id` | `{name}` | group with counts / `true` (409 while users, plans or nodes reference it) |
| GET,POST | `/admin/nodes` | Node fields (`port` may be a number or a string `"443"`, `"20000-30000"`, `"443,8443"`) | `[Node]` (with status) / `Node` |
| GET,PATCH,DELETE | `/admin/nodes/:id` | PATCH: any subset of Node fields (`parent_id: null` detaches; `protocol_settings` is replaced, not merged) | `Node` / `true` (DELETE 409 while the node has children; stats are kept) |
| POST | `/admin/nodes/:id/copy` | — | `Node` (`name + " (copy)"`, `show=false`) |
| POST | `/admin/nodes/sort` | `{ids}` | `true` (sort = position in `ids`) |
| GET | `/admin/nodes/:id/stats?range=7d` (or `30d`) | — | `[{date,u,d}]` one point per day, oldest first, zero-filled (raw bytes of P) |
| GET | `/admin/nodes/:id/online` | — | `[{user_id,email,ips[]}]` (alive on P) |
| GET | `/admin/nodes/:id/audit` | — | `[{at,node_id,user_id,audit_id,route_id,rule}]` soga audit hits of P, newest first (in memory, last 1000 overall) |
| GET,POST | `/admin/routes` | `{remarks, match[] (1..999), action: "block" or "dns", action_value}` | `[Route]` / `Route` (block: `match` joined by a pipe must compile as a Go regexp; dns: `action_value` required unless `match[0]=="main"`) |
| PATCH,DELETE | `/admin/routes/:id` | | `Route` / `true` (DELETE 409 while a node references it) |
### Orders / coupons / payments
| GET | `/admin/orders` | `trade_no`,`user_id`,`email`,`status`,`commission_status`,`plan_id`,`payment_id`,`invite_user_id`,`type`,`date_from`,`date_to` (YYYY-MM-DD, `date_to` inclusive, or RFC 3339) | `[AdminOrder]` = Order + `user{id,email}`, `invite_user_id`, `coupon_id`, `commission_balance`, `surplus_order_ids` |
| GET | `/admin/orders/:id` | — | `AdminOrder` + `commission_logs`, `surplus_orders` |
| POST | `/admin/orders` | `{email* or user_id*, plan_id*, period*, total_amount?}` | 201 `AdminOrder` (assigned, pending; no discount/coupon/surplus/balance; 409 if the user has an unpaid order) |
| POST | `/admin/orders/:id/paid` | — | `AdminOrder` (pending only; `callback_no="manual"` → fulfil) |
| POST | `/admin/orders/:id/cancel` | — | `AdminOrder` (pending only) |
| PATCH | `/admin/orders/:id` | `{commission_status}` (0, 1 or 3) | `AdminOrder` (2 is set by `commission_settle`; a settled commission → 409) |
| GET,POST | `/admin/coupons` | list: `code?`,`name?`; create: `{name*, code?, type*, value*, count? (>1 = batch of generated 8-char codes), limit_use? (remaining uses), limit_use_with_user?, limit_plan_ids?, limit_periods?, started_at*, ended_at*, show? (default true)}` | `[Coupon + used_count]` / 201 `[Coupon + used_count]` (409 duplicate code) |
| GET,PATCH,DELETE | `/admin/coupons/:id` | PATCH: any create field except `count` (null clears limits) | `Coupon + used_count` |
| GET | `/admin/payments` | — | `[Payment(config included) + gateway_label + notify_url]` by `sort` |
| GET | `/admin/payments/gateways` | — | `[{name, label, fields:[{key,label,description,type:"text|password|select|number|textarea",options?,default?,required}]}]` |
| POST | `/admin/payments` | `{gateway*, name*, icon?, config*, notify_domain?, handling_fee_fixed?, handling_fee_percent?, enabled? (default false)}` | 201 `Payment` + `gateway_label` + `notify_url` (uuid generated; required fields → `errors["config.<key>"]`; gateway rules → `errors.config`) |
| GET,PATCH,DELETE | `/admin/payments/:id` | PATCH: any create field (config replaced as a whole) | `Payment` + `notify_url` |
| POST | `/admin/payments/sort` | `{ids}` | `true` |
### Support & content 🧑‍💼
| GET | `/admin/tickets` | `status?`,`reply_status?`,`user_id?`,`email?`,`page`,`page_size`,`sort` (`id created_at updated_at level status reply_status`) | `[Ticket + user]` (default latest activity first) |
| GET | `/admin/tickets/:id` | — | `Ticket` with messages + user |
| POST | `/admin/tickets/:id/reply` | `{message*}` | `Ticket` (reopens a closed ticket, `reply_status=1`; e-mails the owner `ticket_reply`, at most once per ticket per 30 min — an owner reply re-arms it) |
| POST | `/admin/tickets/:id/close` | — | `true` |
| GET | `/admin/notices` | — | `[Notice]` all (hidden included), display order, unpaginated (`total` = count) |
| POST | `/admin/notices` | `{title*, content*, img_url? (http(s) or ""), tags? (≤20 × ≤32 chars), show? (default true), sort?}` | 201 `Notice` |
| PATCH,DELETE | `/admin/notices/:id` | any POST field | `Notice` / `true` |
| POST | `/admin/notices/sort` | `{ids}` (unique, all existing) | `true` (`sort` = position, 1-based) |
| GET | `/admin/knowledge` | `language?`,`category?`,`keyword?` | `[{id,language,category,title,show,sort,created_at,updated_at}]` (no body), display order, unpaginated |
| POST | `/admin/knowledge` | `{language* (≤5), category*, title*, body*, show? (default true), sort?}` | 201 `Knowledge` |
| GET,PATCH,DELETE | `/admin/knowledge/:id` | any POST field | `Knowledge` (raw body) / `Knowledge` / `true` |
| POST | `/admin/knowledge/sort` | `{ids}` | `true` |
### Settings & system
| GET | `/admin/settings` | — | `{"<group>": {"<key>": value}, …, "groups": [{name, keys:[{key,group,type,default,secret?,options?,min?,max?}]}]}` all groups (secrets included) + UI metadata |
| PUT | `/admin/settings` | `{"<key>": value, ...}` (`null` resets a key) | `true`; unknown keys, invalid values and hidden keys (`rulesets_custom`, use `/admin/rulesets`) → 400 per key, nothing saved |
| POST | `/admin/settings/mail/test` | `{email*}` | `true`; sent synchronously with the current mail settings, delivery error → 400 with the driver's message |
| GET | `/admin/rulesets` | — | `{builtin: catalog, custom: {...}}` |
| PUT | `/admin/rulesets` | `{custom:{sources?,categories?,templates?}}` (merged over builtin) | `true` |
| GET | `/admin/system/status` | — | `{version, go_version, uptime (s), db_driver, started_at, scheduler:[{name,spec,running,last_run,last_duration,last_error,next_run,runs,failures}], mail:{pending,sent,failed,driver}, mail_queue (= mail.pending), cache:{items,max_items,hits,misses,hit_ratio}, online:{nodes,users}, goroutines, memory:{alloc,sys}}` |
| GET | `/admin/system/logs?level=&limit=` | — | `[{time,level,msg,attrs}]` newest first (in-memory ring buffer, last 1000); `level` debug (default) / info / warn / error = minimum level; `limit` 1..1000 (default 200) |

| GET | `/admin/settings` | — | `{"<group>": {"<key>": value}}` all groups |
| PUT | `/admin/settings` | `{"<key>": value, ...}` | `true` |
| POST | `/admin/settings/mail/test` | `{email*}` | `true` |
| GET | `/admin/rulesets` | — | `{builtin: catalog, custom: {...}, effective: merged catalog}` |
| PUT | `/admin/rulesets` | `{custom:{sources?,categories?,templates?,mirrors?} \| null}` (merged over builtin by id; validated; `null` resets) | `true` |
| GET | `/admin/system/status` | — | `{version, go_version, uptime, db_driver, started_at, scheduler:[{name,spec,last_run,last_error}], mail_queue, cache}` |
| GET | `/admin/system/logs?level=&limit=` | — | `[{time,level,msg,attrs}]` (in-memory ring buffer, last 1000) |

### Settings keys (group → key: type = default)
- **site**: `app_name`:s="GG Bond", `app_description`:s, `app_url`:s, `logo`:s, `tos_url`:s, `currency`:s="CNY", `currency_symbol`:s="¥", `default_locale`:s="zh-CN", `show_info_to_user`:b=true
- **register**: `stop_register`:b=false, `email_verify`:b=false, `email_whitelist_enable`:b=false, `email_whitelist_suffix`:[]s, `invite_force`:b=false, `register_limit_by_ip_enable`:b=false, `register_limit_count`:i=3, `register_limit_expire`:i=60(min), `password_limit_enable`:b=true, `password_limit_count`:i=5, `password_limit_expire`:i=60, `try_out_plan_id`:i=0, `try_out_hour`:i=0, `captcha_provider`:s="none", `captcha_site_key`:s, `captcha_secret`:s(secret)
- **subscribe**: `subscribe_url`:[]s (alternatives; empty → app_url), `plan_change_enable`:b=true, `reset_traffic_method`:i=0, `surplus_enable`:b=true, `show_info_to_server_enable`:b=true, `show_protocol_to_server_enable`:b=false, `default_remind_expire`:b=true, `default_remind_traffic`:b=true, `ruleset_default_template`:s="standard", `ruleset_default_source`:s="blackmatrix7", `ruleset_mirror`:s="" (e.g. `https://ghproxy.net/`), `subscribe_update_interval`:i=24(h), `allow_user_rule_profile`:b=true
- **order**: `order_timeout_minutes`:i=30, `new_order_event_id`… (not implemented; omit)
- **invite**: `invite_commission`:i=10(%), `invite_gen_limit`:i=5, `invite_never_expire`:b=true, `commission_first_time_enable`:b=true, `commission_auto_check_enable`:b=true, `commission_withdraw_limit`:i=100(unit currency), `commission_withdraw_method`:[]s=["USDT","Alipay"], `withdraw_close_enable`:b=false, `commission_distribution_enable`:b=false, `commission_distribution_l1`:i, `_l2`, `_l3`
- **server**: `server_token`:s(secret, auto-generated 32 chars on first boot), `server_pull_interval`:i=60, `server_push_interval`:i=60, `server_white_list`:[]s (soga white_list), `device_limit_mode`:i=0 (0 strict,1 relaxed)
- **email**: `email_driver`:s="log"|"smtp", `email_host`, `email_port`:i=465, `email_encryption`:s="ssl"|"tls"|"none", `email_username`, `email_password`(secret), `email_from_address`, `email_from_name`, `email_template`:s="default", `remind_mail_enable`:b=true
- **frontend**: `frontend_accent`:s="#007AFF", `frontend_background_url`:s, `frontend_customer_service_url`:s, `frontend_footer_html`:s
- **app**: `windows_download_url`, `windows_version`, `macos_download_url`, `macos_version`, `android_download_url`, `android_version`, `ios_download_url`, `ios_version`
- **security**: `passkey_rp_id`:s="" (empty → host of `app_url`), `passkey_origins`:[]s (extra allowed origins; the `app_url` origin is always allowed), `admin_mfa_required`:b=false, `mfa_issuer`:s="" (empty → `app_name`)
Secrets are returned to admin only and masked in logs.
