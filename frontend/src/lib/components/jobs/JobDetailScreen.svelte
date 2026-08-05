<script lang="ts">
  // JobDetailScreen：job 元信息 + task group + allocs（日志/重启/停止）+ 部署进度。
  // 写操作统一 ConfirmDialog 二次确认 + busyOp 串行（ADR-13）。
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import DeployProgress from './DeployProgress.svelte'
  import AllocObservePanel from './AllocObservePanel.svelte'
  import { t, statusLabel } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    detail,
    allocs,
    loading,
    busyOp,
    clusterID,
    deployEvalID = '',
    deployWarnings = '',
    showDeployProgress = false,
    onBack,
    onRefresh,
    onEvaluate,
    onStop,
    onScale,
    onRestartAlloc,
    onStopAlloc,
    onFetchEval,
    onLoadEvents,
    onLoadLogs,
  }: {
    detail: nomad.JobDetail | null
    allocs: nomad.AllocSummary[]
    loading: boolean
    busyOp: string | null
    clusterID: string
    deployEvalID?: string
    deployWarnings?: string
    showDeployProgress?: boolean
    onBack: () => void
    onRefresh: () => void | Promise<void>
    onEvaluate: (jobID: string) => Promise<unknown>
    onStop: (jobID: string, purge: boolean) => Promise<unknown>
    onScale: (jobID: string, group: string, count: number) => Promise<unknown>
    onRestartAlloc: (allocID: string, taskName: string) => Promise<unknown>
    onStopAlloc: (allocID: string) => Promise<unknown>
    onFetchEval: (evalID: string) => Promise<nomad.EvalInfo | null>
    onLoadEvents: (allocID: string) => Promise<nomad.AllocTaskEvent[]>
    onLoadLogs: (
      allocID: string,
      task: string,
      logType: string,
    ) => Promise<nomad.AllocLogsResult | null>
  } = $props()

  let observeAlloc = $state<nomad.AllocSummary | null>(null)
  let scaleError = $state('')

  type PendingAction =
    | { kind: 'evaluate'; jobID: string }
    | { kind: 'stop'; jobID: string; purge?: boolean }
    | { kind: 'scale'; jobID: string; group: string; count: number }
    | { kind: 'restart'; allocID: string }
    | { kind: 'stopAlloc'; allocID: string }

  let pending = $state<PendingAction | null>(null)

  const scales = $state<Record<string, string>>({})

  // detail 变化时用期望 count 初始化 Scale 输入框。
  $effect(() => {
    for (const tg of detail?.taskGroups ?? []) {
      if (scales[tg.name] === undefined) scales[tg.name] = String(tg.count)
    }
  })

  function statusClass(status: string): string {
    switch (status) {
      case 'running':
        return 'bg-sky-500/15 text-sky-300 border-sky-500/30'
      case 'dead':
      case 'stopped':
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
      case 'failed':
        return 'bg-red-500/15 text-red-300 border-red-500/30'
      default:
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
    }
  }

  function allocStatusClass(status: string): string {
    switch (status) {
      case 'running':
        return 'bg-sky-500/15 text-sky-300 border-sky-500/30'
      case 'complete':
        return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30'
      case 'failed':
        return 'bg-red-500/15 text-red-300 border-red-500/30'
      default:
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
    }
  }

  function confirmMeta() {
    if (!pending)
      return {
        title: '',
        message: '',
        confirmLabel: t('common.confirm'),
        danger: false,
        busy: false,
      }
    switch (pending.kind) {
      case 'evaluate':
        return {
          title: t('jobDetail.confirmEvaluateTitle'),
          message: t('jobDetail.confirmEvaluateBody', { jobID: pending.jobID ?? '' }),
          confirmLabel: t('jobDetail.evaluate'),
          danger: false,
          busy: busyOp === `evaluate:${pending.jobID}`,
        }
      case 'stop':
        return {
          title: t('jobDetail.confirmStopTitle'),
          message: t('jobDetail.confirmStopBody', {
            jobID: pending.jobID ?? '',
            purge: pending.purge ? t('jobDetail.confirmStopPurgeWarn') : '',
          }),
          confirmLabel: t('jobDetail.stop'),
          danger: true,
          busy: busyOp === `stop:${pending.jobID}`,
        }
      case 'scale':
        return {
          title: t('jobDetail.confirmScaleTitle', { group: pending.group ?? '' }),
          message:
            t('jobDetail.confirmScaleBody', {
              jobID: pending.jobID ?? '',
              group: pending.group ?? '',
              count: String(pending.count ?? 0),
            }) + ((pending.count ?? 0) === 0 ? '\n\n' + t('jobDetail.confirmScaleZeroWarn') : ''),
          confirmLabel: t('jobDetail.scale'),
          danger: (pending.count ?? 0) === 0,
          busy: busyOp === `scale:${pending.jobID}`,
        }
      case 'restart':
        return {
          title: t('jobDetail.confirmRestartTitle'),
          message: t('jobDetail.confirmRestartBody', { id: pending.allocID?.slice(0, 8) ?? '' }),
          confirmLabel: t('jobDetail.restart'),
          danger: false,
          busy: busyOp === `restart:${pending.allocID}`,
        }
      case 'stopAlloc':
        return {
          title: t('jobDetail.confirmStopAllocTitle'),
          message: t('jobDetail.confirmStopAllocBody', {
            id: pending.allocID?.slice(0, 8) ?? '',
          }),
          confirmLabel: t('jobDetail.stop'),
          danger: true,
          busy: busyOp === `stopAlloc:${pending.allocID}`,
        }
      default:
        return {
          title: '',
          message: '',
          confirmLabel: t('common.confirm'),
          danger: false,
          busy: false,
        }
    }
  }

  async function confirm(): Promise<void> {
    if (!pending) return
    const p = pending
    try {
      switch (p.kind) {
        case 'evaluate':
          await onEvaluate(p.jobID)
          break
        case 'stop':
          await onStop(p.jobID, p.purge ?? false)
          break
        case 'scale':
          await onScale(p.jobID, p.group, p.count)
          break
        case 'restart':
          await onRestartAlloc(p.allocID, '')
          break
        case 'stopAlloc':
          await onStopAlloc(p.allocID)
          break
      }
    } finally {
      pending = null
    }
  }
