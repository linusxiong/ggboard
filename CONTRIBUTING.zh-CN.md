# 参与贡献

[English](CONTRIBUTING.md) · **简体中文**

欢迎提交 Issue 与 PR。动手前请先读完 [AGENTS.md](AGENTS.md)（唯一权威约定）、[MISTAKES.md](MISTAKES.md)（错题本）
和 `docs/` 里与你改动相关的契约文档。

## 开发环境

| 工具 | 版本 | 来源 |
| --- | --- | --- |
| Go | ≥ 1.27.3 | `go.mod`（CI 用 `1.27.x`，Dockerfile 用 `golang:1.27-alpine`） |
| Node.js | ≥ 22（CI 与 Dockerfile 用 24） | `web/package.json` `engines`、`.github/workflows/ci.yml` |
| pnpm | 12（Dockerfile 固定 `pnpm@12.4.1`） | `ci.yml`、`deploy/Dockerfile` |
| golangci-lint | v2（可选） | `.golangci.yml`；没装时 `make lint` 退回 `go vet` + `gofmt` |

```bash
make run                                   # go run ./cmd/ggboard serve，数据在 ./data
go run ./cmd/ggboard admin create --email admin@example.com
pnpm -C web install && pnpm -C web dev     # Vite 开发服务器，/api 与 /s/ 代理到 http://localhost:8080
                                           # 后端在别处时：GGBOARD_API=http://127.0.0.1:9090 pnpm -C web dev
```

没执行 `make web` 时，二进制里只有占位页（"Frontend not built"），API 照常工作。

## Make 目标

| 目标 | 作用 |
| --- | --- |
| `make help` | 列出目标 |
| `make build` | `CGO_ENABLED=0` 构建 `bin/ggboard`（内嵌 `internal/static/dist`，版本取 `git describe`） |
| `make run` | 从源码运行 `serve` |
| `make test` | `go test ./... -race -count=1` |
| `make lint` | golangci-lint，或 `go vet` + `gofmt -l` |
| `make web` | `pnpm -C web install && build`，再把 `web/dist` 复制到 `internal/static/dist` |
| `make e2e` | `scripts/e2e.sh`（编译、随机端口启动，检查 `/healthz`、`GET /api/v1/guest/config` 与首页），再跑 `scripts/e2e-full.sh`（API 全流程；设 `BASE=…` 可对运行中的实例执行并顺便灌入演示数据） |
| `make tidy` | `go mod tidy`（**只由集成者执行**） |
| `make clean` | 删除 `bin/` 与复制过去的前端产物 |

## 前端脚本

`pnpm -C web <script>`：`dev`、`build`、`preview`、`preview:mock`（mock 数据预览，Lighthouse 用）、`typecheck`（`tsc -b`）、
`lint`（ESLint）、`format` / `format:check`（Prettier）、`test` / `test:watch`（Vitest）、`lh`（构建后依次跑 `lh:desktop` 与
`lh:mobile` 两套 Lighthouse CI，要求四项 100）、`ui:smoke`（对运行中的实例用 puppeteer 逐页巡检：控制台错误、失败请求、
漏翻的 i18n 键、横向溢出；`--axe` 加跑 axe-core）、`routes`。
`typecheck`、`lint`、`test` 会先运行 `scripts/gen-routes.mjs` 生成 `src/routeTree.gen.ts`（已 gitignore）。
README 截图没有 pnpm 脚本：对运行中的实例执行 `node web/scripts/screenshots.mjs`（用法见脚本头注释与
[web/README.md](web/README.md)）。

## 提交前

CI（`.github/workflows/ci.yml`）会跑：`gofmt`、`go build ./...`、`go vet ./...`、`go test ./... -race -count=1`，以及
`pnpm -C web install --frozen-lockfile`、`lint`、`build`。CI 不跑前端类型检查和单测，请本地补上
（手动触发的 `.github/workflows/docker.yml` 负责构建并推送多架构镜像，见 [docs/DEPLOY.zh-CN.md](docs/DEPLOY.zh-CN.md)）：

```bash
make lint && make test
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build
```

## 约定

