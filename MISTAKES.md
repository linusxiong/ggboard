# 错题本 (MISTAKES.md)

记录本项目开发过程中犯过的错误，避免重复。格式：**日期 / 现象 / 根因 / 修复 / 规则**。
每个 agent 犯错后必须追加；开工前必须先读。

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

### 5. zsh 中 `echo =====` 直接报错并中断整条命令
- **现象**：`cat a; echo =====; cat b` 输出 `(eval):1: ===== not found`，后续命令全部没执行。
- **根因**：zsh 的 `=word` 会做 equals 展开（`=cmd` 会被展开成 `cmd` 的路径），`=====` 被当成要查找的命令名。
- **修复**：分隔符加引号：`echo "-----"`。
- **规则**：在 zsh 里，以 `=` 开头的参数一律加引号，分隔符统一用 `"-----"`。

### 6. 支付契约沿用了过时的第三方 API 认知
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

### 7. 在上游 Xboard 里找 `app/Payments` 扑空
- **现象**：`ls xboard-upstream/app/Payments` 报不存在。
- **根因**：cedar2025/Xboard 已把支付方式改成插件，代码在 `plugins-core/<Name>/Plugin.php`。用户 fork 仍在 `app/Payments/*.php`。
- **规则**：对照上游 Xboard 的支付或通知实现时，先查 `plugins-core/`、`plugins/`，再查 `app/Payments/`。

### 11. 写前端参考文档时没先看已有的 `web/` 脚手架
- **现象**：`docs/FRONTEND.md` 初稿自带了一套 `api.ts`、DataTable、表单 hook 和主题方案。写完才发现 `web/src` 里已经有 `@/lib/api`（`Page<T>={items,total}`、`ApiError.errors`）、`@/lib/guards`、`@/components/data-table`、`@/components/form`、`@/lib/toast`、`styles/theme.css`，两套写法互相冲突，只能整篇重写。
- **根因**：开工时只读了 AGENTS/API 契约，没有 `ls web/src`，也没读已有模块的头部注释。
- **修复**：FRONTEND.md 增加 §0「脚手架积木」表，示例全部改用现有模块。
- **规则**：写指南或示例代码之前，先 `git status` 加 `ls` 看目标目录是否已有实现，并读各文件的头部注释。示例必须调用已有积木，不要另起一套。

### 12. HeroUI 文档的 Anatomy 段落可能落后于示例和发布说明
- **现象**：Switch 页面的 Anatomy 段落仍是 v3.1 结构（`Switch.Control` 与 `Switch.Content` 平级，内嵌 `<Label>`）。同页的示例和 v3.2.0 发布说明则是 `Switch.Content > Switch.Control + 纯文本`。
- **根因**：文档站局部没有随破坏性变更同步更新。
- **修复**：以发布说明、示例和 `npm pack @heroui/react` 解出的 `dist/**/*.d.ts` 三方交叉核对。
- **规则**：HeroUI 原文用 `https://heroui.com/en/docs/react/components/<name>.mdx` 获取（注意是 `.mdx`，`.md` 会 404）。Anatomy 与示例冲突时，以示例、发布说明和 `.d.ts` 为准。

### 13. “最新版本”依赖里沿用了上一代 API 习惯
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

### 14. （Go 基础）macOS 的 BSD `sed` 不支持 `\|`，替换悄悄失败
- **现象**：`sed -i '' 's/a\|b/c/'` 退出码为 0，但一处都没替换（`grep -c` 结果为 0）。
- **根因**：BSD sed 的基本正则不支持 `\|` 交替，模式被当成字面量。
- **修复**：改用 `perl -pi -e 's/(a|b)/c/'` 或 Python 读写文件。
- **规则**：批量替换后必须用 `grep -c` 核对替换次数；交替、非贪婪等写法一律用 perl/python。

### 15. （Go 基础）以为 `glebarez/sqlite` 和 `gorm.io/driver/sqlite` 一样有 `sqlite.New(sqlite.Config{...})`
- **现象**：编译报 `undefined: sqlite.New` / `sqlite.Config`。
- **根因**：glebarez 只提供 `Open(dsn)` 和导出的 `Dialector{DriverName, DSN, Conn}` 结构体。
- **修复**：`&sqlite.Dialector{DriverName: sqlite.DriverName, DSN: dsn, Conn: sqlDB}`。
- **规则**：同名但不同作者的驱动不要套用对方的 API，先 `grep -n "^func \|^type " $(go env GOMODCACHE)/<mod>@<ver>/*.go`。

