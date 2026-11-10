/**
 * 歌词子系统：歌词取用 + 歌词库关联。
 *
 * 歌词有两份来源：曲目自带的 track.lyrics，以及关联到歌词库的 LyricSong.lyrics。
 * 关联时优先用歌词库版本（更规整），且服务端不冗余存储，所以每次要单独取。
 *
 * ⚠️ 这里刻意不做时间同步：曲库里的 LRC 时间轴大多不可靠，按它高亮/滚动
 * 反而和实际听感对不上，所以歌词只作为「原文可查看」呈现。
 */
import { computed, onScopeDispose, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { musicApi } from '../api.js'
import { parseLyrics } from '../../../utils/lrc.js'

export function useMusicLyrics(store, playback) {
  // ── 当前播放 ──
  const currentText = ref('')
  const currentSource = ref('local') // 'local' | 'library'
  // ── 歌词抽屉（同时承载「队列」页签，右栏因此可以保持紧凑、不和宠物抢位置） ──
  const drawerOpen = ref(false)
  const drawerTab = ref('lyrics') // 'lyrics' | 'queue'
  const drawerTrack = ref(null)
  const drawerText = ref('')
  const drawerSource = ref('local')

  let resolveToken = 0

  async function resolveLyrics(track) {
    const token = ++resolveToken
    if (!track) return { text: '', source: 'local' }
    // 关联到歌词库的版本优先（更规整），没有关联就用曲目自带的正文。
    // 走 /api/music/tracks/:id/lyrics 这个只读口子：歌词库是后台模块，
    // 但「正在听的这一首的歌词」属于听歌本身，访客也要能看到。
    if (track.lyric_song_id) {
      try {
        const res = await musicApi.trackLyrics(track.id)
        if (token === resolveToken && res.text) return { text: res.text, source: res.source }
      } catch { /* 取不到就回落自带歌词 */ }
    }
    if (token !== resolveToken) return { text: '', source: 'local' }
    // 列表/快照是精简形态（只有 has_lyrics 标记）：确实有自带歌词时才按需取详情，
    // 并回填本地缓存，避免每首切歌都打一次请求。
    if (track.lyrics === undefined && track.has_lyrics) {
      try {
        const full = await musicApi.detail(track.id)
        if (token !== resolveToken) return { text: '', source: 'local' }
        const text = full?.lyrics || ''
        store.patchLocal(track.id, { lyrics: text, note: full?.note || '' })
        return { text, source: 'local' }
      } catch { /* 详情失败按无正文处理，不阻塞歌词抽屉 */ }
    }
    return { text: track.lyrics || '', source: 'local' }
  }

  watch(() => playback.nowPlaying.value?.id, async () => {
    const np = playback.nowPlaying.value
    if (!np) {
      currentText.value = ''
      currentSource.value = 'local'
      return
    }
    const track = store.tracks.value.find(t => t.id === np.id)
    const result = await resolveLyrics(track || np)
    currentText.value = result.text
    currentSource.value = result.source
  })

  // 只做展示：不再按播放进度算「当前行」，避免时间轴不准时来回跳
  const lines = computed(() => parseLyrics(currentText.value))
  const drawerLines = computed(() => parseLyrics(drawerText.value))

  async function openDrawer(track, tab = 'lyrics') {
    drawerTab.value = tab
    if (track) drawerTrack.value = track
    drawerOpen.value = true
    if (!track) return
    const result = await resolveLyrics(track)
    if (drawerTrack.value?.id !== track.id) return
    drawerText.value = result.text
    drawerSource.value = result.source
  }

  /** 关联后如果正好在播这首，立即换上新歌词 */
  async function refreshIfCurrent(id) {
    if (playback.nowPlaying.value?.id !== id) return
    const track = store.tracks.value.find(t => t.id === id)
    const result = await resolveLyrics(track)
    if (playback.nowPlaying.value?.id !== id) return
    currentText.value = result.text
    currentSource.value = result.source
    if (drawerTrack.value?.id === id) {
      drawerText.value = result.text
      drawerSource.value = result.source
    }
  }

  // ── 歌词库关联 ──
  const linkDialog = ref(false)
  const linkTarget = ref(null)
  const linkKeyword = ref('')
  const linkCandidates = ref([])
  const linkLoading = ref(false)
  const linkBusy = ref(false)
  let linkTimer = null

  function openLink(track) {
    linkTarget.value = track
    linkKeyword.value = track.title || ''
    linkCandidates.value = []
    linkDialog.value = true
    searchLink()
  }

  function searchLink() {
    if (!linkTarget.value) return
    clearTimeout(linkTimer)
    linkTimer = setTimeout(async () => {
      linkLoading.value = true
      try {
        linkCandidates.value = await musicApi.lyricCandidates(linkTarget.value.id, linkKeyword.value.trim())
      } catch (e) {
        ElMessage.error(e.message || '检索失败')
      } finally {
        linkLoading.value = false
      }
    }, 220)
  }

  async function doLink(song) {
    const id = linkTarget.value?.id
    if (!id) return
    try {
      await musicApi.linkLyric(id, song.id)
      store.patchLocal(id, { lyric_song_id: song.id })
      linkTarget.value = { ...linkTarget.value, lyric_song_id: song.id }
      ElMessage.success(`已关联「${song.title}」`)
      linkDialog.value = false
      refreshIfCurrent(id)
    } catch (e) {
      ElMessage.error(e.message || '关联失败')
    }
  }

  async function unlinkLyric(track) {
    try {
      await musicApi.unlinkLyric(track.id)
      store.patchLocal(track.id, { lyric_song_id: '' })
      linkTarget.value = { ...track, lyric_song_id: '' }
      ElMessage.success('已解除关联')
      refreshIfCurrent(track.id)
    } catch (e) {
      ElMessage.error(e.message || '解除失败')
    }
  }

  async function autoMatchOne() {
    const id = linkTarget.value?.id
    if (!id || linkBusy.value) return
    linkBusy.value = true
    try {
      const data = await musicApi.autoMatchOne(id)
      if (data.matched) {
        const song = data.lyric_song || {}
        store.patchLocal(id, { lyric_song_id: song.id })
        linkTarget.value = { ...linkTarget.value, lyric_song_id: song.id }
        ElMessage.success(`已按歌名匹配「${song.title}」`)
        linkDialog.value = false
        refreshIfCurrent(id)
      } else if (data.items?.length) {
        linkCandidates.value = data.items
        ElMessage.info('没有唯一同名，下面是候选，点一首即可关联')
      } else {
        ElMessage.warning('歌词库里没有同名歌，换个词搜，或先去歌词库收录')
      }
    } catch (e) {
      ElMessage.error(e.message || '匹配失败')
    } finally {
      linkBusy.value = false
    }
  }

  async function autoMatchAll() {
    if (linkBusy.value) return
    linkBusy.value = true
    try {
      const data = await musicApi.autoMatchAll()
      await store.reload()
      if (data.matched > 0) {
        ElMessage.success(`已按歌名匹配 ${data.matched} 首${data.skipped ? `，${data.skipped} 首需手动挑选` : ''}`)
      } else if (data.total === 0) {
        ElMessage.info('没有待匹配的曲目（都关联过了）')
      } else {
        ElMessage.warning(`${data.skipped} 首没找到唯一同名，可点行内「关联」手动挑`)
      }
    } catch (e) {
      ElMessage.error(e.message || '匹配失败')
    } finally {
      linkBusy.value = false
    }
  }

  onScopeDispose(() => clearTimeout(linkTimer))

  return {
    // 当前播放（仅文本，不做时间同步）
    currentText, currentSource, lines,
    // 抽屉
    drawerOpen, drawerTab, drawerTrack, drawerText, drawerSource,
    drawerLines, openDrawer,
    // 关联
    linkDialog, linkTarget, linkKeyword, linkCandidates, linkLoading, linkBusy,
    openLink, searchLink, doLink, unlinkLyric, autoMatchOne, autoMatchAll,
  }
}
