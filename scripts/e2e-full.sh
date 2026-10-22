#!/usr/bin/env bash
# Full API end-to-end test (bash + curl + python3 -I, no browser). It also works as a seeder for manual
# browser testing.
#
#   scripts/e2e-full.sh                       # build, start a throwaway server on a free port, run, stop
#   BASE=http://127.0.0.1:18090 scripts/e2e-full.sh
#                                             # run against a running server (admin must exist:
#                                             #   ggboard admin create --email admin@example.com --password 'Passw0rd!!')
#   KEEP=1 scripts/e2e-full.sh                # keep the throwaway server running afterwards (prints its URL)
#
# Flows: guest config/plans, admin CRUD (group, plans, nodes vless-REALITY/ss2022/hysteria2, route, coupon,
# epay payment, notice, knowledge), register/login, coupon check, order → epay checkout URL → cancel, order →
# admin mark paid, UniProxy config/user/push/alive, subscribe in every format (+ short link, ETag/304),
# rule profile + preview, knowledge access gate, ticket ↔ staff reply, TOTP setup/enable/login/backup/disable,
# admin users (filter/patch/generate/export/login-as), orders assign/cancel, settings + mail-test error path,
# rulesets, stats, system. Every step asserts the HTTP status and the fields the frontend reads.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/ggboard-e2e-full.XXXXXX")"
PID=""
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Passw0rd!!}"
USER_PASSWORD='Passw0rd!!'
RUN_ID="$(date +%s)$RANDOM"
PASS=0

cleanup() {
	local code=$?
	if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
		if [[ "${KEEP:-0}" == "1" && $code -eq 0 ]]; then
			echo "server still running: $BASE (pid $PID, data $WORK/data)"
			return
		fi
		kill -TERM "$PID" 2>/dev/null || true
		wait "$PID" 2>/dev/null || true
	fi
	if [[ $code -ne 0 && -f "$WORK/server.log" ]]; then
		echo "---- server log (tail) ----"; tail -n 40 "$WORK/server.log"
	fi
	[[ "${KEEP:-0}" == "1" && $code -eq 0 ]] || rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; [[ -f "$WORK/body" ]] && { echo "---- last body ----" >&2; head -c 2000 "$WORK/body" >&2; echo >&2; }; exit 1; }
ok() { PASS=$((PASS + 1)); printf '  ok  %s\n' "$*"; }

# py EXPR — evaluate a Python expression with d = parsed JSON of the last response body (stdout = result).
py() { python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); r=eval(sys.argv[2]); print(r if not isinstance(r,(dict,list,bool)) and r is not None else json.dumps(r))' "$WORK/body" "$1"; }
# check EXPR [MSG] — assert a Python expression on d is truthy.
check() { [[ "$(py "bool($1)")" == "true" ]] || fail "${2:-assertion} :: $1"; }

# call METHOD PATH [JSON] [TOKEN] — sets $STATUS, body in $WORK/body, headers in $WORK/headers.
call() {
	local method=$1 path=$2 data=${3:-} tok=${4:-}
	local args=(-sS -X "$method" -o "$WORK/body" -D "$WORK/headers" -w '%{http_code}' -H 'Accept: application/json')
	[[ -n "$tok" ]] && args+=(-H "Authorization: Bearer $tok")
	[[ -n "$data" ]] && args+=(-H 'Content-Type: application/json' --data-binary "$data")
	STATUS="$(curl "${args[@]}" "$BASE$path")" || fail "curl $method $path"
}
# expect METHOD PATH STATUS [JSON] [TOKEN]
expect() {
	local method=$1 path=$2 want=$3
	call "$method" "$path" "${4:-}" "${5:-}"
	[[ "$STATUS" == "$want" ]] || fail "$method $path → $STATUS (want $want)"
}
# expect_any METHOD PATH "S1 S2" [JSON] [TOKEN] — any of the listed statuses.
expect_any() {
	local method=$1 path=$2 want=" $3 "
	call "$method" "$path" "${4:-}" "${5:-}"
	[[ "$want" == *" $STATUS "* ]] || fail "$method $path → $STATUS (want one of$want)"
}
header() { grep -i "^$1:" "$WORK/headers" | head -n1 | cut -d' ' -f2- | tr -d '\r'; }

# totp SECRET [STEP_OFFSET] — RFC 6238 code (SHA1, 6 digits, 30 s).
totp() {
	python3 -I -c '
import base64,hmac,hashlib,struct,sys,time
s=sys.argv[1].upper(); s+="="*(-len(s)%8)
k=base64.b32decode(s); c=int(time.time())//30+int(sys.argv[2])
h=hmac.new(k,struct.pack(">Q",c),hashlib.sha1).digest(); o=h[-1]&15
print("%06d"%((struct.unpack(">I",h[o:o+4])[0]&0x7fffffff)%1000000))' "$1" "${2:-0}"
}
# Seconds left in the current 30 s TOTP step.
totp_left() { python3 -I -c 'import time; print(30-int(time.time())%30)'; }