### 16. （Go 基础）纯 Go SQLite 驱动按 time.Time 自带的时区写文本，导致时间比较出错
- **现象**：`expired_at > ?` 传入本地时间（+08:00）而库里存的是 UTC 字符串时，比较结果错误。SQLite 的 datetime 是按文本比较的。
- **根因**：glebarez/go-sqlite 用 `t.Format("2006-01-02 15:04:05.999999999-07:00")` 写参数，保留了原有时区偏移。
- **修复**：`internal/db/sqlite_utc.go` 包装 driver.Conn，`CheckNamedValue` 把所有时间参数统一转成 UTC。PostgreSQL 用 timestamptz，MySQL DSN 强制 `loc=UTC`，都不受影响。
- **规则**：查询参数一律传 `time.Time`，不要手写时间字符串；`date` 列统一用 `model.Day(t)`。

### 17. （Go 基础）测试里复用了已带主键的 GORM 结构体变量，读到旧数据
- **现象**：`a.DB.First(&got, u2.ID)` 读回的是 u1 的数据。
- **根因**：`got` 里还留着 u1 的主键，GORM 会把它也加进 WHERE，结果查不到记录。错误又没检查，`got` 就保持了旧值。
- **修复**：每次查询都用新变量。
- **规则**：每个 First/Find 都用新声明的变量，并检查 `.Error`。

### 18. （Go 基础）测试 URL 的 query 里带了 `;`，整个参数被丢弃
- **现象**：`/sort?sort=-a,b;drop` 到了 handler 里 `c.Query("sort")` 为空。
- **根因**：Go 1.17 起 `url.ParseQuery` 遇到含 `;` 的键值对会整对丢弃。
- **修复**：写成 `%3B`。
- **规则**：测试 URL 里的特殊字符一律先 URL 编码。

### 19. （Go 基础）`pkill -f "<绝对路径> serve"` 没有匹配到用相对路径启动的进程
- **现象**：以为烟测服务器已经停了，实际还在运行，日志里也看不到 graceful shutdown。
- **根因**：进程的命令行是 `./ggboard serve`，用绝对路径的模式匹配不上。
- **修复**：先 `pgrep -fl` 确认进程，再 `pkill -TERM -f "ggboard serve"`；e2e 脚本改为直接记录 `$!`，用 PID 来 kill。
- **规则**：后台进程一律记录 PID，用 PID 停止，不要靠模式匹配。

### 20. （协调）测试向量在 zsh 里用双引号计算，`$12` 被展开成了空串
- **现象**：简报给的 md5salt 向量（salt `abc$12`）是 `3d6431cb…`，但这其实是 `md5("password123abc")`。按契约规则（hash 取最后一个 `$` 之后的部分），正确值应为 `md5("password123abc$12")` = `1b84a6e599feeab25ace9f402aaa4858`。
- **根因**：zsh 把 `"$12"` 当作第 12 个位置参数展开成了空串（bash 则展开为 `${1}2`）。
- **修复**：`internal/auth/password_test.go` 改用正确向量，`3d6431cb…` 只作为 salt=`abc` 的用例保留。
- **规则**：含 `$` 的字符串一律用单引号（`printf '%s' 'pa$$'`）；测试向量至少用两种工具交叉验证。

### 21. （前端基础）TypeScript 7 没有编译器 API，typescript-eslint 无法工作
- **现象**：`typescript@7` 的包入口只有 `version.cjs` 和 `unstable/*`，而 `typescript-eslint@8.72` 的 peer 是 `typescript <6.1`，`require('typescript')` 拿不到 API。
- **根因**：TS 7 是 Go 原生实现，不再提供旧的 JS 编译器 API。
- **修复**：`web/.pnpmfile.cjs` 的 `readPackage` 钩子给 `typescript-eslint`、`@typescript-eslint/*` 和 `ts-api-utils` 单独注入 `typescript@6.0.3` 依赖；根目录的 `tsc` 仍是 7。效果等同于 docs/FRONTEND.md §1 里的别名方案。
- **规则**：升级到大版本前，先 `npm view <pkg> peerDependencies`，核对整条工具链能否兼容。

