# Changelog

**English** · [简体中文](CHANGELOG.zh-CN.md)

Notable changes to GG Bond (ggboard), per version. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). When a `v*` tag is pushed, its GitHub release notes are that version's section from this file followed by the same section from [CHANGELOG.zh-CN.md](CHANGELOG.zh-CN.md) (see `.github/workflows/release.yml`), so write the version in both files before tagging.

## [Unreleased]

### Fixed

- Upgrade credit (`surplus_enable`) now covers subscriptions the panel has no order for. Users imported from Xboard (whose orders are not imported) or given a plan by an administrator got no credit when they switched plans, and the new plan's period started from the day of the upgrade, so their remaining time was lost. That time is now valued at the old plan's cheapest price per month (a one-time plan: its one-time price times the unused share of the traffic), less the user's own discount. This estimated part lowers the new order's price at most to zero and is never added to the balance. Credit for orders paid in the panel works as before.

## [1.1.0] - 2026-11-26

Most of this release is the fixes of a security audit (see **Security**). Before upgrading: with `admin_mfa_required` on, administrators and staff have to sign in again once, with their two-factor code or a passkey. Turning on two-factor authentication or adding a passkey now asks for the current password. API requests with a body must send JSON; any other content type gets `415`. Saved link settings that are not http(s) URLs are ignored until they are entered again. The database upgrade (a new `tickets.kind` column) runs by itself when the panel starts.

### Changed

- HeroUI is updated to 3.2.8. Dialogs, menus and tooltips now open and close by fading and moving only, which mid-range phones keep up with. On a phone, an action menu no longer shrinks narrower than its labels, and a picker list leaves room for the check mark beside a long option. Dialog and confirmation buttons wrap onto a second line when they do not fit side by side.
- The Stripe SDK is updated to 87.2.0, which sends API version `2026-11-18.endive`. That is the same `endive` release as before, so webhook endpoints need no change.
- Development: the frontend is linted with Oxlint instead of ESLint, and CI runs the frontend lint, type check, build and unit tests as parallel jobs.

### Fixed

- Switches, text fields and selects animate their state changes: the thumb slides, and borders and backgrounds fade, as HeroUI designs them. These transitions read theme variables that were missing from the built CSS, so browsers ignored them. The same gap put an invisible 3 px border inside search fields; it is gone.

### Security

Security audit of 2026-11-25. It found no way to forge a payment callback or get around a gateway's signature check.

