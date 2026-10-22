# Architecture

One Go binary (`ggboard`) serves:

- the JSON API
- the node-facing APIs (UniProxy for XrayR/V2bX, soga-v1)
- subscriptions and payment callbacks
- a cron scheduler, an in-process mail queue and an in-process notification queue (Telegram bot)
- the embedded React SPA

There is no Redis and no separate worker. A panel needs nothing more than an in-process cache (ristretto), a TTL map and
batched DB writes.

```text
┌──────────────┐   HTTPS    ┌──────────────────────────────────────────────────────────┐
│  React SPA   │ ─────────▶ │ gin engine (internal/router)                             │
│  (embedded)  │            │  /api/v1/guest  /passport  /user  /admin                 │
└──────────────┘            │  /api/v1/client/subscribe · /s/:token   (token)          │
┌──────────────┐            │  /api/v1/server/UniProxy/*              (XrayR / V2bX)   │
│ XrayR / V2bX │ ─────────▶ │  /api/v1/{node,users,…} · /soga/api/v1/* (soga-v1)       │
│ soga         │            │  /api/v1/guest/payment/notify/:gateway/:uuid             │
└──────────────┘            │  /.well-known/{change-password,passkey-endpoints}        │
                            │  /healthz · SPA fallback (internal/static)               │
                            └───────────────┬──────────────────────────────────────────┘
                                            │ internal/service/* · subscription · payment
                                            ▼
              ┌───────────┐  ┌──────────────┐  ┌───────────────┐  ┌──────────────┐  ┌────────────┐
              │ GORM DB   │  │ ristretto    │  │ app.KV        │  │ scheduler    │  │ mail queue │
              │ sqlite/pg/│  │ cache + ETag │  │ app.Online    │  │ (robfig/cron)│  │ smtp / log │
              │ mysql     │  │              │  │ app.Traffic   │  │              │  │            │
              └───────────┘  └──────────────┘  └───────────────┘  └──────────────┘  └────────────┘
```

## Boot sequence (`ggboard serve`)

1. `cli.Execute` builds the cobra tree (`internal/cli`). Its `PersistentPreRunE` runs `config.Load` and `app.Init`
   for every command except `version`, `help` and `completion`.
2. `config.Load` reads values with precedence flags > env `GGBOARD_*` > config file > defaults. The config file is
   `--config`/`-c`, else `$GGBOARD_CONFIG`, else `./config.yaml` when present.
   - Keys: `server.addr`, `server.trusted_proxies[]`, `server.cors_origins[]`, `database.driver`
     (sqlite\|postgres\|mysql), `database.dsn`, `jwt.secret`, `jwt.ttl` (default 168h), `log.level`, `log.format`,
     `frontend.dir`, `data_dir` (default `./data`, created with mode 0700).
   - The SQLite DSN defaults to `<data_dir>/ggboard.db`.
   - An empty `jwt.secret` is generated once into `<data_dir>/jwt.secret` (0600). A secret shorter than 32 characters
     logs a warning.
3. `app.Init` builds the dependency container `*app.App{Cfg, DB, Settings, Cache, KV, Mailer, Log, Logs, Version,
   StartedAt, Online, Traffic, Scheduler}` in this order:
   1. logger (`logx`, with a ring buffer for `/admin/system/logs`)
   2. `db.Open`:
      - SQLite: WAL, `busy_timeout(5000)`, `foreign_keys(1)`, `synchronous(NORMAL)`, `_txlock=immediate`, 8 connections,
        UTC time parameters
      - PostgreSQL/MySQL: 32 open / 8 idle connections, 30 min lifetime. MySQL DSNs get `parseTime`, `loc=UTC` and `utf8mb4`
   3. `db.Migrate` (auto-migrate `model.All()`; idempotent, runs on every start)
   4. `settings.New` (generates `server_token` on first boot)
   5. ristretto cache, `KV` (`cache.TTLMap`), `Online` store, `Traffic` accumulator, mail queue
   6. scheduler (`time.Local`) with every job (see below)
4. `router.New(app)` (`internal/router`):
   - `httpx.NewEngine` sets the middleware chain below and `server.trusted_proxies` (an invalid list falls back to
     trusting none).
   - `GET /healthz` returns `{"data":{"status":"ok","version","uptime"}}`.
   - The `Register` of `guest`, `passport`, `user`, `admin`, `client` and `node` runs in that order. Each mounts its own
     paths under `/api/v1`; `client` also mounts `/s/:token`, `node` mounts `/soga/api/v1`, and `passport` mounts `/.well-known/*`.
   - `static.Mount` serves the SPA as the NoRoute handler.

   The router lives outside `httpx` because every API package imports `httpx`.
