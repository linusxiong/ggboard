# 管理员指南 / Admin guide

面向站长：从第一次登录到接入节点、上架套餐、配置支付与分流规则、从 Xboard 迁移以及登录安全策略。
安装与反向代理见 [DEPLOY.md](DEPLOY.md)；接口细节见 [API.md](API.md)。

*For operators: first login, settings, nodes (XrayR / V2bX / soga), plans and payment gateways, rule sets,
Xboard migration and 2FA / passkey policy. Installation lives in [DEPLOY.md](DEPLOY.md).*

---

## 1. 首次登录 / First login

```bash
ggboard admin create --email admin@example.com              # 省略 --password 时自动生成并只打印一次
ggboard admin create --email support@example.com --staff    # 客服（staff）账户
ggboard admin reset-password --email admin@example.com      # 重置密码（同时注销该用户所有会话）
ggboard admin promote --email user@example.com              # 把已有用户提升为管理员
# Docker：docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com

```

密码至少 8 位。数据库表在 `serve` / `admin` 等命令启动时自动迁移，无需先跑 `ggboard migrate`。

1. 打开 `https://<面板域名>/login` 登录，然后访问 `/admin`（或在账户菜单中选择 **管理后台 / Admin**）。
   管理员进入 `/admin/dashboard`，客服进入 `/admin/tickets`。
2. 立即完成这几件事：
   - **系统设置 → 站点**：填写 `app_url`（如 `https://panel.example.com`，不带结尾 `/`）。订阅链接、支付回调、
     邮件链接和通行密钥都依赖它。
   - **个人中心 → 安全**（`/profile?tab=security`）：为自己开启两步验证或添加通行密钥（见 §8）。
   - **系统设置 → 邮件**：配置 SMTP 并点 **发送测试邮件**。
   - **系统设置 → 节点通讯**：确认 `server_token`（首次启动自动生成 32 位随机值）。

| 角色 | 可用功能 |
|---|---|
| 管理员 `is_admin` | 全部后台页面 |
| 客服 `is_staff` | 工单、公告、知识库；只读查看用户 |

---

## 2. 系统设置导览 / Settings tour

后台 **系统设置 / Settings**（`/admin/settings`）按分组保存，每组单独点 **保存**（⌘/Ctrl+S）。
键名与默认值以 `internal/settings/defs.go` 为准；下表列出全部可见键。

### 站点 `site`

| 键 | 默认 | 说明 |
|---|---|---|
| `app_name` | `GG Bond` | 页面标题、邮件、订阅文件名 |
| `app_description` / `logo` / `tos_url` | 空 | 首页描述、Logo、注册页服务条款链接 |
| `app_url` | 空 | **必填**。站点地址，不含结尾 `/` |
| `currency` / `currency_symbol` | `CNY` / `¥` | ISO 4217 代码与符号；金额全部按“分”存储 |
| `default_locale` | `zh-CN` | `zh-CN` 或 `en` |
| `show_info_to_user` | `true` | 关闭后用户节点列表不显示主机与端口 |

### 注册与登录 `register`

| 键 | 默认 | 说明 |
|---|---|---|
| `stop_register` | `false` | 停止注册 |
| `email_verify` | `false` | 注册需邮箱验证码（需先配好邮件） |
| `email_whitelist_enable` / `email_whitelist_suffix` | `false` / `[]` | 邮箱后缀白名单 |
| `invite_force` | `false` | 必须填写邀请码 |
| `register_limit_by_ip_enable` / `_count` / `_expire` | `false` / 3 / 60 | 同 IP 在 N 分钟内最多注册次数 |
| `password_limit_enable` / `_count` / `_expire` | `true` / 5 / 60 | 密码错误次数与锁定分钟数 |
| `try_out_plan_id` / `try_out_hour` | 0 / 0 | 注册赠送的试用套餐与时长（0 = 不赠送） |
| `captcha_provider` | `none` | `none` / `turnstile` / `recaptcha` / `hcaptcha`；用于注册、登录与发送验证码 |
| `captcha_site_key` / `captcha_secret` | 空 | 对应服务商的站点密钥与服务端密钥 |

### 订阅 `subscribe`

