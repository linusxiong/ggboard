# Subscription generation & rule sets

Packages (all under `internal/subscription`):

| package | role |
| --- | --- |
| `subscription` | `Format` registry, `DetectFormat`, `RenderContext`, `RuleProfile`, catalog types, `AvailableNodes`, `Proxy` (decoded node + credentials), info nodes, policy-group resolution |
| `subscription/rulesets` | embedded `catalog.json`, admin overrides (`rulesets_custom`), profile decode/validate/resolve, per-format rule plan, public catalog view |
| `subscription/formats` | one file per output format, registered from `init()` (blank-import the package) |
| `subscription/engine` | `Generate` (DB + settings → body) and the pure `Render`; the multi-profile store (`LoadProfiles`, `SaveProfileSlug`, `CreateProfile`, …, `EffectiveProfile`) |
| `subscription/guard` | leak protection: age key pairs and encryption, the access log (batched writer, prune job), leak detection |
| `subscription/fixture` | deterministic user + 25 nodes covering every type/transport (golden and API tests) |

Request flow (`engine.Generate`): token user → effective rule profile (`?profile=` slug) → info nodes
(`show_info_to_server_enable`) + `AvailableNodes` (only when `u.IsAvailable`) → query filters → profile node operations →
`emoji=0` / protocol prefix → format filter (`Supports` + optional `SupportsProxy`) → unique names → groups → rule plan →
`Format.Render` → `ETag = sha1(body)`. The client handler then applies age encryption (see below) and records the access.

**Which nodes**: `AvailableNodes` returns nodes with `show = true` whose `group_ids` contain the user's group (a user
without a group gets none), with a known type and decodable `protocol_settings`, ordered by `sort, id`. A child node is an
entry of its own (see below). Node status is **not** a filter: offline nodes stay in the list (the status is only carried
on the proxy). The per-group list is cached for 5 s (`subscription_nodes` namespace); status is evaluated per request.

## Client detection (`flag` query or User-Agent, case-insensitive, first match wins)

| flag/UA contains | format | node types rendered |
| --- | --- | --- |
| `clash.meta`, `mihomo`, `clashmeta`, `clash-verge`, `clash verge`, `verge`, `flclash`, `clash nyanpasu`, `clashx meta` | `clash-meta` | all 9 (mihomo YAML). Skipped: AnyTLS+REALITY (mihomo has none), unknown ss plugins |
| `stash` | `stash` | ss (no chacha20-2022), ssr, vmess, vless (incl. reality/xhttp), trojan, hysteria v1/v2, tuic. No anytls/mieru (Stash docs list them since 2026, kept off for older Stash builds) |
| `clash` (plain), `clashforandroid`, `clashx` | `clash` (classic core) | ss (no 2022 ciphers, no shadow-tls), ssr, vmess (tcp/ws/grpc/h2/tcp-http), trojan (tcp/ws/grpc) |
| `sing-box`, `singbox`, `sfa`, `sfi`, `sfm`, `nekobox`, `hiddify` | `singbox` (**sing-box ≥ 1.12** schema) | ss, vmess, vless, trojan, hysteria v1/v2, tuic, anytls (incl. reality). No ssr/mieru, no xhttp, no tcp-http-header |
| `surfboard` | `surfboard` | ss (AEAD only), vmess, trojan (tcp/ws) |
| `surge` | `surge` | ss (AEAD + 2022 aes), vmess, trojan (tcp/ws), hysteria2, tuic (`tuic-v5`), anytls (tls). **Surge has no VLESS** (manual 2026-10), no ssr/mieru/hysteria v1 |
| `quantumult x`, `quantumult%20x`, `quanx` | `quantumultx` | ss (AEAD, obfs/v2ray-plugin), ssr, vmess & vless (tcp/ws, incl. reality), trojan (tcp/ws), anytls (tls) |
| `shadowrocket` | `shadowrocket` | every type except mieru (share links) |
| `loon` | `loon` | ss (no chacha20-2022), ssr, vmess & vless (tcp/ws/tcp-http), trojan (tcp/ws), hysteria2, anytls (incl. reality). **Loon documents no TUIC** |
| `v2rayn`, `v2rayng`, `nekoray`, `passwall`, `ssrplus`, `sagernet`, `karing`, `v2box`, `shadowsocks` | `v2ray` | all 9 (base64 of share links) |
| default | `v2ray` | |

`flag` may also name a format directly: `clash`, `clash-meta`, `stash`, `singbox`, `surge`, `surfboard`, `quantumultx`, `shadowrocket`, `loon`,
`v2ray`, `ssr` (`ssr` = base64 of `ssr://` links: SSR nodes plus plain shadowsocks as protocol origin / obfs plain).

Every format implements (optional interfaces in brackets):

```go
type Format interface {
    Name() string
    Supports(nodeType string) bool
    Render(ctx *RenderContext) (body []byte, contentType string, err error)
}
// [ProxyFilter]  SupportsProxy(p *Proxy) bool   — drop configurations the client cannot express
// [RuleConsumer] RuleFamily() string            — catalog keys to use (docs/RULESETS.md)
// [AgeCapable]   SupportsAge() bool             — clients can decrypt age-encrypted bodies (only clash-meta)
type RenderContext struct{ User; Nodes; Proxies []Proxy; Groups []ResolvedGroup; Rules *RulePlan; Profile; Catalog;
    Settings; SubscribeURL; RequestHost; AppName; Locale; Mirror; UserAgent; ListOnly; UpdateHours; Now; Location }
```

Content types: `text/yaml; charset=utf-8` (clash family), `application/json; charset=utf-8` (singbox), `text/plain; charset=utf-8` (others).

