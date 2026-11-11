<template>
  <div
    ref="rootRef"
    class="viz-root"
    :class="{ 'ui-idle': !uiVisible, 'is-fullscreen': isFullscreen, 'is-milk': isMilkScene, 'dj-open': djOpen }"
    :style="{ '--accent': palette.a }"
    @pointermove="onPointerMove"
    @pointerdown="bumpUi(); tapNote()"
    @pointerleave="hideNote"
    tabindex="0"
  >
    <!-- three.js 画布挂这里 -->
    <div ref="stageRef" class="viz-stage" />
    <!-- 3D 歌词/歌名点云层:独立透明画布,叠在任何场景(含 MilkDrop)之上 -->
    <div ref="lyricStageRef" class="viz-lyric-stage" :class="{ off: !lyricFxOn }" />

    <div class="viz-vignette" />
    <div class="viz-grain" />
    <div class="viz-flash" :class="{ on: switching }" />
    <!-- MilkDrop 预设经常亮到爆炸,给顶部/右侧 UI 垫一层随 UI 一起淡出的压暗 -->
    <div class="viz-scrim" />

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
          <span class="viz-brand-sub">IMMERSIVE · {{ sceneEn }}</span>
        </div>
      </div>
      <div class="viz-top-right">
        <span class="viz-live" :class="{ on: playerState.isPlaying }">
          <i />{{ playerState.isPlaying ? 'PLAYING' : 'PAUSED' }}
        </span>
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

    <!-- ── 右侧:效果 + 配色 ── -->
    <aside v-show="!isMobile" class="viz-rail">
      <div class="rail-group">
        <span class="rail-label">SCENE</span>
        <button
          v-for="(s, i) in SCENE_LIST" :key="s.id"
          class="rail-item" :class="{ active: sceneId === s.id }"
          :title="s.hint" @click="switchScene(s.id)"
        >
          <em>{{ String(i + 1).padStart(2, '0') }}</em>
          <span>{{ s.label }}</span>
        </button>
      </div>

      <div class="rail-group">
        <span class="rail-label">WORD FX</span>
        <button
          class="rail-item" :class="{ active: lyricFxOn }"
          :title="lyricFxOn ? '关闭 3D 歌词字效 (L)' : '开启 3D 歌词字效 (L)'"
          @click="toggleLyricFx()"
        >
          <em>{{ lyricFxOn ? 'ON' : 'OFF' }}</em>
          <span>3D 歌词</span>
        </button>
      </div>

      <div class="rail-group">
        <span class="rail-label">MILKDROP · OPEN SOURCE</span>
        <button
          v-for="(p, i) in CURATED_MILK" :key="p.key"
          class="rail-item" :class="{ active: sceneId === `milk:${i}` }"
          :title="`${p.key} — ${p.hint}`" @click="switchScene(`milk:${i}`)"
        >
          <em>M{{ i + 1 }}</em>
          <span>{{ p.label }}</span>
        </button>
        <button
          class="rail-item rail-item-shuffle" :class="{ active: sceneId === 'milk:r' }"
          title="随机抽一个开源预设;开着它时切歌会自动换 (M)" @click="switchScene('milk:r')"
        >
          <em>∞</em>
          <span>{{ milkLoading ? '加载中' : '随机' }}</span>
        </button>
      </div>

      <div class="rail-group" :class="{ muted: isMilkScene }" :title="isMilkScene ? 'MilkDrop 预设自带配色' : ''">
        <span class="rail-label">MOOD</span>
        <div class="rail-swatches">
          <button
            v-for="p in PALETTES" :key="p.id"
            class="rail-swatch" :class="{ active: paletteId === p.id }"
            :style="{ '--sw-a': p.a, '--sw-b': p.b, '--sw-c': p.c }"
            :title="`${p.label} · ${p.hint}`" @click="switchPalette(p.id)"
          />
        </div>
      </div>
    </aside>

    <!-- ── 底部控制 ── -->
    <footer class="viz-bottom">
      <!-- 移动端:效果 + 配色压成一条 -->
      <div v-show="isMobile" class="viz-mobile-bar">
        <button
          v-for="s in SCENE_LIST" :key="s.id"
          class="m-chip" :class="{ active: sceneId === s.id }" @click="switchScene(s.id)"
        >{{ s.label }}</button>
        <button class="m-chip" :class="{ active: lyricFxOn }" @click="toggleLyricFx()">3D 歌词</button>
        <span class="m-sep" />
        <button
          v-for="(p, i) in CURATED_MILK" :key="p.key"
          class="m-chip milk" :class="{ active: sceneId === `milk:${i}` }" @click="switchScene(`milk:${i}`)"
        >{{ p.label }}</button>
        <button class="m-chip milk" :class="{ active: sceneId === 'milk:r' }" @click="switchScene('milk:r')">随机</button>
        <span class="m-sep" />
        <button
          v-for="p in PALETTES" :key="p.id"
          class="m-swatch" :class="{ active: paletteId === p.id }"
          :style="{ '--sw-a': p.a, '--sw-b': p.b, '--sw-c': p.c }" @click="switchPalette(p.id)"
        />
      </div>

      <div class="viz-deck">
        <div class="viz-meta">
          <p class="viz-kicker">
            {{ sceneLabel }}
            <template v-if="isMilkScene && milkKey"> · {{ milkKey }}</template>
            <template v-else> · {{ playerState.playlist.length }} 首队列</template>
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
            空格 播放/暂停 · ←→ 快进 · ↑↓ 音量 · N/P 切歌 · 1-6 换效果 · L 歌词字效 · F 全屏
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
// 沉浸播放页 —— 音乐库的"看的那一半"。
//
// 分工:
//   useMediaPlayer 提供声音(并通过 attachAnalyser 暴露 AnalyserNode)
//   MusicVisualizer/drive.js   把频谱压成 bass/mid/high/beat
//   MusicVisualizer/engine.js  three.js 渲染管线 + 场景切换 + 配色
//   MusicVisualizer/scenes.js  6 个场景
//   MusicVisualizer/lyricfx.js 3D 歌词/歌名点云(独立透明层,叠在所有场景之上)
//
// 三个必须守住的点:
//   1. analyser 要在用户手势里 attach,否则 AudioContext 是 suspended → 整首静音;
//   2. keep-alive 页面离开要 stop 渲染,否则后台一直在烧 GPU;
//   3. 没有频谱(浏览器不支持/元素没接管)不能让画面死掉,drive 里有合成节拍兜底。