| 键 | 默认 | 说明 |
|---|---|---|
| `subscribe_url` | `[]` | 订阅域名列表；为空时用 `app_url`。**每次生成链接时随机取一个**（用于分散负载） |
| `plan_change_enable` | `true` | 允许更换套餐 |
| `reset_traffic_method` | 0 | 默认流量重置：0 每月 1 日、1 按购买日每月、2 不重置、3 每年 1 月 1 日、4 按购买日每年 |
| `surplus_enable` | `true` | 更换套餐时折抵原套餐剩余价值 |
| `show_info_to_server_enable` | `true` | 订阅节点列表顶部插入剩余流量 / 到期时间 |
| `show_protocol_to_server_enable` | `false` | 节点名追加协议类型 |
| `default_remind_expire` / `default_remind_traffic` | `true` | 新用户默认开启的提醒邮件 |
| `ruleset_default_template` | `standard` | 默认分流模板（`none` / `minimal` / `standard` / `full`） |
| `ruleset_default_source` | `blackmatrix7` | 默认规则集来源 |
| `ruleset_mirror` | 空 | GitHub raw 加速前缀或镜像 id，空 = 直连 |
| `subscribe_update_interval` | 24 | 建议客户端更新间隔（小时，1–720，写入 `Profile-Update-Interval`） |
| `allow_user_rule_profile` | `true` | 关闭后所有用户使用默认模板，自定义与链接参数都被忽略 |

### 订单 `order`、邀请与佣金 `invite`

| 键 | 默认 | 说明 |
|---|---|---|
| `order_timeout_minutes` | 30 | 未支付订单自动取消（1–10080 分钟） |
| `invite_commission` | 10 | 佣金比例 % |
| `invite_gen_limit` | 5 | 每人未使用邀请码上限 |
| `invite_never_expire` | `true` | 关闭后邀请码用一次即失效 |
| `commission_first_time_enable` | `true` | 仅首单返佣 |
| `commission_auto_check_enable` | `true` | 定时任务自动发放佣金 |
| `commission_withdraw_limit` | 100 | 最低提现金额（站点货币单位） |
| `commission_withdraw_method` | `["USDT","Alipay"]` | 允许的提现方式（后端校验） |
| `withdraw_close_enable` | `false` | 关闭提现，佣金只能转入余额 |
| `commission_distribution_enable` / `_l1` / `_l2` / `_l3` | `false` / 0 | 三级分销比例 % |

### 节点通讯 `server`

| 键 | 默认 | 说明 |
|---|---|---|
| `server_token` | 自动生成 | XrayR `ApiKey` / V2bX `ApiKey` / soga `webapi_key`。点 **重新生成密钥** 并保存后旧值立即失效 |
| `server_pull_interval` / `server_push_interval` | 60 / 60 | 下发给后端的拉取 / 上报间隔（秒） |
| `server_white_list` | `[]` | 下发给 soga 的白名单 |
| `device_limit_mode` | 0 | 设备数统计：0 严格（按在线 IP）、1 宽松（IPv4 /24、IPv6 /64 算一台） |

本页底部 **节点后端配置示例** 会用当前 `app_url` 与密钥生成 XrayR / V2bX / soga 片段。

### 邮件 `email`

| 键 | 默认 | 说明 |
|---|---|---|
| `email_driver` | `log` | `log` 只写日志不发送；`smtp` 真实发送 |
| `email_host` / `email_port` / `email_encryption` | 空 / 465 / `ssl` | `ssl`、`tls`（STARTTLS）或 `none` |
| `email_username` / `email_password` | 空 | SMTP 账户 |
| `email_from_address` / `email_from_name` | 空 | 发件地址与名称 |
| `email_template` | `default` | 当前只有内置模板 `default` |
| `remind_mail_enable` | `true` | 发送到期与流量提醒 |

保存后点 **发送测试邮件**（`POST /api/v1/admin/settings/mail/test`，使用已保存的配置）。

### 前端外观 `frontend`、客户端下载 `app`

`frontend_accent`、`frontend_background_url`、`frontend_customer_service_url`、`frontend_footer_html` 与
`windows|macos|android|ios_download_url` / `_version` 可以在后台保存。下载地址会出现在公开配置
`GET /api/v1/guest/config` 的 `downloads` 中；**当前版本的前端尚未读取这些外观与下载设置**，保存后页面不会变化。

