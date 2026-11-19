import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { usePlaylistStore } from './usePlaylistStore.js'

const { ElMessage, playlistApi } = vi.hoisted(() => {
  const ElMessage = Object.assign(vi.fn(), {
    error: vi.fn(), success: vi.fn(), warning: vi.fn(),
  })
  return {
    ElMessage,
    playlistApi: {
      list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(),
      addTracks: vi.fn(), removeTrack: vi.fn(), detail: vi.fn(),
    },
  }
})

vi.mock('element-plus', () => ({ ElMessage }))
vi.mock('../api.js', () => ({ playlistApi }))

function memoryStorage() {
  const data = new Map()
  return {
    getItem: k => (data.has(k) ? data.get(k) : null),
    setItem: (k, v) => data.set(k, String(v)),
    removeItem: k => data.delete(k),
    clear: () => data.clear(),
  }
}

let scope, pl, admin, tracks

function setup(adminValue = false, initialLocal = []) {
  const storage = memoryStorage()
  if (initialLocal.length) storage.setItem('music.playlists.v1', JSON.stringify(initialLocal))
  vi.stubGlobal('localStorage', storage)
  scope = effectScope()
  admin = ref(adminValue)
  tracks = ref([
    { id: 'mus_a', title: 'A', duration_sec: 10 },
    { id: 'mus_b', title: 'B', duration_sec: 20 },
  ])
  pl = scope.run(() => usePlaylistStore({ tracks }, { isAdmin: admin }))
  return storage
}

beforeEach(() => {
  vi.clearAllMocks()
  playlistApi.list.mockResolvedValue([])
  playlistApi.detail.mockResolvedValue(null)
})

afterEach(() => {
  scope?.stop()
  vi.unstubAllGlobals()
})

describe('歌单来源', () => {
  it('公开歌单与本机歌单合并成一份列表', async () => {
    setup(false, [{ id: 'local_1', name: '私人', track_ids: ['mus_a'] }])
    playlistApi.list.mockResolvedValue([{ id: 'pls_1', name: '公开', track_count: 0, track_ids: [] }])
    await pl.load()
    expect(pl.items.value.map(p => p.source)).toEqual(['shared', 'local'])
    expect(pl.tracksOf(pl.items.value[1]).map(t => t.id)).toEqual(['mus_a'])
  })

  it('公开歌单列表会补详情，卡片才能显示曲目数', async () => {
    setup(false)
    playlistApi.list.mockResolvedValue([{ id: 'pls_1', name: '公开', track_count: 2 }])
    playlistApi.detail.mockResolvedValue({ id: 'pls_1', name: '公开', track_ids: ['mus_a', 'mus_b'], track_count: 2 })
    await pl.load()
    expect(playlistApi.detail).toHaveBeenCalledWith('pls_1')
    expect(pl.countOf(pl.items.value[0])).toBe(2)
  })

  it('后端不可用时本机歌单仍然可用', async () => {
    setup(false, [{ id: 'local_1', name: '私人', track_ids: [] }])
    playlistApi.list.mockRejectedValue(new Error('404'))
    await pl.load()
    expect(pl.items.value).toHaveLength(1)
    expect(pl.items.value[0].source).toBe('local')
  })
})

describe('创建与编辑', () => {
  it('访客创建的歌单只落本机，不选公开', async () => {
    setup(false)
    await pl.load()
    const item = await pl.create({ name: '夜听', scope: 'shared', track_ids: ['mus_a', 'mus_a'] })
    expect(playlistApi.create).not.toHaveBeenCalled()
    expect(item.source).toBe('local')
    expect(item.track_ids).toEqual(['mus_a'])
    expect(pl.selected.value.id).toBe(item.id)
  })

  it('管理员可以创建公开歌单', async () => {
    setup(true)
    playlistApi.create.mockResolvedValue({ id: 'pls_9', name: '官方', track_ids: [], track_count: 0 })
    await pl.load()
    await pl.create({ name: '官方', scope: 'shared' })
    expect(playlistApi.create).toHaveBeenCalledWith({ name: '官方', description: '', track_ids: [] })
    expect(pl.items.value[0].source).toBe('shared')
  })

  it('访客改不了公开歌单', async () => {
    setup(false)
    playlistApi.list.mockResolvedValue([{ id: 'pls_1', name: '官方', track_ids: [] }])
    await pl.load()
    expect(pl.canEdit(pl.items.value[0])).toBe(false)
    expect(await pl.rename(pl.items.value[0], { name: '改了' })).toBeNull()
  })

  it('增删曲目只记 ID，渲染时跳过曲库里已不存在的曲目', async () => {
    setup(false)
    await pl.load()
    const p = await pl.create({ name: '清单', track_ids: ['mus_a'] })
    await pl.addTracks(p, ['mus_b', 'mus_a', 'mus_gone'])
    const fresh = pl.items.value[0]
    expect(fresh.track_ids).toEqual(['mus_a', 'mus_b', 'mus_gone'])
    expect(pl.tracksOf(fresh).map(t => t.id)).toEqual(['mus_a', 'mus_b'])
    expect(pl.countOf(fresh)).toBe(2)
    await pl.removeTrack(fresh, 'mus_a')
    expect(pl.items.value[0].track_ids).toEqual(['mus_b', 'mus_gone'])
  })
})