import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMediaPlayer } from '../composables/useMediaPlayer.js'
import { request } from '../api/client.js'
import { parseLyrics, detectMode, findActiveIndex } from '../utils/lrc.js'
import { createAudioDrive } from './MusicVisualizer/drive.js'
import { createVisualEngine } from './MusicVisualizer/engine.js'
import { SCENE_LIST } from './MusicVisualizer/scenes.js'
import { PALETTES, getPalette } from './MusicVisualizer/palettes.js'
import { CURATED_MILK, createMilkdropEngine } from './MusicVisualizer/milkdrop.js'
import { createDjEngine } from './MusicVisualizer/dj.js'
import { createLyricFx } from './MusicVisualizer/lyricfx.js'
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

const SCENE_STORAGE = 'music_viz_scene'
const MOOD_STORAGE = 'music_viz_palette'

const sceneId = ref(localStorage.getItem(SCENE_STORAGE) || 'aurora')
const paletteId = ref(localStorage.getItem(MOOD_STORAGE) || 'aurora')
const palette = computed(() => getPalette(paletteId.value))
const isMilkScene = computed(() => sceneId.value.startsWith('milk:'))
const sceneLabel = computed(() => {
  if (isMilkScene.value) {
    if (sceneId.value === 'milk:r') return 'MilkDrop 随机'
    return CURATED_MILK[Number(sceneId.value.slice(5))]?.label || 'MilkDrop'
  }
  return SCENE_LIST.find(s => s.id === sceneId.value)?.label || ''
})
const sceneEn = computed(() => {
  if (isMilkScene.value) return 'MILKDROP'
  return SCENE_LIST.find(s => s.id === sceneId.value)?.en || ''
})

