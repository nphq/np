package nomad

// 本文件定义跨 IPC 的稳定 DTO，隔离 hashicorp/nomad/api SDK 类型漂移（report §18 ADR-2）。
// 任何 SDK struct 字段变更都不会直接影响前端，由本层显式映射。

// ClusterInfo 是前端可见的集群信息（含健康状态）。
type ClusterInfo struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Address            string `json:"address"`
	Region             string `json:"region"`
	Namespace          string `json:"namespace"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	HasToken           bool   `json:"hasToken"` // Keychain 里是否有 token（不暴露 token 本身）
	// HasLegacyToken：品牌统一前的旧 service 里存有 token，等待用户编辑重录完成迁移。
	HasLegacyToken bool   `json:"hasLegacyToken,omitempty"`
	Health         string `json:"health"` // "unknown" | "ok" | "down"
	LastChecked    int64  `json:"lastChecked"`
	Pinned         bool   `json:"pinned"` // 收藏/置顶
	SortOrder      int    `json:"sortOrder"`
}

// ClusterList 是 ListClusters 的响应：集群列表（已按 §3.1 排序）+ 活跃集群 ID。
// activeID 由后端唯一裁决，前端不做本地双源。
type ClusterList struct {
	Clusters []ClusterInfo `json:"clusters"`
	ActiveID string        `json:"activeID"`
}

// ClusterHealth 是 TestConnection / 健康检查的返回。
type ClusterHealth struct {
	Status    string `json:"status"` // "ok" | "down"
	Leader    string `json:"leader"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
	Error     string `json:"error,omitempty"`
}

// JobSummary 是任务列表行（首屏全量 + 事件增量共用的最小集）。
type JobSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Priority int    `json:"priority"`
	Status   string `json:"status"`
	// Running/Queued/Pending/Failed/Dead 各状态 alloc 数
	Running int `json:"running"`
	Queued  int `json:"queued"`
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
	Dead    int `json:"dead"`
}

// JobDetail 是任务详情页。
type JobDetail struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Priority    int             `json:"priority"`
	Datacenters []string        `json:"datacenters"`
	CreateIndex uint64          `json:"createIndex"`
	ModifyIndex uint64          `json:"modifyIndex"`
	Summary     JobSummary      `json:"summary"`
	TaskGroups  []TaskGroupInfo `json:"taskGroups"`
}

// TaskGroupInfo 是 Job 详情页里每个 task group 的运行态汇总。
// Count 来自 HCL 定义（期望实例数）；其余字段来自 JobSummary.Summary[name] 的运行态计数。
type TaskGroupInfo struct {
	Name     string `json:"name"`
	Count    int    `json:"count"` // HCL 声明的期望实例数
	Running  int    `json:"running"`
	Queued   int    `json:"queued"`
	Pending  int    `json:"pending"` // Starting + Unknown 归入 Pending
	Failed   int    `json:"failed"`
	Complete int    `json:"complete"`
	Lost     int    `json:"lost"`
}

// JobRunResult 是 RunJob 的提交回执（部署 + 更新同一入口）。
type JobRunResult struct {
	JobID       string `json:"jobID"`
	EvalID      string `json:"evalID"`
	Warnings    string `json:"warnings,omitempty"`
	ModifyIndex uint64 `json:"modifyIndex"`
}

// EvalInfo 是评估状态（部署进度用）。
type EvalInfo struct {
	ID                string `json:"id"`
	JobID             string `json:"jobID"`
	Status            string `json:"status"`
	StatusDescription string `json:"statusDescription"`
	Type              string `json:"type"`
	Priority          int    `json:"priority"`
	BlockedEval       string `json:"blockedEval,omitempty"`
	WaitUntil         int64  `json:"waitUntil,omitempty"`
	// FailedSummary 是调度失败的人类可读摘要（节点不足等）。
	FailedSummary string `json:"failedSummary,omitempty"`
}

// AllocTaskEvent 是任务状态事件（启动失败原因等）。
type AllocTaskEvent struct {
	Task    string `json:"task"`
	Type    string `json:"type"`
	Time    int64  `json:"time"` // ms epoch
	Message string `json:"message"`
	Fails   bool   `json:"fails"`
}

