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

export function createUiStore() {
  const route = $state<Route>(HOME)

  function navigate(page: Page, params: Record<string, string> = {}): void {
    route.page = page
    route.params = params
  }

  return {
    get route() {
      return route
    },
    navigate,
  }
}