Share-link conventions (v2ray/shadowrocket): vmess = base64 JSON `{v:"2",ps,add,port,id,aid,scy,net,type,host,path,tls,sni,fp}` (all values
strings, as Xboard; `type` = tcp header type, `gun` for grpc, xhttp mode for xhttp);
vless `vless://uuid@host:port?encryption=none&security=tls|reality|none&sni=&fp=&pbk=&sid=&type=tcp|ws|grpc|http|httpupgrade|xhttp&headerType=&host=&path=&serviceName=&mode=&extra=&flow=&allowInsecure=1#name`;
ss `ss://base64url(method:password)@host:port[/?plugin=obfs-local;obfs=http;obfs-host=…]#name` (password = `<server_key>:<user_key>` for the AES 2022 ciphers, the user key alone for `2022-blake3-chacha20-poly1305`, see
"Shadowsocks 2022 passwords" below);
ssr `ssr://base64url(host:port:protocol:method:obfs:base64url(pass)/?obfsparam=&protoparam=&remarks=[&group=])`;
trojan `trojan://pass@host:port?security=tls&sni=&type=&host=&path=&serviceName=&allowInsecure=1#name`;
hysteria2 `hysteria2://pass@host:port?sni=&insecure=1&obfs=salamander&obfs-password=&mport=10000-20000#name`;
hysteria (v1) `hysteria://host:port?protocol=udp&auth=&upmbps=&downmbps=&obfs=xplus&obfsParam=&peer=&insecure=1&alpn=#name`;
tuic `tuic://uuid:pass@host:port?sni=&alpn=h3&congestion_control=bbr&udp_relay_mode=native&allow_insecure=1#name`;
anytls `anytls://pass@host:port?sni=&insecure=1[&security=reality&pbk=&sid=&fp=]#name`; mieru `mieru://username:password@host:port?transport=TCP&multiplexing=MULTIPLEXING_LOW#name`.
IPv6 literals are bracketed. Params are emitted only when set (`insecure=1` only when certificate checks are off).

**SSR credentials** (soga/SSPanel single-port): password = node password (`protocol_settings.password`, else
`base64(md5hex(P.created_at)[:16])`, the same value soga receives), protocol param = `"<user id>:<uuid>"`, obfs param = `obfs_param`.

**Shadowsocks 2022 passwords** (every format, `model.SS2022Password`; keys per docs/NODE_PROTOCOLS.md §4):
`<server_key>:<user_key>` (multi-user EIH) for `2022-blake3-aes-128-gcm` / `2022-blake3-aes-256-gcm`; the user key alone for
`2022-blake3-chacha20-poly1305`. SIP022 has no EIH for ChaCha, and one ChaCha outbound with two keys makes sing-box
(`FATAL initialize outbound[N]: Shadowsocks 2022 EIH support only available in AES ciphers`, 1.14.4) and mihomo (`proxy N: …`,
1.19.33; a proxy-provider fails as a whole too) reject the **entire** config. The node stays in every format that can express it
(clash-meta, sing-box, Shadowrocket, v2ray), so the subscription is valid everywhere; whether it connects depends on the backend
accepting per-user ChaCha keys (XrayR/V2bX cannot, NODE_PROTOCOLS.md §2.1.1). Stash, Loon and Surge get no ChaCha 2022 nodes,
classic Clash, Surfboard, Quantumult X and `ssr` no 2022 nodes at all (client table above). The golden tests check every
rendered 2022 password: each `:`-separated key is base64 of the cipher's key size, and only AES ciphers carry two.

**Child nodes** are entries of their own: child `host`, `port`, `name`, `tags`; parent `protocol_settings`, credentials, keys and status.
**Port hopping**: a `port` like `20000-30000` renders the first port plus `ports`/`server_ports`/`port-hopping`/`mport` for
hysteria (and `port-range` for mieru); other types use the first port.

**Node names** are normalized for every format: `,` → `，`, `=` → `＝`, control characters and repeated spaces collapsed,
optional emoji removal (`emoji=0`), an optional protocol prefix such as `[vmess]` plus a space
(`show_protocol_to_server_enable`; hysteria v2 shows `hysteria2`, shadowsocks `ss`). Names colliding with each other, with a
group or with a built-in policy (`DIRECT`, `REJECT`, `direct`, …) get a suffix: `<name> 2`, `<name> 3`, ….

## Endpoints

- `GET` and `HEAD /api/v1/client/subscribe?token=`, `GET` and `HEAD /s/:token` (`internal/api/client`): rate limited per
  token (one shared bucket for both routes, 120/h, burst 30 → 429 + `Retry-After`). Missing, malformed (> 64 chars or not
  `[A-Za-z0-9_-]`) or unknown token → `403 {"code":"forbidden","message":"forbidden"}`. A format that cannot be
  encrypted while the user's age key is on and `subscribe_age_strict` is set → `406 {"code":"not_acceptable"}` (no
  subscription headers). Every served request (200 and 304, GET and HEAD) is written to the access log. The token never reaches the logs:
  the access log records the route pattern (`/s/:token`) and never query strings, and the handler also rewrites the short
  link's path to `/s/[redacted]`. The INFO line `subscription served` has user id, format, client (User-Agent, 80 chars)
  proxy count and whether the body was encrypted. Invalid overrides (`source`, `cats`, `final`, `mirror`) → 400
  `validation_failed` with field errors (`profile` never fails: an unknown value selects the default profile).
- `GET /api/v1/user/subscription/preview?client=<format or client name>&…` (default `clash-meta`): same engine and query
  parameters for the logged-in user; `text/plain` body (v2ray, shadowrocket and ssr are shown base64-decoded), headers
  `X-Subscription-Format` and `Cache-Control: no-store`. `client` goes through `DetectFormat`, so an unknown name previews
  `v2ray`.

## Response headers

