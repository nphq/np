#!/usr/bin/env bash
# bun audit 带重试的封装：audit 请求本身是发往 npm registry 的网络调用，
# 偶发 ConnectionClosed / 超时（CI 曾 4m19s 后报 "audit request failed"），
# 与"发现漏洞"是两种完全不同的失败，必须区分处理：
#   - 网络失败 → 重试（默认 3 次，单次 120s 上限），仍失败才判红；
#   - 真发现 high+ 漏洞 → 立刻判红，不重试。
# 用法：bash scripts/bun-audit.sh [--audit-level=high]
set -uo pipefail

cd "$(dirname "$0")/../frontend"

LEVEL="${1:---audit-level=high}"
ATTEMPTS="${BUN_AUDIT_ATTEMPTS:-3}"
PER_ATTEMPT_SECS="${BUN_AUDIT_TIMEOUT:-120}"

for ((i = 1; i <= ATTEMPTS; i++)); do
  out="$(timeout "$PER_ATTEMPT_SECS" bun audit "$LEVEL" 2>&1)"
  code=$?
  if [ "$code" -eq 0 ]; then
    echo "$out"
    echo "OK: bun audit clean ($LEVEL)"
    exit 0
  fi
  # 124 = timeout  kills bun；或明确的请求失败 → 视为网络抖动，可重试
  if [ "$code" -eq 124 ] || echo "$out" | grep -qi "audit request failed"; then
    echo "WARN: bun audit 网络失败 (attempt $i/$ATTEMPTS, exit $code)，重试…" >&2
    echo "$out" | tail -n 5 >&2
    sleep $((i * 10))
    continue
  fi
  # 其他非 0 退出 = 真发现漏洞（advisory 表），直接判红
  echo "$out" >&2
  echo "FAIL: bun audit 发现 $LEVEL 以上漏洞" >&2
  exit 1
done

echo "FAIL: bun audit 请求连续失败 $ATTEMPTS 次（registry 网络），请重跑" >&2
exit 1