### 22. （前端性能）`@import "@heroui/styles"` 让阻塞渲染的 CSS 达到 457 KB（gzip 45 KB）
- **现象**：首个构建只有一个 CSS 文件，里面是全部约 85 个组件的样式，Lighthouse 的 FCP 直接被拖慢。
- **根因**：Tailwind 无法对 `@layer components` 里的组件 CSS 做 tree-shake。
- **修复**：`web/scripts/heroui-css.ts` 把每个组件的样式表（`@reference` 指向应用 CSS）挂到渲染它的 HeroUI JS 模块上。入口只保留 base、theme 和 utilities（约 11 KB gzip）；原生元素用 `import 'heroui-css:button'` 引入单个组件样式。dev 模式下依赖会被预构建，钩子不生效，所以 dev 改为全量引入。
- **规则**：每引入一个 UI 库，先看它对首屏 CSS 和 JS 的影响，再决定导入方式。

### 23. （前端性能）登录页首屏 JS 达到 232 KB gzip，远超 120 KB 预算
- **现象**：React 19（67 KB）+ TanStack Router（26 KB）+ Query（10 KB）已经约 103 KB。再加上 HeroUI/React Aria 的表单组件和 TanStack Form，公开页超出预算一倍。
- **根因**：在预算极紧的公开页上，直接用了重量级组件库和全局 Provider。
- **修复**：公开页（落地页、登录、注册、找回密码）改用原生元素加 HeroUI 的 BEM 类（`NativeField`/`NativeButton`/`buttonClass`），配合 `useLiteForm` 和内联 SVG 图标；React Query 改为由 `_user`/`_admin` 布局的 `beforeLoad` 懒加载，并注入路由 context。现在落地页约 109 KB，登录页约 111 KB。
- **规则**：先算出框架的固定成本，再决定预算剩下的部分怎么分配。每次构建后都按页面统计首屏 chunk 的总和，不要只看单个 chunk。

### 24. （前端基础）有条件地调用 React `use()`，导致 Hook 顺序错乱
- **现象**：`useT` 在字典未加载时调用 `use(promise)`，加载完成后不再调用。React 报 "called use() to suspend … but did not call use() when it finished"，Hook 顺序也随之变化，登录页的冒烟测试因此失败。
- **根因**：从挂起恢复后重新渲染时，React 要求同一个 `use` 调用仍然存在。另外，每次渲染都新建 `Promise.all` 也会触发 "uncached promise" 警告。
- **修复**：始终调用 `use()`。已加载时传入一个预先标记为 `status: 'fulfilled'` 的 thenable；合并后的 Promise 按 (locale, 列表) 永久缓存。
- **规则**：传给 `use()` 的 Promise 必须稳定且已缓存。不要在“已就绪”和“未就绪”两个分支里分别决定调不调 `use`。

### 25. （前端性能）懒加载区块里骨架屏切换成真实内容，CLS 为 0.077
- **现象**：落地页的 Lighthouse 性能分只有 99，CLS 为 0.077，偏移源是套餐卡片 `<li>`。
- **根因**：骨架列表的 key（0,1,2）和真实列表的 key（plan.id 1,2,3）部分重合，React 复用了这些 DOM 节点，并把高度 320 改成了真实高度。
- **修复**：区块组件改为用 `use(guestPlansResource())` 等待数据，chunk 和数据一起就绪后一次性显示，不再有骨架屏切换。外层的 Suspense 占位提供固定的最小高度。
- **规则**：异步内容要么一次性显示，要么占位尺寸与最终尺寸完全一致。骨架屏和真实列表不能共用 key 空间。用 puppeteer 的 `PerformanceObserver('layout-shift')` 取 sources 来定位偏移源。

### 26. （前端基础）jsdom 里的 DOMException 不是 `instanceof Error`
- **现象**：请求超时被识别成了 `network`，而不是 `timeout`。
- **根因**：jsdom 的 DOMException 不继承 Error，`err instanceof Error ? err.name : ''` 拿不到 `TimeoutError`。
- **修复**：改为按结构读取 `name`（`'name' in err`）。
- **规则**：判断 fetch/abort 的错误类型时，按 `name` 判断，不要依赖 `instanceof Error`。

## 2026-10-22