5. `app.Start()` starts the mail worker and the scheduler. The HTTP server timeouts are: read header 10 s, read 60 s,
   write 120 s, idle 120 s.
6. SIGINT/SIGTERM drains HTTP for 10 s. `app.Close` then stops the scheduler, stops the App components (newest first:
   the notification queue drains, 10 s budget), flushes traffic, drains the mail queue (10 s budget), closes the cache and
   the DB.

## CLI

| command | what |
|---|---|
| `ggboard serve [--addr :8080]` | HTTP server, scheduler and mail queue |
| `ggboard migrate` | create or update the schema (the App migrates on init), then exit |
| `ggboard admin create --email E [--password P] [--staff]` | create an administrator (or staff). A password is generated and printed when omitted |
| `ggboard admin reset-password --email E [--password P]` | set a password (signs out existing sessions) |
| `ggboard admin promote --email E` | grant `is_admin` to an existing user |
| `ggboard import xboard --dsn … [--driver] [--dry-run] [--force] [--with-stats] [--stats-days N] [--source-tz TZ]` | Xboard import, see docs/IMPORT_XBOARD.md |
| `ggboard version` | print the version (no config or DB needed) |

Global flags: `-c/--config`, `--data-dir`, `--log-level`, `--log-format`, `--db-driver`, `--db-dsn`.

## HTTP conventions (`internal/httpx`)

- Success is `200 {"data": …}` and creates are `201 {"data": …}`. Lists are `{"data": [...], "total": N}`.
  An empty success is `{"data": true}`.
- Errors are status + `{"code", "message", "errors": {"field": ["msg"]}}`.
  - Standard codes: `validation_failed` (400), `unauthorized` (401), `payment_failed` (402), `forbidden` (403),
    `not_found` (404), `conflict` (409), `payload_too_large` (413), `rate_limited` (429), `internal` (500).
  - MFA adds `mfa_invalid` (401), `mfa_expired` (401) and `mfa_setup_required` (403).
  - A 429 carries `Retry-After`.
- Body limit: 1 MiB. Node push routes raise it to 8 MiB (`httpx.LargeBody(httpx.NodeBodyLimit)`).
- Auth: `Authorization: Bearer <JWT>` (HS256). Claims are `{sub: user id, email, ver: password_version, iat, exp, iss: "ggboard"}`.
  - A `ver` mismatch → 401 (password change / security reset signs every session out). A banned user → 403.
  - The user row is cached for 30 s.
- Roles: `/admin` is mounted with `RequireStaff` + the MFA admin gate. Every area except tickets, notices, knowledge and
  reading users adds `RequireAdmin`.
- Pagination query: `page` (1-based), `page_size` (default 20, max 200), `sort` (`field` or `-field`); filters are
  query params.
- Node APIs authenticate with the `server_token` setting, compared in constant time. An empty token rejects everything.
  UniProxy sends the `token` query parameter; soga sends the `API-KEY` header.

Middleware chain: request id (`X-Request-ID`, reused when sane) → access log → security headers → CORS (only when
`server.cors_origins` is set) → gzip → recovery → body limit.

- The **access log** records the route pattern (`c.FullPath()`, e.g. `/s/:token`) and never the query string, so tokens
  in paths or queries never reach the access log. An unmatched request (404) logs its literal path. Known gap: the
  `rate limited` (429), 5xx and panic log lines still log `c.Request.URL.Path`, so a throttled `/s/:token` leaks the token.
- **gzip** compresses compressible responses of at least 1 KiB. It skips HEAD requests, upgrades and responses that
  already set `Content-Encoding`.
- `node.Register` sets `RemoveExtraSlash`, so soga's `/soga//api/v1/node` still matches.

## Caching

| what | where | TTL / invalidation |
|---|---|---|
| settings | `settings.Service` snapshot | swapped after every successful `Set`; reads never hit the DB |
| session user | `auth.LoadUser` | 30 s; `auth.InvalidateUser` after password/ban/role/uuid changes |
| available users per group set | `usersvc.AvailableUsersForGroups` | 60 s; `InvalidateNodeUsers()` |
| node config / user list / audit rules bodies, resolved nodes | `nodesvc` namespace | 60 s; `nodesvc.Invalidate()` after node, route or group writes |
| subscription node list | `subscription.AvailableNodes` | 5 s per group; `subscription.InvalidateNodes(a)` |
| stored rule profile | `engine.LoadStoredProfile` | 1 min; dropped on save |
| MFA "has a second factor" | `mfasvc` (admin gate) | 30 s |

