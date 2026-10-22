# 错题本 (MISTAKES.md)

记录本项目开发过程中犯过的错误，避免重复。格式：**日期 / 现象 / 根因 / 修复 / 规则**。
每个 agent 犯错后必须追加；开工前必须先读。原条目没有单独记录的字段标为“（原记录未单列）”。

## 目录（按主题）

| 主题 | 条目 |
| --- | --- |
| Shell / zsh / 命令行工具 | 2, 7, 8, 9, 11, 17, 22, 23, 52, 64, 65, 91, 93, 94, 95 |
| Go | 21, 30, 31, 33, 34, 35, 37, 38, 39, 40, 41, 44, 45, 46, 47, 48, 49, 50, 53, 57, 61, 62, 63, 97, 98, 99 |
| GORM / SQLite / 数据库 | 18, 19, 20, 42, 54, 55, 96 |
| 前端（React / HeroUI / Vite / Vitest / pnpm） | 3, 15, 16, 24, 25, 26, 27, 28, 29, 58, 66, 67, 68, 69, 70, 71, 72, 74, 75, 76, 77, 80, 81, 82, 83, 84, 85, 86 |
| 文档 / 契约 / 协作流程 | 1, 4, 5, 6, 10, 12, 13, 14, 32, 36, 43, 51, 56, 59, 60, 73, 78, 79, 87, 88, 89, 90, 92 |

同一问题被多个 agent 分别记录过（均保留原文，便于对照）：
zsh glob 未加引号 8 / 64 / 94；手算测试向量 23 / 41；路由选项把依赖拉进首屏入口 66 / 84；
Vitest 缓存 `import.meta.glob` 结果 67 / 72 / 77 / 85；`useLiteForm` 抢焦点 68 / 86；浏览器面板共用标签页 73 / 78；rtk 截断长输出 9 / 91。

---

## 2026-10-21

### 1. 以为 XrayR 的 master 分支有源码

- **现象**：`git clone --depth 1 XrayR` 得到空仓库（最后一次提交 "Clear all files"）。
- **根因**：XrayR 项目已清空 master，只有 tag 保留源码。
- **修复**：按 tag 克隆：`git clone --depth 1 --branch v25.3.6 https://github.com/XrayR-project/XrayR.git`。
- **规则**：参考第三方仓库前先 `git ls-remote --tags`，优先用 tag 固定版本。

### 2. `grep -E "->(get|post)"` 被当成命令行选项

- **现象**：macOS 上 `grep` 被别名为 `ugrep`，模式以 `-` 开头被解释为选项。
- **根因**：未用 `-e PATTERN` 或 `--` 分隔。
- **修复**：`grep -E -e "->(get|post)\("`。
- **规则**：模式以 `-` 开头时一律使用 `-e`。

### 3. 假设 HeroUI 还是 v2

- **现象**：计划按 HeroUI v2（NextUI 血统）API 写前端。
- **根因**：未先查 npm 最新版本；`@heroui/react` 最新为 **3.x**（基于 React Aria Components + Tailwind v4，API 与 v2 不兼容）。
- **修复**：所有前端 agent 先读 `https://heroui.com/docs` 的 v3 文档再写组件。
- **规则**：用户要求“依赖全部最新版本”时，动手前用 `npm view <pkg> version` / `go list -m -versions` 核对，并按最新大版本的文档编码。

### 4. 以为 Xboard 用户 fork 与上游 schema 相同

- **现象**：用户 fork（Laravel 10）仍是 `v2_server_vmess/vless/trojan/shadowsocks/hysteria` 分表；上游已统一为 `v2_server`（`protocol_settings` JSON）。
- **根因**：未对比两个仓库的 migrations。
- **修复**：导入器按“表是否存在”自动识别 legacy / upstream 两种 schema。
- **规则**：导入/兼容类功能必须同时对照用户实际运行版本与上游最新版本。

### 5. 契约草图里的规则集 URL 模板未经验证

- **现象**：`docs/SUBSCRIPTION.md` 草图给 blackmatrix7 写了 `rule/sing-box/{name}/{name}.srs`，给 ACL4SSR 写了根目录 `{name}.list`（Surge）。两者实际都返回 404：bm7 的 master/release 分支都没有 sing-box 目录；ACL4SSR 的列表只在 `Clash/` 与 `Clash/Ruleset/` 下。
- **根因**：凭印象写 URL，没有先看仓库目录树。
- **修复**：用 blobless 浅克隆列出目录树，确定真实文件名，在 catalog 中删除或改正；再由验证脚本逐个 HEAD 校验（见 `docs/RULESETS.md` 的 Verification log）。
- **规则**：任何第三方 raw URL 写进代码或文档前，先用 `git clone --depth 1 --filter=blob:none --no-checkout --single-branch --branch <b>` 加 `git ls-tree -r --name-only HEAD` 确认路径，再跑 URL 校验。不要依赖 GitHub API（未认证 60 次/小时）。

### 6. 以为 blackmatrix7 的 `{name}.yaml` 一定是完整规则

- **现象**：bm7 的大规则集（ChinaMax、AdvertisingLite、Apple、Proxy、Global、Netflix）中，`{name}.yaml`（以及 Surge/Shadowrocket/Loon 的 `{name}.list`）只是剩余部分：域名在 `_Domain`，IP 在 `_IP`。单独使用会丢掉绝大部分规则，例如 ChinaMax.yaml 只剩 27 条 KEYWORD/PROCESS-NAME。
- **根因**：没读每个规则目录 README 里的“配置建议”。
- **修复**：catalog 中把这些 RuleRef 标记为 `split: true`。渲染时与 `_domain`/`_ipcidr` 一起使用，或单独使用 `clash_classical`。
- **规则**：使用规则集前先读它的 README 使用说明，并按规则类型统计抽查文件内容。同类坑：Loyalsoldier 的 `.txt` 实为 YAML（`format: yaml`）；MetaCubeX `classical/*.list` 含 `DOMAIN-REGEX`，不能直接给 Surge 用。

### 7. 探测镜像时复用 curl 输出文件，死镜像看起来“内容正确”

- **现象**：批量探测 GitHub raw 代理时，`ghproxy.cc`（证书过期）和 `github.moeyy.xyz`（NXDOMAIN）返回 000，却显示 `payload=1`。临时文件也误写到了 `/tmp`，而不是 scratchpad。
- **根因**：`curl -o file` 失败时不会清空文件，grep 读到的是上一个镜像的响应。
- **修复**：每次探测前删除输出文件；验证脚本按单次请求读取响应体，同时校验状态码和内容。临时文件统一放 scratchpad。
- **规则**：探测类脚本必须同时检查退出码/状态码与本次请求的内容，不复用输出文件。`mirror.ghproxy.com` 已失效（NXDOMAIN，服务迁到 ghfast.top），镜像列表每次刷新都要重新验证。

### 8. zsh 下 `grep --include=*.php` 未加引号，命令根本没执行

- **现象**：`grep -rn "expired_at" app --include=*.php` 直接报 `no matches found: --include=*.php`，没有任何搜索结果，容易误判为“代码里没有这个字段”。
- **根因**：zsh 默认开启 `nomatch`，glob 无匹配时报错而不是像 bash 那样原样传参。
- **修复**：给模式加引号：`--include='*.php'`。
- **规则**：在 zsh 中凡是含 `*`、`?`、`[` 的参数一律加引号；“搜不到”先确认命令真的执行了。

### 9. 长输出被 shell 包装层截断/过滤，差点漏读 PHP 源码

- **现象**：读取 Xboard 模型与迁移时，输出末尾出现 `[rtk] output capped at 200 results` 或 `... (N filtered) [+N hidden: rtk recall …]`，部分 `$casts`/方法被隐藏。
- **根因**：本机 shell 对命令输出做了截断与过滤，一次性 `cat` 多个文件拿到的不是全文。
- **修复**：用 `sed -n 'a,bp'` / `awk '/start/,/end/'` 分段读取，或用 Read 工具读单个文件。
- **规则**：看到 `[rtk]` 提示必须分段重读；不能把被截断的输出当成完整事实写进契约文档。

### 10. 差点相信 Xboard 的列注释/docblock，而不是读写该字段的代码

- **现象**：`v2_ticket.reply_status` 迁移注释写“0:待回复 1:已回复”，但 legacy `TicketService` 在管理员回复后写 0（与注释相反，upstream 2026-04-21 迁移才翻转）；`Plan` docblock 写 `transfer_enable 流量(KB)`、upstream 迁移注释写 “bytes”，代码实际按 GB × 1073741824；upstream `v2_plan.prices` 存的是“元”（小数），不是“分”。
- **根因**：注释与实现长期漂移。
- **修复**：`docs/IMPORT_XBOARD.md` 一律以 Service/Controller/Job 的实际读写为准（工单 `reply_status` 由最后一条消息推导，legacy 无消息时取反；价格 `cents(v)`）。
- **规则**：做导入/兼容时，字段语义以“写入它的代码 + 读取它的代码”为准，注释与 docblock 只作线索；每个单位（分/元、GB/字节、秒/毫秒、时区）都要找到对应代码行。

### 11. zsh 中 `echo =====` 直接报错并中断整条命令

- **现象**：`cat a; echo =====; cat b` 输出 `(eval):1: ===== not found`，后续命令全部没执行。
- **根因**：zsh 的 `=word` 会做 equals 展开（`=cmd` 会被展开成 `cmd` 的路径），`=====` 被当成要查找的命令名。
- **修复**：分隔符加引号：`echo "-----"`。
- **规则**：在 zsh 里，以 `=` 开头的参数一律加引号，分隔符统一用 `"-----"`。

### 12. 支付契约沿用了过时的第三方 API 认知

- **现象**：`docs/PAYMENTS.md` 和任务简报中有 4 处在 2026-10 时已经不成立：
  - Coinbase Commerce `POST /charges`：Commerce 已于 2026-03-31 并入 Coinbase Business，Charge API 停止服务。
  - Stripe `payment_method_types`：stripe-go v87（API `2026-09-30.endive`）已删除该参数，改为 `allowed_payment_method_types`。
  - Epusdt `/api/v1/order/create-transaction`：assimon/epusdt 已变成 GM Pay，v2.0.0 删除了这个接口。
  - BTCPay 把 `InvoicePaymentSettled` 当作“已支付”：该事件按单笔付款触发，部分付款也会触发。
