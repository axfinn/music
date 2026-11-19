import { describe, expect, it, vi } from 'vitest'
import { createWakeLock } from './useWakeLock.js'

// 用最小替身模拟 Wake Lock Sentinel / document，避免依赖真实浏览器与 jsdom 缺失的 wakeLock。
function makeSentinel(fireReleaseOnRequest = false) {
  const handlers = new Map()
  const s = {
    released: false,
    addEventListener(type, fn) { handlers.set(type, fn) },
    removeEventListener() {},
    release: vi.fn(async () => {
      if (s.released) return
      s.released = true
      handlers.get('release')?.()
    }),
  }
  if (fireReleaseOnRequest) queueMicrotask(() => s.release())
  return s
}

function makeEnv({ supported = true, hidden = false, request } = {}) {
  const sentinels = []
  const requestImpl = request || vi.fn(async () => {
    const s = makeSentinel()
    sentinels.push(s)
    return s
  })
  const listeners = new Map()
  const nav = supported ? { wakeLock: { request: requestImpl } } : {}
  const doc = {
    hidden,
    addEventListener(type, fn) { listeners.set(type, fn) },
    removeEventListener(type) { listeners.delete(type) },
  }
  const fireVisibility = () => listeners.get('visibilitychange')?.()
  return { navigator: nav, document: doc, request: requestImpl, sentinels, fireVisibility, listeners }
}

function tick() { return new Promise(resolve => setTimeout(resolve, 0)) }

describe('useWakeLock 屏幕常亮', () => {
  it('浏览器不支持时静默降级，任何调用都不抛错', async () => {
    const env = makeEnv({ supported: false })
    const wake = createWakeLock(env)
    expect(wake.supported).toBe(false)
    wake.bind()
    wake.setWanted(true)
    await tick()
    expect(env.request).not.toHaveBeenCalled()
    expect(wake.locked.value).toBe(false)
    await expect(wake.dispose()).resolves.toBeUndefined()
  })

  it('想要常亮时申请 screen 锁，取消时释放', async () => {
    const env = makeEnv()
    const wake = createWakeLock(env)
    wake.setWanted(true)
    await tick()
    expect(env.request).toHaveBeenCalledWith('screen')
    expect(wake.locked.value).toBe(true)

    wake.setWanted(false)
    await tick()
    expect(env.sentinels[0].release).toHaveBeenCalledTimes(1)
    expect(wake.locked.value).toBe(false)
  })

  it('文档隐藏时释放锁，回到前台后重新申请', async () => {
    const env = makeEnv()
    const wake = createWakeLock(env)
    wake.bind()
    wake.setWanted(true)
    await tick()
    expect(wake.locked.value).toBe(true)

    env.document.hidden = true
    env.fireVisibility()
    await tick()
    expect(env.sentinels[0].release).toHaveBeenCalledTimes(1)
    expect(wake.locked.value).toBe(false)

    env.document.hidden = false
    env.fireVisibility()
    await tick()
    expect(env.request).toHaveBeenCalledTimes(2)
    expect(wake.locked.value).toBe(true)
  })

  it('隐藏状态下不申请锁（浏览器会拒绝）', async () => {
    const env = makeEnv({ hidden: true })
    const wake = createWakeLock(env)
    wake.setWanted(true)
    await tick()
    expect(env.request).not.toHaveBeenCalled()
    expect(wake.locked.value).toBe(false)
  })

  it('浏览器主动释放后同步状态；仍需要时重新申请', async () => {
    const env = makeEnv()
    const wake = createWakeLock(env)
    wake.setWanted(true)
    await tick()
    expect(wake.locked.value).toBe(true)

    // 模拟省电策略触发 sentinel 的 release 事件
    await env.sentinels[0].release()
    await tick()
    expect(wake.locked.value).toBe(true) // 自动补申请，仍保持亮屏
    expect(env.request).toHaveBeenCalledTimes(2)
  })

  it('申请被浏览器拒绝时只降级不抛错', async () => {
    const env = makeEnv({ request: vi.fn().mockRejectedValue(new Error('NotAllowedError')) })
    const wake = createWakeLock(env)
    wake.setWanted(true)
    await tick()
    await tick()
    expect(wake.locked.value).toBe(false)
  })

  it('异步申请期间状态变化：结果到达后立刻退还过期的锁', async () => {
    let resolveRequest
    const env = makeEnv({ request: vi.fn(() => new Promise(r => { resolveRequest = r })) })
    const wake = createWakeLock(env)
    wake.setWanted(true) // 发起申请，尚未 resolve
    wake.setWanted(false) // 立刻暂停 —— 这把锁不该再持有
    const stale = makeSentinel()
    resolveRequest(stale)
    await tick()
    expect(stale.release).toHaveBeenCalledTimes(1)
    expect(wake.locked.value).toBe(false)
  })

  it('dispose 释放锁并解绑 visibilitychange', async () => {
    const env = makeEnv()
    const wake = createWakeLock(env)
    wake.bind()
    wake.setWanted(true)
    await tick()
    await wake.dispose()
    expect(env.sentinels[0].release).toHaveBeenCalledTimes(1)
    expect(wake.locked.value).toBe(false)
    expect(env.listeners.has('visibilitychange')).toBe(false)
    // 解绑后再切后台不应再触发任何申请
    env.fireVisibility()
    await tick()
    expect(env.request).toHaveBeenCalledTimes(1)
  })
})
