# 更新日志

[English](CHANGELOG.md) · **简体中文**

这里记录 GG Bond（ggboard）每个版本的重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。推送 `v*` tag 时，GitHub Release 的说明取自 [CHANGELOG.md](CHANGELOG.md)（英文）里对应版本的小节，后面接本文件的同一小节（见 `.github/workflows/release.yml`），所以发版前先在两个文件里都写好该版本。

## [Unreleased]

## [1.1.1] - 2026-11-28

修复升级折抵。没有数据库或设置变更。

### 修复

- 升级折抵（`surplus_enable`）现在也算面板里没有订单的订阅。从 Xboard 导入的用户（订单不导入）或由管理员开通套餐的用户，更换套餐时折抵为 0，新套餐又从升级当天起算，剩余时间就没了。现在这段时间按原套餐每月最便宜的价格估算（一次性套餐：一次性价格乘以未用流量的比例），并扣掉用户自己的折扣。估算的这部分最多把新订单抵到 0 元，不会退到余额。面板里付过的订单照旧按订单折抵。

## [1.1.0] - 2026-11-26

这个版本主要是一次安全审计的修复（见 **安全**）。升级前请注意：开启了 `admin_mfa_required` 的站点，管理员和客服需要用两步验证码或通行密钥重新登录一次。开启两步验证、添加通行密钥现在需要输入当前密码。带请求体的 API 请求必须发送 JSON，其他类型返回 `415`。已保存的链接类设置如果不是 http(s) 网址，会被忽略，需要重新填写。数据库升级（新增 `tickets.kind` 列）在面板启动时自动完成。

### 变更

- HeroUI 升级到 3.2.8。对话框、菜单和提示的出现与消失只做淡入淡出和位移，中端手机也跟得上。手机上的操作菜单不会再缩得比菜单文字还窄，选择列表里长选项旁边留出了勾选标记的位置。对话框和确认框的按钮放不下一行时会换到第二行。
- Stripe SDK 升级到 87.2.0，请求使用 API 版本 `2026-11-18.endive`。与之前同属 `endive` 版本系列，webhook 端点无需改动。
- 开发相关：前端改用 Oxlint 做代码检查（替代 ESLint），CI 把前端的代码检查、类型检查、构建和单元测试拆成并行任务。

### 修复

- 开关、输入框和下拉选择在状态变化时有过渡动画了：滑块滑动，边框和背景渐变，这是 HeroUI 设计的效果。这些过渡用到的主题变量之前没有进入构建出的 CSS，浏览器直接忽略了它们。同一个原因还让搜索框里多出一圈看不见的 3 px 边框，现在没有了。

### 安全

2026-11-25 安全审计。没有发现伪造支付回调或绕过网关验签的途径。

