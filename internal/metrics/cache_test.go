package metrics

import (
	"testing"

	"github.com/nphq/np/internal/nomad"
)

func nodeLoad(id string, used float64, avail bool) nomad.NodeLoad {
	return nomad.NodeLoad{
		NodeID:    id,
		Capacity:  nomad.ResourceUsage{CPU: 3200, Memory: 8192, Disk: 100000},
		Allocated: nomad.ResourceUsage{CPU: 1600, Memory: 4096},
		Used:      nomad.ResourceUsage{CPU: used, Memory: 100, Disk: 100},
		Available: avail,
	}
}

func TestCacheUpdate_DiffOnlyChanged(t *testing.T) {
	c := NewCache(10)
	p := c.Update([]nomad.NodeLoad{nodeLoad("n1", 100, true), nodeLoad("n2", 200, true)}, nil, true)
	if len(p.Nodes) != 2 {
		t.Fatalf("first update: want 2 nodes changed, got %d", len(p.Nodes))
	}
	if p.Nodes[0].Samples == nil || p.Nodes[1].Samples == nil {
		t.Fatal("samples should be seeded on first update")
	}

	p2 := c.Update([]nomad.NodeLoad{nodeLoad("n1", 100, true), nodeLoad("n2", 200, true)}, nil, true)
	if len(p2.Nodes) != 0 {
		t.Fatalf("no-change update: want 0 changed, got %d (%+v)", len(p2.Nodes), p2.Nodes)
	}

	p3 := c.Update([]nomad.NodeLoad{nodeLoad("n1", 150, true), nodeLoad("n2", 200, true)}, nil, true)
	if len(p3.Nodes) != 1 || p3.Nodes[0].NodeID != "n1" {
		t.Fatalf("delta update: want only n1, got %+v", p3.Nodes)
	}
}

func TestCacheUpdate_NodeRemoved(t *testing.T) {
	c := NewCache(10)
	c.Update([]nomad.NodeLoad{nodeLoad("n1", 100, true), nodeLoad("n2", 200, true)}, nil, true)
	p := c.Update([]nomad.NodeLoad{nodeLoad("n1", 100, true)}, nil, true)
	if len(p.Nodes) != 1 || !p.Nodes[0].Removed || p.Nodes[0].NodeID != "n2" {
		t.Fatalf("removed marker: %+v", p.Nodes)
	}
	loads := c.NodeLoads()
	if len(loads) != 1 || loads[0].NodeID != "n1" {
		t.Fatalf("NodeLoads after removal = %+v", loads)
	}
}

func TestCacheSnapshot_Aggregation(t *testing.T) {
	c := NewCache(10)
	c.Update([]nomad.NodeLoad{nodeLoad("n1", 100, true), nodeLoad("n2", 200, false)}, nil, true)
	cl := c.Snapshot()
	if cl.NodeCount != 2 || cl.NodeUp != 1 {
		t.Fatalf("NodeCount=%d NodeUp=%d, want 2/1", cl.NodeCount, cl.NodeUp)
	}
	if cl.Capacity.CPU != 6400 || cl.Allocated.CPU != 3200 {
		t.Fatalf("capacity=%+v allocated=%+v", cl.Capacity, cl.Allocated)
	}
	// used 只统计可用节点（n1=100）
	if cl.Used.CPU != 100 {
		t.Fatalf("Used.CPU = %v, want 100 (only available)", cl.Used.CPU)
	}
	if cl.AllocLevel != true {
		t.Fatal("allocLevel should be true")
	}
}

func TestCacheSnapshot_TopConsumers(t *testing.T) {
	c := NewCache(10)
	c.Update(nil, []nomad.AllocLoad{
		{AllocID: "a1", JobID: "job-1", Tasks: map[string]nomad.TaskUsage{"t": {CPU: 500, Memory: 100}}},
		{AllocID: "a2", JobID: "job-2", Tasks: map[string]nomad.TaskUsage{"t": {CPU: 800, Memory: 50}}},
		{AllocID: "a3", JobID: "job-3", Tasks: map[string]nomad.TaskUsage{"t": {CPU: 300, Memory: 900}}},
	}, true)
	cl := c.Snapshot()
	if len(cl.TopConsumers) != 3 {
		t.Fatalf("top = %d", len(cl.TopConsumers))
	}
	if cl.TopConsumers[0].AllocID != "a2" || cl.TopConsumers[0].CPU != 800 {
		t.Fatalf("top1 = %+v", cl.TopConsumers[0])
	}
}

func TestCacheClusterSamples_Ring(t *testing.T) {
	orig := nowFunc
	t.Cleanup(func() { nowFunc = orig })
	base := int64(1_700_000_000_000)
	tick = 0
	nowFunc = func() int64 { return base + tick }
	c := NewCache(3)
	for i := 0; i < 5; i++ {
		tick++
		c.Update([]nomad.NodeLoad{nodeLoad("n1", float64(i)*100, true)}, nil, true)
	}
	cl := c.Snapshot()
	if len(cl.Samples) != 3 {
		t.Fatalf("cluster samples = %d, want 3 (ring cap)", len(cl.Samples))
	}
	if cl.Samples[0].CPU != 200 {
		t.Fatalf("oldest sample CPU = %v, want 200 (dropped 0,100)", cl.Samples[0].CPU)
	}
}

var tick int64