### 安全 `security`

后台此组直接以键名显示。

| 键 | 默认 | 说明 |
|---|---|---|
| `admin_mfa_required` | `false` | 强制管理员 / 客服开启两步验证或通行密钥（§8） |
| `passkey_rp_id` | 空 | WebAuthn RP ID，空 = `app_url` 的主机名 |
| `passkey_origins` | `[]` | 额外允许的来源（`app_url` 的来源始终允许） |
| `mfa_issuer` | 空 | 身份验证器中显示的发行方，空 = `app_name` |

### Telegram `telegram`

| 键 | 默认 | 说明 |
|---|---|---|
| `telegram_bot_token` | 空 | BotFather 给的机器人 token；为空时 Telegram 通知整体关闭 |
| `telegram_admin_chat_ids` | `[]` | 接收管理员通知的 Chat ID（个人、群组 `-100…` 或频道 `@频道名`），一行一个 |
| `telegram_events` | 订单已支付、管理员登录、新工单、提现、订阅泄露 | 推送给管理员的事件，见 §9 |
| `telegram_user_notify` | `true` | 已绑定的用户收到自己的支付成功、工单回复、到期与流量提醒 |
| `telegram_discuss_link` | 空 | 交流群链接，显示在用户的 Telegram 绑定页与机器人帮助里 |
| `telegram_webhook_secret` | 首次启动自动生成 | Webhook 地址的最后一段；重置为默认会生成新值，之后需重新 **设置 Webhook** |

---

## 3. 节点接入 / Adding nodes

顺序：**权限组 → 路由规则（可选）→ 节点 → 在服务器上配置后端**。

1. **权限组 / Groups**（`/admin/groups`）：用户购买套餐后获得套餐的权限组，只能使用该组的节点。
2. **路由规则 / Routes**（`/admin/routes`）：下发给 XrayR / V2bX 的路由，动作 `block`（阻断，同时作为 soga
   审计规则）或 `dns`（用指定 DNS 解析匹配的域名）。阻断规则每行一条，所有行用 `|` 连接后必须是合法的 Go 正则。
3. **节点 / Nodes**（`/admin/nodes` → **新建节点**）：协议类型创建后不可更改。

| 字段 | 说明 |
|---|---|
| 地址 / 连接端口 | 下发给客户端；Hysteria 端口跳跃可填范围 `20000-30000` |
| 服务端口 | 后端实际监听的端口 |
| 流量倍率 | 用户流量 × 倍率计费 |
| 权限组 | 未选择则没有用户可用 |
| 父节点 | 子节点用作中转入口：沿用父节点协议配置与凭据，流量计入父节点 |
| 显示 | 关闭后不出现在用户订阅，但后端仍可拉取配置 |
| 协议 / JSON | 协议参数（`protocol_settings`），字段见 [NODE_PROTOCOLS.md](NODE_PROTOCOLS.md) |

保存后节点有了 ID，编辑页的 **对接** 标签会生成该节点专用的后端片段。节点列表状态：**在线**（有流量上报）、
**无流量**（仅心跳）、**离线**。

### 3.1 协议与后端对应表

| 面板协议 | XrayR `NodeType` | V2bX `NodeType` | soga `server_type` |
|---|---|---|---|
| vmess | `V2ray` | `vmess` | `v2ray` |
| vless | `Vless`（加 `EnableVless: true`） | `vless` | `vless` |
| trojan | `Trojan` | `trojan` | `trojan` |
| shadowsocks | `Shadowsocks` | `shadowsocks` | `ss` |
| shadowsocksr | — | — | `ssr` |
| hysteria（v1） | — | `hysteria` | — |
| hysteria（v2） | — | `hysteria2` | `hysteria` |
| tuic | — | `tuic` | — |
| anytls | — | `anytls` | `anytls` |
| mieru | — | — | `mieru` |

### 3.2 XrayR（`PanelType: NewV2board`）

```yaml
# /etc/XrayR/config.yml
Nodes:
  - PanelType: "NewV2board"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: "<server_token>"
      NodeID: 1
      NodeType: Vless        # V2ray | Vless | Trojan | Shadowsocks
      EnableVless: true      # 仅 vless 节点需要
      Timeout: 30
```

