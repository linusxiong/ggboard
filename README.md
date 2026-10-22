<p align="center">
  <img src="web/public/favicon.svg" width="88" alt="GG Bond" />
</p>
<h1 align="center">GG Bond · ggboard</h1>
<!-- screenshots: see docs/screenshots -->
<p align="center">简洁、美观、高性能的代理面板。单个 Go 二进制，内嵌 React 前端。<br/>
<em>A fast, simple proxy panel: one Go binary with an embedded React SPA.</em></p>

<p align="center">
  <a href="LICENSE"><img alt="GPL-3.0" src="https://img.shields.io/badge/license-GPL--3.0-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white">
  <img alt="Lighthouse" src="https://img.shields.io/badge/Lighthouse-100%20%C2%B7%20100%20%C2%B7%20100%20%C2%B7%20100-brightgreen">
</p>

> **状态 / Status：v0.1.0-alpha** — 早期预览版，API、数据模型与配置项在 1.0 前仍可能变化；升级前请先备份 `data/`。
> Early preview: APIs, schema and settings may still change before 1.0 — back up `data/` before upgrading.

---

## 特性 / Features

- **节点对接**：XrayR / V2bX（`UniProxy` 接口）与 soga（`soga-v1` webapi）。
- **全协议**：vmess、vless（含 REALITY / XHTTP）、shadowsocks（含 2022）、shadowsocksr、trojan、hysteria 1/2、anytls、mieru、tuic。
- **订阅与分流自选**：用户在面板里选择模板（无规则 / 精简 / 标准 / 精细）、规则集来源
  （blackmatrix7、Loyalsoldier、MetaCubeX、SagerNet、ACL4SSR、DustinWin）、分类（OpenAI、Claude、Telegram、Netflix…）、
  策略组和 GitHub 镜像；一条订阅链接即可输出 mihomo / Clash / Stash / sing-box / Surge / Surfboard / Quantumult X /
  Shadowrocket / Loon / v2rayN 等格式。
- **真实支付，插件化网关**：易支付（EPay）、Epusdt/BEpusdt、支付宝当面付、Stripe Checkout、Coinbase Business、BTCPay Server。
  新增网关只需实现一个 Go 接口并注册。
- **账户安全**：argon2id 密码、TOTP 两步验证 + 备用码、Passkey（WebAuthn，支持无用户名一键登录），
  完整适配 1Password / Bitwarden / iCloud 钥匙串等密码管理器自动填充。
- **从 Xboard 迁移**：一条命令导入用户、套餐、节点、优惠券、工单、公告、知识库、支付配置与设置（不导入历史订单），
  同时支持旧版分表 schema 与上游统一表 schema，MySQL / PostgreSQL / SQLite 均可。
- **运营功能**：套餐 / 订单 / 优惠券 / 佣金与邀请 / 工单 / 公告 / 知识库 / 统计 / 定时任务 / 邮件提醒。
- **前端**：React 19 + HeroUI v3 + TanStack（Router / Query / Table / Form）+ zustand，Apple HIG 设计语言，
  浅色 / 深色自动切换，中英文，Lighthouse 四项 100 分（桌面预设，`pnpm -C web lh` 作为门槛）。
- **零依赖部署**：SQLite 默认即可运行，无需 Redis / 队列进程；也支持 PostgreSQL 与 MySQL。

## 快速开始 / Quick start

### Docker Compose

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env            # 填入 GGBOARD_JWT_SECRET 等
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com --password 'change-me'
```

面板监听 `127.0.0.1:8080`，请用 nginx / Caddy 反向代理并启用 HTTPS（示例：`deploy/nginx.example.conf`），
并把代理地址写进 `server.trusted_proxies`。完整部署（systemd、TLS、备份、升级、PostgreSQL / MySQL）见
[docs/DEPLOY.md](docs/DEPLOY.md)。

### 二进制 / From source

```bash
make web && make build          # 构建前端并嵌入，产物 bin/ggboard
./bin/ggboard admin create --email admin@example.com --password 'change-me'
./bin/ggboard serve             # 默认 :8080，数据在 ./data
```

配置通过 `config.yaml`（参考 `config.example.yaml`）或环境变量 `GGBOARD_*` 提供；站点名称、邮件、支付等运行时设置在后台“设置”里修改。

## 节点后端接入 / Connecting node backends

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

协议与各后端类型对照、完整配置片段见 [docs/ADMIN_GUIDE.md](docs/ADMIN_GUIDE.md)，接口细节见 [docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md)。

## 从 Xboard 迁移 / Migrating from Xboard

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # 先预演
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

导入保留原主键、密码哈希（bcrypt / md5 / sha256 均可继续登录并自动升级为 argon2id）、订阅 token、UUID 与 `server_token`，
因此现有订阅链接与节点后端无需改动。细节与迁移后检查清单见 [docs/IMPORT_XBOARD.md](docs/IMPORT_XBOARD.md)。

## 文档 / Docs

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 项目约定、技术栈、设计规范、可插拔点 |
| [docs/DEPLOY.md](docs/DEPLOY.md) | 部署：Docker Compose、systemd、nginx / Caddy、配置参考、备份与升级 |
| [docs/ADMIN_GUIDE.md](docs/ADMIN_GUIDE.md) | 管理员指南：设置、节点对接、套餐与支付、规则集、Xboard 迁移、2FA 策略 |
| [docs/USER_GUIDE.md](docs/USER_GUIDE.md) | 用户指南：各客户端一键导入、分流规则、2FA / Passkey、密码管理器 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架构、启动流程、缓存、定时任务、安全 |
| [docs/API.md](docs/API.md) | 全部 HTTP 接口 |
| [docs/DATA_MODEL.md](docs/DATA_MODEL.md) | 数据模型 |
| [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md) · [docs/RULESETS.md](docs/RULESETS.md) | 订阅格式、规则集目录与模板 |
| [docs/PAYMENTS.md](docs/PAYMENTS.md) · [docs/PAYMENTS_GATEWAYS.md](docs/PAYMENTS_GATEWAYS.md) | 支付插件接口与各网关协议 |
| [docs/NODE_PROTOCOLS.md](docs/NODE_PROTOCOLS.md) | XrayR / V2bX / soga 对接协议 |
| [docs/BACKEND.md](docs/BACKEND.md) · [docs/FRONTEND.md](docs/FRONTEND.md) · [web/README.md](web/README.md) | 开发指南 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献指南：开发环境、约定、如何新增插件 |
| [SECURITY.md](SECURITY.md) | 安全策略：漏洞报告与加固清单 |
| [MISTAKES.md](MISTAKES.md) | 错题本：开发中踩过的坑 |

## 开发 / Development

```bash
make test                       # Go 单元测试（-race）
make lint                       # golangci-lint
pnpm -C web install && pnpm -C web dev      # 前端开发服务器，/api 代理到 :8080
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build
pnpm -C web lh                  # Lighthouse（要求四项 100）
```

## 许可证 / License

[GPL-3.0](LICENSE)。`web/src/components/reactbits/` 下的组件来自 [React Bits](https://reactbits.dev)，遵循其自身许可证。
