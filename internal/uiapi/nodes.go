package uiapi

import (
	"context"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/nomad"
)

// NodesService 承载节点列表的 IPC 逻辑。
// ListNodes 返回静态容量（NodeResources）；used/allocated 从 LoadsService
// 的 metrics cache 派生（ADR-11：负载数据单一来源，避免双写）。
type NodesService struct {
	pool  *cluster.Pool
	loads *LoadsService
}

// NewNodesService 创建节点服务。
func NewNodesService(pool *cluster.Pool, loads *LoadsService) *NodesService {
	return &NodesService{pool: pool, loads: loads}
}

// ListNodes 返回集群下的节点列表（容量 + 实时负载）。
func (s *NodesService) ListNodes(ctx context.Context, clusterID string) ([]nomad.NodeSummary, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	nodes, err := nomad.ListNodes(ctx, client)
	if err != nil {
		return nil, Wrap(err)
	}

	// 用负载缓存补 used/allocated（缓存为空时保持 0，前端显示 loading）
	byID := make(map[string]nomad.NodeLoad, len(nodes))
	for _, nl := range s.loads.NodeLoads(ctx, clusterID) {
		byID[nl.NodeID] = nl
	}
	for i := range nodes {
		if nl, ok := byID[nodes[i].ID]; ok {
			nodes[i].CPU = nl.Used.CPU
			nodes[i].CPUTotal = nl.Capacity.CPU
			nodes[i].Memory = nl.Used.Memory
			nodes[i].MemoryTotal = nl.Capacity.Memory
			nodes[i].Disk = nl.Used.Disk
			nodes[i].DiskTotal = nl.Capacity.Disk
			nodes[i].RunningAllocs = nl.RunningAllocs
		}
	}
	return nodes, nil
}
