package uiapi

import (
	"context"
	"fmt"
	"os"
	"sort"
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
	// UseEnvToken：为 true 且 Token 为空时，用 NOMAD_TOKEN（不经前端明文往返）。
	UseEnvToken bool `json:"useEnvToken,omitempty"`
}

// applyEnvToken 在 UseEnvToken 且 Token 为空时从环境补齐 token。
func (in *ClusterInput) applyEnvToken() {
	if in == nil || !in.UseEnvToken || strings.TrimSpace(in.Token) != "" {
		return
	}
	in.Token = os.Getenv("NOMAD_TOKEN")
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
	prefs   *config.PrefsStore
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
// prefs 持久化活跃集群（重启恢复）；可为 nil（无偏好存储时不落盘）。
func NewClusterService(cfg *config.Store, prefs *config.PrefsStore, kr secure.Keyring) *ClusterService {
	s := &ClusterService{
		cfg:     cfg,
		prefs:   prefs,
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

// SetHealthInterval 热更新健康轮询间隔（秒；越界回落 30s）。
func (s *ClusterService) SetHealthInterval(sec int) {
	if sec <= 0 {
		sec = 30
	}
	if s.monitor != nil {
		s.monitor.SetInterval(time.Duration(sec) * time.Second)
	}
}

// ListClusters 返回全部集群（§3.1 排序：Pinned 在前 → SortOrder → Name → ID）
// 及活跃集群 ID（后端唯一裁决，前端不做本地双源）。
// 注意：健康数据读 monitor 缓存，不再同步阻塞探测（M1 §7 验收：首屏不卡）。
// Keychain 查询并行化（上限 8），避免 N 集群串行 IPC 放大尾延迟；结果保序。
func (s *ClusterService) ListClusters() (nomad.ClusterList, *Error) {
	cfgs, err := s.cfg.List()
	if err != nil {
		return nomad.ClusterList{}, Wrap(err)
	}
	sorted := sortedConfigs(cfgs)
	out := make([]nomad.ClusterInfo, len(sorted))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, c := range sorted {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c *config.ClusterConfig) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = s.clusterInfo(c)
		}(i, c)
	}
	wg.Wait()
	return nomad.ClusterList{
		Clusters: out,
		ActiveID: s.ActiveCluster(),
	}, nil
}

// sortedConfigs 按 §3.1 排序规则排序：Pinned 在前，组内 SortOrder 升序 → Name → ID。
// 排序在服务端统一执行，前端不再本地重排，避免两端不一致。
func sortedConfigs(cfgs []*config.ClusterConfig) []*config.ClusterConfig {
	out := append([]*config.ClusterConfig(nil), cfgs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return out
}

// clusterInfo 由 ClusterConfig 构建前端可见的 ClusterInfo（含健康/Keychain 状态）。
func (s *ClusterService) clusterInfo(c *config.ClusterConfig) nomad.ClusterInfo {
	info := nomad.ClusterInfo{
		ID:                 c.ID,
		Name:               c.Name,
		Address:            c.Address,
		Region:             c.Region,
		Namespace:          c.Namespace,
		TLS:                c.TLS,
		InsecureSkipVerify: c.InsecureSkipVerify,
		Pinned:             c.Pinned,
		SortOrder:          c.SortOrder,
		Health:             "unknown",
	}
	// HasToken：Keychain 是否存有 token。不暴露 token 本身。
	// 无新 token 时探测品牌统一前的旧 service：命中则提示用户重录迁移（review C2）。
	if _, err := s.keyring.GetToken(c.ID); err == nil {
		info.HasToken = true
	} else if prober, ok := s.keyring.(legacyKeyringProber); ok {
		if _, err := prober.GetLegacyToken(c.ID); err == nil {
			info.HasLegacyToken = true
		}
	}
	if u, ok := s.monitor.Latest(c.ID); ok {
		info.Health = u.Status
		info.LastChecked = u.LastChecked
	}
	return info
}

// legacyKeyringProber 是能从品牌统一前 service（secure.LegacyServiceName）读 token 的
// Keyring 实现（仅 OSKeyring）。MemoryKeyring 与测试 mock 不实现它 → 探测自然跳过。
type legacyKeyringProber interface {
	GetLegacyToken(clusterID string) (string, error)
}

// AddCluster 新增集群：校验 → 落盘配置 → token 入 Keychain。
// token 入 Keychain 失败时回滚配置：不留无 token 的死集群（review C3）。
// 回滚本身失败时返回 CodeInternal（两个错误都带上，便于排障）。
func (s *ClusterService) AddCluster(in ClusterInput) *Error {
	in.applyEnvToken()
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
	if in.Token != "" {
		if err := s.keyring.SaveToken(in.ID, in.Token); err != nil {
			// 回滚配置，避免残留"添加失败但集群已存在"的无 token 死集群
			if rbErr := s.cfg.Delete(in.ID); rbErr != nil {
				return NewError(CodeInternal,
					"token save failed: %v; and config rollback failed: %v", err, rbErr)
			}
			return NewError(CodeInternal, "token save failed: %v (config rolled back)", err)
		}
	}
	return nil
}

// RemoveCluster 删除集群：停池、清 Keychain、删配置。
// 若删的是当前活跃集群，按 §5.2 回退：下一个 Pinned → 列表第一项 → 清空 active。
func (s *ClusterService) RemoveCluster(clusterID string) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	// 让池里这个集群的 client 失效；否则用同 ID 重加时仍会拿到带旧 token 的旧 client
	s.pool.Invalidate(clusterID)
	if err := s.cfg.Delete(clusterID); err != nil {
		return Wrap(err)
	}
	_ = s.keyring.DeleteToken(clusterID) // 忽略不存在
	if s.ActiveCluster() == clusterID {
		s.activateFallback()
	}
	return nil
}

// activateFallback 是删除活跃集群后的回退（§5.2）：
// 下一个 Pinned（排序已在最前）→ 否则列表第一项 → 否则清空 active。
func (s *ClusterService) activateFallback() {
	cfgs, err := s.cfg.List()
	if err != nil || len(cfgs) == 0 {
		s.clearActive()
		return
	}
	next := sortedConfigs(cfgs)[0]
	if e := s.SetActiveCluster(next.ID); e != nil {
		// 回退失败不阻断删除；至少清空 active 避免悬空
		s.clearActive()
	}
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
// 不写配置、不读 Keychain；token 直接用 input 里的（可为空；UseEnvToken 时读 NOMAD_TOKEN）。
func (s *ClusterService) TestConnectionInput(in ClusterInput) (nomad.ClusterHealth, *Error) {
	in.applyEnvToken()
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
	// Namespace 回填配置值，前端详情页无需二次查询。
	ns := ""
	if clusterID != "" {
		if cfg, err := s.cfg.Get(clusterID); err == nil {
			ns = cfg.Namespace
		}
	}
	if u.Status == "down" {
		return nomad.ClusterHealth{Status: "down", Error: u.Error, Namespace: ns}
	}
	return nomad.ClusterHealth{
		Status:    "ok",
		Leader:    u.Leader,
		Version:   u.Version,
		Namespace: ns,
	}
}

// UpdateCluster 编辑已有集群配置。ID 不可改（系统键）；Token 为空保留旧 token。
// 落盘后 Invalidate Pool 缓存，下次 Get 用新 addr/token 重建 client。
func (s *ClusterService) UpdateCluster(in ClusterInput) *Error {
	in.applyEnvToken()
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

	// 现有集群必须存在；保留 Pinned/SortOrder（编辑不该清掉收藏）
	existing, err := s.cfg.Get(in.ID)
	if err != nil {
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
		Pinned:             existing.Pinned,
		SortOrder:          existing.SortOrder,
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

// SetActiveCluster 激活集群（成功后持久化到 prefs，重启后自动恢复）。
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
	if s.prefs != nil {
		if err := s.prefs.SetActive(clusterID); err != nil {
			// 偏好落盘失败不阻断激活（下次启动恢复不了，但本次会话可用）
			fmt.Printf("WARN: persist active pref: %v\n", err)
		}
	}
	if s.OnActiveChanged != nil {
		s.OnActiveChanged(clusterID)
	}
	// 立即异步探测：不要等 monitor 的 30s 周期，避免侧边栏长期显示 stale down。
	go s.probeActiveAsync(clusterID)
	return nil
}

// probeActiveAsync 激活后立刻探一次；失败仅写 monitor 缓存/事件，不回传调用方。
func (s *ClusterService) probeActiveAsync(clusterID string) {
	target, err := s.pool.ProbeTarget(clusterID)
	if err != nil {
		u := cluster.HealthUpdate{
			ClusterID:   clusterID,
			Status:      "down",
			LastChecked: time.Now().Unix(),
			Error:       fmt.Sprintf("client: %v", err),
		}
		s.monitor.Inject(u)
		return
	}
	_ = s.probeTarget(clusterID, target)
}

// ActiveCluster 返回当前激活集群 ID。
func (s *ClusterService) ActiveCluster() string {
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	return s.active
}

// RestoreActive 在启动时恢复上次活跃集群：prefs 有 activeClusterID 且集群
// 仍存在 → 内部激活（触发 OnActiveChanged → 各 store 拉数）；集群已被删除
// 则清空偏好，回空状态。不阻断启动（集群挂了也恢复选中，健康点显示 down）。
func (s *ClusterService) RestoreActive() {
	if s.prefs == nil {
		return
	}
	id, err := s.prefs.GetActive()
	if err != nil {
		fmt.Printf("WARN: load prefs for RestoreActive: %v\n", err)
		return
	}
	if id == "" {
		return
	}
	if _, err := s.cfg.Get(id); err != nil {
		// 上次的集群已被删掉（如手动清了 clusters.json）→ 清理偏好
		_ = s.prefs.ClearActive()
		return
	}
	s.activeMu.Lock()
	s.active = id
	s.activeMu.Unlock()
	if s.OnActiveChanged != nil {
		s.OnActiveChanged(id)
	}
}

// PinCluster 置顶/取消置顶集群（收藏）。SortOrder 保留原值，不隐式修改。
func (s *ClusterService) PinCluster(clusterID string, pinned bool) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	cfg, err := s.cfg.Get(clusterID)
	if err != nil {
		return Wrap(err)
	}
	if cfg.Pinned == pinned {
		return nil
	}
	cfg.Pinned = pinned
	if err := s.cfg.Update(cfg); err != nil {
		return Wrap(err)
	}
	return nil
}

// clearActive 清空 active（无回退目标时的最终兜底）。
func (s *ClusterService) clearActive() {
	s.activeMu.Lock()
	s.active = ""
	s.activeMu.Unlock()
	if s.prefs != nil {
		_ = s.prefs.ClearActive()
	}
	if s.OnActiveChanged != nil {
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
