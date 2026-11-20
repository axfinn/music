import { describe, it, expect } from 'vitest'
import {
  LYRIC_STYLES, RANDOM_STYLE_ID, MAX_PARTICLE_STYLE, getLyricStyle, normalizeLyricStyle,
  resolveLyricStyle, nextLyricStyle, getLyricPool,
} from './lyricStyles.js'

const ids = LYRIC_STYLES.map(s => s.id)

describe('歌词特效风格表', () => {
  it('id 唯一，粒子/文字/物理三类渲染器都有', () => {
    expect(new Set(ids).size).toBe(ids.length)
    expect(LYRIC_STYLES.some(s => s.renderer === 'particles')).toBe(true)
    expect(LYRIC_STYLES.some(s => s.renderer === 'text')).toBe(true)
    expect(LYRIC_STYLES.some(s => s.renderer === 'physics')).toBe(true)
    expect(ids).toContain('classic')
    // 三个立体特效（漩涡/丝绸波/心跳）必须都在，且都走粒子渲染器
    for (const id of ['vortex', 'wave', 'pulse']) {
      expect(ids).toContain(id)
      expect(getLyricStyle(id).renderer).toBe('particles')
    }
  })

  it('粒子风格的参数齐全且落在 shader 支持的区间', () => {
    for (const s of LYRIC_STYLES.filter(x => x.renderer === 'particles')) {
      expect(s.style).toBeGreaterThanOrEqual(0)
      expect(s.style).toBeLessThanOrEqual(MAX_PARTICLE_STYLE)
      expect(s.pointSize).toBeGreaterThan(0)
      expect(s.opacity).toBeGreaterThan(0)
      expect(s.opacity).toBeLessThanOrEqual(1)
      expect(s.arc).toBeGreaterThan(0)
      expect(s.sway).toBeGreaterThanOrEqual(0)
      // 0 = 不拼歌名(落沙/爆炸),其余要在合理拍数区间
      expect(s.idleBeats).toBeGreaterThanOrEqual(0)
      if (s.idleBeats > 0) expect(s.idleBeats).toBeGreaterThanOrEqual(10)
    }
  })

  it('文字风格带动画类型', () => {
    for (const s of LYRIC_STYLES.filter(x => x.renderer === 'text')) {
      expect(['bounce', 'paragraph', 'marquee']).toContain(s.effect)
    }
  })

  it('每种风格都有 label 和 hint，物理风格不缺渲染器归属', () => {
    for (const s of LYRIC_STYLES) {
      expect(s.label).toBeTruthy()
      expect(s.hint).toBeTruthy()
      expect(['particles', 'text', 'physics']).toContain(s.renderer)
    }
  })

  it('每种风格都标了可读性与节奏，高风险仅限三个氛围玩法', () => {
    for (const s of LYRIC_STYLES) {
      expect(typeof s.readable).toBe('boolean')
      expect(['fast', 'slow', 'any']).toContain(s.tempo)
    }
    const risky = LYRIC_STYLES.filter(s => !s.readable).map(s => s.id).sort()
    expect(risky).toEqual(['explode', 'fall', 'gravity'])
  })
})

describe('风格解析', () => {
  it('具体 id 原样返回，未知值回退经典', () => {
    expect(getLyricStyle('orbit').id).toBe('orbit')
    expect(getLyricStyle('不存在的').id).toBe('classic')
    expect(normalizeLyricStyle('bounce')).toBe('bounce')
    expect(normalizeLyricStyle('乱写')).toBe('classic')
  })

  it('random 是一种可选模式而不是具体风格', () => {
    expect(normalizeLyricStyle(RANDOM_STYLE_ID)).toBe(RANDOM_STYLE_ID)
    expect(ids).not.toContain(RANDOM_STYLE_ID)
  })
})

describe('随机与循环', () => {
  it('具体风格不经随机直接返回', () => {
    expect(resolveLyricStyle('ripple', { previous: 'ripple' })).toBe('ripple')
  })

  it('random 每次都抽具体风格，且避开上一首', () => {
    for (const r of [0, 0.01, 0.5, 0.999999]) {
      const picked = resolveLyricStyle(RANDOM_STYLE_ID, { previous: 'classic', rng: () => r })
      expect(ids).toContain(picked)
      expect(picked).not.toBe('classic')
    }
  })

  it('脏 rng 不会让数组越界', () => {
    for (const bad of [NaN, -5, 9, undefined]) {
      const picked = resolveLyricStyle(RANDOM_STYLE_ID, { rng: () => bad })
      expect(ids).toContain(picked)
    }
  })

  it('随机池只含可读风格，按快慢歌收窄且不会抽空', () => {
    const all = getLyricPool()
    expect(all.length).toBeGreaterThan(0)
    expect(all.every(s => s.readable)).toBe(true)
    expect(all.map(s => s.id)).not.toContain('fall')
    expect(all.map(s => s.id)).not.toContain('explode')
    expect(all.map(s => s.id)).not.toContain('gravity')

    const fast = getLyricPool('fast')
    const slow = getLyricPool('slow')
    expect(fast.length).toBeGreaterThan(0)
    expect(slow.length).toBeGreaterThan(0)
    expect(fast.every(s => s.tempo === 'fast' || s.tempo === 'any')).toBe(true)
    expect(slow.every(s => s.tempo === 'slow' || s.tempo === 'any')).toBe(true)
    expect(fast.map(s => s.id)).toContain('bounce')
    expect(slow.map(s => s.id)).toContain('paragraph')
  })

  it('random 按节奏只在对应池里抽，并规避难读特效', () => {
    for (let i = 0; i < 40; i++) {
      const fast = resolveLyricStyle(RANDOM_STYLE_ID, { tempo: 'fast', rng: () => i / 40 })
      const slow = resolveLyricStyle(RANDOM_STYLE_ID, { tempo: 'slow', rng: () => i / 40 })
      expect(getLyricPool('fast').map(s => s.id)).toContain(fast)
      expect(getLyricPool('slow').map(s => s.id)).toContain(slow)
      expect(['fall', 'explode', 'gravity']).not.toContain(fast)
      expect(['fall', 'explode', 'gravity']).not.toContain(slow)
    }
  })

  it('手工循环能依次走完所有风格并回到开头', () => {
    const seen = []
    let cur = ids[0]
    for (let i = 0; i < ids.length; i++) {
      cur = nextLyricStyle(cur)
      seen.push(cur)
    }
    expect(seen).toEqual(ids.slice(1).concat(ids[0]))
    expect(nextLyricStyle('乱写')).toBe(ids[0])
  })
})
