# Architecture

Single Go binary (`ggboard`) that serves the JSON API, the node-facing APIs (XrayR/UniProxy and soga-v1),
subscription generation, payment callbacks, a cron scheduler and the embedded React frontend.
No Redis, no queue workers: an in-process cache (ristretto) and batched DB writes are enough for a panel.

```
┌──────────────┐   HTTPS    ┌─────────────────────────────────────────────┐
│  React SPA   │ ─────────▶ │ gin router                                   │
│ (embedded)   │            │  /api/v1/guest  /passport  /user  /admin     │
└──────────────┘            │  /api/v1/client/subscribe  (token)           │
┌──────────────┐            │  /api/v1/server/UniProxy/* (XrayR / V2bX)    │
│ XrayR / V2bX │ ─────────▶ │  /api/v1/{node,users,traffic,...} (soga-v1)  │
│ soga         │            │  /api/v1/guest/payment/notify/:gw/:uuid      │
└──────────────┘            └───────────────┬──────────────────────────────┘
                                            │ services (order, user, node, stat, ticket, invite, subscription)
                                            ▼
                     ┌──────────┐   ┌─────────────┐   ┌──────────┐   ┌───────────┐
                     │  GORM DB │   │ ristretto   │   │ cron     │   │ mailer    │
                     │ sqlite/pg│   │ cache+ETag  │   │ jobs     │   │ smtp/log  │
                     └──────────┘   └─────────────┘   └──────────┘   └───────────┘
```

## Boot sequence (`cmd/ggboard serve`)
1. `config.Load()` — viper: defaults ← `config.yaml` (optional) ← env `GGBOARD_*` ← flags. Keys:
   `server.addr`, `server.trusted_proxies[]`, `server.cors_origins[]`, `database.driver` (sqlite|postgres|mysql), `database.dsn`,
   `jwt.secret` (generated once into `<data_dir>/jwt.secret` when empty), `jwt.ttl` (default 168h), `log.level`, `log.format`,
   `data_dir`, `frontend.dir` (optional override of the embedded SPA).
2. `app.New(cfg)` builds the dependency container: `db.Open()` + `db.Migrate()` (auto-migrate `model.All()`, idempotent, every boot),
   `settings.New(db)` (generates `server_token` on first boot), caches, mail queue, `Online`, `Traffic`, scheduler with every job
   registered through `app.RegisterJob`. `*app.App{Cfg, DB, Settings, Cache, KV, Mailer, Log, Logs, Version, StartedAt, Online, Traffic, Scheduler}`.
   Admin users are created with `ggboard admin create`.
3. `router.New(app)` (`internal/router`): `httpx.NewEngine` (request id, access log, security headers, CORS when configured, gzip,
   recovery, body limit) → `api/<pkg>.Register(root, app)` for guest, passport, user, admin, client, node (each mounts its own paths
   under `/api/v1`; client/node also mount `/s/:token` and `/soga/api/v1`) → `static.Mount` as the NoRoute handler
   (embedded SPA, precompressed assets, immutable cache for hashed files, SPA fallback). The router lives outside `httpx`
   because every API package imports `httpx`.
4. `app.Start()` starts the mail worker and the scheduler.
5. Graceful shutdown on SIGINT/SIGTERM: HTTP drain 10s, then `app.Close()` stops the scheduler, flushes traffic, drains mail, closes the DB.

## CLI
- `ggboard serve` — run server.
- `ggboard migrate` — run migrations only.
- `ggboard admin create --email --password` / `admin reset-password` / `admin promote`.
- `ggboard import xboard --dsn "<driver>://..." [--dry-run] [--skip-users ...]` — see `docs/IMPORT_XBOARD.md`.
- `ggboard version`.

## HTTP conventions (`internal/httpx`)
- Success: `200 {"data": ...}`; lists: `{"data": [...], "total": N}`; empty success: `{"data": true}`.
- Errors: status + `{"code": "validation_failed", "message": "...", "errors": {"field": ["msg"]}}`.
  Codes: `validation_failed`(400) `unauthorized`(401) `forbidden`(403) `not_found`(404) `conflict`(409)
  `payload_too_large`(413) `rate_limited`(429) `payment_failed`(402) `internal`(500).
- Body limit: 1 MiB; node push endpoints 8 MiB (`httpx.LargeBody`).
- Auth: `Authorization: Bearer <JWT>`; JWT claims `{sub: user_id, email, iat, exp, ver}`; `ver` is
  bumped on password change / security reset so old tokens die. Middleware loads the user (cached 30s) and rejects banned users.
- Admin middleware: user.is_admin; Staff middleware: is_admin || is_staff.
- Pagination query: `page` (1-based), `page_size` (default 20, max 200), `sort` (`field` or `-field`), filters as query params.
- Node APIs authenticate with the shared `server_token` setting (constant-time compare) — XrayR passes `token` query; soga passes `API-KEY` header.
- All handlers bind + validate with `binding:"required,..."` tags; translate validator errors into `errors` map.

