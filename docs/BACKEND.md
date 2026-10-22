# Backend guide (for feature agents)

The foundation packages under `internal/` are the only way to touch HTTP, auth, settings, cache, mail, jobs
and the CLI. Read this page, `AGENTS.md` and `MISTAKES.md` before adding code. Module: `github.com/linusxiong/ggboard`.

## 1. Adding a route package
`internal/router/router.go` calls `Register` of `internal/api/{guest,passport,user,admin,client,node}`.
Each `Register` receives the **root** router and mounts its own paths (prefix `httpx.APIPrefix` = `/api/v1`), so
`client` can also mount `/s/:token` and `node` can mount `/soga/api/v1/...`. Replace the `_health` stub body:

```go
package user

func Register(r gin.IRouter, a *app.App) {
	g := r.Group(httpx.APIPrefix+"/user", auth.RequireUser(a))
	g.GET("/me", func(c *gin.Context) { httpx.OK(c, toView(auth.CurrentUser(c))) })
	g.PATCH("/me", updateMe(a))
}

func updateMe(a *app.App) gin.HandlerFunc {
	type req struct {
		RemindExpire *bool `json:"remind_expire"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		// ...
	}
}
```
`router.New` is in `internal/router` (not `httpx`) because `httpx` is imported by every API package (import cycle).
Never edit `router.go` to add routes; new packages need an integrator change to `router.Packages`.

## 2. `internal/httpx`
| need | API |
|---|---|
| success / list | `httpx.OK(c, data)` → `{"data":…}`; `httpx.Created(c, data)`; `httpx.List(c, items, total)` → `{"data":[…],"total":N}` (nil → `[]`) |
| errors | `httpx.Fail(c, err)` aborts. `*httpx.Error{Status, Code, Message, Errors}`; constructors `BadRequest`, `FieldError(field,msg)`, `Unauthorized`, `PaymentFailed`, `Forbidden`, `NotFound`, `Conflict`, `PayloadTooLarge`, `RateLimited`, `Internal(err)`, `NewError(status, code, msg)`; `.WithField(f, msg)`, `.WithCause(err)` |
| automatic mapping | `gorm.ErrRecordNotFound`→404, `gorm.ErrDuplicatedKey`→409, `*http.MaxBytesError`→413, validator/JSON errors and any error with `FieldErrors() map[string][]string`→400, everything else→500 (cause logged, never sent) |
| binding | `httpx.Bind(c, &req)` (query for GET/HEAD/DELETE, JSON otherwise; empty body still validated), `httpx.BindQuery`, `httpx.Validate(&v)`, `httpx.ParamID(c, "id")` (404 on bad id). Use `binding:"required,email,min=8,oneof=a b"` tags; error keys are JSON names (`items[0].name`) |
| pagination | `httpx.ParsePage(c) (page, size, offset)` (default 20, max 200); `httpx.ParseSort(c, map[apiField]column) string`; `httpx.Paginate[T](c, query, allowedSort, "id DESC") ([]T, total, err)` |
| rate limit | `httpx.RateLimit(name, httpx.Per(10, time.Hour), burst, httpx.KeyByIP)` middleware; `httpx.KeyByQuery("token")`; shared buckets: `l := httpx.NewLimiter(r, burst)` + `httpx.Limit(name, l, keyFn)`; programmatic `l.Allow(key)` |
| body size | 1 MiB default; node push routes add `httpx.LargeBody(httpx.NodeBodyLimit)` (8 MiB) |
| logging | `httpx.Logger(c)` (request-scoped slog with `request_id`), `httpx.RequestIDFrom(c)` |
Global middleware: request id → access log (path only, never the query) → security headers → CORS (only when configured) → gzip (≥1 KiB compressible bodies; skipped when a handler sets `Content-Encoding`) → recovery → body limit.

## 3. `internal/auth`
- Middlewares: `auth.RequireUser(a)`, `auth.RequireAdmin(a)`, `auth.RequireStaff(a)` (admin or staff), `auth.OptionalUser(a)`.
- `auth.CurrentUser(c) *model.User` is a request-local copy cached for 30 s: **reload the user inside your transaction
  before changing balance, traffic or plan.** After changing password/ban/roles/uuid call `auth.InvalidateUser(a, id)`.
- Tokens: `auth.Issue(a, u) (string, error)`, `auth.Parse(a, tok) (*auth.Claims, error)`; claims `{sub, email, ver, iat, exp, iss}`.
- Passwords: `auth.HashPassword(pw)`, `auth.VerifyPassword(hash, pw)` (argon2id, bcrypt `$2y$/$2a$/$2b$`, Xboard `xb$<algo>$<salt>$<hex>`, `!…` never verifies), `auth.NeedsRehash(hash)`.
- One-time login: `auth.IssueLoginToken(a, userID) string` (5 min, single use), `auth.ConsumeLoginToken(a, tok) (uint64, bool)`.

## 4. `internal/app` (dependency container)
`*app.App` fields: `Cfg *config.Config`, `DB *gorm.DB`, `Settings *settings.Service`, `Cache *cache.Cache`,
`KV *cache.TTLMap[string, any]`, `Mailer *mail.Queue`, `Log *slog.Logger`, `Logs *logx.Ring`, `Version`, `StartedAt`,
`Online *online.Store`, `Traffic *traffic.Accumulator`, `Scheduler *scheduler.Scheduler`; helpers `a.DBDriver()`, `a.FlushTraffic(ctx)`.

- **Cache** (ristretto, may evict): `a.Cache.Get/Set(key, v, ttl)/Delete`, `cache.GetAs[T]`, `cache.GetOrLoad[T](a.Cache, key, ttl, load)`
  (singleflight), namespaced keys `a.Cache.Key(ns, key)` + `a.Cache.InvalidateNamespace(ns)`, `a.Cache.Stats()`.
- **KV** (never evicted early) for email codes, counters, one-time tokens: `Set(k, v, ttl)`, `Get`, `Take`, `Update(k, ttl, keepTTL, fn)`, `TTL`, `Delete`, `Purge`. Prefix keys with your package name.
- **Online** (`internal/app/online`): `Touch(node)`, `TouchPush(node)`, `SetOnline(node, n)`, `SetLoad(node, online.Load{CPU,Mem,Swap,Disk,Uptime})`,
  `SetVersion`, `SetAliveIPs(node, user, ips)`, `SetNodeAlive(node, map[user][]ip)`, `AliveIPs(user)`, `AliveCount(user)`,
  `AliveCountMode(user, device_limit_mode)`, `AliveCounts()`, `AliveCountsMode(mode)`, `NodeAlive(node)`, `OnlineUserCount()`,
  `NodeStatus(id) (online.Status, bool)`, `Snapshot()`, `Forget(id)`. `Status.State(now, online.Window(push, pull))` →
  `StateOnline`(2)/`StateIdle`(1)/`StateOffline`(0); `Status.Online(now, w)`. Alive TTL = `max(300s, 2×server_push_interval)`.
- **Traffic**: `a.Traffic.Add(nodeID, userID, rate, up, down)`; flushed every 10 s and on shutdown in one transaction
  (users.u/d += bytes×rate, `stat_users` (×rate) and `stat_nodes` (raw) daily upserts). Upsert helpers for other code:
  `traffic.Excluded(db, col)`, `traffic.AddExcluded(db, table, col)` (portable `excluded.x` / `VALUES(x)`).

## 5. Settings (`internal/settings`)
`a.Settings.String/Int/Int64/Bool/Strings/Duration(key, unit)/JSON(key, &dst)/Get(key)`; `a.Settings.Set(map[string]any)`
validates against the key list (unknown keys, wrong types, enum and range violations → `settings.FieldErrors`, rendered as 400 by `httpx.Fail`);
`nil` resets a key. `AllGrouped()` (admin, secrets included, hidden keys excluded), `settings.Groups()` (UI metadata),
`settings.IsSecret(key)`, `settings.Lookup(key)`, `settings.Defaults`. Hidden JSON key `rulesets_custom` stores admin rule-set overrides.
Reads never hit the database (snapshot is swapped after each successful Set).

## 6. Mail (`internal/mail`)
Never send inline. `a.Mailer.SendTemplate(to, mail.TplVerifyCode, mail.Data{"Code": "123456"})`. Templates and data keys:
`verify_code{Code, Minutes}`, `remind_expire{ExpiredAt, URL}`, `remind_traffic{Percent, Used, Total, URL}`,
`order_paid{TradeNo, PlanName, Period, Amount, URL}`, `ticket_reply{Subject, Message, URL}` (`Subject` = ticket subject; the rendered
e-mail subject is `MailSubject` and only replaces `Subject` when the caller passes none), `notify{Subject, Title?, Content | ContentHTML (template.HTML, sanitized), URL}`.
Brand (app_name, app_url, logo) and locale (`default_locale`) come from settings. `a.Mailer.Enqueue(mail.Message{...})` for custom
mail, `a.Mailer.SendNow(ctx, msg)` for the admin test mail (synchronous), `a.Mailer.Stats()`. Every outcome is written to `mail_logs`.

## 7. Scheduler and CLI
- Jobs: in your package `func init() { app.RegisterJob("order_timeout", "*/1 * * * *", func(ctx context.Context, a *app.App) error { … }) }`.
  Jobs start only in `ggboard serve`; status via `a.Scheduler.Status() []scheduler.JobStatus`; tests call `a.Scheduler.RunNow(name)`.
  Built-in: `online_cleanup` (*/5 min), `traffic_flush` (@every 10s).
- Subcommands: `func init() { cli.Register(func(a *app.App) *cobra.Command { … }) }` in your package, plus a file in `cmd/ggboard/`
  that blank-imports it. `a` is initialized (config loaded, DB migrated) before `RunE`. Plugin packages (gateways, formats) are
  blank-imported the same way (e.g. `cmd/ggboard/plugins_payment.go`).

## 8. Domain helpers
- `internal/service/user` (`usersvc.New(a)`): `FindByID/FindByEmail/FindByToken(ctx, …)` (`ErrNotFound`), `Create(ctx, email, pw, CreateOptions{…, PasswordHash})`
  (`ErrEmailTaken`), `Authenticate(ctx, email, pw)` (`ErrInvalidCredentials`, `ErrBanned`; upgrades legacy hashes without bumping `password_version`),
  `RecordLogin`, `SetPassword` (bumps version), `ResetSecurity` (new uuid+token, bumps version), `SubscribeURL(u)`, `ComputeNextResetAt(u, plan, now)`,
  `usersvc.NextResetAt(method, expiredAt, now)`, `ResetDay(u)`, `AvailableUsersForGroups(ctx, groupIDs) ([]NodeUser{ID, UUID, SpeedLimit, DeviceLimit}, error)`
  (60 s cache; `InvalidateNodeUsers()` after user changes nodes must see), `usersvc.NewToken()`, `usersvc.NormalizeEmail`.
- `internal/model`: all tables, enums (`OrderStatus*`, `OrderType*`, `CommissionStatus*`, `ResetMethod`, `Ticket*`, `CouponType*`, `Node*` types, `Period*`),
  `model.UserCredentials(u, node)`, `model.NodeServerKey(node)` (preload `Parent` for child nodes), `model.SS2022UserKey`, typed protocol settings
  (`var s model.VlessSettings; s.Decode(n.ProtocolSettings)` or `model.DecodeSettings[model.VlessSettings](m)`), `model.DecodeNetwork(network, m)`,
  `model.Day(t)` for every `date` column, `u.IsAvailable(now)`.
- `internal/service/ticket` (`ticketsvc.New(a)`): `Create` (`ErrOpenExists`: one open ticket per user), `Get(ctx, id, ownerID)`,
  `ReplyAsOwner`, `ReplyAsStaff` (mails `ticket_reply`), `Close`, `CountOpen(ctx, userID)`, `Query(ctx, Filter)`, views `ForOwner`/`ForStaff`.
- `internal/service/invite` (`invitesvc.New(a)`): `Overview`, `Stat`, `CommissionPending(ctx, userID)`, `GenerateCode`, `CommissionQuery`,
  `Transfer`, `Withdraw`. `internal/service/content` (`contentsvc.New(a)`): `NoticeQuery(ctx, onlyVisible)`, `KnowledgeQuery`,
  `KnowledgeIndex`, `Article`, `RenderBody`, `SortNotices`, `SortKnowledge`.
- `internal/payment`: `Gateway`, `Register`, `Get`, `List`, `Names`, `FormatAmount(cents)`, `ParseAmount(s)`, `FailResponse()`.
- `internal/subscription`: `Format`, `RenderContext`, `RuleProfile`, `Catalog`, `RegisterFormat`, `GetFormat`, `FormatNames`, `DetectFormat(flag, ua)`.
- `internal/captcha`: `captcha.FromSettings(a.Settings).Verify(ctx, token, c.ClientIP())`, `captcha.Enabled(a.Settings)`.
- `internal/plugin`: `plugin.NewRegistry[T](kind)` for new plugin points.
- `internal/service/stat` (`statsvc.New(a)`): `PaidStatuses` (orders counted as paid: 1, 3, 4 by `paid_at`), `Paid(ctx, from, to)`,
  `Registers`, `Overview`, `Series`, `NodeRanking`, `UserRanking`, `NodeCounts(ctx, now)` (three-state online count), `SaveDay(ctx, day)`.
- `internal/service/adminuser`: admin user view (`View`, `Views` batch-loads plans/inviters), `Filters`, create/update/delete cascade,
  generate/export/bulk mail; `Nullable[T]` distinguishes absent / null / value in PATCH bodies.
- `internal/service/maintenance`: jobs `traffic_reset`, `expire_remind`, `traffic_remind`, `stats_daily` (exported with a `Clock`
  for tests); `maintenance.Enqueue(ctx, a.Mailer, msg)` waits while the mail queue (1024) is full: use it for any bulk mail.

## 9. Database rules
- SQLite (pure Go), PostgreSQL and MySQL must all work: JSON columns are TEXT, use `clause.OnConflict` + `traffic.Excluded` for upserts,
  never quote identifiers by hand, never use dialect-specific functions without a fallback.
- `show`, `key` and `match` are reserved words in MySQL: never write them in raw SQL strings (`Where("show = ?", true)` breaks on MySQL).
  Use `Where(map[string]any{"show": true})`, `Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}})` or struct conditions;
  GORM quotes identifiers it generates.
- All `time.Time` parameters are normalized to UTC on SQLite by the driver wrapper; always pass `time.Time`, never preformatted strings.
- Do not add `default:true` to bool columns (GORM skips zero values on insert, so `false` would become `true`). Set defaults in code.
- No foreign-key constraints are created; delete dependants explicitly. When saving a struct with loaded relations use
  `db.Omit(clause.Associations)`.
- Never reuse a model variable whose primary key is set for another `First`/`Find`: GORM adds it to the WHERE clause.
- Inside `a.DB.Transaction(func(tx *gorm.DB) error {…})` use `tx` for every query; writes through `a.DB` wait for the lock (SQLite).

## 10. Tests (`internal/testutil`)
```go
a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"email_verify": true}))
r := testutil.NewRouterWith(t, a, passport.Register)            // or router.New(a) for everything
u := testutil.CreateUser(t, a, "alice@example.com")             // password testutil.Password
admin := testutil.CreateAdmin(t, a, "root@example.com")         // CreateStaff too
rec := testutil.Do(t, r, http.MethodPost, "/api/v1/passport/login", map[string]any{"email": u.Email, "password": testutil.Password}, "")
body := testutil.Decode[LoginResponse](t, rec)                   // {"data": …}; DecodeList[T] for lists
e := testutil.DecodeError(t, rec)                                // httpx.Error with Status
mails := testutil.Mails(t, a)                                    // waits for the queue; in-memory mailbox
tok := testutil.Login(t, a, admin)                               // JWT for Do(..., tok)
```
Each test app uses a private SQLite file in `t.TempDir()` (WAL; behaves like production with concurrent readers) and cheap argon2
parameters. `DoWithHeaders` adds headers; `ExpectStatus`, `Ptr`, `MustJSON` are helpers. Run `make test` (`go test ./... -race`)
and `make lint` (golangci-lint v2 with `.golangci.yml` when installed; the foundation is at 0 issues).
