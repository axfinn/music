import { reactive, computed } from 'vue'
import { siteAlbum } from '../site.js'

// ── Singleton reactive state (survives route navigation) ──
function initialVolume() {
  try {
    const v = parseFloat(localStorage.getItem('media_player_volume'))
    if (isFinite(v) && v >= 0 && v <= 1) return v
  } catch {}
  return 1
}

const state = reactive({
  playlist: [],        // all playable items: { id, title, result_type, assetUrl, kind }
  currentIndex: -1,
  isPlaying: false,
  loopMode: 'all',     // 'off' | 'one' | 'all'
  shuffleMode: false,
  shuffleOrder: null,  // 随机模式下的真实播放顺序(playlist 下标的乱序排列),null = 顺序
  currentTime: 0,
  duration: 0,
  volume: initialVolume(),
  error: '',           // 最近一次播放失败的原因,播放成功后清空
})

let audioEl = null
// 连续加载失败计数。坏歌会触发 error → skipNext → 再 error 的连锁跳转,
// 不能一路把整条随机队列跳完(最后 stop() 连队列一起清空,用户看到「点了没反应」)。
// 连续失败到上限就停下说明原因;成功播出一首即归零。
let consecutiveErrors = 0
const MAX_CONSECUTIVE_ERRORS = 3

// ── 频谱分析(AnalyserNode)──
// 沉浸播放页(/music/visualizer)要真实频谱。一旦 createMediaElementSource 接管元素,
// 声音就只从 WebAudio 图里出来,所以整条链必须 source → analyser → destination,
// 少接一环就是"画面有、声音没了"。两个硬约束:
//   1. 一个 <audio> 元素一生只能建一次 MediaElementSource,换歌必须重建并重接;
//   2. AudioContext 未 running 时接管 = 静音(自动播放策略下常是 suspended),
//      所以只在 state === 'running' 之后才接管,resume 成功由 onstatechange 兜底。
let audioCtx = null
let analyserNode = null
let sourceNode = null
let analyserWanted = false
// DJ 效果链插槽(沉浸播放页的「打碟台」注册)。一旦挂上,音频走向变成
// source → fx.input … fx.output → analyser → destination;不挂时保持原链路。
let fxChain = null

function audioContextCtor() {
  if (typeof window === 'undefined') return null
  return window.AudioContext || window.webkitAudioContext || null
}

// fx.output 的落点永远由这里决定:沉浸态进 analyser,离开后回 destination
function rewireFxOutput() {
  if (!fxChain?.output || !audioCtx) return
  try { fxChain.output.disconnect() } catch {}
  try {
    fxChain.output.connect(analyserWanted && analyserNode ? analyserNode : audioCtx.destination)
  } catch {}
}

// 把已有 source 接到「analyser 链路」或「直连输出」二选一(反复调用不会叠加增益)
function routeSource(throughAnalyser) {
  if (!sourceNode || !audioCtx) return
  try { sourceNode.disconnect() } catch {}
  try {
    if (fxChain?.input) sourceNode.connect(fxChain.input)
    else if (throughAnalyser && analyserNode) sourceNode.connect(analyserNode)
    else sourceNode.connect(audioCtx.destination)
  } catch {}
  rewireFxOutput()
}

function connectSource() {
  if (!audioEl || !audioCtx || sourceNode) return
  if (audioCtx.state !== 'running') return
  try {
    sourceNode = audioCtx.createMediaElementSource(audioEl)
  } catch {
    sourceNode = null
    return
  }
  routeSource(analyserWanted)
}

// 沉浸模式进入时调用。幂等,返回 analyser(浏览器不支持 WebAudio 时返回 null)
function attachAnalyser() {
  const Ctx = audioContextCtor()
  if (!Ctx) return null
  analyserWanted = true
  if (!audioCtx) {
    audioCtx = new Ctx()
    analyserNode = audioCtx.createAnalyser()
    analyserNode.fftSize = 2048
    analyserNode.smoothingTimeConstant = 0.8
    analyserNode.minDecibels = -92
    analyserNode.maxDecibels = -14
    analyserNode.connect(audioCtx.destination)
    audioCtx.onstatechange = () => {
      if (audioCtx?.state === 'running') connectSource()
    }
  }
  if (audioCtx.state === 'suspended') audioCtx.resume().catch(() => {})
  connectSource()
  return analyserNode
}

