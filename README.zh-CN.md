# Nomad Panel

面向 [HashiCorp Nomad](https://www.nomadproject.io/) 集群的原生桌面管理工具。

基于 **Wails v3 + Go + Svelte 5 + TypeScript** 构建，追求快速响应、键盘优先操作、多集群舒适管理。

## 功能特性

- **多集群管理** — 添加 / 编辑 / 探测 / 切换集群，后台轮询健康状态；Token 仅存放于系统钥匙串，永不落盘
- **集群总览** — CPU / 内存 / 磁盘三段用量环图，已分配 vs 实时负载对比，节点健康状态与 Top N 资源消耗者
- **节点** — 每节点容量 / 已分配 / 实时用量进度条 + 60 点历史曲线；节点详情页展示其下的所有 Allocation
- **Job** — 列表（运行 / 排队 / 失败状态徽标）+ 详情（Task Group 与 Allocation 层级展开）
- **Job 操作** — 部署（HCL/JSON -> Parse -> Validate -> Register 流水线）、停止（可选 purge）、扩缩容 Task Group、强制评估、重启 / 停止 Allocation；所有写操作均需二次确认
- **容器与非容器** — 「快速创建」面向 Docker；「高级编辑」起步模板与「应用 → 原生」覆盖 `exec` / `raw_exec`（见下文）
- **实时推送** — 负载通过 Nomad Event Stream 增量推送（`load.patch`），非轮询重刷
- **国际化** — 简体中文 / English 标题栏一键切换（默认中文）

## 环境要求

- **Go** ≥ 1.26
- **Bun** ≥ 1.3（唯一包管理器，请勿混用 npm）
- **Wails CLI v3** — `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.3`
- **Nomad** 集群（≥ 2.0；已基于 `hashicorp/nomad:2.0.4` 验证）

## 快速开始

```bash
# 1. 安装前端依赖
cd frontend && bun install && cd ..

# 2. 开发模式启动（Vite HMR，端口 9245）
wails3 dev

# 3. 当前系统生产构建
wails3 build        # 输出 bin/np（Windows 为 bin/np.exe）

# 可选：打包安装包（需各平台工具）
wails3 package                    # 当前 OS 默认打包
wails3 task windows:package       # NSIS 安装包（需 makensis）
wails3 task linux:create:deb      # .deb
wails3 task linux:create:appimage # AppImage
```

本地调试可启动 dev agent：

```bash
nomad agent -dev          # http://127.0.0.1:4646
# 或使用 Docker
docker compose up -d
```

启动应用后点击 **添加集群**，填入 `http://127.0.0.1:4646` 即可连接。

## 部署非容器（原生）应用

Nomad 可直接在宿主机上跑二进制，不必依赖 Docker。本应用中的入口：

| 路径 | 作用 |
| --- | --- |
| **应用 → 原生** | 精选 `exec` / `raw_exec` 样例；可一键部署或自定义 |
| **运行任务 → 高级编辑** | 起步模板：Docker、`exec`、`raw_exec` |

**前提条件**

- **`exec`** — 目标节点上须已有可执行文件（或用 `artifact` 拉取）。隔离性好于 `raw_exec`。
- **`raw_exec`** — 几乎无隔离；须在 Nomad client 配置中显式启用：

```hcl
plugin "raw_exec" {
  config {
    enabled = true
  }
}
```

`nomad agent -dev` 本地开发通常已启用常用驱动；生产环境常见做法是关闭 `raw_exec`。请将 job 的 `command` 指向调度节点上真实存在的路径（例如 `/usr/bin/python3`）。

其他驱动（`java`、`podman` 等）可在「高级编辑」中粘贴完整 HCL/JSON——面板会提交 Nomad 能接受的任意规格。

## 测试

```bash
go test ./...                # Go 单测（不含 e2e）
scripts/e2e.sh               # e2e：通过 Docker 运行真实 Nomad
cd frontend && bun run test  # 前端 store 测试（Vitest）
cd frontend && bun run check # svelte-check（类型检查）
cd frontend && bun run lint  # eslint
```

`internal/e2e` 测试套件依赖 `hashicorp/nomad` Docker 容器，通过 `e2e` build tag 隔离——普通 `go test ./...` 不会执行它。环境开关详见 `scripts/e2e.sh`（`NOMAD_IMAGE` / `E2E_RUN=0` / `-short`）。

## 架构

```
┌─────────────────────────────────────────┐
│  Svelte 5 frontend (runes, Tailwind 4)  │
│  stores ↔ generated Wails bindings      │
└──────────────┬──────────────────────────┘
               │ IPC (bound methods + events)
┌──────────────▼──────────────────────────┐
│  Go backend                             │
│  app.go (thin bound methods)            │
│  internal/uiapi   — IPC-facing services │
│  internal/nomad   — Nomad SDK → DTOs    │
│  internal/cluster — client pool, health │
│  internal/metrics — load collector      │
│  internal/config  — clusters.json       │
│  internal/secure  — OS keychain         │
└─────────────────────────────────────────┘
```

## 项目状态

里程碑 M0–M3c 已完成（工程骨架、连接层、总览 / 节点 / Job、指标流水线、Job 管理、部署观测）。Darwin / Windows / Linux 构建与打包已通过 Wails Taskfile 支持（`wails3 build` / `wails3 package`）。当前依赖 **Wails v3.0.0-beta.3**。

## 贡献

欢迎提交 Issue 与 PR。提交前请确保：

1. `go test ./...` 与 `scripts/e2e.sh` 通过
2. `frontend/` 下 `bun run check && bun run lint && bun run format:check` 通过
3. `golangci-lint run` 无报错（见 `.golangci.yml`）
4. 可用 `task lint` 一键跑与 CI 等价的质量门禁

Hooks：`lefthook` 在 commit 时跑 Prettier/ESLint 与 `golangci-lint fmt` + `golangci-lint run`（由 `frontend` 的 `prepare` 安装）。

## 许可证

[Apache License 2.0](LICENSE)
