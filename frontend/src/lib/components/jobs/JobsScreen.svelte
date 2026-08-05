<script lang="ts">
  // JobsScreen：分页列表（默认 10/页）+ 本页全选 + 批量 purge 删除。
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import { t, statusLabel } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  const PAGE_SIZE = 10

  let {
    jobs,
    loading,
    busy = false,
    onRefresh,
    onSelect,
    onDeleteMany,
  }: {
    jobs: nomad.JobSummary[]
    loading: boolean
    busy?: boolean
    onRefresh: () => void
    onSelect: (jobID: string) => void
    onDeleteMany: (jobIDs: string[]) => Promise<{ ok: number; failed: string[] }>
  } = $props()

  let filter = $state('')
  let page = $state(1)
  /** 选中的 job ID 列表（用数组避免 Set 的 reactivity lint） */
  let selected = $state<string[]>([])
  let confirmOpen = $state(false)
  let pageSelectEl: HTMLInputElement | undefined = $state()

  const filtered = $derived(
    filter.trim()
      ? jobs.filter((j) => j.id.toLowerCase().includes(filter.trim().toLowerCase()))
      : jobs,
  )

  const totalPages = $derived(Math.max(1, Math.ceil(filtered.length / PAGE_SIZE)))

  const pageJobs = $derived.by(() => {
    const p = Math.min(Math.max(1, page), totalPages)
    const start = (p - 1) * PAGE_SIZE
    return filtered.slice(start, start + PAGE_SIZE)
  })

  const pageIDs = $derived(pageJobs.map((j) => j.id))

  const selectedOnPage = $derived(pageIDs.filter((id) => selected.includes(id)))
  const allPageSelected = $derived(
    pageIDs.length > 0 && pageIDs.every((id) => selected.includes(id)),
  )
  const somePageSelected = $derived(
    selectedOnPage.length > 0 && selectedOnPage.length < pageIDs.length,
  )

  $effect(() => {
    void filter
    page = 1
    selected = []
  })

  $effect(() => {
    if (page > totalPages) {
      page = totalPages
      selected = []
    }
  })

  // 列表收缩时丢掉已不存在的选中项（仅在确有失效 ID 时写回，避免无意义更新）
  $effect(() => {
    const alive = filtered.map((j) => j.id)
    const next = selected.filter((id) => alive.includes(id))
    if (next.length !== selected.length) selected = next
  })

  $effect(() => {
    if (pageSelectEl) pageSelectEl.indeterminate = somePageSelected
  })

  function toggleOne(id: string, checked: boolean): void {
    if (checked) {
      if (!selected.includes(id)) selected = [...selected, id]
    } else {
      selected = selected.filter((x) => x !== id)
    }
  }

  function togglePage(checked: boolean): void {
    if (checked) {
      const next = [...selected]
      for (const id of pageIDs) {
        if (!next.includes(id)) next.push(id)
      }
      selected = next
    } else {
      selected = selected.filter((id) => !pageIDs.includes(id))
    }
  }

  function goPage(p: number): void {
    const next = Math.min(Math.max(1, p), totalPages)
    if (next === page) return
    page = next
    selected = []
  }

  async function doDelete(): Promise<void> {
    const ids = [...selected]
    if (ids.length === 0) return
    confirmOpen = false
    const result = await onDeleteMany(ids)
    // 仅保留失败项；busy 早退时 failed=全部，选择不丢
    selected = result.failed
  }

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
  <div class="flex items-start justify-between gap-4">
    <div>
      <h1 class="text-lg font-semibold">{t('jobs.title')}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('jobs.subtitle')}</p>
    </div>
    <div class="flex flex-wrap items-center justify-end gap-2">
      <input
        class="w-52 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-sky-500"
        placeholder={t('jobs.filterPlaceholder')}
        bind:value={filter}
      />
      {#if selected.length > 0}
        <button
          class="rounded border border-red-800/60 bg-red-950/40 px-3 py-1.5 text-xs text-red-300 hover:bg-red-950/70 disabled:opacity-50"
          disabled={busy || loading}
          onclick={() => (confirmOpen = true)}
        >
          {t('jobs.deleteSelected', { count: String(selected.length) })}
        </button>
      {/if}
      <button
        class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
        disabled={loading || busy}
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
      <table class="w-full min-w-[800px] text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[10px] text-zinc-500 uppercase">
            <th class="w-10 px-3 py-2">
              <input
                bind:this={pageSelectEl}
                type="checkbox"
                class="accent-sky-500"
                checked={allPageSelected}
                disabled={busy}
                aria-label={t('jobs.selectPage')}
                onchange={(e) => togglePage(e.currentTarget.checked)}
              />
            </th>
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
          {#each pageJobs as j (j.id)}
            <tr class="border-b border-zinc-800/60 hover:bg-zinc-800/30">
              <td class="px-3 py-2">
                <input
                  type="checkbox"
                  class="accent-sky-500"
                  checked={selected.includes(j.id)}
                  disabled={busy}
                  aria-label={j.id}
                  onclick={(e) => e.stopPropagation()}
                  onchange={(e) => toggleOne(j.id, e.currentTarget.checked)}
                />
              </td>
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

    <div class="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-zinc-500">
      <div>
        {t('jobs.pageSummary', {
          from: String((Math.min(page, totalPages) - 1) * PAGE_SIZE + 1),
          to: String(Math.min(page * PAGE_SIZE, filtered.length)),
          total: String(filtered.length),
          size: String(PAGE_SIZE),
        })}
      </div>
      <div class="flex items-center gap-2">
        <button
          class="rounded border border-zinc-700 px-2.5 py-1 text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
          disabled={page <= 1 || busy}
          onclick={() => goPage(page - 1)}
        >
          {t('jobs.prevPage')}
        </button>
        <span class="tabular-nums text-zinc-400">
          {t('jobs.pageOf', {
            page: String(Math.min(page, totalPages)),
            pages: String(totalPages),
          })}
        </span>
        <button
          class="rounded border border-zinc-700 px-2.5 py-1 text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
          disabled={page >= totalPages || busy}
          onclick={() => goPage(page + 1)}
        >
          {t('jobs.nextPage')}
        </button>
      </div>
    </div>
  {/if}
</div>

{#if confirmOpen}
  <ConfirmDialog
    title={t('jobs.confirmDeleteTitle')}
    message={t('jobs.confirmDeleteBody', { count: String(selected.length) })}
    confirmLabel={t('jobs.delete')}
    danger
    {busy}
    confirmPhrase="DELETE"
    onConfirm={() => void doDelete()}
    onCancel={() => {
      if (busy) return
      confirmOpen = false
    }}
  />
{/if}
