# Xboard → GG Bond 割接手册（terabix）/ Cutover runbook

本文是把现有 Xboard（Docker，legacy 分表 schema，MySQL 8.4）切换到 ggboard 的**完整命令序列**，按 2026-10-22 在生产库
一致性快照副本上的演练结果写成。文中不含任何域名、IP、口令、token：所有站点相关的值都是 `<占位符>`，在服务器上用
shell 变量代入；**不要把代入后的命令写进仓库或聊天记录**。

导入规则本身见 [IMPORT_XBOARD.md](IMPORT_XBOARD.md)，部署细节见 [DEPLOY.md](DEPLOY.md)。

## 0. 演练结论（2026-10-22）/ Rehearsal result

对生产库做 `mysqldump --single-transaction` 快照，恢复到本机一次性 MySQL 8.4 容器后运行导入器（`--dry-run`、真实导入、
`--force` 重导各一次，三次汇总完全一致）：

| 表 | 源 | 导入 | 跳过 | 备注 |
| --- | --- | --- | --- | --- |
| server_groups | 6 | 6 | 0 | |
| plans | 8 | 8 | 0 | 两条 0 价格周期告警（见 §1） |
| node_routes | 0 | 0 | 0 | |
| nodes | 17 | 17 | 0 | 全部 shadowsocks，无 id 重编号 |
| users | 151 | 151 | 0 | 151 个 bcrypt `$2y$` 哈希原样保留，0 个不可用 |
| invite_codes | 8 | 8 | 0 | |
| commission_logs | 25 | 25 | 0 | |
| coupons | 1 | 1 | 0 | |
| payments | 3 | 3 | 0 | 3 条 EPay，uuid 保留，全部启用 |
| tickets / messages | 53 / 117 | 53 / 117 | 0 | |
| notices / knowledge | 1 / 1 | 1 / 1 | 0 | |
| settings | 65 | 49 | 16 | 10 条不支持、6 条忽略（§1） |
| stats / stat_nodes / stat_users | 90 / 499 / 2652 | 同左 | 0 | 最近 90 天 |
| 不导入 | v2_order 2101、v2_log 35、v2_mail_log 1280、v2_server_log 20284、failed_jobs 3 | | | 按需求不迁移订单与日志 |

用脚本把源库与导入结果逐字段比对（分组、套餐与价格、节点与 `protocol_settings`、用户全部列、支付、优惠券、工单、公告、
知识库、设置、统计聚合）：**0 处差异**。随后 `ggboard serve` 验证：

- `/api/v1/server/UniProxy/config|user` 与 soga `/soga/api/v1/node|users`：用 Xboard 原 `server_token` 通过；错误 token 403；
  节点 15 的 `server_key` = `base64(md5(created_at)[0:16])`，与 Xboard 的 SS2022 PSK 算法一致；可用用户 43 人，
  与 Xboard 自己的规则（同组、未封禁、未过期、流量未用完）算出的 43 人一致。
- `/api/v1/client/subscribe` 与 `/s/<token>`：与按 Xboard 规则从源库推导的节点列表**集合、顺序、cipher、host、port、
  `serverKey:userKey` 口令**全部一致；`Subscription-Userinfo` 正确。
- 登录：给副本里的管理员换上一个已知口令的 `$2y$` bcrypt 哈希后 `/passport/login` 200，错口令 401，登录后哈希升级为
  argon2id 且 `password_version` 保持 0。**真实口令登录留到割接当天用自己的账号验证**（演练无法拿到真实口令）。
- 管理 API：用户 151、套餐 8、节点 17、分组 6、支付 3；设置（`app_url`、`subscribe_url`、`server_token`、`invite_commission`、
  `reset_traffic_method`、邮件、注册限制等）与 Xboard 一致。

## 1. 导入告警与处理 / Warnings and how to treat them

演练的 6 条告警全部有解释，没有导入器缺陷（`payments#18` 一条是演练中补上的“未使用的配置键”告警）：

