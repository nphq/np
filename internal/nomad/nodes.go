package nomad

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/nomad/api"
)

// 节点 / allocation 列表的 SDK → DTO 纯映射（ADR-2，同 jobs.go 约定）。
// NodeResources 是 Nomad 1.x+ 的容量来源：CpuShares 为总 MHz（1.x 为 /1024 的
// share 换算，2.x 直接是 MHz），TotalCpuCores 为物理核数。

// ListNodes 拉取节点列表，填充容量与静态属性。used/allocated 由 metrics
// Collector 侧补（见 internal/metrics 与 uiapi/nodes.go）。
func ListNodes(ctx context.Context, client *api.Client) ([]NodeSummary, error) {
	// Nomad 2.x 的 /v1/nodes 列表端点默认不返回 NodeResources（重字段），
	// 需显式传 ?resources=true 才带上容量（1.x 默认带）。
	stubs, _, err := client.Nodes().List((&api.QueryOptions{
		Params: map[string]string{"resources": "true"},
	}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	out := make([]NodeSummary, 0, len(stubs))
	for _, n := range stubs {
		out = append(out, mapNodeListStub(n))
	}
	return out, nil
}

func mapNodeListStub(n *api.NodeListStub) NodeSummary {
	ns := NodeSummary{
		ID:                    n.ID,
		Name:                  n.Name,
		Status:                n.Status,
		SchedulingEligibility: n.SchedulingEligibility,
		Datacenter:            n.Datacenter,
		Class:                 n.NodeClass,
		Version:               n.Version,
	}
	if n.NodeResources != nil {
		ns.CPUTotal = float64(n.NodeResources.Cpu.CpuShares)
		ns.CPUCores = int(n.NodeResources.Cpu.TotalCpuCores)
		ns.MemoryTotal = float64(n.NodeResources.Memory.MemoryMB)
		ns.DiskTotal = float64(n.NodeResources.Disk.DiskMB)
	}
	if len(n.Drivers) > 0 {
		drivers := make([]string, 0, len(n.Drivers))
		for name, info := range n.Drivers {
			if info != nil && info.Detected {
				drivers = append(drivers, name)
			}
		}
		sort.Strings(drivers)
		ns.Drivers = drivers
	}
	return ns
}

// ListAllocations 拉取全量 allocation 列表（含 per-task 声明资源与节点归属）。
// 供 metrics Collector 聚合 allocated 与 running allocs 使用。
// namespace 为空时列表仅含 default namespace（服务端语义），与旧版行为一致。
func ListAllocations(ctx context.Context, client *api.Client, namespace string) ([]AllocSummary, error) {
	stubs, _, err := client.Allocations().List((&api.QueryOptions{Namespace: namespace}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list allocations: %w", err)
	}
	out := make([]AllocSummary, 0, len(stubs))
	for _, a := range stubs {
		out = append(out, mapAllocListStub(a))
	}
	return out, nil
}

func mapAllocListStub(a *api.AllocationListStub) AllocSummary {
	as := AllocSummary{
		ID:            a.ID,
		JobID:         a.JobID,
		TaskGroup:     a.TaskGroup,
		NodeID:        a.NodeID,
		NodeName:      a.NodeName,
		ClientStatus:  a.ClientStatus,
		DesiredStatus: a.DesiredStatus,
		EvalID:        a.EvalID,
		CreateIndex:   a.CreateIndex,
		ModifyIndex:   a.ModifyIndex,
	}
	as.Status = a.ClientStatus
	if a.AllocatedResources != nil {
		if len(a.AllocatedResources.Tasks) > 0 {
			as.TaskResources = make(map[string]ResourceUsage, len(a.AllocatedResources.Tasks))
		}
		for name, t := range a.AllocatedResources.Tasks {
			as.CPU += float64(t.Cpu.CpuShares)
			as.Memory += float64(t.Memory.MemoryMB)
			if as.TaskResources != nil {
				as.TaskResources[name] = ResourceUsage{
					CPU:    float64(t.Cpu.CpuShares),
					Memory: float64(t.Memory.MemoryMB),
				}
			}
		}
		as.Disk = float64(a.AllocatedResources.Shared.DiskMB)
	}
	return as
}
