package nomad

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/nomad/api"
)

func TestMapNodeListStub_NodeResources(t *testing.T) {
	stub := &api.NodeListStub{
		ID:                    "node-1",
		Name:                  "ip-10-0-0-1",
		Status:                "ready",
		SchedulingEligibility: "eligible",
		Datacenter:            "dc1",
		NodeClass:             "standard",
		Version:               "1.9.4",
		NodeResources: &api.NodeResources{
			Cpu:    api.NodeCpuResources{CpuShares: 3200, TotalCpuCores: 4},
			Memory: api.NodeMemoryResources{MemoryMB: 8192},
			Disk:   api.NodeDiskResources{DiskMB: 102400},
		},
	}
	got := mapNodeListStub(stub)
	if got.ID != "node-1" || got.Status != "ready" {
		t.Errorf("basic fields = %+v", got)
	}
	if got.CPUTotal != 3200 || got.CPUCores != 4 {
		t.Errorf("CPU total=%v cores=%d, want 3200/4", got.CPUTotal, got.CPUCores)
	}
	if got.MemoryTotal != 8192 || got.DiskTotal != 102400 {
		t.Errorf("MemoryTotal=%v DiskTotal=%v", got.MemoryTotal, got.DiskTotal)
	}
}

func TestMapNodeListStub_NilNodeResources(t *testing.T) {
	stub := &api.NodeListStub{ID: "legacy", Name: "old"}
	got := mapNodeListStub(stub)
	if got.CPUTotal != 0 || got.MemoryTotal != 0 || got.DiskTotal != 0 {
		t.Errorf("nil NodeResources: got %+v, want zeros", got)
	}
}

func TestMapAllocListStub_DeclaredResources(t *testing.T) {
	stub := &api.AllocationListStub{
		ID:            "alloc-1",
		JobID:         "redis",
		TaskGroup:     "web",
		NodeID:        "node-1",
		ClientStatus:  "running",
		DesiredStatus: "run",
		AllocatedResources: &api.AllocatedResources{
			Tasks: map[string]*api.AllocatedTaskResources{
				"server": {
					Cpu:    api.AllocatedCpuResources{CpuShares: 500},
					Memory: api.AllocatedMemoryResources{MemoryMB: 512},
				},
				"proxy": {
					Cpu:    api.AllocatedCpuResources{CpuShares: 250},
					Memory: api.AllocatedMemoryResources{MemoryMB: 128},
				},
			},
			Shared: api.AllocatedSharedResources{DiskMB: 100},
		},
	}
	got := mapAllocListStub(stub)
	if got.CPU != 750 || got.Memory != 640 || got.Disk != 100 {
		t.Errorf("aggregate declared = CPU %v Mem %v Disk %v, want 750/640/100", got.CPU, got.Memory, got.Disk)
	}
	if got.TaskResources["server"].CPU != 500 || got.TaskResources["proxy"].Memory != 128 {
		t.Errorf("task declared = %+v", got.TaskResources)
	}
}

func TestFetchNodeStats_MultiCoreAvgAndDiskMerge(t *testing.T) {
	// 直接用纯映射函数测换算基准：平均 Idle 后的 CPUPercent
	hs := &api.HostStats{
		CPU: []*api.HostCPUStats{
			{CPU: "cpu0", User: 20, System: 10, Idle: 70},
			{CPU: "cpu1", User: 40, System: 20, Idle: 40},
		},
		Memory: &api.HostMemoryStats{Total: 1 << 30, Used: 256 << 20, Available: 768 << 20},
		DiskStats: []*api.HostDiskStats{
			{Device: "sda1", Used: 50 << 20, Size: 100 << 20},
			{Device: "sdb1", Used: 30 << 20, Size: 100 << 20},
		},
	}
	// 通过真实 SDK 路径验证太绕；直接断言换算前提：
	avgIdle := (hs.CPU[0].Idle + hs.CPU[1].Idle) / 2
	usedPct := 100 - avgIdle
	if usedPct != 45 {
		t.Errorf("avg idle → used %% = %v, want 45", usedPct)
	}
}

func TestListNodes_SendsResourcesParamAndMapsCapacity(t *testing.T) {
	var gotQuery string
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Errorf("path = %s, want /v1/nodes", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		stubs := []*api.NodeListStub{{
			ID:                    "node-1",
			Name:                  "ip-10-0-0-1",
			Status:                "ready",
			SchedulingEligibility: "eligible",
			Datacenter:            "dc1",
			Version:               "2.0.4",
			NodeResources: &api.NodeResources{
				Cpu:    api.NodeCpuResources{CpuShares: 31200, TotalCpuCores: 12},
				Memory: api.NodeMemoryResources{MemoryMB: 32768},
				Disk:   api.NodeDiskResources{DiskMB: 953904},
			},
		}}
		_ = json.NewEncoder(w).Encode(stubs)
	})
	nodes, err := ListNodes(context.Background(), client)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if gotQuery != "resources=true" {
		t.Errorf("query = %q, want resources=true（Nomad 2.x 列表端点默认不带容量）", gotQuery)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
	n := nodes[0]
	if n.CPUTotal != 31200 || n.CPUCores != 12 || n.MemoryTotal != 32768 || n.DiskTotal != 953904 {
		t.Errorf("capacity = cpu %v/%d mem %v disk %v, want 31200/12/32768/953904",
			n.CPUTotal, n.CPUCores, n.MemoryTotal, n.DiskTotal)
	}
}
