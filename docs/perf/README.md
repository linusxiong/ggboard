# 性能测试 / Benchmark

一次在开发机上真实跑出来的负载测试：一个 `ggboard` 进程、默认 SQLite、10,000 个用户、50 个节点，
压订阅生成、节点接口、登录和静态文件。图表、表格和 `results.json` 都由同一个工具生成，`make bench` 可以原样复现。

*A real load test on a development machine — one ggboard process, SQLite, 10,000 users, 50 nodes — covering
subscriptions, the node API, login and static files. Chart, tables and `results.json` come from one tool; `make bench`
reproduces them.*

![GG Bond 吞吐量：每个场景的每秒请求数与 p99 延迟 / Requests per second and p99 latency per scenario](benchmark.svg)

## 结果 / Results（2026-10-23）

| 场景 | 请求 | 连接 | 请求数 | req/s | p50 | p95 | p99 | 错误 | 线上大小 | CPU 服务端 / 压测端 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 订阅 · v2ray（base64）† | `GET /api/v1/client/subscribe?flag=v2ray` | 64 | 85,794 | **13,172** | 3.7 ms | 12 ms | 18 ms | 0 | 4.5 KiB | 10.9 / 1.2 核 |
| 订阅 · mihomo + standard 规则 † | `GET …/subscribe?flag=clash-meta&profile=standard` | 64 | 85,708 | **13,220** | 3.7 ms | 12 ms | 18 ms | 0 | 3.3 KiB | 11.9 / 1.0 核 |
| 节点 · 用户列表，ETag 命中 → 304 | `GET /api/v1/server/UniProxy/user` + `If-None-Match` | 64 | 2,577,163 | **85,902** | 0.97 ms | 2.1 ms | 2.5 ms | 0 | 0 B | 6.1 / 4.6 核 |
| 节点 · 完整用户列表（10,000 人） | `GET /api/v1/server/UniProxy/user` | 64 | 64,644 | **2,153** | 23 ms | 81 ms | 127 ms | 0 | 244.0 KiB | 14.8 / 1.4 核 |
| 节点 · 流量上报（每次 1,000 人） | `POST /api/v1/server/UniProxy/push` | 64 | 112,963 | **3,764** | 17 ms | 23 ms | 27 ms | 0 | 13 B | 6.5 / 0.4 核 |
| 登录 · 密码（argon2id） | `POST /api/v1/passport/login` | 16 | 3,469 | **115** | 138 ms | 159 ms | 174 ms | 0 | 931 B | 16.9 / 0.0 核 |
| 静态 · `index.html`（预渲染，brotli） | `GET /` | 64 | 2,518,622 | **83,951** | 0.98 ms | 2.0 ms | 2.6 ms | 0 | 2.5 KiB | 6.6 / 4.8 核 |
| 静态 · 最大的 JS 分块（brotli，immutable） | `GET /assets/react-<hash>.js` | 64 | 1,511,301 | **50,375** | 1.1 ms | 2.5 ms | 3.5 ms | 0 | 56.8 KiB | 7.6 / 7.0 核 |

† 订阅场景在 6.5 s 后提前结束：每个令牌都已用满本次运行的 25 次额度（服务端每令牌突发 30 次），见下文“方法”。
两个订阅场景各约 8.6 万个样本，没有一个请求被限流。

- 订阅正文（解压后）：v2ray 13.8 KiB（53 条分享链接：50 个节点 + 3 条剩余流量 / 到期 / 重置信息），mihomo 25.0 KiB
  （全部 50 个节点 + 信息条目、18 个 rule-providers）。完整用户列表 JSON 887.6 KiB，gzip 后 244 KiB。
- **流量写库**：一个完整窗口 500,000 个 (节点, 用户) 对（50 节点 × 10,000 用户）由下一次 `traffic_flush` 在一个事务里
  写完（`users.u/d` 更新 + `stat_users` / `stat_nodes` upsert），用时 **4.0 s**。真实节点每 60 s 上报一次，一个 10 s 窗口里
  通常只有约六分之一的节点。
