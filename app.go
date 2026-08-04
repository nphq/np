package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
	"github.com/nphq/np/internal/uiapi"
)

// App 承载全部 Wails bound methods（薄层，转调 uiapi）。
// v3：实现 application.ServiceStartup / ServiceShutdown 接口，
// 在 ServiceStartup 里通过 application.Get() 拿到 *application.App 引用注入给 service。
type App struct {
	ctx      context.Context
	clusters *uiapi.ClusterService
	jobs     *uiapi.JobsService
	nodes    *uiapi.NodesService
	loads    *uiapi.LoadsService
}

// NewApp creates a new App application struct.
func NewApp() *App {
	cfg := config.New(config.MustDefaultPath())
	if err := cfg.Load(); err != nil {
		fmt.Println("WARN: load config:", err)
	}
	clusters := uiapi.NewClusterService(cfg, secure.New())
	loads := uiapi.NewLoadsService(clusters.Pool())
	// 负载 Collector 跟随激活集群启停
	clusters.OnActiveChanged = loads.Activate
	return &App{
		clusters: clusters,
		jobs:     uiapi.NewJobsService(clusters.Pool()),
		nodes:    uiapi.NewNodesService(clusters.Pool(), loads),
		loads:    loads,
	}
}

// ServiceStartup 实现 application.ServiceStartup。在 application.Run 启动时调用。
// 用 application.Get() 拿到自身 app 引用注入给需要 emit 事件的 service。
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
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

// --- jobs ---

// ListJobs 返回集群下的全部 job 摘要。
func (a *App) ListJobs(clusterID string) (any, error) {
	jobs, e := a.jobs.ListJobs(clusterID)
	if e != nil {
		return e, nil
	}
	return jobs, nil
}

// GetJob 返回单个 job 详情。
func (a *App) GetJob(clusterID, jobID string) (any, error) {
	detail, e := a.jobs.GetJob(clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return detail, nil
}

// ListJobAllocations 返回 job 下的 allocation 列表。
func (a *App) ListJobAllocations(clusterID, jobID string) (any, error) {
	allocs, e := a.jobs.ListJobAllocations(clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return allocs, nil
}

// RunJob 部署/更新 job（Parse → Validate → Register）。
func (a *App) RunJob(clusterID, spec, format, namespace string, canonicalize bool) (any, error) {
	res, e := a.jobs.RunJob(clusterID, spec, format, namespace, canonicalize)
	if e != nil {
		return e, nil
	}
	return res, nil
}

// StopJob 停止 job（purge=true 清除历史记录），返回 EvalID。
func (a *App) StopJob(clusterID, jobID string, purge bool) (any, error) {
	evalID, e := a.jobs.StopJob(clusterID, jobID, purge)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// EvaluateJob 强制重新评估 job，返回 EvalID。
func (a *App) EvaluateJob(clusterID, jobID string) (any, error) {
	evalID, e := a.jobs.EvaluateJob(clusterID, jobID)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// ScaleJob 对 task group 扩缩容，返回 EvalID。
func (a *App) ScaleJob(clusterID, jobID, group string, count int) (any, error) {
	evalID, e := a.jobs.ScaleJob(clusterID, jobID, group, count)
	if e != nil {
		return e, nil
	}
	return evalID, nil
}

// RestartAlloc 重启 alloc 的任务（taskName 空=全部）。
func (a *App) RestartAlloc(clusterID, allocID, taskName string) (any, error) {
	if e := a.jobs.RestartAlloc(clusterID, allocID, taskName); e != nil {
		return e, nil
	}
	return nil, nil
}

// StopAlloc 停止 alloc。
func (a *App) StopAlloc(clusterID, allocID string) (any, error) {
	if e := a.jobs.StopAlloc(clusterID, allocID); e != nil {
		return e, nil
	}
	return nil, nil
}

// --- nodes ---

// ListNodes 返回集群下的节点列表（容量 + 实时负载）。
func (a *App) ListNodes(clusterID string) (any, error) {
	nodes, e := a.nodes.ListNodes(clusterID)
	if e != nil {
		return e, nil
	}
	return nodes, nil
}

// --- loads ---

// GetClusterLoad 返回集群负载聚合快照（Overview 页）。
func (a *App) GetClusterLoad(clusterID string) (any, error) {
	cl, e := a.loads.GetClusterLoad(clusterID)
	if e != nil {
		return e, nil
	}
	return cl, nil
}

// GetNodeLoads 返回全部节点负载（Nodes 屏首拉）。
func (a *App) GetNodeLoads(clusterID string) (any, error) {
	nl, e := a.loads.GetNodeLoads(clusterID)
	if e != nil {
		return e, nil
	}
	return nl, nil
}

// GetAllocLoad 返回单个 alloc 的 per-task 用量。
func (a *App) GetAllocLoad(clusterID, allocID string) (any, error) {
	al, e := a.loads.GetAllocLoad(clusterID, allocID)
	if e != nil {
		return e, nil
	}
	return al, nil
}
