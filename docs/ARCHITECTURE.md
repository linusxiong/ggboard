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
1. `config.Load()` — viper: `config.yaml` (optional) ← env `GGBOARD_*` ← flags. Keys:
   `server.addr`, `server.trusted_proxies[]`, `database.driver` (sqlite|postgres|mysql), `database.dsn`,
   `jwt.secret`, `jwt.ttl` (default 168h), `log.level`, `log.format`, `data_dir`, `frontend.dir` (optional override of embedded).
2. `db.Open()` + `db.AutoMigrate(model.All()...)` + seed defaults (admin user prompt via `ggboard admin create`).
3. `settings.New(db)` — typed getters with defaults (`settings.Defaults` map), cached, invalidated on `Save`.
4. `app.New(...)` builds the dependency container `*app.App{Cfg, DB, Settings, Cache, Mailer, Log, Version, StartedAt, Online}`.
5. `httpx.NewRouter(app)` registers middleware (request id, recovery, slog access log, CORS same-origin, gzip for JSON,
   rate limiters) and calls `api/<pkg>.Register(r, app)` for guest, passport, user, admin, client, node, payment.
6. `scheduler.Start(app)` registers cron jobs (see below).
7. `static.Serve(r, app)` mounts the embedded SPA with precompressed assets, immutable cache for hashed files, SPA fallback.
8. Graceful shutdown on SIGTERM (drain 10s).

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
  `rate_limited`(429) `payment_failed`(402) `internal`(500).
- Auth: `Authorization: Bearer <JWT>`; JWT claims `{sub: user_id, email, iat, exp, ver}`; `ver` is
  bumped on password change / security reset so old tokens die. Middleware loads the user (cached 30s) and rejects banned users.
- Admin middleware: user.is_admin; Staff middleware: is_admin || is_staff.
- Pagination query: `page` (1-based), `page_size` (default 20, max 200), `sort` (`field` or `-field`), filters as query params.
- Node APIs authenticate with the shared `server_token` setting (constant-time compare) — XrayR passes `token` query; soga passes `API-KEY` header.
- All handlers bind + validate with `binding:"required,..."` tags; translate validator errors into `errors` map.

## Caching rules
- `settings`: whole map cached, invalidated on save.
- Node user lists (`/UniProxy/user`, `/api/v1/users`): cached per group-set for 60s; ETag = sha1 of JSON body; 304 honored.
- Node config: ETag per node `updated_at`.
- Subscription: generated per request (cheap), node list cached 5s per user; `ETag` on response too.
- Online users / alive IPs / node status: in-memory maps with TTL (600s), surfaced in admin node list.
- Traffic push: aggregated in memory per (node,user) and flushed to DB every 10s in one transaction
  (user.u/d increments + stat_users/stat_nodes daily upserts). Flush also on shutdown.

## Scheduler jobs (`internal/scheduler`)
| job | cron | what |
|---|---|---|
| traffic_reset | `0 0 * * *` | users whose `next_reset_at <= now`: u=d=0, compute next `next_reset_at` per plan `reset_traffic_method` |
| expire_remind | `0 10 * * *` | email users expiring within 24h (remind_expire) |
| traffic_remind | `0 10 * * *` | email users with >80% used (remind_traffic), once per day |
| order_timeout | `*/1 * * * *` | cancel pending orders older than `order_timeout_minutes` (default 30) |
| commission_settle | `0 */1 * * *` | orders with commission_status=1 → pay invite_user, log commission_logs, status=2 (respect `commission_auto_check_enable`) |
| stats_daily | `5 0 * * *` | aggregate yesterday's `stats` row (orders, paid, registers, invites, transfer total) |
| online_cleanup | `*/5 * * * *` | purge expired online/alive entries |
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
Every plugin point is a Go interface + a registry in `internal/plugin` (`Registry[T]` with `Register(name, factory)`, `Get`, `Names`).
Built-ins self-register in `init()`; `cmd/ggboard/main.go` blank-imports the plugin packages.
See `AGENTS.md` §7 for the table.

## Frontend delivery
- `web/` builds to `web/dist` (Vite). The Go build embeds `web/dist` via `internal/static` (`//go:embed all:dist`, copied by `make web`).
- Assets are served with `Cache-Control: public, max-age=31536000, immutable` (hashed names), `index.html` with `no-cache`.
- Brotli/gzip precompressed files served when `Accept-Encoding` allows (`vite-plugin-compression2` emits `.br`/`.gz`).
- `GET /` and unknown paths fall back to `index.html` (SPA). `/api/*` never falls back.

## Testing
- Unit tests next to code. API tests use `testutil.NewTestApp(t)` → in-memory SQLite, seeded settings, gin engine with the package's routes registered; helper `testutil.Login(t, app, user)` returns a JWT.
- `make test` runs `go test ./... -race`; `make e2e` builds the frontend, starts the server on a random port and runs the smoke script in `scripts/e2e.sh`.
- Frontend: `vitest` for utils/stores; Lighthouse via `@lhci/cli` against `vite preview` (see `web/lighthouserc.cjs`).
