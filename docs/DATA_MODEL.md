# Data model (GORM, `internal/model`)

`model.All()` lists every table; `db.Migrate` auto-migrates them on every start (`ggboard migrate` runs only that step).
Every index below comes from a GORM tag. Schema changes AutoMigrate cannot do (a changed primary key) run as idempotent
steps around it in `internal/db/upgrade.go`; the only one so far is the `rule_profiles` upgrade below.

Conventions:

- Table names are plain plural snake_case (no `v2_` prefix). The exceptions are `knowledge`, `user_mfa` and `subscription_access`, which set `TableName()`.
- `id` is a `uint64` auto-increment primary key. `created_at`/`updated_at` are `time.Time`, managed by GORM.
- Money is integer **cents** (`int64`). Traffic is **bytes** (`int64`); the only exception is `plans.transfer_enable`, which is in GB.
- `varchar(N)` means a Go `string` with `size:N` (VARCHAR on PostgreSQL/MySQL, TEXT affinity on SQLite).
- Nullable scalars are Go pointers. Booleans are real `bool` columns with `default:false`. Defaults that should be `true`
  are set in code, never in the tag (docs/BACKEND.md §9).
- JSON columns are TEXT on every driver and are never SQL NULL (empty slice = `[]`, empty map = `{}`):
  `model.JSONSlice[T]` (array), `model.JSONObject[V]` (`map[string]V`), `model.JSONMap` (free-form object) and
  `model.RawJSON` (opaque document).
- `date` columns (`stat_users.record_at`, `stat_nodes.record_at`, `stats.record_at`) hold `model.Day(t)`: the calendar
  day in the server's local zone, stored as midnight UTC.
- No foreign-key constraints are created (`DisableForeignKeyConstraintWhenMigrating`, because imports may contain
  dangling references). Code deletes dependent rows explicitly.

## users

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| email | varchar(191) unique, not null | lower-cased |
| password_hash | varchar(255) not null | argon2id (new users), bcrypt `$2y$`/`$2a$`/`$2b$` or Xboard `xb$<algo>$<salt>$<hex>` (imported). `!…` never verifies |
| uuid | varchar(36) unique, not null | per-user credential, see "Derived credentials" |
| token | varchar(32) unique, not null | subscription token (32 hex characters) |
| plan_id | *uint64 idx | |
| group_id | *uint64 idx | server group whose nodes the user can use |
| transfer_enable | int64 | bytes |
| u, d | int64 | bytes used (upload/download), already multiplied by the node rate |
| expired_at | *time idx | null = never expires |
| next_reset_at | *time idx | next traffic reset, computed from the plan's reset method |
| speed_limit | *int | Mbps, null = unlimited |
| device_limit | *int | null = unlimited |
| balance | int64 | cents |
| commission_balance | int64 | cents |
| commission_type | int8 | 0 follow settings, 1 every paid order, 2 first order only |
| commission_rate | *int | percent override |
| discount | *int | percent |
| invite_user_id | *uint64 idx | |
| is_admin, is_staff, banned | bool | |
| remind_expire, remind_traffic | bool | column default false; new users get the `default_remind_expire` / `default_remind_traffic` settings (default true) |
| last_login_at | *time | |
| last_login_ip | varchar(45) | |
| telegram_id | *int64 idx | chat id of the bound Telegram account (bot `/start <code>`; imported from Xboard verbatim); one Telegram account binds one user |
| telegram_username | *varchar(64) | Telegram `@username` (without `@`) at binding time; cleared on unbind and when an admin edits `telegram_id` |
| remarks | text | admin notes |
| password_version | int | bumped on password change / security reset; carried in the JWT `ver` claim |
| created_at | time idx | |
| updated_at | time | |
| email_verified_at | *time | first proof that the account holder controls the mailbox: registration with an email code (`email_verify` on), or the first `/passport/forgot` reset. Admin-created (`/admin/users`, generate, `ggboard admin create`) and imported accounts get it at creation (imports: `created_at`); the upgrade that added the column set it to `created_at` for every existing row. Null = never verified: the first reset is an ownership change (new uuid/token, passkeys, `user_mfa` and Telegram binding removed; `usersvc.ReclaimAccount`) |

Derived credentials (`model.UserCredentials(u, node)`):

