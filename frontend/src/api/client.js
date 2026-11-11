/**
 * 全站唯一的 HTTP 出入口。
 *
 * 以前音乐库、歌词库、发现流、管理身份各有自己的 fetch 封装，
 * 沉浸播放页甚至直接裸 fetch —— 错误文案、Cookie 携带、状态码处理四种写法。
 * 现在统一在这里：
 *   - 管理会话是 HttpOnly Cookie，所有请求一律带 credentials: 'same-origin'
 *   - 每个请求都带 X-Music-Client: web，供后端反爬中间件识别「自家前端」；
 *     音频 / 封面由 <audio>、<img> 直接发起，不带这个头，走浏览器 UA 通道
 *   - 失败统一抛 ApiError，带 status / missing / 后端原始 data
 *   - 后端没给 error 文案时按状态码兜底，页面不会显示 "undefined"
 *
 * 各业务模块只负责拼 URL 与解析成功响应，不再碰 fetch。
 */

export class ApiError extends Error {
  constructor(message, { status = 0, data = null, missing = false, cause } = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
    this.missing = missing
    if (cause) this.cause = cause
  }
}

const FALLBACK = {
  400: '请求参数有误',
  401: '登录状态已失效，请重新进入后台',
  403: '没有权限执行这个操作',
  404: '资源不存在',
  409: '内容已存在',
  413: '文件超过大小限制',
  429: '操作太频繁，请稍后再试',
  500: '服务器开小差了',
  502: '上游服务暂时不可用',
  503: '服务暂时不可用',
}

/** 构造 JSON 请求参数。FormData（上传）不要用它，浏览器要自己算 boundary。 */
export function json(payload, method = 'POST') {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }
}

/**
 * 发一次 API 请求并解析 JSON。
 * @param {string} path 以 /api 开头的路径
 * @param {{method?: string, body?: any, headers?: object, signal?: AbortSignal, credentials?: string}} options
 * @returns {Promise<object>} 成功响应体
 * @throws {ApiError|Error} 网络异常 / 非 2xx；AbortError 原样抛出供调用方忽略
 */
export async function request(path, { method = 'GET', body, headers, signal, credentials = 'same-origin' } = {}) {
  let res
  try {
    res = await fetch(path, {
      method,
      body,
      headers: { 'X-Music-Client': 'web', ...headers },
      signal,
      credentials,
    })
  } catch (e) {
    if (e?.name === 'AbortError') throw e
    throw new ApiError('网络连接异常，请稍后重试', { cause: e })
  }

  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(data.error || FALLBACK[res.status] || `请求失败 (${res.status})`, {
      status: res.status,
      data,
      missing: !!data.missing,
    })
  }
  return data
}
