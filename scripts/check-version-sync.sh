#!/usr/bin/env bash
# 校验 build/WAILS_VERSION 与 go.mod 中的 wails/v3 依赖版本一致。
#
# 根因（review P0-2）：wails3 CLI 负责生成 bindings、wails/v3 库负责运行时，
# 两者版本不一致会造成绑定/行为漂移（例如 CLI 生成的新方法在旧 runtime 上不可用）。
# build/WAILS_VERSION 是 CLI 版本单一来源，go.mod 是 runtime 版本单一来源，
# 本脚本强制二者相等。
set -euo pipefail

cd "$(dirname "$0")/.."

wails_version="$(tr -d '[:space:]' < build/WAILS_VERSION)"
go_mod_version="$(sed -n 's/^[[:space:]]*github.com\/wailsapp\/wails\/v3[[:space:]]*v\(.*\)$/v\1/p' go.mod | head -1 | tr -d '[:space:]')"

if [ -z "$go_mod_version" ]; then
  echo "FAIL: cannot find github.com/wailsapp/wails/v3 version in go.mod" >&2
  exit 1
fi

if [ "$wails_version" != "$go_mod_version" ]; then
  echo "FAIL: build/WAILS_VERSION ($wails_version) != go.mod wails/v3 ($go_mod_version)" >&2
  echo "      wails3 CLI 版本必须与运行时依赖一致；升级时同步修改两处：" >&2
  echo "      build/WAILS_VERSION 和 go.mod 的 require 行" >&2
  exit 1
fi

echo "OK: wails version in sync ($wails_version)"