- Every protocol uses `uuid` as the UUID and, where a password is needed (trojan, hysteria, anytls, tuic, shadowsocksr,
  plain shadowsocks), as the password.
- shadowsocks 2022: the user key is base64(uuid[:16]) for `2022-blake3-aes-128-gcm` and base64(uuid[:32]) otherwise
  (Xboard `Helper::uuidToBase64`). The client password is `<server_key>:<user_key>`. The node key is
  `protocol_settings.server_key` when set, otherwise `base64(md5hex(created_at_unix)[:16|32])` (Xboard `getServerKey`);
  a child node uses its parent's `created_at`.
- mieru: username = password = `uuid`.
- shadowsocksr node password (soga `config.password`): `protocol_settings.password`, otherwise
  `base64(md5hex(created_at_unix)[:16])`.

## plans

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| group_id | uint64 idx, not null | server group granted on purchase |
| name | varchar(191) not null | |
| content | text | markdown description |
| tags | json []string | |
| transfer_enable | int64 | **GB** (as entered); `TransferBytes()` = GB × 2^30 on purchase |
| speed_limit | *int | Mbps |
| device_limit | *int | |
| capacity_limit | *int | max active subscribers (null = unlimited) |
| prices | json map[string]int64 | period → cents. Keys: `monthly`, `quarterly`, `half_yearly`, `yearly`, `two_yearly`, `three_yearly`, `onetime`, `reset_traffic`. A missing key means not for sale |
| reset_traffic_method | *int8 | null = follow the `reset_traffic_method` setting. 0 on the 1st of each month, 1 monthly on the purchase (expiry) day, 2 never, 3 on Jan 1, 4 yearly on the purchase (expiry) day |
| show | bool | visible in the shop |
| sell | bool | purchasable (false = legacy plan, renew only) |
| renew | bool | existing users may renew |
| sort | int | |
| created_at, updated_at | time | |

## server_groups

`id, name varchar(191), created_at, updated_at`.

## nodes

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| type | varchar(16) idx, not null | `vmess`, `vless`, `shadowsocks`, `shadowsocksr`, `trojan`, `hysteria`, `anytls`, `mieru`, `tuic` |
| name | varchar(191) not null | |
| group_ids | json []uint64 | groups allowed to use the node |
| route_ids | json []uint64 | `node_routes` applied (XrayR routes / soga audit rules) |
| parent_id | *uint64 idx | child node (relay entrance): served as its parent, see docs/NODE_PROTOCOLS.md §5.3 |
| host | varchar(255) not null | address shown to clients |
| port | varchar(32) not null | client port: `443`, a range `20000-30000` or a comma list of both (port hopping) |
| server_port | int not null | port the backend listens on (a child without one takes its parent's) |
| tags | json []string | at most 32, each ≤ 64 characters |
| rate | float64 not null | traffic multiplier 0–100 (admin create defaults to 1) |
| show | bool | |
| sort | int | |
| protocol_settings | json object | per type, below |
| created_at, updated_at | time | `created_at` also seeds the derived SS2022 / SSR node keys |

`protocol_settings` by `type`. Every key is optional: `model.<Type>Settings.Decode` applies the defaults from
`internal/model/protocol.go` and then overlays the stored JSON. Values below are the defaults; alternatives are listed in
comments. A child node's `protocol_settings` are not validated or used (the parent's apply).

