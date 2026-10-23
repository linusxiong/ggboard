#!/usr/bin/env bash
# make bench: start a throwaway ggboard (temp data dir, free port >= 20000, SQLite, log level info, mail driver
# "log"), seed it and run the load generator (scripts/bench), then always stop the server.
#
#   scripts/bench/run.sh                     # full run, writes docs/perf/results.json, results.md, benchmark.svg
#   scripts/bench/run.sh -d 10s -only login  # extra arguments go to the load generator (go run ./scripts/bench -h)
#   BIN=/path/to/ggboard scripts/bench/run.sh
#   KEEP_LOG=1 scripts/bench/run.sh          # keep the temp dir (server log, database) afterwards
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN="${BIN:-$ROOT/bin/ggboard}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/ggboard-bench.XXXXXX")"
PID=""

cleanup() {
	local code=$?
	if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
		kill -TERM "$PID" 2>/dev/null || true
		wait "$PID" 2>/dev/null || true
	fi
	if [[ $code -ne 0 && -f "$WORK/server.log" ]]; then
		echo "---- server log (tail) ----" >&2
		tail -n 40 "$WORK/server.log" >&2
	fi
	if [[ "${KEEP_LOG:-0}" == "1" ]]; then
		echo "kept $WORK" >&2
	else
		rm -rf "$WORK"
	fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

[[ -x "$BIN" ]] || { echo "no server binary at $BIN (run make build)" >&2; exit 1; }
(cd "$ROOT" && go build -o "$WORK/bench" ./scripts/bench)

PORT="$("$WORK/bench" -free-port)"
BASE="http://127.0.0.1:$PORT"
ADMIN_EMAIL="admin@example.com"
BENCH_ADMIN_PASSWORD="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
export BENCH_ADMIN_PASSWORD

# The panel trusts X-Forwarded-For from 127.0.0.1, as behind a local nginx: the login scenario sends one client
# address per request, so the per-IP passport limit applies per simulated client instead of to the load generator.
cat >"$WORK/config.yaml" <<EOF
server:
  addr: "127.0.0.1:$PORT"
  trusted_proxies: ["127.0.0.1"]
data_dir: "$WORK/data"
log:
  level: info
  format: text
  color: never
EOF

"$BIN" --config "$WORK/config.yaml" admin create --email "$ADMIN_EMAIL" --password "$BENCH_ADMIN_PASSWORD" >/dev/null
"$BIN" --config "$WORK/config.yaml" serve --color never >"$WORK/server.log" 2>&1 &
PID=$!
for _ in $(seq 1 100); do
	curl -fsS "$BASE/healthz" >/dev/null 2>&1 && break
	kill -0 "$PID" 2>/dev/null || { echo "server exited early" >&2; exit 1; }
	sleep 0.1
done
echo "==> server pid $PID on $BASE (data $WORK/data)" >&2

COMMIT="$(git -C "$ROOT" describe --always --dirty --exclude '*' 2>/dev/null || echo unknown)"
(cd "$ROOT" && "$WORK/bench" -base "$BASE" -admin-email "$ADMIN_EMAIL" -server-pid "$PID" -server-log "$WORK/server.log" -commit "$COMMIT" -out docs/perf "$@")
