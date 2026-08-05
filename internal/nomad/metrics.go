package nomad

import (
	"fmt"
	"time"

	"github.com/hashicorp/nomad/api"
)

// nowMillis 是采样时间戳（ms epoch）。
func nowMillis() int64 { return time.Now().UnixMilli() }

// 节点 / alloc 实时统计的 SDK → DTO 纯映射。
// 与列表映射的区别：这里保留**原始单位**（CPU 百分比、内存字节），
// MHz/MB 换算需要容量信息（per-core MHz），由 metrics 包在拿到
// NodeResources 之后完成（纯函数可单测）。

// NodeStats 是节点级 HostStats 的原始快照。
type NodeStats struct {
	// CPUPercent 为所有核的平均使用率（0-100）。HostStats.CPU 是 per-core
	// 的 User/System/Idle 百分比，使用率 = 100 - Idle。
	CPUPercent   float64
	MemoryUsedMB float64
	DiskUsedMB   float64
	Time         int64 // ms epoch
}

// AllocStats 是 allocation 级 AllocResourceUsage 的原始快照。
type AllocStats struct {
	AllocID string
	// Tasks 的 CPUPercent 是单核百分比（100 = 满 1 核），内存为 RSS 字节换算 MB。
	Tasks map[string]TaskStats
	Time  int64
}

// TaskStats 是单个 task 的原始统计。
type TaskStats struct {
	CPUPercent   float64
	MemoryUsedMB float64
}

// FetchNodeStats 拉取节点实时用量（每节点 1 调用，A1 源）。
func FetchNodeStats(client *api.Client, nodeID string) (*NodeStats, error) {
	hs, err := client.Nodes().Stats(nodeID, nil)
	if err != nil {
		return nil, fmt.Errorf("node %s stats: %w", nodeID, err)
	}
	ns := &NodeStats{Time: nowMillis()}
	if hs.Memory != nil {
		ns.MemoryUsedMB = float64(hs.Memory.Used) / 1e6
	}
	for _, c := range hs.CPU {
		ns.CPUPercent += 100 - c.Idle
	}
	if n := len(hs.CPU); n > 0 {
		ns.CPUPercent /= float64(n)
	}
	for _, d := range hs.DiskStats {
		ns.DiskUsedMB += float64(d.Used) / 1e6
	}
	return ns, nil
}

// FetchAllocStats 拉取 allocation 实时用量（每 alloc 1 调用，A2 源）。
func FetchAllocStats(client *api.Client, allocID string) (*AllocStats, error) {
	ar, err := client.Allocations().Stats(&api.Allocation{ID: allocID}, nil)
	if err != nil {
		return nil, fmt.Errorf("alloc %s stats: %w", allocID, err)
	}
	as := &AllocStats{
		AllocID: allocID,
		Tasks:   make(map[string]TaskStats, len(ar.Tasks)),
		Time:    nowMillis(),
	}
	for name, t := range ar.Tasks {
		if t == nil || t.ResourceUsage == nil {
			continue
		}
		ts := TaskStats{}
		if c := t.ResourceUsage.CpuStats; c != nil {
			ts.CPUPercent = c.Percent
		}
		if m := t.ResourceUsage.MemoryStats; m != nil {
			ts.MemoryUsedMB = float64(m.RSS) / 1e6
		}
		as.Tasks[name] = ts
	}
	return as, nil
}