XrayR 请求 `/api/v1/server/UniProxy/{config,user,push}`；它忽略面板下发的用户设备数，只使用本地 `DeviceLimit`。它不读取面板下发的拉取间隔，请把
`ControllerConfig.UpdatePeriodic` 设为不大于面板的上报间隔。

### 3.3 V2bX

```jsonc
// /etc/V2bX/config.json → "Nodes": [ … ]
{
  "Core": "sing",            // hysteria / tuic / anytls 用 sing，其余用 xray
  "ApiHost": "https://panel.example.com",
  "ApiKey": "<server_token>",
  "NodeID": 1,
  "NodeType": "vless",
  "Timeout": 30
}
```

V2bX 使用面板下发的拉取 / 上报间隔，并通过 `/api/v1/server/UniProxy/alivelist` 执行设备数限制。

### 3.4 soga（`type=soga-v1`）

```ini
# /etc/soga/soga.conf
type=soga-v1
server_type=v2ray
node_id=1
soga_key=<soga license>
api=webapi
webapi_url=https://panel.example.com/soga/
webapi_key=<server_token>
```

推荐 `webapi_url` 使用 `/soga/` 前缀（soga 许可证绑定完整的 `webapi_url`，也便于在反向代理单独限流）；
soga 请求 `/soga/api/v1/{node,users,audit_rules,white_list,traffic,alive_ip,audit_log,status}`，
不带前缀的 `/api/v1/...` 同样可用。

---

## 4. 套餐、优惠券与支付 / Plans, coupons and payments

### 4.1 套餐 `/admin/plans`

- **价格**：按周期填写（站点货币），留空表示不提供该周期。周期：`monthly`、`quarterly`、`half_yearly`、
  `yearly`、`two_yearly`、`three_yearly`、`onetime`，以及 `reset_traffic`（流量重置包）。
- **权限组**、每周期流量、限速、设备数、容量上限（留空 = 不限）、流量重置方式（留空 = 跟随系统设置）。
- **显示**（商店可见）/ **销售**（允许新购）/ **续费**（允许已有用户续费）：关闭“销售”的套餐只供老用户续费。
- `PATCH /api/v1/admin/plans/:id` 带 `"force_update": true` 时会把权限组、流量和限制同步到所有持有该套餐的用户
  （后台表单暂未提供此选项）。

### 4.2 优惠券 `/admin/coupons`

类型 `1` 固定金额（分）或 `2` 百分比；可限制总次数、每人次数、适用套餐与周期、有效期；一次最多批量生成 500 个，
留空优惠码则随机生成。

### 4.3 支付方式 `/admin/payments`

**添加支付方式** → 选择网关 → 填写网关配置 → 设置显示名称、图标、手续费（固定 + 百分比，由用户承担）→ 启用。
网关创建后不可更换。

