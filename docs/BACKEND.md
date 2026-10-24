# Backend guide (for contributors)

The foundation packages under `internal/` are the only way to touch HTTP, auth, settings, cache, mail, jobs and the CLI.
Read this page, `AGENTS.md` and `MISTAKES.md` before adding code. The module is `github.com/linusxiong/ggboard`.
docs/ARCHITECTURE.md describes the runtime; this page is the API you program against.

## 0. Package layout

```text
cmd/ggboard/            main.go + blank imports: plugins_payment.go, plugins_notify.go, jobs_maintenance.go, import.go;
                        wire_leak.go (subscription.LeakHook → mail + notify)
internal/
  api/{guest,passport,user,admin,client,node}   one Register(r, a) per package
  app/ (+ online/, traffic/)                    dependency container, runtime node state, traffic accumulator
  auth/        JWT, password hashing, middlewares, one-time login tokens
  cache/       ristretto wrapper + TTLMap
  captcha/     none / turnstile / recaptcha / hcaptcha
  cli/         cobra root: serve, migrate, admin, version (+ registered subcommands)
  config/      viper configuration
  console/     terminal style detection (--color / log.color / NO_COLOR) and the lipgloss printer for CLI output
  db/          gorm open (sqlite/postgres/mysql), AutoMigrate and idempotent schema upgrades (upgrade.go)
  httpx/       engine, middleware, responses, errors, binding, pagination, rate limits, gzip
  importer/xboard/                              `ggboard import xboard`
  logx/        slog handler + ring buffer for /admin/system/logs
  mail/ (+ templates/)                          queue, smtp/log drivers, zh-CN/en templates, custom (Markdown) templates
  model/       every GORM model, enums, typed protocol settings, credentials
  notify/ (+ telegram/, notifytest/, telegram/telegramtest/)   notification queue, events, Telegram bot driver
  payment/ (+ gateways/{epay,epusdt,alipay_f2f,stripe,coinbase,btcpay,gwutil})
  plugin/      generic Registry[T]
  router/      assembles engine + API packages + SPA
  scheduler/   named cron jobs with status
  service/{account,adminuser,content,invite,maintenance,mfa,node,order,plan,stat,ticket,user}
  settings/    typed key/value settings (defs.go is the key list)
  static/      embedded SPA (dist/ copied by `make web`)
  subscription/ (+ engine/, formats/, rulesets/, guard/, fixture/)   guard = age keys, access log, leak detection
  testutil/    test app, router, users, request helpers
```

## 1. Adding routes

`internal/router/router.go` calls `Register` of `internal/api/{guest,passport,user,admin,client,node}` (`router.Packages`).
Each `Register` receives the **root** router and mounts its own paths under `httpx.APIPrefix` (`/api/v1`). That is how
`client` can also mount `/s/:token`, `node` can mount `/soga/api/v1/...` and `passport` can mount `/.well-known/*`.
Inside a package, each area has its own `registerXxx(g, a)` called from `routes.go`:

