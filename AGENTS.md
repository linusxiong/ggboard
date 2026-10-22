# AGENTS.md — GG Bond (ggboard) 代理面板

GG Bond（猪猪侠谐音）是一个 **简洁、美观、高性能** 的代理面板。后端 Go，前端 React。
本文件是所有参与开发的人与 AI agent 的**唯一权威约定**。开始任何工作前先读完本文件、
`MISTAKES.md`（错题本）和 `docs/` 下与你任务相关的契约文档。

---

## 1. 用户原始需求（必须全部满足）

1. 后端 **Go**，简洁、美观、高性能。
2. 节点对接协议：
   - **XrayR API**（即 Xboard/V2board 的 `UniProxy` 接口；V2bX 使用同一套接口，作为超集兼容）。
   - **soga `soga-v1` webapi 协议**（文档 `https://aagit.yougotme.cc/soga/soga-v1`，需要 Basic 认证；
     认证信息由用户私下提供，**绝不写入仓库**。协议内容已整理到 `docs/NODE_PROTOCOLS.md`）。
3. 节点协议全部支持：**vmess、vless、shadowsocks、shadowsocksr、trojan、hysteria（1/2）、anytls、mieru**。
4. 用户订阅时可以**自选分流规则与规则集**：面板预置 GitHub 上知名开源规则集（例如国外流量怎么分流、
   OpenAI 走哪个策略组等），用户按需组合；见 `docs/SUBSCRIPTION.md`。
5. 依赖只用**广泛使用、高性能、安全**的开源库，**全部使用最新版本**，不重复造轮子。
6. 用户可购买套餐并**接入真实支付**；**支付插件可插拔**。
7. 支持从 **Xboard** 导入数据（**不导入历史订单**）。所有功能尽量**可插拔**。
8. 功能清晰简单，不做冗余功能；但**后端支持的所有功能与 API 都必须完整实现**，不留桩代码。
9. 参考项目：`https://github.com/linusxiong/Xboard`（用户自己的 fork，Laravel 10，旧版分表 schema）
   与上游 `https://github.com/cedar2025/Xboard`（新版统一 `v2_server` 表）。导入器两种 schema 都要支持。
10. 前端：**HeroUI**（最新大版本）+ reactbits.dev 风格的动效组件（**必须是原创实现**：React Bits 的许可证是 MIT + Commons Clause，与 GPL-3.0 不兼容，不得复制其源码）；深度使用 **TanStack 全家桶**
    （Router / Query / Table / Form / Virtual）与 **zustand**。
11. 前端加载速度极致优化，Lighthouse **Performance / Accessibility / Best Practices（以及 SEO）全部 100 分**。
12. 大量并行 agent 协作完成；子 agent 全部使用 **Opus，xhigh** 推理强度。
13. 设计全部遵循 **Apple Human Interface Guidelines** 的设计与颜色规范。
14. 所有犯过的错误写入 **`MISTAKES.md`（错题本）**，避免重犯。
15. 登录安全：支持 **2FA（TOTP，备用码）** 与 **Passkey（WebAuthn，可无用户名登录、多设备管理）**。
    用 Go 标准实现：`github.com/pquerna/otp` 与 `github.com/go-webauthn/webauthn`；前端 `@simplewebauthn/browser`。
    不引入 better-auth 之类的 Node 认证框架（后端是单个 Go 二进制）。
16. **密码管理器自动填充兼容**（1Password、Vaultwarden/Bitwarden、iCloud 钥匙串、Chrome 等）：
    登录/注册/找回/改密表单必须是真正的 `<form>` + 原生 `<input>`，带正确的 `autocomplete`
    （`username`/`email`、`current-password`、`new-password`、`one-time-code`、Passkey 条件式 UI `webauthn`）、
    `name`/`id`/`<label>`、`type="password"`、`inputmode`；Passkey 使用 discoverable credential（`residentKey: required`）；
    后端提供 `/.well-known/change-password` 与 `/.well-known/passkey-endpoints`。
17. **Telegram 机器人**（可插拔通知）：订单创建/支付、工单、提现、订阅泄露告警推送给管理员；
    **每次管理员/员工登录都推送登录消息**（邮箱、方式、IP、UA、时间）；用户可绑定 Telegram 接收自己的通知。
