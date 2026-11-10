/**
 * 曲库集合运算（纯函数）。
 *
 * 后端只支持 keyword / fav / sort / limit+offset，专辑、歌手聚合与本地视图
 * 全在前端完成。把这些逻辑从组件里剥出来，既让 UI 变薄，也能直接单测。
 */
import { relativeDate } from './format.js'

export const DIMENSIONS = {
  album: 'album',
  artist: 'artist',
}

const FALLBACK = {
  album: '未分类专辑',
  artist: '未知歌手',
}

export function facetNameOf(track, dimension) {
  const key = dimension === DIMENSIONS.album ? 'album' : 'artist'
  const raw = String(track?.[key] || '').trim()
  return raw || FALLBACK[key]
}

export function isPlayable(track) {
  return !!track?.id && track.playable !== false
}

export function playableOnly(tracks) {
  return (tracks || []).filter(isPlayable)
}

/** 按当前维度聚合，用于快速筛选 chips */
export function buildFacets(tracks, dimension, max = 24) {
  const counts = new Map()
  for (const t of tracks || []) {
    const name = facetNameOf(t, dimension)
    counts.set(name, (counts.get(name) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, 'zh-Hans-CN'))
    .slice(0, max)
}

export function applyFacet(tracks, dimension, facet) {
  if (!facet) return tracks || []
  return (tracks || []).filter(t => facetNameOf(t, dimension) === facet)
}

/**
 * 卡片墙数据：每组带曲目数、总时长、代表封面、最近添加时间。
 * 无封面时 cover 为空，由 CoverArt 组件生成确定性渐变封面。
 */
export function groupTracks(tracks, dimension) {
  const map = new Map()
  for (const t of tracks || []) {
    const name = facetNameOf(t, dimension)
    let g = map.get(name)
    if (!g) {
      g = { key: name, name, items: [], duration: 0, coverTrack: null, latest: '' }
      map.set(name, g)
    }
    g.items.push(t)
    g.duration += Number(t.duration_sec) || 0
    if (!g.coverTrack && t.cover_filename) g.coverTrack = t
    if (!g.latest || String(t.created_at || '') > String(g.latest)) g.latest = t.created_at || ''
  }
  return [...map.values()]
    .map(g => ({
      ...g,
      count: g.items.length,
      subtitle: g.latest ? `更新于 ${relativeDate(g.latest)}` : `${g.count} 首曲目`,
    }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, 'zh-Hans-CN'))
}

const compareBy = {
  created: (a, b) => String(b.created_at || '').localeCompare(String(a.created_at || '')),
  title: (a, b) => String(a.title || '').localeCompare(String(b.title || ''), 'zh-Hans-CN'),
  plays: (a, b) => (Number(b.play_count) || 0) - (Number(a.play_count) || 0),
}

export function sortLocal(tracks, sortBy = 'created') {
  const cmp = compareBy[sortBy] || compareBy.created
  return [...(tracks || [])].sort(cmp)
}

export function pickTop(tracks, n = 6, sortBy = 'created') {
  return sortLocal(tracks, sortBy).slice(0, n)
}

/** 发现页用的聚合视图 */
export function libraryInsights(tracks, stats) {
  const list = tracks || []
  const albums = new Set(list.map(t => facetNameOf(t, DIMENSIONS.album))).size
  const artists = new Set(list.map(t => facetNameOf(t, DIMENSIONS.artist))).size
  return {
    total: Number(stats?.total_count ?? list.length) || 0,
    favorites: Number(stats?.favorite_count) || list.filter(t => t.favorite).length,
    totalDuration: Number(stats?.total_duration) || list.reduce((s, t) => s + (Number(t.duration_sec) || 0), 0),
    totalBytes: Number(stats?.total_bytes) || list.reduce((s, t) => s + (Number(t.size_bytes) || 0), 0),
    totalPlays: Number(stats?.total_plays) || list.reduce((s, t) => s + (Number(t.play_count) || 0), 0),
    albums,
    artists,
    recent: pickTop(list, 8, 'created'),
    loved: pickTop(list.filter(t => t.favorite), 6, 'created'),
    mostPlayed: pickTop(list.filter(t => (Number(t.play_count) || 0) > 0), 6, 'plays'),
  }
}

/** 多选批量操作后的集合运算 */
export function withoutIds(tracks, ids) {
  const set = ids instanceof Set ? ids : new Set(ids)
  return (tracks || []).filter(t => !set.has(t.id))
}

export function patchById(tracks, id, patch) {
  return (tracks || []).map(t => (t.id === id ? { ...t, ...patch } : t))
}
