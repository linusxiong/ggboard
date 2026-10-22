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
      "protocol_settings":{...},"status":{"online":3,"last_check_at":"...","last_push_at":"...","load":{...}|null},"created_at","updated_at"}
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
| GET | `/guest/config` | — | `{app_name, app_description, app_url, logo, tos_url, currency, currency_symbol, register_enabled, email_verify, email_whitelist_suffix[], invite_force, captcha:{provider:"none|turnstile|recaptcha|hcaptcha", site_key}, downloads:{windows:{url,version},macos:{...},android:{...},ios:{...}}, locales:["zh-CN","en"]}` |
| GET | `/guest/plans` | — | `[Plan]` where show=true & sell=true (prices only; no capacity leak) |
| ANY | `/guest/payment/notify/:gateway/:uuid` | gateway-specific | gateway-specific body (plugin returns it) |
| GET | `/guest/payment/return/:trade_no` | — | 302 → `/orders/:trade_no` |
| GET | `/guest/rulesets/catalog` | — | catalog (see `docs/SUBSCRIPTION.md`) |

## Passport 🔓 (`/passport`) — rate limited
| method | path | req | res |
|---|---|---|---|
| POST | `/passport/register` | `{email*, password*(min 8), invite_code?, email_code?, captcha?}` | `{token, user}` |
| POST | `/passport/login` | `{email*, password*, captcha?}` | `{token, user}` |
| POST | `/passport/forgot` | `{email*, email_code*, password*}` | `true` |
| POST | `/passport/email/send-code` | `{email*, captcha?}` | `true` (60s cooldown per email, 10/h per IP) |
| POST | `/passport/login/token` | `{token*}` one-time login token (from admin "login as") | `{token, user}` |
Rules: `stop_register` → 403; `email_whitelist_enable`; `invite_force` requires valid invite code; `try_out_plan_id` grants trial on register;
password limit lockout (`password_limit_*`); register-by-ip limit.

## User 🔑 (`/user`)
### Account
| method | path | req | res |
|---|---|---|---|
| GET | `/user/me` | — | `User` |
| PATCH | `/user/me` | `{remind_expire?, remind_traffic?}` | `User` |
| POST | `/user/password` | `{old_password*, new_password*}` | `true` (bumps password_version) |
| GET | `/user/stats` | — | `{pending_orders, open_tickets, notices_count, invite_count, commission_pending}` |
| GET | `/user/subscription` | — | `{subscribe_url, token, plan, transfer_enable, u, d, expired_at, next_reset_at, reset_day, device_limit, speed_limit}` |
| POST | `/user/subscription/reset` | — | `{subscribe_url, token}` (new uuid + token) |
| GET | `/user/subscription/preview?client=clash&...` | same query as client subscribe | `text/plain` body of generated config |
| GET | `/user/traffic-logs` | `page`, `page_size` | `[{record_at, node_id, node_name, rate, u, d}]` |
| GET | `/user/nodes` | — | `[{id,name,type,tags,rate,host,port,is_online,sort}]` (host/port only when `show_info_to_user`=true; default true) |
| POST | `/user/logout` | — | `true` (client discards; server no-op) |
### Rule profile
| GET | `/user/rule-profile` | — | `RuleProfile` (defaults if none) |
| PUT | `/user/rule-profile` | `RuleProfile` | `RuleProfile` |
| GET | `/user/rulesets/catalog` | — | same as guest catalog |
### Shop
| GET | `/user/plans` | — | `[Plan]` (show=true; includes current plan even if hidden, with `sell/renew` flags) |
| GET | `/user/plans/:id` | — | `Plan` |
| GET | `/user/payment-methods` | — | `[{id,name,icon,handling_fee_fixed,handling_fee_percent}]` |
| POST | `/user/coupons/check` | `{code*, plan_id*, period*}` | `{id,code,name,type,value}` or 400 |
| GET | `/user/orders` | `page`,`status?` | `[Order]` |
| POST | `/user/orders` | `{plan_id*, period*, coupon_code?}` | `{trade_no}` (409 if pending order exists) |
| GET | `/user/orders/:trade_no` | — | `Order` |
| POST | `/user/orders/:trade_no/checkout` | `{payment_id?}` (omit when total 0 / balance covers) | `{type:"url"|"qrcode"|"paid", data:"..."}` |
| GET | `/user/orders/:trade_no/status` | — | `{status}` (poll while paying) |
| POST | `/user/orders/:trade_no/cancel` | — | `true` |
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