- **根因**：契约是凭记忆和 Xboard 旧代码写的，没有对照上游最新 tag、changelog 或官方文档。
- **修复**：`docs/PAYMENTS_GATEWAYS.md` 逐一给出核实结果和 Contract notes。
- **规则**：写支付或第三方协议之前，必须核对两样东西：
  - 最新版本：Go 模块用 `curl -s https://proxy.golang.org/<module>/vNN/@latest` 逐个探测大版本号，GitHub 项目用 `gh api repos/<o>/<r>/releases/latest`；
  - 该版本的源码或官方文档。
  签名算法要用上游文档里自带的示例向量复算一遍，确认一致后才能写进文档。

### 13. 在上游 Xboard 里找 `app/Payments` 扑空

- **现象**：`ls xboard-upstream/app/Payments` 报不存在。
- **根因**：cedar2025/Xboard 已把支付方式改成插件，代码在 `plugins-core/<Name>/Plugin.php`。用户 fork 仍在 `app/Payments/*.php`。
- **修复**：（原记录未单列）
- **规则**：对照上游 Xboard 的支付或通知实现时，先查 `plugins-core/`、`plugins/`，再查 `app/Payments/`。

### 14. 写前端参考文档时没先看已有的 `web/` 脚手架

- **现象**：`docs/FRONTEND.md` 初稿自带了一套 `api.ts`、DataTable、表单 hook 和主题方案。写完才发现 `web/src` 里已经有 `@/lib/api`（`Page<T>={items,total}`、`ApiError.errors`）、`@/lib/guards`、`@/components/data-table`、`@/components/form`、`@/lib/toast`、`styles/theme.css`，两套写法互相冲突，只能整篇重写。
- **根因**：开工时只读了 AGENTS/API 契约，没有 `ls web/src`，也没读已有模块的头部注释。
- **修复**：FRONTEND.md 增加 §0「脚手架积木」表，示例全部改用现有模块。
- **规则**：写指南或示例代码之前，先 `git status` 加 `ls` 看目标目录是否已有实现，并读各文件的头部注释。示例必须调用已有积木，不要另起一套。

### 15. HeroUI 文档的 Anatomy 段落可能落后于示例和发布说明

- **现象**：Switch 页面的 Anatomy 段落仍是 v3.1 结构（`Switch.Control` 与 `Switch.Content` 平级，内嵌 `<Label>`）。同页的示例和 v3.2.0 发布说明则是 `Switch.Content > Switch.Control + 纯文本`。
- **根因**：文档站局部没有随破坏性变更同步更新。
- **修复**：以发布说明、示例和 `npm pack @heroui/react` 解出的 `dist/**/*.d.ts` 三方交叉核对。
- **规则**：HeroUI 原文用 `https://heroui.com/en/docs/react/components/<name>.mdx` 获取（注意是 `.mdx`，`.md` 会 404）。Anatomy 与示例冲突时，以示例、发布说明和 `.d.ts` 为准。

### 16. “最新版本”依赖里沿用了上一代 API 习惯

- **现象**：
  - TanStack Query 5.104 已把 `ensureQueryData`、`fetchQuery`、`prefetchQuery` 标为 `@deprecated`，应改用 `queryClient.query()`；`web/src/lib/guards.ts` 仍在用。
  - Vite 8 中 `build.rollupOptions` 改名为 `rolldownOptions`；`manualChunks` 的对象形式已删除，应改用 `output.codeSplitting.groups`。
  - TypeScript 7 不再提供编译器 API，而 `typescript-eslint@8.72` 的 peer 要求 `typescript <6.1.0`；`web/package.json` 却是 `typescript ^7.0.2` 加 `typescript-eslint ^8.72.0`。
  - Apple 系统色作为文字或文字底色达不到 WCAG AA：白字配 `#007AFF` 只有 4.02:1。
- **根因**：凭旧版本记忆写代码，没有检查 peerDependencies、`@deprecated` 标注和对比度。
- **修复**：FRONTEND.md §1/§5/§13/§15 给出替代写法；TS 6/7 共存方案按官方公告使用别名 `"typescript": "npm:@typescript/typescript6@^6.0.2"` 加 `"@typescript/native": "npm:typescript@^7.0.2"`。
- **规则**：
  - 升级或新增依赖后跑 `npm view <pkg> peerDependencies`，并在 `npm pack` 解出的 `.d.ts` 里 `grep -n "@deprecated"` 检查自己要用的 API。
  - 改任何文字色或按钮底色 token，都先计算对比度（≥ 4.5:1），再提交。

### 17. （Go 基础）macOS 的 BSD `sed` 不支持 `\|`，替换悄悄失败

- **现象**：`sed -i '' 's/a\|b/c/'` 退出码为 0，但一处都没替换（`grep -c` 结果为 0）。
- **根因**：BSD sed 的基本正则不支持 `\|` 交替，模式被当成字面量。
- **修复**：改用 `perl -pi -e 's/(a|b)/c/'` 或 Python 读写文件。
- **规则**：批量替换后必须用 `grep -c` 核对替换次数；交替、非贪婪等写法一律用 perl/python。

### 18. （Go 基础）以为 `glebarez/sqlite` 和 `gorm.io/driver/sqlite` 一样有 `sqlite.New(sqlite.Config{...})`

- **现象**：编译报 `undefined: sqlite.New` / `sqlite.Config`。
- **根因**：glebarez 只提供 `Open(dsn)` 和导出的 `Dialector{DriverName, DSN, Conn}` 结构体。
- **修复**：`&sqlite.Dialector{DriverName: sqlite.DriverName, DSN: dsn, Conn: sqlDB}`。
- **规则**：同名但不同作者的驱动不要套用对方的 API，先 `grep -n "^func \|^type " $(go env GOMODCACHE)/<mod>@<ver>/*.go`。

### 19. （Go 基础）纯 Go SQLite 驱动按 time.Time 自带的时区写文本，导致时间比较出错

- **现象**：`expired_at > ?` 传入本地时间（+08:00）而库里存的是 UTC 字符串时，比较结果错误。SQLite 的 datetime 是按文本比较的。
- **根因**：glebarez/go-sqlite 用 `t.Format("2006-01-02 15:04:05.999999999-07:00")` 写参数，保留了原有时区偏移。
- **修复**：`internal/db/sqlite_utc.go` 包装 driver.Conn，`CheckNamedValue` 把所有时间参数统一转成 UTC。PostgreSQL 用 timestamptz，MySQL DSN 强制 `loc=UTC`，都不受影响。
- **规则**：查询参数一律传 `time.Time`，不要手写时间字符串；`date` 列统一用 `model.Day(t)`。

### 20. （Go 基础）测试里复用了已带主键的 GORM 结构体变量，读到旧数据

- **现象**：`a.DB.First(&got, u2.ID)` 读回的是 u1 的数据。
- **根因**：`got` 里还留着 u1 的主键，GORM 会把它也加进 WHERE，结果查不到记录。错误又没检查，`got` 就保持了旧值。
- **修复**：每次查询都用新变量。
- **规则**：每个 First/Find 都用新声明的变量，并检查 `.Error`。

### 21. （Go 基础）测试 URL 的 query 里带了 `;`，整个参数被丢弃

- **现象**：`/sort?sort=-a,b;drop` 到了 handler 里 `c.Query("sort")` 为空。
- **根因**：Go 1.17 起 `url.ParseQuery` 遇到含 `;` 的键值对会整对丢弃。
- **修复**：写成 `%3B`。
- **规则**：测试 URL 里的特殊字符一律先 URL 编码。

### 22. （Go 基础）`pkill -f "<绝对路径> serve"` 没有匹配到用相对路径启动的进程

- **现象**：以为烟测服务器已经停了，实际还在运行，日志里也看不到 graceful shutdown。
- **根因**：进程的命令行是 `./ggboard serve`，用绝对路径的模式匹配不上。
- **修复**：先 `pgrep -fl` 确认进程，再 `pkill -TERM -f "ggboard serve"`；e2e 脚本改为直接记录 `$!`，用 PID 来 kill。
- **规则**：后台进程一律记录 PID，用 PID 停止，不要靠模式匹配。

### 23. （协调）测试向量在 zsh 里用双引号计算，`$12` 被展开成了空串

- **现象**：简报给的 md5salt 向量（salt `abc$12`）是 `3d6431cb…`，但这其实是 `md5("password123abc")`。按契约规则（hash 取最后一个 `$` 之后的部分），正确值应为 `md5("password123abc$12")` = `1b84a6e599feeab25ace9f402aaa4858`。
- **根因**：zsh 把 `"$12"` 当作第 12 个位置参数展开成了空串（bash 则展开为 `${1}2`）。
- **修复**：`internal/auth/password_test.go` 改用正确向量，`3d6431cb…` 只作为 salt=`abc` 的用例保留。
- **规则**：含 `$` 的字符串一律用单引号（`printf '%s' 'pa$$'`）；测试向量至少用两种工具交叉验证。

### 24. （前端基础）TypeScript 7 没有编译器 API，typescript-eslint 无法工作

- **现象**：`typescript@7` 的包入口只有 `version.cjs` 和 `unstable/*`，而 `typescript-eslint@8.72` 的 peer 是 `typescript <6.1`，`require('typescript')` 拿不到 API。
- **根因**：TS 7 是 Go 原生实现，不再提供旧的 JS 编译器 API。
- **修复**：`web/.pnpmfile.cjs` 的 `readPackage` 钩子给 `typescript-eslint`、`@typescript-eslint/*` 和 `ts-api-utils` 单独注入 `typescript@6.0.3` 依赖；根目录的 `tsc` 仍是 7。效果等同于 docs/FRONTEND.md §1 里的别名方案。
- **规则**：升级到大版本前，先 `npm view <pkg> peerDependencies`，核对整条工具链能否兼容。

### 25. （前端性能）`@import "@heroui/styles"` 让阻塞渲染的 CSS 达到 457 KB（gzip 45 KB）

- **现象**：首个构建只有一个 CSS 文件，里面是全部约 85 个组件的样式，Lighthouse 的 FCP 直接被拖慢。
- **根因**：Tailwind 无法对 `@layer components` 里的组件 CSS 做 tree-shake。
- **修复**：`web/scripts/heroui-css.ts` 把每个组件的样式表（`@reference` 指向应用 CSS）挂到渲染它的 HeroUI JS 模块上。入口只保留 base、theme 和 utilities（约 11 KB gzip）；原生元素用 `import 'heroui-css:button'` 引入单个组件样式。dev 模式下依赖会被预构建，钩子不生效，所以 dev 改为全量引入。
- **规则**：每引入一个 UI 库，先看它对首屏 CSS 和 JS 的影响，再决定导入方式。