18. **注册安全**：邮箱验证码才能注册、注册开关、邮箱后缀白名单，后台设置清晰可用；各类表单填写要便利
    （无 +/- 步进器、价格按元填写、标签式列表输入、Enter 提交、内联校验）。
19. **订阅防泄露**：mihomo 1.19.27+ 的 **age 加密订阅**（面板用用户公钥加密，客户端持 `AGE-SECRET-KEY`），
    订阅访问日志、异常 IP 数告警/自动轮换 token、仅轮换 token 的操作。
20. **参考 Sub-Store 打磨自定义订阅**：多套订阅配置（profile）、节点处理流水线（正则过滤/重命名/地区/类型/
    前后缀/旗帜 emoji/排序/去重/数量上限）、实时预览。
21. 开源项目（GPL-3.0），可以 commit / push 到 GitHub；**绝不提交任何个人信息、密钥、服务器地址**，
    `.gitignore` 必须配置好。

---

## 2. 技术栈（已定稿，不要擅自更换）

### 后端（Go 1.27+）
| 领域 | 选型 |
|---|---|
| HTTP | `github.com/gin-gonic/gin` |
| ORM / DB | `gorm.io/gorm` + 驱动 `gorm.io/driver/sqlite`(默认, pure-go `github.com/glebarez/sqlite`)、`postgres`、`mysql` |
| 配置 | `github.com/spf13/viper`（YAML + 环境变量 `GGBOARD_*`） |
| CLI | `github.com/spf13/cobra` |
| 日志 | 标准库 `log/slog`（JSON / text） |
| 校验 | `github.com/go-playground/validator/v10`（gin binding） |
| JWT | `github.com/golang-jwt/jwt/v5` |
| 密码 | `golang.org/x/crypto/bcrypt`（兼容 Xboard 的 `$2y$`）与 `argon2id`（新用户） |
| 定时任务 | `github.com/robfig/cron/v3` |
| 缓存 | 进程内 `github.com/dgraph-io/ristretto/v2`（无需 Redis） |
| 限流 | `golang.org/x/time/rate` |
| 邮件 | `github.com/wneessen/go-mail` |
| UUID | `github.com/google/uuid` |
| YAML/JSON | `gopkg.in/yaml.v3`、标准库 `encoding/json`（Go 1.25+ 可用 v2） |
| 支付 SDK | Stripe `github.com/stripe/stripe-go/v8x`；其余（EPay/Epusdt/AlipayF2F/Coinbase/BTCPay）使用标准库 `net/http` 实现签名协议 |
| 前端嵌入 | `embed` + 预压缩（br/gz）静态文件服务 |

### 前端（web/）
| 领域 | 选型 |
|---|---|
| 构建 | Vite 8 + React 19 + TypeScript 7，包管理 **pnpm** |
| UI | **HeroUI v3**（`@heroui/react`）+ Tailwind CSS v4；原创动效组件在 `web/src/components/effects/`（CSS 动画 + rAF，懒加载） |
| 路由 | `@tanstack/react-router`（文件式路由 + 自动代码分割） |
| 数据 | `@tanstack/react-query`；表格 `@tanstack/react-table`；表单 `@tanstack/react-form`；长列表 `@tanstack/react-virtual` |
| 状态 | `zustand`（auth/session/UI 偏好） |
| 图标 | `lucide-react`（按需导入） |
| 动画 | `motion`（按需、尊重 `prefers-reduced-motion`） |
| i18n | 自研极简 i18n（JSON 字典按需加载），默认 zh-CN，提供 en |

---

## 3. 仓库结构与所有权（并行开发的冲突防线）

```
cmd/ggboard/            CLI 入口 (serve / migrate / import / admin)
internal/
  app/                  依赖容器 (DB, Settings, Cache, Mailer, Logger, Config)
  config/               viper 配置
  db/                   gorm 初始化、自动迁移
  model/                所有 GORM 模型（docs/DATA_MODEL.md 为准）
  httpx/                gin 引擎、中间件、响应封装、错误类型、分页
  auth/                 JWT、密码哈希、鉴权中间件
  settings/             键值设置服务（带默认值与缓存）
  cache/                进程内缓存封装
  mail/                 邮件接口 + SMTP/log 驱动 + 模板
  scheduler/            cron 任务注册
  plugin/               通用插件注册表
  api/guest/ passport/ user/ admin/ client/ node/   各 API 包，每包一个 Register(r, app)
  service/<domain>/     业务逻辑（order, plan, user, node, ticket, coupon, invite, stat ...）
  payment/              支付网关接口 + registry；payment/gateways/<name>/ 每个插件
  subscription/         订阅生成：formats/<client>/，rulesets/（catalog.json + 模板引擎）
  importer/xboard/      Xboard 导入器
  static/               嵌入前端产物并提供服务
web/                    前端
docs/                   契约与设计文档
deploy/                 Dockerfile、compose、systemd、nginx 示例
```

