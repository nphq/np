<script lang="ts">
  // AppsScreen —— 精选应用目录（对标 Lens Charts 浏览，货源为本地固化模板）。
  import { t } from '../../i18n/index.svelte'
  import { toast } from '../../stores/clusters.svelte'
  import type { RunJobInput, RunJobOutcome } from '../../stores/jobs.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import FormBanner from '../FormBanner.svelte'
  import {
    APP_CATALOG,
    buildDockerJobJSON,
    extractJobIDFromSpec,
    findCatalogApp,
    summarizeDockerForm,
    type AppCategory,
    type CatalogApp,
  } from '../../jobs/spec'
  import { clusterHasDriver } from '../../utils/drivers'
  import type { nomad } from '../../types/wails'

  let {
    busy = false,
    selectedId = null,
    existingJobIDs = [],
    nodes = [],
    onSelect,
    onCustomize,
    onRun,
    onDone,
  }: {
    busy?: boolean
    selectedId?: string | null
    existingJobIDs?: string[]
    nodes?: nomad.NodeSummary[]
    onSelect: (appID: string | null) => void
    onCustomize: (appID: string) => void
    onRun: (input: RunJobInput) => Promise<RunJobOutcome>
    onDone: (jobID: string) => void
  } = $props()

  const categories: {
    id: AppCategory | 'all'
    labelKey:
      | 'apps.cat.all'
      | 'apps.cat.web'
      | 'apps.cat.data'
      | 'apps.cat.observability'
      | 'apps.cat.messaging'
      | 'apps.cat.utility'
      | 'apps.cat.native'
  }[] = [
    { id: 'all', labelKey: 'apps.cat.all' },
    { id: 'web', labelKey: 'apps.cat.web' },
    { id: 'data', labelKey: 'apps.cat.data' },
    { id: 'observability', labelKey: 'apps.cat.observability' },
    { id: 'messaging', labelKey: 'apps.cat.messaging' },
    { id: 'utility', labelKey: 'apps.cat.utility' },
    { id: 'native', labelKey: 'apps.cat.native' },
  ]

  let query = $state('')
  let category = $state<AppCategory | 'all'>('all')
  let confirmOpen = $state(false)
  let errorText = $state('')
  let warnings = $state('')

  const selected = $derived(selectedId ? (findCatalogApp(selectedId) ?? null) : null)
  const existingSet = $derived(new Set(existingJobIDs.map((id) => id.toLowerCase())))

  const filtered = $derived(
    APP_CATALOG.filter((app) => {
      if (category !== 'all' && app.category !== category) return false
      const q = query.trim().toLowerCase()
      if (!q) return true
      const title = t(app.titleKey).toLowerCase()
      const desc = t(app.descKey).toLowerCase()
      return (
        app.id.includes(q) ||
        title.includes(q) ||
        desc.includes(q) ||
        app.imageLabel.toLowerCase().includes(q)
      )
    }),
  )

  function categoryName(cat: AppCategory): string {
    switch (cat) {
      case 'web':
        return t('apps.cat.web')
      case 'data':
        return t('apps.cat.data')
      case 'observability':
        return t('apps.cat.observability')
      case 'messaging':
        return t('apps.cat.messaging')
      case 'utility':
        return t('apps.cat.utility')
      case 'native':
        return t('apps.cat.native')
    }
  }

  function appJobID(app: CatalogApp): string {
    if (app.kind === 'form' && app.form) return app.form.jobID.trim()
    if (app.kind === 'hcl' && app.hcl) return extractJobIDFromSpec(app.hcl, 'hcl')
    return ''
  }

  function canDeploy(app: CatalogApp): boolean {
    return (app.kind === 'form' && !!app.form) || (app.kind === 'hcl' && !!app.hcl)
  }

  function requestDeploy(): void {
    if (!selected || !canDeploy(selected)) return
    errorText = ''
    warnings = ''
    confirmOpen = true
  }

  function willOverwrite(app: CatalogApp): boolean {
    const id = appJobID(app)
    return !!id && existingSet.has(id.toLowerCase())
  }

  async function doDeploy(): Promise<void> {
    if (!selected) return
    let input: RunJobInput | null = null
    if (selected.kind === 'form' && selected.form) {
      input = {
        spec: buildDockerJobJSON(selected.form),
        format: 'json',
        namespace: '',
        canonicalize: false,
      }
    } else if (selected.kind === 'hcl' && selected.hcl) {
      input = {
        spec: selected.hcl,
        format: 'hcl',
        namespace: '',
        canonicalize: false,
      }
    }
    if (!input) return
    const outcome = await onRun(input)
    confirmOpen = false
    if (outcome.err) {
      errorText = outcome.err.message
      return
    }
    if (!outcome.ok || !outcome.result) return
    if (outcome.result.warnings) {
      warnings = outcome.result.warnings
      toast({ level: 'info', message: t('toast.registeredWithWarnings') })
    }
    onDone(outcome.result.jobID)
  }

  function appNeedsDocker(app: CatalogApp): boolean {
    if (app.kind === 'form') return true
    if (app.driver === 'exec' || app.driver === 'raw_exec') return false
    return true
  }

  function confirmBody(app: CatalogApp): string {
    const jobID = appJobID(app)
    let body: string
    if (app.kind === 'form' && app.form) {
      const s = summarizeDockerForm(app.form)
      body = t('apps.confirmBody', {
        name: t(app.titleKey),
        jobID: s.jobID,
        image: s.image,
        count: String(s.count),
      })
    } else {
      body = t('apps.confirmBodyHcl', {
        name: t(app.titleKey),
        jobID: jobID || '—',
        driver: app.driver ?? app.imageLabel,
      })
    }
    if (willOverwrite(app)) {
      body += '\n\n' + t('apps.confirmOverwrite', { jobID })
    }
    if (appNeedsDocker(app) && nodes.length > 0 && !clusterHasDriver(nodes, 'docker')) {
      body += '\n\n' + t('deploy.driverWarnDocker')
    }
    return body
  }
