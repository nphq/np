import {
  GetSettings,
  UpdateSettings,
  ResetSettings,
  GetConfigPaths,
} from '../../../bindings/github.com/nphq/np/internal/app/app'
import { isErr, toastErr, toast } from './clusters.svelte'
import { t } from '../i18n/index.svelte'

// settings.svelte.ts —— 通用行为设置（后端 preferences.json 持久化）。
// 外观/语言仍走 localStorage（即时生效）；此处只管需后端生效的行为项：
// 二次确认、启动恢复、轮询间隔、集群默认值。

export interface AppSettings {
  confirmDestructive: boolean
  autoRestoreActive: boolean
  healthIntervalSec: number
  metricsIntervalSec: number
  defaultRegion: string
  defaultNamespace: string
}

export interface ConfigPaths {
  configDir: string
  clusters: string
  preferences: string
}

export const DEFAULT_SETTINGS: AppSettings = {
  confirmDestructive: true,
  autoRestoreActive: true,
  healthIntervalSec: 30,
  metricsIntervalSec: 15,
  defaultRegion: '',
  defaultNamespace: '',
}

export const HEALTH_OPTIONS = [10, 15, 30, 60, 120]
export const METRICS_OPTIONS = [5, 10, 15, 30, 60]

function normalize(raw: Partial<AppSettings>): AppSettings {
  return {
    confirmDestructive: raw.confirmDestructive ?? DEFAULT_SETTINGS.confirmDestructive,
    autoRestoreActive: raw.autoRestoreActive ?? DEFAULT_SETTINGS.autoRestoreActive,
    healthIntervalSec: HEALTH_OPTIONS.includes(raw.healthIntervalSec ?? 0)
      ? (raw.healthIntervalSec as number)
      : DEFAULT_SETTINGS.healthIntervalSec,
    metricsIntervalSec: METRICS_OPTIONS.includes(raw.metricsIntervalSec ?? 0)
      ? (raw.metricsIntervalSec as number)
      : DEFAULT_SETTINGS.metricsIntervalSec,
    defaultRegion: raw.defaultRegion ?? '',
    defaultNamespace: raw.defaultNamespace ?? '',
  }
}

export function createSettingsStore() {
  const state = $state<{ settings: AppSettings; loading: boolean; saving: boolean }>({
    settings: { ...DEFAULT_SETTINGS },
    loading: false,
    saving: false,
  })
  const paths = $state<{ value: ConfigPaths | null }>({ value: null })
  let saveTimer: ReturnType<typeof setTimeout> | undefined

  async function refresh(): Promise<void> {
    state.loading = true
    try {
      const res = await GetSettings()
      if (isErr(res)) {
        toastErr(res)
        return
      }
      state.settings = normalize(res as AppSettings)
    } catch (e) {
      console.error('[settings] refresh failed:', e)
    } finally {
      state.loading = false
    }
  }

  async function persist(now = false): Promise<void> {
    if (saveTimer) clearTimeout(saveTimer)
    const doSave = async () => {
      state.saving = true
      try {
        const res = await UpdateSettings({ ...state.settings })
        if (isErr(res)) toastErr(res)
      } catch (e) {
        console.error('[settings] save failed:', e)
      } finally {
        state.saving = false
      }
    }
    if (now) await doSave()
    else saveTimer = setTimeout(() => void doSave(), 400)
  }

  function set<K extends keyof AppSettings>(key: K, value: AppSettings[K]): void {
    state.settings = { ...state.settings, [key]: value }
    void persist()
  }

  async function reset(): Promise<void> {
    try {
      const res = await ResetSettings()
      if (isErr(res)) {
        toastErr(res)
        return
      }
      state.settings = normalize(res as AppSettings)
      toast({ level: 'success', message: t('settings.resetDone') })
    } catch (e) {
      console.error('[settings] reset failed:', e)
    }
  }

  async function loadPaths(): Promise<void> {
    try {
      const res = await GetConfigPaths()
      if (isErr(res)) return
      paths.value = res as ConfigPaths
    } catch {
      // 忽略：诊断区缺失不阻断设置页
    }
  }

  async function copyText(text: string): Promise<boolean> {
    try {
      await navigator.clipboard.writeText(text)
      toast({ level: 'success', message: t('settings.copied') })
      return true
    } catch {
      // clipboard 在非安全上下文不可用时回退到 prompt 选中
      try {
        window.prompt(t('settings.copyManually'), text)
        return true
      } catch {
        return false
      }
    }
  }

  return {
    get state() {
      return state
    },
    get paths() {
      return paths
    },
    refresh,
    loadPaths,
    set,
    persistNow: () => persist(true),
    reset,
    copyText,
  }
}

export type SettingsStore = ReturnType<typeof createSettingsStore>

// 全局单例：设置页与调用方（删除确认、表单预填）共享同一状态。
export const settings = createSettingsStore()