- `subscription-userinfo: upload=<u>; download=<d>; total=<transfer_enable>; expire=<unix|0>`
- `profile-update-interval: <subscribe_update_interval>` (hours)
- `content-disposition: attachment; filename="<ASCII fallback>"; filename*=UTF-8''<RFC 5987 app_name>` (non-ASCII, `"` and `\` → `_` in the fallback)
- `profile-web-page-url: <app_url>` (only when set)
- `ETag: "<sha1(body)>"`, or `"<sha1(plaintext)>-age-<key tag>"` for an encrypted body (the ciphertext differs on every
  request; the key tag is the first 8 hex digits of SHA-256 of the public key, so a new key never matches). The gzip
  middleware turns it into `W/"…"` when it compresses the body. `If-None-Match` naming the tag exactly (quoted, bare or
  `W/`, also in a list, or `*`) → 304 without body; a plaintext tag never matches an encrypted one or the reverse.
- `X-Subscription-Encryption: age` on encrypted bodies (ggboard-specific, informational).
- `Cache-Control: no-store`.

## Info nodes

When `show_info_to_server_enable`, pseudo nodes (shadowsocks `aes-128-gcm` → `127.0.0.1:1080`, password = uuid) come first, localized
by `default_locale` (zh-CN default / en): `账户已封禁` | `套餐已过期` | `流量已用尽` (only for unavailable users), `剩余流量：X GB`,
`套餐到期：YYYY-MM-DD` (or `长期有效`), `距离下次重置剩余：N 天` (when `next_reset_at` is set and the plan has not expired).
They are members of the `Proxy` select group only (never of url-test/fallback/load-balance or other groups).
Unavailable users (`!u.IsAvailable`: banned, expired or `u + d ≥ transfer_enable`) get only these, and users without a group
get no node entries; with the setting off they get a valid config without proxies (groups fall back to DIRECT). **Shadowrocket** gets `STATUS=🚀↑:…GB,↓:…GB,TOT:…GB💡Expires:…` and
`REMARKS=<app_name>` lines instead of info nodes.

## Filtering query params

`types=vmess,vless,...` (aliases `ss`, `ssr`, `hy`/`hy1`/`hysteria1`, `hy2`/`hysteria2`; `hysteria` = both; default all);
`filter=HK|JP` or `filter=HK,JP` (case-insensitive substring of name or tag; both lists split on `,` and `|`);
`emoji=0` (strip emoji from names; any `strconv.ParseBool` value);
`list=1` (surge, surfboard, loon, quantumultx: proxy lines only, for Surge `policy-path` / QX `[server_remote]` / Loon node subscriptions);
`age=0` (plaintext although the user's age key is on, only while `subscribe_age_optional`; see "Encrypted subscriptions (age)");
`profile=<slug>` (one of the user's rule profiles; see "Multiple profiles").
The `types`/`filter` filters run before the profile's node operations.

## Rule profiles (the user's choice)

```jsonc
RuleProfile {
  "template": "standard",            // id from catalog.templates; "none" = no rules (proxy everything, clients' default)
  "source": "blackmatrix7",          // preferred rule-set source id; per client, categories it has no files for come from other sources
  "categories": {                    // category id -> target policy group name; absent/false/null = disabled
    "ads": "REJECT", "cn": "DIRECT", "lan": "DIRECT", "openai": "AI", "claude": "AI", "telegram": "Telegram", "netflix": "Streaming"
  },
  "groups": [                        // policy groups shown in the client, in order; "Proxy" is always present
    {"name": "Proxy",     "type": "select",   "include": "auto, *"},   // "include": group names, DIRECT/REJECT, "*" = all nodes, else RE2 regex on node names
    {"name": "AI",        "type": "select",   "include": "Proxy, US|SG|JP"},
    {"name": "auto",      "type": "url-test", "include": "*", "url": "http://www.gstatic.com/generate_204", "interval": 300}
  ],
  "final": "Proxy",                  // policy for unmatched traffic (DIRECT, REJECT or a group)
  "geoip_cn_direct": true,           // append GEOIP,CN,DIRECT (sing-box: metacubex geoip/cn.srs rule set)
  "mirror": "default",               // "" = direct GitHub raw; "default" = settings.ruleset_mirror (itself a prefix or a mirror id); a catalog mirror id; or an http(s) prefix ending in "/"
  "dns": "default",                  // "default" (non-fake-IP DNS block) | "fakeip" | "none" (no DNS block)
  "nodes": NodeOps                   // node operations, see "Node operations" (always present; defaults change nothing)
}
```

Stored in `rule_profiles.data` as a **complete** profile: `PUT /user/rule-profile` fills omitted parts from the chosen template
(`categories`, `groups`, `final`, `geoip_cn_direct`; `source` from `ruleset_default_source`, `mirror` `default`, `dns` `default`)
and validates strictly (400 with field errors): the complete profile as stored is at most 256 KiB (checked first; an oversized
profile gets only a `profile` error, about twice what a profile at every limit below needs); known template/source/categories
(unknown JSON fields rejected); 1–32 groups,
unique names (≤ 64 chars, no `, = * " '`, no control/line-separator/noncharacter runes, not DIRECT/REJECT/reserved policy names),
`type` ∈ select/url-test/fallback/load-balance, health-checked groups need an http(s) `url` (≤ 512 bytes) and `interval` 10–86400 s,
include lists ≤ 2048 bytes whose tokens must be groups, DIRECT/REJECT, `*` or valid RE2 (≤ 256 bytes; at most 32 expressions across
all groups, each compiling to ≤ 256 instructions and ≤ 4096 rune ranges, since they are matched against every proxy on every render;
expressions past a count limit are refused without being compiled, and the size is bounded from the parse tree before compiling),
no self-inclusion and no include loops; targets and `final` must be DIRECT, REJECT or a group; valid dns; a custom mirror must be an
`https://` prefix ending in "/". `types`/`filter` query lists accept at most 64 entries.
Group resolution at render time: tokens that match nothing are dropped, references that would loop are dropped, an empty group
gets `Proxy` (or DIRECT when that would loop, and always for `Proxy` itself). A category whose target group is missing routes to `final`.

`PUT` without a `nodes` key keeps the profile's stored node operations (older clients that do not know them cannot
wipe them); `nodes: {}` resets them.

