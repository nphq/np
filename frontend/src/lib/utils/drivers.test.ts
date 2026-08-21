import { describe, expect, it } from 'vitest'
import { clusterHasDriver } from './drivers'
import type { nomad } from '../types/wails'

function node(partial: Partial<nomad.NodeSummary>): nomad.NodeSummary {
  return {
    id: 'n1',
    name: 'n1',
    status: 'ready',
    schedulingEligibility: 'eligible',
    datacenter: 'dc1',
    region: 'global',
    class: '',
    version: '1.9.0',
    cpu: 0,
    cpuTotal: 1000,
    cpuCores: 1,
    memory: 0,
    memoryTotal: 1024,
    disk: 0,
    diskTotal: 10240,
    runningAllocs: 0,
    ...partial,
  }
}

describe('clusterHasDriver', () => {
  it('returns true when a ready node detects the driver', () => {
    expect(clusterHasDriver([node({ drivers: ['exec', 'docker'] })], 'docker')).toBe(true)
  })

  it('ignores ineligible or down nodes', () => {
    const nodes = [
      node({ id: 'down', status: 'down', drivers: ['docker'] }),
      node({
        id: 'drain',
        schedulingEligibility: 'ineligible',
        drivers: ['docker'],
      }),
    ]
    expect(clusterHasDriver(nodes, 'docker')).toBe(false)
  })

  it('returns false when driver is absent', () => {
    expect(clusterHasDriver([node({ drivers: ['exec'] })], 'docker')).toBe(false)
  })
})