// 离开沉浸模式:元素重新直连输出(仍保留 source,不能重复创建)
function detachAnalyser() {
  analyserWanted = false
  routeSource(false)
}

function getAnalyser() {
  return analyserNode && audioCtx && audioCtx.state === 'running' ? analyserNode : null
}

// Butterchurn(MilkDrop)需要自己拿 audioContext 建图,单独暴露一个入口
function getAudioContext() {
  attachAnalyser()
  return audioCtx
}

// ── DJ 效果链的注册/摘除(沉浸播放页)──
// chain 必须提供 input / output 两个 AudioNode,中间怎么连由调用方决定。
function registerFxChain(chain) {
  if (!chain?.input || !chain?.output) return null
  fxChain = chain
  attachAnalyser() // 至少要有 audioCtx,否则挂不上去
  routeSource(true)
  return chain
}

function unregisterFxChain() {
  if (fxChain?.output) {
    try { fxChain.output.disconnect() } catch {}
  }
  fxChain = null
  routeSource(analyserWanted)
}

// 搓碟/变速需要直接摸元素(playbackRate 不在 player.state 里管)
function getAudioEl() {
  return audioEl
}

// Derive currently playing item
function currentTrack() {
  if (state.currentIndex < 0 || state.currentIndex >= state.playlist.length) return null
  return state.playlist[state.currentIndex]
}

function buildPlaylist(items) {
  const list = []
  for (const item of items) {
    const asset = item.primaryAsset || item._assets?.[0]
    if (!asset?.asset_url || (asset.kind !== 'audio' && asset.kind !== 'video')) continue
    list.push({
      id: item.id,
      title: item.title || '未命名',
      artist: item.artist || '',
      cover: item.cover || '',
      result_type: asset.kind,
      assetUrl: asset.asset_url,
      kind: asset.kind,
    })
  }
  return list
}

function onAudioError() {
  const track = currentTrack()
  state.isPlaying = false
  setPlaybackState('paused')
  consecutiveErrors++
  // 单曲列表跳下一首只会绕回自己;连续坏歌则一路跳到队列见底。两者都停下。
  if (state.playlist.length <= 1 || consecutiveErrors >= MAX_CONSECUTIVE_ERRORS) {
    state.error = consecutiveErrors >= MAX_CONSECUTIVE_ERRORS
      ? '连续多首歌曲无法播放，已停止自动切歌（文件缺失或格式不被支持）'
      : `「${track?.title || '这首'}」无法播放(文件缺失或格式不被浏览器支持)`
    return
  }
  state.error = `「${track?.title || '这首'}」无法播放，已跳到下一首`
  skipNext()
}
function onLoadedMetadata() { state.duration = audioEl?.duration || 0 }
function onTimeUpdate() { state.currentTime = audioEl?.currentTime || 0 }

// ── Media Session(系统媒体键 / 锁屏 / 蓝牙耳机控制) ──
let mediaSessionReady = false
function setupMediaSession() {
  if (mediaSessionReady || typeof navigator === 'undefined' || !('mediaSession' in navigator)) return
  mediaSessionReady = true
  const ms = navigator.mediaSession
  const handlers = {
    play: () => { audioEl?.play().catch(() => {}) },
    pause: () => { audioEl?.pause() },
    previoustrack: skipPrev,
    nexttrack: skipNext,
    seekto: (details) => {
      if (audioEl && details?.seekTime != null && isFinite(details.seekTime)) audioEl.currentTime = details.seekTime
    },
  }
  for (const [action, handler] of Object.entries(handlers)) {
    try { ms.setActionHandler(action, handler) } catch {}
  }
}

function updateMediaMetadata(track) {
  if (typeof navigator === 'undefined' || !('mediaSession' in navigator)) return
  try {
    navigator.mediaSession.metadata = new MediaMetadata({
      title: track?.title || '未命名',
      artist: track?.artist || '',
      album: siteAlbum(),
      artwork: track?.cover ? [{ src: track.cover, sizes: '512x512' }] : [],
    })
    navigator.mediaSession.playbackState = 'playing'
  } catch {}
}

