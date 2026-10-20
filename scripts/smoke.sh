#!/usr/bin/env bash
# 三库冒烟：起一个假 GitHub，让灰鸦在指定数据库上跑完「建密钥 → 建规则 → 扫描 → 统计 → 导出」一轮。
#
#   scripts/smoke.sh sqlite   /tmp/smoke.db
#   scripts/smoke.sh mysql    'graycrow:graycrow@tcp(127.0.0.1:13306)/graycrow?charset=utf8mb4&parseTime=True&loc=UTC'
#   scripts/smoke.sh postgres 'host=127.0.0.1 port=15432 user=postgres password=pgpw dbname=graycrow sslmode=disable TimeZone=UTC'
set -euo pipefail

DRIVER=${1:?用法: scripts/smoke.sh <sqlite|mysql|postgres> <dsn>}
DSN=${2:?缺少 DSN}
PORT=${PORT:-18080}
API="http://127.0.0.1:${PORT}/api/v1"
JAR=$(mktemp)
LOG=$(mktemp)

cleanup() {
  [[ -n "${APP_PID:-}" ]] && kill "$APP_PID" 2>/dev/null || true
  [[ -n "${FAKE_PID:-}" ]] && kill "$FAKE_PID" 2>/dev/null || true
}
trap cleanup EXIT

go run ./cmd/fakegithub -addr "127.0.0.1:18081" >"$LOG" 2>&1 &
FAKE_PID=$!
MASTER_KEY=${MASTER_KEY:-smoke-test-master-key-0123456789-abcdefghij} \
  GITHUB_API_BASE=http://127.0.0.1:18081 \
  DB_DRIVER="$DRIVER" DB_DSN="$DSN" PORT="$PORT" \
  go run ./cmd/graycrow >>"$LOG" 2>&1 &
APP_PID=$!

api() { curl -sf -b "$JAR" -c "$JAR" -H 'Content-Type: application/json' -H 'X-Requested-With: graycrow' "$@"; }
jq_get() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval('d'+sys.argv[1]))" "$1"; }

for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break
  sleep 1
done
echo "[$DRIVER] 启动完成: $(curl -s "http://127.0.0.1:${PORT}/healthz")"

api -X POST "$API/auth/login" -d '{"username":"admin","password":"admin123"}' >/dev/null
api -X POST "$API/api-keys" -d '{"name":"smoke","token":"ghp_smoke_token"}' | jq_get "['data']['status']" | xargs echo "[$DRIVER] 密钥探活:"
api -X POST "$API/rules" -d '{"name":"smoke","keywords":["acme.com"],"interval_sec":60,"enabled":true,"dry_run":false}' | jq_get "['data']['id']" >/dev/null

for _ in $(seq 1 40); do
  TOTAL=$(api "$API/stats/overview" | jq_get "['data']['today_new']")
  [[ "$TOTAL" -gt 0 ]] && break
  sleep 1
done

OVERVIEW=$(api "$API/stats/overview")
echo "[$DRIVER] 仪表盘: $(echo "$OVERVIEW" | jq_get "['data']" | head -c 200)"
echo "[$DRIVER] 趋势点数: $(api "$API/stats/trend?range=30d" | jq_get "['data'].__len__()")"
echo "[$DRIVER] 规则分布: $(api "$API/stats/by-rule" | jq_get "['data']" | head -c 120)"
echo "[$DRIVER] 状态分布: $(api "$API/stats/by-status" | jq_get "['data']" | head -c 160)"
echo "[$DRIVER] 额度曲线: $(api "$API/stats/quota?range=7d" | jq_get "['data']" | head -c 160)"
echo "[$DRIVER] 扫描日志: $(api "$API/rules/1/runs" | jq_get "['data']['items'][0]['status']")"
ROWS=$(api "$API/stats/export?type=alerts" | wc -l | tr -d ' ')
echo "[$DRIVER] CSV 导出行数: $ROWS"

if [[ "$TOTAL" -gt 0 && "$ROWS" -gt 1 ]]; then
  echo "[$DRIVER] ✅ 冒烟通过（命中 $TOTAL 条）"
else
  echo "[$DRIVER] ❌ 冒烟失败，日志尾部："
  tail -20 "$LOG"
  exit 1
fi
