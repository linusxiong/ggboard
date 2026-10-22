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
- **修复**：按源码实现 `status = 0` 规则（返回 409），并写进 `docs/API.md`。
- **规则**：同错题 2026-10-21 #10，转述只能当线索，要以 Service/Controller 的实际代码为准，并在汇报里说明偏差。

### 37. （BE-E2 内容）共享包里的未导出标识符可能与其他 agent 冲突
- **现象（险些发生）**：`internal/api/user`、`internal/api/admin` 由 4–5 个 agent 同时往里加文件。`toView`、`listTickets`、`userView`、测试 helper `newEnv` 这类通用名字合并后会重复声明，编译直接失败。
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
- **根因**：重复了错题 #20 的老路：测试向量只用一种方式得出，没有交叉验证。
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
- **现象 / 根因**：
  - `BeginLogin(user)` 在用户没有凭据时直接返回错误，“未知邮箱也返回 options”必须改走 `BeginDiscoverableLogin`；
  - 计数器回退只会设置 `credential.Authenticator.CloneWarning`，`ValidateLogin` 不会报错；
  - 客户端返回了 RP 未请求的扩展输出时，默认（`UnsolicitedOutputPolicyReject`）会让整个仪式失败，部分密码管理器会主动返回 `credProps`；
  - `allowCredentials` 带有 `omitempty`，可发现登录时这个字段是缺省，不是 `[]`。
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
- **规则**：凡是能直接登录或连接的值（密码哈希、uuid、订阅 token、server_token、网关密钥）都不得出现在警告、错误或日志里；自查时 `grep -n '%q' ` 逐条看格式化的是什么。

### 58. （集成）Dockerfile 的前端阶段漏拷 `.pnpmfile.cjs`，`pnpm install --frozen-lockfile` 报 `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`
- **现象**：本地 `pnpm install` 正常，Docker 构建失败：lockfile 里的 `pnpmfileChecksum` 与当前配置不一致。
- **根因**：前端基础 agent 后来加了 `web/.pnpmfile.cjs`（给 typescript-eslint 单独提供 TypeScript 6），而 Dockerfile 只 `COPY` 了 `package.json`、`pnpm-lock.yaml`、`pnpm-workspace.yaml`。
- **修复**：`COPY` 时一并复制 `.pnpmfile.cjs`（以及将来可能出现的 `.npmrc`）。
- **规则**：凡是影响 pnpm 解析的文件（`.pnpmfile.cjs`、`.npmrc`、`pnpm-workspace.yaml`）都必须进入 Docker 的依赖安装层；改了任何一个就本地跑一次 `docker build`。
