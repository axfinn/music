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
// 已因加载失败被跳过的曲目下标。用于打断「一首坏歌 + 单曲列表」时的
// error → skipNext → 绕回自己 → error 无限循环(每轮还发一次网络请求)。
let erroredIndex = -1

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
  state.error = track
    ? `「${track.title}」无法播放(文件缺失或格式不被浏览器支持)`
    : '音频无法播放'
  // 单曲列表 / 已经在这个下标失败过:再 skipNext 只会绕回自己,直接停下。
  if (state.playlist.length <= 1 || erroredIndex === state.currentIndex) {
    erroredIndex = state.currentIndex
    return
  }
  erroredIndex = state.currentIndex
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

function skipNext() {
  if (state.playlist.length === 0) return
  let next
  if (state.shuffleMode && state.shuffleOrder) {
    // 沿洗牌队列前进:走完一圈才轮到 loopMode=off 停下
    const len = state.shuffleOrder.length
    const pos = state.shuffleOrder.indexOf(state.currentIndex)
    if (state.loopMode === 'all' || pos < len - 1) {
      next = state.shuffleOrder[(pos + 1) % len]
    } else {
      stop()
      return
    }
  } else if (state.loopMode === 'all' || state.currentIndex < state.playlist.length - 1) {
    next = (state.currentIndex + 1) % state.playlist.length
  } else {
    // loopMode === 'off' and at the end — stop
    stop()
    return
  }
  playIndex(next)
}

function skipPrev() {
  if (state.playlist.length === 0) return
  let prev
  if (state.shuffleMode && state.shuffleOrder) {
    const pos = state.shuffleOrder.indexOf(state.currentIndex)
    prev = state.shuffleOrder[(pos <= 0 ? state.shuffleOrder.length : pos) - 1]
  } else {
    prev = state.currentIndex <= 0 ? state.playlist.length - 1 : state.currentIndex - 1
  }
  playIndex(prev)
}

function playIndex(idx) {
  if (idx < 0 || idx >= state.playlist.length) return
  state.currentIndex = idx
  const track = state.playlist[idx]
  if (!track) return
  // 队列刚被整体替换过时 order 为空,这里补洗一次(当前曲固定在最前)
  if (state.shuffleMode && !state.shuffleOrder) rebuildShuffleOrder(idx)

  if (track.kind === 'audio') {
    createAudio(track.assetUrl)
    setupMediaSession()
    updateMediaMetadata(track)
    audioEl.play().then(() => {
      state.isPlaying = true
      state.error = ''
    }).catch(() => {
      state.isPlaying = false
      setPlaybackState('paused')
    })
    // 必须在这里 return:下面那句无条件 `state.isPlaying = true` 会把 catch 里
    // 设的 false 覆盖掉,导致播放被浏览器拒绝时 UI 仍显示"播放中"。
    return
  }
  // Video playback is managed in-component since it needs a DOM element
  state.isPlaying = true
}

function play(itemOrIndex) {
  erroredIndex = -1 // 用户主动点播 = 重试,清掉失败标记
  if (typeof itemOrIndex === 'number') {
    playIndex(itemOrIndex)
    return
  }
  // Find item in playlist
  const idx = state.playlist.findIndex(t => t.id === itemOrIndex.id)
  if (idx >= 0) {
    playIndex(idx)
  } else {
    // Add single item to playlist and play
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
  erroredIndex = -1
  state.playlist = buildPlaylist(items)
  state.shuffleOrder = null // 整个队列被替换,旧顺序作废
  if (state.playlist.length > 0) {
    playIndex(Math.min(startIndex, state.playlist.length - 1))
  }
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
