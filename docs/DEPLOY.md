# Deployment

**English** · [简体中文](DEPLOY.zh-CN.md)

GG Bond is **one Go binary**: the SPA, scheduled jobs, mail queue and in-process cache are built in, SQLite is the
default, and it needs no Redis and no extra workers. In production you only need the binary and a reverse proxy that
handles TLS.

Run **one instance** per database. Rate limits, node online state, buffered traffic and caches live in the process,
and scheduled jobs run in every instance: two instances behind a load balancer would each allow the full rate
limits, show different node status and run every job twice.

Paths and commands in this document match `deploy/`, `config.example.yaml`, `internal/config` and `internal/cli` in the
repository one to one. `panel.example.com` stands in for your domain throughout.

- [1. Docker Compose](#1-docker-compose)
- [2. Binary + systemd](#2-binary--systemd)
- [3. Reverse proxy](#3-reverse-proxy)
- [4. Configuration reference](#4-configuration-reference)
- [5. Databases](#5-databases)
- [6. After the first start](#6-after-the-first-start)
- [7. Backups](#7-backups)
- [8. Upgrading](#8-upgrading)
- [9. CLI](#9-cli)
- [10. Health and logs](#10-health-and-logs)

---

## 1. Docker Compose

There are two ways to get the same image. Pick one:

| | Option A: prebuilt image (recommended) | Option B: build from source |
| --- | --- | --- |
| Compose file | `deploy/docker-compose.prebuilt.yml` | `deploy/docker-compose.yml` |
| Needed on the server | Docker (Compose ≥ 2.24); no need to clone the repository | Docker and the whole repository; the first build installs Chromium, pnpm and the Go dependencies and takes a few minutes |
| What sets the version | `GGBOARD_VERSION` in `.env` (the image tag) | The checked-out code |
| Upgrade | Change the version number, then `pull` + `up -d` | `git pull` + `up -d --build` |

### Prebuilt image

For every pushed `v*` tag the release pipeline builds a multi-arch image and pushes it to GHCR (a branch can also be
built by hand); see [the end of this section](#releases-and-images).

**1. Get the files.** Create an empty directory and put the compose file and `.env` in it. `.env` must be in the same
directory as the compose file:

```bash
mkdir ggboard && cd ggboard
curl -fsSLO https://raw.githubusercontent.com/linusxiong/ggboard/main/deploy/docker-compose.prebuilt.yml
curl -fsSL https://raw.githubusercontent.com/linusxiong/ggboard/main/.env.example -o .env
```

**2. Pick a version.** Write the image tag into `.env`:

```bash
echo 'GGBOARD_VERSION=1.0.1' >> .env
```

| `GGBOARD_VERSION` | Meaning |
| --- | --- |
| `1.0.0` | One specific release. Use this in production (the list of versions is on [Releases](https://github.com/linusxiong/ggboard/releases)) |
| `1.0` / `1` | The newest stable release in 1.0.x / 1.x (there is no `0` while the major version is 0) |
| `latest` | The newest stable release (a `v*` tag without a suffix such as `-rc`); used when `GGBOARD_VERSION` is not set for the compose file |
| `main` | The latest manual build of the `main` branch; a rolling tag, good for trying things out first |
| `sha-<7 chars>` | The build of one commit |

While the image package is still private, run `docker login ghcr.io` first: the user name is your GitHub account and
the password is a token with the `read:packages` scope.
For the other entries in `.env` see [§4](#4-configuration-reference). If `GGBOARD_JWT_SECRET` is left empty, a secret
is generated and stored in the data volume.

**3. Start it and create the admin.**

```bash
docker compose -f docker-compose.prebuilt.yml up -d
docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
#   without --password a random password is generated and printed, shown only once
```

Check that it is running:

```bash
docker compose -f docker-compose.prebuilt.yml ps
docker compose -f docker-compose.prebuilt.yml logs --tail 30 ggboard    # the startup summary shows the version, database and data directory
curl -s http://127.0.0.1:8080/healthz                                   # {"data":{"status":"ok",…}}
```

**4. Put it behind a reverse proxy.** The container is published only on `127.0.0.1:8080`. To reach it from outside you
need nginx / Caddy on the host and HTTPS; see [§3](#3-reverse-proxy) for the setup.
To the container, a proxy on the host comes from the Docker bridge gateway, so add
`GGBOARD_SERVER_TRUSTED_PROXIES=<bridge subnet>` to `.env` (look it up with `docker network inspect ggboard_default`,
for example `172.16.0.0/12`), then run `up -d` so the container is recreated.
Then open `https://panel.example.com/login`. The order of settings after you sign in is in
[§6](#6-after-the-first-start).

**5. Upgrade and roll back.** Back up first ([§7](#7-backups)), then change `GGBOARD_VERSION` in `.env`:

```bash
docker compose -f docker-compose.prebuilt.yml pull
docker compose -f docker-compose.prebuilt.yml up -d
docker image prune -f                      # optional: remove old images
```

The new version migrates the database when it starts. To roll back, change the version number back and run `up -d`,
but restore a backup before you roll back to an older version (see [§8](#8-upgrading)).

Key points of `deploy/docker-compose.prebuilt.yml`:

| Item | Value |
| --- | --- |
| Image | `${GGBOARD_IMAGE:-ghcr.io/linusxiong/ggboard}:${GGBOARD_VERSION:-latest}`; `GGBOARD_IMAGE` is for forks and registry mirrors |
| Project name / data volume | Fixed to `ggboard`; data volume `ggboard_ggboard-data` → `/data` (`GGBOARD_DATA_DIR=/data`, owned by 65532 inside the image) |
| Port | `127.0.0.1:8080:8080`, **published on localhost only** |
| Environment | `env_file: .env` (the `.env` beside the compose file; may be absent); `environment:` pins only `GGBOARD_DATA_DIR`. Compose also uses this `.env` to resolve `${GGBOARD_VERSION}` |
| Hardening | `cap_drop: [ALL]` and `no-new-privileges`; the image is distroless and runs as `nonroot` (UID 65532) |
| Stopping | `restart: unless-stopped`, `stop_grace_period: 20s` |

Switching from option B and keeping the existing data: option B's data volume is `deploy_ggboard-data`; option A's
default is `ggboard_ggboard-data`.
First run `docker compose -f deploy/docker-compose.yml down` in the repository (**do not add `-v`**), copy the `.env`
from the repository root into option A's directory, and from then on add `-p deploy` to every command (`-p` on the
command line takes precedence over `name:` in the file), for example
`docker compose -p deploy -f docker-compose.prebuilt.yml up -d`.

### Build from source

`deploy/Dockerfile` is a three-stage build: `node:24-alpine` (pnpm builds the frontend) → `golang:1.27-alpine`
(compiles with `CGO_ENABLED=0` and embeds `web/dist`) → `gcr.io/distroless/static-debian13:nonroot` (no shell; runs as
`nonroot` (UID 65532)).

```bash
git clone https://github.com/linusxiong/ggboard.git && cd ggboard
cp .env.example .env                       # leave GGBOARD_JWT_SECRET empty to generate one; if you set it, it must be a random value such as openssl rand -hex 32
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com
#   without --password a random password is generated and printed
```

Key points of `deploy/docker-compose.yml`:

| Item | Value |
| --- | --- |
| Image | Built locally, `ggboard:${GGBOARD_VERSION:-latest}`; the build argument `VERSION` goes into `ggboard version` |
| Port | `127.0.0.1:8080:8080`, **published on localhost only**; nginx / Caddy on the host faces the outside |
| Data volume | Named volume `ggboard-data` → `/data` (`GGBOARD_DATA_DIR=/data`, owned by 65532 inside the image) |
| Environment | `env_file: ../.env` (the `.env` in the repository root; may be absent); `environment:` pins only `GGBOARD_DATA_DIR`. The database, JWT and the rest all come from `.env`; the SQLite DSN defaults to `/data/ggboard.db` |
| Stopping | `restart: unless-stopped`, `stop_grace_period: 20s` (enough for the server's 10 s drain, the traffic flush and finishing the mail queue) |

Notes:

- Values under `environment:` in compose **take precedence over** `env_file`, so only `GGBOARD_DATA_DIR` goes there.
  To switch to PostgreSQL / MySQL, set `GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN` in `.env`
  (see [§5](#5-databases)).
  Do not write the SQLite DSN as a relative path: the container's working directory is `/data`, so
  `./data/ggboard.db` points to `/data/data/ggboard.db` and cannot be opened. Leave it empty to use the default.
- The image has no shell, and no `curl`/`sqlite3`. Run every maintenance command as
  `docker compose exec ggboard ggboard <subcommand>`.
- Scheduled jobs run in the process time zone (`scheduler.New(..., time.Local)`). The container defaults to UTC; for
  Beijing time add `TZ: Asia/Shanghai` under `environment:` (the binary embeds `time/tzdata`).
- When the reverse proxy runs on the host, the source IP the container sees is the Docker bridge gateway, so
  `GGBOARD_SERVER_TRUSTED_PROXIES` must be set to the bridge subnet (look it up with `docker network inspect`, for
  example `172.16.0.0/12`); see [§3](#3-reverse-proxy).

### Releases and images

For maintainers. There are two pipelines:

- **`.github/workflows/release.yml`**: runs automatically when a `v*` tag is pushed and produces every release
  artifact in one run: binary archives for five platforms with `SHA256SUMS` (see [§2](#2-binary--systemd)), the
  multi-arch image (by calling `docker.yml`), and last the GitHub release for the tag. The release notes are the
  matching `## [X.Y.Z]` section of [`CHANGELOG.md`](../CHANGELOG.md) (English), followed by the same section of
  [`CHANGELOG.zh-CN.md`](../CHANGELOG.zh-CN.md) (Chinese) under a `## 简体中文` heading. A tag with a `-` suffix (such
  as `v1.1.0-rc.1`) is a pre-release.
- **`.github/workflows/docker.yml`**: builds only the image. release.yml calls it; it can also be run by hand
  (Actions → **Docker** → Run workflow). It builds whichever branch or tag you pick under "Use workflow from"; picking
  `main`, for example, gives the two tags `main` and `sha-<7 chars>`.

To release:

1. Write a `## [1.0.0] - <date>` section in both `CHANGELOG.md` (English) and `CHANGELOG.zh-CN.md` (Chinese) and merge
   it into `main`. If either file lacks this section (or the section is empty), the pipeline fails before it pushes
   any image.
2. Tag that commit and push the tag: `git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`. A tag push runs the
   workflow files from the commit the tag points to, so merge changes to the workflows themselves before tagging.
3. Wait for Actions → **Release** to finish. It first checks the tag and the changelogs, then builds the binaries and
   the image in parallel, and creates the release only after both succeed (the release becomes public only once its
   assets are uploaded).
4. After the first push the package may be private: GitHub → Packages → ggboard → Package settings, set the visibility
   to Public. Only then can servers `docker pull` without logging in.

Re-running: Actions → **Release** → Run workflow, with an existing tag. It uses the workflow file of the selected branch
(usually `main`) but still builds the tag's code, and the notes still come from the tag's `CHANGELOG.md` and
`CHANGELOG.zh-CN.md` (tags up to and including v1.0.1 have no `CHANGELOG.zh-CN.md`; their bilingual `CHANGELOG.md` is
used alone). The image is rebuilt and pushed, and the release's notes and assets are replaced (when the repository has
immutable releases turned on, the assets of a published release can no longer be replaced). To change only the notes,
edit the release on GitHub directly.

| Tag / selected ref | Image tags |
| --- | --- |
| Tag `v1.2.3` | `1.2.3`, `1.2`, `1`, `latest`, `sha-<7 chars>` (no `0` while the major version is 0) |
| Pre-release tag `v1.2.3-rc.1` | `1.2.3-rc.1`, `sha-<7 chars>` (`latest` is left alone) |
| Other tags (manual builds only) | `<tag>`, `sha-<7 chars>` |
| Branches (such as `main`; manual builds only) | `<branch name>` (`/` replaced by `-`), `sha-<7 chars>` |

Inputs of a manual Docker workflow run: `push` (unchecked = build without pushing, to check the Dockerfile and warm the
cache), `platforms` (default `linux/amd64,linux/arm64`), `no_cache` (ignore the cache and pull the base images again;
the new layers are still written back to the cache).

- **Version string**: a tag build uses the tag itself (`v1.2.3`); other refs use `git describe --tags --always` (the
  same as `make build`). For one tag, the image's OCI `version`, `ggboard version`, `/healthz` and the released
  binaries all report the same string; the archive file names and the image tags drop the `v`.
- **Metadata**: the image carries OCI labels, and the manifest and index carry the same annotations (the GHCR page reads
  the index annotations): `source`, `revision`, `version`, `created`, `licenses` and others. After the build the job
  reads the registry back and checks the platforms, annotations, labels, each tag's digest and the version the binary
  reports; any mismatch fails the job. SBOM and provenance are attached as well.
- **Cache**: BuildKit's GitHub Actions cache (`mode=max`, which also stores intermediate stages such as the pnpm
  install, Chromium and `go mod download`). Each run can read the cache of its own ref and of the default branch, so
  build once on `main` before a release and the tag build will hit the cache. The repository's cache limit is 10 GB,
  and entries unused for 7 days are evicted.
- **Multi-arch without QEMU**: the frontend and Go build stages run on the runner's native architecture
  (`--platform=$BUILDPLATFORM`), and Go cross-compiles to the target architecture.

For how to use this image on a server, see [option A](#prebuilt-image).

## 2. Binary + systemd

### Release binaries

Every version on the [Releases](https://github.com/linusxiong/ggboard/releases) page has
`ggboard_<version>_<os>_<arch>.tar.gz` for linux and darwin (amd64, arm64), `ggboard_<version>_windows_amd64.zip` and
`SHA256SUMS`. The binaries are statically linked with the frontend embedded and do not depend on glibc.
Each archive holds one directory of the same name containing `ggboard` (`ggboard.exe` on Windows), `LICENSE`,
`README.md` (English), `README.zh-CN.md` (Chinese) and `config.example.yaml`; the linux archives also contain
`ggboard.service`.

```bash
VERSION=1.0.1 ARCH=amd64                     # on arm64 machines use ARCH=arm64
curl -fsSLO "https://github.com/linusxiong/ggboard/releases/download/v$VERSION/ggboard_${VERSION}_linux_${ARCH}.tar.gz"
curl -fsSLO "https://github.com/linusxiong/ggboard/releases/download/v$VERSION/SHA256SUMS"
sha256sum --ignore-missing -c SHA256SUMS     # must print "…: OK"; on macOS use shasum -a 256 --ignore-missing -c SHA256SUMS
tar xzf "ggboard_${VERSION}_linux_${ARCH}.tar.gz" && cd "ggboard_${VERSION}_linux_${ARCH}"
./ggboard version                            # ggboard v1.0.1 (go1.27.x linux/amd64)

sudo useradd --system --home /var/lib/ggboard --shell /usr/sbin/nologin ggboard
sudo install -m 0755 ggboard /usr/local/bin/ggboard
sudo install -d -m 0750 /etc/ggboard         # optional: put GGBOARD_* variables in /etc/ggboard/ggboard.env
sudo cp ggboard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ggboard
```

Then create the admin with the `systemd-run` command below. For upgrades see [§8](#8-upgrading).

### From source

```bash
make web && make build                      # builds bin/ggboard (statically linked, frontend embedded)
sudo useradd --system --home /var/lib/ggboard --shell /usr/sbin/nologin ggboard
sudo install -m 0755 bin/ggboard /usr/local/bin/ggboard
sudo install -d -m 0750 /etc/ggboard        # optional: put GGBOARD_* variables in /etc/ggboard/ggboard.env
sudo cp deploy/ggboard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ggboard
```

### The unit and CLI commands

`deploy/ggboard.service`:

- Runs as the `ggboard` user. `StateDirectory=ggboard` (`/var/lib/ggboard`, mode `0700`) is both the working directory
  and `GGBOARD_DATA_DIR`. Listens on `GGBOARD_SERVER_ADDR=127.0.0.1:8080`.
- `EnvironmentFile=-/etc/ggboard/ggboard.env` (optional). To use YAML, append `--config /etc/ggboard/config.yaml` to
  `ExecStart`.
- `Restart=on-failure`, `KillSignal=SIGTERM`, `TimeoutStopSec=30s`, `LimitNOFILE=65535`.
- Hardening: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`,
  `ProtectKernel*`, `ProtectControlGroups`, `ProtectClock`, `ProtectHostname`,
  `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `RestrictNamespaces`, `RestrictRealtime`, `RestrictSUIDSGID`,
  `LockPersonality`, `MemoryDenyWriteExecute`, `SystemCallArchitectures=native`, and an empty
  `CapabilityBoundingSet`/`AmbientCapabilities`.
  Only `/var/lib/ggboard` is writable, so the SQLite database and `jwt.secret` must stay in the data directory.

Run maintenance commands as **the same user with the same environment**, or they create or rewrite the database files
as root:

```bash
sudo systemd-run --uid=ggboard --gid=ggboard --pty --wait --collect \
  -p WorkingDirectory=/var/lib/ggboard -p EnvironmentFile=-/etc/ggboard/ggboard.env \
  -E GGBOARD_DATA_DIR=/var/lib/ggboard \
  /usr/local/bin/ggboard admin create --email admin@example.com
```

`.env` is read only by compose; the binary itself reads only environment variables and YAML. When running it by hand
you can use `set -a; . ./.env; set +a; ./bin/ggboard serve`.

## 3. Reverse proxy

The panel does not terminate TLS itself and does not send HSTS. The proxy must:

1. Terminate HTTPS, redirect 80 → 443 and send `Strict-Transport-Security`.
2. Pass `Host`, `X-Forwarded-For` and `X-Forwarded-Proto`. `X-Forwarded-Proto: https` is used to build absolute URLs
   such as payment callbacks, Passkey and the subscription's links to itself (when `app_url` is set, it takes
   precedence).
3. Allow **8 MiB** request bodies: ordinary endpoints are capped at 1 MiB, and the node report endpoints
   (`/api/v1/server/UniProxy/{push,alive,status}` and soga's POSTs) at 8 MiB.
4. Put the proxy's address in `server.trusted_proxies`. **Without it, the client IP of every request is the proxy's
   IP**: the login/sign-up rate limits (`/api/v1/passport/*` at 30 per minute per IP, sending verification codes at 10
   per hour per IP), the sign-up IP limit and the IPs in the logs all become "one IP for the whole site".
5. **Keep secrets out of its access log.** Node backends send their key in the URL
   (`/api/v1/server/UniProxy/*?token=…`, fixed by the XrayR / V2bX protocol), and so do subscription links
   (`/api/v1/client/subscribe?token=…`, `/s/<token>`). The panel's own log never records query strings and redacts the
   short link, but a proxy's default access log keeps the full URL. Log the path without its query string and redact
   `/s/`: `deploy/nginx.example.conf` does both. nginx's error log also quotes the full request line when an upstream
   request fails, so keep it readable by root only.

The panel does not use WebSocket, so no `Upgrade` header is needed. Responses are already gzip/br-compressed by the
panel, so the proxy does not need to compress them again.

### nginx

A complete example is in [`deploy/nginx.example.conf`](../deploy/nginx.example.conf) (HTTP/2, TLS 1.2/1.3, HSTS,
`client_max_body_size 8m`, `proxy_read_timeout 120s`, upstream keepalive). Pair it with:

```bash
GGBOARD_SERVER_TRUSTED_PROXIES=127.0.0.1,::1
```

### Caddy

Caddy obtains certificates and redirects 80→443 automatically, and sets `X-Forwarded-For` / `X-Forwarded-Proto` /
`X-Forwarded-Host` by default:

```caddyfile
panel.example.com {
    header Strict-Transport-Security "max-age=63072000; includeSubDomains"
    request_body {
        max_size 8MB
    }
    reverse_proxy 127.0.0.1:8080
}
```

Set `GGBOARD_SERVER_TRUSTED_PROXIES=127.0.0.1,::1` here too. If a CDN (such as Cloudflare) sits in front, make Caddy /
nginx trust the CDN's origin-pull IP ranges, or you get the IPs of the CDN nodes.

## 4. Configuration reference

Order of precedence: **command-line flags > environment variables > config file > defaults**. Config file:
`--config/-c`, otherwise `$GGBOARD_CONFIG`, otherwise `config.yaml` in the current directory if it exists (gitignored;
the template is [`config.example.yaml`](../config.example.yaml)). Environment variable = `GGBOARD_` + the key in upper
case with `.` replaced by `_`; list values are comma-separated. The site name, mail, payments, the node communication
token and other **runtime settings live in the database**. They are changed on the admin console's "Settings" page, not
here.

| Key | Environment variable | Flag | Default | Description |
| --- | --- | --- | --- | --- |
| `server.addr` | `GGBOARD_SERVER_ADDR` | `serve --addr` | `:8080` | Listen address |
| `server.trusted_proxies` | `GGBOARD_SERVER_TRUSTED_PROXIES` | | `[]` | Proxy IPs/CIDRs whose `X-Forwarded-For`/`X-Real-IP` are trusted; empty = trust none; an invalid value fails startup |
| `server.cors_origins` | `GGBOARD_SERVER_CORS_ORIGINS` | | `[]` | Extra allowed cross-origin origins, `*` = any; empty = same origin only (recommended) |
| `database.driver` | `GGBOARD_DATABASE_DRIVER` | `--db-driver` | `sqlite` | `sqlite` / `postgres` / `mysql` |
| `database.dsn` | `GGBOARD_DATABASE_DSN` | `--db-dsn` | SQLite: `<data_dir>/ggboard.db` | Required for PostgreSQL / MySQL |
| `jwt.secret` | `GGBOARD_JWT_SECRET` | | empty | Session signing key. If you set it, it must be at least 32 bytes with at least 8 distinct characters (`openssl rand -hex 32`); a known placeholder (such as the old template's `change-me-…`), or a value that is too short or monotonous, makes startup fail outright. When empty, a random 48-byte value is generated, written to `<data_dir>/jwt.secret` (0600) and reused from then on (the file's content is checked the same way) |
| `jwt.ttl` | `GGBOARD_JWT_TTL` | | `168h` | Session lifetime (Go duration, must be > 0) |
| `auth.argon2_concurrency` | `GGBOARD_AUTH_ARGON2_CONCURRENCY` | | `0` (auto) | How many argon2id password checks / hashes run at the same time (sign-in, sign-up, password change, password reset). Each one takes 64 MiB while it runs; requests beyond the limit **queue for a free slot** instead of taking more memory, so this part peaks at about "the value × 64 MiB". `0` = `min(CPU cores available to Go, 4)` (`GOMAXPROCS`, which follows the CPU quota in a container), i.e. at most 256 MiB. On small-memory machines set 1–2: login peaks queue and get slower, but they do not exhaust memory. Raising it rarely improves throughput (each check already uses 4 threads). A negative value fails startup |
| `log.level` | `GGBOARD_LOG_LEVEL` | `--log-level` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `GGBOARD_LOG_FORMAT` | `--log-format` | `text` | `text` / `json` |
| `log.color` | `GGBOARD_LOG_COLOR` | `--color` | `auto` | Terminal colour and layout: `auto` (colour when the output is a terminal and `NO_COLOR` is not set) / `always` (colour and terminal layout for pipes and files too; takes precedence over `NO_COLOR`) / `never` (no colour; terminals still get the aligned layout). Does not affect JSON logs; see §9 and §10 |
| `frontend.dir` | `GGBOARD_FRONTEND_DIR` | | empty | Serve the SPA from this directory instead of the embedded build (the directory must exist) |
| `data_dir` | `GGBOARD_DATA_DIR` | `--data-dir` | `./data` | Directory of the SQLite database and `jwt.secret`, created with 0700 at startup; `geoip/` inside it holds the automatically downloaded CZ88 IP database (about 27 MB; see §6) |

> **`jwt.secret` is more than the session key**: TOTP two-factor secrets are stored encrypted with an AES-256-GCM key
> derived from it (`internal/service/mfa/crypto.go`). Changing or losing it signs everyone out and **invalidates every
> enrolled TOTP** (backup codes still work).
> Always back it up together with the database.

### Memory

Login peaks use the most memory: every argon2id check needs 64 MiB. Two knobs work together:

1. **`auth.argon2_concurrency`** (see the table above) limits how many checks run **at the same time**, which is the
   upper bound of this part of the memory at peak. The default is at most 4 (256 MiB). On a VPS with 1 GiB of memory
   we suggest `2` (128 MiB); with 512 MiB, `1`. The cost is that the extra logins at peak queue: a request in position
   N waits roughly N / concurrency extra check times (how long one check takes depends on the CPU; measurements on a
   development machine are in [`docs/perf/README.md`](perf/README.md)).
2. **`GOMEMLIMIT`** (an environment variable of the Go runtime, not a ggboard config key, and it cannot be written into
   YAML) is the Go runtime's **soft memory limit**: as total memory approaches it, the GC runs earlier and more often,
   and the runtime returns idle pages to the system to stay under the limit. It **is not a hard limit**: when
   allocation is very fast the GC caps its own CPU use and memory can go past it. Measured on a development machine (a
   single 10 s login burst, concurrency limit 4, two runs each): without it peak RSS was 826–827 MiB, with
   `GOMEMLIMIT=600MiB` 774–808 MiB. In both cases the RSS shown by `ps` did not go down within 3 minutes after the burst
   ([`docs/perf/README.md`](perf/README.md), finding 2). So what really bounds a login burst is item 1; `GOMEMLIMIT`
   makes the whole process reclaim memory more eagerly when memory is tight. We suggest 60–70% of the machine's memory
   left after the database and other processes, for example `GOMEMLIMIT=600MiB` on a 1 GiB VPS. It never makes the
   process exit for going over the limit; setting it too low only keeps the GC running and drives CPU use up.

How to set it: for systemd, add `GOMEMLIMIT=600MiB` to `/etc/ggboard/ggboard.env` (or `Environment=GOMEMLIMIT=600MiB`
in the unit); for Docker Compose, put it in `.env` (`env_file` passes it into the container) or under `environment:`;
when running from source, `GOMEMLIMIT=600MiB ./bin/ggboard serve`.

## 5. Databases

At startup (and before every CLI command that needs the App), GORM AutoMigrate runs on `model.All()`. It is idempotent,
so there is no separate migration step.

**SQLite (default)**: a pure Go driver; `database.dsn` is a file path or a `file:` URI. If the DSN carries no `_pragma`
of its own, `busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)` and `synchronous(NORMAL)` are appended, and
`_txlock=immediate` is set.
Connection pool 8. The data directory will contain `ggboard.db`, `ggboard.db-wal` and `ggboard.db-shm`.

**PostgreSQL**: the DSN is handed to pgx (`pgx.ParseConfig`); both a URL and `key=value` work. Parameters such as
`TimeZone=` are sent to the server as session settings.
Whatever the process time zone is (for example `TZ=Asia/Shanghai` in a container), times are always read back in UTC,
the same as with SQLite and MySQL:

```yaml
database:
  driver: postgres
  dsn: postgres://ggboard:password@db.example.com:5432/ggboard?sslmode=require
```

**MySQL / MariaDB**: must be in go-sql-driver format (not a `mysql://` URL; that is the format of
`import xboard --dsn`).
The panel forces `parseTime=true` and adds `loc=UTC` and `charset=utf8mb4` when they are not given. Tables are created
with `ENGINE=InnoDB utf8mb4_unicode_ci`:

```yaml
database:
  driver: mysql
  dsn: ggboard:password@tcp(db.example.com:3306)/ggboard
```

PostgreSQL / MySQL connection pool: at most 32 connections, 8 idle, at most 30 minutes per connection. Under Compose,
just put `GGBOARD_DATABASE_DRIVER` / `GGBOARD_DATABASE_DSN` in `.env` (for option A the `.env` beside the compose file,
for option B the `.env` in the repository root); the compose file does not need to change.

### Moving from SQLite to PostgreSQL

`ggboard db copy` copies the whole **currently configured database** (`database.driver` / `database.dsn`, the same one
`serve` reads) into the database given by `--to-driver` / `--to-dsn`. It works in either direction (SQLite → PostgreSQL,
and PostgreSQL → SQLite for rolling back; MySQL works too):

1. The target first gets the same schema creation as `serve` (`migrate`), so its tables are exactly the same as on an
   install made directly on PostgreSQL.
2. The source is read in a read-only snapshot transaction, and the target is written in **one transaction**: each table
   is inserted in batches by primary key (500 rows by default), with the primary key and every column kept as they are
   (NULL, 0, false, JSON text byte for byte, timestamps), bypassing GORM's hooks, defaults and automatic timestamps.
3. Checks before commit: both sides have the same tables, `COUNT(*)` is equal for every table, and every column of
   every row, read back from both sides, has the same SHA-256. On PostgreSQL each auto-increment sequence is set to
   `max(id)+1`, and the copy then confirms the next value will not collide. If any step fails, everything is rolled
   back and the target is left with empty tables only.
4. It refuses to run when: source and target are the same database; either side has tables or columns ggboard does not
   create (their data would be lost); the target already has data (it is emptied first, inside the same transaction,
   only with `--overwrite`); a value in the source does not fit in the target (for example longer than a `varchar`
   length, or a NUL byte or invalid UTF-8 that PostgreSQL does not accept; the error names the table, primary key and
   column, without printing the value).

Timestamps are stored at the target's precision: PostgreSQL keeps microseconds, MySQL milliseconds (extra digits from
SQLite are cut off, and the check compares at that precision); `date` columns keep the calendar day. When it finishes
it prints each table's row count and digest (the first 16 characters of the SHA-256); a non-zero exit code means it
failed.

**Stop the server first**: rows written to the source during the copy are not copied, and traffic statistics are
buffered in memory and written to the database only on graceful shutdown (`SIGTERM`).
`--dry-run` runs the full copy and checks and then rolls back (leaving only the created empty tables), so you can
rehearse while the server runs: an SQLite source holds the write lock during the copy, and the server's writes queue
behind it (`busy_timeout` 5 seconds; a database of a few hundred KB finishes in under a second).

**Preparing PostgreSQL (existing server)**. Create a dedicated role and database (from PostgreSQL 15 on, the `public`
schema belongs to the database owner, and the owner can create tables):

```sql
CREATE ROLE ggboard LOGIN PASSWORD '<password>';
CREATE DATABASE ggboard OWNER ggboard ENCODING 'UTF8' TEMPLATE template0;
```

Use the URL form for the DSN, `postgres://ggboard:<password>@<host>:5432/ggboard?sslmode=<mode>`: on the same machine
or in the same Docker network use `sslmode=disable`; over the public internet use `sslmode=require` (encrypted, but the
certificate is not checked) or `verify-full` (checks the certificate and host name; with a private CA also add
`sslrootcert=<CA file>`, and in a container mount that file in).
Characters such as `@ : / ? # % &` in the password must be URL-encoded, for example:

```bash
python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=""))' '<password>'
```

**Preparing PostgreSQL (running alongside the panel)**. `deploy/docker-compose.prebuilt.yml` includes a `postgres`
service that does not start by default (`postgres:17-alpine`, enabled only with `--profile postgres`, data volume
`ggboard_postgres-data`). Set the password in `.env`; the host name in the DSN is `postgres`:

```bash
GGBOARD_POSTGRES_PASSWORD=<output of openssl rand -hex 24>
# add these two lines after the copy (see step 5 below):
# GGBOARD_DATABASE_DRIVER=postgres
# GGBOARD_DATABASE_DSN=postgres://ggboard:<the same password>@postgres:5432/ggboard?sslmode=disable
```

From then on, add `--profile postgres` to every compose command (or write `COMPOSE_PROFILES=postgres` in `.env`);
otherwise `up -d` does not start the database.

**Docker Compose steps** (option A; for option B change `-f` to `deploy/docker-compose.yml` and the volume name to
`deploy_ggboard-data`):

```bash
dc() { docker compose -f docker-compose.prebuilt.yml "$@"; }   # when PostgreSQL runs alongside the panel: ... --profile postgres "$@"
PG_DSN='postgres://ggboard:<URL-encoded password>@<host>:5432/ggboard?sslmode=disable'

# 0. when PostgreSQL runs alongside the panel, start it first: dc --profile postgres up -d postgres
# 1. rehearse (optional; the server can keep running)
dc run --rm ggboard db copy --to-driver postgres --to-dsn "$PG_DSN" --dry-run
# 2. stop the server (SIGTERM; buffered traffic is written to the database)
dc stop ggboard
# 3. back up the data volume (ggboard.db and jwt.secret are both in it)
docker run --rm -v ggboard_ggboard-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/ggboard-data-$(date +%F).tgz -C /data .
# 4. copy and verify
dc run --rm ggboard db copy --to-driver postgres --to-dsn "$PG_DSN"
# 5. switch the config: write into .env (the value of $PG_DSN, as is)
#      GGBOARD_DATABASE_DRIVER=postgres
#      GGBOARD_DATABASE_DSN=postgres://ggboard:<URL-encoded password>@<host>:5432/ggboard?sslmode=disable
# 6. start
dc up -d
dc logs --tail 30 ggboard     # the database line of the startup summary should read postgres · <host>:5432/ggboard
```

`run --rm` uses the same service definition: the same `.env` (which at this point still points to SQLite, the source of
the copy), the same data volume and the same network.
When PostgreSQL runs in another compose project or another container, the ggboard service has to join the Docker
network it is on, and the DSN host is its container name.
Do not edit the official compose file; write an override file instead (find the network name with
`docker network ls`), and from then on pass both `-f` to every command:

```yaml
# docker-compose.local.yml
services:
  ggboard:
    networks: [default, pg]
networks:
  pg:
    external: true
    name: <the network PostgreSQL is on>
```

Keep the data volume mounted: `jwt.secret` is in it (unless you provide `GGBOARD_JWT_SECRET`). If it changes, everyone
has to sign in again and enrolled TOTP stops working (see §4).
The copy does not modify the original `ggboard.db`; do not delete it until you have confirmed everything is right.

**Verify**: sign in with your own account (sessions are unchanged, so signed-in users are not kicked out), and check
that the numbers of users, nodes, plans and orders match the copy summary. Wait one node report interval and check that
the nodes show as online on the admin console's Nodes page. Fetch a subscription once. The row count of each table in
the copy summary is the row count from before the migration.

**Rolling back**:

- Just switched, with no new data worth keeping yet: stop the server, change `GGBOARD_DATABASE_DRIVER` in `.env` back to
  `sqlite`, delete `GGBOARD_DATABASE_DSN` (back to the default `/data/ggboard.db`), and run `up -d`. The original SQLite
  database was never touched; the new data written to PostgreSQL is lost.
- Keeping the new data from PostgreSQL: stop the server. `.env` now points to PostgreSQL, so it is the source of the
  copy; copy it into a new SQLite file in the data volume, then point the config at that file:

  ```bash
  dc stop ggboard
  dc run --rm ggboard db copy --to-driver sqlite --to-dsn /data/ggboard-from-postgres.db
  # .env: GGBOARD_DATABASE_DRIVER=sqlite, GGBOARD_DATABASE_DSN=/data/ggboard-from-postgres.db
  dc up -d
  ```

**systemd**: the same order, with commands run as the `ggboard` user and with the service's environment
(`EnvironmentFile` still points to SQLite at this point):

```bash
sudo systemctl stop ggboard
sudo -u ggboard sqlite3 /var/lib/ggboard/ggboard.db ".backup '/var/lib/ggboard/ggboard-before-postgres.db'"
sudo systemd-run --uid=ggboard --gid=ggboard --pty --wait --collect \
  -p WorkingDirectory=/var/lib/ggboard -p EnvironmentFile=-/etc/ggboard/ggboard.env \
  -E GGBOARD_DATA_DIR=/var/lib/ggboard \
  /usr/local/bin/ggboard db copy --to-driver postgres --to-dsn "$PG_DSN"
# /etc/ggboard/ggboard.env: GGBOARD_DATABASE_DRIVER=postgres, GGBOARD_DATABASE_DSN=<the same DSN> (chmod 600)
sudo systemctl start ggboard
```

Rolling back works the same way: change the driver back to `sqlite` and delete the DSN, or copy the PostgreSQL data
back with `db copy --to-driver sqlite --to-dsn /var/lib/ggboard/ggboard-from-postgres.db` and point the config at it.
For backups after moving to PostgreSQL see [§7](#7-backups).

## 6. After the first start

1. `ggboard admin create --email ...` creates the admin (`--staff` creates a staff account).
2. Admin console **Settings → Site**: fill in `app_url` (such as `https://panel.example.com`). Payment callback URLs,
   subscription links, links in emails and the Passkey RP ID / origin are all based on it. Without `app_url` (and
   without `passkey_rp_id`), Passkeys are unavailable, and the other links can only fall back to the request's Host or
   to relative paths.
3. **Settings → Node connection**: a 32-character `server_token` was generated automatically on the first start. Enter
   it in XrayR / V2bX / soga (see [NODE_PROTOCOLS.md](NODE_PROTOCOLS.md)).
4. **Settings → Email**: the default driver is `log` (writes to the log only). After switching to `smtp`, verify it with
   "Send test email".
5. **Settings → Security**: turning on `admin_mfa_required` is recommended; see [SECURITY.md](../SECURITY.md).
6. **Settings → Telegram** (optional): enter the Bot Token from BotFather and the admin Chat ID (send `/id` in a chat
   with the bot to get it), save, then click "Set webhook" and "Send test message". Telegram only accepts HTTPS webhooks
   (port 443, 80, 88 or 8443), so `app_url` must be a publicly reachable `https://` address, and the reverse proxy must
   let `POST /api/v1/guest/telegram/webhook/<secret>` through (do not put authentication or caching on it).
   See the Telegram section of [ADMIN_GUIDE.md](ADMIN_GUIDE.md) for details.
7. **Settings → Subscription** (optional): setting `subscribe_leak_max_ips` above 0 turns on subscription leak
   detection; see [SECURITY.md](../SECURITY.md).
8. **Settings → Subscription → IP locations** (on by default): the IP locations in the subscription access log come
   from the CZ88 IP database ([metowolf/qqwry.dat](https://github.com/metowolf/qqwry.dat), about 27 MB, IPv4 only),
   stored at `<data_dir>/geoip/qqwry.dat` (the `qqwry.json` beside it records the version and checksum). Startup does
   not wait for it: with no file, the server downloads it in the background 30 seconds after start, and then checks for
   a new version every day at 05:23 (`geoip_update`). On failure the old file is kept and the error is shown on that
   settings page.
   - **Outbound access**: the server must be able to reach `raw.githubusercontent.com` (version number), `github.com` and
     `release-assets.githubusercontent.com` (downloads redirect there from github.com). Behind a proxy, set
     `HTTPS_PROXY` (and `NO_PROXY`) for the service: for systemd in `/etc/ggboard/ggboard.env`, for Docker in `.env`.
   - **No access to GitHub**: point "Database download URL" and "Version file URL" at a mirror (for example a GitHub
     accelerator prefix + the original URL). If the mirror has no `version.json`, empty the version file URL; each check
     then downloads the whole file and compares checksums instead. You can also empty the download URL and put
     `qqwry.dat` into `<data_dir>/geoip/` yourself (the data volume in a container); it takes effect after a restart.
   - The whole database is read into memory (as large as the file, about 27 MB; at the moment of an update the old and
     new copies are both held). If you do not need it, turn off "Show IP locations": nothing more is downloaded, and
     after a restart it is no longer loaded.

## 7. Backups

Only the database and `jwt.secret` need backing up (if the secret comes from an environment variable, back up your
`.env` / `ggboard.env` instead). `<data_dir>/geoip/` does not need backing up; it is downloaded again when missing.

**SQLite (systemd)**: in WAL mode you can take a hot backup while the server runs. Do not `cp` a `.db` that is being
written:

```bash
sqlite3 /var/lib/ggboard/ggboard.db ".backup '/srv/backup/ggboard-$(date +%F).db'"
# or: sqlite3 /var/lib/ggboard/ggboard.db "VACUUM INTO '/srv/backup/ggboard-$(date +%F).db'"
cp /var/lib/ggboard/jwt.secret /srv/backup/ 2>/dev/null || true
```

**SQLite (Docker)**: the image has no `sqlite3`, so stop the container first and then archive the data volume. The
volume is `deploy_ggboard-data` for option B and `ggboard_ggboard-data` for option A (check with `docker volume ls`).
The commands below are for option B; for option A change `-f` to `docker-compose.prebuilt.yml` and the volume name to
`ggboard_ggboard-data`:

```bash
docker compose -f deploy/docker-compose.yml stop
docker run --rm -v deploy_ggboard-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/ggboard-data-$(date +%F).tgz -C /data .
docker compose -f deploy/docker-compose.yml start
```

**PostgreSQL**: `pg_dump`'s custom format (compressed, allows selective restore). The server does not need to stop
(`pg_dump` reads in one consistent snapshot):

```bash
# the postgres service running alongside the panel (-T: no TTY, otherwise the binary output gets altered)
docker compose -f docker-compose.prebuilt.yml --profile postgres exec -T postgres \
  pg_dump -U ggboard -Fc ggboard > ggboard-$(date +%F).dump
# an existing server (the client version must not be older than the server's)
pg_dump "postgres://ggboard:<URL-encoded password>@db.example.com:5432/ggboard?sslmode=require" -Fc -f ggboard-$(date +%F).dump
```

To restore: stop the server, run `pg_restore --clean --if-exists --no-owner -d "<DSN>" ggboard-<date>.dump` (in Docker,
`docker compose … exec -T postgres pg_restore -U ggboard -d ggboard --clean --if-exists --no-owner < ggboard-<date>.dump`),
then start it again.
`jwt.secret` still has to be backed up separately (it is in the data volume / data directory). If you do not want to
install the client, `ggboard db copy --to-driver sqlite --to-dsn <new file>` can also export a verified SQLite snapshot
while the server runs (a PostgreSQL source is read in a snapshot transaction and does not block the server).

**MySQL**: use `mysqldump --single-transaction`.

To restore: stop the server → put back `ggboard.db` (delete the old `-wal`/`-shm` files) and `jwt.secret` → start.
Traffic is buffered in memory in batches and written to the database on graceful shutdown, so stop the server with
`SIGTERM` (`systemctl stop` / `docker compose stop`), not `kill -9`.

> **Rotate `jwt.secret` after restoring a backup or running `import xboard --force`**: session tokens are stateless, so
> tokens issued earlier still pass the signature check, while their user ids may now belong to other accounts. The
> panel rejects tokens whose email does not match the current account, but a restore rolls `password_version` back, so
> old sessions that were invalidated by a password change after the backup become valid again. Rotating (write
> `openssl rand -hex 32` into the config, or delete `<data_dir>/jwt.secret` so it is regenerated) costs this: everyone
> is signed out, enrolled TOTP stops working (backup codes and Passkeys are not affected), and age secret keys held by
> the panel can no longer be shown (public keys and subscription encryption are not affected).

## 8. Upgrading

1. Back up (see above).
2. Docker option A (prebuilt image): change `GGBOARD_VERSION` in `.env`, then run
   `docker compose -f docker-compose.prebuilt.yml pull && docker compose -f docker-compose.prebuilt.yml up -d`.
   Docker option B (build from source): `git pull && docker compose -f deploy/docker-compose.yml up -d --build`.
   Binary: download and verify the new version's archive (see [§2](#release-binaries)), or run
   `git pull && make web && make build`; replace `/usr/local/bin/ggboard` with `sudo install -m 0755`, then run
   `systemctl restart ggboard`.
3. The new version migrates the database when it starts. `ggboard migrate` runs the migration on its own without
   starting the server (it prints `database migrated (<driver>)`, preceded by `✓` in a terminal).
4. Confirm the version with `ggboard version` / `GET /healthz`.

Migrations only add, never remove (AutoMigrate does not drop columns). Restore a backup before you roll back to an older
binary.

When upgrading from a version before the 2026-10-22 security fix: if the configured `jwt.secret` is the template
placeholder (`change-me-to-a-long-random-string`), is shorter than 32 bytes or has fewer than 8 distinct characters, the
new version refuses to start and explains how to generate a new value. For the cost of changing the secret, see the end
of §7.

## 9. CLI

Global flags: `-c/--config`, `--data-dir`, `--log-level`, `--log-format`, `--color`, `--db-driver`, `--db-dsn`.
Every command except `version`, `help` and `completion` loads the config and migrates the database before it runs.

| Command | Purpose |
| --- | --- |
| `ggboard serve [--addr :8080]` | Runs the HTTP server, scheduled jobs and the mail queue; drains for 10 seconds after `SIGINT`/`SIGTERM` |
| `ggboard migrate` | Creates/updates the schema |
| `ggboard admin create --email E [--password P] [--staff]` | Creates an admin (or a staff account); generates and prints a password if none is given |
| `ggboard admin reset-password --email E [--password P]` | Resets the password and invalidates the user's existing sessions |
| `ggboard admin promote --email E` | Promotes an existing user to admin |
| `ggboard import xboard --dsn DSN [--driver] [--dry-run] [--force] [--skip-banned] [--with-stats] [--stats-days N] [--source-tz TZ]` | Imports from Xboard; see [IMPORT_XBOARD.md](IMPORT_XBOARD.md) |
| `ggboard db copy --to-driver D --to-dsn DSN [--overwrite] [--dry-run] [--batch-size N]` | Copies the whole currently configured database into another database (such as SQLite → PostgreSQL) and verifies it; see [§5](#moving-from-sqlite-to-postgresql); stop the server first |
| `ggboard version` | Prints the version, Go version and platform |

**Terminal output.** Each output stream is classified once at startup (not on every log line):

| Output goes to | Logs (`log.format: text`) | Command output (`version`, `migrate`, `admin …`, import report, startup summary) |
| --- | --- | --- |
| Terminal | [tint](https://github.com/lmittmann/tint) colour and alignment: time dimmed, `INF`/`WRN`/`ERR` coloured, keys faint, error values red, 4xx/5xx status codes yellow/red | [lipgloss](https://github.com/charmbracelet/lipgloss) layout: result lines start with `✓` / `!` / `✗`, and the import report is a table |
| Terminal + `NO_COLOR` / `--color=never` / `TERM=dumb` | The same aligned format, without any ANSI escape | The same layout, without ANSI escapes |
| Files, pipes, systemd, docker | slog's plain `key=value` (logfmt), no ANSI escapes | Plain text line by line; command results are word for word the same as in earlier versions (`database migrated (sqlite)`, `generated password: …`, import report), so scripts can parse them; the startup summary is aligned plain text |
| Anything + `--color=always` | Coloured and aligned | Coloured layout |

`log.format: json` logs are always slog's JSON, whatever the terminal. There is only one colour switch: `--color` /
`GGBOARD_LOG_COLOR` / `log.color` (`auto|always|never`), and [`NO_COLOR`](https://no-color.org) is honoured (any
non-empty value turns colour off under `auto`; `--color=always` takes precedence).
`version` does not read the config file; it only looks at `--color` and `GGBOARD_LOG_COLOR`. Errors always go to stderr
(`✗ error: …` in a terminal, `error: …` otherwise), with exit code 1.

Once `serve` is listening it prints a startup summary: the version and how long startup took, the local access address,
the site URL (`app_url`), the database (driver + SQLite file or host/database name, **without user name, password or
parameters**), the data directory, the log level and format, the frontend source, the number of payment gateways and
of enabled payment methods, the enabled notification channels and the number of scheduled jobs. When there is no admin
yet, it suggests `ggboard admin create`. With `log.format: json` no summary is printed (the output stays all JSON; the
startup information is in the `ggboard listening` record).

## 10. Health and logs

- `GET /healthz` → `200 {"data":{"status":"ok","version":"…","uptime":"…"}}`. It needs no authentication and suits a
  liveness probe.
  (The distroless image has no tool that could run an HTTP probe, so the Dockerfile defines no `HEALTHCHECK`; probe it
  from external monitoring.)
- Logs go to stderr (`journalctl -u ggboard` / `docker compose logs`); `log.format: json` makes them easy to collect.
  In a terminal the text format is coloured and aligned; elsewhere it is logfmt (see "Terminal output" in §9). Logs
  produced while the config is being read (such as the notice that `jwt.secret` was generated for the first time) are
  emitted again in the configured format and level once the logger is set up (as JSON in JSON mode). Logs written
  through `slog.Default` also use the configured format and appear on the admin console's log page.
  The access log records the **route template** (such as `/s/:token`) rather than the actual path, so subscription
  tokens never end up in the logs.
- The admin console's **System status** page (`/admin/system`) shows the latest 1000 log lines (an in-memory ring
  buffer, cleared on restart) and the state of the jobs and the mail queue.
