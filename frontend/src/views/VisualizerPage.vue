<template>
  <div
    ref="rootRef"
    class="viz-root"
    :class="{
      'ui-idle': !uiVisible,
      'is-fullscreen': isFullscreen,
      'is-open': isOpenSource,
      'dj-open': djOpen,
    }"
    :style="{
      '--accent': palette.a,
      '--tone-brightness': tone.brightness,
      '--tone-contrast': tone.contrast,
      '--tone-saturate': tone.saturate,
    }"
    @pointermove="onPointerMove"
    @pointerdown="bumpUi(); tapNote()"
    @pointerleave="hideNote"
    tabindex="0"
  >
    <!-- 开源 MilkDrop 画布与 LAB 的 three.js 画布共用这个舞台。
         MilkDrop 的 canvas 后挂且绝对定位, show/hide 决定谁露出来。 -->
    <div ref="stageRef" class="viz-stage" />
    <!-- 3D 歌词/歌名点云层:独立透明画布,叠在任何画面之上 -->
    <div ref="lyricStageRef" class="viz-lyric-stage" :class="{ off: !lyricFxOn }" />

    <div class="viz-vignette" />
    <div class="viz-grain" />
    <div class="viz-flash" :class="{ on: switching }" />
    <!-- 社区预设经常亮到爆炸,给顶部/右侧 UI 垫一层随 UI 一起淡出的压暗 -->
    <div class="viz-scrim" />

    <!-- 首屏引擎下载中的过渡(不会出现"黑屏但不知道在干嘛") -->
    <transition name="boot-fade">
      <div v-if="booting" class="viz-boot">
        <i class="viz-boot-ring" />
        <p>正在载入 MILKDROP</p>
        <span>Butterchurn · 开源预设引擎</span>
      </div>
    </transition>

    <!-- ── 顶部 ── -->
    <header class="viz-top">
      <div class="viz-top-left">
        <button class="viz-icon-btn" title="返回音乐库" @click="goBack">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M15 5l-7 7 7 7" />
          </svg>
        </button>
        <div class="viz-brand">
          <span class="viz-brand-main">沉浸播放</span>
          <span class="viz-brand-sub">IMMERSIVE · {{ isOpenSource ? 'MILKDROP · OPEN SOURCE' : 'LAB · SELF-BUILT' }}</span>
        </div>
      </div>
      <div class="viz-top-right">
        <span class="viz-live" :class="{ on: playerState.isPlaying }">
          <i />{{ playerState.isPlaying ? 'PLAYING' : 'PAUSED' }}
        </span>
        <button
          class="viz-icon-btn" :class="{ active: isMobile ? browserOpen : railOpen }"
          :title="(isMobile ? browserOpen : railOpen) ? '收起预设库 (B)' : '打开预设库 (B)'"
          @click="toggleBrowser"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M4 6h16M4 12h16M4 18h10" />
            <circle cx="18" cy="18" r="2.2" />
          </svg>
        </button>
        <button class="viz-icon-btn" :class="{ active: djOpen }" :title="djOpen ? '收起打碟台 (D)' : 'DJ 打碟台 (D)'" @click="toggleDj">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <circle cx="9" cy="12" r="5.2" />
            <circle cx="9" cy="12" r="1.1" fill="currentColor" stroke="none" />
            <path d="M18 4.5v9.2M18 13.7a2.6 2.6 0 100 5.2 2.6 2.6 0 000-5.2z" />
          </svg>
        </button>
        <button class="viz-icon-btn" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'" @click="toggleFullscreen">
          <svg v-if="isFullscreen" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 4H5v4M15 4h4v4M9 20H5v-4M15 20h4v-4" />
          </svg>
          <svg v-else viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
          </svg>
        </button>
      </div>
    </header>

    <!-- ── 开源预设库:桌面右侧常驻,手机是底部抽屉(同一份 DOM) ── -->
    <div v-if="isMobile && browserOpen" class="viz-drawer-mask" @click="browserOpen = false" />
    <aside
      class="viz-rail"
      :class="{ 'as-drawer': isMobile, open: isMobile ? browserOpen : railOpen }"
    >
      <PresetBrowser
        :entries="presetEntries"
        :current-key="presetKey"
        :favorites="favorites"
        :loading="milkLoading"
        :error="milkError"
        :auto-mode="autoMode"
        :tone="tone"
        :is-open-source="isOpenSource"
        :lyric-fx-on="lyricFxOn"
        :lab-scene-id="labSceneId"
        :scenes="SCENE_LIST"
        :palettes="PALETTES"
        :palette-id="paletteId"
        @select="applyPreset"
        @favorite="toggleFavorite"
        @random="randomPreset"
        @step="stepPreset"
        @auto="setAutoMode"
        @toggle-lyric="toggleLyricFx"
        @tone="setTone"
        @select-lab="switchToLab"
        @back-open="switchToOpen"
        @palette="switchPalette"
        @retry="retry"
        @close="closeBrowser"
      />
    </aside>

    <!-- ── 底部控制 ── -->
    <footer class="viz-bottom">
      <!-- 移动端:当前预设 + 两个常用动作,预设库由顶部按钮/这条入口唤出 -->
      <div v-if="isMobile" class="viz-mobile-bar">
        <button class="m-current" @click="browserOpen = !browserOpen">
          <span>{{ isOpenSource ? 'MILKDROP' : 'LAB' }}</span>
          <strong>{{ isOpenSource ? presetTitle : labLabel }}</strong>
          <em>预设库 ›</em>
        </button>
        <button class="m-chip" :class="{ active: lyricFxOn }" @click="toggleLyricFx()">3D 歌词</button>
        <button class="m-chip" @click="randomPreset()">随机</button>
      </div>

      <div class="viz-deck">
        <div class="viz-meta">
          <p class="viz-kicker" :title="isOpenSource ? presetKey : labLabel">
            <template v-if="isOpenSource">MILKDROP · {{ presetTitle }} <span v-if="presetAuthor">— {{ presetAuthor }}</span></template>
            <template v-else>LAB · {{ labLabel }} · {{ playerState.playlist.length }} 首队列</template>
          </p>
          <div class="viz-title-row">
            <div v-if="track && track.cover" class="viz-cover" :style="{ backgroundImage: `url(${track.cover})` }" />
            <h1 class="viz-title">{{ track ? track.title : '还没开始播放' }}</h1>
          </div>
          <p class="viz-artist">{{ track ? (track.artist || '未知歌手') : '挑一首歌，画面会跟着声音长出来' }}</p>
          <button v-if="!track" class="viz-cta" :disabled="randoming" @click="playRandom">
            {{ randoming ? '找歌中…' : '随机来一首' }}
          </button>
          <transition name="lyric-fade">
            <p v-if="lyricLine && !lyricFxOn" :key="lyricLine" class="viz-lyric">{{ lyricLine }}</p>
          </transition>
        </div>

        <div class="viz-now">
          <div class="viz-timeline">
            <span class="viz-time">{{ player.fmtTime(playerState.currentTime) }}</span>
            <div
              ref="barRef"
              class="viz-bar" :class="{ dragging }"
              @pointerdown="onBarDown" @pointermove="onBarMove" @pointerup="onBarUp" @pointercancel="onBarUp"
            >
              <div class="viz-bar-track" />
              <div class="viz-bar-fill" :style="{ width: barPct + '%' }" />
              <div class="viz-bar-dot" :style="{ left: barPct + '%' }" />
            </div>
            <span class="viz-time">{{ player.fmtTime(playerState.duration) }}</span>
          </div>

          <div class="viz-controls">
            <button class="viz-btn small" :class="{ active: playerState.shuffleMode }" title="随机" @click="player.toggleShuffle()">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M16 3h5v5M4 20L21 3M21 16v5h-5M15 15l6 6M4 4l5 5" />
              </svg>
            </button>
            <button class="viz-btn" title="上一首 (P)" @click="player.skipPrev()">
              <svg viewBox="0 0 24 24" width="19" height="19" fill="currentColor"><path d="M7 5h2v14H7zM20 5v14l-11-7z" /></svg>
            </button>
            <button class="viz-btn play" :title="playerState.isPlaying ? '暂停 (空格)' : '播放 (空格)'" @click="onPlayClick()">
              <svg v-if="playerState.isPlaying" viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M8 5h3v14H8zM13 5h3v14h-3z" /></svg>
              <svg v-else viewBox="0 0 24 24" width="24" height="24" fill="currentColor" style="margin-left: 2px"><path d="M8 4l13 8-13 8z" /></svg>
            </button>
            <button class="viz-btn" title="下一首 (N)" @click="player.skipNext()">
              <svg viewBox="0 0 24 24" width="19" height="19" fill="currentColor"><path d="M17 5h-2v14h2zM4 5v14l11-7z" /></svg>
            </button>
            <button class="viz-btn small" :class="{ active: playerState.loopMode !== 'off' }" title="循环" @click="cycleLoop">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 2l4 4-4 4M3 11V9a4 4 0 014-4h14M7 22l-4-4 4-4M21 13v2a4 4 0 01-4 4H3" />
              </svg>
              <em v-if="playerState.loopMode === 'one'" class="viz-badge">1</em>
            </button>

            <div class="viz-vol">
              <button class="viz-btn small" :title="playerState.volume > 0 ? '静音' : '取消静音'" @click="player.setVolume(playerState.volume > 0 ? 0 : 1)">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                  <path d="M4 9v6h3.5L13 20V4L7.5 9H4z" />
                  <path v-if="playerState.volume > 0" d="M16 8.6a5 5 0 010 6.8" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
                  <path v-else d="M16.5 9.5l4 5M20.5 9.5l-4 5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
                </svg>
              </button>
              <input
                class="viz-vol-slider" type="range" min="0" max="1" step="0.01"
                :value="playerState.volume" aria-label="音量"
                @input="player.setVolume($event.target.value)"
              />
            </div>
          </div>

          <p class="viz-tip">
            空格 播放/暂停 · ←→ 快进 · ↑↓ 音量 · N/P 切歌 · [ ] 换预设 · R 随机 · A 自动 · L 字效 · F 全屏
          </p>
        </div>
      </div>
    </footer>

    <!-- ── DJ 打碟台 ── -->
    <!-- duration 必须显式给:靠 transitionend 收尾时,面板会卡在 leave-from 不消失 -->
    <Transition name="dj-rise" :duration="340">
      <div v-if="djOpen" class="viz-dj">
        <DjConsole
          ref="djConsoleRef"
          :dj="dj"
          :is-playing="playerState.isPlaying"
          :track-title="track?.title || ''"
          @close="toggleDj"
          @bump="bumpUi"
          @toggle="player.togglePlay()"
        />
      </div>
    </Transition>

    <!-- 鼠标指针换成一个小音符:整页 cursor:none,由它自己跟手(带辉光,点一下会缩) -->
    <div ref="noteCursorRef" class="viz-cursor" :class="{ on: noteOn }" aria-hidden="true">
      <svg viewBox="0 0 24 24">
        <ellipse cx="8.2" cy="17.7" rx="5" ry="3.65" transform="rotate(-22 8.2 17.7)" />
        <rect x="11.5" y="3.4" width="2.7" height="14.6" rx="1.2" />
        <path d="M14.2 3.6c3.5 1.1 5.2 3.4 5.4 6.6-1.3-2.1-3-3.1-5.4-3.6z" />
      </svg>
      <i class="viz-cursor-pulse" />
    </div>
  </div>
