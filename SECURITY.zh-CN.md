# 安全策略

[English](SECURITY.md) · **简体中文**

## 报告漏洞

请**不要**用公开 Issue 报告安全问题。请通过 GitHub 私密漏洞报告提交：
<https://github.com/linusxiong/ggboard/security/advisories/new>（仓库 **Security → Report a vulnerability**）。

请写明：受影响的版本或提交、复现步骤或 PoC、影响范围，以及你建议的修复（如果有）。我们会在私密通告里和你沟通，
修复发布后再公开，并在你同意时致谢。这是业余维护的开源项目，响应尽力而为。

## 支持的版本

| 版本 | 状态 |
| --- | --- |
| 最新的 `1.x` 版本（与 `main`） | ✅ 接收安全修复 |
| 更早的 `1.x` 版本、1.0 之前的提交 | ❌ 请升级到最新的 `1.x` 版本 |

安全修复进 `main`，并随下一个补丁版本（`1.x.y`）发布；不向更早的版本做向后移植。

## 加固清单

部署细节见 [docs/DEPLOY.zh-CN.md](docs/DEPLOY.zh-CN.md)。每一项都对应代码里的实际行为。

### 网络与 TLS

- [ ] **只走 HTTPS**：面板自己不处理 TLS，也不发 HSTS。放在 nginx / Caddy 后面，80 跳 443，并由代理发
      `Strict-Transport-Security`（`deploy/nginx.example.conf` 已包含）。
- [ ] **只监听本机**：compose 只发布 `127.0.0.1:8080`，systemd 单元监听 `127.0.0.1:8080`，不要直接暴露 8080 端口。
- [ ] **`server.trusted_proxies` 只填你的代理**（如 `127.0.0.1,::1`，Docker 填网桥网段）。不填时客户端 IP 全是代理 IP，
      按 IP 的限流会变成全站共享；填 `0.0.0.0/0` 则任何人都能伪造 `X-Forwarded-For` 绕过限流。
- [ ] **设置 `app_url`**（`https://...`）：支付回调、订阅链接、邮件链接和 Passkey 的 RP 都以它为准。不设时面板会根据请求的
      `Host` 和 `X-Forwarded-Proto` 拼接绝对 URL。
- [ ] **`server.cors_origins` 保持为空**（仅同源）。SPA 由二进制自己提供，不需要跨域；不要设置 `*`。

### 密钥

- [ ] **`jwt.secret` 用 32+ 字节随机串**（`openssl rand -hex 32`；占位符、过短或单调的值会拒绝启动），并与数据库一起备份。它既签发会话，也派生 TOTP 密钥的 AES-256-GCM 加密密钥：
      更换后所有会话失效，**已绑定的 TOTP 也全部失效**（备用码与 Passkey 不受影响）。留空时面板会生成
      `<data_dir>/jwt.secret`（0600）。
- [ ] **轮换 `server_token`**：首次启动自动生成 32 位随机值。后台 **设置 → 节点通讯 → 重新生成密钥**，保存后旧值立即失效，
      需要同步更新所有节点的 XrayR / V2bX `ApiKey` 与 soga `webapi_key`。节点接口用常量时间比较，token 为空时拒绝一切请求。
      从 Xboard 迁移时，旧 token 至少 16 个字符才会沿用（更短的不导入，保留 ggboard 生成的 token）；如果旧面板可能已泄露，
      迁移后立即轮换。
- [ ] **由他人运营的节点设置自己的节点密钥**（**节点 → 编辑 → 节点密钥**）。持有共享 `server_token` 的人改一下 `node_id`
      就能拉到所有节点的用户与密钥（REALITY 私钥、SS2022 服务端密钥），Xboard 也是如此。设置了节点密钥的节点只接受这个密钥，
      它也不能用于其他节点。无论用哪种密钥，流量、在线与审计上报都只对本节点权限组内的用户生效。
