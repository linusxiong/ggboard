# Data model (GORM, `internal/model`)

Conventions: table names are plain plural snake_case (no `v2_` prefix). `id` is `uint64` autoincrement.
`created_at`/`updated_at` are `time.Time` (GORM auto). Money = integer **cents** (`int64`). Traffic = **bytes** (`int64`).
JSON columns use `datatypes.JSON`-like typed wrappers (`model.JSONSlice[T]`, `model.JSONMap`) that implement
`driver.Valuer`/`sql.Scanner` so they work on SQLite, PostgreSQL and MySQL.
Nullable scalars are pointers. Booleans are real `bool` columns.

## users
| column | type | notes |
|---|---|---|
| id | uint64 PK | |
| email | varchar(191) unique | lower-cased |
| password_hash | varchar(255) | argon2id (new) or bcrypt `$2y$` (imported) |
| uuid | char(36) unique | per-user credential for vmess/vless/tuic; also derives other passwords |
| token | char(32) unique | subscription token |
| plan_id | *uint64 idx | |
| group_id | *uint64 idx | server group the user can see |
| transfer_enable | int64 | bytes |
| u, d | int64 | bytes used (upload/download) |
| expired_at | *time idx | null = never expires |
| next_reset_at | *time idx | next traffic reset, computed from plan reset method |
| speed_limit | *int | Mbps, null = unlimited |
| device_limit | *int | null = unlimited |
| balance | int64 | cents |
| commission_balance | int64 | cents |
| commission_type | int8 | 0 system default, 1 period, 2 one-time |
| commission_rate | *int | percent override |
| discount | *int | percent |
| invite_user_id | *uint64 idx | |
| is_admin, is_staff, banned | bool | |
| remind_expire, remind_traffic | bool | default true |
| last_login_at | *time | |
| last_login_ip | varchar(45) | |
| telegram_id | *int64 | kept for import only |
| remarks | text | admin notes |
| password_version | int | bumped on password change / reset-security; part of JWT |

Derived credentials (shared helper `model.UserCredentials(u, node)`):
- vmess / vless / tuic: `uuid`.
- trojan / hysteria / anytls / shadowsocksr / plain-shadowsocks: password = `uuid`.
- shadowsocks 2022: user key = base64(uuid[:16]) for `2022-blake3-aes-128-gcm`, base64(uuid[:32]) otherwise
  (identical to Xboard `Helper::uuidToBase64`). Node key = base64(md5(created_at_unix)[:16|32]) (Xboard `getServerKey`) unless `protocol_settings.server_key` set.
- mieru: username = `uuid`, password = `uuid`.

## plans
| column | type | notes |
|---|---|---|
| id | uint64 PK | |
| group_id | uint64 idx | server group granted on purchase |
| name | varchar(191) | |
| content | text | markdown description |
| tags | json []string | |
| transfer_enable | int64 | **GB** (as entered); converted to bytes on purchase |
| speed_limit | *int | Mbps |
| device_limit | *int | |
| capacity_limit | *int | max concurrent subscribers (null = unlimited) |
| prices | json map[string]int64 | keys: `monthly quarterly half_yearly yearly two_yearly three_yearly onetime reset_traffic`; absent = not for sale |
| reset_traffic_method | *int8 | null = follow setting `reset_traffic_method`; 0 monthly on 1st, 1 monthly on purchase day, 2 never, 3 yearly on Jan 1, 4 yearly on purchase day |
| show | bool | visible in shop |
| sell | bool | purchasable (false = legacy plan: renew only) |
| renew | bool | existing users may renew |
| sort | int | |

## server_groups
`id, name, created_at, updated_at`.

## nodes
| column | type | notes |
|---|---|---|
| id | uint64 PK | |
| type | varchar(16) idx | `vmess vless shadowsocks shadowsocksr trojan hysteria anytls mieru tuic` |
| name | varchar(191) | |
| group_ids | json []uint64 | groups allowed to use the node |
| route_ids | json []uint64 | node_routes applied (XrayR routes / soga audit) |
| parent_id | *uint64 | child nodes (relay) inherit parent's config; traffic counted on parent |
| host | varchar(255) | address shown to clients |
| port | varchar(32) | client port; may be a range `10000-20000` (hysteria port hopping) |
| server_port | int | port the backend listens on |
| tags | json []string | |
| rate | float64 | traffic multiplier |
| show | bool | |
| sort | int | |
| protocol_settings | json | per type, below |