function setPlaybackState(s) {
  if (typeof navigator === 'undefined' || !('mediaSession' in navigator)) return
  try { navigator.mediaSession.playbackState = s } catch {}
}

function createAudio(url) {
  if (audioEl) {
    audioEl.pause()
    audioEl.removeEventListener('ended', onTrackEnded)
    audioEl.removeEventListener('error', onAudioError)
    audioEl.removeEventListener('loadedmetadata', onLoadedMetadata)
    audioEl.removeEventListener('timeupdate', onTimeUpdate)
    audioEl.src = ''
  }
  // 旧元素的 source 一起废弃:新元素必须新建 MediaElementSource,否则没有频谱
  try { sourceNode?.disconnect() } catch {}
  sourceNode = null
  audioEl = new Audio(url)
  audioEl.volume = state.volume
  audioEl.addEventListener('ended', onTrackEnded)
  audioEl.addEventListener('error', onAudioError)
  audioEl.addEventListener('loadedmetadata', onLoadedMetadata)
  audioEl.addEventListener('timeupdate', onTimeUpdate)
  if (analyserWanted) connectSource()
}

function setVolume(v) {
  const next = Math.min(1, Math.max(0, Number(v) || 0))
  state.volume = next
  if (audioEl) audioEl.volume = next
  try { localStorage.setItem('media_player_volume', String(next)) } catch {}
}

// ── 随机列表:洗牌出一条固定乱序,按顺序走完整个列表不重不漏 ──
function shuffledOrder(len, keepFirst) {
  const arr = Array.from({ length: len }, (_, i) => i)
  for (let i = arr.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
  }
  // keepFirst(当前正在播的)提到最前,后面全是"没听过的顺序"
  if (keepFirst != null && keepFirst >= 0 && keepFirst < len) {
    const idx = arr.indexOf(keepFirst)
    if (idx > 0) {
      arr.splice(idx, 1)
      arr.unshift(keepFirst)
    }
  }
  return arr
}

function rebuildShuffleOrder(keepFirst) {
  if (state.shuffleMode && state.playlist.length > 0) {
    state.shuffleOrder = shuffledOrder(state.playlist.length, keepFirst)
  } else {
    state.shuffleOrder = null
  }
}

/**
 * 切歌前校验洗牌序列还配得上当前队列。
 *
 * 队列是全局单例,任何一条替换 playlist 的路径(单曲点播、后台补齐全库、
 * 旧版本留下的状态)都可能让 shuffleOrder 停留在别的长度。此时
 * shuffleOrder.indexOf(currentIndex) 会得到 -1 或越界下标,playIndex 直接
 * return —— 表现就是「随机播放点了没反应 / 只播一首就停」。
 * 这里统一自愈:长度不符或找不到当前曲就重洗一条,并把当前曲固定在最前。
 */
function ensureShuffleOrder() {
  if (!state.shuffleMode) {
    state.shuffleOrder = null
    return
  }
  const order = state.shuffleOrder
  if (!order || order.length !== state.playlist.length || order.indexOf(state.currentIndex) < 0) {
    rebuildShuffleOrder(state.currentIndex)
  }
}

function onTrackEnded() {
  if (state.loopMode === 'one') {
    if (audioEl) {
      audioEl.currentTime = 0
      audioEl.play().catch(() => {})
    }
  } else {
    skipNext()
  }
}

/** 播放到队尾:停在当前曲,但保留队列(否则 UI 直接退回空态,像「没播过」) */
function endOfQueue() {
  if (audioEl) {
    audioEl.pause()
    try { audioEl.currentTime = 0 } catch { /* 尚未加载到可 seek */ }
  }
  state.isPlaying = false
  state.currentTime = 0
  setPlaybackState('paused')
}

