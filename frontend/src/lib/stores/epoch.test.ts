import { createEpoch } from './epoch.svelte'
import { describe, expect, it } from 'vitest'

describe('createEpoch', () => {
  it('active token 返回 true', () => {
    const e = createEpoch()
    const t = e.acquire()
    expect(e.active(t)).toBe(true)
  })

  it('invalidate 后旧 token 失效', () => {
    const e = createEpoch()
    const t = e.acquire()
    e.invalidate()
    expect(e.active(t)).toBe(false)
  })

  it('acquire 后旧 token 失效（隐式 invalidate）', () => {
    const e = createEpoch()
    const t1 = e.acquire()
    const t2 = e.acquire()
    expect(e.active(t1)).toBe(false)
    expect(e.active(t2)).toBe(true)
  })
})
