/**
 * 纯格式化工具：无 Vue / DOM 依赖，可直接单测。
 */

export function fmtTime(seconds) {
  const s = Number(seconds)
  if (!isFinite(s) || s < 0) return '0:00'
  const total = Math.floor(s)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const sec = total % 60
  return h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
    : `${m}:${String(sec).padStart(2, '0')}`
}

/** 总时长给人看的粗粒度表达：3 小时 24 分 / 24 分钟 / 不到 1 分钟 */
export function fmtDurationHuman(seconds) {
  const s = Number(seconds) || 0
  const h = Math.floor(s / 3600)
  const m = Math.round((s % 3600) / 60)
  if (h > 0) return `${h} 小时 ${m} 分`
  if (m > 0) return `${m} 分钟`
  return '不到 1 分钟'
}

export function humanSize(bytes) {
  const n = Number(bytes) || 0
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`
  if (n >= 1024) return `${Math.round(n / 1024)} KB`
  return `${n} B`
}

export function fmtCount(n) {
  const v = Number(n) || 0
  return v >= 10000 ? `${(v / 10000).toFixed(1)} 万` : String(v)
}

/** 封面首字：中文取首字，英文取首字母，其余兜底音符 */
export function initialOf(text) {
  const ch = String(text || '').trim().charAt(0)
  return ch ? ch.toUpperCase() : '♪'
}

export function formatDate(value) {
  if (!value) return ''
  const d = new Date(value)
  if (isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function relativeDate(value) {
  if (!value) return ''
  const d = new Date(value)
  if (isNaN(d.getTime())) return ''
  const days = Math.floor((Date.now() - d.getTime()) / 86400000)
  if (days <= 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return `${days} 天前`
  if (days < 30) return `${Math.floor(days / 7)} 周前`
  return formatDate(value)
}

export const clamp = (v, min, max) => Math.min(max, Math.max(min, v))

/** 32-bit FNV-1a，用来把任意字符串映射成稳定的视觉参数 */
export function hashOf(str) {
  let h = 0x811c9dc5
  const s = String(str || '')
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

/**
 * 无封面曲目的确定性艺术封面：同一首歌永远得到同一套配色。
 * 返回 CSS 可用的颜色与渐变，避免「一水儿灰色方块」的廉价感。
 */
export function toneFor(seed) {
  const h = hashOf(seed) % 360
  const h2 = (h + 42 + (hashOf(`b:${seed}`) % 60)) % 360
  return {
    hue: h,
    from: `hsl(${h} 62% 52%)`,
    to: `hsl(${h2} 66% 28%)`,
    soft: `hsl(${h} 58% 46%)`,
    gradient: `linear-gradient(145deg, hsl(${h} 62% 54%) 0%, hsl(${h2} 64% 30%) 100%)`,
  }
}

export function pluralTracks(n) {
  return `${n} 首`
}