- 运行结束时服务端：75 个 goroutine，Go 堆 20 MiB；整个运行期间峰值 RSS 2.7 GiB（出现在登录场景，见下文“发现”）。
- 服务端日志里唯一的 WARN 是订阅访问日志缓冲区满（3 次，共丢弃 220,002 行，见下文）；没有 ERROR。

原始数据：[`results.json`](results.json)；工具生成的完整表格：[`results.md`](results.md)（每次运行都会重写，上表是这次运行的抄录）。

*Raw data in `results.json`, the generated table in `results.md` (rewritten by every run; the table above is copied from
the 2026-10-23 run). No request failed; the subscription scenarios stopped after 6.5 s once every token had used its
per-run share of the rate limit (about 86,000 samples each).*

## 测试环境 / Environment

| 项 | 值 |
| --- | --- |
| 机器 | Apple M5 Pro，18 核（6 Super + 12 Performance），48 GB 内存（`sysctl -n machdep.cpu.brand_string hw.memsize`） |
| 系统 | macOS 27.0.1，开始测量时负载平均值 4.56 / 10.23 / 9.06（其他进程没有停） |
| Go | go1.27.3（服务端与压测工具相同），`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`（`make build`） |
| 提交 | `9e613b5`，工作区有未提交改动（`-dirty`）：本次新增的压测工具与文档，以及另一个任务正在修改的 Xboard 导入器；两者都不在被测的请求路径上 |
| 日期 | 2026-10-23 |
| 数据库 | SQLite（纯 Go 驱动），临时数据目录；WAL、`synchronous=NORMAL`、`busy_timeout=5000`、`foreign_keys=1`、`_txlock=immediate`、8 个连接 |
| 服务端配置 | `server.addr` 为 ≥ 20000 的空闲端口（只监听 127.0.0.1），`server.trusted_proxies: ["127.0.0.1"]`，`log.level: info`（文本格式写文件），邮件驱动 `log`（默认） |
| 压测参数 | 64 个连接（登录 16），预热 3 s，计时 30 s，场景间隔 3 s，每次上报 1,000 个用户，每令牌最多 25 次订阅请求 |

*Apple M5 Pro (18 cores, 48 GB), macOS 27.0.1, Go 1.27.3, commit 9e613b5 plus uncommitted changes outside the measured
paths, 2026-10-23, SQLite in WAL mode with 8 connections, info logging to a file.*

## 发现 / Findings

1. **完整用户列表每次请求都重新 gzip（性能问题，未在本任务修复）。** 节点用户列表的 JSON body 已缓存（60 s），但 gzip
   中间件对每个请求重新压缩这 887.6 KiB：服务端 14.8 核只跑出 2,153 req/s，平均每个响应约 6.9 ms CPU。对照实验
   （同一台机器、各 10 s、只跑这个场景，`-only node_users_full -node-encoding …`）：`Accept-Encoding: gzip` 2,363 req/s、
   服务端 15.1 核；`identity` 9,079 req/s、服务端 2.2 核（每个响应约 0.24 ms CPU）。用 macOS `sample` 对同一源码的未剥离
   符号构建采样 5 s，服务端在 CPU 上的样本有 85.5 % 落在 `compress/flate` 与 `hash/crc32`。正常轮询走 ETag
   （304，85,902 req/s），所以只有用户列表变化、所有节点重新拉取时才付这个代价；可以在缓存 body 时一并缓存 gzip 后的字节。
2. **一波并发登录会让 RSS 长到约 2.7 GiB，并且不会马上归还。** 登录场景之前峰值 RSS 709 MiB，登录场景中到 2,732 MiB；
   运行结束时 Go 堆只有 20 MiB，而 runtime 向系统要的内存仍有 2.7 GiB。原因是 argon2id 每次校验分配 64 MiB，并发上限
   是 `max(2, NumCPU)`（这台机器是 18），再加上 GC 余量。2 核 VPS 上上限是 2 个（约 128 MiB 在用）；核多而内存小的机器
   可能需要更小的并发上限或设置 `GOMEMLIMIT`。属于设计取舍，记录在此。
