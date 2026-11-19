/**
 * 歌单中枢：把「管理员发布的公开歌单」和「访客本机的私有歌单」统一成一份列表。
 *
 * 站点没有访客账号，所以访客歌单只落 localStorage；管理员建的可选公开（落库）
 * 或私有（本机）。UI 只看 source / canEdit，不关心数据在哪。
 *
 * 曲库曲目本身永远不复制进歌单，歌单只存 ID：曲库改名/换封面后歌单自动跟着变。
 */
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { playlistApi } from '../api.js'

const LOCAL_KEY = 'music.playlists.v1'

function readLocal() {
  try {
    const raw = JSON.parse(localStorage.getItem(LOCAL_KEY) || '[]')
    if (!Array.isArray(raw)) return []
    return raw
      .filter(p => p && p.id && typeof p.name === 'string' && Array.isArray(p.track_ids))
      .map(p => normalize(p, 'local'))
  } catch {
    return []
  }
}

function normalize(p, source) {
  const ids = Array.isArray(p.track_ids) ? p.track_ids.filter(Boolean) : []
  return {
    id: p.id,
    name: p.name || '未命名歌单',
    description: p.description || '',
    track_ids: ids,
    track_count: Number(p.track_count ?? ids.length) || 0,
    source,
    created_at: p.created_at || '',
    updated_at: p.updated_at || '',
  }
}

function localID() {
  return 'local_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 8)
}

