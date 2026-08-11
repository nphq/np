import {
  EvaluateJob,
  GetAllocLogs,
  GetEvaluation,
  GetJob,
  ListAllocTaskEvents,
  ListJobAllocations,
  ListJobs,
  RestartAlloc,
  RunJob,
  ScaleJob,
  StopAlloc,
  StopJob,
} from '../../../bindings/github.com/nphq/np/app'
import { isErr, toast, toastErr } from './clusters.svelte'
import { createEpoch } from './epoch.svelte'
import { t } from '../i18n/index.svelte'
import type { nomad, uiapi } from '../types/wails'

// jobs.svelte.ts —— job 列表/详情/部署/管理 store。
// 列表 = 首拉全量快照（M2 的 jobs.patch 事件会在后续里程碑接入增量）；
// 写操作统一 busyOp 串行化 + 成功 toast + 列表/详情局部刷新（ADR-13）。
// 异步读（refresh/loadDetail）用 epoch guard：快速切换 job/集群时，
// 旧请求的迟到响应不得覆盖新上下文（评审 P0-2）。

export interface RunJobInput {
  spec: string
  format: 'hcl' | 'json'
  namespace: string
  canonicalize: boolean
}

export interface RunJobOutcome {
  ok: boolean
  result?: nomad.JobRunResult
  err?: uiapi.Error
}

