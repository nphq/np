package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"

	"github.com/nphq/np/internal/nomad"
)

// Collector 是每集群一个的负载轮询器。
//
// tick 流程（默认 15s）：
//
//	A0 静态 — ListNodes / ListAllocations（capacity + 声明资源，顺带返回）
//	A1 节点 — HostStats 并行拉取（并发 8）
//	A2 alloc — AllocStats 分片拉取（并发 4，多 tick 轮完；running 超过
//	 MaxAllocStats 时整体降级为节点级）
//	写入 cache → 帧间 diff → 仅 emit 变化（LoadPatch）
//
// Collector 生命周期挂在 LoadsService（随 SetActiveCluster 启停，ADR-3 语义）。
type Collector struct {
	cfg   Config
	get   ClientGetter
	cache *Cache
	emit  func(LoadPatch)

	mu     sync.Mutex
	shard  int // 分片轮转游标
	cancel context.CancelFunc
	done   chan struct{}

	tickMu sync.Mutex // 串行化 Tick（后台循环与同步刷新并发时防网络风暴）
}

// ClientGetter 返回目标集群的 *api.Client（由 uiapi 注入，走 Client Pool）。
type ClientGetter func() (*api.Client, error)

// Config 是 Collector 的配置。
type Config struct {
	ClusterID        string
	Namespace        string // 集群配置的默认 namespace；空 = 服务端回退 default
	Interval         int64  // 秒，默认 15
	MaxAllocStats    int    // running allocs 超过则降级为节点级，默认 200
	ShardCount       int    // A2 分片数，默认 4
	ShardThreshold   int    // running 达到该值才分片，默认 16
	NodeConcurrency  int    // A1 并发，默认 8
	AllocConcurrency int    // A2 并发，默认 4
	SamplePoints     int    // 每键环形缓冲点数，默认 60
}

// DefaultConfig 返回生产默认值。
func DefaultConfig(clusterID string) Config {
	return Config{
		ClusterID:        clusterID,
		Interval:         15,
		MaxAllocStats:    200,
		ShardCount:       4,
		ShardThreshold:   16,
		NodeConcurrency:  8,
		AllocConcurrency: 4,
		SamplePoints:     60,
	}
}

func (c *Config) applyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 15
	}
	if c.MaxAllocStats <= 0 {
		c.MaxAllocStats = 200
	}
	if c.ShardCount <= 0 {
		c.ShardCount = 4
	}
	if c.ShardThreshold <= 0 {
		c.ShardThreshold = 16
	}
	if c.NodeConcurrency <= 0 {
		c.NodeConcurrency = 8
	}
	if c.AllocConcurrency <= 0 {
		c.AllocConcurrency = 4
	}
	if c.SamplePoints <= 0 {
		c.SamplePoints = 60
	}
}

// LoadPatch 是 load.patch 事件的 payload（仅含变化的节点/alloc + 集群聚合）。
// ClusterID 供前端在激活集群切换后过滤旧集群的迟到事件（epoch guard）。
type LoadPatch struct {
	ClusterID string            `json:"clusterID"`
	Nodes     []nomad.NodeLoad  `json:"nodes"`
	Allocs    []nomad.AllocLoad `json:"allocs"`
	Cluster   nomad.ClusterLoad `json:"cluster"`
}

// NewCollector 创建 Collector。emit 可为 nil（无事件推送，纯缓存）。
func NewCollector(cfg Config, get ClientGetter, emit func(LoadPatch)) *Collector {
	cfg.applyDefaults()
	return &Collector{
		cfg:   cfg,
		get:   get,
		cache: NewCache(cfg.SamplePoints),
		emit:  emit,
	}
}

// Start 立即跑一次 tick，然后按 Interval 周期运行；Stop 后不可复用。
func (c *Collector) Start() {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	c.mu.Unlock()

	go func() {
		defer close(c.done)
		// 首 tick 立即执行，让首屏有数据；失败静默（GetClusterLoad 会转同步刷新）
		_ = c.Tick(ctx)
		ticker := time.NewTicker(time.Duration(c.cfg.Interval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.Tick(ctx)
			}
		}
	}()
}

// Stop 停止采集循环并等待退出。
func (c *Collector) Stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	<-done
}

// Cache 暴露底层缓存（供 uiapi 读取快照/节点/alloc 负载）。
func (c *Collector) Cache() *Cache { return c.cache }

