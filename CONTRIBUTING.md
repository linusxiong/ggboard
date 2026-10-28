# Contributing

**English** · [简体中文](CONTRIBUTING.zh-CN.md)

Issues and pull requests are welcome. Before you start, read [AGENTS.md](AGENTS.md) (the single authoritative set of
conventions), [MISTAKES.md](MISTAKES.md) (the mistake log) and the contract docs in `docs/` that cover your change.

## Setup

| Tool | Version | Source |
| --- | --- | --- |
| Go | ≥ 1.27.3 | `go.mod` (CI uses `1.27.x`, the Dockerfile uses `golang:1.27-alpine`) |
| Node.js | ≥ 22 (CI and the Dockerfile use 24) | `web/package.json` `engines`, `.github/workflows/ci.yml` |
| pnpm | 12 (the Dockerfile pins `pnpm@12.4.1`) | `ci.yml`, `deploy/Dockerfile` |
| golangci-lint | v2 (optional) | `.golangci.yml`; without it, `make lint` falls back to `go vet` + `gofmt` |

```bash
make run                                   # go run ./cmd/ggboard serve, data in ./data
go run ./cmd/ggboard admin create --email admin@example.com
pnpm -C web install && pnpm -C web dev     # Vite dev server; proxies /api and /s/ to http://localhost:8080
                                           # backend elsewhere: GGBOARD_API=http://127.0.0.1:9090 pnpm -C web dev
```

Until you run `make web`, the binary only holds a placeholder page ("Frontend not built"); the API works as usual.

## Make targets

| Target | What it does |
| --- | --- |
| `make help` | Lists the targets |
| `make build` | Builds `bin/ggboard` with `CGO_ENABLED=0` (embeds `internal/static/dist`; the version comes from `git describe`) |
| `make run` | Runs `serve` from source |
| `make test` | `go test ./... -race -count=1` |
| `make lint` | golangci-lint, or `go vet` + `gofmt -l` |
| `make web` | `pnpm -C web install && build`, then copies `web/dist` into `internal/static/dist` |
| `make e2e` | `scripts/e2e.sh` (builds, starts on a random port, checks `/healthz`, `GET /api/v1/guest/config` and the home page), then `scripts/e2e-full.sh` (the full API flow; set `BASE=…` to run it against a running instance and seed it with demo data at the same time) |
| `make tidy` | `go mod tidy` (**run by the integrator only**) |
| `make clean` | Deletes `bin/` and the copied frontend build |

## Frontend scripts

`pnpm -C web <script>`: `dev`, `build`, `preview`, `preview:mock` (preview with mock data, used for Lighthouse),
`typecheck` (`tsc -b`), `lint` (ESLint), `format` / `format:check` (Prettier), `test` / `test:watch` (Vitest), `lh`
(builds, then runs the two Lighthouse CI suites `lh:desktop` and `lh:mobile` in turn; all four categories must score
100), `ui:smoke` (walks every page of a running instance with puppeteer and checks for console errors, failed requests,
untranslated i18n keys and horizontal overflow; `--axe` also runs axe-core), `routes`.
`typecheck`, `lint` and `test` first run `scripts/gen-routes.mjs` to generate `src/routeTree.gen.ts` (gitignored).
The README screenshots have no pnpm script: run `node web/scripts/screenshots.mjs` against a running instance (usage is
in the comment at the top of the script and in [web/README.md](web/README.md)).

## Before you open a PR

CI (`.github/workflows/ci.yml`) runs `gofmt`, `go build ./...`, `go vet ./...` and `go test ./... -race -count=1`, plus
`pnpm -C web install --frozen-lockfile`, `lint` and `build`. CI does not run the frontend type check or unit tests, so
run them locally (the manually triggered `.github/workflows/docker.yml` builds and pushes the multi-arch image; see
[docs/DEPLOY.md](docs/DEPLOY.md)):

```bash
make lint && make test
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build
```

## Conventions

