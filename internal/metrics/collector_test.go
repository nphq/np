package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/nomad/api"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

// mockNomad 是 /v1/nodes /v1/allocations /v1/client/stats /v1/allocation/{id}/stats
// 的最小 mock，字段用 SDK struct 直接 marshal（与真实响应同构）。
type mockNomad struct {
	t           *testing.T
	nodes       []*api.NodeListStub
	allocs      []*api.AllocationListStub
	hostCPU     float64 // 每核 Idle
	hostMemUsed uint64
	allocCPU    float64 // 每个 running alloc 的 task CPU%
	statsHits   atomic.Int64
	allocHits   atomic.Int64
}

func (m *mockNomad) srv() *httptest.Server {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.URL.Path == "/v1/nodes":
			write(m.nodes)
		case r.URL.Path == "/v1/allocations":
			write(m.allocs)
		case r.URL.Path == "/v1/client/stats":
			m.statsHits.Add(1)
			write(&api.HostStats{
				Memory:    &api.HostMemoryStats{Total: 8 << 30, Used: m.hostMemUsed, Available: (8 << 30) - m.hostMemUsed},
				CPU:       []*api.HostCPUStats{{CPU: "cpu0", User: 25, System: 25, Idle: m.hostCPU}},
				DiskStats: []*api.HostDiskStats{{Device: "sda", Mountpoint: "/", Used: 50 << 20, Size: 100 << 20}},
			})
		case len(r.URL.Path) > len("/v1/client/allocation/") && r.URL.Path[:len("/v1/client/allocation/")] == "/v1/client/allocation/" &&
			strings.HasSuffix(r.URL.Path, "/stats"):
			m.allocHits.Add(1)
			write(&api.AllocResourceUsage{
				Tasks: map[string]*api.TaskResourceUsage{
					"web": {
						Timestamp: time.Now().UnixMilli(),
						ResourceUsage: &api.ResourceUsage{
							MemoryStats: &api.MemoryStats{RSS: 256 << 20}, // 256 MiB
							CpuStats:    &api.CpuStats{Percent: m.allocCPU},
						},
					},
				},
			})
		default:
			m.t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(h)
	m.t.Cleanup(srv.Close)
	return srv
}

func allocStub(id, job, node, status string, cpu, mem int64) *api.AllocationListStub {
	return &api.AllocationListStub{
		ID: id, JobID: job, NodeID: node, TaskGroup: "web", ClientStatus: status, DesiredStatus: "run",
		AllocatedResources: &api.AllocatedResources{
			Tasks: map[string]*api.AllocatedTaskResources{
				"web": {Cpu: api.AllocatedCpuResources{CpuShares: cpu}, Memory: api.AllocatedMemoryResources{MemoryMB: mem}},
			},
			Shared: api.AllocatedSharedResources{DiskMB: 100},
		},
	}
}

