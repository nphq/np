package uiapi

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/metrics"
	"github.com/nphq/np/internal/nomad"
)

// LoadsService 承载负载相关的全部 IPC 逻辑。
// 内部持有一个 Collector（绑定当前激活/最近请求的集群），
// 事件经 load.patch 直接推送 LoadPatch（无 Envelope 包装）。
type LoadsService struct {
	pool *cluster.Pool

	ctxMu sync.RWMutex
	ctx   context.Context // Start 注入，nil 时不 emit

	// app 由 SetApp 注入（OnStartup 时），供 EmitEvent 使用；为 nil 时不 emit。
	appMu sync.RWMutex
	app   *application.App

	collMu    sync.Mutex
	coll      *metrics.Collector
	clusterID string
}

// NewLoadsService 创建负载服务。
func NewLoadsService(pool *cluster.Pool) *LoadsService {
	return &LoadsService{pool: pool}
}

// SetApp 注入 *application.App 引用，供 emitLoad 通过 EmitEvent 推送事件。
// 由 App.OnStartup 在 v3 启动阶段调用；早于 Collector 第一轮轮询。
func (s *LoadsService) SetApp(app *application.App) {
	s.appMu.Lock()
	s.app = app
	s.appMu.Unlock()
}

// Start 注入 wails ctx（app.startup 调用）。
func (s *LoadsService) Start(ctx context.Context) {
	s.ctxMu.Lock()
	s.ctx = ctx
	s.ctxMu.Unlock()
}

// Stop 停止当前 Collector（app.shutdown 调用）。
func (s *LoadsService) Stop() {
	s.collMu.Lock()
	defer s.collMu.Unlock()
	s.stopLocked()
}

// Activate 切换 Collector 到指定集群（SetActiveCluster 触发；空串停）。
// 激活路径会启动后台轮询循环；纯请求路径（ensureCollector）不启动，
// 首拉由 refreshSync 同步 tick 兜底。
func (s *LoadsService) Activate(clusterID string) {
	s.collMu.Lock()
	defer s.collMu.Unlock()
	if clusterID == s.clusterID && s.coll != nil {
		return
	}
	s.stopLocked()
	if clusterID == "" {
		s.clusterID = ""
		return
	}
	s.clusterID = clusterID
	s.startLocked(clusterID).Start()
}

// stopLocked 停掉当前 Collector（须持 collMu）。
func (s *LoadsService) stopLocked() {
	if s.coll != nil {
		s.coll.Stop()
		s.coll = nil
	}
}

// startLocked 为集群构造 Collector（须持 collMu；不启动后台循环）。
func (s *LoadsService) startLocked(clusterID string) *metrics.Collector {
	cfg := metrics.DefaultConfig(clusterID)
	coll := metrics.NewCollector(cfg, func() (*api.Client, error) {
		return s.pool.Get(clusterID)
	}, s.emitLoad)
	s.coll = coll
	return coll
}

// emitLoad 是 Collector 的回调：直接推 LoadPatch 给前端（v3 EmitEvent）。
func (s *LoadsService) emitLoad(p metrics.LoadPatch) {
	s.appMu.RLock()
	app := s.app
	s.appMu.RUnlock()
	if app == nil {
		return
	}
	app.Event.Emit("load.patch", p)
}

// ensureCollector 保证 collector 挂在指定集群上（惰性构造，不启动后台循环）。
// 后台循环只在 SetActiveCluster → Activate 时启动；请求路径的首拉
// 由 refreshSync 同步 tick 兜底，二者经 tickMu 串行化、采样按 ms 去重。
func (s *LoadsService) ensureCollector(clusterID string) *metrics.Collector {
	s.collMu.Lock()
	defer s.collMu.Unlock()
	if s.coll != nil && s.clusterID == clusterID {
		return s.coll
	}
	s.stopLocked()
	s.clusterID = clusterID
	return s.startLocked(clusterID)
}

// refreshSync 在缓存为空时同步跑一轮采集（首拉路径）。
func (s *LoadsService) refreshSync(coll *metrics.Collector) {
	if !coll.Cache().Empty() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = coll.Tick(ctx)
}

// GetClusterLoad 返回集群负载聚合快照（Overview 页）。
func (s *LoadsService) GetClusterLoad(clusterID string) (*nomad.ClusterLoad, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	coll := s.ensureCollector(clusterID)
	s.refreshSync(coll)
	cl := coll.Cache().Snapshot()
	return &cl, nil
}

// GetNodeLoads 返回全部节点负载（Nodes 屏首拉）。
func (s *LoadsService) GetNodeLoads(clusterID string) ([]nomad.NodeLoad, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	coll := s.ensureCollector(clusterID)
	s.refreshSync(coll)
	return coll.Cache().NodeLoads(), nil
}

// GetAllocLoad 返回单个 alloc 的 per-task 用量（Job 详情页）。
// 缓存未命中（alloc 非 running / 降级 / 尚未轮询到）返回 CodeNotFound。
func (s *LoadsService) GetAllocLoad(clusterID, allocID string) (*nomad.AllocLoad, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateAllocID(allocID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	coll := s.ensureCollector(clusterID)
	s.refreshSync(coll)
	if al, ok := coll.Cache().AllocLoad(allocID); ok {
		return &al, nil
	}
	return nil, NewError(CodeNotFound, "alloc %s: no usage data (not running or load degraded)", allocID)
}

// NodeLoads 暴露缓存中的节点负载（供 NodesService 派生 NodeSummary）。
func (s *LoadsService) NodeLoads(clusterID string) []nomad.NodeLoad {
	coll := s.ensureCollector(clusterID)
	return coll.Cache().NodeLoads()
}