### 26. （前端性能）登录页首屏 JS 达到 232 KB gzip，远超 120 KB 预算

- **现象**：React 19（67 KB）+ TanStack Router（26 KB）+ Query（10 KB）已经约 103 KB。再加上 HeroUI/React Aria 的表单组件和 TanStack Form，公开页超出预算一倍。
- **根因**：在预算极紧的公开页上，直接用了重量级组件库和全局 Provider。
- **修复**：公开页（落地页、登录、注册、找回密码）改用原生元素加 HeroUI 的 BEM 类（`NativeField`/`NativeButton`/`buttonClass`），配合 `useLiteForm` 和内联 SVG 图标；React Query 改为由 `_user`/`_admin` 布局的 `beforeLoad` 懒加载，并注入路由 context。现在落地页约 109 KB，登录页约 111 KB。
- **规则**：先算出框架的固定成本，再决定预算剩下的部分怎么分配。每次构建后都按页面统计首屏 chunk 的总和，不要只看单个 chunk。

### 27. （前端基础）有条件地调用 React `use()`，导致 Hook 顺序错乱

- **现象**：`useT` 在字典未加载时调用 `use(promise)`，加载完成后不再调用。React 报 "called use() to suspend … but did not call use() when it finished"，Hook 顺序也随之变化，登录页的冒烟测试因此失败。
- **根因**：从挂起恢复后重新渲染时，React 要求同一个 `use` 调用仍然存在。另外，每次渲染都新建 `Promise.all` 也会触发 "uncached promise" 警告。
- **修复**：始终调用 `use()`。已加载时传入一个预先标记为 `status: 'fulfilled'` 的 thenable；合并后的 Promise 按 (locale, 列表) 永久缓存。
- **规则**：传给 `use()` 的 Promise 必须稳定且已缓存。不要在“已就绪”和“未就绪”两个分支里分别决定调不调 `use`。

### 28. （前端性能）懒加载区块里骨架屏切换成真实内容，CLS 为 0.077

- **现象**：落地页的 Lighthouse 性能分只有 99，CLS 为 0.077，偏移源是套餐卡片 `<li>`。
- **根因**：骨架列表的 key（0,1,2）和真实列表的 key（plan.id 1,2,3）部分重合，React 复用了这些 DOM 节点，并把高度 320 改成了真实高度。
- **修复**：区块组件改为用 `use(guestPlansResource())` 等待数据，chunk 和数据一起就绪后一次性显示，不再有骨架屏切换。外层的 Suspense 占位提供固定的最小高度。
- **规则**：异步内容要么一次性显示，要么占位尺寸与最终尺寸完全一致。骨架屏和真实列表不能共用 key 空间。用 puppeteer 的 `PerformanceObserver('layout-shift')` 取 sources 来定位偏移源。

### 29. （前端基础）jsdom 里的 DOMException 不是 `instanceof Error`

- **现象**：请求超时被识别成了 `network`，而不是 `timeout`。
- **根因**：jsdom 的 DOMException 不继承 Error，`err instanceof Error ? err.name : ''` 拿不到 `TimeoutError`。
- **修复**：改为按结构读取 `name`（`'name' in err`）。
- **规则**：判断 fetch/abort 的错误类型时，按 `name` 判断，不要依赖 `instanceof Error`。

## 2026-10-22

### 30. （BE-A 认证）测试预期和自己设计的计数规则对不上

- **现象**：`TestChangePassword` 先输错一次旧密码，再改密成功，然后只再输错一次，就预期下一次请求返回 429。实际返回 200。
- **根因**：`ChangePassword` 成功后会调用 `ClearPasswordFailures` 清零失败计数（这是有意设计）。写测试时忘了这一点，把改密前的那次失败也算了进去。
- **修复**：改密成功后重新输错 `password_limit_count` 次，再断言 429 和 `Retry-After`。
- **规则**：测试计数、限流、锁定这类有状态逻辑之前，先列出哪些操作会让计数增加、哪些会清零，按这张表写断言。

### 31. （BE-A 认证）slog 直接记录指针字段，日志里只有内存地址

- **现象**：`Info("user registered", "invite_user_id", u.InviteUserID)` 中的 `*uint64` 被 slog 打印成 `0xc000…`。
- **根因**：slog 对指针不会解引用，按 `%v` 输出地址。
- **修复**：先解引用到局部变量（nil 记为 0）再记录。
- **规则**：可空模型字段（`*uint64`、`*time.Time`）写日志前先解引用，或改用布尔 / 零值。

### 32. （BE-A 认证）在 Markdown 表格单元格里写了 `|`

- **现象**：给 docs/API.md 的表格加上 `plan:{id,name}|null`、`node_name|null` 后，GFM 会在 `|` 处拆分单元格，即使 `|` 在反引号里也一样。
- **根因**：（原记录未单列）
- **修复**：改成文字说明，例如“`plan` is `{id,name}` or null”。
- **规则**：表格单元格内不写裸 `|`，确实需要时用 `\|` 转义。改完契约文档后要预览表格。已有的 `"none|turnstile|…"` 也有同样的问题，留给文档负责人统一处理。

### 33. （BE-E2 内容）邮件渲染器用邮件标题覆盖了调用方传入的 `Subject`

- **现象**：`ticket_reply` 邮件正文写成「您的工单「【GG Bond】您的工单有新回复」收到了新的回复」，工单标题丢失。基础层的测试只检查了“没有 `<no value>`”，没有发现。
- **根因**：`mail.Renderer.Render` 渲染完邮件标题后执行 `view["Subject"] = subject`，而 `ticket_reply` 的契约正好用 `Subject` 传工单标题，同名键被覆盖。
- **修复**：邮件标题改放 `MailSubject`（layout 的 `<title>` 使用它）；只有调用方没传 `Subject` 时才回填。回归测试见 `internal/mail/subject_test.go`。
- **规则**：渲染器自己注入的键不能和模板数据键重名。模板测试要传有辨识度的数据值，并断言这些值确实出现在正文里，而不是只检查“没有报错”。

### 34. （BE-E2 内容）以为 validator 的 `omitempty` 对“非 nil 指针指向空串”也会跳过

- **现象**：`*string` 字段加 `binding:"omitempty,url"`，PATCH 传 `"img_url": ""`（意图是清空）却返回 400。
- **根因**：validator v10 对指针字段只在 nil 时跳过；指针非 nil 时会继续校验解引用后的值，空串于是被 `url` 规则拒绝。
- **修复**：PATCH 的可选字段一律用 `omitnil`，“空串或合法 URL”这类规则在代码里判断。先写临时测试确认了 `omitnil`、`dive` 穿透 `*[]string`、`unique` 的实际行为。
- **规则**：组合使用指针和 validator 标签时，先用临时测试验证行为再写进 handler，不要凭标签名猜语义。

### 35. （BE-E2 内容）用中间件缓存的用户对象计算邀请统计，返佣比例最多滞后 30 秒

- **现象**：测试中把用户 `commission_rate` 改为 25 后，`GET /user/invite` 仍返回 10。
- **根因**：`auth.CurrentUser(c)` 是缓存 30 秒的副本。管理员修改返佣比例时并不要求调用 `InvalidateUser`。
- **修复**：`invitesvc.Stat` 只接收 `userID`，自己从库里读取 `commission_rate`。
- **规则**：凡是展示“管理员可能刚刚修改过”的用户字段（比例、余额、套餐），或基于这些字段做计算，都要从数据库重新读取；`CurrentUser` 只用来取身份（ID、邮箱、角色）。

### 36. （BE-E2 内容）任务简报转述的 Xboard 规则与源码不一致

- **现象**：简报写的是“有工单在等待客服回复时不能新开工单”（按 `reply_status` 判断）。Xboard（legacy 和 upstream 都一样）的 `createTicket` 实际查的是 `status = 0`：只要有未关闭的工单就不能新开。upstream 的提现申请也走同一条路径，所以有未关闭工单时也无法申请提现。
- **根因**：（原记录未单列）
- **修复**：按源码实现 `status = 0` 规则（返回 409），并写进 `docs/API.md`。
- **规则**：同错题 2026-10-21 #10，转述只能当线索，要以 Service/Controller 的实际代码为准，并在汇报里说明偏差。

### 37. （BE-E2 内容）共享包里的未导出标识符可能与其他 agent 冲突

- **现象**：（险些发生）`internal/api/user`、`internal/api/admin` 由 4–5 个 agent 同时往里加文件。`toView`、`listTickets`、`userView`、测试 helper `newEnv` 这类通用名字合并后会重复声明，编译直接失败。
- **根因**：（原记录未单列）
- **修复**：（原记录未单列）
- **规则**：在共享包里新增的未导出函数、类型、变量和测试 helper，都要带自己领域的前缀（`ticket*`、`invite*`、`commission*`、`content*`、`notice*`、`knowledge*`），并且不要声明 `TestMain`。

### 38. （BE-E1 admin）以为 `binding:"email"` 会先 trim 再校验

- **现象**：测试用 `" New@Example.com "` 创建用户，期望服务层规范化成 `new@example.com`，结果返回 400 `email: must be a valid email address`。
- **根因**：gin 在 handler 之前按原始值做 validator 校验；`usersvc.NormalizeEmail` 的 trim/小写发生在校验之后。
- **修复**：测试改用不带空格的大小写混合地址；契约保持“首尾空格是非法输入”。
- **规则**：binding 标签看到的是原始请求值。需要宽松输入时，先在 handler 里规范化再调用 `httpx.Validate`；否则测试只覆盖大小写，不覆盖空白。

### 39. （BE-E1 admin）在请求里启动的后台 goroutine 用了 `context.Background()`

- **现象**：`golangci-lint` 报 gosec G118（goroutine uses context.Background while request-scoped context is available）。场景是批量邮件超出队列容量（1024）后，剩余收件人交给后台 goroutine 继续入队。
- **根因**：后台任务需要活得比请求久，于是直接新建了根 context，丢掉了请求里的值，也绕过了 linter 能检查的链路。
- **修复**：传 `context.WithoutCancel(ctx)`：保留请求值，但不随请求取消。
- **规则**：故意比请求活得久的 goroutine 一律用 `context.WithoutCancel(reqCtx)`，并在调用处写注释说明原因。

### 40. （BE-E1 admin）`mail.Queue.Enqueue` 不阻塞，队列满时直接返回 `ErrQueueFull`

