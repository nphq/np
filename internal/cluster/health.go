package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nphq/np/internal/config"
)

// HealthUpdate 是一次健康探测的领域结果（clusterID 由 monitor / 调用方填）。
type HealthUpdate struct {
	ClusterID   string `json:"clusterID"`
	Status      string `json:"status"` // "ok" | "down"
	Leader      string `json:"leader"`
	Version     string `json:"version"`
	LastChecked int64  `json:"lastChecked"` // unix seconds
	Error       string `json:"error,omitempty"`
}

// ProbeTarget 是 Probe 所需的 raw HTTP 材料。
// 显式暴露 addr/token/httpClient，避免 *api.Client 内部状态不可达，
// 让 ctx 取消能立即终止底层 net/http（不再泄漏 goroutine）。
type ProbeTarget struct {
	Addr       string
	Token      string
	HTTPClient *http.Client
}

// Probe 用给定 ctx 探测一次目标集群：leader + 版本。
// ctx 取消即终止 HTTP，不再泄漏 goroutine（旧实现靠 goroutine+select
// 兜底，超时后 goroutine 会阻塞到 SDK 调用真返回为止）。
func Probe(ctx context.Context, t ProbeTarget) HealthUpdate {
	checkedAt := time.Now().Unix()

	leader, err := fetchString(ctx, t, "/v1/status/leader")
	if err != nil {
		return healthDown(checkedAt, err)
	}
	if leader == "" {
		return healthDown(checkedAt, fmt.Errorf("no leader"))
	}

	version, _ := fetchVersion(ctx, t)
	return HealthUpdate{
		Status:      "ok",
		Leader:      leader,
		Version:     version,
		LastChecked: checkedAt,
	}
}

func healthDown(checkedAt int64, err error) HealthUpdate {
	return HealthUpdate{Status: "down", LastChecked: checkedAt, Error: err.Error()}
}

// fetchString GET path 并把 body 当作 JSON 字符串解码。
func fetchString(ctx context.Context, t ProbeTarget, path string) (string, error) {
	body, err := rawGet(ctx, t, path)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	var s string
	if err := json.NewDecoder(body).Decode(&s); err != nil {
		return "", fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

// fetchVersion 兼容 agent self 中版本字段的两种形态：
// 新版 config.version 是 string；旧版 config.Version 是 map[string]any。
func fetchVersion(ctx context.Context, t ProbeTarget) (string, error) {
	body, err := rawGet(ctx, t, "/v1/agent/self")
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	var raw struct {
		Config map[string]json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return "", fmt.Errorf("decode agent self: %w", err)
	}
	if v, ok := raw.Config["version"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s, nil
		}
	}
	if v, ok := raw.Config["Version"]; ok {
		var m map[string]string
		if json.Unmarshal(v, &m) == nil {
			return m["Version"], nil
		}
	}
	return "", nil
}

func rawGet(ctx context.Context, t ProbeTarget, path string) (io.ReadCloser, error) {
	addr := t.Addr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+path, nil)
	if err != nil {
		return nil, err
	}
	if t.Token != "" {
		req.Header.Set("X-Nomad-Token", t.Token)
	}
	hc := t.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", path, resp.Status)
	}
	return resp.Body, nil
}

// HealthMonitor 周期性探测所有已配置集群的健康状态，通过 emit 回调通知上层。
type HealthMonitor struct {
	pool     *Pool
	cfg      *config.Store
	interval time.Duration
	timeout  time.Duration
	emit     func(HealthUpdate)

	mu     sync.RWMutex
	latest map[string]HealthUpdate // clusterID -> 最近状态

	stop      chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
	started   chan struct{}
	startOnce sync.Once
}

// NewHealthMonitor 创建 monitor。interval<=0 默认 30s。单次探测超时固定 5s。
func NewHealthMonitor(pool *Pool, cfg *config.Store, interval time.Duration, emit func(HealthUpdate)) *HealthMonitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &HealthMonitor{
		pool:     pool,
		cfg:      cfg,
		interval: interval,
		timeout:  5 * time.Second,
		emit:     emit,
		latest:   map[string]HealthUpdate{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		started:  make(chan struct{}),
	}
}

// SetInterval 更新轮询间隔（<=0 回落 30s；下一次 tick 生效，Run 循环通过读取最新值实现热更新）。
func (m *HealthMonitor) SetInterval(d time.Duration) {
	if d <= 0 {
		d = 30 * time.Second
	}
	m.mu.Lock()
	m.interval = d
	m.mu.Unlock()
}

func (m *HealthMonitor) getInterval() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.interval <= 0 {
		return 30 * time.Second
	}
	return m.interval
}

// Run 阻塞运行直到 ctx 取消或 Stop。启动时立即探一次，之后每 interval 全量探一次。
func (m *HealthMonitor) Run(ctx context.Context) {
	m.startOnce.Do(func() { close(m.started) })
	defer close(m.done)
	m.tick(ctx)
	for {
		// 每轮重读间隔，支持设置页热更新。
		timer := time.NewTimer(m.getInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.stop:
			timer.Stop()
			return
		case <-timer.C:
			m.tick(ctx)
		}
	}
}

// Stop 同步等待 Run 退出。多次调用安全；Run 未启动时直接返回不死锁。
func (m *HealthMonitor) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
	select {
	case <-m.started:
		// Run 已启动，等待退出；若 ctx 已取消导致已退出则立即返回。
		select {
		case <-m.done:
		case <-time.After(10 * time.Second):
		}
	default:
		// Run 从未启动，done 永不关闭，直接返回。
	}
}

// tick 并发探测所有集群（上限 8 并发）。每个集群独立 5s 超时；总体最长约 5s（并行）。
func (m *HealthMonitor) tick(ctx context.Context) {
	cfgs, err := m.cfg.List()
	if err != nil {
		return
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
loop:
	for _, c := range cfgs {
		select {
		case <-ctx.Done():
			break loop
		default:
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		case <-m.stop:
			break loop
		}
		wg.Add(1)
		go func(c *config.ClusterConfig) {
			defer wg.Done()
			defer func() { <-sem }()
			u := m.probeOne(ctx, c.ID)
			m.publish(u)
		}(c)
	}
	wg.Wait()
}

func (m *HealthMonitor) probeOne(ctx context.Context, clusterID string) HealthUpdate {
	target, err := m.pool.ProbeTarget(clusterID)
	if err != nil {
		return HealthUpdate{
			ClusterID:   clusterID,
			Status:      "down",
			LastChecked: time.Now().Unix(),
			Error:       fmt.Sprintf("client: %v", err),
		}
	}
	sub, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	u := Probe(sub, target)
	u.ClusterID = clusterID
	return u
}

// Inject 把一次外部探测（如 TestConnection）的结果喂入缓存并 emit。
func (m *HealthMonitor) Inject(u HealthUpdate) {
	m.publish(u)
}

func (m *HealthMonitor) publish(u HealthUpdate) {
	if u.ClusterID == "" {
		return
	}
	m.mu.Lock()
	m.latest[u.ClusterID] = u
	m.mu.Unlock()
	if m.emit != nil {
		m.emit(u)
	}
}

// Latest 返回某集群的最新缓存状态。
func (m *HealthMonitor) Latest(clusterID string) (HealthUpdate, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.latest[clusterID]
	return u, ok
}
