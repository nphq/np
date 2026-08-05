import {
  ListClusters,
  AddCluster,
  RemoveCluster,
  SetActiveCluster,
  UpdateCluster,
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
      state.clusters = (res as nomad.ClusterInfo[]).map((info) => ({ info }))
      if (state.activeID && !state.clusters.some((c) => c.info.id === state.activeID)) {
        state.activeID = null
      }
    } catch (err) {
      console.error('[clusters] refresh failed:', err)
    } finally {
      state.loading = false
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
      await refresh()
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
      if (state.activeID === id) state.activeID = null
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

  async function setActive(id: string): Promise<void> {
    try {
      const err = await SetActiveCluster(id)
      if (isErr(err)) {
        toastErr(err)
        return
      }
      state.activeID = id
    } catch (err) {
      console.error('[clusters] setActive failed:', err)
    }
  }

  return {
    get state() {
      return state
    },
    refresh,
    addCluster,
    removeCluster,
    updateCluster,
    setActive,
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