export function usePlaylistStore(store, { isAdmin } = {}) {
  const items = ref([])
  const loading = ref(false)
  const loaded = ref(false)
  const selectedId = ref('')
  const isAdminUser = () => !!isAdmin?.value

  const selected = computed(() => items.value.find(p => p.id === selectedId.value) || null)
  const publicItems = computed(() => items.value.filter(p => p.source === 'shared'))
  const localItems = computed(() => items.value.filter(p => p.source === 'local'))

  function canEdit(p) {
    return !!p && (p.source === 'local' || isAdminUser())
  }

  function persistLocal() {
    try {
      const plain = localItems.value.map(p => ({
        id: p.id, name: p.name, description: p.description,
        track_ids: p.track_ids, track_count: p.track_ids.length,
        created_at: p.created_at, updated_at: p.updated_at,
      }))
      localStorage.setItem(LOCAL_KEY, JSON.stringify(plain))
    } catch { /* 无痕模式/配额满：本次会话内仍可用 */ }
  }

  /** 后端不可用时仍能用本地歌单，所以公开歌单拉取失败只记错误不中断。 */
  async function load() {
    loading.value = true
    const local = readLocal()
    let shared = []
    try {
      shared = (await playlistApi.list()).map(p => normalize(p, 'shared'))
      // 列表只给曲目数，不给 ID（避免几万个 ID 撑爆响应）。
      // 公开歌单数量通常是个位数，逐张补详情，卡片封面/曲目数/选择态才准确。
      await Promise.all(shared.filter(p => p.track_count > 0).map(async p => {
        try {
          const full = await playlistApi.detail(p.id)
          if (full) Object.assign(p, normalize(full, 'shared'))
        } catch { /* 单张失败不影响其余 */ }
      }))
    } catch (e) {
      // 旧版后端没有 /api/music/playlists，或网络临时故障
      shared = []
    }
    items.value = [...shared, ...local]
    if (!items.value.some(p => p.id === selectedId.value)) selectedId.value = ''
    loaded.value = true
    loading.value = false
    return items.value
  }

  /** 按歌单顺序取曲目对象；曲库里已删除的歌自动跳过。 */
  function tracksOf(p) {
    if (!p?.track_ids?.length) return []
    const byId = new Map(store.tracks.value.map(t => [t.id, t]))
    return p.track_ids.map(id => byId.get(id)).filter(Boolean)
  }

  function countOf(p) {
    if (!p) return 0
    const resolved = tracksOf(p).length
    return resolved || Number(p.track_count || 0)
  }

  async function select(id) {
    selectedId.value = id
    const p = items.value.find(x => x.id === id)
    // load() 已补齐详情；这里是旧缓存/补齐失败时的兜底。
    if (p?.source === 'shared' && p.track_count > 0 && !p.track_ids?.length) {
      try {
        const full = await playlistApi.detail(id)
        if (full) {
          const item = normalize(full, 'shared')
          items.value = items.value.map(x => (x.id === id ? item : x))
        }
      } catch { /* 读不到详情就显示空态，不阻塞页面 */ }
    }
  }

  async function create({ name, description = '', scope = 'local', track_ids = [] }) {
    const cleanName = String(name || '').trim()
    if (!cleanName) return null
    const ids = [...new Set(track_ids.filter(Boolean))]
    let item
    if (scope === 'shared' && isAdminUser()) {
      item = normalize(await playlistApi.create({ name: cleanName, description, track_ids: ids }), 'shared')
    } else {
      const now = new Date().toISOString()
      item = normalize({
        id: localID(), name: cleanName, description, track_ids: ids,
        track_count: ids.length, created_at: now, updated_at: now,
      }, 'local')
    }
    items.value = [item, ...items.value]
    persistLocal()
    selectedId.value = item.id
    return item
  }

  async function rename(p, patch) {
    if (!canEdit(p)) return null
    const name = patch.name === undefined ? p.name : String(patch.name).trim()
    const description = patch.description === undefined ? p.description : String(patch.description).trim()
    if (!name) return null
    let item
    if (p.source === 'shared') {
      item = normalize(await playlistApi.update(p.id, { name, description }), 'shared')
    } else {
      item = normalize({ ...p, name, description, updated_at: new Date().toISOString() }, 'local')
    }
    items.value = items.value.map(x => (x.id === p.id ? item : x))
    persistLocal()
    return item
  }

  async function remove(p) {
    if (!canEdit(p)) return false
    if (p.source === 'shared') await playlistApi.remove(p.id)
    items.value = items.value.filter(x => x.id !== p.id)
    if (selectedId.value === p.id) selectedId.value = ''
    persistLocal()
    return true
  }

  async function addTracks(p, trackIds) {
    if (!canEdit(p)) return 0
    const ids = [...new Set(trackIds.filter(Boolean))]
    if (!ids.length) return 0
    let nextIds
    if (p.source === 'shared') {
      await playlistApi.addTracks(p.id, ids)
      const fresh = await playlistApi.detail(p.id)
      nextIds = fresh?.track_ids || [...p.track_ids, ...ids]
    } else {
      nextIds = [...p.track_ids, ...ids.filter(id => !p.track_ids.includes(id))]
    }
    const item = normalize({ ...p, track_ids: nextIds, updated_at: new Date().toISOString() }, p.source)
    items.value = items.value.map(x => (x.id === p.id ? item : x))
    persistLocal()
    return ids.length
  }

  async function removeTrack(p, trackId) {
    if (!canEdit(p)) return false
    if (p.source === 'shared') await playlistApi.removeTrack(p.id, trackId)
    const item = normalize({
      ...p,
      track_ids: p.track_ids.filter(id => id !== trackId),
      updated_at: new Date().toISOString(),
    }, p.source)
    items.value = items.value.map(x => (x.id === p.id ? item : x))
    persistLocal()
    return true
  }

  /** 供「更多操作」统一处理失败反馈，避免每个组件各写一遍 ElMessage。 */
  async function run(action, successText) {
    try {
      const result = await action()
      if (successText && result !== null && result !== false) ElMessage.success(successText)
      return result
    } catch (e) {
      ElMessage.error(e?.message || '操作失败')
      return null
    }
  }

  return {
    items, loading, loaded, selectedId, selected, publicItems, localItems,
    isAdmin: isAdminUser, canEdit, load, tracksOf, countOf, select,
    create, rename, remove, addTracks, removeTrack, run,
  }
}
