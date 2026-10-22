# Subscription generation & rule sets

Packages (all under `internal/subscription`):

| package | role |
|---|---|
| `subscription` | `Format` registry, `DetectFormat`, `RenderContext`, `RuleProfile`, catalog types, `AvailableNodes`, `Proxy` (decoded node + credentials), info nodes, policy-group resolution |
| `subscription/rulesets` | embedded `catalog.json`, admin overrides (`rulesets_custom`), profile decode/validate/resolve, per-format rule plan, public catalog view |
| `subscription/formats` | one file per output format, registered from `init()` (blank-import the package) |
| `subscription/engine` | `Generate` (DB + settings → body) and the pure `Render`; stored profile load/save |
| `subscription/fixture` | deterministic user + 25 nodes covering every type/transport (golden and API tests) |

Request flow (`engine.Generate`): token user → effective rule profile → info nodes (`show_info_to_server_enable`) +
`AvailableNodes` (only when `u.IsAvailable`) → query filters → format filter (`Supports` + optional `SupportsProxy`)
→ unique names → groups → rule plan → `Format.Render` → `ETag = sha1(body)`.

## Client detection (`flag` query or User-Agent, case-insensitive, first match wins)
| flag/UA contains | format | node types rendered |
|---|---|---|
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
type RenderContext struct{ User; Nodes; Proxies []Proxy; Groups []ResolvedGroup; Rules *RulePlan; Profile; Catalog;
    Settings; SubscribeURL; RequestHost; AppName; Locale; Mirror; UserAgent; ListOnly; UpdateHours; Now; Location }
```
Content types: `text/yaml; charset=utf-8` (clash family), `application/json; charset=utf-8` (singbox), `text/plain; charset=utf-8` (others).

Share-link conventions (v2ray/shadowrocket): vmess = base64 JSON `{v:"2",ps,add,port,id,aid,scy,net,type,host,path,tls,sni,fp}` (all values
strings, as Xboard; `type` = tcp header type, `gun` for grpc, xhttp mode for xhttp);
vless `vless://uuid@host:port?encryption=none&security=tls|reality|none&sni=&fp=&pbk=&sid=&type=tcp|ws|grpc|http|httpupgrade|xhttp&headerType=&host=&path=&serviceName=&mode=&extra=&flow=&allowInsecure=1#name`;
ss `ss://base64url(method:password)@host:port[/?plugin=obfs-local;obfs=http;obfs-host=…]#name` (password = `<server_key>:<user_key>` for 2022 ciphers, see docs/NODE_PROTOCOLS.md §4);
ssr `ssr://base64url(host:port:protocol:method:obfs:base64url(pass)/?obfsparam=&protoparam=&remarks=[&group=])`;
trojan `trojan://pass@host:port?security=tls&sni=&type=&host=&path=&serviceName=&allowInsecure=1#name`;
hysteria2 `hysteria2://pass@host:port?sni=&insecure=1&obfs=salamander&obfs-password=&mport=10000-20000#name`;
hysteria (v1) `hysteria://host:port?protocol=udp&auth=&upmbps=&downmbps=&obfs=xplus&obfsParam=&peer=&insecure=1&alpn=#name`;
tuic `tuic://uuid:pass@host:port?sni=&alpn=h3&congestion_control=bbr&udp_relay_mode=native&allow_insecure=1#name`;
anytls `anytls://pass@host:port?sni=&insecure=1[&security=reality&pbk=&sid=&fp=]#name`; mieru `mieru://username:password@host:port?transport=TCP&multiplexing=MULTIPLEXING_LOW#name`.
IPv6 literals are bracketed. Params are emitted only when set (`insecure=1` only when certificate checks are off).

**SSR credentials** (soga/SSPanel single-port): password = node password (`protocol_settings.password`, else
`base64(md5hex(P.created_at)[:16])`, the same value soga receives), protocol param = `"<user id>:<uuid>"`, obfs param = `obfs_param`.

