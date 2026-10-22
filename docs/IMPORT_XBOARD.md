# Import from Xboard — `ggboard import xboard`

Spec of `internal/importer/xboard` (package `xboardimport`; the code implements this file). You do not need the PHP sources: every rule below was checked against
the user's fork `linusxiong/Xboard` (Laravel 10, **legacy**, "L") and `cedar2025/Xboard` master as of 2026-05 (**upstream**, "U").
Target schema: `docs/DATA_MODEL.md` (`internal/model`). Notation: `ts(x)` = `time.Unix(x,0).UTC()`; **W** = emit a warning;
"—" = the column does not exist in that version.

## 0. Guarantees

- The source is opened read-only, in one consistent snapshot. The importer never writes to it.
- All target writes run in **one transaction**. Any fatal error rolls back, and the target is left unchanged.
- Primary keys are kept, so `invite_user_id`, `plan_id`, `group_id`, `parent_id` and `ticket_id` stay valid. The one
  exception is legacy nodes whose ids collide (§3.2).
- `v2_order` rows are **never** imported (§7, §8 list the consequences).
- Every lossy conversion emits a W line. No data is dropped silently.

## 1. Connection and detection

### 1.1 DSN (`--dsn "<scheme>://<rest>"`)

| scheme | Go driver | `<rest>` | example |
|---|---|---|---|
| `mysql://` | `github.com/go-sql-driver/mysql` | go-sql-driver DSN, or URL form `user:pass@host[:3306]/db` | `mysql://xboard:secret@tcp(127.0.0.1:3306)/xboard`, `mysql://xboard:secret@127.0.0.1:3306/xboard` |
| `postgres://`, `postgresql://` | `pgx/v5/stdlib` | whole URL, unchanged | `postgres://xboard:secret@127.0.0.1:5432/xboard?sslmode=disable` |
| `sqlite://`, `sqlite3://` | `github.com/glebarez/sqlite` (pure Go) | file path (`sqlite://rel/path` is relative to the working directory) | `sqlite:///srv/xboard/database/database.sqlite` |

With `--driver mysql|postgres|sqlite`, `--dsn` may be a raw driver DSN without a scheme (`ParseDSN` in `dsn.go`).

- **mysql**: the importer forces `parseTime=false`, `charset=utf8mb4`, `interpolateParams=true` and `loc=UTC`, and removes
  any `loc` / `time_zone` parameter. So `?parseTime=true` in the user's DSN is accepted and then overridden. **Never set
  `time_zone`.** Reasons in §1.2.
- **sqlite**: open with `file:<path>?mode=ro&_pragma=busy_timeout(5000)`. The file is whatever Xboard's `.env`
  `DB_DATABASE` points to, relative to the Xboard root.
- **postgres**: tables live in `current_schema()`, which is `public` because Xboard sets `search_path=public`. A
  `search_path` in the URL is honoured.
- **Table prefix**: always `v2_`. Xboard hard-codes its table names and Laravel's `prefix` is `''` in both versions. There is
  no `--prefix` flag.
- The DSN password is never printed. The summary shows only the driver, host and database name.

### 1.2 Reading rules

- **Snapshot**: mysql `START TRANSACTION WITH CONSISTENT SNAPSHOT, READ ONLY`; pg `BEGIN ISOLATION LEVEL REPEATABLE READ
  READ ONLY`; sqlite one read transaction. Stream rows and insert them into the target in batches of 500.
- **Flex scanners are mandatory**, because one Laravel column has different types per driver. `boolean()` is `tinyint(1)` on
  MySQL and SQLite but a real `boolean` on PG. Legacy `v2_plan.reset_traffic_method` and `v2_server_vless.tls` are
  `boolean()` columns that hold 0–4 and 0–2. `decimal` and JSON columns arrive as strings.
  - `flexInt`: int64, bool (1/0), a digit string or []byte (trimmed), or a float. NULL → nil.
  - `flexBool`: true for `1 "1" "true" "on" "yes"` and bool true; false for `0 "0" "" "false" "off" "no"` and NULL.
  - `flexFloat`: numeric string or decimal. `flexStr`: trims trailing spaces (PG `char(n)` pads, e.g. `password_algo char(10)`).
- **Unix int columns** (nearly all Xboard timestamps): use `ts()`. A value above `1e11` is milliseconds: divide by 1000 and W.
  A `created_at` of 0 becomes the import start time.
- **Datetime columns** exist only in U (`v2_server.created_at`/`updated_at`, `v2_plugins`). Read them as text (`CAST(c AS
  CHAR)` mysql, `c::text` pg, plain sqlite) and parse with `time.ParseInLocation("2006-01-02 15:04:05", s, srcTZ)`. Laravel
  writes wall-clock strings in `app.timezone` (hard-coded `Asia/Shanghai` in both versions) and leaves the MySQL session
  time zone alone, so reading in the server's default session zone returns exactly the strings Laravel wrote.
- **`--source-tz`** (default `Asia/Shanghai`) is used to turn datetimes into instants, stat `record_at` midnights into dates
  (§4.14), and to compute `next_reset_at` (§4.5.2).

### 1.3 Detection probes

- **Tables**: mysql `information_schema.tables WHERE table_schema=DATABASE()`; pg `… WHERE table_schema=current_schema()`;
  sqlite `sqlite_master WHERE type='table'`.
- **Columns**: `information_schema.columns`, or `PRAGMA table_info(t)` on sqlite.

| probe | meaning |
|---|---|
| `v2_user` or `v2_settings` missing | fatal: "not an Xboard database" |
| `v2_server` exists and no `v2_server_<proto>` table exists | **upstream** flavor |
| any of `v2_server_{shadowsocks,vmess,trojan,vless,hysteria}` exists and `v2_server` does not | **legacy** flavor |
| both kinds exist | fatal: "half-applied 2025_01_05 server migration, finish it in Xboard first" |
| neither kind exists | fatal: "not an Xboard database" |
| other `v2_server_*` tables (not `v2_server_group`, `v2_server_route`, `v2_server_log`, `v2_server_stat`, `v2_server_machine*`) | ignored, W |
| `v2_plan.prices` | prices are a JSON map in **currency units**. Without it, read the 8 `*_price` columns, which are in **cents** |
| `v2_plan.sell`, `.device_limit`, `.tags`; `v2_user.device_limit`; `v2_notice.sort` | read when present, otherwise use the defaults below |
| `v2_ticket.last_reply_user_id` | U ≥ 2026-04-21, where `reply_status` meaning is already fixed (§4.10) |
| `v2_server_hysteria.version` / `.is_obfs` / `.alpn`; `v2_server_trojan.network` / `.networkSettings` | old forks may lack them. Defaults: version 1, is_obfs 1, alpn 0, network `tcp` |
| `v2_plugins` | upstream payment-plugin enable state (§4.9) |
| `migrations` | the summary prints the row count and the last `migration` name |

Run each column probe independently of the flavor. Forks cherry-pick migrations.

## 2. Shared parsers