// MilkDrop 实际加载到的预设全名(社区名,显示出来更有"在用老牌预设"的实感)
const milkKey = ref('')
const milkLoading = ref(false)

const track = computed(() => {
  const t = playerState.playlist[playerState.currentIndex]
  return t || null
})

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
  if (i < 0 || !lyrics.value[i]) return ''
  return lyrics.value[i].text
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

// ── three.js ──
let engine = null
let drive = null
let milk = null
let milkPromise = null
let resizeObserver = null
let idleTimer = null

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
// 调试探针:用来在无头浏览器里确认效果链真的建起来了(生产构建摇掉)
if (import.meta.env.DEV) window.__dj = dj

function isPlayingNow() {
  return playerState.isPlaying
}

function initEngine() {
  if (!stageRef.value || engine || typeof window === 'undefined') return
  drive = createAudioDrive({ getAnalyser: () => player.getAnalyser(), isPlaying: isPlayingNow })
  engine = createVisualEngine({ container: stageRef.value, drive, isMobile: isMobile.value })
  engine.setPalette(palette.value)
  engine.mountScene('aurora')
  engine.start()

  resizeObserver = new ResizeObserver(() => {
    engine?.resize()
    milk?.resize()
    lyricFx?.resize()
  })
  resizeObserver.observe(stageRef.value)
}

// ── 3D 歌词/歌名字效 ──
// 独立一层透明画布:不参与 engine 的场景切换,所以切到 MilkDrop 也照样有字。
// 一套粒子的两套落点 —— 平时是当前这句歌词(逐字随拍跳),
// 到拍子上就飞过去把这句词重组成歌名。开关存 localStorage。
const LYRIC_FX_STORAGE = 'music_viz_lyricfx'
const lyricStageRef = ref(null)
const lyricFxOn = ref(localStorage.getItem(LYRIC_FX_STORAGE) !== '0')
let lyricFx = null
let sentLine = null
let sentTitle = null

function syncLyricText() {
  if (!lyricFx) return
  const line = lyricLine.value || ''
  if (line !== sentLine) {
    sentLine = line
    lyricFx.setLine(line) // 空串 = 前奏/间奏:粒子散成歌名待着
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
    isPlaying: isPlayingNow,
    isMobile: isMobile.value,
  })
  if (!lyricFx) return
  lyricFx.setPalette(palette.value)
  syncLyricText()
  if (lyricFxOn.value && !document.hidden) lyricFx.start()
}

function toggleLyricFx() {
  lyricFxOn.value = !lyricFxOn.value
  try { localStorage.setItem(LYRIC_FX_STORAGE, lyricFxOn.value ? '1' : '0') } catch {}
  if (lyricFxOn.value) {
    lyricFx?.start()
    syncLyricText()
  } else {
    lyricFx?.stop()
  }
  bumpUi()
}

watch([lyricLine, () => track.value?.title], syncLyricText)

// ── MilkDrop(开源 Butterchurn,首次进入才下载) ──
async function ensureMilk() {
  if (milk) return milk
  if (milkPromise) return milkPromise
  milkLoading.value = true
  milkPromise = createMilkdropEngine({
    container: stageRef.value,
    getAudioContext: player.getAudioContext,
    getAnalyser: player.getAnalyser,
    isMobile: isMobile.value,
  })
    .then(m => {
      milk = m
      milkLoading.value = false
      return m
    })
    .catch(() => {
      milkLoading.value = false
      return null
    })
  return milkPromise
}

async function applyScene(id) {
  if (id.startsWith('milk:')) {
    engine?.stop()
    const m = await ensureMilk()
    if (!m) {
      // 环境不支持(无 WebGL / 加载失败)→ 老老实实退回自研场景
      sceneId.value = 'aurora'
      engine?.start()
      engine?.mountScene('aurora')
      return
    }
    m.show()
    m.start()
    m.resize()
    if (id === 'milk:r') milkKey.value = m.randomPreset()
    else {
      const item = CURATED_MILK[Number(id.slice(5))]
      milkKey.value = item ? m.loadPreset(item.key) : m.randomPreset()
    }
    return
  }
  if (milk) {
    milk.stop()
    milk.hide()
  }
  engine?.start()
  engine?.mountScene(id)
}