</template>

<script setup>
// 沉浸播放页 —— 音乐库「看的那一半」。
//
// 分工:
//   useMediaPlayer 提供声音(并通过 attachAnalyser 暴露 AnalyserNode)
//   MusicVisualizer/milkdrop.js  ★ 默认画面:Butterchurn(MIT)+ 100 个 MilkDrop 社区预设
//   MusicVisualizer/PresetBrowser.vue 预设库 UI(搜索/收藏/随机/自动轮换/画面调节)
//   MusicVisualizer/engine.js + scenes.js  LAB:6 个自研场景,仅实验与 WebGL2 回退
//   MusicVisualizer/lyricfx.js 3D 歌词/歌名点云(独立透明层,叠在任何画面之上)
//   MusicVisualizer/dj.js      打碟台效果链
//
// 三个必须守住的点:
//   1. analyser 要在用户手势里 attach,否则 AudioContext 是 suspended → 整首静音;
//   2. keep-alive 页面离开要 stop 渲染,否则后台一直在烧 GPU;
//   3. 同一时刻只有一个引擎在渲染:开源预设态停 three,LAB 态停 Butterchurn。
//
// 重构要点(开源优先):
//   以前 6 个自研 three.js 场景占着主位、开源预设只是右侧一小撮「补充」,
//   而且默认进页面看到的是自研极光 —— 观感差距全在这里。现在默认直接跑
//   Butterchurn, 预设库是一等公民, 自研场景收进折叠的 LAB 区。

import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMediaPlayer } from '../composables/useMediaPlayer.js'
import { request } from '../api/client.js'
import { parseLyrics, detectMode, findActiveIndex } from '../utils/lrc.js'
import { createAudioDrive } from './MusicVisualizer/drive.js'
import { createVisualEngine } from './MusicVisualizer/engine.js'
import { SCENE_LIST } from './MusicVisualizer/scenes.js'
import { PALETTES, getPalette } from './MusicVisualizer/palettes.js'
import {
  CURATED_MILK, DEFAULT_PRESET, createMilkdropEngine, loadPresetLibrary,
  parsePresetName, resolveKey,
} from './MusicVisualizer/milkdrop.js'
import { createDjEngine } from './MusicVisualizer/dj.js'
import { createLyricFx } from './MusicVisualizer/lyricfx.js'
import PresetBrowser from './MusicVisualizer/PresetBrowser.vue'
import DjConsole from './MusicVisualizer/DjConsole.vue'

const player = useMediaPlayer()
const playerState = player.state
const route = useRoute()
const router = useRouter()

const rootRef = ref(null)
const stageRef = ref(null)
const barRef = ref(null)

const isMobile = ref(typeof window !== 'undefined' ? window.innerWidth < 768 : false)
const isFullscreen = ref(false)
const uiVisible = ref(true)
const switching = ref(false)
const dragging = ref(false)
const dragPct = ref(0)

