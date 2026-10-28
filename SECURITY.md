# Security Policy

**English** · [简体中文](SECURITY.zh-CN.md)

## Reporting a vulnerability

Please **do not** report security issues in a public issue. Submit them through GitHub private vulnerability reporting:
<https://github.com/linusxiong/ggboard/security/advisories/new> (**Security → Report a vulnerability** in the
repository).

Include the affected version or commit, steps to reproduce or a PoC, the impact, and your suggested fix (if you have
one). We will talk to you in the private advisory, publish it after the fix is released, and credit you if you agree.
This is an open-source project maintained in spare time, so responses are best effort.

## Supported versions

| Version | Status |
| --- | --- |
| The latest `1.x` release (and `main`) | ✅ Receives security fixes |
| Older `1.x` releases, commits from before 1.0 | ❌ Upgrade to the latest `1.x` release |

Security fixes go into `main` and ship with the next patch release (`1.x.y`). They are not backported to older releases.

## Hardening checklist

Deployment details are in [docs/DEPLOY.md](docs/DEPLOY.md). Every item matches what the code actually does.

### Network and TLS

- [ ] **HTTPS only**: the panel does not handle TLS itself and does not send HSTS. Put it behind nginx / Caddy, redirect
      80 to 443, and have the proxy send `Strict-Transport-Security` (`deploy/nginx.example.conf` already does).
- [ ] **Listen on localhost only**: compose only publishes `127.0.0.1:8080` and the systemd unit listens on
      `127.0.0.1:8080`. Do not expose port 8080 directly.
- [ ] **Put only your proxies in `server.trusted_proxies`** (e.g. `127.0.0.1,::1`; with Docker, the bridge network's
      subnet). Without it, every client IP is the proxy's IP and the per-IP rate limits are shared by the whole site; with
      `0.0.0.0/0`, anyone can forge `X-Forwarded-For` to get around the rate limits.
- [ ] **Set `app_url`** (`https://...`): payment callbacks, subscription links, links in emails and the Passkey RP are
      all based on it. Without it, the panel builds absolute URLs from the request's `Host` and `X-Forwarded-Proto`.
- [ ] **Keep `server.cors_origins` empty** (same origin only). The binary serves the SPA itself, so no cross-origin
      access is needed. Do not set `*`.

### Secrets

- [ ] **Use 32+ random bytes for `jwt.secret`** (`openssl rand -hex 32`; placeholders and values that are too short or
      too repetitive are refused at startup), and back it up together with the database. It signs sessions and also
      derives the AES-256-GCM key that encrypts TOTP secrets: changing it invalidates every session **and every enrolled
      TOTP** (backup codes and Passkeys are not affected). When it is empty, the panel generates
      `<data_dir>/jwt.secret` (0600).
- [ ] **Rotate `server_token`**: a 32-character random value is generated on first start. In the admin console, use
      **Settings → Node connection → Regenerate token**. Once you save, the old value stops working immediately, and the
      XrayR / V2bX `ApiKey` and soga `webapi_key` must be updated on every node. The node APIs compare the token in
      constant time and reject every request while it is empty. A migration from Xboard keeps the old token when it has at
      least 16 characters (a shorter one is not imported and ggboard keeps its generated token); if the old panel may
      have leaked it, rotate it right after migrating.
- [ ] **File permissions**: the data directory is created as 0700 and `jwt.secret` as 0600. Use 0750 for `/etc/ggboard`,
      and make the environment file readable by root only.
- [ ] **Telegram bot secrets**: `telegram_bot_token` and `telegram_webhook_secret` are only returned by admin endpoints
      and never appear in logs or API errors (the Telegram client strips the request URL, which contains the token, from
      transport errors). The webhook secret is generated on first start. If you suspect it has leaked, use
      **Settings → Telegram → Regenerate secret**, save, and click "Set webhook" again. If the BotFather token leaks,
      `/revoke` it in BotFather first.
- [ ] **Encrypt backups**: the database holds password hashes, encrypted TOTP secrets, encrypted age private keys,
      subscription tokens, `server_token`, the SMTP password, CAPTCHA secrets, the Telegram bot token and payment
      gateway secrets.
- [ ] **Do not commit secrets**: `.gitignore` already excludes `.env*` (except `.env.example`), `config.yaml`, `config.local.yaml`, `*.pem`, `*.key`, `data/` and `*.db`.

### Accounts

- [ ] **Require two-factor authentication for admins**: **Settings → Security → `admin_mfa_required`**. Once it is on, an
      admin or staff account with neither TOTP nor a Passkey gets `403 mfa_setup_required` on `/api/v1/admin/*`, but
      can still enrol at `/user/security`. Enrol yourself before you turn it on.
- [ ] **Passkey**: the RP ID defaults to the host name of `app_url` and can be overridden with `passkey_rp_id`. Add any
      other frontend domains to `passkey_origins`.