// 「随机」模式下换歌就换一套方程 —— 每首歌都有自己的画面,
// 不用再手动点。精选预设是用户显式选的,切歌时不动它。
watch(() => track.value?.id, (id, prev) => {
  if (!prev || !id || !milk) return // 首次进入由 applyScene 抽,别重复
  if (sceneId.value !== 'milk:r') return
  milkKey.value = milk.randomPreset()
})

// ── 交互 ──
function bumpUi() {
  uiVisible.value = true
  // 手势同时也把 AudioContext 唤醒(自动播放策略下第一次交互才 running)
  player.attachAnalyser?.()
  clearTimeout(idleTimer)
  idleTimer = setTimeout(() => {
    // 台子开着时不自动隐藏:不然手停在旋钮上,鼠标和控件一起消失
    if (playerState.isPlaying && !djOpen.value) uiVisible.value = false
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
// 不用 CSS `cursor: url(svg)`:Safari 对 SVG 光标支持差,而且想加辉光/点击回弹
// 都得自己画。所以整页 cursor:none,拿一个 DOM 元素跟手 —— 直接写 transform,
// 不走响应式,每帧鼠标事件也就一次样式写入。
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
  // 摘掉 class → 强制回流 → 挂回去:连点时动画才会从头再放一遍
  el.classList.remove('tap')
  void el.offsetWidth
  el.classList.add('tap')
  noteTapTimer = setTimeout(() => el.classList.remove('tap'), 460)
}

function hideNote() {
  noteOn.value = false
}

function switchScene(id) {
  // 「随机」是动作不是状态:停在随机上再点它,也要重新抽一个
  if (sceneId.value === id && id !== 'milk:r') return
  sceneId.value = id
  try { localStorage.setItem(SCENE_STORAGE, id) } catch {}
  switching.value = true
  // 半程切换:遮罩最亮的瞬间换画面,避免看到旧场景被拆掉
  setTimeout(() => applyScene(id), 140)
  setTimeout(() => { switching.value = false }, 320)
}

function switchPalette(id) {
  paletteId.value = id
  try { localStorage.setItem(MOOD_STORAGE, id) } catch {}
  engine?.setPalette(palette.value)
  lyricFx?.setPalette(palette.value)
}

function cycleLoop() {
  const order = ['all', 'one', 'off']
  const idx = order.indexOf(playerState.loopMode)
  player.setLoopMode(order[(idx + 1) % order.length])
}

function goBack() {
  router.push('/music')
}

// 没歌时:从音乐库随机抽一首(顺带拉一批做队列,下一首/上一首才有得切),
// 不再把用户踢回音乐库找。复用了音乐库的曲目映射和全局播放器的 playAll。
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
  bumpUi() // 手势内唤醒 AudioContext,免得异步拉歌后整首静音
  randoming.value = true
  try {
    const first = await request('/api/music/tracks?limit=1&offset=0')
    const total = Number(first.total || 0)
    if (!total) return
    const limit = 50
    const offset = Math.min(Math.floor(Math.random() * total), Math.max(0, total - limit))
    const data = await request(`/api/music/tracks?limit=${limit}&offset=${offset}`)
    const items = (data.items || []).filter(t => t.id && t.playable !== false).map(mapTrack)
    if (!items.length) return
    player.playAll(items, Math.floor(Math.random() * items.length))
  } catch {
    // 拉取失败就静默,别卡在转圈
  } finally {
    randoming.value = false
  }
}

// 播放键:有歌就暂停/继续;没歌就随机来一首
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
    // 不支持全屏(如 iOS Safari)时退化成"隐藏 UI 的沉浸态",体验也还成立
    uiVisible.value = !uiVisible.value
  }
}

function onFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
  // 全屏切换后容器尺寸变了,等一帧再量
  setTimeout(() => {
    engine?.resize()
    milk?.resize()
  }, 60)
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
  if (!dragging.value) return
  dragPct.value = ratioFromEvent(e) * 100
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
    e.preventDefault()
    onPlayClick()
  } else if (key === 'ArrowRight') {
    e.preventDefault()
    player.seek(Math.min(playerState.duration || 0, playerState.currentTime + 5))
  } else if (key === 'ArrowLeft') {
    e.preventDefault()
    player.seek(Math.max(0, playerState.currentTime - 5))
  } else if (key === 'ArrowUp') {
    e.preventDefault()
    player.setVolume(Math.min(1, playerState.volume + 0.05))
  } else if (key === 'ArrowDown') {
    e.preventDefault()
    player.setVolume(Math.max(0, playerState.volume - 0.05))
  } else if (key === 'n' || key === 'N') {
    player.skipNext()
  } else if (key === 'p' || key === 'P') {
    player.skipPrev()
  } else if (key === 'f' || key === 'F') {
    toggleFullscreen()
  } else if (key >= '1' && key <= '6') {
    const s = SCENE_LIST[Number(key) - 1]
    if (s) switchScene(s.id)
  } else if (key === 'm' || key === 'M') {
    switchScene('milk:r')
  } else if (key === 'd' || key === 'D') {
    toggleDj()
  } else if (key === 'l' || key === 'L') {
    toggleLyricFx()
  } else if (djOpen.value && 'qwerQWER'.includes(key)) {
    // 面板开着时 Q/W/E/R 打鼓垫
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
  // 字效层自己一条命,不跟着场景分支走
  if (hidden) lyricFx?.stop()
  else if (lyricFxOn.value) lyricFx?.start()

  if (isMilkScene.value) {
    if (hidden) milk?.stop()
    else if (milk) milk.start()
    return
  }
  if (hidden) engine?.stop()
  else engine?.start()
}

onMounted(() => {
  initEngine()
  initLyricFx()
  player.attachAnalyser?.()
  bumpUi()
  // 场景用 localStorage 记住:上次停在哪就续上(包括 MilkDrop 预设)
  applyScene(sceneId.value)
  window.addEventListener('resize', onResizeWindow)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('visibilitychange', onVisibility)
  rootRef.value?.focus?.()
})

// keep-alive 下切页只走 deactivated,不走 unmount ——
// 全局键盘监听放在 activated/deactivated,离开后才不会继续吃空格/方向键
onActivated(() => {
  if (isMilkScene.value) milk?.start()
  else engine?.start()
  if (lyricFxOn.value) lyricFx?.start()
  player.attachAnalyser?.()
  window.addEventListener('keydown', onKeydown)
  if (djOpen.value) dj.attach()
})

onDeactivated(() => {
  window.removeEventListener('keydown', onKeydown)
  engine?.stop()
  milk?.stop()
  lyricFx?.stop()
  dj.detach() // 离开页面立刻摘掉效果链,声音回直连
  player.detachAnalyser?.()
})

