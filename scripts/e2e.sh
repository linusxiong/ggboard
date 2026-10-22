#!/usr/bin/env bash
# Smoke test: build the binary, start it on a random free port with a throwaway data dir and check
# that the server answers /healthz and GET /api/v1/guest/config with 200.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/ggboard-e2e.XXXXXX")"
BIN="$WORK/ggboard"
LOG="$WORK/server.log"
PID=""

cleanup() {
	if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
		kill -TERM "$PID" 2>/dev/null || true
		wait "$PID" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT

free_port() {
	if command -v python3 >/dev/null 2>&1; then
		python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'
	else
		echo $((20000 + RANDOM % 40000))
	fi
}

echo "==> building"
(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$BIN" ./cmd/ggboard)

PORT="$(free_port)"
BASE="http://127.0.0.1:$PORT"
echo "==> starting on $BASE"
GGBOARD_SERVER_ADDR="127.0.0.1:$PORT" \
GGBOARD_DATA_DIR="$WORK/data" \
GGBOARD_JWT_SECRET="e2e-secret-0123456789abcdef0123456789" \
GGBOARD_LOG_LEVEL=warn \
	"$BIN" serve >"$LOG" 2>&1 &
PID=$!

for _ in $(seq 1 50); do
	if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then
		break
	fi
	if ! kill -0 "$PID" 2>/dev/null; then
		echo "server exited early:"; cat "$LOG"; exit 1
	fi
	sleep 0.2
done
curl -fsS "$BASE/healthz" >/dev/null || { echo "server did not become ready"; cat "$LOG"; exit 1; }
echo "==> /healthz ok"

status="$(curl -sS -o "$WORK/config.json" -w '%{http_code}' "$BASE/api/v1/guest/config")"
echo "==> GET /api/v1/guest/config -> $status"
cat "$WORK/config.json"; echo
if [[ "$status" != "200" ]]; then
	echo "FAIL: /api/v1/guest/config must return 200"
	exit 1
fi

spa="$(curl -sS -o /dev/null -w '%{http_code} %{content_type}' "$BASE/")"
echo "==> GET / -> $spa"
[[ "$spa" == 200\ text/html* ]] || { echo "FAIL: SPA root"; exit 1; }

echo "e2e OK"
