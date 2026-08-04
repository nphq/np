<script lang="ts">
  import { onMount } from 'svelte'
  import { ClusterInput } from '../stores/clusters.svelte'
  import { TestConnectionInput } from '../../../bindings/github.com/nphq/np/app'
  import { isErr } from '../stores/clusters.svelte'
  import { t } from '../i18n/index.svelte'
  import type { nomad, uiapi } from '../types/wails'

  type Mode = 'add' | 'edit'

  let {
    mode = 'add' as Mode,
    initial = new ClusterInput(),
    hasToken = false,
    onSubmit,
    onClose,
  } = $props<{
    mode?: Mode
    initial?: ClusterInput
    hasToken?: boolean
    onSubmit: (input: ClusterInput) => Promise<boolean>
    onClose: () => void
  }>()

  let form = $state<ClusterInput>(new ClusterInput())
  let saving = $state(false)
  let error = $state('')
  let addressInput: HTMLInputElement | undefined = $state()

  // Test 状态：'' | 'testing' | 'ok:<leader>:<version>' | 'fail:<msg>'
  let testState = $state<{
    kind: 'idle' | 'testing' | 'ok' | 'fail'
    leader?: string
    version?: string
    error?: string
  }>({ kind: 'idle' })

  onMount(() => {
    // 拷贝 initial 防外部 mutation；edit 模式清掉 token（让用户决定是否重填）
    form = new ClusterInput()
    Object.assign(form, initial)
    if (mode === 'edit') form.token = ''
    addressInput?.focus()
  })

  function cancel(): void {
    onClose()
  }

  function inputFor(field: 'id' | 'name' | 'address' | 'region' | 'namespace' | 'token') {
    return {
      value: form[field],
      oninput: (e: Event) => {
        form[field] = (e.target as HTMLInputElement).value
        // 任一字段变动后，上一次的 Test 结果就不再可信
        if (testState.kind !== 'idle') testState = { kind: 'idle' }
      },
    }
  }

  async function submit(): Promise<void> {
    error = ''
    if (!form.id.trim()) {
      error = t('cluster.errId')
      return
    }
    if (!form.address.trim()) {
      error = t('cluster.errAddress')
      return
    }
    saving = true
    try {
      const ok = await onSubmit(form)
      if (ok) onClose()
    } finally {
      saving = false
    }
  }

  async function runTest(): Promise<void> {
    if (!form.id.trim() || !form.address.trim()) {
      error = t('cluster.errAddress')
      return
    }
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
      <button
        class="rounded px-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300"
        onclick={cancel}
        aria-label={t('common.cancel')}
      >
        ✕
      </button>
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
            bind:this={addressInput}
            {...inputFor('id')}
            placeholder={t('cluster.placeholderId')}
            disabled={mode === 'edit'}
            class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500 disabled:cursor-not-allowed disabled:bg-zinc-900 disabled:text-zinc-500"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.name')}</span>
          <input
            {...inputFor('name')}
            placeholder={t('cluster.placeholderName')}
            class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500"
          />
        </label>
      </div>
      {#if mode === 'edit'}
        <div class="text-[10px] text-zinc-600">{t('cluster.idReadonly')}</div>
      {/if}

      <label class="block">
        <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.address')}</span>
        <input
          {...inputFor('address')}
          placeholder={t('cluster.placeholderAddress')}
          spellcheck="false"
          class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500"
        />
      </label>

      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.region')}</span
          >
          <input
            {...inputFor('region')}
            placeholder={t('cluster.placeholderRegion')}
            class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] font-medium text-zinc-400"
            >{t('cluster.namespace')}</span
          >
          <input
            {...inputFor('namespace')}
            placeholder={t('cluster.placeholderNamespace')}
            class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500"
          />
        </label>
      </div>

      <label class="block">
        <span class="mb-1 block text-[11px] font-medium text-zinc-400">{t('cluster.token')}</span>
        <input
          {...inputFor('token')}
          type="password"
          placeholder={mode === 'edit' ? t('cluster.tokenKeepHint') : t('cluster.placeholderToken')}
          autocomplete="off"
          class="w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500"
        />
      </label>
      {#if mode === 'edit' && hasToken}
        <div class="-mt-1 text-[10px] text-emerald-500/80">{t('cluster.tokenSaved')}</div>
      {/if}

      <label class="flex items-center gap-2 text-xs text-zinc-400">
        <input
          type="checkbox"
          checked={form.tls}
          onchange={(e) => {
            form.tls = (e.target as HTMLInputElement).checked
          }}
          class="accent-zinc-500"
        />
        {t('cluster.useHttps')}
      </label>
      {#if form.tls}
        <label class="flex items-center gap-2 text-xs text-zinc-500">
          <input
            type="checkbox"
            checked={form.insecureSkipVerify}
            onchange={(e) => {
              form.insecureSkipVerify = (e.target as HTMLInputElement).checked
            }}
            class="accent-zinc-500"
          />
          {t('cluster.skipVerify')}
        </label>
      {/if}

      {#if testState.kind === 'ok'}
        <div
          class="rounded border border-emerald-900 bg-emerald-950/50 px-3 py-2 text-xs text-emerald-400"
        >
          {t('cluster.testOk', {
            leader: testState.leader || '?',
            version: testState.version || '?',
          })}
        </div>
      {:else if testState.kind === 'fail'}
        <div
          class="rounded border border-amber-900 bg-amber-950/50 px-3 py-2 text-xs text-amber-400"
        >
          {t('cluster.testFail', { error: testState.error || '' })}
        </div>
      {/if}

      {#if error}
        <div class="rounded border border-red-900 bg-red-950/50 px-3 py-2 text-xs text-red-400">
          {error}
        </div>
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