- [ ] **文件权限**：数据目录以 0700 创建，`jwt.secret` 为 0600；`/etc/ggboard` 用 0750，环境文件只给 root 读。
- [ ] **Telegram 机器人密钥**：`telegram_bot_token` 与 `telegram_webhook_secret` 只有管理员接口返回，日志和 API 错误里不会出现
      （Telegram 客户端会从传输错误里去掉含 token 的请求 URL）。webhook 密钥首次启动自动生成；怀疑泄露时在
      **设置 → Telegram → 重新生成密钥**，保存后再点一次“设置 Webhook”。BotFather token 泄露时先在 BotFather 里 `/revoke`。
- [ ] **加密备份**：数据库里有密码哈希、加密的 TOTP 密钥、加密的 age 私钥、订阅 token、`server_token`、SMTP 口令、验证码密钥、
      Telegram 机器人 token 和支付网关密钥。
- [ ] **不要提交机密**：`.gitignore` 已排除 `.env*`（`.env.example` 除外）、`config.yaml`、`config.local.yaml`、`*.pem`、`*.key`、`data/`、`*.db`。

### 账户

- [ ] **强制管理员两步验证**：**设置 → 安全 → `admin_mfa_required`**。开启后，没有 TOTP 也没有 Passkey 的管理员 / 客服访问
      `/api/v1/admin/*` 会得到 `403 mfa_setup_required`，但仍可在 `/user/security` 完成绑定。开启前先给自己绑定。
- [ ] **Passkey**：RP ID 默认取 `app_url` 的主机名，可用 `passkey_rp_id` 覆盖；其他前端域名加到 `passkey_origins`。
- [ ] **登录与注册防护**（**设置 → 注册与登录**）：`password_limit_enable`（默认开，同一邮箱 60 分钟内错 5 次锁定）、
      `register_limit_by_ip_enable`、`email_verify`、`email_whitelist_*`、`invite_force`，以及 `captcha_provider`
      （Turnstile / reCAPTCHA / hCaptcha）。
- [ ] **生产环境使用 SMTP**：`email_driver=log` 只把邮件写进日志；`log.level=debug` 时连邮件正文（验证码、重置链接）也会写入日志。
      用 SMTP 时**同时开启 `captcha_provider`**：`send-code` 是匿名接口，内置限流只按 IP 计（另有全进程每小时 300 封、突发 60 的
      验证码总预算，超出后接口仍返回 `true` 但不再发信），能控制大量 IP 的人仍可以把总预算用完，让正常用户暂时收不到验证码。
      验证码邮件最多占邮件队列的 75%，订单、工单、泄露提醒等事务邮件始终有余量；超过该水位时 `send-code` 返回 429。
      没有账户、且注册已关闭（`stop_register`）或邮箱后缀不在白名单内时，验证码不会发送（接口照常返回 `true`）。
      邮件被 SMTP 服务器以 5xx 拒收（或收件地址无法解析）时只尝试一次；4xx、超时、连接错误才重试，重试在后台等待，不阻塞其他邮件。
- [ ] **配好 SMTP 后开启 `email_verify`**：关闭时任何人都能用别人的邮箱注册。面板记录 `users.email_verified_at`，未验证账户第一次
      通过“找回密码”证明邮箱所有权时按**所有权变更**处理：新密码之外还会更换 UUID 与订阅 token、删除全部 Passkey 和
      TOTP / 备用码、解除 Telegram 绑定（含未使用的绑定码），并写一条 `account: unverified account reclaimed` 审计日志；
      这类账户的合法用户第一次找回密码时也需要重新导入订阅、重新绑定两步验证。开启 `email_verify` 时，没有正确验证码的
      注册请求对已注册与未注册邮箱给出同样的 400（`email_code`），不会暴露邮箱是否已注册。管理员创建、Xboard 导入以及
      升级前已存在的账户视为已验证。
- [ ] **管理员登录提醒**：配置 Telegram 机器人并保留 `telegram_events` 里的 `admin_login`（默认开启）。每次管理员或客服登录
      （口令、口令 + TOTP / 备用码、Passkey）都会把邮箱、角色、方式、IP、UA 和时间推送到 `telegram_admin_chat_ids`，
      陌生登录能第一时间发现。管理员登录、订阅泄露、提现走独立的优先通道（256 条，单独的 worker），
      用户触发的事件（工单、订单）刷满普通队列时也不会被丢弃或延后。
