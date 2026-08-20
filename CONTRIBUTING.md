# Contributing to np

## Wails 升级流程（必读）

升级 Wails 时保持三个来源一致，改完跑一次绑定重生：

1. `go.mod` 的 `github.com/wailsapp/wails/v3` 版本
2. `build/WAILS_VERSION`（版本单一来源，CI / release 都从它装 CLI）
3. README / README.zh-CN 里提到的版本号

CI 用 `scripts/check-version-sync.sh` 强制 1 与 2 一致（CLI 与 runtime 版本漂移
会造成绑定/行为不一致）；本地也可 `wails3 task common:check:version` 校验。

```sh
# 升级 Wails 或增删 bound 方法后必须重生 bindings
wails3 task common:regen-bindings
```

`common:regen-bindings` 会 `wails3 generate bindings -names ./... -clean=true -ts`，
按**方法名（FQN，如 `github.com/nphq/np/internal/app.App.RemoveCluster`）分发**，
不依赖哈希 ID —— 没有 obfuscated 注册表，也不存在 ID 漂移/碰撞问题。
注意服务必须位于非 main 包（见 `internal/app` 包注释）：Wails 生成器对
`package main` 硬编码 `main.` 前缀而运行时按模块路径索引，会按名找不到方法。

重生后验证：

```sh
go build ./...          # Go 侧编译
go test ./...           # app_test.go 的 callBinding 按 FQN 名调用，能抓改名漏改
cd frontend && bun run check && bun run test
wails3 task common:check:bindings   # 临时目录重生 bindings 并断言无漂移
git diff frontend/bindings          # 与预期 diff 一致（不应有意外漂移）
```

## 打包与版本号

- 版本号单一来源：`internal/version.Version`（默认 `"dev"`），生产构建由
  `-ldflags -X ...version.Version={{.VERSION}}` 注入。
- `build/config.yml`、`build/darwin/Info.plist`、`build/darwin/Info.dev.plist`、
  `build/windows/nsis/wails_tools.nsh`、`build/linux/nfpm/nfpm.yaml` 里的
  `__VERSION__` 占位符由 `wails3 task common:render:version` 渲染（`package`
  task 自动执行），打包后 `wails3 task common:restore:version` 还原占位符。