### 1. （FE-1）路由 `loader` 里静态导入 query 工厂，React Query 被打进了所有页面的入口
- **现象**：给 `/dashboard` `/subscription` `/profile` 写了 `loader: () => queryClient.query(subscriptionQuery())` 之后，入口 chunk 从 101.5 KB 涨到 115.2 KB（gzip），登录页首屏超过 120 KB 预算。
- **根因**：TanStack Router 的 `autoCodeSplitting` 默认只拆 `component`/`errorComponent`/`notFoundComponent`/`pendingComponent`，`loader` 和 `validateSearch` 留在 routeTree 里，而 routeTree 属于入口。loader 静态导入的 `@/lib/api/queries` 又带上了 `@tanstack/react-query` 和全部 API 模块。
- **修复**：loader 内 `const q = await import('@/lib/api/queries')` 动态导入；入口回到 103.4 KB，登录页 111.3 KB。
- **规则**：路由文件顶层只放 `component` 会用到的导入；loader 需要的模块一律在 loader 内动态 `import()`。每次加 loader 后按页面统计首屏 chunk（入口 + 路由 chunk 的静态依赖闭包），和改动前对比。

### 2. （FE-1）Vitest 5 的文件系统模块缓存让新增的 i18n JSON 在测试里“不存在”
- **现象**：新增 `src/i18n/zh-CN/dashboard.json` 后，dev server 正常，测试里却一直报 `[i18n] missing namespace zh-CN/dashboard`，`availableNamespaces()` 只返回旧的 5 个命名空间。
- **根因**：`node_modules/.vitest-cache` 按模块内容缓存转换结果，`src/i18n/index.ts` 本身没变，所以 `import.meta.glob('./*/*.json')` 的展开结果一直是旧的。删除 `node_modules/.vite/vitest` 没有用。
- **修复**：`rm -rf node_modules/.vitest-cache` 后重跑。
- **规则**：新增/删除被 `import.meta.glob` 匹配的文件后，测试表现与 dev 不一致时，先清 `node_modules/.vitest-cache`。CI 从干净目录安装，不受影响。

### 3. （FE-1）`useLiteForm` 提交失败后用 rAF 聚焦首个错误字段，抢走了用户正在输入的焦点
- **现象**：登录冒烟测试偶发失败：邮箱框只有 `a`，其余字符 `@example.com…` 被打进了密码框。
- **根因**：首次空提交后 `requestAnimationFrame` 才去 focus 第一个 `[aria-invalid]`；用户（或 user-event）此时已经点进邮箱框开始输入，邮箱的错误被清掉，rAF 回调执行时第一个错误字段变成了密码框。真实用户快速输入时也会遇到。
- **修复**：记录提交时的 `document.activeElement`，rAF 时焦点已经被用户移动就不再抢焦点。
- **规则**：延迟执行的焦点管理必须先确认焦点仍在原处；偶发失败的测试先看 DOM 快照里的值落在哪个字段。

### 4. （FE-1）业务接口的 401 会被 API client 当成会话过期而登出
- **现象**：协调说明里 `/user/security/passkeys/finish` 验证失败可能返回 401；共享 client 对任何带 token 的 401 都会 `logout()` 并跳转登录页。
- **根因**：client 只按状态码判断会话，不知道哪些 401 是业务错误。
- **修复**：`RequestOptions.keepSession`，passkey finish 调用时传 `true`；登录页的 401/403/429 按状态映射成本地化文案（后端 message 是英文）。
- **规则**：新增会返回 401 的业务端点时，前端调用处显式声明 `keepSession`；契约里尽量用 400/409 表达业务失败。

### 5. （FE-1）测试里 `getByRole('alert')` 命中了上一个用例残留的 HeroUI toast
- **现象**：`Found multiple elements with the role "alert"`，其中一个是上一个测试的“登录成功”toast。
- **根因**：HeroUI 的 toast 队列是模块级单例，`cleanup()` 不会清空它。
- **修复**：按文本查找后断言 `role="alert"`：`expect(screen.getByText('…')).toHaveAttribute('role', 'alert')`。
- **规则**：断言表单错误时不要用全局唯一的 `getByRole('alert')`。

### 6. （FE-1）`findByLabelText(/邮箱/)` 同时匹配“邮箱”和“邮箱验证码”，一直重试到测试超时
- **现象**：注册/找回页的测试 5 秒超时，看不到明确报错。
- **根因**：`findBy*` 遇到多个匹配会抛错并持续重试，直到超时，表现成“页面没渲染出来”。
- **修复**：用精确字符串 `findByLabelText('邮箱')`。
- **规则**：label 有包含关系时用精确字符串；`findBy*` 超时先检查是不是匹配到了多个元素。