</script>

<div class="mx-auto w-full max-w-5xl p-6">
  <div class="flex items-start justify-between">
    <div>
      <button
        class="mb-3 rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-400 hover:bg-zinc-800"
        onclick={onBack}
      >
        {t('jobDetail.back')}
      </button>
      {#if detail}
        <div class="flex items-center gap-2">
          <span
            class="rounded-full border px-2 py-0.5 text-[10px] font-medium {statusClass(
              detail.status,
            )}">{statusLabel(detail.status)}</span
          >
          <h1 class="text-lg font-semibold">{detail.name || detail.id}</h1>
        </div>
        <div class="mt-1 font-mono text-xs text-zinc-500">
          {detail.id} · {detail.namespace || 'default'} · v{detail.createIndex}
        </div>
        <div class="mt-1 text-xs text-zinc-500">
          {detail.type} · priority {detail.priority} · {detail.datacenters.join(', ')}
        </div>
      {:else}
        <h1 class="text-lg font-semibold">{t('jobDetail.notFound')}</h1>
        <p class="mt-2 font-mono text-sm text-zinc-400">{loading ? t('jobDetail.loading') : ''}</p>
      {/if}
    </div>
    <div class="flex items-center gap-2">
      {#if detail}
        <button
          class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
          disabled={busyOp !== null}
          onclick={() => (pending = { kind: 'evaluate', jobID: detail.id })}
        >
          {t('jobDetail.evaluate')}
        </button>
        <button
          class="rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-500 disabled:opacity-50"
          disabled={busyOp !== null}
          onclick={() => (pending = { kind: 'stop', jobID: detail.id, purge: false })}
        >
          {t('jobDetail.stop')}
        </button>
        <button
          class="rounded border border-red-700 px-3 py-1.5 text-xs text-red-300 hover:bg-red-900/50 disabled:opacity-50"
          disabled={busyOp !== null}
          onclick={() => (pending = { kind: 'stop', jobID: detail.id, purge: true })}
        >
          {t('jobDetail.stopPurge')}
        </button>
        <button
          class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
          disabled={loading}
          onclick={onRefresh}
        >
          {loading ? t('jobDetail.refreshing') : t('jobDetail.refresh')}
        </button>
      {/if}
    </div>
  </div>

  {#if detail && showDeployProgress}
    <DeployProgress
      {detail}
      {allocs}
      evalID={deployEvalID}
      warnings={deployWarnings}
      {clusterID}
      {onRefresh}
      {onFetchEval}
    />
  {/if}

  {#if detail}
    <div class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each detail.taskGroups as tg (tg.name)}
        <div class="rounded border border-zinc-800 bg-zinc-900/50 p-4">
          <div class="flex items-center justify-between">
            <div class="text-sm font-medium text-zinc-200">{tg.name}</div>
            <div class="text-[11px] text-zinc-500">
              {t('jobDetail.desired')} <span class="font-mono text-zinc-300">{tg.count}</span>
            </div>
          </div>
          <div class="mt-3 grid grid-cols-5 gap-2 text-center">
            <div>
              <div class="font-mono text-sm text-sky-300 tabular-nums">{tg.running}</div>
              <div class="text-[10px] text-zinc-600">{t('jobDetail.running')}</div>
            </div>
            <div>
              <div class="font-mono text-sm text-zinc-300 tabular-nums">{tg.queued}</div>
              <div class="text-[10px] text-zinc-600">{t('jobDetail.queued')}</div>
            </div>
            <div>
              <div class="font-mono text-sm text-amber-300 tabular-nums">{tg.pending}</div>
              <div class="text-[10px] text-zinc-600">{t('jobDetail.pending')}</div>
            </div>
            <div>
              <div class="font-mono text-sm text-red-300 tabular-nums">{tg.failed}</div>
              <div class="text-[10px] text-zinc-600">{t('jobDetail.failed')}</div>
            </div>
            <div>
              <div class="font-mono text-sm text-zinc-400 tabular-nums">{tg.complete}</div>
              <div class="text-[10px] text-zinc-600">{t('jobDetail.complete')}</div>
            </div>
          </div>
          <div class="mt-3 flex flex-col gap-1">
            <div class="flex items-center gap-2">
              <input
                type="number"
                min="0"
                class="w-20 rounded border bg-zinc-950 px-2 py-1 text-right font-mono text-xs text-zinc-200 outline-none focus:border-sky-500 {scaleError
                  ? 'border-red-700'
                  : 'border-zinc-700'}"
                bind:value={scales[tg.name]}
                placeholder={String(tg.count)}
                oninput={() => (scaleError = '')}
              />
              <button
                class="rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
                disabled={busyOp !== null}
                onclick={() => {
                  const raw = scales[tg.name]
                  const n = Number(raw)
                  if (raw === '' || Number.isNaN(n) || n < 0 || !Number.isInteger(n)) {
                    scaleError = t('jobDetail.errScale')
                    return
                  }
                  scaleError = ''
                  pending = { kind: 'scale', jobID: detail.id, group: tg.name, count: n }
                }}
              >
                {t('jobDetail.scale')}
              </button>
            </div>
            <span class="text-[10px] text-zinc-600">{t('jobDetail.hintScale')}</span>
            {#if scaleError}
              <span class="text-[10px] text-red-400">{scaleError}</span>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    <div class="mt-6 flex items-center justify-between">
      <h2 class="text-sm font-semibold text-zinc-300">{t('jobDetail.allocations')}</h2>
      <span class="text-[11px] text-zinc-600">{allocs.length}</span>
    </div>
    <div class="mt-2 overflow-x-auto rounded border border-zinc-800 bg-zinc-900/40">
      <table class="w-full min-w-[720px] text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[10px] text-zinc-500 uppercase">
            <th class="px-3 py-2 font-medium">{t('jobDetail.col.alloc')}</th>
            <th class="px-3 py-2 font-medium">{t('jobDetail.col.status')}</th>
            <th class="px-3 py-2 font-medium">{t('jobDetail.col.group')}</th>
            <th class="px-3 py-2 font-medium">{t('jobDetail.col.node')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('nodes.col.cpu')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('nodes.col.mem')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobDetail.col.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each allocs as a (a.id)}
            <tr class="border-b border-zinc-800/60 last:border-0 hover:bg-zinc-800/20">
              <td class="px-3 py-2">
                <div class="font-mono text-zinc-200">{a.id.slice(0, 8)}</div>
                <div class="text-[10px] text-zinc-600">{a.desiredStatus}</div>
              </td>
              <td class="px-3 py-2">
                <span
                  class="inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium {allocStatusClass(
                    a.status,
                  )}">{statusLabel(a.status)}</span
                >
              </td>
              <td class="px-3 py-2 font-mono text-zinc-300">{a.taskGroup}</td>
              <td class="px-3 py-2 font-mono text-[11px] text-zinc-500"
                >{a.nodeName || a.nodeID.slice(0, 8)}</td
              >
              <td class="px-3 py-2 text-right font-mono text-zinc-400 tabular-nums"
                >{Math.round(a.cpu)}</td
              >
              <td class="px-3 py-2 text-right font-mono text-zinc-400 tabular-nums"
                >{Math.round(a.memory)}</td
              >
              <td class="px-3 py-2">
                <div class="flex justify-end gap-1">
                  <button
                    class="rounded border border-zinc-700 px-1.5 py-0.5 text-[10px] text-zinc-400 hover:bg-zinc-800"
                    title={t('observe.open')}
                    onclick={() => (observeAlloc = a)}
                  >
                    {t('observe.open')}
                  </button>
                  <button
                    class="rounded border border-zinc-700 px-1.5 py-0.5 text-[10px] text-zinc-400 hover:bg-zinc-800 disabled:opacity-50"
                    title={t('jobDetail.restartTitle')}
                    disabled={busyOp !== null}
                    onclick={() => (pending = { kind: 'restart', allocID: a.id })}
                  >
                    {t('jobDetail.restart')}
                  </button>
                  <button
                    class="rounded border border-red-800 px-1.5 py-0.5 text-[10px] text-red-300 hover:bg-red-900/50 disabled:opacity-50"
                    title={t('jobDetail.stopAllocTitle')}
                    disabled={busyOp !== null}
                    onclick={() => (pending = { kind: 'stopAlloc', allocID: a.id })}
                  >
                    {t('jobDetail.stop')}
                  </button>
                </div>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="7" class="px-3 py-8 text-center text-zinc-600">
                {loading ? t('jobDetail.loadingAllocs') : t('jobDetail.noAllocs')}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if pending}
    {@const meta = confirmMeta()}
    <ConfirmDialog
      title={meta.title}
      message={meta.message}
      confirmLabel={meta.confirmLabel}
      danger={meta.danger}
      busy={meta.busy}
      confirmPhrase={pending.kind === 'stop' && pending.purge ? (pending.jobID ?? '') : ''}
      onConfirm={() => void confirm()}
      onCancel={() => {
        if (meta.busy) return
        pending = null
      }}
    />
  {/if}

  {#if observeAlloc}
    <AllocObservePanel
      alloc={observeAlloc}
      {clusterID}
      onClose={() => (observeAlloc = null)}
      {onLoadEvents}
      {onLoadLogs}
    />
  {/if}
</div>
