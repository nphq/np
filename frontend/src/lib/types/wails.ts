// 前端自有的 Wails 后端类型镜像。
//
// 为什么不用 frontend/bindings/.../models.js？
// Wails v3 的绑定生成器只扫描 bound method 签名来决定导出哪些类型。
// 为了避开 Wails 调度器对 typed-nil *uiapi.Error 的处理（详见 app.go 注释），
// App 方法都改成了 (any, error) 签名，结果就是 *uiapi.Error / nomad.ClusterInfo
// 不再出现在签名里、被生成器丢掉。每次 `wails3 generate bindings`
// 都会重写 models.js，所以这里手动维护一份镜像，让前端类型检查稳定。
//
// 字段必须和 Go 端 uiapi.Error / nomad.ClusterInfo 的 JSON tag 对齐。
//
// namespace 是刻意为之：uiapi.Error 避免与全局 Error 冲突；
// nomad 前缀让 DTO 与 Go 包名一一对应。
/* eslint-disable @typescript-eslint/no-namespace */

export namespace uiapi {
  export interface Error {
    code: string
    message: string
  }

  // DiscoveredCluster 是「从环境/文件发现」的候选，供 UI 预填。
  // token 明文绝不进响应（hasToken 只表示是否有值）。
  export interface DiscoveredCluster {
    source: string // "env" | "file"
    suggestedID: string
    name: string
    address: string
    region: string
    namespace: string
    tls: boolean
    insecureSkipVerify: boolean
    hasToken: boolean
  }
}

// Page 是顶层路由枚举：Overview/Jobs/Allocs/Nodes/Apps + Job 详情 + Run Job + Settings。
export type Page =
  | 'overview'
  | 'jobs'
  | 'allocs'
  | 'nodes'
  | 'apps'
  | 'job-detail'
  | 'node-detail'
  | 'job-run'
  | 'settings'

export namespace nomad {
  export interface ClusterInfo {
    id: string
    name: string
    address: string
    region: string
    namespace: string
    tls: boolean
    insecureSkipVerify: boolean
    hasToken: boolean
    health: string
    lastChecked: number
    pinned: boolean
    sortOrder: number
  }

  // ClusterList 是 ListClusters 的响应：已排序列表 + 活跃集群 ID。
  // activeID 由后端唯一裁决，前端不做本地双源。
  export interface ClusterList {
    clusters: ClusterInfo[]
    activeID: string
  }

  // ClusterHealth 是 TestConnection / TestConnectionInput 的返回。
  export interface ClusterHealth {
    status: string // "ok" | "down"
    leader: string
    version: string
    namespace: string
    error: string
  }

  export interface AllocSummary {
    id: string
    jobID: string
    taskGroup: string
    nodeID: string
    nodeName: string
    status: string
    clientStatus: string
    desiredStatus: string
    evalID: string
    createIndex: number
    modifyIndex: number
    cpu: number
    memory: number
    disk: number
    taskResources?: Record<string, ResourceUsage>
  }

  export interface NodeSummary {
    id: string
    name: string
    status: string
    schedulingEligibility: string
    datacenter: string
    region: string
    class: string
    version: string
    cpu: number // 已用 MHz
    cpuTotal: number
    cpuCores: number
    memory: number // 已用 MB
    memoryTotal: number
    disk: number
    diskTotal: number
    runningAllocs: number
    /** 节点已指纹检测到的任务驱动（Detected=true） */
    drivers?: string[]
  }

  // --- 负载 DTO＼�---

  export interface ResourceUsage {
    cpu: number // MHz
    memory: number // MB
    disk: number // MB
  }

  export interface LoadSample {
    time: number // ms epoch
    cpu: number
    memory: number
    disk: number
  }

  export interface NodeLoad {
    nodeID: string
    capacity: ResourceUsage
    allocated: ResourceUsage
    used: ResourceUsage
    samples: LoadSample[]
    available: boolean
    runningAllocs: number
    removed?: boolean
  }

  export interface TaskUsage {
    cpu: number // MHz
    memory: number // MB
    pct: number // 0-1 相对声明资源
  }

  export interface AllocLoad {
    allocID: string
    nodeID: string
    jobID: string
    tasks: Record<string, TaskUsage>
    time: number
  }

  export interface AllocConsumer {
    allocID: string
    jobID: string
    cpu: number
    memory: number
  }

  export interface ClusterLoad {
    capacity: ResourceUsage
    allocated: ResourceUsage
    used: ResourceUsage
    nodeCount: number
    nodeUp: number
    allocLevel: boolean
    topConsumers?: AllocConsumer[]
    samples: LoadSample[]
    updatedAt: number
  }

  // --- Job 部署/管理 DTO＼�---

  export interface JobSummary {
    id: string
    name: string
    type: string
    priority: number
    status: string
    running: number
    queued: number
    pending: number
    failed: number
    dead: number
  }

  export interface TaskGroupInfo {
    name: string
    count: number
    running: number
    queued: number
    pending: number
    failed: number
    complete: number
    lost: number
  }

  export interface JobDetail {
    id: string
    name: string
    namespace: string
    type: string
    status: string
    priority: number
    datacenters: string[]
    createIndex: number
    modifyIndex: number
    summary: JobSummary
    taskGroups: TaskGroupInfo[]
  }

  export interface JobRunResult {
    jobID: string
    evalID: string
    warnings?: string
    modifyIndex: number
  }

  export interface EvalInfo {
    id: string
    jobID: string
    status: string
    statusDescription: string
    type: string
    priority: number
    blockedEval?: string
    waitUntil?: number
    failedSummary?: string
  }

  export interface AllocTaskEvent {
    task: string
    type: string
    time: number
    message: string
    fails: boolean
  }

  export interface AllocLogsResult {
    allocID: string
    task: string
    logType: string
    content: string
    truncated: boolean
  }
}
