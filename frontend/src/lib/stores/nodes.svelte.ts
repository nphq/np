import { ListNodes } from '../../../bindings/github.com/nphq/np/internal/app/app'
import { isErr, toastErr } from './clusters.svelte'
import { createEpoch } from './epoch.svelte'
import type { nomad } from '../types/wails'

// nodes.svelte.ts —— 节点列表 store。
// 行数据 = NodeSummary（ListNodes，含容量与负载缓存派生值，ADR-11）；
// 实时用量条的增量来自 loads store（load.patch），此处只做首拉/清空。
// refresh 用 epoch guard：快速切集群时旧响应不得覆盖新上下文。

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

  const epoch = createEpoch()

  async function refresh(clusterID: string): Promise<void> {
    const mine = epoch.acquire()
    state.loading = true
    try {
      const res = await ListNodes(clusterID)
      if (!epoch.active(mine)) return
      if (isErr(res)) {
        toastErr(res)
        return
      }
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      state.byId = new Map((res as nomad.NodeSummary[]).map((n) => [n.id, n]))
    } catch (err) {
      if (epoch.active(mine)) console.error('[nodes] refresh failed:', err)
    } finally {
      // loading 属于 UI 信号：被更新请求作废的响应照常复位，
      // 否则 setActive 等仅作废不重拉的场景会卡死 spinner。
      state.loading = false
    }
  }

  function clear(): void {
    epoch.invalidate() // 作废在飞请求（切集群）
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