- [ ] **Sign-in and sign-up protection** (**Settings → Sign-up & sign-in**): `password_limit_enable` (on by default:
      5 wrong passwords for the same email within 60 minutes lock it), `register_limit_by_ip_enable`, `email_verify`,
      `email_whitelist_*`, `invite_force`, and `captcha_provider` (Turnstile / reCAPTCHA / hCaptcha).
- [ ] **Use SMTP in production**: `email_driver=log` only writes emails to the log, and with `log.level=debug` even the
      message bodies (verification codes, reset links) are written to the log. When you use SMTP, **turn on
      `captcha_provider` as well**: `send-code` is an anonymous endpoint and its built-in rate limit only counts per IP
      (there is also a process-wide budget for verification codes of 300 emails per hour with a burst of 60; past it the
      endpoint still returns `true` but sends nothing), so someone who controls many IPs can still use up the whole
      budget and keep legitimate users from receiving codes for a while. Verification-code emails take at most 75% of
      the mail queue, so transactional emails (orders, tickets, leak alerts and so on) always have room; above that mark,
      `send-code` returns 429. When there is no account and sign-up is closed (`stop_register`) or the email suffix is
      not on the whitelist, no code is sent (the endpoint still returns `true`). An email that the SMTP server rejects
      with a 5xx (or whose recipient address cannot be resolved) is tried only once. Only 4xx replies, timeouts and
      connection errors are retried, and the retries wait in the background without blocking other emails.
- [ ] **Turn on `email_verify` once SMTP is set up**: while it is off, anyone can sign up with someone else's email
      address. The panel records `users.email_verified_at`. The first time someone proves ownership of an unverified
      account's email through password reset, it is handled as a **change of ownership**: besides the new password, the
      UUID and subscription token are replaced, all Passkeys and TOTP / backup codes are deleted, the Telegram link is
      removed (including unused binding codes), and an `account: unverified account reclaimed` audit log entry is
      written. The legitimate owner of such an account therefore also has to import the subscription again and set up
      two-factor authentication again after their first password reset. With `email_verify` on, a sign-up request
      without a correct code gets the same 400 (`email_code`) for registered and unregistered emails, so it does not
      reveal whether an email is registered. Accounts created by an admin, imported from Xboard, or existing before the
      upgrade count as verified.
- [ ] **Admin sign-in alerts**: configure the Telegram bot and keep `admin_login` in `telegram_events` (on by default).
      Every admin or staff sign-in (password, password + TOTP / backup code, Passkey) pushes the email, role, method,
      IP, UA and time to `telegram_admin_chat_ids`, so you notice an unfamiliar sign-in right away. Admin sign-ins,
      subscription leaks and withdrawals use a separate priority lane (256 entries, its own worker), so they are not
      dropped or delayed even when user-triggered events (tickets, orders) fill the normal queue.
- [ ] **Staff accounts**: staff can only handle tickets, notices and the knowledge base, and have read-only access to
      users (they cannot see UUIDs or subscription tokens). The knowledge base placeholders `{{subscribeUrl}}`,
      `{{urlEncodeSubscribeUrl}}` and `{{safeBase64SubscribeUrl}}` expand to **each reader's own** subscription URL
      (token included), so only admins can write them: when staff create or edit a body that contains these
      placeholders, the request gets `403` (resubmitting a body an admin wrote, unchanged, does not count as an edit, and
      staff can still change the title, category and visibility). After upgrading from an older version, have an admin
      review the articles that contain these placeholders, especially those with external images or links.

### Subscription leak protection

- [ ] **Leak detection**: in **Settings → Subscription**, set `subscribe_leak_max_ips` above 0 (e.g. 10). When one
      subscription token is fetched from more than that many distinct IPs within 24 hours (IPv6 counted per /64), the
      user gets an email (and a Telegram message if they bound Telegram) and the admins get a Telegram
      `subscription_leak` alert. `subscribe_leak_action=rotate` also
      replaces the token at once (only the subscription link changes; node credentials and sessions stay the same).
      Access logs are kept for 30 days, up to the latest 100 entries per user.
- [ ] **age-encrypted subscriptions**: users can generate an age key on the Subscription page (mihomo 1.19.27+, Clash
      Party and FlClash can decrypt it). The defaults favour compatibility: `subscribe_age_optional` (on by default)
      allows `age=0` to get plaintext, and while `subscribe_age_strict` is off (the default), other formats still return
      plaintext. To make sure a leaked link cannot get plaintext, **turn off `subscribe_age_optional` and turn on
      `subscribe_age_strict`** (other formats then get 406). age private keys are stored encrypted with a key derived
      from `jwt.secret`, so after `jwt.secret` changes, users have to generate a new key.