摘自 [AGENTS.md](AGENTS.md) §3、§4、§8、§9，冲突时以 AGENTS.md 为准。

- **契约优先**：接口以 `docs/API.md`、`docs/DATA_MODEL.md`、`docs/NODE_PROTOCOLS.md`、`docs/SUBSCRIPTION.md`、`docs/PAYMENTS.md` 为准。
  契约有缺陷时先改文档，再改代码，并在 PR 里说明。
- **无桩代码**：每个端点都要完整实现，并至少有一个测试。
- **错误**：统一返回 `httpx.Error`（HTTP 状态码 + `code` + `message` + 可选字段级 `errors`）；不 panic、不吞错，日志用 `slog`。
- **安全**：校验所有输入；密码只存哈希；节点 token 常量时间比较；支付回调必须验签；关键操作写审计日志。
- **性能**：节点用户列表、订阅、配置下发走缓存并支持 `ETag`/304；流量批量写入；禁止 N+1 查询。
- **单位**：金额为整数“分”，流量为字节（int64），API 时间为 RFC3339，可空字段用 `null`。
- **路由**：每个 API 包在自己的 `Register` 里挂路由，由 `internal/router/router.go` 统一调用。
- **所有权**：只改分配给你的目录。共享文件（`go.mod`、`internal/model/*`、路由总表、`web/package.json`、导航、i18n 公共词条）只做追加；
  不要跑 `go mod tidy`；不要为了让自己的代码通过而修改别人的测试。
- **风格**：`gofmt` / `go vet` 干净；前端 ESLint + `tsc` 干净。界面遵循 Apple HIG（AGENTS.md §5）与性能预算（§6），
  颜色只用 token，不加载 web 字体。
- **提交信息**：Conventional Commits，如 `feat(api): ...`、`fix(web): ...`、`docs: ...`。
- **错题本**：踩过的坑（反复的编译错误、误解契约、库 API 用错、破坏他人代码、性能回退、安全问题）都要追加到
  [MISTAKES.md](MISTAKES.md)，格式为 现象 / 根因 / 修复 / 规则。
- **秘密**：仓库里不能出现真实域名、IP、邮箱、口令或 token。本地机密放 `.env`、`config.yaml`、`config.local.yaml`（已 gitignore）；
  测试数据用 `example.com` 和明显的假值。
- **双语文档**：面向用户的文档成对存在：`X.md` 是英文版（默认版本，链接时排在前面），`X.zh-CN.md` 是简体中文版，
  两版的章节相同、顺序相同、编号相同；改了其中一版，就在同一个提交里同步另一版。成对的文档有 README、CHANGELOG、
  CONTRIBUTING、SECURITY、`docs/DEPLOY`、`docs/ADMIN_GUIDE`、`docs/USER_GUIDE`、`docs/MIGRATION_TERABIX`、`docs/perf/README`。
  发布说明取自 `CHANGELOG.md` 与 `CHANGELOG.zh-CN.md` 里的 `## [X.Y.Z]` 一节，缺了任何一个，发布工作流都会失败。
  契约与开发文档（`docs/API.md`、`docs/DATA_MODEL.md`、`docs/ARCHITECTURE.md` ……、`web/README.md`）只有英文版。

## 测试

Go 测试用 `internal/testutil`：

- `NewTestApp(t, ...)`：每个测试一个临时目录里的 SQLite 库，argon2id 降为低成本参数，可用 `WithSettings`、`WithConfig` 定制；
  `Mailbox(t, a)` 返回内存邮件驱动。
- `NewRouterWith(t, a, pkg.Register...)` 组装引擎；`CreateUser` / `CreateAdmin` / `CreateStaff` / `Login` 建账号、取 token；
  `Do` / `DoWithHeaders` 发请求；`Decode`、`DecodeList`、`DecodeError`、`ExpectStatus` 断言。
- `GGBOARD_TEST_LOG=1 go test ./...` 把测试应用的日志打到 stderr。
- 订阅格式用 golden 文件（`internal/subscription/formats/testdata/*.golden`），有意改动输出后执行
  `go test ./internal/subscription/formats -update` 重新生成并审阅 diff。

