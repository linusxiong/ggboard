# 更新日志 / Changelog

这里记录 GG Bond（ggboard）每个版本的重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。推送 `v*` tag 时，GitHub Release 的说明就取自下面对应版本的小节（见 `.github/workflows/release.yml`），所以发版前先在这里写好该版本。

*Notable changes per version, in [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format with [Semantic Versioning](https://semver.org/). Each tag's GitHub release notes are the matching section below.*

## [Unreleased]

## [1.0.0] - 2026-10-25

首个正式版本：单个 Go 二进制，内嵌 React 前端、定时任务、邮件队列与进程内缓存；默认 SQLite，也支持 PostgreSQL 与 MySQL，不需要 Redis 或额外的 worker。

*First stable release: one Go binary with the React SPA, scheduler, mail queue and cache built in. SQLite by default, PostgreSQL and MySQL supported, no Redis.*

### 节点对接 / Node backends

- XrayR（`PanelType: NewV2board`，即 `UniProxy` 接口）、V2bX（同一套接口的超集）与 soga（`soga-v1` webapi，推荐 `/soga/` 前缀），节点用后台生成的 `server_token` 认证。
- 协议：vmess、vless（含 REALITY / XHTTP）、shadowsocks（含 2022）、shadowsocksr、trojan、hysteria 1/2、tuic、anytls、mieru。
- 用户列表与节点配置走缓存并支持 `ETag` / 304，gzip 后的响应一并缓存；流量上报批量写入。
- 每个用户的限速、设备数（V2bX、soga）与连接数 `conn_limit`（下发为 soga 的 `tcp_limit`），可在套餐或用户上设置。

### 订阅 / Subscriptions

- 一条订阅链接按 `flag` 参数或客户端 User-Agent 输出 mihomo（Clash Meta）、Clash、Stash、sing-box、Surge、Surfboard、Quantumult X、Shadowrocket、Loon、v2rayN（base64）与 SSR 格式，用户页提供各客户端一键导入。
- 用户自选分流：模板（无规则 / 精简 / 标准 / 精细）、规则集来源（blackmatrix7、Loyalsoldier、MetaCubeX、SagerNet、ACL4SSR、DustinWin）、分类（OpenAI、Claude、Telegram、Netflix…）、策略组与 GitHub 镜像。所选来源没有某客户端的格式时按分类自动换用其他来源，编辑器里列出每个客户端实际使用的来源；sing-box 1.14+ 通过 `http_clients` 下载规则集。管理员可在后台覆盖内置的规则集目录。
- 多套订阅配置（每人最多 10 套，各有独立链接 `?profile=<slug>`）与参考 Sub-Store 的节点处理：按类型 / 地区 / 正则筛选、去重、正则重命名、旗帜 emoji、前后缀、排序、数量上限，带实时预览。
- 订阅防泄露：mihomo 1.19.27+ 的 age 加密订阅（可选严格模式）；订阅访问日志（IP、客户端、格式）；同一 token 24 小时内的 IP 数超限时邮件 + Telegram 告警或自动轮换链接；“只换链接”与“重置订阅”分开。
- Shadowsocks 2022：AES 加密下发 `服务端密钥:用户密钥`，ChaCha20 加密只下发用户密钥，sing-box 与 mihomo 都能加载。

### 套餐与支付 / Plans and payments

- 套餐按周期定价（含一次性与流量重置包），订单、优惠券（固定金额或百分比，可批量生成）、邀请返利与佣金（提现或转入余额）。
- 可插拔的支付网关，回调一律验签：易支付（EPay）、Epusdt / BEpusdt、支付宝当面付、Stripe Checkout、Coinbase Business、BTCPay Server。支付方式可选 16 个原创预设图标或自定义图片 / emoji。金额以“分”存储，后台按“元”填写。

### 用户与安全 / Accounts and security

- 密码使用 argon2id；从 Xboard 导入的 bcrypt / md5 / sha256 哈希可以继续登录，并在登录时升级。同时进行的 argon2id 计算由 `auth.argon2_concurrency` 限制，登录高峰的内存有上限。
- 两步验证（TOTP + 备用码，可强制管理员开启）与通行密钥（WebAuthn，可无用户名登录，多设备管理）。
- 登录、注册、找回与改密表单兼容 1Password、Bitwarden / Vaultwarden、iCloud 钥匙串与 Chrome 的自动填充，并提供 `/.well-known/change-password` 与 `/.well-known/passkey-endpoints`。
- 注册：邮箱验证码、注册开关、邮箱后缀白名单，可选 Turnstile / reCAPTCHA / hCaptcha。
- 会话与账户邮箱绑定，改密或重置密码后旧会话失效；启动时拒绝过短或占位的 `jwt.secret`；登录、注册与发送验证码按 IP 限流。
- Telegram 机器人：新订单 / 支付、工单、提现申请与订阅泄露推送给管理员，每次管理员 / 客服登录都推送提醒（邮箱、方式、IP、UA、时间）；用户绑定后接收自己的支付、工单回复与到期 / 流量提醒。

### 管理后台 / Admin panel

- 仪表盘与统计；用户管理：多条件筛选、批量生成、群发邮件、导出 CSV、以此用户登录、重置订阅或流量、封禁。
- 节点（拖拽排序）、权限组、路由、套餐（可“同步到现有用户”，即 `force_update`）、订单、优惠券、支付方式、工单、公告与知识库（Markdown，支持付费可见内容与订阅地址占位符）。
- 邮件模板：六封系统邮件的标题与 Markdown 正文可按简体中文 / English 分别改写，带变量与实时预览。
- 系统状态：运行状态、定时任务、邮件队列、缓存与最近 1000 条日志。
- 前端：React 19、HeroUI v3、TanStack（Router / Query / Table / Form / Virtual）与 zustand；Apple HIG 设计语言，深浅色自动切换，中文 / English；说明收进 ⓘ 弹出层；公开页面在构建时预渲染，Lighthouse 四项 100（桌面与移动端）。

### 从 Xboard 迁移 / Migrating from Xboard

- `ggboard import xboard` 支持旧版分表与上游统一表两种 schema，源库可以是 MySQL、PostgreSQL 或 SQLite；导入用户、套餐、节点、优惠券、工单、公告、知识库、支付配置与设置，不导入历史订单。
- 保留原主键、密码哈希、订阅 token、UUID、`server_token` 与 `telegram_id`，现有订阅链接与节点后端不用改；`--dry-run` 预演，`--skip-banned` 跳过已封禁的用户，`--with-stats` 一并导入流量统计。

### 部署与运维 / Deployment and operations

- 每个 `v*` tag 由 GitHub Actions 发布 linux（amd64、arm64）、darwin（amd64、arm64）与 windows/amd64 的二进制压缩包和 `SHA256SUMS`，以及多架构镜像 `ghcr.io/linusxiong/ggboard`（本版本为 `1.0.0`、`1.0`、`1` 与 `latest`）。
- Docker：distroless 镜像，以 nonroot 运行；`deploy/docker-compose.prebuilt.yml`（预构建镜像）与 `deploy/docker-compose.yml`（从源码构建）。另有加固过的 systemd unit 与 nginx 示例。
- 命令行：`serve`、`migrate`、`admin create | reset-password | promote`、`import xboard`、`db copy`、`version`；在终端里日志与输出彩色对齐，用 `--color` 或 `NO_COLOR` 控制。
- `ggboard db copy` 把当前数据库整库复制到另一个库（SQLite → PostgreSQL，也能复制回 SQLite；MySQL 同样可用），保留主键和每一列，在一个事务里写入，提交前核对行数与每一行的 SHA-256 并重置 PostgreSQL 序列，`--dry-run` 预演；步骤见 `docs/DEPLOY.md` §5。PostgreSQL 读回的时间与 SQLite、MySQL 一样是 UTC，与进程时区无关。
- 定时任务：订单超时、佣金结算、流量重置、到期与流量提醒、每日统计；`GET /healthz` 用于存活探针。
- `make bench`：可复现的压测（10,000 个用户、50 个节点），结果见 `docs/perf/`。

[Unreleased]: https://github.com/linusxiong/ggboard/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.0