free_port() { python3 -I -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'; }

# ---------------------------------------------------------------------------------------------------- server
if [[ -z "${BASE:-}" ]]; then
	BIN="$WORK/ggboard"
	echo "==> building"
	(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$BIN" ./cmd/ggboard)
	export GGBOARD_DATA_DIR="$WORK/data" GGBOARD_JWT_SECRET="e2e-full-secret-0123456789abcdef0123456789" GGBOARD_LOG_LEVEL=warn
	"$BIN" admin create --email "$ADMIN_EMAIL" --password "$ADMIN_PASSWORD" >/dev/null
	PORT="$(free_port)"
	BASE="http://127.0.0.1:$PORT"
	GGBOARD_SERVER_ADDR="127.0.0.1:$PORT" "$BIN" serve >"$WORK/server.log" 2>&1 &
	PID=$!
	for _ in $(seq 1 100); do
		curl -fsS "$BASE/healthz" >/dev/null 2>&1 && break
		kill -0 "$PID" 2>/dev/null || fail "server exited early"
		sleep 0.1
	done
fi
curl -fsS "$BASE/healthz" >/dev/null || fail "server not ready at $BASE"
echo "==> $BASE"

# ---------------------------------------------------------------------------------------------------- guest
echo "==> guest"
expect GET /api/v1/guest/config 200
check "d['data']['app_name'] and 'register_enabled' in d['data'] and 'captcha' in d['data']" "guest config shape"
expect GET /api/v1/guest/rulesets/catalog 200
check "len(d['data']['templates'])>0 and len(d['data']['sources'])>0" "catalog"
ok "guest config + catalog"

# ---------------------------------------------------------------------------------------------------- admin login + settings
echo "==> admin"
expect POST /api/v1/passport/login 200 "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
check "d['data'].get('mfa_required') is None or d['data'].get('mfa_required') is False" "admin without MFA"
ADMIN="$(py "d['data']['token']")"
check "d['data']['user']['is_admin'] is True" "admin flag"
expect GET /api/v1/admin/settings 200 "" "$ADMIN"
SERVER_TOKEN="$(py "d['data']['server']['server_token']")"
check "len(d['data']['groups'])>=9" "settings groups metadata"
[[ ${#SERVER_TOKEN} -ge 16 ]] || fail "server_token"
expect PUT /api/v1/admin/settings 200 "{\"app_url\":\"$BASE\",\"app_description\":\"E2E demo panel\",\"email_verify\":false,\"stop_register\":false}" "$ADMIN"
expect PUT /api/v1/admin/settings 400 '{"no_such_key":1}' "$ADMIN"
check "'no_such_key' in d.get('errors',{})" "unknown key → field error"
ok "admin login, settings get/put (+ unknown key 400)"

# mail test error path: SMTP on a closed port must answer 400 with the driver error, then restore the log driver.
expect PUT /api/v1/admin/settings 200 '{"email_driver":"smtp","email_host":"127.0.0.1","email_port":1,"email_encryption":"none"}' "$ADMIN"
expect POST /api/v1/admin/settings/mail/test 400 '{"email":"admin@example.com"}' "$ADMIN"
check "d['message']" "mail error message"
expect PUT /api/v1/admin/settings 200 '{"email_driver":null,"email_host":null,"email_port":null,"email_encryption":null}' "$ADMIN"
expect POST /api/v1/admin/settings/mail/test 200 '{"email":"admin@example.com"}' "$ADMIN"
ok "mail test: smtp error → 400, log driver → 200"

# ---------------------------------------------------------------------------------------------------- catalog CRUD
expect POST /api/v1/admin/groups 201 "{\"name\":\"Standard $RUN_ID\"}" "$ADMIN"
GROUP="$(py "d['data']['id']")"
expect PATCH "/api/v1/admin/groups/$GROUP" 200 "{\"name\":\"Standard $RUN_ID\"}" "$ADMIN"
expect GET /api/v1/admin/groups 200 "" "$ADMIN"
check "any(g['id']==$GROUP and 'user_count' in g for g in d['data'])" "group list"
ok "group create/patch/list"

PRO="{\"group_id\":$GROUP,\"name\":\"Pro\",\"content\":\"- 100 GB / month\\n- 3 devices\\n- Streaming unlock\",\"tags\":[\"Popular\"],\"transfer_enable\":100,\"device_limit\":3,\"prices\":{\"monthly\":1500,\"quarterly\":4000,\"yearly\":15000,\"reset_traffic\":500},\"reset_traffic_method\":1,\"show\":true,\"sell\":true,\"renew\":true}"
expect POST /api/v1/admin/plans 201 "$PRO" "$ADMIN"
PLAN_PRO="$(py "d['data']['id']")"
check "d['data']['prices']['quarterly']==4000 and d['data']['reset_traffic_method']==1" "plan prices"
expect POST /api/v1/admin/plans 201 "{\"group_id\":$GROUP,\"name\":\"Lifetime 500G\",\"content\":\"One-time 500 GB, never expires\",\"transfer_enable\":500,\"prices\":{\"onetime\":9900},\"show\":true,\"sell\":true}" "$ADMIN"
PLAN_ONCE="$(py "d['data']['id']")"
expect PATCH "/api/v1/admin/plans/$PLAN_ONCE" 200 '{"sort":2,"tags":["Lifetime"]}' "$ADMIN"
expect POST /api/v1/admin/plans/sort 200 "{\"ids\":[$PLAN_PRO,$PLAN_ONCE]}" "$ADMIN"
expect GET /api/v1/admin/plans 200 "" "$ADMIN"
check "len([p for p in d['data'] if p['id'] in ($PLAN_PRO,$PLAN_ONCE)])==2 and all('user_count' in p for p in d['data'])" "plan list"
expect GET /api/v1/guest/plans 200
check "any(p['id']==$PLAN_PRO for p in d['data']) and all('group_id' not in p for p in d['data'])" "guest plans"
ok "plans: create (periodic + reset_traffic, onetime), patch, sort, list, guest"

# REALITY key pair (X25519, base64url without padding).
KEYS="$(cd "$WORK" && openssl genpkey -algorithm X25519 -outform DER -out k.der 2>/dev/null && openssl pkey -inform DER -in k.der -pubout -outform DER -out p.der 2>/dev/null &&
	python3 -I -c 'import base64,sys; e=lambda b: base64.urlsafe_b64encode(b[-32:]).rstrip(b"=").decode(); print(e(open(sys.argv[1],"rb").read()), e(open(sys.argv[2],"rb").read()))' k.der p.der)"
PRIV="${KEYS% *}"; PUB="${KEYS#* }"
[[ ${#PRIV} -eq 43 && ${#PUB} -eq 43 ]] || fail "x25519 keygen"
SSKEY="$(python3 -I -c 'import base64,os; print(base64.b64encode(os.urandom(16)).decode())')"

VLESS="{\"type\":\"vless\",\"name\":\"Tokyo 01 REALITY\",\"group_ids\":[$GROUP],\"host\":\"jp1.example.com\",\"port\":443,\"server_port\":443,\"tags\":[\"JP\",\"Netflix\"],\"rate\":1,\"show\":true,\"protocol_settings\":{\"network\":\"tcp\",\"tls\":2,\"flow\":\"xtls-rprx-vision\",\"tls_settings\":{\"server_name\":\"www.example.org\",\"dest\":\"www.example.org:443\",\"server_port\":\"443\",\"public_key\":\"$PUB\",\"private_key\":\"$PRIV\",\"short_id\":\"0123abcd\",\"fingerprint\":\"chrome\"}}}"
expect POST /api/v1/admin/nodes 201 "$VLESS" "$ADMIN"
N_VLESS="$(py "d['data']['id']")"
check "d['data']['status']['state'] in ('online','idle','offline')" "node status"
expect POST /api/v1/admin/nodes 201 "{\"type\":\"shadowsocks\",\"name\":\"Hong Kong SS2022\",\"group_ids\":[$GROUP],\"host\":\"hk1.example.com\",\"port\":\"8388\",\"server_port\":8388,\"tags\":[\"HK\"],\"rate\":1.5,\"show\":true,\"protocol_settings\":{\"cipher\":\"2022-blake3-aes-128-gcm\",\"server_key\":\"$SSKEY\"}}" "$ADMIN"
N_SS="$(py "d['data']['id']")"
expect POST /api/v1/admin/nodes 201 "{\"type\":\"hysteria\",\"name\":\"Singapore HY2\",\"group_ids\":[$GROUP],\"host\":\"sg1.example.com\",\"port\":\"20000-30000\",\"server_port\":443,\"tags\":[\"SG\"],\"rate\":0.5,\"show\":true,\"protocol_settings\":{\"version\":2,\"up_mbps\":200,\"down_mbps\":500,\"obfs\":\"salamander\",\"obfs_password\":\"e2e-obfs-password\",\"server_name\":\"sg1.example.com\",\"hop_interval\":30}}" "$ADMIN"
N_HY="$(py "d['data']['id']")"
expect POST /api/v1/admin/nodes 400 "{\"type\":\"vless\",\"name\":\"bad\",\"group_ids\":[$GROUP],\"host\":\"x.example.com\",\"port\":443,\"server_port\":443,\"protocol_settings\":{\"tls\":2,\"tls_settings\":{\"private_key\":\"not-a-key\"}}}" "$ADMIN"
check "any(k.startswith('protocol_settings') for k in d.get('errors',{}))" "nested protocol_settings errors"
expect PATCH "/api/v1/admin/nodes/$N_SS" 200 '{"tags":["HK","IPLC"]}' "$ADMIN"
expect_any POST "/api/v1/admin/nodes/$N_HY/copy" "200 201" "" "$ADMIN"
N_COPY="$(py "d['data']['id']")"
check "d['data']['show'] is False and d['data']['name'].endswith('(copy)')" "copy"
expect DELETE "/api/v1/admin/nodes/$N_COPY" 200 "" "$ADMIN"
expect GET /api/v1/admin/nodes 200 "" "$ADMIN"
check "len([n for n in d['data'] if n['id'] in ($N_VLESS,$N_SS,$N_HY)])==3" "node list"
expect GET "/api/v1/admin/nodes/$N_VLESS/stats?range=7d" 200 "" "$ADMIN"
check "len(d['data'])==7" "node stats 7 points"
expect GET "/api/v1/admin/nodes/$N_VLESS/online" 200 "" "$ADMIN"
ok "nodes: vless REALITY, ss2022, hysteria2 (+ 400 nested errors, patch, copy, delete, stats, online)"

expect_any POST /api/v1/admin/routes "200 201" '{"remarks":"Block ads","match":["(.*\\.)?ads\\.example\\.com$","tracker"],"action":"block"}' "$ADMIN"
ROUTE="$(py "d['data']['id']")"
expect POST /api/v1/admin/routes 400 '{"remarks":"bad","match":["(unclosed"],"action":"block"}' "$ADMIN"
expect PATCH "/api/v1/admin/nodes/$N_VLESS" 200 "{\"route_ids\":[$ROUTE]}" "$ADMIN"
expect DELETE "/api/v1/admin/routes/$ROUTE" 409 "" "$ADMIN"
expect GET /api/v1/admin/routes 200 "" "$ADMIN"
ok "routes: create, invalid regexp 400, attach, delete-in-use 409"

STARTS="$(python3 -I -c 'import datetime as t; print((t.datetime.now(t.timezone.utc)-t.timedelta(days=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
ENDS="$(python3 -I -c 'import datetime as t; print((t.datetime.now(t.timezone.utc)+t.timedelta(days=30)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
COUPON_CODE="WELCOME$RUN_ID"
expect POST /api/v1/admin/coupons 201 "{\"name\":\"Welcome 10%\",\"code\":\"$COUPON_CODE\",\"type\":2,\"value\":10,\"limit_use\":100,\"limit_plan_ids\":[$PLAN_PRO],\"started_at\":\"$STARTS\",\"ended_at\":\"$ENDS\"}" "$ADMIN"
COUPON="$(py "d['data'][0]['id']")"
expect POST /api/v1/admin/coupons 409 "{\"name\":\"dup\",\"code\":\"$COUPON_CODE\",\"type\":1,\"value\":100,\"started_at\":\"$STARTS\",\"ended_at\":\"$ENDS\"}" "$ADMIN"
expect PATCH "/api/v1/admin/coupons/$COUPON" 200 '{"limit_use_with_user":2}' "$ADMIN"
expect GET /api/v1/admin/coupons 200 "" "$ADMIN"
check "any(c['id']==$COUPON and 'used_count' in c for c in d['data'])" "coupon list"
ok "coupons: create, duplicate 409, patch, list"

expect GET /api/v1/admin/payments/gateways 200 "" "$ADMIN"
check "any(g['name']=='epay' and any(f['key']=='pid' for f in g['fields']) for g in d['data'])" "gateway fields"
expect POST /api/v1/admin/payments 400 '{"gateway":"epay","name":"EPay","config":{"url":"https://pay.example.com"}}' "$ADMIN"
check "'config.pid' in d.get('errors',{})" "required config field error"
expect POST /api/v1/admin/payments 201 '{"gateway":"epay","name":"Alipay (EPay)","icon":"","config":{"url":"https://pay.example.com","pid":"1000","key":"e2e-dummy-key","type":"alipay"},"handling_fee_percent":1,"enabled":true}' "$ADMIN"
PAYMENT="$(py "d['data']['id']")"
check "'/api/v1/guest/payment/notify/epay/' in d['data']['notify_url']" "notify url"
expect GET /api/v1/admin/payments 200 "" "$ADMIN"
ok "payments: gateways, required-field 400, create epay, list"

expect POST /api/v1/admin/notices 201 '{"title":"Welcome to GG Bond","content":"Maintenance window **Sunday 02:00 UTC**. Nodes may restart.","tags":["Maintenance"],"show":true}' "$ADMIN"
NOTICE="$(py "d['data']['id']")"
expect PATCH "/api/v1/admin/notices/$NOTICE" 200 '{"sort":1}' "$ADMIN"
expect GET /api/v1/admin/notices 200 "" "$ADMIN"
KB_BODY='# Getting started\n\nYour subscription: `{{subscribeUrl}}`\n\n<!--access start-->\nSecret members-only setup steps.\n<!--access end-->\n\n[Import to Clash](clash://install-config?url={{urlEncodeSubscribeUrl}})'
expect POST /api/v1/admin/knowledge 201 "{\"language\":\"zh-CN\",\"category\":\"Getting started\",\"title\":\"Clash setup\",\"body\":\"$KB_BODY\",\"show\":true}" "$ADMIN"
KB="$(py "d['data']['id']")"
expect POST /api/v1/admin/knowledge 201 "{\"language\":\"en\",\"category\":\"Getting started\",\"title\":\"Clash setup (EN)\",\"body\":\"$KB_BODY\",\"show\":true}" "$ADMIN"
expect GET /api/v1/admin/knowledge 200 "" "$ADMIN"
check "any(k['id']==$KB and 'body' not in k for k in d['data'])" "knowledge list"
expect GET "/api/v1/admin/knowledge/$KB" 200 "" "$ADMIN"
check "'{{subscribeUrl}}' in d['data']['body']" "raw body"
ok "notices + knowledge (raw body with placeholders and access block)"

# ---------------------------------------------------------------------------------------------------- user register/login
echo "==> user"
USER_EMAIL="${USER_EMAIL:-user$RUN_ID@example.com}"
expect POST /api/v1/passport/register 200 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
USER="$(py "d['data']['token']")"
USER_ID="$(py "d['data']['user']['id']")"
expect POST /api/v1/passport/register 409 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
expect POST /api/v1/passport/login 401 "{\"email\":\"$USER_EMAIL\",\"password\":\"wrong-password\"}"
expect POST /api/v1/passport/login 200 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
USER="$(py "d['data']['token']")"
expect GET /api/v1/user/me 200 "" "$USER"
check "d['data']['email']=='$USER_EMAIL' and d['data']['plan_id'] is None" "me"
expect PATCH /api/v1/user/me 200 '{"remind_traffic":false}' "$USER"
check "d['data']['remind_traffic'] is False" "patch me"
expect GET /api/v1/user/stats 200 "" "$USER"
check "set(['pending_orders','open_tickets','notices_count','invite_count','commission_pending'])<=set(d['data'])" "stats keys"
expect GET /api/v1/user/me 401
ok "register (+409), login (+401), me, patch me, stats, 401 without token"

# Knowledge gate: no plan yet → access block replaced.
expect GET /api/v1/user/knowledge 200 "" "$USER"
check "'Getting started' in d['data']" "knowledge index by category"
expect GET "/api/v1/user/knowledge/$KB" 200 "" "$USER"
check "'knowledge-no-access' in d['data']['body'] and 'Secret members-only' not in d['data']['body'] and '{{subscribeUrl}}' not in d['data']['body']" "gated body"
expect GET /api/v1/user/notices 200 "" "$USER"
check "any(n['id']==$NOTICE for n in d['data'])" "notices"
ok "knowledge gated without plan, notices"

# ---------------------------------------------------------------------------------------------------- shop
echo "==> shop"
expect GET /api/v1/user/plans 200 "" "$USER"
check "any(p['id']==$PLAN_PRO for p in d['data'])" "user plans"
expect GET "/api/v1/user/plans/$PLAN_PRO" 200 "" "$USER"
expect GET /api/v1/user/payment-methods 200 "" "$USER"
check "any(m['id']==$PAYMENT and m['gateway']=='epay' for m in d['data'])" "payment methods"
expect POST /api/v1/user/coupons/check 200 "{\"code\":\"$COUPON_CODE\",\"plan_id\":$PLAN_PRO,\"period\":\"monthly\"}" "$USER"
check "d['data']['type']==2 and d['data']['value']==10" "coupon check"
expect POST /api/v1/user/coupons/check 400 "{\"code\":\"$COUPON_CODE\",\"plan_id\":$PLAN_ONCE,\"period\":\"onetime\"}" "$USER"
check "'code' in d.get('errors',{})" "coupon wrong plan → errors.code"
expect POST /api/v1/user/orders 400 "{\"plan_id\":$PLAN_PRO,\"period\":\"onetime\"}" "$USER"
expect POST /api/v1/user/orders 201 "{\"plan_id\":$PLAN_PRO,\"period\":\"monthly\",\"coupon_code\":\"$COUPON_CODE\"}" "$USER"
TRADE1="$(py "d['data']['trade_no']")"
expect POST /api/v1/user/orders 409 "{\"plan_id\":$PLAN_PRO,\"period\":\"yearly\"}" "$USER"
check "'trade_no' in d.get('errors',{})" "pending order 409 errors.trade_no"
expect GET "/api/v1/user/orders/$TRADE1" 200 "" "$USER"
check "d['data']['total_amount']==1350 and d['data']['discount_amount']==150 and d['data']['status']==0 and d['data']['plan']['name']=='Pro'" "order amounts"
expect POST "/api/v1/user/orders/$TRADE1/checkout" 200 "{\"payment_id\":$PAYMENT}" "$USER"
check "d['data']['type']=='url' and d['data']['data'].startswith('https://pay.example.com/submit.php?')" "epay url"
PAYURL="$(py "d['data']['data']")"
python3 -I -c '
import sys,urllib.parse as u
q=dict(u.parse_qsl(u.urlsplit(sys.argv[1]).query)); t=sys.argv[2]
assert q["out_trade_no"]==t and q["pid"]=="1000" and q["sign_type"]=="MD5" and len(q["sign"])==32, q
assert q["return_url"].endswith("/api/v1/guest/payment/return/"+t), q["return_url"]
assert "/api/v1/guest/payment/notify/epay/" in q["notify_url"], q["notify_url"]
assert q["money"]=="13.64", q["money"]' "$PAYURL" "$TRADE1" || fail "epay redirect URL shape: $PAYURL"
code="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' "$BASE/api/v1/guest/payment/return/$TRADE1?trade_status=TRADE_SUCCESS")"
[[ "$code" == "302 $BASE/orders/$TRADE1" ]] || fail "payment return redirect: $code"
expect GET "/api/v1/user/orders/$TRADE1/status" 200 "" "$USER"
check "d['data']['status']==0" "still pending"
expect POST "/api/v1/user/orders/$TRADE1/cancel" 200 "" "$USER"
expect POST "/api/v1/user/orders/$TRADE1/cancel" 409 "" "$USER"
ok "plans, payment methods, coupon check, order (+400/409), epay checkout URL + return 302, cancel (+409)"

expect POST /api/v1/user/orders 201 "{\"plan_id\":$PLAN_PRO,\"period\":\"quarterly\"}" "$USER"
TRADE2="$(py "d['data']['trade_no']")"
expect GET "/api/v1/admin/orders?trade_no=$TRADE2" 200 "" "$ADMIN"
ORDER2="$(py "d['data'][0]['id']")"
check "d['data'][0]['user']['email']=='$USER_EMAIL'" "admin order user"
expect POST "/api/v1/admin/orders/$ORDER2/paid" 200 "" "$ADMIN"
check "d['data']['status'] in (1,3) and d['data']['callback_no']=='manual'" "mark paid"
expect POST "/api/v1/admin/orders/$ORDER2/paid" 409 "" "$ADMIN"
expect GET "/api/v1/admin/orders/$ORDER2" 200 "" "$ADMIN"
expect GET /api/v1/user/orders 200 "" "$USER"
check "d['total']>=2 and d['data'][0]['trade_no']=='$TRADE2'" "user orders newest first"
expect GET /api/v1/user/me 200 "" "$USER"
check "d['data']['plan_id']==$PLAN_PRO and d['data']['transfer_enable']==100*1024**3 and d['data']['expired_at']" "plan granted"
ok "order → admin mark paid (+409) → plan granted"

# ---------------------------------------------------------------------------------------------------- nodes ↔ panel
echo "==> node API"
NQ="token=$SERVER_TOKEN&node_id=$N_VLESS&node_type=vless"
expect GET "/api/v1/server/UniProxy/config?$NQ" 200
check "d['protocol']=='vless' and d['tls']==2 and d['tls_settings']['private_key']=='$PRIV'" "uniproxy vless reality config"
expect GET "/api/v1/server/UniProxy/config?token=wrong&node_id=$N_VLESS&node_type=vless" 403
expect GET "/api/v1/server/UniProxy/user?$NQ" 200
UUID="$(py "[u['uuid'] for u in d['users'] if u['id']==$USER_ID][0]")"
ETAG="$(header ETag)"
code="$(curl -sS -o /dev/null -w '%{http_code}' -H "If-None-Match: $ETAG" "$BASE/api/v1/server/UniProxy/user?$NQ")"
[[ "$code" == 304 ]] || fail "user list ETag → $code"
expect GET "/api/v1/server/UniProxy/config?token=$SERVER_TOKEN&node_id=$N_HY&node_type=hysteria2" 200
check "d['server_port']==443 and d['obfs']=='salamander'" "hy2 config"
expect GET "/api/v1/server/UniProxy/config?token=$SERVER_TOKEN&node_id=$N_SS&node_type=shadowsocks" 200
check "d['cipher']=='2022-blake3-aes-128-gcm' and d['server_key']" "ss2022 config"
expect POST "/api/v1/server/UniProxy/push?$NQ" 200 "{\"$USER_ID\":[1073741824,3221225472]}"
expect POST "/api/v1/server/UniProxy/push?token=$SERVER_TOKEN&node_id=$N_SS&node_type=shadowsocks" 200 "{\"$USER_ID\":[536870912,536870912]}"
expect POST "/api/v1/server/UniProxy/alive?$NQ" 200 "{\"$USER_ID\":[\"203.0.113.7\"]}"
expect GET "/api/v1/server/UniProxy/alivelist?$NQ" 200
ok "UniProxy config (vless/hy2/ss2022), 403, user + ETag 304, push, alive, alivelist"

# Traffic is flushed every 10 s: wait for it.
echo "  .. waiting for the traffic flush"
want=$((1073741824 + 3221225472 + (536870912 + 536870912) * 3 / 2))
for _ in $(seq 1 30); do
	call GET /api/v1/user/subscription "" "$USER"
	[[ "$(py "d['data']['u']+d['data']['d']")" == "$want" ]] && break
	sleep 1
done
check "d['data']['u']+d['data']['d']==$want" "traffic (rate applied)"
SUB_TOKEN="$(py "d['data']['token']")"
SUB_URL="$(py "d['data']['subscribe_url']")"
check "d['data']['plan']['name']=='Pro' and d['data']['subscribe_url'].startswith('$BASE')" "subscription info"
expect GET "/api/v1/admin/nodes/$N_VLESS/online" 200 "" "$ADMIN"
check "any(r['user_id']==$USER_ID and '203.0.113.7' in r['ips'] for r in d['data'])" "online drawer"
expect GET /api/v1/admin/nodes 200 "" "$ADMIN"
check "[n for n in d['data'] if n['id']==$N_VLESS][0]['status']['state']=='online'" "node online after push"
expect GET /api/v1/user/traffic-logs 200 "" "$USER"
check "len(d['data'])>=1 and d['data'][0]['node_name']" "traffic logs"
expect GET /api/v1/user/nodes 200 "" "$USER"
check "len(d['data'])==3" "user nodes"
ok "traffic flushed with rates, subscription info, online drawer, node online, traffic logs, user nodes"

# ---------------------------------------------------------------------------------------------------- subscribe
echo "==> subscribe"
for flag in clash-meta clash stash singbox surge surfboard quantumultx shadowrocket loon v2ray ssr; do
	code="$(curl -sS -o "$WORK/sub" -D "$WORK/headers" -w '%{http_code}' "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=$flag")"
	[[ "$code" == 200 ]] || fail "subscribe flag=$flag → $code"
	[[ "$flag" == ssr ]] || [[ -s "$WORK/sub" ]] || fail "subscribe flag=$flag empty"
	header subscription-userinfo | grep -q 'upload=' || fail "subscribe flag=$flag: no subscription-userinfo"
done
grep -q "Tokyo 01 REALITY" <(curl -sS "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=clash-meta") || fail "clash-meta misses the vless node"
grep -q '"type": *"hysteria2"' <(curl -sS "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=singbox") || fail "singbox misses hysteria2"
base64 -d <<<"$(curl -sS "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=v2ray")" 2>/dev/null | grep -q '^vless://' || fail "v2ray base64 has no vless://"
# node-list resources (QX [server_remote], Loon node subscriptions) get proxy lines only with list=1
for flag in quantumultx loon surge; do
	first="$(curl -sS "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=$flag&list=1" | head -n1)"
	[[ -n "$first" && "$first" != \[* ]] || fail "flag=$flag&list=1 must be proxy lines, got: $first"
	first="$(curl -sS "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=$flag" | head -n1)"
	[[ "$first" == \[* || "$first" == \#* ]] || fail "flag=$flag must be a full config, got: $first"
done
code="$(curl -sS -o /dev/null -w '%{http_code}' -A 'ClashMetaForAndroid/2.11' "$BASE/s/$SUB_TOKEN")"
[[ "$code" == 200 ]] || fail "short link → $code"
ETAG="$(curl -sS -o /dev/null -D - "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=clash-meta" | grep -i '^etag:' | cut -d' ' -f2- | tr -d '\r')"
code="$(curl -sS -o /dev/null -w '%{http_code}' -H "If-None-Match: $ETAG" "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=clash-meta")"
[[ "$code" == 304 ]] || fail "subscribe ETag → $code"
code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/client/subscribe?token=not-a-token")"
[[ "$code" == 403 ]] || fail "bad token → $code"
code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/client/subscribe?token=$SUB_TOKEN&flag=clash-meta&cats=__nope__")"
[[ "$code" == 400 ]] || fail "invalid override → $code"
ok "subscribe in 11 formats, short link, ETag 304, 403 bad token, 400 bad override"

expect GET /api/v1/user/rule-profile 200 "" "$USER"
check "'template' in d['data'] and 'effective' in d['data'] and d['data']['editable'] is True" "rule profile view"
TEMPLATES="$(curl -sS "$BASE/api/v1/guest/rulesets/catalog" | python3 -I -c 'import json,sys; d=json.load(sys.stdin)["data"]; print(" ".join(t["id"] for t in d["templates"]))')"
OTHER_TPL="$(tr ' ' '\n' <<<"$TEMPLATES" | grep -v '^standard$' | head -n1)"
expect PUT /api/v1/user/rule-profile 200 "{\"template\":\"$OTHER_TPL\"}" "$USER"
check "d['data']['template']=='$OTHER_TPL' and d['data']['customized'] is True" "rule profile saved"
expect PUT /api/v1/user/rule-profile 400 '{"template":"__nope__"}' "$USER"
for client in clash-meta singbox surge; do
	call GET "/api/v1/user/subscription/preview?client=$client" "" "$USER"
	[[ "$STATUS" == 200 ]] || fail "preview $client → $STATUS"
	[[ "$(header X-Subscription-Format)" == "$client" ]] || fail "preview $client format header: $(header X-Subscription-Format)"
	[[ -s "$WORK/body" ]] || fail "preview $client empty"
done
ok "rule profile get/put (+400), preview clash-meta/singbox/surge"

expect GET "/api/v1/user/knowledge/$KB" 200 "" "$USER"
check "'Secret members-only' in d['data']['body'] and '$SUB_URL' in d['data']['body'] and 'knowledge-no-access' not in d['data']['body']" "unlocked body"
ok "knowledge unlocked with a valid plan, placeholders replaced"

# ---------------------------------------------------------------------------------------------------- invite
expect POST /api/v1/user/invite/codes 201 "" "$USER"
INVITE="$(py "d['data']['code']")"
expect GET /api/v1/user/invite 200 "" "$USER"
check "any(c['code']=='$INVITE' for c in d['data']['codes']) and 'commission_rate' in d['data']['stat']" "invite overview"
expect GET /api/v1/user/invite/commissions 200 "" "$USER"
ok "invite code + overview + commissions"

# ---------------------------------------------------------------------------------------------------- tickets
echo "==> tickets"
expect POST /api/v1/user/tickets 201 '{"subject":"Cannot connect to Tokyo","level":1,"message":"The REALITY node times out since this morning."}' "$USER"
TICKET="$(py "d['data']['id']")"
expect POST /api/v1/user/tickets 409 '{"subject":"Second","level":0,"message":"another"}' "$USER"
expect GET /api/v1/admin/tickets 200 "" "$ADMIN"
check "any(t['id']==$TICKET and t['user']['email']=='$USER_EMAIL' for t in d['data'])" "admin tickets"
expect POST "/api/v1/admin/tickets/$TICKET/reply" 200 '{"message":"We restarted the node, please try again."}' "$ADMIN"
check "d['data']['reply_status']==1" "staff reply status"
expect GET "/api/v1/user/tickets/$TICKET" 200 "" "$USER"
check "len(d['data']['messages'])==2 and d['data']['messages'][1]['is_me'] is False and 'restarted' in d['data']['messages'][1]['message']" "user sees staff reply"
expect POST "/api/v1/user/tickets/$TICKET/reply" 200 '{"message":"Works now, thanks!"}' "$USER"
expect POST "/api/v1/user/tickets/$TICKET/close" 200 "" "$USER"
expect POST "/api/v1/user/tickets/$TICKET/reply" 400 '{"message":"again"}' "$USER"
# leave one open ticket for the UI
expect POST /api/v1/user/tickets 201 '{"subject":"Invoice for my order","level":0,"message":"Could you send an invoice for my last order?"}' "$USER"
ok "ticket create (+409), staff reply → user sees it, user reply, close (+400 reply)"

# ---------------------------------------------------------------------------------------------------- MFA
echo "==> MFA"
expect GET /api/v1/user/security 200 "" "$USER"
check "d['data']['totp_enabled'] is False and d['data']['passkeys']==[]" "security view"
expect POST /api/v1/user/security/totp/setup 200 "" "$USER"
SECRET="$(py "d['data']['secret']")"
check "d['data']['otpauth_url'].startswith('otpauth://totp/')" "otpauth url"
expect POST /api/v1/user/security/totp/enable 400 '{"code":"000000"}' "$USER"
[[ "$(totp_left)" -lt 4 ]] && sleep 4
expect POST /api/v1/user/security/totp/enable 200 "{\"code\":\"$(totp "$SECRET")\"}" "$USER"
BACKUP1="$(py "d['data']['backup_codes'][0]")"
BACKUP2="$(py "d['data']['backup_codes'][1]")"
check "len(d['data']['backup_codes'])==10" "10 backup codes"
expect POST /api/v1/passport/login 200 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
check "d['data']['mfa_required'] is True and d['data']['methods']==['totp'] and 'token' not in d['data']" "mfa step"
MFA="$(py "d['data']['mfa_token']")"
expect POST /api/v1/passport/login/mfa 401 "{\"mfa_token\":\"$MFA\",\"code\":\"000000\"}"
# the enable step consumed the current TOTP step; the next one (±1 window) must be accepted
expect POST /api/v1/passport/login/mfa 200 "{\"mfa_token\":\"$MFA\",\"code\":\"$(totp "$SECRET" 1)\"}"
USER="$(py "d['data']['token']")"
expect POST /api/v1/passport/login 200 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
MFA="$(py "d['data']['mfa_token']")"
expect POST /api/v1/passport/login/mfa 200 "{\"mfa_token\":\"$MFA\",\"code\":\"$(tr 'a-z' 'A-Z' <<<"$BACKUP1")\"}"
USER="$(py "d['data']['token']")"
expect GET /api/v1/user/security 200 "" "$USER"
check "d['data']['totp_enabled'] is True and d['data']['backup_codes_remaining']==9" "backup code consumed"
expect POST /api/v1/user/security/backup-codes 200 "{\"code\":\"$BACKUP2\"}" "$USER"
check "len(d['data']['backup_codes'])==10" "regenerated"
BACKUP3="$(py "d['data']['backup_codes'][0]")"
expect POST /api/v1/user/security/totp/disable 400 "{\"password\":\"wrong-password\",\"code\":\"$BACKUP3\"}" "$USER"
check "'password' in d.get('errors',{})" "disable wrong password → errors.password"
expect POST /api/v1/user/security/totp/disable 200 "{\"password\":\"$USER_PASSWORD\",\"code\":\"$BACKUP3\"}" "$USER"
expect_any POST /api/v1/user/security/passkeys/begin "200 400" '{}' "$USER"
ok "TOTP setup, enable (+400), login → mfa (+401), TOTP + backup code login, regenerate, disable (+400), passkey begin"

expect POST /api/v1/user/password 400 "{\"old_password\":\"wrong-password\",\"new_password\":\"N3wPassw0rd!!\"}" "$USER"
expect POST /api/v1/user/password 200 "{\"old_password\":\"$USER_PASSWORD\",\"new_password\":\"$USER_PASSWORD\"}" "$USER"
expect GET /api/v1/user/me 401 "" "$USER"
expect POST /api/v1/passport/login 200 "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASSWORD\"}"
USER="$(py "d['data']['token']")"
ok "change password (+400) revokes the old session"

# ---------------------------------------------------------------------------------------------------- admin users / orders
echo "==> admin users/orders"
expect GET "/api/v1/admin/users?email=${USER_EMAIL%@*}&plan_id=$PLAN_PRO" 200 "" "$ADMIN"
check "d['total']==1 and d['data'][0]['id']==$USER_ID and d['data'][0]['plan']['name']=='Pro'" "user filter"
expect GET "/api/v1/admin/users?expired=never&page_size=200" 200 "" "$ADMIN"
expect GET "/api/v1/admin/users/$USER_ID" 200 "" "$ADMIN"
check "'stats' in d['data'] and d['data']['stats']['orders_total']>=1" "user detail stats"
expect PATCH "/api/v1/admin/users/$USER_ID" 200 '{"remarks":"e2e customer","speed_limit":100,"balance":2000}' "$ADMIN"
check "d['data']['remarks']=='e2e customer' and d['data']['balance']==2000" "user patch"
expect PATCH "/api/v1/admin/users/1" 409 '{"is_admin":false}' "$ADMIN"
expect POST /api/v1/admin/users/generate 200 "{\"email_prefix\":\"gen$RUN_ID\",\"email_suffix\":\"example.com\",\"count\":3,\"plan_id\":$PLAN_ONCE}" "$ADMIN"
check "d['data']['count']==3 and d['data']['csv'].count('@example.com')==3" "generate"
code="$(curl -sS -o "$WORK/export.csv" -w '%{http_code} %{content_type}' -X POST -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' --data "{\"email\":\"gen$RUN_ID\"}" "$BASE/api/v1/admin/users/export")"
[[ "$code" == 200\ text/csv* ]] || fail "export → $code"
[[ "$(wc -l <"$WORK/export.csv" | tr -d ' ')" == 4 ]] || fail "export rows: $(cat "$WORK/export.csv")"
expect POST "/api/v1/admin/users/$USER_ID/login-token" 200 "" "$ADMIN"
LOGIN_URL="$(py "d['data']['url']")"
[[ "$LOGIN_URL" == "$BASE/login?token="* ]] || fail "login-as url: $LOGIN_URL"
expect POST /api/v1/passport/login/token 200 "{\"token\":\"${LOGIN_URL#*token=}\"}"
check "d['data']['user']['id']==$USER_ID" "login as"
expect_any POST /api/v1/passport/login/token "400 401" "{\"token\":\"${LOGIN_URL#*token=}\"}"
ok "users: filter, detail, patch, self-demote 409, generate, export CSV, login-as (single use)"

expect POST /api/v1/admin/orders 201 "{\"email\":\"$USER_EMAIL\",\"plan_id\":$PLAN_ONCE,\"period\":\"onetime\",\"total_amount\":0}" "$ADMIN"
ORDER3="$(py "d['data']['id']")"
expect POST "/api/v1/admin/orders/$ORDER3/cancel" 200 "" "$ADMIN"
check "d['data']['status']==2" "admin cancel"
expect GET "/api/v1/admin/orders?status=2&email=$USER_EMAIL" 200 "" "$ADMIN"
check "d['total']>=2" "orders filter"
ok "orders: assign, cancel, filter"

# ---------------------------------------------------------------------------------------------------- stats / system / rulesets
expect GET /api/v1/admin/stats/overview 200 "" "$ADMIN"
check "d['data']['totals']['users']>=2 and d['data']['nodes']['total']>=3 and d['data']['today']['paid_orders']>=1" "overview"
for m in income orders registers traffic; do
	expect GET "/api/v1/admin/stats/series?metric=$m&range=7d" 200 "" "$ADMIN"
	check "len(d['data'])==7" "series $m"
done
expect GET /api/v1/admin/stats/nodes/ranking 200 "" "$ADMIN"
expect GET /api/v1/admin/stats/users/ranking 200 "" "$ADMIN"
expect GET /api/v1/admin/system/status 200 "" "$ADMIN"
check "d['data']['version'] and len(d['data']['scheduler'])>=6" "system status"
expect GET "/api/v1/admin/system/logs?level=info&limit=50" 200 "" "$ADMIN"
expect GET /api/v1/admin/rulesets 200 "" "$ADMIN"
check "'builtin' in d['data'] and 'effective' in d['data']" "rulesets"
expect PUT /api/v1/admin/rulesets 200 '{"custom":null}' "$ADMIN"
expect GET /api/v1/admin/stats/overview 403 "" "$USER"
ok "stats overview/series/rankings, system status/logs, rulesets, 403 for users"

# ---------------------------------------------------------------------------------------------------- SPA delivery
echo "==> SPA"
for p in / /login /register /forgot /dashboard /admin/nodes; do
	code="$(curl -sS -o /dev/null -w '%{http_code} %{content_type}' -H 'Accept-Encoding: br' "$BASE$p")"
	[[ "$code" == 200\ text/html* ]] || fail "SPA $p → $code"
done
cc="$(curl -sS -o /dev/null -D - "$BASE/login" | grep -i '^cache-control:' | tr -d '\r')"
[[ "$cc" == *no-cache* ]] || fail "/login cache-control: $cc"
code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/nope")"
[[ "$code" == 404 ]] || fail "/api 404 → $code"
code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/.well-known/passkey-endpoints")"
[[ "$code" == 200 ]] || fail "passkey-endpoints → $code"
ok "SPA routes, no-cache HTML, /api 404, well-known"

echo "e2e-full OK ($PASS groups passed) — user $USER_EMAIL / $USER_PASSWORD, admin $ADMIN_EMAIL"