| 告警 | 类型 | 处理 |
| --- | --- | --- |
| `[plans#3] three_year_price is 0: the three_yearly period stays purchasable for free` | Xboard 数据 | 若不是有意免费，割接前在 Xboard 后台把套餐 3 的三年价清空（NULL = 不售卖）；若有意，忽略 |
| `[plans#5] month_price is 0: the monthly period stays purchasable for free` | Xboard 数据 | 套餐 5 是免费/试用套餐（22 个用户），行为与 Xboard 相同，忽略 |
| `[settings] dropped (not supported by ggboard): change_order_event_id, email_gmail_limit_enable, email_template, force_https, frontend_theme_sidebar, new_order_event_id, renew_order_event_id, safe_mode_enable, telegram_bot_enable, telegram_bot_token` | 预期 | HTTPS 与“安全模式”由 nginx-proxy-manager 承担（§5）；Telegram 设置不导入，割接后在后台重新填写（§5.1） |
| `[settings] ignored settings: server_license, server_log_enable, server_v2ray_domain, server_v2ray_protocol, theme_v2board, theme_xboard` | 预期 | 主题与旧版 v2ray 字段，无需处理 |
| `[coupons] limit_use_with_user is kept, but per-user usage history is lost with the orders` | 预期 | 唯一一张优惠券已过期，忽略 |
| `[payments#18] config keys not used by ggboard dropped: product_name` | 预期 | EPay 不用该键，忽略 |
| 以 `(--skip-banned)` 结尾的告警（`invite_user_id of N users pointed to a banned user → NULL`、`user_id 0`、`last_reply_user_id … → NULL`；`[users#<id>] banned admin / staff member not imported`） | 预期（加了 `--skip-banned`，演练时未加） | 按 IMPORT_XBOARD.md §3.5 处理；出现 `banned admin` 先确认还有未封禁的管理员，出现 `no admin was imported` 就在导入后执行 `gg run --rm ggboard admin create --email <管理员邮箱>` |

正式割接时如果出现**不在此表中**的告警，先停下来读 [IMPORT_XBOARD.md](IMPORT_XBOARD.md) 对应章节再决定。

## 2. 割接前准备（任意时间，不停机）/ Preparation

### 2.1 Xboard 侧

1. 订单：割接后 Xboard 的订单不会迁移。选一个没有待支付订单的时段；后台“订单”里把待支付订单关掉。
2. 佣金：`commission_auto_check_enable=0`，待确认佣金不会跟过去。后台核对并确认完佣金再割接。
3. 套餐 3 的三年价（§1）。
4. Telegram：ggboard 有自己的机器人（后台“系统设置 → Telegram”），但 Xboard 的 `telegram_*` 设置**不导入**。
   割接后**继续用同一个机器人**（同一个 BotFather token）：用户的 `telegram_id` 原样导入，同一个机器人才能继续给已绑定的
   用户发消息（新机器人无法主动联系从未与它对话过的用户）。现在先记下 Xboard 后台的 `telegram_bot_token` 和
   `telegram_discuss_link`，以及哪些管理员绑定过 Telegram（Xboard 把管理员通知发给绑定了 Telegram 的管理员账号，
   ggboard 改为发往 `telegram_admin_chat_ids` 列出的会话）。webhook 不必提前删除：Xboard 的回调地址
   `/api/v1/guest/telegram/webhook?access_token=…` 在 ggboard 上是 404，§5.1 点“设置 Webhook”时会直接替换成 ggboard 的地址。

### 2.2 服务器上准备 ggboard（不影响 Xboard）

ggboard 以 Docker Compose 运行在同一台机器上，加入 nginx-proxy-manager 所在的 Docker 网络，由 NPM 以容器名转发；
数据库用默认 SQLite（151 个用户足够；备份就是复制一个文件，见 DEPLOY.md §7）。

```bash
# 变量：只在当前 shell 里存在，不要写进文件
export XB_MYSQL='<1Panel 的 MySQL 容器名>'                 # docker ps 里 mysql:8.4 那个
export XB_NET=$(docker inspect "$XB_MYSQL" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')
export NPM_NET=$(docker inspect nginx-proxy-manager --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')
export NPM_CIDR=$(docker network inspect "$NPM_NET" --format '{{(index .IPAM.Config 0).Subnet}}')
export XB_DB_PASSWORD=$(docker exec xboard sh -c 'grep ^DB_PASSWORD= /www/.env | cut -d= -f2-' | tr -d '\r"'"'"'')
export XB_DB_PASSWORD_URL=$(python3 -c 'import os,urllib.parse;print(urllib.parse.quote(os.environ["XB_DB_PASSWORD"],safe=""))')
mkdir -p -m 700 /root/backup/xboard

git clone https://github.com/linusxiong/ggboard.git /opt/ggboard && cd /opt/ggboard
cp .env.example .env
# .env 里至少设置：
#   GGBOARD_JWT_SECRET=<openssl rand -hex 32 的输出；占位符或短于 32 字节会拒绝启动；不设则自动生成到数据卷的 jwt.secret>
#   GGBOARD_SERVER_TRUSTED_PROXIES=<$NPM_CIDR 的值>
#   TZ=Asia/Shanghai
cat > deploy/docker-compose.local.yml <<EOF
services:
  ggboard:
    networks:
      - default
      - npm
networks:
  npm:
    external: true
    name: ${NPM_NET}
EOF
alias gg='docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local.yml -p ggboard'
gg build
gg run --rm ggboard migrate          # 建空库；不要执行 admin create（管理员随导入一起过来）
```

