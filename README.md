<p align="center">
  <img src="web/public/favicon.svg" width="88" alt="GG Bond" />
</p>
<h1 align="center">GG Bond · ggboard</h1>
<!-- screenshots: see docs/screenshots -->
<p align="center">A simple, good-looking, fast proxy panel: one Go binary with an embedded React frontend.</p>

<p align="center">
  <a href="https://github.com/linusxiong/ggboard/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/linusxiong/ggboard"></a>
  <a href="LICENSE"><img alt="GPL-3.0" src="https://img.shields.io/badge/license-GPL--3.0-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white">
  <img alt="Lighthouse" src="https://img.shields.io/badge/Lighthouse-100%20%C2%B7%20100%20%C2%B7%20100%20%C2%B7%20100-brightgreen">
</p>

<p align="center"><b>English</b> · <a href="README.zh-CN.md">简体中文</a></p>

> **Status: stable since 1.0** — changes per version are in [CHANGELOG.md](CHANGELOG.md), binaries and images on
> [Releases](https://github.com/linusxiong/ggboard/releases). Back up `data/` before upgrading.

---

## Features

- **Node backends**: XrayR / V2bX (the `UniProxy` API) and soga (the `soga-v1` webapi).
- **Every protocol**: vmess, vless (including REALITY / XHTTP), shadowsocks (including 2022), shadowsocksr, trojan,
  hysteria 1/2, anytls, mieru, tuic.
- **Routing users choose themselves**: in the panel, users pick a template (None / Minimal / Standard / Full), a rule-set
  source (blackmatrix7, Loyalsoldier, MetaCubeX, SagerNet, ACL4SSR, DustinWin), categories (OpenAI, Claude, Telegram,
  Netflix…), policy groups and a GitHub mirror. One subscription link serves mihomo / Clash / Stash / sing-box / Surge /
  Surfboard / Quantumult X / Shadowrocket / Loon / v2rayN and more.
- **Subscription profiles and node operations** (modelled on Sub-Store): up to 10 profiles per user, each with its own
  link (`?profile=<slug>`), routing rules and node pipeline — filter by type / region (47 regions, detected by flag →
  name → code) / regex, dedupe, regex rename, flag emoji, prefix and suffix, sort, count limit — with a live preview.
- **Subscription leak protection**: **age-encrypted subscriptions** for mihomo 1.19.27+ (Clash Party and FlClash take
  the private key directly) with an optional strict mode; a subscription access log (IP and location, client, format;
  locations come from the CZ88 IP database, updated daily); email + Telegram alerts, or automatic link rotation, when one
  token is used from too many IPs within 24 hours. Rotating only the link is separate from resetting the subscription.
- **Telegram bot** (a pluggable notification channel): new orders / payments / cancellations, new tickets / replies,
  withdrawal requests and subscription leaks go to admins; **every admin and staff sign-in sends an alert** (email,
  method, IP, user agent, time). Users bind with a one-time code to get their own payment, ticket-reply and expiry /
  traffic reminders.
- **Real payments, pluggable gateways**: EPay, Epusdt/BEpusdt, Alipay Face-to-Face, Stripe Checkout, Coinbase Business,
  BTCPay Server. A new gateway is one Go interface plus a registration. Payment methods use one of 16 original preset
  icons (Alipay, WeChat Pay, USDT, Stripe…) or a custom image / emoji.
- **Account security**: argon2id passwords, TOTP two-factor authentication with backup codes, passkeys (WebAuthn,
  including one-tap sign-in without a username), and full autofill support for 1Password / Bitwarden / iCloud Keychain
  and other password managers. Registration can require an email code, allow only some email domains, or be closed;
  Turnstile / reCAPTCHA / hCaptcha are optional; 2FA can be made mandatory for admins.
- **Migration from Xboard**: one command imports users, plans, nodes, coupons, tickets, announcements, knowledge base,
  payment settings and site settings (past orders are not imported). Both the legacy per-type tables and upstream's
  unified table are supported, from MySQL / PostgreSQL / SQLite.
- **Operations**: plans / orders / coupons / commissions and invites / tickets / announcements / knowledge base /
  statistics / scheduled jobs / email reminders.
- **Frontend**: React 19 + HeroUI v3 + TanStack (Router / Query / Table / Form) + zustand, in the Apple HIG design
  language (system colors, iOS segmented controls, grouped forms, a bottom save bar). Light and dark follow the system;
  the UI is in Chinese and English; every motion component is original. Public pages are prerendered at build time and
  score 100 in all four Lighthouse categories, on desktop and mobile.
- **No extra services to deploy**: runs on SQLite out of the box, with no Redis or queue workers. PostgreSQL and MySQL
  are supported too: `ggboard db copy` copies existing data across and verifies it row by row (and copies it back), see
  [DEPLOY.md §5](docs/DEPLOY.md#moving-from-sqlite-to-postgresql).

## Performance

One process, the default SQLite, 10,000 users and 50 nodes. The load generator runs on the same machine as the panel,
over loopback (no TLS, no reverse proxy), with 64 connections per scenario (16 for sign-in). The numbers come from one
real run, and `make bench` reproduces them.

<p align="center"><img src="docs/perf/benchmark.svg" width="860" alt="Requests per second and p99 latency per scenario"></p>

| Scenario | req/s | p99 |
| --- | ---: | ---: |
| Subscription · v2ray (base64) | 13,208 | 19 ms |
| Subscription · mihomo + standard routing rules (18 rule sets) | 13,322 | 18 ms |
| Node fetches the user list, ETag hit → 304 | 86,836 | 2.5 ms |
| Node fetches the full user list (10,000 users, gzip) | 29,794 | 6.1 ms |
| Node reports traffic (1,000 users per report, about 4.28 million rows/s) | 4,282 | 24 ms |
| Password sign-in (argon2id, 64 MiB, deliberately slow) | 104 | 164 ms |

Machine: Apple M5 Pro (18 cores) · 48 GB · macOS 27.0.1 · Go 1.27.3 · 2026-10-23 · commit `8db6777`.
Method, every scenario (static files included), caveats and findings: [docs/perf/README.md](docs/perf/README.md).

## Screenshots

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/landing-light.png" alt="Landing page (light)"></td>
    <td width="50%"><img src="docs/screenshots/landing-dark.png" alt="Landing page (dark)"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/login-light.png" alt="Sign in"></td>
    <td><img src="docs/screenshots/dashboard-light.png" alt="User dashboard"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/subscription-profiles-light.png" alt="Subscription profiles: link, one-click import and segmented tabs"></td>
    <td><img src="docs/screenshots/subscription-nodeops-light.png" alt="Node operations with live preview"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/subscription-rules-light.png" alt="Rule builder"></td>
    <td><img src="docs/screenshots/subscription-age-light.png" alt="Subscription security (access log, link rotation) and age encryption"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/shop-light.png" alt="Shop"></td>
    <td><img src="docs/screenshots/orders-light.png" alt="Orders with segmented filter"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/order-light.png" alt="Order detail"></td>
    <td><img src="docs/screenshots/admin-dashboard-light.png" alt="Admin dashboard"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-node-editor-light.png" alt="Admin node editor (VLESS REALITY)"></td>
    <td><img src="docs/screenshots/admin-payments-icons-light.png" alt="Payment methods and preset icons"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-settings-telegram-light.png" alt="Settings: Telegram"></td>
    <td><img src="docs/screenshots/admin-settings-register-light.png" alt="Settings: registration and sign-in"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/dashboard-dark.png" alt="User dashboard (dark)"></td>
    <td><img src="docs/screenshots/subscription-profiles-dark.png" alt="Subscription profiles (dark)"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-dashboard-dark.png" alt="Admin dashboard (dark)"></td>
    <td><img src="docs/screenshots/admin-node-editor-dark.png" alt="Admin node editor (dark)"></td>
  </tr>
</table>

All screenshots (1440×900, light and dark) are in [`docs/screenshots/`](docs/screenshots/). The base scenes come from
`node web/scripts/screenshots.mjs` run against a demo instance seeded by `scripts/e2e-full.sh`; the profile, node
operations, age, order filter, payment icon, Telegram and registration shots were added the same way. The UI in the
screenshots is in Chinese; only `example.com` demo accounts appear, and subscription tokens and secrets are masked.

## Quick start

### Docker Compose · prebuilt image (recommended: no clone, no build)

```bash
mkdir ggboard && cd ggboard
curl -fsSLO https://raw.githubusercontent.com/linusxiong/ggboard/main/deploy/docker-compose.prebuilt.yml
curl -fsSL https://raw.githubusercontent.com/linusxiong/ggboard/main/.env.example -o .env
echo 'GGBOARD_VERSION=1.0.1' >> .env       # image tag: pin a release; main is a rolling build of the main branch
docker compose -f docker-compose.prebuilt.yml up -d
docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
```

Without `--password`, a random password is generated and printed once. `.env` must sit in the same directory as the
compose file; an empty `GGBOARD_JWT_SECRET` is generated for you. The panel listens on `127.0.0.1:8080` only: put
nginx / Caddy in front with HTTPS (see [docs/DEPLOY.md](docs/DEPLOY.md)). The
[release workflow](.github/workflows/release.yml) builds an image (amd64 + arm64) for every `v*` tag and publishes it to
`ghcr.io/linusxiong/ggboard` (for example `1.0.0`, `1.0`, `1`, `latest`). To upgrade, change `GGBOARD_VERSION`, then
`pull` and `up -d`.

### Docker Compose · build from source

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env            # empty GGBOARD_JWT_SECRET is generated; if you set it, use 32+ random bytes (openssl rand -hex 32)
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com --password 'change-me'
```

The panel listens on `127.0.0.1:8080`: put nginx / Caddy in front with HTTPS (example: `deploy/nginx.example.conf`) and
add the proxy's address to `server.trusted_proxies`. Full deployment (both Compose setups, systemd, TLS, backups,
upgrades, PostgreSQL / MySQL) is in [docs/DEPLOY.md](docs/DEPLOY.md).

### Binary · release

The [Releases](https://github.com/linusxiong/ggboard/releases) page has archives for linux / darwin (amd64, arm64) and
windows/amd64, plus `SHA256SUMS`. The binaries are statically linked with the frontend embedded. Downloading, verifying
and installing as a systemd service is covered in
[docs/DEPLOY.md §2](docs/DEPLOY.md#release-binaries).

### Binary · from source

```bash
make web && make build          # build the frontend and embed it; output: bin/ggboard
./bin/ggboard admin create --email admin@example.com --password 'change-me'
./bin/ggboard serve             # :8080 by default, data in ./data
```

Configuration comes from `config.yaml` (see `config.example.yaml`) or `GGBOARD_*` environment variables. Runtime
settings such as the site name, mail and payments are edited under Settings in the admin console.

## Connecting node backends

The `server_token` is under **Settings → Node connection** in the admin console. Create the node on the **Nodes**
page first to get its node ID. A node can also have its own **Node token**, which then replaces `server_token` for
that node only (recommended when nodes are run by different people; see the [admin guide](docs/ADMIN_GUIDE.md#3-adding-nodes)).

**XrayR** (panel type `NewV2board`, i.e. the `UniProxy` API):

```yaml
Nodes:
  - PanelType: "NewV2board"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: "<server_token>"
      NodeID: 1
      NodeType: Vless        # V2ray | Vless | Trojan | Shadowsocks
      EnableVless: true      # vless nodes only
```

**V2bX** (the same `UniProxy` API, plus hysteria / hysteria2 / tuic / anytls):

```json
{ "Core": "sing", "ApiHost": "https://panel.example.com", "ApiKey": "<server_token>", "NodeID": 1, "NodeType": "vless" }
```

**soga** (protocol `soga-v1`; the `/soga/` prefix is recommended):

```bash
soga config type=soga-v1 server_type=vless node_id=1 webapi_url=https://panel.example.com/soga/ webapi_key=<server_token>
```

Which protocols each backend supports and full config snippets: [docs/ADMIN_GUIDE.md](docs/ADMIN_GUIDE.md). API details:
[docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md).

## Migrating from Xboard

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # rehearse first
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

The import keeps primary keys, password hashes (bcrypt / md5 / sha256 all still sign in and are upgraded to argon2id),
subscription tokens, UUIDs and users' `telegram_id`, so existing subscription links keep working unchanged. A
`server_token` of at least 16 characters is imported as is, so node backends need no change either; a shorter one is not
imported, ggboard keeps the token it generated, and the import report tells you to copy it into every node backend. Xboard's Telegram settings are not imported: after the migration, enter the same bot's token
under **Settings → Telegram** and click "Set webhook". Details and a post-migration checklist are in
[docs/IMPORT_XBOARD.md](docs/IMPORT_XBOARD.md); the full command sequence of one real cutover is in
[docs/MIGRATION_TERABIX.md](docs/MIGRATION_TERABIX.md).

## Docs

Language column: **EN + ZH** means there is also a Simplified Chinese copy (`*.zh-CN.md`, linked at the top of each doc); **EN** and **ZH** mean the doc exists in that language only.

| Doc | Contents | Language |
| --- | --- | --- |
| [AGENTS.md](AGENTS.md) | Project conventions, tech stack, design rules, plugin points | ZH |
| [CHANGELOG.md](CHANGELOG.md) | Changes per version (also the GitHub release notes) | EN + ZH |
| [docs/DEPLOY.md](docs/DEPLOY.md) | Deployment: Docker Compose, systemd, nginx / Caddy, configuration reference, backups and upgrades | EN + ZH |
| [docs/ADMIN_GUIDE.md](docs/ADMIN_GUIDE.md) | Admin guide: settings, node backends, plans and payments (preset icons), Telegram bot, leak protection, rule sets, 2FA policy | EN + ZH |
| [docs/USER_GUIDE.md](docs/USER_GUIDE.md) | User guide: one-click import per client, profiles and node operations, age encryption, access log, Telegram binding, 2FA / passkeys, password managers | EN + ZH |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Architecture, boot sequence, caching, scheduled jobs, security | EN |
| [docs/API.md](docs/API.md) | Every HTTP endpoint | EN |
| [docs/DATA_MODEL.md](docs/DATA_MODEL.md) | Data model | EN |
| [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md) · [docs/RULESETS.md](docs/RULESETS.md) | Subscription formats, profiles, node operations and the region table, age encryption and leak detection, rule-set catalog and templates | EN |
| [docs/PAYMENTS.md](docs/PAYMENTS.md) · [docs/PAYMENTS_GATEWAYS.md](docs/PAYMENTS_GATEWAYS.md) | Payment plugin interface and each gateway's wire protocol | EN |
| [docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md) | XrayR / V2bX / soga protocols | EN |
| [docs/IMPORT_XBOARD.md](docs/IMPORT_XBOARD.md) · [docs/MIGRATION_TERABIX.md](docs/MIGRATION_TERABIX.md) | Xboard importer spec and migration checklist; the runbook of one real cutover | EN · EN + ZH |
| [docs/BACKEND.md](docs/BACKEND.md) · [docs/FRONTEND.md](docs/FRONTEND.md) · [web/README.md](web/README.md) | Developer guides | EN |
| [docs/perf/README.md](docs/perf/README.md) | Benchmark method, every scenario, caveats and findings | EN + ZH |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contributing: setup, conventions, adding plugins (payments, subscription formats, CAPTCHA, notification channels) | EN + ZH |
| [SECURITY.md](SECURITY.md) | Security policy: reporting vulnerabilities and a hardening checklist | EN + ZH |
| [docs/REVIEW_2026-10-22.md](docs/REVIEW_2026-10-22.md) | Backend security and correctness review | EN |
| [docs/E2E_2026-10-22.md](docs/E2E_2026-10-22.md) | End-to-end integration run: defect table and Lighthouse scores | EN |
| [MISTAKES.md](MISTAKES.md) | Lessons learned: mistakes made during development | ZH |

## Development

```bash
make test                       # Go unit tests (-race)
make lint                       # golangci-lint
pnpm -C web install && pnpm -C web dev      # frontend dev server, /api proxied to :8080
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build   # build prerenders public pages (needs Chrome)
pnpm -C web lh                  # Lighthouse, desktop + mobile: all four categories must score 100
make e2e                        # smoke test + full API end-to-end run (scripts/e2e-full.sh; BASE=… seeds demo data into a running instance)
make bench                      # load test: starts a throwaway instance with 10,000 users, writes docs/perf/ (results.json, results.md, benchmark.svg)
```

## Credits

- IP location data: the [CZ88 (纯真)](https://www.cz88.net/) community IP database, published weekly as `qqwry.dat` by
  [metowolf/qqwry.dat](https://github.com/metowolf/qqwry.dat). The panel downloads it into the data directory at run
  time; neither the repository nor the images contain the data file.

## License

[GPL-3.0](LICENSE). All code, including the motion components in `web/src/components/effects/`, is original, with no
third-party license restrictions.
