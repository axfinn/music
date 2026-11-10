/**
 * 音乐库 API 客户端 —— 全站唯一的网络出入口。
 *
 * 后端契约（backend/routes/musiclib.go）保持不变，这里只负责：
 *   - 统一错误归一化（后端成功响应无 code 字段，错误统一 { error }）
 *   - URL 拼装，尤其是封面 URL 的版本号（后端 /cover/:id 缓存 24h 且 URL 固定，
 *     换封面后必须靠 ?v=<cover_filename> 破缓存）
 *   - 让上层 composable 完全不碰 fetch，方便单测与替换
 */

const API = '/api/music'

/** 统一请求；失败抛 Error，附带 status / missing 供上层做差异化提示 */
async function request(path, { method = 'GET', body, headers, signal } = {}) {
  let res
  try {
    // 管理员会话是 Cookie（HttpOnly），要显式带上才会随请求发送。
    res = await fetch(`${API}${path}`, { method, body, headers, signal, credentials: 'same-origin' })
  } catch (e) {
    if (e?.name === 'AbortError') throw e
    throw new Error('网络连接异常，请稍后重试')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const fallback = {
      400: '请求参数有误',
      404: '资源不存在',
      413: '文件超过大小限制',
      429: '操作太频繁，请稍后再试',
      500: '服务器开小差了',
    }[res.status] || `请求失败 (${res.status})`
    const err = new Error(data.error || fallback)
    err.status = res.status
    err.missing = !!data.missing
    throw err
  }
  return data
}

const json = (payload, method = 'POST') => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(payload),
})

export const AUDIO_EXT = [
  '.mp3', '.wav', '.flac', '.m4a', '.aac', '.ogg', '.oga',
  '.opus', '.webm', '.wma', '.aiff', '.aif', '.ape',
]
export const AUDIO_ACCEPT = AUDIO_EXT.join(',')
export const MAX_AUDIO_BYTES = 200 * 1024 * 1024
export const MAX_COVER_BYTES = 15 * 1024 * 1024
export const COVER_EXT = ['.jpg', '.jpeg', '.png', '.webp', '.gif', '.avif']

export function isAudioFile(file) {
  const name = (file?.name || '').toLowerCase()
  return AUDIO_EXT.some(ext => name.endsWith(ext)) || /^audio\//.test(file?.type || '')
}

export function isImageFile(file) {
  const name = (file?.name || '').toLowerCase()
  return COVER_EXT.some(ext => name.endsWith(ext)) || /^image\//.test(file?.type || '')
}

/** 音频流（后端 ServeContent 原生支持 Range，可直接拖动进度条 / 下载） */
export const streamUrl = id => `${API}/stream/${encodeURIComponent(id)}`

/** 封面；v 用文件名做版本号，换封面立即失效旧缓存 */
export function coverUrl(track) {
  if (!track?.cover_filename) return ''
  const v = encodeURIComponent(track.cover_filename)
  return `${API}/cover/${encodeURIComponent(track.id)}?v=${v}`
}

export const musicApi = {
  async list({ keyword = '', fav = false, sort = 'created', limit = 100, offset = 0, signal } = {}) {
    const p = new URLSearchParams({ limit: String(limit), offset: String(offset), sort })
    if (keyword.trim()) p.set('keyword', keyword.trim())
    if (fav) p.set('fav', '1')
    const data = await request(`/tracks?${p}`, { signal })
    return { items: data.items || [], total: Number(data.total || 0) }
  },

  /**
   * 首屏快照：一次拿齐全量精简曲目 + 统计，替掉以前的逐页补齐。
   * rev 是上一次返回的 revision；数据与筛选都没变时后端只回 not_modified。
   */
  async snapshot({ keyword = '', fav = false, sort = 'created', rev = '', signal } = {}) {
    const p = new URLSearchParams({ sort })
    if (keyword.trim()) p.set('keyword', keyword.trim())
    if (fav) p.set('fav', '1')
    if (rev) p.set('rev', rev)
    const data = await request(`/snapshot?${p}`, { signal })
    if (data.not_modified) return { notModified: true, revision: data.revision || rev }
    return {
      items: data.items || [],
      total: Number(data.total || 0),
      stats: data.stats || null,
      revision: data.revision || '',
      truncated: !!data.truncated,
    }
  },

  /** 列表是精简形态，歌词 / 备注正文按需单独取详情 */
  detail: id => request(`/tracks/${encodeURIComponent(id)}`).then(d => d.item || null),

  stats: () => request('/stats').then(d => d.stats || null),

  async upload(file, meta = {}) {
    const fd = new FormData()
    fd.append('file', file)
    if (meta.title) fd.append('title', meta.title)
    if (meta.artist) fd.append('artist', meta.artist)
    if (meta.album) fd.append('album', meta.album)
    if (meta.duration > 0) fd.append('duration', String(meta.duration))
    return request('/tracks', { method: 'POST', body: fd }).then(d => d.item)
  },

  update(id, patch) {
    return request(`/tracks/${encodeURIComponent(id)}`, json(patch, 'PATCH')).then(d => d.item)
  },

  uploadCover(id, file) {
    const fd = new FormData()
    fd.append('cover', file)
    return request(`/tracks/${encodeURIComponent(id)}/cover`, { method: 'POST', body: fd })
      .then(d => d.cover_filename)
  },

  favorite: (id, favorite) =>
    request(`/tracks/${encodeURIComponent(id)}/favorite`, json({ favorite }))
      .then(d => !!d.favorite),

  played: id => request(`/tracks/${encodeURIComponent(id)}/played`, { method: 'POST' }),

  remove: id => request(`/tracks/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // ── 歌词库关联（后台操作，访客无权）──
  lyricCandidates: (id, q) =>
    request(`/tracks/${encodeURIComponent(id)}/lyric-match?q=${encodeURIComponent(q || '')}`)
      .then(d => d.items || []),

  linkLyric: (id, lyricSongId) =>
    request(`/tracks/${encodeURIComponent(id)}/lyrics`, json({ lyric_song_id: lyricSongId }))
      .then(d => d.item),

  unlinkLyric: id => request(`/tracks/${encodeURIComponent(id)}/lyrics`, { method: 'DELETE' }),

  autoMatchOne: id => request(`/tracks/${encodeURIComponent(id)}/lyrics/auto`, { method: 'POST' }),

  autoMatchAll: () => request('/lyrics/auto-match', { method: 'POST' }),

  repair: () => request('/maintenance/repair', { method: 'POST' }),

  /**
   * 这首歌的歌词（公开只读）。
   *
   * 歌词库本身是后台模块，但「正在听的这一首的歌词」属于听歌本身，
   * 所以走音乐库这边的只读口子，后端会优先给关联到歌词库的版本。
   */
  trackLyrics: id =>
    request(`/tracks/${encodeURIComponent(id)}/lyrics`).then(d => ({
      text: d.text || '',
      source: d.source || 'local',
    })),
}
