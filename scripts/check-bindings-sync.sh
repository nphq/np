#!/usr/bin/env bash
# 在临时目录重新生成 Wails bindings（name 分发），断言与已提交的 frontend/bindings 一致。
#
# 防止（review P0-1）：改了 bound 方法/入参后忘记重新生成 frontend/bindings，
# 导致前端调用签名与 Go 侧脱节。生成到临时目录，不改动仓库内文件。
# 依赖：wails3 CLI（版本应与 go.mod 一致，见 check-version-sync.sh）。
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

if ! wails3 generate bindings -names ./... -clean=true -ts -d "$tmp_dir" >/dev/null 2>&1; then
  echo "FAIL: wails3 generate bindings failed (is the CLI installed and matching go.mod?)" >&2
  exit 1
fi

if diff -r -q frontend/bindings "$tmp_dir" >/dev/null 2>&1; then
  echo "OK: frontend/bindings in sync with Go code"
else
  echo "FAIL: frontend/bindings 与 Go 代码不一致" >&2
  echo "      请运行: wails3 task common:regen-bindings 并提交变更" >&2
  echo "" >&2
  diff -r frontend/bindings "$tmp_dir" | head -60 >&2 || true
  exit 1
fi
