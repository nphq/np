package app

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
	"github.com/nphq/np/internal/uiapi"
	"github.com/nphq/np/internal/version"
)

// App 承载全部 Wails bound methods（薄层，转调 uiapi）。
// v3：实现 application.ServiceStartup / ServiceShutdown 接口，
// 在 ServiceStartup 里通过 application.Get() 拿到 *application.App 引用注入给 service。
//
// 注意：服务必须放在非 main 包。Wails v3 生成器对 package main 硬编码 FQN 前缀
// "main"，而运行时按 reflect.PkgPath()（模块路径）索引 —— 二者不一致会导致按名
// 分发（$Call.ByName）找不到方法。放在 internal/app 后两边都是
// "github.com/nphq/np/internal/app"（review P0-1）。
type App struct {
	ctx      context.Context
	clusters *uiapi.ClusterService
	jobs     *uiapi.JobsService
	nodes    *uiapi.NodesService
	loads    *uiapi.LoadsService
	settings *uiapi.SettingsService
	prefs    *config.PrefsStore
}

// NewApp creates a new App application struct.
// 配置加载失败仅告警（stderr）不阻断启动：空配置仍可进空状态页添加集群。
func NewApp() *App {
	cfg := config.New(config.MustDefaultPath())
	if err := cfg.Load(); err != nil {
		log.Printf("WARN: load config: %v", err)
	}
	prefs := config.NewPrefs(config.MustPrefsPath())
	if err := prefs.Load(); err != nil {
		log.Printf("WARN: load prefs: %v", err)
	}
	clusters := uiapi.NewClusterService(cfg, prefs, secure.New())
	loads := uiapi.NewLoadsService(clusters.Pool())
	// 负载 Collector 跟随激活集群启停
	clusters.OnActiveChanged = loads.Activate
	settings := uiapi.NewSettingsService(prefs, clusters, loads)
	// 启动即应用已存轮询间隔（早于 monitor 第一轮探测）。
	if st, err := prefs.GetSettings(); err == nil {
		clusters.SetHealthInterval(st.HealthIntervalSec)
		loads.SetMetricsInterval(st.MetricsIntervalSec)
		// AutoRestore 关时不恢复上次活跃（保持空状态）。
		if st.AutoRestoreActive {
			clusters.RestoreActive()
		}
	} else {
		// 恢复上次活跃集群（prefs 有值且集群仍存在时触发 OnActiveChanged → Loads 拉数）
		clusters.RestoreActive()
	}
	return &App{
		clusters: clusters,
		jobs:     uiapi.NewJobsService(clusters.Pool()),
		nodes:    uiapi.NewNodesService(clusters.Pool(), loads),
		loads:    loads,
		settings: settings,
		prefs:    prefs,
	}
}

// ServiceStartup 实现 application.ServiceStartup。在 application.Run 启动时调用。
// 用 application.Get() 拿到自身 app 引用注入给需要 emit 事件的 service。
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	app := application.Get()
	a.clusters.SetApp(app)
	a.loads.SetApp(app)
	a.clusters.Start(ctx)
	a.loads.Start(ctx)
	return nil
}

// ServiceShutdown 实现 application.ServiceShutdown。在 app 退出前调用。
func (a *App) ServiceShutdown() error {
	a.loads.Stop()
	a.clusters.Stop()
	a.clusters.Pool().Close()
	return nil
}

// 返回值约定：所有 bound 方法返回 (any, error)。
// 错误以 *uiapi.Error 形式放在 data 槽（第一个返回值），error 槽恒为 nil。
// 这是 Wails dispatcher 的妥协：它对返回值做 `.(error)` 类型断言，命中就当
// error 处理；nil *uiapi.Error 是 typed-nil，断言拿到非 nil 接口，后续 .Error()
// 调用即使用 nil-safe 实现也会把成功路径误判为错误。把错误塞 data 槽绕开此问题。
//
// ctx 注入：v3 的 bound method 首参为 context.Context 时由运行时注入
// （needsContext），并自动透传到 SDK 调用（QueryOptions.WithContext），
// 使 IPC 请求受 app 生命周期 ctx 约束。

// --- clusters ---

// ListClusters 返回全部集群及健康状态。
func (a *App) ListClusters() (any, error) {
	clusters, e := a.clusters.ListClusters()
	if e != nil {
		return e, nil
	}
	return clusters, nil
}

