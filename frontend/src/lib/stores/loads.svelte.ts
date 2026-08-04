import { GetClusterLoad, GetNodeLoads } from '../../../bindings/github.com/nphq/np/app'
import { Events } from '@wailsio/runtime'
import { isErr, toastErr } from './clusters.svelte'
import type { nomad } from '../types/wails'

// loads.svelte.ts —— 集群负载 store。
// 数据流：GetClusterLoad/GetNodeLoads 首拉 → load.patch 事件增量合入。

interface LoadPatch {
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

  // 订阅后端 load.patch；payload 是 LoadPatch（无 Envelope 包装）。
  Events.On('load.patch', (ev) => {
    const p = ev.data as LoadPatch | null
    if (!p) return
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
    state.loading = true
    try {
      const [cl, nodes] = await Promise.all([GetClusterLoad(clusterID), GetNodeLoads(clusterID)])
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
      state.stale = true
    } finally {
      state.loading = false
    }
  }

  function clear(): void {
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
