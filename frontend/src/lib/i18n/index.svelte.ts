import zh, { type MessageKey } from './dictionaries/zh'
import en from './dictionaries/en'

// i18n 极简实现＼�ADR-14）：零依赖类型安全字典。
// locale 是 $state，组件读它即自动响应式；切换即时生效。
// 持久化 localStorage["nm.locale"]；同时联动 <html lang>。

export type Locale = 'zh' | 'en'

const dicts: Record<Locale, Record<MessageKey, string>> = { zh, en }

export const LOCALE_KEY = 'nm.locale'

const SUPPORTED: Locale[] = ['zh', 'en']

function loadInitial(): Locale {
  try {
    const v = localStorage.getItem(LOCALE_KEY) as Locale | null
    if (v && SUPPORTED.includes(v)) return v
  } catch {
    // localStorage 不可用（隐私模式等）时回落默认
  }
  return 'zh' // 默认中文＼�
}

export function createI18n() {
  let locale = $state<Locale>(loadInitial())

  function persist(): void {
    try {
      localStorage.setItem(LOCALE_KEY, locale)
      document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en'
    } catch {
      // 静默失败：内存态仍生效
    }
  }

  function t(key: MessageKey, params?: Record<string, string | number>): string {
    const raw = dicts[locale][key] ?? key
    if (!params) return raw
    return raw.replace(/\{(\w+)\}/g, (m, name: string) =>
      params[name] !== undefined ? String(params[name]) : m,
    )
  }

  // status('running') → 本地化状态词；字典缺失时回落原值。
  function status(raw: string): string {
    const key = `status.${raw}` as MessageKey
    return dicts[locale][key] ?? raw
  }

  function setLocale(next: Locale): void {
    locale = next
    persist()
  }

  function toggle(): void {
    setLocale(locale === 'zh' ? 'en' : 'zh')
  }

  return {
    get locale() {
      return locale
    },
    t,
    status,
    setLocale,
    toggle,
  }
}

// 全局单例：store/组件共享同一 locale 状态。
export const i18n = createI18n()
export const t = i18n.t
export const statusLabel = i18n.status
