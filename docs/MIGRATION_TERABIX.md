# Xboard → GG Bond cutover runbook (terabix)

**English** · [简体中文](MIGRATION_TERABIX.zh-CN.md)

This is the **complete command sequence** for switching an existing Xboard (Docker, legacy per-type table schema,
MySQL 8.4) over to ggboard. It is written from the rehearsal on 2026-10-22 against a consistent snapshot copy of the
production database. It contains no domains, IPs, passwords or tokens: every site-specific value is a `<placeholder>`
that you fill in on the server with shell variables. **Do not put the filled-in commands into the repository or a chat
log.**

The import rules themselves are in [IMPORT_XBOARD.md](IMPORT_XBOARD.md); deployment details are in [DEPLOY.md](DEPLOY.md).

## 0. Rehearsal result (2026-10-22)

A `mysqldump --single-transaction` snapshot of the production database was restored into a throwaway MySQL 8.4
container on the local machine, and the importer was run against it (once each with `--dry-run`, as a real import, and
as a `--force` re-import; the three summaries were identical):

| Table | Source | Imported | Skipped | Notes |
| --- | --- | --- | --- | --- |
| server_groups | 6 | 6 | 0 | |
| plans | 8 | 8 | 0 | Two zero-price period warnings (see §1) |
| node_routes | 0 | 0 | 0 | |
| nodes | 17 | 17 | 0 | All shadowsocks, no id renumbering |
| users | 151 | 151 | 0 | 151 bcrypt `$2y$` hashes kept as is, 0 unusable |
| invite_codes | 8 | 8 | 0 | |
| commission_logs | 25 | 25 | 0 | |
| coupons | 1 | 1 | 0 | |
| payments | 3 | 3 | 0 | 3 EPay, uuid kept, all enabled |
| tickets / messages | 53 / 117 | 53 / 117 | 0 | |
| notices / knowledge | 1 / 1 | 1 / 1 | 0 | |
| settings | 65 | 49 | 16 | 10 unsupported, 6 ignored (§1) |
| stats / stat_nodes / stat_users | 90 / 499 / 2652 | Same as source | 0 | Last 90 days |
| Not imported | v2_order 2101, v2_log 35, v2_mail_log 1280, v2_server_log 20284, failed_jobs 3 | | | Orders and logs are not migrated, as required |

A script compared the source database with the import result field by field (groups, plans and prices, nodes and
`protocol_settings`, every user column, payments, coupons, tickets, notices, knowledge base, settings, aggregated
stats): **0 differences**. Then `ggboard serve` was checked:

- `/api/v1/server/UniProxy/config|user` and soga `/soga/api/v1/node|users`: pass with Xboard's original `server_token`;
  a wrong token gets 403. Node 15's `server_key` = `base64(md5(created_at)[0:16])`, the same as Xboard's SS2022 PSK
  algorithm. 43 users are available, the same 43 that Xboard's own rules give (same group, not banned, not expired,
  traffic not used up).
- `/api/v1/client/subscribe` and `/s/<token>`: compared with the node list derived from the source database by Xboard's
  rules, the **set, order, cipher, host, port and `serverKey:userKey` password** all match; `Subscription-Userinfo` is
  correct.
- Sign-in: after the admin in the copy was given a `$2y$` bcrypt hash of a known password, `/passport/login` returned
  200 and a wrong password 401; after sign-in the hash was upgraded to argon2id and `password_version` stayed 0.
  **Sign-in with a real password is left for cutover day, with your own account** (the rehearsal has no access to real
  passwords).
- Admin API: 151 users, 8 plans, 17 nodes, 6 groups, 3 payments; the settings (`app_url`, `subscribe_url`,
  `server_token`, `invite_commission`, `reset_traffic_method`, mail, registration limits and so on) match Xboard.

## 1. Warnings and how to treat them

All 6 warnings in the rehearsal are explained, and none of them is an importer bug (the `payments#18` one is the
"unused config keys" warning that was added during the rehearsal):

