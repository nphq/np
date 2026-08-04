package uiapi

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/nomad"
	"github.com/nphq/np/internal/secure"
)

// ClusterInput 是 AddCluster 的前端入参。Token 只进 Keychain，绝不落盘。
type ClusterInput struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Address            string `json:"address"`
	Region             string `json:"region"`
	Namespace          string `json:"namespace"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
	Token              string `json:"token"` // 可选：留空则沿用 Keychain 已有 token
}

// Validate 一次性收集全部校验错误，方便前端内联展示（一次提示完所有问题）。
func (in ClusterInput) Validate() *Error {
	var errs []string
	if err := ValidateClusterID(in.ID); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ValidateClusterName(in.Name); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := ValidateAddress(in.Address); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ValidateNamespace(in.Namespace); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ValidateRegion(in.Region); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return NewError(CodeInvalidInput, "%s", strings.Join(errs, "; "))
}

// ClusterService 承载集群相关的全部 IPC 逻辑。
type ClusterService struct {
	cfg     *config.Store
	keyring secure.Keyring
	pool    *cluster.Pool
	monitor *cluster.HealthMonitor

	// ctx 由 Start 注入，供 monitor 后台循环使用；为 nil 时表示尚未启动。
	ctxMu sync.RWMutex
	ctx   context.Context

	// app 由 SetApp 注入（OnStartup 时），供 EmitEvent 使用；为 nil 时不 emit。
	appMu sync.RWMutex
	app   *application.App

	activeMu sync.RWMutex
	active   string

	// OnActiveChanged 在 Start 之前赋值；active 切换时同步触发。
	OnActiveChanged func(clusterID string)
}

// SetApp 注入 *application.App 引用，供 emitHealth 通过 EmitEvent 推送事件。
// 由 App.OnStartup 在 v3 启动阶段调用；早于 monitor 第一轮探测。
func (s *ClusterService) SetApp(app *application.App) {
	s.appMu.Lock()
	s.app = app
	s.appMu.Unlock()
}

// NewClusterService 创建集群服务，并挂上健康 monitor（Run 由 Start 触发）。
func NewClusterService(cfg *config.Store, kr secure.Keyring) *ClusterService {
	s := &ClusterService{
		cfg:     cfg,
		keyring: kr,
		pool:    cluster.NewPool(cfg, kr),
	}
	s.monitor = cluster.NewHealthMonitor(s.pool, cfg, 30*time.Second, s.emitHealth)
	return s
}

// Start 注入 wails ctx 并启动后台健康 monitor。在 app.startup 中调用。
func (s *ClusterService) Start(ctx context.Context) {
	s.ctxMu.Lock()
	s.ctx = ctx
	s.ctxMu.Unlock()
	go s.monitor.Run(ctx)
}

// Stop 同步等待 monitor 退出。在 app.shutdown 中调用。
func (s *ClusterService) Stop() {
	if s.monitor != nil {
		s.monitor.Stop()
	}
}

// Pool 暴露底层客户端池（供 stream / jobs 等服务复用）。
func (s *ClusterService) Pool() *cluster.Pool { return s.pool }

// ListClusters 返回全部集群及健康状态。
// 注意：健康数据读 monitor 缓存，不再同步阻塞探测（M1 §7 验收：首屏不卡）。
func (s *ClusterService) ListClusters() ([]nomad.ClusterInfo, *Error) {
	cfgs, err := s.cfg.List()
	if err != nil {
		return nil, Wrap(err)
	}
	out := make([]nomad.ClusterInfo, 0, len(cfgs))
	for _, c := range cfgs {
		info := nomad.ClusterInfo{
			ID:                 c.ID,
			Name:               c.Name,
			Address:            c.Address,
			Region:             c.Region,
			Namespace:          c.Namespace,
			TLS:                c.TLS,
			InsecureSkipVerify: c.InsecureSkipVerify,
			Health:             "unknown",
		}
		// HasToken：Keychain 是否存有 token。不暴露 token 本身。
		if _, err := s.keyring.GetToken(c.ID); err == nil {
			info.HasToken = true
		}
		if u, ok := s.monitor.Latest(c.ID); ok {
			info.Health = u.Status
			info.LastChecked = u.LastChecked
		}
		out = append(out, info)
	}
	return out, nil
}

// AddCluster 新增集群：校验 → 落盘配置 → token 入 Keychain。
func (s *ClusterService) AddCluster(in ClusterInput) *Error {
	if e := in.Validate(); e != nil {
		return e
	}
	addr, _ := ValidateAddress(in.Address) // Validate() 已通过

	cfg := &config.ClusterConfig{
		ID:                 in.ID,
		Name:               in.Name,
		Address:            addr,
		Region:             in.Region,
		Namespace:          in.Namespace,
		TLS:                in.TLS,
		InsecureSkipVerify: in.InsecureSkipVerify,
	}
	if err := s.cfg.Add(cfg); err != nil {
		return Wrap(err)
	}
	// token 入库失败不回滚配置（可后补），但返回警告
	if in.Token != "" {
		if err := s.keyring.SaveToken(in.ID, in.Token); err != nil {
			return NewError(CodeInternal, "config saved, but token save failed: %v", err)
		}
	}
	return nil
}

// RemoveCluster 删除集群：停池、清 Keychain、删配置。
func (s *ClusterService) RemoveCluster(clusterID string) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	// 先把 active 清掉（若删的正是当前 active），否则 M2 registry 会留悬空 ID
	s.clearActiveIfMatches(clusterID)
	// 让池里这个集群的 client 失效；否则用同 ID 重加时仍会拿到带旧 token 的旧 client
	s.pool.Invalidate(clusterID)
	if err := s.cfg.Delete(clusterID); err != nil {
		return Wrap(err)
	}
	_ = s.keyring.DeleteToken(clusterID) // 忽略不存在
	return nil
}

// TestConnection 手动探测集群连通性（5s 超时）。同步探测一次，结果也喂入 monitor 缓存。
func (s *ClusterService) TestConnection(clusterID string) (nomad.ClusterHealth, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nomad.ClusterHealth{}, NewError(CodeInvalidInput, "%v", err)
	}
	target, err := s.pool.ProbeTarget(clusterID)
	if err != nil {
		return nomad.ClusterHealth{}, Wrap(err)
	}
	return s.probeTarget(clusterID, target), nil
}

// TestConnectionInput 用未落盘的 ClusterInput 探测连通性（添加集群前的 Test 按钮）。
// 不写配置、不读 Keychain；token 直接用 input 里的（可为空）。
func (s *ClusterService) TestConnectionInput(in ClusterInput) (nomad.ClusterHealth, *Error) {
	if e := in.Validate(); e != nil {
		return nomad.ClusterHealth{}, e
	}
	addr, _ := ValidateAddress(in.Address)
	target := cluster.NewProbeTarget(addr, in.Token, in.TLS, in.InsecureSkipVerify)
	return s.probeTarget(in.ID, target), nil
}

// probeTarget 是 TestConnection / TestConnectionInput 的共享实现。
// 结果同时喂入 monitor 缓存（clusterID 为空时不喂——TestConnectionInput 时还没落盘）。
func (s *ClusterService) probeTarget(clusterID string, target cluster.ProbeTarget) nomad.ClusterHealth {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u := cluster.Probe(ctx, target)
	u.ClusterID = clusterID
	if clusterID != "" {
		s.monitor.Inject(u)
	}
	if u.Status == "down" {
		return nomad.ClusterHealth{Status: "down", Error: u.Error}
	}
	return nomad.ClusterHealth{
		Status:  "ok",
		Leader:  u.Leader,
		Version: u.Version,
	}
}

// UpdateCluster 编辑已有集群配置。ID 不可改（系统键）；Token 为空保留旧 token。
// 落盘后 Invalidate Pool 缓存，下次 Get 用新 addr/token 重建 client。
func (s *ClusterService) UpdateCluster(in ClusterInput) *Error {
	if err := ValidateClusterID(in.ID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateClusterName(in.Name); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	addr, err := ValidateAddress(in.Address)
	if err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateNamespace(in.Namespace); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateRegion(in.Region); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}

	// 现有集群必须存在
	if _, err := s.cfg.Get(in.ID); err != nil {
		return Wrap(err)
	}

	cfg := &config.ClusterConfig{
		ID:                 in.ID,
		Name:               in.Name,
		Address:            addr,
		Region:             in.Region,
		Namespace:          in.Namespace,
		TLS:                in.TLS,
		InsecureSkipVerify: in.InsecureSkipVerify,
	}
	if err := s.cfg.Update(cfg); err != nil {
		return Wrap(err)
	}
	// Token 语义：非空才更新；空保留 Keychain 已有 token
	if in.Token != "" {
		if err := s.keyring.SaveToken(in.ID, in.Token); err != nil {
			return NewError(CodeInternal, "config saved, but token save failed: %v", err)
		}
	}
	// 让 Pool 失效，下次 Get 用新 addr/token 重建 client
	s.pool.Invalidate(in.ID)
	return nil
}

// SetActiveCluster 激活集群。
func (s *ClusterService) SetActiveCluster(clusterID string) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if _, err := s.cfg.Get(clusterID); err != nil {
		return Wrap(err)
	}
	s.activeMu.Lock()
	s.active = clusterID
	s.activeMu.Unlock()
	if s.OnActiveChanged != nil {
		s.OnActiveChanged(clusterID)
	}
	return nil
}

// ActiveCluster 返回当前激活集群 ID。
func (s *ClusterService) ActiveCluster() string {
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	return s.active
}

// clearActiveIfMatches 是 CAS 风格的 active 清空：仅当当前 active 等于 id 时才清。
// 用于 RemoveCluster：删除 active 集群时清空，删别的集群不影响 active。
func (s *ClusterService) clearActiveIfMatches(id string) {
	s.activeMu.Lock()
	changed := false
	if s.active == id {
		s.active = ""
		changed = true
	}
	s.activeMu.Unlock()
	if changed && s.OnActiveChanged != nil {
		s.OnActiveChanged("")
	}
}

// emitHealth 是 HealthMonitor 的回调：直接把 HealthUpdate 通过 v3 EmitEvent
// 推给前端（事件名 "cluster.health"）。HealthUpdate 自带 ClusterID，前端按 id 路由。
// 若 app 尚未注入（SetApp 未调用），仅更新 monitor 缓存不 emit。
func (s *ClusterService) emitHealth(u cluster.HealthUpdate) {
	s.appMu.RLock()
	app := s.app
	s.appMu.RUnlock()
	if app == nil {
		return
	}
	app.Event.Emit("cluster.health", u)
}
