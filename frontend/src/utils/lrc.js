// LRC 解析与同步算法(纯函数)。
// 同时被 MusicLyricsPlayer.vue(组件运行时)与 MusicLibraryTool.vue(歌词抽屉/Now Playing)复用,
// 从 .vue 文件里抽出来,避免跨文件 import <script> 导出的脆弱写法,也方便单独测试。

const LRC_TAG = /^\[(\d{1,2}):(\d{1,2})(?:[.:](\d{1,3}))?\]\s*/
const LRC_META = /^\[(ti|ar|al|by|offset|length):.*\]$/i
// 句末标点 — 当原文没有 \n 且也没有 LRC 时间戳时,按这些标点把长串切成行
const SENTENCE_BREAK = /[。！？!?；;]+|\n+/g

export function parseLyrics(text) {
  if (!text || typeof text !== 'string') return []
  const out = []
  const raw = text.split(/\r?\n/)
  for (const line of raw) {
    const trimmed = line.trim()
    if (!trimmed) continue
    if (LRC_META.test(trimmed)) continue

    let rest = trimmed
    const times = []
    while (true) {
      const m = rest.match(LRC_TAG)
      if (!m) break
      const min = parseInt(m[1], 10)
      const sec = parseInt(m[2], 10)
      let ms = 0
      if (m[3]) {
        const padded = m[3].padEnd(3, '0').slice(0, 3)
        ms = parseInt(padded, 10)
      }
      times.push(min * 60 + sec + ms / 1000)
      rest = rest.slice(m[0].length)
    }
    const body = rest.trim()
    if (!body) continue
    if (times.length > 0) {
      for (const t of times) out.push({ time: t, text: body })
    } else {
      out.push({ time: null, text: body })
    }
  }

  // 如果整篇没有换行、也没有 LRC 标签,试按句末标点切;支持一些歌词被打成单行的情况
  if (out.length <= 1 && text && !text.includes('\n') && !LRC_TAG.test(text.trim())) {
    const sentences = text
      .split(SENTENCE_BREAK)
      .map(s => s.trim())
      .filter(Boolean)
    if (sentences.length > 1) {
      return sentences.map(text => ({ time: null, text }))
    }
  }

  const hasAnyTime = out.some(l => l.time !== null)
  if (hasAnyTime) {
    out.sort((a, b) => {
      if (a.time === null) return 1
      if (b.time === null) return -1
      return a.time - b.time
    })
  }
  return out
}

export function detectMode(lines) {
  if (!lines || lines.length === 0) return 'linear'
  const withTime = lines.filter(l => l.time !== null).length
  return withTime / lines.length >= 0.3 ? 'lrc' : 'linear'
}

// 找当前行:LRC 模式用真实时间戳,线性模式按 currentTime / duration 等分。
// 返回 -1 表示"当前时间还没到第一行"(例:纯 LRC 还没到第一句)。
export function findActiveIndex(lines, currentTime, duration, mode) {
  if (!lines || lines.length === 0) return -1
  if (mode === 'lrc') {
    const timed = lines.filter(l => l.time !== null)
    if (timed.length === 0) {
      return Math.min(Math.max(0, Math.floor((currentTime / Math.max(duration, 1)) * lines.length)), lines.length - 1)
    }
    if (currentTime < timed[0].time) return -1
    // 最后一个时间戳之后:停在最后一行
    if (currentTime >= timed[timed.length - 1].time) {
      return lines.lastIndexOf(timed[timed.length - 1])
    }
    let idx = -1
    for (let i = 0; i < lines.length; i++) {
      if (lines[i].time !== null && lines[i].time <= currentTime) idx = i
    }
    return idx
  }
  // linear
  if (!duration || duration <= 0) return 0
  if (currentTime >= duration) return lines.length - 1
  if (currentTime <= 0) return 0
  return Math.min(Math.floor((currentTime / duration) * lines.length), lines.length - 1)
}

// 给每行算一个"点击跳到这里"的秒数。
// LRC 模式:用真实时间戳;线性模式:把整首歌按时长平均分给每行。
export function computeSeekTime(line, index, total, duration, mode) {
  if (mode === 'lrc' && line.time !== null) return line.time
  if (mode === 'linear' && duration > 0 && total > 0) {
    return (index / total) * duration
  }
  return 0
}

export function formatLyricsTime(s) {
  if (!s || !isFinite(s) || s < 0) return '0:00'
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}
