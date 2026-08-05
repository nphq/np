<script lang="ts">
  // 通用二次确认：危险操作默认聚焦取消；Enter 仅在确认按钮聚焦时生效；
  // 可选 confirmPhrase 要求用户键入指定文本后才能确认（purge 等）。
  import { onMount } from 'svelte'
  import { t } from '../i18n/index.svelte'

  let {
    title,
    message,
    confirmLabel = t('common.confirm'),
    danger = false,
    busy = false,
    confirmPhrase = '',
    onConfirm,
    onCancel,
  }: {
    title: string
    message: string
    confirmLabel?: string
    danger?: boolean
    busy?: boolean
    /** 非空时，用户必须完整输入该短语才能点确认 */
    confirmPhrase?: string
    onConfirm: () => void
    onCancel: () => void
  } = $props()

  let cancelBtn: HTMLButtonElement | undefined = $state()
  let confirmBtn: HTMLButtonElement | undefined = $state()
  let typed = $state('')

  const phraseOK = $derived(!confirmPhrase || typed.trim() === confirmPhrase)

  onMount(() => {
    // 危险操作先聚焦取消，降低误触 Enter 确认的概率
    ;(danger ? cancelBtn : confirmBtn)?.focus()
  })

  function onKeydown(e: KeyboardEvent): void {
    if (busy) return
    if (e.key === 'Escape') {
      e.preventDefault()
      onCancel()
      return
    }
    // 仅当确认按钮已聚焦，或 Cmd/Ctrl+Enter 时确认（避免全局 Enter 误触）
    if (e.key === 'Enter') {
      const meta = e.metaKey || e.ctrlKey
      const onConfirmEl = document.activeElement === confirmBtn
      if ((meta || onConfirmEl) && phraseOK) {
        e.preventDefault()
        onConfirm()
      }
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<button
  class="fixed inset-0 z-50 flex w-full cursor-default items-center justify-center border-none bg-black/60 p-0"
  aria-label={t('common.cancel')}
  disabled={busy}
  onclick={() => !busy && onCancel()}
></button>

<div
  class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center"
  role="dialog"
  aria-modal="true"
  aria-labelledby="confirm-title"
>
  <div
    class="pointer-events-auto w-[400px] max-w-[92vw] rounded-lg border border-zinc-700 bg-zinc-900 p-4 shadow-2xl"
  >
    <h2 id="confirm-title" class="text-sm font-semibold text-zinc-100">{title}</h2>
    <p class="mt-2 text-xs whitespace-pre-wrap text-zinc-400">{message}</p>

    {#if confirmPhrase}
      <label class="mt-3 block text-[11px] text-zinc-500">
        <span class="flex flex-wrap items-center gap-2">
          <span>{t('confirm.typeToConfirm', { phrase: confirmPhrase })}</span>
          {#if confirmPhrase === 'DELETE'}
            <button
              type="button"
              class="rounded border border-zinc-600 bg-zinc-800 px-1.5 py-0.5 font-mono text-[10px] text-zinc-200 hover:border-sky-500 hover:bg-zinc-700 hover:text-white disabled:opacity-50"
              disabled={busy || typed.trim() === confirmPhrase}
              title={t('confirm.fillPhrase', { phrase: confirmPhrase })}
              onclick={(e) => {
                e.preventDefault()
                typed = confirmPhrase
              }}
            >
              {confirmPhrase}
            </button>
          {/if}
        </span>
        <input
          class="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 font-mono text-xs text-zinc-100 outline-none focus:border-red-500"
          bind:value={typed}
          disabled={busy}
          spellcheck="false"
          autocomplete="off"
          placeholder={confirmPhrase}
        />
      </label>
    {/if}

    <div class="mt-4 flex justify-end gap-2">
      <button
        bind:this={cancelBtn}
        class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
        disabled={busy}
        onclick={onCancel}
      >
        {t('common.cancel')}
      </button>
      <button
        bind:this={confirmBtn}
        class="rounded px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50 {danger
          ? 'bg-red-600 hover:bg-red-500'
          : 'bg-zinc-100 text-zinc-900 hover:bg-white'}"
        disabled={busy || !phraseOK}
        onclick={onConfirm}
      >
        {busy ? t('common.working') : confirmLabel}
      </button>
    </div>
  </div>
</div>
