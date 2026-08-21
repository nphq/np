<script lang="ts">
  // 部署进度：Register 回执 → Eval → Alloc → Running/Failed（轮询详情）。
  import { onDestroy } from 'svelte'
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    detail,
    allocs,
    evalID = '',
    warnings = '',
    clusterID,
    onRefresh,
    onFetchEval,
  }: {
    detail: nomad.JobDetail | null
    allocs: nomad.AllocSummary[]
    evalID?: string
    warnings?: string
    clusterID: string
    onRefresh: () => Promise<void> | void
    onFetchEval: (evalID: string) => Promise<nomad.EvalInfo | null>
  } = $props()

  type Phase = 'submitted' | 'evaluating' | 'placing' | 'running' | 'failed' | 'complete'

  let evalInfo = $state<nomad.EvalInfo | null>(null)
  let polling = $state(true)
  let lastPoll = $state(0)
  let timer: ReturnType<typeof setInterval> | undefined

  const desired = $derived(
    (detail?.taskGroups ?? []).reduce((n, tg) => n + Math.max(0, tg.count), 0),
  )
  const running = $derived((detail?.taskGroups ?? []).reduce((n, tg) => n + tg.running, 0))
  const failed = $derived((detail?.taskGroups ?? []).reduce((n, tg) => n + tg.failed, 0))
  const pending = $derived(
    (detail?.taskGroups ?? []).reduce((n, tg) => n + tg.pending + tg.queued, 0),
  )
  const complete = $derived((detail?.taskGroups ?? []).reduce((n, tg) => n + tg.complete, 0))
  const isBatch = $derived(detail?.type === 'batch' || detail?.type === 'sysbatch')

  const phase = $derived.by((): Phase => {
    const st = (evalInfo?.status ?? '').toLowerCase()
    if (st === 'failed' || (failed > 0 && running === 0 && pending === 0 && allocs.length > 0)) {
      return 'failed'
    }
    if (evalInfo?.failedSummary) {
      // 调度失败：若 Nomad 仍在等容量/自动重试（blocked eval 存在），不是终态 ——
      // 保持 evaluating 让轮询继续，等 blocked eval 落地为 alloc 后转 placing/running。
      if (running === 0 && allocs.length === 0 && !evalInfo.blockedEval) return 'failed'
    }
    if (isBatch && desired > 0 && complete >= desired && failed === 0) return 'complete'
    if (desired > 0 && running >= desired && pending === 0 && failed === 0) return 'running'
    if (detail?.status === 'running' && desired === 0 && running > 0) return 'running'
    if (allocs.length > 0 || pending > 0) return 'placing'
    if (st === 'complete' || st === 'blocked' || st === 'pending' || evalID) return 'evaluating'
    return 'submitted'
  })

  const terminal = $derived(phase === 'running' || phase === 'failed' || phase === 'complete')

  async function tick(): Promise<void> {
    await onRefresh()
    if (evalID) {
      const info = await onFetchEval(evalID)
      if (info) evalInfo = info
    }
    lastPoll = Date.now()
  }

  $effect(() => {
    // 进入/换 job 时启动轮询
    void clusterID
    void detail?.id
    if (timer) clearInterval(timer)
    polling = true
    void tick()
    timer = setInterval(() => {
      if (!polling) return
      if (terminal) {
        polling = false
        return
      }
      void tick()
    }, 2000)
    return () => {
      if (timer) clearInterval(timer)
    }
  })

  onDestroy(() => {
    if (timer) clearInterval(timer)
  })

  $effect(() => {
    if (terminal) polling = false
  })

  function phaseLabel(p: Phase): string {
    switch (p) {
      case 'submitted':
        return t('deploy.phase.submitted')
      case 'evaluating':
        return t('deploy.phase.evaluating')
      case 'placing':
        return t('deploy.phase.placing')
      case 'running':
        return t('deploy.phase.running')
      case 'failed':
        return t('deploy.phase.failed')
      case 'complete':
        return t('deploy.phase.complete')
    }
  }

  const steps: Phase[] = ['submitted', 'evaluating', 'placing', 'running']

  function stepDone(s: Phase): boolean {
    const order: Phase[] = ['submitted', 'evaluating', 'placing', 'running', 'complete']
    const cur = phase === 'failed' ? 'placing' : phase === 'complete' ? 'running' : phase
    return order.indexOf(s) <= order.indexOf(cur)
  }
