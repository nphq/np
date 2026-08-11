package version

// 当前版本。默认 dev；release CI 通过 -ldflags 注入
// （-X github.com/nphq/np/internal/version.Version={{.VERSION}}），
// 移除 ldflags 也不破坏构建（回滚友好，见 review.md 任务一）。
var Version = "dev"

// Build 返回简短版本信息，用于关于对话框 / 设置页。
func Build() string { return Version }
