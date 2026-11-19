<script setup>
import { computed, ref } from 'vue'
import {
  VideoPlay, VideoPause, ArrowLeftBold, ArrowRightBold, Rank, RefreshRight,
  List, Mute, Star, Edit, Connection, MagicStick, Document,
} from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import { fmtTime, clamp } from '../utils/format.js'

const { store, playback, lyrics, ui, isAdmin } = useMusicContext()
const { state, nowPlaying, hasPlayback } = playback
const { currentSource } = lyrics

const barRef = ref(null)
const dragging = ref(false)
const dragPct = ref(0)
const barPct = computed(() => dragging.value ? dragPct.value * 100 : playback.progress.value)

const currentTrack = computed(() => {
  const id = nowPlaying.value?.id
  return id ? store.tracks.value.find(t => t.id === id) || null : null
})

// 空态封面的色调跟随「发现页推荐」那一首，和整页氛围光保持一致
const idleSeed = computed(() => store.insights.value.recent[0]?.id || 'music')

function pctFromEvent(e) {
  const rect = barRef.value?.getBoundingClientRect()
  if (!rect || !rect.width) return 0
  return clamp((e.clientX - rect.left) / rect.width, 0, 1)
}
function onBarDown(e) {
  if (!state.duration) return
  e.currentTarget.setPointerCapture?.(e.pointerId)
  dragging.value = true
  dragPct.value = pctFromEvent(e)
}
function onBarMove(e) {
  if (dragging.value) dragPct.value = pctFromEvent(e)
}
function onBarUp(e) {
  if (!dragging.value) return
  const pct = pctFromEvent(e)
  dragging.value = false
  if (state.duration) playback.player.seek(pct * state.duration)
}

// 倍速：在预设档位间循环切换，并同步给全局播放器（持久化到 localStorage）。
// 用「找下一档更快」而不是 indexOf，这样即便 localStorage 里存着非预设值
// （旧版本 / 手工改过）也不会从 -1 跳到 0.5× 这种突兀位置。
const ratePresets = playback.player.getRatePresets()
const rateLabel = computed(() => `${state.playbackRate}×`)
function cycleRate() {
  const next = ratePresets.find(r => r > state.playbackRate + 1e-6) ?? ratePresets[0]
  playback.player.setRate(next)
}

function onFavorite() {
  if (currentTrack.value) store.toggleFavorite(currentTrack.value)
}
function onEdit() { if (currentTrack.value) ui.openEdit(currentTrack.value) }
function onLink() { if (currentTrack.value) lyrics.openLink(currentTrack.value) }
function openLyrics() { lyrics.openDrawer(currentTrack.value || nowPlaying.value, 'lyrics') }
function openQueue() { lyrics.openDrawer(currentTrack.value || nowPlaying.value, 'queue') }
</script>

