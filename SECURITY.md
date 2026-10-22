# 安全策略 / Security Policy

## 报告漏洞 / Reporting a vulnerability

请**不要**用公开 Issue 报告安全问题。请通过 GitHub 私密漏洞报告提交：
<https://github.com/linusxiong/ggboard/security/advisories/new>（仓库 **Security → Report a vulnerability**）。

请写明：受影响的版本或提交、复现步骤或 PoC、影响范围，以及你建议的修复（如果有）。我们会在私密通告里和你沟通，
修复发布后再公开，并在你同意时致谢。这是业余维护的开源项目，响应尽力而为。

*Report privately via GitHub Security Advisories, never in a public issue.*

## 支持的版本 / Supported versions

| 版本 | 状态 |
| --- | --- |
| `v0.1.0-alpha`（`main`） | ✅ 接收安全修复 |
| 更早的提交 | ❌ 请升级到最新 `main` |

项目处于 alpha 阶段：安全修复只进 `main`，不做向后移植。

## 加固清单 / Hardening checklist

部署细节见 [docs/DEPLOY.md](docs/DEPLOY.md)。每一项都对应代码里的实际行为。

### 网络与 TLS

- [ ] **只走 HTTPS**：面板自己不处理 TLS，也不发 HSTS。放在 nginx / Caddy 后面，80 跳 443，并由代理发
      `Strict-Transport-Security`（`deploy/nginx.example.conf` 已包含）。
- [ ] **只监听本机**：compose 只发布 `127.0.0.1:8080`，systemd 单元监听 `127.0.0.1:8080`，不要直接暴露 8080 端口。
- [ ] **`server.trusted_proxies` 只填你的代理**（如 `127.0.0.1,::1`，Docker 填网桥网段）。不填时客户端 IP 全是代理 IP，
      按 IP 的限流会变成全站共享；填 `0.0.0.0/0` 则任何人都能伪造 `X-Forwarded-For` 绕过限流。
- [ ] **设置 `app_url`**（`https://...`）：支付回调、订阅链接、邮件链接和 Passkey 的 RP 都以它为准。不设时面板会根据请求的
      `Host` 和 `X-Forwarded-Proto` 拼接绝对 URL。
- [ ] **`server.cors_origins` 保持为空**（仅同源）。SPA 由二进制自己提供，不需要跨域；不要设置 `*`。

### 密钥 / Secrets

- [ ] **`jwt.secret` 用 32+ 位随机串**，并与数据库一起备份。它既签发会话，也派生 TOTP 密钥的 AES-256-GCM 加密密钥：
      更换后所有会话失效，**已绑定的 TOTP 也全部失效**（备用码与 Passkey 不受影响）。留空时面板会生成
      `<data_dir>/jwt.secret`（0600）。
- [ ] **轮换 `server_token`**：首次启动自动生成 32 位随机值。后台 **设置 → 节点通讯 → 重新生成密钥**，保存后旧值立即失效，
      需要同步更新所有节点的 XrayR / V2bX `ApiKey` 与 soga `webapi_key`。节点接口用常量时间比较，token 为空时拒绝一切请求。
      从 Xboard 迁移时会沿用旧 token；如果旧面板可能已泄露，迁移后立即轮换。
- [ ] **文件权限**：数据目录以 0700 创建，`jwt.secret` 为 0600；`/etc/ggboard` 用 0750，环境文件只给 root 读。
- [ ] **加密备份**：数据库里有密码哈希、加密的 TOTP 密钥、订阅 token、`server_token`、SMTP 口令、验证码密钥和支付网关密钥。
- [ ] **不要提交机密**：`.gitignore` 已排除 `.env*`（`.env.example` 除外）、`config.yaml`、`config.local.yaml`、`*.pem`、`*.key`、`data/`、`*.db`。

### 账户 / Accounts

- [ ] **强制管理员两步验证**：**设置 → 安全 → `admin_mfa_required`**。开启后，没有 TOTP 也没有 Passkey 的管理员 / 客服访问
      `/api/v1/admin/*` 会得到 `403 mfa_setup_required`，但仍可在 `/user/security` 完成绑定。开启前先给自己绑定。
- [ ] **Passkey**：RP ID 默认取 `app_url` 的主机名，可用 `passkey_rp_id` 覆盖；其他前端域名加到 `passkey_origins`。
- [ ] **登录与注册防护**（**设置 → 注册与登录**）：`password_limit_enable`（默认开，同一邮箱 60 分钟内错 5 次锁定）、
      `register_limit_by_ip_enable`、`email_verify`、`email_whitelist_*`、`invite_force`，以及 `captcha_provider`
      （Turnstile / reCAPTCHA / hCaptcha）。
- [ ] **生产环境使用 SMTP**：`email_driver=log` 只把邮件写进日志；`log.level=debug` 时连邮件正文（验证码、重置链接）也会写入日志。

### 内置限流 / Built-in rate limits

进程内令牌桶，超限返回 `429 rate_limited` 并带 `Retry-After`：

| 范围 | 限额 | 维度 |
| --- | --- | --- |
| `/api/v1/passport/*` | 30 次/分钟 | 客户端 IP |
| `POST /api/v1/passport/email/send-code` | 另加 10 次/小时 | 客户端 IP |
| `/api/v1/user/security/*` | 60 次/分钟 | 用户 |
| `POST /api/v1/user/coupons/check`、`POST /api/v1/user/orders`、`POST /api/v1/user/orders/:trade_no/checkout` | 30 次/分钟，突发 10 | 用户 |
| `GET/HEAD /api/v1/client/subscribe`、`/s/:token` | 120 次/小时，突发 30 | 订阅 token |

### 响应头 / Headers

- 所有响应：`X-Content-Type-Options: nosniff`、`Referrer-Policy: strict-origin-when-cross-origin`、`X-Frame-Options: DENY`。
- HTML 文档另加 CSP（`internal/static/static.go`）：

  ```text
  default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self';
  connect-src 'self'; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'
  ```

  不要在代理上放宽它。已知限制：这条 CSP 没有放行验证码服务的脚本和 iframe（`challenges.cloudflare.com`、`www.recaptcha.net`、
  `js.hcaptcha.com`），所以启用验证码前请先确认登录页的组件能正常加载。

### 支付回调 / Payment webhooks

- 回调地址 `/api/v1/guest/payment/notify/<gateway>/<uuid>`，`uuid` 是每个支付方式独有的随机值。
- 每个网关都会验签：EPay、Epusdt 用 MD5 签名，支付宝当面付用 RSA2（SHA256），Stripe 用 `Stripe-Signature`，
  Coinbase 与 BTCPay 用 HMAC-SHA256（Coinbase 还检查 300 秒的时间窗）。验签失败返回 `400 fail` 并记 WARN 日志，订单不变。
- 网关密钥（webhook secret、商户密钥）只填在后台的支付方式里，回调域名（`notify_domain`）用 HTTPS。

### 日志 / Logs

- 访问日志记录**路由模板**（如 `/s/:token`），不记录查询字符串，所以订阅 token 不会进访问日志。
  已知限制（alpha）：限流命中（`rate limited`）、5xx 与 panic 日志目前仍写原始路径，`/s/:token` 被限流时 token 会出现在 INFO 日志里；
  对外共享日志前请先脱敏。
- 生产环境用 `log.level=info`；日志会采集到别处时用 `log.format=json`。