// AddCluster 新增集群（token 只进 Keychain）。
func (a *App) AddCluster(in uiapi.ClusterInput) (any, error) {
	if e := a.clusters.AddCluster(in); e != nil {
		return e, nil
	}
	return nil, nil
}

// RemoveCluster 删除集群。
func (a *App) RemoveCluster(clusterID string) (any, error) {
	if e := a.clusters.RemoveCluster(clusterID); e != nil {
		return e, nil
	}
	return nil, nil
}

// TestConnection 手动探测连通性。
func (a *App) TestConnection(clusterID string) (any, error) {
	h, e := a.clusters.TestConnection(clusterID)
	if e != nil {
		return e, nil
	}
	return h, nil
}

// TestConnectionInput 用未落盘的入参探测连通性（添加前的 Test 按钮）。
func (a *App) TestConnectionInput(in uiapi.ClusterInput) (any, error) {
	h, e := a.clusters.TestConnectionInput(in)
	if e != nil {
		return e, nil
	}
	return h, nil
}

// UpdateCluster 编辑集群配置（token 空保留旧值）。
func (a *App) UpdateCluster(in uiapi.ClusterInput) (any, error) {
	if e := a.clusters.UpdateCluster(in); e != nil {
		return e, nil
	}
	return nil, nil
}

// SetActiveCluster 激活集群。
func (a *App) SetActiveCluster(clusterID string) (any, error) {
	if e := a.clusters.SetActiveCluster(clusterID); e != nil {
		return e, nil
	}
	return nil, nil
}

// PinCluster 置顶/取消置顶集群（收藏）。
func (a *App) PinCluster(clusterID string, pinned bool) (any, error) {
	if e := a.clusters.PinCluster(clusterID, pinned); e != nil {
		return e, nil
	}
	return nil, nil
}

// DiscoverClusters 探测本机可用连接候选（环境变量/常见配置）。
func (a *App) DiscoverClusters() (any, error) {
	list, e := a.clusters.DiscoverClusters()
	if e != nil {
		return e, nil
	}
	return list, nil
}

// ImportFromEnv 从 NOMAD_* 环境变量一键导入并激活集群。
func (a *App) ImportFromEnv(name string) (any, error) {
	info, e := a.clusters.ImportFromEnv(name)
	if e != nil {
		return e, nil
	}
	return info, nil
}

// --- jobs ---

// ListJobs 返回集群下的全部 job 摘要。
func (a *App) ListJobs(ctx context.Context, clusterID string) (any, error) {
	jobs, e := a.jobs.ListJobs(ctx, clusterID)
	if e != nil {
		return e, nil
	}
	return jobs, nil
}