| Warning | Type | Action |
| --- | --- | --- |
| `[plans#3] three_year_price is 0: the three_yearly period stays purchasable for free` | Xboard data | If it is not meant to be free, clear plan 3's three-year price in the Xboard admin before the cutover (NULL = not for sale); if it is, ignore it |
| `[plans#5] month_price is 0: the monthly period stays purchasable for free` | Xboard data | Plan 5 is the free / trial plan (22 users) and behaves as in Xboard; ignore |
| `[settings] dropped (not supported by ggboard): change_order_event_id, email_gmail_limit_enable, email_template, force_https, frontend_theme_sidebar, new_order_event_id, renew_order_event_id, safe_mode_enable, telegram_bot_enable, telegram_bot_token` | Expected | HTTPS and "safe mode" are handled by nginx-proxy-manager (§5); the Telegram settings are not imported, so fill them in again in the admin after the cutover (§5.1) |
| `[settings] ignored settings: server_license, server_log_enable, server_v2ray_domain, server_v2ray_protocol, theme_v2board, theme_xboard` | Expected | Themes and old v2ray fields; nothing to do |
| `[coupons] limit_use_with_user is kept, but per-user usage history is lost with the orders` | Expected | The only coupon has expired; ignore |
| `[payments#18] config keys not used by ggboard dropped: product_name` | Expected | EPay does not use that key; ignore |
| Warnings ending in `(--skip-banned)` (`invite_user_id of N users pointed to a banned user → NULL`, `user_id 0`, `last_reply_user_id … → NULL`; `[users#<id>] banned admin / staff member not imported`) | Expected (with `--skip-banned`, which the rehearsal did not use) | Handle as in IMPORT_XBOARD.md §3.5. On `banned admin`, first make sure an unbanned admin is left; on `no admin was imported`, run `gg run --rm ggboard admin create --email <admin-email>` after the import |

If the real cutover shows a warning that is **not in this table**, stop and read the matching section of
[IMPORT_XBOARD.md](IMPORT_XBOARD.md) before you decide.

**Cutover log (2026-10-24)**: done by this runbook with a 13-second maintenance window (`docker stop xboard` → snapshot
→ import → `gg up -d`) and `--skip-banned`: users 151 → 69 imported / 82 skipped, admins 2 → 1, tickets 47/101, every
other table as in §0; all 8 warnings were in the table above. The subscription, node 15's config and user list
(43 users), `/s/<token>`, the 403 and the sign-in page were verified to match over the public internet. The only
surprise: Xboard's `app_url` setting was an **old domain that had been out of use for a long time** (its DNS no longer
existed). The importer carried it over as is, and both the importer's checklist and the curl commands in §3.1/§4 go by
it, so it looked as if "the panel domain is unreachable". From now on, during the §2.3 rehearsal, first check `app_url`
against the panel domain actually set up in NPM. If they differ, change ggboard's `app_url` after the import (admin →
Settings → Site, or edit the `settings` table and run `gg restart`), then do §4.

## 2. Preparation (any time, no downtime)

### 2.1 Xboard side

1. Orders: Xboard's orders are not migrated in the cutover. Pick a time with no unpaid orders, and close the unpaid
   orders under "Orders" in the admin.
2. Commission: `commission_auto_check_enable=0`, so commission waiting for confirmation does not carry over. Review and
   confirm the commission in the admin before the cutover.
3. Plan 3's three-year price (§1).
4. Telegram: ggboard has its own bot (admin "Settings → Telegram"), but Xboard's `telegram_*` settings are **not
   imported**. After the cutover, **keep using the same bot** (the same BotFather token): users' `telegram_id` is
   imported as is, and only the same bot can keep messaging users who are already bound (a new bot cannot start a
   conversation with users who have never talked to it). Write down `telegram_bot_token` and `telegram_discuss_link`
   from the Xboard admin now, and which admins have bound Telegram (Xboard sends admin notifications to the admin
   accounts that have bound Telegram; ggboard sends them to the chats listed in `telegram_admin_chat_ids` instead). You
   do not need to delete the webhook beforehand: Xboard's callback URL
   `/api/v1/guest/telegram/webhook?access_token=…` is a 404 on ggboard, and clicking "Set webhook" in §5.1 replaces it
   with ggboard's URL.

### 2.2 Prepare ggboard on the server (Xboard is not affected)

ggboard runs with Docker Compose on the same machine and joins the Docker network nginx-proxy-manager is on; NPM
forwards to it by container name. The database is the default SQLite (enough for 151 users; a backup is a copy of one
file, see DEPLOY.md §7).