## Caching rules
- `settings`: whole map cached, invalidated on save.
- Node user lists (`/UniProxy/user`, `/api/v1/users`): cached per group-set for 60s; ETag = sha1 of JSON body; 304 honored.
- Node config: ETag = quoted sha1 of the response body (XrayR/V2bX compare the header value, see docs/NODE_PROTOCOLS.md).
- Subscription: generated per request (cheap), node list cached 5s per user; `ETag` on response too.
- Online users / alive IPs / node status: in-memory maps with TTL `max(300s, 2×push_interval)`, surfaced in admin node list (three-state node status: online / idle / offline per docs/NODE_PROTOCOLS.md §5).
- Traffic push: aggregated in memory per (node,user) and flushed to DB every 10s in one transaction
  (user.u/d increments + stat_users/stat_nodes daily upserts). Flush also on shutdown.

## Scheduler jobs (`internal/scheduler`)
| job | cron | what |
|---|---|---|
| traffic_reset | `0 0 * * *` | users whose `next_reset_at <= now`, 500 per transaction: u=d=0 and the next `next_reset_at` when the plan still resets (plan present, effective method ≠ never, `expired_at` set), otherwise only `next_reset_at` is cleared; node user lists invalidated |
| expire_remind | `0 10 * * *` | `remind_mail_enable`: email users (not banned, `remind_expire`) whose `expired_at` is in (now, now+24h]; once per 24 h per user (`app.KV`) |
| traffic_remind | `0 10 * * *` | `remind_mail_enable`: email users (not banned, not expired, `remind_traffic`) with 80% ≤ used < 100%, once per 24 h per user (`app.KV`) |
| order_timeout | `*/1 * * * *` | cancel pending orders older than `order_timeout_minutes` (default 30) |
| commission_settle | `0 */1 * * *` | orders with commission_status=1 → pay invite_user, log commission_logs, status=2 (respect `commission_auto_check_enable`) |
| stats_daily | `5 0 * * *` | upsert yesterday's `stats` row: orders created, orders paid (status 1/3/4 by `paid_at`), commission_logs, registers, invites, `stat_nodes` traffic |
| online_cleanup | `*/5 * * * *` | purge expired online/alive entries and `app.KV` |
| traffic_flush | `@every 10s` | flush the in-memory traffic accumulator (also on shutdown) |
| mail_queue | continuous | in-process worker goroutine with bounded channel + retry (mail is never sent inline in a request) |

## Security
- Passwords: argon2id for new hashes; bcrypt (`$2y$`, `$2a$`, `$2b$`) verified for imported users and transparently upgraded on login.
- Login/register/send-code rate limits per IP + per email; optional captcha (turnstile/recaptcha/hcaptcha).
- Subscription endpoint rate-limited per token; invalid token → 403 without leaking existence.
- Payment notify: every gateway verifies signature; idempotent `paid()`; amount compared with order.
- Admin config values that are secrets (SMTP password, payment configs, server_token) are never returned to non-admin; config endpoint returns them to admin only.
- CORS: same-origin by default; `server.cors_origins` to widen.
- Headers: `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options: DENY` on SPA, CSP on SPA (`default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'`).

## Pluggability
Every plugin point is a Go interface + a registry in `internal/plugin` (`Registry[T]` with `Register(name, v)`, `Get`, `Names`, `Each`).
Jobs register with `app.RegisterJob(name, spec, fn)` and CLI subcommands with `cli.Register(factory)`, both from `init()`.
Built-ins self-register in `init()`; `cmd/ggboard/main.go` blank-imports the plugin packages.
See `AGENTS.md` §7 for the table.

## Frontend delivery
- `web/` builds to `web/dist` (Vite). The Go build embeds `web/dist` via `internal/static` (`//go:embed all:dist`, copied by `make web`).
- Assets are served with `Cache-Control: public, max-age=31536000, immutable` (hashed names), `index.html` with `no-cache`.
- Brotli/gzip precompressed files served when `Accept-Encoding` allows (`vite-plugin-compression2` emits `.br`/`.gz`).
- `GET /` and unknown paths fall back to `index.html` (SPA). `/api/*` never falls back.

## Testing
- Unit tests next to code. API tests use `testutil.NewTestApp(t)` → private SQLite file in `t.TempDir()` (WAL, so concurrency behaves
  like production; an in-memory DB with one connection deadlocks on legitimate nested reads), default settings, in-memory mailbox;
  `testutil.NewRouterWith(t, app, pkg.Register)` builds the engine; `testutil.Login(t, app, user)` returns a JWT. See `docs/BACKEND.md`.
- `make test` runs `go test ./... -race`; `make e2e` builds the frontend, starts the server on a random port and runs the smoke script in `scripts/e2e.sh`.
- Frontend: `vitest` for utils/stores; Lighthouse via `@lhci/cli` against `vite preview` (see `web/lighthouserc.cjs`).
