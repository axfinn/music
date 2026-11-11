import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, json, request } from './client.js'

function mockFetch(status, body, { ok = status >= 200 && status < 300 } = {}) {
  const fn = vi.fn().mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(body),
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => vi.unstubAllGlobals())

describe('request', () => {
  it('带同源凭据并解析成功响应', async () => {
    const fetchMock = mockFetch(200, { items: [1] })
    await expect(request('/api/music/tracks')).resolves.toEqual({ items: [1] })
    expect(fetchMock).toHaveBeenCalledWith('/api/music/tracks', {
      method: 'GET',
      body: undefined,
      headers: undefined,
      signal: undefined,
      credentials: 'same-origin',
    })
  })

  it('透传 method / body / headers / signal', async () => {
    const fetchMock = mockFetch(200, { ok: true })
    const ctrl = new AbortController()
    await request('/api/admin/login', { ...json({ password: 'x' }), signal: ctrl.signal })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: '{"password":"x"}',
      headers: { 'Content-Type': 'application/json' },
      signal: ctrl.signal,
    })
  })

  it('优先使用后端给的错误文案并带上 status / missing', async () => {
    mockFetch(404, { error: '这首不在库里', missing: true }, { ok: false })
    await expect(request('/api/music/tracks/1')).rejects.toMatchObject({
      name: 'ApiError',
      message: '这首不在库里',
      status: 404,
      missing: true,
    })
  })

  it('后端没给文案时按状态码兜底', async () => {
    mockFetch(429, {}, { ok: false })
    await expect(request('/api/music/tracks')).rejects.toMatchObject({
      status: 429,
      message: '操作太频繁，请稍后再试',
    })
  })

  it('响应不是 JSON 也不炸', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.reject(new Error('not json')),
    }))
    await expect(request('/api/whatever')).rejects.toMatchObject({
      status: 500,
      message: '服务器开小差了',
    })
  })

  it('网络异常包成 ApiError，AbortError 原样抛', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    await expect(request('/api/site')).rejects.toBeInstanceOf(ApiError)

    const abort = Object.assign(new Error('aborted'), { name: 'AbortError' })
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(abort))
    await expect(request('/api/site')).rejects.toBe(abort)
  })
})

describe('json', () => {
  it('默认 POST，也支持 PATCH', () => {
    expect(json({ a: 1 })).toEqual({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{"a":1}',
    })
    expect(json({ a: 1 }, 'PATCH').method).toBe('PATCH')
  })
})