```bash
# Variables: they exist only in the current shell; do not write them into files
export XB_MYSQL='<the 1Panel MySQL container name>'                 # the mysql:8.4 one in docker ps
export XB_NET=$(docker inspect "$XB_MYSQL" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')
export NPM_NET=$(docker inspect nginx-proxy-manager --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')
export NPM_CIDR=$(docker network inspect "$NPM_NET" --format '{{(index .IPAM.Config 0).Subnet}}')
export XB_DB_PASSWORD=$(docker exec xboard sh -c 'grep ^DB_PASSWORD= /www/.env | cut -d= -f2-' | tr -d '\r"'"'"'')
export XB_DB_PASSWORD_URL=$(python3 -c 'import os,urllib.parse;print(urllib.parse.quote(os.environ["XB_DB_PASSWORD"],safe=""))')
mkdir -p -m 700 /root/backup/xboard

git clone https://github.com/linusxiong/ggboard.git /opt/ggboard && cd /opt/ggboard
cp .env.example .env
# In .env, set at least:
#   GGBOARD_JWT_SECRET=<output of openssl rand -hex 32; a placeholder or a value shorter than 32 bytes is refused at startup; if unset, one is generated into jwt.secret on the data volume>
#   GGBOARD_SERVER_TRUSTED_PROXIES=<the value of $NPM_CIDR>
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
gg run --rm ggboard migrate          # creates the empty database; do not run admin create (the admins come with the import)
```

### 2.3 Rehearsal (recommended; read-only on the production database)

The importer opens only a `READ ONLY` consistent-snapshot transaction on the source database, and `--dry-run` runs all
reads and writes and then rolls back, so you can try it directly against the production database:

```bash
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned --dry-run
```

The warnings should match §1. To rehearse offline on the local machine, take a dump as in §3.2, restore it into a
temporary container started with `docker run -d -p 127.0.0.1:33068:3306 mysql:8.4`, and point the DSN at
`127.0.0.1:33068`. Afterwards, `docker rm -f` the container and delete the dump.

## 3. Cutover (maintenance window of about 20 minutes)

While the panel is unavailable, the nodes (XrayR/soga) keep working with their cached user lists; only sign-in,
subscription refreshes and purchases are affected.

### 3.1 Record the old panel's output (before the switch, for comparison in §4)

Use your own account's subscription token and the `server_token` (admin "System settings → Node"):

```bash
cd /root/backup/xboard
curl -s -A 'v2rayN/7.0' "https://<sub-domain-1>/api/v1/client/subscribe?token=<token>" > sub-before.txt
curl -s "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=<server_token>" > cfg15-before.json
curl -s "https://<panel-domain>/api/v1/server/UniProxy/user?node_id=15&node_type=shadowsocks&token=<server_token>" > users15-before.json
```

Also write down in NPM the current Forward Hostname/Port of the three Proxy Hosts (the panel domain and the two
subscription domains); a rollback needs them.

### 3.2 Stop Xboard and take the final snapshot

```bash
docker stop xboard                                   # no new writes from this moment on; the container and volumes are kept
docker exec -e MYSQL_PWD="$XB_DB_PASSWORD" "$XB_MYSQL" mysqldump --protocol=tcp -h 127.0.0.1 -u xboard \
  --single-transaction --quick --triggers --no-tablespaces --set-gtid-purged=OFF \
  --default-character-set=utf8mb4 --hex-blob xboard | gzip -6 > /root/backup/xboard/xboard-final-$(date +%F).sql.gz
gzip -t /root/backup/xboard/xboard-final-$(date +%F).sql.gz
```

This dump is the rollback point and the basis for any later re-import. The MySQL container itself keeps running, and
not a single byte of Xboard's database is changed.

### 3.3 Import

```bash
cd /opt/ggboard
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned --dry-run
# Once the warnings match §1, drop --dry-run for the real import:
docker run --rm --network "$XB_NET" -v ggboard_ggboard-data:/data -e GGBOARD_DATA_DIR=/data \
  ggboard:latest import xboard \
  --dsn "mysql://xboard:${XB_DB_PASSWORD_URL}@${XB_MYSQL}:3306/xboard" --with-stats --skip-banned
```