- 向 `/api/v1/passport/login` 这类匿名 JSON 接口发送 `Content-Type: application/x-yaml` 和几百字节的 YAML 别名，就能耗尽面板内存、让进程退出。JSON 接口现在只接受 `application/json`（或 `application/*+json`，或不带 Content-Type），其他类型在读取请求体之前返回 `415 unsupported_media_type`。BSON 请求体原先还会跳过字段校验，现在同样被拒绝。
- 订阅用户自己的重命名规则（`allow_user_rule_profile`，默认开启）能让节点名成倍增长，生成订阅或预览时把面板内存耗尽。现在每一步重命名之后节点名都截断到 1024 字节，每一步的计算量也有上限。
- 开启 `admin_mfa_required` 时，只绑定了通行密钥的管理员或客服仅凭密码就能进入管理后台。会话现在记录登录方式，后台要求本次登录用过两步验证码或通行密钥（否则返回 `403 mfa_reauth_required`，面板会退出该会话并要求这样重新登录）。只有为当前 RP ID 注册的通行密钥才算数。**开启了 `admin_mfa_required` 的站点，升级后管理员和客服需要重新登录一次。**
- 开启两步验证和添加通行密钥现在需要输入当前密码。被盗的会话无法再植入一把改密码后依然有效的通行密钥，也无法绑定自己的 TOTP 把账户主人锁在外面。
- 邮箱从未验证过的账户在找回密码时会被清除第二因素（视为邮箱主人收回账户）。被提升的管理员一直处于未验证状态，因此拿到他们的邮箱就能去掉两步验证。现在管理员和客服账户不会再被收回，提升账户（后台或 `ggboard admin promote`）时会把邮箱标记为已验证。
- 提现申请原先不占用佣金，客服按工单打款的同时，同一笔佣金还能转入余额花掉。现在有未关闭的提现申请时，佣金转入余额会被拒绝（409）。升级前提交的申请和从 Xboard 导入的申请都能识别。处理申请的顺序：先调低用户佣金，再打款，最后关闭工单。
- 管理员、客服以及开启了两步验证的账户用通行密钥登录时，必须完成用户验证（PIN、指纹或面容）；未输入 PIN 的安全密钥会被拒绝。
- 两步验证码输错的次数另外限制为每 24 小时 30 次（每小时 20 次的限制不变）。登录框里的错误密码不会再挡住账户主人在已登录会话里的操作：确认当前密码有单独的按账户计数的限制。
- 节点上报在线时把任意字符串都算作一台设备，一个节点就能让与其他节点共享的用户在所有节点上达到设备上限。现在只统计 IP 地址。
- Surge 和 Surfboard 的 `#!MANAGED-CONFIG` 链接含有订阅 token。TLS 由不在 `server.trusted_proxies` 里的代理终止时，`subscribe_url` 主机上的这个链接可能还是 `http://`。现在对 `subscribe_url` 里 `https://` 条目的主机一律使用 https。
- `//s/<token>/` 和 `//api/...` 原先返回前端页面和 200，现在返回 JSON 404。
- 有容量上限的套餐可能超卖：未支付的订单现在在支付、取消或超时之前占用名额，同时购买最后一个名额也不会再都成功。
- 针对未知订单或金额不符的支付通知被重放时，每次请求都会给管理员发邮件；现在每笔交易只提醒一次。已支付订单再次收到付款时，现在会提醒管理员，不再只记日志。
- 请求进行中被降权的管理员，仍能读取订单列表和订单详情。
- 自定义邮件模板里，写在 HTML 标签内的占位符可能把用户输入的文字（如工单标题）变成标签或属性。
- GeoIP 更新的错误信息不再复述下载或版本服务器返回的内容，并拒绝从 https 跳转到明文 http。
- API 响应默认带 `Cache-Control: no-store`。
- `app_url`、`logo`、`tos_url`、`telegram_discuss_link` 和客户端下载链接必须是 http(s) 网址（没写协议的会补上 `https://`）。已保存的值如果不是（例如相对路径或 `tg://` 链接）会被忽略，需要重新填写；Xboard 导入器会丢弃这样的值并给出提示。
- 节点自己的 token 不能再等于 `server_token`，`server_token` 也不能再设成某个节点自己的 token。三级佣金比例之和不能再超过 100%。客服不能再按 UUID 或订阅 token 筛选用户（这两个字段对客服本来就不可见）。
- Docker 镜像以 0700 权限创建 `/data`（新数据卷会继承；已有数据卷可用 `docker run --rm -v <数据卷>:/data alpine chmod 700 /data` 收紧），`deploy/docker-compose.yml` 去掉全部 capability 并启用 `no-new-privileges`。GitHub Actions 固定到提交 SHA，Dockerfile 的基础镜像固定到 digest。

