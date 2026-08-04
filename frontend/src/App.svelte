<script lang="ts">
  import { onMount } from 'svelte'
  import { createClustersStore, toastState } from './lib/stores/clusters.svelte'
  import { createUiStore } from './lib/stores/ui.svelte'
  import { createLoadsStore } from './lib/stores/loads.svelte'
  import { createNodesStore } from './lib/stores/nodes.svelte'
  import { createJobsStore } from './lib/stores/jobs.svelte'
  import { i18n, t } from './lib/i18n/index.svelte'
  import AddClusterDialog from './lib/components/AddClusterDialog.svelte'
  import OverviewScreen from './lib/components/load/OverviewScreen.svelte'
  import NodesScreen from './lib/components/nodes/NodesScreen.svelte'
  import NodeDetailScreen from './lib/components/nodes/NodeDetailScreen.svelte'
  import JobsScreen from './lib/components/jobs/JobsScreen.svelte'
  import JobDetailScreen from './lib/components/jobs/JobDetailScreen.svelte'
  import JobRunScreen from './lib/components/jobs/JobRunScreen.svelte'
  import type { ClusterInput } from './lib/stores/clusters.svelte'
  import type { Page } from './lib/types/wails'

  const clusters = createClustersStore()
  const toasts = toastState()
  const ui = createUiStore()
  const loads = createLoadsStore()
  const nodes = createNodesStore()
  const jobs = createJobsStore()

  let showAddDialog = $state(false)
  let editTarget = $state<ClusterInput | null>(null)
  let editHasToken = $state(false)
  let confirmRemove = $state<string | null>(null)

  onMount(() => {
    void clusters.refresh()
  })

  // 集群切换（或启动时已有 activeID）：清空旧数据并首拉负载/节点/job。
  $effect(() => {
    const id = clusters.state.activeID
    loads.clear()
    nodes.clear()
    jobs.clear()
    if (!id) return
    void loads.refresh(id)
    void nodes.refresh(id)
    void jobs.refresh(id)
  })

  // 进入 job-detail 时拉详情 + allocs。
  $effect(() => {
    const page = ui.route.page
    const jobID = ui.route.params.jobID
    const id = clusters.state.activeID
    if (page !== 'job-detail' || !jobID || !id) return
    void jobs.loadDetail(id, jobID)
  })

  const navItems = $derived<{ page: Page; label: string }[]>([
    { page: 'overview', label: t('nav.overview') },
    { page: 'jobs', label: t('nav.jobs') },
    { page: 'allocs', label: t('nav.allocs') },
    { page: 'nodes', label: t('nav.nodes') },
    { page: 'job-run', label: t('nav.runJob') },
  ])

  // 语言切换联动 <html lang>。
  $effect(() => {
    document.documentElement.lang = i18n.locale === 'zh' ? 'zh-CN' : 'en'
  })

  const active = $derived(
    clusters.state.clusters.find((c) => c.info.id === clusters.state.activeID) ?? null,
  )

  function healthDot(health: string): string {
    switch (health) {
      case 'ok':
        return 'bg-emerald-500'
      case 'down':
        return 'bg-red-500'
      default:
        return 'bg-zinc-600'
    }
  }

  async function onAdd(input: ClusterInput): Promise<boolean> {
    return clusters.addCluster(input)
  }

  async function onEdit(input: ClusterInput): Promise<boolean> {
    return clusters.updateCluster(input)
  }

  function openEdit(id: string): void {
    const item = clusters.state.clusters.find((c) => c.info.id === id)
    if (!item) return
    const info = item.info
    editHasToken = info.hasToken
    editTarget = {
      id: info.id,
      name: info.name,
      address: info.address,
      region: info.region,
      namespace: info.namespace,
      tls: info.tls,
      insecureSkipVerify: info.insecureSkipVerify,
      token: '',
    }
  }

  async function handleRemove(id: string): Promise<void> {
    confirmRemove = null
    await clusters.removeCluster(id)
  }
</script>

