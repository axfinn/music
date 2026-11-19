import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMediaPlayer } from './useMediaPlayer.js'

// 播放器是全局单例 <audio> 的封装,这里用最小替身替代浏览器实现:
// 不加载网络、可手动模拟 error 事件与 play() 被浏览器拒绝。
class FakeAudio {
  constructor(src) {
    this.src = src
    this.volume = 1
    this.currentTime = 0
    this.duration = 0
    this._handlers = {}
  }
  addEventListener(type, fn) {
    (this._handlers[type] ||= []).push(fn)
  }
  removeEventListener(type, fn) {
    this._handlers[type] = (this._handlers[type] || []).filter(f => f !== fn)
  }
  pause() {}
  play() {
    if (globalThis.__rejectPlay) {
      const err = new Error('blocked by autoplay policy')
      err.name = 'NotAllowedError'
      return Promise.reject(err)
    }
    if (globalThis.__failAudio) {
      // 每首最多失败一次,防止旧实现的无限连跳把测试卡死
      if (FakeAudio.failCount++ < 10) {
        Promise.resolve().then(() => (this._handlers.error || []).forEach(fn => fn()))
      }
      return new Promise(() => {}) // 加载失败:play() 不 resolve
    }
    return Promise.resolve()
  }
}
FakeAudio.failCount = 0

const track = n => ({
  id: `mus_${n}`,
  title: `测试曲目 ${n}`,
  primaryAsset: { kind: 'audio', asset_url: `/api/music/stream/mus_${n}` },
})
const tracks = n => Array.from({ length: n }, (_, i) => track(i))

const player = useMediaPlayer()

beforeEach(() => {
  vi.stubGlobal('Audio', FakeAudio)
  globalThis.__rejectPlay = false
  globalThis.__failAudio = false
  FakeAudio.failCount = 0
  player.stop()
  player.setShuffle(false)
  player.setLoopMode('all')
  player.state.error = ''
})

afterEach(() => {
  globalThis.__rejectPlay = false
  globalThis.__failAudio = false
  player.stop()
  vi.unstubAllGlobals()
})

