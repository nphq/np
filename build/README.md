# Build Directory

Build files and platform packaging assets for Nomad Panel (`np`).

```
build/
  Taskfile.yml          # shared tasks (frontend, bindings, icons)
  config.yml            # Wails project metadata
  appicon.png / .svg
  darwin/               # macOS .app / Info.plist / Taskfile
  windows/              # Windows .exe / NSIS / Taskfile
  linux/                # Linux binary / AppImage / nfpm / Taskfile
```

## Commands (from repo root)

```bash
wails3 build                         # current OS → bin/np[.exe]
wails3 package                       # current OS default package

# Explicit platform tasks
wails3 task darwin:build
wails3 task darwin:package           # .app bundle

wails3 task windows:build            # CGO_ENABLED=0; cross-compile OK
wails3 task windows:package          # NSIS installer (requires makensis)

wails3 task linux:build              # needs CGO + GTK4/WebKitGTK headers
wails3 task linux:create:deb
wails3 task linux:create:rpm
wails3 task linux:create:aur
wails3 task linux:create:appimage
```

## Platform notes

### macOS (`build/darwin`)

- `Info.plist` / `Info.dev.plist` — bundle metadata
- `icons.icns` — generated from `appicon.png` via `common:generate:icons`

### Windows (`build/windows`)

- `icon.ico`, `info.json`, `wails.exe.manifest` — embedded via `.syso`
- `nsis/` — NSIS installer (`project.nsi` + `wails_tools.nsh`)
- Packaging downloads the WebView2 bootstrapper into `nsis/` on demand

### Linux (`build/linux`)

- Requires GTK4 + WebKitGTK 6.0 at build and runtime
- `nfpm/nfpm.yaml` — deb / rpm / Arch packages
- `appimage/` — AppImage helper script used by `wails3 generate appimage`