Node responses (`config`, `user`, soga `node`/`users`/`audit_rules`) carry `ETag: "<sha1 hex of the body>"`. A request
whose `If-None-Match` contains the tag gets 304 (substring compare, so quoted, bare and `W/` forms all match).
Subscriptions are generated per request and also carry a sha1 `ETag` with 304 support.

## Runtime node state

`app.Online` keeps, per effective node, `last_check_at` (any authenticated node request), `last_push_at` (push/traffic),
online users, load and the backend's User-Agent as `version`. Status uses the window `W = 2 × max(push, pull interval)`
and has three states (docs/NODE_PROTOCOLS.md §5.1):

- **2 online**: push within W
- **1 online, no push** (`StateIdle`): only a check within W
- **0 offline**

`is_online` means status ≥ 1. Alive IPs per user expire after `max(300 s, 2 × server_push_interval)` and are counted by
`device_limit_mode`. Everything is in memory, so after a restart nodes show offline until their next poll.

Traffic reports go to `app.Traffic`, aggregated per (node, user). They are flushed every 10 s and on shutdown, in one
transaction: `users.u/d += bytes × rate`, plus daily upserts of `stat_users` (× rate) and `stat_nodes` (raw).

## Scheduler jobs

Jobs run only in `ggboard serve`. Specs use the 5-field cron format in the server's local time zone. The scheduler
recovers panics, skips a run while the previous one is still running, and reports status in `/admin/system/status`.

| job | spec | registered in | what |
|---|---|---|---|
| `online_cleanup` | `*/5 * * * *` | `internal/app` | purge expired alive IPs / node entries and `app.KV` |
| `traffic_flush` | `@every 10s` | `internal/app` | flush the traffic accumulator |
| `order_timeout` | `*/1 * * * *` | `service/order` | cancel pending orders older than `order_timeout_minutes` (default 30). Balance and coupon use are given back. At most 500 per run |
| `commission_settle` | `0 */1 * * *` | `service/order` | see below |
| `traffic_reset` | `0 0 * * *` | `service/maintenance` | see below |
| `expire_remind` | `0 10 * * *` | `service/maintenance` | remind users (not banned, `remind_expire`) whose `expired_at` is in (now, now + 24 h]: a mail with `remind_mail_enable`, a Telegram message when bound (`telegram_user_notify`). At most once per 24 h per user |
| `traffic_remind` | `0 10 * * *` | `service/maintenance` | remind users (not banned, not expired, `remind_traffic`) who used 80 % ≤ x < 100 %, by mail and Telegram as above. At most once per 24 h per user |
| `stats_daily` | `5 0 * * *` | `service/maintenance` | upsert yesterday's `stats` row (docs/DATA_MODEL.md) |
| `subscription_access_flush` | `@every 10s` | `subscription/guard` | write buffered subscription access rows, then run leak detection (docs/SUBSCRIPTION.md) |
| `subscription_access_prune` | `37 4 * * *` | `subscription/guard` | delete access rows older than 30 days; keep the newest 100 per user |

`commission_settle`:

1. With `commission_auto_check_enable`, completed orders whose commission has been pending for 72 h become 1 (approved).
2. Every approved order is paid out to the invite chain (l1/l2/l3 shares when `commission_distribution_enable` is on).
   The money goes to `commission_balance`, or to `balance` with `withdraw_close_enable`.
3. Each payout is logged in `commission_logs` and the order becomes 2 (valid).

`traffic_reset` handles users with `next_reset_at <= now`, 500 per transaction:

- If the plan still resets (plan present, effective method ≠ never, `expired_at` set), it sets u = d = 0 and computes
  the next `next_reset_at`.
- Otherwise it only clears `next_reset_at`.
- It then invalidates the node user lists.

The mail queue is not a cron job. It is a worker goroutine with a bounded channel (1024) and 3 retries
(1 s, 2 s, 4 s). Requests never send mail inline; every outcome is written to `mail_logs`.

## Notifications (`internal/notify`)

Feature code describes what happened as a `notify.Event{Type, Title, Body, Fields, UserID, Admin, At}` and calls
`notify.Emit(a, ev)`, which never blocks:

```text
order / ticket / invite / passport / maintenance ──Emit──▶ bounded queue (1024) ──1 worker──▶ every configured Notifier
                                                     │ full → dropped, WARN, failed++        ├─ SendAdmin(ev)        (ev.Admin)
                                     EmitWait (bulk) ┘ waits for room                        └─ SendUser(chatRef, ev) (ev.UserID reachable)
```