### 2.3 演练（推荐；只读生产库）

导入器对源库只开 `READ ONLY` 一致性快照事务，`--dry-run` 读写全部执行后回滚，可以直接对生产库试跑：

```bash
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned --dry-run
```

告警应与 §1 一致。想在本机离线演练，按 §3.2 取一份 dump，恢复到 `docker run -d -p 127.0.0.1:33068:3306 mysql:8.4` 的临时容器，
DSN 指向 `127.0.0.1:33068` 即可；用完 `docker rm -f` 并删除 dump。

## 3. 割接（维护窗口约 20 分钟）/ Cutover

节点端（XrayR/soga）在面板不可用期间继续用缓存的用户列表工作，只有登录、订阅刷新、购买受影响。

### 3.1 记录旧面板的输出（切换前，供 §4 对比）

用自己账号的订阅 token 与 `server_token`（后台“系统设置 → 节点”）：

```bash
cd /root/backup/xboard
curl -s -A 'v2rayN/7.0' "https://<sub-domain-1>/api/v1/client/subscribe?token=<token>" > sub-before.txt
curl -s "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=<server_token>" > cfg15-before.json
curl -s "https://<panel-domain>/api/v1/server/UniProxy/user?node_id=15&node_type=shadowsocks&token=<server_token>" > users15-before.json
```

同时在 NPM 里记下三个 Proxy Host（面板域名、两个订阅域名）当前的 Forward Hostname/Port，回滚要用。

### 3.2 停 Xboard，取最终快照

```bash
docker stop xboard                                   # 从此刻起没有新写入；容器与卷保留
docker exec -e MYSQL_PWD="$XB_DB_PASSWORD" "$XB_MYSQL" mysqldump --protocol=tcp -h 127.0.0.1 -u xboard \
  --single-transaction --quick --triggers --no-tablespaces --set-gtid-purged=OFF \
  --default-character-set=utf8mb4 --hex-blob xboard | gzip -6 > /root/backup/xboard/xboard-final-$(date +%F).sql.gz
gzip -t /root/backup/xboard/xboard-final-$(date +%F).sql.gz
```

这份 dump 是回滚点，也是以后重导的依据；MySQL 容器本身不停，Xboard 的库一个字节都不会被改。

### 3.3 导入

```bash
cd /opt/ggboard
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned --dry-run
# 告警与 §1 一致后，去掉 --dry-run 正式导入：
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned
```

`--skip-banned`：被封禁（`banned=1`）的用户连同只属于他们的邀请码、佣金记录、工单、流量统计都不导入，其余行里指向他们的引用按 [IMPORT_XBOARD.md](IMPORT_XBOARD.md) §3.5 置 NULL / 0；汇总的 `users` 行出现 `skipped banned N`，dry-run 与正式导入要一致。注意封禁状态不会带过来：这些邮箱在 ggboard 里可以重新注册。

如果需要重来（例如先修了 Xboard 里的数据再导），加 `--force`：同一事务里先清空导入表再导（`--skip-banned` 要一起带上，否则封禁用户会被导回来）。

### 3.4 启动 ggboard，切换反向代理

```bash
gg up -d
gg logs --tail 20 ggboard                           # 看到 "ggboard listening"
docker run --rm --network "$NPM_NET" curlimages/curl -s http://ggboard:8080/healthz   # {"data":{"status":"ok",...}}
```

nginx-proxy-manager（Hosts → Proxy Hosts）里依次编辑面板域名和两个订阅域名：

