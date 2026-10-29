# 更新日志

[English](CHANGELOG.md) · **简体中文**

这里记录 GG Bond（ggboard）每个版本的重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。推送 `v*` tag 时，GitHub Release 的说明取自 [CHANGELOG.md](CHANGELOG.md)（英文）里对应版本的小节，后面接本文件的同一小节（见 `.github/workflows/release.yml`），所以发版前先在两个文件里都写好该版本。

## [Unreleased]

### 新增

- 节点可以设置自己的**节点密钥**（节点 → 编辑 → 节点密钥；后台节点接口的 `token` 字段，新增 `nodes.token` 列，启动时自动添加）。留空则继续使用共享的 `server_token`；设置后本节点只接受自己的密钥，这个密钥也不能用于其他节点，运营某个节点的人无法再改 `node_id` 读取其他节点的用户与密钥。子节点使用父节点的密钥；多个节点可以共用一个密钥（例如一个 soga 对接 `node_id=1,2,3`）；复制节点会生成新密钥。XrayR、V2bX、soga 无需改动：每个节点配置本来就有自己的密钥项。

### 变更

- README 和面向用户的文档（部署、管理员与用户指南、Xboard 割接手册、性能测试说明、贡献指南、安全策略和本更新日志）各拆成两份：英文 `X.md`（默认显示）和简体中文 `X.zh-CN.md`，顶部互相链接。发布压缩包附带两份 README，Release 说明同时包含英文和中文的更新日志小节。

### 修复

- 群发邮件表单不再提示“支持 Markdown”：内容按纯文本发送，保留换行。
- 用户在早先一次结账（另一种支付方式）的支付页上付款（换了支付方式，或两次结账并发）时，只要金额与该支付方式自己的手续费相符，订单就会自动履约，不再需要管理员手动确认。
- 订阅访问记录写库失败（数据库错误，或读取访问记录的请求中途断开）时，这批记录会放回缓冲区等下次写入，不再丢失（泄露检测的输入也随之保留）。
- Telegram 消息按转义后的长度截断：工单主题或正文里全是 `&`、`<` 时，给管理员的提醒会超过 Telegram 的长度上限而被丢弃。
- 节点页的设置入口与 Xboard 导入报告的检查清单改用设置分组的实际名称：系统设置 → 节点通讯（导入报告为英文 Settings → Node connection）。
- 文档：从 Xboard 导入时 `server_token` 至少 16 个字符才会保留；订阅疑似泄露提醒也会通过 Telegram 发给已绑定的用户；管理员指南里泄露处理选项的名称与后台一致。

### 安全

- 节点的流量、在线与审计上报只对该节点服务的用户（本节点及其子节点的权限组）生效。此前持有节点密钥的人可以替任意用户上报流量、耗尽其额度。
- `POST /user/coupons/check` 对隐藏套餐的应答与不存在的套餐完全相同，无法再枚举隐藏套餐的 ID 与周期；创建订单也先检查可见性。
- 订阅查询参数 `cats` 与 `types`、`filter` 一样最多 64 项，不再为每个未知分类各返回一行错误。
- 验签不通过的支付回调按 IP 限流（60 次/分钟，突发 120；用完后在查询支付方式之前直接返回 429）；验签通过的回调从不受限。
- 展示给用户的工单消息不再带客服账号的 ID（客服作者显示为 `0`）。
- 用户取消订单只会更新自己的订单，不再依赖事务回滚兜底。
- 文档：如何让反向代理的访问日志不记录节点密钥与订阅 token（`deploy/nginx.example.conf` 已这样配置），以及 ggboard 只运行单个实例（限流与在线状态都在进程内）。

## [1.0.1] - 2026-10-28

以安全加固为主的补丁版本，另有两个小功能。配置项与数据库结构都没有变化，换镜像或二进制后重启即可。

### 新增

- 订阅访问记录显示 IP 归属地（`中国 广东 深圳 电信`，后台与用户中心都有）：数据来自纯真 IP 库（metowolf/qqwry.dat，约 27 MB，仅 IPv4），存放在 `<data_dir>/geoip/`，每天 05:23 自动检查更新（`geoip_update`），下载失败或文件无效时保留旧库；设置 → 订阅 → IP 归属地可以关闭、换镜像地址，查看版本并「立即更新」（`GET /admin/geoip`、`POST /admin/geoip/update`）。
- 管理员「重置订阅密钥」（用户详情里的按钮与订阅标签的「轮换订阅链接」）后，系统会发邮件并在用户绑定了 Telegram 时推送消息，告诉用户订阅链接和 UUID 已被重置、旧链接失效、已退出登录，请重新登录网站获取新链接；通知里不含新链接。新增可在 **邮件模板** 页面自定义的 `subscription_reset` 模板与只发给用户的 Telegram 事件 `subscription_reset`；邮件排队失败只记 WARN，不影响重置。用户自己轮换 / 重置订阅和泄露检测的自动轮换不发这条通知。

