package metrics

import (
	"sort"
	"sync"

	"github.com/nphq/np/internal/nomad"
)

// Cache 是每集群的负载缓存。
// 唯一事实源：capacity/allocated 静态 + used 实时 + 环形缓冲。
// 所有读取走 Snapshot/NodeLoads/AllocLoad，写只在 Collector.Tick 内。
type Cache struct {
	mu     sync.RWMutex
	points int

	nodes    map[string]nomad.NodeLoad
	allocs   map[string]nomad.AllocLoad
	cluster  []nomad.LoadSample // 集群级环形缓冲
	allocsOn bool               // A2 是否开启
	lastTick int64              // ms epoch
}

// NewCache 创建缓存。
func NewCache(points int) *Cache {
	if points <= 0 {
		points = 60
	}
	return &Cache{
		points: points,
		nodes:  map[string]nomad.NodeLoad{},
		allocs: map[string]nomad.AllocLoad{},
	}
}

// Empty 表示尚未有任何节点数据（首拉前的判定）。
func (c *Cache) Empty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.nodes) == 0
}

// Update 写入一轮采样并做帧间 diff，返回仅含变化的 LoadPatch。
// allocLevel 是 A2 开关（由 collector 依据 running 数判定）。
func (c *Cache) Update(nodeLoads []nomad.NodeLoad, allocLoads []nomad.AllocLoad, allocLevel bool) LoadPatch {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.allocsOn = allocLevel
	now := nowFunc()
	// 节点：合并 samples 环形缓冲；diff 出变化
	nodesIn := make(map[string]bool, len(nodeLoads))
	changed := make([]nomad.NodeLoad, 0, len(nodeLoads))
	for _, nl := range nodeLoads {
		nodesIn[nl.NodeID] = true
		prev, ok := c.nodes[nl.NodeID]
		sample := usageSample(now, nl.Used)
		if ok && len(prev.Samples) > 0 && prev.Samples[len(prev.Samples)-1].Time == now {
			sample = nomad.LoadSample{} // 同 ms 双 tick 去重（防并发首拉）
		}
		if ok {
			nl.Samples = appendSample(prev.Samples, sample, c.points)
		} else {
			nl.Samples = []nomad.LoadSample{sample}
		}
		if !ok || nodeLoadChanged(prev, nl) {
			changed = append(changed, nl)
		}
		c.nodes[nl.NodeID] = nl
	}
	// 清理消失的节点 → 发 remove 标记
	for id := range c.nodes {
		if !nodesIn[id] {
			delete(c.nodes, id)
			changed = append(changed, nomad.NodeLoad{NodeID: id, Removed: true})
		}
	}

	// alloc：diff 出变化
	allocChanged := make([]nomad.AllocLoad, 0, len(allocLoads))
	for _, al := range allocLoads {
		prev, ok := c.allocs[al.AllocID]
		if !ok || allocLoadChanged(prev, al) {
			allocChanged = append(allocChanged, al)
		}
		c.allocs[al.AllocID] = al
	}

	// 集群级采样：仅在有可用节点时追加
	var sum nomad.ResourceUsage
	avail := 0
	for _, nl := range c.nodes {
		if nl.Available {
			sum.CPU += nl.Used.CPU
			sum.Memory += nl.Used.Memory
			sum.Disk += nl.Used.Disk
			avail++
		}
	}
	if avail > 0 {
		sample := usageSample(now, sum)
		if len(c.cluster) == 0 || c.cluster[len(c.cluster)-1].Time != now {
			c.cluster = appendSample(c.cluster, sample, c.points)
		}
	}
	c.lastTick = now

	return LoadPatch{Nodes: changed, Allocs: allocChanged}
}

// Snapshot 聚合出集群负载快照（Overview 数据源）。
func (c *Cache) Snapshot() nomad.ClusterLoad {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cl := nomad.ClusterLoad{
		AllocLevel: c.allocsOn,
		UpdatedAt:  c.lastTick,
		Samples:    append([]nomad.LoadSample(nil), c.cluster...),
	}
	for _, nl := range c.nodes {
		cl.Capacity.CPU += nl.Capacity.CPU
		cl.Capacity.Memory += nl.Capacity.Memory
		cl.Capacity.Disk += nl.Capacity.Disk
		cl.Allocated.CPU += nl.Allocated.CPU
		cl.Allocated.Memory += nl.Allocated.Memory
		cl.Allocated.Disk += nl.Allocated.Disk
		cl.NodeCount++
		if nl.Available {
			cl.Used.CPU += nl.Used.CPU
			cl.Used.Memory += nl.Used.Memory
			cl.Used.Disk += nl.Used.Disk
			cl.NodeUp++
		}
	}
	cl.TopConsumers = c.topConsumersLocked(5)
	return cl
}

// NodeLoads 返回全部节点负载（Nodes 屏首拉）。
func (c *Cache) NodeLoads() []nomad.NodeLoad {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]nomad.NodeLoad, 0, len(c.nodes))
	for _, nl := range c.nodes {
		if nl.Removed {
			continue
		}
		out = append(out, nl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

// AllocLoad 返回单个 alloc 的负载；ok=false 表示缓存中没有（或已降级）。
func (c *Cache) AllocLoad(allocID string) (nomad.AllocLoad, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	al, ok := c.allocs[allocID]
	return al, ok
}

// topConsumersLocked 按 CPU 用量取 top n（需持锁调用）。
func (c *Cache) topConsumersLocked(n int) []nomad.AllocConsumer {
	if len(c.allocs) == 0 {
		return nil
	}
	consumers := make([]nomad.AllocConsumer, 0, len(c.allocs))
	for id, al := range c.allocs {
		var cpu, mem float64
		for _, t := range al.Tasks {
			cpu += t.CPU
			mem += t.Memory
		}
		consumers = append(consumers, nomad.AllocConsumer{AllocID: id, JobID: al.JobID, CPU: cpu, Memory: mem})
	}
	sort.Slice(consumers, func(i, j int) bool { return consumers[i].CPU > consumers[j].CPU })
	if len(consumers) > n {
		consumers = consumers[:n]
	}
	return consumers
}