// GetJob 返回单个 job 详情。
func (a *App) GetJob(ctx context.Context, clusterID, jobID string) (any, error) {
	detail, e := a.jobs.GetJob(ctx, clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return detail, nil
}

// ListJobAllocations 返回 job 下的 allocation 列表。
func (a *App) ListJobAllocations(ctx context.Context, clusterID, jobID string) (any, error) {
	allocs, e := a.jobs.ListJobAllocations(ctx, clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return allocs, nil
}

// RunJob 部署/更新 job（Parse → Validate → Register）。
func (a *App) RunJob(ctx context.Context, clusterID, spec, format, namespace string, canonicalize bool) (any, error) {
	res, e := a.jobs.RunJob(ctx, clusterID, spec, format, namespace, canonicalize)
	if e != nil {
		return e, nil
	}
	return res, nil
}

// StopJob 停止 job（purge=true 清除历史记录），返回 EvalID。
func (a *App) StopJob(ctx context.Context, clusterID, jobID string, purge bool) (any, error) {
	evalID, e := a.jobs.StopJob(ctx, clusterID, jobID, purge)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// EvaluateJob 强制重新评估 job，返回 EvalID。
func (a *App) EvaluateJob(ctx context.Context, clusterID, jobID string) (any, error) {
	evalID, e := a.jobs.EvaluateJob(ctx, clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// ScaleJob 对 task group 扩缩容，返回 EvalID。
func (a *App) ScaleJob(ctx context.Context, clusterID, jobID, group string, count int) (any, error) {
	evalID, e := a.jobs.ScaleJob(ctx, clusterID, jobID, group, count)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// RestartAlloc 重启 alloc 的任务（taskName 空=全部）。
func (a *App) RestartAlloc(ctx context.Context, clusterID, allocID, taskName string) (any, error) {
	if e := a.jobs.RestartAlloc(ctx, clusterID, allocID, taskName); e != nil {
		return e, nil
	}
	return nil, nil
}

// StopAlloc 停止 alloc。
func (a *App) StopAlloc(ctx context.Context, clusterID, allocID string) (any, error) {
	if e := a.jobs.StopAlloc(ctx, clusterID, allocID); e != nil {
		return e, nil
	}
	return nil, nil
}

// GetEvaluation 返回评估状态（部署进度）。
func (a *App) GetEvaluation(ctx context.Context, clusterID, evalID string) (any, error) {
	info, e := a.jobs.GetEvaluation(ctx, clusterID, evalID)
	if e != nil {
		return e, nil
	}
	return info, nil
}

// ListAllocTaskEvents 返回 alloc 任务事件时间线。
func (a *App) ListAllocTaskEvents(ctx context.Context, clusterID, allocID string) (any, error) {
	events, e := a.jobs.ListAllocTaskEvents(ctx, clusterID, allocID)
	if e != nil {
		return e, nil
	}
	return events, nil
}

// GetAllocLogs 拉取 alloc 任务日志快照。
func (a *App) GetAllocLogs(ctx context.Context, clusterID, allocID, task, logType string) (any, error) {
	logs, e := a.jobs.GetAllocLogs(ctx, clusterID, allocID, task, logType)
	if e != nil {
		return e, nil
	}
	return logs, nil
}

// --- nodes ---

// ListNodes 返回集群下的节点列表（容量 + 实时负载）。
func (a *App) ListNodes(ctx context.Context, clusterID string) (any, error) {
	nodes, e := a.nodes.ListNodes(ctx, clusterID)
	if e != nil {
		return e, nil
	}
	return nodes, nil
}

// ListClusterAllocations 返回集群内的全量 allocation（Allocs 页）。
func (a *App) ListClusterAllocations(ctx context.Context, clusterID string) (any, error) {
	allocs, e := a.jobs.ListAllocations(ctx, clusterID)
	if e != nil {
		return e, nil
	}
	return allocs, nil
}

// --- loads ---

// GetClusterLoad 返回集群负载聚合快照（Overview 页）。
func (a *App) GetClusterLoad(ctx context.Context, clusterID string) (any, error) {
	cl, e := a.loads.GetClusterLoad(ctx, clusterID)
	if e != nil {
		return e, nil
	}
	return cl, nil
}

// GetNodeLoads 返回全部节点负载（Nodes 屏首拉）。
func (a *App) GetNodeLoads(ctx context.Context, clusterID string) (any, error) {
	nl, e := a.loads.GetNodeLoads(ctx, clusterID)
	if e != nil {
		return e, nil
	}
	return nl, nil
}

// GetAllocLoad 返回单个 alloc 的 per-task 用量。
func (a *App) GetAllocLoad(ctx context.Context, clusterID, allocID string) (any, error) {
	al, e := a.loads.GetAllocLoad(ctx, clusterID, allocID)
	if e != nil {
		return e, nil
	}
	return al, nil
}

// --- meta ---

// GetVersion 返回构建时注入的版本号（未注入时为 "dev"）。
func (a *App) GetVersion() (any, error) {
	return version.Build(), nil
}

// --- settings ---

// GetSettings 返回通用设置（归一化默认值）。
func (a *App) GetSettings() (any, error) {
	st, e := a.settings.GetSettings()
	if e != nil {
		return e, nil
	}
	return st, nil
}

// UpdateSettings 校验并落盘通用设置，同时热更新轮询间隔。
func (a *App) UpdateSettings(in uiapi.SettingsInput) (any, error) {
	if e := a.settings.UpdateSettings(in); e != nil {
		return e, nil
	}
	return nil, nil
}

// ResetSettings 恢复出厂设置并返回。
func (a *App) ResetSettings() (any, error) {
	st, e := a.settings.ResetSettings()
	if e != nil {
		return e, nil
	}
	return st, nil
}

// GetConfigPaths 返回配置文件路径（诊断/备份用）。
func (a *App) GetConfigPaths() (any, error) {
	p, e := a.settings.GetConfigPaths()
	if e != nil {
		return e, nil
	}
	return p, nil
}
