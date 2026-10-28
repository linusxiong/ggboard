# Changelog

**English** · [简体中文](CHANGELOG.zh-CN.md)

Notable changes to GG Bond (ggboard), per version. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). When a `v*` tag is pushed, its GitHub release notes are that version's section from this file followed by the same section from [CHANGELOG.zh-CN.md](CHANGELOG.zh-CN.md) (see `.github/workflows/release.yml`), so write the version in both files before tagging.

## [Unreleased]

### Changed

- The README and the user-facing docs (deployment, admin and user guides, the Xboard cutover runbook, the benchmark write-up, contributing guide, security policy and this changelog) now come in two files each: English in `X.md`, shown by default, and Simplified Chinese in `X.zh-CN.md`, linked from the top of each. Release archives ship both READMEs, and release notes carry the English and the Chinese section of the changelog.

### Fixed

- The bulk mail form no longer says the content supports Markdown: it is sent as plain text, with line breaks kept.
- The settings link on the Nodes page and the checklist of the Xboard import report now name the settings tab as it is called, Settings → Node connection.
- Docs: an Xboard `server_token` is kept on import only when it has at least 16 characters; suspected-leak alerts also reach users on Telegram when they have bound it; the admin guide names the leak action as the console does.

## [1.0.1] - 2026-10-28

A patch release focused on security hardening, with two small features. Neither the configuration nor the database schema changed: swap the image or binary and restart.

### Added

- The subscription access log shows IP locations (`中国 广东 深圳 电信`, in both the admin panel and the user dashboard). The data comes from the CZ88 IP database (metowolf/qqwry.dat, about 27 MB, IPv4 only), stored in `<data_dir>/geoip/` and checked for updates every day at 05:23 (`geoip_update`); when a download fails or the file is invalid, the old database is kept. Under Settings → Subscription → IP locations you can turn it off, change the mirror URL, see the version and run “Update now” (`GET /admin/geoip`, `POST /admin/geoip/update`).
- After an admin resets a user's subscription secret (the “Reset subscription secret” button in the user details, or “Rotate link” on the Subscription tab), the user gets an email, and a Telegram message if they have linked Telegram, saying that their subscription link and UUID were reset, the old link no longer works and they were signed out, and that they should sign in to the website again to get the new link. The notice does not contain the new link. New: a `subscription_reset` email template that can be customised on the **Email templates** page, and a user-only Telegram event `subscription_reset`. If queueing the email fails, only a WARN is logged and the reset still goes through. Users rotating or resetting their own subscription, and the automatic rotation by leak detection, do not send this notice.

### Fixed

- The traffic ranking date follows the server's statistics day (the process time zone, `TZ`); the default and the upper limit no longer come from the browser's time zone. `GET /admin/stats/overview` adds `today.date` and `timezone`, and the ⓘ next to the ranking title shows the server time zone.
- The order list's date filter sends the start and end instants of whole days in the admin's local time zone, so the filtered results match the times shown in the list.

### Security

