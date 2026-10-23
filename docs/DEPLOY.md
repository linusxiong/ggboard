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

同一个镜像有两种拿法，选一种：

| | 方式 A：预构建镜像（推荐） | 方式 B：从源码构建 |
| --- | --- | --- |
| Compose 文件 | `deploy/docker-compose.prebuilt.yml` | `deploy/docker-compose.yml` |
| 服务器上需要 | Docker（Compose ≥ 2.24），不用克隆仓库 | Docker 和整个仓库；首次构建要装 Chromium、pnpm 和 Go 依赖，需要几分钟 |
| 版本由什么决定 | `.env` 里的 `GGBOARD_VERSION`（镜像 tag） | 当前检出的代码 |
| 升级 | 改版本号，`pull` + `up -d` | `git pull` + `up -d --build` |

### 方式 A：预构建镜像 / Prebuilt image

镜像由手动触发的流水线构建并发布到 GHCR，怎么构建见[本节末尾](#构建并发布镜像--publishing-the-image)。

**1. 取文件。** 新建一个空目录，放进 compose 文件和 `.env`；`.env` 必须和 compose 文件在同一个目录：

```bash
mkdir ggboard && cd ggboard
curl -fsSLO https://raw.githubusercontent.com/linusxiong/ggboard/main/deploy/docker-compose.prebuilt.yml
curl -fsSL https://raw.githubusercontent.com/linusxiong/ggboard/main/.env.example -o .env
```

**2. 选版本。** 把镜像 tag 写进 `.env`：

```bash
echo 'GGBOARD_VERSION=main' >> .env
```

| `GGBOARD_VERSION` | 含义 |
| --- | --- |
| `0.1.0` | 一个具体的发布版本，生产环境用这种 |
| `0.1` | 0.1.x 里最新的一版（`1` 同理；主版本为 0 时没有） |
| `latest` | 最新的稳定发布版；在构建出不带 `-rc` 之类后缀的 `v*` tag 之前不存在 |
| `main` | `main` 分支最近一次手动构建，滚动更新，适合先试用 |
| `sha-<7 位>` | 某个提交的构建 |

镜像包还是私有时，先 `docker login ghcr.io`：用户名是 GitHub 账号，密码是有 `read:packages` 权限的 token。
`.env` 里的其余项见 [§4](#4-配置参考--configuration-reference)；`GGBOARD_JWT_SECRET` 留空会自动生成并存在数据卷里。

**3. 启动并创建管理员。**

```bash
docker compose -f docker-compose.prebuilt.yml up -d
docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
#   不带 --password 时会生成并打印一个随机密码，只显示一次
```

确认它在跑：

```bash
docker compose -f docker-compose.prebuilt.yml ps
docker compose -f docker-compose.prebuilt.yml logs --tail 30 ggboard    # 启动摘要里有版本、数据库和数据目录
curl -s http://127.0.0.1:8080/healthz                                   # {"data":{"status":"ok",…}}
```

**4. 放到反向代理后面。** 容器只发布到 `127.0.0.1:8080`，对外要靠宿主机上的 nginx / Caddy 和 HTTPS，配法见 [§3](#3-反向代理与-tls--reverse-proxy)。
宿主机上的代理对容器来说来自 Docker 网桥网关，所以在 `.env` 里加 `GGBOARD_SERVER_TRUSTED_PROXIES=<网桥网段>`
（`docker network inspect ggboard_default` 查看，例如 `172.16.0.0/12`），改完 `up -d` 让容器重建。
然后打开 `https://panel.example.com/login`，登录后的设置顺序见 [§6](#6-首次启动后--after-the-first-start)。

**5. 升级与回滚。** 先备份（[§7](#7-备份--backups)），再改 `.env` 里的 `GGBOARD_VERSION`：

```bash
docker compose -f docker-compose.prebuilt.yml pull
docker compose -f docker-compose.prebuilt.yml up -d
docker image prune -f                      # 可选：清理旧镜像
```

新版本启动时自动迁移数据库。回滚是把版本号改回去再 `up -d`，但回滚到旧版本之前要先恢复备份（见 [§8](#8-升级--upgrading)）。

`deploy/docker-compose.prebuilt.yml` 要点：

| 项 | 值 |
| --- | --- |
| 镜像 | `${GGBOARD_IMAGE:-ghcr.io/linusxiong/ggboard}:${GGBOARD_VERSION:-latest}`；`GGBOARD_IMAGE` 给 fork 和镜像站用 |
| 项目名 / 数据卷 | 固定为 `ggboard`；数据卷 `ggboard_ggboard-data` → `/data`（`GGBOARD_DATA_DIR=/data`，镜像内属主 65532） |
| 端口 | `127.0.0.1:8080:8080`，**只发布到本机** |
| 环境变量 | `env_file: .env`（compose 文件旁边的 `.env`，可缺省）；`environment:` 只固定 `GGBOARD_DATA_DIR`。Compose 也用这个 `.env` 解析 `${GGBOARD_VERSION}` |
| 加固 | `cap_drop: [ALL]` 和 `no-new-privileges`；镜像是 distroless，以 `nonroot`（UID 65532）运行 |
| 停止 | `restart: unless-stopped`，`stop_grace_period: 20s` |

从方式 B 切过来并沿用原来的数据：方式 B 的数据卷是 `deploy_ggboard-data`，方式 A 默认的是 `ggboard_ggboard-data`。
先在仓库里 `docker compose -f deploy/docker-compose.yml down`（**不要加 `-v`**），把仓库根目录的 `.env` 复制到方式 A 的目录，
之后所有命令都加 `-p deploy`（命令行的 `-p` 优先于文件里的 `name:`），例如
`docker compose -p deploy -f docker-compose.prebuilt.yml up -d`。

*Option A needs only `deploy/docker-compose.prebuilt.yml` and a `.env` beside it: set `GGBOARD_VERSION` (a release such as `0.1.0`,
or `main`), `up -d`, create the admin, put a TLS proxy in front. Upgrade by changing the version, then `pull` and `up -d`.*

### 方式 B：从源码构建 / Build from source

`deploy/Dockerfile` 是三段构建：`node:24-alpine`（pnpm 构建前端）→ `golang:1.27-alpine`（`CGO_ENABLED=0` 编译并内嵌 `web/dist`）
→ `gcr.io/distroless/static-debian12:nonroot`（无 shell，以 `nonroot`（UID 65532）运行）。

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env                       # GGBOARD_JWT_SECRET 留空即自动生成；自填须为 openssl rand -hex 32 这类随机值
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
| 环境变量 | `env_file: ../.env`（仓库根目录的 `.env`，可缺省）；`environment:` 只固定 `GGBOARD_DATA_DIR`。数据库、JWT 等都由 `.env` 决定，SQLite 的 DSN 缺省为 `/data/ggboard.db` |
| 停止 | `restart: unless-stopped`，`stop_grace_period: 20s`（够服务端 10s 排空 + 刷流量 + 发完邮件队列） |

注意：

- compose 里 `environment:` 的值**优先于** `env_file`，所以那里只放 `GGBOARD_DATA_DIR`；要换 PostgreSQL / MySQL，
  在 `.env` 里写 `GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN` 即可（见 [§5](#5-数据库--databases)）。
  SQLite 的 DSN 不要写成相对路径：容器的工作目录是 `/data`，`./data/ggboard.db` 会指到 `/data/data/ggboard.db` 而打不开，留空用默认值。
- 镜像里没有 shell，也没有 `curl`/`sqlite3`；运维命令都用 `docker compose exec ggboard ggboard <子命令>`。
- 定时任务按进程时区运行（`scheduler.New(..., time.Local)`）。容器默认 UTC，需要北京时间可在 `environment:` 加
  `TZ: Asia/Shanghai`（二进制已内嵌 `time/tzdata`）。
- 反向代理在宿主机时，容器看到的来源 IP 是 Docker 网桥网关，所以 `GGBOARD_SERVER_TRUSTED_PROXIES` 要填网桥网段
  （`docker network inspect` 查看，例如 `172.16.0.0/12`），见 [§3](#3-反向代理与-tls--reverse-proxy)。

*Compose publishes 8080 on localhost only, keeps state in the `ggboard-data` volume, and its `environment:` block
overrides `.env`, so only `GGBOARD_DATA_DIR` is set there and the database settings live in `.env`.*

### 构建并发布镜像 / Publishing the image

给维护者：镜像由手动触发的流水线 `.github/workflows/docker.yml` 构建，并以多架构镜像推送到 `ghcr.io/linusxiong/ggboard`。

1. 把工作流文件合并到默认分支，Actions 页面才会出现 **Docker** 的 Run workflow 按钮。
2. Actions → **Docker** → Run workflow，“Use workflow from” 选 `main`，其余保持默认：得到 `main` 和 `sha-<7 位>` 两个 tag。
3. 发版：打 tag 并推送（`git tag v0.1.0 && git push origin v0.1.0`），再对这个 tag 跑一次 Run workflow（推 tag 本身不会触发构建）。
4. 首次推送后包可能是私有的：GitHub → Packages → ggboard → Package settings，把可见性改成 Public，服务器上才能匿名 `docker pull`。

| 选择的 ref | 镜像 tag |
| --- | --- |
| tag `v1.2.3` | `1.2.3`、`1.2`、`1`、`latest`、`sha-<7 位>`（主版本为 0 时没有 `0`） |
| 预发布 tag `v1.2.3-rc.1` | `1.2.3-rc.1`、`sha-<7 位>`（不动 `latest`） |
| 其他 tag | `<tag>`、`sha-<7 位>` |
| 分支（如 `main`） | `<分支名>`（`/` 换成 `-`）、`sha-<7 位>` |

输入项：`push`（取消勾选 = 只构建不推送，用来验证 Dockerfile 并预热缓存）、`platforms`（默认 `linux/amd64,linux/arm64`）、
`no_cache`（忽略缓存并重新拉取基础镜像，新层仍会写回缓存）。

- **元数据**：镜像带 OCI 标签，manifest 和 index 上也带同样的注解（GHCR 页面读的是 index 注解）：`source`、`revision`、
  `version`、`created`、`licenses` 等。`version` 取 `git describe --tags --always`，与 `ggboard version` 输出一致。
  构建完会回读 registry，核对平台、注解、标签、每个 tag 的 digest 和二进制报告的版本，任一不符就让任务失败；同时附带 SBOM 与 provenance。
- **缓存**：BuildKit 的 GitHub Actions 缓存（`mode=max`，把 pnpm 安装、Chromium、`go mod download` 这些中间阶段也存下来）。
  每次运行能读自己所在 ref 和默认分支的缓存，所以发版前先在 `main` 上构建一次，tag 构建就能命中；缓存仓库上限 10 GB，7 天未用会被淘汰。
- **多架构不用 QEMU**：前端和 Go 编译阶段都在 runner 本机架构上运行（`--platform=$BUILDPLATFORM`），Go 交叉编译到目标架构。

服务器上怎么用这个镜像，见[方式 A](#方式-a预构建镜像--prebuilt-image)。

*A manual workflow builds and pushes a multi-arch image to GHCR (tags: semver → `1.2.3`/`1.2`/`1`/`latest`, branch → its name, plus
`sha-<short>`), with OCI labels and annotations, SBOM and provenance, a GitHub Actions layer cache, and a post-push check of what the registry holds.*

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
| `jwt.secret` | `GGBOARD_JWT_SECRET` | | 空 | 会话签名密钥。自填时须至少 32 字节、至少 8 种不同字符（`openssl rand -hex 32`）；已知占位符（如旧模板的 `change-me-…`）、过短或单调的值会让启动直接报错。为空时生成 48 字节随机值写入 `<data_dir>/jwt.secret`（0600）并沿用（该文件内容同样校验） |
| `jwt.ttl` | `GGBOARD_JWT_TTL` | | `168h` | 会话有效期（Go duration，必须 > 0） |
| `auth.argon2_concurrency` | `GGBOARD_AUTH_ARGON2_CONCURRENCY` | | `0`（自动） | 同时进行的 argon2id 密码校验 / 哈希个数（登录、注册、改密、找回）。每个运行时占 64 MiB，超出的请求**排队等空位**而不是再占内存，所以这部分峰值约为“该值 × 64 MiB”。`0` = `min(Go 可用的 CPU 核数, 4)`（`GOMAXPROCS`，容器里跟随 CPU 限额），即最多 256 MiB。小内存机器可设 1–2：登录高峰会排队、变慢，但不会因此吃满内存；调高很少能提高吞吐（每次校验本身已用 4 个线程）。负数启动报错 |
| `log.level` | `GGBOARD_LOG_LEVEL` | `--log-level` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `GGBOARD_LOG_FORMAT` | `--log-format` | `text` | `text` / `json` |
| `log.color` | `GGBOARD_LOG_COLOR` | `--color` | `auto` | 终端颜色与排版：`auto`（输出是终端且未设 `NO_COLOR` 时着色）/ `always`（管道、文件也着色并用终端排版，优先于 `NO_COLOR`）/ `never`（不着色，终端仍对齐排版）。不影响 JSON 日志，见第 9、10 节 |
| `frontend.dir` | `GGBOARD_FRONTEND_DIR` | | 空 | 从该目录提供 SPA，代替内嵌产物（目录必须存在） |
| `data_dir` | `GGBOARD_DATA_DIR` | `--data-dir` | `./data` | SQLite 库与 `jwt.secret` 所在目录，启动时以 0700 创建 |

> **`jwt.secret` 不只是会话密钥**：TOTP 两步验证的密钥以它派生的 AES-256-GCM 密钥加密保存
> （`internal/service/mfa/crypto.go`）。更换或丢失它会让所有人退出登录，并让**已绑定的 TOTP 全部失效**（备用码仍可用）。
> 务必与数据库一起备份。

*Flags > env > file > defaults. `jwt.secret` also encrypts TOTP secrets: changing it signs everyone out and breaks
enrolled authenticators.*

### 内存 / Memory

登录高峰最占内存：argon2id 每次校验要 64 MiB。两个旋钮配合使用：

1. **`auth.argon2_concurrency`**（见上表）限制**同时**有几个校验在跑，即高峰期这部分内存的上限。默认最多 4 个（256 MiB）；
   1 GiB 内存的 VPS 建议设 `2`（128 MiB），512 MiB 的设 `1`。代价是高峰时多出来的登录排队：排在第 N 位的请求大约多等
   N / 并发数 次校验的时间（一次校验多久取决于 CPU；开发机上的实测见 [`docs/perf/README.md`](perf/README.md)）。
2. **`GOMEMLIMIT`**（Go 运行时的环境变量，不是 ggboard 的配置键，也不能写进 YAML）是进程内存的**软上限**：接近它时 Go 会更频繁地 GC，
   并把空闲内存更快还给系统。不设时，高峰过后 Go 归还内存很慢，`RSS` 会在一段时间里停在峰值附近。建议设为机器内存减去数据库等
   其他进程后的六七成，例如 1 GiB 的 VPS 设 `GOMEMLIMIT=600MiB`。它不会让进程因超限而退出；设得过低只会让 GC 一直跑、CPU 变高。

写法：systemd 在 `/etc/ggboard/ggboard.env` 里加 `GOMEMLIMIT=600MiB`（或 unit 里 `Environment=GOMEMLIMIT=600MiB`）；Docker Compose 写进
`.env`（`env_file` 会传进容器）或 `environment:`；源码运行时 `GOMEMLIMIT=600MiB ./bin/ggboard serve`。

*Login bursts are the memory peak (64 MiB per argon2id check). `auth.argon2_concurrency` bounds how many run at once
(default min(GOMAXPROCS, 4), at most 256 MiB; extra logins queue instead of allocating). `GOMEMLIMIT`, a Go runtime
environment variable, is a soft limit for the whole process that also makes Go return idle memory sooner, e.g.
`GOMEMLIMIT=600MiB` on a 1 GiB VPS.*

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

PostgreSQL / MySQL 连接池：最多 32 个连接、8 个空闲、单连接最长 30 分钟。Compose 下把 `GGBOARD_DATABASE_DRIVER` /
`GGBOARD_DATABASE_DSN` 写进 `.env` 即可（方式 A 是 compose 文件旁边的 `.env`，方式 B 是仓库根目录的 `.env`），不用改 compose 文件。

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

**SQLite（Docker）**：镜像里没有 `sqlite3`，先停容器再打包数据卷。卷名方式 B 是 `deploy_ggboard-data`，方式 A 是
`ggboard_ggboard-data`（`docker volume ls` 查看）；下面是方式 B 的命令，方式 A 把 `-f` 换成 `docker-compose.prebuilt.yml`、卷名换成 `ggboard_ggboard-data`：

```bash
docker compose -f deploy/docker-compose.yml stop
docker run --rm -v deploy_ggboard-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/ggboard-data-$(date +%F).tgz -C /data .
docker compose -f deploy/docker-compose.yml start
```

**PostgreSQL / MySQL**：用 `pg_dump` / `mysqldump`。

恢复：停服务 → 放回 `ggboard.db`（删掉旧的 `-wal`/`-shm`）与 `jwt.secret` → 启动。流量按批缓冲在内存、在优雅退出时落库，
所以停服务请用 `SIGTERM`（`systemctl stop` / `docker compose stop`），不要 `kill -9`。

> **恢复备份或执行 `import xboard --force` 之后应轮换 `jwt.secret`**：会话 token 是无状态的，此前签发的 token 仍能通过签名校验，
> 而用户 id 可能已经分给了别的账户。面板会拒绝邮箱与当前账户不符的 token，但恢复会把 `password_version` 回滚，
> 备份之后因改密而失效的旧会话会重新生效。轮换（`openssl rand -hex 32` 写入配置，或删除 `<data_dir>/jwt.secret` 让它重新生成）
> 的代价：所有人退出登录，已绑定的 TOTP 失效（备用码与 Passkey 不受影响），面板托管的 age 私钥无法再显示（公钥与订阅加密不受影响）。

## 8. 升级 / Upgrading

1. 备份（见上）。
2. Docker 方式 A（预构建镜像）：改 `.env` 里的 `GGBOARD_VERSION`，再
   `docker compose -f docker-compose.prebuilt.yml pull && docker compose -f docker-compose.prebuilt.yml up -d`。
   Docker 方式 B（源码构建）：`git pull && docker compose -f deploy/docker-compose.yml up -d --build`。
   二进制：`git pull && make web && make build`，替换 `/usr/local/bin/ggboard`，`systemctl restart ggboard`。
3. 新版本启动时自动迁移数据库；`ggboard migrate` 可在不启动服务的情况下单独执行迁移（打印 `database migrated (<driver>)`，终端里前面带 `✓`）。
4. `ggboard version` / `GET /healthz` 确认版本。

迁移只增不删（AutoMigrate 不删除列）；回滚到旧二进制前请先恢复备份。

从 2026-10-22 安全修复之前的版本升级时：配置的 `jwt.secret` 若是模板占位符（`change-me-to-a-long-random-string`）、
短于 32 字节或少于 8 种不同字符，新版本会拒绝启动并提示如何生成新值；换密钥的代价见第 7 节末尾。

## 9. 命令行 / CLI

全局参数：`-c/--config`、`--data-dir`、`--log-level`、`--log-format`、`--color`、`--db-driver`、`--db-dsn`。
除 `version`、`help`、`completion` 外，每个命令执行前都会加载配置并迁移数据库。

| 命令 | 作用 |
| --- | --- |
| `ggboard serve [--addr :8080]` | 运行 HTTP 服务、定时任务与邮件队列；`SIGINT`/`SIGTERM` 后 10 秒排空 |
| `ggboard migrate` | 创建/更新表结构 |
| `ggboard admin create --email E [--password P] [--staff]` | 创建管理员（或客服）；不给密码则生成并打印 |
| `ggboard admin reset-password --email E [--password P]` | 重置密码并让该用户已有会话失效 |
| `ggboard admin promote --email E` | 把已有用户提升为管理员 |
| `ggboard import xboard --dsn DSN [--driver] [--dry-run] [--force] [--skip-banned] [--with-stats] [--stats-days N] [--source-tz TZ]` | 从 Xboard 导入，见 [IMPORT_XBOARD.md](IMPORT_XBOARD.md) |
| `ggboard version` | 打印版本、Go 版本与平台 |

**终端输出 / Terminal output**。每个输出流在启动时判断一次（不是每条日志都判断）：

| 输出到 | 日志（`log.format: text`） | 命令输出（`version`、`migrate`、`admin …`、导入报告、启动摘要） |
| --- | --- | --- |
| 终端 | [tint](https://github.com/lmittmann/tint) 彩色对齐：时间变暗、`INF`/`WRN`/`ERR` 着色、键名变淡、错误值红色、4xx/5xx 状态码黄/红 | [lipgloss](https://github.com/charmbracelet/lipgloss) 排版：结果行以 `✓` / `!` / `✗` 开头，导入报告是表格 |
| 终端 + `NO_COLOR` / `--color=never` / `TERM=dumb` | 同上的对齐格式，不含任何 ANSI 转义 | 同上的排版，不含 ANSI 转义 |
| 文件、管道、systemd、docker | slog 原样的 `key=value`（logfmt），无 ANSI 转义 | 逐行纯文本，命令结果与旧版逐字相同（`database migrated (sqlite)`、`generated password: …`、导入报告），便于脚本解析；启动摘要是对齐的纯文本 |
| 任意 + `--color=always` | 彩色对齐 | 彩色排版 |

`log.format: json` 的日志永远是 slog 的 JSON，与终端无关。颜色开关只有一个：`--color` / `GGBOARD_LOG_COLOR` / `log.color`
（`auto|always|never`），并遵守 [`NO_COLOR`](https://no-color.org)（非空即在 `auto` 下关闭颜色，`--color=always` 优先）。
`version` 不读配置文件，只看 `--color` 与 `GGBOARD_LOG_COLOR`。错误一律写到 stderr（终端里是 `✗ error: …`，否则 `error: …`），退出码 1。

`serve` 开始监听后打印启动摘要：版本与启动耗时、本机访问地址、站点 URL（`app_url`）、数据库（驱动 + SQLite 文件或主机/库名，
**不含用户名、口令与参数**）、数据目录、日志级别与格式、前端来源、支付网关数与已启用的支付方式数、已启用的通知渠道、定时任务数；
还没有管理员时提示 `ggboard admin create`。`log.format: json` 时不打印摘要（输出保持全是 JSON，启动信息见 `ggboard listening` 记录）。

*Each stream is classified once at startup. Terminal: colourful aligned logs (tint) and styled command output (lipgloss).
Files, pipes, systemd and docker: slog's logfmt and plain lines (command results unchanged), never an ANSI escape. JSON logs keep slog's format.
One switch, `--color` / `GGBOARD_LOG_COLOR` / `log.color` (auto|always|never), plus `NO_COLOR`.*

## 10. 健康检查与日志 / Health and logs

- `GET /healthz` → `200 {"data":{"status":"ok","version":"…","uptime":"…"}}`，不需要鉴权，适合做存活探针。
  （distroless 镜像里没有可执行 HTTP 探测的工具，Dockerfile 未定义 `HEALTHCHECK`，请在外部监控里探测。）
- 日志写到 stderr（`journalctl -u ggboard` / `docker compose logs`），`log.format: json` 便于采集。终端里 text 格式彩色对齐，
  非终端是 logfmt（见第 9 节“终端输出”）。读取配置期间产生的日志（如首次生成 `jwt.secret` 的提示）会在 logger 建好后按配置的
  格式与级别补发（JSON 模式下也是 JSON），通过 `slog.Default` 打出的日志同样走配置的格式并进入后台日志页。
  访问日志记录**路由模板**（如 `/s/:token`）而不是实际路径，避免订阅 token 进日志。
- 后台 **系统状态** 页（`/admin/system`）可以看到最近 1000 条日志（内存环形缓冲，重启后清空）、任务与邮件队列状态。