- [ ] Remind users: when a link leaks, use "Rotate link" first (it only replaces the token). Use "Reset subscription"
      only when the node credentials (UUID) may have leaked too (the UUID and token are replaced together, and every
      device has to import the subscription again).

### Built-in rate limits

In-process token buckets. A request over the limit gets `429 rate_limited` with `Retry-After`:

| Scope | Limit | Keyed by |
| --- | --- | --- |
| `/api/v1/passport/*` | 30 per minute | Client IP |
| `POST /api/v1/passport/email/send-code` | Another 10 per hour on top; for IPv6, also 60 per hour per /48 in total | Client IP (IPv6 per /64 and /48) |
| Same, all clients combined | 300 emails per hour, burst 60 (past it the endpoint returns `true` but sends nothing; one WARN log line per minute) | Whole process |
| `/api/v1/user/security/*` | 60 per minute | User |
| `POST /api/v1/user/coupons/check`, `POST /api/v1/user/orders`, `POST /api/v1/user/orders/:trade_no/checkout` | 30 per minute, burst 10 | User |
| `PUT/POST/PATCH/DELETE /api/v1/user/rule-profile(s)…`, `GET /api/v1/user/subscription/preview` | 60 per minute, burst 30 | User |
| `/api/v1/user/subscription/age` (writes), `POST /api/v1/user/subscription/rotate-token` | 30 per minute, burst 10 | User |
| `POST /api/v1/user/telegram/bind-code` | 10 per minute, burst 5 | User |
| `POST /api/v1/user/tickets` | 30 per hour, burst 10 (every new ticket notifies the admins) | User |
| `POST /api/v1/user/tickets/:id/reply` | 120 per hour, burst 30 | User |
| `POST /api/v1/guest/telegram/webhook/:secret` | 600 per minute, burst 120 | Client IP |
| `GET/HEAD /api/v1/client/subscribe`, `/s/:token` | 120 per hour, burst 30 | Subscription token |
| Same, unknown or malformed tokens | 60 per hour, burst 30 | Client IP (IPv6 per /64) |

### Headers

- All responses: `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`,
  `X-Frame-Options: DENY`, `Cross-Origin-Opener-Policy: same-origin` (a cross-origin page gets no handle to the panel's
  window, so it cannot count iframes to infer the sign-in state; CAPTCHA iframes are not affected).
- HTML documents also get a CSP (`internal/static/static.go`):

  ```text
  default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self';
  connect-src 'self'; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'
  ```

  `img-src` allows `https:` because Gravatar avatars, notice cover images (`img_url`) and custom payment method icons
  are external https images, and knowledge base articles can reference external images too. That is exactly why the
  knowledge base placeholders that expand to the reader's subscription URL can only be written by admins (see "Staff
  accounts" above). Do not loosen it at the proxy. When a CAPTCHA is enabled, `static.CSPFor` adds script / frame
  sources only for **the one service that is currently enabled** (Turnstile: `challenges.cloudflare.com`; reCAPTCHA:
  `www.recaptcha.net`, `www.google.com`, `www.gstatic.com`; hCaptcha also adds connect / style); no other service is
  allowed.

### Payment webhooks

- The callback URL is `/api/v1/guest/payment/notify/<gateway>/<uuid>`, where `uuid` is a random value unique to each
  payment method.
- Every gateway verifies the signature: EPay and Epusdt use MD5 signatures, Alipay F2F (Face-to-Face) uses RSA2
  (SHA256), Stripe uses `Stripe-Signature`, and Coinbase and BTCPay use HMAC-SHA256 (Coinbase also checks a 300-second
  time window). When verification fails, the callback gets `400 fail` and a WARN log entry is written; the order is not
  changed.
- Public entry points of a BTCPay store, such as POS, Pay Button and crowdfunding, let anyone create and pay an invoice
  with an `orderId`, amount and currency of their choice, and its callback signature is just as valid. When the panel
  creates an invoice, it writes `ggboardBinding` (an HMAC keyed with the webhook secret, covering the payment method
  uuid, order number, amount and currency), and it only accepts settled invoices whose binding matches; anything else
  is only acknowledged and logged as WARN. Invoices created before a webhook secret rotation and paid after it have to
  be confirmed by hand.
- Enter gateway secrets (webhook secret, merchant key) only in the payment method in the admin console, and use HTTPS
  for the callback domain (`notify_domain`).

### Logs

- The access log, as well as the logs for rate limit hits (`rate limited`), 5xx errors (`request failed`) and panics,
  record only the **route template** (e.g. `/s/:token`, `/api/v1/guest/telegram/webhook/:secret`) and never the query
  string, so subscription tokens and webhook secrets do not end up in the logs. Only 404s that match no route record the
  raw path. The subscription access log (`subscription_access`) stores only an 8-character hash label of the token.
- Use `log.level=info` in production. Use `log.format=json` when the logs are collected somewhere else.