- A request to an anonymous JSON endpoint such as `/api/v1/passport/login` with `Content-Type: application/x-yaml` and a few hundred bytes of YAML aliases could exhaust the panel's memory and kill the process. JSON endpoints now accept only `application/json` (or `application/*+json`, or no Content-Type) and answer anything else with `415 unsupported_media_type` before reading the body. BSON bodies also skipped field validation; they are refused the same way.
- A subscriber's own rename rules (`allow_user_rule_profile`, on by default) could multiply node names until rendering a subscription or its preview ran the panel out of memory. Node names are cut to 1024 bytes after every rename step, and each step's work is bounded.
- With `admin_mfa_required` on, an administrator or staff member whose only second factor was a passkey got into the admin area with the password alone. Sessions now record how they signed in, and the admin area needs a session that used the two-factor code or a passkey (`403 mfa_reauth_required` otherwise; the panel signs such a session out and asks for that sign-in). Only passkeys registered for the current RP ID count. **With `admin_mfa_required` on, administrators and staff have to sign in again once after upgrading.**
- Turning on two-factor authentication and adding a passkey now ask for the current password, so a stolen session cannot plant a passkey that survives a password change, or a TOTP secret that locks the owner out.
- A password reset of an account whose email was never verified removes its second factors (it is treated as the mailbox owner reclaiming the account). Promoted administrators stayed unverified, so access to their mailbox was enough to strip their two-factor authentication. Administrator and staff accounts are never reclaimed now, and promoting an account (admin panel or `ggboard admin promote`) marks its email verified.
- A commission withdrawal request did not hold the commission, so the same amount could be moved to the balance and spent while staff paid it out. While a withdrawal request is open, moving commission to the balance is refused (409). Requests filed before the upgrade, and those imported from Xboard, are recognised. Settle a request by lowering the user's commission, paying, then closing the ticket.
- Passkey sign-ins of administrators, staff and accounts with two-factor authentication must include user verification (PIN, fingerprint or face); a security key used without its PIN is refused for them.
- Wrong two-factor codes are also limited to 30 per 24 hours (on top of 20 per hour). Wrong passwords at the sign-in form no longer block the owner's own changes from a signed-in session: confirming the current password has its own per-account limit.
- A node's alive report counted any string as a device, so one node could push a user shared with other nodes to their device limit everywhere. Only IP addresses count now.
- Surge and Surfboard `#!MANAGED-CONFIG` URLs, which contain the subscription token, could stay on `http://` for a `subscribe_url` host behind a TLS proxy that is not in `server.trusted_proxies`. They use https for the hosts of `https://` entries of `subscribe_url`.
- `//s/<token>/` and `//api/...` answered with the web app's page and status 200 instead of the JSON 404.
- Plans with a capacity limit could be oversold: unpaid orders now hold a seat until they are paid, cancelled or time out, and concurrent purchases of the last seat no longer both succeed.
- Replayed payment notifications for an unknown order or with a mismatching amount mailed the administrator on every request; they alert once per transaction. A second payment for an order that is already paid now alerts the administrator instead of only being logged.
- An administrator demoted while a request was running could still read the order list and order details.
- In custom mail templates, a placeholder written inside an HTML tag could turn user text, such as a ticket subject, into tags or attributes.
- GeoIP update errors no longer repeat what the download or version server answered, and redirects from https to plain http are refused.
- API responses default to `Cache-Control: no-store`.
- `app_url`, `logo`, `tos_url`, `telegram_discuss_link` and the client download URLs must be http(s) URLs (a value without a scheme gets `https://`). A saved value that is not, such as a relative path or a `tg://` link, is ignored and has to be entered again; the Xboard importer drops such values with a warning.
- A node's own token can no longer equal `server_token`, and `server_token` can no longer be set to a node's own token. The three commission distribution levels can no longer add up to more than 100%. Staff can no longer filter users by UUID or subscription token, which they cannot see.
- The Docker image creates `/data` with mode 0700 (new volumes inherit it; tighten an existing one with `docker run --rm -v <volume>:/data alpine chmod 700 /data`), and `deploy/docker-compose.yml` drops all capabilities with `no-new-privileges`. GitHub Actions are pinned to commit SHAs and the Dockerfile base images to digests.

