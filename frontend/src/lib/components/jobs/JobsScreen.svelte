<script lang="ts">
  // JobsScreen 是 job 列表屏＼�：状态徽章 + 计数列 + 行点击进详情。
  // 状态徽章配色对标 Nomad UI：running=sky / queued=pending=amber / failed=red / dead=zinc。
  import { t, statusLabel } from '../../i18n/index.svelte'
  let {
    jobs,
    loading,
    onRefresh,
    onSelect,
  }: {
    jobs: import('../../types/wails').nomad.JobSummary[]
    loading: boolean
    onRefresh: () => void
    onSelect: (jobID: string) => void
  } = $props()

  let filter = $state('')

  const filtered = $derived(
    filter.trim()
      ? jobs.filter((j) => j.id.toLowerCase().includes(filter.trim().toLowerCase()))
      : jobs,
  )

  function statusClass(status: string): string {
    switch (status) {
      case 'running':
        return 'bg-sky-500/15 text-sky-300 border-sky-500/30'
      case 'pending':
        return 'bg-amber-400/15 text-amber-300 border-amber-400/30'
      case 'dead':
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
      case 'stopped':
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
      case 'failed':
        return 'bg-red-500/15 text-red-300 border-red-500/30'
      default:
        return 'bg-zinc-600/20 text-zinc-400 border-zinc-600/30'
    }
  }

  function typeClass(type: string): string {
    switch (type) {
      case 'system':
        return 'text-amber-300'
      case 'batch':
        return 'text-sky-300'
      default:
        return 'text-zinc-300'
    }
  }
</script>

<div class="mx-auto w-full max-w-6xl p-6">
  <div class="flex items-start justify-between">
    <div>
      <h1 class="text-lg font-semibold">{t('jobs.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('jobs.subtitle')}</p>
    </div>
    <div class="flex items-center gap-2">
      <input
        class="w-52 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-sky-500"
        placeholder={t('jobs.filterPlaceholder')}
        bind:value={filter}
      />
      <button
        class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
        disabled={loading}
        onclick={onRefresh}
      >
        {loading ? t('common.refreshing') : t('common.refresh')}
      </button>
    </div>
  </div>

  {#if filtered.length === 0}
    <div class="mt-16 text-center text-sm text-zinc-500">
      {loading ? t('jobs.loading') : filter.trim() ? t('jobs.filterEmpty') : t('jobs.empty')}
    </div>
  {:else}
    <div class="mt-6 overflow-x-auto rounded border border-zinc-800 bg-zinc-900/40">
      <table class="w-full min-w-[760px] text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[10px] text-zinc-500 uppercase">
            <th class="px-3 py-2 font-medium">{t('jobs.col.job')}</th>
            <th class="px-3 py-2 font-medium">{t('jobs.col.status')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobs.col.running')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobs.col.queued')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobs.col.pending')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobs.col.failed')}</th>
            <th class="px-3 py-2 font-medium">{t('jobs.col.type')}</th>
            <th class="px-3 py-2 font-medium text-right">{t('jobs.col.priority')}</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as j (j.id)}
            <tr class="border-b border-zinc-800/60 hover:bg-zinc-800/30">
              <td class="max-w-64 cursor-pointer px-3 py-2" onclick={() => onSelect(j.id)}>
                <div class="truncate font-medium text-zinc-200">{j.name || j.id}</div>
                <div class="truncate font-mono text-[10px] text-zinc-600">{j.id}</div>
              </td>
              <td class="cursor-pointer px-3 py-2" onclick={() => onSelect(j.id)}>
                <span
                  class="inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium {statusClass(
                    j.status,
                  )}">{statusLabel(j.status)}</span
                >
              </td>
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-zinc-300 tabular-nums"
                onclick={() => onSelect(j.id)}>{j.running}</td
              >
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-zinc-400 tabular-nums"
                onclick={() => onSelect(j.id)}>{j.queued}</td
              >
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-zinc-400 tabular-nums"
                onclick={() => onSelect(j.id)}>{j.pending}</td
              >
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-red-300 tabular-nums"
                onclick={() => onSelect(j.id)}>{j.failed}</td
              >
              <td
                class="cursor-pointer px-3 py-2 font-mono text-[10px] {typeClass(j.type)}"
                onclick={() => onSelect(j.id)}>{j.type}</td
              >
              <td
                class="cursor-pointer px-3 py-2 text-right font-mono text-zinc-400 tabular-nums"
                onclick={() => onSelect(j.id)}>{j.priority}</td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
