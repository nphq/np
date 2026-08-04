<script lang="ts">
  // Sparkline 是 60 点历史曲线（SVG polyline）。
  // 数据来自后端环形缓冲（15s × 15min），无外部图表库。
  import { sparklinePoints } from '../../utils/format'

  let {
    values = [],
    color = 'text-sky-400',
    width = 96,
    height = 22,
    label = '',
  }: {
    values?: number[]
    color?: string
    width?: number
    height?: number
    label?: string
  } = $props()

  const points = $derived(sparklinePoints(values, width, height))
  const last = $derived(values.length > 0 ? values[values.length - 1] : null)
</script>

<div class="flex items-center gap-1.5" title={label}>
  <svg {width} {height} class="shrink-0 overflow-visible">
    <polyline {points} fill="none" stroke="currentColor" stroke-width="1.5" class={color} />
  </svg>
  {#if last !== null}
    <span class="w-11 shrink-0 text-right text-[10px] text-zinc-500 tabular-nums">
      {last >= 1000 ? `${(last / 1000).toFixed(1)}G` : `${Math.round(last)}`}
    </span>
  {/if}
</div>
