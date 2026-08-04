<script lang="ts">
  // OverviewScreen 是集群负载总览页＼�：标题 + 刷新 + 环形图主体。
  import ClusterOverview from './ClusterOverview.svelte'
  import { t } from '../../i18n/index.svelte'
  import type { nomad } from '../../types/wails'

  let {
    clusterName,
    load,
    busy,
    onRefresh,
    onSelectAlloc,
  }: {
    clusterName: string
    load: nomad.ClusterLoad | null
    busy: boolean
    onRefresh: () => void
    onSelectAlloc?: (allocID: string) => void
  } = $props()
</script>

<div class="mx-auto w-full max-w-5xl p-6">
  <div class="flex items-start justify-between">
    <div>
      <h1 class="text-lg font-semibold">{clusterName}</h1>
      <p class="mt-1 text-xs text-zinc-500">{t('overview.polling')}</p>
    </div>
    <button
      class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-50"
      disabled={busy}
      onclick={onRefresh}
    >
      {busy ? t('common.refreshing') : t('common.refresh')}
    </button>
  </div>

  {#if !load}
    <div class="mt-16 text-center text-sm text-zinc-500">
      {busy ? t('overview.collecting') : t('overview.noDataYet')}
    </div>
  {:else}
    <div class="mt-6">
      <ClusterOverview {load} {onSelectAlloc} />
    </div>
  {/if}
</div>