```jsonc
// vmess
{"network":"tcp",            // tcp | ws | grpc | http | h2 | httpupgrade | xhttp
 "network_settings":{}, "tls":0, // tls: 0 none | 1 tls
 "tls_settings":{"server_name":"","allow_insecure":false}}
// vless
{"network":"tcp", "network_settings":{},
 "tls":0,                     // 0 none | 1 tls | 2 reality
 "flow":"",                   // "" | xtls-rprx-vision | xtls-rprx-vision-udp443
 "encryption":"none",         // client-side value written into subscriptions
 "decryption":"",             // server-side VLESS encryption: "" / "none" or mlkem768x25519plus.<mode>.<ticket>[.<padding>].<private key>
 "tls_settings":{"server_name":"","allow_insecure":false,
   "public_key":"","private_key":"","short_id":"",   // reality: X25519 keys (base64url), short_id = even-length hex ≤ 16
   "dest":"example.com:443","server_port":"443",     // reality handshake target ("host" or "host:port")
   "fingerprint":"chrome"}}
// shadowsocks
{"cipher":"aes-128-gcm",      // aes-128-gcm | aes-256-gcm | chacha20-ietf-poly1305 | 2022-blake3-aes-128-gcm | 2022-blake3-aes-256-gcm | 2022-blake3-chacha20-poly1305
 "server_key":"",             // SS2022 only: base64 of 16/32 bytes; empty = derived from created_at
 "obfs":"plain",              // plain | http | tls
 "obfs_settings":{"path":"/","host":""}, "plugin":"", "plugin_opts":""}
// shadowsocksr
{"cipher":"chacha20-ietf", "password":"",   // password: node-level SSR password; empty = derived from created_at
 "protocol":"origin", "protocol_param":"", "obfs":"plain", "obfs_param":"",
 "single_port_type":""}       // "" | protocol | obfs
// trojan
{"network":"tcp",            // tcp | ws | grpc
 "network_settings":{}, "server_name":"", "allow_insecure":false}
// hysteria
{"version":2,                // 1 | 2
 "up_mbps":100, "down_mbps":100,              // 0–100000
 "ignore_client_bandwidth":false,             // hysteria2: ignore client-declared bandwidth (BBR)
 "obfs":"plain", "obfs_password":"",          // plain | salamander (password required)
 "server_name":"", "allow_insecure":false, "alpn":"h3", "hop_interval":30}
// anytls
{"tls":1,                    // 1 tls | 2 reality
 "server_name":"", "allow_insecure":false, "padding_scheme":[],
 "tls_settings":{"public_key":"","private_key":"","short_id":"","dest":"","server_names":[]}}
// mieru
{"transport":"TCP",          // TCP | UDP
 "multiplexing":"MULTIPLEXING_LOW"}  // MULTIPLEXING_OFF | _LOW | _MIDDLE | _HIGH
// tuic
{"congestion_control":"bbr", // cubic | bbr | new_reno
 "zero_rtt_handshake":false,
 "udp_relay_mode":"native",  // native | quic
 "server_name":"", "allow_insecure":false, "alpn":"h3"}
```

`network_settings` follows the xray style and is normalised by `model.DecodeNetwork`:

- ws: `{"path":"/","headers":{"Host":"..."}}`
- grpc: `{"serviceName":"..."}` (`service_name` is also accepted)
- http/h2: `{"path":"/","host":["..."]}`
- httpupgrade: `{"path":"/","host":"..."}`
- xhttp: `{"path":"/","host":"","mode":"auto","extra":{}}`
- tcp: `{"header":{"type":"none|http","request":{"path":["/"],"headers":{"Host":["..."]}}}}`

Reality `tls_settings.dest` is stored as `host:port`. The node APIs split it into `dest` (host) and `server_port`
(string); see docs/NODE_PROTOCOLS.md.

## node_routes

`id, remarks varchar(191), match json []string, action varchar(16) not null, action_value varchar(255), created_at, updated_at`.

- `action` is `block` or `dns`.
- `match` holds 1–999 entries. `block` entries joined with `|` must compile as one Go regexp.
- A `dns` route needs `action_value` (the DNS server) unless `match[0]` is `main`.
- Routes are delivered to XrayR/V2bX as `routes[]`; `block` routes become soga `audit_rules`.

## orders

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| trade_no | varchar(32) unique, not null | `YYYYMMDDHHMMSS` (local time) + 6 random digits |
| user_id | uint64 idx, not null | |
| plan_id | uint64 idx, not null | |
| invite_user_id | *uint64 idx | inviter credited with the commission |
| coupon_id | *uint64 idx | |
| payment_id | *uint64 | |
| type | int8 not null | 1 new, 2 renew, 3 upgrade, 4 reset_traffic |
| period | varchar(16) not null | price key (see `plans.prices`) |
| callback_no | varchar(191) | gateway transaction id, or `free` / `balance` / `manual` |
| total_amount | int64 | final payable amount |
| handling_amount | int64 | gateway fee |
| discount_amount | int64 | coupon + user discount |
| surplus_amount | int64 | credit from the previous plan on upgrade |
| refund_amount | int64 | returned to the balance when the surplus exceeds the price |
| balance_amount | int64 | paid from the balance |
| surplus_order_ids | json []uint64 | orders whose remaining value was credited |
| status | int8 idx | 0 pending, 1 processing, 2 cancelled, 3 completed, 4 discounted (superseded by an upgrade) |
| commission_status | int8 idx | 0 pending, 1 processing (approved), 2 valid (paid out), 3 invalid |
| commission_balance | int64 | commission amount |
| paid_at | *time idx | |
| created_at | time idx | |
| updated_at | time | |

