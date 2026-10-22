# 部署 / Deployment

GG Bond 是**一个 Go 二进制**：内嵌 SPA、定时任务、邮件队列、进程内缓存，默认 SQLite，不需要 Redis 或额外的 worker。
生产环境只需要它和一个负责 TLS 的反向代理。

*One static binary (SPA, scheduler, mail queue and cache built in, SQLite by default) behind a TLS reverse proxy.*

文中路径、命令都与仓库里的 `deploy/`、`config.example.yaml`、`internal/config`、`internal/cli` 一一对应。
域名一律用 `panel.example.com` 占位。

- [1. Docker Compose](#1-docker-compose)
- [2. 二进制 + systemd / Binary + systemd](#2-二进制--systemd--binary--systemd)
- [3. 反向代理与 TLS / Reverse proxy](#3-反向代理与-tls--reverse-proxy)
- [4. 配置参考 / Configuration reference](#4-配置参考--configuration-reference)
- [5. 数据库 / Databases](#5-数据库--databases)
- [6. 首次启动后 / After the first start](#6-首次启动后--after-the-first-start)
- [7. 备份 / Backups](#7-备份--backups)
- [8. 升级 / Upgrading](#8-升级--upgrading)
- [9. 命令行 / CLI](#9-命令行--cli)
- [10. 健康检查与日志 / Health and logs](#10-健康检查与日志--health-and-logs)

---

## 1. Docker Compose

`deploy/Dockerfile` 是三段构建：`node:24-alpine`（pnpm 构建前端）→ `golang:1.27-alpine`（`CGO_ENABLED=0` 编译并内嵌 `web/dist`）
→ `gcr.io/distroless/static-debian12:nonroot`（无 shell，以 `nonroot`（UID 65532）运行）。

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env                       # 至少把 GGBOARD_JWT_SECRET 换成 32+ 位随机串
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com
#   不带 --password 时会生成并打印一个随机密码
```

`deploy/docker-compose.yml` 要点：

| 项 | 值 |
| --- | --- |
| 镜像 | 本地构建，`ggboard:${GGBOARD_VERSION:-latest}`；构建参数 `VERSION` 写入 `ggboard version` |
| 端口 | `127.0.0.1:8080:8080`，**只发布到本机**，由宿主机上的 nginx / Caddy 对外 |
| 数据卷 | 命名卷 `ggboard-data` → `/data`（`GGBOARD_DATA_DIR=/data`，镜像内属主 65532） |
| 环境变量 | `env_file: ../.env`（仓库根目录的 `.env`，可缺省）；`environment:` 固定 `GGBOARD_DATA_DIR`、`GGBOARD_DATABASE_DRIVER`（默认 `sqlite`）、`GGBOARD_DATABASE_DSN`（默认 `/data/ggboard.db`） |
| 停止 | `restart: unless-stopped`，`stop_grace_period: 20s`（够服务端 10s 排空 + 刷流量 + 发完邮件队列） |

注意：

- compose 里 `environment:` 的值**优先于** `env_file`。根目录 `.env` 里的 `GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN`
  在 compose 下不生效；要换 PostgreSQL / MySQL，直接改 compose 文件里这两行（见 [§5](#5-数据库--databases)）。
- 镜像里没有 shell，也没有 `curl`/`sqlite3`；运维命令都用 `docker compose exec ggboard ggboard <子命令>`。
- 定时任务按进程时区运行（`scheduler.New(..., time.Local)`）。容器默认 UTC，需要北京时间可在 `environment:` 加
  `TZ: Asia/Shanghai`（二进制已内嵌 `time/tzdata`）。
- 反向代理在宿主机时，容器看到的来源 IP 是 Docker 网桥网关，所以 `GGBOARD_SERVER_TRUSTED_PROXIES` 要填网桥网段
  （`docker network inspect` 查看，例如 `172.16.0.0/12`），见 [§3](#3-反向代理与-tls--reverse-proxy)。

*Compose publishes 8080 on localhost only, keeps state in the `ggboard-data` volume, and its `environment:` block
overrides `.env` for the database variables.*

## 2. 二进制 + systemd / Binary + systemd

```bash
make web && make build                      # 产物 bin/ggboard（静态链接，内嵌前端）
sudo useradd --system --home /var/lib/ggboard --shell /usr/sbin/nologin ggboard
sudo install -m 0755 bin/ggboard /usr/local/bin/ggboard
sudo install -d -m 0750 /etc/ggboard        # 可选：/etc/ggboard/ggboard.env 放 GGBOARD_* 变量
sudo cp deploy/ggboard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ggboard
```

`deploy/ggboard.service`：

- 以 `ggboard` 用户运行；`StateDirectory=ggboard`（`/var/lib/ggboard`，模式 `0700`）同时作为工作目录与
  `GGBOARD_DATA_DIR`；监听 `GGBOARD_SERVER_ADDR=127.0.0.1:8080`。
- `EnvironmentFile=-/etc/ggboard/ggboard.env`（可选）；想用 YAML 就在 `ExecStart` 后加 `--config /etc/ggboard/config.yaml`。
- `Restart=on-failure`，`KillSignal=SIGTERM`，`TimeoutStopSec=30s`，`LimitNOFILE=65535`。
- 加固：`NoNewPrivileges`、`ProtectSystem=strict`、`ProtectHome`、`PrivateTmp`、`PrivateDevices`、`ProtectKernel*`、
  `ProtectControlGroups`、`ProtectClock`、`ProtectHostname`、`RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`、
  `RestrictNamespaces`、`RestrictRealtime`、`RestrictSUIDSGID`、`LockPersonality`、`MemoryDenyWriteExecute`、
  `SystemCallArchitectures=native`、空 `CapabilityBoundingSet`/`AmbientCapabilities`。
  只有 `/var/lib/ggboard` 可写，所以 SQLite 库与 `jwt.secret` 必须留在数据目录。

运维命令要用**同一用户、同一环境**运行，否则会以 root 身份新建/改写数据库文件：

```bash
sudo systemd-run --uid=ggboard --gid=ggboard --pty --wait --collect \
  -p WorkingDirectory=/var/lib/ggboard -p EnvironmentFile=-/etc/ggboard/ggboard.env \
  -E GGBOARD_DATA_DIR=/var/lib/ggboard \
  /usr/local/bin/ggboard admin create --email admin@example.com
```

`.env` 只被 compose 读取；二进制本身只读环境变量与 YAML。手动运行时可 `set -a; . ./.env; set +a; ./bin/ggboard serve`。

*The unit runs as `ggboard` with a strict sandbox; only `/var/lib/ggboard` is writable. Run CLI commands as the
same user with the same environment.*

## 3. 反向代理与 TLS / Reverse proxy

面板自己不终止 TLS，也不发 HSTS。代理需要：

1. 终止 HTTPS，80 → 443 跳转，发 `Strict-Transport-Security`。
2. 传 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto`。`X-Forwarded-Proto: https` 用于生成支付回调、Passkey、订阅自引用等绝对 URL
   （设置了 `app_url` 时以它为准）。
3. 允许 **8 MiB** 请求体：普通接口上限 1 MiB，节点上报接口（`/api/v1/server/UniProxy/{push,alive,status}`、soga 的 POST）为 8 MiB。
4. 把代理地址写进 `server.trusted_proxies`。**不配置时所有请求的客户端 IP 都是代理 IP**：登录/注册限流
   （`/api/v1/passport/*` 每 IP 30 次/分钟，发验证码每 IP 10 次/小时）、注册 IP 限制和日志里的 IP 都会变成"全站共用一个 IP"。

面板不使用 WebSocket，无需 `Upgrade` 头；响应已由面板自己 gzip/br 压缩，代理不必再压。

### nginx

完整示例见 [`deploy/nginx.example.conf`](../deploy/nginx.example.conf)（HTTP/2、TLS 1.2/1.3、HSTS、`client_max_body_size 8m`、
`proxy_read_timeout 120s`、upstream keepalive）。配合：

```bash
GGBOARD_SERVER_TRUSTED_PROXIES=127.0.0.1,::1
```

### Caddy

Caddy 自动申请证书、自动 80→443，并默认设置 `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host`：

```caddyfile
panel.example.com {
    header Strict-Transport-Security "max-age=63072000; includeSubDomains"
    request_body {
        max_size 8MB
    }
    reverse_proxy 127.0.0.1:8080
}
```

同样设置 `GGBOARD_SERVER_TRUSTED_PROXIES=127.0.0.1,::1`。前面再套 CDN（如 Cloudflare）时，要让 Caddy / nginx 信任 CDN 的回源网段，
否则拿到的是 CDN 节点 IP。

*Terminate TLS in nginx or Caddy, forward `X-Forwarded-*`, allow 8 MiB bodies, and list the proxy in
`server.trusted_proxies` or every client shares the proxy's IP for rate limiting.*

## 4. 配置参考 / Configuration reference

来源优先级：**命令行参数 > 环境变量 > 配置文件 > 默认值**。配置文件：`--config/-c`，否则 `$GGBOARD_CONFIG`，否则当前目录存在的
`config.yaml`（已 gitignore，模板 [`config.example.yaml`](../config.example.yaml)）。环境变量 = `GGBOARD_` + 键名大写、`.` 换成 `_`；
列表类变量用逗号分隔。站点名、邮件、支付、节点通讯密钥等**运行时设置在数据库里**，在后台“设置”页修改，不在这里。

| 键 | 环境变量 | 参数 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `server.addr` | `GGBOARD_SERVER_ADDR` | `serve --addr` | `:8080` | 监听地址 |
| `server.trusted_proxies` | `GGBOARD_SERVER_TRUSTED_PROXIES` | | `[]` | 信任其 `X-Forwarded-For`/`X-Real-IP` 的代理 IP/CIDR；空 = 不信任；非法值启动报错 |
| `server.cors_origins` | `GGBOARD_SERVER_CORS_ORIGINS` | | `[]` | 额外允许的跨域来源，`*` = 任意；空 = 仅同源（推荐） |
| `database.driver` | `GGBOARD_DATABASE_DRIVER` | `--db-driver` | `sqlite` | `sqlite` / `postgres` / `mysql` |
| `database.dsn` | `GGBOARD_DATABASE_DSN` | `--db-dsn` | SQLite：`<data_dir>/ggboard.db` | PostgreSQL / MySQL 必填 |
| `jwt.secret` | `GGBOARD_JWT_SECRET` | | 空 | 会话签名密钥，建议 32+ 字符（更短会告警）。为空时生成 48 字节随机值写入 `<data_dir>/jwt.secret`（0600）并沿用 |
| `jwt.ttl` | `GGBOARD_JWT_TTL` | | `168h` | 会话有效期（Go duration，必须 > 0） |
| `log.level` | `GGBOARD_LOG_LEVEL` | `--log-level` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `GGBOARD_LOG_FORMAT` | `--log-format` | `text` | `text` / `json` |
| `frontend.dir` | `GGBOARD_FRONTEND_DIR` | | 空 | 从该目录提供 SPA，代替内嵌产物（目录必须存在） |
| `data_dir` | `GGBOARD_DATA_DIR` | `--data-dir` | `./data` | SQLite 库与 `jwt.secret` 所在目录，启动时以 0700 创建 |

> **`jwt.secret` 不只是会话密钥**：TOTP 两步验证的密钥以它派生的 AES-256-GCM 密钥加密保存
> （`internal/service/mfa/crypto.go`）。更换或丢失它会让所有人退出登录，并让**已绑定的 TOTP 全部失效**（备用码仍可用）。
> 务必与数据库一起备份。

*Flags > env > file > defaults. `jwt.secret` also encrypts TOTP secrets: changing it signs everyone out and breaks
enrolled authenticators.*

## 5. 数据库 / Databases

启动时（以及每个需要 App 的 CLI 命令前）都会对 `model.All()` 执行 GORM AutoMigrate，幂等，无需单独迁移步骤。

**SQLite（默认）**：纯 Go 驱动，`database.dsn` 是文件路径或 `file:` URI。未自带 `_pragma` 时自动追加
`busy_timeout(5000)`、`foreign_keys(1)`、`journal_mode(WAL)`、`synchronous(NORMAL)`，并设 `_txlock=immediate`。
连接池 8。数据目录里会出现 `ggboard.db`、`ggboard.db-wal`、`ggboard.db-shm`。

**PostgreSQL**：DSN 原样交给 `gorm.io/driver/postgres`，URL 或 `key=value` 均可：

```yaml
database:
  driver: postgres
  dsn: postgres://ggboard:password@db.example.com:5432/ggboard?sslmode=require
```

**MySQL / MariaDB**：必须是 go-sql-driver 格式（不是 `mysql://` URL，那是 `import xboard --dsn` 的格式）。
面板强制 `parseTime=true`，未指定时补 `loc=UTC`、`charset=utf8mb4`；建表使用 `ENGINE=InnoDB utf8mb4_unicode_ci`：

```yaml
database:
  driver: mysql
  dsn: ggboard:password@tcp(db.example.com:3306)/ggboard
```

PostgreSQL / MySQL 连接池：最多 32 个连接、8 个空闲、单连接最长 30 分钟。Compose 下改 `deploy/docker-compose.yml` 中的
`GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN` 两行即可。

## 6. 首次启动后 / After the first start

1. `ggboard admin create --email ...` 创建管理员（`--staff` 创建客服）。
2. 后台 **设置 → 站点**：填写 `app_url`（如 `https://panel.example.com`）。支付回调地址、订阅链接、邮件里的链接、
   Passkey 的 RP ID / origin 都以它为准。不填 `app_url`（也没设 `passkey_rp_id`）时 Passkey 不可用，
   其他链接只能退回请求的 Host 或相对路径。
3. **设置 → 节点通讯**：首次启动已自动生成 32 位 `server_token`，填进 XrayR / V2bX / soga（见 [NODE_PROTOCOLS.md](NODE_PROTOCOLS.md)）。
4. **设置 → 邮件**：默认驱动 `log`（只写日志）；切到 `smtp` 后用“发送测试邮件”验证。
5. **设置 → 安全**：建议开启 `admin_mfa_required`，见 [SECURITY.md](../SECURITY.md)。
6. **设置 → Telegram**（可选）：填 BotFather 的 Bot Token 和管理员 Chat ID（在机器人对话里发 `/id` 获取），保存后点
   “设置 Webhook”与“发送测试消息”。Telegram 只接受 HTTPS 的 webhook（端口 443、80、88 或 8443），所以 `app_url` 必须是
   公网可达的 `https://` 地址；反向代理要放行 `POST /api/v1/guest/telegram/webhook/<密钥>`（不要对它加认证或缓存）。
   详见 [ADMIN_GUIDE.md](ADMIN_GUIDE.md) 的 Telegram 一节。
7. **设置 → 订阅**（可选）：`subscribe_leak_max_ips` 大于 0 时开启订阅泄露检测，见 [SECURITY.md](../SECURITY.md)。

## 7. 备份 / Backups

需要备份的只有数据库和 `jwt.secret`（若由环境变量提供，则备份你的 `.env` / `ggboard.env`）。

**SQLite（systemd）**：WAL 模式下可在线热备，不要直接 `cp` 正在写的 `.db`：

```bash
sqlite3 /var/lib/ggboard/ggboard.db ".backup '/srv/backup/ggboard-$(date +%F).db'"
# 或：sqlite3 /var/lib/ggboard/ggboard.db "VACUUM INTO '/srv/backup/ggboard-$(date +%F).db'"
cp /var/lib/ggboard/jwt.secret /srv/backup/ 2>/dev/null || true
```

**SQLite（Docker）**：镜像里没有 `sqlite3`，先停容器再打包数据卷（卷名形如 `deploy_ggboard-data`，`docker volume ls` 查看）：

```bash
docker compose -f deploy/docker-compose.yml stop
docker run --rm -v deploy_ggboard-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/ggboard-data-$(date +%F).tgz -C /data .
docker compose -f deploy/docker-compose.yml start
```

**PostgreSQL / MySQL**：用 `pg_dump` / `mysqldump`。

恢复：停服务 → 放回 `ggboard.db`（删掉旧的 `-wal`/`-shm`）与 `jwt.secret` → 启动。流量按批缓冲在内存、在优雅退出时落库，
所以停服务请用 `SIGTERM`（`systemctl stop` / `docker compose stop`），不要 `kill -9`。

## 8. 升级 / Upgrading

1. 备份（见上）。
2. Docker：`git pull && docker compose -f deploy/docker-compose.yml up -d --build`。
   二进制：`git pull && make web && make build`，替换 `/usr/local/bin/ggboard`，`systemctl restart ggboard`。
3. 新版本启动时自动迁移数据库；`ggboard migrate` 可在不启动服务的情况下单独执行迁移（打印 `database migrated (<driver>)`）。
4. `ggboard version` / `GET /healthz` 确认版本。

迁移只增不删（AutoMigrate 不删除列）；回滚到旧二进制前请先恢复备份。

## 9. 命令行 / CLI

全局参数：`-c/--config`、`--data-dir`、`--log-level`、`--log-format`、`--db-driver`、`--db-dsn`。
除 `version`、`help`、`completion` 外，每个命令执行前都会加载配置并迁移数据库。

| 命令 | 作用 |
| --- | --- |
| `ggboard serve [--addr :8080]` | 运行 HTTP 服务、定时任务与邮件队列；`SIGINT`/`SIGTERM` 后 10 秒排空 |
| `ggboard migrate` | 创建/更新表结构 |
| `ggboard admin create --email E [--password P] [--staff]` | 创建管理员（或客服）；不给密码则生成并打印 |
| `ggboard admin reset-password --email E [--password P]` | 重置密码并让该用户已有会话失效 |
| `ggboard admin promote --email E` | 把已有用户提升为管理员 |
| `ggboard import xboard --dsn DSN [--driver] [--dry-run] [--force] [--with-stats] [--stats-days N] [--source-tz TZ]` | 从 Xboard 导入，见 [IMPORT_XBOARD.md](IMPORT_XBOARD.md) |
| `ggboard version` | 打印版本、Go 版本与平台 |

## 10. 健康检查与日志 / Health and logs

- `GET /healthz` → `200 {"data":{"status":"ok","version":"…","uptime":"…"}}`，不需要鉴权，适合做存活探针。
  （distroless 镜像里没有可执行 HTTP 探测的工具，Dockerfile 未定义 `HEALTHCHECK`，请在外部监控里探测。）
- 日志写到 stderr（`journalctl -u ggboard` / `docker compose logs`），`log.format: json` 便于采集。
  访问日志记录**路由模板**（如 `/s/:token`）而不是实际路径，避免订阅 token 进日志。
- 后台 **系统状态** 页（`/admin/system`）可以看到最近 1000 条日志（内存环形缓冲，重启后清空）、任务与邮件队列状态。