- The Stripe webhook no longer logs the SDK's explanation of a failed signature. Since stripe-go 87.2.0 that text can quote the length and the last four characters of the configured signing secret, and anyone can trigger it with a crafted `Stripe-Signature` header.
- The MySQL driver is updated to 1.10.2 ([GHSA-9qjq-77h9-895w](https://github.com/go-sql-driver/mysql/security/advisories/GHSA-9qjq-77h9-895w): a truncated handshake packet crashes the client before TLS starts, which matters if your DSN sets `tls=true`; [GHSA-4v5x-9mc8-gr98](https://github.com/go-sql-driver/mysql/security/advisories/GHSA-4v5x-9mc8-gr98): rows of one query handed to a later one after a failed `LOAD DATA LOCAL INFILE`, which ggboard never runs). SQLite and PostgreSQL installations are not affected.
- DOMPurify is updated to 3.5.0 ([GHSA-hrxw-jjj6-2g2j](https://github.com/cure53/DOMPurify/security/advisories/GHSA-hrxw-jjj6-2g2j), in the `SAFE_FOR_TEMPLATES` option, which the panel does not use).
- The PostgreSQL driver pgx is updated to 5.13.0, which fixes three advisories. Two are rated high: SQL injection in simple-protocol batches ([GHSA-j38q-h3ph-jjpq](https://github.com/jackc/pgx/security/advisories/GHSA-j38q-h3ph-jjpq)), which ggboard does not use, and `require_auth` / `channel_binding` in the connection string not being enforced when the server skips authentication ([GHSA-95fj-3fxw-g62g](https://github.com/jackc/pgx/security/advisories/GHSA-95fj-3fxw-g62g)), which matters if your DSN sets either. The third is a panic on a malformed server response ([GHSA-w9rp-vfrh-q8vv](https://github.com/jackc/pgx/security/advisories/GHSA-w9rp-vfrh-q8vv)). SQLite and MySQL installations are not affected.
- go-mail is updated to 0.8.2 ([GHSA-f82v-42xw-72cj](https://github.com/wneessen/go-mail/security/advisories/GHSA-f82v-42xw-72cj), [GHSA-w8w6-94hg-2qpm](https://github.com/wneessen/go-mail/security/advisories/GHSA-w8w6-94hg-2qpm)). ggboard uses neither the EML parser nor the List-Unsubscribe helper they are in.
- DOMPurify is updated to 3.4.18 ([GHSA-qm7q-q8h9-jg8q](https://github.com/cure53/DOMPurify/security/advisories/GHSA-qm7q-q8h9-jg8q), in an option the panel does not use) and marked to 18.2.0, which no longer takes quadratic time on unmatched emphasis markers. Both render Markdown in the panel, such as notices and knowledge-base articles.
- `golang.org/x/crypto` is updated to 0.59.0, which no longer ships the unmaintained `openpgp` package (never used by ggboard).
- Development only: Vite is updated to 8.3.8. Earlier 8.3 releases let a request run code on a dev server that was started with `--host` ([GHSA-p4f5-76q4-ffq7](https://github.com/vitejs/vite/security/advisories/GHSA-p4f5-76q4-ffq7)). The released binaries and images never contain the dev server.

## [1.0.4] - 2026-11-15

### Added

- Administrators get a Telegram alert when a user replies to a ticket (`ticket_user_reply`, on by default). Each ticket alerts at most once in 30 minutes, and a staff reply starts over, so a reply to an answer is never missed. Sites that already saved their event list get the event added once on upgrade. The existing event **Ticket reply** is now called **Staff replied to a ticket**.

### Changed

- With **Reduce motion** on, the panel now removes only movement, scaling and rotation. Spinners keep turning, slowly, instead of freezing (a frozen spinner looked like a hang), toasts fade as designed, and colour changes stay smooth. Before, one global rule cut every animation and transition short.
- Uptime and other durations are written the way the language writes them where the browser has `Intl.DurationFormat` (`1天2小时3分钟` instead of `1天 2小时 3分钟`). Older browsers keep the previous format.
- Headings wrap into lines of similar length, so a title no longer leaves a single word or character on its last line.

### Fixed

- The plain subscription link works in Quantumult X, which answered it with "response invalid". Quantumult X now gets the node list by default, as with the one-tap import. Add `&list=0` to get its whole configuration.
- The whole Quantumult X configuration calls its main group `Proxies`, because `proxy` is a built-in policy there, and a health-checked group with one server becomes `static`.
- Choosing light or dark no longer shows the other scheme first while a page loads. The landing, sign-in, sign-up and reset pages are prerendered and stayed in the system scheme until the app had started, for about half a second; every other page flashed its background. A small script now applies the choice before the first paint on every page.
- Typing Chinese, Japanese or Korean: the Enter that picks a candidate in the input method no longer renames a group, adds a node filter pattern or commits a policy-group name halfway through. Chip fields handle this in Safari as well.
- A toast shown while a dialog is open, such as “Copied” in the two-factor dialog or a failed confirmation, can now be read by screen readers and dismissed. It used to be hidden from them together with the page behind the dialog. Each toast is announced once, and dismissing a focused toast moves focus to the next one or back to where it was.
- The backup-codes file carries today's date where you are. It used the UTC date, so before 08:00 in UTC+8 it showed yesterday.
- After a failed submit, focus goes to the first invalid field, not to the ⓘ beside its label. The password, two-factor and passkey forms now move focus too; they used to leave it on the button without saying anything.
- **Send code** keeps keyboard focus when its cooldown starts, and screen readers are told that the code was sent.
- The invite-code field no longer capitalises the first letter or autocorrects on phones. The code is case-sensitive, so a typed code could fail.
- On phones, the settings and mail-template chip strips fade at an edge while there is more to scroll. Their scroll bar is hidden, so nothing showed that they scroll.
- If the server refuses a passkey the browser has just created, the passkey provider is told, so it stops offering a passkey that cannot sign in (Chrome 132, Safari 26 and later).
- Accessibility details: a required field's name no longer ends in “*” for screen readers; the current tab in the phone tab bar is bold as well as blue, and **More** shows as current when one of its pages is open; the staff ticket thread scrolls with the keyboard; chip remove buttons are 24 px.

### Security

- EPay and Epusdt refuse a payment notification whose signed string contains `notify_url=`, `return_url=` or `redirect_url=` anywhere, even inside another value, so the signature of a checkout request can never pass as a callback. Before, only the parameter names were checked, so a plan or site name that itself spelled out the fields of a successful callback after a `&` made the checkout signature of that order verify as a paid notification. Only administrators set those names, and a plain `&` such as `Pro & Family` was not enough. EPay now signs `&` in the product name as `＆`, and Epusdt rejects callback URLs whose request Host contains `&`, `?`, `#` or user information, as EPay already did.
- Built with Go 1.27.4, which fixes a crash in the `net/http` server ([CVE-2026-97054](https://pkg.go.dev/vuln/GO-2026-6859)): an HTTP/1 server that limits request bodies with `MaxBytesReader`, as the panel does for every request, could panic when a handler read the body while it was writing the response headers. `go.mod` now requires Go 1.27.4, so a build from source uses it too.

## [1.0.3] - 2026-11-05

### Added

- Routes: a **Push** button on each route puts it on all nodes or a chosen set in one go, instead of editing every node (`POST /admin/routes/:id/apply`). A new column shows how many nodes have the route. The dialog sets exactly the nodes you pick, so unticking a node withdraws the route from it. Nodes pick the change up on their next config pull; child nodes keep using their parent's routes.

### Changed

- Node tags are now edited as chips: press Return or comma to add, paste several at once, and pick from the tags other nodes already use. The tag field has its own full-width row in the node sheet, and the name field moved to one as well.

### Fixed

- The **Show** switch in the node sheet lines up with its ⓘ and with the Sort field. Every switch placed next to a taller field gets the same fix.
- The node list names a child node's parent instead of showing `[object Object]`.
- The node sheet no longer offers routes on a child node, where they had no effect because a child uses its parent's routes. Saving a child clears any routes of its own.
- The help on the node tag field no longer says tags appear in subscription node names. They do not: they appear in the users' node list and are read by keyword filters and region detection.
- Opening the site while signed in shows the console button from the first paint. The landing page is a prerendered copy of the signed-out page, so **Log in** used to stay on screen until the app had loaded, for over a second on a slow connection.
- Quantumult X no longer gets an empty subscription when nodes use AES Shadowsocks 2022. The panel left those nodes out although this client supports the ciphers, so the one-tap import came back empty and Quantumult X reported invalid content.
- Subscription links from Xboard keep working. `flag=meta`, `flag=general` and `flag=qx` are understood, `types=all` no longer drops every node, and `flag=clash` no longer forces classic Clash on a client whose User-Agent shows a newer core, which could leave it with no nodes.
- The `clash-meta/*`, `clash surgio/*` and `Quantumultx` User-Agents now get the right format.
- Quantumult X also gets AnyTLS Reality nodes, and Surfboard 2.28.1 or later gets AES Shadowsocks 2022 nodes.

## [1.0.2] - 2026-11-02

### Added

- Nodes can have their own **node token** (Nodes → edit → Node token; `token` on the admin node API, new column `nodes.token`, added automatically on start). Empty keeps the shared `server_token`. Once set, the node accepts only its own token and the token works for no other node, so whoever runs one node can no longer change `node_id` to read other nodes' users and keys. Child nodes use their parent's token, several nodes may share one (one soga with `node_id=1,2,3`), and a copied node gets a fresh one. XrayR, V2bX and soga need no change: each node entry already has its own key setting.

- Routes: paste a whole list into the match box and it is added in one go. Quotes and trailing commas (`"geosite:bbc",`, `'domain:rfa.org'`), several quoted items on one line and JSON arrays are all accepted; the box tidies the text and drops repeats when you leave it, and the admin API cleans `match` the same way. Raw DNS JSON routes (`main` first) are left alone. A block route that uses a prefix only soga understands (`geosite:`, `geoip:`, `domain:`, `full:`, `ip:`, `port:`) now shows a note that XrayR and V2bX cannot take it from the panel.

### Changed

- The README and the user-facing docs (deployment, admin and user guides, the Xboard cutover runbook, the benchmark write-up, contributing guide, security policy and this changelog) now come in two files each: English in `X.md`, shown by default, and Simplified Chinese in `X.zh-CN.md`, linked from the top of each. Release archives ship both READMEs, and release notes carry the English and the Chinese section of the changelog.

### Fixed

- The bulk mail form no longer says the content supports Markdown: it is sent as plain text, with line breaks kept.
- A payment made on the page of an earlier checkout with another payment method (the buyer switched methods, or two checkouts raced) now fulfils the order when the amount matches that method's own fee, instead of waiting for an admin to confirm it by hand.
- A failed write of the subscription access log (a database error, or the request of an access-log read going away) puts the rows back for the next flush instead of losing them, along with the input of leak detection.
- Telegram messages are cut to size by their escaped length: an admin alert for a ticket whose subject or body is full of `&` or `<` was too long for Telegram and dropped.
- The settings link on the Nodes page and the checklist of the Xboard import report now name the settings tab as it is called, Settings → Node connection.
- Docs: an Xboard `server_token` is kept on import only when it has at least 16 characters; suspected-leak alerts also reach users on Telegram when they have bound it; the admin guide names the leak action as the console does.

### Security

- Node traffic, online and audit reports only count for users the node serves (its groups and its children's groups). Before, anyone holding the node key could report traffic for any user and use up their allowance.
- `POST /user/coupons/check` answers a hidden plan exactly like an unknown one, so plan IDs and periods of hidden plans can no longer be enumerated; order creation checks visibility before anything else too.
- The `cats` subscription query list is limited to 64 entries like `types` and `filter`, instead of answering an error line per unknown category.
- Payment notifications that fail verification are limited per IP (60 per minute, burst 120; 429 before the payment lookup once used up). Verified callbacks are never limited.
- Ticket messages shown to the user no longer carry the staff account's ID (staff authors appear as `0`).
- A user's order cancellation only ever updates that user's own order, rather than relying on a rollback.
- Docs: how to keep node keys and subscription tokens out of the reverse proxy's access log (`deploy/nginx.example.conf` now does it), and that ggboard runs as a single instance (rate limits and online state are per process).

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

[Unreleased]: https://github.com/linusxiong/ggboard/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.1.0
[1.0.4]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.4
[1.0.3]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.3
[1.0.2]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.2
[1.0.1]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.1
[1.0.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.0
