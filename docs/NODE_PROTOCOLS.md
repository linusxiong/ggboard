# Node protocols: UniProxy (XrayR / V2bX) and soga-v1

Contract for `internal/api/node/` (handlers), `internal/service/node/` (rendering, reports, validation) and
`internal/app/online/` (runtime status). The code implements this file; change both together. Every behaviour was checked
against these sources:
XrayR tag v25.3.6 (`api/newV2board`, `service/controller`), V2bX (`api/panel`, `core/{xray,sing,hy2}`, `limiter`, `node`),
Xboard fork + upstream (`Middleware/Server.php`, `UniProxyController`, `ServerService`, `Models/Server`, `DeviceStateService`,
`Helper`), and the soga-v1 webapi document plus the soga config pages. Citations in [brackets] are there for reviewers.

Conventions:

- Bodies are UTF-8 JSON, sent with `Content-Type: application/json; charset=utf-8`. Responses are compact JSON from `encoding/json`.
- "int" means a JSON number with no fraction. **Never emit `null` where this file says int/string/bool.**
- `ps` = `nodes.protocol_settings`. `ts` = `ps.tls_settings`. `uuid` = `users.uuid` (36 chars, lowercase, with hyphens).
- **P** is the *effective node* of a request: the node itself, or its parent when `parent_id` is set (§5.3). Everything is served
  from P and recorded on P.
- `push_iv` and `pull_iv` are the settings `server_push_interval` and `server_pull_interval` (seconds, default 60).

---

## 1. Overview and authentication

### 1.1 Mounts

| client | auth | paths (exact) |
| --- | --- | --- |
| XrayR (`PanelType: NewV2board`), V2bX | query `token`, `node_id`, `node_type` | `/api/v1/server/UniProxy/{config,user,push,alive,alivelist,status}`: root only (clients hard-code `ApiHost + "/api/v1/server/UniProxy/..."`) |
| soga (`type=soga-v1`) | headers `API-KEY`, `NODE-ID`, `NODE-TYPE` | `/api/v1/{node,users,audit_rules,white_list,traffic,alive_ip,audit_log,status}` **and** the same handlers under `/soga/api/v1/...` |

The settings key `server_token` is the shared secret for both clients (XrayR/V2bX `ApiKey`, soga `webapi_key`).
Node endpoints use no JWT, no CORS and no user rate limiters. Only failed token checks are limited: 30 failures per client
address (IPv6 per /64, refilled at 60/h); after that the address gets 429 (soga: `{"code":429,...}` shape) without the token
being compared, so `server_token` cannot be guessed online. Admins can only save a `server_token` of at least 16 characters.
POST bodies are capped at `httpx.NodeBodyLimit` (8 MiB, about 250k users per push). `node.Register` sets gin `engine.RemoveExtraSlash = true` as a defensive measure: then `.../soga//api/v1/node` still routes if a client joins
`webapi_url` naively.

Backend configuration snippets (use placeholders only; never commit real values):

```yaml
# XrayR config.yml  (NodeType: V2ray|Vmess|Vless|Trojan|Shadowsocks; it sends node_type=lowercase, or "vless" for V2ray+EnableVless)
PanelType: "NewV2board"
ApiConfig: { ApiHost: "https://panel.example.com", ApiKey: "<server_token>", NodeID: 3, NodeType: Vless, EnableVless: true }
```

```jsonc
// V2bX config.json Nodes[] (NodeType: vmess|vless|trojan|shadowsocks|hysteria|hysteria2|tuic|anytls; "v2ray" is sent as vmess)
{ "Core": "sing", "ApiHost": "https://panel.example.com", "ApiKey": "<server_token>", "NodeID": 3, "NodeType": "vless", "Timeout": 30 }
```

```ini
# /etc/soga/soga.conf   (server_type: v2ray|vless|trojan|ss|ssr|hysteria|anytls|mieru; node_id may be "1,2,3" if all share one type)
type=soga-v1
server_type=ss
node_id=7
soga_key=<soga license>
api=webapi
webapi_url=https://panel.example.com/soga/
webapi_key=<server_token>
```

**Recommended soga `webapi_url`: `https://<panel>/soga/`.** soga appends `api/v1/<endpoint>`. The soga licence binds to the full
`webapi_url`, and a dedicated prefix lets a reverse proxy route or limit node traffic on its own. `https://<panel>/` (root
mount) works too.

### 1.2 UniProxy authentication (every request, GET and POST)

Query parameters: `token`, `node_id`, `node_type`. XrayR and V2bX send all three on every call, in alphabetical order.
Check them in this order:

1. **token**: required. Compare it with `server_token` using `crypto/subtle.ConstantTimeCompare`. If `server_token` is empty,
   reject everything (the Xboard fork does the same).
2. **node_id**: required, a decimal uint64.
3. **node_type**: optional (upstream allows null). Normalise it: `strings.ToLower(strings.TrimSpace(v))`, then apply aliases
   `v2ray→vmess`, `ss`/`shadowsocks-plugin→shadowsocks`, `ssr→shadowsocksr` and `hysteria2→hysteria` (record that
   `hysteria2` was requested). Treat `v2node` as absent (`nodesvc.CanonicalType`, shared with soga).
   Values accepted after aliasing: `vmess vless trojan shadowsocks hysteria tuic anytls mieru`; anything else is
   400 `unsupported node_type "<v>"`. `shadowsocksr` is not a UniProxy type: a shadowsocksr node is "not found" with
   `node <id> is shadowsocksr, which UniProxy does not support`.
4. Load the node by id. If the node is missing, or `node_type` was given and `node.type` differs, the node is "not found".
   If `hysteria2` was requested and `ps.version != 2`, the node is also "not found" (Xboard has no version check; ggboard
   adds it).
5. Resolve P (§5.3), set `LastCheckAt(P) = now` (§5.1), and continue.

| failure | Xboard (reference) | **ggboard** |
| --- | --- | --- |
| token missing or wrong, or `server_token` unset | 422 `{"message":"Invalid token","errors":{"token":["Invalid token"]}}` (Laravel validation under ForceJson) | **403** `{"code":"forbidden","message":"invalid token"}` |
| node_id missing or not a uint64 > 0; bad node_type | 422 | **400** `{"code":"validation_failed","message":"node_id must be a positive integer"}` (or `unsupported node_type …`) |
| node not found / type mismatch | `ApiException` default code 400 → `Handler::render` → `fail([400,msg])` → **HTTP 400** `{"status":"fail","message":"Server does not exist","data":null,"error":null}` (not 500) | **400** `{"code":"validation_failed","message":"server is not exist"}`, or `"node 3 is vless, requested vmess"` |

XrayR treats status `> 399` as a failure and V2bX treats `>= 400` as one. Both only log the body.
The access log records the route pattern only, never query strings, so `token` never reaches the logs.

### 1.3 soga-v1 authentication