// ── 持久化 ────────────────────────────────────────────────
const STORAGE = {
  preset: 'music_viz_preset',
  favorites: 'music_viz_favorites',
  auto: 'music_viz_auto',
  tone: 'music_viz_tone',
  lab: 'music_viz_lab_scene',
  palette: 'music_viz_palette',
}
function read(key, fallback) {
  try { return localStorage.getItem(key) ?? fallback } catch { return fallback }
}
function readJSON(key, fallback) {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : fallback
  } catch { return fallback }
}
function write(key, value) {
  try { localStorage.setItem(key, typeof value === 'string' ? value : JSON.stringify(value)) } catch {}
}

// 旧版存的是自研场景 id / milk:序号, 这里迁移成「预设全名」
function initialPreset() {
  const saved = read(STORAGE.preset, '')
  if (saved) return saved
  const legacy = read('music_viz_scene', '')
  if (legacy.startsWith('milk:')) {
    const i = Number(legacy.slice(5))
    if (Number.isInteger(i) && CURATED_MILK[i]) return CURATED_MILK[i].key
    if (legacy === 'milk:r') return CURATED_MILK[Math.floor(Math.random() * CURATED_MILK.length)].key
  }
  // 默认给「没播放时也有完整画面」的开源预设, 而不是自研场景
  return DEFAULT_PRESET
}

// 开源(open) / 自研实验(lab)
const source = ref('open')
const isOpenSource = computed(() => source.value === 'open')
const presetKey = ref(initialPreset())
const presetEntries = ref([])
const favorites = ref(readJSON(STORAGE.favorites, []))
const autoMode = ref(read(STORAGE.auto, 'track'))
const tone = ref({ brightness: 0.94, contrast: 1.06, saturate: 1.02, ...readJSON(STORAGE.tone, {}) })
const labSceneId = ref(read(STORAGE.lab, 'aurora'))
const paletteId = ref(read(STORAGE.palette, 'aurora'))
const palette = computed(() => getPalette(paletteId.value))
const railOpen = ref(true)
const browserOpen = ref(false)
const milkLoading = ref(true)
const milkError = ref(false)
const booting = ref(true)

const presetInfo = computed(() =>
  presetEntries.value.find(e => e.key === presetKey.value) || parsePresetName(presetKey.value))
const presetTitle = computed(() => presetInfo.value.title)
const presetAuthor = computed(() => presetInfo.value.author)
const labLabel = computed(() => SCENE_LIST.find(s => s.id === labSceneId.value)?.label || '自建场景')

const track = computed(() => playerState.playlist[playerState.currentIndex] || null)
const barPct = computed(() => {
  if (dragging.value) return dragPct.value
  const d = playerState.duration
  return d > 0 ? Math.min(100, (playerState.currentTime / d) * 100) : 0
})

// ── 歌词(有就慢慢滚一行,没有就留白) ──
const lyrics = ref([])
const lyricsMode = ref('linear')
const lyricCache = new Map()

const activeLyric = computed(() => {
  if (!lyrics.value.length) return -1
  return findActiveIndex(lyrics.value, playerState.currentTime, playerState.duration, lyricsMode.value)
})
const lyricLine = computed(() => {
  const i = activeLyric.value
  return i < 0 || !lyrics.value[i] ? '' : lyrics.value[i].text
})

function applyLyrics(text) {
  const lines = parseLyrics(text || '')
  lyrics.value = lines
  lyricsMode.value = detectMode(lines)
}

async function loadLyrics(id) {
  if (!id || !String(id).startsWith('mus_')) {
    lyrics.value = []
    return
  }
  if (lyricCache.has(id)) {
    applyLyrics(lyricCache.get(id))
    return
  }
  try {
    const data = await request(`/api/music/tracks/${id}`)
    const text = data.item?.lyrics || ''
    lyricCache.set(id, text)
    if (track.value?.id === id) applyLyrics(text)
  } catch {
    if (track.value?.id === id) lyrics.value = []
  }
}
watch(() => track.value?.id, id => loadLyrics(id), { immediate: true })

// ── 引擎 ──────────────────────────────────────────────────
let engine = null       // LAB 的 three.js 引擎, 懒创建
let drive = null
let milk = null         // 开源 Butterchurn 引擎
let milkPromise = null
let resizeObserver = null
let idleTimer = null
let autoTimer = 0
let pageActive = true

async function ensureMilk() {
  if (milk) return milk
  if (milkPromise) return milkPromise
  milkPromise = createMilkdropEngine({
    container: stageRef.value,
    getAudioContext: player.getAudioContext,
    getAnalyser: player.getAnalyser,
    isMobile: isMobile.value,
  }).then(m => { milk = m; return m }).catch(() => null)
  return milkPromise
}

function ensureEngine() {
  if (engine || !stageRef.value || typeof window === 'undefined') return
  drive = createAudioDrive({ getAnalyser: () => player.getAnalyser(), isPlaying: () => playerState.isPlaying })
  engine = createVisualEngine({ container: stageRef.value, drive, isMobile: isMobile.value })
  engine.setPalette(palette.value)
  engine.mountScene(labSceneId.value)
}

function setupResizeObserver() {
  if (!stageRef.value || resizeObserver) return
  resizeObserver = new ResizeObserver(() => {
    engine?.resize()
    milk?.resize()
    lyricFx?.resize()
  })
  resizeObserver.observe(stageRef.value)
}

/** 页面启动:先列出预设, 再拉引擎。任何一步失败都可靠退回 LAB。 */
async function boot() {
  milkLoading.value = true
  booting.value = true
  try {
    const lib = await loadPresetLibrary()
    presetEntries.value = lib.entries
    // 版本改名时把存下来的全名校准到真实 key
    if (!lib.keys.includes(presetKey.value)) {
      const real = resolveKey(lib.keys, presetKey.value)
      if (real) presetKey.value = real
    }
  } catch {
    // 预设包单独失败也可能只是网络抖动; 交给下面的引擎加载兜底
  }
  if (source.value !== 'open') {
    milkLoading.value = false
    booting.value = false
    return
  }
  const m = await ensureMilk()
  milkLoading.value = false
  booting.value = false
  if (!m) {
    fallbackToLab()
    return
  }
  m.show()
  m.resize()
  if (pageActive && !document.hidden) m.start()
  const real = m.loadPreset(presetKey.value, 0) || m.randomPreset()
  if (real) {
    presetKey.value = real
    write(STORAGE.preset, real)
  }
}

async function retry() {
  milkPromise = null
  if (milk) { milk.dispose(); milk = null }
  milkError.value = false
  source.value = 'open'
  boot()
}

function fallbackToLab() {
  milkError.value = true
  source.value = 'lab'
  milk?.stop()
  milk?.hide()
  ensureEngine()
  engine?.start()
  engine?.mountScene(labSceneId.value)
}

function flash() {
  switching.value = true
  setTimeout(() => { switching.value = false }, 320)
}

function persistPreset() {
  write(STORAGE.preset, presetKey.value)
}

