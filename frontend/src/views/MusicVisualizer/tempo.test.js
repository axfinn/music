import { describe, it, expect } from 'vitest'
import {
  estimateBpm, classifyTempo, tempoFromOnsets, createTempoDetector,
} from './tempo.js'

/** 按固定 BPM 造一串拍点(秒) */
function beats(bpm, count, start = 0) {
  const gap = 60 / bpm
  return Array.from({ length: count }, (_, i) => start + i * gap)
}

describe('节奏检测', () => {
  it('从均匀拍点估出 BPM 并分快慢', () => {
    const fast = tempoFromOnsets(beats(140, 10))
    expect(fast.bpm).toBeGreaterThanOrEqual(130)
    expect(fast.bpm).toBeLessThanOrEqual(150)
    expect(fast.tempo).toBe('fast')
    expect(fast.confidence).toBeGreaterThan(0.6)

    const slow = tempoFromOnsets(beats(72, 10))
    expect(slow.bpm).toBeGreaterThanOrEqual(68)
    expect(slow.bpm).toBeLessThanOrEqual(78)
    expect(slow.tempo).toBe('slow')
  })

  it('落在中间区间的 BPM 不强行贴标签', () => {
    // estimateBpm 把 BPM 折进 76..168,中段留给「不确定」→ 随机走全池
    expect(classifyTempo(110, 1)).toBe('')
  })

  it('拍点不够或间隔抖动太大时置信度低,不做判断', () => {
    expect(estimateBpm([]).bpm).toBe(0)
    expect(estimateBpm([0, 0.5]).bpm).toBe(0)
    const jitter = [0, 0.5, 1.7, 1.95, 3.9, 5.1, 7.2]
    const r = estimateBpm(jitter)
    expect(r.confidence).toBeLessThan(0.4)
    expect(classifyTempo(r.bpm, r.confidence)).toBe('')
  })

  it('真实慢歌不被折回成快歌', () => {
    // 72BPM 的 0.83s 间隔落在有效区间,必须保持慢歌标签
    const r = tempoFromOnsets(beats(72, 10))
    expect(r.bpm).toBeGreaterThanOrEqual(68)
    expect(r.bpm).toBeLessThanOrEqual(78)
    expect(r.tempo).toBe('slow')
  })

  it('超出有效间隔的极端估计被折回', () => {
    // 4s 一个 onset 超出上限被过滤 → 数据不足(不会误折 15BPM 成 240)
    expect(estimateBpm(beats(15, 8)).bpm).toBe(0)
    // 0.1s 一个 onset 低于下限被过滤
    expect(estimateBpm(beats(600, 8)).bpm).toBe(0)
  })

  it('脏数据不抛错', () => {
    expect(() => estimateBpm([0, NaN, 2, Infinity, 3, 4, 5])).not.toThrow()
    expect(classifyTempo(NaN, 1)).toBe('')
    expect(classifyTempo(140, 0.99)).toBe('fast')
    expect(classifyTempo(140, 0.1)).toBe('') // 置信度不足不判断
  })
})

describe('节奏检测器', () => {
  it('没有分析器时安全降级', () => {
    const d = createTempoDetector({ getAnalyser: () => null })
    const s = d.sample(0.12)
    expect(s.tempo).toBe('')
    expect(s.bpm).toBe(0)
    d.reset()
  })

  it('reset 清空历史', () => {
    const d = createTempoDetector({ getAnalyser: () => null })
    d.sample(0.12)
    d.reset()
    expect(d.state.beats).toBe(0)
    expect(d.state.tempo).toBe('')
  })
})