3. **订阅访问日志在持续高频访问下会丢行。** 访问日志先进内存缓冲（上限 10,000 行），每 10 s 批量写库，满了就丢弃
   （从不阻塞请求），所以持续超过约 1,000 次 / 秒就会丢：这次运行丢了 220,002 行（3 条 WARN）。泄露检测按这些行统计
   IP 数，在这种洪峰下会少算；真实的订阅刷新频率比这低几个数量级。
4. **两种订阅格式的吞吐几乎一样（13,172 / 13,220 req/s），而机器没有跑满**（服务端 10.9–11.9 核 + 压测端约 1 核，共 18 核）。
   说明瓶颈在每个请求都要走的公共步骤，而不是正文生成。另一次 3 s 采样里服务端线程 56 % 的样本在等待；在 CPU 上的部分，
   Go 运行时（分配 / GC）占 33 %、gzip 13 %、SQLite 8 %、正则 6 %，没有单一热点。可能是每次请求按令牌查库（8 个连接的池）
   或串行的日志写入，本次没有定位，**不是已确认的问题**；对真实负载（一万用户每天刷新几次订阅）没有影响。
5. **一次写 500,000 个流量对的 flush 持有 SQLite 写锁 4.0 s。** 其他写操作在此期间排队（`busy_timeout` 为 5 s）。
   正常上报节奏下窗口要小得多；节点和用户再多一个数量级、而且同时上报时，需要关注 flush 时长逼近 `busy_timeout` 和
   10 s 的调度间隔。

对照实验和采样不在 `make bench` 里，数字来自同一天、同一台机器上单独的几次运行（采样时用的是不带 `-s -w` 的构建，
采样本身会拖慢服务端，所以那几次的吞吐不作为结果）。

*Findings: (1) the cached 887.6 KiB user list is re-gzipped on every request: 85.5 % of on-CPU samples in compress/flate;
identity encoding gives 9,079 req/s on 2.2 cores versus 2,363 on 15.1 (not fixed here); (2) a login burst grows RSS to
2.7 GiB (64 MiB per argon2id verification × NumCPU slots) and Go returns it lazily; (3) the subscription access log drops
rows above ~1,000 req/s sustained (220,002 in this run); (4) both subscription formats plateau at ~13.2k req/s with the
machine not saturated: a shared per-request step, not identified; (5) a 500,000-pair traffic flush holds the SQLite write
lock for 4.0 s. The A/B runs and the profile are separate diagnostic runs, not part of `make bench`.*

## 怎么复现 / Reproduce

```bash
make bench                                   # 构建、在 ≥ 20000 的空闲端口起一个临时实例（临时数据目录）、压测、写 docs/perf、停服务
make bench BENCH_ARGS="-d 10s"               # 参数原样传给压测工具：-c 连接数、-d 时长、-users、-nodes、-only sub_v2ray,login …
KEEP_LOG=1 make bench                        # 保留临时目录（服务端日志、数据库）便于排查
go run ./scripts/bench -render docs/perf/results.json   # 只根据 results.json 重画 results.md 和 benchmark.svg
./scripts/bench/run.sh -only node_users_full -d 10s -node-encoding identity -out /tmp/perf-identity
                                             # “发现 1”的对照实验：-out 指到别处，免得覆盖 docs/perf
```

对已经在运行的实例（例如测试机）：

```bash
BENCH_ADMIN_PASSWORD='…' go run ./scripts/bench -base http://127.0.0.1:23456 -admin-email admin@example.com -out /tmp/perf
```

工具会通过管理 API 幂等地补齐数据（已存在的分组 / 套餐 / 节点 / 用户直接复用），所以可以重复运行；但订阅令牌的
限流桶要 15 分钟才回满，**同一实例 15 分钟内重跑订阅场景会出现 429**（报告里记为错误，不会被当成成功）。登录场景要求
服务端信任压测机的 `X-Forwarded-For`（`server.trusted_proxies`），否则 30 次 / 分钟 / IP 的限流会让它全部 429。
只在测试实例上运行：它会创建 10,000 个 `bench-…@example.com` 账号和 50 个 `*.bench.example.com` 节点。

