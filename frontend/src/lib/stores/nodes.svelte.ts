import { ListNodes } from '../../../bindings/github.com/nphq/np/app'
import { isErr, toastErr } from './clusters.svelte'
import type { nomad } from '../types/wails'

// nodes.svelte.ts —— 节点列表 store。
// 行数据 = NodeSummary（ListNodes，含容量与负载缓存派生值，ADR-11）；
// 实时用量条的增量来自 loads store（load.patch），此处只做首拉/清空。

export function createNodesStore() {
  const state = $state<{
    byId: Map<string, nomad.NodeSummary>
    loading: boolean
  }>({
    // 整表替换，不原地 mutate（同 jobs store 约定）
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    byId: new Map(),
    loading: false,
  })

  const list = $derived([...state.byId.values()])

  async function refresh(clusterID: string): Promise<void> {
    state.loading = true
    try {
      const res = await ListNodes(clusterID)
      if (isErr(res)) {
        toastErr(res)
        return
      }
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      state.byId = new Map((res as nomad.NodeSummary[]).map((n) => [n.id, n]))
    } catch (err) {
      console.error('[nodes] refresh failed:', err)
    } finally {
      state.loading = false
    }
  }

  function clear(): void {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    state.byId = new Map()
  }

  return {
    get state() {
      return state
    },
    get list() {
      return list
    },
    refresh,
    clear,
  }
}
