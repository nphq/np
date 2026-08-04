<script lang="ts">
  // ConfirmDialog 是通用二次确认对话框＼�M4 复用）。
  // ESC/遮罩关闭；Enter 确认；danger=true 时确认键为红色。
  import { t } from '../i18n/index.svelte'
  let {
    title,
    message,
    confirmLabel = t('common.confirm'),
    danger = false,
    busy = false,
    onConfirm,
    onCancel,
  }: {
    title: string
    message: string
    confirmLabel?: string
    danger?: boolean
    busy?: boolean
    onConfirm: () => void
    onCancel: () => void
  } = $props()

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') onCancel()
    if (e.key === 'Enter' && !busy) onConfirm()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<button
  class="fixed inset-0 z-50 flex w-full cursor-default items-center justify-center border-none bg-black/60 p-0"
  aria-label={t('common.cancel')}
  onclick={() => !busy && onCancel()}
></button>

<div class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center">
  <div
    class="pointer-events-auto w-[400px] rounded-lg border border-zinc-700 bg-zinc-900 p-4 shadow-2xl"
  >
    <h2 class="text-sm font-semibold">{title}</h2>
    <p class="mt-2 text-xs whitespace-pre-wrap text-zinc-400">{message}</p>
    <div class="mt-4 flex justify-end gap-2">
      <button
        class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
        disabled={busy}
        onclick={onCancel}
      >
        {t('common.cancel')}
      </button>
      <button
        class="rounded px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50 {danger
          ? 'bg-red-600 hover:bg-red-500'
          : 'bg-zinc-100 text-zinc-900 hover:bg-white'}"
        disabled={busy}
        onclick={onConfirm}
      >
        {busy ? t('common.working') : confirmLabel}
      </button>
    </div>
  </div>
</div>