- **现象**：设计批量邮件和提醒任务时，如果按“循环 `SendTemplate`”实现，用户超过 1024 个就会丢信。这个问题差点漏掉。
- **根因**：基础层邮件队列容量固定（默认 1024），满了不会等待，而是立即返回错误。
- **修复**：新增 `maintenance.Enqueue(ctx, q, msg)`，遇到 `ErrQueueFull` 时退避重试，直到 ctx 结束。批量邮件超出容量的部分交给后台 goroutine 发送。分别有 1100 封和 1300 封的测试覆盖。
- **规则**：凡是“对一批用户发信”的代码都不要直接循环 `Enqueue`/`SendTemplate`，要用 `maintenance.Enqueue`，并写一个超过 1024 封的测试。

### 41. （BE-B 节点）手算测试向量，两个期望值都写错了

- **现象**：`TestSogaUserShapes` 里 `base64(uuid[:32])` 的期望值，以及加入 `tlsSettings` 后 §7.1 的 ETag，都是凭手算写进测试的，结果两处都不对。实现本身没有问题。
- **根因**：重复了错题 #23 的老路：测试向量只用一种方式得出，没有交叉验证。
- **修复**：用 `python3 -I` 独立计算 `base64.b64encode(u[:32])` 和 `sha1(compact body)` 后再写入测试。改文档示例时，也用脚本从文档中抽出 JSON、压缩后重算 ETag，确认文档、测试、实现三方一致。
- **规则**：凡是哈希、base64、ETag 这类向量，都先用第二种工具算出来再写进测试或文档。文档示例里的 ETag 必须按“示例原文压缩后的字节”重新计算。

### 42. （BE-B 节点）测试夹具里的节点 `rate` 默认是 0，流量被记成免费

- **现象**：soga 的 traffic 上报返回 200，flush 之后 `users.u/d` 仍然是 0。
- **根因**：`model.Node.Rate` 在数据库层没有默认值。admin API 创建节点时会默认设为 1，但测试直接 `DB.Create` 的节点 rate 为 0，按倍率 0 计费，于是全部免费。这是合法配置，不是 bug。
- **修复**：测试节点显式写 `Rate: 1`。
- **规则**：绕过 API、直接写库造数据时，要逐个确认“API 层默认值”有没有带上，比如 rate、show、sort、协议默认值。断言流量时，先确认节点倍率。

### 43. （协调）任务简报与契约文档冲突时，以契约为准并记录偏差

- **现象**：BE-B 简报与 `docs/NODE_PROTOCOLS.md` 有几处不一致：
  - 节点不存在：简报写 404，契约要求 400，soga 路径上绝不能返回 404。
  - audit_log：简报写 INFO 日志，契约写 `slog.Warn`。
  - alive：简报说用 `SetNodeAlive`（整节点替换），契约 §2.4 要求逐用户替换，未上报的用户保留到 TTL。
  - alivelist：简报说所有 GET 都带 ETag，契约写明 alivelist 不带。
- **根因**：简报是对契约的摘要，细节有损。
- **修复**：一律按契约实现。简报额外要求的内容（`tlsSettings` 双写、`xver` 字符串）如果与契约不矛盾，就先改契约文档（重新计算 §7.1 的 ETag），再写代码。所有偏差都写进汇报。
- **规则**：开工时把简报和契约逐条对照，列出冲突清单。冲突时以契约为准；需要改契约的，先改文档、再改代码，并在汇报中写明。

### 44. （BE-A2 MFA）`httpx.Bind` 对 DELETE 只读 query，带 body 的 DELETE 契约会读不到参数

- **现象**：契约要求 `DELETE /user/security/passkeys/:id {password*}`，按习惯写 `httpx.Bind(c, &in)` 时，`password` 永远为空，请求总是 400。
- **根因**：`httpx.Bind` 对 GET/HEAD/DELETE 绑定 query string，不读 JSON body。
- **修复**：DELETE 使用 `c.ShouldBindJSON`，遇到 `io.EOF` 时退回 `httpx.Validate`（见 `internal/api/user/security.go` 的 `bindBody`）。密码不能放进 query。
- **规则**：写 handler 前先看 `httpx.Bind` 对该 HTTP 方法的绑定来源；DELETE 需要 body 时必须显式按 JSON 绑定，并为“空 body”写测试。

### 45. （BE-A2 MFA）往 `model.All()`、设置分组、`/guest/config` 追加内容，会打破基础层的精确计数测试

- **现象**：新增 2 个模型、1 个设置分组和 `passkey_enabled` 字段后，`model_test`（`len(All()) == 20`）、`settings_test`（`len(Groups()) == 9`）和 `guest/config_test`（精确的键列表）都失败。
- **根因**：这些测试断言的是精确数量或精确键集合，任何按约定进行的追加都会让它们失败。
- **修复**：只做最小修改：数量改为 22、10，键列表追加 `passkey_enabled`。已在汇报中列出。
- **规则**：追加共享结构（模型、设置键或分组、公开 DTO 字段）之前，先 `grep -rn "len(All()\|len(groups)\|want := \[\]string"` 找出精确断言，与改动一起最小化更新，并在汇报中写明。

### 46. （BE-A2 MFA）go-webauthn v0.18 的几个默认行为与直觉不同

- **现象**：
  - `BeginLogin(user)` 在用户没有凭据时直接返回错误，“未知邮箱也返回 options”必须改走 `BeginDiscoverableLogin`；
  - 计数器回退只会设置 `credential.Authenticator.CloneWarning`，`ValidateLogin` 不会报错；
  - 客户端返回了 RP 未请求的扩展输出时，默认（`UnsolicitedOutputPolicyReject`）会让整个仪式失败，部分密码管理器会主动返回 `credProps`；
  - `allowCredentials` 带有 `omitempty`，可发现登录时这个字段是缺省，不是 `[]`。
- **根因**：（与现象合并记录，见上）
- **修复**：邮箱登录先查用户和凭据，没有时退回可发现登录；`CloneWarning` 为真时返回 401；主动请求 `credProps`，并设置 `ExtensionsUnsolicitedOutputPolicy: Ignore`；契约中写明 `allowCredentials` 会被省略。sign_count 用条件更新防止并发重放。
- **规则**：接入安全库时逐个读 `Validate*`/`Finish*` 的源码，确认哪些检查返回 error、哪些只设置标志；用软件认证器（`internal/service/mfa/mfatest`）把每条失败路径都走一遍。

### 47. （BE-A2 MFA）简报给的“单行追加”没有按 gofmt 对齐

- **现象**：按简报原样追加 `registerSecurity(g, a) // user/security.go (BE-A2)` 后，`gofmt -l` 报出 `internal/api/user/routes.go`。
- **根因**：同一块中的行尾注释由 gofmt 统一对齐，原样粘贴的行与相邻行的列宽不一致。
- **修复**：按相邻行的列宽对齐为 `registerSecurity(g, a)    // user/security.go     (BE-A2)`。
- **规则**：修改共享文件后立即运行 `gofmt -l .`。“只追加一行”也算格式改动。

### 48. （BE-C 商城）测试按 UTC 计算“加一个月”，服务按服务器时区计算，跨夏令时差 1 小时

- **现象**：`expired_at` 断言失败，实际值比期望晚 1 小时（10-22 EDT + 1 个月 = 11-22 EST）。
- **根因**：服务用 `now.AddDate(0, months, 0)`，`now` 是本地时间（与 PHP `strtotime('+1 month')` 一致，日历月保持墙钟时间）；测试拿从数据库读回的 UTC 时间做 `AddDate`。
- **修复**：测试期望值改为 `paidAt.Local().AddDate(...)`，并用 `TZ=UTC`、`TZ=Asia/Shanghai` 各跑一遍。
- **规则**：凡是“日历月/日”的计算，测试必须在与代码相同的时区里算期望值；时间相关测试至少换两个 `TZ` 跑。

### 49. （BE-C 商城）以为覆盖 `svc.Now` 就能让 `Scheduler.RunNow` 的任务也用假时钟

- **现象**：`RunNow("commission_settle")` 没有结算任何订单。
- **根因**：`app.RegisterJob` 注册的函数每次 `New(a)` 新建服务，用的是真实时钟，测试里改的是另一个实例。
- **修复**：测试改为把数据库里的 `paid_at` 往前调，再跑 `RunNow`。
- **规则**：通过调度器跑任务的测试，靠修改数据（时间戳）制造条件，不要依赖替换服务实例上的时钟。

### 50. （BE-C 商城）`e.WithField(...)` 不接返回值被 errcheck 报错

- **现象**：`golangci-lint` 报 `Error return value of e.WithField is not checked`。
- **根因**：`(*httpx.Error).WithField` 返回 `*Error`（实现了 `error`），丢弃返回值会被 errcheck 当成未处理的错误。
- **修复**：改用 `map[string][]string` 收集字段错误，最后一次性构造 `httpx.BadRequest` 并赋值 `Errors`。
- **规则**：累积多个字段错误时用收集器，不要以语句形式调用返回 `error` 类型的链式方法。

### 51. （BE-C 商城）任务简报说 Xboard 取消订单会归还优惠券次数，实际两个版本都不归还

- **现象**：对照 `CouponService::use` 与两版 `OrderService::cancel`：创建订单时 `limit_use - 1`，取消只退余额，不退券。
- **根因**：简报凭印象描述 Xboard 行为，没有对照代码。
- **修复**：按简报实现“创建扣减、取消归还”（避免超时取消白白耗掉券），在 `docs/PAYMENTS.md`、`docs/DATA_MODEL.md` 写明这是相对 Xboard 的有意差异。
- **规则**：“照 Xboard 做”之前先读对应的 Service 代码确认；与 Xboard 不同的地方必须写进契约文档。

### 52. （BE-C 商城）批量文本替换误伤类型名

- **现象**：用脚本把测试里的 `.ResponseRecorder` 全部删掉，连 `*httptest.ResponseRecorder` 也变成了 `*httptest`，编译失败。
- **根因**：替换模式太宽，没有限定上下文。
- **修复**：单独修回类型名。
- **规则**：批量替换的模式要带足上下文（如 `).ResponseRecorder`），替换后先 `go build`/`grep` 核对再继续。

### 53. （BE-C 商城）gosec G704 把支付网关请求当成 SSRF

- **现象**：`gwutil.Do` 中 `client.Do(req)` 被 G704 标记为“taint 分析 SSRF”。
- **根因**：网关地址来自管理员配置（EPay/Epusdt/BTCPay 本来就是自托管地址），属于设计如此。
- **修复**：在该行加 `//nolint:gosec // G704: gateway endpoints come from admin configuration by design`，并保留 https 校验（EPay/Epusdt 的 http 只告警）。
- **规则**：外呼地址来自管理员配置时，集中在一个函数里发请求并在那里统一说明 nolint 理由，不要到处散落。