soga sends these request headers on every call: `API-KEY` (= `webapi_key`), `NODE-ID`, and `NODE-TYPE`. GET requests also
carry `If-None-Match` once soga has an ETag. Go canonicalises them as `Api-Key`, `Node-Id`, `Node-Type`; read them with
`r.Header.Get`.

Validation is the same as §1.2, using the headers:

- Documented `NODE-TYPE` values: `vmess vless shadowsocks shadowsocksr trojan hysteria anytls mieru`.
- Also accept `v2ray→vmess`, `ss`/`shadowsocks-plugin→shadowsocks`, `ssr→shadowsocksr`, `hysteria2→hysteria`.
- `tuic` → 400 `unsupported NODE-TYPE "tuic"` (a tuic node requested without a type is 400 `… which soga does not support`).
- A missing NODE-TYPE means "match by id only".
- soga runs hysteria **2** only (soga page "Xboard 对接 hysteria"), so a hysteria node with `ps.version == 1` is "not found".

Errors carry an **int** `code` equal to the HTTP status: 403 `{"code":403,"message":"invalid API-KEY"}`, 400
`{"code":400,"message":"node not found"}` (also `NODE-ID must be a positive integer` and type mismatches), 413
`request body too large` for POST bodies over 8 MiB.
**Never answer 404 on a soga path.** The soga doc warns that a 404 from any of the 8 endpoints stops soga from working, so
all of them must exist.

---

## 2. UniProxy endpoints (`/api/v1/server/UniProxy/*`)

### 2.1 `GET /config`

Returns 200 with a bare JSON object (no envelope) and an `ETag`, or 304 (§6.6). Keys present for **every** type:

| key | type | value |
| --- | --- | --- |
| `protocol` | string | `P.type` |
| `listen_ip` | string | `"0.0.0.0"` |
| `server_port` | int | `P.server_port`. Must be > 0 (XrayR errors with "server port must > 0"). |
| `host` | string | `P.host` |
| `network` | string\|null | vmess/vless/trojan: `ps.network`, default `"tcp"`. Other types: `null`. |
| `networkSettings` | object\|null | `ps.network_settings`, or `null` if unset |
| `network_settings` | object\|null | **same value**. Emit both keys (see below). |
| `base_config` | object | `{"push_interval":push_iv,"pull_interval":pull_iv}`, both int, **never null**. V2bX dereferences it with no nil check, and its `intervalToTime(nil)` panics [node.go]. XrayR ignores it. |
| `routes` | array | §2.1.9. Always present, `[]` when there are none. |

Why both settings keys: XrayR reads `networkSettings` for vmess and trojan, and `network_settings` for vless when
`EnableVless` is on [newV2board/model.go]. V2bX reads `network_settings` and overrides it with `networkSettings` when that
is non-empty [panel/node.go]. Upstream sends camelCase only; the fork sent both for vless.

The key must be **present** for vmess, vless and trojan (`null` is fine):

- V2bX xray skips stream settings when the raw value is empty, then dereferences `in.StreamSetting` → nil-pointer panic.
- V2bX trojan calls `json.Unmarshal` on an empty value → "unexpected end of JSON input" [core/xray/inbound.go].

#### 2.1.1 shadowsocks

| key | type | mapping |
| --- | --- | --- |
| `cipher` | string | `ps.cipher` (required by admin validation) |
| `server_key` | string\|null | 2022 ciphers: `ps.server_key` if set, otherwise derived (§4). `null` for other ciphers. |
| `obfs` | string\|null | `ps.obfs` (`plain`\|`http`\|`tls`), or `null` if empty. XrayR maps `http` to a TCP http header using `obfs_settings.path` (default `/`) [fork key]. |
| `obfs_settings` | object\|null | `ps.obfs_settings` `{"path","host"}`, or `null`. Must be an object or null, never `[]`. |
| `plugin`, `plugin_opts` | string\|null | `ps.plugin` and `ps.plugin_opts`, or `null` if empty (upstream keys) |

XrayR and V2bX switch to multi-user SS2022 mode **only when `server_key` is non-empty** [XrayR inboundbuilder; V2bX
core/xray/ss.go `if serverKey == ""`]. Upstream emits no `server_key` for `2022-blake3-chacha20-poly1305`; ggboard emits the
32-byte key. Admin UI must warn that chacha20-2022 is unreliable multi-user: V2bX's sing core only base64-encodes user keys
for the two AES ciphers [core/sing/user.go].

