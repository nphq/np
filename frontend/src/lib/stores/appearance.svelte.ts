// appearance.svelte.ts —— 显示偏好（主题 / 字体 / 字号），持久化 localStorage。
export type ThemePref = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'
export type FontFamily = 'system' | 'sans' | 'mono'
export type FontSize = 'sm' | 'md' | 'lg'

interface AppearanceSettings {
  theme: ThemePref
  fontFamily: FontFamily
  fontSize: FontSize
}

const STORAGE_KEY = 'nm.appearance'

const DEFAULTS: AppearanceSettings = {
  theme: 'dark',
  fontFamily: 'system',
  fontSize: 'md',
}

function readStored(): AppearanceSettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<AppearanceSettings>
    return {
      theme:
        parsed.theme === 'light' || parsed.theme === 'system' || parsed.theme === 'dark'
          ? parsed.theme
          : DEFAULTS.theme,
      fontFamily:
        parsed.fontFamily === 'sans' ||
        parsed.fontFamily === 'mono' ||
        parsed.fontFamily === 'system'
          ? parsed.fontFamily
          : DEFAULTS.fontFamily,
      fontSize:
        parsed.fontSize === 'sm' || parsed.fontSize === 'lg' || parsed.fontSize === 'md'
          ? parsed.fontSize
          : DEFAULTS.fontSize,
    }
  } catch {
    return { ...DEFAULTS }
  }
}

function systemIsLight(): boolean {
  try {
    return window.matchMedia('(prefers-color-scheme: light)').matches
  } catch {
    return false
  }
}

function resolveTheme(pref: ThemePref): ResolvedTheme {
  if (pref === 'system') return systemIsLight() ? 'light' : 'dark'
  return pref
}

function applyDOM(settings: AppearanceSettings): ResolvedTheme {
  const resolved = resolveTheme(settings.theme)
  const root = document.documentElement
  root.dataset.theme = resolved
  root.dataset.fontFamily = settings.fontFamily
  root.dataset.fontSize = settings.fontSize
  root.style.colorScheme = resolved
  return resolved
}

function createAppearanceStore() {
  let settings = $state<AppearanceSettings>(readStored())
  let resolved = $state<ResolvedTheme>(resolveTheme(settings.theme))

  function persist(): void {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(settings))
    } catch {
      // 静默
    }
  }

  function apply(): void {
    resolved = applyDOM(settings)
    persist()
  }

  function setTheme(theme: ThemePref): void {
    settings = { ...settings, theme }
    apply()
  }

  function setFontFamily(fontFamily: FontFamily): void {
    settings = { ...settings, fontFamily }
    apply()
  }

  function setFontSize(fontSize: FontSize): void {
    settings = { ...settings, fontSize }
    apply()
  }

  /** 启动时调用一次：应用 DOM + 监听系统主题。 */
  function init(): void {
    apply()
    try {
      const mq = window.matchMedia('(prefers-color-scheme: light)')
      mq.addEventListener('change', () => {
        if (settings.theme === 'system') resolved = applyDOM(settings)
      })
    } catch {
      // ignore
    }
  }

  return {
    get settings() {
      return settings
    },
    get resolved() {
      return resolved
    },
    init,
    setTheme,
    setFontFamily,
    setFontSize,
  }
}

export const appearance = createAppearanceStore()