**Child nodes** are entries of their own: child `host`, `port`, `name`, `tags`; parent `protocol_settings`, credentials, keys and status.
**Port hopping**: a `port` like `20000-30000` renders the first port plus `ports`/`server_ports`/`port-hopping`/`mport` for
hysteria (and `port-range` for mieru); other types use the first port.

**Node names** are normalized for every format: `,` → `，`, `=` → `＝`, control characters and repeated spaces collapsed,
optional emoji removal (`emoji=0`), optional `[vmess] ` prefix (`show_protocol_to_server_enable`; hysteria v2 shows `hysteria2`,
shadowsocks `ss`). Names colliding with each other, with a group or with a built-in policy (`DIRECT`, `REJECT`, `direct`, …)
get ` 2`, ` 3`, ….

## Endpoints
- `GET|HEAD /api/v1/client/subscribe?token=` and `GET|HEAD /s/:token`: rate limited per token (shared bucket, 120/h, burst 30 → 429 +
  `Retry-After`). Missing, malformed (> 64 chars or not `[A-Za-z0-9_-]`) or unknown token → `403 {"code":"forbidden","message":"forbidden"}`.
  The short link's path is redacted (`/s/[redacted]`) before the access log runs; the INFO log line has user id, format and
  client only. Invalid overrides (`profile`, `source`, `cats`, `final`, `mirror`) → 400 with field errors.
- `GET /api/v1/user/subscription/preview?client=<format or client name>&…` (default `clash-meta`): same engine for the
  logged-in user, `text/plain` body (base64 formats are shown decoded), header `X-Subscription-Format`.

