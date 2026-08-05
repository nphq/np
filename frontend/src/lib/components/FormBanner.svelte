<script lang="ts">
  // 表单顶部错误 / 警告条，统一校验与服务器回执展示。

  let {
    kind = 'error',
    title = '',
    message = '',
  }: {
    kind?: 'error' | 'warning' | 'info'
    title?: string
    message: string
  } = $props()

  const box = $derived(
    kind === 'error'
      ? 'border-red-800 bg-red-950/60 text-red-300'
      : kind === 'warning'
        ? 'border-amber-400/30 bg-amber-400/10 text-amber-300'
        : 'border-sky-800/50 bg-sky-950/40 text-sky-300',
  )
</script>

{#if message}
  <div class="rounded border px-3 py-2 text-xs {box}">
    {#if title}
      <div class="text-[11px] font-medium tracking-wide uppercase opacity-90">{title}</div>
    {/if}
    <pre class="mt-1 whitespace-pre-wrap {title ? 'opacity-90' : ''}">{message}</pre>
  </div>
{/if}