- Stripe webhook 不再把 SDK 对验签失败的说明写进日志。从 stripe-go 87.2.0 起，这段说明可能写出所配置签名密钥的长度和末 4 位，而任何人都能用构造的 `Stripe-Signature` 请求头触发它。
- MySQL 驱动升级到 1.10.2（[GHSA-9qjq-77h9-895w](https://github.com/go-sql-driver/mysql/security/advisories/GHSA-9qjq-77h9-895w)：握手包被截断时客户端在 TLS 建立前崩溃，DSN 里设置了 `tls=true` 的站点受影响；[GHSA-4v5x-9mc8-gr98](https://github.com/go-sql-driver/mysql/security/advisories/GHSA-4v5x-9mc8-gr98)：`LOAD DATA LOCAL INFILE` 失败后，一个查询的结果行会交给后面的查询，ggboard 从不执行这类语句）。使用 SQLite 或 PostgreSQL 的站点不受影响。
- DOMPurify 升级到 3.5.0（[GHSA-hrxw-jjj6-2g2j](https://github.com/cure53/DOMPurify/security/advisories/GHSA-hrxw-jjj6-2g2j)，问题在面板没有使用的 `SAFE_FOR_TEMPLATES` 选项里）。
- PostgreSQL 驱动 pgx 升级到 5.13.0，修复三条公告。其中两条为高危：simple-protocol 批量查询的 SQL 注入（[GHSA-j38q-h3ph-jjpq](https://github.com/jackc/pgx/security/advisories/GHSA-j38q-h3ph-jjpq)），ggboard 没有用到；以及服务器跳过认证时，连接串里的 `require_auth` / `channel_binding` 不生效（[GHSA-95fj-3fxw-g62g](https://github.com/jackc/pgx/security/advisories/GHSA-95fj-3fxw-g62g)），DSN 里设置了这两项的站点受影响。第三条是服务器返回畸形响应时 panic（[GHSA-w9rp-vfrh-q8vv](https://github.com/jackc/pgx/security/advisories/GHSA-w9rp-vfrh-q8vv)）。使用 SQLite 或 MySQL 的站点不受影响。
- go-mail 升级到 0.8.2（[GHSA-f82v-42xw-72cj](https://github.com/wneessen/go-mail/security/advisories/GHSA-f82v-42xw-72cj)、[GHSA-w8w6-94hg-2qpm](https://github.com/wneessen/go-mail/security/advisories/GHSA-w8w6-94hg-2qpm)）。两处问题分别在 EML 解析器和 List-Unsubscribe 辅助函数里，ggboard 都没有用到。
- DOMPurify 升级到 3.4.18（[GHSA-qm7q-q8h9-jg8q](https://github.com/cure53/DOMPurify/security/advisories/GHSA-qm7q-q8h9-jg8q)，问题在面板没有使用的选项里），marked 升级到 18.2.0，未配对的强调标记不再导致平方级耗时。面板里的 Markdown（如公告和知识库文章）由这两个库渲染。
- `golang.org/x/crypto` 升级到 0.59.0，不再包含已停止维护的 `openpgp` 包（ggboard 从未使用）。
- 仅开发环境：Vite 升级到 8.3.8。此前的 8.3 版本在开发服务器以 `--host` 启动时，一个请求就能在上面执行代码（[GHSA-p4f5-76q4-ffq7](https://github.com/vitejs/vite/security/advisories/GHSA-p4f5-76q4-ffq7)）。发布的二进制和镜像里没有开发服务器。

## [1.0.4] - 2026-11-15

### 新增

- 用户回复工单时，管理员会收到 Telegram 提醒（`ticket_user_reply`，默认开启）。同一工单 30 分钟内最多提醒一次，客服回复后重新计时，所以用户对客服回复的追问不会漏掉。已保存过事件列表的站点，升级时会自动加入一次这个事件。原来的事件 **工单回复** 改名为 **客服回复工单**。

### 变更

- 系统开启 **减弱动态效果** 时，面板只去掉位移、缩放和旋转。加载转圈改为缓慢转动，不再停住（停住的转圈看起来像卡死），提示条按设计淡入淡出，颜色变化保持平滑。此前一条全局规则把所有动画和过渡都截断了。
- 浏览器支持 `Intl.DurationFormat` 时，运行时长等时间段按语言自身的写法显示（`1天2小时3分钟`，不再是 `1天 2小时 3分钟`）。旧浏览器保持原来的格式。
- 标题换行时各行长度接近，最后一行不再只剩一个词或一个字。

### 修复

- 普通订阅链接现在可以在 Quantumult X 中使用，之前会提示 “response invalid”。Quantumult X 默认拿到节点列表，和一键导入一样；加上 `&list=0` 可以拿到完整配置。
- Quantumult X 完整配置的主策略组改名为 `Proxies`，因为 `proxy` 是它的内置策略；只有一个节点的测速组改为 `static`。
- 手动选择浅色或深色后，页面加载时不再先显示另一种配色。首页、登录、注册和重置密码页是预渲染的，应用启动前一直按系统配色显示，约半秒；其他页面的背景也会闪一下。现在每个页面都有一个小脚本在首次绘制前应用所选配色。
- 用输入法输入中文、日文或韩文时，按回车选词不再提前重命名权限组、添加节点过滤规则或提交策略组名称。标签输入框在 Safari 中也已处理。
- 对话框打开时出现的提示条（例如两步验证对话框里的“已复制”，或确认操作失败）现在可以被读屏软件读到，也可以关闭。此前它和对话框后面的页面一起被隐藏。每条提示只播报一次；关闭有焦点的提示条后，焦点移到下一条，或回到原来的位置。
- 备用码文件里的日期改为你所在时区的当天日期。此前用的是 UTC 日期，东八区早上 8 点前会显示成昨天。
- 表单提交失败后，焦点移到第一个出错的字段，而不是它标签旁的 ⓘ。修改密码、两步验证和通行密钥的表单现在也会移动焦点；此前焦点留在按钮上，没有任何提示。
- **发送验证码** 进入倒计时后，键盘焦点留在按钮上，读屏软件会播报验证码已发送。
- 邀请码输入框在手机上不再自动大写首字母，也不再自动纠正。邀请码区分大小写，手动输入时可能因此失败。
- 手机上，系统设置和邮件模板的标签条在还能滚动的一侧边缘渐隐。它们的滚动条是隐藏的，此前看不出可以滚动。
- 服务器拒绝浏览器刚创建的通行密钥时，会通知通行密钥管理器，它不再提供一个无法登录的通行密钥（Chrome 132、Safari 26 及以上）。
- 无障碍细节：读屏软件读必填字段名称时不再带“*”；手机底部标签栏的当前标签除了变蓝还会加粗，打开 **更多** 里的页面时 **更多** 也显示为当前；客服工单对话可以用键盘滚动；标签的删除按钮为 24 px。

### 安全

- 易支付和 Epusdt 的支付回调，只要签名串里任何位置出现 `notify_url=`、`return_url=` 或 `redirect_url=`（包括藏在别的参数值里）就拒绝，下单请求的签名再也不能当作回调通过。此前只检查参数名，所以套餐名或站点名在 `&` 后面直接写出一组支付成功的回调字段时，那笔订单的下单签名可以当作已支付通知通过验签。这些名称只有管理员能设置，普通的 `&`（如 `Pro & Family`）不足以构成攻击。易支付现在把商品名里的 `&` 换成 `＆` 再签名；Epusdt 也和易支付一样，拒绝请求 Host 中含 `&`、`?`、`#` 或用户信息的回调地址。
- 使用 Go 1.27.4 构建，修复 `net/http` 服务端的一个崩溃问题（[CVE-2026-97054](https://pkg.go.dev/vuln/GO-2026-6859)）：用 `MaxBytesReader` 限制请求体大小的 HTTP/1 服务（面板对每个请求都这样限制），处理函数一边读请求体一边写响应头时可能 panic。`go.mod` 现在要求 Go 1.27.4，从源码构建也会用到它。

## [1.0.3] - 2026-11-05

### 新增

- 路由规则：每条规则新增 **下发** 按钮，一次用到所有节点或选中的部分节点，不用再逐个编辑节点（`POST /admin/routes/:id/apply`）。新增一列显示有多少节点在用这条规则。下发结果就是你选中的节点，取消选中即从该节点撤回。节点在下次拉取配置时生效；子节点继续沿用父节点的规则。

### 变更

- 节点标签改为气泡输入：按回车或逗号添加，可一次粘贴多个，也可以点选其他节点已有的标签。节点编辑页里标签和名称都改为独占一行。

### 修复

- 节点编辑页的 **显示** 开关现在和它的 ⓘ、旁边的排序字段对齐；其他与更高字段同排的开关也一并修复。
- 节点列表的子节点行正确显示父节点名称，不再是 `[object Object]`。
- 子节点的编辑页不再显示路由规则。子节点沿用父节点的规则，这里填了本来也不生效；保存子节点时会清掉它自己的规则。
- 节点标签的说明不再写“显示在订阅节点名中”。标签实际不进节点名，而是显示在用户的节点列表里，并用于关键词筛选和地区识别。
- 已登录时打开网站，首屏就显示 **进入控制台**。首页是按未登录状态预渲染的快照，以前 **登录** 按钮要等应用加载完才会换掉，慢网络下要等一秒以上。
- 节点使用 AES 系列 Shadowsocks 2022 时，Quantumult X 不再拿到空订阅。面板之前把这类节点过滤掉了，但这个客户端其实支持这些加密方式，结果一键导入返回空内容，Quantumult X 提示内容无效。
- Xboard 时期的订阅链接可以继续使用：支持 `flag=meta`、`flag=general`、`flag=qx`，`types=all` 不再过滤掉全部节点；`flag=clash` 也不再把 UA 显示为新内核的客户端强制成经典 Clash，这种情况可能一个节点都拿不到。
- `clash-meta/*`、`clash surgio/*` 和 `Quantumultx` 这些 User-Agent 现在能拿到正确的格式。
- Quantumult X 现在也能拿到 AnyTLS Reality 节点，Surfboard 2.28.1 及以上版本能拿到 AES 系列 Shadowsocks 2022 节点。

## [1.0.2] - 2026-11-02

### 新增

- 节点可以设置自己的**节点密钥**（节点 → 编辑 → 节点密钥；后台节点接口的 `token` 字段，新增 `nodes.token` 列，启动时自动添加）。留空则继续使用共享的 `server_token`；设置后本节点只接受自己的密钥，这个密钥也不能用于其他节点，运营某个节点的人无法再改 `node_id` 读取其他节点的用户与密钥。子节点使用父节点的密钥；多个节点可以共用一个密钥（例如一个 soga 对接 `node_id=1,2,3`）；复制节点会生成新密钥。XrayR、V2bX、soga 无需改动：每个节点配置本来就有自己的密钥项。

- 路由规则：整份列表可以直接粘贴进匹配框一次加完。带引号和结尾逗号的写法（`"geosite:bbc",`、`'domain:rfa.org'`）、一行多个带引号的条目、JSON 数组都能识别；离开输入框时自动整理并去重，后台接口也用同样的方式清理 `match`。原始 DNS JSON 路由（首行为 `main`）保持原样。阻断规则用到只有 soga 认的前缀（`geosite:`、`geoip:`、`domain:`、`full:`、`ip:`、`port:`）时，编辑框会提示 XrayR / V2bX 无法从面板接收。

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

[Unreleased]: https://github.com/linusxiong/ggboard/compare/v1.1.1...HEAD
[1.1.1]: https://github.com/linusxiong/ggboard/releases/tag/v1.1.1
[1.1.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.1.0
[1.0.4]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.4
[1.0.3]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.3
[1.0.2]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.2
[1.0.1]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.1
[1.0.0]: https://github.com/linusxiong/ggboard/releases/tag/v1.0.0
