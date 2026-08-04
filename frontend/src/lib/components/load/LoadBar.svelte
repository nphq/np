<script lang="ts">
  // LoadBar 是三段用量条：
  // 背景=容量 capacity（灰）→ allocated（蓝）→ used（绿/黄/红按阈值）。
  // 全部为 CSS 宽度，零图表库。
  import { level, utilization } from '../../utils/format'
  import { t } from '../../i18n/index.svelte'

  let {
    capacity = 0,
    allocated = 0,
    used = 0,
    showUsedLabel = false,
  }: {
    capacity?: number
    allocated?: number
    used?: number
    showUsedLabel?: boolean
  } = $props()

  const usedU = $derived(utilization(used, capacity))
  const allocU = $derived(utilization(allocated, capacity))
  const usedLv = $derived(level(usedU))
  const pct = $derived(usedU === null ? t('common.na') : `${Math.round(usedU * 100)}%`)

  const usedCls = $derived(
    usedLv === 'crit' ? 'bg-red-500' : usedLv === 'warn' ? 'bg-amber-400' : 'bg-emerald-500',
  )
</script>

<div
  class="group flex items-center gap-2"
  title={`${t('overview.used')} ${pct} · ${t('overview.allocated')} ${Math.round((allocU ?? 0) * 100)}%`}
>
  <div class="relative h-1.5 w-full min-w-12 overflow-hidden rounded-full bg-zinc-800">
    {#if capacity > 0}
      {#if allocU !== null && allocU > 0}
        <div
          class="absolute inset-y-0 left-0 rounded-full bg-sky-500/60"
          style="width: {allocU * 100}%"
        ></div>
      {/if}
      {#if usedU !== null && usedU > 0}
        <div
          class="absolute inset-y-0 left-0 rounded-full {usedCls}"
          style="width: {usedU * 100}%"
        ></div>
      {/if}
    {/if}
  </div>
  {#if showUsedLabel}
    <span class="w-8 shrink-0 text-right text-[10px] text-zinc-400 tabular-nums">{pct}</span>
  {/if}
</div>