- **`parseIDList(raw) []uint64`**, used for `group_id`, `route_id`, `group_ids`, `route_ids` and `limit_plan_ids`. Accepts
  NULL/`""` → `[]`; a JSON array of numbers or numeric strings (`["1","2"]`, `[1,2]`); a JSON **object** (PHP `array_filter` then
  `json_encode` produces `{"0":"1","2":"3"}`: take values in key order); a bare scalar `3` / `"3"`; non-JSON text split
  on `,`, `\n` and `|`. Non-numeric or ≤ 0 elements are dropped with W; duplicates removed, order kept.
- **`parseStringList(raw) []string`**: the same shapes, for `tags`, `match`, `email_whitelist_suffix`, etc. Trim each item
  and drop empty ones (and repeats). Plain text splits on newlines when it has any, else on `,`; `|` never splits
  strings, because route rules contain regex alternations such as `regexp:(a|b)\.example\.com`.
- **`cents(v)`**: `int64(math.Round(v*100))`. Decimal strings such as `"19.99"` give 1999.
- **`nz(v)`**: 0 or NULL becomes `nil`. Used for Xboard columns where "0 = unlimited".

## 3. Order, IDs, idempotency

### 3.1 Write order (one transaction)

settings → server_groups → plans → node_routes → nodes (two passes: insert with `parent_id` NULL, then `UPDATE parent_id`)
→ users (two passes for `invite_user_id`) → invite_codes → commission_logs → coupons → payments → tickets → ticket_messages
→ notices → knowledge → stats, stat_nodes, stat_users (only with `--with-stats`) → sequence reset (§3.3).
Reference checks run against in-memory sets of imported ids. The target has no DB-level FKs
(`DisableForeignKeyConstraintWhenMigrating`).

Implementation notes: settings are translated right after plans, because `try_out_plan_id` is checked against the
imported plans. Every node id and user id is known before the first insert (nodes are read completely, and a
pre-pass over `v2_user` resolves email collisions), so `parent_id` and `invite_user_id` are resolved in memory and
each table is inserted in one pass. The result is the same as the two passes above.

### 3.2 IDs

- **Kept ids**: every table except `nodes` and `stat_*` is inserted with the source `id`. Upstream nodes keep `v2_server.id`.
- **Legacy nodes**: the 5 legacy tables each have their own id space, so ids can collide.
  1. Process types in Xboard's own merge order: shadowsocks, vmess, trojan, hysteria, vless. Within a type, go by id ascending.
  2. A row keeps its legacy id if no earlier row has taken it.
  3. Colliding rows get `max(all legacy server ids)+1, +2, …`, in processing order.
  4. Record every result in `nodeMap[(type, legacyID)] → nodeID`. Use the map for `parent_id` (always same-type in L) and for
     `v2_stat_server(server_type, server_id)`.

  Each renumbered node emits W: `vmess #3 "HK-01" → node 12: change NodeID in this server's XrayR/soga config`. Backends
  address nodes by `node_id`+`node_type`. The user's live install is shadowsocks-only, so no node is renumbered there.
- **Upstream `code`**: `ServerService::getServer` matches `code` **before** `id`, and the 2025-01 migration set
  `code = <legacy id>` on every converted node. When `code` is non-empty and differs from `id`, emit W:
  `backend may use NodeID <code>; ggboard resolves by id → set NodeID=<id>`.
- **stat_\* rows** get fresh ids.

### 3.3 Sequences

- **PostgreSQL**: before commit, run this for every table with kept ids:
  `SELECT setval(pg_get_serial_sequence('<t>','id'), COALESCE((SELECT MAX(id) FROM <t>),0)+1, false);`
- **MySQL (InnoDB) and SQLite**: no action needed. InnoDB raises AUTO_INCREMENT on explicit ids, and SQLite `AUTOINCREMENT`
  updates `sqlite_sequence`.

### 3.4 Idempotency

- **Default**: abort with exit 1, writing nothing, if any of these target tables has rows: users, plans, server_groups, nodes,
  node_routes, orders, payments, coupons, invite_codes, commission_logs, tickets, ticket_messages, notices, knowledge,
  stat_users, stat_nodes, stats, rule_profiles, passkeys, user_mfa (second factors are keyed by user id, so leftovers would
  attach to the imported user that receives the same id).
  - Run the import right after `ggboard migrate` and **before** `ggboard admin create`. Imported `is_admin` users stay admins.
  - The `settings` table is allowed to be non-empty, because defaults are seeded on boot.
  - Not in the list: `user_mfa` and `passkeys` (and `mail_logs`, which is keyed by email). They are neither checked nor
    cleared, so `--force` on a panel that already had users keeps their 2FA and passkey rows, keyed by user ids that the
    import then reuses. Delete them by hand before a `--force` run (tracked as a code bug).
- **`--force`**: inside the same transaction, `DELETE FROM` all the tables above (children first), then import. Imported
  settings keys are upserted. Other settings keys are not touched.
- **No upsert-by-id mode**. Rows deleted in Xboard would linger, and node remaps are not stable between runs. To refresh,
  re-run with `--force`.
- **`--dry-run`**: performs every read and write (so unique constraints are exercised), then rolls back. Its summary is
  identical to a real run. It skips only two things: the PostgreSQL sequence reset (§3.3; `setval` is not
  transactional and would survive the rollback) and the reload of the running settings snapshot.

## 4. Table mappings

### 4.1 server_groups ← `v2_server_group` (L = U)

`id`, `name` verbatim. `created_at`, `updated_at` = `ts()`.

### 4.2 plans ← `v2_plan`

