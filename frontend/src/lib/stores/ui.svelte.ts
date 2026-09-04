import type { Page } from '../types/wails'

// ui.svelte.ts —— 极简路由 store＼�。
// MVP 页面少，不引第三方 router：page + params 两个字段足够。
// 快捷键（⌘K / 1/2/3）与侧栏导航统一走 navigate()。

export interface Route {
  page: Page
  // 例如 job-detail: { jobID: "nginx" }；clusterID 从 clusters store 取
  params: Record<string, string>
}

const HOME: Route = { page: 'overview', params: {} }

function parseHash(): Route {
  // #/jobs/<id> / #/overview 等；解析失败回 HOME。
  try {
    const h = window.location.hash.replace(/^#\/?/, '')
    if (!h) return HOME
    const [page, ...rest] = h.split('/')
    const params: Record<string, string> = {}
    if ((page === 'job-detail' || page === 'node-detail') && rest[0]) {
      params[page === 'job-detail' ? 'jobID' : 'nodeID'] = decodeURIComponent(rest[0])
    }
    return { page: page as Route['page'], params }
  } catch {
    return HOME
  }
}

function toHash(r: Route): string {
  if (r.page === 'job-detail' && r.params.jobID)
    return `#/job-detail/${encodeURIComponent(r.params.jobID)}`
  if (r.page === 'node-detail' && r.params.nodeID)
    return `#/node-detail/${encodeURIComponent(r.params.nodeID)}`
  return `#/${r.page}`
}

export function createUiStore() {
  const route = $state<Route>(typeof window !== 'undefined' ? parseHash() : HOME)
  let syncing = false

  function navigate(page: Page, params: Record<string, string> = {}): void {
    route.page = page
    route.params = params
    // 同步到 URL hash，支持深链/刷新恢复/后退。
    if (typeof window !== 'undefined') {
      syncing = true
      window.location.hash = toHash(route)
      // hashchange 异步触发，用 microtask 复位避免回环。
      queueMicrotask(() => (syncing = false))
    }
  }

  // 后退/前进/外部深链进入时回合同步。
  if (typeof window !== 'undefined') {
    window.addEventListener('hashchange', () => {
      if (syncing) return
      const next = parseHash()
      route.page = next.page
      route.params = next.params
    })
  }

  return {
    get route() {
      return route
    },
    navigate,
  }
}
