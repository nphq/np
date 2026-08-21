import type { nomad } from '../types/wails'

/** 可调度节点上是否至少有一个已检测到指定驱动。 */
export function clusterHasDriver(nodes: nomad.NodeSummary[], driver: string): boolean {
  const want = driver.trim().toLowerCase()
  if (!want) return false
  for (const n of nodes) {
    if (!isSchedulable(n)) continue
    for (const d of n.drivers ?? []) {
      if (d.toLowerCase() === want) return true
    }
  }
  return false
}

function isSchedulable(n: nomad.NodeSummary): boolean {
  const status = (n.status ?? '').toLowerCase()
  if (status && status !== 'ready') return false
  const elig = (n.schedulingEligibility ?? '').toLowerCase()
  if (elig && elig !== 'eligible') return false
  return true
}