<template>
  <aside class="np ml-card" :class="{ 'np--idle': !hasPlayback }" aria-label="正在播放">
    <div class="np-head">
      <span class="ml-eyebrow">NOW PLAYING</span>
      <span v-if="hasPlayback" class="np-live"><i /> 播放中</span>
    </div>

    <template v-if="hasPlayback && nowPlaying">
      <div class="np-art">
        <CoverArt :track="nowPlaying" size="xl" vinyl :playing="state.isPlaying" />
      </div>

      <div class="np-meta">
        <div class="np-title-row">
          <h2 :title="nowPlaying.title">{{ nowPlaying.title }}</h2>
          <span v-if="currentSource === 'library'" class="ml-tag accent">歌词库</span>
        </div>
        <p>{{ nowPlaying.artist || '未知歌手' }}<template v-if="nowPlaying.album"> · {{ nowPlaying.album }}</template></p>
        <div class="np-meta-actions">
          <button class="ml-icon-btn" :class="{ active: nowPlaying.favorite }" :aria-label="nowPlaying.favorite ? '取消收藏' : '收藏'" @click="onFavorite">
            <el-icon :size="15"><Star /></el-icon>
          </button>
          <button v-if="isAdmin" class="ml-icon-btn" title="编辑信息与歌词" aria-label="编辑信息与歌词" @click="onEdit"><el-icon :size="15"><Edit /></el-icon></button>
          <button v-if="isAdmin" class="ml-icon-btn" :class="{ active: nowPlaying.lyricSongId }" title="关联歌词库" aria-label="关联歌词库" @click="onLink"><el-icon :size="15"><Connection /></el-icon></button>
          <button class="ml-icon-btn" title="沉浸播放" aria-label="沉浸播放" @click="playback.openVisualizer()"><el-icon :size="15"><MagicStick /></el-icon></button>
        </div>
      </div>

      <div class="np-progress">
        <span class="np-time">{{ fmtTime(dragging ? dragPct * state.duration : state.currentTime) }}</span>
        <div
          ref="barRef" class="np-bar" :class="{ dragging }"
          role="slider" tabindex="0"
          aria-label="播放进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(barPct)"
          @pointerdown="onBarDown" @pointermove="onBarMove" @pointerup="onBarUp" @pointercancel="onBarUp"
          @keydown.left.prevent="playback.player.seek(Math.max(0, state.currentTime - 5))"
          @keydown.right.prevent="playback.player.seek(Math.min(state.duration, state.currentTime + 5))"
        >
          <div class="np-bar-track"><div class="np-bar-fill" :style="{ width: barPct + '%' }" /></div>
          <span v-if="dragging" class="np-bar-tip" :style="{ left: barPct + '%' }">{{ fmtTime(dragPct * state.duration) }}</span>
        </div>
        <span class="np-time">{{ fmtTime(state.duration) }}</span>
      </div>

      <div class="np-controls">
        <button class="ml-icon-btn" :class="{ active: state.shuffleMode }" title="随机播放" aria-label="随机播放" @click="playback.player.toggleShuffle()">
          <el-icon :size="16"><Rank /></el-icon>
        </button>
        <button class="ml-icon-btn" title="上一首" aria-label="上一首" @click="playback.player.skipPrev()">
          <el-icon :size="19"><ArrowLeftBold /></el-icon>
        </button>
        <button class="np-play" :aria-label="state.isPlaying ? '暂停' : '播放'" @click="playback.player.togglePlay()">
          <el-icon :size="22"><component :is="state.isPlaying ? VideoPause : VideoPlay" /></el-icon>
        </button>
        <button class="ml-icon-btn" title="下一首" aria-label="下一首" @click="playback.player.skipNext()">
          <el-icon :size="19"><ArrowRightBold /></el-icon>
        </button>
        <button class="ml-icon-btn np-rate" :class="{ active: state.playbackRate !== 1 }" :title="`倍速：${state.playbackRate}×（点击切换）`" aria-label="切换播放倍速" @click="cycleRate()">
          <span class="rate-num">{{ rateLabel }}</span>
        </button>
        <button class="ml-icon-btn" :class="{ active: state.loopMode !== 'off' }" :title="`循环模式：${state.loopMode}`" aria-label="切换循环模式" @click="playback.cycleLoop()">
          <el-icon :size="16"><RefreshRight /></el-icon>
          <span v-if="state.loopMode === 'one'" class="loop-one">1</span>
        </button>
      </div>

      <div class="np-volume">
        <button class="ml-icon-btn" :aria-label="state.volume === 0 ? '取消静音' : '静音'" @click="playback.toggleMute()">
          <el-icon v-if="state.volume === 0" :size="15"><Mute /></el-icon>
          <svg v-else viewBox="0 0 24 24" width="15" height="15" fill="currentColor" aria-hidden="true">
            <path d="M3 9v6h4l5 5V4L7 9H3z" />
            <path d="M16 8.5a5 5 0 0 1 0 7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
        </button>
        <input
          class="vol" type="range" min="0" max="1" step="0.01"
          :value="state.volume" aria-label="音量"
          @input="playback.player.setVolume($event.target.value)"
        >
        <span class="vol-num">{{ Math.round(state.volume * 100) }}</span>
      </div>

      <div class="np-actions">
        <button class="ml-btn ghost np-act" @click="openLyrics">
          <el-icon :size="14"><Document /></el-icon> 歌词
        </button>
        <button class="ml-btn ghost np-act" @click="openQueue">
          <el-icon :size="14"><List /></el-icon> 队列 <span class="tab-count">{{ state.playlist.length }}</span>
        </button>
      </div>
    </template>

    <div v-else class="np-idle">
      <div class="idle-art"><CoverArt :seed="idleSeed" :track="{ id: idleSeed, title: '音' }" size="xl" /></div>
      <h3>让音乐响起来</h3>
      <p>点列表里的任意一首歌，或者从下面开始</p>
      <button class="ml-btn primary" @click="playback.playAllVisible({ shuffle: true })">
        <el-icon :size="14"><VideoPlay /></el-icon> 随机播放
      </button>
      <div class="idle-keys">
        <span><span class="ml-kbd">空格</span> 播放 / 暂停</span>
        <span><span class="ml-kbd">/</span> 搜索</span>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.np {
  position: sticky;
  /* 顶部对齐主内容卡片；底部同时避开常驻播放条与右下角宠物浮窗，
     否则宠物会压在控件上把点击吃掉（实测 900px 高时队列按钮点不到）。 */
  top: var(--ml-sticky-top, 58px);
  align-self: start;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px;
  max-height: calc(100dvh - var(--app-bar-h, 48px) - 36px - var(--player-bar-h, 0px) - var(--np-pet-reserve, 300px));
  overflow: hidden;
}
.np-head { display: flex; align-items: center; justify-content: space-between; }
.np-live { display: inline-flex; align-items: center; gap: 5px; font-size: 10px; color: var(--ml-accent); }
.np-live i { width: 6px; height: 6px; border-radius: 50%; background: var(--ml-accent); }