- [ ] **客服账户**：客服只能处理工单、公告、知识库并只读用户（看不到 UUID 与订阅 token）。知识库占位符
      `{{subscribeUrl}}`、`{{urlEncodeSubscribeUrl}}`、`{{safeBase64SubscribeUrl}}` 会展开成**每位读者自己的**订阅地址（含 token），
      所以只有管理员可以写入：客服新建或修改的正文含这些占位符时返回 `403`（原样提交管理员写好的正文不算修改，客服仍可改标题、
      分类与显示状态）。从旧版本升级后，请管理员检查一遍含这些占位符的文章，特别是带外部图片或链接的。

### 订阅防泄露

- [ ] **泄露检测**：**设置 → 订阅** 里把 `subscribe_leak_max_ips` 设为大于 0（如 10）。同一订阅 token 24 小时内被超过该数量的
      不同 IP（IPv6 按 /64 计）拉取时，用户会收到邮件（绑定了 Telegram 时还有一条消息），管理员收到 Telegram
      `subscription_leak`；`subscribe_leak_action=rotate` 还会立即更换 token（只换订阅链接，节点凭据与会话不变）。访问日志保留 30 天、每个用户最新 100 条。
- [ ] **age 加密订阅**：用户可在订阅页生成 age 密钥（mihomo 1.19.27+、Clash Party、FlClash 可解密）。默认设置偏向兼容：
      `subscribe_age_optional`（默认开）允许 `age=0` 取明文，`subscribe_age_strict`（默认关）时其他格式仍返回明文。
      要让泄露的链接拿不到明文，需要**关闭 `subscribe_age_optional` 并开启 `subscribe_age_strict`**（其他格式改为 406）。
      age 私钥用 `jwt.secret` 派生的密钥加密保存，更换 `jwt.secret` 后用户需重新生成密钥。
- [ ] 提醒用户：链接泄露时优先用“轮换订阅链接”（只换 token）；只有怀疑节点凭据（UUID）也泄露时才“重置订阅”
      （UUID 与 token 一起更换，所有设备都要重新导入）。

### 内置限流

进程内令牌桶，超限返回 `429 rate_limited` 并带 `Retry-After`：

| 范围 | 限额 | 维度 |
| --- | --- | --- |
| `/api/v1/passport/*` | 30 次/分钟 | 客户端 IP |
| `POST /api/v1/passport/email/send-code` | 另加 10 次/小时；IPv6 另加每个 /48 共 60 次/小时 | 客户端 IP（IPv6 按 /64 与 /48） |
| 同上，全部客户端合计 | 300 封/小时，突发 60（超出后返回 `true` 但不发信，WARN 日志每分钟一条） | 全进程 |
| `/api/v1/user/security/*` | 60 次/分钟 | 用户 |
| `POST /api/v1/user/coupons/check`、`POST /api/v1/user/orders`、`POST /api/v1/user/orders/:trade_no/checkout` | 30 次/分钟，突发 10 | 用户 |
| `PUT/POST/PATCH/DELETE /api/v1/user/rule-profile(s)…`、`GET /api/v1/user/subscription/preview` | 60 次/分钟，突发 30 | 用户 |
| `/api/v1/user/subscription/age`（写操作）、`POST /api/v1/user/subscription/rotate-token` | 30 次/分钟，突发 10 | 用户 |
| `POST /api/v1/user/telegram/bind-code` | 10 次/分钟，突发 5 | 用户 |
| `POST /api/v1/user/tickets` | 30 次/小时，突发 10（每次创建都会通知管理员） | 用户 |
| `POST /api/v1/user/tickets/:id/reply` | 120 次/小时，突发 30 | 用户 |
| `POST /api/v1/guest/telegram/webhook/:secret` | 600 次/分钟，突发 120 | 客户端 IP |
| `GET/HEAD /api/v1/client/subscribe`、`/s/:token` | 120 次/小时，突发 30 | 订阅 token |
| 同上，未知或格式错误的 token | 60 次/小时，突发 30 | 客户端 IP（IPv6 按 /64） |
| 节点接口，错误的密钥（包括其他节点的节点密钥） | 30 次，每小时补充 60 次；正确的密钥从不受限 | 客户端 IP（IPv6 按 /64） |
| `ANY /api/v1/guest/payment/notify/…`，验签不通过的回调 | 60 次/分钟，突发 120；用完后在查询支付方式之前直接 429；验签通过的回调从不计数 | 客户端 IP（IPv6 按 /64） |

