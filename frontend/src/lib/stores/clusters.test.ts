import { beforeEach, describe, expect, it, vi } from 'vitest'

// 竞态防护测试（评审 P0-2）：mock 掉 Wails bindings，用可控 deferred 验证
// 快速切换时旧请求的迟到响应不会覆盖新上下文。

vi.mock('../../../bindings/github.com/nphq/np/internal/app/app', () => ({
  ListClusters: vi.fn(),
  AddCluster: vi.fn(),
  RemoveCluster: vi.fn(),
  SetActiveCluster: vi.fn(),
  UpdateCluster: vi.fn(),
  PinCluster: vi.fn(),
  DiscoverClusters: vi.fn(),
  ImportFromEnv: vi.fn(),
  TestConnection: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(), Off: vi.fn() },
}))

import * as app from '../../../bindings/github.com/nphq/np/internal/app/app'
import { Events } from '@wailsio/runtime'
import { createClustersStore } from './clusters.svelte'
import type { nomad } from '../types/wails'

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

function listResponse(
  clusterID: string,
  overrides: Partial<nomad.ClusterInfo> = {},
): nomad.ClusterList {
  return {
    clusters: [
      {
        id: clusterID,
        name: clusterID,
        address: `http://${clusterID}:4646`,
        health: 'unknown',
        lastChecked: 0,
        ...overrides,
      },
    ],
    activeID: clusterID,
  } as unknown as nomad.ClusterList
}

type HealthHandler = (ev: { data: unknown }) => void

function captureHealthHandler(): HealthHandler {
  let handler: HealthHandler | undefined
  vi.mocked(Events.On).mockImplementation(((_name: string, cb: (ev: unknown) => void) => {
    handler = cb as HealthHandler
  }) as never)
  return (ev) => {
    if (!handler) throw new Error('health handler not registered')
    handler(ev)
  }
}

describe('clusters store 竞态防护', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('连续 refresh：慢的旧响应不得覆盖新数据', async () => {
    const d1 = deferred<nomad.ClusterList>()
    const d2 = deferred<nomad.ClusterList>()
    vi.mocked(app.ListClusters)
      .mockReturnValueOnce(d1.promise as never)
      .mockReturnValueOnce(d2.promise as never)
    const store = createClustersStore()

    const p1 = store.refresh()
    const p2 = store.refresh()
    // 新请求先回，旧请求（更早发起）后回：state 必须保留新数据。
    d2.resolve(listResponse('c2'))
    await p2
    d1.resolve(listResponse('c1'))
    await p1

    expect(store.state.clusters).toHaveLength(1)
    expect(store.state.clusters[0].info.id).toBe('c2')
    expect(store.state.activeID).toBe('c2')
  })

  it('setActive 作废在飞 refresh：旧响应不得写入，并重拉列表', async () => {
    const d1 = deferred<nomad.ClusterList>()
    const dSet = deferred<null>()
    const dReload = deferred<nomad.ClusterList>()
    vi.mocked(app.ListClusters)
      .mockReturnValueOnce(d1.promise as never)
      .mockReturnValueOnce(dReload.promise as never)
    vi.mocked(app.SetActiveCluster).mockReturnValueOnce(dSet.promise as never)
    const store = createClustersStore()

    const p1 = store.refresh()
    const pSet = store.setActive('c1')
    dSet.resolve(null)
    // setActive 成功后会再调 ListClusters；先让重拉完成
    await Promise.resolve()
    dReload.resolve(listResponse('c1'))
    await pSet
    d1.resolve(listResponse('stale'))
    await p1

    expect(store.state.activeID).toBe('c1')
    expect(store.state.clusters).toHaveLength(1)
    expect(store.state.clusters[0].info.id).toBe('c1')
    expect(store.state.loading).toBe(false)
  })

  it('importFromEnv 同 ID 再导入会递增 activeEpoch', async () => {
    vi.mocked(app.ImportFromEnv).mockResolvedValue({
      id: 'local',
      name: 'From environment',
    } as never)
    vi.mocked(app.ListClusters).mockResolvedValue(listResponse('local') as never)
    const store = createClustersStore()

    const r1 = await store.importFromEnv('A')
    expect(r1.ok).toBe(true)
    const epoch1 = store.state.activeEpoch
    expect(epoch1).toBeGreaterThan(0)

    const r2 = await store.importFromEnv('A')
    expect(r2.ok).toBe(true)
    expect(store.state.activeEpoch).toBe(epoch1 + 1)
    expect(store.state.activeID).toBe('local')
  })

  it('stale refresh 的错误不弹 toast（stale toast 抑制）', async () => {
    const d1 = deferred<unknown>()
    const d2 = deferred<nomad.ClusterList>()
    vi.mocked(app.ListClusters)
      .mockReturnValueOnce(d1.promise as never)
      .mockReturnValueOnce(d2.promise as never)
    const store = createClustersStore()

    const p1 = store.refresh()
    store.refresh()
    d2.resolve(listResponse('c2'))
    // 旧请求迟到的错误响应：不得 toast（toastErr 不导出，以不抛/不覆盖断言）
    d1.resolve({ code: 'ERR', message: 'boom' })
    await p1

    expect(store.state.clusters[0].info.id).toBe('c2')
  })

  it('refresh 不得用过期快照覆盖更新的 health', async () => {
    const emit = captureHealthHandler()
    vi.mocked(app.ListClusters).mockResolvedValueOnce(
      listResponse('c1', { health: 'unknown', lastChecked: 1 }) as never,
    )
    const store = createClustersStore()
    await store.refresh()

    const dList = deferred<nomad.ClusterList>()
    vi.mocked(app.ListClusters).mockReturnValueOnce(dList.promise as never)
    const p = store.refresh()
    emit({
      data: { clusterID: 'c1', status: 'ok', lastChecked: 100, leader: '127.0.0.1:4647' },
    })
    expect(store.state.clusters[0].info.health).toBe('ok')

    // 慢快照仍是 unknown / 更早时间戳 → 合入后必须保留 ok
    dList.resolve(listResponse('c1', { health: 'unknown', lastChecked: 1 }))
    await p

    expect(store.state.clusters[0].info.health).toBe('ok')
    expect(store.state.clusters[0].info.lastChecked).toBe(100)
    expect(store.state.clusters[0].leader).toBe('127.0.0.1:4647')
  })

  it('列表尚空时的 health 事件在 refresh 后合入', async () => {
    const emit = captureHealthHandler()
    const store = createClustersStore()
    emit({
      data: { clusterID: 'c1', status: 'ok', lastChecked: 50, version: '1.9.4' },
    })
    expect(store.state.clusters).toHaveLength(0)

    vi.mocked(app.ListClusters).mockResolvedValueOnce(
      listResponse('c1', { health: 'unknown', lastChecked: 0 }) as never,
    )
    await store.refresh()

    expect(store.state.clusters[0].info.health).toBe('ok')
    expect(store.state.clusters[0].version).toBe('1.9.4')
  })
})
