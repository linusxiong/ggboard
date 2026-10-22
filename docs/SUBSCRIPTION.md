# Subscription generation & rule sets

Package `internal/subscription`. Input: user, available nodes (by group, sorted), options. Output: body + headers.

## Client detection (`flag` query or User-Agent, case-insensitive, first match wins)
| flag contains | format |
|---|---|
| `clash.meta`, `mihomo`, `clashmeta`, `clash-verge`, `clash verge`, `verge`, `flclash`, `clash nyanpasu`, `clashx meta` | `clash-meta` (mihomo YAML: full feature set incl. hysteria2, tuic, anytls, vless-reality, mieru) |
| `stash` | `stash` (mihomo YAML variant; no mieru/anytls) |
| `clash` (plain), `clashforandroid`, `clashx` | `clash` (classic YAML; only ss/vmess/trojan/ssr) |
| `sing-box`, `singbox`, `sfa`, `sfi`, `sfm`, `nekobox`, `hiddify` | `singbox` (JSON, 1.11+ schema with `rule_set`) |
| `surge` | `surge` (conf; ss/vmess/trojan/hysteria2/tuic/vless (xtls-vision not supported) |
| `surfboard` | `surfboard` (surge subset: ss/vmess/trojan) |
| `quantumult x`, `quantumult%20x`, `quanx` | `quantumultx` (conf lines; ss/ssr/vmess/trojan/vless) |
| `shadowrocket` | `shadowrocket` (base64 share links + `REMARKS`/`STATUS`; supports all) |
| `loon` | `loon` (conf; ss/ssr/vmess/vless/trojan/hysteria2/tuic/anytls? -> ss/ssr/vmess/vless/trojan/hysteria2/tuic) |
| `v2rayn`, `v2rayng`, `nekoray`, `passwall`, `ssrplus`, `sagernet`, `karing`, `v2box`, `shadowsocks` | `v2ray` (base64 of share links: vmess:// vless:// ss:// ssr:// trojan:// hysteria2:// hysteria:// anytls:// tuic:// mieru://) |
| default | `v2ray` |
`flag` may also name a format directly: `clash`, `clash-meta`, `singbox`, `surge`, `quantumultx`, `shadowrocket`, `loon`, `v2ray`, `ssr`.

Every format implements:
```go
type Format interface {
    Name() string
    Supports(nodeType string) bool
    Render(ctx *RenderContext) (body []byte, contentType string, err error)
}
type RenderContext struct{ User *model.User; Nodes []*model.Node; Profile RuleProfile; Catalog *Catalog; Settings ...; SubscribeURL string; AppName string }
```
Formats self-register in `init()` via `subscription.RegisterFormat`.

Share-link conventions (v2ray/shadowrocket): vmess = base64 JSON `{v:"2",ps,add,port,id,aid:0,net,type,host,path,tls,sni,alpn,fp}`;
vless `vless://uuid@host:port?encryption=none&security=tls|reality&sni=&fp=&pbk=&sid=&type=ws|grpc|...&path=&host=&serviceName=&flow=#name`;
ss `ss://base64(method:password)@host:port#name` (password = `model.UserCredentials(u, node).Password`, i.e. `<server_key>:<user_key>` for 2022 ciphers, see docs/NODE_PROTOCOLS.md §4); ssr `ssr://base64(host:port:protocol:method:obfs:base64(pass)/?obfsparam=&protoparam=&remarks=)`;
trojan `trojan://pass@host:port?security=tls&sni=&type=&path=&host=&allowInsecure=1#name`;
hysteria2 `hysteria2://pass@host:port?sni=&insecure=1&obfs=salamander&obfs-password=&mport=10000-20000#name`;
hysteria (v1) `hysteria://host:port?auth=&upmbps=&downmbps=&obfsParam=&peer=&insecure=&alpn=#name`;
tuic `tuic://uuid:pass@host:port?sni=&alpn=h3&congestion_control=bbr&udp_relay_mode=native&allow_insecure=1#name`;
anytls `anytls://pass@host:port?sni=&insecure=1#name`; mieru `mieru://username:password@host:port?transport=TCP&multiplexing=MULTIPLEXING_LOW#name`.

## Response headers
- `subscription-userinfo: upload=<u>; download=<d>; total=<transfer_enable>; expire=<unix|0>`
- `profile-update-interval: <subscribe_update_interval>`
- `content-disposition: attachment; filename*=UTF-8''<app_name>` (filename ASCII-safe fallback)
- `profile-web-page-url: <app_url>`
- `ETag` (sha1 of body), honor `If-None-Match` → 304.
- `Cache-Control: no-store`.

## Info nodes
When `show_info_to_server_enable`, prepend pseudo nodes named `剩余流量：X GB` / `套餐到期：YYYY-MM-DD` (or "长期有效")
(and `距离下次重置剩余 N 天` when applicable) pointing to `127.0.0.1:1080` shadowsocks (aes-128-gcm). Unavailable users get only these.
Names are localized by `default_locale`.

## Filtering query params
`types=vmess,vless,...` (default all); `filter=HK|JP` (name/tag substring match); `emoji=0` (strip emoji from names).

## Rule profiles (the user's choice)
```jsonc
RuleProfile {
  "template": "standard",            // id from catalog.templates; "none" = no rules (proxy everything, clients' default)
  "source": "blackmatrix7",          // preferred rule-set source id; categories fall back to other sources that carry them
  "categories": {                    // category id -> target policy group name; absent/false = disabled
    "ads": "REJECT", "cn": "DIRECT", "lan": "DIRECT", "openai": "AI", "claude": "AI", "telegram": "Telegram", "netflix": "Streaming"
  },
  "groups": [                        // policy groups shown in the client, in order; "Proxy" is always present
    {"name": "Proxy",     "type": "select",   "include": "auto, *"},   // "include": regex/glob on node names, "*" = all
    {"name": "AI",        "type": "select",   "include": "Proxy, US|SG|JP"},
    {"name": "Streaming", "type": "select",   "include": "Proxy, *"},
    {"name": "Telegram",  "type": "select",   "include": "Proxy, *"},
    {"name": "auto",      "type": "url-test", "include": "*", "url": "http://www.gstatic.com/generate_204", "interval": 300}
  ],
  "final": "Proxy",                  // policy for unmatched traffic
  "geoip_cn_direct": true,           // append GEOIP,CN,DIRECT (uses mirror where applicable)
  "mirror": "",                      // "" = direct GitHub raw; "default" = settings.ruleset_mirror; or a custom prefix
  "dns": "default"                   // "default" | "fakeip" | "none" (format-specific DNS block)
}
```
Stored in `rule_profiles.data`; defaults come from the chosen template. Query params override per request:
`profile=<template>`, `source=`, `cats=openai,telegram` (enable only these, using template targets), `final=`, `mirror=1|0`.
Admin can disable user customization (`allow_user_rule_profile=false`) → everyone gets `ruleset_default_template`.

## Catalog (`internal/subscription/rulesets/catalog.json`, embedded; admin overrides merged from setting `rulesets_custom`)
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
Rule emission per format:
- **clash-meta / stash**: `rule-providers` (`type: http`, `behavior`, `format: yaml|text|mrs`, `interval: 86400`, `path: ./rules/<id>.<ext>`) + `rules: RULE-SET,<id>,<target>`; add `GEOIP,CN,DIRECT`(if enabled) + `MATCH,<final>`.
  Prefer `metacubex` `.mrs` domain/ipcidr sets when the category has them (fastest), else the chosen source.
- **clash (classic)**: no rule-providers → inline `GEOIP,CN,DIRECT`, `MATCH,<final>` + `DOMAIN-SUFFIX` lists fetched? No: classic gets only `GEOIP,CN,DIRECT` + `MATCH` (documented limitation).
- **singbox**: `route.rule_set[]` remote (`type: remote`, `format: binary|source`, `url`, `download_detour: Proxy`, `update_interval: 24h`) + `route.rules[]` `{rule_set: [...], outbound: target}`; `final`; DNS block per `dns` option.
- **surge / loon / shadowrocket / quantumultx**: `RULE-SET,<url>,<target>` (surge/loon/shadowrocket) or `[filter_remote]` lines with `tag` (quanx); `FINAL,<final>`; `GEOIP,CN,DIRECT`.
- **v2ray base64**: no rules (clients don't support).
`mirror` prefixes every raw URL (`<mirror>https://raw.githubusercontent.com/...`).
