<script lang="ts">
  // ClusterOverview 是 Overview 页主体＼�：CPU/Mem/Disk 环形图 +
  // 节点健康 + top 5 消费者。环形图用 stroke-dasharray 实现。
  import { formatMHz, formatMB, formatPct, level, utilization } from '../../utils/format'
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    load,
    onSelectJob,
  }: {
    load: nomad.ClusterLoad
    onSelectJob?: (jobID: string) => void
  } = $props()

  interface Donut {
    label: string
    used: number
    allocated: number
    capacity: number
    unit: string
    fmt: (v: number) => string
  }

  const donuts = $derived<Donut[]>([
    {
      label: t('overview.cpu'),
      used: load.used.cpu,
      allocated: load.allocated.cpu,
      capacity: load.capacity.cpu,
      unit: formatMHz(load.capacity.cpu),
      fmt: formatMHz,
    },
    {
      label: t('overview.memory'),
      used: load.used.memory,
      allocated: load.allocated.memory,
      capacity: load.capacity.memory,
      unit: formatMB(load.capacity.memory),
      fmt: formatMB,
    },
    {
      label: t('overview.disk'),
      used: load.used.disk,
      allocated: load.allocated.disk,
      capacity: load.capacity.disk,
      unit: formatMB(load.capacity.disk),
      fmt: formatMB,
    },
  ])

  const R = 40
  const C = 2 * Math.PI * R

  const updated = $derived(load.updatedAt ? new Date(load.updatedAt).toLocaleTimeString() : '—')
</script>

<div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
  {#each donuts as d (d.label)}
    {@const u = utilization(d.used, d.capacity)}
    {@const lv = level(u)}
    {@const dash = u === null ? 0 : u * C}
    {@const stroke =
      lv === 'crit' ? '#f7768e' : lv === 'warn' ? '#e0af68' : lv === 'ok' ? '#9ece6a' : '#3b4261'}
    <div class="flex flex-col items-center rounded border border-zinc-800 bg-zinc-900/50 p-4">
      <div class="relative h-28 w-28">
        <svg viewBox="0 0 100 100" class="h-full w-full -rotate-90">
          <circle cx="50" cy="50" r={R} fill="none" stroke="#292e42" stroke-width="10" />
          <circle
            cx="50"
            cy="50"
            r={R}
            fill="none"
            {stroke}
            stroke-width="10"
            stroke-linecap="round"
            stroke-dasharray="{dash} {C}"
          />
        </svg>
        <div class="absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-lg font-semibold tabular-nums">{formatPct(u ?? -1)}</span>
          <span class="text-[10px] text-zinc-500">{t('overview.used')}</span>
        </div>
      </div>
      <div class="mt-2 text-xs font-medium text-zinc-300">{d.label}</div>
      <div class="mt-1 flex flex-col items-center text-[10px] text-zinc-500 tabular-nums">
        <span>
          {t('overview.used')} <span class="text-zinc-300">{d.fmt(d.used)}</span>
          / {d.unit}
        </span>
        <span class="mt-0.5">
          {t('overview.allocated')}
          {d.fmt(d.allocated)}
        </span>
      </div>
    </div>
  {/each}
</div>

<div class="mt-4 grid grid-cols-1 gap-6 lg:grid-cols-2">
  <!-- 节点健康 -->
  <section class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
    <h3 class="text-xs font-semibold text-zinc-400 uppercase">{t('overview.nodes')}</h3>
    <div class="mt-3 flex items-center gap-4">
      <div class="text-2xl font-semibold tabular-nums">
        {load.nodeUp}<span class="text-sm text-zinc-500">/{load.nodeCount}</span>
      </div>
      <div class="text-xs text-zinc-500">
        {#if load.nodeCount > 0 && load.nodeUp === load.nodeCount}
          <span class="text-emerald-400">{t('overview.allHealthy')}</span>
        {:else if load.nodeUp < load.nodeCount}
          <span class="text-red-400"
            >{t('overview.unreachable', { n: load.nodeCount - load.nodeUp })}</span
          >
        {:else}
          {t('overview.noData')}
        {/if}
      </div>
      {#if !load.allocLevel}
        <span
          class="rounded bg-amber-400/10 px-2 py-0.5 text-[10px] text-amber-300"
          title={t('overview.nodeLevelTitle')}
        >
          {t('overview.nodeLevelOnly')}
        </span>
      {/if}
    </div>
  </section>

  <!-- Top 消费者 -->
  <section class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
    <h3 class="text-xs font-semibold text-zinc-400 uppercase">{t('overview.topConsumers')}</h3>
    {#if !load.topConsumers || load.topConsumers.length === 0}
      <div class="mt-3 text-xs text-zinc-600">{t('overview.noAllocData')}</div>
    {:else}
      <ul class="mt-2 divide-y divide-zinc-800/70">
        {#each load.topConsumers as c (c.allocID)}
          <li>
            <button
              class="flex w-full items-center justify-between py-1.5 text-left hover:bg-zinc-800/40 {onSelectJob
                ? 'cursor-pointer'
                : ''}"
              onclick={() => {
                const jobID = c.jobID?.trim()
                if (jobID) onSelectJob?.(jobID)
              }}
            >
              <div class="min-w-0">
                <div class="truncate font-mono text-xs text-zinc-300">{c.jobID}</div>
                <div class="truncate font-mono text-[10px] text-zinc-600">
                  {c.allocID.slice(0, 8)}
                </div>
              </div>
              <div class="flex shrink-0 items-center gap-3 text-[10px] text-zinc-400 tabular-nums">
                <span class="text-sky-300">{formatMHz(c.cpu)}</span>
                <span class="w-14 text-right text-amber-300">{formatMB(c.memory)}</span>
              </div>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<div class="mt-2 text-right text-[10px] text-zinc-600">
  {t('overview.lastUpdate', { time: updated })}
</div>