## 新增插件

插件 = 一个新包或新文件，在 `init()` 里注册，由空白导入链接进二进制。现有的空白导入：

| 文件 | 链接什么 |
| --- | --- |
| `cmd/ggboard/plugins_payment.go` | `internal/payment/gateways`（其 `all.go` 再导入每个网关） |
| `cmd/ggboard/plugins_notify.go` | `internal/notify/telegram`（Telegram 通知渠道） |
| `cmd/ggboard/wire_leak.go` | 不是空白导入：把 `subscription.LeakHook` 设为“默认邮件 + `notify.EmitSubscriptionLeak`” |
| `cmd/ggboard/import.go` | `internal/importer/xboard`（`ggboard import` 子命令） |
| `cmd/ggboard/jobs_maintenance.go` | `internal/service/maintenance`（定时任务） |
| `internal/subscription/engine/engine.go`、`internal/api/client/routes.go` | `internal/subscription/formats` |

### 支付网关

接口在 `internal/payment/gateway.go`：

```go
type Gateway interface {
    Name() string    // registry id, e.g. "epay"
    Label() string   // human name
    Fields() []Field // admin form schema
    Pay(ctx context.Context, cfg map[string]string, req PayRequest) (*PayResult, error)
    Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*NotifyResult, *NotifyResponse, error)
}
```

1. 新建 `internal/payment/gateways/<name>/`，实现接口，并 `func init() { payment.Register(&Gateway{}) }`。
   签名、金额换算、HTTP 调用可复用 `internal/payment/gateways/gwutil`。
2. 在 `internal/payment/gateways/all.go` 加一行空白导入。
3. `Notify` 必须验签：验证失败返回 `(nil, nil, err)`（调用方回 400 `fail`），已验证但不是成功支付返回 `(nil, resp, nil)`。
4. 后台的网关表单直接由 `Fields()` 渲染（`GET /api/v1/admin/payments/gateways`），无需改前端。
5. 用 httptest 测试 `Pay` 的请求与 `Notify` 的验签（正确签名、错误签名、重放、金额不符），并在
   [docs/PAYMENTS_GATEWAYS.md](docs/PAYMENTS_GATEWAYS.md) 补充协议说明。

### 订阅格式

接口在 `internal/subscription/format.go`：

```go
type Format interface {
    Name() string
    Supports(nodeType string) bool
    Render(ctx *RenderContext) (body []byte, contentType string, err error)
}
```

可选：实现 `ProxyFilter`（`SupportsProxy(*Proxy) bool`，按传输/加密细节剔除节点）、`RuleConsumer`（`RuleFamily() string`，输出规则集）
和 `AgeCapable`（`SupportsAge() bool`：客户端能解密 age 加密订阅时返回 true，目前只有 clash-meta；见 docs/SUBSCRIPTION.md）。

1. 在 `internal/subscription/formats/` 新建文件，`func init() { subscription.RegisterFormat(myFormat{}) }`。
2. 在 `format.go` 加名称常量，并在 `detect.go` 的 `formatNames`（`?flag=` 可用的名字）与 `detectRules`（User-Agent 识别）里登记——
   不登记的话这个格式永远不会被选中。
3. 在 `formats_test.go` 增加 golden 用例，并更新 [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md)。

### 验证码

接口在 `internal/captcha/captcha.go`：

```go
type Verifier interface {
    Verify(ctx context.Context, token, remoteIP string) error
}
type Factory func(secret string, client *http.Client) Verifier

func Register(name string, f Factory)
```

1. 在 `init()` 里 `captcha.Register("<name>", factory)`；走 siteverify 协议的服务可直接复用 `captcha.SiteVerify`。
2. 在 `internal/settings/defs.go` 的 `captcha_provider` 选项里加上名字（设置写入会校验选项）。
3. 在 `web/src/components/Captcha.tsx` 的 `PROVIDERS` 里登记前端脚本与全局对象。
4. 在 `internal/static/static.go` 的 `captchaSources` 里登记该服务的 script / frame（以及需要的 connect / style）来源：
   `static.CSPFor` 只为当前启用的那一个验证码服务放行，其余页面仍是严格的默认 `CSP`。