// ── 预设操作 ──────────────────────────────────────────────
function applyPreset(key, blend = 1.6) {
  if (source.value !== 'open') {
    switchToOpen(false)
  }
  const real = milk ? milk.loadPreset(key, blend) : key
  if (real) {
    presetKey.value = real
    persistPreset()
  }
  flash()
  bumpUi()
}

function randomPreset() {
  if (!milk) {
    // 引擎还没下载完:先记一个精选, boot 结束会应用它
    const pool = CURATED_MILK.filter(p => p.key !== presetKey.value)
    presetKey.value = (pool.length ? pool : CURATED_MILK)[Math.floor(Math.random() * (pool.length || CURATED_MILK.length))].key
    persistPreset()
    bumpUi()
    return
  }
  const real = milk.randomPreset()
  if (real) {
    presetKey.value = real
    persistPreset()
  }
  flash()
  bumpUi()
}

function stepPreset({ dir, keys }) {
  if (!milk) return
  const real = milk.stepPreset(dir, keys)
  if (real) {
    presetKey.value = real
    persistPreset()
  }
  flash()
  bumpUi()
}

function toggleFavorite(key) {
  const i = favorites.value.indexOf(key)
  favorites.value = i < 0 ? [...favorites.value, key] : favorites.value.filter(k => k !== key)
  write(STORAGE.favorites, favorites.value)
  bumpUi()
}

function setTone(next) {
  tone.value = next
  write(STORAGE.tone, next)
  bumpUi()
}

function setAutoMode(mode) {
  autoMode.value = mode
  write(STORAGE.auto, mode)
  bumpUi()
}

// 定时轮换:只在开源态、播放中、页面可见时才换, 免得后台悄悄烧 GPU
function clearAuto() {
  if (autoTimer) clearInterval(autoTimer)
  autoTimer = 0
}
watch(autoMode, mode => {
  clearAuto()
  if (mode === 'time') {
    autoTimer = setInterval(() => {
      if (!pageActive || document.hidden || !playerState.isPlaying || source.value !== 'open') return
      randomPreset()
    }, 45000)
  }
}, { immediate: true })

// 每曲一换:首次进入由 boot 应用上次预设, 不重复抽
watch(() => track.value?.id, (id, prev) => {
  if (!prev || !id || !milk) return
  if (autoMode.value !== 'track' || source.value !== 'open') return
  randomPreset()
})

function switchToLab(id) {
  source.value = 'lab'
  labSceneId.value = id
  write(STORAGE.lab, id)
  milk?.stop()
  milk?.hide()
  ensureEngine()
  engine?.start()
  engine?.mountScene(id)
  if (isMobile.value) browserOpen.value = false
  flash()
  bumpUi()
}

async function switchToOpen(doFlash = true) {
  source.value = 'open'
  engine?.stop()
  const m = await ensureMilk()
  if (!m) {
    fallbackToLab()
    return
  }
  m.show()
  m.resize()
  if (pageActive && !document.hidden) m.start()
  const real = m.loadPreset(presetKey.value, 1.2)
  if (real) {
    presetKey.value = real
    persistPreset()
  }
  if (isMobile.value) browserOpen.value = false
  if (doFlash) flash()
  bumpUi()
}

// ── 3D 歌词/歌名字效 ──
// 独立一层透明画布:不参与场景切换,所以切到开源预设也照样有字。
const LYRIC_FX_STORAGE = 'music_viz_lyricfx'
const lyricStageRef = ref(null)
// 默认关:首屏只让开源预设说话,自建字效不再盖住画面;也少一个 WebGL context。
// 之前明确开过(存了 1)的用户不受影响。
const lyricFxOn = ref(read(LYRIC_FX_STORAGE, '0') === '1')
let lyricFx = null
let sentLine = null
let sentTitle = null

function syncLyricText() {
  if (!lyricFx) return
  const line = lyricLine.value || ''
  if (line !== sentLine) {
    sentLine = line
    lyricFx.setLine(line)
  }
  const title = track.value?.title || ''
  if (title !== sentTitle) {
    sentTitle = title
    lyricFx.setTitle(title, track.value?.artist || '')
  }
}

function initLyricFx() {
  if (lyricFx || !lyricStageRef.value) return
  lyricFx = createLyricFx({
    container: lyricStageRef.value,
    getAnalyser: () => player.getAnalyser(),
    isPlaying: () => playerState.isPlaying,
    isMobile: isMobile.value,
  })
  if (!lyricFx) return
  lyricFx.setPalette(palette.value)
  syncLyricText()
  if (lyricFxOn.value && !document.hidden) lyricFx.start()
}

function toggleLyricFx() {
  lyricFxOn.value = !lyricFxOn.value
  write(LYRIC_FX_STORAGE, lyricFxOn.value ? '1' : '0')
  if (lyricFxOn.value) {
    if (!lyricFx) initLyricFx() // 懒创建:没开过时连第二个 WebGL context 都不建
    else lyricFx.start()
    syncLyricText()
  } else {
    lyricFx?.stop()
  }
  bumpUi()
}
watch([lyricLine, () => track.value?.title], syncLyricText)

// ── DJ 打碟台 ──
// 引擎只建图(挂到 player 的 fxChain 插槽上),面板打开才 attach;
// 收起时 reset 回中性,免得把 EQ 染色/变速带出这个页面。
const djOpen = ref(false)
const djConsoleRef = ref(null)
const dj = createDjEngine({ player })

function toggleDj() {
  djOpen.value = !djOpen.value
  if (djOpen.value) player.attachAnalyser?.()
  else dj.reset()
  bumpUi()
}
if (import.meta.env.DEV) window.__dj = dj

// ── 交互 ──
function bumpUi() {
  uiVisible.value = true
  // 手势同时也把 AudioContext 唤醒(自动播放策略下第一次交互才 running)
  player.attachAnalyser?.()
  clearTimeout(idleTimer)
  idleTimer = setTimeout(() => {
    const typing = document.activeElement?.tagName === 'INPUT'
    // 抽屉开着 / 搜索框有焦点 / 台子开着时不自动隐藏, 否则手停在控件上控件没了
    if (playerState.isPlaying && !djOpen.value && !browserOpen.value && !typing) uiVisible.value = false
  }, 3800)
}

function onPointerMove(e) {
  bumpUi()
  if (e.pointerType !== 'touch') moveNote(e.clientX, e.clientY)
  if (!engine || !stageRef.value) return
  const rect = stageRef.value.getBoundingClientRect()
  const nx = ((e.clientX - rect.left) / rect.width) * 2 - 1
  const ny = ((e.clientY - rect.top) / rect.height) * 2 - 1
  engine.setPointer(nx, ny)
  lyricFx?.setPointer(nx, ny)
}

// ── 音符光标 ──
const noteCursorRef = ref(null)
const noteOn = ref(false)
let noteTapTimer = 0

function moveNote(x, y) {
  const el = noteCursorRef.value
  if (!el) return
  if (!noteOn.value) noteOn.value = true
  el.style.transform = `translate3d(${x}px, ${y}px, 0)`
}
function tapNote() {
  const el = noteCursorRef.value
  if (!el) return
  noteOn.value = true
  clearTimeout(noteTapTimer)
  el.classList.remove('tap')
  void el.offsetWidth
  el.classList.add('tap')
  noteTapTimer = setTimeout(() => el.classList.remove('tap'), 460)
}
function hideNote() { noteOn.value = false }

