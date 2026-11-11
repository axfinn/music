import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { useLibraryStore } from './useLibraryStore.js'

const { ElMessage, musicApi, firstPage, allTracks, stats } = vi.hoisted(() => {
  const ElMessage = Object.assign(vi.fn(), {
    error: vi.fn(), success: vi.fn(), warning: vi.fn(),
  })
  const stats = { total_count: 8, favorite_count: 1, total_duration: 100, total_bytes: 1000, total_plays: 20 }
  const allTracks = Array.from({ length: 8 }, (_, i) => ({
    id: `mus_${i}`, title: `曲目 ${i}`, artist: '', album: '', cover_filename: '',
    playable: true, favorite: i === 0, play_count: i,
    created_at: `2026-01-0${8 - i}T00:00:00+08:00`,
  }))
  const firstPage = allTracks.slice(0, 3)
  const musicApi = {
    list: vi.fn(),
    snapshot: vi.fn(),
    stats: vi.fn(),
  }
  return { ElMessage, musicApi, firstPage, allTracks, stats }
})

vi.mock('element-plus', () => ({ ElMessage }))
vi.mock('../api.js', () => ({ musicApi }))

const snapshotData = () => ({
  items: allTracks, total: allTracks.length, stats, revision: 'r1', truncated: false,
})

let scope, store

beforeEach(() => {
  vi.clearAllMocks()
  musicApi.list.mockResolvedValue({ items: firstPage, total: allTracks.length, stats, revision: 'r1' })
  musicApi.snapshot.mockResolvedValue(snapshotData())
  musicApi.stats.mockResolvedValue(stats)
  scope = effectScope()
  store = scope.run(() => useLibraryStore())
})

afterEach(() => {
  scope?.stop()
})

describe('首屏两段式加载', () => {
  it('第一页到手就解锁页面，不等全量快照', async () => {
    let releaseSnapshot
    musicApi.snapshot.mockImplementation(() => new Promise(resolve => { releaseSnapshot = resolve }))

    const pending = store.load()
    await vi.waitFor(() => expect(store.loading.value).toBe(false))

    expect(store.tracks.value).toEqual(firstPage)
    expect(store.total.value).toBe(allTracks.length)
    expect(musicApi.list).toHaveBeenCalledTimes(1)
    expect(musicApi.list.mock.calls[0][0]).toMatchObject({ limit: 100, offset: 0, withStats: true })
    await vi.waitFor(() => expect(musicApi.snapshot).toHaveBeenCalled())
    // 不能把 /tracks 的 revision 传给 /snapshot，否则首屏永远拿不到全量
    expect(musicApi.snapshot.mock.calls[0][0].rev).toBe('')

    releaseSnapshot(snapshotData())
    await pending
    expect(store.tracks.value).toEqual(allTracks)
  })

  it('?play= 的 ready 信号仍然等全量到位', async () => {
    let releaseSnapshot
    musicApi.snapshot.mockImplementation(() => new Promise(resolve => { releaseSnapshot = resolve }))
    let ready = false
    store.whenReady().then(() => { ready = true })

    const pending = store.load()
    await vi.waitFor(() => expect(store.loading.value).toBe(false))
    expect(ready).toBe(false)

    releaseSnapshot(snapshotData())
    await pending
    expect(ready).toBe(true)
  })

  it('全量快照固定按 created 取，切排序零请求且本地重排', async () => {
    await store.load()
    expect(musicApi.snapshot.mock.calls[0][0].sort).toBe('created')

    const calls = musicApi.list.mock.calls.length + musicApi.snapshot.mock.calls.length
    store.setSortBy('plays')
    await nextTick()

    expect(musicApi.list.mock.calls.length + musicApi.snapshot.mock.calls.length).toBe(calls)
    expect(store.visibleTracks.value[0].play_count).toBe(7)
  })

  it('同一筛选 reload 命中 not_modified 时保持全量，不退回第一页', async () => {
    await store.load()
    expect(store.tracks.value.length).toBe(allTracks.length)

    musicApi.snapshot.mockResolvedValue({ notModified: true, revision: 'r1' })
    await store.reload()

    expect(store.tracks.value.length).toBe(allTracks.length)
    expect(musicApi.list).toHaveBeenCalledTimes(1) // 缓存命中，连第一页都不用拉
  })

  it('旧后端没有 /snapshot 时退回逐页补齐，首屏不白屏', async () => {
    musicApi.snapshot.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
    musicApi.list.mockImplementation(async ({ offset = 0 } = {}) => (
      offset === 0
        ? { items: firstPage, total: allTracks.length, stats }
        : { items: allTracks.slice(3), total: allTracks.length }
    ))

    await store.load()
    await vi.waitFor(() => expect(store.tracks.value.length).toBe(allTracks.length))

    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('后台快照 5xx 时保留第一页，不弹错也不打出一串分页请求', async () => {
    musicApi.snapshot.mockRejectedValue(new Error('500'))

    await store.load()

    expect(store.tracks.value).toEqual(firstPage)
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(musicApi.list).toHaveBeenCalledTimes(1)
  })

  it('首屏轻量请求失败时才报错，并放行 ready 不挂死', async () => {
    musicApi.list.mockRejectedValue(new Error('网络异常'))

    await store.load()

    expect(ElMessage.error).toHaveBeenCalled()
    expect(store.error.value).toBe('网络异常')
    expect(musicApi.snapshot).not.toHaveBeenCalled()
    await expect(store.whenReady()).resolves.toBeUndefined()
  })
})
