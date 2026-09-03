import { ListClusterAllocations } from '../../../bindings/github.com/nphq/np/internal/app/app'
import { isErr, toastErr } from './clusters.svelte'
import { createEpoch } from './epoch.svelte'
import type { nomad } from '../types/wails'

// allocs.svelte.ts —— 集群级 allocation 列表 store（Allocs 页）。
// 数据源：ListClusterAllocations（全量状态，区别于 loads cache 仅 running）。
// 与其它 store 同约定：epoch guard 防切集群竞态；整表替换不原地 mutate。

export function createAllocsStore() {
  const state = $state<{
    list: nomad.AllocSummary[]
    loading: boolean
  }>({ list: [], loading: false })

  const epoch = createEpoch()

  async function refresh(clusterID: string): Promise<void> {
    const mine = epoch.acquire()
    state.loading = true
    try {
      const res = await ListClusterAllocations(clusterID)
      if (!epoch.active(mine)) return // 切集群后旧响应作废
      if (isErr(res)) {
        toastErr(res)
        return
      }
      state.list = res as nomad.AllocSummary[]
    } catch (err) {
      if (epoch.active(mine)) console.error('[allocs] refresh failed:', err)
    } finally {
      state.loading = false
    }
  }

  function clear(): void {
    epoch.invalidate()
    state.list = []
    state.loading = false
  }

  return {
    get state() {
      return state
    },
    refresh,
    clear,
  }
}