- Forward Hostname / IP：`ggboard`，Forward Port：`8080`，Scheme：`http`；
- SSL 页签不动（证书沿用），Force SSL 保持开启（代替 Xboard 的 `force_https`）；
- Advanced 加上 `client_max_body_size 8m;`（节点上报流量的请求体上限）。

Xboard 的 `safe_mode_enable`（只允许从 `app_url` 的域名访问）在这里等价于：NPM 只有这三个域名指向 ggboard，没有默认站点
转发到它。需要限制 `/admin` 的话在 NPM 的 Custom Locations 里按来源 IP 限制。

### 3.5 节点端

`server_token` 原样导入，XrayR 的 `ApiKey`、soga 的 `webapi_key` 不用改；节点 id 没有重编号，`NodeID`/`node_id` 不用改。
如果 XrayR 的 `ApiHost` / soga 的 `webapi_url` 就是面板域名，节点端**什么都不用动**——域名在 NPM 层已经指向 ggboard。
只有当某个节点配置写的是别的地址（IP、旧域名）时才改成 `https://<panel-domain>`（soga 推荐 `https://<panel-domain>/soga/`，
根路径也兼容）并重启 `XrayR`/`soga`。

## 4. 验证 / Verification

```bash
cd /root/backup/xboard
curl -s -A 'v2rayN/7.0' "https://<sub-domain-1>/api/v1/client/subscribe?token=<token>" > sub-after.txt
curl -sI -A 'v2rayN/7.0' "https://<sub-domain-2>/api/v1/client/subscribe?token=<token>" | grep -iE '^(HTTP|subscription-userinfo)'
curl -s "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=<server_token>" > cfg15-after.json
curl -s "https://<panel-domain>/api/v1/server/UniProxy/user?node_id=15&node_type=shadowsocks&token=<server_token>" > users15-after.json
curl -s "https://<panel-domain>/soga/api/v1/node" -H "API-KEY: <server_token>" -H "NODE-ID: 15" -H "NODE-TYPE: ss" | head -c 200; echo
curl -s -o /dev/null -w '%{http_code}\n' "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=wrong"   # 403

# 节点配置与用户集合必须一致
diff <(jq -S '{server_key,server_port,cipher}' cfg15-before.json) <(jq -S '{server_key,server_port,cipher}' cfg15-after.json) && echo config-ok
diff <(jq -S '[.users[].uuid]|sort' users15-before.json) <(jq -S '[.users[].uuid]|sort' users15-after.json) && echo users-ok

# 订阅：两边的 ss:// 链接编码细节不同，按 名称|cipher|host|port|口令 归一化后比较
python3 - sub-before.txt sub-after.txt <<'EOF'
import base64, sys, urllib.parse
def rows(path):
    raw = open(path, 'rb').read().strip()
    out = []
    for line in base64.b64decode(raw + b'=' * (-len(raw) % 4)).decode().splitlines():
        if not line.startswith('ss://'): continue
        body, _, frag = line[5:].partition('#')
        userinfo, _, hostport = body.rpartition('@')
        userinfo = base64.urlsafe_b64decode(userinfo + '=' * (-len(userinfo) % 4)).decode()
        host, _, port = hostport.split('/')[0].split('?')[0].rpartition(':')
        out.append('|'.join([urllib.parse.unquote(frag), userinfo.split(':', 1)[0], host, port, userinfo.split(':', 1)[1]]))
    return out
a, b = rows(sys.argv[1]), rows(sys.argv[2])
print('before', len(a), 'after', len(b), 'same set:', set(a) == set(b), 'same order:', a == b)
for x in sorted(set(a) ^ set(b)): print('  differs:', x.split('|')[0])
EOF
```

然后在浏览器里：

1. 用自己的账号和**原口令**登录 `https://<panel-domain>`（登录后哈希自动升级为 argon2id）；再用“忘记密码”走一遍邮件。
2. 后台 → 节点：17 个节点，几分钟内在线数应与 Xboard 时相同（XrayR/soga 每 120 s 拉取一次）。
3. 后台 → 支付：3 条 EPay 均启用；下一笔小额订单走一次 EPay 回调（回调 URL 随订单发送，GM Pay/epusdt 侧无需改）。
4. 后台 → 设置 → 邮件：发送测试邮件（SMTP 设置已导入，包括 `SSL`）。
5. 后台 → 工单：53 张工单（加了 `--skip-banned` 时减去汇总 `tickets` 行的 `skipped banned` 数）、回复状态正确；用户端打开知识库与公告。
6. 任一客户端刷新订阅、连通节点。

