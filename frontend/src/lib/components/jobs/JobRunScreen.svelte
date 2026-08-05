<script lang="ts">
  // JobRunScreen：表单 / 高级编辑；提交前本地校验 + 覆盖既有任务提示。
  import { toast } from '../../stores/clusters.svelte'
  import { t } from '../../i18n/index.svelte'
  import type { RunJobInput, RunJobOutcome } from '../../stores/jobs.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import FormBanner from '../FormBanner.svelte'
  import JobDockerForm from './JobDockerForm.svelte'
  import JobSpecEditor from './JobSpecEditor.svelte'
  import {
    STARTER_HCL,
    buildDockerJobJSON,
    defaultDockerForm,
    extractJobIDFromSpec,
    findCatalogApp,
    starterHCL,
    summarizeDockerForm,
    tryFormatJSON,
    type DockerJobForm,
    type SpecStarterID,
  } from '../../jobs/spec'
  import {
    issuesList,
    validateDockerForm,
    validateNamespace,
    type DockerFormIssues,
  } from '../../utils/validate'
  import type { MessageKey } from '../../i18n/dictionaries/zh'

  let {
    busy,
    presetAppId = null,
    existingJobIDs = [],
    onRun,
    onDone,
    onBrowseApps,
  }: {
    busy: boolean
    presetAppId?: string | null
    existingJobIDs?: string[]
    onRun: (input: RunJobInput) => Promise<RunJobOutcome>
    onDone: (jobID: string) => void
    onBrowseApps?: () => void
  } = $props()

  type Tab = 'form' | 'advanced'

  let tab = $state<Tab>('form')
  let form = $state<DockerJobForm>(defaultDockerForm())
  let spec = $state(STARTER_HCL)
  let format = $state<'hcl' | 'json'>('hcl')
  let namespace = $state('')
  let canonicalize = $state(false)
  let errorText = $state('')
  let warnings = $state('')
  let formIssues = $state<DockerFormIssues>({})
  let confirmOpen = $state(false)
  let formatConfirm = $state<'hcl' | null>(null)
  let starterConfirm = $state<SpecStarterID | null>(null)
  let pending = $state<RunJobInput | null>(null)
  let appliedPreset = $state<string | null>(null)
  let activeStarter = $state<SpecStarterID>('docker')

  const starters: SpecStarterID[] = ['docker', 'exec', 'raw_exec']
  const formSummary = $derived(summarizeDockerForm(form))
  const existingSet = $derived(new Set(existingJobIDs.map((id) => id.toLowerCase())))

  $effect(() => {
    const id = presetAppId
    if (!id || id === appliedPreset) return
    const app = findCatalogApp(id)
    if (!app) return
    appliedPreset = id
    errorText = ''
    warnings = ''
    formIssues = {}
    if (app.kind === 'form' && app.form) {
      form = { ...app.form }
      tab = 'form'
      return
    }
    if (app.kind === 'hcl' && app.hcl) {
      spec = app.hcl
      format = 'hcl'
      tab = 'advanced'
      if (app.driver === 'exec') activeStarter = 'exec'
      else if (app.driver === 'raw_exec') activeStarter = 'raw_exec'
      else activeStarter = 'docker'
    }
  })

  function switchTab(next: Tab): void {
    if (next === tab) return
    if (tab === 'form' && next === 'advanced') {
      spec = buildDockerJobJSON(form)
      format = 'json'
      activeStarter = 'docker'
    }
    errorText = ''
    warnings = ''
    formIssues = {}
    tab = next
  }

  function requestFormatSwitch(next: 'hcl' | 'json'): void {
    if (next === format) return
    if (next === 'hcl' && format === 'json') {
      const isStarterJSON = spec.trim() === buildDockerJobJSON(defaultDockerForm()).trim()
      if (!isStarterJSON && spec.trim() !== '') {
        formatConfirm = 'hcl'
        return
      }
    }
    applyFormat(next)
  }

  function applyFormat(next: 'hcl' | 'json'): void {
    if (next === 'json' && format === 'hcl' && spec.trim() === starterHCL(activeStarter).trim()) {
      spec = buildDockerJobJSON(defaultDockerForm())
      activeStarter = 'docker'
    } else if (next === 'hcl' && format === 'json') {
      spec = starterHCL(activeStarter === 'docker' ? 'docker' : activeStarter)
    }
    errorText = ''
    warnings = ''
    format = next
    formatConfirm = null
  }

  function requestStarter(id: SpecStarterID): void {
    if (format !== 'hcl') return
    if (id === activeStarter && spec.trim() === starterHCL(id).trim()) return
    if (spec.trim() !== '' && spec.trim() !== starterHCL(activeStarter).trim()) {
      starterConfirm = id
      return
    }
    applyStarter(id)
  }

  function applyStarter(id: SpecStarterID): void {
    spec = starterHCL(id)
    activeStarter = id
    format = 'hcl'
    errorText = ''
    warnings = ''
    starterConfirm = null
  }

  function formatSpec(): void {
    if (format !== 'json') return
    const res = tryFormatJSON(spec)
    if (!res.ok) {
      errorText = res.error
      return
    }
    errorText = ''
    spec = res.text
  }

  function requestRun(): void {
    errorText = ''
    warnings = ''
    formIssues = {}

    if (!validateNamespace(namespace.trim())) {
      errorText = t('runJob.err.namespace')
      return
    }

    if (tab === 'form') {
      const issues = validateDockerForm(form, namespace)
      formIssues = issues
      const keys = issuesList(issues)
      if (keys.length > 0) {
        errorText = keys.map((k) => t(k as MessageKey)).join('\n')
        return
      }
      pending = {
        spec: buildDockerJobJSON(form),
        format: 'json',
        namespace,
        canonicalize: false,
      }
    } else {
      if (!spec.trim()) {
        errorText = t('runJob.err.specRequired')
        return
      }
      pending = { spec, format, namespace, canonicalize }
    }
    confirmOpen = true
  }

  function pendingJobID(): string {
    if (!pending) return ''
    if (tab === 'form') return formSummary.jobID
    return extractJobIDFromSpec(pending.spec, pending.format) || ''
  }

  function willOverwrite(): boolean {
    const id = pendingJobID()
    return !!id && existingSet.has(id.toLowerCase())
  }

  function confirmSummary(): string {
    if (!pending) return ''
    let body: string
    if (tab === 'form') {
      const id = extractJobIDFromSpec(pending.spec, pending.format) || formSummary.jobID
      body = t('runJob.confirmBodyForm', {
        jobID: id,
        type: formSummary.type,
        image: formSummary.image,
        count: String(formSummary.count),
        namespace: pending.namespace.trim() || 'default',
      })
    } else {
      const id = extractJobIDFromSpec(pending.spec, pending.format) || '—'
      body = t('runJob.confirmBodyAdvanced', {
        jobID: id,
        format: pending.format.toUpperCase(),
        namespace: pending.namespace.trim() || 'default',
      })
    }
    if (willOverwrite()) {
      body += '\n\n' + t('runJob.confirmOverwrite', { jobID: pendingJobID() })
    }
    return body
  }

  async function doRun(): Promise<void> {
    if (!pending) return
    const input = pending
    // 保持对话框 busy，直到提交结束
    const outcome = await onRun(input)
    confirmOpen = false
    pending = null
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

  const canRun = $derived(
    tab === 'form' ? form.jobID.trim() !== '' && form.image.trim() !== '' : spec.trim() !== '',
  )

  function tabClass(id: Tab): string {
    return tab === id
      ? 'border-b-2 border-sky-400 px-3 py-2 text-xs font-medium text-sky-300'
      : 'border-b-2 border-transparent px-3 py-2 text-xs text-zinc-500 hover:text-zinc-300'
  }
</script>

<div class="mx-auto w-full max-w-4xl p-6">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h1 class="text-lg font-semibold">{t('runJob.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('runJob.subtitle')}</p>
    </div>
    <button
      class="shrink-0 rounded bg-zinc-100 px-4 py-1.5 text-xs font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
      disabled={busy || !canRun}
      onclick={requestRun}
    >
      {busy ? t('runJob.running') : t('runJob.runButton')}
    </button>
  </div>

  <nav class="mt-6 flex gap-1 border-b border-zinc-800">
    <button class={tabClass('form')} onclick={() => switchTab('form')}>
      {t('runJob.tab.form')}
    </button>
    <button class={tabClass('advanced')} onclick={() => switchTab('advanced')}>
      {t('runJob.tab.advanced')}
    </button>
  </nav>

  <div class="mt-4 flex flex-wrap items-center gap-3">
    <label class="flex flex-col gap-0.5 text-xs text-zinc-400">
      <span class="flex items-center gap-1.5">
        {t('runJob.namespace')}
        <input
          class="w-36 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-sky-500"
          placeholder={t('runJob.namespacePlaceholder')}
          bind:value={namespace}
          spellcheck="false"
        />
      </span>
      <span class="text-[10px] text-zinc-600">{t('runJob.hintNamespace')}</span>
    </label>
    {#if tab === 'advanced'}
      <div class="flex rounded border border-zinc-700">
        <button
          class="rounded-l px-3 py-1.5 text-xs {format === 'hcl'
            ? 'bg-zinc-100 font-medium text-zinc-900'
            : 'text-zinc-400 hover:bg-zinc-800'}"
          onclick={() => requestFormatSwitch('hcl')}
        >
          HCL
        </button>
        <button
          class="rounded-r px-3 py-1.5 text-xs {format === 'json'
            ? 'bg-zinc-100 font-medium text-zinc-900'
            : 'text-zinc-400 hover:bg-zinc-800'}"
          onclick={() => requestFormatSwitch('json')}
        >
          JSON
        </button>
      </div>
      {#if format === 'hcl'}
        <label class="flex items-center gap-1.5 text-xs text-zinc-400">
          <input type="checkbox" class="accent-sky-500" bind:checked={canonicalize} />
          {t('runJob.canonicalize')}
        </label>
      {:else}
        <button
          class="rounded border border-zinc-700 px-2.5 py-1.5 text-xs text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
          onclick={formatSpec}
        >
          {t('runJob.formatJson')}
        </button>
      {/if}
    {/if}
  </div>

  {#if errorText}
    <div class="mt-3">
      <FormBanner kind="error" title={t('runJob.validationFailed')} message={errorText} />
    </div>
  {/if}

  {#if warnings}
    <div class="mt-3">
      <FormBanner kind="warning" title={t('runJob.warnings')} message={warnings} />
    </div>
  {/if}

  <div class="mt-4">
    {#if tab === 'form'}
      <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
        <p class="text-xs text-zinc-500">{t('runJob.form.hint')}</p>
        {#if onBrowseApps}
          <button class="text-[11px] text-sky-400 hover:text-sky-300" onclick={onBrowseApps}>
            {t('runJob.tpl.openApps')}
          </button>
        {/if}
      </div>
      <JobDockerForm bind:form issues={formIssues} />
    {:else}
      {#if format === 'hcl'}
        <div class="mb-3 flex flex-wrap items-center gap-2">
          <span class="text-[11px] text-zinc-500">{t('runJob.starter.label')}</span>
          {#each starters as s (s)}
            <button
              class="rounded px-2.5 py-1 text-[11px] {activeStarter === s &&
              spec.trim() === starterHCL(s).trim()
                ? 'bg-zinc-100 font-medium text-zinc-900'
                : 'border border-zinc-700 text-zinc-400 hover:bg-zinc-800'}"
              onclick={() => requestStarter(s)}
            >
              {t(
                s === 'docker'
                  ? 'runJob.starter.docker'
                  : s === 'exec'
                    ? 'runJob.starter.exec'
                    : 'runJob.starter.raw_exec',
              )}
            </button>
          {/each}
        </div>
        <p class="mb-3 text-[11px] leading-5 text-zinc-600">{t('runJob.starter.hint')}</p>
      {/if}
      <JobSpecEditor
        bind:value={spec}
        language={format}
        placeholder={format === 'hcl' ? t('runJob.placeholderHcl') : t('runJob.placeholderJson')}
      />
      <div class="mt-3 text-[11px] text-zinc-600">
        {format === 'hcl' ? t('runJob.hint') : t('runJob.hintJson')}
      </div>
    {/if}
  </div>
</div>

{#if confirmOpen && pending}
  <ConfirmDialog
    title={t('runJob.confirmTitle')}
    message={confirmSummary()}
    confirmLabel={t('runJob.runButton')}
    danger={willOverwrite()}
    {busy}
    onConfirm={() => void doRun()}
    onCancel={() => {
      if (busy) return
      confirmOpen = false
      pending = null
    }}
  />
{/if}

{#if formatConfirm === 'hcl'}
  <ConfirmDialog
    title={t('runJob.formatSwitchTitle')}
    message={t('runJob.formatSwitchBody')}
    confirmLabel={t('runJob.formatSwitchConfirm')}
    danger
    onConfirm={() => applyFormat('hcl')}
    onCancel={() => (formatConfirm = null)}
  />
{/if}

{#if starterConfirm}
  <ConfirmDialog
    title={t('runJob.starter.replaceTitle')}
    message={t('runJob.starter.replaceBody')}
    confirmLabel={t('runJob.starter.replaceConfirm')}
    danger
    onConfirm={() => {
      const id = starterConfirm
      if (id) applyStarter(id)
    }}
    onCancel={() => (starterConfirm = null)}
  />
{/if}