### 54. （Xboard 导入）SQLite 夹具类型写得随意，换到 PostgreSQL / MySQL 加载就报错

- **现象**：同一份 `testdata/*_schema.sql` 在 SQLite 上一切正常；集成测试把它转到 PostgreSQL 时报 `integer out of range`（100 GiB 的 `transfer_enable` 塞进了 `integer`），转到 MySQL 时报 3780 外键列类型不兼容（`v2_server.machine_id` bigint 引用 `integer` 主键）。
- **根因**：SQLite 是动态类型，`integer` 能存 64 位、也不检查外键两端类型；转写迁移时把 `bigInteger()` 和 `id()`（bigint unsigned）都写成了 `integer`。
- **修复**：夹具里 `bigInteger()` 列写 `BIGINT`；集成测试转换 DDL 时主键统一用 `bigint`。
- **规则**：跨库夹具按 Laravel 的真实列类型转写（`bigInteger`→BIGINT、`id()`→bigint、`boolean`→各库原生布尔），并至少在 PostgreSQL 上真跑一次加载，SQLite 通过不代表 DDL 正确。

### 55. （Xboard 导入）契约要求的种子行违反了迁移里的 NOT NULL 约束

- **现象**：`docs/IMPORT_XBOARD.md` §9 要求 legacy 夹具里有 “hysteria `version` NULL” 和 “`group_id` NULL” 的节点，但迁移把 `v2_server_hysteria.version` 定义为 `NOT NULL DEFAULT 1`，legacy `group_id` 也是 NOT NULL，这两行根本插不进去。
- **根因**：写契约时只看了读取代码的分支（`NULL → 1`），没对照建表迁移的约束。
- **修复**：夹具用 `version = 0`，并在健壮性测试里 `DROP COLUMN` 模拟缺列的老 fork；NULL 的 group 列表改由 upstream 的 `group_ids` 覆盖；文档已更正。
- **规则**：设计夹具前先逐列核对迁移里的 NOT NULL / default / unique；“NULL 怎么处理”的分支要说明它在哪种 schema 下才可能出现。

### 56. （Xboard 导入）两份契约对同一网关配置键说法不一

- **现象**：`IMPORT_XBOARD.md` §4.9 写 Stripe 导入 `publishable_key`、`currency` 转大写、Coinbase 保持启用、AlipayF2F `sandbox=false`；而 `PAYMENTS_GATEWAYS.md` 已核实：hosted Checkout 不用 publishable key、currency 用小写 ISO、键名是 `payment_methods`、Coinbase Commerce 已于 2026-03-31 关闭（旧 key 失效，必须禁用）、`sandbox` 是 `0`/`1`。
- **根因**：导入文档早于网关文档定稿，之后没有回头同步。
- **修复**：按 `PAYMENTS_GATEWAYS.md` 实现并更正 `IMPORT_XBOARD.md` §4.9。
- **规则**：写映射前，把目标字段的每个键名、取值格式与“拥有该字段”的契约逐一核对；两份文档冲突时以更新、更具体的那份为准，并在汇报里写明。

### 57. （Xboard 导入）警告文本差点把用户凭据打进日志

- **现象**：重复 uuid / 无效 token 的警告初稿用 `%q` 打印了原值；这些警告会写到 stdout 并镜像到 slog。
- **根因**：把 uuid、订阅 token 当成普通标识符，忘了它们就是节点密码 / 订阅凭据。
- **修复**：警告只写用户 id 和原因，不写值。
- **规则**：凡是能直接登录或连接的值（密码哈希、uuid、订阅 token、server_token、网关密钥）都不得出现在警告、错误或日志里；自查时 `grep -n '%q'` 逐条看格式化的是什么。

### 58. （集成）Dockerfile 的前端阶段漏拷 `.pnpmfile.cjs`，`pnpm install --frozen-lockfile` 报 `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`

- **现象**：本地 `pnpm install` 正常，Docker 构建失败：lockfile 里的 `pnpmfileChecksum` 与当前配置不一致。
- **根因**：前端基础 agent 后来加了 `web/.pnpmfile.cjs`（给 typescript-eslint 单独提供 TypeScript 6），而 Dockerfile 只 `COPY` 了 `package.json`、`pnpm-lock.yaml`、`pnpm-workspace.yaml`。
- **修复**：`COPY` 时一并复制 `.pnpmfile.cjs`（以及将来可能出现的 `.npmrc`）。
- **规则**：凡是影响 pnpm 解析的文件（`.pnpmfile.cjs`、`.npmrc`、`pnpm-workspace.yaml`）都必须进入 Docker 的依赖安装层；改了任何一个就本地跑一次 `docker build`。

### 59. （订阅）契约里的客户端能力表没有核对官方文档

- **现象**：`docs/SUBSCRIPTION.md` 写 Surge 支持 VLESS、Loon 支持 TUIC、Stash 不支持 anytls/mieru、sing-box 按 1.11 schema 输出。
  核对后：Surge 手册（2026-10）的协议页里根本没有 VLESS；Loon 节点文档没有 TUIC；Stash 文档已列出 anytls 与 mieru；
  sing-box 最新稳定版是 1.14.3，旧 DNS server 格式（`address`）与 block/dns 特殊出站已被移除，anytls 出站从 1.12 才有。
- **根因**：能力表凭印象和 Xboard 旧代码写成，没有逐个客户端查文档。
- **修复**：按官方文档实现各格式的 `Supports`/`SupportsProxy`，sing-box 统一输出 1.12+ schema（typed DNS、rule action），
  契约表格改为实际渲染范围；Stash 仍按契约不下发 anytls/mieru（兼容旧版本，已在汇报中说明）。
- **规则**：写订阅格式前，每个客户端都要打开它的官方手册确认协议、字段名和最低版本；表格里的每一项都要能指到出处。

### 60. （订阅）Surge 手册旧链接 404

- **现象**：`https://manual.nssurge.com/policy/proxy.html` 返回 404。
- **根因**：手册已拆成按协议的页面：`policies/shadowsocks.html`、`policies/vmess.html`、`policies/trojan.html`、`policies/hysteria2.html`、
  `policies/tuic.html`、`policies/anytls.html`，TLS 公共参数在 `policies/tls.html`。
- **修复**：（原记录未单列）
- **规则**：官方文档链接 404 时先搜同站新路径，不要回退到第三方转述。

### 61. （订阅）sing-box hysteria2 的 `server_ports` 与 `server_port` 互斥

- **现象**：端口跳跃节点同时写 `server_port` 和 `server_ports` 会被 sing-box 拒绝（文档：Conflicts with `server_port`）。
- **根因**：（原记录未单列）
- **修复**：有端口跳跃时只输出 `server_ports`（`"20000:30000"`，冒号分隔）和 `hop_interval`。
- **规则**：同一能力在不同客户端的写法差异很大：mihomo `ports: 20000-30000`、Stash 逗号列表、Surge `port-hopping=a-b;c`、
  Loon `server-ports="a:b"`、sing-box `["a:b"]`；每种都要按各自文档写测试。

### 62. （订阅）短链接 `/s/:token` 的 token 会进访问日志

- **现象**：基础设施的 AccessLog 记录 `c.Request.URL.Path`，查询串里的 token 被去掉了，但路径里的 token 没有。
- **根因**：（原记录未单列）
- **修复**：`/s/:token` 处理器在读取参数后把 `c.Request.URL.Path` 改写成 `/s/[redacted]`（AccessLog 在 `c.Next()` 之后才读路径），
  测试断言日志环形缓冲里不出现 token。
- **规则**：凡是路径参数携带凭据的路由，都要检查访问日志、panic 日志里有没有它；更好的做法是 AccessLog 记录 `c.FullPath()`（交集成者评估）。

### 63. （订阅）用 map 指针做缓存键，GC 后地址可能被复用

- **现象**：`rulesets.Current` 想用设置值（map）的指针判断 `rulesets_custom` 是否变化；缓存只保存了指针值时，旧 map 被回收后新 map
  可能分配在同一地址，导致命中过期的合并结果。
- **根因**：（原记录未单列）
- **修复**：缓存条目同时持有设置值本身（保持可达，地址不会被复用），再比较指针。
- **规则**：用指针身份做缓存键时，必须让缓存持有该对象的引用；否则改用内容哈希或版本号。

### 64. zsh 下 `grep --include=*.go` 又一次没加引号（重犯第 8 条）

- **现象**：`grep -rn "_health" --include=*.go internal` 报 `no matches found`，命令没有执行。
- **根因**：（原记录未单列）
- **修复**：（原记录未单列）
- **规则**：重申：zsh 中所有含 `*` 的参数一律加引号（`--include='*.go'`）；看到 “no matches found” 先怀疑 shell 而不是代码。

### 65. （集成）冒烟测试时旧进程还占着端口，新二进制根本没起来，404 被误判为“路由没合并”

- **现象**：合并订阅分支后重新 `go build` 并 `./ggboard serve`，`/api/v1/client/subscribe` 仍 404，而 `/s/<token>` 返回 200（其实是 SPA 兜底页）。
- **根因**：上一轮的 `ggboard serve` 没被 `pkill` 干净，新进程 `bind: address already in use` 退出，旧进程继续应答；没先看 `server.log`。
- **修复**：`lsof -iTCP:<port> -sTCP:LISTEN` 确认监听进程并 kill，再启动；之后一切正常。
- **规则**：启动服务后先 `head server.log` 看到 "listening" 再发请求；非 `/api` 路径的 200 可能只是 SPA 兜底，不能当作接口存在的证据。

### 66. （FE-1）路由 `loader` 里静态导入 query 工厂，React Query 被打进了所有页面的入口

- **现象**：给 `/dashboard` `/subscription` `/profile` 写了 `loader: () => queryClient.query(subscriptionQuery())` 之后，入口 chunk 从 101.5 KB 涨到 115.2 KB（gzip），登录页首屏超过 120 KB 预算。
- **根因**：TanStack Router 的 `autoCodeSplitting` 默认只拆 `component`/`errorComponent`/`notFoundComponent`/`pendingComponent`，`loader` 和 `validateSearch` 留在 routeTree 里，而 routeTree 属于入口。loader 静态导入的 `@/lib/api/queries` 又带上了 `@tanstack/react-query` 和全部 API 模块。
- **修复**：loader 内 `const q = await import('@/lib/api/queries')` 动态导入；入口回到 103.4 KB，登录页 111.3 KB。
- **规则**：路由文件顶层只放 `component` 会用到的导入；loader 需要的模块一律在 loader 内动态 `import()`。每次加 loader 后按页面统计首屏 chunk（入口 + 路由 chunk 的静态依赖闭包），和改动前对比。