function toggleBrowser() {
  if (isMobile.value) browserOpen.value = !browserOpen.value
  else railOpen.value = !railOpen.value
  bumpUi()
}
function closeBrowser() {
  if (isMobile.value) browserOpen.value = false
  else railOpen.value = false
  bumpUi()
}

function switchPalette(id) {
  paletteId.value = id
  write(STORAGE.palette, id)
  engine?.setPalette(palette.value)
  lyricFx?.setPalette(palette.value)
}

function cycleLoop() {
  const order = ['all', 'one', 'off']
  const idx = order.indexOf(playerState.loopMode)
  player.setLoopMode(order[(idx + 1) % order.length])
}
function goBack() { router.push('/music') }

// 没歌时:从音乐库随机抽一首(顺带拉一批做队列,下一首/上一首才有得切)
const randoming = ref(false)
function mapTrack(t) {
  return {
    id: t.id,
    title: t.title,
    artist: t.artist,
    cover: t.cover_filename ? `/api/music/cover/${t.id}` : '',
    primaryAsset: { kind: 'audio', asset_url: `/api/music/stream/${t.id}` },
  }
}
async function playRandom() {
  if (randoming.value) return
  bumpUi()
  randoming.value = true
  try {
    const first = await request('/api/music/tracks?limit=1&offset=0')
    const total = Number(first.total || 0)
    if (!total) return
    const limit = 50
    const offset = Math.min(Math.floor(Math.random() * total), Math.max(0, total - limit))
    const data = await request(`/api/music/tracks?limit=${limit}&offset=${offset}`)
    const items = (data.items || []).filter(t => t.id && t.playable !== false).map(mapTrack)
    if (items.length) player.playAll(items, Math.floor(Math.random() * items.length))
  } catch {
    // 拉取失败就静默,别卡在转圈
  } finally {
    randoming.value = false
  }
}
function onPlayClick() {
  if (track.value) player.togglePlay()
  else playRandom()
}

async function toggleFullscreen() {
  const el = rootRef.value
  if (!el) return
  try {
    if (!document.fullscreenElement) {
      const req = el.requestFullscreen || el.webkitRequestFullscreen
      if (!req) throw new Error('unsupported')
      await req.call(el)
    } else {
      await document.exitFullscreen?.()
    }
  } catch {
    uiVisible.value = !uiVisible.value
  }
}
function onFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
  setTimeout(() => { engine?.resize(); milk?.resize() }, 60)
}

// ── 进度条拖拽 ──
function ratioFromEvent(e) {
  const el = barRef.value
  if (!el) return 0
  const rect = el.getBoundingClientRect()
  return Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
}
function onBarDown(e) {
  if (!playerState.duration) return
  dragging.value = true
  dragPct.value = ratioFromEvent(e) * 100
  barRef.value?.setPointerCapture?.(e.pointerId)
}
function onBarMove(e) {
  if (dragging.value) dragPct.value = ratioFromEvent(e) * 100
}
function onBarUp(e) {
  if (!dragging.value) return
  const ratio = ratioFromEvent(e)
  dragging.value = false
  player.seek(ratio * playerState.duration)
}

// ── 键盘 ──
function onKeydown(e) {
  const tag = (e.target?.tagName || '').toLowerCase()
  if (tag === 'input' || tag === 'textarea') return
  const key = e.key
  if (key === ' ' || key === 'Spacebar') {
    e.preventDefault(); onPlayClick()
  } else if (key === 'ArrowRight') {
    e.preventDefault(); player.seek(Math.min(playerState.duration || 0, playerState.currentTime + 5))
  } else if (key === 'ArrowLeft') {
    e.preventDefault(); player.seek(Math.max(0, playerState.currentTime - 5))
  } else if (key === 'ArrowUp') {
    e.preventDefault(); player.setVolume(Math.min(1, playerState.volume + 0.05))
  } else if (key === 'ArrowDown') {
    e.preventDefault(); player.setVolume(Math.max(0, playerState.volume - 0.05))
  } else if (key === 'n' || key === 'N') {
    player.skipNext()
  } else if (key === 'p' || key === 'P') {
    player.skipPrev()
  } else if (key === 'f' || key === 'F') {
    toggleFullscreen()
  } else if (key === '[' || key === ']') {
    const keys = presetEntries.value.filter(x => x.curated || x.verified).map(x => x.key)
    stepPreset({ dir: key === '[' ? -1 : 1, keys })
  } else if (key === 'r' || key === 'R') {
    randomPreset()
  } else if (key === 'a' || key === 'A') {
    const order = ['off', 'track', 'time']
    setAutoMode(order[(order.indexOf(autoMode.value) + 1) % order.length])
  } else if (key === 'b' || key === 'B') {
    toggleBrowser()
  } else if (key === 'm' || key === 'M') {
    switchToOpen()
  } else if (key === 'l' || key === 'L') {
    toggleLyricFx()
  } else if (key >= '1' && key <= '6') {
    const s = SCENE_LIST[Number(key) - 1]
    if (s) switchToLab(s.id)
  } else if (key === 'd' || key === 'D') {
    toggleDj()
  } else if (djOpen.value && 'qwerQWER'.includes(key)) {
    djConsoleRef.value?.playPad({ q: 'kick', w: 'snare', e: 'hat', r: 'clap' }[key.toLowerCase()])
  }
  bumpUi()
}

// ── 生命周期 ──
function onResizeWindow() {
  isMobile.value = window.innerWidth < 768
  engine?.resize()
  milk?.resize()
  lyricFx?.resize()
}
function onVisibility() {
  const hidden = document.hidden
  if (hidden) lyricFx?.stop()
  else if (lyricFxOn.value) lyricFx?.start()

  if (source.value === 'open') {
    if (hidden) milk?.stop()
    else if (milk && pageActive) milk.start()
    return
  }
  if (hidden) engine?.stop()
  else engine?.start()
}

onMounted(() => {
  pageActive = true
  setupResizeObserver()
  if (lyricFxOn.value) initLyricFx()
  player.attachAnalyser?.()
  bumpUi()
  boot()
  window.addEventListener('resize', onResizeWindow)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('visibilitychange', onVisibility)
  rootRef.value?.focus?.()
  const mood = route.query.mood
  if (mood && PALETTES.some(x => x.id === mood)) switchPalette(String(mood))
})

// keep-alive 下切页只走 deactivated,不走 unmount ——
// 全局键盘监听放在 activated/deactivated,离开后才不会继续吃空格/方向键
onActivated(() => {
  pageActive = true
  if (source.value === 'open') {
    if (milk && !document.hidden) milk.start()
  } else engine?.start()
  if (lyricFxOn.value) lyricFx?.start()
  player.attachAnalyser?.()
  window.addEventListener('keydown', onKeydown)
  if (djOpen.value) dj.attach()
})