**所有权规则**
- 每个 agent 只改自己被分配的目录/文件；需要改共享文件（`go.mod`、`internal/model/*`、路由总表、
  `web/package.json`、导航配置、i18n 公共词条）时，只做**追加**，并在汇报中明确列出。
- 不要运行 `go mod tidy`（由集成者统一执行）；需要新依赖用 `go get <mod>@latest` 一次即可。
- 不要修改他人目录下的测试以让自己的代码通过。
- 路由集中在各 API 包的 `Register`，由 `internal/router/router.go` 统一调用。

---

## 4. 工程规范

- **契约优先**：接口以 `docs/API.md`、`docs/DATA_MODEL.md`、`docs/NODE_PROTOCOLS.md`、
  `docs/SUBSCRIPTION.md`、`docs/PAYMENTS.md` 为准；发现契约有缺陷，先改文档再改代码并在汇报中说明。
- **无桩代码**：每个端点都要有完整实现与至少一个测试（httptest + 内存 SQLite）。
- **错误处理**：统一 `httpx.Error`（HTTP 状态码 + `code` + `message` + 可选 `errors` 字段级信息）；
  不要 panic，不要吞错误；日志用 `slog`。
- **安全**：所有输入校验；密码只存哈希；JWT 密钥来自配置；节点接口用常量时间比较 token；
  支付回调必须验签；订阅 token 查询做限流；默认 CORS 仅同源；关键操作写审计日志（slog）。
- **性能**：节点 `user` 列表、订阅生成、配置下发都要走缓存并支持 `ETag`/304；
  流量上报批量写入（事务 + 批量 upsert）；N+1 查询禁止。
- **金额**：整数“分”，货币从设置读取。**流量**：字节（int64）。**时间**：DB 用 datetime，
  API 用 RFC3339 字符串，可空字段用 `null`。
- **代码风格**：`gofmt`/`go vet` 必须干净；前端 `eslint` + `tsc --noEmit` 必须干净。
- **测试命令**：`make test`（Go）、`pnpm -C web test`（前端单元）、`make e2e`（启动服务跑冒烟）。
- **提交**：Conventional Commits（`feat: ...`、`fix: ...`、`docs: ...`）。

---

## 5. 设计规范（Apple HIG）

- 字体：`-apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", "Helvetica Neue", "Inter", sans-serif`；
  **不加载任何 web 字体**（性能 + 原生感）。
- 颜色：Apple 系统色（light / dark 两套）。主色 `systemBlue` `#007AFF` / dark `#0A84FF`，
  成功 `systemGreen` `#34C759` / `#30D158`，警告 `systemOrange` `#FF9500` / `#FF9F0A`，
  危险 `systemRed` `#FF3B30` / `#FF453A`，灰阶 `systemGray` 1–6，分组背景 `#F2F2F7` / `#000000`，
  次级背景 `#FFFFFF` / `#1C1C1E`，三级 `#F2F2F7` / `#2C2C2E`。标签色 `label`/`secondaryLabel`(60%)/`tertiaryLabel`(30%)。
- 圆角：卡片 16–20px，按钮 10–12px，输入 10px；分割线 0.5px；毛玻璃导航 `backdrop-filter: saturate(180%) blur(20px)`。
- 字号层级：Large Title 34/700、Title1 28、Title2 22、Title3 20、Headline 17/600、Body 17、
  Callout 16、Subhead 15、Footnote 13、Caption 12。
- 留白慷慨、层级清晰、动效短促（200–300ms，ease-out），尊重 `prefers-reduced-motion`。
- 深浅色自动跟随系统并可手动切换；所有颜色通过 CSS 变量/Tailwind token 引用，禁止硬编码色值到组件。

---

## 6. 前端性能预算（Lighthouse 四个 100）

