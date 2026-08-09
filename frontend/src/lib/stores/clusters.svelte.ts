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
} from '../../../bindings/github.com/nphq/np/app'
import { Events } from '@wailsio/runtime'
import type { uiapi, nomad } from '../types/wails'

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
  }
}

export function createClustersStore() {
  const state = $state<ClustersState>(empty())

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
    state.loading = true
    try {
      const res = await ListClusters()
      if (isErr(res)) {
        toastErr(res)
        return
      }
      const list = res as nomad.ClusterList
      state.clusters = list.clusters.map((info) => ({ info }))
      // activeID 以服务端为准（含启动恢复 + 删除回退，前端不做本地双源）
      state.activeID = list.activeID || null
    } catch (err) {
      console.error('[clusters] refresh failed:', err)
    } finally {
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
    try {
      const err = await SetActiveCluster(id)
      if (isErr(err)) {
        toastErr(err)
        return { ok: false, error: (err as uiapi.Error).message }
      }
      state.activeID = id
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