function skipNext() {
  if (state.playlist.length === 0) return
  ensureShuffleOrder()
  let next
  if (state.shuffleMode && state.shuffleOrder) {
    // 沿洗牌队列前进:走完一圈才轮到 loopMode=off 停下
    const len = state.shuffleOrder.length
    const pos = state.shuffleOrder.indexOf(state.currentIndex)
    if (state.loopMode === 'all' || pos < len - 1) {
      next = state.shuffleOrder[(pos + 1) % len]
    } else {
      endOfQueue()
      return
    }
  } else if (state.loopMode === 'all' || state.currentIndex < state.playlist.length - 1) {
    next = (state.currentIndex + 1) % state.playlist.length
  } else {
    // loopMode === 'off' and at the end — stop
    endOfQueue()
    return
  }
  // 兜底:无论如何都要给出一个真实存在的下标,不能静默 return
  if (!Number.isInteger(next) || next < 0 || next >= state.playlist.length) {
    next = (state.currentIndex + 1) % state.playlist.length
  }
  playIndex(next)
}

function skipPrev() {
  if (state.playlist.length === 0) return
  ensureShuffleOrder()
  let prev
  if (state.shuffleMode && state.shuffleOrder) {
    const pos = state.shuffleOrder.indexOf(state.currentIndex)
    prev = state.shuffleOrder[(pos <= 0 ? state.shuffleOrder.length : pos) - 1]
  } else {
    prev = state.currentIndex <= 0 ? state.playlist.length - 1 : state.currentIndex - 1
  }
  if (!Number.isInteger(prev) || prev < 0 || prev >= state.playlist.length) {
    prev = state.currentIndex <= 0 ? state.playlist.length - 1 : state.currentIndex - 1
  }
  playIndex(prev)
}

function playIndex(idx) {
  if (idx < 0 || idx >= state.playlist.length) return
  state.currentIndex = idx
  const track = state.playlist[idx]
  if (!track) return
  // 队列刚被整体替换 / 洗牌序列过期时,这里补洗一次(当前曲固定在最前)
  ensureShuffleOrder()

  if (track.kind === 'audio') {
    createAudio(track.assetUrl)
    setupMediaSession()
    updateMediaMetadata(track)
    Promise.resolve(audioEl.play()).then(() => {
      state.isPlaying = true
      state.error = ''
      consecutiveErrors = 0
    }).catch(err => {
      state.isPlaying = false
      setPlaybackState('paused')
      // AbortError 通常是快速切歌时旧元素的 play() 被取消,新的一首会接管,不用打扰用户
      if (err?.name === 'AbortError') return
      state.error = '浏览器拦截了自动播放，请再点一次播放键'
    })
    // 必须在这里 return:下面那句无条件 `state.isPlaying = true` 会把 catch 里
    // 设的 false 覆盖掉,导致播放被浏览器拒绝时 UI 仍显示"播放中"。
    return
  }
  // Video playback is managed in-component since it needs a DOM element
  state.isPlaying = true
  state.error = ''
  consecutiveErrors = 0
}

function play(itemOrIndex) {
  consecutiveErrors = 0 // 用户主动点播 = 重试,清掉连续失败标记
  if (typeof itemOrIndex === 'number') {
    playIndex(itemOrIndex)
    return
  }
  // Find item in playlist
  const idx = state.playlist.findIndex(t => t.id === itemOrIndex.id)
  if (idx >= 0) {
    playIndex(idx)
  } else {
    // 队列被整首替换成单曲:旧的洗牌序列必须作废,否则它仍是 N 个下标,
    // 下一首会算出越界下标直接不播(随机模式下点歌「只有一首」的根因)。
    state.shuffleOrder = null
    const track = itemOrIndex.primaryAsset || itemOrIndex._assets?.[0]
    if (!track?.asset_url) return
    state.playlist = [{
      id: itemOrIndex.id,
      title: itemOrIndex.title || '未命名',
      artist: itemOrIndex.artist || '',
      cover: itemOrIndex.cover || '',
      result_type: track.kind,
      assetUrl: track.asset_url,
      kind: track.kind,
    }]
    playIndex(0)
  }
}

function togglePlay() {
  if (!audioEl) return
  if (state.isPlaying) {
    audioEl.pause()
    state.isPlaying = false
    setPlaybackState('paused')
  } else {
    audioEl.play().then(() => {
      state.isPlaying = true
      setPlaybackState('playing')
    }).catch(() => {})
  }
}

