import { describe, it, expect } from 'vitest'
import {
  fmtTime, fmtDurationHuman, humanSize, fmtCount, initialOf,
  formatDate, relativeDate, clamp, hashOf, toneFor,
} from './format.js'

describe('fmtTime', () => {
  it('常规分秒', () => {
    expect(fmtTime(0)).toBe('0:00')
    expect(fmtTime(9)).toBe('0:09')
    expect(fmtTime(65)).toBe('1:05')
    expect(fmtTime(599)).toBe('9:59')
  })
  it('跨小时用三段式', () => {
    expect(fmtTime(3600)).toBe('1:00:00')
    expect(fmtTime(3661.9)).toBe('1:01:01')
  })
  it('脏数据不炸', () => {
    expect(fmtTime(NaN)).toBe('0:00')
    expect(fmtTime(-5)).toBe('0:00')
    expect(fmtTime(undefined)).toBe('0:00')
    expect(fmtTime('125')).toBe('2:05')
  })
})

describe('fmtDurationHuman', () => {
  it('按粒度切单位', () => {
    expect(fmtDurationHuman(0)).toBe('不到 1 分钟')
    expect(fmtDurationHuman(600)).toBe('10 分钟')
    expect(fmtDurationHuman(12240)).toBe('3 小时 24 分')
  })
})

describe('humanSize / fmtCount', () => {
  it('字节单位换算', () => {
    expect(humanSize(0)).toBe('0 B')
    expect(humanSize(2048)).toBe('2 KB')
    expect(humanSize(2.5 * 1024 ** 2)).toBe('2.5 MB')
    expect(humanSize(2 * 1024 ** 3)).toBe('2.00 GB')
  })
  it('大数缩写', () => {
    expect(fmtCount(9999)).toBe('9999')
    expect(fmtCount(12345)).toBe('1.2 万')
  })
})

describe('initialOf / formatDate / relativeDate', () => {
  it('首字母与兜底', () => {
    expect(initialOf('晴天')).toBe('晴')
    expect(initialOf(' hello ')).toBe('H')
    expect(initialOf('')).toBe('♪')
  })
  it('日期可解析', () => {
    expect(formatDate('2026-11-04T10:00:00Z')).toMatch(/^2026-11-0[34]$/)
    expect(formatDate('garbage')).toBe('')
  })
  it('相对日期分档', () => {
    const day = 86400000
    expect(relativeDate(new Date(Date.now()).toISOString())).toBe('今天')
    expect(relativeDate(new Date(Date.now() - day).toISOString())).toBe('昨天')
    expect(relativeDate(new Date(Date.now() - 3 * day).toISOString())).toBe('3 天前')
    expect(relativeDate('')).toBe('')
  })
})

describe('clamp / hashOf / toneFor', () => {
  it('clamp 边界', () => {
    expect(clamp(5, 0, 1)).toBe(1)
    expect(clamp(-5, 0, 1)).toBe(0)
    expect(clamp(0.4, 0, 1)).toBe(0.4)
  })
  it('hash 稳定且不同输入有区分', () => {
    expect(hashOf('abc')).toBe(hashOf('abc'))
    expect(hashOf('abc')).not.toBe(hashOf('abd'))
  })
  it('同一首歌永远得到同一套封面配色', () => {
    const a = toneFor('mus_123')
    const b = toneFor('mus_123')
    expect(a.gradient).toBe(b.gradient)
    expect(a.gradient).toContain('linear-gradient')
    expect(toneFor('mus_999').hue).toBeGreaterThanOrEqual(0)
  })
})
