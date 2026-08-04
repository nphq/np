<script lang="ts">
  // JobRunScreen 是「部署」入口＼�对标 /ui/jobs/run）：
  // HCL/JSON 规格编辑 → Run（后端 Parse→Validate→Register 流水）。
  // 解析/校验错误由后端归一为 invalid_input 并携带全部条目，此处内联展示。
  import { toast } from '../../stores/clusters.svelte'
  import { t } from '../../i18n/index.svelte'
  import type { RunJobInput, RunJobOutcome } from '../../stores/jobs.svelte'

  let {
    busy,
    onRun,
    onDone,
  }: {
    busy: boolean
    onRun: (input: RunJobInput) => Promise<RunJobOutcome>
    onDone: (jobID: string) => void
  } = $props()

  const starterHCL = `job "example" {
 datacenters = ["dc1"]
 type = "service"

 group "web" {
 count = 2

 task "server" {
 driver = "docker"
 config {
 image = "nginx:latest"
 }

 resources {
 cpu = 500
 memory = 256
 }
 }
 }
}`

  let spec = $state(starterHCL)
  let format = $state<'hcl' | 'json'>('hcl')
  let namespace = $state('')
  let canonicalize = $state(false)
  let errorText = $state('')
  let warnings = $state('')

  function switchFormat(next: 'hcl' | 'json'): void {
    if (next === format) return
    spec = ''
    errorText = ''
    warnings = ''
    format = next
  }

  async function run(): Promise<void> {
    errorText = ''
    warnings = ''
    const outcome = await onRun({ spec, format, namespace, canonicalize })
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
</script>

<div class="mx-auto w-full max-w-4xl p-6">
  <div class="flex items-start justify-between">
    <div>
      <h1 class="text-lg font-semibold">{t('runJob.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('runJob.subtitle')}</p>
    </div>
  </div>

  <div class="mt-6 flex items-center gap-2">
    <div class="flex rounded border border-zinc-700">
      <button
        class="rounded-l px-3 py-1.5 text-xs {format === 'hcl'
          ? 'bg-zinc-100 font-medium text-zinc-900'
          : 'text-zinc-400 hover:bg-zinc-800'}"
        onclick={() => switchFormat('hcl')}
      >
        HCL
      </button>
      <button
        class="rounded-r px-3 py-1.5 text-xs {format === 'json'
          ? 'bg-zinc-100 font-medium text-zinc-900'
          : 'text-zinc-400 hover:bg-zinc-800'}"
        onclick={() => switchFormat('json')}
      >
        JSON
      </button>
    </div>
    <label class="flex items-center gap-1.5 text-xs text-zinc-400">
      {t('runJob.namespace')}
      <input
        class="w-36 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-sky-500"
        placeholder={t('runJob.namespacePlaceholder')}
        bind:value={namespace}
      />
    </label>
    <label class="flex items-center gap-1.5 text-xs text-zinc-400">
      <input type="checkbox" class="accent-sky-500" bind:checked={canonicalize} />
      {t('runJob.canonicalize')}
    </label>
    <div class="flex-1"></div>
    <button
      class="rounded bg-zinc-100 px-4 py-1.5 text-xs font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
      disabled={busy || spec.trim() === ''}
      onclick={() => void run()}
    >
      {busy ? t('runJob.running') : t('runJob.runButton')}
    </button>
  </div>

  {#if errorText}
    <div class="mt-3 rounded border border-red-800 bg-red-950/60 p-3">
      <div class="text-[11px] font-medium text-red-300 uppercase">
        {t('runJob.validationFailed')}
      </div>
      <pre class="mt-1 text-xs whitespace-pre-wrap text-red-300/90">{errorText}</pre>
    </div>
  {/if}

  {#if warnings}
    <div class="mt-3 rounded border border-amber-400/30 bg-amber-400/10 p-3">
      <div class="text-[11px] font-medium text-amber-300 uppercase">{t('runJob.warnings')}</div>
      <pre class="mt-1 text-xs whitespace-pre-wrap text-amber-300/90">{warnings}</pre>
    </div>
  {/if}

  <div class="mt-4">
    <textarea
      class="h-[420px] w-full resize-none rounded border border-zinc-800 bg-zinc-950 p-3 font-mono text-xs leading-5 text-zinc-200 outline-none focus:border-sky-500"
      spellcheck="false"
      placeholder={format === 'hcl' ? t('runJob.placeholderHcl') : t('runJob.placeholderJson')}
      bind:value={spec}></textarea>
  </div>

  <div class="mt-3 text-[11px] text-zinc-600">
    {format === 'hcl' ? t('runJob.hint') : t('runJob.hintJson')}
  </div>
</div>
