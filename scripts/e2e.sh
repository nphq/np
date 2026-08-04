#!/usr/bin/env bash
# 运行 e2e 测试：通过 Docker 或本地 nomad 二进制拉起 hashicorp/nomad server，跑 internal/e2e。
# 用法：
#   scripts/e2e.sh             # 默认 hashicorp/nomad:2.0.4
#   NOMAD_IMAGE=hashicorp/nomad:1.9.4 scripts/e2e.sh
#   scripts/e2e.sh -short      # 跳过真 Nomad 容器/进程启动，仅编译或跑基本测试
set -euo pipefail

cd "$(dirname "$0")/.."

# 支持 E2E_RUN=0 / E2E_RUN=false 跳过运行整个脚本
if [ "${E2E_RUN:-true}" = "false" ] || [ "${E2E_RUN:-1}" = "0" ]; then
  echo "==> E2E_RUN is disabled (false/0). Skipping E2E test execution."
  exit 0
fi

short_mode=false
for arg in "$@"; do
  if [ "$arg" = "-short" ]; then
    short_mode=true
  fi
done

if [ "$short_mode" = "false" ]; then
  has_docker=true
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    has_docker=false
  fi

  has_nomad=true
  if ! command -v nomad >/dev/null 2>&1; then
    has_nomad=false
  fi

  if [ "$has_docker" = "false" ] && [ "$has_nomad" = "false" ]; then
    echo "ERROR: Neither docker daemon nor nomad binary is available." >&2
    exit 1
  fi

  export NOMAD_IMAGE="${NOMAD_IMAGE:-hashicorp/nomad:2.0.4}"

  if [ "$has_docker" = "true" ]; then
    echo "==> Using Docker image: NOMAD_IMAGE=$NOMAD_IMAGE"
  else
    echo "==> Docker not available. Using local nomad binary: $(command -v nomad)"
  fi
else
  echo "==> Short mode enabled. Skipping Nomad daemon checks and startup."
fi

go test -tags=e2e -v -timeout 5m ./internal/e2e/... "$@"