onBeforeUnmount(() => {
  clearTimeout(idleTimer)
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

// 路由带 ?palette= 时给个直达配色的入口(分享用)
onMounted(() => {
  const p = route.query.mood
  if (p && PALETTES.some(x => x.id === p)) switchPalette(String(p))
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

/* 音符光标:整页藏掉系统指针,交给 .viz-cursor 自己跟手。
   触屏设备本来就没有指针,媒体查询直接跳过。 */
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
  /* 描一圈深色边:亮场景(白云/霓虹)上纯白音符会糊掉 */
  stroke: rgba(4, 8, 14, 0.55);
  stroke-width: 0.8;
  /* 音符头正好压在指针位置上,像自带热点的系统光标 */
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
/* 3D 歌词点云层:压在场景画布上、暗角之下(同 z-index 靠 DOM 顺序压住,更自然) */
.viz-lyric-stage {
  position: absolute; inset: 0; z-index: 1; pointer-events: none;
  transition: opacity 0.5s ease;
}
.viz-lyric-stage.off { opacity: 0; }
.viz-lyric-stage :deep(.viz-canvas-lyricfx) { display: block; width: 100% !important; height: 100% !important; }
/* MilkDrop 自己的 canvas 叠在 three 上面,隐藏时不占位、不刷新(亮度/对比在下方 .is-milk 里统一调) */
.viz-stage :deep(.viz-canvas-milk) {
  position: absolute; inset: 0; background: #000;
}

/* 暗角 + 颗粒:把"网页"压成"片子" */
.viz-vignette {
  position: absolute; inset: 0; z-index: 1; pointer-events: none;
  background: radial-gradient(120% 92% at 50% 42%, rgba(0, 0, 0, 0) 32%, rgba(0, 0, 0, 0.42) 74%, rgba(0, 0, 0, 0.72) 100%);
}
.viz-grain {
  position: absolute; inset: -20%; z-index: 1; pointer-events: none; opacity: 0.05; mix-blend-mode: overlay;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='140' height='140' filter='url(%23n)'/%3E%3C/svg%3E");
}

/* 换场景时的一次柔光闪白 */
.viz-flash {
  position: absolute; inset: 0; z-index: 2; pointer-events: none;
  background: radial-gradient(60% 50% at 50% 55%, var(--accent) 0%, transparent 70%);
  opacity: 0; transition: opacity 0.32s ease;
}
.viz-flash.on { opacity: 0.18; }

/* ── 顶部 ── */
.viz-top {
  position: absolute; top: 0; left: 0; right: 0; z-index: 5;
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
.viz-live {
  display: inline-flex; align-items: center; gap: 7px;
  font-size: 9px; letter-spacing: 0.26em; color: rgba(255, 255, 255, 0.34);
  padding: 5px 10px; border-radius: 20px; border: 1px solid rgba(255, 255, 255, 0.08);
}
.viz-live i {
  width: 5px; height: 5px; border-radius: 50%; background: rgba(255, 255, 255, 0.3); transition: all 0.3s;
}
.viz-live.on { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 35%, transparent); }
.viz-live.on i { background: var(--accent); box-shadow: 0 0 10px var(--accent); animation: pulse-dot 1.6s ease-in-out infinite; }
@keyframes pulse-dot { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.45; transform: scale(0.7); } }

/* ── 右侧 rail ── */
.viz-rail {
  position: absolute; z-index: 5; right: clamp(16px, 2.4vw, 30px); top: 50%; transform: translateY(-50%);
  display: flex; flex-direction: column; gap: 22px; align-items: flex-end;
  max-height: calc(100vh - 120px); overflow-y: auto; overflow-x: hidden;
  padding: 4px 0; scrollbar-width: none;
  transition: opacity 0.45s ease, transform 0.45s ease;
}
.viz-rail::-webkit-scrollbar { display: none; }
.rail-group { display: flex; flex-direction: column; align-items: flex-end; gap: 9px; }
.rail-group.muted { opacity: 0.3; pointer-events: none; }
.rail-label { font-size: 8px; letter-spacing: 0.3em; color: rgba(255, 255, 255, 0.28); white-space: nowrap; }
.rail-item {
  display: flex; align-items: baseline; gap: 8px;
  background: none; border: none; padding: 3px 0; cursor: pointer;
  color: rgba(255, 255, 255, 0.34); font-size: 12px; letter-spacing: 0.1em;
  transition: color 0.22s ease, transform 0.22s ease;
  font-family: inherit;
}
.rail-item em { font-style: normal; font-size: 8px; letter-spacing: 0.1em; opacity: 0.55; }
.rail-item:hover { color: rgba(255, 255, 255, 0.8); transform: translateX(-3px); }
.rail-item.active { color: #fff; }
.rail-item.active em { color: var(--accent); opacity: 1; }
.rail-item-shuffle em { font-size: 11px; letter-spacing: 0; }
.rail-item-shuffle.active em { color: var(--accent); }
.rail-swatches { display: flex; flex-direction: column; gap: 9px; align-items: flex-end; }
.rail-swatch {
  width: 17px; height: 17px; border-radius: 50%; cursor: pointer; padding: 0;
  border: 1px solid rgba(255, 255, 255, 0.22);
  background: linear-gradient(135deg, var(--sw-a), var(--sw-b) 55%, var(--sw-c));
  transition: transform 0.22s ease, box-shadow 0.22s ease;
}
.rail-swatch:hover { transform: scale(1.16); }
.rail-swatch.active { box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.15), 0 0 0 4px rgba(255, 255, 255, 0.1), 0 0 18px var(--sw-b); transform: scale(1.1); }

/* ── 底部 ── */
.viz-bottom {
  position: absolute; left: 0; right: 0; bottom: 0; z-index: 5;
  padding: 0 clamp(16px, 3vw, 34px) calc(20px + env(safe-area-inset-bottom, 0px));
  background: linear-gradient(to top, rgba(3, 4, 8, 0.86), rgba(3, 4, 8, 0.42) 52%, rgba(3, 4, 8, 0));
  transition: opacity 0.45s ease, transform 0.45s ease;
}
.viz-deck { display: flex; align-items: flex-end; justify-content: space-between; gap: clamp(20px, 4vw, 56px); }
.viz-meta { min-width: 0; flex: 0 1 46%; }
.viz-kicker { margin: 0 0 8px; font-size: 9px; letter-spacing: 0.3em; color: rgba(255, 255, 255, 0.4); text-transform: uppercase; }
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

/* ── 空态(不盖住画面,只放在标题下方) ── */
.viz-cta {
  margin-top: 14px; padding: 9px 22px; border-radius: 30px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.22); background: rgba(255, 255, 255, 0.07);
  color: #fff; font-size: 12px; letter-spacing: 0.14em;
  backdrop-filter: blur(14px); transition: all 0.22s ease;
}
.viz-cta:hover { background: #fff; color: #08090d; }

/* ── 移动端 ── */
.viz-mobile-bar { display: flex; align-items: center; gap: 6px; overflow-x: auto; padding-bottom: 12px; scrollbar-width: none; }
.viz-mobile-bar::-webkit-scrollbar { display: none; }
.m-chip {
  flex: 0 0 auto; padding: 5px 12px; border-radius: 20px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.12); background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.5); font-size: 11px; letter-spacing: 0.08em;
  transition: all 0.2s ease;
}
.m-chip.active { color: #08090d; background: #fff; border-color: transparent; }
.m-chip.milk { border-color: rgba(245, 158, 11, 0.35); color: rgba(245, 158, 11, 0.85); }
.m-chip.milk.active { background: linear-gradient(135deg, #f59e0b, #ef4444); color: #fff; border-color: transparent; }
.m-sep { flex: 0 0 auto; width: 1px; height: 14px; background: rgba(255, 255, 255, 0.14); margin: 0 3px; }
.m-swatch {
  flex: 0 0 auto; width: 17px; height: 17px; border-radius: 50%; cursor: pointer; padding: 0;
  border: 1px solid rgba(255, 255, 255, 0.22);
  background: linear-gradient(135deg, var(--sw-a), var(--sw-b) 55%, var(--sw-c));
  transition: transform 0.2s ease;
}
.m-swatch.active { transform: scale(1.15); box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.35); }

/* ── MilkDrop 态 ──
   社区预设自带暗角 + 高密度细节,页面这套暗角/颗粒如果照旧叠加,
   等于给画面又压了一层灰,亮点全被吃掉。所以这个态下反着来:
   削弱自家暗角、几乎去掉颗粒、底部文字改用更强的压暗层保可读性。 */
.viz-root.is-milk .viz-vignette { opacity: 0.45; }
.viz-root.is-milk .viz-grain { opacity: 0.02; }
.viz-root.is-milk .viz-bottom {
  /* 预设底部经常是高对比色块,纯线性渐变压不住,加长暗部区间 */
  background: linear-gradient(to top, rgba(3, 4, 8, 0.94), rgba(3, 4, 8, 0.62) 44%, rgba(3, 4, 8, 0));
}
/* rail 直接压在画面细节上,给文字一层投影才读得清 */
.viz-root.is-milk .viz-rail { text-shadow: 0 1px 10px rgba(0, 0, 0, 0.95), 0 0 26px rgba(0, 0, 0, 0.7); }
.viz-root.is-milk .rail-label { color: rgba(255, 255, 255, 0.42); }
/* 0.34 的白在亮色预设上基本看不见,这个态下抬到 0.52 */
.viz-root.is-milk .rail-item { color: rgba(255, 255, 255, 0.52); }
.viz-root.is-milk .rail-item.active,
.viz-root.is-milk .rail-item:hover { color: #fff; }
.viz-root.is-milk .viz-stage :deep(.viz-canvas-milk) {
  /* 预设普遍偏亮:继承的 0.88 反馈是"有点闷",这里抬回一点并保住对比 */
  filter: brightness(0.94) contrast(1.06) saturate(1.02);
}
/* 顶部/右侧压暗:只在 MilkDrop 态出现,并且跟着 UI 一起淡出 ——
   亮色预设下 rail 文字几乎读不出来,单靠 text-shadow 压不住大面积高光 */
.viz-scrim {
  position: absolute; inset: 0; z-index: 4; pointer-events: none;
  opacity: 0; transition: opacity 0.45s ease;
  background:
    linear-gradient(to bottom, rgba(3, 4, 8, 0.7), rgba(3, 4, 8, 0.28) 9%, rgba(3, 4, 8, 0) 17%),
    linear-gradient(to left, rgba(3, 4, 8, 0.72), rgba(3, 4, 8, 0.3) 9%, rgba(3, 4, 8, 0) 17%);
}
.viz-root.is-milk .viz-scrim { opacity: 1; }
.viz-root.ui-idle .viz-scrim { opacity: 0; }

/* ── DJ 打碟台 ── */
.viz-dj {
  position: absolute; left: 0; right: 0; bottom: 0; z-index: 30;
  padding: 0 16px 18px; pointer-events: auto;
}
/* 台子占了下半屏,底部 deck 整体上移,歌词/进度不被盖住 */
.viz-root.dj-open .viz-bottom { bottom: 336px; }
.viz-root.dj-open { cursor: default; }
.dj-rise-enter-active, .dj-rise-leave-active {
  transition: opacity 0.26s ease, transform 0.34s cubic-bezier(0.22, 1, 0.36, 1);
}
.dj-rise-enter-from, .dj-rise-leave-to { opacity: 0; transform: translateY(26px); }

/* UI 隐藏态 */
.ui-idle .viz-top { opacity: 0; transform: translateY(-10px); pointer-events: none; }
.ui-idle .viz-bottom { opacity: 0; transform: translateY(14px); pointer-events: none; }
.ui-idle .viz-rail { opacity: 0; transform: translate(8px, -50%); pointer-events: none; }

@media (max-width: 768px) {
  .viz-brand-sub { display: none; }
  .viz-live { display: none; }
  .viz-deck { flex-direction: column; align-items: stretch; gap: 14px; }
  .viz-meta { flex: 0 0 auto; }
  .viz-title { font-size: 26px; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .viz-title-row { gap: 10px; }
  .viz-cover { width: 38px; height: 38px; border-radius: 8px; }
  .viz-lyric { font-size: 12px; margin-top: 8px; }
  /* 横条一定会溢出(6 自研 + 6 精选 + 随机 + 配色),右缘做淡出,免得被裁一半的色块看着像 bug */
  .viz-mobile-bar {
    -webkit-mask-image: linear-gradient(to right, #000 calc(100% - 30px), transparent 100%);
    mask-image: linear-gradient(to right, #000 calc(100% - 30px), transparent 100%);
  }
  .viz-now { max-width: none; }
  .viz-controls { justify-content: center; gap: 6px; }
  .viz-vol { margin-left: auto; }
  .viz-vol-slider { width: 56px; }
  .viz-tip { display: none; }
  .viz-btn.play { width: 48px; height: 48px; }
  .viz-icon-btn { width: 34px; height: 34px; }
  /* 竖屏放不下 deck + 台子:台子接管,deck 让位(台子里有播放/暂停) */
  .viz-root.dj-open .viz-bottom { display: none; }
  .viz-root.dj-open .viz-rail { display: none; }
  .viz-dj { padding: 0; }
}
@media (max-height: 520px) {
  .viz-tip { display: none; }
  .viz-lyric { display: none; }
}
</style>