## Client subscription 🔓 (token)
`GET /api/v1/client/subscribe?token=<token>[&flag=clash][&types=vmess,vless][&filter=HK,JP][&profile=standard][&source=blackmatrix7][&cats=openai,telegram][&mirror=1]`
- Detects client from `flag` or `User-Agent` (see `docs/SUBSCRIPTION.md`). Headers: `subscription-userinfo`, `profile-update-interval`, `content-disposition`, `profile-web-page-url`, `ETag`.
- Invalid/banned/expired-with-no-traffic → still 200 with info-only nodes when `show_info_to_server_enable`, else empty.
- Also mounted at `GET /s/:token` (short link) — same handler.

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
### Plans / groups / nodes / routes
| GET,POST | `/admin/plans` | Plan fields | `[Plan + user_count]` / `Plan` |
| GET,PATCH,DELETE | `/admin/plans/:id` | | |
| POST | `/admin/plans/sort` | `{ids:[...]}` | `true` |
| GET,POST | `/admin/groups` | `{name}` | `[{id,name,user_count,plan_count,node_count}]` |
| PATCH,DELETE | `/admin/groups/:id` | | |
| GET,POST | `/admin/nodes` | Node fields | `[Node]` (with status) |
| GET,PATCH,DELETE | `/admin/nodes/:id` | | |
| POST | `/admin/nodes/:id/copy` | — | `Node` |
| POST | `/admin/nodes/sort` | `{ids}` | `true` |
| GET | `/admin/nodes/:id/stats?range=7d` | — | `[{date,u,d}]` |
| GET | `/admin/nodes/:id/online` | — | `[{user_id,email,ips[]}]` |
| GET,POST | `/admin/routes` | `{remarks, match[], action, action_value}` | |
| PATCH,DELETE | `/admin/routes/:id` | | |
### Orders / coupons / payments
| GET | `/admin/orders` | `trade_no`,`user_id`,`email`,`status`,`commission_status`,`plan_id`,`payment_id`,`date_from`,`date_to` | `[Order + user.email]` |
| GET | `/admin/orders/:id` | — | `Order` |
| POST | `/admin/orders` | `{email* or user_id*, plan_id*, period*, total_amount?}` | `Order` (assigned, pending) |
| POST | `/admin/orders/:id/paid` | — | `Order` (manual confirm → fulfil) |
| POST | `/admin/orders/:id/cancel` | — | `Order` |
| PATCH | `/admin/orders/:id` | `{commission_status}` | `Order` |
| GET,POST | `/admin/coupons` | create: `{name*, code?, type*, value*, count? (batch), limit_use?, limit_use_with_user?, limit_plan_ids?, limit_periods?, started_at*, ended_at*, show?}` | `[Coupon + used_count]` / `[Coupon]` |
| PATCH,DELETE | `/admin/coupons/:id` | | |
| GET | `/admin/payments` | — | `[Payment(config included)]` |
| GET | `/admin/payments/gateways` | — | `[{name, label, fields:[{key,label,description,type:"text|password|select|number|textarea",options?,default?,required}]}]` |
| POST | `/admin/payments` | `{gateway*, name*, icon?, config*, notify_domain?, handling_fee_fixed?, handling_fee_percent?, enabled?}` | `Payment` + `notify_url` |
| PATCH,DELETE | `/admin/payments/:id` | | |
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
| GET | `/admin/settings` | — | `{"<group>": {"<key>": value}}` all groups |
| PUT | `/admin/settings` | `{"<key>": value, ...}` | `true` |
| POST | `/admin/settings/mail/test` | `{email*}` | `true` |
| GET | `/admin/rulesets` | — | `{builtin: catalog, custom: {...}}` |
| PUT | `/admin/rulesets` | `{custom:{sources?,categories?,templates?}}` (merged over builtin) | `true` |
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
Secrets are returned to admin only and masked in logs.