`protocol_settings` by `type` (all keys optional unless marked *):
```jsonc
// vmess
{"network":"tcp|ws|grpc|http|h2|httpupgrade|xhttp","network_settings":{...},"tls":0,"tls_settings":{"server_name":"","allow_insecure":false}}
// vless
{"network":"tcp","network_settings":{...},"tls":0|1|2,"flow":"|xtls-rprx-vision","encryption":"none",
 "tls_settings":{"server_name":"","allow_insecure":false,"public_key":"","private_key":"","short_id":"","dest":"example.com:443","server_port":"443","fingerprint":"chrome"}}
// shadowsocks
{"cipher":"aes-128-gcm|aes-256-gcm|chacha20-ietf-poly1305|2022-blake3-aes-128-gcm|2022-blake3-aes-256-gcm|2022-blake3-chacha20-poly1305",
 "server_key":"","obfs":"plain|http|tls","obfs_settings":{"path":"/","host":""},"plugin":"","plugin_opts":""}
// shadowsocksr
{"cipher":"chacha20-ietf","protocol":"auth_chain_a","protocol_param":"","obfs":"http_simple","obfs_param":"","single_port_type":"protocol|obfs"}
// trojan
{"network":"tcp|ws|grpc","network_settings":{...},"server_name":"","allow_insecure":false}
// hysteria
{"version":2,"up_mbps":100,"down_mbps":100,"obfs":"plain|salamander","obfs_password":"","server_name":"","allow_insecure":false,"alpn":"h3","hop_interval":30}
// anytls
{"tls":1|2,"server_name":"","allow_insecure":false,"padding_scheme":[],"tls_settings":{"public_key":"","private_key":"","short_id":"","dest":"","server_names":[]}}
// mieru
{"transport":"TCP","multiplexing":"MULTIPLEXING_LOW|MULTIPLEXING_MIDDLE|MULTIPLEXING_HIGH"}
// tuic
{"congestion_control":"bbr","zero_rtt_handshake":false,"udp_relay_mode":"native","server_name":"","allow_insecure":false,"alpn":"h3"}
```
`network_settings` (xray style): ws `{"path":"/","headers":{"Host":"..."}}`; grpc `{"serviceName":"..."}`;
http/h2 `{"path":"/","host":["..."]}`; httpupgrade `{"path":"/","host":"..."}`; xhttp `{"path":"/","host":"","mode":"auto","extra":{}}`;
tcp `{"header":{"type":"none|http"}}`.

Runtime (not persisted): `NodeStatus{LastCheckAt, LastPushAt, Online int, Load{cpu,mem,swap,disk,uptime}, Version}` in `app.Online`.

## node_routes
`id, remarks varchar, match json []string, action varchar(16) (block|dns), action_value varchar, created_at, updated_at`.
Delivered to XrayR as `routes[]`; `block` routes become soga `audit_rules`.

## orders
| column | type | notes |
|---|---|---|
| id | uint64 PK | |
| trade_no | char(32) unique | `YYYYMMDDHHMMSS` + random |
| user_id | uint64 idx | |
| plan_id | uint64 | |
| invite_user_id | *uint64 | |
| coupon_id | *uint64 | |
| payment_id | *uint64 | |
| type | int8 | 1 new, 2 renew, 3 upgrade, 4 reset_traffic |
| period | varchar(16) | price key |
| callback_no | varchar(191) | gateway txn id |
| total_amount | int64 | final payable |
| handling_amount | int64 | gateway fee |
| discount_amount | int64 | coupon + user discount |
| surplus_amount | int64 | credit from previous plan on upgrade |
| refund_amount | int64 | returned to balance when surplus exceeds price |
| balance_amount | int64 | paid from balance |
| surplus_order_ids | json []uint64 | |
| status | int8 idx | 0 pending, 1 processing, 2 cancelled, 3 completed, 4 discounted |
| commission_status | int8 | 0 pending, 1 processing, 2 valid, 3 invalid |
| commission_balance | int64 | commission amount |
| paid_at | *time | |

## payments
`id, uuid char(32) unique, gateway varchar(32), name, icon, config json (secret), notify_domain, handling_fee_fixed int64, handling_fee_percent float64, enabled bool, sort int, timestamps`.

## coupons
`id, code varchar(64) unique, name, type int8 (1 fixed amount, 2 percent), value int64, show bool, limit_use *int, limit_use_with_user *int, limit_plan_ids json []uint64, limit_periods json []string, started_at time, ended_at time, timestamps`.
Usage counted from `orders` (status != cancelled).

## invite_codes
`id, user_id idx, code char(32) unique, status bool (consumed, only when invite_never_expire=false), pv int, timestamps`.

## commission_logs
`id, invite_user_id idx, user_id, trade_no, order_amount int64, get_amount int64, created_at`.

## tickets / ticket_messages
tickets: `id, user_id idx, subject, level int8 (0 low 1 medium 2 high), status int8 (0 open 1 closed), reply_status int8 (0 awaiting staff, 1 replied), last_reply_user_id *uint64, timestamps`.
ticket_messages: `id, ticket_id idx, user_id, message text, created_at`.

## notices
`id, title, content text (markdown), img_url, tags json []string, show bool, sort int, timestamps`.

## knowledge
`id, language char(5), category varchar, title varchar, body text (markdown), sort int, show bool, timestamps`.

## settings
`key varchar(64) PK, value text (JSON encoded), updated_at`. Defaults live in code (`settings.Defaults`). Keys and groups in `docs/API.md` §Admin settings.

## stat_users / stat_nodes / stats
- stat_users: `id, user_id idx, node_id idx, rate float64, u, d int64, record_at date idx` unique(user_id,node_id,record_at).
- stat_nodes: `id, node_id idx, u, d int64, record_at date` unique(node_id,record_at).
- stats: `id, record_at date unique, order_count, order_total, paid_count, paid_total, commission_count, commission_total, register_count, invite_count, transfer_used_total`.

## mail_logs
`id, email, subject, template, error text nullable, created_at`.

## rule_profiles
`user_id uint64 PK, data json (see docs/SUBSCRIPTION.md), updated_at`.

## Ephemeral (memory only)
- email verification codes (10 min TTL), login attempts, register-by-ip counters.
- online users per node, alive IPs per user (`map[userID]set[ip]` with TTL 2×push_interval), node status.
