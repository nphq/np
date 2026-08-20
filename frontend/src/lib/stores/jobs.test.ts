import { beforeEach, describe, expect, it, vi } from 'vitest'

// 竞态防护测试（评审 P0-2）：job 详情/列表的 epoch guard 与 busyOp 串行化。

vi.mock('../../../bindings/github.com/nphq/np/internal/app/app', () => ({
  ListJobs: vi.fn(),
  GetJob: vi.fn(),
  ListJobAllocations: vi.fn(),
  GetAllocLogs: vi.fn(),
  EvaluateJob: vi.fn(),
  GetEvaluation: vi.fn(),
  ListAllocTaskEvents: vi.fn(),
  RestartAlloc: vi.fn(),
  RunJob: vi.fn(),
  ScaleJob: vi.fn(),
  StopAlloc: vi.fn(),
  StopJob: vi.fn(),
}))

import * as app from '../../../bindings/github.com/nphq/np/internal/app/app'
import { createJobsStore } from './jobs.svelte'

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

describe('jobs store 竞态防护', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(app.ListJobAllocations).mockResolvedValue([] as never)
  })

  it('快速切换 job：loadDetail 旧响应不得覆盖新 job', async () => {
    const dA = deferred<unknown>()
    const dB = deferred<unknown>()
    vi.mocked(app.GetJob)
      .mockReturnValueOnce(dA.promise as never)
      .mockReturnValueOnce(dB.promise as never)
    const store = createJobsStore()

    const pA = store.loadDetail('dev', 'A')
    const pB = store.loadDetail('dev', 'B')
    dB.resolve({ id: 'B', name: 'job-b' })
    await pB
    dA.resolve({ id: 'A', name: 'job-a' })
    await pA

    expect(store.state.detail).not.toBeNull()
    expect(store.state.detail?.id).toBe('B')
  })

  it('切换集群后 loadDetail 迟到响应不写入（clear 作废在飞请求）', async () => {
    const dA = deferred<unknown>()
    vi.mocked(app.GetJob).mockReturnValueOnce(dA.promise as never)
    const store = createJobsStore()

    const pA = store.loadDetail('dev', 'A')
    store.clear()
    dA.resolve({ id: 'A', name: 'job-a' })
    await pA

    expect(store.state.detail).toBeNull()
    expect(store.state.byId.size).toBe(0)
  })

  it('busyOp 串行化：写操作进行中再触发直接拒绝', async () => {
    const dE = deferred<unknown>()
    vi.mocked(app.EvaluateJob).mockReturnValueOnce(dE.promise as never)
    const store = createJobsStore()

    const p1 = store.evaluate('dev', 'A')
    const p2 = store.evaluate('dev', 'A')
    dE.resolve({ id: 'eval-1' })
    await p1
    await p2

    expect(vi.mocked(app.EvaluateJob)).toHaveBeenCalledTimes(1)
  })

  it('list refresh 慢响应被后续 refresh 作废', async () => {
    const d1 = deferred<unknown>()
    const d2 = deferred<unknown>()
    vi.mocked(app.ListJobs)
      .mockReturnValueOnce(d1.promise as never)
      .mockReturnValueOnce(d2.promise as never)
    const store = createJobsStore()

    const p1 = store.refresh('dev')
    const p2 = store.refresh('dev')
    d2.resolve([{ id: 'J2' }])
    await p2
    d1.resolve([{ id: 'J1' }])
    await p1

    expect(store.state.byId.has('J2')).toBe(true)
    expect(store.state.byId.has('J1')).toBe(false)
  })
})
