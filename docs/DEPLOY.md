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

每推送一个 `v*` tag，发布流水线就构建多架构镜像并推送到 GHCR（也可以手动构建某个分支），见[本节末尾](#发布版本与镜像--releases-and-images)。

**1. 取文件。** 新建一个空目录，放进 compose 文件和 `.env`；`.env` 必须和 compose 文件在同一个目录：

```bash
mkdir ggboard && cd ggboard
curl -fsSLO https://raw.githubusercontent.com/linusxiong/ggboard/main/deploy/docker-compose.prebuilt.yml
curl -fsSL https://raw.githubusercontent.com/linusxiong/ggboard/main/.env.example -o .env
```

**2. 选版本。** 把镜像 tag 写进 `.env`：

```bash
echo 'GGBOARD_VERSION=1.0.0' >> .env
```

| `GGBOARD_VERSION` | 含义 |
| --- | --- |
| `1.0.0` | 一个具体的发布版本，生产环境用这种（版本列表见 [Releases](https://github.com/linusxiong/ggboard/releases)） |
| `1.0` / `1` | 1.0.x / 1.x 里最新的稳定版（主版本为 0 时没有 `0`） |
| `latest` | 最新的稳定发布版（不带 `-rc` 之类后缀的 `v*` tag）；compose 文件里没设 `GGBOARD_VERSION` 时用它 |
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

*Option A needs only `deploy/docker-compose.prebuilt.yml` and a `.env` beside it: set `GGBOARD_VERSION` (a release such as `1.0.0`,
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

### 发布版本与镜像 / Releases and images

给维护者。两条流水线：

- **`.github/workflows/release.yml`**：推送 `v*` tag 时自动运行，一次产出全部发布物：五个平台的二进制压缩包与 `SHA256SUMS`
  （见 [§2](#2-二进制--systemd--binary--systemd)）、多架构镜像（调用 `docker.yml`），最后创建这个 tag 的 GitHub Release，
  说明取自 [`CHANGELOG.md`](../CHANGELOG.md) 里对应的 `## [X.Y.Z]` 小节。tag 带 `-` 后缀（如 `v1.1.0-rc.1`）时是预发布版。
- **`.github/workflows/docker.yml`**：只构建镜像。release.yml 调用它；也可以手动运行（Actions → **Docker** → Run workflow），
  “Use workflow from” 选哪个分支或 tag 就构建哪个，例如选 `main` 得到 `main` 和 `sha-<7 位>` 两个 tag。

发版：

1. 在 `CHANGELOG.md` 写好 `## [1.0.0] - <日期>` 小节并合并到 `main`。没有这一节（或这一节是空的）时，流水线在推送任何镜像之前就失败。
2. 在这个提交上打 tag 并推送：`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`。推 tag 时运行的是 tag 所指提交里的工作流文件，
   所以工作流本身的改动要先合并再打 tag。
3. 等 Actions → **Release** 跑完：先核对 tag 和更新日志，然后二进制与镜像并行构建，两者都成功后才创建 Release（附件上传完才公开）。
4. 首次推送后包可能是私有的：GitHub → Packages → ggboard → Package settings，把可见性改成 Public，服务器上才能匿名 `docker pull`。

重跑：Actions → **Release** → Run workflow，填已有的 tag。用的是所选分支（通常是 `main`）上的工作流文件，构建的仍是 tag 的代码，
说明也仍取自 tag 里的 `CHANGELOG.md`；镜像重新构建并推送，Release 的说明和附件被替换（仓库开启了 immutable releases 时，
已发布 Release 的附件不能再替换）。只改说明的话，直接在 GitHub 上编辑这个 Release。

| tag / 选择的 ref | 镜像 tag |
| --- | --- |
| tag `v1.2.3` | `1.2.3`、`1.2`、`1`、`latest`、`sha-<7 位>`（主版本为 0 时没有 `0`） |
| 预发布 tag `v1.2.3-rc.1` | `1.2.3-rc.1`、`sha-<7 位>`（不动 `latest`） |
| 其他 tag（只能手动构建） | `<tag>`、`sha-<7 位>` |
| 分支（如 `main`，只能手动构建） | `<分支名>`（`/` 换成 `-`）、`sha-<7 位>` |

手动运行 Docker 工作流的输入项：`push`（取消勾选 = 只构建不推送，用来验证 Dockerfile 并预热缓存）、`platforms`（默认 `linux/amd64,linux/arm64`）、
`no_cache`（忽略缓存并重新拉取基础镜像，新层仍会写回缓存）。

- **版本号**：tag 构建就是 tag 本身（`v1.2.3`），其他 ref 取 `git describe --tags --always`（与 `make build` 相同）。同一个 tag 的
  镜像 OCI `version`、`ggboard version`、`/healthz` 与发布二进制报告的是同一个字符串；压缩包文件名和镜像 tag 里的版本去掉了 `v`。
- **元数据**：镜像带 OCI 标签，manifest 和 index 上也带同样的注解（GHCR 页面读的是 index 注解）：`source`、`revision`、
  `version`、`created`、`licenses` 等。构建完会回读 registry，核对平台、注解、标签、每个 tag 的 digest 和二进制报告的版本，任一不符就让任务失败；同时附带 SBOM 与 provenance。
- **缓存**：BuildKit 的 GitHub Actions 缓存（`mode=max`，把 pnpm 安装、Chromium、`go mod download` 这些中间阶段也存下来）。
  每次运行能读自己所在 ref 和默认分支的缓存，所以发版前先在 `main` 上构建一次，tag 构建就能命中；缓存仓库上限 10 GB，7 天未用会被淘汰。
- **多架构不用 QEMU**：前端和 Go 编译阶段都在 runner 本机架构上运行（`--platform=$BUILDPLATFORM`），Go 交叉编译到目标架构。

服务器上怎么用这个镜像，见[方式 A](#方式-a预构建镜像--prebuilt-image)。

*Pushing a `v*` tag runs `release.yml`: binaries for five platforms with `SHA256SUMS`, the multi-arch image through `docker.yml`
(semver → `1.2.3`/`1.2`/`1`/`latest`, plus `sha-<short>`), then a GitHub release whose notes are the tag's `CHANGELOG.md` section.
`docker.yml` can still be run by hand for any branch or tag. Images carry OCI labels and annotations, SBOM and provenance, use a
GitHub Actions layer cache, and are checked against the registry after the push.*

## 2. 二进制 + systemd / Binary + systemd

### 二进制安装（Release） / Release binaries

[Releases](https://github.com/linusxiong/ggboard/releases) 页面每个版本都有 linux、darwin（amd64、arm64）的
`ggboard_<版本>_<系统>_<架构>.tar.gz`、`ggboard_<版本>_windows_amd64.zip` 和 `SHA256SUMS`。二进制静态链接、内嵌前端，不依赖 glibc；
压缩包里是一个同名目录，内有 `ggboard`（Windows 为 `ggboard.exe`）、`LICENSE`、`README.md`、`config.example.yaml`，linux 包另有 `ggboard.service`。

```bash
VERSION=1.0.0 ARCH=amd64                     # arm64 机器写 ARCH=arm64
curl -fsSLO "https://github.com/linusxiong/ggboard/releases/download/v$VERSION/ggboard_${VERSION}_linux_${ARCH}.tar.gz"
curl -fsSLO "https://github.com/linusxiong/ggboard/releases/download/v$VERSION/SHA256SUMS"
sha256sum --ignore-missing -c SHA256SUMS     # 必须输出 "…: OK"；macOS 用 shasum -a 256 --ignore-missing -c SHA256SUMS
tar xzf "ggboard_${VERSION}_linux_${ARCH}.tar.gz" && cd "ggboard_${VERSION}_linux_${ARCH}"
./ggboard version                            # ggboard v1.0.0 (go1.27.x linux/amd64)

sudo useradd --system --home /var/lib/ggboard --shell /usr/sbin/nologin ggboard
sudo install -m 0755 ggboard /usr/local/bin/ggboard
sudo install -d -m 0750 /etc/ggboard         # 可选：/etc/ggboard/ggboard.env 放 GGBOARD_* 变量
sudo cp ggboard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ggboard
```

然后用下面的 `systemd-run` 命令创建管理员。升级见 [§8](#8-升级--upgrading)。

### 从源码构建 / From source

```bash
make web && make build                      # 产物 bin/ggboard（静态链接，内嵌前端）
sudo useradd --system --home /var/lib/ggboard --shell /usr/sbin/nologin ggboard
sudo install -m 0755 bin/ggboard /usr/local/bin/ggboard
sudo install -d -m 0750 /etc/ggboard        # 可选：/etc/ggboard/ggboard.env 放 GGBOARD_* 变量
sudo cp deploy/ggboard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ggboard
```

### 服务与运维命令 / The unit and CLI commands

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

*Download the archive for your platform from Releases, check it against `SHA256SUMS`, install the binary and `ggboard.service`
(or build from source). The unit runs as `ggboard` with a strict sandbox; only `/var/lib/ggboard` is writable. Run CLI commands as
the same user with the same environment.*

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
2. **`GOMEMLIMIT`**（Go 运行时的环境变量，不是 ggboard 的配置键，也不能写进 YAML）是 Go 运行时的**软内存上限**：总内存接近它时
   GC 会更早、更频繁地运行，运行时也会把空闲页还给系统以留在上限之下。它**不是硬上限**：分配极快时 GC 会限制自己的 CPU 占用，
   内存可以超过它。开发机上实测（单独一个 10 s 的登录高峰，并发上限 4，各两次）：不设时峰值 RSS 826–827 MiB，`GOMEMLIMIT=600MiB`
   时 774–808 MiB；两种情况下高峰过后 3 分钟内 `ps` 看到的 RSS 都没有下降（[`docs/perf/README.md`](perf/README.md)“发现”2）。所以真正限住登录高峰的是
   第 1 条；`GOMEMLIMIT` 用来让整个进程在内存紧张时更积极地回收。建议设为机器内存减去数据库等其他进程后的六七成，例如 1 GiB 的 VPS
   设 `GOMEMLIMIT=600MiB`。它不会让进程因超限而退出；设得过低只会让 GC 一直跑、CPU 变高。

写法：systemd 在 `/etc/ggboard/ggboard.env` 里加 `GOMEMLIMIT=600MiB`（或 unit 里 `Environment=GOMEMLIMIT=600MiB`）；Docker Compose 写进
`.env`（`env_file` 会传进容器）或 `environment:`；源码运行时 `GOMEMLIMIT=600MiB ./bin/ggboard serve`。

*Login bursts are the memory peak (64 MiB per argon2id check). `auth.argon2_concurrency` bounds how many run at once
(default min(GOMAXPROCS, 4), at most 256 MiB; extra logins queue instead of allocating). `GOMEMLIMIT`, a Go runtime
environment variable, is a soft limit for the whole process (more GC and earlier release of idle pages as memory
approaches it, but it can be exceeded): in measured 10 s login bursts it lowered peak RSS from 826–827 to 774–808 MiB. Use e.g.
`GOMEMLIMIT=600MiB` on a 1 GiB VPS.*

## 5. 数据库 / Databases

启动时（以及每个需要 App 的 CLI 命令前）都会对 `model.All()` 执行 GORM AutoMigrate，幂等，无需单独迁移步骤。

**SQLite（默认）**：纯 Go 驱动，`database.dsn` 是文件路径或 `file:` URI。未自带 `_pragma` 时自动追加
`busy_timeout(5000)`、`foreign_keys(1)`、`journal_mode(WAL)`、`synchronous(NORMAL)`，并设 `_txlock=immediate`。
连接池 8。数据目录里会出现 `ggboard.db`、`ggboard.db-wal`、`ggboard.db-shm`。

**PostgreSQL**：DSN 交给 pgx（`pgx.ParseConfig`），URL 或 `key=value` 均可；`TimeZone=` 之类的参数作为会话设置发给服务端。
无论进程时区是什么（例如容器里 `TZ=Asia/Shanghai`），读回的时间一律是 UTC，与 SQLite、MySQL 相同：

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

### 从 SQLite 迁移到 PostgreSQL / Moving from SQLite to PostgreSQL

`ggboard db copy` 把**当前配置的数据库**（`database.driver` / `database.dsn`，和 `serve` 读的是同一个）整库复制到
`--to-driver` / `--to-dsn` 指定的库，方向不限（SQLite → PostgreSQL，以及回滚用的 PostgreSQL → SQLite；MySQL 同样可用）：

1. 目标库先执行与 `serve` 相同的建表（`migrate`），表结构和直接在 PostgreSQL 上安装的一模一样。
2. 源库在一个只读快照事务里读，目标库在**一个事务**里写：每张表按主键分批（默认 500 行）插入，主键和每一列原样保留
   （NULL、0、false、JSON 文本逐字节、时间戳），不经过 GORM 的钩子、默认值与自动时间戳。
3. 提交前校验：两边的表一致、每张表 `COUNT(*)` 相等、每一行所有列从两边读回后的 SHA-256 相同；PostgreSQL 上把每个自增序列
   设到 `max(id)+1`，再确认下一个值不会撞号。任何一步失败都整体回滚，目标库里只留下空表。
4. 拒绝执行：源和目标是同一个库；任一边有 ggboard 不创建的表或列（其中的数据会丢）；目标库已有数据（加 `--overwrite`
   才会在同一个事务里先清空）；源里的值目标存不下（例如超过 `varchar` 长度、PostgreSQL 不接受的 NUL 字节或非法 UTF-8，
   报错里写明表、主键和列，不打印值）。

时间戳按目标库的精度保存：PostgreSQL 保留到微秒、MySQL 到毫秒（SQLite 里多出的位数被截掉，校验也按这个精度比较）；
`date` 列保持日历日。完成后打印每张表的行数和摘要（SHA-256 前 16 位），退出码非 0 即失败。

**必须先停服务**：复制期间写进源库的行不会被复制，流量统计在内存里缓冲、在优雅退出（`SIGTERM`）时才落库。
`--dry-run` 完整执行复制和校验后回滚（只留下建好的空表），可以在服务运行时先演练：SQLite 源会在复制期间持有写锁，
服务端的写入排队等待（`busy_timeout` 5 秒，几百 KB 的库不到一秒就完成）。

**准备 PostgreSQL（已有服务器）**。建一个专用角色和库（PostgreSQL 15 起 `public` schema 归库的属主，属主可以建表）：

```sql
CREATE ROLE ggboard LOGIN PASSWORD '<口令>';
CREATE DATABASE ggboard OWNER ggboard ENCODING 'UTF8' TEMPLATE template0;
```

DSN 用 URL 形式 `postgres://ggboard:<口令>@<主机>:5432/ggboard?sslmode=<模式>`：同一台机器或同一个 Docker 网络里用
`sslmode=disable`，经公网用 `sslmode=require`（加密但不校验证书）或 `verify-full`（校验证书与主机名；私有 CA 再加
`sslrootcert=<CA 文件>`，容器里要把它挂进去）。
口令里的 `@ : / ? # % &` 等字符必须做 URL 编码，例如：

```bash
python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=""))' '<口令>'
```

**准备 PostgreSQL（随面板一起跑）**。`deploy/docker-compose.prebuilt.yml` 带一个默认不启动的 `postgres` 服务
（`postgres:17-alpine`，`--profile postgres` 才启用，数据卷 `ggboard_postgres-data`）。在 `.env` 里设口令，DSN 的主机名就是 `postgres`：

```bash
GGBOARD_POSTGRES_PASSWORD=<openssl rand -hex 24 的输出>
# 复制完成后再加这两行（见下面第 5 步）：
# GGBOARD_DATABASE_DRIVER=postgres
# GGBOARD_DATABASE_DSN=postgres://ggboard:<同一个口令>@postgres:5432/ggboard?sslmode=disable
```

以后每条 compose 命令都带 `--profile postgres`（或在 `.env` 里写 `COMPOSE_PROFILES=postgres`），否则 `up -d` 不会启动数据库。

**Docker Compose 步骤**（方式 A；方式 B 把 `-f` 换成 `deploy/docker-compose.yml`，卷名换成 `deploy_ggboard-data`）：

```bash
dc() { docker compose -f docker-compose.prebuilt.yml "$@"; }   # 随面板跑 PostgreSQL 时：... --profile postgres "$@"
PG_DSN='postgres://ggboard:<URL 编码的口令>@<主机>:5432/ggboard?sslmode=disable'

# 0. 随面板跑 PostgreSQL 时先启动它：dc --profile postgres up -d postgres
# 1. 演练（可选，服务可以不停）
dc run --rm ggboard db copy --to-driver postgres --to-dsn "$PG_DSN" --dry-run
# 2. 停服务（SIGTERM，缓冲的流量落库）
dc stop ggboard
# 3. 备份数据卷（ggboard.db 与 jwt.secret 都在里面）
docker run --rm -v ggboard_ggboard-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/ggboard-data-$(date +%F).tgz -C /data .
# 4. 复制并校验
dc run --rm ggboard db copy --to-driver postgres --to-dsn "$PG_DSN"
# 5. 切换配置：在 .env 里写入（$PG_DSN 的值原样写进去）
#      GGBOARD_DATABASE_DRIVER=postgres
#      GGBOARD_DATABASE_DSN=postgres://ggboard:<URL 编码的口令>@<主机>:5432/ggboard?sslmode=disable
# 6. 启动
dc up -d
dc logs --tail 30 ggboard     # 启动摘要里数据库一行应是 postgres · <主机>:5432/ggboard
```

`run --rm` 用的是同一个服务定义：同样的 `.env`（此时还指向 SQLite，即复制的源）、同样的数据卷和网络。
PostgreSQL 在另一个 compose 项目或另一个容器里时，ggboard 服务要加入它所在的 Docker 网络，DSN 的主机写它的容器名；
不改官方 compose 文件，另写一个覆盖文件（`docker network ls` 查看网络名），之后每条命令都带上两个 `-f`：

```yaml
# docker-compose.local.yml
services:
  ggboard:
    networks: [default, pg]
networks:
  pg:
    external: true
    name: <PostgreSQL 所在的网络>
```

数据卷要一直挂着：`jwt.secret` 在里面（除非用 `GGBOARD_JWT_SECRET` 提供），换了它所有人都要重新登录、已绑定的 TOTP 失效（见 §4）。
原来的 `ggboard.db` 复制后不会被修改，确认无误前不要删。

**验证**：用自己的账号登录（会话不变，已登录的人不会被踢出），看用户数、节点、套餐和订单与复制摘要一致；
等一个节点上报周期，在后台节点页看到节点在线；随便拉一次订阅。复制摘要里每张表的行数就是迁移前的行数。

**回滚**：

- 刚切换、还没有需要保留的新数据：停服务，把 `.env` 里的 `GGBOARD_DATABASE_DRIVER` 改回 `sqlite`、删掉
  `GGBOARD_DATABASE_DSN`（回到默认的 `/data/ggboard.db`），`up -d`。原 SQLite 库没被动过，PostgreSQL 上产生的新数据就丢了。
- 要带上 PostgreSQL 上的新数据：停服务，此时 `.env` 指向 PostgreSQL，所以它是复制的源，复制到数据卷里的一个新 SQLite 文件，
  再把配置指过去：

  ```bash
  dc stop ggboard
  dc run --rm ggboard db copy --to-driver sqlite --to-dsn /data/ggboard-from-postgres.db
  # .env：GGBOARD_DATABASE_DRIVER=sqlite，GGBOARD_DATABASE_DSN=/data/ggboard-from-postgres.db
  dc up -d
  ```

**systemd**：同样的顺序，命令以 `ggboard` 用户、带服务的环境运行（`EnvironmentFile` 此时还指向 SQLite）：

```bash
sudo systemctl stop ggboard
sudo -u ggboard sqlite3 /var/lib/ggboard/ggboard.db ".backup '/var/lib/ggboard/ggboard-before-postgres.db'"
sudo systemd-run --uid=ggboard --gid=ggboard --pty --wait --collect \
  -p WorkingDirectory=/var/lib/ggboard -p EnvironmentFile=-/etc/ggboard/ggboard.env \
  -E GGBOARD_DATA_DIR=/var/lib/ggboard \
  /usr/local/bin/ggboard db copy --to-driver postgres --to-dsn "$PG_DSN"
# /etc/ggboard/ggboard.env：GGBOARD_DATABASE_DRIVER=postgres、GGBOARD_DATABASE_DSN=<同一个 DSN>（chmod 600）
sudo systemctl start ggboard
```

回滚同上：把驱动改回 `sqlite`、删掉 DSN，或用 `db copy --to-driver sqlite --to-dsn /var/lib/ggboard/ggboard-from-postgres.db`
把 PostgreSQL 的数据复制回来再指过去。迁移到 PostgreSQL 之后的备份见 [§7](#7-备份--backups)。

*`ggboard db copy --to-driver postgres --to-dsn …` copies the configured database (the one `serve` uses) into another one:
migrate, then every row with its primary key and columns as stored, in one transaction, verified by row counts and a
SHA-256 of every row on both sides, with PostgreSQL sequences moved past the copied ids; any error rolls back. It refuses
the same database, unknown tables or columns, and a non-empty target without `--overwrite`. Stop the server, back up the
data volume, copy, set `GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN`, keep `jwt.secret`, start. Roll back by
switching the config back, or by copying PostgreSQL into a new SQLite file.*

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

**PostgreSQL**：`pg_dump` 的自定义格式（压缩、可选择性恢复），服务不用停（它在一个一致性快照里读）：

```bash
# 随面板跑的 postgres 服务（-T：不分配 TTY，否则二进制输出会被改写）
docker compose -f docker-compose.prebuilt.yml --profile postgres exec -T postgres \
  pg_dump -U ggboard -Fc ggboard > ggboard-$(date +%F).dump
# 已有服务器（客户端版本不低于服务端）
pg_dump "postgres://ggboard:<URL 编码的口令>@db.example.com:5432/ggboard?sslmode=require" -Fc -f ggboard-$(date +%F).dump
```

恢复：停服务，`pg_restore --clean --if-exists --no-owner -d "<DSN>" ggboard-<日期>.dump`（Docker 里用
`docker compose … exec -T postgres pg_restore -U ggboard -d ggboard --clean --if-exists --no-owner < ggboard-<日期>.dump`），再启动。
`jwt.secret` 照旧要单独备份（在数据卷 / 数据目录里）。不想装客户端时，`ggboard db copy --to-driver sqlite --to-dsn <新文件>`
也能在服务运行时导出一份经过校验的 SQLite 快照（PostgreSQL 源在快照事务里读，不阻塞服务）。

**MySQL**：用 `mysqldump --single-transaction`。

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
   二进制：下载并校验新版本的压缩包（见 [§2](#二进制安装release--release-binaries)），或 `git pull && make web && make build`；
   用 `sudo install -m 0755` 替换 `/usr/local/bin/ggboard`，再 `systemctl restart ggboard`。
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
| `ggboard db copy --to-driver D --to-dsn DSN [--overwrite] [--dry-run] [--batch-size N]` | 把当前配置的数据库整库复制到另一个库（如 SQLite → PostgreSQL）并校验，见 [§5](#从-sqlite-迁移到-postgresql--moving-from-sqlite-to-postgresql)；先停服务 |
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
