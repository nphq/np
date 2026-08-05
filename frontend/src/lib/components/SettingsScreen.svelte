<script lang="ts">
  // SettingsScreen —— 对标 Lens Preferences：应用 / 显示 / 关于。
  import { Browser } from '@wailsio/runtime'
  import { i18n, t } from '../i18n/index.svelte'
  import type { Locale } from '../i18n/index.svelte'
  import { appearance } from '../stores/appearance.svelte'
  import type { FontFamily, FontSize, ThemePref } from '../stores/appearance.svelte'

  const SITE_URL = 'https://github.com/nphq/np'
  const FEEDBACK_URL = 'https://github.com/nphq/np/issues/new'

  type Section = 'application' | 'display' | 'about'

  let section = $state<Section>('application')

  const sections: {
    id: Section
    labelKey: 'settings.nav.application' | 'settings.nav.display' | 'settings.nav.about'
  }[] = [
    { id: 'application', labelKey: 'settings.nav.application' },
    { id: 'display', labelKey: 'settings.nav.display' },
    { id: 'about', labelKey: 'settings.nav.about' },
  ]

  const locales: { id: Locale; label: string }[] = [
    { id: 'zh', label: '中文' },
    { id: 'en', label: 'English' },
  ]

  const themes: {
    id: ThemePref
    labelKey:
      'settings.display.themeDark' | 'settings.display.themeLight' | 'settings.display.themeSystem'
  }[] = [
    { id: 'dark', labelKey: 'settings.display.themeDark' },
    { id: 'light', labelKey: 'settings.display.themeLight' },
    { id: 'system', labelKey: 'settings.display.themeSystem' },
  ]

  const fonts: {
    id: FontFamily
    labelKey:
      'settings.display.fontSystem' | 'settings.display.fontSans' | 'settings.display.fontMono'
  }[] = [
    { id: 'system', labelKey: 'settings.display.fontSystem' },
    { id: 'sans', labelKey: 'settings.display.fontSans' },
    { id: 'mono', labelKey: 'settings.display.fontMono' },
  ]

  const sizes: {
    id: FontSize
    labelKey: 'settings.display.sizeSm' | 'settings.display.sizeMd' | 'settings.display.sizeLg'
  }[] = [
    { id: 'sm', labelKey: 'settings.display.sizeSm' },
    { id: 'md', labelKey: 'settings.display.sizeMd' },
    { id: 'lg', labelKey: 'settings.display.sizeLg' },
  ]

  function navClass(id: Section): string {
    return section === id
      ? 'rounded bg-zinc-800 px-2.5 py-1.5 text-left text-sm font-medium text-zinc-100'
      : 'rounded px-2.5 py-1.5 text-left text-sm text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200'
  }

  function segmentClass(active: boolean): string {
    return active ? 'bg-zinc-100 font-medium text-zinc-900' : 'text-zinc-400 hover:bg-zinc-800'
  }

  function openExternal(url: string): void {
    void Browser.OpenURL(url).catch(() => {
      window.open(url, '_blank', 'noopener,noreferrer')
    })
  }
</script>

