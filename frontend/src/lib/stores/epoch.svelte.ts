// epoch.svelte.ts —— 异步请求竞态防护（generation token）。
//
// 背景：Wails bindings 返回普通 Promise，不支持 AbortSignal；IPC 请求已发到
// Go 端也无法中断。快速切换集群/job 时，旧请求的慢响应可能晚于新请求到达，
// 把陈旧数据写进新上下文。epoch guard 不省请求，但保证**写 state 前的校验**：
// token 已被后续 acquire/invalidate 作废的响应直接丢弃。
export function createEpoch() {
  let current = 0
  return {
    /** 发起一次异步操作前调用，返回这次操作的 token */
    acquire(): number {
      return ++current
    },
    /** 取消所有在飞的 token（切集群/job 时调用） */
    invalidate(): void {
      current++
    },
    /** 写 state 前校验：token 仍是最新则 true */
    active(token: number): boolean {
      return token === current
    },
  }
}
