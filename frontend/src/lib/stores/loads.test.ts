import { beforeEach, describe, expect, it, vi } from 'vitest'

// 竞态防护测试（评审 P0-2）：loads store 的 refresh epoch guard +
// load.patch 事件按 clusterID 过滤（切集群后旧集群的迟到 patch 不得合入）。

vi.mock('../../../bindings/github.com/nphq/np/app', () => ({
  GetClusterLoad: vi.fn(),
  GetNodeLoads: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(), Off: vi.fn() },
}))

import * as app from '../../../bindings/github.com/nphq/np/app'
import { Events } from '@wailsio/runtime'
import { createLoadsStore } from './loads.svelte'

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

type PatchHandler = (ev: { data: unknown }) => void

function capturePatchHandler(): PatchHandler {
  let handler: PatchHandler | undefined
  vi.mocked(Events.On).mockImplementation(((_name: string, cb: (ev: unknown) => void) => {
    handler = cb as PatchHandler
  }) as never)
  return (ev) => {
    if (!handler) throw new Error('patch handler not registered')
    handler(ev)
  }
}

describe('loads store 竞态防护', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('连续 refresh：旧集群的慢响应不得覆盖新集群数据', async () => {
    const d1 = deferred<unknown>()
    const d2 = deferred<unknown>()
    const d1n = deferred<unknown>()
    const d2n = deferred<unknown>()
    vi.mocked(app.GetClusterLoad)
      .mockReturnValueOnce(d1.promise as never)
      .mockReturnValueOnce(d2.promise as never)
    vi.mocked(app.GetNodeLoads)
      .mockReturnValueOnce(d1n.promise as never)
      .mockReturnValueOnce(d2n.promise as never)
    const store = createLoadsStore()

    const p1 = store.refresh('A')
    const p2 = store.refresh('B')
    d2.resolve({ updatedAt: 2, cpu: 0.2, mem: 0.2 })
    d2n.resolve([{ nodeID: 'n-B', cpu: 0.2, mem: 0.2 }])
    await p2
    d1.resolve({ updatedAt: 1, cpu: 0.9, mem: 0.9 })
    d1n.resolve([{ nodeID: 'n-A', cpu: 0.9, mem: 0.9 }])
    await p1

    expect(store.state.cluster?.updatedAt).toBe(2)
    expect(store.state.nodes.has('n-B')).toBe(true)
    expect(store.state.nodes.has('n-A')).toBe(false)
  })

  it('load.patch：旧集群 clusterID 的 patch 被丢弃，激活集群的合入', async () => {
    const d1 = deferred<unknown>()
    const d1n = deferred<unknown>()
    vi.mocked(app.GetClusterLoad).mockReturnValueOnce(d1.promise as never)
    vi.mocked(app.GetNodeLoads).mockReturnValueOnce(d1n.promise as never)
    const emit = capturePatchHandler()
    const store = createLoadsStore()

    const p = store.refresh('B')
    d1.resolve({ updatedAt: 2 })
    d1n.resolve([{ nodeID: 'n-B1' }])
    await p

    emit({ data: { clusterID: 'A', nodes: [{ nodeID: 'n-A1' }], cluster: { updatedAt: 99 } } })
    expect(store.state.nodes.has('n-A1')).toBe(false)
    expect(store.state.cluster?.updatedAt).toBe(2)

    emit({ data: { clusterID: 'B', nodes: [{ nodeID: 'n-B2' }], cluster: { updatedAt: 3 } } })
    expect(store.state.nodes.has('n-B2')).toBe(true)
    expect(store.state.cluster?.updatedAt).toBe(3)
  })

  it('load.patch：无 clusterID 字段的旧版 payload 兼容合入', async () => {
    const d1 = deferred<unknown>()
    const d1n = deferred<unknown>()
    vi.mocked(app.GetClusterLoad).mockReturnValueOnce(d1.promise as never)
    vi.mocked(app.GetNodeLoads).mockReturnValueOnce(d1n.promise as never)
    const emit = capturePatchHandler()
    const store = createLoadsStore()

    const p = store.refresh('B')
    d1.resolve({ updatedAt: 2 })
    d1n.resolve([])
    await p

    emit({ data: { nodes: [{ nodeID: 'n-legacy' }], cluster: { updatedAt: 4 } } })
    expect(store.state.nodes.has('n-legacy')).toBe(true)
    expect(store.state.cluster?.updatedAt).toBe(4)
  })
})
