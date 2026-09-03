<script lang="ts">
  import { onMount } from 'svelte'
  import { ClusterInput } from '../stores/clusters.svelte'
  import { TestConnectionInput } from '../../../bindings/github.com/nphq/np/internal/app/app'
  import { isErr } from '../stores/clusters.svelte'
  import { t } from '../i18n/index.svelte'
  import FormBanner from './FormBanner.svelte'
  import {
    addressLooksHTTPS,
    isValidClusterID,
    validateAddress,
    validateClusterName,
    validateNamespace,
    validateRegion,
  } from '../utils/validate'
  import type { nomad, uiapi } from '../types/wails'

  type Mode = 'add' | 'edit'
  type SubmitResult = { ok: true } | { ok: false; error?: string }

  let {
    mode = 'add' as Mode,
    initial = new ClusterInput(),
    hasToken = false,
    hasLegacyToken = false,
    discovered = [] as uiapi.DiscoveredCluster[],
    onSubmit,
    onClose,
  } = $props<{
    mode?: Mode
    initial?: ClusterInput
    hasToken?: boolean
    hasLegacyToken?: boolean
    discovered?: uiapi.DiscoveredCluster[]
    onSubmit: (input: ClusterInput) => Promise<boolean | SubmitResult>
    onClose: () => void
  }>()

  let form = $state<ClusterInput>(new ClusterInput())
  let saving = $state(false)
  let error = $state('')
  let fieldErrors = $state<Record<string, string>>({})
  let idInput: HTMLInputElement | undefined = $state()
  let addressInput: HTMLInputElement | undefined = $state()

  let testState = $state<{
    kind: 'idle' | 'testing' | 'ok' | 'fail'
    leader?: string
    version?: string
    error?: string
  }>({ kind: 'idle' })

  // 从环境/文件发现的候选（不含 token 明文）；仅填充非敏感字段。
  let envTokenHint = $state('')

  onMount(() => {
    form = new ClusterInput()
    Object.assign(form, initial)
    if (mode === 'edit') form.token = ''
    if (mode === 'edit') addressInput?.focus()
    else idInput?.focus()
  })

  // 用发现的候选预填非敏感字段（M2.2）。Token 不在 Discover 响应里；
  // hasToken 时置 useEnvToken：Test/Save 由后端读 NOMAD_TOKEN，不把明文填进 input。
  function fillFromEnv(): void {
    const d = discovered[0]
    if (!d || mode === 'edit') return
    form.id = d.suggestedID
    form.name = d.name
    form.address = d.address
    form.region = d.region
    form.namespace = d.namespace
    form.tls = d.tls
    form.insecureSkipVerify = d.insecureSkipVerify
    form.token = ''
    form.useEnvToken = !!d.hasToken
    envTokenHint = d.hasToken ? t('discover.usingEnvToken') : ''
    markDirty()
  }

  function cancel(): void {
    onClose()
  }

  function markDirty(): void {
    if (testState.kind !== 'idle') testState = { kind: 'idle' }
    error = ''
  }

  function onAddressInput(e: Event): void {
    const v = (e.target as HTMLInputElement).value
    form.address = v
    // https:// 地址自动勾选 TLS，避免协议与开关不一致
    if (addressLooksHTTPS(v)) form.tls = true
    markDirty()
    delete fieldErrors.address
    fieldErrors = { ...fieldErrors }
  }

  function validateLocal(): boolean {
    const next: Record<string, string> = {}
    if (mode === 'add') {
      const id = form.id.trim()
      if (!id) next.id = t('cluster.errId')
      else if (!isValidClusterID(id)) next.id = t('cluster.errIdFormat')
    }
    if (!validateClusterName(form.name)) next.name = t('cluster.errName')
    const addr = validateAddress(form.address)
    if (!addr.ok) {
      next.address =
        addr.reason === 'empty'
          ? t('cluster.errAddress')
          : addr.reason === 'port'
            ? t('cluster.errAddressPort')
            : t('cluster.errAddressHost')
    }
    if (!validateRegion(form.region.trim())) next.region = t('cluster.errRegion')
    if (!validateNamespace(form.namespace.trim())) next.namespace = t('cluster.errNamespace')
    if (form.insecureSkipVerify && !form.tls) {
      next.tls = t('cluster.errSkipNeedsTls')
    }
    fieldErrors = next
    if (Object.keys(next).length > 0) {
      error = Object.values(next).join('\n')
      return false
    }
    error = ''
    return true
  }

  function fieldClass(name: string): string {
    const bad = !!fieldErrors[name]
    return `w-full rounded border bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none disabled:cursor-not-allowed disabled:bg-zinc-900 disabled:text-zinc-500 ${
      bad ? 'border-red-700 focus:border-red-500' : 'border-zinc-700 focus:border-zinc-500'
    }`
  }

  async function submit(): Promise<void> {
    if (!validateLocal()) return
    saving = true
    error = ''
    try {
      const res = await onSubmit(form)
      if (res === true || (typeof res === 'object' && res.ok)) {
        onClose()
        return
      }
      const msg =
        typeof res === 'object' && !res.ok && res.error ? res.error : t('cluster.errSaveFailed')
      error = msg
    } finally {
      saving = false
    }
  }

  async function runTest(): Promise<void> {
    if (!validateLocal()) return
    testState = { kind: 'testing' }
    error = ''
    try {
      const res = await TestConnectionInput(form)
      if (isErr(res)) {
        testState = { kind: 'fail', error: (res as uiapi.Error).message }
        return
      }
      const h = res as nomad.ClusterHealth
      if (h.status === 'ok') {
        testState = { kind: 'ok', leader: h.leader, version: h.version }
      } else {
        testState = { kind: 'fail', error: h.error || 'unknown' }
      }
    } catch (e) {
      testState = { kind: 'fail', error: String(e) }
    }
  }
