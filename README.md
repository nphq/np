# Nomad Panel

A desktop client for [HashiCorp Nomad](https://www.nomadproject.io/).

Built with **Wails v3 + Go + Svelte 5 + TypeScript**, designed to be fast, keyboard-first, and pleasant to use with multiple clusters.

## Features

- **Multi-cluster management** — add, edit, probe, and switch between clusters; health status polled in the background. Tokens live in the OS keychain only, never on disk.
- **Cluster overview** — CPU / memory / disk donut gauges with allocated vs. real-time usage, node health, and top resource consumers.
- **Nodes** — capacity / allocated / used bars per node with 60-point sparkline history, plus per-node detail with its allocations.
- **Jobs** — list with status badges (running / queued / failed), detail view with task groups and allocations.
- **Job operations** — run (HCL/JSON spec with Parse → Validate → Register pipeline), stop (with optional purge), scale task groups, force evaluate, restart / stop allocations. All write actions require confirmation.
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

# 3. Build a production binary
wails3 build        # outputs bin/nomad-manager
```

For a local playground, start a dev agent:

```bash
nomad agent -dev          # http://127.0.0.1:4646
# or via docker compose:
docker compose up -d
```

Then open the app, click **Add Cluster**, and point it at `http://127.0.0.1:4646`.

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

Milestones M0–M3c are complete (scaffolding, connection layer, overview/nodes/jobs read-only screens, metrics pipeline, job management operations). Log viewer (M3) and packaging polish (M4/M5) are on the roadmap. The project depends on **Wails v3.0.0-beta.3**.

## Contributing

Issues and pull requests are welcome. Before submitting a PR:

1. `go test ./...` and `scripts/e2e.sh` pass
2. `bun run check && bun run lint && bun run format:check` pass in `frontend/`
3. `golangci-lint run` reports no issues

## License

[Apache License 2.0](LICENSE)
