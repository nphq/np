<script lang="ts">
  // NodeDetailScreen 是节点详情＼�：HostStats 曲线 + 该节点 alloc 列表。
  // 数据 = NodeSummary（ListNodes）+ NodeLoad（loads store）+ AllocLoad（loads store，按 nodeID 过滤）。
  import LoadBar from '../load/LoadBar.svelte'
  import Sparkline from '../load/Sparkline.svelte'
  import { formatMHz, formatMB, level, utilization } from '../../utils/format'
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    node,
    load,
    allocs,
    onBack,
  }: {
    node: nomad.NodeSummary | null
    load: nomad.NodeLoad | null
    allocs: nomad.AllocLoad[]
    onBack: () => void
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

  const capacity = $derived(
    load
      ? load.capacity
      : node
        ? { cpu: node.cpuTotal, memory: node.memoryTotal, disk: node.diskTotal }
        : { cpu: 0, memory: 0, disk: 0 },
  )
  const used = $derived(
    load
      ? load.used
      : node
        ? { cpu: node.cpu, memory: node.memory, disk: node.disk }
        : { cpu: 0, memory: 0, disk: 0 },
  )
  const allocated = $derived(load ? load.allocated : { cpu: 0, memory: 0, disk: 0 })

  const cpuU = $derived(utilization(used.cpu, capacity.cpu))
  const memU = $derived(utilization(used.memory, capacity.memory))
  const diskU = $derived(utilization(used.disk, capacity.disk))

  const samples = $derived(load?.samples ?? [])

  // allocTotals 汇总 per-task 用量为单行总量。
  function allocTotals(a: nomad.AllocLoad): { cpu: number; memory: number } {
    let cpu = 0
    let memory = 0
    for (const t of Object.values(a.tasks)) {
      cpu += t.cpu
      memory += t.memory
    }
    return { cpu, memory }
  }
</script>

{#if !node}
  <div class="mx-auto w-full max-w-4xl p-6 text-sm text-zinc-500">{t('nodes.notFound')}</div>
{:else}
  <div class="mx-auto w-full max-w-4xl p-6">
    <div class="flex items-start justify-between">
      <div>
        <button
          class="mb-3 rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-400 hover:bg-zinc-800"
          onclick={onBack}
        >
          {t('nodes.back')}
        </button>
        <div class="flex items-center gap-2">
          <span class={`h-2.5 w-2.5 rounded-full ${statusDot(node.status)}`}></span>
          <h1 class="text-lg font-semibold">{node.name || node.id}</h1>
        </div>
        <div class="mt-1 font-mono text-xs text-zinc-500">{node.id}</div>
        <div class="mt-1 text-xs text-zinc-500">
          {node.datacenter}{node.class ? ` · ${node.class}` : ''} · {node.status} · v{node.version}
        </div>
      </div>
    </div>

    <div class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-3">
      <div class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
        <div class="text-[11px] text-zinc-500 uppercase">{t('nodes.col.cpu')}</div>
        <div class="mt-2 flex items-center gap-2">
          <LoadBar capacity={capacity.cpu} allocated={allocated.cpu} used={used.cpu} />
        </div>
        <div class="mt-1 text-[10px] text-zinc-500 tabular-nums">
          {formatMHz(used.cpu)} / {formatMHz(capacity.cpu)} · {cpuU === null
            ? t('common.na')
            : `${Math.round(cpuU * 100)}%`}
        </div>
        <Sparkline
          values={samples.map((s) => s.cpu)}
          color={level(cpuU) === 'crit'
            ? 'text-red-500'
            : level(cpuU) === 'warn'
              ? 'text-amber-400'
              : 'text-sky-400'}
          width={160}
          height={28}
          label={t('nodes.sparkCpuHistory', {
            used: formatMHz(used.cpu),
            cap: formatMHz(capacity.cpu),
          })}
        />
      </div>
      <div class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
        <div class="text-[11px] text-zinc-500 uppercase">{t('nodes.col.mem')}</div>
        <div class="mt-2 flex items-center gap-2">
          <LoadBar capacity={capacity.memory} allocated={allocated.memory} used={used.memory} />
        </div>
        <div class="mt-1 text-[10px] text-zinc-500 tabular-nums">
          {formatMB(used.memory)} / {formatMB(capacity.memory)} · {memU === null
            ? t('common.na')
            : `${Math.round(memU * 100)}%`}
        </div>
        <Sparkline
          values={samples.map((s) => s.memory)}
          color="text-amber-400"
          width={160}
          height={28}
          label={t('nodes.sparkMemHistory', {
            used: formatMB(used.memory),
            cap: formatMB(capacity.memory),
          })}
        />
      </div>
      <div class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
        <div class="text-[11px] text-zinc-500 uppercase">{t('nodes.col.disk')}</div>
        <div class="mt-2 flex items-center gap-2">
          <LoadBar capacity={capacity.disk} used={used.disk} />
        </div>
        <div class="mt-1 text-[10px] text-zinc-500 tabular-nums">
          {formatMB(used.disk)} / {formatMB(capacity.disk)} · {diskU === null
            ? t('common.na')
            : `${Math.round(diskU * 100)}%`}
        </div>
        <div class="mt-3 text-[11px] text-zinc-500 uppercase">{t('nodes.runningAllocs')}</div>
        <div class="mt-1 text-xl font-semibold tabular-nums">
          {load?.runningAllocs ?? node.runningAllocs}
        </div>
      </div>
    </div>

    <div class="mt-6 rounded border border-zinc-800 bg-zinc-900/50">
      <div class="border-b border-zinc-800 px-4 py-2 text-[11px] text-zinc-500 uppercase">
        {t('nodes.allocsOnNode')}
      </div>
      {#if allocs.length === 0}
        <div class="px-4 py-6 text-xs text-zinc-600">
          {t('nodes.noAllocLevel')}
        </div>
      {:else}
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b border-zinc-800 text-[10px] text-zinc-500 uppercase">
              <th class="px-4 py-2 font-medium">{t('jobDetail.col.alloc')}</th>
              <th class="px-4 py-2 font-medium">{t('nodes.col.job')}</th>
              <th class="px-4 py-2 font-medium text-right">{t('nodes.col.cpu')}</th>
              <th class="px-4 py-2 font-medium text-right">{t('nodes.col.mem')}</th>
            </tr>
          </thead>
          <tbody>
            {#each allocs as a (a.allocID)}
              {@const tot = allocTotals(a)}
              <tr class="border-b border-zinc-800/60 last:border-0 hover:bg-zinc-800/30">
                <td class="px-4 py-2 font-mono text-zinc-300">{a.allocID.slice(0, 12)}</td>
                <td class="px-4 py-2 font-mono text-zinc-300">{a.jobID}</td>
                <td class="px-4 py-2 text-right text-sky-300 tabular-nums">{formatMHz(tot.cpu)}</td>
                <td class="px-4 py-2 text-right text-amber-300 tabular-nums"
                  >{formatMB(tot.memory)}</td
                >
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
{/if}