</script>

<div
  class="mt-4 rounded border p-4 {phase === 'failed'
    ? 'border-red-800 bg-red-950/40'
    : phase === 'running' || phase === 'complete'
      ? 'border-emerald-800/60 bg-emerald-950/20'
      : 'border-sky-800/50 bg-sky-950/20'}"
>
  <div class="flex items-start justify-between gap-3">
    <div>
      <div class="text-[11px] font-semibold tracking-wide text-zinc-400 uppercase">
        {t('deploy.title')}
      </div>
      <div class="mt-1 text-sm font-medium text-zinc-100">{phaseLabel(phase)}</div>
      {#if evalID}
        <div class="mt-1 font-mono text-[11px] text-zinc-500">
          eval {evalID.slice(0, 8)}
          {#if evalInfo?.status}
            · {evalInfo.status}{evalInfo.statusDescription
              ? ` — ${evalInfo.statusDescription}`
              : ''}
          {/if}
        </div>
      {/if}
    </div>
    <div class="text-right text-[11px] text-zinc-500">
      {#if polling}
        {t('deploy.polling')}
      {:else}
        {t('deploy.settled')}
      {/if}
      {#if lastPoll}
        <div class="font-mono text-[10px] text-zinc-600">
          {new Date(lastPoll).toLocaleTimeString()}
        </div>
      {/if}
    </div>
  </div>

  <ol class="mt-4 flex flex-wrap gap-2">
    {#each steps as s (s)}
      <li
        class="rounded-full border px-2.5 py-1 text-[10px] {stepDone(s)
          ? phase === 'failed' && s === 'placing'
            ? 'border-red-500/40 bg-red-500/10 text-red-300'
            : 'border-sky-500/40 bg-sky-500/10 text-sky-300'
          : 'border-zinc-700 text-zinc-600'}"
      >
        {phaseLabel(s)}
      </li>
    {/each}
  </ol>

  <div class="mt-3 grid grid-cols-2 gap-2 text-[11px] sm:grid-cols-4">
    <div class="rounded bg-zinc-950/50 px-2 py-1.5">
      <div class="text-zinc-600">{t('deploy.stat.desired')}</div>
      <div class="font-mono text-zinc-200">{desired}</div>
    </div>
    <div class="rounded bg-zinc-950/50 px-2 py-1.5">
      <div class="text-zinc-600">{t('deploy.stat.running')}</div>
      <div class="font-mono text-sky-300">{running}</div>
    </div>
    <div class="rounded bg-zinc-950/50 px-2 py-1.5">
      <div class="text-zinc-600">{t('deploy.stat.pending')}</div>
      <div class="font-mono text-amber-300">{pending}</div>
    </div>
    <div class="rounded bg-zinc-950/50 px-2 py-1.5">
      <div class="text-zinc-600">{t('deploy.stat.failed')}</div>
      <div class="font-mono text-red-300">{failed}</div>
    </div>
  </div>

  {#if warnings}
    <pre
      class="mt-3 max-h-24 overflow-auto rounded border border-amber-400/20 bg-amber-400/5 p-2 text-[11px] whitespace-pre-wrap text-amber-200/90">{warnings}</pre>
  {/if}
  {#if evalInfo?.failedSummary && phase !== 'running' && phase !== 'complete'}
    <pre
      class="mt-3 max-h-24 overflow-auto rounded border border-red-800 bg-red-950/50 p-2 text-[11px] whitespace-pre-wrap text-red-300/90">{evalInfo.failedSummary}</pre>
    {#if phase === 'failed'}
      <p class="mt-3 text-[11px] text-red-300/80">
        {#if /missing drivers/i.test(evalInfo.failedSummary)}
          {t('deploy.missingDriversHint')}
        {:else if allocs.length === 0}
          {t('deploy.placementHint')}
        {:else}
          {t('deploy.failedHint')}
        {/if}
      </p>
    {:else if phase === 'evaluating'}
      <p class="mt-3 text-[11px] text-amber-300/80">{t('deploy.retryingHint')}</p>
    {/if}
  {/if}
</div>
