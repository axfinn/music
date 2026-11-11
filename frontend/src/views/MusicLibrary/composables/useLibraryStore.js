/**
 * 曲库数据中枢：首屏 / 快照 / 搜索 / 排序 / 收藏筛选 / 统计 / CRUD / 多选 / 偏好持久化。
 *
 * 组件只消费这里的 computed 与方法，不再自己拼 fetch。
 *
 * 首屏分两段，别让 1MB 级别的全量快照挡住第一帧：
 *   1) /tracks 只取第一页 + 服务端总数/统计（几十 KB），立刻解锁 loading
 *   2) /snapshot 在后台补齐全量精简曲目（专辑/歌手聚合、facets、随机播放队列要用）
 * 全量快照固定按 created 取，排序交给本地；切排序因此不再重传整份曲库。
 * revision 做增量：数据与筛选都没变时后端只回 not_modified，零载荷。
 * 同一筛选已缓存全量时，连第一页都不用等，直接用缓存出首屏。
 * ensureComplete() / loadMore() 仅作为快照超限或旧后端降级时的分页兜底。
 */
import { computed, onScopeDispose, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { musicApi } from '../api.js'
import {
  applyFacet, buildFacets, DIMENSIONS, facetNameOf, groupTracks,
  libraryInsights, patchById, playableOnly, sortLocal, withoutIds,
} from '../utils/collections.js'

const PAGE_SIZE = 100       // 列表视图每次追加
const FETCH_SIZE = 200      // 聚合视图补齐全量时用大页，减少往返
const MAX_TRACKS = 5000     // 防御性上限；后端单次 limit 上限 2000，offset 可继续翻页
const MAX_BATCH = 50        // 批量操作上限，避免误选几百首打出一片请求
const PREFS_KEY = 'musiclib.prefs.v2'

export const SECTIONS = ['discover', 'all', 'favorites', 'recent', 'albums', 'artists']

function readPrefs() {
  try {
    const raw = JSON.parse(localStorage.getItem(PREFS_KEY) || '{}')
    // 兼容旧版本只存了视图模式的场景
    const legacyView = localStorage.getItem('musiclib_view')
    return {
      section: SECTIONS.includes(raw.section) ? raw.section : 'discover',
      sortBy: ['created', 'title', 'plays'].includes(raw.sortBy) ? raw.sortBy : 'created',
      favOnly: !!raw.favOnly,
      viewMode: ['list', 'grid'].includes(raw.viewMode || legacyView) ? (raw.viewMode || legacyView) : 'list',
    }
  } catch {
    return { section: 'discover', sortBy: 'created', favOnly: false, viewMode: 'list' }
  }
}

export function useLibraryStore() {
  const prefs = readPrefs()

  // ── 数据 ──
  const tracks = ref([])
  const total = ref(0)
  const stats = ref(null)
  const loading = ref(true)
  const loadingMore = ref(false)
  const error = ref('')
  // 后端「数据版本 + 筛选条件」token，原样回传即可命中增量
  const revision = ref('')

  // ── 视图条件 ──
  const keyword = ref('')
  const sortBy = ref(prefs.sortBy)
  const favOnly = ref(prefs.favOnly)
  const section = ref(prefs.section)
  const viewMode = ref(prefs.viewMode)
  const facetFilter = ref('')
  const selected = ref(new Set())

  let reqSeq = 0
  let controller = null
  let searchTimer = null
  let ensurePromise = null
  // 上一次快照拿到的全量精简曲目，以及它对应的筛选条件。
  // 有了它，回到同一视图时首屏可以零请求直接出内容。
  let fullItems = []
  let fullKey = ''
  // 首次加载完成信号：keep-alive 下 onActivated 可能早于 onMounted 的 await 结束，
  // 「?play=」直达播放必须等曲库真的到货再找，否则会误报「没找到」。
  let resolveReady = null
  const ready = new Promise(resolve => { resolveReady = resolve })

  const isFavoriteSection = computed(() => section.value === 'favorites')
  const effectiveFav = computed(() => favOnly.value || isFavoriteSection.value)
  const dimension = computed(() => (section.value === 'albums' ? DIMENSIONS.album : DIMENSIONS.artist))
  const needsFullLibrary = computed(() => ['discover', 'recent', 'albums', 'artists'].includes(section.value))
  const hasMore = computed(() => tracks.value.length < total.value && tracks.value.length < MAX_TRACKS)
  const isFiltering = computed(() => !!keyword.value.trim() || effectiveFav.value || !!facetFilter.value)

  const facets = computed(() => buildFacets(tracks.value, dimension.value))
  const visibleTracks = computed(() => {
    let list = applyFacet(tracks.value, dimension.value, facetFilter.value)
    // 「最近」是固定语义，不受排序下拉影响
    if (section.value === 'recent') return sortLocal(list, 'created').slice(0, 40)
    // 全量快照统一按 created 取，所以任何非默认排序都在本地重排。
    // setSortBy 因此不必回服务端，切排序不再重传整份曲库。
    return sortBy.value === 'created' ? list : sortLocal(list, sortBy.value)
  })
  const groups = computed(() => groupTracks(visibleTracks.value, dimension.value))
  const insights = computed(() => libraryInsights(tracks.value, stats.value))
  const playableVisible = computed(() => playableOnly(visibleTracks.value))
  /**
   * 展示用条数：未筛选时列表是分页加载的，内存里只有前 N 条，
   * 这时候要用服务端 total，否则头上会写成「100 首」。
   */
  const displayCount = computed(() => {
    // 聚合视图里「859」是曲目数，头上的数字应该是 207 个专辑 / 5 位歌手
    if (!facetFilter.value && (section.value === 'albums' || section.value === 'artists')) {
      return groups.value.length
    }
    return isFiltering.value ? visibleTracks.value.length : Math.max(total.value, tracks.value.length)
  })
  /** 展示用总时长：未筛选时用服务端统计，避免只算已加载的那几页 */
  const displayDuration = computed(() => (
    isFiltering.value
      ? visibleTracks.value.reduce((s, t) => s + (Number(t.duration_sec) || 0), 0)
      : insights.value.totalDuration
  ))

  function listParams(offset) {
    return {
      keyword: keyword.value,
      fav: effectiveFav.value,
      sort: sortBy.value,
      limit: PAGE_SIZE,
      offset,
      signal: controller?.signal,
    }
  }

  // 全量快照与排序解耦，所以缓存 key 不含 sortBy
  const filterKey = () => `${keyword.value.trim()}|${effectiveFav.value}`

  // ready 只由最后一次 load 结算：被抢占的请求不能提前放行 ?play= 直达播放
  function settleReady(seq) {
    if (seq !== reqSeq) return
    resolveReady?.()
    resolveReady = null
  }

  /**
   * 两段式加载。load() 的 Promise 仍然等全量快照结束（保持 onMounted 与 ?play=
   * 的时序），但 loading 在第一页到手时就关掉，页面不用等那一大包快照。
   */
  async function load() {
    const seq = ++reqSeq
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    loading.value = true
    error.value = ''

    const key = filterKey()
    const cached = fullKey === key && fullItems.length ? fullItems : null
    let firstOk = !!cached
    if (cached) {
      // 同一筛选已有全量：零请求直接出首屏，后面的快照只用来校验数据版本
      tracks.value = cached
      loading.value = false
    } else {
      // 阶段一：只取第一页（默认 100 条），几十 KB，立刻可渲染、可播放
      try {
        const first = await musicApi.list({ ...listParams(0), withStats: true, signal })
        if (seq !== reqSeq) return
        tracks.value = first.items || []
        total.value = first.total || 0
        if (first.stats) stats.value = first.stats
        firstOk = true
      } catch (e) {
        if (e?.name === 'AbortError' || seq !== reqSeq) return
        error.value = e.message || '曲库加载失败'
        ElMessage.error(error.value)
      } finally {
        if (seq === reqSeq) loading.value = false
      }
    }
    if (!firstOk) {
      settleReady(seq)
      return
    }

    // 阶段二：后台补齐全量。失败不弹错、不覆盖阶段一已经能用的数据。
    try {
      await hydrateSnapshot(seq, signal)
    } finally {
      settleReady(seq)
    }
  }

  /** 后台全量补齐；只由 load() 调用，不占用首屏 loading */
  async function hydrateSnapshot(seq, signal) {
    // 请求发出前把筛选条件定格，避免防抖期间 keyword 变了却把旧结果缓存成新 key
    const key = filterKey()
    const params = {
      keyword: keyword.value,
      fav: effectiveFav.value,
      // 固定 created：排序是纯视图变换，不该让 rev token 跟着变、重传整份曲库
      sort: 'created',
      rev: revision.value,
      signal,
    }
    try {
      const data = await musicApi.snapshot(params)
      if (seq !== reqSeq) return
      if (data.notModified) {
        // 后台补齐失败后重试等边缘情况：手里只有第一页，但缓存里有全量
        if (fullKey === key && fullItems.length > tracks.value.length) {
          tracks.value = fullItems
        }
        return
      }
      fullItems = data.items || []
      fullKey = key
      tracks.value = fullItems
      total.value = Math.max(total.value, data.total || 0)
      revision.value = data.revision || ''
      if (data.stats) stats.value = data.stats
      // 超过后端 2 万条上限时才继续翻页
      if (data.truncated) ensureComplete()
    } catch (e) {
      if (e?.name === 'AbortError' || seq !== reqSeq) return
      // 前端可能先于后端发布（热部署只叠 dist）：旧后端没有 /snapshot，
      // 退回「第一页 + 逐页补齐」，不会白屏。
      if (e.status === 404 || e.status === 405) {
        fullItems = []
        fullKey = ''
        revision.value = ''
        refreshStats()
        if (needsFullLibrary.value && hasMore.value) ensureComplete()
        return
      }
      // 超时 / 5xx：保留第一页可用数据，别在后台打出一串分页请求；
      // 用户仍可手动「加载更多」，下次 load 会用 rev 再试一次。
    }
  }

  /** 聚合视图要全量；逐页补齐，幂等且并发安全 */
  function ensureComplete() {
    if (!hasMore.value || ensurePromise) return ensurePromise
    ensurePromise = (async () => {
      while (hasMore.value) {
        const offset = tracks.value.length
        try {
          const data = await musicApi.list({
            ...listParams(offset), limit: FETCH_SIZE, signal: undefined,
          })
          if (!data.items.length) break
          const known = new Set(tracks.value.map(t => t.id))
          tracks.value = [...tracks.value, ...data.items.filter(t => !known.has(t.id))]
          total.value = Math.max(total.value, data.total)
          if (data.items.length < FETCH_SIZE) break
        } catch {
          break
        }
      }
    })().finally(() => { ensurePromise = null })
    return ensurePromise
  }

  async function loadMore() {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
    try {
      const data = await musicApi.list({ ...listParams(tracks.value.length), signal: undefined })
      const known = new Set(tracks.value.map(t => t.id))
      tracks.value = [...tracks.value, ...(data.items || []).filter(t => !known.has(t.id))]
      total.value = Math.max(total.value, data.total || 0)
    } catch (e) {
      ElMessage.error(e.message || '加载更多失败')
    } finally {
      loadingMore.value = false
    }
  }

  async function refreshStats() {
    try { stats.value = await musicApi.stats() } catch { /* 统计失败不阻塞主流程 */ }
  }

  // 输入即搜：防抖 + 请求序号，快速打字不会被旧响应覆盖
  function setKeyword(value) {
    keyword.value = value
    clearTimeout(searchTimer)
    searchTimer = setTimeout(load, 300)
  }

  function setSection(next) {
    if (!SECTIONS.includes(next) || section.value === next) return
    section.value = next
    facetFilter.value = ''
    selected.value = new Set()
    // 切到「收藏」再切回「发现」时 effectiveFav 变了，必须重新取数，
    // 否则发现页只剩收藏子集；同一筛选命中缓存时首屏零请求。
    load()
  }

  function setFacet(name) {
    facetFilter.value = facetFilter.value === name ? '' : name
    selected.value = new Set()
  }

  // 排序是纯视图变换：visibleTracks 本地重排即可，不回服务端重传整份曲库
  function setSortBy(v) { sortBy.value = v }
  function setViewMode(v) { viewMode.value = v }
  function toggleFavOnly() { favOnly.value = !favOnly.value; load() }
  function setFavFilter(v) { favOnly.value = !!v; load() }

  // ── 多选 ──
  function toggleSelect(id) {
    const next = new Set(selected.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selected.value = next
  }
  function selectAll(ids) {
    selected.value = new Set(ids || visibleTracks.value.map(t => t.id))
  }
  function clearSelection() { selected.value = new Set() }

  // ── 写操作：就地更新，避免整页重新请求打断播放与滚动位置 ──
  async function toggleFavorite(track) {
    try {
      const favorite = await musicApi.favorite(track.id, !track.favorite)
      tracks.value = patchById(tracks.value, track.id, { favorite })
      refreshStats()
      return favorite
    } catch (e) {
      ElMessage.error(e.message || '收藏失败')
      return null
    }
  }

  async function batchFavorite(ids, favorite) {
    if (ids.length > MAX_BATCH) return ElMessage.warning(`单次最多操作 ${MAX_BATCH} 首，请缩小选择范围`)
    let done = 0
    for (const id of ids) {
      try { await musicApi.favorite(id, favorite); done++ } catch { /* 单条失败不影响其余 */ }
    }
    const set = new Set(ids)
    tracks.value = tracks.value.map(t => (set.has(t.id) ? { ...t, favorite } : t))
    refreshStats()
    ElMessage.success(`已${favorite ? '收藏' : '取消收藏'} ${done} 首`)
    clearSelection()
  }

  async function remove(track) {
    await musicApi.remove(track.id)
    tracks.value = withoutIds(tracks.value, [track.id])
    total.value = Math.max(0, total.value - 1)
    refreshStats()
  }

  async function batchRemove(ids) {
    if (ids.length > MAX_BATCH) return ElMessage.warning(`单次最多删除 ${MAX_BATCH} 首，请缩小选择范围`)
    let done = 0
    for (const id of ids) {
      try { await musicApi.remove(id); done++ } catch { /* 继续删除其余 */ }
    }
    tracks.value = withoutIds(tracks.value, ids)
    total.value = Math.max(0, total.value - done)
    refreshStats()
    ElMessage.success(`已删除 ${done} 首`)
    clearSelection()
  }

  async function saveTrack(id, patch) {
    const item = await musicApi.update(id, patch)
    if (item) {
      // 详情接口返回正文，顺手缓存进列表：下次编辑/看歌词不用再请求
      tracks.value = patchById(tracks.value, id, {
        ...item,
        has_lyrics: !!item.lyrics,
        has_note: !!item.note,
      })
    }
    return item
  }

  /** 只改本地字段（歌词库关联等服务端已落库的场景），不发请求 */
  function patchLocal(id, patch) {
    tracks.value = patchById(tracks.value, id, patch)
  }

  async function saveCover(id, file) {
    const coverFilename = await musicApi.uploadCover(id, file)
    tracks.value = patchById(tracks.value, id, { cover_filename: coverFilename })
    return coverFilename
  }

  /** 上传完成 / 歌词关联后刷新，保留当前滚动位置 */
  async function reload() { await load() }

  // 偏好持久化
  watch([section, sortBy, favOnly, viewMode], () => {
    try {
      localStorage.setItem(PREFS_KEY, JSON.stringify({
        section: section.value, sortBy: sortBy.value,
        favOnly: favOnly.value, viewMode: viewMode.value,
      }))
    } catch { /* 无痕模式忽略 */ }
  })

  onScopeDispose(() => {
    clearTimeout(searchTimer)
    controller?.abort()
  })

  return {
    // state
    tracks, total, stats, loading, loadingMore, error,
    keyword, sortBy, favOnly, section, viewMode, facetFilter, selected,
    // computed
    effectiveFav, dimension, needsFullLibrary, hasMore, isFiltering,
    facets, visibleTracks, groups, insights, playableVisible, displayCount, displayDuration,
    // actions
    load, loadMore, ensureComplete, refreshStats, setKeyword, setSection, setFacet,
    whenReady: () => ready,
    setSortBy, setViewMode, toggleFavOnly, setFavFilter, reload,
    toggleSelect, selectAll, clearSelection,
    toggleFavorite, batchFavorite, remove, batchRemove, saveTrack, saveCover, patchLocal,
    facetNameOf,
  }
}
