import { GetClusterLoad, GetNodeLoads } from '../../../bindings/github.com/nphq/np/internal/app/app'
import { Events } from '@wailsio/runtime'
import { isErr, toastErr } from './clusters.svelte'
import { createEpoch } from './epoch.svelte'
import type { nomad } from '../types/wails'

// loads.svelte.ts —— 集群负载 store。
// 数据流：GetClusterLoad/GetNodeLoads 首拉 → load.patch 事件增量合入。
// 竞态防护（评审 P0-2）：
//  - 首拉/刷新用 epoch guard，快速切集群时旧响应不得覆盖新数据；
//  - load.patch 事件无 token，用 payload 的 clusterID 与激活集群比对过滤
//    （后端 LoadPatch 已补 ClusterID 字段）。

interface LoadPatch {
  clusterID?: string
  nodes?: nomad.NodeLoad[]
  allocs?: nomad.AllocLoad[]
  cluster: nomad.ClusterLoad
}

export type LoadsState = {
  nodes: Map<string, nomad.NodeLoad>
  allocs: Map<string, nomad.AllocLoad>
  cluster: nomad.ClusterLoad | null
  loading: boolean
  stale: boolean
  lastUpdate: number | null
}

function empty(): LoadsState {
  return {
    nodes: new Map(),
    allocs: new Map(),
    cluster: null,
    loading: false,
    stale: false,
    lastUpdate: null,
  }
}

export function createLoadsStore() {
  const state = $state<LoadsState>(empty())

  // 首拉/刷新的 epoch guard（事件流不走这里，见 load.patch 的 clusterID 过滤）。
  const epoch = createEpoch()
  // 当前数据所属集群（refresh 同步置位；clear 清空）。事件流用它过滤。
  let currentClusterID: string | null = null

  // 订阅后端 load.patch；payload 是 LoadPatch（无 Envelope 包装）。
  Events.On('load.patch', (ev) => {
    const p = ev.data as LoadPatch | null
    if (!p) return
    // 激活集群切换后，旧集群的最后一个 patch 可能晚于新集群首拉到达：
    // clusterID 不匹配直接丢弃（后端早于 ClusterID 字段的版本无此字段，
    // 走 `p.clusterID &&` 短路保留兼容）。
    if (p.clusterID && currentClusterID && p.clusterID !== currentClusterID) return
    if (p.nodes) {
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      const next = new Map(state.nodes)
      for (const n of p.nodes) {
        if (n.removed) next.delete(n.nodeID)
        else next.set(n.nodeID, n)
      }
      state.nodes = next
    }
    if (p.allocs) {
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      const next = new Map(state.allocs)
      for (const a of p.allocs) next.set(a.allocID, a)
      state.allocs = next
    }
    if (p.cluster) {
      state.cluster = p.cluster
      state.lastUpdate = p.cluster.updatedAt || Date.now()
      state.stale = false
    }
  })

  // refresh 首拉/手动刷新：集群聚合 + 节点负载并行。
  async function refresh(clusterID: string): Promise<void> {
    const mine = epoch.acquire()
    currentClusterID = clusterID
    state.loading = true
    try {
      const [cl, nodes] = await Promise.all([GetClusterLoad(clusterID), GetNodeLoads(clusterID)])
      if (!epoch.active(mine)) return // 被更新的 refresh 作废
      if (isErr(cl)) {
        toastErr(cl)
        state.stale = true
        return
      }
      if (isErr(nodes)) {
        toastErr(nodes)
        state.stale = true
        return
      }
      state.cluster = cl as nomad.ClusterLoad
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      state.nodes = new Map((nodes as nomad.NodeLoad[]).map((n) => [n.nodeID, n]))
      state.lastUpdate = state.cluster.updatedAt || Date.now()
      state.stale = false
    } catch {
      if (epoch.active(mine)) state.stale = true
    } finally {
      // loading 属于 UI 信号：被更新请求作废的响应照常复位，
      // 否则 setActive 等仅作废不重拉的场景会卡死 spinner。
      state.loading = false
    }
  }

  function clear(): void {
    epoch.invalidate() // 作废在飞首拉/刷新
    currentClusterID = null
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    state.nodes = new Map()
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    state.allocs = new Map()
    state.cluster = null
    state.stale = false
    state.lastUpdate = null
  }

  return {
    get state() {
      return state
    },
    refresh,
    clear,
  }
}
