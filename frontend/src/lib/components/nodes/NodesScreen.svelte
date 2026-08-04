<script lang="ts">
  // NodesScreen 是节点列表屏：容量/已分配/实时用量三段条 + 60 点历史 sparkline
  // （对标 Lens Nodes 视图）。行数据 = NodeSummary ⊕ NodeLoad。
  import LoadBar from '../load/LoadBar.svelte'
  import Sparkline from '../load/Sparkline.svelte'
  import { formatMHz, formatMB, level, utilization } from '../../utils/format'
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    nodes,
    loads,
    busy,
    onRefresh,
    onSelect,
  }: {
    nodes: nomad.NodeSummary[]
    loads: Map<string, nomad.NodeLoad>
    busy: boolean
    onRefresh: () => void
    onSelect: (nodeID: string) => void
  } = $props()

  function statusDot(status: string): string {
    switch (status) {
      case 'ready':
        return 'bg-emerald-500'
      case 'down':
        return 'bg-red-500'
      case 'initializing':
        return 'bg-amber-400'
      default:
        return 'bg-zinc-600'
    }
  }

  function nodeLoad(n: nomad.NodeSummary): nomad.NodeLoad | undefined {
    return loads.get(n.id)
  }

  // barValues 返回 LoadBar 三段值：capacity / allocated / used。
  // 优先用 NodeLoad（实时），否则退回 NodeSummary 的静态派生值。
  function barValues(n: nomad.NodeSummary, kind: 'cpu' | 'memory' | 'disk') {
    const nl = nodeLoad(n)
    let capacity = n.cpuTotal
    let allocated = 0
    let used = n.cpu
    if (kind === 'memory') {
      capacity = n.memoryTotal
      used = n.memory
    } else if (kind === 'disk') {
      capacity = n.diskTotal
      used = n.disk
    }
    if (nl) {
      if (kind === 'cpu') {
        capacity = nl.capacity.cpu
        allocated = nl.allocated.cpu
        used = nl.used.cpu
      } else if (kind === 'memory') {
        capacity = nl.capacity.memory
        allocated = nl.allocated.memory
        used = nl.used.memory
      } else {
        capacity = nl.capacity.disk
        allocated = nl.allocated.disk
        used = nl.used.disk
      }
    }
    return { capacity, allocated, used }
  }

  const cpuColor = $derived((pct: number | null) =>
    level(pct) === 'crit'
      ? 'text-red-500'
      : level(pct) === 'warn'
        ? 'text-amber-400'
        : 'text-sky-400',
  )
</script>

<div class="mx-auto w-full max-w-6xl p-6">
  <div class="flex items-start justify-between">
    <div>
      <h1 class="text-lg font-semibold">{t('nodes.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('nodes.subtitle')}</p>
    </div>
    <button
      class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
      disabled={busy}
      onclick={onRefresh}
    >
      {busy ? t('common.refreshing') : t('common.refresh')}
    </button>
  </div>

  {#if nodes.length === 0}
    <div class="mt-16 text-center text-sm text-zinc-500">
      {busy ? t('nodes.loading') : t('nodes.empty')}
    </div>
  {:else}
    <div class="mt-6 overflow-x-auto rounded border border-zinc-800 bg-zinc-900/40">
      <table class="w-full min-w-[880px] text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[10px] text-zinc-500 uppercase">
            <th class="px-3 py-2 font-medium">{t('nodes.col.node')}</th>
            <th class="px-3 py-2 font-medium">{t('nodes.col.cpu')}</th>
            <th class="px-3 py-2 font-medium">{t('nodes.col.mem')}</th>
            <th class="px-3 py-2 font-medium">{t('nodes.col.disk')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('nodes.col.allocs')}</th>
            <th class="px-3 py-2 font-medium">{t('nodes.col.version')}</th>
          </tr>
        </thead>
        <tbody>
          {#each nodes as n (n.id)}
            {@const nl = nodeLoad(n)}
            {@const cpu = barValues(n, 'cpu')}
            {@const mem = barValues(n, 'memory')}
            {@const disk = barValues(n, 'disk')}
            {@const cpuU = utilization(cpu.used, cpu.capacity)}
            {@const memU = utilization(mem.used, mem.capacity)}
            <tr class="border-b border-zinc-800/60 hover:bg-zinc-800/30">
              <td class="max-w-52 cursor-pointer px-3 py-2" onclick={() => onSelect(n.id)}>
                <div class="flex items-center gap-2">
                  <span class={`h-2 w-2 shrink-0 rounded-full ${statusDot(n.status)}`}></span>
                  <div class="min-w-0">
                    <div class="truncate font-medium text-zinc-200">{n.name || n.id}</div>
                    <div class="truncate font-mono text-[10px] text-zinc-600">
                      {n.id.slice(0, 8)}
                    </div>
                  </div>
                </div>
                <div class="mt-1 text-[10px] text-zinc-600">
                  {n.datacenter}{n.class ? ` · ${n.class}` : ''}
                </div>
              </td>
              <td class="cursor-pointer px-3 py-2" onclick={() => onSelect(n.id)}>
                <div class="flex items-center gap-2">
                  <LoadBar capacity={cpu.capacity} allocated={cpu.allocated} used={cpu.used} />
                  <span class="w-14 shrink-0 text-right text-[10px] text-zinc-400 tabular-nums">
                    {cpu.used > 0 ? `${Math.round((cpuU ?? 0) * 100)}%` : t('common.na')}
                  </span>
                </div>
                <Sparkline
                  values={(nl?.samples ?? []).map((s) => s.cpu)}
                  color={cpuColor(cpuU)}
                  width={96}
                  height={20}
                  label={t('nodes.sparkCpuHistory', {
                    used: formatMHz(cpu.used),
                    cap: formatMHz(cpu.capacity),
                  })}
                />
              </td>
              <td class="cursor-pointer px-3 py-2" onclick={() => onSelect(n.id)}>
                <div class="flex items-center gap-2">
                  <LoadBar capacity={mem.capacity} allocated={mem.allocated} used={mem.used} />
                  <span class="w-14 shrink-0 text-right text-[10px] text-zinc-400 tabular-nums">
                    {mem.used > 0 ? `${Math.round((memU ?? 0) * 100)}%` : t('common.na')}
                  </span>
                </div>
                <Sparkline
                  values={(nl?.samples ?? []).map((s) => s.memory)}
                  color="text-amber-400"
                  width={96}
                  height={20}
                  label={t('nodes.sparkMemHistory', {
                    used: formatMB(mem.used),
                    cap: formatMB(mem.capacity),
                  })}
                />
              </td>
              <td class="cursor-pointer px-3 py-2" onclick={() => onSelect(n.id)}>
                <LoadBar capacity={disk.capacity} allocated={disk.allocated} used={disk.used} />
                <div class="mt-0.5 text-[10px] text-zinc-500 tabular-nums">
                  {formatMB(disk.used)} / {formatMB(disk.capacity)}
                </div>
              </td>
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-zinc-300 tabular-nums"
                onclick={() => onSelect(n.id)}>{n.runningAllocs}</td
              >
              <td
                class="cursor-pointer px-3 py-2 font-mono text-zinc-500"
                onclick={() => onSelect(n.id)}>{n.version}</td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