*`make bench` builds, starts a throwaway instance, runs everything and always stops the server. Against an existing
instance the seeding is idempotent, but the per-token subscription buckets need 15 minutes to refill, and login needs
`server.trusted_proxies` to cover the load generator. Use a test instance only.*

## 测了什么 / Scenarios

| id | 请求 | 经过的代码路径 |
| --- | --- | --- |
| `sub_v2ray` | `GET /api/v1/client/subscribe?token=…&flag=v2ray` | 令牌校验 → 按令牌查用户（SQLite，每次请求一条索引查询）→ 每令牌限流桶 → 节点列表缓存（5 s / 分组）→ 生成 50 条分享链接并 base64 → 订阅访问日志（内存缓冲，每 10 s 批量写库）→ gzip |
| `sub_clash_rules` | 同上，`flag=clash-meta&profile=standard` | 同上，但生成完整的 mihomo YAML：50 个代理、策略组、`standard` 模板的规则集（rule-providers）与规则；用户的规则配置缓存 1 min |
| `node_users_304` | `GET /api/v1/server/UniProxy/user` + `If-None-Match` | 节点令牌常量时间比较 → 节点解析（缓存）→ 用户列表 body 缓存（60 s）→ ETag 命中，返回 304 空 body。这是 XrayR / V2bX 每个拉取周期的常态 |
| `node_users_full` | 同上，不带 `If-None-Match` | 同上，但每次都发送完整的 10,000 用户 JSON（缓存的 body，由 gzip 中间件按请求压缩）。节点首次启动或用户变动后才会这样拉 |
| `node_push` | `POST /api/v1/server/UniProxy/push`，每次 1,000 个用户的流量 | 解析 JSON → 写入内存累加器（按 (节点, 用户) 聚合）；数据库写入由每 10 s 一次的 `traffic_flush` 在一个事务里批量完成 |
| `login` | `POST /api/v1/passport/login` | 每 IP 限流 → 按邮箱查用户 → argon2id 校验（64 MiB、3 轮、4 线程，最多 `NumCPU` 个并发）→ 写 `last_login_*` → 签发 JWT |
| `static_index` | `GET /`（`Accept-Encoding: br, gzip`） | 内嵌、构建时预渲染并预压缩的 `index.html.br`，`Cache-Control: no-cache` |
| `static_asset` | `GET /assets/<name>-<hash>.js` | 首页引用的最大一个 JS 分块的预压缩 `.br`，`immutable` 缓存一年 |

每个场景开始前先发一个探测请求校验返回内容（base64 能解码且至少有 50 条链接、YAML 含 `proxies` / `rule-providers`
和全部 50 个节点名、每个节点的用户列表都有 10,000 人且带 ETag、push 返回 `{"data":true}`、登录拿到 token、静态文件是 `br` 且资源带
`immutable`），内容不对就直接失败，不会把错误页面的速度当成结果。

*Every scenario first checks one response for correctness (decodable links, YAML with proxies and rule providers,
10,000 users plus an ETag, `{"data":true}`, a token, brotli with `immutable`), so an error page can never be measured as
a result.*

## 方法 / Method

- **数据**：通过管理 API 创建分组 `Bench`、套餐 `Bench 200 GB`（每用户 200 GB、3 台设备）、50 个节点（10 个地区 ×
  vless REALITY / vmess WS / trojan / shadowsocks 2022 / hysteria2 / anytls 轮换）、10,000 个用户（`POST /admin/users/generate`，
  每批 500，一年后到期），然后分页读回每个用户的 id、邮箱和订阅令牌。
