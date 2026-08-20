import {
  ListClusters,
  AddCluster,
  RemoveCluster,
  SetActiveCluster,
  UpdateCluster,
  PinCluster,
  DiscoverClusters,
  ImportFromEnv,
  TestConnection,
} from '../../../bindings/github.com/nphq/np/internal/app/app'
import { Events } from '@wailsio/runtime'
import type { uiapi, nomad } from '../types/wails'
import { createEpoch } from './epoch.svelte'

export class ClusterInput {
  id: string = ''
  name: string = ''
  address: string = ''
  region: string = ''
  namespace: string = ''
  tls: boolean = false
  insecureSkipVerify: boolean = false
  token: string = ''
  /** 为 true 且 token 为空时，后端用 NOMAD_TOKEN（不经前端明文往返填入输入框）。 */
  useEnvToken: boolean = false
}

export interface ClusterListItem {
  info: nomad.ClusterInfo
  // 探测后补充
  leader?: string
  version?: string
}

export type ClustersState = {
  clusters: ClusterListItem[]
  activeID: string | null
  loading: boolean
  // 环境/文件发现候选（null = 尚未探测）。Discover 纯读、不含 token。
  discovered: uiapi.DiscoveredCluster[] | null
  // 每次成功激活（含同 ID 再导入）递增，驱动工作负载强制重拉。
  activeEpoch: number
}

interface ClusterHealthPayload {
  clusterID: string
  status: string
  leader?: string
  version?: string
  lastChecked?: number
  error?: string
}

function empty(): ClustersState {
  return {
    clusters: [],
    activeID: null,
    loading: false,
    discovered: null,
    activeEpoch: 0,
  }
}