任一步失败且短时间修不好 → §6 回滚。

## 5. 割接后 / After cutover

- Xboard 容器保持 **停止但不删除**（`docker ps -a` 里 Exited），MySQL 里的 `xboard` 库原样保留；至少观察两周再决定清理。
- 备份：`/root/backup/xboard/xboard-final-*.sql.gz` 与 ggboard 的 SQLite（DEPLOY.md §7）异地各存一份，dump 含用户哈希与支付密钥，权限 600。
- 订单历史没有迁移：客服需要时查 Xboard 的 dump 或临时 `docker start xboard` 只读查看（此时 NPM 仍指向 ggboard，不影响用户）。
- `invite_commission=0`、`commission_distribution_enable=0`：佣金设置与 Xboard 一致，不需要动。
- `safe_mode_enable`、`force_https` 已由 NPM 承担；`email_template`、主题设置不迁移（ggboard 自带模板与主题）。

### 5.1 重新配置 Telegram

导入器丢弃 Xboard 的全部 `telegram_*` 设置，按下面的顺序在后台“系统设置 → Telegram”重新填写（详见
[ADMIN_GUIDE.md](ADMIN_GUIDE.md) 的 Telegram 一节）：

1. **Bot Token**（`telegram_bot_token`）：填 §2.1 记下的 Xboard 机器人 token，保存。
2. **设置 Webhook**：点一下即可，ggboard 用 `<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>` 调用
   Telegram 的 `setWebhook`，替换掉 Xboard 的旧地址（`app_url` 必须是 HTTPS，端口 443/80/88/8443）。
3. **管理员 Chat ID**（`telegram_admin_chat_ids`）：管理员在与机器人的私聊（或要接收通知的群组）里发送 `/id`，把回复的
   Chat ID 填进来；也可以直接用后台用户详情里该管理员账号导入过来的 `telegram_id`。
4. **通知事件**（`telegram_events`）：默认推送订单支付、管理员登录、新工单、提现申请和订阅泄露；按需增减。
5. **交流群链接**（`telegram_discuss_link`）：填 Xboard 的同名设置。**用户通知**（`telegram_user_notify`）默认开启。
6. 点“发送测试消息”，每个管理员 Chat ID 都应收到一条测试消息。

已绑定的用户无需重新绑定（`telegram_id` 已导入），只是用户中心不显示 Telegram 用户名（Xboard 不保存它，
用户解绑后重新绑定即可补上）。Xboard 的机器人命令（`/bind`、`/traffic`、`/getlatesturl`、回复工单）在 ggboard 中
不存在：ggboard 用一次性绑定码（个人中心 → 账户 → Telegram，`/start <code>`）绑定，机器人只回答 `/start`、`/unbind`、
`/id` 和 `/help`。

## 6. 回滚 / Rollback

```bash
cd /opt/ggboard && gg stop
# NPM：把三个 Proxy Host 的 Forward Hostname/Port 改回 §3.1 记录的值
docker start xboard
```

Xboard 的库从未被改动，回滚后状态等于停机那一刻。割接后在 ggboard 里产生的订单、工单、注册不会回到 Xboard
（ggboard 没有反向导出），所以回滚决定应在数小时内做出；需要保留的数据只能人工补录。

## 7. 已知差异 / Known differences

| 项 | Xboard | ggboard |
| --- | --- | --- |
| 订单 / 支付历史 | 2101 条 | 不迁移（需求） |
| Telegram 机器人 | 启用（`/bind` 等命令，通知发给绑定了 Telegram 的管理员） | 有，但设置需重新填写（§5.1）；`telegram_id` 原样导入，同一机器人可继续推送；管理员通知发往 `telegram_admin_chat_ids`；绑定改用一次性绑定码 |
| 佣金首购判断（`commission_first_time_enable`） | 看历史订单 | 当前为关闭，无影响 |
| 升级套餐折抵（`surplus_enable`） | 按历史订单算 | 导入用户首次升级无折抵（IMPORT_XBOARD.md §8.4） |
| 主题 / 邮件模板 | `theme_*`、`email_template` | 不迁移 |
| 订阅短链 | 无 | `https://<sub-domain>/s/<token>` 同样可用 |
| 时区 | `Asia/Shanghai` | 容器设 `TZ=Asia/Shanghai`，流量重置日期按同一时区 |