`--skip-banned`: banned users (`banned=1`) are not imported, together with the invite codes, commission logs, tickets
and traffic stats that belong only to them; references to them in the remaining rows are set to NULL / 0 as described
in [IMPORT_XBOARD.md](IMPORT_XBOARD.md) §3.5. The `users` row of the summary shows `skipped banned N`, and the dry run
and the real import must agree. Note that the ban does not carry over: these email addresses can sign up again on
ggboard.

If you need to start over (for example after fixing data in Xboard first), add `--force`: it empties the imported
tables and imports again in the same transaction (pass `--skip-banned` along with it, or the banned users are imported
back).

### 3.4 Start ggboard and switch the reverse proxy

```bash
gg up -d
gg logs --tail 20 ggboard                           # look for "ggboard listening"
docker run --rm --network "$NPM_NET" curlimages/curl -s http://ggboard:8080/healthz   # {"data":{"status":"ok",...}}
```

In nginx-proxy-manager (Hosts → Proxy Hosts), edit the panel domain and the two subscription domains one after another:

- Forward Hostname / IP: `ggboard`, Forward Port: `8080`, Scheme: `http`;
- leave the SSL tab alone (the certificates stay), and keep Force SSL on (it takes the place of Xboard's
  `force_https`);
- under Advanced, add `client_max_body_size 8m;` (the request body limit for nodes reporting traffic).

Xboard's `safe_mode_enable` (allow access only through the `app_url` domain) is equivalent here to: in NPM only these
three domains point to ggboard, and no default site forwards to it. To restrict `/admin`, limit it by source IP in NPM's
Custom Locations.

### 3.5 Node side

`server_token` is imported as is, so XrayR's `ApiKey` and soga's `webapi_key` stay the same; node ids were not
renumbered, so `NodeID`/`node_id` stay the same. If XrayR's `ApiHost` / soga's `webapi_url` is the panel domain,
**nothing changes on the nodes**: the domain already points to ggboard at the NPM layer. Only when a node config uses
another address (an IP, an old domain) change it to `https://<panel-domain>` (for soga, `https://<panel-domain>/soga/`
is recommended; the root path works too) and restart `XrayR`/`soga`.

## 4. Verification

```bash
cd /root/backup/xboard
curl -s -A 'v2rayN/7.0' "https://<sub-domain-1>/api/v1/client/subscribe?token=<token>" > sub-after.txt
curl -sI -A 'v2rayN/7.0' "https://<sub-domain-2>/api/v1/client/subscribe?token=<token>" | grep -iE '^(HTTP|subscription-userinfo)'
curl -s "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=<server_token>" > cfg15-after.json
curl -s "https://<panel-domain>/api/v1/server/UniProxy/user?node_id=15&node_type=shadowsocks&token=<server_token>" > users15-after.json
curl -s "https://<panel-domain>/soga/api/v1/node" -H "API-KEY: <server_token>" -H "NODE-ID: 15" -H "NODE-TYPE: ss" | head -c 200; echo
curl -s -o /dev/null -w '%{http_code}\n' "https://<panel-domain>/api/v1/server/UniProxy/config?node_id=15&node_type=shadowsocks&token=wrong"   # 403

# The node config and the user set must match
diff <(jq -S '{server_key,server_port,cipher}' cfg15-before.json) <(jq -S '{server_key,server_port,cipher}' cfg15-after.json) && echo config-ok
diff <(jq -S '[.users[].uuid]|sort' users15-before.json) <(jq -S '[.users[].uuid]|sort' users15-after.json) && echo users-ok

# Subscription: the two sides encode ss:// links differently, so normalise to name|cipher|host|port|password and compare
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

Then in the browser:

1. Sign in to `https://<panel-domain>` with your own account and its **original password** (the hash is upgraded to
   argon2id automatically after sign-in); then go through "Forgot password?" once to test the email.
2. Admin → Nodes: 17 nodes; within a few minutes the online counts should be the same as under Xboard (XrayR/soga pull
   every 120 s).
3. Admin → Payments: all 3 EPay methods are enabled; let the next small order go through one EPay callback (the callback
   URL is sent with each order, so nothing changes on the GM Pay/epusdt side).