Taken from [AGENTS.md](AGENTS.md) §3, §4, §8 and §9. Where they disagree, AGENTS.md wins.

- **Contract first**: the interfaces are defined by `docs/API.md`, `docs/DATA_MODEL.md`, `docs/NODE_PROTOCOLS.md`,
  `docs/SUBSCRIPTION.md` and `docs/PAYMENTS.md`. When a contract is wrong, fix the doc first, then the code, and say so
  in the PR.
- **No stubs**: every endpoint is fully implemented and has at least one test.
- **Errors**: always return `httpx.Error` (HTTP status + `code` + `message` + optional field-level `errors`); don't panic,
  don't swallow errors, log with `slog`.
- **Security**: validate all input; store only password hashes; compare node tokens in constant time; payment callbacks
  must verify the signature; write an audit log for key operations.
- **Performance**: the node user list, subscriptions and config delivery go through the cache and support `ETag`/304;
  traffic is written in batches; no N+1 queries.
- **Units**: money is integer cents (“分”), traffic is bytes (int64), API times are RFC3339, nullable fields use `null`.
- **Routes**: each API package mounts its routes in its own `Register`, and `internal/router/router.go` calls them all.
- **Ownership**: change only the directories assigned to you. Shared files (`go.mod`, `internal/model/*`, the route
  table, `web/package.json`, navigation, shared i18n strings) are append-only; don't run `go mod tidy`; don't change
  someone else's tests to make your code pass.
- **Style**: `gofmt` / `go vet` clean; frontend ESLint + `tsc` clean. The UI follows the Apple HIG (AGENTS.md §5) and the
  performance budget (§6), uses only color tokens and loads no web fonts.