令牌桶在进程内。只运行一个实例：负载均衡后面的每个实例都会各自放行完整的额度。

### 响应头

- 所有响应：`X-Content-Type-Options: nosniff`、`Referrer-Policy: strict-origin-when-cross-origin`、`X-Frame-Options: DENY`、
  `Cross-Origin-Opener-Policy: same-origin`（跨源页面拿不到面板窗口的句柄，无法数 iframe 推断登录状态；验证码 iframe 不受影响）。
- HTML 文档另加 CSP（`internal/static/static.go`）：

  ```text
  default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self';
  connect-src 'self'; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'
  ```

  `img-src` 放行 `https:`，因为 Gravatar 头像、公告封面（`img_url`）和支付方式的自定义图标都是外部 https 图片，知识库文章
  也可以引用外部图片；正因如此，能展开成读者订阅地址的知识库占位符只允许管理员写入（见上文“客服账户”）。
  不要在代理上放宽它。启用验证码时，`static.CSPFor` 只为**当前启用的那一个**服务追加 script / frame 来源（Turnstile：
  `challenges.cloudflare.com`；reCAPTCHA：`www.recaptcha.net`、`www.google.com`、`www.gstatic.com`；hCaptcha 另加
  connect / style），其他服务一律不放行。

### 支付回调

- 回调地址 `/api/v1/guest/payment/notify/<gateway>/<uuid>`，`uuid` 是每个支付方式独有的随机值。
- 每个网关都会验签：EPay、Epusdt 用 MD5 签名，支付宝当面付用 RSA2（SHA256），Stripe 用 `Stripe-Signature`，
  Coinbase 与 BTCPay 用 HMAC-SHA256（Coinbase 还检查 300 秒的时间窗）。验签失败返回 `400 fail` 并记 WARN 日志，订单不变。
- BTCPay 商店的 POS、Pay Button、众筹等公开入口能让任何人用自选的 `orderId`、金额和币种建发票并付款，回调签名同样有效。
  面板建发票时写入 `ggboardBinding`（以 webhook secret 为密钥，覆盖支付方式 uuid、订单号、金额、币种的 HMAC），
  只认绑定匹配的已结算发票，其余只应答并记 WARN。轮换 webhook secret 前创建、之后才付款的发票需要人工确认。
- 验签通过、但支付方式不是订单最近一次结账所用的（用户换了支付方式后又付了旧页面，或两次结账并发）：按该支付方式自己的
  手续费计算应付额，金额一致即履约；否则只应答、不履约，并提醒管理员。只有结账才能在商户账户下为订单开出支付页，
  所以这样的付款是真实的。
- 网关密钥（webhook secret、商户密钥）只填在后台的支付方式里，回调域名（`notify_domain`）用 HTTPS。

### 日志

- 访问日志以及限流命中（`rate limited`）、5xx（`request failed`）与 panic 日志都只记录**路由模板**（如 `/s/:token`、
  `/api/v1/guest/telegram/webhook/:secret`），不记录查询字符串，所以订阅 token 与 webhook 密钥不会进日志；只有未匹配
  任何路由的 404 记录原始路径。订阅访问日志（`subscription_access`）只存 token 的 8 位哈希标签。
- 前面的反向代理默认记录完整 URL，其中有节点密钥（`/api/v1/server/UniProxy/*?token=`）和订阅 token：让代理只记录不带
  查询串的路径，并把 `/s/` 脱敏（[DEPLOY.zh-CN.md §3](docs/DEPLOY.zh-CN.md#3-反向代理与-tls)、`deploy/nginx.example.conf`）。
- 生产环境用 `log.level=info`；日志会采集到别处时用 `log.format=json`。
