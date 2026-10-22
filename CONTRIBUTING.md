# 参与贡献 / Contributing

欢迎提交 Issue 与 PR。动手前请先读完 [AGENTS.md](AGENTS.md)（唯一权威约定）、[MISTAKES.md](MISTAKES.md)（错题本）
和 `docs/` 里与你改动相关的契约文档。

*Read AGENTS.md, MISTAKES.md and the relevant `docs/` contract before you change anything.*

## 开发环境 / Setup

| 工具 | 版本 | 来源 |
| --- | --- | --- |
| Go | ≥ 1.27.1 | `go.mod` |
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

## Make 目标 / Make targets

| 目标 | 作用 |
| --- | --- |
| `make help` | 列出目标 |
| `make build` | `CGO_ENABLED=0` 构建 `bin/ggboard`（内嵌 `internal/static/dist`，版本取 `git describe`） |
| `make run` | 从源码运行 `serve` |
| `make test` | `go test ./... -race -count=1` |
| `make lint` | golangci-lint，或 `go vet` + `gofmt -l` |
| `make web` | `pnpm -C web install && build`，再把 `web/dist` 复制到 `internal/static/dist` |
| `make e2e` | `scripts/e2e.sh`：编译、随机端口启动，检查 `/healthz` 与 `GET /api/v1/guest/config` |
| `make tidy` | `go mod tidy`（**只由集成者执行**） |
| `make clean` | 删除 `bin/` 与复制过去的前端产物 |

## 前端脚本 / Frontend scripts

`pnpm -C web <script>`：`dev`、`build`、`preview`、`preview:mock`（mock 数据预览，Lighthouse 用）、`typecheck`（`tsc -b`）、
`lint`（ESLint）、`format` / `format:check`（Prettier）、`test` / `test:watch`（Vitest）、`lh` / `lh:mobile`（构建后跑 Lighthouse CI，
要求四项 100）、`routes`。`typecheck`、`lint`、`test` 会先运行 `scripts/gen-routes.mjs` 生成 `src/routeTree.gen.ts`（已 gitignore）。

## 提交前 / Before you open a PR

CI（`.github/workflows/ci.yml`）会跑：`gofmt`、`go build ./...`、`go vet ./...`、`go test ./... -race -count=1`，以及
`pnpm -C web install --frozen-lockfile`、`lint`、`build`。CI 不跑前端类型检查和单测，请本地补上：

```bash
make lint && make test
pnpm -C web typecheck && pnpm -C web lint && pnpm -C web test && pnpm -C web build
```

## 约定 / Conventions

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

## 测试 / Tests

Go 测试用 `internal/testutil`：

- `NewTestApp(t, ...)`：每个测试一个临时目录里的 SQLite 库，argon2id 降为低成本参数，可用 `WithSettings`、`WithConfig` 定制；
  `Mailbox(t, a)` 返回内存邮件驱动。
- `NewRouterWith(t, a, pkg.Register...)` 组装引擎；`CreateUser` / `CreateAdmin` / `CreateStaff` / `Login` 建账号、取 token；
  `Do` / `DoWithHeaders` 发请求；`Decode`、`DecodeList`、`DecodeError`、`ExpectStatus` 断言。
- `GGBOARD_TEST_LOG=1 go test ./...` 把测试应用的日志打到 stderr。
- 订阅格式用 golden 文件（`internal/subscription/formats/testdata/*.golden`），有意改动输出后执行
  `go test ./internal/subscription/formats -update` 重新生成并审阅 diff。

## 新增插件 / Adding a plugin

插件 = 一个新包或新文件，在 `init()` 里注册，由空白导入链接进二进制。现有的空白导入：

| 文件 | 链接什么 |
| --- | --- |
| `cmd/ggboard/plugins_payment.go` | `internal/payment/gateways`（其 `all.go` 再导入每个网关） |
| `cmd/ggboard/import.go` | `internal/importer/xboard`（`ggboard import` 子命令） |
| `cmd/ggboard/jobs_maintenance.go` | `internal/service/maintenance`（定时任务） |
| `internal/subscription/engine/engine.go`、`internal/api/client/routes.go` | `internal/subscription/formats` |

### 支付网关 / Payment gateway

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

### 订阅格式 / Subscription format

接口在 `internal/subscription/format.go`：

```go
type Format interface {
    Name() string
    Supports(nodeType string) bool
    Render(ctx *RenderContext) (body []byte, contentType string, err error)
}
```

可选：实现 `ProxyFilter`（`SupportsProxy(*Proxy) bool`，按传输/加密细节剔除节点）和 `RuleConsumer`（`RuleFamily() string`，输出规则集）。

1. 在 `internal/subscription/formats/` 新建文件，`func init() { subscription.RegisterFormat(myFormat{}) }`。
2. 在 `format.go` 加名称常量，并在 `detect.go` 的 `formatNames`（`?flag=` 可用的名字）与 `detectRules`（User-Agent 识别）里登记——
   不登记的话这个格式永远不会被选中。
3. 在 `formats_test.go` 增加 golden 用例，并更新 [docs/SUBSCRIPTION.md](docs/SUBSCRIPTION.md)。

### 验证码 / Captcha

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
4. 第三方验证码脚本需要放行 CSP：`internal/static/static.go` 的 `CSP` 目前是 `script-src 'self'`，内置的三个服务也需要相应放行。

### 邮件驱动 / Mail driver

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

### 其他扩展点 / Other hooks

- **CLI 子命令**：`cli.Register(func(a *app.App) *cobra.Command { ... })`，执行前 App 已加载配置并完成迁移
  （设置注解 `ggboard/no-app: "true"` 可跳过）。参考 `internal/importer/xboard/cli.go`。
- **定时任务**：`app.RegisterJob(name, cronSpec, func(ctx context.Context, a *app.App) error { ... })`，在 `init()` 里调用，
  名字重复会 panic，只在 `serve` 中运行。参考 `internal/service/maintenance`。
- **数据导入器**：参考 `internal/importer/xboard`，以 CLI 子命令的形式挂载。

## 许可证 / License

贡献即表示同意以 [GPL-3.0](LICENSE) 发布你的代码。
