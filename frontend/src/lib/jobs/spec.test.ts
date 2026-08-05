import { describe, expect, it } from 'vitest'
import { buildDockerJobJSON, defaultDockerForm, extractJobIDFromSpec, tryFormatJSON } from './spec'

describe('buildDockerJobJSON', () => {
  it('builds a service job with port mapping', () => {
    const json = buildDockerJobJSON({
      ...defaultDockerForm(),
      jobID: 'nginx',
      count: 2,
      image: 'nginx:1.27',
      portLabel: 'http',
      portTo: 80,
    })
    const job = JSON.parse(json)
    expect(job.ID).toBe('nginx')
    expect(job.Type).toBe('service')
    expect(job.TaskGroups[0].Count).toBe(2)
    expect(job.TaskGroups[0].Tasks[0].Config.image).toBe('nginx:1.27')
    expect(job.TaskGroups[0].Networks[0].DynamicPorts[0]).toEqual({
      Label: 'http',
      To: 80,
    })
  })

  it('includes command and args when set', () => {
    const json = buildDockerJobJSON({
      ...defaultDockerForm(),
      type: 'batch',
      portLabel: '',
      portTo: null,
      command: 'sleep',
      argsText: '3600',
    })
    const job = JSON.parse(json)
    expect(job.TaskGroups[0].Tasks[0].Config.command).toBe('sleep')
    expect(job.TaskGroups[0].Tasks[0].Config.args).toEqual(['3600'])
  })
})

describe('tryFormatJSON / extractJobIDFromSpec', () => {
  it('formats and extracts ids', () => {
    const raw = '{"ID":"a","Name":"a"}'
    const fmt = tryFormatJSON(raw)
    expect(fmt.ok).toBe(true)
    if (fmt.ok) expect(fmt.text).toContain('\n')
    expect(extractJobIDFromSpec(raw, 'json')).toBe('a')
    expect(extractJobIDFromSpec('job "demo" {\n}', 'hcl')).toBe('demo')
  })
})