onDeactivated(() => {
  pageActive = false
  window.removeEventListener('keydown', onKeydown)
  engine?.stop()
  milk?.stop()
  lyricFx?.stop()
  dj.detach()
  player.detachAnalyser?.()
})

onBeforeUnmount(() => {
  pageActive = false
  clearTimeout(idleTimer)
  clearTimeout(noteTapTimer)
  clearAuto()
  window.removeEventListener('resize', onResizeWindow)
  window.removeEventListener('keydown', onKeydown)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('visibilitychange', onVisibility)
  resizeObserver?.disconnect()
  resizeObserver = null
  player.detachAnalyser?.()
  engine?.dispose()
  engine = null
  lyricFx?.dispose()
  lyricFx = null
  milk?.dispose()
  milk = null
  milkPromise = null
  dj.dispose()
})
</script>

<style scoped>
.viz-root {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 100%;
  background: #05060a;
  color: #f5f7fa;
  overflow: hidden;
  outline: none;
  user-select: none;
  -webkit-user-select: none;
  font-family: 'Inter', 'Helvetica Neue', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
.viz-root.ui-idle { cursor: none; }

/* 音符光标 */
@media (hover: hover) and (pointer: fine) {
  .viz-root,
  .viz-root * { cursor: none; }
}
.viz-cursor {
  position: fixed;
  left: 0;
  top: 0;
  width: 0;
  height: 0;
  z-index: 60;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.18s ease;
  transform: translate3d(-200px, -200px, 0);
  will-change: transform;
  color: #fff;
}
.viz-cursor.on { opacity: 1; }
.viz-cursor svg {
  position: absolute;
  width: 30px;
  height: 30px;
  display: block;
  fill: currentColor;
  stroke: rgba(4, 8, 14, 0.55);
  stroke-width: 0.8;
  transform: translate(-10px, -22px) rotate(-8deg);
  transform-origin: 10px 22px;
  filter: drop-shadow(0 0 4px var(--accent)) drop-shadow(0 0 11px var(--accent));
  transition: transform 0.14s cubic-bezier(0.3, 1.6, 0.5, 1);
}
.viz-cursor.tap svg { transform: translate(-10px, -22px) rotate(-8deg) scale(0.72); }
.viz-cursor-pulse {
  position: absolute;
  left: 0;
  top: 0;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 1.5px solid var(--accent);
  opacity: 0;
  transform: translate(-13px, -13px) scale(0.2);
}
.viz-cursor.tap .viz-cursor-pulse { animation: note-ring 0.42s ease-out; }
@keyframes note-ring {
  0% { transform: translate(-13px, -13px) scale(0.2); opacity: 0.85; }
  100% { transform: translate(-13px, -13px) scale(1.6); opacity: 0; }
}
@media (hover: none) {
  .viz-cursor { display: none; }
}

.viz-stage { position: absolute; inset: 0; z-index: 0; }
.viz-stage :deep(.viz-canvas) { display: block; width: 100% !important; height: 100% !important; }
.viz-lyric-stage {
  position: absolute; inset: 0; z-index: 1; pointer-events: none;
  transition: opacity 0.5s ease;
}
.viz-lyric-stage.off { opacity: 0; }
.viz-lyric-stage :deep(.viz-canvas-lyricfx) { display: block; width: 100% !important; height: 100% !important; }
/* Butterchurn 的 canvas 后挂且绝对定位,盖住 LAB 的 three 画布;亮度由画面调节控制 */
.viz-stage :deep(.viz-canvas-milk) {
  position: absolute; inset: 0; background: #000;
  filter: brightness(var(--tone-brightness, 0.94)) contrast(var(--tone-contrast, 1.06)) saturate(var(--tone-saturate, 1.02));
}

/* 暗角 + 颗粒:把"网页"压成"片子" */
.viz-vignette {
  position: absolute; inset: 0; z-index: 1; pointer-events: none;
  background: radial-gradient(120% 92% at 50% 42%, rgba(0, 0, 0, 0) 32%, rgba(0, 0, 0, 0.42) 74%, rgba(0, 0, 0, 0.72) 100%);
}
/* inset 必须留在 0:早先用 -20% 让颗粒层比画面大,结果它是全页唯一超宽元素,
   焦点滚动/平滑滚动偶发会把整页横向拽走一截,右侧露出黑边。noise 本来就是平铺的。 */
.viz-grain {
  position: absolute; inset: 0; z-index: 1; pointer-events: none; opacity: 0.05; mix-blend-mode: overlay;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='140' height='140' filter='url(%23n)'/%3E%3C/svg%3E");
}
/* 社区预设自带暗角 + 高密度细节,页面这套暗角/颗粒照旧叠加会把亮点吃掉 */
.viz-root.is-open .viz-vignette { opacity: 0.45; }
.viz-root.is-open .viz-grain { opacity: 0.02; }

.viz-flash {
  position: absolute; inset: 0; z-index: 2; pointer-events: none;
  background: radial-gradient(60% 50% at 50% 55%, var(--accent) 0%, transparent 70%);
  opacity: 0; transition: opacity 0.32s ease;
}
.viz-flash.on { opacity: 0.14; }

/* 首屏载入 */
.viz-boot {
  position: absolute; left: 50%; top: 46%; z-index: 3; pointer-events: none;
  transform: translate(-50%, -50%); text-align: center;
}
.viz-boot-ring {
  display: block; width: 42px; height: 42px; margin: 0 auto 20px; border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-top-color: var(--accent);
  animation: boot-spin 1.1s linear infinite;
}
@keyframes boot-spin { to { transform: rotate(360deg); } }
.viz-boot p { margin: 0; font-size: 11px; letter-spacing: 0.36em; color: rgba(255, 255, 255, 0.72); }
.viz-boot span { display: block; margin-top: 9px; font-size: 9px; letter-spacing: 0.14em; color: rgba(255, 255, 255, 0.3); }
.boot-fade-leave-active { transition: opacity 0.6s ease; }
.boot-fade-leave-to { opacity: 0; }

/* ── 顶部 ── */
.viz-top {
  position: absolute; top: 0; left: 0; right: 0; z-index: 7;
  display: flex; align-items: center; justify-content: space-between;
  padding: calc(18px + env(safe-area-inset-top, 0px)) clamp(16px, 3vw, 34px) 18px;
  transition: opacity 0.45s ease, transform 0.45s ease;
}
.viz-top-left, .viz-top-right { display: flex; align-items: center; gap: 14px; }
.viz-icon-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 38px; height: 38px; border-radius: 50%; cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(14px); -webkit-backdrop-filter: blur(14px);
  transition: all 0.22s ease;
}
.viz-icon-btn:hover { background: rgba(255, 255, 255, 0.12); color: #fff; border-color: rgba(255, 255, 255, 0.26); }
.viz-icon-btn.active {
  color: var(--accent); border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 16%, transparent);
  box-shadow: 0 0 16px color-mix(in srgb, var(--accent) 34%, transparent);
}
.viz-brand { display: flex; flex-direction: column; gap: 3px; }
.viz-brand-main { font-size: 13px; letter-spacing: 0.16em; color: rgba(255, 255, 255, 0.9); }
.viz-brand-sub { font-size: 9px; letter-spacing: 0.3em; color: rgba(255, 255, 255, 0.36); }
/* 开源预设常有高亮线框直接穿过左上角/左下角,光靠顶层 scrim 压不住细字 */
.viz-root.is-open .viz-brand { text-shadow: 0 1px 9px rgba(0, 0, 0, 0.95), 0 0 22px rgba(0, 0, 0, 0.8); }
.viz-root.is-open .viz-brand-sub { color: rgba(255, 255, 255, 0.55); }
.viz-root.is-open .viz-kicker,
.viz-root.is-open .viz-title,
.viz-root.is-open .viz-artist,
.viz-root.is-open .viz-lyric,
.viz-root.is-open .viz-cta { text-shadow: 0 1px 10px rgba(0, 0, 0, 0.95), 0 2px 26px rgba(0, 0, 0, 0.8); }
.viz-root.is-open .viz-kicker { color: rgba(255, 255, 255, 0.72); }
.viz-root.is-open .viz-artist { color: rgba(255, 255, 255, 0.66); }
.viz-root.is-open .viz-tip { color: rgba(255, 255, 255, 0.52); }
.viz-live {
  display: inline-flex; align-items: center; gap: 7px;
  font-size: 9px; letter-spacing: 0.26em; color: rgba(255, 255, 255, 0.34);
  padding: 5px 10px; border-radius: 20px; border: 1px solid rgba(255, 255, 255, 0.08);
}
.viz-live i { width: 5px; height: 5px; border-radius: 50%; background: rgba(255, 255, 255, 0.3); transition: all 0.3s; }
.viz-live.on { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 35%, transparent); }
.viz-live.on i { background: var(--accent); box-shadow: 0 0 10px var(--accent); animation: pulse-dot 1.6s ease-in-out infinite; }
@keyframes pulse-dot { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.45; transform: scale(0.7); } }

