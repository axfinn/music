/**
 * 封面主色提取 —— 用当前播放曲目的封面驱动整页氛围光。
 *
 * 只在浏览器里跑；任何跨域 / 解码失败都静默返回 null，UI 会退回确定性配色。
 * 结果按 URL 缓存（封面 URL 已带文件名版本号，换封面天然失效）。
 */

const CACHE = new Map()
const PENDING = new Map()

function rgbToHsl(r, g, b) {
  const rn = r / 255, gn = g / 255, bn = b / 255
  const max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn)
  const l = (max + min) / 2
  const d = max - min
  if (d === 0) return { h: 0, s: 0, l }
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h
  if (max === rn) h = ((gn - bn) / d + (gn < bn ? 6 : 0)) / 6
  else if (max === gn) h = ((bn - rn) / d + 2) / 6
  else h = ((rn - gn) / d + 4) / 6
  return { h: h * 360, s, l }
}

function dominantFromPixels(data) {
  // 20° 一个色相桶，按「饱和度 × 数量」加权，避免大片纯黑/纯白把主色带跑
  const buckets = new Map()
  for (let i = 0; i < data.length; i += 4) {
    const a = data[i + 3]
    if (a < 140) continue
    const { h, s, l } = rgbToHsl(data[i], data[i + 1], data[i + 2])
    if (l < 0.09 || l > 0.93 || s < 0.12) continue
    const key = Math.floor(h / 20) * 20
    const weight = 0.35 + s
    const b = buckets.get(key) || { weight: 0, h: 0, s: 0, l: 0 }
    b.weight += weight
    b.h += h * weight
    b.s += s * weight
    b.l += l * weight
    buckets.set(key, b)
  }
  let best = null
  for (const b of buckets.values()) if (!best || b.weight > best.weight) best = b
  if (!best || best.weight < 2) return null
  return {
    h: Math.round(best.h / best.weight),
    s: Math.min(0.85, Math.max(0.35, best.s / best.weight)),
    l: 0.55,
  }
}

/** @returns {Promise<{h:number,s:number,l:number}|null>} */
export function extractDominant(url) {
  if (!url || typeof window === 'undefined' || typeof document === 'undefined') return Promise.resolve(null)
  if (CACHE.has(url)) return Promise.resolve(CACHE.get(url))
  if (PENDING.has(url)) return PENDING.get(url)

  const job = new Promise(resolve => {
    const img = new Image()
    let settled = false
    const done = (value) => {
      if (settled) return
      settled = true
      CACHE.set(url, value)
      PENDING.delete(url)
      resolve(value)
    }
    // 封面加载失败 / 卡住不能拖累页面
    const timer = setTimeout(() => done(null), 4000)
    const finish = (v) => { clearTimeout(timer); done(v) }
    img.onload = () => {
      try {
        const canvas = document.createElement('canvas')
        canvas.width = 32
        canvas.height = 32
        const ctx = canvas.getContext('2d', { willReadFrequently: true })
        if (!ctx) return finish(null)
        ctx.drawImage(img, 0, 0, 32, 32)
        const pixels = ctx.getImageData(0, 0, 32, 32).data
        finish(dominantFromPixels(pixels))
      } catch {
        finish(null)
      }
    }
    img.onerror = () => finish(null)
    img.src = url
  })

  PENDING.set(url, job)
  return job
}

/** 把主色换算成页面用的两团柔光与强调色 */
export function paletteToCss(p, fallbackHue = 24) {
  const h = p?.h ?? fallbackHue
  const s = p?.s ?? 0.6
  const hue2 = (h + 42) % 360
  return {
    glowA: `hsla(${h} ${Math.round(s * 100)}% 55% / 0.34)`,
    glowB: `hsla(${hue2} ${Math.round(s * 88)}% 48% / 0.26)`,
    accent: `hsl(${h} ${Math.round(s * 100)}% 58%)`,
    accent2: `hsl(${hue2} ${Math.round(s * 94)}% 62%)`,
    accentSoft: `hsla(${h} ${Math.round(s * 100)}% 58% / 0.16)`,
  }
}
