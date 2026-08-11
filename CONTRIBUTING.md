# Contributing to np

## Wails 升级流程（必读）

升级 Wails 时保持三个来源一致，改完跑一次绑定重生：

1. `go.mod` 的 `github.com/wailsapp/wails/v3` 版本
2. `build/WAILS_VERSION`（版本单一来源，CI / release 都从它装 CLI）
3. README / README.zh-CN 里提到的版本号

```sh
# 升级 Wails 后必须重生 bindings + obfuscated IDs
wails3 task common:regen-bindings
```

`common:regen-bindings` 会：

1. `wails3 generate bindings -obfuscated ./... -clean=true -ts` 重新生成
   `frontend/bindings/**` 与 `wails_obfuscated.gen.go`；
2. 剥掉 `wails_obfuscated.gen.go` 的 `//go:build wails_obfuscated` tag ——
   这是 Wails v3 beta.3+ 的 workaround：generator 对 `package main` 发
   `main.App.*` ID，而 runtime 按真实 module path 哈希；剥 tag 后绑定的
   ID 在普通构建里保持稳定。

重生后验证：

```sh
go build ./...          # Go 侧编译
cd frontend && bun run check && bun run test
git diff frontend/bindings  # 与预期 diff 一致（不应有意外漂移）
```

## 打包与版本号

- 版本号单一来源：`internal/version.Version`（默认 `"dev"`），生产构建由
  `-ldflags -X ...version.Version={{.VERSION}}` 注入。
- `build/config.yml`、`build/darwin/Info.plist`、`build/darwin/Info.dev.plist`、
  `build/windows/nsis/wails_tools.nsh`、`build/linux/nfpm/nfpm.yaml` 里的
  `__VERSION__` 占位符由 `wails3 task common:render:version` 渲染（`package`
  task 自动执行），打包后 `wails3 task common:restore:version` 还原占位符。