### 67. （FE-1）Vitest 5 的文件系统模块缓存让新增的 i18n JSON 在测试里“不存在”

- **现象**：新增 `src/i18n/zh-CN/dashboard.json` 后，dev server 正常，测试里却一直报 `[i18n] missing namespace zh-CN/dashboard`，`availableNamespaces()` 只返回旧的 5 个命名空间。
- **根因**：`node_modules/.vitest-cache` 按模块内容缓存转换结果，`src/i18n/index.ts` 本身没变，所以 `import.meta.glob('./*/*.json')` 的展开结果一直是旧的。删除 `node_modules/.vite/vitest` 没有用。
- **修复**：`rm -rf node_modules/.vitest-cache` 后重跑。
- **规则**：新增/删除被 `import.meta.glob` 匹配的文件后，测试表现与 dev 不一致时，先清 `node_modules/.vitest-cache`。CI 从干净目录安装，不受影响。

### 68. （FE-1）`useLiteForm` 提交失败后用 rAF 聚焦首个错误字段，抢走了用户正在输入的焦点

- **现象**：登录冒烟测试偶发失败：邮箱框只有 `a`，其余字符 `@example.com…` 被打进了密码框。
- **根因**：首次空提交后 `requestAnimationFrame` 才去 focus 第一个 `[aria-invalid]`；用户（或 user-event）此时已经点进邮箱框开始输入，邮箱的错误被清掉，rAF 回调执行时第一个错误字段变成了密码框。真实用户快速输入时也会遇到。
- **修复**：记录提交时的 `document.activeElement`，rAF 时焦点已经被用户移动就不再抢焦点。
- **规则**：延迟执行的焦点管理必须先确认焦点仍在原处；偶发失败的测试先看 DOM 快照里的值落在哪个字段。

### 69. （FE-1）业务接口的 401 会被 API client 当成会话过期而登出

- **现象**：协调说明里 `/user/security/passkeys/finish` 验证失败可能返回 401；共享 client 对任何带 token 的 401 都会 `logout()` 并跳转登录页。
- **根因**：client 只按状态码判断会话，不知道哪些 401 是业务错误。
- **修复**：`RequestOptions.keepSession`，passkey finish 调用时传 `true`；登录页的 401/403/429 按状态映射成本地化文案（后端 message 是英文）。
- **规则**：新增会返回 401 的业务端点时，前端调用处显式声明 `keepSession`；契约里尽量用 400/409 表达业务失败。

### 70. （FE-1）测试里 `getByRole('alert')` 命中了上一个用例残留的 HeroUI toast

- **现象**：`Found multiple elements with the role "alert"`，其中一个是上一个测试的“登录成功”toast。
- **根因**：HeroUI 的 toast 队列是模块级单例，`cleanup()` 不会清空它。
- **修复**：按文本查找后断言 `role="alert"`：`expect(screen.getByText('…')).toHaveAttribute('role', 'alert')`。
- **规则**：断言表单错误时不要用全局唯一的 `getByRole('alert')`。

### 71. （FE-1）`findByLabelText(/邮箱/)` 同时匹配“邮箱”和“邮箱验证码”，一直重试到测试超时

- **现象**：注册/找回页的测试 5 秒超时，看不到明确报错。
- **根因**：`findBy*` 遇到多个匹配会抛错并持续重试，直到超时，表现成“页面没渲染出来”。
- **修复**：用精确字符串 `findByLabelText('邮箱')`。
- **规则**：label 有包含关系时用精确字符串；`findBy*` 超时先检查是不是匹配到了多个元素。

### 72. （FE-2 商城/工单）Vitest 5 的 `fsModuleCache` 缓存了 `import.meta.glob` 的展开结果，新增的 i18n 词典在测试里“不存在”

- **现象**：新建 `src/i18n/*/shop.json` 等文件后，dev server 正常，但 Vitest 里报 `[i18n] missing namespace zh-CN/shop`，页面只显示 key（`title`、`subtitle`）。`availableNamespaces('zh-CN')` 在测试里只有旧的 5 个。
- **根因**：Vitest 5 默认开启持久化模块缓存（`node_modules/.vitest-cache`），按源文件内容做 key。`src/i18n/index.ts` 本身没变，缓存里还是旧的 glob 展开结果。
- **修复**：`pnpm exec vitest --clearCache` 后重跑。
- **规则**：新增/删除被 `import.meta.glob` 匹配的文件（词典、路由片段等）后，先清 Vitest 缓存；测试里出现“文件明明在却找不到”时第一时间怀疑缓存。集成者可考虑在 `vite.config.ts` 的 `test` 里设 `fsModuleCache: false`。

### 73. （FE-2）共享的浏览器面板里没带 `tabId`，操作落到了另一个 agent 的标签页

- **现象**：点击-输入-提交的一组 `browser_batch` 操作中途，另一个并行 agent 打开了自己的标签页（`localhost:5291/login`）并成为活动标签；我没写 `tabId` 的后续操作把测试文字输入到了对方的登录表单里。
- **根因**：内置浏览器面板被多个 agent 共享，省略 `tabId` 时操作的是“当前活动标签”，而活动标签会被别人切换。
- **修复**：此后每个浏览器调用都显式传 `tabId`，并用 `form_input` + `ref` 代替依赖焦点的键盘输入。
- **规则**：并行开发时，所有 `mcp__Claude_Browser__*` 调用（含 batch 里的每一项）都必须带自己的 `tabId`；开始前先 `tabs_context` 记下自己的 tab；dev server 用独占端口（本次 5287 / mock 8787），不要用默认 5173。

### 74. （FE-2）把 `<form>` 包在 `Modal.Body`/`Modal.Footer` 外面后，HeroUI 的间距消失

- **现象**：新建工单、佣金转入/提现弹窗里，标题和第一个字段、正文和按钮行贴在一起。
- **根因**：HeroUI 用相邻兄弟选择器给间距（`.modal__header + .modal__body { mt-2 }`、`.modal__body + .modal__footer { mt-5 }`），中间多了一层 `<form>` 就不再是兄弟元素；`display: contents` 也救不了兄弟选择器。
- **修复**：表单内的 `Modal.Body` 加 `mt-2`，`Modal.Footer` 加 `mt-5`。
- **规则**：在 HeroUI 复合组件的部件之间插入包裹元素前，先查它的 CSS 是否依赖 `+`/`>` 结构选择器。

### 75. （FE-2）共享的 `Markdown` 组件（DOMPurify 默认 URI 白名单）会删掉 `clash://` 之类的一键导入链接

- **现象**：知识库文章里的「一键导入 Clash」链接渲染后没有 `href`。
- **根因**：DOMPurify 默认 `ALLOWED_URI_REGEXP` 只放行 http(s)/mailto/tel 等协议，客户端深链接全部被当成危险 URL。
- **修复**：知识库改用 `features/content/KnowledgeMarkdown.tsx`：同样懒加载 `marked` + `dompurify`，但用独立的 DOMPurify 实例和显式的客户端协议白名单（`javascript:`/`data:` 仍被删除，有测试覆盖）。
- **规则**：渲染第三方/管理员内容时，先确认链接协议需求；放宽白名单只能用显式列表，不能用“除了 javascript 都放行”。

### 76. （FE-2）观察到：`src/test/login.test.tsx` 在整套测试并行跑时偶发超时

- **现象**：`Unable to find role="alert"`，单独跑或重跑都通过；基线（未加 FE-2 代码）3 次里也失败过 1 次。
- **根因**：`findBy*` 默认 1 s 超时，机器负载高（多个 agent 并行构建/测试）时不够。
- **修复**：（原记录未单列）
- **规则**：新写的页面级测试统一给 `findBy*` 传 `{ timeout: 4000 }`；该 smoke 测试的属主应同样放宽超时（本次未修改他人测试）。

### 77. （FE-3）新增的 i18n JSON 在 Vitest 里不可见，`t()` 直接返回 key

- **现象**：新加的 `admin-nodes.json` 在浏览器里正常，但 `ProtocolForm.test.tsx` 渲染出 `nodes.ps.tcpHeader` 之类的原始 key；`availableNamespaces('zh-CN')` 只列出旧的 5 个命名空间。
- **根因**：`src/i18n/index.ts` 用 `import.meta.glob` 枚举字典，Vitest 把转换结果缓存在 `node_modules/.vitest-cache`，新文件加入后缓存里的 glob 结果没有失效。
- **修复**：`rm -rf node_modules/.vitest-cache` 后重跑，测试即拿到新字典。
- **规则**：新增会被 `import.meta.glob` 匹配的文件（字典、路由片段等）后，若测试表现得像文件不存在，先清 Vitest 缓存再排查代码。

### 78. （FE-3）浏览器面板的标签页被其他 agent 共用

- **现象**：调试 `/admin/nodes` 时，`tab-3` 突然跳到了另一个 worktree 的 `localhost:5291/subscription`。
- **根因**：内置浏览器面板在所有会话间共享，不带 `tabId` 的操作作用于“当前活动标签页”，其他 agent 也在用。
- **修复**：用 `preview_start({url})` 新开自己的标签页，之后所有调用都显式传 `tabId`；dev server 与 mock 使用不常见端口（5183 / 8093）。
- **规则**：并行开发时，浏览器验证一律在自己新开的标签页里进行并显式传 `tabId`；端口不要用默认的 5173 / 8080。

### 79. （FE-3）截图滞后于 DOM，坐标点击小图标不触发 React Aria `onPress`

- **现象**：点击后立即截图仍显示点击前的画面（抽屉、确认框“没有出现”）；按截图坐标点击表格里的删除图标没有反应，而 JS `element.click()` 能正常打开确认框。
- **根因**：截图异步且可能取到上一帧；缩放后的视口坐标落在 16px 图标边缘时，指针事件未命中按钮。
- **修复**：用 `javascript_tool` / `read_page` 核对 DOM 状态，用 `find` 返回的 `ref` 或 JS 触发点击；需要看画面时再截一次。
- **规则**：判断“有没有渲染出来”以 DOM 查询为准，截图只用于看布局。

### 80. （FE-3）`react-hooks/purity` 禁止在渲染期调用 `Date.now()`