</script>

<button
  class="fixed inset-0 z-50 w-full cursor-default border-none bg-black/60 p-0"
  aria-label={t('common.cancel')}
  onclick={() => cancel()}
></button>

<div
  class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center"
  role="dialog"
  tabindex="-1"
  aria-modal="true"
  aria-label={t(mode === 'edit' ? 'cluster.editTitle' : 'cluster.addTitle')}
  onkeydown={(e) => {
    if (e.key === 'Escape') cancel()
  }}
>
  <div
    class="pointer-events-auto w-[440px] max-w-[92vw] rounded-lg border border-zinc-700 bg-zinc-900 shadow-2xl"
  >
    <div class="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
      <h2 class="text-sm font-semibold text-zinc-100">
        {t(mode === 'edit' ? 'cluster.editTitle' : 'cluster.addTitle')}
      </h2>
      <div class="flex items-center gap-1">
        {#if mode === 'add' && discovered.length > 0}
          <button
            type="button"
            class="rounded border border-zinc-700 px-2 py-0.5 text-[11px] text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
            title={t('discover.fillFromEnvTitle')}
            onclick={fillFromEnv}
          >
            {t('discover.fillFromEnv')}
          </button>
        {/if}
        <button
          class="rounded px-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300"
          onclick={cancel}
          aria-label={t('common.cancel')}
        >
          ✕
        </button>
      </div>
    </div>

    <form
      class="space-y-3 px-4 py-4"
      onsubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.id')}</span>
          <input
            bind:this={idInput}
            value={form.id}
            oninput={(e) => {
              form.id = (e.target as HTMLInputElement).value
              markDirty()
              delete fieldErrors.id
              fieldErrors = { ...fieldErrors }
            }}
            placeholder={t('cluster.placeholderId')}
            disabled={mode === 'edit'}
            spellcheck="false"
            class={fieldClass('id')}
          />
          <span class="mt-0.5 block text-[10px] text-zinc-600">{t('cluster.hintId')}</span>
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.name')}</span>
          <input
            value={form.name}
            oninput={(e) => {
              form.name = (e.target as HTMLInputElement).value
              markDirty()
            }}
            placeholder={t('cluster.placeholderName')}
            class={fieldClass('name')}
          />
        </label>
      </div>
      {#if mode === 'edit'}
        <div class="text-[10px] text-zinc-600">{t('cluster.idReadonly')}</div>
      {/if}

      <label class="block">
        <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.address')}</span>
        <input
          bind:this={addressInput}
          value={form.address}
          oninput={onAddressInput}
          placeholder={t('cluster.placeholderAddress')}
          spellcheck="false"
          class={fieldClass('address')}
        />
        <span class="mt-0.5 block text-[10px] text-zinc-600">{t('cluster.hintAddress')}</span>
      </label>

      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.region')}</span
          >
          <input
            value={form.region}
            oninput={(e) => {
              form.region = (e.target as HTMLInputElement).value
              markDirty()
            }}
            placeholder={t('cluster.placeholderRegion')}
            class={fieldClass('region')}
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400"
            >{t('cluster.namespace')}</span
          >
          <input
            value={form.namespace}
            oninput={(e) => {
              form.namespace = (e.target as HTMLInputElement).value
              markDirty()
            }}
            placeholder={t('cluster.placeholderNamespace')}
            class={fieldClass('namespace')}
          />
        </label>
      </div>

      <label class="block">
        <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.token')}</span>
        <input
          value={form.token}
          oninput={(e) => {
            form.token = (e.target as HTMLInputElement).value
            // 用户手填 token 后不再走环境变量回填
            form.useEnvToken = false
            envTokenHint = ''
            markDirty()
          }}
          type="password"
          placeholder={mode === 'edit' ? t('cluster.tokenKeepHint') : t('cluster.placeholderToken')}
          autocomplete="off"
          class={fieldClass('token')}
        />
        <span class="mt-0.5 block text-[10px] text-zinc-600">{t('cluster.hintToken')}</span>
      </label>
      {#if envTokenHint}
        <div class="-mt-1 text-[10px] text-amber-500/90">{envTokenHint}</div>
      {/if}
      {#if mode === 'edit' && hasToken}
        <div class="-mt-1 text-[10px] text-emerald-500/80">{t('cluster.tokenSaved')}</div>
      {/if}
      {#if mode === 'edit' && hasLegacyToken && !hasToken}
        <div class="-mt-1 text-[10px] text-amber-500/90">{t('cluster.legacyTokenTip')}</div>
      {/if}

      <label class="flex items-center gap-2 text-xs text-zinc-400">
        <input
          type="checkbox"
          checked={form.tls}
          onchange={(e) => {
            form.tls = (e.target as HTMLInputElement).checked
            if (!form.tls) form.insecureSkipVerify = false
            markDirty()
          }}
          class="accent-zinc-500"
        />
        {t('cluster.useHttps')}
      </label>
      {#if form.tls}
        <label class="flex flex-col gap-1 text-xs text-zinc-500">
          <span class="flex items-center gap-2">
            <input
              type="checkbox"
              checked={form.insecureSkipVerify}
              onchange={(e) => {
                form.insecureSkipVerify = (e.target as HTMLInputElement).checked
                markDirty()
              }}
              class="accent-zinc-500"
            />
            {t('cluster.skipVerify')}
          </span>
          {#if form.insecureSkipVerify}
            <span class="pl-6 text-[10px] text-amber-500/90">{t('cluster.hintSkipVerify')}</span>
          {/if}
        </label>
      {/if}

      {#if testState.kind === 'ok'}
        <FormBanner
          kind="info"
          message={t('cluster.testOk', {
            leader: testState.leader || '?',
            version: testState.version || '?',
          })}
        />
      {:else if testState.kind === 'fail'}
        <FormBanner
          kind="warning"
          message={t('cluster.testFail', { error: testState.error || '' })}
        />
      {/if}

      {#if error}
        <FormBanner kind="error" title={t('cluster.errTitle')} message={error} />
      {/if}

      <div class="flex justify-between gap-2 pt-1">
        <button
          type="button"
          disabled={testState.kind === 'testing'}
          onclick={() => void runTest()}
          class="rounded border border-zinc-700 px-3 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
        >
          {testState.kind === 'testing' ? t('cluster.testing') : t('cluster.test')}
        </button>
        <div class="flex gap-2">
          <button
            type="button"
            class="rounded border border-zinc-700 px-3 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800"
            onclick={cancel}
          >
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            disabled={saving}
            class="rounded bg-zinc-100 px-3 py-1.5 text-sm font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
          >
            {saving
              ? t('common.saving')
              : mode === 'edit'
                ? t('cluster.save')
                : t('cluster.saveConnect')}
          </button>
        </div>
      </div>
    </form>
  </div>
</div>