- **Channels** register a `notify.Factory` from `init()` (`notify.Register`, a `plugin.Registry`). The factory is called per
  event and returns nil while the channel is not configured, so settings changes apply at once. Emit returns immediately
  when no channel is configured (no queue is created).
- **Events** are localized when built (`default_locale`, zh-CN or en; templates in `internal/notify/i18n.go`). `Title` and `Body`
  are plain text; `Fields` are `{key, value}` pairs whose labels come from `notify.Label`. Builders (`OrderNew`, `OrderPaid`,
  `OrderCancel`, `AdminLogin`, `TicketNew`, `TicketReply`, `Withdraw`, `SubscriptionLeak`, `RemindExpire`, `RemindTraffic`,
  `Test`) and the generic `notify.New(a, type, body, fields...)`. `notify.EmitSubscriptionLeak` matches `subscription.LeakHook`.
- **Delivery**: one attempt plus 3 retries per channel and side (administrators / user), waiting 1 s, 2 s, 4 s, or the
  channel's `retry_after` when longer (429, capped at 60 s). `notify.Permanent` errors (Telegram 4xx such as "chat not found")
  and `notify.ErrSkipped` (event not subscribed, no destination) end at once. A channel that fans out to several chats marks
  each served chat with `notify.MarkDone`, so a retry never sends twice. Failures are logged at WARN and counted.
- **Stats**: `notify.Stats(a)` → `/admin/system/status` `notify:{pending, sent, failed, drivers}` (outcomes count one per
  channel and side; skips count as neither).
- **Lifecycle**: the queue is an App component (`a.Component`), created on first use and stopped by `app.Close`, which drains
  it for up to 10 s; after that pending retries are abandoned and counted as failed.
- **Telegram** (`internal/notify/telegram`): active with `telegram_bot_token`. Administrator chats (`telegram_admin_chat_ids`)
  receive the types listed in `telegram_events`; users who bound their account (`users.telegram_id`) receive their own
  `order_paid`, `ticket_reply` and reminders while `telegram_user_notify` is on. Binding uses one-time codes in `app.KV`
  (10 min) and the `t.me/<bot>?start=<code>` deep link; the webhook answers bot commands in its HTTP response. `getMe` is
  cached 1 h in `a.Cache`, keyed by a hash of the token. See docs/API.md §Telegram.

## Security

- **Passwords**: argon2id (m = 64 MiB, t = 3, p = 4) for new hashes. Imported users can still sign in with bcrypt
  `$2y$`/`$2a$`/`$2b$` and Xboard `xb$…` hashes, which are upgraded transparently on login without bumping
  `password_version`.
- **Second factor**: TOTP (RFC 6238, ±1 step, replay guard via `totp_last_step`) with single-use backup codes, and
  passkeys (WebAuthn discoverable credentials, usernameless login).
  - With `admin_mfa_required`, admins and staff without a second factor get `403 mfa_setup_required` on `/admin/*`.
  - TOTP secrets are encrypted with a key derived from `jwt.secret`.
- **Rate limits**:
  - `/passport/*`: 30 requests/min per IP; `send-code`: 10 requests/h per IP.
  - Per email: a code cooldown and the password lockout (`password_limit_*`); registrations per IP (`register_limit_*`).
  - `/user/security/*` and order create / checkout / coupon check: rate limited.
  - Subscriptions: 120 requests/h per token, burst 30.
  - An optional captcha (turnstile, recaptcha, hcaptcha).
- **Subscriptions**: a malformed or unknown token gets 403 without revealing whether it exists. A banned, expired or
  over-quota user gets a valid document with only the info entries and no nodes.
- **Payment notify**: every gateway verifies its signature; fulfilment is idempotent and compares the amount with the order.
- **Secrets**: `server_token`, `email_password`, `captcha_secret`, `telegram_bot_token`, `telegram_webhook_secret` and payment
  configs are only served by admin-only endpoints. The Telegram client strips the request URL (which embeds the bot token) from
  transport errors and never follows redirects; the webhook secret travels in a path parameter, which logs never record.
- **Proxies & limits**: per-IP rate limits key IPv6 clients by /64. `X-Forwarded-For`/`X-Real-IP`/`X-Forwarded-Proto` are
  honoured only from `server.trusted_proxies` (`httpx.RequestScheme`/`RequestBase` for links built without `app_url`).