### 修复

- 流量排行的日期跟随服务器的统计日（进程时区 `TZ`），不再按浏览器所在时区取默认值和上限；`GET /admin/stats/overview` 新增 `today.date` 与 `timezone`，排行标题旁的 ⓘ 显示服务器时区。
- 订单列表的日期筛选按管理员本地时区的整天发送起止时刻，筛选结果与列表里显示的时间一致。

### 安全

- 安全变更对在途请求立即生效：改密、重置订阅、封禁、管理员降权之后，已经通过鉴权的请求在写入和返回之前会重新核对账号（UUID、邮箱、安全版本与角色），过期会话一律 401；订单、工单、邀请与佣金、订阅配置、两步验证、通行密钥、age 密钥、Telegram 绑定与后台各管理接口都不会再读写同一用户 ID 下替换后的账号数据。
- 订阅生成期间 token、套餐、分组、流量或封禁状态发生变化时，丢弃这次结果并返回 403，不再下发旧的节点配置。
- 支付回调：每个成功回调都必须带正数金额并与订单应付额一致，Stripe 与 Coinbase 按当前汇率和币种核对（非 1:1 汇率时不再跳过）；验签之后、履约之前订单报价被并发修改时不履约。易支付严格校验回调地址并拒绝商户请求字段，防止未设置 `app_url` 时把下单签名当作支付成功回调。
- 整数溢出：超大的套餐价格、折扣、流量（GB 换算）与已用流量之和不再绕回负数，不会被当作免费订单，也不会让流量已用尽的用户重新获得节点权限。
- 未匹配路由（404）在访问日志里记为 `/[unmatched]`，拼错的订阅或 webhook 地址里的密钥不会写进日志。
- Xboard 导入器把从源库元数据读到的表名作为标识符引用，不再直接拼进 SQL。
- `golang.org/x/net` 升级到 v0.62.0（v0.61.0 的两个已修复漏洞，ggboard 的代码路径不受影响）。

## [1.0.0] - 2026-10-25

首个正式版本：单个 Go 二进制，内嵌 React 前端、定时任务、邮件队列与进程内缓存；默认 SQLite，也支持 PostgreSQL 与 MySQL，不需要 Redis 或额外的 worker。

### 节点对接

- XrayR（`PanelType: NewV2board`，即 `UniProxy` 接口）、V2bX（同一套接口的超集）与 soga（`soga-v1` webapi，推荐 `/soga/` 前缀），节点用后台生成的 `server_token` 认证。
- 协议：vmess、vless（含 REALITY / XHTTP）、shadowsocks（含 2022）、shadowsocksr、trojan、hysteria 1/2、tuic、anytls、mieru。
- 用户列表与节点配置走缓存并支持 `ETag` / 304，gzip 后的响应一并缓存；流量上报批量写入。
- 每个用户的限速、设备数（V2bX、soga）与连接数 `conn_limit`（下发为 soga 的 `tcp_limit`），可在套餐或用户上设置。

### 订阅

- 一条订阅链接按 `flag` 参数或客户端 User-Agent 输出 mihomo（Clash Meta）、Clash、Stash、sing-box、Surge、Surfboard、Quantumult X、Shadowrocket、Loon、v2rayN（base64）与 SSR 格式，用户页提供各客户端一键导入。
- 用户自选分流：模板（无规则 / 精简 / 标准 / 精细）、规则集来源（blackmatrix7、Loyalsoldier、MetaCubeX、SagerNet、ACL4SSR、DustinWin）、分类（OpenAI、Claude、Telegram、Netflix…）、策略组与 GitHub 镜像。所选来源没有某客户端的格式时按分类自动换用其他来源，编辑器里列出每个客户端实际使用的来源；sing-box 1.14+ 通过 `http_clients` 下载规则集。管理员可在后台覆盖内置的规则集目录。
- 多套订阅配置（每人最多 10 套，各有独立链接 `?profile=<slug>`）与参考 Sub-Store 的节点处理：按类型 / 地区 / 正则筛选、去重、正则重命名、旗帜 emoji、前后缀、排序、数量上限，带实时预览。
- 订阅防泄露：mihomo 1.19.27+ 的 age 加密订阅（可选严格模式）；订阅访问日志（IP、客户端、格式）；同一 token 24 小时内的 IP 数超限时邮件 + Telegram 告警或自动轮换链接；“只换链接”与“重置订阅”分开。
- Shadowsocks 2022：AES 加密下发 `服务端密钥:用户密钥`，ChaCha20 加密只下发用户密钥，sing-box 与 mihomo 都能加载。

### 套餐与支付