```json
{"protocol":"shadowsocks","listen_ip":"0.0.0.0","server_port":8388,"host":"ss.example.com","network":null,"networkSettings":null,"network_settings":null,
 "cipher":"aes-128-gcm","server_key":null,"obfs":"http","obfs_settings":{"path":"/","host":"www.example.net"},"plugin":null,"plugin_opts":null,
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

```json
{"protocol":"shadowsocks","listen_ip":"0.0.0.0","server_port":8389,"host":"ss.example.com","network":null,"networkSettings":null,"network_settings":null,
 "cipher":"2022-blake3-aes-256-gcm","server_key":"YjVkODdhNWRlMWEzNWIyMmQwMjM0NThjNDYyMGVjYWE=","obfs":null,"obfs_settings":null,"plugin":null,"plugin_opts":null,
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

In the second example, `P.created_at` is `1735689600`, so `md5("1735689600")` = `b5d87a5de1a35b22d023458c4620ecaa`, and all 32 hex
characters are base64-encoded.

#### 2.1.2 vmess

| key | type | mapping |
| --- | --- | --- |
| `tls` | int | `ps.tls` (0\|1), default 0. The backend terminates TLS with its own local certificate. |
| `tls_settings` | object | `{"server_name":ts.server_name or "","allow_insecure":ts.allow_insecure or false}` |
| `tlsSettings` | object | **same value** as `tls_settings`. V2bX (vmess/vless) uses `tlsSettings` instead of `tls_settings` when it is present; sending both keeps panels that emit either spelling interchangeable. XrayR ignores it. |

```json
{"protocol":"vmess","listen_ip":"0.0.0.0","server_port":10086,"host":"v.example.com","network":"ws",
 "networkSettings":{"path":"/ws","headers":{"Host":"v.example.com"}},"network_settings":{"path":"/ws","headers":{"Host":"v.example.com"}},
 "tls":1,"tls_settings":{"server_name":"v.example.com","allow_insecure":false},"tlsSettings":{"server_name":"v.example.com","allow_insecure":false},
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

`network_settings` values pass through as stored (xray style, DATA_MODEL). Clients read:

- ws: `path`, `headers.Host`
- grpc: `serviceName`
- httpupgrade: `path`, `host`
- tcp: `header`
- xhttp: the whole object, as SplitHTTP settings

V2bX xray rejects `http`/`h2` ("the network type is not vail").

#### 2.1.3 vless

| key | type | mapping |
| --- | --- | --- |
| `tls` | int | `ps.tls`: 0 none, 1 tls (local certificate), 2 reality |
| `flow` | string | `ps.flow`, `""` if unset |
| `server_name` | string | `ts.server_name` |
| `tls_settings` | object | tls 0 or 1: `{"server_name","allow_insecure"}`. tls 2: the reality object below. |
| `tlsSettings` | object | **same value** as `tls_settings` (see vmess). |
| `decryption` | string\|null | `ps.decryption` (§8 delta) when non-empty and not `"none"`, otherwise `null` (upstream key) |
| `encryption` | string | **only when `decryption` is set and has at least 4 `.`-segments**: its first segment, e.g. `mlkem768x25519plus`. **Omit the key otherwise.** V2bX rejects every non-empty value except `mlkem768x25519plus`, including `"none"` [core/xray/inbound.go]. |
| `encryption_settings` | object | only alongside `encryption`. Split `decryption` on `.` into `s[0..n-1]`: `{"mode":s[1],"ticket":s[2],"server_padding":join(s[3:n-1],"."),"private_key":s[n-1]}`. V2bX rebuilds `method.mode.ticket[.server_padding].private_key` from these. |

**Reality object** (`tls == 2`):

| key | type | mapping |
| --- | --- | --- |
| `server_name` | string | `ts.server_name`. XrayR and V2bX use `[server_name]` as `ServerNames`. |
| `dest` | string | **host only**: the host part of `ts.dest` (`net.SplitHostPort`, or the raw value if it has no port). Falls back to `ts.server_name` when `ts.dest` is empty. |
| `server_port` | **string** | the port part of `ts.dest`, else `ts.server_port`, else `"443"` |
| `private_key`, `public_key`, `short_id` | string | `ts.*`. `short_id` may be `""`. Clients ignore `public_key`. |
| `allow_insecure` | bool | `ts.allow_insecure` |
| `xver` | string | **Only when `ts.xver` > 0**, as a JSON *string* such as `"1"`: V2bX declares `xver,string` and fails the whole decode on a number. XrayR's field is unexported and never parsed. Absent by default. |

Why `dest` is split into host and port:

- XrayR builds `Dest = dest + ":" + server_port` [v2board.go].
- V2bX xray uses `"%s:%s"`. V2bX sing uses `dest` as the host and `server_port` as the port.
- Our stored `"example.com:443"` would therefore become `example.com:443:443`.

`server_port` must be a JSON string: V2bX unmarshals it into a Go `string` and rejects a number.

tls 1 with TCP and vision:

```json
{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"host":"vl.example.com","network":"tcp","networkSettings":null,"network_settings":null,
 "tls":1,"flow":"xtls-rprx-vision","server_name":"vl.example.com","tls_settings":{"server_name":"vl.example.com","allow_insecure":false},
 "tlsSettings":{"server_name":"vl.example.com","allow_insecure":false},"decryption":null,
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

tls 0 with xhttp and VLESS encryption (`ps.decryption = "mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL"`):

```json
{"protocol":"vless","listen_ip":"0.0.0.0","server_port":8080,"host":"vl.example.com","network":"xhttp",
 "networkSettings":{"path":"/xh","host":"vl.example.com","mode":"auto","extra":{}},"network_settings":{"path":"/xh","host":"vl.example.com","mode":"auto","extra":{}},
 "tls":0,"flow":"","server_name":"","tls_settings":{"server_name":"","allow_insecure":false},"tlsSettings":{"server_name":"","allow_insecure":false},
 "decryption":"mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL","encryption":"mlkem768x25519plus",
 "encryption_settings":{"mode":"native","ticket":"600s","server_padding":"","private_key":"PRIVATE_KEY_BASE64URL"},
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

For tls 2 (reality), see the full transcript in §7.1.

#### 2.1.4 trojan

| key | type | mapping |
| --- | --- | --- |
| `server_name` | string | `ps.server_name` |
| `tls` | int | `1` (constant: trojan is always TLS) |
| `tls_settings` | object | `{"server_name":ps.server_name,"allow_insecure":ps.allow_insecure}` |

XrayR always builds TLS over TCP and reads only `host` and `server_name` [parseTrojanNodeResponse]. V2bX supports tcp, ws and grpc.

```json
{"protocol":"trojan","listen_ip":"0.0.0.0","server_port":443,"host":"t.example.com","network":"grpc",
 "networkSettings":{"serviceName":"trojan-grpc"},"network_settings":{"serviceName":"trojan-grpc"},"server_name":"t.example.com",
 "tls":1,"tls_settings":{"server_name":"t.example.com","allow_insecure":false},"base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

#### 2.1.5 hysteria (v1 and v2; V2bX `NodeType` `hysteria` or `hysteria2`)

| key | type | mapping |
| --- | --- | --- |
| `version` | int | `ps.version`, default 2 |
| `server_name` | string | `ps.server_name` |
| `tls_settings` | object | `{"server_name","allow_insecure"}`. The backend uses its local certificate. |
| `up_mbps`, `down_mbps` | int | `ps.up_mbps` and `ps.down_mbps`, default 0. Must be integers (V2bX `int`). |
| `obfs` | string\|null | **v1**: `ps.obfs_password` when it is non-empty and `ps.obfs != "plain"`, else `null`. In v1 the obfs value *is* the password (V2bX sing passes it as `Hysteria.Obfs`). **v2**: `"salamander"` when `ps.obfs=="salamander"` and `ps.obfs_password != ""`, else `null`. |
| `obfs-password` | string\|null | **v2 only. The key contains a hyphen** (`json:"obfs-password"` in V2bX). Value: `ps.obfs_password`, or `null`. Never send `obfs:"salamander"` with an empty password: V2bX sing would then use the literal `"salamander"` as the password. |
| `ignore_client_bandwidth` | bool | **v2 only**. `ps.ignore_client_bandwidth`, default false. V2bX-only; §8 delta. |

Port hopping (`node.port` = `"20000-30000"`) is configured on the client and firewall side. The backend listens on `server_port` only.

```json
{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":36712,"host":"hy.example.com","network":null,"networkSettings":null,"network_settings":null,
 "version":1,"server_name":"hy.example.com","tls_settings":{"server_name":"hy.example.com","allow_insecure":false},"up_mbps":200,"down_mbps":1000,
 "obfs":"obfs-pass-123","base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

```json
{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":36712,"host":"hy2.example.com","network":null,"networkSettings":null,"network_settings":null,
 "version":2,"server_name":"hy2.example.com","tls_settings":{"server_name":"hy2.example.com","allow_insecure":false},"up_mbps":0,"down_mbps":0,
 "obfs":"salamander","obfs-password":"obfs-pass-123","ignore_client_bandwidth":false,"base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

#### 2.1.6 tuic (V2bX only)

Keys:

- `version`: 5.
- `server_name`: `ps.server_name`.
- `congestion_control`: `ps.congestion_control`, default `"cubic"` (the upstream default).
- `zero_rtt_handshake`: `ps.zero_rtt_handshake`, bool.
- `udp_relay_mode`: `ps.udp_relay_mode`, default `"native"`.
- `alpn`: `ps.alpn` split on `,` into an array, default `["h3"]`.
- `auth_timeout` and `heartbeat`: `"3s"` (upstream constants).
- `tls_settings`: `{"server_name","allow_insecure"}`.

Users authenticate with uuid as both UUID and password [core/sing/user.go].

```json
{"protocol":"tuic","listen_ip":"0.0.0.0","server_port":8443,"host":"tu.example.com","network":null,"networkSettings":null,"network_settings":null,
 "version":5,"server_name":"tu.example.com","congestion_control":"bbr","zero_rtt_handshake":false,"udp_relay_mode":"native","alpn":["h3"],
 "auth_timeout":"3s","heartbeat":"3s","tls_settings":{"server_name":"tu.example.com","allow_insecure":false},
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

#### 2.1.7 anytls (V2bX only; TLS comes from the backend's local certificate, and V2bX anytls has no reality)

Keys:

- `tls`: int, `ps.tls`, default 1.
- `server_name`: `ps.server_name`.
- `tls_settings`: for tls 1, `{"server_name","allow_insecure"}`. For tls 2, the vless reality object, with
  `server_name = ts.server_names[0]` or else `ps.server_name`.
- `padding_scheme`: []string. Use `ps.padding_scheme`; if it is empty, use the default
  `["stop=8","0=30-30","1=100-400","2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000","3=9-9,500-1000","4=500-1000","5=500-1000","6=500-1000","7=500-1000"]`
  (the upstream default).

```json
{"protocol":"anytls","listen_ip":"0.0.0.0","server_port":8443,"host":"a.example.com","network":null,"networkSettings":null,"network_settings":null,
 "tls":1,"server_name":"a.example.com","tls_settings":{"server_name":"a.example.com","allow_insecure":false},
 "padding_scheme":["stop=8","0=30-30","1=100-400","2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000","3=9-9,500-1000","4=500-1000","5=500-1000","6=500-1000","7=500-1000"],
 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

#### 2.1.8 mieru (upstream shape; no XrayR or V2bX support. soga is the real consumer, §3)

```json
{"protocol":"mieru","listen_ip":"0.0.0.0","server_port":2999,"host":"m.example.com","network":null,"networkSettings":null,"network_settings":null,
 "transport":"TCP","traffic_pattern":"","multiplexing":"MULTIPLEXING_LOW","base_config":{"push_interval":60,"pull_interval":60},"routes":[]}
```

`transport` = `ps.transport`, default `"TCP"`. `traffic_pattern` = `""`. `multiplexing` = `ps.multiplexing`, or `""`.

#### 2.1.9 routes

Emit the `node_routes` rows whose ids are in `P.route_ids`, sorted by id:
`{"id":int,"match":[string],"action":"block"|"dns","action_value":string}`. `action_value` is `""` when empty, never `null`.
Upstream omits `routes` when there are none; both clients accept `[]`.

How clients read routes:

- **XrayR**: for `block`, `regexp.MustCompile(strings.Join(match,"|"))`. **An invalid pattern panics XrayR**, so the admin API
  must reject a route whose joined match fails `regexp.Compile`. For `dns`, it creates a nameserver at `action_value` for
  `match` domains.
- **V2bX**: for `block`, entries starting `protocol:` become protocol blocks, a leading `regexp:` is stripped, and everything
  else is a domain regexp. `match` may be an array or a comma string. For `dns`, if `match[0]=="main"` then `match[1:]`
  concatenated is raw DNS JSON; otherwise it creates a nameserver.
- **soga**: §3.3.

### 2.2 `GET /user`

200: `{"users":[{"id":int,"uuid":string,"speed_limit":int,"device_limit":int}, ...]}`, sorted by `id`, with an ETag; or 304.

- **Selection** (also used by soga `/users`). Include a user when all of these hold:
  - `group_id IN groups(P) ∪ groups(children of P)` (§5.3)
  - `banned = false`
  - `expired_at IS NULL OR expired_at > now`
  - `u + d < transfer_enable`

  This matches Xboard `getAvailableUsers`, plus the child-group union. A node with no groups gets `{"users":[]}`.
- `speed_limit`: `users.speed_limit` in **Mbps**; `null` becomes `0` (unlimited).
  - XrayR converts to bytes/s as `Mbps*1000000/8`; a local `SpeedLimit > 0` overrides it.
  - V2bX uses the smallest non-zero value of the node-local and user limits, `*1000000/8`.
- `device_limit`: `users.device_limit`; `null` becomes `0` (unlimited). XrayR ignores it and uses its local `DeviceLimit`
  [v2board.go "todo"]. V2bX enforces it via `alivelist` (§5.2).
- V2bX sends `X-Response-Format: msgpack`. **Ignore it** and answer JSON with `Content-Type: application/json`. V2bX only
  msgpack-decodes when the response Content-Type contains `application/x-msgpack`.

### 2.3 `POST /push`

Body: `{"<uid>":[upload,download], ...}` (bytes since the last push). Example: `{"1":[1024,4096],"2":[0,512]}`.
XrayR and V2bX send it only when at least one user has traffic [XrayR controller.go `if len(userTraffic) > 0`].

- Decode into `map[string]json.RawMessage`. Treat the literal body `[]` as empty.
- Keep an entry only if its key parses as an id in 1..2^63−1 (larger ids cannot exist and cannot be bound by
  database/sql) and its value is a 2-element array of finite numbers in 0..2^50 (truncate them to int64; 1 PiB per
  push bounds the int64 usage counters). Drop anything else silently, as Xboard does.
- Charging, with `rate = P.rate`:
  - `users.u += floor(up*rate)` and `users.d += floor(down*rate)`
  - `stat_users(user, P, rate)` += the rated values
  - `stat_nodes(P)` += the raw values (Xboard: TrafficFetchJob, StatUserJob, StatServerJob)
  - Writes are buffered and flushed every 10 s (ARCHITECTURE). Unknown uids are ignored at flush.
- Set `LastPushAt(P) = now` and `Online(P) = number of kept entries` (§5.1). Do this even for an empty body.
- Response: 200 `{"data":true}`. Invalid JSON, or a body that is neither an object nor `[]`, gets 400
  `{"code":"validation_failed","message":"invalid data format"}` (upstream answers 422).

### 2.4 `POST /alive` (V2bX; XrayR newV2board's `ReportNodeOnlineUsers` is a no-op)

Body: `{"<uid>":["1.2.3.4","2001:db8::1"], ...}`. V2bX sends it every push interval, only for users above
`DeviceOnlineMinTraffic`.

For each uid:

1. Normalise each IP: strip `:port`, and turn `[v6]:port` into `v6`. Addresses longer than 64 bytes are dropped.
2. Deduplicate; keep at most 256 addresses per user and node (memory bound of the online store).
3. **Replace** the IP set stored for `(uid, P)` with this list, stamped `now` (Xboard `setDevices`). Users not in the body
   keep their entries until the TTL expires (§5.2).

Response: `{"data":true}`. Invalid JSON gets 400 `{"code":"validation_failed","message":"invalid online data"}`.

### 2.5 `GET /alivelist` (V2bX)

200: `{"alive":{"<uid>":count, ...}}`. It contains users who are eligible on P (selection as in §2.2), have
`device_limit > 0`, and have `count > 0` (§5.2).

The value is **always an object** (`{}` when empty, never `null` or `[]`), and the response has no ETag. If the request
fails, V2bX falls back to an empty map. V2bX refuses a new IP when `device_limit <= alive[uid]` [limiter.go].

### 2.6 `POST /status` (upstream extra; XrayR and V2bX never call it)

Body: `{"cpu":float 0..100,"mem":{"total":int,"used":int},"swap":{...},"disk":{...}}` (bytes; optional `uptime` seconds).
Stored as `online.Load` of P, with `mem`/`swap`/`disk` converted to used/total percentages (2 decimals) and `cpu` capped at
100. Answer `{"data":true,"code":0,"message":"success"}`. All four fields are required here: a missing or invalid one gets 400
`invalid status data` with a field error.

---

## 3. soga-v1 endpoints (`[/soga]/api/v1/*`)

Conventions:

- GET responses are bare JSON (an object for `/node`, arrays for the others), with an ETag or 304 (§6.6).
- POST responses are 200 `{"code":0,"message":""}`. `code != 0` means failure; we only use it together with a 4xx status.
- POST bodies are JSON arrays. Accept `[]` and `null` as empty.
- Every authenticated request, GET or POST, sets `LastCheckAt(P)` (§5.1).

### 3.1 `GET /api/v1/node`

Returns `{"basic":{"pull_interval":pull_iv,"push_interval":push_iv,"speed_limit":0},"config":{...}}`.

- `basic.speed_limit` is the node's total limit in Mbps. ggboard has none, so it is always 0.
- `config.port` is **`P.server_port`** (the listen port, not the client `port`) for every type.

`stream_type` maps from `ps.network` one-to-one:

- `tcp ws http h2 grpc`. A `tcp` network with an http header is still sent as `tcp`, because soga has no header obfs.
- `xhttp`: vless only.
- `httpupgrade`: no soga equivalent. It is emitted verbatim and logged with `slog.Warn` when the body is rendered.

Optional keys appear only in these cases:

- `path`: when the stream is `ws`, `http`, `h2` or `httpupgrade`. Value: `network_settings.path`.
- `service_name`: when the stream is `grpc`. Value: `network_settings.serviceName`.
- `xhttp_settings`: when the stream is `xhttp`. Value:
  `{"host":ns.host,"path":ns.path,"mode":ns.mode or "auto","extra":ns.extra or {}}`.

| soga type | `config` keys (mapping) |
| --- | --- |
| vmess | `port`, `stream_type`, `tls_type` (`ps.tls` 0→`"none"`, 1→`"tls"`), optional keys |
| trojan | `port`, `stream_type`, optional keys. There is no `tls_type`; trojan always uses TLS with the certificate from soga's config. |
| ss | `port`; `cipher` = `ps.cipher`; `password` = the 2022 **node key** (base64, §4) for 2022 ciphers, otherwise `""`; `obfs`: `plain`/`""`→`"plain"`, `http`→`"simple_obfs_http"`, `tls`→`"simple_obfs_tls"`; when obfs ≠ plain also `path` = `obfs_settings.path` and `host` = `obfs_settings.host` |
| ssr | `port`; `method` = `ps.cipher`; `password` = `ps.password` (§8 delta), or if empty `base64(md5hex(P.created_at_unix)[:16])`; `protocol` = `ps.protocol`; `obfs` = `ps.obfs`; `single_port_type` = `ps.single_port_type` (`protocol`\|`obfs`), default `"protocol"` |
| vless | `port`, `stream_type`, `tls_type` (0→`none`, 1→`tls`, 2→`reality`), `flow` = `ps.flow` or `""`, `decryption` = `ps.decryption` or `""`, plus optional keys. **When reality**, also: `server_names` = `[ts.server_name]`, `private_key` = `ts.private_key`, `short_ids` = `[ts.short_id]`, `dest` = `"host:port"` (dest host and port from §2.1.3, **joined back**) |
| hysteria (hy2) | `port`; `obfs` = `"salamander"` if `ps.obfs=="salamander"` and the password is non-empty, else `"plain"`; `obfs_password` (underscore here) = `ps.obfs_password` or `""`; `up_mbps` and `down_mbps` int, where 0/0 means BBR |
| anytls | `port`; `tls_type` (`ps.tls` 1→`tls`, 2→`reality`, 0→`none`); for reality also `server_names` = `ts.server_names`, or `[ps.server_name]` if empty, plus `private_key`, `short_ids` = `[ts.short_id]` and `dest` = `"host:port"`; `padding_scheme` = `ps.padding_scheme` or `[]` (soga uses its default when empty) |
| mieru | `port`, `transport` = `ps.transport` or `"TCP"` (soga supports TCP only) |

Examples. These are the `config` objects only; `basic` is as above. For ss2022, see §7.2.

```jsonc
{"port":10086,"stream_type":"ws","tls_type":"tls","path":"/ws"}                                             // vmess
{"port":443,"stream_type":"grpc","service_name":"trojan-grpc"}                                              // trojan
{"port":8388,"cipher":"aes-128-gcm","password":"","obfs":"simple_obfs_http","path":"/","host":"www.example.net"} // ss + obfs
{"port":443,"method":"chacha20-ietf","password":"YjVkODdhNWRlMWEzNWIyMg==","protocol":"auth_chain_a","obfs":"plain","single_port_type":"protocol"} // ssr
{"port":443,"stream_type":"tcp","tls_type":"reality","flow":"xtls-rprx-vision","decryption":"","server_names":["www.example.org"],
 "private_key":"PRIVATE_KEY_BASE64URL","short_ids":["0123abcd"],"dest":"www.example.org:443"}             // vless reality
{"port":443,"stream_type":"xhttp","tls_type":"tls","flow":"","decryption":"",
 "xhttp_settings":{"host":"x.example.com","path":"/xh","mode":"auto","extra":{}}}                        // vless xhttp
{"port":36712,"obfs":"salamander","obfs_password":"obfs-pass-123","up_mbps":0,"down_mbps":0}                 // hysteria
{"port":8443,"tls_type":"tls","padding_scheme":[]}                                                          // anytls
{"port":2999,"transport":"TCP"}                                                                             // mieru
```

### 3.2 `GET /api/v1/users`

Returns a JSON array sorted by id, using the §2.2 selection, with an ETag. `[]` when empty. Every element has:

- `id`: int
- `speed_limit`: int Mbps, null → 0
- `device_limit`: int, null → 0
- `tcp_limit`: int, always 0 (ggboard has no TCP-connection limit)

Each element also carries type-specific credential fields (§4):

| NODE-TYPE | credential fields |
| --- | --- |
| vmess, vless | `"uuid": uuid` |
| trojan, hysteria, anytls, shadowsocksr, shadowsocks (non-2022) | `"password": uuid` |
| shadowsocks 2022 | `"password": base64(uuid[:16])` for aes-128, `base64(uuid[:32])` otherwise (user PSK, §4) |
| mieru | `"username": uuid, "password": uuid` |

Example (vless): `[{"id":1,"uuid":"3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f","speed_limit":0,"device_limit":3,"tcp_limit":0}]`.
Example (mieru): `[{"id":1,"username":"3b1b6f5e-...","password":"3b1b6f5e-...","speed_limit":0,"device_limit":0,"tcp_limit":0}]`.

soga applies the lower of the panel's limits and its local `user_speed_limit`, `user_conn_limit` and `node_speed_limit`.

### 3.3 `GET /api/v1/audit_rules`

Returns `[{"id":int,"rule":string}]` with an ETag. For each `block` route in `P.route_ids` (sorted by id) and each match
entry at index `i`, emit `{"id": route.id*1000 + i, "rule": entry}`. Skip entries prefixed `protocol:`, which soga cannot express.
The admin API must cap `match` at 999 entries.

soga rule syntax:

- a plain string matches as a substring
- `regexp:` is a regular expression
- `domain:` matches the domain and its subdomains
- `full:` matches the exact domain
- `geosite:`, `ip:` (CIDR), `geoip:`, `port:a-b,c`

The same route rows reach XrayR as Go regexps (§2.1.9). Plain domain strings behave sensibly under both clients; prefixed
forms only work on soga and V2bX.

`dns` routes are not sent to soga, which configures DNS locally in `dns.yml`.

### 3.4 `GET /api/v1/white_list`

Returns the settings value `server_white_list` ([]string) verbatim, `[]` if unset, with an ETag. soga merges it with its local
`whiteList`. The syntax is the same as for audit rules.

### 3.5 `POST /api/v1/traffic`

Body: `[{"id":uid,"u":bytes,"d":bytes}]`. Uses the **same pipeline as §2.3**: the same rate and stats, `LastPushAt`, and
`Online` = the number of distinct kept ids. Entries with `id <= 0` or negative values are dropped; duplicate ids are summed.
soga may skip users below `submit_traffic_min_traffic` KB.

### 3.6 `POST /api/v1/alive_ip`

Body: `[{"id":uid,"ips":["1.2.3.4"]}]`. Same handling as §2.4.

IPs may arrive CIDR-masked, e.g. `192.168.1.0`, when soga sets `user_ip_limit_cidr_prefix_v4/v6` without `submit_real_ip`.
Store them as given. soga skips users below `submit_alive_ip_min_traffic` KB.

### 3.7 `POST /api/v1/audit_log`

Body: `[{"user_id":int,"audit_id":int}]`. soga already deduplicates repeated hits of the same rule by the same user.
Malformed entries are dropped.

For each entry, call `slog.Warn("audit rule hit", "node_id",P.id, "user_id",…, "route_id",audit_id/1000, "rule",<entry text>)`.
Also keep the hit in an in-memory ring buffer of the last 1000 hits across all nodes. `GET /api/v1/admin/nodes/:id/audit`
returns the hits of that node's P, newest first, as `[{"at","node_id","user_id","audit_id","route_id","rule"}]`. Nothing is
persisted (there is no audit table) and the buffer is empty after a restart.

### 3.8 `POST /api/v1/status`

Body: `{"cpu":23.5,"mem":{"total":int,"used":int},"swap":{...},"disk":{...},"uptime":int seconds}`. Stored like §2.6 in
`online.Load{cpu,mem,swap,disk,uptime}` of P, except that every field is optional (missing = 0). An invalid value gets 400
`{"code":400,"message":"invalid status field: <name>"}`.

---

## 4. Credential and key derivation (one helper: `model.UserCredentials(u, P)`)

| node type | UniProxy sends | backend derives (verified) | soga `/users` | subscription credential |
| --- | --- | --- | --- | --- |
| vmess / vless | `uuid` | id = uuid | `uuid` | uuid |
| trojan | `uuid` | password = uuid (XrayR userbuilder; V2bX sing user.go) | `password`=uuid | uuid |
| shadowsocks AEAD | `uuid` | password = uuid (XrayR `Passwd = UUID`; V2bX core/xray/ss.go) | `password`=uuid | uuid |
| shadowsocks 2022 | `uuid` + config `server_key` | user key = `base64(uuid[:16])` for aes-128, `base64(uuid[:32])` otherwise (XrayR `checkShadowsocksPassword`, only for panel types containing "V2board"; V2bX xray identical; V2bX sing aes-only) | `password` = user key; node `config.password` = node key | `"<nodeKey>:<userKey>"` (Xboard `generateServerPassword`) |
| hysteria v1 / v2 | `uuid` | auth string / password = uuid (V2bX sing user.go, hy2 user.go) | `password`=uuid (hy2 only) | uuid |
| tuic | `uuid` | uuid = uuid, password = uuid | n/a | uuid / uuid |
| anytls | `uuid` | password = uuid | `password`=uuid | uuid |
| shadowsocksr | n/a | n/a | `password`=uuid; node `config.password` = node SSR password | per SUBSCRIPTION.md (soga/SSPanel single-port rules) |
| mieru | n/a | n/a | `username`=uuid, `password`=uuid | uuid / uuid |

Derivations. `uuid[:n]` is the first n *bytes of the 36-char string, hyphens included*. This is byte-identical to Xboard
`Helper::uuidToBase64` / `getServerKey` and to the client code above.

- User key: `base64.StdEncoding.EncodeToString([]byte(uuid[:n]))`, where n = 16 for `2022-blake3-aes-128-gcm` and 32 for
  `2022-blake3-aes-256-gcm` and `2022-blake3-chacha20-poly1305`.
- Node key: `ps.server_key` if set. Otherwise `base64(hex(md5(strconv.FormatInt(P.created_at.Unix(),10)))[:n])`, with the
  same n. For example, created_at `1735689600` gives 16 → `YjVkODdhNWRlMWEzNWIyMg==` and 32 →
  `YjVkODdhNWRlMWEzNWIyMmQwMjM0NThjNDYyMGVjYWE=`.
- soga needs the node key in base64, per the soga doc ("当加密方式为 ss2022 时需要节点密码，格式为 base64"). The soga doc does not
  state the user-password format. We send the user PSK because it is the only value consistent with the
  `"<nodeKey>:<userKey>"` that clients receive. **Verify this against a live soga during the e2e smoke test.**
- Importer: node keys depend on `created_at`, so the Xboard importer must keep `created_at` (or write the derived key into
  `ps.server_key`). Otherwise every imported SS2022 subscription breaks.

---

## 5. Node status, devices, child nodes

### 5.1 Online semantics (in-memory `app.Online`, keyed by P)

- `LastCheckAt` is set on **every** authenticated node request. `LastPushAt` is set on every successful `push` or `traffic`,
  including empty ones.
- `Online` is the number of distinct uids in the latest push or traffic body. It reads as 0 once `LastPushAt` is older than W.
- Window `W = 2 × max(push_iv, pull_iv)` seconds.
- Status (`online.State*`; the admin API shows it as the string `state`):
  - 2 `online`: `LastPushAt` is within W
  - 1 `idle` (online, no push): only `LastCheckAt` is within W
  - 0 `offline`

  `is_online` (used by `/user/nodes` and the admin totals) is `status ≥ 1`. Subscriptions do not filter on it.
- Every authenticated request also stores the backend's `User-Agent` (first 64 bytes) as the node's `version`.
- Admin view (`GET /api/v1/admin/nodes`, `/:id`): `status: {state, online, last_check_at, last_push_at, load, version}`
  of P (`nodesvc.StatusView`).

"Last push within 2×push_interval" on its own would make idle, healthy nodes flap to offline: XrayR and V2bX skip the push
when there is no traffic (verified). Upstream Xboard uses the same three-state model with a fixed 300 s
(`Server::getAvailableStatusAttribute`).

V2bX and soga take their intervals from `base_config` and `basic`. XrayR ignores them and uses its own `UpdatePeriodic`, so
tell admins to set that value to ≤ `push_iv`. The state lives in memory, so after a panel restart nodes show offline until
their next poll.

### 5.2 Alive IPs and device limits

- Store (`internal/app/online`): per node P and user, `{ips, expires}`, replaced by each UniProxy `alive` / soga `alive_ip`
  report (an empty list removes the entry). TTL `T = max(300, 2×push_iv)` s from the report; Xboard's
  `DeviceStateService` uses `TTL=300`. The `online_cleanup` job (every 5 min) purges expired entries.
- `count(uid)` (`device_limit_mode` setting):
  - **0 strict**: the number of distinct IPs, across **all** nodes, seen within T.
  - **1 relaxed**: the same count after collapsing IPv4 to /24 and IPv6 to /64. This mode is ggboard-defined: Xboard stores
    the key but never reads it.
- Enforcement happens on the backend:
  - V2bX: via `alivelist`.
  - soga: per node from `/users.device_limit`. Limits apply across nodes only when soga's `redis_ip_limit_*` is configured.
  - XrayR newV2board: ignores the panel's `device_limit`.

  The panel never blocks anything itself. `/admin/nodes/:id/online` reads this store.

### 5.3 Child nodes (`parent_id`)

- A child is a relay entrance shown in subscriptions. Validate on save: same `type` as the parent, and the parent has no
  `parent_id` itself.
- Effective node `P = parent` (or the node itself). A backend that authenticates with a child id is answered **as P**:
  - P's `server_port`, `protocol_settings`, routes, and SS2022 node key (from P's `created_at`/`server_key`; Xboard
    `generateServerPassword` also uses the parent's `created_at`)
  - P's user list
  - traffic, alive and status recorded on P
- P's user list is selected from `groups(P) ∪ groups(children of P)`, because relay users arrive at P's backend.
  Xboard uses only the node's own `group_ids`; ggboard widens this deliberately.
- Traffic is charged at **P.rate**, because the backend cannot tell relayed traffic from direct traffic (Xboard behaves the
  same). The child's own `rate` is display-only. A child's status, online count and load are P's.
- Subscriptions for a child use the child's `host`, `port`, `name` and `tags` with P's credentials and keys.

---

## 6. Edge cases

1. **Node not found**: 400 (§1.2/§1.3). Never 404 on soga paths.
2. **Type mismatch** (including `hysteria2` against a v1 node, or soga against a v1 hysteria node): 400 with an explicit message.
3. **`show=false`**: still served. Xboard's `getServer` ignores `show`, and its V1 middleware has no enabled check; we adopt
   that. `show` only hides the node from subscriptions and user lists.
4. **No groups / no eligible users**: return `{"users":[]}` or `[]` with 200 and an ETag. XrayR errors with
   `"users is null"` on an empty list:
   - at startup the controller's `Start` fails, so the node does not come up;
   - during periodic sync it logs the error and **keeps the previous user list**, so expired or banned users keep access
     until XrayR restarts.

   Document this in the admin UI. V2bX and soga handle `[]` correctly.
5. **`server_token` unset**: every node request gets 403. The first boot generates the token (API.md).
6. **ETag**:
   - `tag = hex(sha1(body))`, where `body` is the exact uncompressed bytes sent. Header: `ETag: "<tag>"` (quoted, 40
     lowercase hex characters).
   - **304** when `strings.Contains(r.Header.Get("If-None-Match"), tag)` (Xboard `str_contains`). This accepts the quoted,
     bare and `W/` forms. A 304 has no body; also set `ETag` on it, which RFC 9110 allows and both clients tolerate.
   - Applies to UniProxy `config` and `user`, and to soga `node`, `users`, `audit_rules` and `white_list`. Not to
     `alivelist` or to POSTs.
   - Bodies must be deterministic: struct field order, rows sorted by id, and map keys only where `encoding/json` sorts them.
   - The tag is a content hash and **not** `nodes.updated_at`: settings (`base_config`) and routes change the body without
     touching the node. Note also that Xboard bumps `updated_at` on every push.
   - Cache (`nodesvc`, 60 s): config bodies per (P, push/pull interval), so interval changes apply at once; user bodies
     per P (UniProxy and soga shapes), rebuilt whenever the underlying per-group-set user list (`usersvc`, 60 s) changes.
     Admin writes to nodes, routes or groups call `nodesvc.Invalidate`, which drops both.
   - gzip middleware may compress the body. It then marks the ETag weak (`W/"<tag>"`, RFC 9110); clients echo it back and the
     substring comparison still matches, so the tag itself never changes.
7. **Bad push/traffic entries**: non-numeric, negative or wrong-length entries are dropped. Unknown uids are ignored.
   Traffic from banned or expired users is still charged (Xboard does the same).
8. **Several backends with one node id**: traffic is summed. `Online` reflects the latest pusher. The Xboard fork's
   per-backend sum via an `id` query param is not implemented.
9. **Secrets in logs**: the access log records route patterns only (no query strings, no headers), so neither `token`
   nor `API-KEY` is logged.
10. **uuid reset or user ban**: the user list body changes, so the ETag changes. Backends then drop the user within one
    pull interval plus the cache TTL.

---

## 7. Worked examples

(Bodies are shown wrapped for reading. On the wire they are compact, and the ETag values were computed over the compact bytes.)

### 7.1 XrayR, vless + reality, node 3

Node data:

- `type=vless`, `server_port=443`, `host=hk1.example.com`, `route_ids=[1]`
- `ps = {"network":"tcp","tls":2,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.example.org","dest":"www.example.org:443","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd"}}`
- Route 1: `{"match":["example-blocked.com"],"action":"block"}`

```http
GET /api/v1/server/UniProxy/config?node_id=3&node_type=vless&token=<server_token> HTTP/1.1
Host: panel.example.com

HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
ETag: "2bb00202abbadb48db07c406a2a1851180f3a03d"

{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"host":"hk1.example.com","network":"tcp","networkSettings":null,"network_settings":null,
 "tls":2,"flow":"xtls-rprx-vision","server_name":"www.example.org",
 "tls_settings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL",
                 "private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},
 "tlsSettings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL",
                "private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},
 "decryption":null,"base_config":{"push_interval":60,"pull_interval":60},
 "routes":[{"id":1,"match":["example-blocked.com"],"action":"block","action_value":""}]}
```

XrayR then builds a REALITY inbound with Dest `www.example.org:443`, ServerNames `[www.example.org]`, and ShortIds `[0123abcd]`.

```http
GET /api/v1/server/UniProxy/user?node_id=3&node_type=vless&token=<server_token> HTTP/1.1

HTTP/1.1 200 OK
ETag: "19b74f5415250b984cc6359bfd8bd041e146d05f"

{"users":[{"id":1,"uuid":"3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f","speed_limit":0,"device_limit":3},
          {"id":2,"uuid":"9d4c2e1a-0b3f-4a5c-8e7d-6f1a2b3c4d5e","speed_limit":100,"device_limit":0}]}
```

```http
GET /api/v1/server/UniProxy/user?node_id=3&node_type=vless&token=<server_token> HTTP/1.1
If-None-Match: "19b74f5415250b984cc6359bfd8bd041e146d05f"

HTTP/1.1 304 Not Modified
ETag: "19b74f5415250b984cc6359bfd8bd041e146d05f"
```

In this exchange XrayR receives 304, maps it to `UserNotModified`, and keeps its user list. User 2 is capped at 12,500,000 B/s.

```http
POST /api/v1/server/UniProxy/push?node_id=3&node_type=vless&token=<server_token> HTTP/1.1
Content-Type: application/json

{"1":[1048576,52428800],"2":[2048,8192]}

HTTP/1.1 200 OK
{"data":true}
```

If P.rate is 1.5, user 1 is charged u=1572864 and d=78643200. `LastPushAt(3)=now` and `Online(3)=2`.

The same node driven by V2bX would also send:

- `POST /alive` with `{"1":["203.0.113.7"]}` → `{"data":true}`
- `GET /alivelist` → `{"alive":{"1":1}}`. Only user 1 has `device_limit > 0`.

### 7.2 soga, shadowsocks 2022 (aes-128), node 7, `webapi_url=https://panel.example.com/soga/`

Node data: `type=shadowsocks`, `server_port=8388`, `created_at=1735689600` (unix), `ps={"cipher":"2022-blake3-aes-128-gcm","obfs":"plain"}`.

```http
GET /soga/api/v1/node HTTP/1.1
Host: panel.example.com
API-KEY: <server_token>
NODE-ID: 7
NODE-TYPE: shadowsocks

HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
ETag: "f8d278af3b4f513c6ab09ac8025dbc61e9e60e0c"

{"basic":{"pull_interval":60,"push_interval":60,"speed_limit":0},
 "config":{"port":8388,"cipher":"2022-blake3-aes-128-gcm","password":"YjVkODdhNWRlMWEzNWIyMg==","obfs":"plain"}}
```

```http
GET /soga/api/v1/users HTTP/1.1
API-KEY: <server_token>
NODE-ID: 7
NODE-TYPE: shadowsocks

HTTP/1.1 200 OK
ETag: "e2e3870b1bd7ccc5eb600870fb0627dce73202d2"

[{"id":1,"password":"M2IxYjZmNWUtN2MyYS00ZA==","speed_limit":0,"device_limit":3,"tcp_limit":0},
 {"id":2,"password":"OWQ0YzJlMWEtMGIzZi00YQ==","speed_limit":100,"device_limit":0,"tcp_limit":0}]
```

The two passwords are `base64("3b1b6f5e-7c2a-4d")` and `base64("9d4c2e1a-0b3f-4a")`. User 1's subscription password is
`YjVkODdhNWRlMWEzNWIyMg==:M2IxYjZmNWUtN2MyYS00ZA==`. A repeat request with `If-None-Match: "e2e3…02d2"` gets 304 with no body.

```http
POST /soga/api/v1/traffic HTTP/1.1
API-KEY: <server_token>
NODE-ID: 7
NODE-TYPE: shadowsocks
Content-Type: application/json

[{"id":1,"u":1024,"d":4096},{"id":2,"u":0,"d":512}]

HTTP/1.1 200 OK
{"code":0,"message":""}
```

Further POSTs to the same path prefix with the same headers:

| request | body | response |
| --- | --- | --- |
| `POST /soga/api/v1/alive_ip` | `[{"id":1,"ips":["203.0.113.7","198.51.100.9"]}]` | `{"code":0,"message":""}` |
| `POST /soga/api/v1/audit_log` | `[{"user_id":1,"audit_id":1000}]` | `{"code":0,"message":""}`; logged as route 1, entry 0 |
| `POST /soga/api/v1/status` | `{"cpu":12.5,"mem":{"total":2147483648,"used":734003200},"swap":{"total":0,"used":0},"disk":{"total":42949672960,"used":10737418240},"uptime":86400}` | `{"code":0,"message":""}` |

With the wrong key, the response is 403 `{"code":403,"message":"invalid API-KEY"}`.

---

## 8. Deltas from Xboard (all implemented)

- `protocol_settings` additions:
  - vless `decryption` (server-side VLESS encryption string; `encryption` is kept for subscriptions only)
  - hysteria `ignore_client_bandwidth` (bool)
  - shadowsocksr `password` (node password)
  - optional `server_key` already exists for shadowsocks
- Node config ETag is `sha1(body)`, not "per node `updated_at`" as ARCHITECTURE currently says.
- Alive-IP TTL is `max(300, 2×push_interval)`. This reconciles DATA_MODEL (2× push) with ARCHITECTURE (600 s).
- Online status follows the three-state model in §5.1. `device_limit_mode` follows §5.2.
- Admin validation (`nodesvc.ValidateRoute`, `nodesvc.CheckNode`): route `match` must compile as a Go regexp when joined
  with `|`, and has 1–999 entries. A child's type equals its parent's, and the parent is not itself a child. The admin UI
  warns on SS `2022-blake3-chacha20-poly1305`; `httpupgrade` on a soga node is only logged (`slog.Warn`) when soga fetches
  `/node`.