/* ── 预设库外壳(桌面右侧 / 手机底部抽屉共用一份 DOM) ── */
.viz-rail {
  position: absolute; z-index: 6;
  right: clamp(16px, 2.4vw, 30px); top: 50%;
  width: 272px; height: min(560px, calc(100vh - 190px));
  opacity: 0; pointer-events: none;
  transform: translate(22px, -50%);
  transition: opacity 0.45s ease, transform 0.45s cubic-bezier(0.22, 1, 0.36, 1);
}
.viz-rail.open { opacity: 1; pointer-events: auto; transform: translate(0, -50%); }
.viz-rail.as-drawer {
  position: fixed;
  left: 12px; right: 12px; top: auto; bottom: calc(12px + env(safe-area-inset-bottom, 0px));
  width: auto; height: min(62vh, 540px);
  transform: translateY(115%);
}
.viz-rail.as-drawer.open { transform: translateY(0); }
.viz-drawer-mask {
  position: fixed; inset: 0; z-index: 5;
  background: rgba(3, 4, 8, 0.5);
  backdrop-filter: blur(2px); -webkit-backdrop-filter: blur(2px);
}

/* 顶部/右侧压暗:亮色预设下 UI 文字几乎读不出来,单靠 text-shadow 压不住大面积高光 */
.viz-scrim {
  position: absolute; inset: 0; z-index: 4; pointer-events: none;
  opacity: 0; transition: opacity 0.45s ease;
  background:
    linear-gradient(to bottom, rgba(3, 4, 8, 0.7), rgba(3, 4, 8, 0.28) 9%, rgba(3, 4, 8, 0) 17%),
    linear-gradient(to left, rgba(3, 4, 8, 0.72), rgba(3, 4, 8, 0.3) 9%, rgba(3, 4, 8, 0) 17%);
}
.viz-root.is-open .viz-scrim { opacity: 1; }
.viz-root.ui-idle .viz-scrim { opacity: 0; }

