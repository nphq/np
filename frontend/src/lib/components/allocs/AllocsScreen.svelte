<script lang="ts">
  // AllocsScreen：集群级 allocation 列表（全状态），支持关键字过滤与状态筛选。
  // 行点击跳转对应 job 详情；详细 alloc 操作（日志/事件/重启）在详情页完成。
  import { t, statusLabel } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    allocs,
    loading,
    busy,
    onRefresh,
    onSelect,
  }: {
    allocs: nomad.AllocSummary[]
    loading: boolean
    busy: boolean
    onRefresh: () => void
    onSelect: (jobID: string) => void
  } = $props()

  let query = $state('')
  let status = $state('')

  const statuses = $derived(
    Array.from(new Set(allocs.map((a) => a.clientStatus || 'unknown'))).sort(),
  )

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase()
    return allocs.filter((a) => {
      if (status && a.clientStatus !== status) return false
      if (!q) return true
      return (
        a.id.toLowerCase().includes(q) ||
        a.jobID.toLowerCase().includes(q) ||
        (a.taskGroup || '').toLowerCase().includes(q) ||
        (a.nodeName || '').toLowerCase().includes(q)
      )
    })
  })
</script>

<section class="mx-auto w-full max-w-5xl p-6">
  <div class="mb-4 flex items-center justify-between">
    <h1 class="text-lg font-semibold">{t('nav.allocs')}</h1>
    <button
      class="rounded border border-zinc-700 px-2 py-1 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
      disabled={loading || busy}
      onclick={onRefresh}>{t('common.refresh')}</button
    >
  </div>

  <div class="mb-3 flex items-center gap-2 text-xs">
    <input
      type="search"
      placeholder={t('allocs.search')}
      value={query}
      oninput={(e) => (query = (e.target as HTMLInputElement).value)}
      class="w-64 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-zinc-200 placeholder-zinc-600 focus:border-sky-600 focus:outline-none"
    />
    <select
      class="rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-zinc-300 focus:border-sky-600 focus:outline-none"
      value={status}
      onchange={(e) => (status = (e.target as HTMLSelectElement).value)}
    >
      <option value="">{t('allocs.filterAll')}</option>
      {#each statuses as s (s)}
        <option value={s}>{statusLabel(s)}</option>
      {/each}
    </select>
    <span class="text-zinc-600">
      {filtered.length} / {allocs.length}
    </span>
  </div>

  {#if loading && allocs.length === 0}
    <div class="p-8 text-center text-xs text-zinc-600">{t('common.loading')}</div>
  {:else if filtered.length === 0}
    <div class="p-8 text-center text-xs text-zinc-600">{t('allocs.empty')}</div>
  {:else}
    <div class="overflow-x-auto rounded border border-zinc-800">
      <table class="w-full text-left text-sm">
        <thead class="border-b border-zinc-800 bg-zinc-900/60 text-xs text-zinc-500">
          <tr>
            <th class="px-3 py-2 font-medium">{t('allocs.col.job')}</th>
            <th class="px-3 py-2 font-medium">{t('allocs.col.group')}</th>
            <th class="px-3 py-2 font-medium">{t('allocs.col.node')}</th>
            <th class="px-3 py-2 font-medium">{t('allocs.col.status')}</th>
            <th class="px-3 py-2 font-medium">{t('allocs.col.desired')}</th>
            <th class="px-3 py-2 font-medium">{t('allocs.col.allocID')}</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as a (a.id)}
            <tr
              class="cursor-pointer border-b border-zinc-800/60 last:border-0 hover:bg-zinc-800/40"
              role="button"
              tabindex="0"
              onclick={() => onSelect(a.jobID)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') onSelect(a.jobID)
              }}
            >
              <td class="px-3 py-1.5 font-mono text-sky-300">{a.jobID}</td>
              <td class="px-3 py-1.5 text-zinc-300">{a.taskGroup || '—'}</td>
              <td class="px-3 py-1.5 text-zinc-400">{a.nodeName || a.nodeID?.slice(0, 8) || '—'}</td
              >
              <td class="px-3 py-1.5">
                <span
                  class="rounded px-1.5 py-0.5 text-[10px] font-medium
                  {a.clientStatus === 'running'
                    ? 'bg-emerald-500/15 text-emerald-400'
                    : a.clientStatus === 'complete'
                      ? 'bg-sky-500/15 text-sky-400'
                      : a.clientStatus === 'failed' || a.clientStatus === 'lost'
                        ? 'bg-red-500/15 text-red-400'
                        : 'bg-amber-500/15 text-amber-400'}"
                  >{statusLabel(a.clientStatus || 'unknown')}</span
                >
              </td>
              <td class="px-3 py-1.5 text-zinc-500">{a.desiredStatus || '—'}</td>
              <td class="px-3 py-1.5 font-mono text-[11px] text-zinc-600">
                {a.id.slice(0, 8)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>