4. Admin → Settings → Email: send a test email (the SMTP settings were imported, `SSL` included).
5. Admin → Tickets: 53 tickets (with `--skip-banned`, minus the `skipped banned` count in the summary's `tickets` row),
   with the correct reply status; on the user side, open Knowledge and Notices.
6. Refresh the subscription in any client and connect to a node.

If any step fails and cannot be fixed quickly → roll back (§6).

## 5. After cutover

- Keep the Xboard container **stopped but not deleted** (Exited in `docker ps -a`), and leave the `xboard` database in
  MySQL as it is; watch for at least two weeks before deciding on any cleanup.
- Backups: store one off-site copy each of `/root/backup/xboard/xboard-final-*.sql.gz` and ggboard's SQLite
  (DEPLOY.md §7). The dump contains user hashes and payment secrets; permissions 600.
- Order history was not migrated: when support needs it, look it up in the Xboard dump, or run `docker start xboard`
  for a while to look read-only (NPM still points to ggboard then, so users are not affected).
- `invite_commission=0`, `commission_distribution_enable=0`: the commission settings match Xboard; nothing to change.
- `safe_mode_enable` and `force_https` are now handled by NPM; `email_template` and the theme settings are not migrated
  (ggboard comes with its own templates and theme).

### 5.1 Reconfigure Telegram

The importer drops all of Xboard's `telegram_*` settings. Fill them in again in the admin under "Settings → Telegram",
in this order (details in the Telegram section of [ADMIN_GUIDE.md](ADMIN_GUIDE.md)):

1. **Bot token** (`telegram_bot_token`): enter the Xboard bot token you wrote down in §2.1, and save.
2. **Set webhook**: just click it. ggboard calls Telegram's `setWebhook` with
   `<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>`, replacing Xboard's old URL (`app_url` must be
   HTTPS, on port 443/80/88/8443).
3. **Admin chat IDs** (`telegram_admin_chat_ids`): each admin sends `/id` in a private chat with the bot (or in a group
   that should receive the notifications) and enters the Chat ID from the reply here. You can also use the
   `telegram_id` imported for that admin account, shown in the user details in the admin.
4. **Events** (`telegram_events`): by default order payments, admin sign-ins, new tickets, withdrawal requests and
   subscription leaks are pushed; add or remove events as needed.
5. **Community link** (`telegram_discuss_link`): enter Xboard's setting of the same name. **User notifications**
   (`telegram_user_notify`) is on by default.
6. Click "Send test message"; every admin Chat ID should receive a test message.

Users who are already bound do not need to bind again (`telegram_id` was imported); the only difference is that the user
area does not show their Telegram username (Xboard does not store it; it appears once the user unbinds and binds again).
Xboard's bot commands (`/bind`, `/traffic`, `/getlatesturl`, replying to tickets) do not exist in ggboard: ggboard binds
with a one-time code (Profile → Account → Telegram, `/start <code>`), and the bot only answers `/start`, `/unbind`,
`/id` and `/help`.

## 6. Rollback

```bash
cd /opt/ggboard && gg stop
# NPM: set the Forward Hostname/Port of the three Proxy Hosts back to the values recorded in §3.1
docker start xboard
```

Xboard's database was never changed, so after a rollback the state is exactly what it was when Xboard was stopped.
Orders, tickets and sign-ups created in ggboard after the cutover do not go back to Xboard (ggboard has no reverse
export), so decide on a rollback within a few hours; any data that must be kept can only be re-entered by hand.

## 7. Known differences

| Item | Xboard | ggboard |
| --- | --- | --- |
| Order / payment history | 2101 rows | Not migrated (requirement) |
| Telegram bot | Enabled (`/bind` and other commands; notifications go to admins who have bound Telegram) | Available, but the settings must be filled in again (§5.1); `telegram_id` is imported as is, so the same bot can keep sending messages; admin notifications go to `telegram_admin_chat_ids`; binding uses one-time codes |
| First-purchase commission check (`commission_first_time_enable`) | Looks at past orders | Currently off, so no effect |
| Upgrade credit (`surplus_enable`) | Computed from past orders | Imported users get no credit on their first upgrade (IMPORT_XBOARD.md §8.4) |
| Themes / email templates | `theme_*`, `email_template` | Not migrated |
| Subscription short link | None | `https://<sub-domain>/s/<token>` works as well |
| Time zone | `Asia/Shanghai` | The container sets `TZ=Asia/Shanghai`; traffic reset dates use the same zone |