<div class="flex h-full min-h-0 w-full">
  <aside class="flex w-48 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/40 p-3">
    <div class="mb-3 px-1 text-[11px] font-semibold tracking-wide text-zinc-500 uppercase">
      {t('settings.navTitle')}
    </div>
    <nav class="flex flex-col gap-0.5">
      {#each sections as s (s.id)}
        <button class={navClass(s.id)} onclick={() => (section = s.id)}>
          {t(s.labelKey)}
        </button>
      {/each}
    </nav>
  </aside>

  <div class="min-h-0 flex-1 overflow-y-auto p-6">
    {#if section === 'application'}
      <h1 class="text-lg font-semibold">{t('settings.application.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('settings.application.subtitle')}</p>

      <section class="mt-8 max-w-lg">
        <h2 class="text-xs font-semibold tracking-wide text-zinc-400 uppercase">
          {t('settings.application.language')}
        </h2>
        <p class="mt-1 text-xs text-zinc-600">{t('settings.application.languageHint')}</p>
        <div class="mt-3 flex overflow-hidden rounded border border-zinc-700">
          {#each locales as loc (loc.id)}
            <button
              class="flex-1 px-3 py-2 text-xs {segmentClass(i18n.locale === loc.id)}"
              onclick={() => i18n.setLocale(loc.id)}
            >
              {loc.label}
            </button>
          {/each}
        </div>
      </section>
    {:else if section === 'display'}
      <h1 class="text-lg font-semibold">{t('settings.display.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('settings.display.subtitle')}</p>

      <section class="mt-8 max-w-lg">
        <h2 class="text-xs font-semibold tracking-wide text-zinc-400 uppercase">
          {t('settings.display.theme')}
        </h2>
        <p class="mt-1 text-xs text-zinc-600">{t('settings.display.themeHint')}</p>
        <div class="mt-3 flex overflow-hidden rounded border border-zinc-700">
          {#each themes as th (th.id)}
            <button
              class="flex-1 px-3 py-2 text-xs {segmentClass(appearance.settings.theme === th.id)}"
              onclick={() => appearance.setTheme(th.id)}
            >
              {t(th.labelKey)}
            </button>
          {/each}
        </div>
        {#if appearance.settings.theme === 'system'}
          <p class="mt-2 text-[11px] text-zinc-600">
            {t('settings.display.themeResolved', {
              theme:
                appearance.resolved === 'light'
                  ? t('settings.display.themeLight')
                  : t('settings.display.themeDark'),
            })}
          </p>
        {/if}
      </section>

      <section class="mt-8 max-w-lg">
        <h2 class="text-xs font-semibold tracking-wide text-zinc-400 uppercase">
          {t('settings.display.font')}
        </h2>
        <p class="mt-1 text-xs text-zinc-600">{t('settings.display.fontHint')}</p>
        <div class="mt-3 flex overflow-hidden rounded border border-zinc-700">
          {#each fonts as f (f.id)}
            <button
              class="flex-1 px-3 py-2 text-xs {segmentClass(
                appearance.settings.fontFamily === f.id,
              )}"
              onclick={() => appearance.setFontFamily(f.id)}
            >
              {t(f.labelKey)}
            </button>
          {/each}
        </div>
        <p
          class="mt-3 rounded border border-zinc-800 bg-zinc-900/50 px-3 py-2 text-sm text-zinc-300"
        >
          {t('settings.display.fontPreview')}
        </p>
      </section>

      <section class="mt-8 max-w-lg">
        <h2 class="text-xs font-semibold tracking-wide text-zinc-400 uppercase">
          {t('settings.display.size')}
        </h2>
        <p class="mt-1 text-xs text-zinc-600">{t('settings.display.sizeHint')}</p>
        <div class="mt-3 flex overflow-hidden rounded border border-zinc-700">
          {#each sizes as s (s.id)}
            <button
              class="flex-1 px-3 py-2 text-xs {segmentClass(appearance.settings.fontSize === s.id)}"
              onclick={() => appearance.setFontSize(s.id)}
            >
              {t(s.labelKey)}
            </button>
          {/each}
        </div>
      </section>
    {:else}
      <h1 class="text-lg font-semibold">{t('settings.about.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('settings.about.subtitle')}</p>

      <dl class="mt-8 max-w-lg space-y-4 text-sm">
        <div class="flex justify-between gap-4 border-b border-zinc-800 pb-3">
          <dt class="text-zinc-500">{t('settings.about.app')}</dt>
          <dd class="font-medium text-zinc-200">{t('app.title')}</dd>
        </div>
        <div class="flex justify-between gap-4 border-b border-zinc-800 pb-3">
          <dt class="text-zinc-500">{t('settings.about.version')}</dt>
          <dd class="font-mono text-zinc-300">0.1.0</dd>
        </div>
      </dl>

      <div class="mt-8 max-w-lg overflow-hidden rounded border border-zinc-800">
        <div class="flex items-center gap-3 border-b border-zinc-800 px-3 py-3">
          <svg
            class="h-4 w-4 shrink-0 text-zinc-400"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.75"
            aria-hidden="true"
          >
            <circle cx="12" cy="12" r="9" />
            <path d="M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18" />
          </svg>
          <span class="min-w-0 flex-1 text-sm text-zinc-200">{t('settings.about.website')}</span>
          <button
            type="button"
            class="shrink-0 rounded border border-zinc-600 px-2.5 py-1 text-xs text-zinc-300 hover:border-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
            onclick={() => openExternal(SITE_URL)}
          >
            {t('settings.about.websiteAction')}
          </button>
        </div>
        <div class="flex items-center gap-3 px-3 py-3">
          <svg
            class="h-4 w-4 shrink-0 text-zinc-400"
            viewBox="0 0 24 24"
            fill="currentColor"
            aria-hidden="true"
          >
            <path
              d="M12 2C6.48 2 2 6.58 2 12.26c0 4.52 2.87 8.35 6.84 9.71.5.1.68-.22.68-.49 0-.24-.01-.87-.01-1.71-2.78.62-3.37-1.37-3.37-1.37-.45-1.18-1.11-1.5-1.11-1.5-.91-.64.07-.63.07-.63 1 .07 1.53 1.06 1.53 1.06.89 1.56 2.34 1.11 2.91.85.09-.66.35-1.11.63-1.37-2.22-.26-4.55-1.14-4.55-5.07 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.31.1-2.73 0 0 .84-.27 2.75 1.05A9.3 9.3 0 0 1 12 6.84c.85 0 1.71.12 2.51.34 1.91-1.32 2.75-1.05 2.75-1.05.55 1.42.2 2.47.1 2.73.64.72 1.03 1.63 1.03 2.75 0 3.94-2.34 4.81-4.57 5.06.36.32.68.94.68 1.9 0 1.37-.01 2.48-.01 2.81 0 .27.18.6.69.49A10.03 10.03 0 0 0 22 12.26C22 6.58 17.52 2 12 2z"
            />
          </svg>
          <span class="min-w-0 flex-1 text-sm text-zinc-200">{t('settings.about.feedback')}</span>
          <button
            type="button"
            class="shrink-0 rounded border border-zinc-600 px-2.5 py-1 text-xs text-zinc-300 hover:border-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
            onclick={() => openExternal(FEEDBACK_URL)}
          >
            {t('settings.about.feedbackAction')}
          </button>
        </div>
      </div>
    {/if}
  </div>
</div>