**图标**：在图标网格里点选内置图标（支付宝、微信支付、Stripe、银行卡、PayPal、加密货币、比特币、USDT、以太坊、
银行转账、扫码支付、余额 / 钱包、易支付、Epusdt、BTCPay、Coinbase），显示名称输入框左侧会实时预览用户看到的样子。
新建时会按网关自动选一个（易支付按“支付方式”`type` 选支付宝 / 微信 / USDT，Stripe 按 `payment_methods` 选
银行卡 / 支付宝 / 微信，Epusdt → USDT，当面付 → 支付宝），手动点选后不再自动更换。选 **自定义 URL** 可填图片地址
（`https://…` 或站内 `/…` 路径，建议正方形 SVG/PNG）或单个 Emoji。内置图标保存为 `preset:<id>`
（如 `preset:alipay`），格式见 [API.md › Payment icons](API.md#payment-icons)。

回调地址（Notify URL）在保存后显示，格式为：

```text
<回调域名 notify_domain，或 app_url>/api/v1/guest/payment/notify/<gateway>/<uuid>
```

`uuid` 每个支付方式唯一；支付完成后买家经 `/api/v1/guest/payment/return/<trade_no>` 回到订单页。

| 网关 `gateway` | 必填配置 | 回调设置 |
|---|---|---|
| 易支付 `epay` | `url`（不含 `/submit.php`）、`pid`、`key`；可选 `type`（`alipay`/`wxpay`/`qqpay` 等） | 下单时自动携带，无需手动配置；仅人民币 |
| Epusdt / BEpusdt `epusdt` | `api_url`、`api_token`；`currency_type` 默认 `usdt.trc20` | 自动携带 |
| 支付宝当面付 `alipay_f2f` | `app_id`、`private_key`（RSA2 ≥ 2048）、`alipay_public_key`（支付宝公钥）；`sandbox` | 自动携带；仅公钥模式、仅人民币 |
| Stripe `stripe` | `secret_key`、`webhook_secret`；可选 `currency`、`exchange_rate`、`payment_methods`（`auto`/`card`/`alipay`/`wechat_pay`） | Dashboard → Developers → Webhooks 添加端点，URL 填回调地址，至少订阅 `checkout.session.completed` 与 `checkout.session.async_payment_succeeded`（`async_payment_failed`、`expired` 可选，只做确认） |
| Coinbase Business `coinbase` | `api_key_name`、`api_key_secret`（ECDSA P-256 PEM）、`webhook_secret`；`exchange_rate`（每单位站点货币对应 USDC） | 用 `cdp data webhooks subscriptions create` 为 `checkout.payment.*`（及可选 `checkout.refund.*`）创建订阅，目标为回调地址；仅以 `checkout.payment.success` 开通 |
| BTCPay Server `btcpay` | `server_url`、`store_id`、`api_key`（`btcpay.store.cancreateinvoice` + `btcpay.store.canviewinvoices`）、`webhook_secret` | Store → Settings → Webhooks 新建，Payload URL 填回调地址；发票状态为 `Settled` 才开通 |

Stripe / Coinbase / BTCPay 的回调地址必须是公网 HTTPS。各网关协议细节见 [PAYMENTS_GATEWAYS.md](PAYMENTS_GATEWAYS.md)。

### 4.4 订单 `/admin/orders`

可筛选、**标记为已支付**（立即开通并计算佣金，先在支付平台核实到账）、**取消订单**（退回已抵扣余额）、
修改佣金状态，或 **为用户分配订单**（创建待支付订单）。

---

## 5. 规则集与模板 / Rule sets and templates

- **系统设置 → 订阅**：`ruleset_default_template`、`ruleset_default_source`、`ruleset_mirror`、
  `allow_user_rule_profile`（见 §2）。
- **规则集 / Rule sets**（`/admin/rulesets`）：标签页 **来源 / 分类 / 模板 / 镜像** 展示内置目录合并覆盖后的生效结果
  （只读）。内置来源：blackmatrix7、MetaCubeX、Loyalsoldier、SagerNet、ACL4SSR、DustinWin；内置模板：
  `none`（全局代理）、`minimal`（国内直连）、`standard`（标准分流）、`full`（精细分流）；内置镜像：`direct`、
  `ghfast`、`gh-proxy`、`ghproxy-net`、`gh-llkk`。
- **自定义覆盖**：JSON 结构与内置目录相同，`sources` 按键、`categories` / `templates` / `mirrors` 按 `id` 合并，
  同 id 替换、新 id 追加；保存（`PUT /api/v1/admin/rulesets`）后对新生成的订阅立即生效，提交 `null` 恢复内置目录。
  表单可快速添加镜像或自定义分类；新分类的 id 会写进用户的规则配置，之后不要修改。

目录与模板格式见 [RULESETS.md](RULESETS.md)，订阅输出见 [SUBSCRIPTION.md](SUBSCRIPTION.md)。

---

## 6. 用户与内容 / Users and content

- **用户 / Users**：新建、批量生成（下载含密码的 CSV）、群发邮件（Markdown，后台队列发送）、导出；单个用户可
  **以此用户登录**（一次性链接，建议在隐私窗口打开）、**重置订阅密钥**（UUID 与订阅 token 重新生成）、
  **重置流量**、**封禁**、删除（连带订单、工单与流量记录）。
- **工单 / 公告 / 知识库**：客服也可使用。知识库正文支持占位符 `{{siteName}}`、`{{subscribeUrl}}`、
  `{{urlEncodeSubscribeUrl}}`、`{{safeBase64SubscribeUrl}}`；`<!--access start-->…<!--access end-->` 之间的内容
  只对有有效套餐的用户显示。
- **系统状态 / System**（`/admin/system`）：运行状态、定时任务、邮件队列、缓存与运行日志，每 30 秒刷新。

---

## 7. 从 Xboard 迁移 / Migrating from Xboard

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # 先预演（全部回滚）
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

| 参数 | 说明 |
|---|---|
| `--dsn` | 必填：`mysql://…`、`postgres://…` 或 `sqlite:///path` |
| `--driver` | DSN 没有 scheme 时指定 `mysql` / `postgres` / `sqlite` |
| `--dry-run` | 读写全部执行后回滚 |
| `--force` | 先删除 ggboard 已有数据（用户、套餐、节点…）再导入 |
| `--with-stats` / `--stats-days` | 同时导入统计，默认最近 90 天 |
| `--source-tz` | Xboard 时区，默认 `Asia/Shanghai` |

**导入前**：暂停销售并等待待支付订单结束（订单不导入）；在 Xboard 结算佣金；先 `--dry-run` 并逐条阅读警告。

**导入后检查清单**：

1. 重启 ggboard，让运行中的进程加载导入的设置。
2. 后端：`server_token` 原样复制，XrayR / V2bX / soga 的密钥不变；把 `ApiHost` / `webapi_url` 指向 ggboard
   （或迁移旧域名 DNS），并按警告修改被重新编号的 `NodeID`。
3. 支付：EPay / Epusdt / 当面付无需改动；Stripe、Coinbase、BTCPay 需在服务商后台把 Webhook 改为新的回调地址后再启用。
4. 订阅：`curl -sI "https://<host>/api/v1/client/subscribe?token=<真实 token>"` 应返回 200 并带
   `subscription-userinfo`；`/s/<token>` 短链接同样可用；确认 `subscribe_url` 包含用户已在用的域名。
5. 登录：分别用 bcrypt 与 md5/sha256 旧哈希用户登录一次（成功后自动升级为 argon2id）。
6. 设置：发送测试邮件、复核人机验证密钥；Xboard 的 `secure_path` 不再生效，如需隐藏 `/admin` 请在反向代理上限制。
7. 保留 Xboard 数据库作为只读备份（导入器从不修改它）。

完整映射与警告含义见 [IMPORT_XBOARD.md](IMPORT_XBOARD.md)。

---

## 8. 两步验证与通行密钥策略 / 2FA and passkey policy

- **强制管理员 MFA**：开启 `admin_mfa_required` 后，未开启两步验证且没有通行密钥的管理员与客服访问后台接口会得到
  `403 mfa_setup_required`，前端提示并跳转到 **个人中心 → 安全** 完成设置（该页面不受限制）。建议先为自己开启再打开开关。
  普通用户不受此开关影响，可自愿开启。
- **通行密钥前提**：必须设置 `app_url`（或 `passkey_rp_id`），否则公开配置里 `passkey_enabled` 为 `false`，
  前端隐藏通行密钥。用户须通过站点正式域名（HTTPS）访问。
- **多个域名**：`passkey_rp_id` 设为共同的上级域名（如 `example.com`），并把其他来源（如 `https://www.example.com`）
  加入 `passkey_origins`。通行密钥与注册时的 RP ID 绑定，修改 `passkey_rp_id` 后已有通行密钥将无法使用。
- **TOTP 密钥加密**：身份验证器密钥用 `jwt.secret` 派生的密钥加密存储，轮换 `jwt.secret` 会让所有已绑定的身份验证器失效
  （同时注销所有会话）。
- **找回**：用户丢失身份验证器时，可用备用码登录，或用已添加的通行密钥直接登录。后台与 CLI **没有**重置他人两步验证的功能，
  `ggboard admin reset-password` 也不会关闭两步验证；万不得已时由运维在数据库中删除该用户在 `user_mfa` 表中的行
  （关闭 TOTP 与备用码），或删除 `passkeys` 表中对应的行。

---

## 9. Telegram 通知 / Telegram notifications

面板通过 Telegram 机器人把关键事件推送给管理员，并让用户绑定自己的 Telegram 接收订单与工单通知。
*The panel pushes key events to admin chats through a Telegram bot; users can bind their own Telegram account.*

### 9.1 创建机器人

1. 在 Telegram 里打开 [@BotFather](https://t.me/BotFather)，发送 `/newbot`，按提示填写名称与用户名（必须以 `bot` 结尾）。
2. BotFather 回复的 `123456789:AA…` 就是 token，填入 **系统设置 → Telegram → `telegram_bot_token`** 并保存。
   token 等同于机器人密码：不要截图外传；泄露后在 BotFather 里 `/revoke` 换新。
3. 可选：`/setdescription`、`/setuserpic` 设置简介与头像；`/setcommands` 填入
   `start - 绑定面板账户`、`unbind - 解除绑定`、`id - 查看 Chat ID`。

### 9.2 设置 Webhook

机器人通过 Webhook 接收命令。先确认 **站点 → `app_url`** 是可公网访问的 HTTPS 地址（Telegram 只接受 443、80、88、8443 端口），
然后在 **系统设置 → Telegram** 点 **设置 Webhook**（`POST /api/v1/admin/settings/telegram/webhook`）。面板把
`<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>` 注册给 Telegram，并显示 Telegram 的原始答复
（`ok: true`、`Webhook was set` 即成功）。更换 `app_url`、重置 `telegram_webhook_secret` 或更换 token 后要重新设置一次。
同一个 token 同时只能有一个 Webhook：如果旧面板（如 Xboard）还在用这个机器人，设置后旧面板就收不到命令了。

### 9.3 获取 Chat ID 并测试

1. **个人**：私聊机器人发送 `/id`，机器人回复 `Chat ID：123456789`。
2. **群组**：把机器人拉进群，在群里发送 `/id`（多个机器人时发 `/id@你的机器人`），得到 `-100…` 开头的 ID。
3. **频道**：把机器人设为频道管理员，`telegram_admin_chat_ids` 直接填 `@频道用户名`。
4. 把这些 ID 填入 `telegram_admin_chat_ids`（一行一个）并保存，点 **发送测试消息**（`POST …/telegram/test`），
   结果会逐个列出每个 Chat 是否成功（常见失败：`chat not found` = ID 错误或机器人没进群；`bot was blocked` = 被拉黑）。

### 9.4 事件

`telegram_events` 决定管理员收到哪些事件（默认前五项）：

| 事件 | 键 | 内容 |
|---|---|---|
| 🧾 新订单 | `order_new` | 用户下单：邮箱、套餐、周期、金额、订单号（后台手动分配的订单不推送） |
| 💰 订单已支付 | `order_paid` | 以上 + 支付方式、流水号；**默认开启** |
| 🧾 订单已取消 | `order_cancel` | 超时自动取消或管理员取消（用户自己取消不推送） |
| 🔐 管理员登录 | `admin_login` | 每次管理员 / 客服登录：角色、方式（密码、密码 + 两步验证、密码 + 备用码、通行密钥）、IP、设备、时间；**默认开启** |
| 🎫 新工单 | `ticket_new` | 主题、优先级、用户与首条消息摘要；**默认开启** |
| 🎫 工单有新回复 | `ticket_reply` | 客服的每条回复（本来就发给用户；管理员一般不需要） |
| 💸 提现申请 | `withdraw` | 用户、金额、方式与打码后的账号；**默认开启** |
| 🚨 订阅疑似泄露 | `subscription_leak` | 订阅泄露检测命中；**默认开启** |

用户侧（`telegram_user_notify` 开启时，仅限已绑定的用户）：自己的订单支付成功、工单回复、到期提醒与流量提醒。
到期 / 流量提醒随每天 10:00 的提醒任务一起发送，与邮件提醒共用“24 小时一次”的限制；关闭 `remind_mail_enable`
只停邮件，已绑定 Telegram 的用户仍会收到 Telegram 提醒。

发送失败会自动重试 3 次（1 s / 2 s / 4 s，遇到 Telegram 限流按其要求等待），仍失败的写入 **系统 → 日志**（WARN），
**系统状态** 的 `notify` 一栏显示排队、成功、失败数与启用的渠道。服务器无法直连 `api.telegram.org` 时通知无法送达，
请为服务器配置可用的出站网络。

### 9.5 从 Xboard 迁移

导入器保留每个用户的 `telegram_id`。若沿用 Xboard 原来的机器人 token，已绑定的用户无需重新绑定即可收到通知；
记得按 §9.2 把 Webhook 改到 ggboard。Xboard 的 `telegram_*` 设置不会被导入，需要在这里重新填写。
