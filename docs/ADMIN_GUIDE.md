# 管理员指南 / Admin guide

面向站长：从第一次登录到接入节点、上架套餐、配置支付与分流规则、从 Xboard 迁移、登录安全策略、Telegram 通知以及订阅防泄露。
安装与反向代理见 [DEPLOY.md](DEPLOY.md)；接口细节见 [API.md](API.md)。

*For operators: first login, settings, nodes (XrayR / V2bX / soga), plans and payment gateways, rule sets,
Xboard migration, 2FA / passkey policy, Telegram notifications and subscription leak protection. Installation lives in
[DEPLOY.md](DEPLOY.md).*

---

## 1. 首次登录 / First login

```bash
ggboard admin create --email admin@example.com              # 省略 --password 时自动生成并只打印一次
ggboard admin create --email support@example.com --staff    # 客服（staff）账户
ggboard admin reset-password --email admin@example.com      # 重置密码（同时注销该用户所有会话）
ggboard admin promote --email user@example.com              # 把已有用户提升为管理员
# Docker（预构建镜像）：docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
# Docker（源码构建）：docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com

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
| --- | --- |
| 管理员 `is_admin` | 全部后台页面 |
| 客服 `is_staff` | 工单、公告、知识库；只读查看用户 |

---

## 2. 系统设置导览 / Settings tour

后台 **系统设置 / Settings**（`/admin/settings`）依次分为 **站点、注册与登录、订阅、订单、邀请与佣金、节点通讯、邮件、
前端外观、客户端下载、安全、Telegram** 11 个分组，组内再按小节排列；依附于开关或下拉框的字段（白名单后缀、IP 注册次数、
验证码密钥、SMTP 服务器、分销比例等）只在开关打开后出现。每组单独点 **保存**（⌘/Ctrl+S），切换分组时未保存的修改会保留。
键名与默认值以 `internal/settings/defs.go` 为准；下表按界面顺序列出全部可见键，括号内为界面名称。

### 站点 `site`

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `app_name` | `GG Bond` | 页面标题、邮件、订阅文件名 |
| `app_description` / `logo` / `tos_url` | 空 | 首页描述、Logo、注册页服务条款链接 |
| `app_url` | 空 | **必填**。站点地址，不含结尾 `/` |
| `currency` / `currency_symbol` | `CNY` / `¥` | ISO 4217 代码与符号；金额全部按“分”存储 |
| `default_locale` | `zh-CN` | `zh-CN` 或 `en` |
| `show_info_to_user` | `true` | 关闭后用户节点列表不显示主机与端口 |

### 注册与登录 `register`

界面分为 **注册**、**试用**、**防滥用**、**人机验证** 四个小节：

| 键（界面名称） | 默认 | 说明 |
| --- | --- | --- |
| `stop_register`（开放注册） | `false` | 界面是与键值相反的“开放注册”开关：关掉开关即 `stop_register: true`，新用户无法注册，已有账户不受影响 |
| `email_verify`（邮箱验证） | `false` | 注册需邮箱验证码。验证码靠邮件发送：先在 **邮件** 组配好 SMTP 并发送测试邮件；发信方式仍是“仅记录日志”时这里会提示验证码不会真正发出。关闭时注册不证明邮箱所有权（任何人都能用别人的邮箱注册），这类账户第一次找回密码时按所有权变更处理：更换 UUID 与订阅 token，清除 Passkey、两步验证和 Telegram 绑定 |
| `email_whitelist_enable` / `email_whitelist_suffix`（邮箱后缀白名单 / 允许的邮箱后缀） | `false` / `[]` | 打开后出现后缀输入框：只填域名（如 `gmail.com`，精确匹配），按回车或逗号添加，可一次粘贴多个；允许的后缀会显示在注册页邮箱输入框下方 |
| `invite_force`（强制邀请码） | `false` | 没有有效邀请码时无法注册 |
| `try_out_plan_id` / `try_out_hour`（试用套餐 / 试用时长） | 0 / 0 | 注册赠送的试用套餐与时长（小时）；选“不赠送试用”时不显示时长 |
| `register_limit_by_ip_enable` / `_count` / `_expire`（IP 注册限制） | `false` / 3 / 60 | 同一 IP 在 N 分钟内最多注册的账户数 |
| `password_limit_enable` / `_count` / `_expire`（密码错误锁定） | `true` / 5 / 60 | 允许错误次数与锁定分钟数（按邮箱计） |
| `captcha_provider`（验证码服务） | `none` | `none` / `turnstile` / `recaptcha` / `hcaptcha`；用于注册、登录与发送邮箱验证码 |
| `captcha_site_key` / `captcha_secret`（Site Key / Secret Key） | 空 | 选择服务商后出现：公开的站点密钥与只保存在服务器的服务端密钥 |

### 订阅 `subscribe`

界面分为 **订阅地址**、**套餐变更**、**订阅内容与提醒**、**分流规则**、**防泄露**、**IP 归属地** 六个小节：

| 键（界面名称） | 默认 | 说明 |
| --- | --- | --- |
| `subscribe_url`（订阅地址） | `[]` | 订阅域名列表；为空时用 `app_url`。**每次生成链接时随机取一个**（用于分散负载） |
| `subscribe_update_interval`（订阅更新间隔） | 24 | 建议客户端更新间隔（小时，1–720，写入 `Profile-Update-Interval`） |
| `plan_change_enable`（允许更换套餐） | `true` | 关闭后用户只能续费当前套餐 |
| `surplus_enable`（升级折抵） | `true` | 更换套餐时折抵原套餐剩余价值 |
| `reset_traffic_method`（默认流量重置方式） | 0 | 0 每月 1 日、1 按购买日每月、2 不重置、3 每年 1 月 1 日、4 按购买日每年；套餐可单独设置 |
| `show_info_to_server_enable`（订阅中显示套餐信息） | `true` | 订阅节点列表顶部插入剩余流量 / 到期时间 |
| `show_protocol_to_server_enable`（节点名称显示协议） | `false` | 节点名追加协议类型 |
| `default_remind_expire` / `default_remind_traffic` | `true` | 新用户默认开启的到期 / 流量提醒 |
| `allow_user_rule_profile`（允许用户自定义分流） | `true` | 关闭后所有用户使用默认模板，订阅配置、节点处理与链接里的规则参数都被忽略 |
| `ruleset_default_template`（默认分流模板） | `standard` | `none` / `minimal` / `standard` / `full` |
| `ruleset_default_source`（默认规则集来源） | `blackmatrix7` | 优先使用的规则仓库 |
| `ruleset_mirror`（规则集镜像） | 空 | GitHub raw 加速前缀或镜像 id，空 = 直连 |
| `subscribe_age_optional`（允许临时关闭加密） | `true` | 用户开启 age 加密后，链接加 `&age=0` 仍可取得明文 |
| `subscribe_age_strict`（严格加密） | `false` | 用户开启加密后，不支持 age 的格式返回 406，而不是退回明文 |
| `subscribe_leak_max_ips`（泄露检测阈值） | 0 | 24 小时内同一订阅 token 被超过 N 个不同 IP（IPv6 按 /64）获取时触发；0 = 关闭 |
| `subscribe_leak_action`（触发后） | `notify` | 阈值大于 0 时出现：`notify` 或 `rotate`（轮换 token），详见 §10 |
| `geoip_enable`（显示 IP 归属地） | `true` | 访问记录里显示 IP 的地区与运营商（纯真 IP 库，仅 IPv4），每天 05:23 自动检查更新 |
| `geoip_download_url`（数据库下载地址） | metowolf/qqwry.dat 最新发布 | 访问不了 GitHub 时换成镜像；留空 = 不自动下载，只用手动放到 `<data_dir>/geoip/qqwry.dat` 的文件 |
| `geoip_version_url`（版本信息地址） | metowolf/qqwry.dat 的 `version.json` | 版本没变就不下载；留空 = 每次检查都下载整个文件并比较校验和 |

**IP 归属地** 小节下方的卡片显示数据库版本、IP 段数、大小、下载与检查时间、上次失败的原因，**立即更新** 马上检查一次
（下载超过 60 秒会转到后台，卡片自动刷新）。设置有未保存的更改时按钮不可用。部署要求见 DEPLOY.md §6。

### 订单 `order`、邀请与佣金 `invite`

| 键 | 默认 | 说明 |
| --- | --- | --- |
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
| --- | --- | --- |
| `server_token` | 自动生成 | XrayR `ApiKey` / V2bX `ApiKey` / soga `webapi_key`。点 **重新生成密钥** 并保存后旧值立即失效 |
| `server_pull_interval` / `server_push_interval` | 60 / 60 | 下发给后端的拉取 / 上报间隔（秒） |
| `server_white_list` | `[]` | 下发给 soga 的白名单 |
| `device_limit_mode` | 0 | 设备数统计：0 严格（按在线 IP）、1 宽松（IPv4 /24、IPv6 /64 算一台） |

本页底部 **节点后端配置示例** 会用当前 `app_url` 与密钥生成 XrayR / V2bX / soga 片段。

### 邮件 `email`

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `email_driver` | `log` | `log` 只写日志不发送；`smtp` 真实发送 |
| `email_host` / `email_port` / `email_encryption` | 空 / 465 / `ssl` | `ssl`、`tls`（STARTTLS）或 `none` |
| `email_username` / `email_password` | 空 | SMTP 账户 |
| `email_from_address` / `email_from_name` | 空 | 发件地址与名称 |
| `remind_mail_enable` | `true` | 发送到期与流量提醒 |

保存后点 **发送测试邮件**（`POST /api/v1/admin/settings/mail/test`，使用已保存的配置）。

旧设置 `email_template` 已不再显示：它只为兼容 Xboard 导入而保留，服务端从不读取。邮件内容改在下面的 **邮件模板** 页面里定制。

### 邮件模板 `/admin/mail-templates`

入口：侧边栏 **系统 → 邮件模板**，或 **系统设置 → 邮件** 右上角的 **邮件模板** 按钮；只有管理员可用（客服 403）。
六封系统邮件 —— **邮箱验证码** `verify_code`、**到期提醒** `remind_expire`、**流量提醒** `remind_traffic`、
**支付成功** `order_paid`、**工单回复** `ticket_reply`、**通用通知** `notify`（群发邮件、订阅泄露提醒等）—— 都可以按
**简体中文 / English** 分别改写 **邮件标题** 和 **Markdown 正文**。正文会套进站点统一的邮件外观（Logo 或站点名、白色卡片、
页脚），只需写卡片里的内容；没有自定义的语言继续使用内置模板，左侧列表在每个模板下标出“中文 / 英文：已自定义 / 默认”。

**变量**：和公告、知识库的 `{{siteName}}` 写法相同，点编辑器工具栏里的变量按钮即可在光标处插入，按钮下方列出每个变量的含义。
写法是 `{{name}}`，区分大小写、不能有空格（`{{ code }}`、`{{Code}}` 都算未知变量，会原样出现在邮件里）；标题里同样可以用。
带 ★（界面上是红色 `*`）的变量必须出现在 **正文** 里（写在标题里不算）。每个模板都能用 `{{siteName}}`（站点名称）、
`{{siteUrl}}`（`app_url`，可能为空）和 `{{year}}`（当前年份）：

| 模板 | 变量（★ 必填） | 说明 |
| --- | --- | --- |
| 邮箱验证码 `verify_code` | `code` ★、`minutes` | `minutes` 默认 10，与内置邮件一致 |
| 到期提醒 `remind_expire` | `expiredAt` ★、`url` | `url` 是续费页面（未提供时为 `siteUrl`） |
| 流量提醒 `remind_traffic` | `percent` ★、`used`、`total`、`url` | `percent` 是纯数字（如 `86`） |
| 支付成功 `order_paid` | `tradeNo` ★、`amount` ★、`planName`、`period`、`url` | `amount` 带货币符号；`url` 是订单页面 |
| 工单回复 `ticket_reply` | `ticketSubject` ★、`message`、`url` | `message` 是客服的回复（多行） |
| 通用通知 `notify` | `content` ★、`title`、`url` | `title` 是调用方给的标题，默认标题就是 `{{title}}`，群发邮件与泄露提醒保留各自的主题；`content` 可多行 |

所有 `url` 在调用方没有提供时都回落到 `siteUrl`。

**正文**：Markdown（GFM，可用表格、删除线、任务列表，裸链接自动变成链接，单个换行保留为换行，也允许原始 HTML）。
单独成段的链接（例如 `[查看订单]({{url}})`）会渲染成邮件里的按钮；替换后地址为空的链接（没有配置 `app_url`）会被去掉。
在原始 HTML 属性里使用变量时要加引号（`<a href="{{url}}">`）。变量的值会被转义，不会被当成 Markdown 或 HTML。

**实时预览**：编辑时（停顿约 0.4 秒后）编辑器下方用示例数据渲染出真实的邮件，可在 **邮件 / 纯文本** 之间切换；预览中的链接
不可点击。缺少的必填变量（黄色）和未知变量（红色）会以标签直接显示在预览上方，保存前就能改正。

**保存、测试与恢复**：

- **保存**（底部保存栏或 ⌘/Ctrl+S）：服务端再校验一次（标题 1–255 字、单行；正文不超过 64 KB；无未知变量；必填变量齐全），
  出错会标在对应的输入框上。保存后立即生效，无需重启。
- **发送测试邮件**：把 **当前编辑的内容**（包括还没保存的修改）用示例数据渲染后发到指定邮箱（默认是你自己的邮箱），
  发信方式使用 **已保存** 的邮件设置；发送失败时邮箱输入框下会显示 SMTP 的错误信息。
- **恢复默认**：确认后删除该语言的自定义内容，改回内置模板（只在已自定义时可用）。
- 有未保存的修改时，切换模板、切换语言或离开页面都会先询问。

*Email templates (`/admin/mail-templates`, admins only): each of the six system emails can be rewritten per locale
(zh-CN / en) as a subject plus a Markdown body inside the standard layout. Insert `{{variables}}` from the toolbar (★ =
required in the body), watch the live preview (unknown / missing placeholders are flagged inline), send a test of the
current draft, or restore the built-in template. The legacy `email_template` setting is gone from the UI.*

### 前端外观 `frontend`、客户端下载 `app`

`frontend_accent`、`frontend_background_url`、`frontend_customer_service_url`、`frontend_footer_html` 与
`windows|macos|android|ios_download_url` / `_version` 可以在后台保存。下载地址会出现在公开配置
`GET /api/v1/guest/config` 的 `downloads` 中；**当前版本的前端尚未读取这些外观与下载设置**，保存后页面不会变化。

### 安全 `security`

| 键（界面名称） | 默认 | 说明 |
| --- | --- | --- |
| `passkey_rp_id`（通行密钥 RP ID） | 空 | WebAuthn RP ID（域名，不含端口），空 = `app_url` 的主机名；IP 地址无法使用通行密钥 |
| `passkey_origins`（通行密钥额外来源） | `[]` | 额外允许的来源（`app_url` 的来源始终允许） |
| `admin_mfa_required`（管理员必须启用两步验证） | `false` | 强制管理员 / 客服开启两步验证或通行密钥（§8） |
| `mfa_issuer`（验证器显示名称） | 空 | 身份验证器中显示的发行方，空 = `app_name` |

### Telegram `telegram`

界面分为 **机器人**、**管理员通知**、**用户** 三个小节，最下方的 **连接测试** 有 **发送测试消息** 与 **设置 Webhook** 两个按钮（§9）。

| 键（界面名称） | 默认 | 说明 |
| --- | --- | --- |
| `telegram_bot_token`（Bot Token） | 空 | BotFather 给的机器人 token；为空时 Telegram 通知整体关闭 |
| `telegram_webhook_secret`（Webhook 密钥） | 首次启动自动生成 | Webhook 地址的最后一段（至少 16 位）。点输入框旁的 **重新生成密钥** 并保存（或接口提交 `null`，由服务端生成 32 位新值）后，要重新 **设置 Webhook** |
| `telegram_admin_chat_ids`（管理员 Chat ID） | `[]` | 接收管理员通知的会话：个人或群组的数字 ID（群组、频道为 `-100…`）或 `@频道名`，输入后按回车添加 |
| `telegram_events`（通知事件） | 订单支付、管理员登录、新工单、提现申请、订阅泄露 | 推送给管理员会话的事件，见 §9.4 |
| `telegram_user_notify`（用户通知） | `true` | 已绑定的用户收到自己的支付成功、工单回复、到期与流量提醒 |
| `telegram_discuss_link`（交流群链接） | 空 | 显示在用户的 Telegram 卡片与机器人帮助里 |

---

## 3. 节点接入 / Adding nodes

顺序：**权限组 → 路由规则（可选）→ 节点 → 在服务器上配置后端**。

1. **权限组 / Groups**（`/admin/groups`）：用户购买套餐后获得套餐的权限组，只能使用该组的节点。
2. **路由规则 / Routes**（`/admin/routes`）：下发给 XrayR / V2bX 的路由，动作 `block`（阻断，同时作为 soga
   审计规则）或 `dns`（用指定 DNS 解析匹配的域名）。阻断规则每行一条，所有行用 `|` 连接后必须是合法的 Go 正则。
3. **节点 / Nodes**（`/admin/nodes` → **新建节点**）：协议类型创建后不可更改。

| 字段 | 说明 |
| --- | --- |
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
| --- | --- | --- | --- |
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

XrayR 请求 `/api/v1/server/UniProxy/{config,user,push}`；它忽略面板下发的用户设备数，只使用本地 `DeviceLimit`，也没有连接数限制。它不读取面板下发的拉取间隔，请把
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

V2bX 使用面板下发的拉取 / 上报间隔，并通过 `/api/v1/server/UniProxy/alivelist` 执行设备数限制；它不支持连接数限制
（本地 `ConnLimit` 配置已不再生效）。节点前有中转时见 §3.5。

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
不带前缀的 `/api/v1/...` 同样可用。soga 执行面板下发的限速、设备数与**连接数**（用户的 `conn_limit` 作为 `tcp_limit` 下发），
与本地的 `user_speed_limit`、`user_conn_limit` 取较小值。

### 3.5 中转机后面的节点：设备数与连接数

节点前面有中转机时，后端看到的所有连接都来自中转机的 IP：每个用户无论开几台设备都只算一台，**设备数限制失效**，
节点“在线用户”里的 IP 也都是中转机的 IP。两种办法：

- **让后端拿到真实 IP**：中转机发送 PROXY protocol，V2bX 节点开启 `EnableProxyProtocol`，设备数限制即恢复正常。
- **改用连接数限制**：在套餐或用户上填写 **连接数**（`conn_limit`，每个用户同时的 TCP 连接数，留空或 0 = 不限）。
  只有 soga 支持（`tcp_limit`）；XrayR 与 V2bX 都无法限制连接数。

| 限制 | XrayR | V2bX | soga |
| --- | --- | --- | --- |
| 限速 `speed_limit` | 支持 | 支持 | 支持 |
| 设备数 `device_limit` | 不支持（只用本地 `DeviceLimit`） | 支持（`alivelist`） | 支持 |
| 连接数 `conn_limit` | 不支持 | 不支持 | 支持（`tcp_limit`） |

---

## 4. 套餐、优惠券与支付 / Plans, coupons and payments

### 4.1 套餐 `/admin/plans`

- **价格**：按周期填写（站点货币），留空表示不提供该周期。周期：`monthly`、`quarterly`、`half_yearly`、
  `yearly`、`two_yearly`、`three_yearly`、`onetime`，以及 `reset_traffic`（流量重置包）。
- **权限组**、每周期流量、限速、设备数、连接数（只有 soga 执行，见 §3.5）、容量上限（留空 = 不限）、流量重置方式（留空 = 跟随系统设置）。
  购买（含续费）、试用、后台新建 / 批量生成用户时，套餐的限速、设备数、连接数会复制到用户上；修改套餐不会立即改变已有用户
  （下次付费订单时才按新值），除非使用下面的 `force_update`。
- **显示**（商店可见）/ **销售**（允许新购）/ **续费**（允许已有用户续费）：关闭“销售”的套餐只供老用户续费。
- **同步到现有用户**：编辑有用户持有的套餐时，“流量与限制”末尾的开关打开后保存（确认对话框会写明影响人数），会把权限组、流量额度
  和限制（限速、设备数、连接数）覆盖到所有持有该套餐的用户，含已过期的；单独为用户调整过的值会丢失，已用流量和到期时间不变。
  对应 `PATCH /api/v1/admin/plans/:id` 的 `"force_update": true`。

### 4.2 优惠券 `/admin/coupons`

类型 **固定金额**（按站点货币的“元”填写，接口内部存为分）或 **百分比折扣**（1–100）；可限制总次数、每人次数、适用套餐与周期、
有效期；一次最多批量生成 500 个，留空优惠码则随机生成。

### 4.3 支付方式 `/admin/payments`

**添加支付方式** → 选择网关 → 填写网关配置 → 设置显示名称、图标、手续费（固定 + 百分比，由用户承担）→ 启用。
网关创建后不可更换。

**图标**：在图标网格里点选内置图标，表单会实时预览用户在结算页看到的样子。内置图标保存为 `preset:<id>`：

| 图标 | `<id>` | 图标 | `<id>` |
| --- | --- | --- | --- |
| 支付宝 | `alipay` | 银行转账 | `bank` |
| 微信支付 | `wechat` | 扫码支付 | `qrcode` |
| Stripe | `stripe` | 余额 / 钱包 | `wallet` |
| 银行卡 | `card` | 易支付 | `epay` |
| PayPal | `paypal` | Epusdt | `epusdt` |
| 加密货币 | `crypto` | BTCPay | `btcpay` |
| 比特币 | `btc` | Coinbase | `coinbase` |
| USDT | `usdt` | 以太坊 | `eth` |

新建时会按网关与配置自动选一个，手动点选后不再自动更换：

- 易支付：`type` 为 `alipay` → 支付宝，`wxpay` / `wechat` → 微信支付，`usdt…` → USDT，其他或留空 → 易支付；
- Stripe：`payment_methods` 为 `card` → 银行卡，`alipay` → 支付宝，`wechat_pay` → 微信支付，`auto` → Stripe；
- Epusdt：`currency_type` 留空或 `usdt…` → USDT，其他（如 `tron.trx`）→ 加密货币；
- 当面付 → 支付宝，Coinbase → Coinbase，BTCPay → BTCPay。

选 **自定义 URL** 可填图片地址（`https://…` 或站内 `/…` 路径，建议正方形 SVG/PNG）或单个 Emoji。完整格式见
[API.md › Payment icons](API.md#payment-icons)；从 Xboard 导入的旧图标值不会妨碍修改其他字段。

回调地址（Notify URL）在保存后显示，格式为：

```text
<回调域名 notify_domain，或 app_url>/api/v1/guest/payment/notify/<gateway>/<uuid>
```

`uuid` 每个支付方式唯一；支付完成后买家经 `/api/v1/guest/payment/return/<trade_no>` 回到订单页。

| 网关 `gateway` | 必填配置 | 回调设置 |
| --- | --- | --- |
| 易支付 `epay` | `url`（不含 `/submit.php`）、`pid`、`key`；可选 `type`（`alipay`/`wxpay`/`qqpay` 等） | 下单时自动携带，无需手动配置；仅人民币 |
| Epusdt / BEpusdt `epusdt` | `api_url`、`api_token`；`currency_type` 默认 `usdt.trc20` | 自动携带 |
| 支付宝当面付 `alipay_f2f` | `app_id`、`private_key`（RSA2 ≥ 2048）、`alipay_public_key`（支付宝公钥）；`sandbox` | 自动携带；仅公钥模式、仅人民币 |
| Stripe `stripe` | `secret_key`、`webhook_secret`；可选 `currency`、`exchange_rate`、`payment_methods`（`auto`/`card`/`alipay`/`wechat_pay`） | Dashboard → Developers → Webhooks 添加端点，URL 填回调地址，至少订阅 `checkout.session.completed` 与 `checkout.session.async_payment_succeeded`（`async_payment_failed`、`expired` 可选，只做确认） |
| Coinbase Business `coinbase` | `api_key_name`、`api_key_secret`（ECDSA P-256 PEM）、`webhook_secret`；`exchange_rate`（每单位站点货币对应 USDC） | 用 `cdp data webhooks subscriptions create` 为 `checkout.payment.*`（及可选 `checkout.refund.*`）创建订阅，目标为回调地址；仅以 `checkout.payment.success` 开通 |
| BTCPay Server `btcpay` | `server_url`、`store_id`、`api_key`（`btcpay.store.cancreateinvoice` + `btcpay.store.canviewinvoices`）、`webhook_secret` | Store → Settings → Webhooks 新建，Payload URL 填回调地址；发票状态为 `Settled` 且带面板写入的绑定签名才开通（轮换 Webhook 密钥前创建的未付发票需人工确认） |

Stripe / Coinbase / BTCPay 的回调地址必须是公网 HTTPS。各网关协议细节见 [PAYMENTS_GATEWAYS.md](PAYMENTS_GATEWAYS.md)。

### 4.4 订单 `/admin/orders`

- **筛选**：顶部分段控件按状态切换（全部 / 待支付 / 开通中 / 已取消 / 已完成 / 已折抵），另可按订单号、用户（邮箱或 ID）、
  佣金状态、套餐、支付方式与日期筛选，筛选条件保存在地址栏里。
- **标记为已支付**（立即开通并计算佣金，先在支付平台核实到账）、**取消订单**（退回已抵扣的余额与优惠券次数），仅限待支付订单。
- **佣金状态**（订单详情）：待确认 → **发放中** 表示批准，`commission_settle` 每小时把它发放给邀请人并改为 **有效**；
  **无效** 表示不发放。“有效”只能由定时任务写入（手动选择会被拒绝），已发放的佣金不能再改。
- **分配订单**：为用户（邮箱或 ID）创建一笔待支付订单，金额留空按套餐价格；用户可自行支付，也可由你标记为已支付。

---

## 5. 规则集与模板 / Rule sets and templates

- **系统设置 → 订阅**：`ruleset_default_template`、`ruleset_default_source`、`ruleset_mirror`、
  `allow_user_rule_profile`（见 §2）。
- **规则集 / Rule sets**（`/admin/rulesets`）：标签页 **来源 / 分类 / 模板 / 镜像** 展示内置目录合并覆盖后的生效结果
  （只读）。内置来源：blackmatrix7、MetaCubeX、Loyalsoldier、SagerNet、ACL4SSR、DustinWin；内置模板：
  `none`（全局代理）、`minimal`（国内直连）、`standard`（标准分流）、`full`（精细分流）；内置镜像：`direct`、
  `ghfast`、`gh-proxy`、`ghproxy-net`、`gh-llkk`。
- **自定义覆盖**（第五个标签）：JSON 结构与内置目录相同，`sources` 按键、`categories` / `templates` / `mirrors` 按 `id` 合并，
  同 id 替换、新 id 追加；保存（`PUT /api/v1/admin/rulesets`）后对新生成的订阅立即生效，提交 `null` 恢复内置目录。
  表单可快速添加镜像或自定义分类；新分类的 id 会写进用户的规则配置，之后不要修改。

目录与模板格式见 [RULESETS.md](RULESETS.md)，订阅输出见 [SUBSCRIPTION.md](SUBSCRIPTION.md)。

---

## 6. 用户与内容 / Users and content

- **用户 / Users**：按邮箱搜索，分段控件筛选状态（全部状态 / 正常 / 已封禁），另有套餐、权限组、角色、到期筛选与
  **更多筛选**（用户 ID、邀请人 ID、订阅 Token、备注）。工具栏：**新建用户**、**批量生成**（下载含密码的 CSV）、
  **群发邮件**（发给当前筛选的用户；正文按纯文本发送，空行分段，后台队列异步发送）、**导出 CSV**。
- **用户详情** 分 **概况 / 资料 / 订阅** 三个标签：概况显示套餐、流量、订阅 Token 与短链、绑定的 Telegram Chat ID；
  资料可编辑全部字段；**订阅** 标签显示 age 加密状态与访问记录（见 §10）。每行的更多菜单：**以此用户登录**（一次性链接，
  5 分钟有效，建议在隐私窗口打开；不能用于管理员或客服账户）、**重置订阅密钥**（UUID 与订阅 token 重新生成，用户需重新导入，
  登录会话失效）、**重置流量**、**封禁 / 解除封禁**、删除（连带订单、工单、流量记录、订阅配置与密钥）。
- **工单 / 公告 / 知识库**：客服也可使用。工单列表顶部的分段控件切换 **待回复 / 进行中 / 已关闭 / 全部**，可按用户邮箱或 ID 搜索，
  ↑/↓ 在列表中移动；回复已关闭的工单会重新打开它。公告与知识库用 Markdown 编辑器（编辑 / 预览）。知识库正文支持占位符
  `{{siteName}}`、`{{subscribeUrl}}`、`{{urlEncodeSubscribeUrl}}`、`{{safeBase64SubscribeUrl}}`；工具栏的 **付费可见** 插入的
  `<!--access start-->…<!--access end-->` 之间的内容只对有有效套餐的用户显示。后三个订阅占位符会展开成每位读者自己的订阅地址
  （含 token），**只有管理员可以写入**：客服保存含它们的新正文会被拒绝（403），但可以继续修改管理员文章的标题、分类与显示状态。
- **系统状态 / System**（`/admin/system`）：运行状态、定时任务、邮件队列、缓存与运行日志，每 30 秒刷新。

---

## 7. 从 Xboard 迁移 / Migrating from Xboard

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # 先预演（全部回滚）
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

| 参数 | 说明 |
| --- | --- |
| `--dsn` | 必填：`mysql://…`、`postgres://…` 或 `sqlite:///path` |
| `--driver` | DSN 没有 scheme 时指定 `mysql` / `postgres` / `sqlite` |
| `--dry-run` | 读写全部执行后回滚 |
| `--force` | 先删除 ggboard 已有数据（用户、套餐、节点…）再导入 |
| `--skip-banned` | 不导入被封禁的用户及只属于他们的数据（工单、邀请码、统计…），保留行里指向他们的引用会置空；汇总里显示 `skipped banned N`。注意封禁状态不会带过来，这些邮箱在 ggboard 里可以重新注册；预演、正式导入和 `--force` 重导都要带上这个开关。规则见 IMPORT_XBOARD.md §3.5 |
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
6. 设置：发送测试邮件、复核人机验证密钥；Telegram 设置不导入，按 §9.5 重新填写并设置 Webhook；Xboard 的 `secure_path`
   不再生效，如需隐藏 `/admin` 请在反向代理上限制。
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
2. BotFather 回复的 `123456789:AA…` 就是 token，填入 **系统设置 → Telegram → Bot Token**（`telegram_bot_token`）并保存。
   token 等同于机器人密码：不要截图外传；泄露后在 BotFather 里 `/revoke` 换新。
3. 可选：`/setdescription`、`/setuserpic` 设置简介与头像；`/setcommands` 填入
   `start - 绑定面板账户`、`unbind - 解除绑定`、`id - 查看 Chat ID`。

### 9.2 设置 Webhook

机器人通过 Webhook 接收命令。先确认 **站点 → `app_url`** 是可公网访问的 HTTPS 地址（Telegram 只接受 443、80、88、8443 端口），
然后在 **系统设置 → Telegram → 连接测试** 点 **设置 Webhook**（`POST /api/v1/admin/settings/telegram/webhook`）。面板把
`<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>` 注册给 Telegram（只接收消息更新，并附带
`secret_token`）：成功时显示“Webhook 已设置：<地址>”，失败时显示 Telegram 返回的错误码与描述；缺少 Bot Token 或
`app_url` 时对应字段会标红。Telegram 设置有未保存的修改时两个按钮不可用，先保存。
更换 `app_url`、重新生成 Webhook 密钥或更换 token 后要重新设置一次。同一个 token 同时只能有一个 Webhook：
如果旧面板（如 Xboard）还在用这个机器人，设置后旧面板就收不到命令了。

### 9.3 获取 Chat ID 并测试

1. **个人**：私聊机器人发送 `/id`，机器人回复 `Chat ID：123456789`（`/id` 在任何会话都可用，绑定前也能用）。
2. **群组**：把机器人拉进群，在群里发送 `/id`（多个机器人时发 `/id@你的机器人`），得到 `-100…` 开头的 ID。
3. **频道**：把机器人设为频道管理员，**管理员 Chat ID** 直接填 `@频道用户名`。
4. 把这些 ID 逐个填入 **管理员 Chat ID**（`telegram_admin_chat_ids`，输入后按回车）并保存，然后点 **发送测试消息**
   （`POST …/telegram/test`，同步发送）。页面显示“已发送 N 条，失败 M 条”，并列出每个失败的 Chat 及原因（常见失败：
   `chat not found` = ID 错误或机器人没进群；`bot was blocked` = 被拉黑）。

### 9.4 事件

**通知事件**（`telegram_events`）决定管理员会话收到哪些事件：

| 消息标题 | 键（设置里的名称） | 内容 |
| --- | --- | --- |
| 🧾 新订单 | `order_new`（新订单） | 用户下单：邮箱、套餐、周期、金额、订单号（后台手动分配的订单不推送） |
| 💰 订单已支付 | `order_paid`（订单支付） | 以上 + 支付方式、流水号；**默认开启** |
| 🧾 订单已取消 | `order_cancel`（订单取消） | 超时自动取消或管理员取消（用户自己取消不推送） |
| 🔐 管理员登录 | `admin_login`（管理员登录） | 每次管理员 / 客服登录：邮箱、角色、方式（密码、密码 + 两步验证、密码 + 备用码、通行密钥）、IP、设备（UA）、时间；**默认开启** |
| 🎫 新工单 | `ticket_new`（新工单） | 主题、优先级、用户与首条消息摘要；**默认开启** |
| 🎫 工单有新回复 | `ticket_reply`（工单回复） | 客服的每条回复（本来就发给用户；管理员一般不需要） |
| 💸 提现申请 | `withdraw`（提现申请） | 用户、金额、方式与打码后的账号；**默认开启** |
| 🚨 订阅疑似泄露 | `subscription_leak`（订阅泄露） | 泄露检测命中的用户与不同 IP 数（§10）；**默认开启** |

**登录提醒**：保持 `admin_login` 开启，任何管理员或客服账户每次登录（包括通行密钥登录）都会推送一条消息；
看到不认识的 IP 或设备时，立即修改该账户密码（会注销所有会话）并检查两步验证。普通用户登录不推送。

用户侧（`telegram_user_notify` 开启时，仅限已绑定的用户）：自己的订单支付成功、工单回复、到期提醒与流量提醒。
到期 / 流量提醒随每天 10:00 的提醒任务一起发送，与邮件提醒共用“24 小时一次”的限制；关闭 `remind_mail_enable`
只停邮件，已绑定 Telegram 的用户仍会收到 Telegram 提醒。

发送失败会自动重试 3 次（1 s / 2 s / 4 s，遇到 Telegram 限流按其要求等待），仍失败的记在 **系统状态 → 运行日志**
（WARN 级别）；排队、成功、失败数与启用的渠道见 `GET /api/v1/admin/system/status` 的 `notify` 字段（后台页面暂不显示）。
服务器无法直连 `api.telegram.org` 时通知无法送达，请为服务器配置可用的出站网络。

### 9.5 从 Xboard 迁移

导入器保留每个用户的 `telegram_id`。若沿用 Xboard 原来的机器人 token，已绑定的用户无需重新绑定即可收到通知；
记得按 §9.2 把 Webhook 改到 ggboard。Xboard 的 `telegram_*` 设置不会被导入，需要在这里重新填写：

- **Bot Token**：Xboard 的 `telegram_bot_token`（或在 BotFather 换新）；
- **交流群链接**：Xboard 的 `telegram_discuss_link`；
- **管理员 Chat ID**：Xboard 把管理员通知发给绑定了 Telegram 的管理员账户，ggboard 改为发到这里列出的会话。
  管理员账户的 Chat ID 就是 **用户详情 → 概况** 里的 Telegram 一栏，也可以私聊机器人发 `/id` 获取；
- 按需勾选 **通知事件**，保存后点 **设置 Webhook** 与 **发送测试消息**。

---

## 10. 订阅防泄露 / Subscription protection

在 **系统设置 → 订阅 → 防泄露** 配置（§2）。两项机制互相独立：

**age 加密订阅**：由用户在订阅页自行生成密钥并开启（见 [USER_GUIDE.md](USER_GUIDE.md) §3.4）。开启后只有 Clash Meta（mihomo）
格式会加密；其他格式默认仍给明文。

| 设置 | 作用 |
| --- | --- |
| **允许临时关闭加密** `subscribe_age_optional`（默认开） | 链接加 `&age=0` 时返回明文，便于临时排查 |
| **严格加密** `subscribe_age_strict`（默认关） | 不支持 age 的格式（Stash、sing-box、Surge 等）返回 406，不再退回明文 |

默认组合偏向兼容：泄露的链接仍可以通过 `&age=0` 或换一种格式拿到明文。要真正防泄露，请开启 **严格加密** 并关闭
**允许临时关闭加密**，并提醒已开启加密的用户只使用支持 age 的客户端。设置只影响已开启加密的用户。

**泄露检测**：**泄露检测阈值** `subscribe_leak_max_ips` 大于 0 时生效。同一个订阅 token 在 24 小时内被超过 N 个不同 IP
（IPv6 按 /64 计）获取，同一 token 每 24 小时最多触发一次：

- **触发后** = `notify`（界面“仅通知管理员”）：给用户发一封“订阅链接疑似泄露”邮件，订阅不受影响；
- **触发后** = `rotate`（界面“通知并重置订阅”）：先轮换该用户的订阅 token（UUID 与登录会话不变，旧链接立即失效），
  再把新链接发邮件给用户；
- 两种情况都会写一条 WARN 日志，并在 **通知事件** 勾选了“订阅泄露”时推送到管理员 Telegram（§9.4）。

阈值按家庭或多设备用户的正常 IP 数设置（例如 10–20），太低会误报。访问记录每 10 秒批量写入，保留每个用户最新 100 条、
最长 30 天；按 token 计数，轮换或重置后的新链接重新计数。

**查看单个用户**：**用户 → 用户详情 → 订阅** 标签显示：

- age 加密状态（已开启 / 未开启）与公钥（管理员看不到私钥）；**重置 age 密钥** 为该用户生成新密钥对，开关保持不变，
  用户需要在订阅页查看新私钥并更新客户端（`POST /api/v1/admin/users/:id/subscription/age/reset`）；
- 近 24 小时不同 IP 数与最新 100 条访问记录（完整 IP 与归属地、客户端、格式、Token 标识、时间；
  `GET /api/v1/admin/users/:id/subscription/access-log`），Token 标识相同的行来自同一个订阅链接；
- **轮换订阅链接**：管理端没有单独的 token 轮换接口，这个按钮调用 **重置订阅密钥**，UUID 与 token 一起更换，
  用户需要重新导入订阅，登录会话也会失效。只想换链接时，请让用户在订阅页自己 **轮换订阅链接**。

这些接口与标签仅管理员可用，客服看不到。
