import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useMediaPlayer } from '../../../composables/useMediaPlayer.js'
// 用真实播放器做端到端验证:队列长度、洗牌序列、当前曲都能直接读
import { useMusicPlayback } from './useMusicPlayback.js'

const { ElMessage, musicApi, allTracks } = vi.hoisted(() => {
  const ElMessage = Object.assign(
    vi.fn(() => ({ close: vi.fn() })),
    { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
  )
  const allTracks = Array.from({ length: 8 }, (_, i) => ({
    id: `mus_${i}`, title: `曲目 ${i}`, artist: '', cover_filename: '',
  }))
  const musicApi = {
    snapshot: vi.fn(async () => ({ items: allTracks, total: allTracks.length, revision: 'r1', truncated: false })),
    list: vi.fn(async () => ({ items: [], total: allTracks.length })),
    played: vi.fn(async () => ({})),
  }
  return { ElMessage, musicApi, allTracks }
})
vi.mock('element-plus', () => ({ ElMessage }))
vi.mock('../api.js', () => ({
  musicApi,
  coverUrl: () => '',
  streamUrl: id => `/api/music/stream/${id}`,
}))
vi.mock('../utils/palette.js', () => ({
  extractDominant: vi.fn(async () => null),
  paletteToCss: vi.fn(() => ''),
}))

class FakeAudio {
  constructor(src) { this.src = src; this.volume = 1; this.currentTime = 0; this.duration = 0; this._h = {} }
  addEventListener() {}
  removeEventListener() {}
  pause() {}
  play() { return Promise.resolve() }
}

const player = useMediaPlayer()
let scope, store, playback

beforeEach(() => {
  vi.stubGlobal('Audio', FakeAudio)
  vi.clearAllMocks()
  player.stop()
  player.setShuffle(false)
  player.setLoopMode('all')
  player.state.error = ''
  store = {
    tracks: ref(allTracks),
    visibleTracks: ref(allTracks),
    insights: { value: { recent: [] } },
    hasMore: { value: false },
    ensureComplete: vi.fn(async () => {}),
    refreshStats: vi.fn(),
    whenReady: () => Promise.resolve(),
  }
  scope = effectScope()
  playback = scope.run(() => useMusicPlayback(store, {})) || null
})

afterEach(() => {
  scope?.stop()
  player.stop()
  vi.unstubAllGlobals()
})

describe('playAllVisible 随机播放', () => {
  it('视图只有 1 首:点击同步起播,随后自动扩成全库随机队列', async () => {
    store.visibleTracks.value = [allTracks[3]]

    const pending = playback.playAllVisible({ shuffle: true })
    // 关键:play() 必须已在用户点击的同步栈里发生,不能等网络回来才播
    expect(player.state.playlist).toHaveLength(1)
    expect(player.state.currentIndex).toBe(0)

    await pending
    expect(musicApi.snapshot).toHaveBeenCalled()
    expect(player.state.playlist).toHaveLength(allTracks.length)
    expect(player.state.shuffleOrder).toHaveLength(allTracks.length)
    // 起播的那首仍在播,补齐队列没有打断声音
    expect(player.state.playlist[player.state.currentIndex].id).toBe(allTracks[3].id)
    expect(ElMessage.success).toHaveBeenCalledWith(`已生成 ${allTracks.length} 首的随机列表`)
  })

  it('视图 0 首(搜索无结果):从全库起播,而不是只弹一个警告', async () => {
    store.visibleTracks.value = []

    await playback.playAllVisible({ shuffle: true })

    expect(musicApi.snapshot).toHaveBeenCalled()
    expect(player.state.playlist).toHaveLength(allTracks.length)
    expect(player.state.currentIndex).toBeGreaterThanOrEqual(0)
    expect(ElMessage.warning).not.toHaveBeenCalledWith('当前没有可播放的歌曲')
  })

  it('视图有多首:洗当前视图,不去额外拉全库', async () => {
    store.visibleTracks.value = allTracks.slice(0, 4)

    const pending = playback.playAllVisible({ shuffle: true })
    await pending

    expect(musicApi.snapshot).not.toHaveBeenCalled()
    expect(player.state.playlist).toHaveLength(4)
    expect(player.state.shuffleOrder).toHaveLength(4)
    expect(player.state.shuffleMode).toBe(true)
  })

  it('顺序「播放全部」不开启随机,分页补齐后队列变完整', async () => {
    store.visibleTracks.value = allTracks.slice(0, 2)
    store.hasMore = { value: true }
    store.ensureComplete = vi.fn(async () => { store.visibleTracks.value = allTracks })

    const pending = playback.playAllVisible()
    expect(player.state.playlist).toHaveLength(2) // 同步起播
    await pending

    expect(store.ensureComplete).toHaveBeenCalled()
    expect(player.state.playlist).toHaveLength(allTracks.length)
    expect(player.state.shuffleMode).toBe(false)
  })
})