export function createClustersStore() {
  const state = $state<ClustersState>(empty())

  // 集群切换（setActive）时作废所有在飞请求；refresh 自己 acquire 的 token
  // 也会被后续 refresh 隐式作废（epoch 语义）。
  const epoch = createEpoch()

  // 订阅后端 cluster.health 事件；payload 自带 clusterID。
  Events.On('cluster.health', (ev) => {
    const p = ev.data as ClusterHealthPayload | null
    if (!p || !p.clusterID) return
    applyHealth(p)
  })

  function applyHealth(p: ClusterHealthPayload): void {
    const item = state.clusters.find((c) => c.info.id === p.clusterID)
    if (!item) return
    item.info.health = p.status
    if (p.lastChecked !== undefined) item.info.lastChecked = p.lastChecked
    if (p.leader !== undefined) item.leader = p.leader
    if (p.version !== undefined) item.version = p.version
  }

  async function refresh(): Promise<void> {
    const mine = epoch.acquire()
    state.loading = true
    try {
      const res = await ListClusters()
      if (!epoch.active(mine)) return // 被后续切换/刷新作废
      if (isErr(res)) {
        toastErr(res)
        return
      }
      const list = res as nomad.ClusterList
      state.clusters = list.clusters.map((info) => ({ info }))
      // activeID 以服务端为准（含启动恢复 + 删除回退，前端不做本地双源）
      state.activeID = list.activeID || null
    } catch (err) {
      if (epoch.active(mine)) console.error('[clusters] refresh failed:', err)
    } finally {
      // loading 属于 UI 信号：被更新请求作废的响应照常复位，
      // 否则 setActive 等仅作废不重拉的场景会卡死 spinner。
      state.loading = false
    }
  }

  // discover 探测本机可用连接候选（NOMAD_* 环境变量等）。纯读、无副作用。
  async function discover(): Promise<uiapi.DiscoveredCluster[]> {
    try {
      const res = await DiscoverClusters()
      if (isErr(res)) {
        toastErr(res)
        return []
      }
      state.discovered = res as uiapi.DiscoveredCluster[]
      return state.discovered
    } catch (err) {
      console.error('[clusters] discover failed:', err)
      state.discovered = []
      return []
    }
  }

  // importFromEnv 一键导入（服务端读 env，token 不经前端往返）。
  async function importFromEnv(
    name: string,
  ): Promise<{ ok: true; info: nomad.ClusterInfo } | { ok: false; error: string }> {
    try {
      const res = await ImportFromEnv(name)
      if (isErr(res)) {
        toastErr(res)
        return { ok: false, error: (res as uiapi.Error).message }
      }
      const info = res as nomad.ClusterInfo
      await refresh()
      // 同 Address 再导入时 activeID 可能不变，仍需让 jobs/nodes/loads 重连。
      state.activeEpoch++
      return { ok: true, info }
    } catch (err) {
      console.error('[clusters] importFromEnv failed:', err)
      return { ok: false, error: `${err}` }
    }
  }

  async function addCluster(
    input: ClusterInput,
  ): Promise<{ ok: true } | { ok: false; error: string }> {
    try {
      const err = await AddCluster(input)
      if (isErr(err)) {
        return { ok: false, error: (err as { message: string }).message }
      }
      // 添加成功后默认激活；激活失败不得伪装成整体成功（对话框会误关）
      const act = await setActive(input.id)
      await refresh()
      if (!act.ok) {
        return {
          ok: false,
          error: act.error || 'cluster saved, but activation failed',
        }
      }
      return { ok: true }
    } catch (err) {
      console.error('[clusters] addCluster failed:', err)
      return { ok: false, error: `${err}` }
    }
  }

  async function removeCluster(id: string): Promise<boolean> {
    try {
      const err = await RemoveCluster(id)
      if (isErr(err)) {
        toastErr(err)
        return false
      }
      // 删除后的 active 回退由服务端裁决（§5.2），refresh 统一同步
      await refresh()
      return true
    } catch (err) {
      console.error('[clusters] removeCluster failed:', err)
      return false
    }
  }

  async function updateCluster(
    input: ClusterInput,
  ): Promise<{ ok: true } | { ok: false; error: string }> {
    try {
      const err = await UpdateCluster(input)
      if (isErr(err)) {
        return { ok: false, error: (err as { message: string }).message }
      }
      await refresh()
      return { ok: true }
    } catch (err) {
      console.error('[clusters] updateCluster failed:', err)
      return { ok: false, error: `${err}` }
    }
  }

  async function setActive(id: string): Promise<{ ok: true } | { ok: false; error: string }> {
    // acquire 会作废旧上下文的所有在飞 token（含旧集群的 refresh/操作），
    // 并给自己发新 token：后来的切换自然覆盖先前的。
    const mine = epoch.acquire()
    try {
      const err = await SetActiveCluster(id)
      if (!epoch.active(mine)) return { ok: false, error: 'superseded by newer cluster switch' }
      if (isErr(err)) {
        toastErr(err)
        return { ok: false, error: (err as uiapi.Error).message }
      }
      state.activeID = id
      state.activeEpoch++
      // 上面的 acquire 已作废在飞 ListClusters；必须重拉，否则侧边栏可能空列表。
      await refresh()
      return { ok: true }
    } catch (err) {
      console.error('[clusters] setActive failed:', err)
      return { ok: false, error: `${err}` }
    }
  }

  async function pinCluster(id: string, pinned: boolean): Promise<void> {
    try {
      const err = await PinCluster(id, pinned)
      if (isErr(err)) {
        toastErr(err)
        return
      }
      await refresh()
    } catch (err) {
      console.error('[clusters] pinCluster failed:', err)
    }
  }

  // testConnection 手动探测一次（结果也写入健康缓存，事件流会推送回来）。
  async function testConnection(id: string): Promise<void> {
    try {
      const res = await TestConnection(id)
      if (isErr(res)) {
        toastErr(res)
        return
      }
      const h = res as nomad.ClusterHealth
      const item = state.clusters.find((c) => c.info.id === id)
      if (item) {
        item.info.health = h.status
        if (h.leader) item.leader = h.leader
        if (h.version) item.version = h.version
      }
    } catch (err) {
      console.error('[clusters] testConnection failed:', err)
    }
  }

  return {
    get state() {
      return state
    },
    refresh,
    discover,
    importFromEnv,
    addCluster,
    removeCluster,
    updateCluster,
    setActive,
    pinCluster,
    testConnection,
  }
}

export function isErr<T>(res: T | uiapi.Error): res is uiapi.Error {
  return !!(res && typeof res === 'object' && 'code' in res && 'message' in res)
}

// --- toast 极简实现（M3 再扩展命令面板/通知中心） ---
export type ToastLevel = 'info' | 'success' | 'error'

export interface ToastItem {
  id: number
  level: ToastLevel
  message: string
}

let nextToastID = 1
const toasts = $state<ToastItem[]>([])

export function toast(t: { level: ToastLevel; message: string }): void {
  const id = nextToastID++
  toasts.push({ id, ...t })
  setTimeout(() => {
    const idx = toasts.findIndex((x) => x.id === id)
    if (idx >= 0) toasts.splice(idx, 1)
  }, 4000)
}

export function toastErr(err: uiapi.Error): void {
  toast({ level: 'error', message: err.message })
}

export function toastState(): ToastItem[] {
  return toasts
}