Query params override per request: `profile=<slug>` selects one of the user's profiles (see "Multiple profiles"); when
the value is not one of the user's slugs but a catalog template id, it keeps its original meaning, a template override
of the default profile (template categories/groups/final; stored source/mirror/dns/nodes kept); any other value selects
the default profile;
`source=`, `cats=openai,telegram` (enable only these: stored target, else template target, else the category's `default_target`;
`cats=` empty disables all), `final=`, `mirror=0|1|<mirror id>` (`0`/`false` = direct, `1`/`true` = profile mirror or `default`).
Stale stored references (template/source/category removed by the admin) fall back to the defaults silently.
Admin can disable user customization (`allow_user_rule_profile=false`) → everyone gets `ruleset_default_template` (stored
profiles and query overrides are ignored; `PUT /user/rule-profile` → 403).

`GET /user/rule-profile` returns the user's default profile (stored, or the default template's) with extra fields:
`{"customized": bool (a profile is stored), "editable": allow_user_rule_profile, "effective": RuleProfile (what subscriptions use),
"id", "slug", "name", "is_default"}`. `GET /user/rule-profiles/:slug` returns the same view for any profile.

## Multiple profiles

A user has up to **10** named rule profiles (`rule_profiles` rows: `slug` unique per user, `name`, `is_default`, `data`).
Exactly one is the default; it is what `GET/PUT /user/rule-profile` (kept as an alias, so older SPA builds keep working)
and a subscription without `?profile=` use. Rules:

- A user without rows has a **virtual default profile** (`slug` `default`, name `默认`) that follows the admin's default
  template. Listing profiles, saving any profile or creating one writes its row first, so a new profile never becomes the
  default by accident.
- Rows migrated from the one-profile-per-user table of earlier releases become `default` / `默认` / `is_default=true`
  (see docs/DATA_MODEL.md).
- `data` is `null` while a profile follows the defaults (`customized: false`); `PUT /user/rule-profiles/:slug` stores a
  complete profile exactly like `PUT /user/rule-profile`.
- Slugs match `^[a-z0-9][a-z0-9_-]{0,31}$`. `POST /user/rule-profiles` without `slug` derives one from the name (ASCII
  letters and digits, other runs become `-`, at most 28 characters; `-2`, `-3`, … on collisions; `profile-N` when nothing
  is left, e.g. for Chinese names). Names: 1–64 characters, no control or invisible characters.