A user can hold at most one order in status 0 or 1. "Paid" for statistics means status 1, 3 or 4, bucketed by `paid_at`.

## payments

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| uuid | varchar(32) unique, not null | 32 hex characters; part of the notify URL `/api/v1/guest/payment/notify/:gateway/:uuid` |
| gateway | varchar(32) not null | registered gateway name (`epay`, `epusdt`, `alipay_f2f`, `stripe`, `coinbase`, `btcpay`) |
| name | varchar(191) not null | |
| icon | varchar(255) | `""`, `preset:<id>` (built-in mark), image URL / `/path` / `data:image/…`, or one emoji — see [API.md › Payment icons](API.md#payment-icons) |
| config | json map[string]string | gateway settings; secret, admin only |
| notify_domain | varchar(255) | overrides `app_url` for callback URLs |
| handling_fee_fixed | int64 | cents |
| handling_fee_percent | float64 | |
| enabled | bool | |
| sort | int | |
| created_at, updated_at | time | |

## coupons

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| code | varchar(64) unique, not null | |
| name | varchar(191) not null | |
| type | int8 not null | 1 fixed amount (cents), 2 percent |
| value | int64 not null | |
| show | bool | |
| limit_use | *int | **remaining** uses, null = unlimited |
| limit_use_with_user | *int | uses per user |
| limit_plan_ids | json []uint64 | |
| limit_periods | json []string | |
| started_at, ended_at | time not null | |
| created_at, updated_at | time | |

`limit_use` follows Xboard: one use is taken when an order is created and given back when that order is cancelled
(Xboard does not give it back). The admin `used_count` counts orders with status ≠ cancelled.
`limit_use_with_user` counts the user's paid orders (status not pending or cancelled).

## invite_codes

`id, user_id uint64 idx, code varchar(32) unique, status bool, pv int, created_at, updated_at`.
New codes are 32 random alphanumeric characters (imported Xboard codes keep their length). `status` = consumed; it is only
set when `invite_never_expire` is false.

## commission_logs

`id, invite_user_id uint64 idx, user_id uint64, trade_no varchar(32) idx, order_amount int64, get_amount int64, created_at idx`.

`user_id = 0` (and `ticket_messages.user_id = 0`) means the user was not imported: `import xboard --skip-banned` keeps the
row but drops the reference to a banned user (docs/IMPORT_XBOARD.md §3.5).

## tickets / ticket_messages

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| user_id | uint64 idx, not null | |
| subject | varchar(255) not null | |
| level | int8 | 0 low, 1 medium, 2 high |
| status | int8 idx | 0 open, 1 closed |
| reply_status | int8 idx | 0 awaiting staff, 1 replied |
| last_reply_user_id | *uint64 | |
| created_at, updated_at | time | |

ticket_messages: `id, ticket_id uint64 idx, user_id uint64, message text not null, created_at`.

## notices

`id, title varchar(255), content text (markdown), img_url varchar(255), tags json []string, show bool, sort int, created_at, updated_at`.

## knowledge

`id, language varchar(5) idx, category varchar(191), title varchar(255), body text (markdown), sort int, show bool, created_at, updated_at`.

## settings

`key varchar(64) PK, value text (JSON-encoded), updated_at`. Defaults live in code (`internal/settings/defs.go`,
`settings.Defaults`); the full key list is in docs/API.md "Settings keys". Use `internal/settings`, never this table.

## stat_users / stat_nodes / stats

| table | columns | indexes |
| --- | --- | --- |
| stat_users | `id, user_id uint64, node_id uint64, rate float64, u int64, d int64, record_at date` | `user_id`, `node_id`, `record_at`; unique `uniq_stat_users_day(user_id, node_id, record_at)` |
| stat_nodes | `id, node_id uint64, u int64, d int64, record_at date` | `record_at`; unique `uniq_stat_nodes_day(node_id, record_at)` |
| stats | `id, record_at date, order_count, order_total, paid_count, paid_total, commission_count, commission_total, register_count, invite_count, transfer_used_total` (int64) | unique `record_at` |

- `stat_users.u/d` are already multiplied by the node rate. `stat_nodes.u/d` are raw bytes.
- `stats` is written by the `stats_daily` job:
  - `order_*`: orders created that day.
  - `paid_*`: orders with status 1, 3 or 4, by `paid_at`.
  - `commission_*`: `commission_logs.get_amount`.
  - `register_count` / `invite_count`: users created that day, and how many of them were invited.
  - `transfer_used_total`: `stat_nodes` u + d.

## mail_logs

`id, email varchar(191) idx, subject varchar(255), template varchar(64), error *text (null = sent), created_at`.

## mail_templates

`id, name varchar(32), locale varchar(5), subject varchar(255), body text (markdown), created_at, updated_at`;
unique `uniq_mail_templates_name_locale(name, locale)`.

One row per built-in template name (`internal/mail.TemplateNames`) and locale (`zh-CN`, `en`) that an admin has customised
(docs/API.md "Mail templates"). No row = the built-in template. `subject` is stored trimmed (≤ 255 characters, one line);
`body` is Markdown with `{{name}}` placeholders, at most 65535 bytes (fits MySQL `TEXT`). The row is validated on write
(upsert on the unique index) and the mail queue reloads all rows after every change (`mail.Queue.ReloadTemplates`, also at
start; a load error is logged and the built-ins are used), so nothing else caches them. Rows of unknown names or locales are
ignored. Not imported from Xboard (`v2_mail_templates` differs) and not cleared by `import xboard --force`.

## rule_profiles

A user's named rule profiles (at most 10, exactly one default; docs/SUBSCRIPTION.md "Multiple profiles").

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| user_id | uint64 not null | first column of the unique index below, which also serves lookups by user |
| slug | varchar(32) not null | `^[a-z0-9][a-z0-9_-]{0,31}$`; unique per user: `idx_rule_profiles_user_slug (user_id, slug)` |
| name | varchar(64) not null | |
| is_default | bool | the profile used without `?profile=`; when no row is flagged, the oldest row is the default |
| data | json | complete `RuleProfile` document, or `null` while the profile follows the admin's default template |
| created_at, updated_at | time | |

Upgrade from the first release (`user_id` primary key, one row per user): `db.Migrate` renames the old table to
`rule_profiles_v1` before AutoMigrate (detected by the missing `id` column), creates the new table, copies every row as
`slug = "default"`, `name = "默认"`, `is_default = true` (same `data`, `created_at = updated_at` = the old `updated_at`;
users that already have a `default` row keep it) and drops `rule_profiles_v1`. An interrupted upgrade finishes on the next
start; when both tables exist with the old schema, migration stops with an error instead of guessing. Tested on SQLite
and (integration tag) PostgreSQL 16 and MySQL 8.4.

## subscription_keys

One row per user who created an age key (docs/SUBSCRIPTION.md "Encrypted subscriptions (age)").

| column | type | notes |
| --- | --- | --- |
| user_id | uint64 PK (no auto-increment) | |
| public_key | varchar(128) not null | age X25519 recipient `age1…` |
| private_key_enc | text not null | `v1.` + base64url(nonce ‖ AES-256-GCM ciphertext) of the `AGE-SECRET-KEY-1…` identity. Key = HKDF-SHA256(`jwt.secret`, info `ggboard/subscription/age-identity/v1`), AAD = `ggboard:age:<user id>` |
| enabled | bool | encrypt supported formats |
| created_at | time | creation of the current key pair (regeneration resets it) |

## subscription_access

Access log of successful subscription fetches (docs/SUBSCRIPTION.md "Access log and leak detection"). Written in batches
every 10 s; `subscription_access_prune` keeps the newest 100 rows per user and nothing older than 30 days.

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| user_id | uint64 not null | `idx_subscription_access_user_at (user_id, at)` |
| ip | varchar(45) | client IP (IPv4-mapped addresses stored as IPv4) |
| ip_bucket | varchar(64) | the IPv4 address, or the IPv6 /64 prefix (`2001:db8:1:2::/64`); leak detection counts distinct buckets |
| token_tag | varchar(16) | first 8 hex digits of SHA-256 of the token used; a rotated token starts a new leak window |
| client | varchar(64) | first product token of the User-Agent (`clash-verge/v2.2.3`), `unknown` when empty |
| format | varchar(16) | rendered format (`clash-meta`, `v2ray`, …) |
| at | time idx | |

## user_mfa

One row per user, created on first use (`internal/service/mfa`).

| column | type | notes |
| --- | --- | --- |
| user_id | uint64 PK (no auto-increment) | |
| totp_secret | text | `v1.` + base64url(nonce ‖ AES-256-GCM ciphertext). Key = HKDF-SHA256(`jwt.secret`, info `ggboard/mfa/totp-secret/v1`), AAD = `ggboard:totp:<user id>`. Empty when TOTP is off. Rotating `jwt.secret` invalidates every enrolled authenticator |
| totp_enabled | bool | |
| totp_enabled_at | *time | |
| totp_last_step | int64 | last accepted TOTP time step (unix / 30); replay guard |
| backup_codes | json []string | hex SHA-256 of `ggboard-backup-code:<code>` for each unused backup code |
| webauthn_handle | *varchar(64) unique | hex of the 16-byte WebAuthn user handle shared by all of the user's passkeys |
| created_at, updated_at | time | |

## passkeys

| column | type | notes |
| --- | --- | --- |
| id | uint64 PK | |
| user_id | uint64 idx, not null | |
| rp_id | varchar(255) not null | RP ID the credential was registered for; only passkeys of the current RP ID are used to sign in |
| credential_hash | varchar(64) unique, not null | hex SHA-256 of the raw credential id (portable unique lookup) |
| credential_id | text not null | base64url (unpadded) credential id |
| credential | text not null | JSON of the go-webauthn `Credential` (public key, flags, attestation, transports) |
| name | varchar(64) not null | |
| sign_count | int64 | authenticator signature counter; must increase when non-zero |
| backup_eligible, backup_state | bool | BE / BS flags |
| transports | json []string | |
| last_used_at | *time | |
| created_at, updated_at | time | |

`sign_count`, `backup_state` and `last_used_at` are updated after every successful assertion.

## Ephemeral state (memory only, lost on restart)

Kept in `app.KV` (a TTL map that never evicts early) or `app.Online`:

- Email verification codes (10 min) and their resend cooldown, failed-password counters per email
  (`password_limit_*`) and registrations per IP (`register_limit_*`).
- MFA:
  - pending TOTP secrets (10 min)
  - MFA login tokens (5 min, single use)
  - WebAuthn ceremony state (2 min, single use)
  - wrong second-factor counters (1 h window)
  - the cached "has a second factor" flag used by the admin gate (30 s)
- One-time admin login tokens (`/admin/users/:id/login-token`, 5 min, single use).
- Reminder markers of `expire_remind` / `traffic_remind` (24 h).
- Node runtime state (`online.Status`): `online`, `last_check_at`, `last_push_at`, `load{cpu,mem,swap,disk,uptime}` and
  `version` per effective node. This drives the three-state node status (docs/NODE_PROTOCOLS.md §5.1).
- Alive IPs per user and node, with TTL `max(300 s, 2 × server_push_interval)` (docs/NODE_PROTOCOLS.md §5.2).
- Buffered traffic (`app.Traffic`), flushed to `users`, `stat_users` and `stat_nodes` every 10 s and on shutdown.
- Buffered subscription access rows (at most 10 000), flushed to `subscription_access` every 10 s (not on shutdown); the
  leak-detection markers "bucket already counted for this token" and "token already alerted" (24 h each).
- Telegram: one-time binding codes (`/start <code>`, 10 min; a new code revokes the previous one). The bot username from
  `getMe` is cached in `app.Cache` for 1 h.
- Pending notifications in the notification queue (at most 1024, plus 256 in the priority lane); on shutdown the queue drains for up to 10 s, the rest
  is lost.
