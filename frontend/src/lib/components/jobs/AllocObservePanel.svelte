<script lang="ts">
  // 分配观察面板：任务事件 + stdout/stderr 快照。
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    alloc,
    clusterID,
    onClose,
    onLoadEvents,
    onLoadLogs,
  }: {
    alloc: nomad.AllocSummary
    clusterID: string
    onClose: () => void
    onLoadEvents: (allocID: string) => Promise<nomad.AllocTaskEvent[]>
    onLoadLogs: (
      allocID: string,
      task: string,
      logType: string,
    ) => Promise<nomad.AllocLogsResult | null>
  } = $props()

  let tab = $state<'events' | 'stdout' | 'stderr'>('events')
  let events = $state<nomad.AllocTaskEvent[]>([])
  let logs = $state<nomad.AllocLogsResult | null>(null)
  let loading = $state(false)
  let task = $state('')
  let error = $state('')

  async function refresh(): Promise<void> {
    loading = true
    error = ''
    try {
      if (tab === 'events') {
        events = await onLoadEvents(alloc.id)
        if (!task && events.length > 0) task = events[0]!.task
      } else {
        const res = await onLoadLogs(alloc.id, task, tab)
        logs = res
        if (res?.task) task = res.task
      }
    } catch (err) {
      error = `${err}`
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void alloc.id
    void tab
    void clusterID
    void refresh()
  })
</script>

<div class="fixed inset-0 z-50 flex items-stretch justify-end">
  <button
    class="absolute inset-0 cursor-default border-none bg-black/50 p-0"
    aria-label={t('common.cancel')}
    onclick={onClose}
  ></button>
  <div
    class="relative z-10 flex h-full w-full max-w-xl flex-col border-l border-zinc-700 bg-zinc-950 shadow-2xl"
  >
    <div class="flex items-start justify-between gap-3 border-b border-zinc-800 px-4 py-3">
      <div>
        <div class="text-sm font-semibold text-zinc-100">{t('observe.title')}</div>
        <div class="mt-0.5 font-mono text-[11px] text-zinc-500">
          {alloc.id.slice(0, 8)} · {alloc.clientStatus} · {alloc.taskGroup}
        </div>
      </div>
      <button
        class="rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-400 hover:bg-zinc-800"
        onclick={onClose}
      >
        {t('common.cancel')}
      </button>
    </div>

    <div class="flex items-center gap-1 border-b border-zinc-800 px-3 py-2">
      {#each ['events', 'stdout', 'stderr'] as id (id)}
        <button
          class="rounded px-2.5 py-1 text-[11px] {tab === id
            ? 'bg-zinc-100 font-medium text-zinc-900'
            : 'text-zinc-400 hover:bg-zinc-800'}"
          onclick={() => (tab = id as 'events' | 'stdout' | 'stderr')}
        >
          {id === 'events' ? t('observe.tab.events') : id}
        </button>
      {/each}
      <div class="flex-1"></div>
      <button
        class="rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-400 hover:bg-zinc-800 disabled:opacity-50"
        disabled={loading}
        onclick={() => void refresh()}
      >
        {loading ? t('common.refreshing') : t('common.refresh')}
      </button>
    </div>

    {#if error}
      <pre
        class="m-3 rounded border border-red-800 bg-red-950/50 p-2 text-[11px] text-red-300">{error}</pre>
    {/if}

    <div class="min-h-0 flex-1 overflow-auto p-3">
      {#if tab === 'events'}
        {#if events.length === 0}
          <p class="text-xs text-zinc-600">{t('observe.noEvents')}</p>
        {:else}
          <ul class="space-y-2">
            {#each events as ev, i (i)}
              <li
                class="rounded border border-zinc-800 bg-zinc-900/50 px-3 py-2 text-[11px] {ev.fails
                  ? 'border-red-800/60'
                  : ''}"
              >
                <div class="flex items-center justify-between gap-2">
                  <span class="font-medium text-zinc-200">{ev.type}</span>
                  <span class="font-mono text-zinc-600"
                    >{ev.time ? new Date(ev.time).toLocaleTimeString() : ''}</span
                  >
                </div>
                <div class="mt-0.5 text-zinc-500">{ev.task}</div>
                <div class="mt-1 whitespace-pre-wrap text-zinc-300">{ev.message}</div>
              </li>
            {/each}
          </ul>
        {/if}
      {:else if logs}
        <div class="mb-2 flex items-center justify-between text-[11px] text-zinc-500">
          <span class="font-mono">{logs.task} · {logs.logType}</span>
          {#if logs.truncated}
            <span>{t('observe.truncated')}</span>
          {/if}
        </div>
        <pre
          class="rounded border border-zinc-800 bg-black/40 p-3 font-mono text-[11px] leading-4 whitespace-pre-wrap text-zinc-300">{logs.content ||
            t('observe.emptyLogs')}</pre>
      {:else if !loading}
        <p class="text-xs text-zinc-600">{t('observe.emptyLogs')}</p>
      {/if}
    </div>
  </div>
</div>