.np-art { width: min(100%, 184px); align-self: center; }
.np-meta { text-align: center; }
.np-title-row { display: flex; align-items: center; justify-content: center; gap: 8px; }
.np-meta h2 {
  margin: 0; font-size: 18px; font-weight: 750; letter-spacing: -0.02em;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 200px;
}
.np-meta p { margin: 4px 0 8px; font-size: 12px; color: var(--ml-text-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.np-meta-actions { display: flex; align-items: center; justify-content: center; gap: 2px; }
.np-meta-actions .active :deep(svg) { fill: currentColor; }

.np-progress { display: flex; align-items: center; gap: 8px; }
.np-time { font-size: 10px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; min-width: 34px; text-align: center; }
.np-bar { position: relative; flex: 1; height: 18px; display: flex; align-items: center; cursor: pointer; touch-action: none; }
.np-bar-track { width: 100%; height: 5px; border-radius: 99px; background: var(--ml-surface-2); overflow: hidden; }
.np-bar-fill { height: 100%; border-radius: 99px; background: var(--ml-grad); transition: width 0.12s linear; }
.np-bar:hover .np-bar-track, .np-bar.dragging .np-bar-track { height: 7px; }
.np-bar-tip {
  position: absolute; bottom: 20px; transform: translateX(-50%);
  padding: 3px 7px; border-radius: 7px; white-space: nowrap;
  background: var(--ml-elevated); color: var(--ml-text);
  border: 1px solid var(--ml-line); box-shadow: var(--ml-shadow);
  font-size: 10px; font-variant-numeric: tabular-nums; pointer-events: none;
}

.np-controls { display: flex; align-items: center; justify-content: center; gap: 4px; }
.np-controls .ml-icon-btn { position: relative; }
.np-play {
  width: 46px; height: 46px; margin: 0 4px;
  border: none; border-radius: 50%; cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--ml-grad); color: var(--ml-on-accent);
  box-shadow: 0 8px 20px var(--ml-accent-soft);
  transition: transform 0.16s ease;
}
.np-play:hover { transform: scale(1.06); }
.loop-one {
  position: absolute; transform: translate(9px, -9px);
  font-size: 9px; font-weight: 800; color: var(--ml-accent);
}

/* 倍速按钮：做成紧凑胶囊而非圆形图标，容纳「0.75×」这类文字 */
.np-rate { width: auto; min-width: 34px; padding: 0 7px; border-radius: 999px; }
.rate-num { font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; letter-spacing: -0.01em; }

.np-volume { display: flex; align-items: center; gap: 7px; }
.vol {
  flex: 1; height: 4px; appearance: none; -webkit-appearance: none;
  border-radius: 99px; background: var(--ml-surface-2); outline: none; cursor: pointer;
}
.vol::-webkit-slider-thumb {
  appearance: none; -webkit-appearance: none;
  width: 11px; height: 11px; border-radius: 50%;
  background: var(--ml-accent); border: none;
}
.vol::-moz-range-thumb { width: 11px; height: 11px; border-radius: 50%; background: var(--ml-accent); border: none; }
.vol-num { font-size: 10px; color: var(--ml-text-3); width: 24px; text-align: right; font-variant-numeric: tabular-nums; }

.np-actions { display: flex; gap: 6px; padding-top: 6px; border-top: 1px solid var(--ml-line); }
.np-act { flex: 1; height: 32px; font-size: 12px; }
.tab-count { font-size: 10px; opacity: 0.7; }

.np-idle { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; flex: 1; text-align: center; }
.idle-art { width: min(72%, 200px); opacity: 0.9; }
.np-idle h3 { margin: 6px 0 0; font-size: 17px; }
.np-idle p { margin: 0; font-size: 12px; color: var(--ml-text-3); max-width: 200px; line-height: 1.6; }
.idle-keys { display: flex; gap: 12px; margin-top: 10px; font-size: 10px; color: var(--ml-text-3); }
.idle-keys span { display: inline-flex; align-items: center; gap: 4px; }

@media (max-width: 1240px) {
  .np { padding: 12px; gap: 7px; }
  .np-art { width: min(100%, 150px); }
}
/* 平板（769~1080）不显示右侧播放面板；移动端改用下面的固定迷你条。
   隐藏规则放在组件内部，避免父级 .ml-shell 的 display 规则误伤迷你条。 */
@media (min-width: 769px) and (max-width: 1080px) {
  .np { display: none; }
}

@media (max-width: 768px) {
  /* 手机上把「正在播放」收成常驻底部迷你条。它原本是内容流末尾的一整块，
     页面能滚动之后仍要滑很久才够得着暂停 / 切歌，等于听不了歌。 */
  .np {
    position: fixed;
    /* 桌面样式给了 top（sticky 偏移），fixed 时必须清掉，
       否则 top / bottom 同时存在会把迷你条顶到页面上方。 */
    top: auto;
    left: 8px; right: 8px;
    bottom: calc(8px + env(safe-area-inset-bottom, 0px));
    z-index: 40;
    display: grid;
    grid-template-columns: 42px minmax(0, 1fr) auto 34px;
    grid-template-areas: "art meta controls lyrics";
    align-items: center;
    gap: 8px;
    max-height: none;
    padding: 8px 10px 12px;
    border-radius: 16px;
    background: var(--ml-elevated);
    box-shadow: var(--ml-shadow-lg);
    /* 手机端宠物默认收成一枚 🐾，只需要让开它那一小块 */
    --np-pet-reserve: 0px;
  }
  /* 没在播放时不占底部空间，首页本身就有「开始播放」入口 */
  .np--idle { display: none; }

  .np-head, .np-volume, .np-meta-actions, .np-time { display: none; }

  .np-art { grid-area: art; width: 42px; align-self: center; }
  /* size-xl 的大字是 92px，缩进 42px 封面会被裁成半个字，压回小字号 */
  .np-art :deep(.size-xl) { border-radius: 9px; box-shadow: none; }
  .np-art :deep(.size-xl .art-letter) { font-size: 22px; }
  .np-art :deep(.size-xl .art-note) { font-size: 9px; }
  .np-art :deep(.art-play) { display: none; }
  .np-title-row .ml-tag { display: none; }
  .np-meta { grid-area: meta; text-align: left; min-width: 0; }
  .np-title-row { justify-content: flex-start; min-width: 0; }
  .np-meta h2 { font-size: 13px; max-width: none; min-width: 0; }
  .np-meta p { margin: 2px 0 0; font-size: 11px; }

  /* 进度条贴着迷你条底边，仍可点按拖动 */
  .np-progress { position: absolute; left: 10px; right: 10px; bottom: 3px; height: 4px; gap: 0; }
  .np-bar { height: 4px; }
  .np-bar-track { height: 3px; }
  .np-bar-tip { bottom: 12px; }

  .np-controls { grid-area: controls; gap: 2px; }
  /* 只留 上一首 / 播放 / 下一首，随机与循环交给沉浸播放页 */
  .np-controls > :first-child, .np-controls > :last-child { display: none; }
  .np-controls .ml-icon-btn { width: 32px; height: 32px; }
  /* 倍速按钮在手机上保持胶囊形，避免「0.75×」被压成圆点 */
  .np-controls .np-rate { width: auto; min-width: 30px; padding: 0 5px; }
  .np-rate .rate-num { font-size: 10px; }
  .np-play { width: 36px; height: 36px; margin: 0 2px; box-shadow: none; }

  .np-actions { grid-area: lyrics; display: block; padding: 0; border: 0; }
  /* 歌词按钮收成图标，队列入口在手机上让位 */
  .np-act { width: 34px; height: 34px; padding: 0; font-size: 0; justify-content: center; }
  .np-act:nth-child(2), .np-act .tab-count { display: none; }
}

/* 宽屏：播放面板与左右两栏齐平，不再是一张只有半屏高的孤零零小卡。
   空态没有控件，可以让它撑满整列；底部留 96px 避开宠物浮窗和常驻播放条。 */
@media (min-width: 1241px) {
  .np {
    gap: 12px; padding: 20px 18px;
    /* 放大封面与控件后，原来给宠物浮窗预留的 300px 会把 max-height 压到
       ~500px，底部的「歌词 / 队列」按钮直接被 overflow:hidden 裁掉。
       宽屏把预留收窄，并允许纵向滚动兜底（放得下时不会出现滚动条）。 */
    --np-pet-reserve: 120px;
    overflow-y: auto;
    overflow-x: hidden;
  }
  .np-art { width: min(100%, 216px); }
  .np-title-row { gap: 6px; }
  .np-meta h2 { flex: 1; min-width: 0; max-width: none; font-size: 21px; }
  .np-title-row .ml-tag { flex-shrink: 0; white-space: nowrap; }
  .np-meta p { font-size: 13px; }
  .np-time, .rate-num, .vol-num { font-size: 11px; }
  .np-act { height: 38px; font-size: 13px; }
  .np-controls { gap: 6px; }
  .np-play { width: 52px; height: 52px; }

  .np--idle {
    --np-pet-reserve: 0px;
    min-height: calc(100dvh - var(--app-bar-h, 56px) - 96px - var(--player-bar-h, 0px));
  }
  .np-idle { gap: 14px; }
  .idle-art { width: min(64%, 210px); }
  .np-idle h3 { font-size: 23px; }
  .np-idle p { max-width: 240px; font-size: 13px; }
  .np-idle .ml-btn { height: 42px; font-size: 14px; }
  .idle-keys { margin-top: 0; font-size: 11px; }
}
</style>
