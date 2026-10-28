<p align="center">
  <img src="web/public/favicon.svg" width="88" alt="GG Bond" />
</p>
<h1 align="center">GG Bond · ggboard</h1>
<!-- screenshots: see docs/screenshots -->
<p align="center">简洁、美观、高性能的代理面板。单个 Go 二进制，内嵌 React 前端。</p>

<p align="center">
  <a href="https://github.com/linusxiong/ggboard/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/linusxiong/ggboard"></a>
  <a href="LICENSE"><img alt="GPL-3.0" src="https://img.shields.io/badge/license-GPL--3.0-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white">
  <img alt="Lighthouse" src="https://img.shields.io/badge/Lighthouse-100%20%C2%B7%20100%20%C2%B7%20100%20%C2%B7%20100-brightgreen">
</p>

<p align="center"><a href="README.md">English</a> · <b>简体中文</b></p>

> **状态：1.0 正式版** — 每个版本的变更见 [CHANGELOG.zh-CN.md](CHANGELOG.zh-CN.md)，二进制与镜像见 [Releases](https://github.com/linusxiong/ggboard/releases)；
> 升级前请先备份 `data/`。

---

## 特性

- **节点对接**：XrayR / V2bX（`UniProxy` 接口）与 soga（`soga-v1` webapi）。
- **全协议**：vmess、vless（含 REALITY / XHTTP）、shadowsocks（含 2022）、shadowsocksr、trojan、hysteria 1/2、anytls、mieru、tuic。
- **订阅与分流自选**：用户在面板里选择模板（无规则 / 精简 / 标准 / 精细）、规则集来源
  （blackmatrix7、Loyalsoldier、MetaCubeX、SagerNet、ACL4SSR、DustinWin）、分类（OpenAI、Claude、Telegram、Netflix…）、
  策略组和 GitHub 镜像；一条订阅链接即可输出 mihomo / Clash / Stash / sing-box / Surge / Surfboard / Quantumult X /
  Shadowrocket / Loon / v2rayN 等格式。
- **多套订阅配置与节点处理**（参考 Sub-Store）：每个用户最多 10 套配置，各有独立链接（`?profile=<slug>`）、分流规则与
  节点处理流水线——按类型 / 地区（47 个地区，旗帜 → 名称 → 代码识别）/ 正则筛选、去重、正则重命名、旗帜 emoji、
  前后缀、排序、数量上限，并带实时预览。
- **订阅防泄露**：mihomo 1.19.27+ 的 **age 加密订阅**（Clash Party、FlClash 可直接填私钥）、可选严格模式；订阅访问日志
  （IP 与归属地、客户端、格式；归属地来自每日自动更新的纯真 IP 库）；同一 token 24 小时内 IP 数超限时邮件 + Telegram 告警，或自动轮换链接；“只换链接”与“重置订阅”分开。
- **Telegram 机器人**（可插拔通知渠道）：新订单 / 支付 / 取消、新工单 / 回复、提现申请、订阅泄露推送给管理员；
  **每次管理员 / 客服登录都推送提醒**（邮箱、方式、IP、UA、时间）；用户用一次性绑定码绑定后接收自己的支付、工单回复与
  到期 / 流量提醒。
- **真实支付，插件化网关**：易支付（EPay）、Epusdt/BEpusdt、支付宝当面付、Stripe Checkout、Coinbase Business、BTCPay Server。
  新增网关只需实现一个 Go 接口并注册；支付方式可选 16 个原创预设图标（支付宝、微信、USDT、Stripe…）或自定义图片 / emoji。
- **账户安全**：argon2id 密码、TOTP 两步验证 + 备用码、Passkey（WebAuthn，支持无用户名一键登录），
  完整适配 1Password / Bitwarden / iCloud 钥匙串等密码管理器自动填充；注册可要求邮箱验证码、限定邮箱后缀、关闭注册，
  可选 Turnstile / reCAPTCHA / hCaptcha；可强制管理员两步验证。
- **从 Xboard 迁移**：一条命令导入用户、套餐、节点、优惠券、工单、公告、知识库、支付配置与设置（不导入历史订单），
  同时支持旧版分表 schema 与上游统一表 schema，MySQL / PostgreSQL / SQLite 均可。
- **运营功能**：套餐 / 订单 / 优惠券 / 佣金与邀请 / 工单 / 公告 / 知识库 / 统计 / 定时任务 / 邮件提醒。
- **前端**：React 19 + HeroUI v3 + TanStack（Router / Query / Table / Form）+ zustand，Apple HIG 设计语言
  （系统色、iOS 分段控件、分组表单、底部保存栏），浅色 / 深色自动切换，中英文；动效组件全部原创；公开页面构建时预渲染，
  Lighthouse 四项 100 分（桌面与移动端）。
- **零依赖部署**：SQLite 默认即可运行，无需 Redis / 队列进程；也支持 PostgreSQL 与 MySQL，`ggboard db copy`
  把已有数据整库复制过去并逐行校验（也能复制回来），见 [DEPLOY.zh-CN.md §5](docs/DEPLOY.zh-CN.md#从-sqlite-迁移到-postgresql)。

## 性能

单个进程、默认 SQLite、10,000 个用户、50 个节点，压测工具与面板在同一台机器上走回环网络（无 TLS、无反向代理），
每个场景 64 个连接（登录 16 个）。数字来自一次真实运行，`make bench` 可以复现。

<p align="center"><img src="docs/perf/benchmark.svg" width="860" alt="每个场景的每秒请求数与 p99 延迟"></p>

| 场景 | 吞吐（req/s） | p99 |
| --- | ---: | ---: |
| 订阅 · v2ray（base64） | 13,208 | 19 ms |
| 订阅 · mihomo + standard 分流规则（18 个规则集） | 13,322 | 18 ms |
| 节点拉用户列表，ETag 命中 → 304 | 86,836 | 2.5 ms |
| 节点拉完整用户列表（10,000 人，gzip） | 29,794 | 6.1 ms |
| 节点上报流量（每次 1,000 人，即每秒约 428 万条） | 4,282 | 24 ms |
| 密码登录（argon2id，64 MiB，有意为之的慢） | 104 | 164 ms |

测试机：Apple M5 Pro（18 核）· 48 GB · macOS 27.0.1 · Go 1.27.3 · 2026-10-23 · commit `8db6777`。
方法、全部场景（含静态文件）、注意事项和发现见 [docs/perf/README.zh-CN.md](docs/perf/README.zh-CN.md)。

## 截图

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/landing-light.png" alt="首页（浅色）"></td>
    <td width="50%"><img src="docs/screenshots/landing-dark.png" alt="首页（深色）"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/login-light.png" alt="登录"></td>
    <td><img src="docs/screenshots/dashboard-light.png" alt="用户仪表盘"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/subscription-profiles-light.png" alt="订阅配置：链接、一键导入与分段标签"></td>
    <td><img src="docs/screenshots/subscription-nodeops-light.png" alt="节点处理与实时预览"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/subscription-rules-light.png" alt="分流规则编辑器"></td>
    <td><img src="docs/screenshots/subscription-age-light.png" alt="订阅安全（访问日志、轮换链接）与 age 加密"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/shop-light.png" alt="购买套餐"></td>
    <td><img src="docs/screenshots/orders-light.png" alt="我的订单（分段筛选）"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/order-light.png" alt="订单详情"></td>
    <td><img src="docs/screenshots/admin-dashboard-light.png" alt="管理后台概览"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-node-editor-light.png" alt="节点编辑（VLESS REALITY）"></td>
    <td><img src="docs/screenshots/admin-payments-icons-light.png" alt="支付方式与预设图标"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-settings-telegram-light.png" alt="系统设置 · Telegram"></td>
    <td><img src="docs/screenshots/admin-settings-register-light.png" alt="系统设置 · 注册与登录"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/dashboard-dark.png" alt="用户仪表盘（深色）"></td>
    <td><img src="docs/screenshots/subscription-profiles-dark.png" alt="订阅配置（深色）"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/admin-dashboard-dark.png" alt="管理后台概览（深色）"></td>
    <td><img src="docs/screenshots/admin-node-editor-dark.png" alt="节点编辑（深色）"></td>
  </tr>
</table>

全部截图（1440×900，浅色与深色）见 [`docs/screenshots/`](docs/screenshots/)：基础场景由
`node web/scripts/screenshots.mjs` 对运行中的演示实例（`scripts/e2e-full.sh` 填充数据）生成，订阅配置 / 节点处理 / age /
订单筛选 / 支付图标 / Telegram / 注册设置几张是同样方式补拍的。截图里只有 `example.com` 演示账号，订阅 token 与密钥均已遮盖。

## 快速开始

### Docker Compose · 预构建镜像（推荐，不用克隆、不用编译）

```bash
mkdir ggboard && cd ggboard
curl -fsSLO https://raw.githubusercontent.com/linusxiong/ggboard/main/deploy/docker-compose.prebuilt.yml
curl -fsSL https://raw.githubusercontent.com/linusxiong/ggboard/main/.env.example -o .env
echo 'GGBOARD_VERSION=1.0.1' >> .env       # 镜像 tag：固定到一个发布版本；main 是 main 分支的滚动构建
docker compose -f docker-compose.prebuilt.yml up -d
docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
```

不带 `--password` 时会生成随机密码并只打印一次。`.env` 必须和 compose 文件放在同一个目录；`GGBOARD_JWT_SECRET` 留空会自动生成。
面板只监听 `127.0.0.1:8080`，请用 nginx / Caddy 反向代理并启用 HTTPS（见 [docs/DEPLOY.zh-CN.md](docs/DEPLOY.zh-CN.md)）。
每个 `v*` tag 由[发布工作流](.github/workflows/release.yml)构建镜像（amd64 + arm64），发布在 `ghcr.io/linusxiong/ggboard`
（如 `1.0.0`、`1.0`、`1`、`latest`）；升级时改 `GGBOARD_VERSION`，再 `pull` 和 `up -d`。

### Docker Compose · 从源码构建

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env            # GGBOARD_JWT_SECRET 留空会自动生成；自填须 32+ 字节随机值（openssl rand -hex 32）
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com --password 'change-me'
```

面板监听 `127.0.0.1:8080`，请用 nginx / Caddy 反向代理并启用 HTTPS（示例：`deploy/nginx.example.conf`），
并把代理地址写进 `server.trusted_proxies`。完整部署（两种 Compose、systemd、TLS、备份、升级、PostgreSQL / MySQL）见
[docs/DEPLOY.zh-CN.md](docs/DEPLOY.zh-CN.md)。

### 二进制 · Release

[Releases](https://github.com/linusxiong/ggboard/releases) 页面提供 linux / darwin（amd64、arm64）与 windows/amd64 的压缩包和
`SHA256SUMS`，二进制内嵌前端、静态链接。下载、校验并装成 systemd 服务的步骤见
[docs/DEPLOY.zh-CN.md §2](docs/DEPLOY.zh-CN.md#二进制安装release)。

### 二进制 · 从源码构建

```bash
make web && make build          # 构建前端并嵌入，产物 bin/ggboard
./bin/ggboard admin create --email admin@example.com --password 'change-me'
./bin/ggboard serve             # 默认 :8080，数据在 ./data
```

配置通过 `config.yaml`（参考 `config.example.yaml`）或环境变量 `GGBOARD_*` 提供；站点名称、邮件、支付等运行时设置在后台“设置”里修改。

## 节点后端接入

后台 **设置 → 节点通讯** 可看到 `server_token`；先在 **节点** 页创建节点，拿到节点 ID。

**XrayR**（面板类型 `NewV2board`，即 `UniProxy` 接口）：

```yaml
Nodes:
  - PanelType: "NewV2board"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: "<server_token>"
      NodeID: 1
      NodeType: Vless        # V2ray | Vless | Trojan | Shadowsocks
      EnableVless: true      # 仅 vless 节点需要
```

**V2bX**（同一套 `UniProxy` 接口，另支持 hysteria / hysteria2 / tuic / anytls）：

```json
{ "Core": "sing", "ApiHost": "https://panel.example.com", "ApiKey": "<server_token>", "NodeID": 1, "NodeType": "vless" }
```

**soga**（协议 `soga-v1`，推荐 `/soga/` 前缀）：

```bash
soga config type=soga-v1 server_type=vless node_id=1 webapi_url=https://panel.example.com/soga/ webapi_key=<server_token>
```

协议与各后端类型对照、完整配置片段见 [docs/ADMIN_GUIDE.zh-CN.md](docs/ADMIN_GUIDE.zh-CN.md)，接口细节见 [docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md)。

## 从 Xboard 迁移

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # 先预演
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

导入保留原主键、密码哈希（bcrypt / md5 / sha256 均可继续登录并自动升级为 argon2id）、订阅 token、UUID 与用户的 `telegram_id`，因此现有订阅链接无需改动；`server_token` 至少 16 个字符时原样导入，节点后端也不用改，
更短时不导入、沿用 ggboard 自己生成的 token，导入报告会提示把它填进每个节点后端。Xboard 的 Telegram 设置不导入：迁移后在
**设置 → Telegram** 填回同一个机器人的 token 并点“设置 Webhook”。细节与迁移后检查清单见
[docs/IMPORT_XBOARD.md](docs/IMPORT_XBOARD.md)，一次真实割接的完整命令序列见 [docs/MIGRATION_TERABIX.zh-CN.md](docs/MIGRATION_TERABIX.zh-CN.md)。

## 文档

语言一栏：**中英两版** 指另有英文版（每篇顶部可切换）；**英文**、**中文** 指只有这一种语言。

| 文档 | 内容 | 语言 |
| --- | --- | --- |
| [AGENTS.md](AGENTS.md) | 项目约定、技术栈、设计规范、可插拔点 | 中文 |
| [CHANGELOG.zh-CN.md](CHANGELOG.zh-CN.md) | 每个版本的变更（也是 GitHub Release 的说明） | 中英两版 |
| [docs/DEPLOY.zh-CN.md](docs/DEPLOY.zh-CN.md) | 部署：Docker Compose、systemd、nginx / Caddy、配置参考、备份与升级 | 中英两版 |
| [docs/ADMIN_GUIDE.zh-CN.md](docs/ADMIN_GUIDE.zh-CN.md) | 管理员指南：设置、节点对接、套餐与支付（预设图标）、Telegram 机器人、订阅防泄露、规则集、2FA 策略 | 中英两版 |
| [docs/USER_GUIDE.zh-CN.md](docs/USER_GUIDE.zh-CN.md) | 用户指南：各客户端一键导入、订阅配置与节点处理、age 加密、访问日志、Telegram 绑定、2FA / Passkey、密码管理器 | 中英两版 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架构、启动流程、缓存、定时任务、安全 | 英文 |
| [docs/API.md](docs/API.md) | 全部 HTTP 接口 | 英文 |
| [docs/DATA_MODEL.md](docs/DATA_MODEL.md) | 数据模型 | 英文 |
| [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md) · [docs/RULESETS.md](docs/RULESETS.md) | 订阅格式、多套配置、节点处理与地区表、age 加密与泄露检测、规则集目录与模板 | 英文 |
| [docs/PAYMENTS.md](docs/PAYMENTS.md) · [docs/PAYMENTS_GATEWAYS.md](docs/PAYMENTS_GATEWAYS.md) | 支付插件接口与各网关协议 | 英文 |
| [docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md) | XrayR / V2bX / soga 对接协议 | 英文 |
| [docs/IMPORT_XBOARD.md](docs/IMPORT_XBOARD.md) · [docs/MIGRATION_TERABIX.zh-CN.md](docs/MIGRATION_TERABIX.zh-CN.md) | Xboard 导入器规格与迁移检查清单；一次真实割接的手册 | 英文 · 中英两版 |
| [docs/BACKEND.md](docs/BACKEND.md) · [docs/FRONTEND.md](docs/FRONTEND.md) · [web/README.md](web/README.md) | 开发指南 | 英文 |
| [docs/perf/README.zh-CN.md](docs/perf/README.zh-CN.md) | 压测方法、全部场景、注意事项与发现 | 中英两版 |
| [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md) | 贡献指南：开发环境、约定、如何新增插件（支付、订阅格式、验证码、通知渠道） | 中英两版 |
| [SECURITY.zh-CN.md](SECURITY.zh-CN.md) | 安全策略：漏洞报告与加固清单 | 中英两版 |
| [docs/REVIEW_2026-10-22.md](docs/REVIEW_2026-10-22.md) | 后端安全与正确性评审记录 | 英文 |
| [docs/E2E_2026-10-22.md](docs/E2E_2026-10-22.md) | 端到端集成测试记录：缺陷表与 Lighthouse 分数 | 英文 |
| [MISTAKES.md](MISTAKES.md) | 错题本：开发中踩过的坑 | 中文 |

## 开发

```bash
make test                       # Go 单元测试（-race）
make lint                       # golangci-lint
pnpm -C web install && pnpm -C web dev      # 前端开发服务器，/api 代理到 :8080
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build   # build 含公开页预渲染（需 Chrome）
pnpm -C web lh                  # Lighthouse：桌面 + 移动端，要求四项 100
make e2e                        # 冒烟 + 完整 API 端到端流程（scripts/e2e-full.sh，也可 BASE=… 为运行中的实例填充演示数据）
make bench                      # 压测：起一个临时实例灌入 1 万用户，产出 docs/perf/（results.json、results.md、benchmark.svg）
```

## 致谢

- IP 归属地数据：[纯真 CZ88](https://www.cz88.net/) 社区版 IP 库，由 [metowolf/qqwry.dat](https://github.com/metowolf/qqwry.dat)
  每周同步发布为 `qqwry.dat`。面板运行时自动下载到数据目录，仓库与镜像里都不包含该数据文件。

## 许可证

[GPL-3.0](LICENSE)。全部代码（含 `web/src/components/effects/` 下的动效组件）均为原创，无第三方许可证限制。