/* ── 底部 ── */
.viz-bottom {
  position: absolute; left: 0; right: 0; bottom: 0; z-index: 5;
  padding: 0 clamp(16px, 3vw, 34px) calc(20px + env(safe-area-inset-bottom, 0px));
  background: linear-gradient(to top, rgba(3, 4, 8, 0.86), rgba(3, 4, 8, 0.42) 52%, rgba(3, 4, 8, 0));
  transition: opacity 0.45s ease, transform 0.45s ease;
}
.viz-root.is-open .viz-bottom {
  /* 预设底部经常是高对比色块,纯线性渐变压不住,加长暗部区间 */
  background: linear-gradient(to top, rgba(3, 4, 8, 0.94), rgba(3, 4, 8, 0.62) 44%, rgba(3, 4, 8, 0));
}
/* 左下歌名/歌手那一片再垫一层局部暗晕:高亮线框穿过时细字才读得出来 */
.viz-root.is-open .viz-bottom::before {
  content: '';
  position: absolute; inset: 0;
  pointer-events: none;
  background: radial-gradient(48% 100% at 10% 100%, rgba(3, 4, 8, 0.78), rgba(3, 4, 8, 0.34) 52%, transparent 100%);
}
.viz-deck {
  position: relative; z-index: 1;
  display: flex; align-items: flex-end; justify-content: space-between; gap: clamp(20px, 4vw, 56px);
}
.viz-meta { min-width: 0; flex: 0 1 46%; }
.viz-kicker {
  margin: 0 0 8px; font-size: 10px; letter-spacing: 0.12em; color: rgba(255, 255, 255, 0.54);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.viz-kicker span { color: rgba(255, 255, 255, 0.28); }
.viz-title-row { display: flex; align-items: center; gap: 14px; min-width: 0; }
.viz-cover {
  width: 46px; height: 46px; border-radius: 10px; flex-shrink: 0;
  background-size: cover; background-position: center;
  box-shadow: 0 8px 26px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.12) inset;
}
.viz-title {
  margin: 0; font-size: clamp(26px, 3.4vw, 46px); font-weight: 200;
  letter-spacing: -0.02em; line-height: 1.1; color: #fff;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  text-shadow: 0 2px 30px rgba(0, 0, 0, 0.55);
}
.viz-artist { margin: 8px 0 0; font-size: 12px; letter-spacing: 0.13em; color: rgba(255, 255, 255, 0.45); }
.viz-lyric {
  margin: 12px 0 0; font-size: 14px; line-height: 1.6; color: color-mix(in srgb, var(--accent) 70%, #ffffff);
  max-width: 34em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  text-shadow: 0 0 22px color-mix(in srgb, var(--accent) 40%, transparent);
}
.lyric-fade-enter-active, .lyric-fade-leave-active { transition: opacity 0.35s ease, transform 0.35s ease; }
.lyric-fade-enter-from { opacity: 0; transform: translateY(6px); }
.lyric-fade-leave-to { opacity: 0; transform: translateY(-6px); }
.lyric-fade-leave-active { position: absolute; }

.viz-now { flex: 1 1 auto; max-width: 620px; display: flex; flex-direction: column; gap: 12px; }
.viz-timeline { display: flex; align-items: center; gap: 12px; }
.viz-time { font-size: 10px; letter-spacing: 0.12em; color: rgba(255, 255, 255, 0.42); font-variant-numeric: tabular-nums; min-width: 34px; text-align: center; }
.viz-bar { position: relative; flex: 1; height: 18px; cursor: pointer; touch-action: none; }
.viz-bar-track {
  position: absolute; left: 0; right: 0; top: 50%; height: 2px; transform: translateY(-50%);
  border-radius: 2px; background: rgba(255, 255, 255, 0.16);
}
.viz-bar-fill {
  position: absolute; left: 0; top: 50%; height: 2px; transform: translateY(-50%);
  border-radius: 2px; background: linear-gradient(90deg, var(--accent), #fff);
  box-shadow: 0 0 14px color-mix(in srgb, var(--accent) 70%, transparent);
}
.viz-bar-dot {
  position: absolute; top: 50%; width: 7px; height: 7px; margin-left: -3.5px;
  border-radius: 50%; background: #fff; transform: translateY(-50%) scale(0.001);
  transition: transform 0.2s ease; box-shadow: 0 0 12px var(--accent);
}
.viz-bar:hover .viz-bar-dot, .viz-bar.dragging .viz-bar-dot { transform: translateY(-50%) scale(1); }

.viz-controls { display: flex; align-items: center; gap: 8px; }
.viz-btn {
  position: relative; display: inline-flex; align-items: center; justify-content: center;
  width: 42px; height: 42px; border-radius: 50%; cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: rgba(255, 255, 255, 0.045); color: rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(16px); -webkit-backdrop-filter: blur(16px);
  transition: all 0.2s ease;
  font-family: inherit;
}
.viz-btn:hover { background: rgba(255, 255, 255, 0.13); color: #fff; transform: translateY(-1px); }
.viz-btn:active { transform: translateY(0) scale(0.96); }
.viz-btn.small { width: 34px; height: 34px; color: rgba(255, 255, 255, 0.5); border-color: rgba(255, 255, 255, 0.08); }
.viz-btn.small.active { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 45%, transparent); }
.viz-btn.play {
  width: 54px; height: 54px; margin: 0 6px;
  background: #fff; color: #08090d; border-color: transparent;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.45), 0 0 0 0 color-mix(in srgb, var(--accent) 60%, transparent);
}
.viz-btn.play:hover { background: #fff; transform: scale(1.04); box-shadow: 0 10px 34px rgba(0, 0, 0, 0.5), 0 0 26px color-mix(in srgb, var(--accent) 45%, transparent); }
.viz-badge {
  position: absolute; top: 2px; right: 2px; font-style: normal; font-size: 8px;
  color: #08090d; background: var(--accent); border-radius: 50%;
  width: 11px; height: 11px; line-height: 11px; text-align: center;
}
.viz-vol { display: flex; align-items: center; gap: 6px; margin-left: 10px; }
.viz-vol-slider {
  -webkit-appearance: none; appearance: none; width: 74px; height: 2px; border-radius: 2px;
  background: rgba(255, 255, 255, 0.18); outline: none; cursor: pointer;
}
.viz-vol-slider::-webkit-slider-thumb {
  -webkit-appearance: none; width: 9px; height: 9px; border-radius: 50%;
  background: #fff; box-shadow: 0 0 10px var(--accent);
}
.viz-vol-slider::-moz-range-thumb { width: 9px; height: 9px; border: none; border-radius: 50%; background: #fff; }
.viz-tip { margin: 2px 0 0; font-size: 9px; letter-spacing: 0.14em; color: rgba(255, 255, 255, 0.22); }

.viz-cta {
  margin-top: 14px; padding: 9px 22px; border-radius: 30px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.22); background: rgba(255, 255, 255, 0.07);
  color: #fff; font-size: 12px; letter-spacing: 0.14em;
  backdrop-filter: blur(14px); transition: all 0.22s ease;
}
.viz-cta:hover { background: #fff; color: #08090d; }

/* ── 移动端底部入口 ── */
.viz-mobile-bar { display: flex; align-items: center; gap: 6px; padding-bottom: 12px; }
.m-current {
  flex: 1; min-width: 0; display: flex; align-items: baseline; gap: 7px;
  padding: 7px 12px; border-radius: 20px; cursor: pointer; font-family: inherit; text-align: left;
  border: 1px solid rgba(255, 255, 255, 0.16); background: rgba(8, 10, 16, 0.78);
  color: rgba(255, 255, 255, 0.8);
  backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
}
.m-current span { font-size: 8px; letter-spacing: 0.16em; color: var(--accent); flex: 0 0 auto; }
.m-current strong {
  flex: 1; min-width: 0; font-size: 11px; font-weight: 400;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.m-current em { font-style: normal; font-size: 9px; color: rgba(255, 255, 255, 0.68); flex: 0 0 auto; }
.m-chip {
  flex: 0 0 auto; padding: 6px 12px; border-radius: 20px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.12); background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.5); font-size: 11px; letter-spacing: 0.08em;
  transition: all 0.2s ease;
}
.m-chip.active { color: #08090d; background: #fff; border-color: transparent; }

/* ── DJ 打碟台 ── */
.viz-dj {
  position: absolute; left: 0; right: 0; bottom: 0; z-index: 30;
  padding: 0 16px 18px; pointer-events: auto;
}
.viz-root.dj-open .viz-bottom { bottom: 336px; }
.viz-root.dj-open { cursor: default; }
.dj-rise-enter-active, .dj-rise-leave-active {
  transition: opacity 0.26s ease, transform 0.34s cubic-bezier(0.22, 1, 0.36, 1);
}
.dj-rise-enter-from, .dj-rise-leave-to { opacity: 0; transform: translateY(26px); }

/* UI 隐藏态 */
.ui-idle .viz-top { opacity: 0; transform: translateY(-10px); pointer-events: none; }
.ui-idle .viz-bottom { opacity: 0; transform: translateY(14px); pointer-events: none; }
/* 手机抽屉是用户显式打开的,不跟着 UI 一起淡出 */
.ui-idle .viz-rail:not(.as-drawer) { opacity: 0; pointer-events: none; }

@media (max-width: 768px) {
  .viz-brand-sub { display: none; }
  .viz-live { display: none; }
  /* 当前预设名已在底部入口条里,不再重复占一行 */
  .viz-kicker { display: none; }
  .viz-deck { flex-direction: column; align-items: stretch; gap: 14px; }
  .viz-meta { flex: 0 0 auto; }
  .viz-title { font-size: 26px; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .viz-title-row { gap: 10px; }
  .viz-cover { width: 38px; height: 38px; border-radius: 8px; }
  .viz-lyric { font-size: 12px; margin-top: 8px; }
  .viz-now { max-width: none; }
  .viz-controls { justify-content: center; gap: 6px; }
  .viz-vol { margin-left: auto; }
  .viz-vol-slider { width: 56px; }
  .viz-tip { display: none; }
  .viz-btn.play { width: 48px; height: 48px; }
  .viz-icon-btn { width: 34px; height: 34px; }
  .viz-root.dj-open .viz-bottom { display: none; }
  .viz-root.dj-open .viz-rail { display: none; }
  .viz-dj { padding: 0; }
}
@media (max-height: 520px) {
  .viz-tip { display: none; }
  .viz-lyric { display: none; }
}
</style>