### 邮件驱动

接口在 `internal/mail/mailer.go`：

```go
type Mailer interface {
    Send(ctx context.Context, msg Message) error
}
```

邮件驱动**不是注册表**：内置 `smtp`、`log` 由 `mail.FromSettings` 的 `switch` 选择。新增驱动需要：

1. 实现 `Mailer`；
2. 在 `FromSettings` 增加分支；
3. 在 `defs.go` 的 `email_driver` 选项里加上名字，并补充需要的设置键。

所有邮件经 `app.Mailer`（`internal/mail/queue.go`）异步投递，请求处理里不要同步发信。

### 通知渠道

接口在 `internal/notify/notify.go`（Telegram 是内置实现，见 `internal/notify/telegram`）：

```go
type Notifier interface {
    Name() string
    SendAdmin(ctx context.Context, ev Event) error                // administrator destinations; ErrSkipped when not subscribed
    SendUser(ctx context.Context, chatRef string, ev Event) error // one user, addressed by UserAddresser.ChatRef
}
type UserAddresser interface{ ChatRef(u *model.User) string }   // optional: "" = user not reachable
type Factory func(a *app.App) Notifier                           // nil while the channel is not configured

func Register(name string, f Factory)
```

1. 新建 `internal/notify/<name>/`，在 `init()` 里 `notify.Register("<name>", factory)`；factory 每个事件都会调用，
   必须便宜并读取当前设置（未配置时返回 nil，`notify.Emit` 便不会建队列）。
2. 在 `cmd/ggboard/` 加一个空白导入文件（参考 `plugins_notify.go`）。
3. 永久失败（如 4xx）用 `notify.Permanent(err)` 包装，避免无谓重试；向多个会话扇出时用 `notify.Done` / `notify.MarkDone`
   让重试只补发失败的会话；需要的设置键追加到 `internal/settings/defs.go` 的新分组。
4. 测试：`notifytest.Attach(t, a)` 记录事件；参考 `internal/notify/telegram/telegramtest` 的假 Bot API 服务器。
   事件类型与字段见 [docs/API.md](docs/API.md) §Telegram 与 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) §Notifications。

### 前端动效与图标

- **动效必须原创**：放在 `web/src/components/effects/`（CSS keyframes、IntersectionObserver、rAF），懒加载、尊重
  `prefers-reduced-motion`。React Bits（reactbits.dev）的许可证是 MIT + Commons Clause，与 GPL-3.0 不兼容，
  **不得复制其源码**，只能作视觉参考（AGENTS.md §1 第 10 条、MISTAKES.md #107）。
- **支付预设图标**：新增预设 = 在 `web/src/components/icons/payments/glyphs.tsx` 加一个原创 SVG 组件，并在 `presets.ts`
  的 `PAYMENT_ICON_IDS` 与预设表里登记 id（`[a-z0-9_-]{1,32}`）；后端接受任何格式正确的 `preset:<id>`，无需改 Go 代码。
  新网关的默认图标在 `web/src/features/admin/payments/gateways.ts` 的 `defaultIconPreset` 里映射。`icons/payments/`
  只能被订单页与后台支付页导入，不能进 `vite.config.ts` 里公开页也加载的 `chrome` 分组（MISTAKES.md #143）。

### 其他扩展点

- **CLI 子命令**：`cli.Register(func(a *app.App) *cobra.Command { ... })`，执行前 App 已加载配置并完成迁移
  （设置注解 `ggboard/no-app: "true"` 可跳过）。参考 `internal/importer/xboard/cli.go`。
- **定时任务**：`app.RegisterJob(name, cronSpec, func(ctx context.Context, a *app.App) error { ... })`，在 `init()` 里调用，
  名字重复会 panic，只在 `serve` 中运行。参考 `internal/service/maintenance`。
- **数据导入器**：参考 `internal/importer/xboard`，以 CLI 子命令的形式挂载。

## 许可证

贡献即表示同意以 [GPL-3.0](LICENSE) 发布你的代码。
