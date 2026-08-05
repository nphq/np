<script lang="ts">
  // Docker 服务表单：字段 → Nomad API JSON Job Spec。
  import { t } from '../../i18n/index.svelte'
  import type { MessageKey } from '../../i18n/dictionaries/zh'
  import { buildDockerJobJSON, type DockerJobForm, type JobType } from '../../jobs/spec'
  import type { DockerFormIssues } from '../../utils/validate'

  let {
    form = $bindable(),
    showPreview = true,
    issues = {},
  }: {
    form: DockerJobForm
    showPreview?: boolean
    issues?: DockerFormIssues
  } = $props()

  const preview = $derived(buildDockerJobJSON(form))

  const types: JobType[] = ['service', 'batch', 'system']
  const envPlaceholder = 'KEY=value\nANOTHER=1'

  function fieldClass(bad = false): string {
    return `w-full rounded border bg-zinc-900 px-2.5 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 ${
      bad ? 'border-red-700 focus:border-red-500' : 'border-zinc-700 focus:border-sky-500'
    }`
  }

  function hint(key: string | undefined, fallback: string): string {
    return key ? t(key as MessageKey) : fallback
  }
</script>

<div class="grid gap-4 sm:grid-cols-2">
  <label class="block text-xs text-zinc-400">
    {t('runJob.form.jobID')}
    <input
      class="mt-1 {fieldClass(!!issues.jobID)}"
      bind:value={form.jobID}
      placeholder="example"
      spellcheck="false"
    />
    <span class="mt-0.5 block text-[10px] {issues.jobID ? 'text-red-400' : 'text-zinc-600'}">
      {hint(issues.jobID, t('runJob.form.hintJobID'))}
    </span>
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.type')}
    <select class="mt-1 {fieldClass()}" bind:value={form.type}>
      {#each types as ty (ty)}
        <option value={ty}>{ty}</option>
      {/each}
    </select>
  </label>

  <label class="block text-xs text-zinc-400 sm:col-span-2">
    {t('runJob.form.datacenters')}
    <input class="mt-1 {fieldClass()}" bind:value={form.datacenters} placeholder="dc1, dc2" />
    <span class="mt-0.5 block text-[10px] text-zinc-600">{t('runJob.form.hintDatacenters')}</span>
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.groupName')}
    <input class="mt-1 {fieldClass()}" bind:value={form.groupName} />
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.count')}
    <input
      class="mt-1 {fieldClass(!!issues.count)}"
      type="number"
      min="1"
      bind:value={form.count}
    />
    {#if issues.count}
      <span class="mt-0.5 block text-[10px] text-red-400">{t(issues.count as MessageKey)}</span>
    {/if}
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.taskName')}
    <input class="mt-1 {fieldClass()}" bind:value={form.taskName} />
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.image')}
    <input
      class="mt-1 {fieldClass(!!issues.image)}"
      bind:value={form.image}
      placeholder="nginx:latest"
      spellcheck="false"
    />
    <span class="mt-0.5 block text-[10px] {issues.image ? 'text-red-400' : 'text-zinc-600'}">
      {hint(issues.image, t('runJob.form.hintImage'))}
    </span>
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.portLabel')}
    <input
      class="mt-1 {fieldClass(!!issues.port)}"
      bind:value={form.portLabel}
      placeholder="http"
    />
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.portTo')}
    <input
      class="mt-1 {fieldClass(!!issues.port)}"
      type="number"
      min="1"
      max="65535"
      value={form.portTo ?? ''}
      oninput={(e) => {
        const v = (e.currentTarget as HTMLInputElement).value
        form.portTo = v === '' ? null : Number(v)
      }}
    />
    <span class="mt-0.5 block text-[10px] {issues.port ? 'text-red-400' : 'text-zinc-600'}">
      {hint(issues.port, t('runJob.form.hintPort'))}
    </span>
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.cpu')}
    <input class="mt-1 {fieldClass(!!issues.cpu)}" type="number" min="1" bind:value={form.cpu} />
    {#if issues.cpu}
      <span class="mt-0.5 block text-[10px] text-red-400">{t(issues.cpu as MessageKey)}</span>
    {/if}
  </label>

  <label class="block text-xs text-zinc-400">
    {t('runJob.form.memory')}
    <input
      class="mt-1 {fieldClass(!!issues.memory)}"
      type="number"
      min="1"
      bind:value={form.memory}
    />
    {#if issues.memory}
      <span class="mt-0.5 block text-[10px] text-red-400">{t(issues.memory as MessageKey)}</span>
    {/if}
  </label>

  <label class="block text-xs text-zinc-400 sm:col-span-2">
    {t('runJob.form.env')}
    <textarea
      class="mt-1 h-20 resize-y font-mono {fieldClass(!!issues.env)}"
      bind:value={form.envText}
      placeholder={envPlaceholder}
      spellcheck="false"></textarea>
    <span class="mt-0.5 block text-[10px] {issues.env ? 'text-red-400' : 'text-zinc-600'}">
      {hint(issues.env, t('runJob.form.hintEnv'))}
    </span>
  </label>
</div>

{#if showPreview}
  <details class="mt-4 rounded border border-zinc-800 bg-zinc-950/60 open:pb-0">
    <summary
      class="cursor-pointer px-3 py-2 text-[11px] font-medium tracking-wide text-zinc-500 uppercase select-none hover:text-zinc-300"
    >
      {t('runJob.form.preview')}
    </summary>
    <pre
      class="max-h-56 overflow-auto border-t border-zinc-800 p-3 font-mono text-[11px] leading-4 text-zinc-400">{preview}</pre>
  </details>
{/if}