describe('随机播放队列', () => {
  it('洗牌序列是完整排列,沿队列走一圈不重不漏也不越界', () => {
    player.setShuffle(true)
    player.playAll(tracks(5), 2)

    expect(player.state.playlist).toHaveLength(5)
    expect(player.state.shuffleOrder).toHaveLength(5)
    expect(new Set(player.state.shuffleOrder).size).toBe(5)
    expect(player.state.shuffleOrder[0]).toBe(2) // 起点固定在最前

    const seen = new Set([player.state.currentIndex])
    for (let i = 0; i < 4; i++) {
      player.skipNext()
      expect(player.state.currentIndex).toBeGreaterThanOrEqual(0)
      expect(player.state.currentIndex).toBeLessThan(5)
      seen.add(player.state.currentIndex)
    }
    expect(seen.size).toBe(5)
  })

  it('随机模式下点播队列外的单曲,洗牌序列会跟着收缩到 1 首', () => {
    player.setShuffle(true)
    player.playAll(tracks(5), 0)
    expect(player.state.shuffleOrder).toHaveLength(5)

    player.play(track(99))

    expect(player.state.playlist).toHaveLength(1)
    expect(player.state.shuffleOrder).toHaveLength(1)
    player.skipNext()
    expect(player.state.currentIndex).toBe(0)
    expect(player.state.playlist).toHaveLength(1)
  })

  it('过期的洗牌序列(下标越界)会被自愈,切歌不会静默失败', () => {
    player.setShuffle(true)
    player.playAll(tracks(5), 0)
    // 模拟队列已被替换、旧序列却留了下来:全是越界下标
    player.state.shuffleOrder = [9, 8, 7, 6, 5]

    player.skipNext()

    expect(player.state.currentIndex).not.toBe(0) // 确实切走了
    expect(player.state.currentIndex).toBeGreaterThanOrEqual(0)
    expect(player.state.currentIndex).toBeLessThan(5)
    expect(player.state.playlist).toHaveLength(5)
  })

  it('replaceQueue 扩展队列时保持正在播的曲目、不中断播放', () => {
    player.setShuffle(true)
    player.playAll(tracks(5), 2)
    const currentId = player.state.playlist[player.state.currentIndex].id
    const audioBefore = player.getAudioEl()

    expect(player.replaceQueue(tracks(9), { shuffle: true })).toBe(true)

    expect(player.state.playlist).toHaveLength(9)
    expect(player.state.playlist[player.state.currentIndex].id).toBe(currentId)
    expect(player.getAudioEl()).toBe(audioBefore) // 没有重建 <audio>
    expect(player.state.shuffleOrder).toHaveLength(9)
    expect(new Set(player.state.shuffleOrder).size).toBe(9)
    expect(player.state.shuffleOrder[0]).toBe(player.state.currentIndex)

    player.skipNext()
    expect(player.state.currentIndex).toBeGreaterThanOrEqual(0)
    expect(player.state.currentIndex).toBeLessThan(9)
  })

  it('不循环模式播到队尾只停下,不把整条队列清空', async () => {
    player.setShuffle(true)
    player.setLoopMode('off')
    player.playAll(tracks(5), 0)
    await new Promise(resolve => setTimeout(resolve, 0)) // 等 play() 的成功回调落地
    // 让当前曲落在洗牌序列最后一位
    player.state.shuffleOrder = [1, 2, 3, 4, 0]

    player.skipNext()

    expect(player.state.isPlaying).toBe(false)
    expect(player.state.playlist).toHaveLength(5)
    expect(player.state.currentIndex).toBe(0) // 停在当前曲,可再点播放键重听
  })

  it('播放被浏览器拦截时留下可读原因,不再静默', async () => {
    globalThis.__rejectPlay = true
    player.playAll(tracks(3), 0)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(player.state.isPlaying).toBe(false)
    expect(player.state.error).toContain('浏览器拦截')
  })

  it('连续坏歌只跳有限次并保留队列,不会一路跳到清空', async () => {
    globalThis.__failAudio = true
    player.setLoopMode('all')
    player.playAll(tracks(5), 0)
    await new Promise(resolve => setTimeout(resolve, 20))

    expect(FakeAudio.failCount).toBeLessThanOrEqual(3)
    expect(player.state.playlist).toHaveLength(5)
    expect(player.state.error).toContain('连续多首')
  })
})

describe('倍速播放', () => {
  it('setRate 写入 state 并作用到当前 <audio>', () => {
    player.playAll(tracks(1), 0)
    const el = player.getAudioEl()
    player.setRate(1.5)
    expect(player.state.playbackRate).toBe(1.5)
    expect(el.playbackRate).toBe(1.5)
  })

  it('切歌后保持已选倍速,新元素也带上 rate', () => {
    player.playAll(tracks(3), 0)
    player.setRate(0.75)
    const oldEl = player.getAudioEl()
    player.skipNext()
    const newEl = player.getAudioEl()
    expect(newEl).not.toBe(oldEl)
    expect(newEl.playbackRate).toBe(0.75)
  })

  it('非法或越界倍速被夹到合法范围', () => {
    player.playAll(tracks(1), 0)
    player.setRate(99)
    expect(player.state.playbackRate).toBeLessThanOrEqual(3)
    player.setRate(-2)
    expect(player.state.playbackRate).toBeGreaterThanOrEqual(0.25)
    player.setRate('abc')
    expect(player.state.playbackRate).toBe(1)
  })

  it('预设档位循环切换,回到 1× 再往外走会 wrap', () => {
    const presets = player.getRatePresets()
    player.playAll(tracks(1), 0)
    player.setRate(1)
    const start = presets.indexOf(1)
    player.setRate(presets[(start + 1) % presets.length])
    expect(player.state.playbackRate).toBe(presets[(start + 1) % presets.length])
  })
})