export function createJobsStore() {
  const state = $state<{
    byId: Map<string, nomad.JobSummary>
    loading: boolean
    detail: nomad.JobDetail | null
    detailAllocs: nomad.AllocSummary[]
    detailLoading: boolean
    busyOp: string | null
    /** 最近一次部署回执，详情页用于进度条 */
    lastDeploy: nomad.JobRunResult | null
  }>({
    // 整表替换，不原地 mutate（同 nodes store 约定）
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    byId: new Map(),
    loading: false,
    detail: null,
    detailAllocs: [],
    detailLoading: false,
    busyOp: null,
    lastDeploy: null,
  })

  const list = $derived([...state.byId.values()])

  // 异步读写共享 state 的 epoch：loadDetail/refresh 各自 acquire，
  // 后发请求会作废先前 token（job/集群快速切换的竞态防护）。
  const epoch = createEpoch()

  // tryAcquire 串行化写操作。busyOp 的检查-赋值在同步段内完成、
  // 无 await 间隔，原子性由 JS event loop 保证（评审确认非真实 race，
  // 抽 helper 把不变量文档化）。
  function tryAcquire(op: string): boolean {
    if (state.busyOp !== null) return false
    state.busyOp = op
    return true
  }

  async function refresh(clusterID: string): Promise<void> {
    const mine = epoch.acquire()
    state.loading = true
    try {
      const res = await ListJobs(clusterID)
      if (!epoch.active(mine)) return // 已被新请求作废，丢弃迟到响应
      if (isErr(res)) {
        toastErr(res)
        return
      }
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      state.byId = new Map((res as nomad.JobSummary[]).map((j) => [j.id, j]))
    } catch (err) {
      if (epoch.active(mine)) console.error('[jobs] refresh failed:', err)
    } finally {
      // loading 属于 UI 信号：被更新请求作废的响应照常复位，
      // 否则 setActive 等仅作废不重拉的场景会卡死 spinner。
      state.loading = false
    }
  }

  async function loadDetail(clusterID: string, jobID: string): Promise<void> {
    const mine = epoch.acquire()
    state.detailLoading = true
    try {
      const [detailRes, allocsRes] = await Promise.all([
        GetJob(clusterID, jobID),
        ListJobAllocations(clusterID, jobID),
      ])
      if (!epoch.active(mine)) return // job A → job B 快速切换：A 的响应丢弃
      if (isErr(detailRes)) {
        toastErr(detailRes)
        state.detail = null
        return
      }
      state.detail = detailRes as nomad.JobDetail
      if (isErr(allocsRes)) {
        state.detailAllocs = []
        toastErr(allocsRes)
        return
      }
      state.detailAllocs = allocsRes as nomad.AllocSummary[]
    } catch (err) {
      if (epoch.active(mine)) console.error('[jobs] loadDetail failed:', err)
    } finally {
      // 同 refresh：spinner 复位不依赖 token 是否仍活跃。
      state.detailLoading = false
    }
  }

  async function reloadDetail(clusterID: string): Promise<void> {
    const jobID = state.detail?.id
    if (!jobID) return
    try {
      await loadDetail(clusterID, jobID)
    } catch (err) {
      console.error('[jobs] reloadDetail failed:', err)
    }
  }

  // runJob 部署/更新：错误（含校验错误）不弹 toast，交还调用方内联展示。
  async function runJob(clusterID: string, input: RunJobInput): Promise<RunJobOutcome> {
    if (!tryAcquire('run')) return { ok: false }
    try {
      const res = await RunJob(
        clusterID,
        input.spec,
        input.format,
        input.namespace,
        input.canonicalize,
      )
      if (isErr(res)) {
        return { ok: false, err: res }
      }
      const result = res as nomad.JobRunResult
      state.lastDeploy = result
      toast({
        level: 'success',
        message: t('toast.jobSubmitted'),
      })
      await refresh(clusterID)
      return { ok: true, result }
    } catch (err) {
      console.error('[jobs] runJob failed:', err)
      return { ok: false, err: { code: 'runtime', message: `${err}` } }
    } finally {
      state.busyOp = null
    }
  }

  async function stop(clusterID: string, jobID: string, purge: boolean): Promise<boolean> {
    if (!tryAcquire(`stop:${jobID}`)) return false
    try {
      const res = await StopJob(clusterID, jobID, purge)
      if (isErr(res)) {
        toastErr(res)
        return false
      }
      toast({ level: 'success', message: t('toast.stopped', { jobID, evalID: `${res}` }) })
      await refresh(clusterID)
      await reloadDetail(clusterID)
      return true
    } catch (err) {
      console.error('[jobs] stop failed:', err)
      return false
    } finally {
      state.busyOp = null
    }
  }

  /** 批量停止（可选 purge）；串行调用，结束后统一刷新列表。 */
  async function stopMany(
    clusterID: string,
    jobIDs: string[],
    purge: boolean,
  ): Promise<{ ok: number; failed: string[] }> {
    if (!tryAcquire('stopMany')) return { ok: 0, failed: [...jobIDs] }
    const ids = jobIDs
      .map((id) => id.trim())
      .filter(Boolean)
      .filter((id, i, arr) => arr.indexOf(id) === i)
    if (ids.length === 0) {
      state.busyOp = null
      return { ok: 0, failed: [] }
    }

    let ok = 0
    const failed: string[] = []
    try {
      for (const jobID of ids) {
        try {
          const res = await StopJob(clusterID, jobID, purge)
          if (isErr(res)) {
            failed.push(jobID)
            continue
          }
          ok++
        } catch {
          failed.push(jobID)
        }
      }
      if (failed.length === 0) {
        toast({
          level: 'success',
          message: t('toast.stoppedMany', { ok: String(ok), total: String(ids.length) }),
        })
      } else {
        toast({
          level: 'error',
          message: t('toast.stoppedManyPartial', {
            ok: String(ok),
            fail: String(failed.length),
            total: String(ids.length),
          }),
        })
      }
      await refresh(clusterID)
      return { ok, failed }
    } finally {
      state.busyOp = null
    }
  }

  async function evaluate(clusterID: string, jobID: string): Promise<boolean> {
    if (!tryAcquire(`evaluate:${jobID}`)) return false
    try {
      const res = await EvaluateJob(clusterID, jobID)
      if (isErr(res)) {
        toastErr(res)
        return false
      }
      toast({ level: 'success', message: t('toast.evaluated', { jobID, evalID: `${res}` }) })
      await reloadDetail(clusterID)
      return true
    } catch (err) {
      console.error('[jobs] evaluate failed:', err)
      return false
    } finally {
      state.busyOp = null
    }
  }

  async function scale(
    clusterID: string,
    jobID: string,
    group: string,
    count: number,
  ): Promise<boolean> {
    if (!tryAcquire(`scale:${jobID}`)) return false
    try {
      const res = await ScaleJob(clusterID, jobID, group, count)
      if (isErr(res)) {
        toastErr(res)
        return false
      }
      toast({
        level: 'success',
        message: t('toast.scaled', { jobID, group, count: String(count), evalID: `${res}` }),
      })
      await refresh(clusterID)
      await reloadDetail(clusterID)
      return true
    } catch (err) {
      console.error('[jobs] scale failed:', err)
      return false
    } finally {
      state.busyOp = null
    }
  }

  async function restartAlloc(
    clusterID: string,
    allocID: string,
    taskName: string,
  ): Promise<boolean> {
    if (!tryAcquire(`restart:${allocID}`)) return false
    try {
      const res = await RestartAlloc(clusterID, allocID, taskName)
      if (isErr(res)) {
        toastErr(res)
        return false
      }
      toast({ level: 'success', message: t('toast.restartedAlloc', { id: allocID.slice(0, 8) }) })
      await reloadDetail(clusterID)
      return true
    } catch (err) {
      console.error('[jobs] restartAlloc failed:', err)
      return false
    } finally {
      state.busyOp = null
    }
  }

  async function stopAlloc(clusterID: string, allocID: string): Promise<boolean> {
    if (!tryAcquire(`stopAlloc:${allocID}`)) return false
    try {
      const res = await StopAlloc(clusterID, allocID)
      if (isErr(res)) {
        toastErr(res)
        return false
      }
      toast({ level: 'success', message: t('toast.stoppedAlloc', { id: allocID.slice(0, 8) }) })
      await reloadDetail(clusterID)
      return true
    } catch (err) {
      console.error('[jobs] stopAlloc failed:', err)
      return false
    } finally {
      state.busyOp = null
    }
  }

  function clear(): void {
    epoch.invalidate() // 作废在飞请求（切集群时旧上下文的迟到响应不得写入）
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    state.byId = new Map()
    state.detail = null
    state.detailAllocs = []
    state.busyOp = null
    state.lastDeploy = null
  }

  async function getEvaluation(clusterID: string, evalID: string): Promise<nomad.EvalInfo | null> {
    try {
      const res = await GetEvaluation(clusterID, evalID)
      if (isErr(res)) {
        toastErr(res)
        return null
      }
      return res as nomad.EvalInfo
    } catch (err) {
      console.error('[jobs] getEvaluation failed:', err)
      return null
    }
  }

  async function listAllocEvents(
    clusterID: string,
    allocID: string,
  ): Promise<nomad.AllocTaskEvent[]> {
    try {
      const res = await ListAllocTaskEvents(clusterID, allocID)
      if (isErr(res)) {
        toastErr(res)
        return []
      }
      return (res as nomad.AllocTaskEvent[]) ?? []
    } catch (err) {
      console.error('[jobs] listAllocEvents failed:', err)
      return []
    }
  }

  async function getAllocLogs(
    clusterID: string,
    allocID: string,
    task: string,
    logType: string,
  ): Promise<nomad.AllocLogsResult | null> {
    try {
      const res = await GetAllocLogs(clusterID, allocID, task, logType)
      if (isErr(res)) {
        toastErr(res)
        return null
      }
      return res as nomad.AllocLogsResult
    } catch (err) {
      console.error('[jobs] getAllocLogs failed:', err)
      return null
    }
  }

  function clearLastDeploy(): void {
    state.lastDeploy = null
  }

  return {
    get state() {
      return state
    },
    get list() {
      return list
    },
    refresh,
    loadDetail,
    reloadDetail,
    runJob,
    stop,
    stopMany,
    evaluate,
    scale,
    restartAlloc,
    stopAlloc,
    getEvaluation,
    listAllocEvents,
    getAllocLogs,
    clearLastDeploy,
    clear,
  }
}