- **现象**：lint 报 “Cannot call impure function during render”（判断用户是否过期、生成 key）。
- **根因**：React Compiler 规则要求渲染是纯函数。
- **修复**：新增 `features/admin/shared/useNow.ts`（`useSyncExternalStore` + 每分钟一次的共享时钟）；弹层的 `key` 改为实体 id。
- **规则**：渲染里需要“当前时间”时用 `useNow()`，不要直接 `Date.now()`；`key` 用稳定的业务 id。

### 81. （FE-3）`@tanstack/query/no-unstable-deps`：把 `useMutation()` 的返回值放进 `useMemo` 依赖

- **现象**：表格列 `useMemo` 依赖了整个 mutation 对象，lint 报错（且每次渲染都会重建列）。
- **根因**：`useMutation` 的结果对象每次渲染都是新引用。
- **修复**：解构出 `mutate` / `isPending` 再放入依赖。
- **规则**：列定义、回调依赖里只放 `mutate`、`isPending` 这类稳定或原始值。

### 82. （FE-3）`applyServerErrors` 只匹配顶层字段，嵌套路径错误会落到表单横幅

- **现象**：节点保存返回 `errors: {"protocol_settings.tls_settings.private_key": [...]}`，按 `name in form.state.values` 匹配不到任何字段。
- **根因**：通用助手只认表单顶层 key；协议配置是一个整体 JSON 字段。
- **修复**：`splitNodeErrors()` 先按 `protocol_settings.` 前缀拆分：协议错误交给 `ProtocolForm` 按路径显示，其余再交给 `applyServerErrors`，并自动切到有错误的标签页。
- **规则**：后端按 JSON 路径返回错误时，前端要按路径路由到具体输入；不要假设通用助手能处理嵌套 key。

### 83. （FE-3）React Aria 集合的 key 不能是空字符串；流量坐标轴按十进制取整

- **现象**：`flow: ""`（无流控）、“无父节点”等选项用 `''` 作 key 时选择不稳定；节点流量图的 Y 轴出现 186 GB / 140 GB / 93 GB 这样的刻度。
- **根因**：空字符串 key 与“未选择”混淆；`niceTicks` 在字节值上按 10 的幂取整，再按 1024 进制显示。
- **修复**：空值选项使用哨兵 key（`'0'`、`'__empty__'`）；图表新增 `tickUnit` + `byteUnit()`，按显示单位取整（0 / 50 / 100 / 150 / 200 GB）。
- **规则**：Select/ListBox 的 key 永远非空；带单位的轴先换算到显示单位再取整。

### 84. （FE-4 管理后台 B）路由文件的 `validateSearch` 依赖把 React Query 拉进了首屏入口

- **现象**：`/admin/rulesets` 的路由文件从 `features/admin/rulesets/catalog.ts` 导入 `RULESET_TABS`，而该模块顶部 `import { qk } from '@/lib/api/queries'`。`queries.ts` 又导入 `@tanstack/react-query` 和全部 API 模块，结果它们全部进入了 `routeTree` 所在的入口 chunk。登录页冒烟测试也随之变慢、变得不稳定。
- **根因**：自动代码分割只拆 `component`，`validateSearch`、`loader`、`beforeLoad` 及其 import 都留在入口 chunk。
- **修复**：把路由选项依赖的常量和解析函数拆成零依赖小模块（`rulesets/tabs.ts`、`orders/search.ts`）；URL → API 的映射（`orders/query.ts`、`tickets/query.ts`）只由组件导入。
- **规则**：路由文件中除 `component` 以外的代码只能 import 零依赖或已在入口中的模块。每次 `pnpm build` 后用 `grep -o 'assets/[^"]*\.js' dist/index.html` 加 gzip 统计首屏体积，并和改动前对比。

### 85. （FE-4）`git stash -u` 之后 vitest 缓存的 `import.meta.glob` 结果过期，新增 i18n 字典“消失”

- **现象**：为了对比基线执行了 `git stash push -u` → 跑测试 → `git stash pop`。之后所有用新命名空间的测试都只渲染出 key（`title`、`description`），单独跑也失败。
- **根因**：vitest 5 的模块缓存 `node_modules/.vitest-cache` 以导入方（`src/i18n/index.ts`）的内容为键，缓存了 glob 展开结果。stash 期间新 JSON 文件不存在，pop 回来后 `index.ts` 内容没变，缓存仍然命中。
- **修复**：`rm -rf web/node_modules/.vitest-cache`。
- **规则**：新增或删除 `import.meta.glob` 匹配的文件后，如果测试“看不到”它们，先清 `.vitest-cache`。对比基线时优先用 `git worktree` 或单独 clone，不要用 `stash -u`。

### 86. （FE-4 发现，属基础层）`useLiteForm` 提交失败后用 rAF 聚焦第一个无效字段，抢走用户正在输入的焦点

- **现象**：并行跑测试时登录冒烟测试大约 60% 失败。调试输出显示 email=`a`、password=`@example.comwrong-password`：输入第一个字符后焦点跳到了密码框。
- **根因**：空表单提交后，`requestAnimationFrame` 才去找 `[aria-invalid="true"]` 并聚焦。如果在这一帧内用户已经开始在邮箱框输入，邮箱框的错误先被清掉，于是“第一个无效字段”变成了密码框。CPU 越忙，这个竞态越容易出现，真实用户快速输入时也可能遇到。
- **修复**：记录提交时的 `document.activeElement`，rAF 回调中如果焦点已经变化就不再抢焦点（`src/hooks/useLiteForm.ts`，最小改动）。
- **规则**：异步（rAF、setTimeout、await 之后）移动焦点前，先确认焦点仍在原位置。测试只在并行时失败，要先打印实际 DOM 状态再下结论，不要直接加大超时。

### 87. （W3 文档）文档冲突按“两边都保留”合并，留下重复表格和过期说法

- **现象**：合并后的 `docs/API.md` 里 Guest / Account / Settings / System 的表格行各出现两次；`ARCHITECTURE.md` 的定时任务表重复了 `commission_settle`、`stats_daily`；`SUBSCRIPTION.md` 还留着一整段“Original sketch, kept for history”；多处写着已被代码推翻的行为，比如“封禁用户的订阅返回 403”。
- **根因**：十二个 agent 并行改文档，冲突一律按“keep both sides”解决，合并后没人对照最终代码重读。
- **修复**：W3 文档专项：逐条路由、逐个模型、逐个设置键对照代码重写；用脚本检查表格列数和重复行。
- **规则**：文档冲突不能按“两边都保留”自动解决，必须对照代码人工合并。合并后跑一遍重复行检测和表格列数检查；写进契约的行为要同时核对 handler 和 service 两层。

### 88. （W3 文档）用 `s.index('---', start)` 找章节结尾，命中了表格分隔行

- **现象**：替换 `AGENTS.md` §7 的插件表时，新表写进去了，旧表的数据行和旧结尾句却留在新表下面。
- **根因**：章节之间的分隔线是 `---`，但表格分隔行 `|---|---|---|` 里也有 `---`。`index` 找到的是表头下面那一行，替换范围只盖住了表头。
- **修复**：删掉残留的旧表，并用 `git diff AGENTS.md` 确认只剩新表。
- **规则**：脚本替换 Markdown 章节时，按下一个二级标题（`##` 开头的行）或整行 `\n---\n` 定位结尾，不要搜裸子串 `---`。任何区间替换之后都要立刻看 `git diff`。

### 89. （W3 文档）批量改 Markdown 的脚本和 `markdownlint --fix` 改坏了内容

- **现象**：
  - 一个按区间替换的脚本把结尾锚点放在代码示例之后，示例被删掉了。
  - 给标题和列表补空行的正则在相邻列表项之间也插了空行，把列表拆散了。
  - `markdownlint --fix` 把 `{a,b}` 形式的 URL 自动链接截断在逗号处。
- **根因**：正则和锚点没有考虑代码块、`1.` 加空格的有序列表标记和花括号 URL；修改后先跑 lint，没有先看 diff。
- **修复**：从 git 恢复被删的示例并清理多余空行；花括号 URL 改为放进代码 span。
- **规则**：批量修改 Markdown 后先 `git diff` 逐段核对，再跑 lint。`--fix` 先在副本上试，或只开 MD022/MD031/MD032/MD058 这类只加空行的规则。

### 90. （W3 文档）首版文档有上百条 markdownlint 空行与表格错误

- **现象**：重写后的 `API.md` 首版有 128 条 MD022/MD031/MD032/MD058。新版 markdownlint（0.42）还新增了 MD060，紧凑写法 `|---|` 被报为表格列样式问题。
- **根因**：沿用了旧文档的紧凑风格：标题、列表、表格、代码块前后不留空行。
- **修复**：用脚本补空行，新文件的分隔行改为 `| --- |`。
- **规则**：标题、列表、表格、代码块前后各留一个空行。表格单元格里的 `|` 一律写成 `\|` 或改写措辞，包括反引号代码里的，因为 GFM 在代码 span 里同样会按 `|` 拆列。lint 结果按规则 ID 计数汇总，不要用 `sort | uniq` 去重原始输出。

### 91. （W3 文档）重犯 #9：长输出被 rtk 截断，差点按残缺内容写文档

- **现象**：多个文档 fork 读 `grep`/`cat` 时输出末尾出现 `(N filtered)`、`[+N hidden: rtk recall …]`，部分路由和字段没显示出来。
- **根因**：没先读错题本 #9，又一次把截断后的输出当成了全文。
- **修复**：改用 `/usr/bin/grep`、`sed -n 'a,bp'` 分段读取，长文件用 Read 工具整篇读。
- **规则**：同 #9。核对契约前先确认输出里没有 rtk 的截断提示；有就分段重读。

### 92. （W3 文档）凭印象写交叉引用

- **现象**：`DATA_MODEL.md` 引用了 `MISTAKES.md` 里不存在的条目；旧条目 41 写“见错题 #20”，实际应为 #23。
- **根因**：交叉引用凭记忆写，没有 grep 目标文件；编号重排后也没有复查引用。
- **修复**：改为指向真实位置（`BACKEND.md` §9、错题 #23）。
- **规则**：写交叉引用前先 grep 目标文件确认标题或编号存在；重新编号后全文搜索 `#\d+` 和 `§` 复查引用。

<!-- 在本行上方按同样格式追加新条目（### N. 标题 + 现象/根因/修复/规则 四个字段），编号顺延，并把编号补进上方目录。 -->