## Response headers
- `subscription-userinfo: upload=<u>; download=<d>; total=<transfer_enable>; expire=<unix|0>`
- `profile-update-interval: <subscribe_update_interval>` (hours)
- `content-disposition: attachment; filename="<ASCII fallback>"; filename*=UTF-8''<RFC 5987 app_name>` (non-ASCII, `"` and `\` → `_` in the fallback)
- `profile-web-page-url: <app_url>` (only when set)
- `ETag: "<sha1(body)>"`; `If-None-Match` containing the hex (quoted, bare or `W/`) → 304 without body.
- `Cache-Control: no-store`.

## Info nodes
When `show_info_to_server_enable`, pseudo nodes (shadowsocks `aes-128-gcm` → `127.0.0.1:1080`, password = uuid) come first, localized
by `default_locale` (zh-CN default / en): `账户已封禁` | `套餐已过期` | `流量已用尽` (only for unavailable users), `剩余流量：X GB`,
`套餐到期：YYYY-MM-DD` (or `长期有效`), `距离下次重置剩余：N 天` (when `next_reset_at` is set and the plan has not expired).
They are members of the `Proxy` select group only (never of url-test/fallback/load-balance or other groups).
Unavailable users (banned, expired, traffic used up, no plan/group) get only these; with the setting off they get a valid
config without proxies (groups fall back to DIRECT). **Shadowrocket** gets `STATUS=🚀↑:…GB,↓:…GB,TOT:…GB💡Expires:…` and
`REMARKS=<app_name>` lines instead of info nodes.

## Filtering query params
`types=vmess,vless,...` (aliases `ss`, `ssr`, `hy`/`hysteria1`, `hy2`/`hysteria2`; `hysteria` = both; default all);
`filter=HK|JP` or `filter=HK,JP` (case-insensitive substring of name or tag); `emoji=0` (strip emoji from names);
`list=1` (surge, surfboard, loon, quantumultx: proxy lines only, for Surge `policy-path` / QX `[server_remote]` / Loon node subscriptions).

## Rule profiles (the user's choice)
```jsonc
RuleProfile {
  "template": "standard",            // id from catalog.templates; "none" = no rules (proxy everything, clients' default)
  "source": "blackmatrix7",          // preferred rule-set source id; categories fall back to other sources that carry them
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
  "mirror": "default",               // "" = direct GitHub raw; "default" = settings.ruleset_mirror; a catalog mirror id; or an https prefix ending in "/"
  "dns": "default"                   // "default" (non-fake-IP DNS block) | "fakeip" | "none" (no DNS block)
}
```
Stored in `rule_profiles.data` as a **complete** profile: `PUT /user/rule-profile` fills omitted parts from the chosen template
(`categories`, `groups`, `final`, `geoip_cn_direct`; `source` from `ruleset_default_source`, `mirror` `default`, `dns` `default`)
and validates strictly (400 with field errors): known template/source/categories (unknown JSON fields rejected); 1–32 groups,
unique names (≤ 64 chars, no `, = * " '`, no control/line-separator/noncharacter runes, not DIRECT/REJECT/reserved policy names),
`type` ∈ select/url-test/fallback/load-balance, health-checked groups need an http(s) `url` (≤ 512 bytes) and `interval` 10–86400 s,
include lists ≤ 2048 bytes whose tokens must be groups, DIRECT/REJECT, `*` or valid RE2 (≤ 256 bytes; at most 32 expressions across
all groups, each compiling to ≤ 256 instructions and ≤ 4096 rune ranges, since they are matched against every proxy on every render),
no self-inclusion and no include loops; targets and `final` must be DIRECT, REJECT or a group; valid dns; a custom mirror must be an
`https://` prefix ending in "/". `types`/`filter` query lists accept at most 64 entries.
Group resolution at render time: tokens that match nothing are dropped, references that would loop are dropped, an empty group
gets `Proxy` (or DIRECT when that would loop, and always for `Proxy` itself). A category whose target group is missing routes to `final`.

Query params override per request: `profile=<template>` (template categories/groups/final; stored source/mirror/dns kept),
`source=`, `cats=openai,telegram` (enable only these: stored target, else template target, else the category's `default_target`;
`cats=` empty disables all), `final=`, `mirror=0|1|<mirror id>` (0 = direct, 1 = profile mirror or `default`).
Stale stored references (template/source/category removed by the admin) fall back to the defaults silently.
Admin can disable user customization (`allow_user_rule_profile=false`) → everyone gets `ruleset_default_template` (stored
profiles and query overrides are ignored; `PUT /user/rule-profile` → 403).

`GET /user/rule-profile` returns the stored profile (or the default template's) with three extra fields:
`{"customized": bool (a profile is stored), "editable": allow_user_rule_profile, "effective": RuleProfile (what subscriptions use)}`.

`GET /user/rulesets/catalog` = `GET /guest/rulesets/catalog` (guest response has `Cache-Control: public, max-age=300`):
```jsonc
{"sources": {"<id>": {"name","homepage","license","order","description","clients":["clash-meta",...]}},   // no URL templates
 "categories": [{"id","name","default_target","order","sources":["blackmatrix7",...],"clients":["clash-meta",...]}],
 "templates": [Template], "mirrors": [{"id","name","prefix"}],
 "clients": ["clash-meta","stash","singbox","surge","surfboard","loon","quantumultx"],   // formats that receive rule sets
 "defaults": {"template","source","mirror":"default","mirror_prefix"}, "editable": bool}
```
Admin: `GET /admin/rulesets` → `{builtin: Catalog, custom: <rulesets_custom value or {}>, effective: merged Catalog}`;
`PUT /admin/rulesets {"custom": Catalog-shaped overrides | null}` → `true` (unknown fields, invalid sources/categories/templates/mirrors → 400
with field errors on the merged catalog; `null` resets). Overrides merge by id (see docs/RULESETS.md).

## Catalog (`internal/subscription/rulesets/catalog.json`, embedded; admin overrides merged from setting `rulesets_custom`)
Original sketch, kept for history: the verified schema (format keys, `url_meta`, `split`, `formats`, mirrors) is in docs/RULESETS.md.
```jsonc
{
  "sources": {
    "blackmatrix7": {"name":"blackmatrix7/ios_rule_script","homepage":"https://github.com/blackmatrix7/ios_rule_script",
      "urls": {   // template vars: {name}
        "clash":        "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/{name}/{name}.yaml",
        "clash_domain": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/{name}/{name}_Domain.yaml",
        "surge":        "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/{name}/{name}.list",
        "quantumultx":  "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/QuantumultX/{name}/{name}.list",
        "shadowrocket": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Shadowrocket/{name}/{name}.list",
        "loon":         "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Loon/{name}/{name}.list",
        "singbox":      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/sing-box/{name}/{name}.srs"
      }},
    "loyalsoldier": {"name":"Loyalsoldier/clash-rules", "urls": {"clash":"https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/{name}.txt"}},
    "metacubex":   {"name":"MetaCubeX/meta-rules-dat", "urls": {"clash_geosite":"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/{name}.mrs","clash_geoip":".../geo/geoip/{name}.mrs","singbox_geosite":"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/{name}.srs","singbox_geoip":".../geo/geoip/{name}.srs"}},
    "acl4ssr":     {"name":"ACL4SSR/ACL4SSR", "urls": {"clash":"https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/{name}.list","surge":"https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/{name}.list"}},
    "sagernet":    {"name":"SagerNet/sing-geosite", "urls": {"singbox_geosite":"https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-{name}.srs","singbox_geoip":"https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-{name}.srs"}}
  },
  "categories": [
    {"id":"ads","name":{"zh-CN":"广告拦截","en":"Ad blocking"},"default_target":"REJECT","order":10,
     "rules":{"blackmatrix7":{"name":"Advertising","behavior":"classical"},"loyalsoldier":{"name":"reject","behavior":"domain"},"acl4ssr":{"name":"BanAD","behavior":"classical"}}},
    {"id":"lan","name":{"zh-CN":"局域网","en":"LAN"},"default_target":"DIRECT","order":20,
     "rules":{"blackmatrix7":{"name":"Lan","behavior":"classical"},"loyalsoldier":{"name":"private","behavior":"domain"},"metacubex":{"geosite":"private","geoip":"private"},"sagernet":{"geosite":"private","geoip":"private"}}},
    {"id":"openai","name":{"zh-CN":"OpenAI / ChatGPT","en":"OpenAI"},"default_target":"AI","order":30,
     "rules":{"blackmatrix7":{"name":"OpenAI","behavior":"classical"},"metacubex":{"geosite":"openai"},"sagernet":{"geosite":"openai"}}},
    {"id":"claude","name":{"zh-CN":"Claude / Anthropic","en":"Claude"},"default_target":"AI","order":31,"rules":{"blackmatrix7":{"name":"Claude"},"metacubex":{"geosite":"anthropic"}}},
    {"id":"gemini","name":{"zh-CN":"Gemini / Google AI","en":"Gemini"},"default_target":"AI","order":32,"rules":{"blackmatrix7":{"name":"Gemini"},"metacubex":{"geosite":"google-gemini"}}},
    {"id":"telegram","default_target":"Telegram","order":40,"rules":{"blackmatrix7":{"name":"Telegram"},"loyalsoldier":{"name":"telegramcidr","behavior":"ipcidr"},"metacubex":{"geosite":"telegram","geoip":"telegram"},"sagernet":{"geosite":"telegram","geoip":"telegram"}}},
    {"id":"youtube","default_target":"Streaming","order":50,"rules":{"blackmatrix7":{"name":"YouTube"},"metacubex":{"geosite":"youtube"}}},
    {"id":"netflix","default_target":"Streaming","order":51,"rules":{"blackmatrix7":{"name":"Netflix"},"metacubex":{"geosite":"netflix","geoip":"netflix"}}},
    {"id":"disney","default_target":"Streaming","order":52,"rules":{"blackmatrix7":{"name":"Disney"},"metacubex":{"geosite":"disney"}}},
    {"id":"spotify","default_target":"Streaming","order":53,"rules":{"blackmatrix7":{"name":"Spotify"},"metacubex":{"geosite":"spotify"}}},
    {"id":"tiktok","default_target":"Streaming","order":54,"rules":{"blackmatrix7":{"name":"TikTok"},"metacubex":{"geosite":"tiktok"}}},
    {"id":"github","default_target":"Proxy","order":60,"rules":{"blackmatrix7":{"name":"GitHub"},"metacubex":{"geosite":"github"}}},
    {"id":"google","default_target":"Proxy","order":61,"rules":{"blackmatrix7":{"name":"Google"},"loyalsoldier":{"name":"google","behavior":"domain"},"metacubex":{"geosite":"google"}}},
    {"id":"microsoft","default_target":"Microsoft","order":62,"rules":{"blackmatrix7":{"name":"Microsoft"},"metacubex":{"geosite":"microsoft"}}},
    {"id":"apple","default_target":"Apple","order":63,"rules":{"blackmatrix7":{"name":"Apple"},"loyalsoldier":{"name":"apple","behavior":"domain"},"metacubex":{"geosite":"apple"}}},
    {"id":"twitter","default_target":"Proxy","order":64,"rules":{"blackmatrix7":{"name":"Twitter"},"metacubex":{"geosite":"twitter"}}},
    {"id":"steam","default_target":"Games","order":65,"rules":{"blackmatrix7":{"name":"Steam"},"metacubex":{"geosite":"steam"}}},
    {"id":"bilibili","default_target":"DIRECT","order":70,"rules":{"blackmatrix7":{"name":"BiliBili"},"metacubex":{"geosite":"bilibili"}}},
    {"id":"gfw","name":{"zh-CN":"被墙域名","en":"GFW list"},"default_target":"Proxy","order":80,"rules":{"loyalsoldier":{"name":"gfw","behavior":"domain"},"metacubex":{"geosite":"gfw"},"sagernet":{"geosite":"gfw"},"blackmatrix7":{"name":"Global"}}},
    {"id":"cn","name":{"zh-CN":"中国大陆","en":"China"},"default_target":"DIRECT","order":90,"rules":{"blackmatrix7":{"name":"ChinaMax"},"loyalsoldier":{"name":"direct","behavior":"domain"},"metacubex":{"geosite":"cn","geoip":"cn"},"sagernet":{"geosite":"cn","geoip":"cn"},"acl4ssr":{"name":"ChinaDomain"}}}
  ],
  "templates": [
    {"id":"none","name":{"zh-CN":"无规则（全局代理）","en":"None (global)"},"categories":{},"groups":[{"name":"Proxy","type":"select","include":"auto, *"},{"name":"auto","type":"url-test","include":"*"}],"final":"Proxy","geoip_cn_direct":false},
    {"id":"minimal","name":{"zh-CN":"精简（国内直连）","en":"Minimal"},"categories":{"lan":"DIRECT","cn":"DIRECT"},"groups":[...Proxy, auto],"final":"Proxy","geoip_cn_direct":true},
    {"id":"standard","name":{"zh-CN":"标准分流","en":"Standard"},"categories":{"ads":"REJECT","lan":"DIRECT","openai":"AI","claude":"AI","gemini":"AI","telegram":"Telegram","youtube":"Streaming","netflix":"Streaming","disney":"Streaming","spotify":"Streaming","microsoft":"Microsoft","apple":"Apple","gfw":"Proxy","cn":"DIRECT"},"groups":[Proxy, AI, Streaming, Telegram, Microsoft(select: DIRECT, Proxy), Apple(select: DIRECT, Proxy), auto],"final":"Proxy","geoip_cn_direct":true},
    {"id":"full","name":{"zh-CN":"精细分流","en":"Full"},"categories":"all categories enabled with their default_target","groups":[... + Games, Google, GitHub],"final":"Proxy","geoip_cn_direct":true}
  ]
}
```
Rule emission per format (docs/RULESETS.md is authoritative for source/key selection):
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
`mirror` prefixes every GitHub URL (`<mirror>https://raw.githubusercontent.com/...`); URLs on other hosts (admin sources) are left alone.