- `copy_from: <slug>` copies that profile's document; the default profile cannot be deleted (409); `POST
  /user/rule-profiles/:slug/default` switches the default.
- Subscription links: `GET /user/subscription` returns `profiles: [{slug, name, is_default, subscribe_url}]`, where the
  default profile's link is the plain subscription URL and every other link appends `&profile=<slug>`. With
  `allow_user_rule_profile=false` every profile write is refused (403) and subscriptions ignore stored profiles.
- Writes run in a transaction that holds the user's row lock, so concurrent creates never exceed the limit and exactly
  one profile stays the default. Profiles are cached per user for a minute (`rule_profile` namespace + a per-user
  generation bumped on every write).

## Node operations

Sub-Store-style operations stored in each profile under `nodes`. They apply to **node entries only** (info entries keep
their place in front and are never filtered or renamed), for every format, in this order:

1. **filters**, all of which must pass: `types` (node types `vmess`, `vless`, `shadowsocks`, `shadowsocksr`, `trojan`,
   `hysteria`, `anytls`, `mieru`, `tuic` or the `?types=` aliases `ss`, `ssr`, `hy`/`hy1`/`hysteria1`, `hy2`/`hysteria2`),
   `regions` (the node's region, see below, is one of the listed codes), `include` (the name matches at least one
   expression), `exclude` (the name matches none);
2. **dedupe**: keep the first node per protocol (`ss`, `ssr`, `vmess`, `vless`, `trojan`, `hysteria`, `hysteria2`, `tuic`,
   `anytls`, `mieru`), host (case-insensitive) and port, in the admin's order (`sort`, `id`);
3. **rename** rules in order: every match of `match` is replaced by `replace` (Go `regexp.ReplaceAllString`: `$1`,
   `${1}`, `${name}`; write `${1}x` when letters or digits follow, `$1x` means the group named `1x`, `$$` is a literal `$`);
4. **emoji**: `keep` (default), `strip` (remove every emoji), `add` (prepend the region flag and a space when the name has
   no flag emoji yet and the region is known);
5. **prefix** / **suffix** (plain text);
6. **sort**: `default` (keep `sort`, `id`), `name` (natural, case-insensitive: `HK 2` before `HK 10`), `region` (by the
   position of the region in `regions` when it is set, otherwise by the table below; unknown regions last; stable);
7. **limit**: keep the first N nodes (0 = all).

Afterwards the usual name normalization runs again (`,` → `，`, `=` → `＝`, whitespace collapsed), then `emoji=0` and the
protocol prefix (`show_protocol_to_server_enable`). Expressions are RE2, unanchored, matched against the normalized
node name (add `(?i)` for case-insensitive matching); an expression that no longer compiles (stored before a rule change)
matches literally.

```jsonc
NodeOps {
  "include": ["香港|HK", "(?i)japan"],   // RE2; empty = every node
  "exclude": ["(?i)expire|剩余"],
  "types":   ["vless", "hy2"],           // empty = every type
  "regions": ["HK", "JP", "UK"],         // ISO 3166-1 alpha-2 (UK is accepted for GB); empty = every region
  "rename":  [{"match": "^(\\S+) (\\d+)$", "replace": "${1}-$2"}],
  "prefix": "", "suffix": "",
  "emoji": "keep",                       // keep | add | strip
  "sort": "default",                     // default | name | region
  "dedupe": false,
  "limit": 0                             // 0 = all
}
```

Validation (`PUT` → 400 with `nodes.*` field errors): at most 32 expressions across `include` and `exclude`; at most 64
rename rules (expressions past either limit are not compiled, only the first 64 rules are checked); every expression
non-empty, ≤ 256 bytes, valid RE2 within the group-include complexity limits (≤ 256 instructions, ≤ 4096 rune ranges);
`replace` ≤ 256 bytes; `prefix`/`suffix` ≤ 32 characters; no control, line-separator
or noncharacter runes in replacements, prefixes and suffixes; ≤ 16 known `types`; ≤ 64 known `regions`; `emoji`,
`sort` from the lists above; `limit` 0–1000. Unknown keys are rejected like the rest of the profile.

**Region detection** (`subscription.DetectRegion`), first match wins:

1. the first **flag emoji** in the name (two regional indicator symbols: `🇭🇰` → HK, `🇬🇧` → GB; any flag counts, also one
   outside the table);
2. the first row of the table whose **name keyword** occurs in the name or in one of the node's tags, as a
   case-insensitive substring;
3. the first row whose **token** occurs in the name or a tag as a whole word: case-sensitive, not preceded or followed by
   an ASCII letter (`HK01`, `JP-2` and `[US]` match, `hk01` and `PHK` do not), and `CN` never matches `CN2` (a carrier
   route).

The row order is a priority: relay names usually start with the domestic entry city (`上海-香港 IPLC` is HK, so CN comes
last) and 印度尼西亚 must win over 印度. Unknown names have no region: `regions` drops them, `emoji: add` leaves them alone,
`sort: region` puts them last.

| # | code | name keywords (case-insensitive substrings) | tokens (upper case, whole word) |
| --- | --- | --- | --- |
| 1 | HK | 香港, Hong Kong, HongKong, 深港, 沪港, 京港, 广港, 杭港, 莞港 | HK, HKG, HKT, HKBN, HGC, WTT |
| 2 | MO | 澳门, 澳門, Macao, Macau | MO, MAC, CTM |
| 3 | TW | 台湾, 臺灣, 台灣, 台北, 臺北, 新北, 台中, 高雄, 彰化, Taiwan, Taipei | TW, TWN, TPE, CHT, HINET |
| 4 | JP | 日本, 东京, 東京, 大阪, 埼玉, 名古屋, Japan, Tokyo, Osaka | JP, JPN, TYO, NRT, HND, KIX, OSA |
| 5 | KR | 韩国, 韓國, 南韩, 南韓, 首尔, 首爾, 春川, Korea, Seoul | KR, KOR, SEL, ICN |
| 6 | SG | 新加坡, 狮城, 獅城, Singapore | SG, SGP, SIN |
| 7 | US | 美国, 美國, 美西, 美东, 美東, 洛杉矶, 洛杉磯, 圣何塞, 聖何塞, 西雅图, 西雅圖, 芝加哥, 纽约, 紐約, 硅谷, 矽谷, 达拉斯, 達拉斯, 凤凰城, 鳳凰城, 波特兰, 弗里蒙特, United States, Los Angeles, San Jose, Seattle, Chicago, New York, Dallas, Silicon Valley | US, USA, LAX, SJC, SEA, ORD, JFK, DFW, NYC |
| 8 | GB | 英国, 英國, 伦敦, 倫敦, United Kingdom, Britain, England, London | GB, GBR, UK, LHR |
| 9 | DE | 德国, 德國, 法兰克福, 法蘭克福, 柏林, Germany, Frankfurt, Berlin | DE, DEU |
| 10 | FR | 法国, 法國, 巴黎, France, Paris | FR, FRA, CDG |
| 11 | NL | 荷兰, 荷蘭, 阿姆斯特丹, Netherlands, Amsterdam | NL, NLD, AMS |
| 12 | RU | 俄罗斯, 俄羅斯, 莫斯科, 伯力, 圣彼得堡, Russia, Moscow | RU, RUS |
| 13 | CA | 加拿大, 多伦多, 多倫多, 温哥华, 溫哥華, 蒙特利尔, Canada, Toronto, Vancouver, Montreal | CA, CAN, YVR, YYZ |
| 14 | AU | 澳大利亚, 澳大利亞, 澳洲, 悉尼, 墨尔本, 墨爾本, Australia, Sydney, Melbourne | AU, AUS, SYD |
| 15 | ID | 印尼, 印度尼西亚, 印度尼西亞, 雅加达, 雅加達, Indonesia, Jakarta | ID, IDN, CGK |
| 16 | IN | 印度, 孟买, 孟買, India, Mumbai | IN, IND, BOM |
| 17 | TH | 泰国, 泰國, 曼谷, Thailand, Bangkok | TH, THA, BKK |
| 18 | VN | 越南, 胡志明, 河内, 河內, Vietnam, Hanoi | VN, VNM, SGN |
| 19 | MY | 马来西亚, 馬來西亞, 吉隆坡, Malaysia, Kuala Lumpur | MY, MYS, KUL |
| 20 | PH | 菲律宾, 菲律賓, 马尼拉, 馬尼拉, Philippines, Manila | PH, PHL, MNL |
| 21 | TR | 土耳其, 伊斯坦布尔, Turkey, Türkiye, Istanbul | TR, TUR, IST |
| 22 | AE | 阿联酋, 阿聯酋, 迪拜, 杜拜, United Arab Emirates, Dubai | AE, ARE, UAE, DXB |
| 23 | IL | 以色列, Israel | IL, ISR |
| 24 | BR | 巴西, 圣保罗, 聖保羅, Brazil, Sao Paulo, São Paulo | BR, BRA, GRU |
| 25 | AR | 阿根廷, Argentina | AR, ARG |
| 26 | MX | 墨西哥, Mexico | MX, MEX |
| 27 | CL | 智利, Chile | CL, CHL |
| 28 | ZA | 南非, 约翰内斯堡, South Africa, Johannesburg | ZA, ZAF, JNB |
| 29 | EG | 埃及, Egypt | EG, EGY |
| 30 | NG | 尼日利亚, 尼日利亞, Nigeria | NG, NGA |
| 31 | IT | 意大利, 義大利, 米兰, 米蘭, Italy, Milan | IT, ITA, MXP |
| 32 | ES | 西班牙, 马德里, 馬德里, Spain, Madrid | ES, ESP, MAD |
| 33 | PT | 葡萄牙, 里斯本, Portugal, Lisbon | PT, PRT |
| 34 | CH | 瑞士, 苏黎世, 蘇黎世, Switzerland, Zurich | CH, CHE, ZRH |
| 35 | SE | 瑞典, 斯德哥尔摩, Sweden, Stockholm | SE, SWE |
| 36 | NO | 挪威, Norway, Oslo | NO, NOR |
| 37 | FI | 芬兰, 芬蘭, 赫尔辛基, Finland, Helsinki | FI, FIN, HEL |
| 38 | DK | 丹麦, 丹麥, Denmark, Copenhagen | DK, DNK |
| 39 | PL | 波兰, 波蘭, 华沙, 華沙, Poland, Warsaw | PL, POL, WAW |
| 40 | AT | 奥地利, 奧地利, 维也纳, 維也納, Austria, Vienna | AT, AUT, VIE |
| 41 | BE | 比利时, 比利時, Belgium, Brussels | BE, BEL |
| 42 | IE | 爱尔兰, 愛爾蘭, 都柏林, Ireland, Dublin | IE, IRL, DUB |
| 43 | UA | 乌克兰, 烏克蘭, Ukraine, Kyiv | UA, UKR |
| 44 | KZ | 哈萨克斯坦, 哈薩克, Kazakhstan | KZ, KAZ |
| 45 | MN | 蒙古, Mongolia (never when the name contains 内蒙古 or 內蒙古) | MN, MNG |
| 46 | NZ | 新西兰, 新西蘭, 紐西蘭, 奥克兰, New Zealand, Auckland | NZ, NZL, AKL |
| 47 | CN | 中国, 中國, 回国, 回國, 内蒙古, 內蒙古, 北京, 上海, 广州, 廣州, 深圳, 杭州, 江苏, 江蘇, China | CN, CHN |

### Compared with Sub-Store

Sub-Store's built-ins (`backend/src/core/proxy-utils/processors/index.js`, `utils/geo.js`, commit `a6d09c7`, 2026-10-22)
and what ggboard offers instead:

| Sub-Store | ggboard |
| --- | --- |
| Region Filter (HK, TW, US, SG, JP, UK, DE, KR by flag) | `regions` (47 regions, flag → keyword → token) |
| Regex Filter (keep / remove) | `include` / `exclude` |
| Type Filter | `types` |
| Useless Filter (names like 流量/过期/expire, non-ASCII ciphers) | not needed: info entries are generated separately and never filtered; node settings are validated by the admin API |
| Flag Operator (add / remove; TW flag optionally replaced) | `emoji: add` / `strip` (flags are never replaced) |
| Regex Rename / Regex Delete Operator | `rename` (an empty `replace` deletes) |
| Sort Operator (asc / desc / random), Regex Sort Operator | `sort: name` (natural) / `region`; no random order (stable configs and ETags) |
| Handle Duplicate Operator (delete / rename by fields) | `dedupe` (protocol + host + port); duplicate names always get `<name> 2`, `<name> 3`, … |
| Quick Setting Operator (udp, tfo, skip-cert-verify, …) | not offered: transport settings belong to the node, set by the admin |
| Resolve Domain Operator | not offered: no DNS lookups on every render; clients resolve |
| Script Filter / Script Operator / Conditional Filter | not offered: no user scripts on the server |

`GET /user/rulesets/catalog` = `GET /guest/rulesets/catalog` (guest response has `Cache-Control: public, max-age=300`):

```jsonc
{"sources": {"<id>": {"name","homepage","license","order","description","clients":["clash-meta",...]}},   // no URL templates
 "categories": [{"id","name","default_target","order","sources":["blackmatrix7",...],"clients":["clash-meta",...],
                 "source_clients": {"<source id>": ["clash-meta",...]}}],   // per source: the clients its files serve here
 "templates": [Template], "mirrors": [{"id","name","prefix"}],
 "clients": ["clash-meta","stash","singbox","surge","surfboard","loon","quantumultx"],   // formats that receive rule sets
 "preferred_sources": {"clash-meta": "metacubex"},   // tried before the profile's source (docs/RULESETS.md, step 2)
 "defaults": {"template","source","mirror":"default","mirror_prefix"}, "editable": bool}