- 首屏 JS（登录页/首页）gzip 后 < 120KB，LCP < 1.2s（本地），TBT < 50ms，CLS 0。
- 路由级代码分割；重型组件（动效、图表、Markdown 渲染）`lazy` + 可见时加载；
  动画在 `requestIdleCallback` 后启动。
- 不加载 web 字体；图标按需；图片用 SVG/WebP 并带尺寸；`<meta>` 完整（title/description/viewport/theme-color）。
- 产物预压缩（`.br`/`.gz`），hash 文件名 `immutable` 缓存一年，`index.html` 不缓存。
- 无 console 错误、无第三方脚本、合理的 CSP。
- CI 用 `@lhci/cli` 对构建产物跑分，低于 100 视为失败（performance 允许 ≥ 98 的抖动，但目标 100）。

---

## 7. 可插拔点（Registry 模式）

| 插件点 | 接口位置 | 内置实现 |
|---|---|---|
| 支付网关 | `internal/payment/gateway.go`（`payment.Register`；汇总导入 `internal/payment/gateways/all.go`） | epay, epusdt, alipay_f2f, stripe, coinbase (Business Checkouts), btcpay |
| 订阅格式 | `internal/subscription/format.go`（`subscription.RegisterFormat`） | clash-meta(mihomo), clash(classic), stash, singbox, surge, surfboard, quantumultx, shadowrocket, loon, v2ray(base64), ssr |
| 规则集来源 | `internal/subscription/rulesets/catalog.json` | blackmatrix7, Loyalsoldier, MetaCubeX, ACL4SSR, SagerNet(sing-geosite/geoip), DustinWin |
| 验证码 | `internal/captcha/captcha.go`（`captcha.Register`） | none, turnstile, recaptcha, hcaptcha |
| 数据导入 | `internal/cli`（`cli.Register` 注册子命令） | `import xboard`（legacy 分表 / upstream 统一表） |
| 定时任务 | `internal/app/app.go`（`app.RegisterJob`，见 `docs/ARCHITECTURE.md`） | online_cleanup, traffic_flush（`internal/app`）；order_timeout, commission_settle（`service/order`）；traffic_reset, expire_remind, traffic_remind, stats_daily（`service/maintenance`） |
| 邮件驱动 | `internal/mail/mailer.go`（`mail.FromSettings` 按 `email_driver` 选择，非 registry） | smtp, log |
| 节点对接协议 | `internal/api/node/`（同一包内两组路由，非 registry） | xrayr(UniProxy, V2bX 超集), soga-v1 |

新增 registry 类插件 = 新建一个包 + 在 `init()` 里 `Register` + 在汇总文件里加一行空白导入；不改核心代码。
详见 `CONTRIBUTING.md`。

---

## 8. 秘密与隐私

- 仓库中**禁止**出现：soga 文档的 Basic 认证、用户服务器地址/主机名/IP、邮箱、数据库口令、任何 token。
- 本地机密放 `.env`/`config.local.yaml`（已 gitignore）。示例配置用 `config.example.yaml`。
- 测试数据用 `example.com` 域名与明显的假值。

---

## 9. 错题本制度

- 任何 agent 在工作中犯错（编译错误反复、误解契约、库 API 用错、破坏他人代码、性能回退、安全问题），
  都要在 **`MISTAKES.md`** 追加一条：日期 / 现象 / 根因 / 修复 / 以后怎么避免。
- 开工前先读错题本，避免重蹈覆辙。

---

## 10. 参考资料（本地镜像路径由主 agent 在任务简报中给出）

- Xboard（用户 fork, legacy schema）：`https://github.com/linusxiong/Xboard`
- Xboard（上游, 新 schema + 插件）：`https://github.com/cedar2025/Xboard`
- XrayR（master 已被清空，使用 tag `v25.3.6`）：`https://github.com/XrayR-project/XrayR`
- V2bX（UniProxy 超集：hysteria2/tuic/anytls）：`https://github.com/wyx2685/V2bX`
- soga 文档（需认证）：`https://aagit.yougotme.cc/soga/soga-v1`，整理版见 `docs/NODE_PROTOCOLS.md`
- HeroUI：`https://heroui.com/docs`；TanStack：`https://tanstack.com`（reactbits.dev 仅作视觉参考，不复制代码）
- Apple HIG：`https://developer.apple.com/design/human-interface-guidelines`