// Tick 跑一轮采集并 emit 变化。供周期循环与同步刷新（GetClusterLoad 首拉）共用。
// ctx 会一路透传到 SDK 调用（QueryOptions.WithContext），保证超时/取消生效。
func (c *Collector) Tick(ctx context.Context) error {
	c.tickMu.Lock()
	defer c.tickMu.Unlock()
	client, err := c.get()
	if err != nil {
		return err
	}
	nodes, err := nomad.ListNodes(ctx, client)
	if err != nil {
		return fmt.Errorf("load: list nodes: %w", err)
	}
	allocs, err := nomad.ListAllocations(ctx, client, c.cfg.Namespace)
	if err != nil {
		return fmt.Errorf("load: list allocations: %w", err)
	}

	// perCore MHz：单核基准（alloc 的 CPU% 是单核百分比）
	perCore := make(map[string]float64, len(nodes))
	byID := make(map[string]nomad.NodeSummary, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
		if n.CPUTotal > 0 {
			cores := n.CPUCores
			if cores <= 0 {
				cores = 1
			}
			perCore[n.ID] = n.CPUTotal / float64(cores)
		}
	}

	// allocated（按节点求和）+ running allocs + alloc→node 归属
	allocated := make(map[string]nomad.ResourceUsage, len(nodes))
	byNodeAllocs := make(map[string]int)
	allocNode := make(map[string]string, len(allocs))
	running := make([]nomad.AllocSummary, 0)
	for _, a := range allocs {
		ru := allocated[a.NodeID]
		ru.CPU += a.CPU
		ru.Memory += a.Memory
		ru.Disk += a.Disk
		allocated[a.NodeID] = ru
		byNodeAllocs[a.NodeID]++
		allocNode[a.ID] = a.NodeID
		if a.ClientStatus == "running" {
			running = append(running, a)
		}
	}

	// A1：HostStats 并行
	nodeIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}
	hosts := make(map[string]*nomad.NodeStats, len(nodeIDs))
	var hmu sync.Mutex
	runParallel(nodeIDs, c.cfg.NodeConcurrency, func(id string) {
		st, err := nomad.FetchNodeStats(ctx, client, id)
		hmu.Lock()
		defer hmu.Unlock()
		if err != nil {
			hosts[id] = nil
			return
		}
		hosts[id] = st
	})

	// A2：AllocStats 分片（或降级为节点级）
	allocLevel := len(running) <= c.cfg.MaxAllocStats
	c.mu.Lock()
	shard := c.shard
	c.shard++
	c.mu.Unlock()
	var shardAllocs []nomad.AllocSummary
	if allocLevel && len(running) > 0 {
		if len(running) > c.cfg.ShardThreshold {
			n := len(running)
			start := (shard % c.cfg.ShardCount) * n / c.cfg.ShardCount
			end := ((shard % c.cfg.ShardCount) + 1) * n / c.cfg.ShardCount
			shardAllocs = running[start:end]
		} else {
			shardAllocs = running
		}
	}
	allocStats := make(map[string]*nomad.AllocStats, len(shardAllocs))
	var amu sync.Mutex
	runParallel(shardAllocs, c.cfg.AllocConcurrency, func(a nomad.AllocSummary) {
		st, err := nomad.FetchAllocStats(ctx, client, a.ID)
		amu.Lock()
		defer amu.Unlock()
		if err != nil {
			return
		}
		allocStats[a.ID] = st
	})

	// 组装 NodeLoad（percent → MHz 换算）
	nodeLoads := make([]nomad.NodeLoad, 0, len(nodes))
	for _, n := range nodes {
		nl := nomad.NodeLoad{
			NodeID:        n.ID,
			Capacity:      nomad.ResourceUsage{CPU: n.CPUTotal, Memory: n.MemoryTotal, Disk: n.DiskTotal},
			Allocated:     allocated[n.ID],
			RunningAllocs: byNodeAllocs[n.ID],
		}
		if st := hosts[n.ID]; st != nil {
			nl.Available = true
			nl.Used = nomad.ResourceUsage{
				CPU:    percentToMHz(st.CPUPercent, n.CPUTotal),
				Memory: st.MemoryUsedMB,
				Disk:   st.DiskUsedMB,
			}
		}
		nodeLoads = append(nodeLoads, nl)
	}

	// 组装 AllocLoad（单核百分比 → MHz；Pct 相对声明资源）
	declared := allocDeclaredByTask(allocs)
	allocLoads := make([]nomad.AllocLoad, 0, len(allocStats))
	for id, st := range allocStats {
		if st == nil {
			continue
		}
		al := nomad.AllocLoad{AllocID: id, NodeID: allocNode[id], Time: st.Time}
		for _, a := range allocs {
			if a.ID == id {
				al.JobID = a.JobID
				break
			}
		}
		pc := perCore[allocNode[id]]
		al.Tasks = make(map[string]nomad.TaskUsage, len(st.Tasks))
		for name, ts := range st.Tasks {
			tu := nomad.TaskUsage{
				CPU:    percentToMHz(ts.CPUPercent, pc),
				Memory: ts.MemoryUsedMB,
			}
			if d, ok := declared[id][name]; ok && d.CPU > 0 {
				tu.Pct = tu.CPU / d.CPU
			}
			al.Tasks[name] = tu
		}
		allocLoads = append(allocLoads, al)
	}

	// 更新 cache + diff → patch
	patch := c.cache.Update(nodeLoads, allocLoads, allocLevel)
	patch.ClusterID = c.cfg.ClusterID
	patch.Cluster = c.cache.Snapshot()
	if c.emit != nil {
		c.emit(patch)
	}
	return nil
}

// allocDeclaredByTask 提取每个 alloc 的 per-task 声明资源（AllocatedResources）。
func allocDeclaredByTask(allocs []nomad.AllocSummary) map[string]map[string]nomad.ResourceUsage {
	out := make(map[string]map[string]nomad.ResourceUsage)
	for _, a := range allocs {
		if len(a.TaskResources) > 0 {
			out[a.ID] = a.TaskResources
		}
	}
	return out
}

// percentToMHz 把 CPU 百分比（0-100）换算为 MHz。
// total 为目标总量：节点级传总 MHz，alloc 级传单核 MHz。
func percentToMHz(percent, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return percent / 100 * total
}

// runParallel 以 limit 并发执行 fn，等待全部完成。
func runParallel[T any](items []T, limit int, fn func(T)) {
	if limit <= 0 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(v T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(v)
		}(it)
	}
	wg.Wait()
}
