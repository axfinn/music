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

// 「随机播放」不能只洗当前筛选出来的结果:搜索命中 1 首、收藏只有 1 首、
// 下钻到单曲专辑时,原来的实现会生成一条只有 1 首(甚至 0 首)的队列。
// 用 /snapshot 的全量精简曲目做兜底,rev 命中时后端只回 not_modified,零载荷。
let fullLibraryCache = { rev: '', items: [] }
let shuffleReqSeq = 0

async function fetchFullLibrary() {
  const data = await musicApi.snapshot({ rev: fullLibraryCache.rev })
  if (data.notModified) return fullLibraryCache.items
  let items = data.items || []
  // /snapshot 上限 20000,真的超了再按页补,避免队列被悄悄截断
  if (data.truncated) {
    const seen = new Set(items.map(t => t.id))
    let offset = items.length
    while (true) {
      const page = await musicApi.list({ limit: 200, offset })
      const batch = page.items || []
      const fresh = batch.filter(t => t.id && !seen.has(t.id))
      if (!fresh.length) break
      fresh.forEach(t => seen.add(t.id))
      items = items.concat(fresh)
      offset += batch.length
      if (batch.length < 200 || offset >= page.total) break
    }
  }
  fullLibraryCache = { rev: data.revision || '', items }
  return items
}

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

  /**
   * 「随机播放 / 播放全部」入口。
   *
   * 两个踩过的坑:
   *  1. play() 必须发生在用户点击的同步调用栈里。原实现先 await 分页请求,
   *     浏览器的 transient activation 过期后 play() 被静默拒绝 —— 点了没反应。
   *     所以先用当前视图立刻起播,再异步补齐队列并无缝扩展(不重建 <audio>)。
   *  2. 当前视图可能只筛出 0/1 首(搜索、收藏、下钻单曲专辑)。随机播放这时
   *     回退到全库快照,不再退化成「只有一首歌」。
   */
  async function playAllVisible({ shuffle = false } = {}) {
    const seq = ++shuffleReqSeq
    const viewList = playableOnly(store.visibleTracks.value)

    // ① 同步起播:以下调用链里不能有任何 await
    player.setShuffle(shuffle)
    let started = false
    if (viewList.length) {
      const start = shuffle ? Math.floor(Math.random() * viewList.length) : 0
      playTracks(viewList, start)
      started = state.playlist.length > 0
    }

    const needFull = shuffle && viewList.length < 2
    const needPaging = !shuffle && !!store.hasMore?.value
    if (!needFull && !needPaging) {
      if (!viewList.length) ElMessage.warning('当前没有可播放的歌曲')
      else if (shuffle) ElMessage.success(`已生成 ${state.playlist.length} 首的随机列表`)
      return
    }

    const loadingMsg = started ? null : ElMessage({ message: '正在读取完整曲库…', duration: 0 })
    try {
      let source
      if (needFull) {
        source = playableOnly(await fetchFullLibrary())
      } else {
        await store.ensureComplete()
        source = playableOnly(store.visibleTracks.value)
      }
      if (seq !== shuffleReqSeq) return // 连点了两次,只认最后一次
      if (!source.length) {
        if (!started) ElMessage.warning('曲库里没有可播放的歌曲')
        return
      }
      if (started) {
        // 保持正在播的那首不中断,只把队列换成完整的一份
        player.replaceQueue(source.map(toPlayerTrack), { shuffle })
      } else {
        playTracks(source, shuffle ? Math.floor(Math.random() * source.length) : 0)
      }
      if (shuffle) ElMessage.success(`已生成 ${state.playlist.length} 首的随机列表`)
    } catch {
      if (seq === shuffleReqSeq && !started) ElMessage.warning('曲库读取失败，请稍后重试')
    } finally {
      loadingMsg?.close()
    }
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

  // ── 播放失败不能只记在 state 里:以前 UI 没有展示 error,
  //    浏览器拒绝自动播放 / 文件缺失时用户只看到「点了没反应」──
  let lastError = ''
  watch(() => state.error, (msg) => {
    if (!msg) { lastError = ''; return }
    if (msg === lastError) return
    lastError = msg
    ElMessage.warning(msg)
  })

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