func testCollector(t *testing.T, m *mockNomad, cfg Config) (*Collector, *api.Client, func() LoadPatch) {
	t.Helper()
	srv := m.srv()
	store := config.New(t.TempDir() + "/clusters.json")
	_ = store.Add(&config.ClusterConfig{ID: "test", Name: "t", Address: srv.URL})
	pool := cluster.NewPool(store, secure.NewMemory())
	client, err := pool.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var last LoadPatch
	emit := func(p LoadPatch) {
		mu.Lock()
		last = p
		mu.Unlock()
	}
	coll := NewCollector(cfg, func() (*api.Client, error) { return client, nil }, emit)
	return coll, client, func() LoadPatch {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

func TestCollectorTick_FullFlow(t *testing.T) {
	m := &mockNomad{
		t: t,
		nodes: []*api.NodeListStub{
			{ID: "n1", Name: "node-1", Status: "ready", Version: "1.9.4",
				NodeResources: &api.NodeResources{Cpu: api.NodeCpuResources{CpuShares: 3200, TotalCpuCores: 4}, Memory: api.NodeMemoryResources{MemoryMB: 8192}, Disk: api.NodeDiskResources{DiskMB: 100000}}},
			{ID: "n2", Name: "node-2", Status: "ready", Version: "1.9.4",
				NodeResources: &api.NodeResources{Cpu: api.NodeCpuResources{CpuShares: 1600, TotalCpuCores: 2}, Memory: api.NodeMemoryResources{MemoryMB: 4096}, Disk: api.NodeDiskResources{DiskMB: 50000}}},
		},
		allocs: []*api.AllocationListStub{
			allocStub("a1", "redis", "n1", "running", 500, 512),
			allocStub("a2", "redis", "n1", "running", 500, 512),
			allocStub("a3", "cleanup", "n2", "complete", 500, 512),
		},
		hostCPU:     50, // 平均 Idle 50 → used 50%
		hostMemUsed: 1 << 30,
		allocCPU:    25, // 25% × 单核 800MHz = 200MHz
	}
	coll, _, last := testCollector(t, m, DefaultConfig("test"))

	if err := coll.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	loads := coll.Cache().NodeLoads()
	if len(loads) != 2 {
		t.Fatalf("nodes = %d, want 2", len(loads))
	}
	n1 := loads[0]
	if n1.Capacity.CPU != 3200 || n1.Capacity.Memory != 8192 {
		t.Errorf("n1 capacity = %+v", n1.Capacity)
	}
	if n1.Allocated.CPU != 1000 { // a1+a2 各 500
		t.Errorf("n1 allocated CPU = %v, want 1000", n1.Allocated.CPU)
	}
	if n1.Used.CPU != 1600 { // 50% × 3200
		t.Errorf("n1 used CPU = %v, want 1600", n1.Used.CPU)
	}
	if n1.RunningAllocs != 2 {
		t.Errorf("n1 runningAllocs = %d, want 2", n1.RunningAllocs)
	}
	if !n1.Available {
		t.Error("n1 should be available")
	}
	if n1.Samples == nil || len(n1.Samples) != 1 {
		t.Errorf("n1 samples = %d, want 1", len(n1.Samples))
	}

	al, ok := coll.Cache().AllocLoad("a1")
	if !ok {
		t.Fatal("alloc a1 missing from cache")
	}
	task := al.Tasks["web"]
	if task.CPU != 200 { // 25% × (3200/4=800) 单核 MHz
		t.Errorf("a1 web CPU = %v, want 200", task.CPU)
	}
	if task.Memory != 256 {
		t.Errorf("a1 web mem = %v, want 256", task.Memory)
	}
	if task.Pct < 0.39 || task.Pct > 0.41 { // 200/500
		t.Errorf("a1 web pct = %v, want ~0.4", task.Pct)
	}

	cl := coll.Cache().Snapshot()
	if cl.NodeCount != 2 || cl.NodeUp != 2 {
		t.Fatalf("cluster = %d/%d up", cl.NodeCount, cl.NodeUp)
	}
	if cl.Used.CPU != 1600+800 { // n1 50% + n2 50%(×1600)
		t.Errorf("cluster used CPU = %v", cl.Used.CPU)
	}
	if len(cl.TopConsumers) != 2 {
		t.Errorf("top consumers = %d, want 2", len(cl.TopConsumers))
	}
	if cl.TopConsumers[0].JobID != "redis" {
		t.Errorf("top1 = %+v", cl.TopConsumers[0])
	}
	if !cl.AllocLevel {
		t.Error("allocLevel should be true (2 running ≤ 200)")
	}

	// emit 回调收到 patch（首轮 2 节点 + 2 running alloc）
	p := last()
	if len(p.Nodes) != 2 || len(p.Allocs) != 2 {
		t.Fatalf("first patch = %d nodes, %d allocs", len(p.Nodes), len(p.Allocs))
	}
	if p.Cluster.NodeCount != 2 {
		t.Fatalf("patch cluster = %+v", p.Cluster)
	}
}

func TestCollectorTick_DowngradeOverMaxAllocStats(t *testing.T) {
	m := &mockNomad{t: t, nodes: []*api.NodeListStub{
		{ID: "n1", Name: "n", Status: "ready",
			NodeResources: &api.NodeResources{Cpu: api.NodeCpuResources{CpuShares: 3200, TotalCpuCores: 4}, Memory: api.NodeMemoryResources{MemoryMB: 8192}}},
	}}
	for i := 0; i < 5; i++ {
		m.allocs = append(m.allocs, allocStub("a"+string(rune('0'+i)), "job", "n1", "running", 100, 100))
	}
	cfg := DefaultConfig("test")
	cfg.MaxAllocStats = 4 // 5 running > 4 → 降级
	coll, _, _ := testCollector(t, m, cfg)

	if err := coll.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	cl := coll.Cache().Snapshot()
	if cl.AllocLevel {
		t.Error("allocLevel should be false (downgraded)")
	}
	if m.allocHits.Load() != 0 {
		t.Errorf("alloc stats should not be fetched when downgraded, got %d hits", m.allocHits.Load())
	}
	if _, ok := coll.Cache().AllocLoad("a0"); ok {
		t.Error("alloc cache should be empty when downgraded")
	}
}

func TestCollectorTick_Sharding(t *testing.T) {
	m := &mockNomad{t: t, nodes: []*api.NodeListStub{
		{ID: "n1", Name: "n", Status: "ready",
			NodeResources: &api.NodeResources{Cpu: api.NodeCpuResources{CpuShares: 3200, TotalCpuCores: 4}, Memory: api.NodeMemoryResources{MemoryMB: 8192}}},
	}}
	for i := 0; i < 20; i++ {
		m.allocs = append(m.allocs, allocStub("alloc-"+string(rune('a'+i)), "job", "n1", "running", 100, 100))
	}
	cfg := DefaultConfig("test")
	cfg.ShardCount = 4
	cfg.ShardThreshold = 16
	coll, _, _ := testCollector(t, m, cfg)

	if err := coll.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if m.allocHits.Load() != 5 {
		t.Errorf("shard tick should fetch 20/4=5 allocs, got %d", m.allocHits.Load())
	}
	// 4 tick 后全部入缓存
	for i := 0; i < 3; i++ {
		if err := coll.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if m.allocHits.Load() != 20 {
		t.Errorf("4 ticks should fetch all 20, got %d", m.allocHits.Load())
	}
	if _, ok := coll.Cache().AllocLoad("alloc-a"); !ok {
		t.Error("alloc-a should be cached after full rotation")
	}
}

func TestCollectorStartStop(t *testing.T) {
	m := &mockNomad{t: t, nodes: []*api.NodeListStub{
		{ID: "n1", Name: "n", Status: "ready",
			NodeResources: &api.NodeResources{Cpu: api.NodeCpuResources{CpuShares: 3200, TotalCpuCores: 4}, Memory: api.NodeMemoryResources{MemoryMB: 8192}}},
	}}
	cfg := DefaultConfig("test")
	cfg.Interval = 1
	coll, _, last := testCollector(t, m, cfg)

	coll.Start()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p := last()
		if p.Cluster.NodeCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("collector did not emit first tick within 5s")
		}
		time.Sleep(50 * time.Millisecond)
	}
	coll.Stop() // 不应死锁/panic
	// Stop 后缓存仍可读
	if coll.Cache().NodeLoads() == nil {
		t.Fatal("cache should survive Stop")
	}
}