- 套餐按周期定价（含一次性与流量重置包），订单、优惠券（固定金额或百分比，可批量生成）、邀请返利与佣金（提现或转入余额）。
- 可插拔的支付网关，回调一律验签：易支付（EPay）、Epusdt / BEpusdt、支付宝当面付、Stripe Checkout、Coinbase Business、BTCPay Server。支付方式可选 16 个原创预设图标或自定义图片 / emoji。金额以“分”存储，后台按“元”填写。

### 用户与安全

- 密码使用 argon2id；从 Xboard 导入的 bcrypt / md5 / sha256 哈希可以继续登录，并在登录时升级。同时进行的 argon2id 计算由 `auth.argon2_concurrency` 限制，登录高峰的内存有上限。
- 两步验证（TOTP + 备用码，可强制管理员开启）与通行密钥（WebAuthn，可无用户名登录，多设备管理）。
- 登录、注册、找回与改密表单兼容 1Password、Bitwarden / Vaultwarden、iCloud 钥匙串与 Chrome 的自动填充，并提供 `/.well-known/change-password` 与 `/.well-known/passkey-endpoints`。
- 注册：邮箱验证码、注册开关、邮箱后缀白名单，可选 Turnstile / reCAPTCHA / hCaptcha。
- 会话与账户邮箱绑定，改密或重置密码后旧会话失效；启动时拒绝过短或占位的 `jwt.secret`；登录、注册与发送验证码按 IP 限流。
- Telegram 机器人：新订单 / 支付、工单、提现申请与订阅泄露推送给管理员，每次管理员 / 客服登录都推送提醒（邮箱、方式、IP、UA、时间）；用户绑定后接收自己的支付、工单回复与到期 / 流量提醒。

### 管理后台

- 仪表盘与统计；用户管理：多条件筛选、批量生成、群发邮件、导出 CSV、以此用户登录、重置订阅或流量、封禁。
- 节点（拖拽排序）、权限组、路由、套餐（可“同步到现有用户”，即 `force_update`）、订单、优惠券、支付方式、工单、公告与知识库（Markdown，支持付费可见内容与订阅地址占位符）。
- 邮件模板：六封系统邮件的标题与 Markdown 正文可按简体中文 / English 分别改写，带变量与实时预览。
- 系统状态：运行状态、定时任务、邮件队列、缓存与最近 1000 条日志。
- 前端：React 19、HeroUI v3、TanStack（Router / Query / Table / Form / Virtual）与 zustand；Apple HIG 设计语言，深浅色自动切换，中文 / English；说明收进 ⓘ 弹出层；公开页面在构建时预渲染，Lighthouse 四项 100（桌面与移动端）。

### 从 Xboard 迁移

- `ggboard import xboard` 支持旧版分表与上游统一表两种 schema，源库可以是 MySQL、PostgreSQL 或 SQLite；导入用户、套餐、节点、优惠券、工单、公告、知识库、支付配置与设置，不导入历史订单。
- 保留原主键、密码哈希、订阅 token、UUID、`server_token`（至少 16 个字符时）与 `telegram_id`，现有订阅链接与节点后端不用改；`--dry-run` 预演，`--skip-banned` 跳过已封禁的用户，`--with-stats` 一并导入流量统计。

### 部署与运维

- 每个 `v*` tag 由 GitHub Actions 发布 linux（amd64、arm64）、darwin（amd64、arm64）与 windows/amd64 的二进制压缩包和 `SHA256SUMS`，以及多架构镜像 `ghcr.io/linusxiong/ggboard`（本版本为 `1.0.0`、`1.0`、`1` 与 `latest`）。
- Docker：distroless 镜像，以 nonroot 运行；`deploy/docker-compose.prebuilt.yml`（预构建镜像）与 `deploy/docker-compose.yml`（从源码构建）。另有加固过的 systemd unit 与 nginx 示例。
- 命令行：`serve`、`migrate`、`admin create | reset-password | promote`、`import xboard`、`db copy`、`version`；在终端里日志与输出彩色对齐，用 `--color` 或 `NO_COLOR` 控制。
- `ggboard db copy` 把当前数据库整库复制到另一个库（SQLite → PostgreSQL，也能复制回 SQLite；MySQL 同样可用），保留主键和每一列，在一个事务里写入，提交前核对行数与每一行的 SHA-256 并重置 PostgreSQL 序列，`--dry-run` 预演；步骤见 `docs/DEPLOY.md` §5。PostgreSQL 读回的时间与 SQLite、MySQL 一样是 UTC，与进程时区无关。
- 定时任务：订单超时、佣金结算、流量重置、到期与流量提醒、每日统计；`GET /healthz` 用于存活探针。
- `make bench`：可复现的压测（10,000 个用户、50 个节点），结果见 `docs/perf/`。

[Unreleased]: https://github.com/linusxiong/ggboard/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.1
[1.0.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.0