```go
func Register(r gin.IRouter, a *app.App) {
    g := r.Group(httpx.APIPrefix+"/user", auth.RequireUser(a))
    registerAccount(g, a)
    // ...
}

func registerAccount(g *gin.RouterGroup, a *app.App) {
    g.GET("/me", getMe(a))
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

Admin areas mount under the `/admin` group (`RequireStaff` + `mfasvc.AdminGate`). Every admin-only area adds
`auth.RequireAdmin(a)` on its own sub-group; staff may only use tickets, notices, knowledge and read users.
`router.New` is in `internal/router` (not `httpx`) because every API package imports `httpx`, which would be an
import cycle. A new API package needs one line in `router.Packages`.

## 2. `internal/httpx`

| need | API |
| --- | --- |
| success / list | `httpx.OK(c, data)` → `{"data":…}` (`OK(c, true)` for empty successes); `httpx.Created(c, data)` → 201; `httpx.List(c, items, total)` → `{"data":[…],"total":N}` (nil → `[]`) |
| errors | `httpx.Fail(c, err)` aborts. `*httpx.Error{Status, Code, Message, Errors}`. Constructors: `BadRequest`, `FieldError(field, msg)`, `Unauthorized`, `PaymentFailed`, `Forbidden`, `NotFound`, `Conflict`, `PayloadTooLarge`, `RateLimited`, `Internal(err)`, `NewError(status, code, msg)`, `Validation(err)`. Modifiers: `.WithField(f, msg)`, `.WithCause(err)`. `httpx.AsError(err)` converts any error |
| automatic mapping | `gorm.ErrRecordNotFound` → 404, `gorm.ErrDuplicatedKey` → 409, `*http.MaxBytesError` → 413. Validator/JSON errors and any error with `FieldErrors() map[string][]string` → 400. Everything else → 500; the cause is logged, never sent |
| binding | `httpx.Bind(c, &req)`: query for GET/HEAD/DELETE, JSON otherwise; an empty body is still validated. Also `httpx.BindQuery`, `httpx.Validate(&v)`, `httpx.ParamID(c, "id")` (404 on a bad id). Use tags like `binding:"required,email,min=8,oneof=a b"`; error keys are JSON names (`items[0].name`) |
| pagination | `httpx.ParsePage(c) (page, size, offset)` (default 20, max 200); `httpx.ParseSort(c, map[apiField]column) string`; `httpx.Paginate[T](c, query, allowedSort, "id DESC") ([]T, total, err)` |
| rate limit | Middleware: `httpx.RateLimit(name, httpx.Per(10, time.Hour), burst, httpx.KeyByIP)`; key helper `httpx.KeyByQuery("token")`. Shared buckets: `l := httpx.NewLimiter(r, burst)` + `httpx.Limit(name, l, keyFn)`. In code: `l.Allow(key)`, `l.RetryAfter(key)`. Denied requests get 429 with `Retry-After` |
| body size | 1 MiB by default. Node push routes add `httpx.LargeBody(httpx.NodeBodyLimit)` (8 MiB); `httpx.SetBodyLimit(c, n)` works before the body is read |
| logging | `httpx.Logger(c)` (request-scoped slog with `request_id`), `httpx.RequestIDFrom(c)` |

Global middleware: request id → access log → security headers → CORS (only when configured) → gzip → recovery → body limit.

- The access log records the route pattern (`httpx.LogPath(c)`, i.e. `c.FullPath()`), never the query string; so do the
  `rate limited`, `request failed` and `panic recovered` lines. Unmatched requests log their literal path.
- gzip only touches compressible bodies of at least 1 KiB and skips handlers that set `Content-Encoding`. A cached body
  can keep its encoding (`httpx.GzipBytes`) and be sent with `httpx.DataPrecompressed(c, status, contentType, body, gzipped)`:
  same headers as the middleware plus `Content-Length`, no per-response compression; identity clients get `c.Data` unchanged.
- `account.Fail` / `RetryError` add `Retry-After` to the service-level 429s (code cooldown, password lockout, MFA failures).

## 3. `internal/auth`

- Middlewares: `auth.RequireUser(a)`, `auth.RequireAdmin(a)`, `auth.RequireStaff(a)` (admin or staff), `auth.OptionalUser(a)`.
  - A missing or invalid token, or a stale `ver` → 401.
  - A banned user → 403.
- `auth.CurrentUser(c) *model.User` is a request-local copy of a row cached for 30 s (`auth.LoadUser`).
  **Reload the user inside your transaction before changing balance, traffic or plan.** After changing password, ban,
  roles or uuid, call `auth.InvalidateUser(a, id)`. Also available: `auth.CurrentClaims(c)` and `auth.BearerToken(c)`.
- Tokens: `auth.Issue(a, u) (string, error)` and `auth.Parse(a, tok) (*auth.Claims, error)`. `IssueWith` / `ParseWith`
  take an explicit secret. The HS256 claims are `{sub, email, ver, iat, exp, iss: "ggboard"}`.
- Passwords:
  - `auth.HashPassword(pw)` produces argon2id.
  - `auth.VerifyPassword(hash, pw)` accepts argon2id, bcrypt `$2y$/$2a$/$2b$` and Xboard `xb$<algo>$<salt>$<hex>`; `!…` never verifies.
  - `auth.NeedsRehash(hash)` reports hashes to upgrade.
  - `auth.SetParams` / `auth.CurrentParams` (tests use cheap parameters).
- One-time login ("login as"): `auth.IssueLoginToken(a, userID) string` (5 min, single use) and
  `auth.ConsumeLoginToken(a, tok) (uint64, bool)`.

## 4. `internal/app` (dependency container)

`*app.App` fields: `Cfg *config.Config`, `DB *gorm.DB`, `Settings *settings.Service`, `Cache *cache.Cache`,
`KV *cache.TTLMap[string, any]`, `Mailer *mail.Queue`, `Log *slog.Logger`, `Logs *logx.Ring`, `Version`, `StartedAt`,
`Online *online.Store`, `Traffic *traffic.Accumulator`, `Scheduler *scheduler.Scheduler`.
Helpers: `a.DBDriver()`, `a.FlushTraffic(ctx)`, `a.Start()`, `a.Close(ctx)`,
`a.Component(name, build) any` (per-App singleton built on first use; values implementing `app.Stopper` are stopped by `Close`
after the scheduler, newest first; nil after `Close` — the notification queue lives here).

- **Cache** (ristretto, may evict):
  - `a.Cache.Get` / `Set(key, v, ttl)` / `Delete` / `Clear`, `cache.GetAs[T]`.
  - `cache.GetOrLoad[T](a.Cache, key, ttl, load)` (singleflight).
  - Namespaced keys: `a.Cache.Key(ns, key)` + `a.Cache.InvalidateNamespace(ns)`. Metrics: `a.Cache.Stats()`.
- **KV** (never evicted early; default TTL 10 min) for email codes, counters and one-time tokens:
  `Set(k, v, ttl)`, `Get`, `Take`, `Update(k, ttl, keepTTL, fn)`, `TTL`, `Delete`, `Purge`. Prefix keys with your
  package name (`account:code:…`, `mfa:token:…`).
- **Online** (`internal/app/online`):
  - Writers: `Touch(node)`, `TouchPush(node)`, `SetOnline(node, n)`, `SetLoad(node, online.Load{CPU,Mem,Swap,Disk,Uptime})`,
    `SetVersion`, `SetAliveIPs(node, user, ips)`, `SetNodeAlive(node, map[user][]ip)`.
  - Readers: `AliveIPs(user)`, `AliveCount(user)`, `AliveCountMode(user, mode)`, `AliveCounts()`, `AliveCountsMode(mode)`,
    `NodeAlive(node)`, `OnlineUserCount()`, `NodeStatus(id) (online.Status, bool)`, `Snapshot()`.
  - Maintenance: `Forget(id)`, `Purge()`.
  - State: `Status.State(now, online.Window(push, pull))` returns `StateOnline` (2), `StateIdle` (1, online, no push) or
    `StateOffline` (0). `Status.Online(now, w)` gives the online user count. Alive TTL = `max(300 s, 2 × server_push_interval)`.
- **Traffic**: `a.Traffic.Add(nodeID, userID, rate, up, down)`, flushed every 10 s and on shutdown in one transaction:
  `users.u/d += bytes × rate`, plus daily upserts of `stat_users` (× rate) and `stat_nodes` (raw). Upsert helpers for
  other code: `traffic.Excluded(db, col)` and `traffic.AddExcluded(db, table, col)` (portable `excluded.x` / `VALUES(x)`).

## 5. Settings (`internal/settings`)

- Reads: `a.Settings.String/Int/Int64/Bool/Strings/Duration(key, unit)/JSON(key, &dst)/Get(key)/All()`.
- Writes: `a.Settings.Set(map[string]any)` / `SetContext(ctx, …)` validates against `defs.go`. Unknown keys, wrong types,
  enum, option, range and `MinLen` violations become `settings.FieldErrors`, which `httpx.Fail` renders as 400. `nil` resets
  a key to its default, except the generated secrets `server_token` and `telegram_webhook_secret`, which get a fresh
  random value.
- Admin metadata: `AllGrouped()` (secrets included, hidden keys excluded) and `settings.Groups()` (UI metadata, group order
  site, register, subscribe, order, invite, server, email, frontend, app, security, telegram).
- Helpers: `settings.IsSecret(key)` (`server_token`, `email_password`, `captcha_secret`, `telegram_bot_token`,
  `telegram_webhook_secret`), `settings.Lookup(key)`, `settings.Defaults`, `settings.SortedKeys()`,
  `settings.RandomToken(n)` (alphanumeric).
- The hidden JSON key `rulesets_custom` stores admin rule-set overrides.
- Reads never hit the database: the snapshot is swapped after each successful `Set`; `Reload(ctx)` re-reads it.

## 6. Mail (`internal/mail`)

Never send mail inline. Use `a.Mailer.SendTemplate(to, mail.TplVerifyCode, mail.Data{"Code": "123456"})`.

Templates and their data keys:

- `verify_code{Code, Minutes}`
- `remind_expire{ExpiredAt, URL}`
- `remind_traffic{Percent, Used, Total, URL}`
- `order_paid{TradeNo, PlanName, Period, Amount, URL}`
- `ticket_reply{Subject, Message, URL}`: `Subject` is the ticket subject. The rendered e-mail subject is `MailSubject`,
  and it only replaces `Subject` when the caller passes none.
- `notify{Subject, Title?, Content | ContentHTML (template.HTML, sanitized), URL}`

Brand (`app_name`, `app_url`, `logo`) and locale (`default_locale`, zh-CN/en) come from settings.

Custom templates (`mail_templates` rows, docs/API.md "Mail templates"): `Renderer.Render` first looks for an override for
(name, `NormalizeLocale(brand.Locale)`) and, when there is one, renders the admin's subject and Markdown body instead of the
built-in files (`Renderer.RenderCustom`); the caller's `Data` is unchanged, and without an override the built-in path is
byte-for-byte the old one. Catalogue, validation, defaults and values are in `internal/mail/custom.go`, the Markdown pipeline
in `internal/mail/markdown.go`: `{{name}}` placeholders → opaque alphanumeric tokens (random per render, equal length) →
goldmark (`github.com/yuin/goldmark`, GFM, `html.WithUnsafe()` because the author is an admin, `html.WithHardWraps()`) → AST
pass: links with an empty address once substituted are dropped, a top-level paragraph holding only a link becomes the layout's
button (`mailButton` node), every node gets the layout's inline style attribute (code blocks through an own node renderer)
→ tokens replaced in one `strings.Replacer` pass by HTML-escaped values (`message`/`content` keep line breaks as `<br>`,
trusted `ContentHTML` is inserted raw) → the usual `layout` (`MailSubject` = the rendered subject, same header and footer).
The plain-text part is a walk of the same AST (raw HTML stripped with `golang.org/x/net/html`) with raw values, plus the
`AppName` header and `--` footer of the built-in text path. Subject: raw values, `strings.Fields`-collapsed; empty → the
built-in subject.

- `mail.Variables(name) []Variable{Name, Required}`: the catalogue (template variables, then `siteName`, `siteUrl`, `year`),
  mapped from the `Data` keys above: `Code`→`code`, `Minutes`→`minutes` (default 10), `ExpiredAt`→`expiredAt`,
  `Percent`→`percent`, `Used`→`used`, `Total`→`total`, `TradeNo`→`tradeNo`, `PlanName`→`planName`, `Period`→`period`,
  `Amount`→`amount`, ticket `Subject`→`ticketSubject`, `Message`→`message`, notify `Title` (else the built-in subject, i.e.
  the caller's `Subject`)→`title`, `Content`/`ContentHTML`→`content`, `URL`→`url` (else `AppURL`). `nil` = unknown template.
- `mail.Validate(name, subject, body) Validation{Limit, Strict, Unknown, Missing}`: `Limit` (size) fails every request,
  `Strict` (empty / multi-line subject, blank body, unknown and missing placeholders) only PUT and the test mail; `Errors()`
  merges both. `mail.SampleData(name, locale)` feeds preview and test mail; `mail.DefaultCustom(name, locale)` is the
  built-in template as Markdown (GET `default`); `mail.TemplateLocales` / `IsTemplateLocale`, `MaxCustomSubject` (255
  characters), `MaxCustomBody` (65535 bytes).
- `Queue.ReloadTemplates(ctx)` re-reads the table into the renderer (`Renderer.SetOverrides`, an atomic map swap; reloads are
  serialized). `NewQueue` calls it once (a load error is logged, the built-ins are used); the admin API calls it after every
  write (the table has at most 12 rows). `Queue.RenderCustom(to, name, locale, mail.Custom{Subject, Body}, data)` renders a
  draft in a given locale, ignoring the stored override (admin preview and test mail).

- `a.Mailer.Enqueue(mail.Message{...})` sends custom mail.
- `a.Mailer.SendNow(ctx, msg)` sends synchronously (admin test mail).
- `a.Mailer.Stats()` reports the queue.
- The queue holds 1024 messages and retries temporary failures 3 times (1 s, 2 s, 4 s) from a retry list, without holding
  up the next message; permanent ones (`mail.IsPermanent`: SMTP 5yz, bad address, errors a driver wraps with `mail.Permanent`)
  are recorded after one attempt. Every outcome is written to `mail_logs`.
- `Message.Class`: mail that anonymous requests trigger is `mail.ClassAnonymous` and is refused (`ErrQueueFull`) once 75 % of
  the queue is occupied; everything else is `ClassTransactional` (the zero value).
- The driver comes from `email_driver` (`smtp` or `log`); tests use `*mail.Memory`.

## 7. Scheduler and CLI

- Jobs: register from your package's `init()`, e.g.
  `func init() { app.RegisterJob("order_timeout", "*/1 * * * *", func(ctx context.Context, a *app.App) error { … }) }`
  (duplicate names panic).
  - Jobs start only in `ggboard serve`.
  - Status: `a.Scheduler.Status() []scheduler.JobStatus` (`name, spec, running, last_run, last_duration, last_error, next_run, runs, failures`).
  - Tests call `a.Scheduler.RunNow(name)`.
  - Built-in: `online_cleanup` (`*/5 * * * *`), `traffic_flush` (`@every 10s`). Feature jobs: `order_timeout`,
    `commission_settle` (`service/order`); `traffic_reset`, `expire_remind`, `traffic_remind`, `stats_daily`
    (`service/maintenance`, linked by `cmd/ggboard/jobs_maintenance.go`); `subscription_access_flush` (`@every 10s`) and
    `subscription_access_prune` (daily, `subscription/guard`, linked through `internal/api/client`).
- Subcommands: put `func init() { cli.Register(func(a *app.App) *cobra.Command { … }) }` in your package, plus a file in
  `cmd/ggboard/` that blank-imports it (e.g. `import.go` → `internal/importer/xboard`). `a` is initialized (config
  loaded, DB migrated) before `RunE`, unless the command sets the `cli.AnnotationNoApp` annotation.
- Command output: `p := cli.Output(cmd, a)`, then `p.Success(…)`, `p.Warning(…)`, `p.Fields(…)`, `p.Table(…)`; build text
  with `p.Bold/Faint/Accent/Good/Caution/Bad/Text(s)`, which clean untrusted text (no escape sequences reach the
  terminal). Never `fmt.Print` to stdout: the printer keeps plain, script-friendly lines for pipes and files and styles
  only terminals. Errors are returned from `RunE` (printed by `cli.Run` on stderr, exit code 1).
- Plugin packages are blank-imported the same way (`cmd/ggboard/plugins_payment.go` → `internal/payment/gateways`,
  `cmd/ggboard/plugins_notify.go` → `internal/notify/telegram`). Subscription formats are linked by `internal/api/client`.

## 8. Domain helpers

Every service is cheap to build: call `New(a)` per request. Import aliases used across the code: `usersvc`, `nodesvc`,
`mfasvc`, `ticketsvc`, `statsvc`, `plansvc`, `ordersvc`, `invitesvc`, `contentsvc`.

- **`internal/model`**:
  - all tables and enums: `OrderStatus*`, `OrderType*`, `CommissionStatus*`, `CommissionType*`, `ResetMethod`,
    `Ticket*`, `CouponType*`, the `Node*` types, `Period*`, `RouteAction*`
  - credentials: `model.UserCredentials(u, node)`, `model.NodeServerKey(node)` (preload `Parent` for child nodes),
    `model.SS2022UserKey`
  - typed protocol settings: `var s model.VlessSettings; s.Decode(n.ProtocolSettings)`, or
    `model.DecodeSettings[model.VlessSettings](m)`, or `model.NewSettingsFor(type)`; plus `model.DecodeNetwork(network, m)`
  - `model.Day(t)` for every `date` column; `u.IsAvailable(now)`
- **`service/user`** (`usersvc`):
  - lookup: `FindByID/FindByEmail/FindByToken(ctx, …)` (`ErrNotFound`)
  - `Create(ctx, email, pw, CreateOptions{…, PasswordHash})` (`ErrEmailTaken`)
  - `Authenticate(ctx, email, pw)` (`ErrInvalidCredentials`, `ErrBanned`; upgrades legacy hashes without bumping
    `password_version`)
  - `RecordLogin`, `SetPassword` (bumps the version), `ResetSecurity` (new uuid and token, bumps the version),
    `RotateToken` (new subscription token only), `SubscribeURL(u)`
  - traffic reset: `ComputeNextResetAt(u, plan, now)`, `usersvc.NextResetAt(method, expiredAt, now)`, `ResetDay(u)`
  - `AvailableUsersForGroups(ctx, groupIDs) ([]NodeUser{ID, UUID, SpeedLimit, DeviceLimit, ConnLimit}, error)`: 60 s cache; call
    `InvalidateNodeUsers()` after any user change nodes must see. The JSON of `NodeUser` is the UniProxy element
    (`ConnLimit` is `json:"-"`; only soga gets it, as `tcp_limit`)
  - `usersvc.NewToken()`, `usersvc.NormalizeEmail`
- **`service/account`**: rules shared by passport and user:
  - email codes: `SendCode`, `ConsumeCode`
  - captcha: `VerifyCaptcha`
  - login and lockout: `Login`, `VerifyLogin`, `LoginWithToken`
  - `Register` (whitelist, invites, trial plan, per-IP limit), `ResetPassword`, `ChangePassword`
  - dashboard `Stats`; views `ToUserView` / `ToAdminUserView`
  - `account.Fail(c, err)` adds `Retry-After` for `*RetryError`
- **`service/mfa`** (`mfasvc`):
  - `SealString` / `OpenString`: the TOTP-secret AES-256-GCM scheme for other secrets the owner must read again (own HKDF
    info label per use, owner id as AAD)
  - TOTP: `SetupTOTP`, `EnableTOTP`, `DisableTOTP`, `RegenerateBackupCodes`
  - password + code login: `Challenge` / `CompleteLogin`
  - passkeys: `BeginRegistration` / `FinishRegistration`, `BeginLogin` / `FinishLogin`, `ListPasskeys`, `RenamePasskey`,
    `DeletePasskey`
  - `Overview`, `AdminGate(a)`, `RPFromSettings`
- **`service/node`** (`nodesvc`):
  - lookup: `Resolve(ctx, id)` (child → effective parent P)
  - rendering (cached, with ETag): `UniProxyConfig`, `SogaNode`, `UniProxyUsers`, `SogaUsers`, `AuditRules`, `WhiteList`
  - reports: `RecordTraffic`, `RecordAlive`, `RecordLoad`, `RecordAudit`
  - status: `Status` / `Window` / `IsOnline`
  - admin validation: `CheckNode`, `ValidateSettings`, `ValidateRoute`
  - `Invalidate()` after any node, route or group write
- **`service/order`** (`ordersvc`):
  - `Create`, `Assign` (admin), `Checkout`
  - `HandleNotify` (verified gateway callback), `Paid` (idempotent fulfilment), `MarkPaid`, `Cancel`
  - `CheckCoupon`, `ComputeQuote`, `SetCommissionStatus`
  - jobs `CancelExpired` and `SettleCommissions`
  - every money change locks the user row first (`lockUser`)
- **`service/plan`** (`plansvc`, package functions taking `db`): `ListForGuest`, `ListForUser`, `GetForUser`, `ListForAdmin`,
  `HasCapacity`, `ActiveUserCount(s)`, `ValidatePrices`, `PeriodLabel`.
- **`service/ticket`** (`ticketsvc`):
  - `Create` (`ErrOpenExists`: one open ticket per user), `Get(ctx, id, ownerID)`
  - `ReplyAsOwner`, `ReplyAsStaff` (mails `ticket_reply`), `Close`
  - `CountOpen(ctx, userID)`, `Query(ctx, Filter)`, views `ForOwner` / `ForStaff`
- **`service/invite`** (`invitesvc`): `Overview`, `Stat`, `CommissionPending(ctx, userID)`, `GenerateCode`,
  `CommissionQuery`, `Transfer`, `Withdraw`.
- **`service/content`** (`contentsvc`): `NoticeQuery(ctx, onlyVisible)`, `KnowledgeQuery`, `KnowledgeIndex`, `Article`,
  `RenderBody`, `SortNotices`, `SortKnowledge`.
- **`service/stat`** (`statsvc`):
  - `PaidStatuses` (orders counted as paid: 1, 3, 4, by `paid_at`)
  - `Paid(ctx, from, to)`, `Registers`, `Overview`, `Series`
  - `NodeRanking`, `UserRanking`, `NodeCounts(ctx, now)` (three-state online count)
  - `Aggregate` / `SaveDay(ctx, day)`
- **`service/adminuser`**:
  - views: `View`, `Views` (batch-loads plans and inviters), `Detail`
  - filters; `Create` / `Update` / `Delete` with cascade
  - `ResetSecret`, `ResetTraffic`, `LoginURL`, `Generate`, `Export` (CSV), `Mail`
  - `Nullable[T]` distinguishes absent / null / value in PATCH bodies
- **`service/maintenance`**:
  - jobs `TrafficReset`, `ExpireRemind`, `TrafficRemind`, `StatsDaily`, each taking an explicit `Clock` for tests
  - `maintenance.Enqueue(ctx, a.Mailer, msg)` waits while the mail queue is full: use it for any bulk mail
- **`internal/notify`** (notifications, docs/ARCHITECTURE.md §Notifications):
  - `notify.Emit(a, ev)` never blocks (dropped + WARN when its lane is full: 1024 slots, or 256 in the priority lane with its
    own worker for `notify.IsPriority` types admin_login/subscription_leak/withdraw); `notify.EmitWait(ctx, a, ev)` for bulk
    producers; `notify.Enabled(a)` before doing extra queries for an event; `notify.Reachable(a, u)` for user channels
  - builders: `OrderNew/OrderPaid/OrderCancel(a, notify.Order{…})`, `AdminLogin(a, notify.Login{…})`,
    `TicketNew/TicketReply(a, notify.Ticket{…})`, `Withdraw(a, notify.Withdrawal{…})`, `RemindExpire`, `RemindTraffic`, `Test`;
    `SubscriptionLeak(a, u, distinct)` (+ `EmitSubscriptionLeak`, assignable to `subscription.LeakHook`); anything else:
    `notify.New(a, type, body, notify.F("key", value), …)`
  - drivers: `notify.Register(name, func(a) notify.Notifier)` (nil = not configured); `notify.Permanent(err)`,
    `notify.ErrSkipped`, `notify.Done/MarkDone` for fan-out retries; `notify.Stats(a)`
  - tests: `rec := notifytest.Attach(t, a)` then `rec.Last(t, notify.TypeOrderPaid)`; Telegram: `telegramtest.Enable(t, a)`
- **`internal/payment`**:
  - `Gateway` interface: `Name`, `Label`, `Fields`, `Pay`, `Notify`
  - `Register`, `Get`, `List`, `Names`, `FormatAmount(cents)`, `ParseAmount(s)`, `FailResponse()`
  - see docs/PAYMENTS.md
- **`internal/subscription`**:
  - `Format` (`Name`, `Supports`, `Render`; optional `ProxyFilter`, `RuleConsumer`), `RenderContext`
  - `RegisterFormat`, `GetFormat`, `FormatNames`, `DetectFormat(flag, ua)`, `SupportsAge(name)` / `AgeFormats()`
    (formats implementing `AgeCapable`)
  - `subscription.AvailableNodes(ctx, a, u)`: show + group, sort/id, child nodes, three-state status; 5 s cache per group.
    Call `subscription.InvalidateNodes(a)` after node or group writes that must show at once
  - `subscription/rulesets`: `Current(a.Settings)` (merged catalog), `ResolveProfile`, `BuildPlan`, `ValidateProfile`, `PublicView`
  - `subscription/engine`: `Generate`, `EffectiveProfile` (`?profile=` slug), profiles `LoadProfiles`, `DefaultOf`,
    `StoredProfileSlug`, `SaveProfile` (default) / `SaveProfileSlug`, `CreateProfile`, `UpdateProfileMeta`, `DeleteProfile`,
    `SetDefaultProfile`, `EnsureDefault`
  - node operations: `subscription.NodeOps`, `ApplyNodeOps`, `DetectRegion`, `RegionCode`, `RegionFlag`
  - `subscription/guard`: age keys `LoadKey`, `GenerateKey`, `SetKeyEnabled`, `DeleteKey`, `View`, `Encrypt`, `Decide`;
    access log `Record` (non-blocking), `FlushAccess`, `PruneAccess`, `ListAccess`, `MaskIP`
  - `subscription.LeakHook` (default `DefaultLeakHook`: WARN log + `notify` mail): the seam for leak notifications
  - see docs/SUBSCRIPTION.md and docs/RULESETS.md
- **`internal/captcha`**: `captcha.FromSettings(a.Settings).Verify(ctx, token, c.ClientIP())`, `captcha.Enabled(a.Settings)`.
- **`internal/plugin`**: `plugin.NewRegistry[T](kind)` for new plugin points.

## 9. Database rules

- SQLite (pure Go), PostgreSQL and MySQL must all work:
  - JSON columns are TEXT
  - use `clause.OnConflict` + `traffic.Excluded` for upserts
  - never quote identifiers by hand
  - never use dialect-specific functions without a fallback
- `show`, `key` and `match` are reserved words in MySQL, so never write them in raw SQL strings
  (`Where("show = ?", true)` breaks on MySQL). Use `Where(map[string]any{"show": true})`,
  `Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}})` (or `plansvc.OrderBySort()`) or struct conditions.
  GORM quotes the identifiers it generates.
- The driver wrapper normalizes every `time.Time` parameter to UTC on SQLite. Always pass `time.Time`, never preformatted
  strings. Use `model.Day(t)` for `date` columns.
- Do not add `default:true` to bool columns: GORM skips zero values on insert, so `false` would become `true`. Set defaults in code.
- No foreign-key constraints are created, so delete dependants explicitly. When saving a struct with loaded relations use
  `db.Omit(clause.Associations)`.
- Never reuse a model variable whose primary key is set for another `First`/`Find`: GORM adds it to the WHERE clause.
- Inside `a.DB.Transaction(func(tx *gorm.DB) error {…})` use `tx` for every query. On SQLite, writes through `a.DB`
  wait for the lock.

## 10. Tests (`internal/testutil`)

```go
a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"email_verify": true}))
r := testutil.NewRouterWith(t, a, passport.Register)            // or router.New(a) for everything
u := testutil.CreateUser(t, a, "alice@example.com")             // password testutil.Password
admin := testutil.CreateAdmin(t, a, "root@example.com")         // CreateStaff too
rec := testutil.Do(t, r, http.MethodPost, "/api/v1/passport/login", map[string]any{"email": u.Email, "password": testutil.Password}, "")
body := testutil.Decode[LoginResponse](t, rec)                   // {"data": …}; DecodeList[T] for lists
e := testutil.DecodeError(t, rec)                                // httpx.Error with Status
mails := testutil.Mails(t, a)                                    // waits for the queue; in-memory mailbox (Mailbox for the raw one)
tok := testutil.Login(t, a, admin)                               // JWT for Do(..., tok)
```

- Each test app uses a private SQLite file in `t.TempDir()` (WAL, so it behaves like production with concurrent readers)
  and cheap argon2 parameters (`testutil.FastHashing`).
- `WithConfig(fn)` tweaks the config. Other helpers: `DoWithHeaders` (adds headers), `ExpectStatus`, `Ptr`, `MustJSON`.
  MFA tests use `internal/service/mfa/mfatest`.
- Custom mail templates: unit tests use `r, _ := mail.NewRenderer(); r.SetOverrides(map[string]map[string]mail.Custom{…})`
  (no database); flow tests `PUT /api/v1/admin/mail-templates/:name` (or insert a `model.MailTemplate` row and call
  `a.Mailer.ReloadTemplates(ctx)`) and then read `testutil.Mails`.
- Run `make test` (`go test ./... -race -count=1`) and `make lint`. `make lint` uses golangci-lint v2 with
  `.golangci.yml` when installed, otherwise `go vet` + a gofmt check.

| make target | what |
| --- | --- |
| `make build` | static binary `bin/ggboard` (`CGO_ENABLED=0`, version from `git describe`) embedding `internal/static/dist` |
| `make run` | `go run ./cmd/ggboard serve` |
| `make test` | Go tests with the race detector |
| `make lint` | golangci-lint, or `go vet` + gofmt |
| `make web` | `pnpm -C web install && pnpm -C web build`, then copy `web/dist` into `internal/static/dist` |
| `make e2e` | `scripts/e2e.sh` (build, start on a random port, smoke-test `/healthz`, `/api/v1/guest/config` and `/`), then `scripts/e2e-full.sh` (the main API flows end to end; `BASE=…` runs it against a running server) |
| `make tidy` / `make clean` | `go mod tidy` / remove `bin` and the copied frontend |
