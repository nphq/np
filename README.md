# Nomad Panel

A desktop client for [HashiCorp Nomad](https://www.nomadproject.io/).

Built with **Wails v3 + Go + Svelte 5 + TypeScript**, designed to be fast, keyboard-first, and pleasant to use with multiple clusters.

## Features

- **Multi-cluster management** — add, edit, probe, and switch between clusters; health status polled in the background. Tokens live in the OS keychain only, never on disk.
- **Cluster overview** — CPU / memory / disk donut gauges with allocated vs. real-time usage, node health, and top resource consumers.
- **Nodes** — capacity / allocated / used bars per node with 60-point sparkline history, plus per-node detail with its allocations.
- **Jobs** — list with status badges (running / queued / failed), detail view with task groups and allocations.
- **Job operations** — run (HCL/JSON spec with Parse → Validate → Register pipeline), stop (with optional purge), scale task groups, force evaluate, restart / stop allocations. All write actions require confirmation.
- **Container & native workloads** — Quick create builds Docker jobs; Advanced HCL starters and Apps → Native cover `exec` / `raw_exec` (see below).
- **Live updates** — metrics stream in via events (`load.patch`), not full reloads.
- **i18n** — Simplified Chinese and English UI, switchable in the title bar (defaults to Chinese).

## Requirements

- **Go** ≥ 1.26
- **Bun** ≥ 1.3 (the only package manager used; do not use npm)
- **Wails CLI v3** — `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.3`
- A **Nomad** cluster (≥ 2.0; verified against `hashicorp/nomad:2.0.4`)

## Getting Started

```bash
# 1. Install frontend dependencies
cd frontend && bun install && cd ..

# 2. Run in dev mode (Vite HMR on port 9245)
wails3 dev

# 3. Build a production binary for the current OS
wails3 build        # outputs bin/np (or bin/np.exe on Windows)

# Optional: package installers (platform-specific tooling required)
wails3 package                    # current OS default package
wails3 task windows:package       # NSIS installer (needs makensis)
wails3 task linux:create:deb      # .deb (needs nfpm via wails3)
wails3 task linux:create:appimage # AppImage
```

For a local playground, start a dev agent:

```bash
nomad agent -dev          # http://127.0.0.1:4646
# or via docker compose:
docker compose up -d
```

Then open the app, click **Add Cluster**, and point it at `http://127.0.0.1:4646`.

## Deploying non-container (native) jobs

Nomad can run binaries on the host without Docker. In this app:

| Path | What it does |
| --- | --- |
| **Apps → Native** | Curated `exec` / `raw_exec` samples; Deploy or Customize |
| **Run Job → Advanced** | Starter chips: Docker, `exec`, `raw_exec` |

**Prerequisites**

- **`exec`** — binary must exist on the client node (or use an `artifact` stanza to download it). Isolation is stronger than `raw_exec`.
- **`raw_exec`** — almost no isolation; must be enabled on the Nomad client:

```hcl
plugin "raw_exec" {
  config {
    enabled = true
  }
}
```

`nomad agent -dev` typically enables common drivers for local experiments; production clients often leave `raw_exec` disabled. Point the job’s `command` at a path that exists on the scheduled nodes (e.g. `/usr/bin/python3`).

For other drivers (`java`, `podman`, …), paste a full job HCL/JSON in **Advanced** — the panel submits whatever Nomad accepts.

## Testing

```bash
go test ./...                # Go unit tests (no e2e tag)
scripts/e2e.sh               # e2e: real Nomad via Docker (falls back to local binary)
cd frontend && bun run test  # frontend store tests (Vitest)
cd frontend && bun run check # svelte-check
cd frontend && bun run lint  # eslint
```

The `internal/e2e` suite runs against a real `hashicorp/nomad` container and is isolated behind the `e2e` build tag — plain `go test ./...` skips it. See `scripts/e2e.sh` for environment toggles (`NOMAD_IMAGE`, `E2E_RUN=0`, `-short`).

## Architecture

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

## Project Status

Milestones M0–M3c are complete (scaffolding, connection layer, overview/nodes/jobs screens, metrics pipeline, job management, deploy observability). Darwin / Windows / Linux builds and packaging are supported via Wails Taskfiles (`wails3 build` / `wails3 package`). The project depends on **Wails v3.0.0-beta.3**.

## Contributing

Issues and pull requests are welcome. Before submitting a PR:

1. `go test ./...` and `scripts/e2e.sh` pass
2. `bun run check && bun run lint && bun run format:check` pass in `frontend/`
3. `golangci-lint run` reports no issues (see `.golangci.yml`)
4. Prefer `task lint` for a single CI-equivalent quality gate

Hooks: `lefthook` runs Prettier/ESLint and `golangci-lint fmt` + `golangci-lint run` on commit (installed via `frontend` `prepare`).

## License

[Apache License 2.0](LICENSE)