- **负载模型**：闭环。每个连接读完上一个响应才发下一个请求，没有思考时间；因此吞吐是“饱和吞吐”，p50 / p95 / p99 是
  饱和状态下的延迟，不是固定到达率下的延迟。默认 64 个连接（登录 16 个），每个场景先 3 s 不计入的预热，再计时 30 s，
  场景之间空闲 3 s。延迟从发请求计到读完响应体，百分位数用最近秩法（nearest rank）对全部成功响应计算。
- **成功的定义**：只有预期状态码算成功（304 场景里 200 和 304 都算，另外报告 304 比例），其他状态码和传输错误都记为
  错误并写进 `results.json` 的 `statuses`；req/s 只按成功响应计算。
- **订阅限流**：服务端对每个订阅令牌限流 120 次 / 小时、突发 30。为了不触发限流，两个订阅场景各用一半用户（各 5,000
  个令牌），轮流使用，每个令牌在一次运行里最多用 25 次（含预热和探测）。令牌额度用完时场景提前结束，报告实际的测量
  时长（表里标 †）。
- **登录**：每个请求带一个不同的 `X-Forwarded-For` 地址（RFC 2544 压测网段 198.18.0.0/15），服务端配置
  `server.trusted_proxies: ["127.0.0.1"]`，相当于部署在本机 nginx 之后：每个模拟客户端仍受 30 次 / 分钟的限制。
- **流量写库**：push 场景结束后，工具先等累加器清空，再在两次 `traffic_flush` 之间推送一个完整窗口
  （50 节点 × 10,000 用户 = 500,000 个 (节点, 用户) 对），读取下一次 `traffic_flush` 的耗时（`/admin/system/status`）。
- **CPU 与内存**：`make bench` 把服务进程号传给工具，工具在每个计时窗口前后用 `ps` 读服务端 CPU 时间、每 250 ms 读一次
  RSS，用 `getrusage` 读自己的 CPU 时间，得出两边“平均占用几个核”和服务端峰值 RSS。`make bench` 还把服务端日志里的
  WARN / ERROR 按消息计数写进报告。
- **请求头**：订阅和节点请求带 `Accept-Encoding: gzip`（Go 写的客户端与 XrayR / V2bX 默认如此），静态文件带
  `br, gzip`（浏览器）。工具读取线上字节，不解压，`Body` 列是压缩后的平均大小。

*Closed loop, 3 s warm-up, 30 s measured, nearest-rank percentiles over successful responses, every other status
counted as an error. Subscription tokens are rotated so none exceeds 25 requests (the server allows a burst of 30); login
sends one X-Forwarded-For address per request through a trusted local proxy setting. A dedicated window of 500,000
(node, user) pairs times the traffic flush. Server CPU and RSS come from `ps`, the load generator's CPU from `getrusage`,
and the server log's WARN/ERROR lines are counted into the report.*

## 注意事项 / Caveats

- **同一台机器、回环网络**：压测工具和服务端共用同一台机器的 CPU（压测工具最多占约 7 个核，见表中“CPU”一列：304 和
  静态文件场景里服务端只用了 6–8 个核，瓶颈更可能在压测端）；没有 TLS、没有反向代理、没有真实网络延迟。生产环境前面还有 nginx / Caddy 和 TLS，数字会不同。
- **单进程、SQLite**：默认配置（WAL、`synchronous=NORMAL`、8 个连接）。PostgreSQL / MySQL 没有测。
- **日志照常输出**：服务端用默认的 `info` 级别，每个请求一行访问日志（写到临时目录的文件）。
- **机器上其他进程没有停**：运行开始时的负载平均值记录在 `results.json` 的 `machine.load_average_at_start`。
- **这是这台机器的数字**：换一台机器要重新跑；不要拿它和别的项目在别的机器上的数字直接比较。
- 真实负载远低于这些上限：一万用户的客户端通常几小时刷新一次订阅，50 个节点每 60 s 拉一次用户列表（绝大多数是 304）、
  推一次流量。

*Same machine over loopback (the load generator uses up to ~7 cores, so the 304 and static rows are likely limited by
the client, not the server), no TLS, no proxy, one process, SQLite, info
logging on, background processes not stopped. These are this machine's numbers; real traffic is far below them.*
