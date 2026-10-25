# Rule-set catalog

`internal/subscription/rulesets/catalog.json` is the embedded, machine-readable list of open-source
rule sets that users can combine in their subscription (see `docs/SUBSCRIPTION.md` → *Rule profiles*).
It lists:

- **sources**: GitHub repos and their raw-URL templates, one template per *format key*;
- **categories**: things a user can route, such as `openai` or `cn`, with the exact file name(s) each source publishes;
- **templates**: the `none`, `minimal`, `standard` and `full` presets;
- **mirrors**: GitHub raw proxy prefixes.

ggboard never vendors rule data. It only emits URLs, and clients download the files themselves.
Admin overrides (setting `rulesets_custom`, edited through `GET`/`PUT /api/v1/admin/rulesets`) use the same schema and
merge by id; see [Admin overrides](#admin-overrides).

Every URL in the file was verified on 2026-10-21 (see [Verification log](#verification-log)).

---

## Schema

Top-level keys, always in this order:

```jsonc
{
  "sources":    { "<source id>": Source },   // object; fallback priority comes from Source.order, not key order
  "categories": [ Category ],                // sorted by Category.order
  "templates":  [ Template ],                // built-in: none, minimal, standard, full
  "mirrors":    [ Mirror ],                  // first entry is always {"id":"direct","prefix":""}
  "meta":       { "generated_at": "<RFC3339>", "version": 1 }   // version = schema version
}
```

### Source

| field | type | meaning |
| --- | --- | --- |
| `name` | string | Upstream repo name for display, e.g. `blackmatrix7/ios_rule_script` |
| `homepage` | url | Repo page |
| `license` | string | License note (SPDX id + remark). All sources are referenced by URL only. |
| `order` | int | Fallback priority. Lower values are tried first when the preferred source lacks a category. |
| `description` | `{zh-CN, en}` | One line for the UI |
| `urls` | `{format key: template}` | Raw URL template. `{name}` is the only placeholder. It is not URL-escaped, because names may contain `!` (`geolocation-!cn`) or `/` (`Ruleset/OpenAi`), and both are valid in a path. |
| `url_meta` | `{format key: {field, behavior, format}}` | Has the same keys as `urls`. `field` names the RuleRef field that fills `{name}`: `name`, `geosite` or `geoip`. `behavior` is `domain`, `ipcidr` or `classical`. `format` is `yaml`, `text` or `mrs` for mihomo, `binary` or `source` for sing-box, and `text` for Surge-family lists. |
| `aliases` | `{old key: key}` (optional) | Key names from the original design sketch (`clash_geosite`, `clash_geoip`, `singbox_geosite`, `singbox_geoip`; metacubex and sagernet only). Resolve `urls[aliases[k]]` when `urls[k]` is missing. |

Source ids: `blackmatrix7` (order 10), `metacubex` (20), `loyalsoldier` (30), `sagernet` (40), `acl4ssr` (50), `dustinwin` (60).

### Format keys

| key | consumed by | behavior | format | sources |
| --- | --- | --- | --- | --- |
| `clash` | clash-meta, stash | classical | yaml (bm7, Loyalsoldier), text (ACL4SSR) | blackmatrix7, loyalsoldier, acl4ssr |
| `clash_classical` | clash-meta, stash (standalone alternative) | classical | yaml | blackmatrix7 (split lists only) |
| `clash_domain` | clash-meta, stash | domain | yaml / text | blackmatrix7 (split), loyalsoldier, metacubex, dustinwin |
| `clash_ipcidr` | clash-meta, stash | ipcidr | yaml / text | blackmatrix7 (split), loyalsoldier, metacubex, dustinwin |
| `clash_mrs_domain` | clash-meta only | domain | mrs | metacubex, dustinwin |
| `clash_mrs_ipcidr` | clash-meta only | ipcidr | mrs | metacubex, dustinwin |
| `singbox` | singbox | domain | binary (`.srs`) | metacubex, sagernet, dustinwin |
| `singbox_ipcidr` | singbox | ipcidr | binary | metacubex, sagernet, dustinwin |
| `singbox_source` | singbox | domain | source (`.json`) | metacubex, dustinwin |
| `singbox_source_ipcidr` | singbox | ipcidr | source | metacubex, dustinwin |
| `surge` | surge, surfboard | classical | text (`RULE-SET`) | blackmatrix7, acl4ssr |
| `surge_domain` | surge, surfboard | domain | text (`DOMAIN-SET`) | blackmatrix7 (split) |
| `quantumultx` | quantumultx | classical | text | blackmatrix7 |
| `shadowrocket` | shadowrocket | classical | text (`RULE-SET`) | blackmatrix7, acl4ssr |
| `shadowrocket_domain` | shadowrocket | domain | text (`DOMAIN-SET`) | blackmatrix7 (split) |
| `loon` | loon | classical | text | blackmatrix7, acl4ssr |
| `loon_domain` | loon | domain | text (domain set) | blackmatrix7 (split) |

A key's *family* is its client prefix. For example, the `surge` family is `surge` plus `surge_domain`.
For sing-box keys, `behavior` is informational: a `.srs` describes itself. It does tell the domain set from the IP set.

### Category

```jsonc
{ "id": "cn", "name": {"zh-CN": "中国大陆", "en": "Mainland China"}, "default_target": "DIRECT", "order": 90,
  "rules": { "<source id>": RuleRef | [RuleRef, ...] } }
```

- `id` is stored in users' `rule_profiles`, so it must never be renamed.
- `default_target` is a group name from the `full` template, or `DIRECT` or `REJECT`.
- `order` sets the rule emission order, ascending. More specific categories come before broader ones
  (gemini < google, copilot < microsoft, icloud < apple, youtube < google). `gfw`, `global` and `cn` come last.
- A source appears in `rules` only if it publishes the category. A value can be an array when a category needs
  several files from one source, e.g. Loyalsoldier `cn` = `direct` (domain) + `cncidr` (ipcidr).

### RuleRef

| field | meaning |
| --- | --- |
| `name` | File name for name-based sources (blackmatrix7, loyalsoldier, acl4ssr). It may include a sub-path (ACL4SSR `Ruleset/OpenAi`). |
| `behavior` | Behavior of the `name` file. It is informational for bm7 and ACL4SSR (`classical`), and authoritative for Loyalsoldier, where `clash`, `clash_domain` and `clash_ipcidr` share one template. The lint enforces that `behavior` matches the key. |
| `geosite` / `geoip` | Domain-set and IP-set names for geo-style sources (metacubex, sagernet, dustinwin). One RuleRef can carry both. |
| `split` | Set only on blackmatrix7 refs. It is true for the 6 large lists that ship `_Classical` / `_Domain` / `_IP` variants (`AdvertisingLite`, `Netflix`, `Apple`, `Proxy`, `Global`, `ChinaMax`). For these, `{name}.yaml` / `{name}.list` holds only the remainder, so never emit it without its `_domain`/`_ipcidr` siblings (or use `clash_classical` alone). |
| `formats` | The format keys verified to exist for this ref. Consumers must use only these keys. |

URL expansion: `url = sources[s].urls[k].replace("{name}", ref[sources[s].url_meta[k].field])`, then the mirror prefix is applied.

### Template and group

```jsonc
{ "id": "standard", "name": {...}, "description": {...},
  "categories": { "<category id>": "<target>" },          // explicit map; absent = disabled
  "groups": [ {"name":"Proxy","type":"select","include":"auto, *"},
              {"name":"auto","type":"url-test","include":"*","url":"http://www.gstatic.com/generate_204","interval":300} ],
  "final": "Proxy", "geoip_cn_direct": true }
```

- `type` is one of `select`, `url-test`, `fallback`, `load-balance`. The last three require `url` and `interval` (seconds).
- `include` is a comma-separated token list, with spaces trimmed and order kept. Each token is one of:
  - a group name from the same template,
  - `DIRECT` or `REJECT`,
  - `*` for all nodes,
  - a Go RE2 regular expression, unanchored, matched against node names.

  Tokens that match nothing are dropped. A group left with no members gets `Proxy`; the `Proxy` group itself gets `DIRECT`.
- `Proxy` is always present.
- Every category target must be a group, `DIRECT` or `REJECT`. When a user enables a category whose target
  group the template lacks (e.g. `steam` → `Games` under `standard`), the renderer routes it to `final`.

| template | categories | groups | final | geoip_cn_direct |
| --- | --- | --- | --- | --- |
| `none` | none | Proxy, auto | Proxy | false |
| `minimal` | lan, cn → DIRECT | Proxy, auto | Proxy | true |
| `standard` | ads, lan, openai, claude, gemini, telegram, youtube, netflix, disney, spotify, microsoft, apple, gfw, cn | Proxy, AI, Streaming, Telegram, Microsoft, Apple, auto | Proxy | true |
| `full` | all 32, each with its `default_target` | Proxy, AI, Streaming, Telegram, Microsoft, Apple, Games, Google, GitHub, auto | Proxy | true |

### Mirror

`{ "id": "ghfast", "name": {"zh-CN": "...", "en": "..."}, "prefix": "https://ghfast.top/" }`. `name` is optional
(admin-defined mirrors may omit it; the UI then shows the id). `prefix` is `""` (direct) or an http(s) URL ending in `/`
without `?` or `#`. The prefix is prepended to the full raw URL (`<prefix>https://raw.githubusercontent.com/...`); only
GitHub URLs (`raw.githubusercontent.com`, `github.com`, `gist.githubusercontent.com`) are prefixed.

`RuleProfile.mirror` accepts `""` (direct), `"default"` (= setting `ruleset_mirror`, which may itself be a prefix or a
mirror id; empty = direct), a catalog mirror id, or a custom prefix. The UI offers this list.

### Changes from the original design sketch

The first contract draft sketched the catalog inline in `docs/SUBSCRIPTION.md` (since removed). All changes below are
backward compatible: sketch field names and shapes still parse.

1. Sources gained `license`, `order`, `description`, `url_meta` and `aliases`. `urls` stays a map of string templates.
2. Format keys now follow the vocabulary above. The sketch's metacubex/sagernet keys (`clash_geosite`,
   `clash_geoip`, `singbox_geosite`, `singbox_geoip`) remain available through `aliases`.
3. RuleRef gained `formats` (required) and `split`. A `rules[source]` value may be an array.
4. A new top-level `mirrors` key was added. `meta` is `{generated_at, version}`.
5. Corrections made after checking the repo trees:
   - blackmatrix7 publishes **no sing-box files** on either branch, so the sketch's `singbox` template was dropped.
   - ACL4SSR has **no root-level `{name}.list`** (the sketch's Surge template returns 404). The lists live in `Clash/` and `Clash/Ruleset/`.
   - For bm7's large lists, `{name}.yaml` is only the remainder (`split`).
   - Loyalsoldier `.txt` files are **YAML payloads** (`format: yaml`).
   - SagerNet `sing-geoip` has **only country codes** (no `private` or `telegram`), and `sing-geosite` has **no `gfw`** and no `.json`.
   - bm7 `Global` is "all foreign sites" (35k rules), not a GFW list. `gfw` now uses bm7 `Proxy` (7k rules), and `Global` moved to the new `global` category.
   - `ads` uses bm7 `AdvertisingLite` (38k rules) instead of `Advertising` (292k), which is risky on iOS network extensions.
6. `default_target` for `github` and `google` is `GitHub` and `Google`, to match the groups the `full` template adds.
   New categories: `applications`, `ai`, `copilot`, `icloud`, `global`, `hbo`, `twitch`, `bahamut`, `facebook`, `paypal`, `speedtest`, `epic`.

---

## How each format consumes the catalog

**Which profile.** A user has up to 10 named rule profiles (docs/SUBSCRIPTION.md → *Multiple profiles*).
`engine.EffectiveProfile` picks the one a request uses from `?profile=`:

1. a **slug** of one of the user's profiles selects that profile;
2. otherwise a catalog **template id** keeps the original meaning of `?profile=`: the default profile with that template's
   categories, groups and `final` (its stored source, mirror, DNS and node operations are kept);
3. anything else (or no parameter) selects the **default profile**, never an error.

With `allow_user_rule_profile=false` everyone gets `ruleset_default_template`, whatever is stored or requested. The
profile's node operations (`nodes`: filters, rename, emoji, sort, limit, with the region table in docs/SUBSCRIPTION.md →
*Node operations*) shape the proxy list before groups and rules are resolved; they never change which rule sets are used.
`source`, `cats`, `final` and `mirror` then override the selected profile for that request.

1. **Enabled categories** come from the profile's `categories`, or the template's when the profile has none.
   `cats=` narrows them. Sort them by category `order`, never by map order. Resolve each target (see the template rules).
2. **Choose a source per category.** Candidates are tried in this order:
   - the format's preferred source: `metacubex` for clash-meta, as required by SUBSCRIPTION.md;
   - `profile.source`;
   - every other source by `order`.

   The first source that has a RuleRef with at least one key of the client's family wins.
   For example, a sing-box user who chose `blackmatrix7` falls back to `metacubex`.
   So the chosen source never costs a client rules: whatever the source, each client gets every enabled category that
   some source publishes for it (tested for every source × client). blackmatrix7, Loyalsoldier and ACL4SSR have no
   sing-box files, so with any of them sing-box gets the same MetaCubeX sets as a profile that chose `metacubex`.
3. **Pick keys per RuleRef**, in array order, from its `formats`:

   | client format | keys used |
   | --- | --- |
   | clash-meta | If any `clash_mrs_*` key is present, use only those. Otherwise use every present key of `clash`, `clash_domain`, `clash_ipcidr`. Never use `clash_classical` together with them. |
   | stash | Every present key of `clash`, `clash_domain`, `clash_ipcidr` (no mrs). |
   | singbox | `singbox` and `singbox_ipcidr` when present, otherwise `singbox_source` and `singbox_source_ipcidr`. |
   | surge, surfboard | `surge` (`RULE-SET`) and `surge_domain` (`DOMAIN-SET`) |
   | shadowrocket | `shadowrocket` (`RULE-SET`) and `shadowrocket_domain` (`DOMAIN-SET`) |
   | loon | `loon` and `loon_domain` (bm7 says use them together) |
   | quantumultx | `quantumultx` |
   | clash (classic), v2ray, ssr | The catalog is not used. Classic Clash gets only `GEOIP,CN,DIRECT` and `MATCH`. |

   The `shadowrocket` keys stay in the catalog, but the shadowrocket format is links-only (Shadowrocket users configure rules in the
   app), so it consumes no rule sets. The implementation is `rulesets.BuildPlan` (`internal/subscription/rulesets/plan.go`); the
   public catalog (`GET /guest/rulesets/catalog`) lists per category and source which formats can use them
   (`source_clients`) and the per-format preferred source (`preferred_sources`), so the rule builder shows which source
   each client actually gets.

4. **Name each rule set** deterministically as `<category>[-<n>]-<domain|ip|classical>`.
   `n` (starting at 1) is added only when the RuleRef value is an array. The suffix comes from the key's behavior.
   For mihomo, the `path` is `./rules/<id>.<ext of url>`.
5. **Emit rules.**
   - Within a category, emit domain and classical sets before IP sets.
   - Append `GEOIP,CN,DIRECT` when `geoip_cn_direct` is set, then the final rule.
   - Recommended: give IP sets `,no-resolve` except in `lan` and `cn`, so domain traffic does not trigger DNS lookups.

clash-meta example (`cn` → metacubex):

```yaml
rule-providers:
  cn-domain: {type: http, behavior: domain, format: mrs, interval: 86400, path: ./rules/cn-domain.mrs,
              url: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/cn.mrs"}
  cn-ip:     {type: http, behavior: ipcidr, format: mrs, interval: 86400, path: ./rules/cn-ip.mrs,
              url: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/cn.mrs"}
rules: [RULE-SET,cn-domain,DIRECT, RULE-SET,cn-ip,DIRECT, GEOIP,CN,DIRECT, MATCH,Proxy]
```

Other formats:

- **sing-box** (schema of sing-box ≥ 1.12: typed DNS servers, rule actions):
  - Each key becomes a `route.rule_set` entry `{"type":"remote","tag":"cn-domain","format":"binary","url":…,"update_interval":"24h"}`.
  - The download route depends on the sing-box version in the User-Agent (details in docs/SUBSCRIPTION.md, Rule emission per format):
    sing-box ≥ 1.14 gets top-level `"http_clients":[{"tag":"rule-set","detour":"Proxy"}]` + `"route":{"default_http_client":"rule-set"}`
    (no `detour` when a mirror is set: a detour to the empty `direct` outbound is rejected); older or unnamed versions get
    `"download_detour":"Proxy"` on every entry, `"direct"` when a mirror is set (deprecated in 1.14, removed in 1.16).
  - Each category gets a rule `{"rule_set":["cn-domain","cn-ip"],"outbound":…}`.
  - sing-box ≥ 1.12 has no GeoIP database, so `geoip_cn_direct` adds the metacubex `geoip/cn.srs` rule set with outbound `direct`.
- **Surge, Surfboard**: `RULE-SET,<url>,<target>`, `DOMAIN-SET,<url>,<target>`, `GEOIP,CN,DIRECT`, `FINAL,<final>,dns-failed`.
- **Loon**: `[Remote Rule]` `<url>, policy=<target>, tag=<id>, enabled=true`.
- **Quantumult X**: `[filter_remote]` `<url>, tag=<id>, force-policy=<target>, update-interval=86400, opt-parser=false, enabled=true`.

### Coverage (category × source, file names as published)

`ip:` marks the geoip / IP-set name.

| category | target | blackmatrix7 | metacubex | loyalsoldier | sagernet | acl4ssr | dustinwin |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ads` | REJECT | `AdvertisingLite` | `category-ads-all` | `reject` | `category-ads-all` | `BanAD`, `BanProgramAD` | `ads` |
| `lan` | DIRECT | `Lan` | `private` / ip:`private` | `private`, `lancidr` | `private` | `LocalAreaNetwork` | `private` / ip:`privateip` |
| `applications` | DIRECT | — | — | `applications` | — | — | — |
| `openai` | AI | `OpenAI` | `openai` | — | `openai` | `Ruleset/OpenAi` | — |
| `claude` | AI | `Claude` | `anthropic` | — | `anthropic` | `Ruleset/Claude` | — |
| `gemini` | AI | `Gemini` | `google-gemini` | — | `google-gemini` | `Ruleset/Gemini` | — |
| `copilot` | AI | `Copilot` | `bing`, `github-copilot` | — | `bing`, `github-copilot` | — | — |
| `ai` | AI | — | `category-ai-!cn` | — | `category-ai-!cn` | `Ruleset/AI` | `ai` |
| `telegram` | Telegram | `Telegram` | `telegram` / ip:`telegram` | `telegramcidr` | `telegram` | `Telegram` | ip:`telegramip` |
| `youtube` | Streaming | `YouTube` | `youtube` | — | `youtube` | `Ruleset/YouTube` | `youtube` |
| `netflix` | Streaming | `Netflix` | `netflix` / ip:`netflix` | — | `netflix` | `Ruleset/Netflix`, `Ruleset/NetflixIP` | `netflix` / ip:`netflixip` |
| `disney` | Streaming | `Disney` | `disney` | — | `disney` | `Ruleset/DisneyPlus` | `disney` |
| `hbo` | Streaming | `HBO` | `hbo` | — | `hbo` | `Ruleset/HBO` | `max` |
| `spotify` | Streaming | `Spotify` | `spotify` | — | `spotify` | `Ruleset/Spotify` | `spotify` |
| `tiktok` | Streaming | `TikTok` | `tiktok` | — | `tiktok` | `Ruleset/TikTok` | `tiktok` |
| `twitch` | Streaming | `Twitch` | `twitch` | — | `twitch` | `Ruleset/Twitch` | — |
| `bahamut` | Streaming | `Bahamut` | `bahamut` | — | `bahamut` | `Ruleset/Bahamut` | — |
| `bilibili` | DIRECT | `BiliBili` | `bilibili` | — | `bilibili` | `Ruleset/Bilibili` | `bilibili` |
| `github` | GitHub | `GitHub` | `github` | — | `github` | `Ruleset/Github` | — |
| `google` | Google | `Google` | `google` / ip:`google` | `google` | `google` | `Ruleset/Google` | — |
| `microsoft` | Microsoft | `Microsoft` | `microsoft` | — | `microsoft` | `Microsoft` | — |
| `icloud` | Apple | `iCloud` | `icloud` | `icloud` | `icloud` | — | — |
| `apple` | Apple | `Apple` | `apple` | `apple` | `apple` | `Apple` | — |
| `twitter` | Proxy | `Twitter` | `twitter` / ip:`twitter` | — | `twitter` | `Ruleset/Twitter` | — |
| `facebook` | Proxy | `Facebook`, `Instagram`, `Whatsapp` | `facebook` / ip:`facebook`, `instagram`, `whatsapp` | — | `facebook`, `instagram`, `whatsapp` | `Ruleset/Facebook`, `Ruleset/Instagram`, `Ruleset/Whatsapp` | — |
| `paypal` | Proxy | `PayPal` | `paypal` | — | `paypal` | — | — |
| `speedtest` | Proxy | `Speedtest` | `category-speedtest` | — | `category-speedtest` | — | `networktest` |
| `steam` | Games | `Steam` | `steam` | — | `steam` | `Ruleset/Steam` | — |
| `epic` | Games | `Epic` | `epicgames` | — | `epicgames` | `Ruleset/Epic` | — |
| `gfw` | Proxy | `Proxy` | `gfw` | `gfw` | — | `ProxyGFWlist` | `gfw` |
| `global` | Proxy | `Global` | `geolocation-!cn` | `proxy`, `tld-not-cn` | `geolocation-!cn` | — | `proxy`, `tld-proxy` |
| `cn` | DIRECT | `ChinaMax` | `cn` / ip:`cn` | `direct`, `cncidr` | `cn` / ip:`cn` | `ChinaDomain`, `ChinaIp` | `cn` / ip:`cnip` |

Coverage per client (any source):

| client | categories | missing |
| --- | --- | --- |
| clash-meta, stash | 32/32 | — |
| singbox, surge, surfboard, loon (and the unused shadowrocket keys) | 31/32 | `applications`. Loyalsoldier publishes PROCESS-NAME rules only for Clash. |
| quantumultx | 30/32 | `applications`, `ai`. bm7 has no aggregate AI list. |

---

## Mirrors

| id | prefix | status 2026-10-21 |
| --- | --- | --- |
| `direct` | `""` | — |
| `ghfast` | `https://ghfast.top/` | 200 + content OK. Successor of `mirror.ghproxy.com`. |
| `gh-proxy` | `https://gh-proxy.com/` | 200 + content OK |
| `ghproxy-net` | `https://ghproxy.net/` | 200 + content OK. Returned 429 once under burst. |
| `gh-llkk` | `https://gh.llkk.cc/` | 200 + content OK |

Rejected candidates:

| prefix | reason |
| --- | --- |
| `mirror.ghproxy.com` | NXDOMAIN (the service moved to ghfast.top) |
| `ghproxy.cc` | expired TLS certificate |
| `github.moeyy.xyz` | NXDOMAIN |
| `gh.ddlc.top` | persistent 429 |
| `ghps.cc`, `gh.h233.eu.org` | 403 |
| `hub.gitmirror.com` | no response |
| `ghproxy.link` | rewrites `https://` to `https%3A/`, so the payload is wrong |

Public mirrors come and go. Re-verify them on every catalog refresh, and keep `direct` first.

---

## Admin overrides

- Stored in the hidden JSON setting `rulesets_custom` (group `subscribe`, default `{}`), a catalog document that may omit
  any section. `rulesets.Current` merges it over the built-in file by id: a source, category, template or mirror whose id
  exists replaces the built-in entry, a new id is appended, and categories are re-sorted by `order` (then id). The merge is
  cached until the setting changes. To stop using a built-in category, leave it out of the template or profile
  `categories` map.
- `GET /api/v1/admin/rulesets` → `{builtin, custom, effective}`; `PUT /api/v1/admin/rulesets {"custom": … }` → `true`.
  `custom: null` (or omitted) resets. Unknown JSON fields are rejected (400 on `custom`), then the **merged** catalog is
  validated (400 with field errors such as `sources.<id>.urls.<key>` or `templates[2].groups[0].include`):
  - ids (source, category, template, mirror) match `^[a-z0-9][a-z0-9_-]{0,31}$` and are unique;
  - a source has a `name` and at least one URL template; every key is a known format key and every template is an http(s)
    URL containing `{name}`; `url_meta` and `aliases` point at existing templates;
  - a category has a `default_target` and at least one source; every RuleRef lists `formats` the source has templates for,
    with the field (`name`, `geosite` or `geoip`) those templates need;
  - templates are validated like user profiles (groups, targets, `final`; see docs/SUBSCRIPTION.md);
  - a mirror `prefix` is `""` or an http(s) URL ending in `/`.
- An invalid stored value is ignored at render time (the built-in catalog is used), so subscriptions never break.
- Related settings (group `subscribe`): `allow_user_rule_profile` (default `true`; `false` forces everyone onto
  `ruleset_default_template` and makes every profile write — `PUT /user/rule-profile`, `/user/rule-profiles*` — answer
  403), `ruleset_default_template` (`standard`),
  `ruleset_default_source` (`blackmatrix7`), `ruleset_mirror` (`""`).

---

## How to add …

### A source

1. Discover exact paths without the API rate limit (60/h unauthenticated). Fetch trees only:
   `git clone --depth 1 --filter=blob:none --no-checkout --single-branch --branch <b> <repo> d && git -C d ls-tree -r --name-only HEAD`.
   Read the per-list README for "use together" notes, such as bm7's `配置建议`.
2. Add `sources.<id>` with every field. Pick a unique `order`. Use existing format keys. If you need a new key,
   add it to the format-key table, document how each client consumes it, and teach the renderers.
3. Add RuleRefs to the categories. Use `name` for file-name sources and `geosite`/`geoip` for geo databases.
   If several keys share one template, set `behavior` so the right key applies.
4. Set `formats` to every plausible key, run the verification (below), and let it prune the 404s.

### A category

1. Choose a lowercase `id`. It is permanent.
2. Fill in `name.zh-CN`, `name.en`, `default_target` and `order` (keep specific-before-broad).
3. For every source that has it, add the exact file names.
4. Add the category to the `full` template with its `default_target`. The lint enforces this.
5. If `default_target` is a new group, add that group to `full`.

### A template

1. Add an object with a unique `id`.
2. Give it an explicit `categories` map and a `groups` list that includes `Proxy`.
3. Make every target a group, `DIRECT` or `REJECT`. Give `url-test`, `fallback` and `load-balance` groups a `url` and an `interval`.
4. The lint currently pins the template list to `none`, `minimal`, `standard`, `full`. Update that check, and
   `docs/SUBSCRIPTION.md`, when adding one.

### Pitfalls found while building this catalog

- **bm7 split lists.** On the 6 large lists, `{name}.yaml` or `.list` without `_Domain`/`_IP` silently drops most rules.
- **Loyalsoldier `.txt` is YAML.** It is `format: yaml`, not `text`.
- **MetaCubeX `geo/*/classical/*.list` contains `DOMAIN-REGEX`.** It is not mapped to Surge-family clients.
- **ACL4SSR `Download.list` contains `URL-REGEX`.** It is not used. Every ACL4SSR list in the catalog uses only DOMAIN, DOMAIN-SUFFIX,
  DOMAIN-KEYWORD, IP-CIDR and IP-CIDR6, so the same file is safe for Clash, Surge, Shadowrocket and Loon.
- **ACL4SSR's own `Clash/mrs/*_domain.mrs` files are not referenced.** Their flat names would need a second placeholder,
  and metacubex already covers mrs.
- **DustinWin `applications` is not mapped.** It is PROCESS-NAME rules; Loyalsoldier covers Clash.
- **DustinWin `games`, `media` and `*-cn` sets are not mapped.** They are aggregates that do not match one category.

---

## Verification log

### 2026-10-21

Method:

- A script expands every category × source × RuleRef × format key into a URL and checks each unique URL with `HEAD`.
- When `HEAD` returns 403, 405 or 501, it falls back to a ranged `GET`.
- Redirects are followed, the timeout is 10 s, and 8 workers run at once.
- 429, 5xx and network errors are retried 3 times with backoff and then reported, never pruned. Only 404/410 count as missing.
- The script also lints the schema rules above: key order, field references, behavior consistency, the `split` flag,
  template targets and groups, the `full` template covering every category, and mirror prefixes.
- Mirrors are checked with `GET <prefix>https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/lancidr.txt`.
  The body must start with `payload:`.

| pass | combinations | unique URLs | 200 | removed (404) | errors | lint |
| --- | --- | --- | --- | --- | --- | --- |
| 1, candidate (every plausible key) | 763 | 670 | 506 | **164** | 0 | split flags only, which is expected before pruning |
| 2, final `catalog.json` | 599 | 506 | **506** | 0 | 0 | 0 errors |

- **Removed.** All 164 removed URLs were blackmatrix7 variant keys for names that do not ship variants:
  - `clash_ipcidr` ×30
  - `clash_domain` ×27
  - `surge_domain` ×27
  - `shadowrocket_domain` ×27
  - `loon_domain` ×27
  - `clash_classical` ×26

  No RuleRef, source entry or category was removed. Every base key of every source resolved.
- **Mirrors:** 4/4 kept mirrors passed the content check, and 8 candidates were rejected (see [Mirrors](#mirrors)).
  `gh-proxy.org`, `gh.idayer.com` and `cdn.`/`hk.`/`edgeone.gh-proxy.com` also worked. They were not added because
  the `gh-proxy.*` hosts are aliases of `gh-proxy.com` (and redirect to `gh-proxy.org`), and `gh.idayer.com` is a small single-operator host.
- **Snapshots checked:**
  - blackmatrix7 `master@f9e403a`
  - Loyalsoldier `release@18b7eb3`
  - MetaCubeX `meta@5b62abd` and `sing@f7c7799`
  - sing-geosite `rule-set@50375ff`
  - sing-geoip `rule-set@eb32cd6`
  - ACL4SSR `master@575637d`
  - DustinWin `mihomo-ruleset@0db4f62` and `sing-box-ruleset@c7230c5`

  The catalog points at branch heads, not SHAs, so upstream renames can break URLs. Re-run the check on every refresh.
- **Not sourced:** none. Partial gaps are listed under Coverage.

Minimal re-check, run with Python 3 `-I` from a directory outside the repo:

```python
import json, sys, urllib.request, concurrent.futures as cf
cat = json.load(open(sys.argv[1], encoding="utf-8"))
urls = sorted({s["urls"][k].replace("{name}", r[s["url_meta"][k]["field"]])
               for c in cat["categories"] for sid, v in c["rules"].items() for s in [cat["sources"][sid]]
               for r in (v if isinstance(v, list) else [v]) for k in r["formats"]})
def head(u):
    try:
        return urllib.request.urlopen(urllib.request.Request(u, method="HEAD"), timeout=10).status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception as e:
        return repr(e)
with cf.ThreadPoolExecutor(8) as ex:
    bad = [(u, s) for u, s in zip(urls, ex.map(head, urls)) if s != 200]
print(len(urls), "checked;", len(bad), "not OK"); [print(s, u) for u, s in bad]
```