<div class="flex h-screen w-screen flex-col bg-zinc-950 text-zinc-100">
  <!-- TitleBar -->
  <header
    class="flex h-10 shrink-0 items-center justify-between border-b border-zinc-800 bg-zinc-900 px-4 select-none"
  >
    <div class="flex items-center gap-2">
      <span class="h-2.5 w-2.5 rounded-full bg-red-500"></span>
      <span class="text-sm font-semibold tracking-wide">{t('app.title')}</span>
    </div>
    <div class="flex items-center gap-3 text-xs text-zinc-400">
      <div class="flex overflow-hidden rounded border border-zinc-700">
        <button
          class="px-2 py-0.5 {i18n.locale === 'zh'
            ? 'bg-zinc-100 font-medium text-zinc-900'
            : 'text-zinc-400 hover:bg-zinc-800'}"
          onclick={() => i18n.setLocale('zh')}
          title="中文"
        >
          中
        </button>
        <button
          class="px-2 py-0.5 {i18n.locale === 'en'
            ? 'bg-zinc-100 font-medium text-zinc-900'
            : 'text-zinc-400 hover:bg-zinc-800'}"
          onclick={() => i18n.setLocale('en')}
          title="English"
        >
          EN
        </button>
      </div>
      <span class={`h-2 w-2 rounded-full ${active ? healthDot(active.info.health) : 'bg-zinc-600'}`}
      ></span>
      <span>{active ? active.info.health : t('app.noCluster')}</span>
    </div>
  </header>

  <!-- Body -->
  <div class="flex min-h-0 flex-1">
    <!-- Sidebar -->
    <aside class="flex w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/50 p-2">
      <div class="mb-1 px-2 text-xs font-semibold text-zinc-500 uppercase">{t('app.views')}</div>
      <nav class="mb-3 flex flex-col gap-0.5">
        {#each navItems as item (item.page)}
          <button
            class="rounded px-2 py-1.5 text-left text-sm hover:bg-zinc-800 {ui.route.page ===
            item.page
              ? 'bg-zinc-800 font-medium text-zinc-100'
              : 'text-zinc-400'}"
            onclick={() => ui.navigate(item.page)}
          >
            {item.label}
          </button>
        {/each}
      </nav>

      <div class="mb-2 border-t border-zinc-800"></div>

      <div class="mb-2 px-2 text-xs font-semibold text-zinc-500 uppercase">{t('app.clusters')}</div>
      <div class="flex-1 overflow-y-auto">
        {#if clusters.state.loading}
          <div class="px-2 py-1 text-xs text-zinc-600">{t('common.loading')}</div>
        {:else if clusters.state.clusters.length === 0}
          <div class="px-2 py-1 text-xs text-zinc-600">{t('app.noClustersHint')}</div>
        {:else}
          {#each clusters.state.clusters as item (item.info.id)}
            {@const c = item.info}
            <button
              class="group mb-1 flex w-full cursor-pointer items-center justify-between rounded px-2 py-1.5 text-left text-sm hover:bg-zinc-800 {c.id ===
              clusters.state.activeID
                ? 'bg-zinc-800 text-zinc-100'
                : 'text-zinc-300'}"
              onclick={() => void clusters.setActive(c.id)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') void clusters.setActive(c.id)
              }}
            >
              <span class="flex min-w-0 items-center gap-2">
                <span class={`h-2 w-2 shrink-0 rounded-full ${healthDot(c.health)}`}></span>
                <span class="truncate">{c.name || c.id}</span>
              </span>
              <span class="flex shrink-0 items-center">
                <span
                  role="button"
                  tabindex="0"
                  class="hidden rounded px-1 text-[11px] text-zinc-500 hover:bg-zinc-700 hover:text-zinc-200 group-hover:inline"
                  title={t('cluster.edit')}
                  onclick={(e) => {
                    e.stopPropagation()
                    e.preventDefault()
                    openEdit(c.id)
                  }}
                  onkeydown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.stopPropagation()
                      e.preventDefault()
                      openEdit(c.id)
                    }
                  }}>✎</span
                >
                {#if c.id === clusters.state.activeID}
                  <span
                    role="button"
                    tabindex="0"
                    class="hidden rounded px-1 text-[11px] text-zinc-500 hover:bg-zinc-700 hover:text-zinc-200 group-hover:inline"
                    title={t('app.removeTitle')}
                    onclick={(e) => {
                      e.stopPropagation()
                      e.preventDefault()
                      confirmRemove = c.id
                    }}
                    onkeydown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.stopPropagation()
                        e.preventDefault()
                        confirmRemove = c.id
                      }
                    }}>✕</span
                  >
                {/if}
              </span>
            </button>
          {/each}
        {/if}
      </div>
      <button
        class="mt-2 rounded border border-zinc-700 px-2 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800"
        onclick={() => (showAddDialog = true)}
      >
        {t('app.addCluster')}
      </button>
    </aside>

    <!-- Content -->
    <main class="flex min-w-0 flex-1 flex-col overflow-y-auto">
      {#if active}
        {#if ui.route.page === 'overview'}
          <OverviewScreen
            clusterName={active.info.name || active.info.id}
            load={loads.state.cluster}
            busy={loads.state.loading}
            onRefresh={() => void loads.refresh(active.info.id)}
            onSelectAlloc={(allocID) => {
              const al = loads.state.allocs.get(allocID)
              ui.navigate('job-detail', { jobID: al?.jobID ?? allocID })
            }}
          />
        {:else if ui.route.page === 'nodes'}
          <NodesScreen
            nodes={nodes.list}
            loads={loads.state.nodes}
            busy={nodes.state.loading}
            onRefresh={() => void nodes.refresh(active.info.id)}
            onSelect={(nodeID) => ui.navigate('node-detail', { nodeID })}
          />
        {:else if ui.route.page === 'node-detail'}
          <NodeDetailScreen
            node={nodes.list.find((n) => n.id === ui.route.params.nodeID) ?? null}
            load={loads.state.nodes.get(ui.route.params.nodeID) ?? null}
            allocs={[...loads.state.allocs.values()].filter(
              (a) => a.nodeID === ui.route.params.nodeID,
            )}
            onBack={() => ui.navigate('nodes')}
          />
        {:else if ui.route.page === 'jobs'}
          <JobsScreen
            jobs={jobs.list}
            loading={jobs.state.loading}
            onRefresh={() => void jobs.refresh(active.info.id)}
            onSelect={(jobID) => ui.navigate('job-detail', { jobID })}
          />
        {:else if ui.route.page === 'job-detail'}
          <JobDetailScreen
            detail={jobs.state.detail}
            allocs={jobs.state.detailAllocs}
            loading={jobs.state.detailLoading}
            busyOp={jobs.state.busyOp}
            onBack={() => ui.navigate('jobs')}
            onRefresh={() => void jobs.reloadDetail(active.info.id)}
            onEvaluate={(jobID) => void jobs.evaluate(active.info.id, jobID)}
            onStop={(jobID, purge) => void jobs.stop(active.info.id, jobID, purge)}
            onScale={(jobID, group, count) => void jobs.scale(active.info.id, jobID, group, count)}
            onRestartAlloc={(allocID, taskName) =>
              void jobs.restartAlloc(active.info.id, allocID, taskName)}
            onStopAlloc={(allocID) => void jobs.stopAlloc(active.info.id, allocID)}
          />
        {:else if ui.route.page === 'job-run'}
          <JobRunScreen
            busy={jobs.state.busyOp !== null}
            onRun={(input) => jobs.runJob(active.info.id, input)}
            onDone={(jobID) => ui.navigate('job-detail', { jobID })}
          />
        {:else}
          <section class="mx-auto w-full max-w-4xl p-6">
            <h1 class="text-lg font-semibold">{t('nav.allocs')}</h1>
            <p class="mt-8 text-xs text-zinc-600">{t('app.allocsPlaceholder')}</p>
          </section>
        {/if}
      {:else}
        <div class="flex flex-1 items-center justify-center text-zinc-500">
          <div class="text-center">
            <div class="text-sm font-medium text-zinc-400">
              {t('app.connectedEmpty')}
            </div>
            <button
              class="mt-4 rounded bg-zinc-100 px-3 py-1.5 text-sm font-medium text-zinc-900 hover:bg-white"
              onclick={() => (showAddDialog = true)}
            >
              {t('app.addCluster')}
            </button>
          </div>
        </div>
      {/if}
    </main>
  </div>

  <!-- StatusBar -->
  <footer
    class="flex h-7 shrink-0 items-center justify-between border-t border-zinc-800 bg-zinc-900 px-3 text-[11px] text-zinc-500"
  >
    <span>
      {t('app.statusbar.health')}: {active?.info.health ?? 'n/a'} · {t('app.statusbar.load')}: {loads
        .state.lastUpdate
        ? new Date(loads.state.lastUpdate).toLocaleTimeString()
        : 'n/a'}
      {loads.state.stale ? ` · ${t('app.statusbar.stale')}` : ''}
    </span>
    <span>{t('app.statusbar.clusterCount', { n: clusters.state.clusters.length })}</span>
  </footer>

  <!-- Toasts -->
  <div class="pointer-events-none fixed top-12 right-4 z-[60] flex w-80 flex-col gap-2">
    {#each toasts as toast (toast.id)}
      <div
        class="pointer-events-auto rounded border px-3 py-2 text-xs shadow-lg
        {toast.level === 'error'
          ? 'border-red-800 bg-red-950 text-red-300'
          : toast.level === 'success'
            ? 'border-emerald-800 bg-emerald-950 text-emerald-300'
            : 'border-zinc-700 bg-zinc-900 text-zinc-200'}"
      >
        {toast.message}
      </div>
    {/each}
  </div>

  <!-- Confirm remove -->
  {#if confirmRemove}
    <button
      class="fixed inset-0 z-50 w-full cursor-default border-none bg-black/60 p-0"
      aria-label={t('common.cancel')}
      onclick={() => (confirmRemove = null)}
    ></button>
    <div class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center">
      <div
        class="pointer-events-auto w-[360px] rounded-lg border border-zinc-700 bg-zinc-900 p-4 shadow-2xl"
      >
        <h2 class="text-sm font-semibold">{t('cluster.removeTitle')}</h2>
        <p class="mt-2 text-xs text-zinc-400">
          {t('cluster.removeBody', { name: confirmRemove })}
        </p>
        <div class="mt-4 flex justify-end gap-2">
          <button
            class="rounded border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
            onclick={() => (confirmRemove = null)}
          >
            {t('common.cancel')}
          </button>
          <button
            class="rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-500"
            onclick={() => void handleRemove(confirmRemove!)}
          >
            {t('cluster.remove')}
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showAddDialog}
    <AddClusterDialog mode="add" onSubmit={onAdd} onClose={() => (showAddDialog = false)} />
  {/if}
  {#if editTarget}
    <AddClusterDialog
      mode="edit"
      initial={editTarget}
      hasToken={editHasToken}
      onSubmit={onEdit}
      onClose={() => {
        editTarget = null
        editHasToken = false
      }}
    />
  {/if}
</div>