- **Hashing budget**: argon2id derivations run at most `NumCPU` (≥ 2) at a time; unknown emails at login still spend one
  derivation (no timing oracle). Password/MFA/register-by-IP counters reserve an attempt before verifying (no check-then-act races).
- **Logs** carry route patterns (`httpx.LogPath`), never raw paths of matched routes or query strings. With a captcha provider
  enabled, `static.CSPFor` adds exactly that provider's script/frame (hCaptcha also connect/style) origins to the CSP.
- **CORS**: same-origin by default; `server.cors_origins` widens it.
- **Headers**: every response sets `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`
  and `X-Frame-Options: DENY`. HTML documents add this CSP:
  `default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`.

## Pluggability

Every plugin point is a Go interface plus a registry built on `plugin.NewRegistry[T](kind)` (`Register`, `Get`, `Has`,
`Names`, `Each`, `Len`; duplicate names panic). Built-ins register in `init()` and are linked by blank imports:

| plugin point | registry | built-ins | linked by |
|---|---|---|---|
| payment gateway | `payment.Register` | epay, epusdt, alipay_f2f, stripe, coinbase, btcpay | `cmd/ggboard/plugins_payment.go` → `internal/payment/gateways` |
| subscription format | `subscription.RegisterFormat` | clash-meta, stash, clash, singbox, surge, surfboard, quantumultx, shadowrocket, loon, v2ray, ssr | `internal/api/client` → `internal/subscription/formats` |
| captcha | `captcha.Register` | none, turnstile, recaptcha, hcaptcha | `internal/captcha` itself |
| mail driver | `mail.FromSettings` (`email_driver`) | smtp, log (memory in tests) | `internal/mail` |
| notification channel | `notify.Register(name, factory)` | telegram (`notifytest` "record" in tests) | `cmd/ggboard/plugins_notify.go` → `internal/notify/telegram` (also imported by the API packages) |
| jobs | `app.RegisterJob(name, spec, fn)` | see above | `cmd/ggboard/jobs_maintenance.go`; `service/order` through the API packages |
| CLI subcommands | `cli.Register(factory)` | `import xboard` | `cmd/ggboard/import.go` |

See `AGENTS.md` §7.

## Frontend delivery (`internal/static`)

- `make web` builds `web/` with Vite and copies `web/dist` into `internal/static/dist`, which is embedded with
  `//go:embed all:dist`. `frontend.dir` serves a directory instead.
- Files under `assets/` (content-hashed) get `Cache-Control: public, max-age=31536000, immutable`. Everything else,
  including `index.html` and `sw.js`, gets `no-cache` with a sha256 ETag.
- `.br`/`.gz` siblings (emitted by `vite-plugin-compression2`) are served when `Accept-Encoding` allows.
- Public routes are prerendered at build time (`web/scripts/prerender.mjs`, run by `pnpm build` when a Chrome/Chromium
  binary is available; the Docker web stage installs Chromium): `/` → `index.html`, `/login` → `login/index.html`,
  `/register`, `/forgot`. A snapshot page paints before any JS: it loads only CSS plus a tiny `assets/boot-*.js`, which
  starts the app after the first paint; the app replaces the snapshot with an identical tree in one commit (no hydration,
  see `web/src/main.tsx`). Snapshot forms are inert until then and typed values are carried over.
- Fallbacks:
  - `<path>/index.html` (prerendered) is served when present; other unknown GET/HEAD paths fall back to `spa.html`
    (the plain shell), else `index.html`.
  - Missing asset-like paths (with an extension, or under `assets/`) return 404.
  - `/api/*` and `/soga/*` return a JSON 404.
  - Without a built frontend, a small "Frontend not built" page is served.

## Testing

- Unit tests sit next to the code. `testutil.NewTestApp(t)` gives each test a private SQLite file in `t.TempDir()`.
  - It uses WAL so concurrency behaves like production. A one-connection in-memory DB deadlocks on legitimate
    nested reads.
  - It also provides default settings, cheap argon2 parameters and an in-memory mailbox.
  - `testutil.NewRouterWith(t, app, pkg.Register)` builds an engine; `testutil.Login(t, app, user)` returns a JWT.
    See docs/BACKEND.md §10.
- `make test` runs `go test ./... -race -count=1`.
- `make e2e` (`scripts/e2e.sh`) builds the binary, starts it on a random port with a throwaway data dir, and checks
  `/healthz`, `GET /api/v1/guest/config` and the SPA root. It does not build the frontend; run `make web` first to
  test the real SPA.
- Frontend: `vitest` for utils and stores; Lighthouse via `@lhci/cli` (see `web/lighthouserc.cjs`).
