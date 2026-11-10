import { describe, it, expect } from 'vitest'
import {
  facetNameOf, buildFacets, applyFacet, groupTracks, isPlayable, playableOnly,
  sortLocal, pickTop, libraryInsights, withoutIds, patchById, DIMENSIONS,
} from './collections.js'

const t = (id, extra = {}) => ({
  id,
  title: `Song ${id}`,
  artist: '',
  album: '',
  duration_sec: 100,
  play_count: 0,
  playable: true,
  created_at: '2026-01-01T00:00:00Z',
  ...extra,
})

const fixture = [
  t('mus_1', { artist: 'A', album: 'Alpha', play_count: 9, duration_sec: 200, created_at: '2026-03-01' }),
  t('mus_2', { artist: 'A', album: 'Beta', play_count: 2, duration_sec: 120, created_at: '2026-02-01' }),
  t('mus_3', { artist: 'B', album: 'Alpha', play_count: 5, duration_sec: 180, created_at: '2026-01-01', favorite: true }),
  t('mus_4', { artist: '', album: '', play_count: 0, playable: false }),
]

describe('facetNameOf', () => {
  it('空值回落到占位名', () => {
    expect(facetNameOf(t('x'), DIMENSIONS.album)).toBe('未分类专辑')
    expect(facetNameOf(t('x'), DIMENSIONS.artist)).toBe('未知歌手')
  })
  it('去掉首尾空白', () => {
    expect(facetNameOf(t('x', { artist: '  周杰伦  ' }), DIMENSIONS.artist)).toBe('周杰伦')
  })
})

describe('buildFacets / applyFacet', () => {
  it('按数量降序聚合并带计数', () => {
    const f = buildFacets(fixture, DIMENSIONS.album)
    expect(f[0]).toEqual({ name: 'Alpha', count: 2 })
    expect(f.find(x => x.name === '未分类专辑').count).toBe(1)
  })
  it('max 限制生效', () => {
    expect(buildFacets(fixture, DIMENSIONS.artist, 1)).toHaveLength(1)
  })
  it('空筛选返回全量，指定筛选只留命中项', () => {
    expect(applyFacet(fixture, DIMENSIONS.album, '')).toHaveLength(4)
    expect(applyFacet(fixture, DIMENSIONS.album, 'Alpha').map(x => x.id)).toEqual(['mus_1', 'mus_3'])
  })
})

describe('groupTracks', () => {
  it('累计时长、封面代表、分组计数正确', () => {
    const groups = groupTracks(fixture, DIMENSIONS.album)
    const alpha = groups.find(g => g.name === 'Alpha')
    expect(alpha.count).toBe(2)
    expect(alpha.duration).toBe(380)
    expect(alpha.items.map(x => x.id)).toEqual(['mus_1', 'mus_3'])
  })
  it('取组内第一张有封面的曲目作为代表', () => {
    const withCover = [
      t('a', { album: 'X' }),
      t('b', { album: 'X', cover_filename: 'b.png' }),
      t('c', { album: 'X', cover_filename: 'c.png' }),
    ]
    expect(groupTracks(withCover, DIMENSIONS.album)[0].coverTrack.id).toBe('b')
  })
  it('空输入返回空数组', () => {
    expect(groupTracks([], DIMENSIONS.album)).toEqual([])
  })
})

describe('播放性', () => {
  it('playable === false 被拦下', () => {
    expect(isPlayable(fixture[3])).toBe(false)
    expect(playableOnly(fixture).map(x => x.id)).toEqual(['mus_1', 'mus_2', 'mus_3'])
  })
})

describe('排序与首页聚合', () => {
  it('sortLocal 不修改原数组', () => {
    const input = [...fixture]
    const byPlays = sortLocal(input, 'plays')
    expect(byPlays[0].id).toBe('mus_1')
    expect(input[0].id).toBe('mus_1')
  })
  it('created 按时间倒序', () => {
    expect(pickTop(fixture.filter(x => x.created_at), 2, 'created').map(x => x.id)).toEqual(['mus_1', 'mus_2'])
  })
  it('libraryInsights 汇总收藏 / 时长 / 歌手专辑数', () => {
    const s = libraryInsights(fixture, { total_count: 4, favorite_count: 1, total_duration: 500, total_bytes: 1024, total_plays: 16 })
    expect(s.total).toBe(4)
    expect(s.favorites).toBe(1)
    expect(s.albums).toBe(3) // Alpha / Beta / 未分类专辑
    expect(s.artists).toBe(3) // A / B / 未知歌手
    expect(s.mostPlayed[0].id).toBe('mus_1')
    expect(s.loved[0].id).toBe('mus_3')
  })
  it('stats 为空时从列表兜底', () => {
    const s = libraryInsights(fixture, null)
    expect(s.total).toBe(4)
    expect(s.totalDuration).toBe(600)
  })
})

describe('集合编辑', () => {
  it('withoutIds 支持 Set 与数组', () => {
    expect(withoutIds(fixture, new Set(['mus_2'])).map(x => x.id)).toEqual(['mus_1', 'mus_3', 'mus_4'])
    expect(withoutIds(fixture, ['mus_2'])).toHaveLength(3)
  })
  it('patchById 只改目标项且返回新对象', () => {
    const out = patchById(fixture, 'mus_2', { favorite: true })
    expect(out[1].favorite).toBe(true)
    expect(out[0]).toBe(fixture[0])
    expect(out[1]).not.toBe(fixture[1])
  })
})
