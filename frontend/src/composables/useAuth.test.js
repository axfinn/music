import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  login, logout, refreshAuth, register, sendCode, useAuth,
} from './useAuth.js'

function mockFetch(status, body) {
  const fn = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

beforeEach(async () => {
  // 每个用例前重置模块级状态：先登出，把 user 清掉。
  mockFetch(200, { authenticated: false })
  await refreshAuth()
})
afterEach(() => vi.unstubAllGlobals())

describe('useAuth 会员身份', () => {
  it('refreshAuth 读取可用性与当前会员', async () => {
    mockFetch(200, {
      available: true,
      authenticated: true,
      user: { id: 'usr_1', email: 'a@b.com', nickname: '阿' },
    })
    const user = await refreshAuth()
    expect(user.email).toBe('a@b.com')
    const { isMember, available } = useAuth()
    expect(isMember.value).toBe(true)
    expect(available.value).toBe(true)
  })

  it('未登录时 isMember 为 false', async () => {
    mockFetch(200, { available: true, authenticated: false, user: null })
    await refreshAuth()
    expect(useAuth().isMember.value).toBe(false)
  })

  it('接口异常时降级为未登录而不是抛错', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    await expect(refreshAuth()).resolves.toBeNull()
    expect(useAuth().isMember.value).toBe(false)
  })

  it('注册携带正确参数并写入会员状态', async () => {
    const fn = mockFetch(200, { authenticated: true, user: { id: 'usr_2', email: 'new@b.com' } })
    await register('new@b.com', '123456', 'secret', '新用户')
    const [url, opts] = fn.mock.calls[0]
    expect(url).toBe('/api/auth/register')
    const payload = JSON.parse(opts.body)
    expect(payload).toEqual({
      email: 'new@b.com', code: '123456', password: 'secret', nickname: '新用户',
    })
    expect(useAuth().user.value.id).toBe('usr_2')
  })

  it('sendCode 带用途发送', async () => {
    const fn = mockFetch(200, { sent: true, ttl: 600 })
    await sendCode('a@b.com', 'reset')
    const [url, opts] = fn.mock.calls[0]
    expect(url).toBe('/api/auth/code')
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com', purpose: 'reset' })
  })

  it('登录失败时抛出错误且不改变会员状态', async () => {
    mockFetch(401, { error: '邮箱或密码不正确' })
    await expect(login('a@b.com', 'wrong')).rejects.toThrow('邮箱或密码不正确')
    expect(useAuth().isMember.value).toBe(false)
  })

  it('logout 清空会员状态', async () => {
    mockFetch(200, { authenticated: true, user: { id: 'usr_3', email: 'a@b.com' } })
    await login('a@b.com', 'secret')
    expect(useAuth().isMember.value).toBe(true)
    mockFetch(200, { authenticated: false })
    await logout()
    expect(useAuth().isMember.value).toBe(false)
  })
})
