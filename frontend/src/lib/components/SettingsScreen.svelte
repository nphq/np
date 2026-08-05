<script lang="ts">
  // SettingsScreen —— 对标 Lens Preferences：应用级设置，与集群视图分离。
  import { i18n, t } from '../i18n/index.svelte'
  import type { Locale } from '../i18n/index.svelte'

  type Section = 'application' | 'about'

  let section = $state<Section>('application')

  const sections: { id: Section; labelKey: 'settings.nav.application' | 'settings.nav.about' }[] = [
    { id: 'application', labelKey: 'settings.nav.application' },
    { id: 'about', labelKey: 'settings.nav.about' },
  ]

  const locales: { id: Locale; label: string }[] = [
    { id: 'zh', label: '中文' },
    { id: 'en', label: 'English' },
  ]

  function navClass(id: Section): string {
    return section === id
      ? 'rounded bg-zinc-800 px-2.5 py-1.5 text-left text-sm font-medium text-zinc-100'
      : 'rounded px-2.5 py-1.5 text-left text-sm text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200'
  }
</script>

<div class="flex h-full min-h-0 w-full">
  <!-- Preferences 左侧分类（Lens 风格） -->
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

  <div class="min-w-0 flex-1 overflow-y-auto p-6">
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
              class="flex-1 px-3 py-2 text-xs {i18n.locale === loc.id
                ? 'bg-zinc-100 font-medium text-zinc-900'
                : 'text-zinc-400 hover:bg-zinc-800'}"
              onclick={() => i18n.setLocale(loc.id)}
            >
              {loc.label}
            </button>
          {/each}
        </div>
      </section>

      <section class="mt-8 max-w-lg opacity-60">
        <h2 class="text-xs font-semibold tracking-wide text-zinc-400 uppercase">
          {t('settings.application.theme')}
        </h2>
        <p class="mt-1 text-xs text-zinc-600">{t('settings.application.themeHint')}</p>
        <div
          class="mt-3 rounded border border-dashed border-zinc-700 px-3 py-2 text-xs text-zinc-500"
        >
          {t('settings.application.themeDark')}
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
        <div class="flex justify-between gap-4 border-b border-zinc-800 pb-3">
          <dt class="text-zinc-500">{t('settings.about.stack')}</dt>
          <dd class="text-right text-zinc-300">{t('settings.about.stackValue')}</dd>
        </div>
      </dl>
    {/if}
  </div>
</div>