### 93. （后端评审）shell 包装层把 golangci-lint 的输出换成了一条无关的 ESLint 提示
- **现象**：`golangci-lint run ./... > lint.txt 2>&1` 之后 `tail lint.txt` 显示 "The unix formatter is no longer part of core ESLint"，看起来像 lint 根本没跑，其实有 1 个 goimports 问题。
- **根因**：本机 shell 包装层（rtk）对命令输出做过滤/改写（同错题 #9）。
- **修复**：直接 `golangci-lint run ./... > file`，再用 `od -c`/`sed -n` 读文件原文；以退出码为准。
- **规则**：质量门禁一律看退出码 + 重定向到文件的原始输出；输出内容和命令对不上时，先怀疑包装层。

### 94. （后端评审）zsh 未加引号的 glob 第三次让 grep 整条没执行（重犯 #8、2026-10-22 #6）
- **现象**：`grep -n ... internal/model/mail*.go` 和 `grep ... Dockerfile*` 报 `no matches found`，同一条命令里的其他 grep 也没跑。
- **规则**：zsh 里任何含 `*` 的路径都先 `ls` 确认存在，或加引号交给工具展开；“no matches found” 说明命令没执行，不是“没有结果”。

### 95. （后端评审）用 Python 普通字符串写替换脚本，`\pL` 变成 `\\pL` 后断言失败
- **现象**：替换 `validate.go` 注释里的 `[\pL\pN]{1000}` 时，脚本里写成 `'\\\\pL'`，与文件内容对不上，`assert old in s` 失败（好在断言拦住了，没有写坏文件）。
- **规则**：含反斜杠的源码片段一律用 `r'''…'''` 原始字符串；每个替换都 `assert s.count(old) == 1`。

### 96. （后端评审）GORM 扫描进“内嵌未导出类型”的匿名结构体，字段全是零值
- **现象**：`[]struct{ knowledgeRow; Body string }` 查询后 id/title/category 全为空，测试才发现。
- **根因**：GORM 解析 schema 时忽略未导出的内嵌类型字段。
- **修复**：内嵌导出类型（`KnowledgeSummary`）并显式列出其余字段。
- **规则**：给 GORM 的扫描目标只用导出字段/导出内嵌类型；新查询必须有断言字段值的测试。

### 97. （后端评审）凭直觉定阈值：用指令数限制正则成本，挡不住真正昂贵的模式
- **现象**：`MaxRegexInsts = 2000` 放过了 `[\pL\pN]{1000}`（1002 条指令，但含 150 万个 rune 区间）。
- **修复**：在 scratchpad 写小程序实测几种模式的指令数、rune 区间数和编译耗时，再定 256 指令 + 4096 区间两个上限。
- **规则**：任何“成本上限”先实测代表性输入（正常值和攻击值），把测量结果写进常量注释。

### 98. （后端评审）改缓存键方案前没查谁依赖旧的命名空间失效语义
- **现象**：把规则档缓存改成每用户命名空间后，`TestStoredProfiles` 用 `InvalidateNamespace(engine.ProfileNamespace)` 清缓存失效。
- **修复**：键同时带全局命名空间和每用户代数（`Key(ProfileNamespace, Key(perUserNS, ""))`），两种失效都生效。
- **规则**：改缓存键/失效方式前先 `grep -rn "InvalidateNamespace(<ns>)\|<ns>"`，包括测试。

### 99. （后端评审）“修复前必失败”验证时只 stash 了实现文件，测试引用了新常量导致整个包编译失败
- **现象**：`git stash push <impl files>` 后 `go test` 报 build failed，不能证明测试在旧代码上失败。
- **规则**：回归测试尽量只通过旧 API 观察行为（HTTP 请求、已有导出函数）；需要新常量时，用字面量写一份只依赖旧 API 的断言来做 stash 验证。

### 1. （E2E 集成）FE-1 #1 的坑被后来的功能路由原样重犯，React Query 又进了公开页首屏
- **现象**：合并全部前端分支后，落地页入口静态 JS 从约 109 KB 涨到 124 KB gzip（超 120 KB 预算），
  `dist/index.html` 多了 `tanstack-query` 与 `queries` 两个 modulepreload；构建日志里一直有 `INEFFECTIVE_DYNAMIC_IMPORT`。
- **根因**：两处叠加。① shop / invite / orders / orders/$tradeNo / tickets/$id 的 `loader` 直接调用顶层静态导入的
  query 工厂（`userPlansQuery()` 等），loader 留在入口 chunk；② invite、tickets/$id 从 `@/lib/api` 桶文件导入
  `userApi`——TanStack 代码分割器把只给组件用的绑定挪走后，**入口里留下一条裸 `import "@/lib/api"`**，桶文件连带
  `admin.ts`、`queries.ts` 与 React Query 一起被求值。
- **修复**：loader 内 `const q = await import('@/lib/api/queries')`；路由文件直接导入 `@/lib/api/user`；
  vite.config 新增低优先级 `queries` 分组，防止被提升进入口。入口回到 111.8 KB。
- **规则**：路由文件里除 `component` 外的代码只能用入口已有或零依赖模块，**永远不要在路由文件导入桶文件**。
  每次合并后执行 `grep -o 'import"./[^"]*"' dist/assets/index-*.js`（裸导入）并按页面统计首屏 JS。

### 2. （E2E 集成）预渲染后移动端 LCP 没有变化：React 替换节点、Lantern 统计首绘前的请求
- **现象**：公开页加了预渲染快照，移动端 Lighthouse 仍是 91–95；本地观测 FCP 36 ms，但 LCP 56 ms，
  模拟 FCP 恒为 1951 ms。
- **根因**：① Chrome 还没报告快照那一帧的 LCP 候选，React（createRoot）就把节点换掉了，LCP 落到替换后的同尺寸元素上；
  ② Lantern 的悲观图把“观测 FCP 之前**开始**的所有请求”算进 FCP/LCP，Vite 注入的 modulepreload 在 HTML 解析时就全部开始了；
  ③ 落地页 Features 区块是懒加载的，手机首屏里它比 hero 文本大，成为 392 ms 的新 LCP。
- **修复**：快照页只保留 CSS 与一个 `assets/boot-*.js`，收到第一条 LCP（Safari：FCP+50 ms）后才加载应用；
  应用先 `router.load()` 再一次性提交完整页面；Features 改为路由内静态组件。移动端四页 100。
- **规则**：预渲染不等于首屏快：用 `PerformanceObserver('largest-contentful-paint')` 确认 LCP 时间点等于 FCP，
  并看 Lighthouse `network-requests` 里哪些请求开始于观测 FCP 之前。手机首屏可见的内容不得懒加载。
  客户端快照不能 `hydrateRoot`（客户端 `useId` 与水合格式不同，属性不匹配在生产环境静默保留，会让
  `aria-describedby` 指向不存在的 id）。

### 3. （E2E 集成）IP 形式的 `app_url` 让通行密钥接口 500
- **现象**：`app_url=http://127.0.0.1:18090` 时 `POST /user/security/passkeys/begin` 返回 500，`/guest/config` 却报告
  `passkey_enabled: true`，前端显示可点的“添加通行密钥”。
- **根因**：RP ID 直接取 `app_url` 的主机名，go-webauthn 在配置校验阶段拒绝 IP（"must be a domain and not an IP"），
  被包装成 `httpx.Internal`。
- **修复**：IP 形式的 RP ID 视为“未配置”（400 + `passkey_enabled=false`），并加测试；前端按 `passkey_enabled` 禁用入口并本地化提示。
- **规则**：第三方库的配置校验错误属于“功能不可用”，不是 500；派生配置（RP ID、回调域名）要覆盖 IP、localhost、带端口三种输入。

### 4. （E2E 集成）类型声明合并掩盖了“公开 catalog”与“完整 catalog”的形状差异
- **现象**：用户端分流规则里每个来源都显示“覆盖 0 / 32 个分类”，回退来源提示从不出现；单测全绿。
- **根因**：公开 catalog 的分类是 `sources: [...]`（契约 docs/SUBSCRIPTION.md），前端类型却写成完整 catalog 的
  `rules: {source: ref}`；单测读取的是仓库里的完整 `catalog.json`，所以永远测不到真实接口返回的形状。
- **修复**：`categorySources()` 兼容两种形状，类型补上 `sources?`/`clients?`，新增“公开形状”测试。
- **规则**：前端单测的夹具必须来自**接口返回**（或按契约构造），不能直接读后端的内部数据文件；
  同名类型出现两套形状时拆成两个类型名，不要靠声明合并。

### 5. （E2E 集成）对比度只在纯色背景上验证过，soft 色块上的文字不达标
- **现象**：axe/Lighthouse 报 color-contrast：`bg-accent-soft` 上的 `text-label-blue` 4.25:1，`danger-soft`
  按钮 4.09:1（hover 3.69:1），暗色模式同样 3.9:1。
- **根因**：`--label-*` 是按白底与 #F2F2F7 校验的；HeroUI 的 soft 背景是 15%（hover 20%）的着色叠加，没人按叠加后的颜色算。
- **修复**：新增 `--label-{blue,green,orange,red}-soft`（浅色 / 深色各一套，覆盖 20% hover 仍 ≥ 4.6:1），
  HeroUI `*-soft-foreground` 指向它们。
- **规则**：改任何前景色或 soft/tint 背景时，按“背景色 × 透明度叠加到卡片与画布”计算最差对比度；
  `pnpm ui:smoke --axe` 两种主题都要跑。

### 6. （E2E 集成）先 `queryClient.clear()` 再离开页面，退出登录多出一个 401
- **现象**：在个人中心退出登录，控制台出现 `GET /user/me 401`。
- **根因**：`clear()` 时 profile 页的 `useQuery(meQuery())` 仍挂载，缓存被清空后立即无 token 重新请求。
- **修复**：先 `navigate('/login')`（卸载外壳），再 `clear()`。
- **规则**：清缓存、换会话之类的全局操作放在离开受保护区域之后；检查控制台是否出现“登出后的请求”。

### 7. （E2E 集成）lhci 的 `puppeteerScript` 相对路径按 cwd 解析
- **现象**：`puppeteerScript: path.join(__dirname, …)` 报 `Cannot find module '<cwd>/<abs path>'`。
- **根因**：lhci 把该值与 `process.cwd()` 拼接，不识别绝对路径。
- **修复**：写成 `'./scripts/lh-login.cjs'`，从 `web/` 运行。
- **规则**：lhci 配置里的文件路径一律写相对 `web/` 的路径，并实际跑一次对应模式。