function stop() {
  consecutiveErrors = 0
  if (audioEl) {
    audioEl.pause()
    audioEl.removeEventListener('ended', onTrackEnded)
    audioEl.removeEventListener('error', onAudioError)
    audioEl.removeEventListener('loadedmetadata', onLoadedMetadata)
    audioEl.removeEventListener('timeupdate', onTimeUpdate)
    audioEl.src = ''
    audioEl = null
  }
  try { sourceNode?.disconnect() } catch {}
  sourceNode = null
  state.isPlaying = false
  state.currentIndex = -1
  state.currentTime = 0
  state.duration = 0
  state.playlist = []
  state.shuffleOrder = null
}

function setLoopMode(mode) {
  state.loopMode = mode
}

function toggleShuffle() {
  state.shuffleMode = !state.shuffleMode
  // 开:从当前曲往后重新洗牌;关:回到列表原顺序
  rebuildShuffleOrder(state.currentIndex)
}

function setShuffle(v) {
  const next = !!v
  if (next !== state.shuffleMode || (next && !state.shuffleOrder)) {
    state.shuffleMode = next
    rebuildShuffleOrder(state.currentIndex)
  }
}

function seek(time) {
  if (audioEl) {
    audioEl.currentTime = time
  }
}

function setPlaylist(items) {
  state.playlist = buildPlaylist(items)
  state.shuffleOrder = null // 整个队列被替换,旧顺序作废
}

function addToPlaylist(item) {
  const track = item.primaryAsset || item._assets?.[0]
  if (!track?.asset_url) return
  const exists = state.playlist.find(t => t.id === item.id)
  if (!exists) {
    // 插入前先让洗牌序列与队列对齐,否则 splice 会把新下标插进一条过期序列
    ensureShuffleOrder()
    state.playlist.push({
      id: item.id,
      title: item.title || '未命名',
      artist: item.artist || '',
      cover: item.cover || '',
      result_type: track.kind,
      assetUrl: track.asset_url,
      kind: track.kind,
    })
    // 随机模式下把新歌插进洗牌队列的随机位置(不排在当前曲之前)
    if (state.shuffleMode && state.shuffleOrder) {
      const insertPos = Math.max(1, Math.floor(Math.random() * (state.shuffleOrder.length + 1)))
      state.shuffleOrder.splice(insertPos, 0, state.playlist.length - 1)
    }
  }
}

function playAll(items, startIndex = 0) {
  consecutiveErrors = 0
  state.playlist = buildPlaylist(items)
  state.shuffleOrder = null // 整个队列被替换,旧顺序作废
  if (state.playlist.length > 0) {
    playIndex(Math.min(startIndex, state.playlist.length - 1))
  }
}

/**
 * 后台把队列整体换成更完整的一份,但「正在播的那首」保持不动。
 *
 * 随机播放入口会先在用户点击的同步栈里起播,再异步补齐全库;这里不能重新
 * createAudio(会打断播放),只改队列与洗牌序列,并把当前曲固定在队首。
 * 返回是否真的替换成功(空列表视为失败,保留原队列)。
 */
function replaceQueue(items, { shuffle = state.shuffleMode } = {}) {
  const list = buildPlaylist(items)
  if (!list.length) return false
  const current = currentTrack()
  let idx = current ? list.findIndex(t => t.id === current.id) : -1
  // 当前曲不在新队列里(例如筛选态补齐全库时数据有差异):把它留在队首,
  // 否则 currentIndex 会指向别的歌,界面与实际声音对不上。
  if (idx < 0 && current) {
    list.unshift(current)
    idx = 0
  }
  state.shuffleMode = !!shuffle
  state.playlist = list
  state.shuffleOrder = null
  state.currentIndex = idx
  if (state.shuffleMode) rebuildShuffleOrder(idx >= 0 ? idx : null)
  return true
}

// Format seconds to mm:ss
function fmtTime(s) {
  if (!s || !isFinite(s)) return '0:00'
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}

export function useMediaPlayer() {
  return {
    state,
    currentTrack,
    play,
    stop,
    togglePlay,
    skipNext,
    skipPrev,
    seek,
    setVolume,
    setLoopMode,
    toggleShuffle,
    setShuffle,
    setPlaylist,
    addToPlaylist,
    playAll,
    replaceQueue,
    buildPlaylist,
    fmtTime,
    attachAnalyser,
    detachAnalyser,
    getAnalyser,
    getAudioContext,
    registerFxChain,
    unregisterFxChain,
    getAudioEl,
  }
}