- **Commit messages**: Conventional Commits, e.g. `feat(api): ...`, `fix(web): ...`, `docs: ...`.
- **Mistake log**: every pitfall you hit (repeated compile errors, a misread contract, a misused library API, breaking
  someone else's code, a performance regression, a security issue) gets appended to [MISTAKES.md](MISTAKES.md) as
  symptom / root cause / fix / rule.
- **Secrets**: no real domains, IPs, emails, passwords or tokens in the repository. Keep local secrets in `.env`,
  `config.yaml` and `config.local.yaml` (gitignored); test data uses `example.com` and obviously fake values.
- **Bilingual docs**: user-facing docs come in pairs: `X.md` in English (the default, linked first) and `X.zh-CN.md` in
  Simplified Chinese, with the same sections in the same order and the same section numbers. A change to one goes into
  the other in the same commit. The pairs are README, CHANGELOG, CONTRIBUTING, SECURITY, `docs/DEPLOY`,
  `docs/ADMIN_GUIDE`, `docs/USER_GUIDE`, `docs/MIGRATION_TERABIX` and `docs/perf/README`. The release notes come from
  the `## [X.Y.Z]` section of both `CHANGELOG.md` and `CHANGELOG.zh-CN.md`, and the release workflow fails if either is
  missing. The contract and developer docs (`docs/API.md`, `docs/DATA_MODEL.md`, `docs/ARCHITECTURE.md`, …,
  `web/README.md`) are English only.

## Tests

Go tests use `internal/testutil`:

- `NewTestApp(t, ...)`: one SQLite database per test in a temporary directory, with argon2id turned down to cheap
  parameters; customise it with `WithSettings` and `WithConfig`. `Mailbox(t, a)` returns the in-memory mail driver.
- `NewRouterWith(t, a, pkg.Register...)` assembles the engine; `CreateUser` / `CreateAdmin` / `CreateStaff` / `Login`
  create accounts and get tokens; `Do` / `DoWithHeaders` send requests; `Decode`, `DecodeList`, `DecodeError` and
  `ExpectStatus` assert.
- `GGBOARD_TEST_LOG=1 go test ./...` prints the test app's logs to stderr.
- Subscription formats use golden files (`internal/subscription/formats/testdata/*.golden`). After an intended change to
  the output, run `go test ./internal/subscription/formats -update` to regenerate them, and review the diff.

## Adding a plugin

A plugin is a new package or file that registers itself in `init()` and is linked into the binary by a blank import. The
existing blank imports:

| File | What it links |
| --- | --- |
| `cmd/ggboard/plugins_payment.go` | `internal/payment/gateways` (its `all.go` in turn imports each gateway) |
| `cmd/ggboard/plugins_notify.go` | `internal/notify/telegram` (the Telegram notification channel) |
| `cmd/ggboard/wire_leak.go` | Not a blank import: sets `subscription.LeakHook` to "default mail + `notify.EmitSubscriptionLeak`" |
| `cmd/ggboard/import.go` | `internal/importer/xboard` (the `ggboard import` subcommand) |
| `cmd/ggboard/jobs_maintenance.go` | `internal/service/maintenance` (scheduled jobs) |
| `internal/subscription/engine/engine.go`, `internal/api/client/routes.go` | `internal/subscription/formats` |

### Payment gateway

The interface is in `internal/payment/gateway.go`:

```go
type Gateway interface {
    Name() string    // registry id, e.g. "epay"
    Label() string   // human name
    Fields() []Field // admin form schema
    Pay(ctx context.Context, cfg map[string]string, req PayRequest) (*PayResult, error)
    Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*NotifyResult, *NotifyResponse, error)
}
```

1. Create `internal/payment/gateways/<name>/`, implement the interface, and add
   `func init() { payment.Register(&Gateway{}) }`. Signing, amount conversion and HTTP calls can reuse
   `internal/payment/gateways/gwutil`.
2. Add a blank import to `internal/payment/gateways/all.go`.
3. `Notify` must verify the signature: when verification fails, return `(nil, nil, err)` (the caller answers 400
   `fail`); when it is verified but is not a successful payment, return `(nil, resp, nil)`.
4. The admin gateway form is rendered straight from `Fields()` (`GET /api/v1/admin/payments/gateways`); no frontend
   change is needed.
5. Test the `Pay` request and the `Notify` signature check with httptest (valid signature, bad signature, replay, amount
   mismatch), and document the protocol in [docs/PAYMENTS_GATEWAYS.md](docs/PAYMENTS_GATEWAYS.md).

### Subscription format

The interface is in `internal/subscription/format.go`:

```go
type Format interface {
    Name() string
    Supports(nodeType string) bool
    Render(ctx *RenderContext) (body []byte, contentType string, err error)
}
```

Optional: implement `ProxyFilter` (`SupportsProxy(*Proxy) bool`, drops nodes based on transport/encryption details),
`RuleConsumer` (`RuleFamily() string`, outputs rule sets) and `AgeCapable` (`SupportsAge() bool`: return true when the
client can decrypt age-encrypted subscriptions; today that is only clash-meta; see docs/SUBSCRIPTION.md).

1. Create a file in `internal/subscription/formats/` with `func init() { subscription.RegisterFormat(myFormat{}) }`.
2. Add a name constant in `format.go`, and register the format in `detect.go` in `formatNames` (the names `?flag=`
   accepts) and `detectRules` (User-Agent detection). If you don't, the format is never selected.
3. Add a golden case to `formats_test.go` and update [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md).

### Captcha

The interface is in `internal/captcha/captcha.go`:

```go
type Verifier interface {
    Verify(ctx context.Context, token, remoteIP string) error
}
type Factory func(secret string, client *http.Client) Verifier

func Register(name string, f Factory)
```

1. Call `captcha.Register("<name>", factory)` in `init()`; services that speak the siteverify protocol can reuse
   `captcha.SiteVerify` as is.
2. Add the name to the `captcha_provider` options in `internal/settings/defs.go` (writing the setting checks it against
   the options).
3. Register the frontend script and global object in `PROVIDERS` in `web/src/components/Captcha.tsx`.
4. Register the service's script / frame sources (and connect / style if it needs them) in `captchaSources` in
   `internal/static/static.go`: `static.CSPFor` only allows the one CAPTCHA service that is currently enabled, and the
   other pages keep the strict default `CSP`.

### Mail driver

The interface is in `internal/mail/mailer.go`:

```go
type Mailer interface {
    Send(ctx context.Context, msg Message) error
}
```

Mail drivers are **not a registry**: the built-in `smtp` and `log` are picked by a `switch` in `mail.FromSettings`. A new
driver needs:

1. an implementation of `Mailer`;
2. a new case in `FromSettings`;
3. its name in the `email_driver` options in `defs.go`, plus any settings keys it needs.

All mail is delivered asynchronously through `app.Mailer` (`internal/mail/queue.go`); don't send mail synchronously in a
request handler.

### Notification channel

The interface is in `internal/notify/notify.go` (Telegram is the built-in implementation; see `internal/notify/telegram`):

```go
type Notifier interface {
    Name() string
    SendAdmin(ctx context.Context, ev Event) error                // administrator destinations; ErrSkipped when not subscribed
    SendUser(ctx context.Context, chatRef string, ev Event) error // one user, addressed by UserAddresser.ChatRef
}
type UserAddresser interface{ ChatRef(u *model.User) string }   // optional: "" = user not reachable
type Factory func(a *app.App) Notifier                           // nil while the channel is not configured

func Register(name string, f Factory)
```

1. Create `internal/notify/<name>/` and call `notify.Register("<name>", factory)` in `init()`. The factory is called for
   every event, so it must be cheap and read the current settings (return nil while the channel is not configured, and
   `notify.Emit` then creates no queue for it).
2. Add a blank-import file in `cmd/ggboard/` (see `plugins_notify.go`).
3. Wrap permanent failures (e.g. a 4xx) in `notify.Permanent(err)` to avoid pointless retries; when fanning out to
   several chats, use `notify.Done` / `notify.MarkDone` so a retry only resends to the chats that failed; append the
   settings keys you need to a new group in `internal/settings/defs.go`.
4. Tests: `notifytest.Attach(t, a)` records events; see the fake Bot API server in
   `internal/notify/telegram/telegramtest`. Event types and fields are in [docs/API.md](docs/API.md) §Telegram and
   [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) §Notifications.

### Frontend effects and icons

- **Effects must be original**: they live in `web/src/components/effects/` (CSS keyframes, IntersectionObserver, rAF),
  load lazily and respect `prefers-reduced-motion`. React Bits (reactbits.dev) is licensed MIT + Commons Clause, which is
  incompatible with GPL-3.0: **do not copy its source**; use it only as a visual reference (AGENTS.md §1 item 10,
  MISTAKES.md #107).
- **Payment preset icons**: a new preset is one original SVG component in
  `web/src/components/icons/payments/glyphs.tsx`, with its id (`[a-z0-9_-]{1,32}`) registered in `PAYMENT_ICON_IDS` and
  the preset table in `presets.ts`; the backend accepts any well-formed `preset:<id>`, so no Go change is needed. The
  default icon of a new gateway is mapped in `defaultIconPreset` in `web/src/features/admin/payments/gateways.ts`.
  `icons/payments/` may only be imported by the orders page and the admin payments page, and must not go into the
  `chrome` chunk in `vite.config.ts` that public pages load too (MISTAKES.md #143).

### Other hooks

- **CLI subcommands**: `cli.Register(func(a *app.App) *cobra.Command { ... })`. Before the command runs, the App has
  loaded the config and run the migrations (set the annotation `ggboard/no-app: "true"` to skip that). See
  `internal/importer/xboard/cli.go`.
- **Scheduled jobs**: `app.RegisterJob(name, cronSpec, func(ctx context.Context, a *app.App) error { ... })`, called in
  `init()`. A duplicate name panics; jobs only run under `serve`. See `internal/service/maintenance`.
- **Data importers**: see `internal/importer/xboard`; they are mounted as a CLI subcommand.

## License

By contributing, you agree to release your code under [GPL-3.0](LICENSE).