| ours | L | U | conversion |
|---|---|---|---|
| id | id | id | keep |
| group_id | group_id (NOT NULL) | group_id (nullable) | NULL or unknown group → 0, W "plan grants no group" |
| name, content | name, content | same | `content` NULL → `""` |
| tags | — | tags (JSON) | `parseStringList`; L → `[]` |
| transfer_enable | transfer_enable (**GB**) | transfer_enable (**GB**) | verbatim; NULL → 0. The U column comment says "bytes" but the code multiplies by 1073741824 on purchase |
| speed_limit | speed_limit | speed_limit | `nz()` |
| device_limit | — | device_limit | `nz()` |
| capacity_limit | capacity_limit | capacity_limit | verbatim. NULL = unlimited; **0 = sold out**, because both versions check `limit − active > 0` |
| prices | 8 cents columns | `prices` JSON | see below |
| reset_traffic_method | reset_traffic_method | same | `flexInt`; NULL → null (follow setting); outside 0..4 → null, W |
| show | show | show | `flexBool` |
| sell | = `show` (U's migration did the same) | sell | `flexBool` |
| renew | renew | renew | `flexBool` |
| sort | sort | sort | NULL → 0 |

**Prices, legacy (cents)**: `month_price`→`monthly`, `quarter_price`→`quarterly`, `half_year_price`→`half_yearly`,
`year_price`→`yearly`, `two_year_price`→`two_yearly`, `three_year_price`→`three_yearly`, `onetime_price`→`onetime`,
`reset_price`→`reset_traffic` (same table as U's `convert_order_period_fields`). NULL → key absent (not for sale). 0 is
kept, because L lets users buy it for free; W.

**Prices, upstream**: the keys are already ours (`Plan::PERIOD_*`), but the values are **decimal currency units** (the
2025-01 migration divided the old cents by 100). Value → `cents(v)`. NULL, `""` or ≤ 0 → key absent (U's
`getActivePeriods` hides them and its admin form strips them). Unknown keys are dropped with W.

### 4.3 node_routes ← `v2_server_route`

| ours | L / U | conversion |
|---|---|---|
| id, remarks | id, remarks | verbatim |
| match | match (text) | `parseStringList`. Usually a JSON array, but can be a JSON object (because of `array_filter`) or a newline/comma list from pre-JSON v2board |
| action | action | `block`, `dns` verbatim. U also allows `direct` and `proxy`: **skip** such a route with W and drop it from every node's `route_ids` |
| action_value | action_value | NULL → `""` |
| created_at, updated_at | int | `ts()` |

### 4.4 nodes ← `v2_server_<type>` (L) / `v2_server` (U)

**Common columns**:

| ours | L | U | conversion |
|---|---|---|---|
| id | §3.2 | id | |
| type | table suffix | type | U: normalise `v2ray→vmess`, `hysteria2→hysteria`. U `socks`, `naive`, `http` → skip node, W |
| name, host | same | same | verbatim |
| group_ids | group_id (JSON in a string) | group_ids (JSON, string elements) | `parseIDList`; drop unknown groups with W |
| route_ids | route_id | route_ids | `parseIDList`; drop unknown or skipped routes |
| parent_id | parent_id (same-type id) | parent_id (`v2_server.id`) | L: map through `nodeMap`. Missing parent → NULL, W. A grandchild is allowed but gets W |
| port | port (string; `int` on vless) | port | string; ranges such as `20000-30000` kept |
| server_port | server_port | server_port | int |
| tags | tags | tags | `parseStringList` |
| rate | rate (string(11)) | rate (decimal) | `flexFloat`; invalid → 1.0, W |
| show | show | show | `flexBool`. U: if `enabled` is false → `show=false`, W |
| sort | sort | sort | NULL → 0 |
| created_at, updated_at | int → `ts()` | datetime (§1.2) | |

**Dropped columns** (W when non-empty or enabled):

- L: `excludes` (hides nodes from mainland-China visitors) and `ips`. In the fork, `ips` lets child nodes with their own
  rate account traffic by backend IP. Ours always counts on the parent.
- U: `code` (§3.2), `machine_id`, `transfer_enable`/`u`/`d` (node quota), `rate_time_enable`/`rate_time_ranges`,
  `custom_outbounds`, `custom_routes`, `cert_config`.

**Materialised keys.** Xboard derives the SS2022 server PSK and the hysteria obfs password from the node's `created_at`:
`base64(hex(md5(decimal(created_at)))[0:n])`. That is std base64, with padding, of the first n **hex characters**.

- For child nodes, use the **parent's** `created_at`. Both versions do this for clients, and the backend polls the parent.
- Write the result into `protocol_settings` instead of relying on our `created_at` fallback. That fallback ignores parents,
  and U datetimes depend on the time zone.
- Key length n:
  - 16 for `2022-blake3-aes-128-gcm` and for hysteria;
  - 32 for `2022-blake3-aes-256-gcm` and `2022-blake3-chacha20-poly1305`.
- Example: created_at `1700000000` gives `K16 = MjQ5MjBkZWNmODNmNjk2MA==`.

**protocol_settings from L columns**:

| type | ours |
|---|---|
| shadowsocks | `cipher` = cipher. A cipher outside our enum (L also allowed `aes-192-gcm`) is kept, with W.<br>`obfs` = `"http"` if obfs = `http`, else `"plain"`.<br>`obfs_settings` = JSON(obfs_settings) `{path,host}` when http.<br>`plugin` = `""`, `plugin_opts` = `""`.<br>`server_key` = K(n) for 2022 ciphers only. |
| vmess | `network` = network. `kcp`, `quic` and `domainsocket` are kept with `show=false`, W.<br>`network_settings` = JSON(`networkSettings`), verbatim (already Xray style).<br>`tls` = tls (0/1).<br>`tls_settings` = `{server_name: tlsSettings.serverName ?? tlsSettings.server_name, allow_insecure: flexBool(tlsSettings.allowInsecure ?? .allow_insecure)}`. L stores **camelCase** keys here.<br>`rules`, `ruleSettings` and `dnsSettings` are dropped (W if non-empty). |
| trojan | `network` = network ?? `"tcp"`.<br>`network_settings` = JSON(`networkSettings`). The column name is camelCase.<br>`server_name` = server_name.<br>`allow_insecure` = `flexBool(allow_insecure)`. |
| vless | `network` = network, with `splithttp` renamed to `xhttp`. Unsupported values are handled as for vmess.<br>`network_settings` = JSON(network_settings).<br>`tls` = `flexInt(tls)`: 0, 1, or 2 (2 = reality).<br>`flow` = flow ?? `""`.<br>`encryption` = `"none"`.<br>`tls_settings` = `{server_name, allow_insecure: allow_insecure ?? allowInsecure, public_key, private_key, short_id, server_port}`. When tls=2, also set `dest` = `server_name:server_port` (port defaults to `"443"`) and `fingerprint` = `"chrome"`. |
| hysteria | `version` = `flexInt(version)`. NULL or 0 → 1, W (1 is the column default and the only pre-2023-09 version). The 2023-09-29 migration declares the column `NOT NULL DEFAULT 1`, so NULL only appears as a missing column in old forks.<br>`up_mbps`, `down_mbps` = up_mbps, down_mbps.<br>`server_name` = server_name.<br>`allow_insecure` = insecure.<br>`alpn`: v1 maps `{0:"hysteria",1:"http/1.1",2:"h2",3:"h3"}[alpn]`; v2 uses `"h3"`.<br>`obfs` = is_obfs ? `"salamander"` : `"plain"`.<br>`obfs_password` = is_obfs ? K16 : `""`. In v1, a non-plain obfs means `obfs_password` *is* the obfs string (`docs/NODE_PROTOCOLS.md` §2.1.5). |

**protocol_settings from U `protocol_settings` JSON**. Every key that is not listed here is dropped. `multiplex`, `utls`,
`ech` and `encryption` produce W only when `enabled`/`open` is true.

| type | ours |
|---|---|
| shadowsocks | `cipher`, `obfs` (NULL → `"plain"`), `obfs_settings`, `plugin` ?? `""`, `plugin_opts` ?? `""`.<br>`server_key` = K(n) for 2022 ciphers. U sends no key for chacha20-2022; ours does. |
| vmess | `network`, `network_settings`, `tls`, `tls_settings{server_name, allow_insecure}`.<br>Dropped: `rules`, `multiplex`, `utls`, `tls_settings.ech`. |
| trojan | `network` ?? `"tcp"`, `network_settings`.<br>`server_name` = `tls_settings.server_name` ?: `server_name`. U's 2026-04-22 migration made `tls_settings` authoritative.<br>`allow_insecure` = `tls_settings.allow_insecure` OR `allow_insecure`.<br>If `tls=2` (reality): `show=false`, W. Our trojan has no reality. |
| vless | `tls`, `flow`, `network`, `network_settings`.<br>`encryption` = `"none"`. If `encryption.enabled` → W; the VLESS-encryption keys are dropped.<br>If tls=2, `tls_settings` comes from `reality_settings{server_name, server_port, public_key, private_key, short_id, allow_insecure}`, plus `dest` = `server_name:server_port` and `fingerprint` = `utls.fingerprint` ?: `"chrome"`.<br>If tls=1, `tls_settings` = `{server_name, allow_insecure}`. |
| hysteria | `version`.<br>`up_mbps` = `bandwidth.up`, `down_mbps` = `bandwidth.down`.<br>`obfs` = `obfs.open` ? (`obfs.type` ?: `"salamander"`) : `"plain"`.<br>`obfs_password` = `obfs.password` ?? `""`. The U migration materialised it from each node's *own* `created_at`, so for a child use the parent's password (the backend serves the parent's config); W if they differ. Open with an empty password → W.<br>`server_name` = `tls.server_name`, `allow_insecure` = `tls.allow_insecure`.<br>`hop_interval` if set. |
| tuic | `congestion_control`, `udp_relay_mode`.<br>`alpn` = join(alpn array, `","`).<br>`server_name`, `allow_insecure` from `tls`.<br>`zero_rtt_handshake` = false.<br>`version` ≠ 5 → W. |
| anytls | `tls` = 1.<br>`server_name`, `allow_insecure` from `tls`.<br>`padding_scheme` verbatim. |
| mieru | `transport` ?? `"TCP"`. `traffic_pattern` is dropped (W if non-empty). |

### 4.5 users ← `v2_user` (L = U, plus U-only columns)

| ours | source | conversion |
|---|---|---|
| id | id | keep |
| email | email | `lower(trim())`. Collisions: §5. U lower-cases emails on write; L does not |
| password_hash | password, password_algo, password_salt | §4.5.1 |
| uuid | uuid | verbatim (Xboard `guid(true)`, 36 chars). Empty, longer than 36 or duplicate → new UUIDv4 for the higher id, W ("re-import subscription"). uuid and token are credentials: warnings never print them |
| token | token | verbatim (32 hex). Empty, longer than 32 or duplicate → new token for the higher id, W ("old subscription URL dies") |
| plan_id | plan_id | plan not imported → NULL, W |
| group_id | group_id | group not imported → NULL, W |
| transfer_enable, u, d | same (bytes, bigint) | verbatim; a negative u/d → 0, W |
| expired_at | expired_at (bigint, nullable, default 0) | **NULL → null (never expires)**. **0 → `ts(created_at)`** (expired/never bought; Xboard's "unavailable" query is `expired_at < now OR expired_at = 0`). > 0 → `ts()` |
| next_reset_at | (U column ignored) | recomputed, §4.5.2 |
| speed_limit | speed_limit | `nz()` |
| device_limit | U device_limit; L — | `nz()` |
| balance, commission_balance | same (cents) | verbatim |
| commission_type | commission_type | 0, 1, 2 verbatim; anything else → 0, W |
| commission_rate | commission_rate (U casts to float) | round to int; NULL → null |
| discount | discount | `nz()` |
| invite_user_id | invite_user_id | user not imported, or self → NULL, W. Applied in the second pass |
| is_admin, is_staff, banned | same | `flexBool` |
| remind_expire, remind_traffic | same (nullable tinyint) | `flexBool`; **NULL → false** (Xboard checks `if ($user->remind_expire)`) |
| last_login_at | last_login_at | NULL → null, else `ts()` |
| last_login_ip | last_login_ip (int, never written by Xboard) | `""` |
| telegram_id | telegram_id | verbatim |
| remarks | remarks | NULL → `""` |
| password_version | — | 0 |
| created_at, updated_at | int | `ts()` |

Dropped columns: `t` (last traffic time); U-only `online_count`, `last_online_at`, `last_reset_at`, `reset_count`, `next_reset_at`.

#### 4.5.1 Password hashes

Xboard's `Helper::multiPasswordVerify` works like this. U added `sha256salt`.

| `password_algo` (trimmed) | Xboard verify | stored `password_hash` |
|---|---|---|
| NULL, `""`, unknown | PHP `password_verify` | verbatim if it starts with `$2` (bcrypt `$2y$`, which our bcrypt verifier accepts) or `$argon2id$` (PHP `PASSWORD_ARGON2ID`, same PHC string as ours). Otherwise `!xb-unusable`, W ("user must reset password") |
| `md5` | `md5(pw)` | `xb$md5$$<hash>` |
| `sha256` | `sha256(pw)` | `xb$sha256$$<hash>` |
| `md5salt` | `md5(pw . salt)` | `xb$md5salt$<salt>$<hash>` |
| `sha256salt` | `sha256(pw . salt)` | `xb$sha256salt$<salt>$<hash>` |

A stored hash that is not hex of the algorithm's length (32 for md5, 64 for sha256) is `!xb-unusable`, W.

**Format** `xb$<algo>$<salt>$<hexhash>`. Parse: strip `xb$`; `algo` = text up to the next `$`; `hash` = text after the
**last** `$` (hex, so no `$`); `salt` = everything in between (may be empty, may contain `$`).

**Verification** (`internal/auth`, a contract addition for that package): `subtle.ConstantTimeCompare(lowerhex(H(pw+salt)),
lower(hash))`. On success, rehash with argon2id and save; **do not** bump `password_version` (the user changed nothing).
A value starting with `!` never verifies. Vectors for `password123`: md5 `482c811da5d5b4bc6d497ffa98491e38`; sha256
`ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f`; md5salt with salt `abc$12` `1b84a6e599feeab25ace9f402aaa4858`.

#### 4.5.2 `next_reset_at` (recomputed for every user)

Let L = `--source-tz`, now = the import start time in L, E = `expired_at` in L, and M = `plan.reset_traffic_method`, or
the imported effective setting `reset_traffic_method` when the plan's value is NULL (§4.13).

`next_reset_at` = **null** when the user has no (imported) plan, M = 2, `expired_at` is NULL (lifetime: neither version
resets these users), or `expired_at` ≤ now. Otherwise:

| M | next_reset_at |
|---|---|
| 0 | `Date(now.Y, now.M+1, 1, 00:00:00, L)`. Go's normalisation avoids Carbon's `addMonth` overflow on the 31st |
| 1 | `d=E.Day`, `t=E.Clock`.<br>Candidate: `Date(now.Y, now.M, min(d, lastDay(now.Y,now.M)), t)`. If the candidate is after now, use it.<br>Otherwise `Date(Y', M', min(d, lastDay(Y',M')), t)` for the next month |
| 3 | `Date(now.Y+1, 1, 1, 00:00:00, L)` |
| 4 | `m=E.Month`, `d=E.Day`, `t=E.Clock`.<br>Candidate: `Date(now.Y, m, min(d, lastDay(now.Y,m)), t)`. If after now, use it. Otherwise the same with `now.Y+1` |

Store the result as a UTC instant. This is U's `TrafficResetService`, except that we clamp both month branches (U clamps
only the next-month one). The anchor is the **expiry** day. `DATA_MODEL.md` says "purchase day", which is the same day for
period purchases because expiry = purchase + N months; §8 asks for the wording to be aligned.

### 4.6 invite_codes ← `v2_invite_code`

`id`, `code`, `pv`, `created_at`, `updated_at` map directly; `user_id` must be an imported user (else skip, W);
`status` = `flexBool` (true means consumed).

`code` has no unique constraint in Xboard (8 random characters). On a duplicate, keep the lowest id and skip the rest, W.
Duplicates are compared case-insensitively, because a MySQL target's unique index uses a case-insensitive collation.

### 4.7 commission_logs ← `v2_commission_log` (all rows; these are not orders)

`id`, `invite_user_id`, `user_id`, `trade_no` (char(36)), `order_amount`, `get_amount` (cents), `created_at` = `ts()`.

A row whose `invite_user_id` is not an imported user is still imported, with W. The log is historical.

### 4.8 coupons ← `v2_coupon`

| ours | source | conversion |
|---|---|---|
| id, name | same | |
| code | code (string(255), **not unique** in Xboard) | longer than 64 characters, or a duplicate (keep the lowest id; compared case-insensitively, see §4.6) → skip, W |
| type, value | type (1 fixed cents, 2 percent), value | verbatim; a type other than 1 or 2 → skip, W |
| show | show | `flexBool` |
| limit_use | limit_use | verbatim. In Xboard this is the **remaining** count, decremented on every use. We count usage from orders, which start empty, so the remaining count is correct |
| limit_use_with_user | limit_use_with_user | verbatim. Per-user history is lost with the orders, W once if any coupon sets it |
| limit_plan_ids | limit_plan_ids (JSON) | `parseIDList`; drop plans that were not imported |
| limit_periods | limit_period (JSON) | `parseStringList`. Map legacy names (`month_price`→`monthly` … `reset_price`→`reset_traffic`) and keep only our 8 keys |
| started_at, ended_at, created_at, updated_at | int | `ts()` |

### 4.9 payments ← `v2_payment`

**Common columns**:

| ours | source | conversion |
|---|---|---|
| id | id | keep |
| uuid | uuid (8 random mixed-case chars, from `randomChar(8)`) | verbatim. Keeping it keeps the notify path suffix. Empty or a case-insensitive duplicate → new random uuid, W |
| gateway | payment (class name) | table below |
| name | name | verbatim |
| icon | icon | NULL → `""` |
| config | config (JSON text) | per-gateway key map below. Secrets are never logged |
| notify_domain | notify_domain | NULL → `""` |
| handling_fee_fixed | handling_fee_fixed (cents) | NULL → 0 |
| handling_fee_percent | handling_fee_percent (decimal(5,2), a percentage) | NULL → 0 |
| enabled | enable | `flexBool`, AND the rules below |
| sort | sort | NULL → 0 |

U only: if `v2_plugins` has a row for the gateway's plugin `code` (`epay`, `alipay_f2f`, `btcpay`, `coinbase`,
`coin_payments`, `mgate`) with `is_enabled=0`, set `enabled=false`, W.

**Gateway mapping**:

| Xboard `payment` | ours | config key map | after import |
|---|---|---|---|
| `EPay` | `epay` | `url` (rtrim `/`), `pid`, `key`, `type` (a value not in `alipay/wxpay/qqpay/""` is kept, W) | works. The notify URL is sent with each order |
| `EpusdtPay` | `epusdt` | `epusdt_pay_url`→`api_url`, `epusdt_pay_apitoken`→`api_token`, `currency_types`→`currency_type` (empty → `usdt.trc20`) | works |
| `AlipayF2F` | `alipay_f2f` | `app_id`, `private_key`, `public_key`→`alipay_public_key`, `sandbox`=`"0"` (a `0`/`1` select, PAYMENTS_GATEWAYS.md §4). `product_name` dropped, W | works |
| `StripeALLInOne` (also old `StripeAlipay`, `StripeWepay`, `StripeCredit`, `StripeCheckout`) | `stripe` | `stripe_sk_live`→`secret_key`, `stripe_webhook_key`→`webhook_secret`, `currency`→`currency` (**lower-cased**, ISO 4217 as Stripe expects). `payment_method`→`payment_methods`: `cards`/`card`→`card`, `alipay`, `wechat_pay`, else `auto`. For the old classes the class name implies it: Alipay → alipay, Wepay → wechat_pay, Credit/Checkout → card. `description` dropped. No `publishable_key`: hosted Checkout does not use it (PAYMENTS_GATEWAYS.md §0.2) | **`enabled=false`**, W: point the webhook at the new URL and subscribe it to `checkout.session.completed` and `checkout.session.async_payment_succeeded`. Xboard converted CNY amounts to `currency` at a live exchange rate. We charge the order amount in `settings.currency`, so W when the two differ: set `exchange_rate` |
| `Coinbase` | `coinbase` | none: Coinbase Commerce closed on 2026-03-31 and its API key / webhook key do not work with Coinbase Business (PAYMENTS_GATEWAYS.md §0.1). Config `{}` | **`enabled=false`**, W: create a CDP API key and a webhook, fill `api_key_name`, `api_key_secret`, `webhook_secret`, then enable |
| `BTCPay` | `btcpay` | `btcpay_url`→`server_url` (rtrim `/`), `btcpay_storeId`→`store_id`, `btcpay_api_key`→`api_key`, `btcpay_webhook_key`→`webhook_secret` | enabled as in source; W: update the webhook URL |
| anything else (`CoinPayments`, `MGate`, third-party plugins) | `lower(name)` | verbatim | **`enabled=false`**, W: "gateway not available in ggboard; kept for reference" |

**Notify URL**: Xboard uses `/api/v1/guest/payment/notify/<ClassName>/<uuid>` and ggboard uses `…/notify/<gateway>/<uuid>`.
The payment is looked up by uuid (kept), and the `:gateway` segment also accepts the lower-cased Xboard class names
(`epay`, `epusdtpay`, `alipayf2f`, `stripeallinone`, `stripealipay`, `stripewepay`, `stripecredit`, `stripecheckout`,
`coinbase`, `btcpay`; `ordersvc.GatewayMatches`), so URLs already registered in a gateway dashboard keep working. The
warnings and the checklist still print the canonical URL, built from the payment's `notify_domain`, else `app_url`. The
user's live install has 3 EPay rows, all of which import as enabled with no action needed.

### 4.10 tickets ← `v2_ticket`, ticket_messages ← `v2_ticket_message`

- **tickets**: `id`, `subject` map directly; `user_id` must be an imported user (else skip the ticket and its messages, W).
  `level` = `flexInt` 0/1/2 (withdraw tickets use 2); `status` 0 open, 1 closed; timestamps `ts()`.
- **`reply_status`, derived** (ours: 0 = awaiting staff, 1 = replied):
  1. If the ticket has messages, take the newest (max id): its `user_id` = ticket `user_id` → 0, else 1. Set
     `last_reply_user_id` to that `user_id`.
  2. No messages: U with `last_reply_user_id` → stored value verbatim. Otherwise **invert** it (`1 − v`): L's
     `TicketService` writes 0 after a staff reply and 1 after a user reply, contrary to the column comment, and U's
     2026-04-21 migration swapped the values.
- **ticket_messages**: `id`, `ticket_id`, `user_id`, `message` map directly. `created_at` = `ts()`. Messages whose ticket
  was skipped are skipped too.

### 4.11 notices ← `v2_notice`

`id`, `title`, `content` directly; `img_url` NULL → `""`; `tags` = `parseStringList`; `show` = `flexBool`. `sort`: U `sort`
(NULL → 0). L has no column and orders notices by `created_at DESC`, so assign `sort` = rank by `created_at DESC, id DESC`
(0, 1, …) to keep that order.

### 4.12 knowledge ← `v2_knowledge` (L = U)

`id`, `language` (char(5), trimmed), `category`, `title`, `body` directly; `sort` NULL → 0; `show` = `flexBool`;
timestamps `ts()`. Bodies may contain Xboard's `<!--access start-->…<!--access end-->` blocks, which Xboard shows only to users with a valid
plan. ggboard honours them the same way (`content.RenderBody` replaces each block with a `knowledge-no-access` notice for users
without a valid plan). The importer still counts those articles and emits W, with a stale message (see §8).

### 4.13 settings ← `v2_settings` (`name`, `value`)

U's "optimised" table has the same columns; only `value` became `mediumtext`.

- **Reading**: lower-case `name` (U lower-cases on save); on duplicate names the highest `id` wins. `value` is text: arrays
  JSON-encoded, scalars raw, booleans `"1"`, `"0"`, `""` or `"true"`/`"false"` (U).
- **Writing**: coerce to the target type (`flexBool`, `flexInt`, `parseStringList`) and store JSON-encoded. For **absent**
  keys, write a value only when Xboard's effective default differs from ours (marked "absent →" below).
- Unlisted keys (incl. `theme_*`, plugin configs) are reported as `ignored settings: k1, k2, …`. Xboard also falls back to
  `config/v2board.php`, which the importer cannot read; the operator must check the summary.

| ours | Xboard source | rule |
|---|---|---|
| `app_name` `app_description` `app_url` `logo` `tos_url` `currency` `currency_symbol` | same names | verbatim strings |
| `stop_register` `email_verify` `email_whitelist_enable` `invite_force` `register_limit_by_ip_enable` `password_limit_enable` | same | bool |
| `register_limit_count` `register_limit_expire` `password_limit_count` `password_limit_expire` `try_out_plan_id` | same | int. `try_out_plan_id` must be an imported plan, else 0, W |
| `try_out_hour` | same | int. Absent → 1 |
| `email_whitelist_suffix` | same (JSON array or comma string) | `parseStringList`. Absent and whitelist enabled → Xboard's list: `gmail.com qq.com 163.com yahoo.com sina.com 126.com outlook.com yeah.net foxmail.com` |
| `captcha_provider` `captcha_site_key` `captcha_secret` | L: `recaptcha_enable`, `recaptcha_site_key`, `recaptcha_key`. U: `captcha_enable`, `captcha_type`, and `recaptcha_*`, `turnstile_site_key`, `turnstile_secret_key` | Not enabled → `none`. `recaptcha` → `recaptcha` + its keys. `turnstile` → `turnstile` + its keys. `recaptcha-v3` → `none`, W |
| `subscribe_url` | same (**comma-separated string**) | split on `,` and trim. Drop entries with Xboard patterns `*`, `[a-b]`, `[uuid]`, `{uuid}`, W (unsupported) |
| `plan_change_enable` `surplus_enable` `show_protocol_to_server_enable` `default_remind_expire` `default_remind_traffic` | same | bool |
| `show_info_to_server_enable` | same | bool. **Absent → false** (Xboard default 0) |
| `reset_traffic_method` | same | int 0..4. **Absent → L 0, U 1.** U's `TrafficResetService` falls back to `RESET_TRAFFIC_MONTHLY`, even though its admin form shows 0. W |
| `invite_commission` `invite_gen_limit` `commission_withdraw_limit` `commission_distribution_l1/_l2/_l3` | same | int (round, W if the value had a fraction). NULL → omit |
| `invite_never_expire` | same | bool. **Absent → false** |
| `commission_first_time_enable` `commission_auto_check_enable` `withdraw_close_enable` `commission_distribution_enable` | same | bool |
| `commission_withdraw_method` | same (JSON array) | `parseStringList`. **Absent → `["支付宝","USDT","Paypal"]`** (Xboard's Dict) |
| `server_token` | same | verbatim, replacing our auto-generated token. Empty → keep ours, W ("reconfigure every backend") |
| `server_pull_interval` `server_push_interval` `device_limit_mode` (U) | same | int |
| `email_host` `email_port` `email_username` `email_password` `email_from_address` | same | strings; port → int |
| `email_driver` | (derived) | `smtp` if `email_host` is non-empty, else leave as is |
| `email_encryption` | same | `ssl` / `tls` verbatim; NULL or `""` → `none` |
| `remind_mail_enable` | U same; L — (L always sends) | U: bool, **absent → false**. L: leave ours (true) |
| `frontend_background_url` | same | verbatim |
| `windows_*`, `macos_*`, `android_*` (`_version`, `_download_url`) | same | verbatim |

Explicitly dropped (listed in the summary): `force_https`, `secure_path`, `frontend_admin_path`, `safe_mode_enable`,
`email_gmail_limit_enable`, `frontend_theme*`, `email_template` (template sets differ), `telegram_*`, `new_order_event_id`,
`renew_order_event_id`, `change_order_event_id`, `try_out_enable`, `server_ws_enable`, `server_ws_url`,
`ticket_must_wait_reply`, `recaptcha_v3_*`, `subscribe_template_*`, `subscribe_path` (W if ≠ `"s"`: our short link is
fixed at `/s/:token`).

### 4.14 stats (only with `--with-stats`; window `--stats-days`, default 90)

- **Row filter**: `record_type='d'` and `record_at ≥ (midnight(now in L) − N days)`.
- **`record_at` conversion**: `record_at` is a unix **local midnight**. Convert it to a date in L. For example,
  `1729440000` → `2024-10-21`; using UTC would give the 20th.

| ours | source | conversion |
|---|---|---|
| stats | `v2_stat` | the count/total columns map one to one. `transfer_used_total` (varchar) → int64. The record_at unique index is missing on sqlite: on duplicates, the highest id wins |
| stat_nodes | `v2_stat_server` | `node_id`:<br>L: `nodeMap[(normType(server_type), server_id)]`.<br>U: the `v2_server` row with that `id` and type, else the one with that `code` and type.<br>No match → drop (counted). Then sum u/d per (node_id, date) |
| stat_users | `v2_stat_user` | `node_id = 0`; Xboard records no server. (A fork whose `v2_stat_user` has `server_id` [+ `server_type`] maps it like stat_nodes; unmapped → 0.) Aggregate per (user_id, node_id, date). Drop users that were not imported |

**stat_users aggregation**. We store **charged** bytes, as `users.u/d` do (*assumption, §8*). L rows hold raw bytes per
`server_rate` (charged = `u·rate`); U rows are already charged (its job multiplies by the rate). `rate` = `Σcharged / Σraw`
rounded to 2 decimals; for U, raw = `charged/rate`, skipping rate 0; nothing derivable → rate 1.

### 4.15 Skipped tables

| table(s) | reason |
|---|---|
| `v2_order` | requirement: no historical orders |
| `v2_log`, `v2_admin_audit_log`, `v2_mail_log`, `v2_server_log`, `v2_traffic_reset_logs`, `failed_jobs` | operational logs with no value after cutover |
| `v2_tutorial` | from old v2board (`migrate:fromV2b`); replaced by knowledge |
| `v2_plugins` | PHP plugins; only read for payment enable state |
| `v2_gift_card_template`, `v2_gift_card_code`, `v2_gift_card_usage` | no gift-card feature in ggboard |
| `v2_subscribe_templates`, `v2_mail_templates` | our subscription and mail templates differ |
| `v2_server_machine`, `v2_server_machine_load_history` | no machine agent in ggboard |
| `migrations`, `personal_access_tokens`, `v2_server_stat` | framework or legacy internals |

## 5. Validation and edge cases

| case | handling |
|---|---|
| `plan_id` → deleted plan | `plan_id` = NULL, W. `transfer_enable`/`expired_at` kept; `next_reset_at` null (Xboard's reset job also skips them) |
| `group_id` NULL | kept NULL; the user sees no nodes, as in Xboard |
| `expired_at` in the past, or 0 | kept as expired (§4.5); `next_reset_at` null |
| emails differing only by case | only possible on pg/sqlite sources; MySQL's ci unique index prevents it. The winner is the account with the latest `max(last_login_at, t, updated_at)` (tie: lowest id) and keeps the lower-cased email. The others get `<local>+dup<id>@<domain>`, W with both ids so the admin can merge |
| empty or invalid email (no `@`) | imported lower-cased, W |
| duplicate `uuid` / `token` | §4.5 |
| hysteria `version` NULL | 1, W |
| node `parent_id` | two-pass insert; missing parent → NULL; child keys use the parent's `created_at` (§4.4) |
| `capacity_limit` | verbatim (0 = sold out); W when the number of active users already exceeds the limit |
| commission state on orders | not needed. Pending commissions are lost unless settled in Xboard first (§7) |
| `next_reset_at` | always recomputed (§4.5.2), never copied |
| `u+d` overflow | none possible: bigint → int64 on both sides |
| invalid JSON in any JSON column | the parser's fallback applies; otherwise empty value + W, never a fatal error |
| unknown `server_type` in stats | `v2ray→vmess`, `hysteria2→hysteria`; others dropped (counted) |
| value longer than the ggboard column | Xboard `string()` is varchar(255); some of our columns are shorter (names 191, port 32). PostgreSQL and strict MySQL reject over-long values, so names are truncated and URLs (`icon`, `img_url`, `notify_domain`) dropped, W |

## 6. CLI

```bash
ggboard import xboard --dsn "mysql://xboard:secret@tcp(127.0.0.1:3306)/xboard?parseTime=true" \
  [--dry-run] [--with-stats] [--stats-days 90] [--force] [--source-tz Asia/Shanghai]
```

- The target DB comes from the normal ggboard config (`database.driver` / `database.dsn`).
- There is no partial-import flag (such as `--skip-users`): partial imports would break references.
- Exit codes: 0 success (warnings allowed; dry run too), 1 fatal (nothing written) or usage error (the foundation's
  `cli.Run` maps every command error to 1; usage errors wrap `xboardimport.ErrUsage`). Warnings go to stdout and are
  mirrored to slog at WARN. Secrets are never printed.
- `--driver mysql|postgres|sqlite` lets `--dsn` be a raw driver DSN without a scheme. The MySQL URL form
  `mysql://user:pass@host:3306/db` is accepted besides the go-sql-driver form.
- The summary ends with the post-import checklist of §7, filled with this run's specifics (renumbered nodes, `code`
  nodes, webhook URLs, disabled payments, unusable hashes, access-block articles, a missing `app_url`).

Summary format:

```text
Xboard import — source mysql 127.0.0.1/xboard · flavor legacy · migrations 33 (last 2024_03_19_103149_modify_icon_column_to_v2_payment_table)
mode: DRY-RUN (rolled back) · source-tz Asia/Shanghai · stats: off
table             source  imported  skipped
server_groups          3         3        0
plans                  6         6        0
node_routes            2         2        0
nodes                 14        14        0   shadowsocks 14 · renumbered 0
users               1532      1532        0   md5/sha256 legacy hashes 41 · unusable 0
invite_codes         210       210        0
commission_logs      388       388        0
coupons               12        11        1
payments               3         3        0   epay 3 (enabled 3)
tickets / messages  97/402    97/402      0
notices / knowledge  5/18      5/18       0
settings              71        48       23   ignored: frontend_theme, telegram_bot_enable, …
warnings (4):
  [coupons#7] duplicate code "WELCOME" (kept #3) — skipped
  [users#88] invite_user_id 77 not found → NULL
  [settings] subscribe_url entry "https://*.example.com" uses a wildcard — dropped
  [nodes#5] excludes not supported — dropped
next: docs/IMPORT_XBOARD.md §7 post-import checklist
```

## 7. Operator checklist

### Before the import

1. Put Xboard in maintenance or stop selling. Wait for pending payments: pending orders are **not** migrated.
2. Settle commissions: run `php artisan check:commission` in Xboard. Unsettled order commissions do not carry over.
3. Run with `--dry-run`, read every warning, then run for real.

### After the import

1. **Backends.** `server_token` was copied 1:1, so XrayR `ApiKey`, V2bX and soga keys stay valid. Point `ApiHost`/
   `webapi_url` at ggboard (or move the old domain's DNS). Apply every node-renumber (`NodeID`) and `code` warning.
2. **Payments.** EPay, Epusdt and AlipayF2F work as-is. Stripe and BTCPay: the old webhook URL keeps working (class-name
   alias, §4.9), but switching it to `https://<host>/api/v1/guest/payment/notify/<stripe|btcpay>/<uuid>` is cleaner
   (Stripe: subscribe `checkout.session.completed` and `checkout.session.async_payment_succeeded`); then enable Stripe.
   Coinbase: create Coinbase Business (CDP) credentials and a webhook to `…/notify/coinbase/<uuid>`, then enable. Review
   and delete the disabled unknown gateways.
3. **Subscriptions.** `curl -sI "https://<host>/api/v1/client/subscribe?token=<a real token>"` → 200 with
   `subscription-userinfo` (same path as L; U's short link `/s/<token>` works if `subscribe_path` was `s`). Check that
   `subscribe_url` lists the domains users already have.
4. **Logins.** Log in as one bcrypt and one md5/sha256 user (if any); the hash upgrades to argon2id. Users with unusable
   hashes must use "forgot password".
5. **Settings.** Send a test mail (`/admin/settings/mail/test`); re-check captcha keys; re-set dropped items you need
   (theme, Telegram). Xboard's `secure_path` no longer applies: restrict `/admin` at the proxy if you relied on it.
6. **Content.** Knowledge articles that use access blocks (W) work as in Xboard; the warning is informational (§4.12).
7. **Keep the Xboard DB** as a read-only backup. The importer never modifies it.

## 8. Contract follow-ups for other owners

Status as of 2026-10-22: 1, 2 and 5 are implemented; 3, 6 (scheduler zone) and 7 are open; 4 is accepted.

1. **auth** (done, `internal/auth/password.go`): verify `xb$<algo>$<salt>$<hash>` (§4.5.1) and upgrade to argon2id on success.
2. **payment router** (done, `ordersvc.GatewayMatches`): map Xboard class names to gateway ids in `:gateway` (`EPay`,
   `EpusdtPay`, `AlipayF2F`, `StripeALLInOne` and the old Stripe classes, `Coinbase`, `BTCPay`), because lookup is by uuid.
   Statically registered webhooks keep working without changes.
3. **order/invite**: with `commission_first_time_enable`, an invitee who already paid in Xboard looks like a first-time
   buyer. Proposal: add `users.first_paid_at *time`. The importer would fill it from `MIN(paid_at)` over `v2_order` rows with
   status 3 or 4. That reads orders but does not import them. Until then, document the risk.
4. **upgrade surplus**: upgrade credit is computed from previous orders, so imported users get none on their first upgrade.
   Accepted limitation.
5. **knowledge** (done, `internal/service/content`): replace `<!--access start-->…<!--access end-->` for users without a
   valid plan. The importer's W text ("ggboard shows to every user") and the checklist line still predate this.
6. **DATA_MODEL wording**: reset methods 1 and 4 anchor on the **expiry** day (§4.5.2). `stat_users.u/d` = charged bytes and
   `node_id = 0` = "unknown node (imported)". The traffic-log API must left-join nodes. The scheduler should run
   reset jobs in the same zone as `--source-tz`; there is no timezone setting yet.
7. **nodes**: if renumbering ever becomes unacceptable, add `nodes.code` (unique per type) and resolve `node_id` against
   `code` before `id`, as U does.

## 9. Test plan

**Fixtures.** Commit SQL, not binary `.sqlite` files, in `internal/importer/xboard/testdata/`.

| file | contents |
|---|---|
| `legacy_schema.sql` | SQLite DDL transcribed from the 33 L migrations. Booleans are `INTEGER`, `bigInteger()` columns `BIGINT` (SQLite does not care; PostgreSQL rejects 100 GiB in an `integer` when the suite loads the seed there). **No** `v2_stat_user` / `v2_stat` unique indexes on sqlite, matching the migrations |
| `upstream_schema.sql` | the result of running all U migrations: `v2_server`, `prices` JSON, `next_reset_at`, gift-card tables, `v2_subscribe_templates`, `v2_plugins`, `v2_server_machine`, `surplus_credit`, `last_reply_user_id` |
| `legacy_seed.sql`, `upstream_seed.sql` | fake data on `example.com` |

To check the transcription manually, run `php artisan migrate --database=sqlite` in both reference checkouts and diff
`.schema` (there is no make target).

The helper `newSource(t, flavor, extraSQL...) string` builds a temp sqlite file from schema + seed and returns
`sqlite://…`. The target is `testutil.NewTestApp(t)` (a private sqlite file). The build-tagged `integration` suite
(`integration_test.go`) runs against servers given by `GGBOARD_IT_POSTGRES_DSN` / `GGBOARD_IT_MYSQL_DSN` (each test
creates and drops its own database; the file header has the `docker run` lines for PostgreSQL 16 and MySQL 8.4):

- sources: the same seed loaded into PostgreSQL / MySQL with driver-adjusted DDL (real `boolean` and `timestamp`
  columns on PostgreSQL, `tinyint(1)` on MySQL) must produce a target dump identical to the SQLite-source import;
- targets: the SQLite fixtures imported into PostgreSQL / MySQL targets (dry run, real run, next id = max+1, `--force`).

**Seed must include**:

- L nodes: shadowsocks `2022-blake3-aes-128-gcm` with `created_at=1700000000`, its child (`parent_id`), one with `obfs=http`;
  vmess with **id 1** (collides with ss #1) plus a child of it, `tlsSettings` in camelCase; vless reality (tls=2); trojan
  with `networkSettings`; hysteria v1 (alpn 3, is_obfs 1) and one with `version` 0 (the column is `NOT NULL DEFAULT 1`;
  the robustness test drops the column to cover old forks); `group_id` values `["1","2"]`, `[1]`, `{"0":"1","2":"3"}`,
  `1,2`, `"1,2"`, `""` (L `group_id` is NOT NULL; the NULL case is U's `group_ids`); routes whose `match` is a JSON
  object / newline-separated. L plan with `month_price=1000, onetime_price=0`.
- U: `v2_server` rows of every type incl. socks; `code` ≠ id; `enabled=0`; `prices` `{"monthly":9.99,"yearly":"99.9",
  "onetime":0}`; a `v2_plugins` row `epay` with `is_enabled=0`.
- Users: bcrypt `$2y$`, md5, sha256, md5salt (salt `abc$12`), sha256salt (U), unknown algo with a non-bcrypt hash;
  `expired_at` NULL / 0 / future / past; deleted plan; invite → missing user; self-invite; duplicate token; pg-only
  emails `A@example.com` + `a@example.com`.
- Tickets whose last message is from staff / from the user / no messages. Settings: booleans `"1"`/`""`/`"true"`;
  `subscribe_url` `"https://a.example.com,https://*.example.com"`; `reset_traffic_method` absent. Stats: `record_at`
  1729440000; L stat_user rows with rates 1 and 0.5 for the same user and day.

**Assertions**:

1. Detection: legacy, upstream, both (fatal), neither (fatal).
2. Per-table counts equal the summary. Unchanged-id tables keep ids. `next insert id = max+1` on sqlite, mysql and pg (pg
   checks the sequence reset).
3. Nodes: vmess #1 is renumbered past the max legacy id and its child's `parent_id` and stat_nodes rows follow; U `code`
   W emitted; socks skipped; `enabled=0` → `show=false`.
4. SS2022 `server_key = MjQ5MjBkZWNmODNmNjk2MA==`. The child uses the parent's key. Hysteria `obfs_password` = K16 of
   its own (or its parent's) `created_at`.
5. Every `group_id` variant gives `[1,2]`, `[1]`, `[1,3]`, `[1,2]`, `[]`.
6. Prices: L `{monthly:1000, onetime:0}` + W; U `9.99→999`, `"99.9"→9990`, `onetime` absent.
7. Passwords: each fixture verifies `password123` through the auth package, and fails for `wrong`. The unknown algo gives
   `!xb-unusable`.
8. `expired_at`: NULL → nil; 0 → `created_at`. `next_reset_at` with clock `2026-10-21T12:00+08:00`: M=1,
   E=`2027-01-31 10:00 +08` → `2026-10-31T02:00Z`; same E with clock `2026-11-15` → `2026-11-30 10:00 +08`; M=0 →
   `2026-11-01T00:00+08`; M=3 → `2027-01-01T00:00+08`; M=2, NULL expiry or past expiry → nil.
9. Tickets: `reply_status` derived from the last message; the no-message L ticket is inverted.
10. Settings: the type coercions; absent `show_info_to_server_enable` → false; absent `reset_traffic_method` → 0 (L) / 1 (U);
    wildcard `subscribe_url` dropped with W; `server_token` copied.
11. Payments: each gateway's key map. Stripe is disabled. Unknown gateways are disabled. The plugin-disabled epay is
    disabled.
12. Stats: `1729440000 → 2024-10-21`. The L stat_user pair aggregates to `u = u1 + 0.5·u2`, with rate = Σcharged/Σraw.
13. Idempotency: a second run fails with exit 1 and no writes; `--force` gives a byte-identical dump; `--dry-run` leaves
    every table empty and prints the same counts as the real run.
14. Robustness: a malformed JSON column produces W, not a crash. The source DB is unchanged (checksum the sqlite file
    before and after).