// AllocLogsResult 是分配任务日志快照（非 follow）。
type AllocLogsResult struct {
	AllocID   string `json:"allocID"`
	Task      string `json:"task"`
	LogType   string `json:"logType"` // stdout | stderr
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// JobValidateResult 是服务端校验结果（RunJob 前置步骤的内联报错载体）。
type JobValidateResult struct {
	DriverConfigValidated bool     `json:"driverConfigValidated"`
	ValidationErrors      []string `json:"validationErrors"`
	Warnings              string   `json:"warnings"`
}

// AllocSummary 是分配列表行。
type AllocSummary struct {
	ID            string `json:"id"`
	JobID         string `json:"jobID"`
	TaskGroup     string `json:"taskGroup"`
	NodeID        string `json:"nodeID"`
	NodeName      string `json:"nodeName"`
	Status        string `json:"status"`
	ClientStatus  string `json:"clientStatus"`
	DesiredStatus string `json:"desiredStatus"`
	EvalID        string `json:"evalID"`
	CreateIndex   uint64 `json:"createIndex"`
	ModifyIndex   uint64 `json:"modifyIndex"`
	// 声明资源（AllocatedResources 聚合）：CPU MHz / Memory MB / Disk MB
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
	// TaskResources 是 per-task 声明资源（Pct 计算用；列表 stub 有则填）
	TaskResources map[string]ResourceUsage `json:"taskResources,omitempty"`
}

// NodeSummary 是节点列表行。
// CPU/CPUTotal 单位为 MHz，Memory/MemoryTotal 单位为 MB，Disk/DiskTotal 为 MB。
// 静态容量来自 NodeResources（ListNodes 时返回）；used/allocated 由负载
// Collector 写入（uiapi/nodes.go 从 metrics cache 派生，ADR-11 单一数据源）。
type NodeSummary struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	Status                string  `json:"status"`
	SchedulingEligibility string  `json:"schedulingEligibility"`
	Datacenter            string  `json:"datacenter"`
	Region                string  `json:"region"`
	Class                 string  `json:"class"`
	Version               string  `json:"version"`
	CPU                   float64 `json:"cpu"` // 已用 MHz
	CPUTotal              float64 `json:"cpuTotal"`
	CPUCores              int     `json:"cpuCores"`
	Memory                float64 `json:"memory"` // 已用 MB
	MemoryTotal           float64 `json:"memoryTotal"`
	Disk                  float64 `json:"disk"` // 已用 MB
	DiskTotal             float64 `json:"diskTotal"`
	RunningAllocs         int     `json:"runningAllocs"`
	// Drivers 是节点已指纹检测到的任务驱动名（Detected=true），用于部署前校验。
	Drivers []string `json:"drivers,omitempty"`
}

// --- 负载 DTO---

// ResourceUsage 是统一的三元组（CPU MHz / Memory MB / Disk MB）。
type ResourceUsage struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
}

// LoadSample 是历史曲线的一个点（环形缓冲，默认 60 点）。
type LoadSample struct {
	Time   int64   `json:"time"` // ms epoch
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
}

// NodeLoad 是 Nodes 屏用量条的数据源（load.patch 增量 / GetNodeLoads 首拉）。
type NodeLoad struct {
	NodeID        string        `json:"nodeID"`
	Capacity      ResourceUsage `json:"capacity"`
	Allocated     ResourceUsage `json:"allocated"`
	Used          ResourceUsage `json:"used"`
	Samples       []LoadSample  `json:"samples"`
	Available     bool          `json:"available"` // HostStats 本 tick 是否成功
	RunningAllocs int           `json:"runningAllocs"`
	Removed       bool          `json:"removed,omitempty"` // load.patch 删除标记
}

// TaskUsage 单个 task 的用量；Pct 为相对 alloc 声明资源的占比（0-1）。
type TaskUsage struct {
	CPU    float64 `json:"cpu"`    // MHz
	Memory float64 `json:"memory"` // MB
	Pct    float64 `json:"pct"`
}

// AllocLoad 是 per-task 用量（Job 详情页 / top 消费者）。
type AllocLoad struct {
	AllocID string               `json:"allocID"`
	NodeID  string               `json:"nodeID"` // 所在节点（NodeDetail 的 alloc 列表按此过滤）
	JobID   string               `json:"jobID"`
	Tasks   map[string]TaskUsage `json:"tasks"`
	Time    int64                `json:"time"` // ms epoch
}

// AllocConsumer 是 top 消费排行行。
type AllocConsumer struct {
	AllocID string  `json:"allocID"`
	JobID   string  `json:"jobID"`
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"memory"`
}

// ClusterLoad 是 Overview 页的聚合快照。
type ClusterLoad struct {
	Capacity     ResourceUsage   `json:"capacity"`
	Allocated    ResourceUsage   `json:"allocated"`
	Used         ResourceUsage   `json:"used"`
	NodeCount    int             `json:"nodeCount"`
	NodeUp       int             `json:"nodeUp"`
	AllocLevel   bool            `json:"allocLevel"` // A2 alloc 级统计是否开启（>200 自动降级）
	TopConsumers []AllocConsumer `json:"topConsumers,omitempty"`
	Samples      []LoadSample    `json:"samples"`
	UpdatedAt    int64           `json:"updatedAt"` // ms epoch
}