```

The source a client gets for a category is the first of `preferred_sources[client]`, the profile's `source`, then `sources`
(by order) whose `source_clients` lists that client; this is the rule `BuildPlan` applies. A source without files for a
client never costs that client rules: with the default `blackmatrix7` (no sing-box files), a sing-box subscription takes
every category from MetaCubeX `.srs` sets, byte for byte what a profile that chose `metacubex` gets, so users never switch
sources for one client. The rule builder shows the effective source per client from these fields.

Admin: `GET /admin/rulesets` → `{builtin: Catalog, custom: <rulesets_custom value or {}>, effective: merged Catalog}`;
`PUT /admin/rulesets {"custom": <Catalog-shaped overrides> or null}` → `true` (unknown fields → 400 on `custom`; invalid
sources/categories/templates/mirrors → 400 with field errors on the merged catalog; `null` or an omitted `custom` resets).
Overrides merge by id; see docs/RULESETS.md → *Admin overrides*.

## Encrypted subscriptions (age)

Verified 2026-10-22 against the sources listed at the end of this section.

**What mihomo does** (since **v1.19.27**, release notes: `358fa5eb feat: add age-secret-key for proxy-provider and
-age-secret-key for cmdline to support age armor format config decryption`, plus `1abd1261`, `b60e687e`, `d7ac7b52`):

- **Per provider**: a `proxy-providers` entry takes `age-secret-key: AGE-SECRET-KEY-1…` (validated when the config
  loads). Every downloaded body goes through `age.DecryptBytes`: when it **starts with**
  `-----BEGIN AGE ENCRYPTED FILE-----` (strict prefix, no leading whitespace or BOM) it is decrypted, otherwise it is
  used as is (plaintext keeps working). The result is parsed as YAML with a `proxies` list, or else as share links.
- **Whole config**: the core decrypts an armored main config with keys from the command line `-age-secret-key` or the
  environment variable `CLASH_AGE_SECRET_KEY` (no YAML key, the config cannot carry its own key).
- **Wire format**: only the official **ASCII armor** of age-encryption.org/v1 (binary age is not recognized); recipients:
  X25519 (`age1…` / `AGE-SECRET-KEY-1…`) and the mlkem768-x25519 hybrid (`age1pq1…` / `AGE-SECRET-KEY-PQ-1…`). No content
  type, header or file name is checked; the documented `X-Age-Public-Key` request header is only a convention a user can
  add under the provider's `header:` (the core never sends the public key itself).
- Tools: `mihomo age keygen` / `keygen-pq` / `convert <secret key>` / `encrypt` / `decrypt`.

**Clients**: **Clash Party** (formerly Mihomo Party, v1.9.4+ / 2.x: "新增 Age 加密配置") has an `ageSecretKey` per
subscription profile and decrypts the downloaded subscription itself (it also trims a BOM and leading whitespace);
**FlClash** (v0.9.x) stores an `age-secret-key` per profile and hands it to the core (`isAgeEncrypted` checks the exact
prefix); **Clash Verge Rev** (v2.5.8, 2026-10-09) has no age support in its code, so Verge users need a local profile with
a `proxy-providers` entry carrying `age-secret-key` (the bundled core decrypts it). Stash, the classic Clash core,
**sing-box** (it links `filippo.io/age` only to encrypt crash reports), Surge, Quantumult X, Loon, Shadowrocket and the
v2ray family cannot decrypt subscriptions. OpenClash, OneXray and Bettbox send `X-Age-Public-Key`.

**What ggboard does** (`internal/subscription/guard`):

- Each user can create one X25519 key pair (`filippo.io/age` v1.3.2; `subscription_keys`). The public key encrypts;
  the identity is stored sealed (AES-256-GCM, key = HKDF-SHA256(`jwt.secret`, info `ggboard/subscription/age-identity/v1`),
  AAD `ggboard:age:<user id>`) so the owner can display it again; rotating `jwt.secret` makes it unreadable (the public
  key keeps working; regenerate to see a private key again). Admins see the public key only.
- Formats declare support with `SupportsAge()`: only **clash-meta** (mihomo YAML, which mihomo providers, Clash Party
  and FlClash decrypt). With the key **enabled**:

  | request | `subscribe_age_optional` (default on) | `subscribe_age_strict` (default off) | response |
  | --- | --- | --- | --- |
  | clash-meta | any | any | armored age, `Content-Type: text/plain; charset=utf-8` |
  | clash-meta with `age=0` (any `strconv.ParseBool` false) | on | any | plaintext |
  | clash-meta with `age=0` | off | any | armored age (`age=0` ignored) |
  | any other format | — | off | plaintext |
  | any other format | — | on | 406 `not_acceptable` (with `age=0` and optional on: plaintext) |

  The body is the whole rendered config encrypted to the user's key (`-----BEGIN AGE ENCRYPTED FILE-----` first);
  `Subscription-Userinfo`, `Profile-Update-Interval` and `Content-Disposition` (unchanged file name, which clients show as
  the profile name) are still sent, plus `X-Subscription-Encryption: age`. Verified: mihomo v1.19.33's own
  `component/age.DecryptBytes` decrypts the output of `guard.Encrypt`.
- `X-Age-Public-Key` is **ignored** on purpose: encrypting to a key the request names would hand plaintext to anyone
  holding a leaked link. Real leak protection needs `subscribe_age_strict` on and `subscribe_age_optional` off; the
  defaults favour compatibility (a leaked link still yields plaintext through `age=0` or another format).
- The user preview (`/user/subscription/preview`) always shows plaintext.

Sources: [mihomo v1.19.27 release](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.27);
mihomo v1.19.33 source `component/age/age.go`, `adapter/provider/provider.go` (`NewProxiesParser`), `config/config.go`
(`UnmarshalRawConfig`), `main.go`, `docs/config.yaml`;
[Meta-Docs proxy-providers](https://wiki.metacubex.one/config/proxy-providers/) (`age-secret-key`, `X-Age-Public-Key`);
[ChonglangTV: mihomo Age key](https://www.chonglangtv.org/t/topic/2339) (1.19.27, key distribution scenarios, Bettbox v1.18.0);
[Clash Party](https://github.com/mihomo-party-org/clash-party) `src/main/utils/age.ts`, `src/main/config/profile.ts`;
[FlClash](https://github.com/chen08209/FlClash) `lib/common/file.dart`, `core/protocol.go`;
[sing-box](https://github.com/SagerNet/sing-box) `experimental/libbox/log.go`;
[OpenClash](https://github.com/vernesong/OpenClash) `openclash_curl.sh`.

## Access log and leak detection

- Every served subscription (200 or 304, GET or HEAD, both routes) queues a `subscription_access` row: client IP
  (`c.ClientIP()`, IPv4-mapped addresses as IPv4), `ip_bucket` (the IPv4 address, or the IPv6 /64), `token_tag` (first 8
  hex digits of SHA-256 of the token used), `client` (first product token of the User-Agent, ≤ 64 printable ASCII
  bytes, `unknown` when empty), the format and the time. Requests never wait for the database: rows are buffered in
  memory (at most 10 000; more are dropped and counted in a WARN line) and written in batches of 500 by the
  `subscription_access_flush` job every 10 s (and before the access-log endpoints answer). Rows still buffered at
  shutdown are lost (≤ 10 s).
- `subscription_access_prune` (daily 04:37) deletes rows older than 30 days and keeps the newest 100 per user.
- `GET /user/subscription/access-log`: newest 50, IP masked (`203.0.113.*`, `2001:db8:1:2:*`).
  `GET /admin/users/:id/subscription/access-log`: newest 100, unmasked, with `ip_bucket` and `token_tag`.
- **Leak detection** (`subscribe_leak_max_ips` > 0): after each flush, every (user, token tag) fetched from a bucket it
  was not seen from in the last 24 h (per process) is recounted: distinct `ip_bucket` values of that token within the
  last 24 hours. Above the limit, at most once per token and 24 h (in-memory marker; a restart can repeat one alert):
  with `subscribe_leak_action = rotate` the token is rotated (`usersvc.RotateToken`: new token only, uuid and sessions
  kept), then `subscription.LeakHook(a, user, distinct)` runs. The default hook logs at WARN and mails the user
  (template `notify`, `default_locale`): the counts and, after a rotation, the new subscription link; the button opens
  `<app_url>/subscription`. Because rows carry the token tag, a rotated or reset token starts a fresh count, also after a
  restart. Nothing happens when the token changed before the check.
- The `ggboard` binary sets `subscription.LeakHook` in `cmd/ggboard/wire_leak.go` to `DefaultLeakHook` followed by
  `notify.EmitSubscriptionLeak`, so administrators also get the `subscription_leak` notification (Telegram, when it is in
  `telegram_events`; the user is told by mail only). Integrators may replace the hook and call
  `subscription.DefaultLeakHook` from the replacement; `nil` disables the notification (a rotation still happens).

## Catalog

`internal/subscription/rulesets/catalog.json` (embedded) holds 6 sources (`blackmatrix7`, `metacubex`, `loyalsoldier`,
`sagernet`, `acl4ssr`, `dustinwin`), 32 categories, the templates `none`, `minimal`, `standard`, `full` and 5 mirrors
(`direct`, `ghfast`, `gh-proxy`, `ghproxy-net`, `gh-llkk`). Admin overrides from the hidden setting `rulesets_custom` are
merged by id. Schema, coverage, source selection and verification are in docs/RULESETS.md.

## Rule emission per format

docs/RULESETS.md is authoritative for source and key selection.

- Every format with rules starts with the request host forced direct (`DOMAIN,<host>,DIRECT`, sing-box `domain` rule, QX `host,`), so
  profile refreshes never loop through a proxy (skipped for IP hosts).
- **clash-meta / stash**: `rule-providers` (`type: http` for mihomo only, `behavior`, `format: yaml|text|mrs`, `url`, `path: ./rules/<tag>.<ext>`,
  `interval: 86400`) + `RULE-SET,<tag>,<target>[,no-resolve]` (IP sets outside lan/cn); then `GEOIP,CN,DIRECT` (if enabled) and `MATCH,<final>`.
  clash-meta prefers `metacubex` `.mrs` sets; with a mirror it also sets `geox-url` (geoip/geosite/mmdb/asn from meta-rules-dat `release`).
  DNS (`dns != none`): `default` = redir-host with domestic DoH + foreign fallback (geoip filter), `fakeip` = fake-ip (198.18.0.1/16) with filter list;
  Stash gets `default-nameserver` + `nameserver` only.
- **clash (classic)**: no rule providers; `GEOIP,CN,DIRECT` + `MATCH` (documented limitation).
- **singbox (≥ 1.12)**: typed DNS servers (`https` remote via `Proxy`, `https` local, `fakeip`), DNS rules (request host and DIRECT
  domain sets → local, A/AAAA → fakeip), `route.default_domain_resolver: local`; inbounds `tun` + `mixed` (127.0.0.1:2080);
  outbounds = groups (`selector`/`urltest`; fallback/load-balance map to urltest), proxies, `direct`; route rules
  `sniff` → `hijack-dns` → `ip_is_private` direct → host → one rule per category (`rule_set: [...]`, `outbound` or `action: reject`)
  → `geoip-cn` (metacubex `geoip/cn.srs`, unless a category already ships that file); `route.rule_set` entries
  `{type: remote, format: binary|source, url, download_detour: Proxy|direct (mirror), update_interval: 24h}`; `final` (REJECT final =
  trailing `reject` action, final `direct`); `experimental.cache_file` enabled (`store_fakeip` with fakeip). No clash_api.
- **surge / surfboard**: managed profile `#!MANAGED-CONFIG <request URL> interval=<hours*3600> strict=false`, `[General]`, `[Proxy]`,
  `[Proxy Group]`, `[Rule]` with `RULE-SET,<url>,<target>` (classical), `DOMAIN-SET,<url>,<target>` (domain), `GEOIP,CN,DIRECT`,
  `FINAL,<final>,dns-failed`.
- **loon**: `[General]`, `[Proxy]`, `[Proxy Group]`, `[Rule]` (host, `GEOIP,CN,DIRECT`, `FINAL`), `[Remote Rule]`
  `<url>, policy=<target>, tag=<tag>, enabled=true`. Loon reads `[Proxy]` from the same URL when added as a node subscription.
- **quantumultx**: `[general]`, `[dns]`, `[policy]` (`static`, `url-latency-benchmark`, `available`, `round-robin`; `direct`/`reject`
  lowercase), `[server_local]`, `[filter_remote]` `<url>, tag=<tag>, force-policy=<target>, update-interval=86400, opt-parser=false, enabled=true`,
  `[filter_local]` (`host, <host>, direct`, `geoip, cn, direct`, `final, <final>`). `?list=1` returns the server lines only (for `[server_remote]`).
- **shadowrocket / v2ray / ssr**: no rules (share links only).

`mirror` prefixes every GitHub URL (`raw.githubusercontent.com`, `github.com`, `gist.githubusercontent.com`:
`<mirror>https://raw.githubusercontent.com/...`); URLs on other hosts (admin sources) are left alone.