- Security changes take effect on in-flight requests immediately: after a password change, a subscription reset, a ban or an admin demotion, a request that has already passed authentication rechecks the account (UUID, email, security version and role) before it writes or responds, and a stale session always gets 401. Orders, tickets, invites and commissions, subscription profiles, two-factor authentication, passkeys, age keys, Telegram linking and all admin endpoints no longer read or write account data under the same user ID once that account has been replaced.
- If the token, plan, group, traffic or ban status changes while a subscription is being generated, the result is discarded and 403 is returned; stale node configuration is no longer delivered.
- Payment callbacks: every successful callback must carry a positive amount that matches the amount due on the order. Stripe and Coinbase are checked at the current exchange rate and in their currency (no longer skipped when the rate is not 1:1). If the order's quote is changed concurrently after the signature check and before fulfilment, the order is not fulfilled. EPay strictly checks the callback URL and rejects merchant request fields, so an order-creation signature can no longer be used as a successful payment callback when `app_url` is not set.
- Integer overflow: very large plan prices, discounts, traffic (the GB conversion) and sums of used traffic no longer wrap around to negative numbers, so they can neither be treated as free orders nor give a user who has used up their traffic access to nodes again.
- Unmatched routes (404) are recorded as `/[unmatched]` in the access log, so a secret in a mistyped subscription or webhook URL is never written to the log.
- The Xboard importer quotes table names read from the source database's metadata as identifiers instead of splicing them into SQL.
- `golang.org/x/net` upgraded to v0.62.0 (fixes two vulnerabilities in v0.61.0; ggboard's code paths are not affected).

## [1.0.0] - 2026-10-25

The first stable release: a single Go binary with the React frontend, scheduled jobs, mail queue and in-process cache built in. SQLite by default, PostgreSQL and MySQL also supported; no Redis or separate worker needed.

### Node backends

- XrayR (`PanelType: NewV2board`, i.e. the `UniProxy` API), V2bX (a superset of the same API) and soga (the `soga-v1` webapi, `/soga/` prefix recommended). Nodes authenticate with a `server_token` generated in the admin panel.
- Protocols: vmess, vless (including REALITY / XHTTP), shadowsocks (including 2022), shadowsocksr, trojan, hysteria 1/2, tuic, anytls, mieru.
- The user list and node configuration are cached and support `ETag` / 304; the gzipped response is cached along with them. Traffic reports are written in batches.
- Per-user speed limit, device limit (V2bX, soga) and connection limit `conn_limit` (sent to soga as `tcp_limit`), set on a plan or on a user.

### Subscriptions

- One subscription link outputs mihomo (Clash Meta), Clash, Stash, sing-box, Surge, Surfboard, Quantumult X, Shadowrocket, Loon, v2rayN (base64) and SSR formats, chosen by the `flag` parameter or the client's User-Agent; the user page offers one-click import for each client.
- Routing chosen by the user: template (None / Minimal / Standard / Full), rule-set source (blackmatrix7, Loyalsoldier, MetaCubeX, SagerNet, ACL4SSR, DustinWin), categories (OpenAI, Claude, Telegram, Netflix…), policy groups and GitHub mirror. When the chosen source has no format for a client, another source is used automatically, per category, and the editor lists the source each client actually uses; sing-box 1.14+ downloads rule sets through `http_clients`. Admins can override the built-in rule-set catalog in the admin panel.
- Multiple subscription profiles (up to 10 per user, each with its own link `?profile=<slug>`) and node operations modelled on Sub-Store: filter by type / region / regex, dedupe, regex rename, flag emoji, prefix and suffix, sort, count limit, with a live preview.
- Subscription leak protection: age-encrypted subscriptions for mihomo 1.19.27+ (with an optional strict mode); a subscription access log (IP, client, format); email + Telegram alerts or automatic link rotation when one token is used from more IPs than allowed within 24 hours; “Rotate link” (the link only) is separate from “Reset subscription”.
- Shadowsocks 2022: with AES ciphers the key is delivered as `server-key:user-key`, with ChaCha20 ciphers only the user key is delivered; both sing-box and mihomo load them.

### Plans and payments

- Plans priced per billing period (including one-time plans and traffic reset packs), orders, coupons (fixed amount or percentage, can be generated in bulk), invite rewards and commissions (withdrawn or moved to the balance).
- Pluggable payment gateways, every callback signature-checked: EPay, Epusdt / BEpusdt, Alipay Face-to-Face, Stripe Checkout, Coinbase Business, BTCPay Server. A payment method can use one of 16 original preset icons or a custom image / emoji. Amounts are stored in cents (分) and entered in yuan (元) in the admin panel.

### Accounts and security

- Passwords use argon2id; bcrypt / md5 / sha256 hashes imported from Xboard can still sign in and are upgraded at sign-in. `auth.argon2_concurrency` limits concurrent argon2id computations, so memory during a login peak has an upper bound.
- Two-factor authentication (TOTP + backup codes, can be made mandatory for admins) and passkeys (WebAuthn, sign-in without a username, multiple devices managed).
- The sign-in, registration, password recovery and password change forms work with the autofill of 1Password, Bitwarden / Vaultwarden, iCloud Keychain and Chrome; `/.well-known/change-password` and `/.well-known/passkey-endpoints` are served.
- Registration: email verification code, a registration switch, an email domain allowlist, optional Turnstile / reCAPTCHA / hCaptcha.
- Sessions are bound to the account email, and old sessions stop working after a password change or reset; startup refuses a `jwt.secret` that is too short or a placeholder; sign-in, registration and sending verification codes are rate limited per IP.
- Telegram bot: new orders / payments, tickets, withdrawal requests and subscription leaks are sent to admins, and every admin / staff sign-in sends an alert (email, method, IP, UA, time); users who link Telegram receive their own payments, ticket replies and expiry / traffic reminders.

### Admin panel

- Dashboard and statistics; user management: multi-condition filters, bulk generation, bulk email, CSV export, “Log in as user”, reset subscription or traffic, ban.
- Nodes (drag-and-drop ordering), permission groups, routes, plans (with “Apply to current users”, i.e. `force_update`), orders, coupons, payment methods, tickets, announcements and knowledge base (Markdown, with paid-only content and subscription URL placeholders).
- Email templates: the subject and Markdown body of the six system emails can be rewritten separately for Simplified Chinese / English, with variables and a live preview.
- System: runtime status, scheduled jobs, mail queue, cache and the latest 1000 log lines.
- Frontend: React 19, HeroUI v3, TanStack (Router / Query / Table / Form / Virtual) and zustand; Apple HIG design language, automatic light / dark switching, Chinese / English; explanations go into ⓘ popovers; public pages are prerendered at build time and score 100 in all four Lighthouse categories (desktop and mobile).

### Migrating from Xboard

- `ggboard import xboard` supports both schemas, the legacy split tables and the upstream unified table, from a MySQL, PostgreSQL or SQLite source database; it imports users, plans, nodes, coupons, tickets, announcements, knowledge base, payment configuration and settings, but not past orders.
- Original primary keys, password hashes, subscription tokens, UUIDs, `server_token` (when it has at least 16 characters) and `telegram_id` are kept, so existing subscription links and node backends need no changes; `--dry-run` rehearses the import, `--skip-banned` skips banned users, `--with-stats` also imports traffic statistics.

### Deployment and operations

- For every `v*` tag, GitHub Actions publishes binary archives for linux (amd64, arm64), darwin (amd64, arm64) and windows/amd64 with `SHA256SUMS`, and the multi-arch image `ghcr.io/linusxiong/ggboard` (`1.0.0`, `1.0`, `1` and `latest` for this version).
- Docker: a distroless image that runs as nonroot; `deploy/docker-compose.prebuilt.yml` (prebuilt image) and `deploy/docker-compose.yml` (build from source). There are also a hardened systemd unit and an nginx example.
- CLI: `serve`, `migrate`, `admin create | reset-password | promote`, `import xboard`, `db copy`, `version`; in a terminal, logs and output are coloured and aligned, controlled with `--color` or `NO_COLOR`.
- `ggboard db copy` copies the whole current database into another one (SQLite → PostgreSQL, and back to SQLite as well; MySQL works too), keeping primary keys and every column. It writes in one transaction, and before committing it checks row counts and every row's SHA-256 and resets PostgreSQL sequences; `--dry-run` rehearses the copy. Steps in `docs/DEPLOY.md` §5. Times read back from PostgreSQL are UTC, as with SQLite and MySQL, regardless of the process time zone.
- Scheduled jobs: order timeout, commission settlement, traffic reset, expiry and traffic reminders, daily statistics; `GET /healthz` for liveness probes.
- `make bench`: a reproducible load test (10,000 users, 50 nodes); results in `docs/perf/`.

[Unreleased]: https://github.com/linusxiong/ggboard/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.1
[1.0.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.0