</script>

<div class="flex h-full min-h-0 w-full">
  <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
    <div class="shrink-0 border-b border-zinc-800 px-6 py-5">
      <h1 class="text-lg font-semibold">{t('apps.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('apps.subtitle')}</p>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <input
          class="w-56 rounded border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-sky-500"
          placeholder={t('apps.search')}
          bind:value={query}
        />
        <div class="flex flex-wrap gap-1">
          {#each categories as c (c.id)}
            <button
              class="rounded px-2.5 py-1 text-[11px] {category === c.id
                ? 'bg-zinc-100 font-medium text-zinc-900'
                : 'border border-zinc-700 text-zinc-400 hover:bg-zinc-800'}"
              onclick={() => (category = c.id)}
            >
              {t(c.labelKey)}
            </button>
          {/each}
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto p-6">
      {#if filtered.length === 0}
        <p class="text-xs text-zinc-600">{t('apps.empty')}</p>
      {:else}
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {#each filtered as app (app.id)}
            <button
              class="rounded border p-4 text-left transition-colors {selectedId === app.id
                ? 'border-sky-600 bg-zinc-900'
                : 'border-zinc-800 bg-zinc-950/80 hover:border-zinc-600 hover:bg-zinc-900'}"
              onclick={() => onSelect(app.id)}
            >
              <div class="flex items-start justify-between gap-2">
                <div class="text-sm font-medium text-zinc-100">{t(app.titleKey)}</div>
                <span
                  class="shrink-0 rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-400 uppercase"
                >
                  {categoryName(app.category)}
                </span>
              </div>
              <p class="mt-2 line-clamp-2 text-xs leading-5 text-zinc-500">{t(app.descKey)}</p>
              <div class="mt-3 font-mono text-[11px] text-zinc-600">{app.imageLabel}</div>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <aside class="flex w-80 shrink-0 flex-col border-l border-zinc-800 bg-zinc-900/40">
    {#if selected}
      <div class="border-b border-zinc-800 px-4 py-4">
        <div class="text-sm font-semibold text-zinc-100">{t(selected.titleKey)}</div>
        <div class="mt-1 text-[11px] text-zinc-500 uppercase">
          {categoryName(selected.category)}
        </div>
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <p class="text-xs leading-5 text-zinc-400">{t(selected.descKey)}</p>
        <dl class="mt-4 space-y-3 text-xs">
          <div>
            <dt class="text-zinc-600">
              {selected.category === 'native' ||
              selected.driver === 'exec' ||
              selected.driver === 'raw_exec'
                ? t('apps.detail.runtime')
                : t('apps.detail.image')}
            </dt>
            <dd class="mt-0.5 font-mono text-zinc-300">{selected.imageLabel}</dd>
          </div>
          {#if selected.form}
            <div>
              <dt class="text-zinc-600">{t('apps.detail.jobID')}</dt>
              <dd class="mt-0.5 text-zinc-300">{selected.form.jobID}</dd>
            </div>
            <div>
              <dt class="text-zinc-600">{t('apps.detail.type')}</dt>
              <dd class="mt-0.5 text-zinc-300">{selected.form.type}</dd>
            </div>
            <div>
              <dt class="text-zinc-600">{t('apps.detail.resources')}</dt>
              <dd class="mt-0.5 text-zinc-300">
                {selected.form.cpu} MHz · {selected.form.memory} MB · ×{selected.form.count}
              </dd>
            </div>
          {:else if selected.hcl}
            <div>
              <dt class="text-zinc-600">{t('apps.detail.jobID')}</dt>
              <dd class="mt-0.5 text-zinc-300">{appJobID(selected) || '—'}</dd>
            </div>
          {/if}
        </dl>
        {#if errorText}
          <div class="mt-4">
            <FormBanner kind="error" title={t('runJob.validationFailed')} message={errorText} />
          </div>
        {/if}
        {#if warnings}
          <div class="mt-4">
            <FormBanner kind="warning" title={t('runJob.warnings')} message={warnings} />
          </div>
        {/if}
        {#if selected && appNeedsDocker(selected) && nodes.length > 0 && !clusterHasDriver(nodes, 'docker')}
          <div class="mt-4">
            <FormBanner
              kind="warning"
              title={t('runJob.warnings')}
              message={t('deploy.driverWarnDocker')}
            />
          </div>
        {/if}
        <p class="mt-4 text-[11px] text-zinc-600">
          {selected.category === 'native' ? t('apps.detail.hintNative') : t('apps.detail.hint')}
        </p>
      </div>
      <div class="flex shrink-0 flex-col gap-2 border-t border-zinc-800 p-4">
        {#if canDeploy(selected)}
          <button
            class="rounded bg-zinc-100 px-3 py-2 text-xs font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
            disabled={busy}
            onclick={requestDeploy}
          >
            {busy ? t('apps.deploying') : t('apps.deploy')}
          </button>
        {/if}
        <button
          class="rounded border border-zinc-700 px-3 py-2 text-xs text-zinc-300 hover:bg-zinc-800"
          onclick={() => onCustomize(selected.id)}
        >
          {t('apps.customize')}
        </button>
      </div>
    {:else}
      <div class="flex flex-1 items-center justify-center p-6 text-center text-xs text-zinc-600">
        {t('apps.selectHint')}
      </div>
    {/if}
  </aside>
</div>

{#if confirmOpen && selected}
  <ConfirmDialog
    title={t('apps.confirmTitle')}
    message={confirmBody(selected)}
    confirmLabel={t('apps.deploy')}
    danger={willOverwrite(selected)}
    {busy}
    onConfirm={() => void doDeploy()}
    onCancel={() => {
      if (busy) return
      confirmOpen = false
    }}
  />
{/if}
