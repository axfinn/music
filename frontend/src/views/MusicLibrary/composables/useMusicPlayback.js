/**
 * 音乐库 ↔ 全局播放器（composables/useMediaPlayer.js）的唯一桥接层。
 *
 * 硬约束（来自 App.vue / 沉浸播放页，改动会连锁破坏别的页面）：
 *   - 曲目 id 必须以 'mus_' 开头，播放计数与「回到曲库」跳转都靠它识别
 *   - 交给播放器的条目必须带 primaryAsset: { kind:'audio', asset_url }
 *   - 播放条是全局单例，本层只负责「喂队列」，不自己建 <audio>
 */
import { computed, onScopeDispose, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useMediaPlayer } from '../../../composables/useMediaPlayer.js'
import { coverUrl, musicApi, streamUrl } from '../api.js'
import { playableOnly } from '../utils/collections.js'
import { toneFor } from '../utils/format.js'
import { extractDominant, paletteToCss } from '../utils/palette.js'

const LOOP_MODES = ['off', 'all', 'one']

export function useMusicPlayback(store, { route, router, focusSearch } = {}) {
  const player = useMediaPlayer()
  const state = player.state

  const toPlayerTrack = (t) => ({
    id: t.id,
    title: t.title || '未命名',
    artist: t.artist || '',
    cover: coverUrl(t),
    primaryAsset: { kind: 'audio', asset_url: streamUrl(t.id) },
  })

  const isCurrent = (t) => state.playlist[state.currentIndex]?.id === t?.id
  const hasPlayback = computed(() => state.currentIndex >= 0 && state.playlist.length > 0)
  const progress = computed(() =>
    state.duration > 0 ? Math.min(100, (state.currentTime / state.duration) * 100) : 0)

  const nowPlaying = computed(() => {
    const cur = state.playlist[state.currentIndex]
    if (!cur?.id?.startsWith('mus_')) return null
    const local = store.tracks.value.find(t => t.id === cur.id)
    return {
      id: cur.id,
      title: cur.title,
      artist: local?.artist || cur.artist || '',
      album: local?.album || '',
      cover: local ? coverUrl(local) : cur.cover,
      cover_filename: local?.cover_filename || '', // CoverArt 靠它判断有没有真实封面
      favorite: !!local?.favorite,
      lyricSongId: local?.lyric_song_id || '',
      lyrics: local?.lyrics || '',
      duration: Number(local?.duration_sec) || 0,
    }
  })

  // ── 封面驱动的整页氛围色 ──
  // 有播放就跟随正在播放的封面；没播放就跟随「发现页」推荐的那首，
  // 否则首屏会出现「hero 是绿色封面、整页却是紫色氛围」的错位。
  const ambientSource = computed(() => {
    if (nowPlaying.value) {
      return {
        id: nowPlaying.value.id,
        cover: nowPlaying.value.cover || '',
        hue: toneFor(nowPlaying.value.id || nowPlaying.value.title || 'music').hue,
      }
    }
    const featured = store.insights?.value?.recent?.[0]
    if (featured) {
      return { id: featured.id, cover: coverUrl(featured), hue: toneFor(featured.id).hue }
    }
    return { id: 'music', cover: '', hue: toneFor('music').hue }
  })

  const ambient = ref(paletteToCss(null, 24))
  watch(ambientSource, async (src) => {
    const p = src.cover ? await extractDominant(src.cover) : null
    // 快速切歌时只认最后一次结果
    if (ambientSource.value.id !== src.id) return
    ambient.value = paletteToCss(p, src.hue)
  }, { immediate: true })

  function playTrack(track) {
    if (track.playable === false) {
      ElMessage.error(`「${track.title}」的音频文件已缺失，无法播放`)
      return
    }
    if (isCurrent(track)) {
      player.togglePlay()
      return
    }
    const queue = playableOnly(store.visibleTracks.value)
    const idx = queue.findIndex(t => t.id === track.id)
    const list = idx >= 0 ? queue : [track, ...queue.filter(t => t.id !== track.id)]
    player.playAll(list.map(toPlayerTrack), idx >= 0 ? idx : 0)
  }

  function playTracks(list, startIndex = 0) {
    const playable = playableOnly(list)
    if (!playable.length) {
      ElMessage.warning('当前没有可播放的歌曲')
      return
    }
    player.playAll(playable.map(toPlayerTrack), Math.min(startIndex, playable.length - 1))
  }

  async function playAllVisible({ shuffle = false } = {}) {
    // 列表是分页的：不补齐的话「播放全部 859 首」实际只会播已加载的那一页
    if (store.hasMore?.value) {
      const loadingMsg = ElMessage({ message: '正在读取完整曲库…', duration: 0 })
      try { await store.ensureComplete() } finally { loadingMsg.close() }
    }
    const list = playableOnly(store.visibleTracks.value)
    if (!list.length) return ElMessage.warning('当前没有可播放的歌曲')
    const start = shuffle ? Math.floor(Math.random() * list.length) : 0
    player.setShuffle(shuffle)
    playTracks(list, start)
    if (shuffle) ElMessage.success(`已生成 ${list.length} 首的随机列表`)
  }

  function playGroup(group) { playTracks(group.items) }

  function cycleLoop() {
    const i = LOOP_MODES.indexOf(state.loopMode)
    player.setLoopMode(LOOP_MODES[(i + 1) % LOOP_MODES.length])
  }

  function queuePosLabel(i) {
    if (state.shuffleMode && state.shuffleOrder) {
      const pos = state.shuffleOrder.indexOf(i)
      if (pos >= 0) return pos + 1
    }
    return i + 1
  }

  function toggleMute() { player.setVolume(state.volume === 0 ? 1 : 0) }

  // ── 切歌记一次播放数（只记本曲库的曲目，且同一 index 不重复）──
  let lastCountedId = ''
  watch(() => state.currentIndex, () => {
    const cur = state.playlist[state.currentIndex]
    if (!cur?.id?.startsWith('mus_') || cur.id === lastCountedId) return
    lastCountedId = cur.id
    musicApi.played(cur.id).then(() => store.refreshStats()).catch(() => {})
  })

  // ── 歌词库 / 其他页面「在音乐库播放」直达：/music?play=<id> ──
  let lastPlayReq = ''
  async function handlePlayQuery() {
    const pid = route?.query?.play
    if (!pid || pid === lastPlayReq) return
    lastPlayReq = String(pid)
    // onActivated 可能比 onMounted 的首次加载更早跑，先等曲库到货
    await store.whenReady?.()
    // 目标曲目可能在第 2 页之后，先把全量补齐再找，避免误报「没找到」
    await store.ensureComplete()
    const found = store.tracks.value.find(t => t.id === pid)
    if (found) playTrack(found)
    else ElMessage.warning('没找到要播放的曲目')
    // 清掉 query 但不走 router：App.vue 的视图 key 是 route.fullPath，
    // router.replace 会让整页重挂载、曲库重新拉一遍（实测多出 5 个请求）。
    try {
      history.replaceState(history.state, '', `${route.path}${route.hash || ''}`)
    } catch { /* 忽略 */ }
  }

  /** 沉浸播放页复用全局播放器，进去前保证有声音（这次点击也是浏览器要的用户手势） */
  function openVisualizer() {
    if (!hasPlayback.value) {
      const first = playableOnly(store.visibleTracks.value)[0]
      if (first) playTrack(first)
    }
    router?.push('/music/visualizer')
  }

  /** 键盘快捷键；顶层组件负责在 activated / deactivated 里挂摘 */
  function handleKeydown(e) {
    const tgt = e.target
    if (tgt && (tgt.isContentEditable || /^(INPUT|TEXTAREA|SELECT|BUTTON|A)$/.test(tgt.tagName))) return
    if (e.metaKey || e.ctrlKey || e.altKey) return
    switch (e.key) {
      case ' ':
        if (!state.playlist.length) return
        e.preventDefault()
        player.togglePlay()
        break
      case 'ArrowLeft':
        e.preventDefault()
        player.seek(Math.max(0, state.currentTime - 5))
        break
      case 'ArrowRight':
        e.preventDefault()
        player.seek(Math.min(state.duration, state.currentTime + 5))
        break
      case 'm': case 'M':
        toggleMute()
        break
      case '/':
        e.preventDefault()
        focusSearch?.()
        break
      case 'j': case 'J':
        player.skipNext()
        break
      case 'k': case 'K':
        player.skipPrev()
        break
      default:
        break
    }
  }

  onScopeDispose(() => { /* 全局播放器不随页面销毁，状态刻意保留 */ })

  return {
    player, state, nowPlaying, hasPlayback, progress, ambient,
    toPlayerTrack, isCurrent, playTrack, playTracks, playAllVisible, playGroup,
    cycleLoop, queuePosLabel, toggleMute, handlePlayQuery, openVisualizer, handleKeydown,
  }
}
