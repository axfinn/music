// 节奏检测 —— 给「随机歌词特效」判断这首歌是快歌还是慢歌。
//
// 为什么自己写:
//   歌词特效的随机池要按节奏收窄(快歌抽跳动/心跳/弹幕,慢歌抽星轨/丝绸波/段落),
//   只需要一个粗粒度 fast/slow 标签,不需要精确 BPM。引入 Web Audio 节拍库
//   (web-audio-beat-detector 等)要整段解码音频,代价太大;这里从现成的
//   AnalyserNode 低频能量做瞬态检测,每 100~150ms 采一次即可,零额外上下文。
//
// 思路:
//   1. 取低频(≤180Hz)能量,和最近 ~3.6s 的均值比,越过阈值且过了冷却 → 一个 onset;
//   2. onset 间隔落在 0.25~1.25s(48~240BPM)的才算拍,取中位数得 BPM;
//   3. 把 BPM 折进 76~168 的常用区间,消掉半拍/双倍误判;
//   4. 间隔数量 + 离散度给置信度,不够稳就返回 tempo=''(随机走全池)。
//
// 纯函数 estimateBpm / classifyTempo 单独导出,便于 vitest 用合成拍点测。
// 检测器本身不碰 rAF:页面用一个只在「播放中 + 随机模式」才跑的定时器喂 sample(dt)。

const MIN_GAP = 0.26          // 两个 onset 的最小间隔(s),短于此是同一个鼓点的余波
const MIN_INTERVAL = 0.25     // 有效拍间隔下限(240BPM)
const MAX_INTERVAL = 1.25     // 有效拍间隔上限(48BPM)
const MAX_ONSETS = 20
const WINDOW = 14             // 只保留最近 14s 的 onset,切歌/变速后不会被旧拍拖住
const HIST = 36               // 低频能量滑动窗口(按 100ms 采样 ≈ 3.6s)
// 折回边界必须和有效间隔区间对齐(60/1.25=48, 60/0.25=240),否则会把
// 72BPM 这种真实慢歌(间隔 0.83s,本来完全有效)翻倍成 144 的快歌。
const BPM_MIN = 48
const BPM_MAX = 240
const CONF_MIN = 0.4          // 低于这个置信度不按节奏挑池

const clamp01 = v => (v < 0 ? 0 : v > 1 ? 1 : v)

/**
 * 从一串 onset 时间戳估 BPM。时间单位秒,须单调递增。
 * @returns {{bpm:number, confidence:number}} bpm=0 表示数据不够
 */
export function estimateBpm(times) {
  if (!Array.isArray(times) || times.length < 4) return { bpm: 0, confidence: 0 }
  const gaps = []
  for (let i = 1; i < times.length; i++) {
    const g = times[i] - times[i - 1]
    if (Number.isFinite(g) && g >= MIN_INTERVAL && g <= MAX_INTERVAL) gaps.push(g)
  }
  if (gaps.length < 3) return { bpm: 0, confidence: 0 }

  const sorted = [...gaps].sort((a, b) => a - b)
  const median = sorted[sorted.length >> 1]
  let bpm = 60 / median
  // 半拍/漏拍会让 BPM 落在常见区间外,倍频折回来(140 的半拍 280 → 140;70 的双拍 → 140)
  while (bpm < BPM_MIN) bpm *= 2
  while (bpm > BPM_MAX) bpm /= 2

  // 离散度:间隔越整齐,越可能是稳定的拍子而不是随机的鼓点
  let dev = 0
  for (const g of gaps) dev += Math.abs(g - median)
  const spread = dev / gaps.length / median
  const confidence = clamp01(gaps.length / 8) * clamp01(1 - spread * 2.4)
  return { bpm: Math.round(bpm), confidence: clamp01(confidence) }
}

/** BPM → 快/慢标签。置信度不足或拿不到 BPM 时返回 ''(随机走全池)。 */
export function classifyTempo(bpm, confidence = 1) {
  if (!Number.isFinite(bpm) || bpm <= 0 || confidence < CONF_MIN) return ''
  if (bpm >= 124) return 'fast'
  if (bpm <= 96) return 'slow'
  return ''
}

/** 从对齐的拍点序列直接算标签,测试用 */
export function tempoFromOnsets(times) {
  const { bpm, confidence } = estimateBpm(times)
  return { bpm, confidence, tempo: classifyTempo(bpm, confidence) }
}

/**
 * @param {{getAnalyser: () => AnalyserNode|null|undefined}} opts
 */
export function createTempoDetector({ getAnalyser } = {}) {
  let bins = null
  let energy = []
  let onsets = []
  let t = 0
  let lastOnset = -10
  let state = { bpm: 0, confidence: 0, tempo: '', beats: 0 }

  function reset() {
    energy = []
    onsets = []
    t = 0
    lastOnset = -10
    state = { bpm: 0, confidence: 0, tempo: '', beats: 0 }
  }

  /** 每 ~100ms 调一次;dt 是距上次采样的秒数。返回当前节奏状态。 */
  function sample(dt) {
    const an = getAnalyser?.()
    const step = Math.min(Math.max(Number(dt) > 0 ? dt : 0.1, 0.001), 0.2)
    t += step
    if (!an) return state

    if (!bins || bins.length !== an.frequencyBinCount) bins = new Uint8Array(an.frequencyBinCount)
    an.getByteFrequencyData(bins)
    // binHz ≈ 22050 / frequencyBinCount;取 ≤180Hz 的均值当低频能量
    const binHz = 22050 / bins.length
    const hi = Math.max(2, Math.min(bins.length, Math.round(180 / binHz)))
    let sum = 0
    for (let i = 1; i < hi; i++) sum += bins[i]
    const bass = sum / Math.max(1, hi - 1) / 255

    energy.push(bass)
    if (energy.length > HIST) energy.shift()
    let avg = 0
    for (const v of energy) avg += v
    avg /= energy.length

    // 瞬态:低频明显高于近期均值 + 过了冷却 → 记一拍
    if (bass > 0.06 && bass > avg * 1.3 && t - lastOnset >= MIN_GAP) {
      lastOnset = t
      onsets.push(t)
      if (onsets.length > MAX_ONSETS) onsets.shift()
      const est = estimateBpm(onsets)
      state = {
        bpm: est.bpm,
        confidence: est.confidence,
        tempo: classifyTempo(est.bpm, est.confidence),
        beats: onsets.length,
      }
    }
    // 淘汰过期 onset,变速后不会被旧拍拖住
    while (onsets.length && t - onsets[0] > WINDOW) onsets.shift()

    return state
  }

  return { sample, reset, get state() { return state } }
}
