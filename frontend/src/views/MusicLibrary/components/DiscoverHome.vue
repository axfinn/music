<script setup>
import { computed } from 'vue'
import { VideoPlay, VideoPause, Headset, Star, Clock, MagicStick, UploadFilled } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import { fmtDurationHuman, fmtCount, fmtTime } from '../utils/format.js'

const { store, playback, upload, isAdmin } = useMusicContext()
const { insights, tracks, loading } = store
const { state, nowPlaying, hasPlayback } = playback

const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 18) return '下午好'
  return '晚上好'
})

const heroTrack = computed(() => {
  if (nowPlaying.value) return store.tracks.value.find(t => t.id === nowPlaying.value.id) || nowPlaying.value
  return insights.value.recent[0] || null
})
const heroTitle = computed(() => nowPlaying.value?.title || (heroTrack.value?.title ? '从这首开始' : '把音乐放进来'))
const heroSub = computed(() => {
  if (nowPlaying.value) return `${nowPlaying.value.artist || '未知歌手'} · 正在播放`
  if (heroTrack.value) return `${heroTrack.value.artist || '未知歌手'} · 最近添加的一首`
  return '拖入音频文件，或点「上传音乐」建立你自己的曲库'
})
const heroArt = computed(() => heroTrack.value || { id: 'welcome', title: '音乐' })

function onHeroPlay() {
  if (hasPlayback.value && nowPlaying.value) playback.player.togglePlay()
  else if (heroTrack.value?.id) playback.playTrack(heroTrack.value)
  else if (isAdmin.value) upload.open()
}

function onPlayRecent(t) { playback.playTrack(t) }

const quickActions = computed(() => [
  { key: 'shuffle', label: '随机播放', sub: `${insights.value.total} 首`, icon: Headset, run: () => playback.playAllVisible({ shuffle: true }) },
  { key: 'fav', label: '我喜欢的', sub: `${insights.value.favorites} 首`, icon: Star, run: () => store.setSection('favorites') },
  { key: 'recent', label: '最近添加', sub: `${insights.value.recent.length} 首`, icon: Clock, run: () => store.setSection('recent') },
  { key: 'viz', label: '沉浸播放', sub: '音律可视化', icon: MagicStick, run: () => playback.openVisualizer() },
])
</script>

<template>
  <div class="discover">
    <!-- Hero -->
    <section class="hero ml-card">
      <div class="hero-art">
        <CoverArt
          :track="heroArt" size="xl" vinyl
          :playing="state.isPlaying && !!nowPlaying"
        />
      </div>
      <div class="hero-body">
        <span class="ml-eyebrow">{{ nowPlaying ? 'NOW PLAYING' : `${greeting} · 为你推荐` }}</span>
        <h2>{{ heroTitle }}</h2>
        <p>{{ heroSub }}</p>
        <div class="hero-actions">
          <button class="ml-btn primary hero-play" @click="onHeroPlay">
            <el-icon :size="16"><component :is="state.isPlaying && nowPlaying ? VideoPause : VideoPlay" /></el-icon>
            {{ state.isPlaying && nowPlaying ? '暂停' : heroTrack?.id ? '开始播放' : isAdmin ? '上传音乐' : '暂无曲目' }}
          </button>
          <button class="ml-btn" @click="playback.playAllVisible({ shuffle: true })">随机来点</button>
          <button class="ml-btn ghost" @click="playback.openVisualizer()">沉浸模式</button>
        </div>
        <div class="hero-stats">
          <span><strong>{{ fmtCount(insights.total) }}</strong> 首曲目</span>
          <span><strong>{{ insights.artists }}</strong> 位歌手</span>
          <span><strong>{{ fmtDurationHuman(insights.totalDuration) }}</strong> 总时长</span>
        </div>
      </div>
    </section>

    <!-- 快捷入口 -->
    <div class="quick-grid">
      <button v-for="a in quickActions" :key="a.key" class="quick ml-card" @click="a.run()">
        <span class="quick-icon"><el-icon :size="18"><component :is="a.icon" /></el-icon></span>
        <span class="quick-text"><strong>{{ a.label }}</strong><small>{{ a.sub }}</small></span>
        <span class="quick-arrow">→</span>
      </button>
    </div>

    <!-- 最近添加 -->
    <section class="shelf">
      <h3 class="ml-section-title">最近添加 <small>刚入库的声音</small></h3>
      <div v-if="loading" class="shelf-skeleton">
        <div v-for="n in 5" :key="n" class="skel-card ml-skeleton" />
      </div>
      <div v-else-if="!insights.recent.length" class="shelf-empty ml-card">
        <el-icon :size="26"><UploadFilled /></el-icon>
        <span>{{ isAdmin ? '还没有曲目，上传几首开始吧' : '还没有曲目，等管理员上传' }}</span>
        <button v-if="isAdmin" class="ml-btn primary" @click="upload.open()">上传音乐</button>
      </div>
      <div v-else class="shelf-row ml-scroll">
        <div
          v-for="(t, i) in insights.recent" :key="t.id"
          class="shelf-card" role="button" tabindex="0"
          :aria-label="`播放 ${t.title}`"
          :style="{ animationDelay: `${i * 45}ms` }"
          @click="onPlayRecent(t)" @keydown.enter="onPlayRecent(t)"
        >
          <div class="shelf-art">
            <CoverArt :track="t" size="xl" overlay :playing="playback.isCurrent(t) && state.isPlaying" @play="onPlayRecent(t)" />
          </div>
          <span class="shelf-title">{{ t.title }}</span>
          <span class="shelf-sub">{{ t.artist || '未知歌手' }} · {{ fmtTime(t.duration_sec) }}</span>
        </div>
      </div>
    </section>

    <!-- 双栏：常听 / 收藏 -->
    <div class="dual">
      <section v-if="insights.mostPlayed.length" class="rank ml-card">
        <h3 class="ml-section-title">常听的歌 <small>按播放次数</small></h3>
        <button
          v-for="(t, i) in insights.mostPlayed" :key="t.id"
          class="rank-row" @click="onPlayRecent(t)"
        >
          <span class="rank-no">{{ String(i + 1).padStart(2, '0') }}</span>
          <CoverArt :track="t" size="xs" />
          <span class="rank-meta"><strong>{{ t.title }}</strong><small>{{ t.artist || '未知歌手' }}</small></span>
          <span class="rank-count">{{ fmtCount(t.play_count) }} 次</span>
        </button>
      </section>

      <section v-if="insights.loved.length" class="loved ml-card">
        <h3 class="ml-section-title">我喜欢的 <small>{{ insights.favorites }} 首</small></h3>
        <div class="loved-grid">
          <button v-for="t in insights.loved" :key="t.id" class="loved-item" @click="onPlayRecent(t)">
            <CoverArt :track="t" size="xs" />
            <span class="loved-meta"><strong>{{ t.title }}</strong><small>{{ t.artist || '未知歌手' }}</small></span>
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.discover { display: flex; flex-direction: column; gap: 26px; }

.hero {
  display: grid;
  grid-template-columns: minmax(150px, 230px) minmax(0, 1fr);
  gap: 28px;
  padding: 24px;
  overflow: hidden;
  position: relative;
  background:
    radial-gradient(circle at 8% 20%, var(--ml-glow-a), transparent 62%),
    radial-gradient(circle at 100% 100%, var(--ml-glow-b), transparent 55%),
    var(--ml-surface);
}
.hero-art { align-self: center; }
.hero-body { display: flex; flex-direction: column; justify-content: center; gap: 8px; min-width: 0; }
.hero-body h2 {
  margin: 4px 0 0; font-size: clamp(24px, 3vw, 38px);
  font-weight: 800; letter-spacing: -0.035em; line-height: 1.15;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.hero-body p { margin: 0; font-size: 13px; color: var(--ml-text-2); }
.hero-actions { display: flex; flex-wrap: wrap; gap: 9px; margin-top: 12px; }
.hero-play { height: 40px; padding: 0 20px; }
.hero-stats { display: flex; flex-wrap: wrap; gap: 20px; margin-top: 14px; font-size: 12px; color: var(--ml-text-3); }
.hero-stats strong { color: var(--ml-text); font-size: 14px; margin-right: 3px; }

.quick-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 10px; }
.quick {
  display: flex; align-items: center; gap: 11px;
  padding: 12px; cursor: pointer; text-align: left;
  transition: transform 0.18s ease, border-color 0.18s ease;
}
.quick:hover { transform: translateY(-3px); border-color: var(--ml-accent); }
.quick-icon {
  width: 36px; height: 36px; border-radius: 11px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--ml-accent-soft); color: var(--ml-accent);
}
.quick-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.quick-text strong { font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.quick-text small { font-size: 11px; color: var(--ml-text-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.quick-arrow { margin-left: auto; color: var(--ml-text-3); font-size: 15px; }

.shelf h3, .rank h3, .loved h3 { margin-bottom: 12px; }
.shelf-row { display: flex; gap: 14px; overflow-x: auto; padding: 2px 2px 12px; scrollbar-width: thin; }
.shelf-card {
  flex: 0 0 138px; display: flex; flex-direction: column; gap: 4px;
  cursor: pointer; animation: ml-fade-up 0.45s ease both;
}
.shelf-art { border-radius: 12px; overflow: hidden; transition: transform 0.2s ease; }
.shelf-card:hover .shelf-art { transform: translateY(-3px); }
.shelf-title { margin-top: 6px; font-size: 13px; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.shelf-sub { font-size: 11px; color: var(--ml-text-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.shelf-skeleton { display: flex; gap: 14px; }
.skel-card { flex: 0 0 138px; height: 170px; }
.shelf-empty { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 34px; color: var(--ml-text-3); font-size: 13px; }

.dual { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; }
.rank, .loved { padding: 16px; }
.rank h3, .loved h3 { font-size: 16px; }
.rank-row, .loved-item {
  display: flex; align-items: center; gap: 10px;
  width: 100%; padding: 6px; border: none; border-radius: 10px;
  background: transparent; color: var(--ml-text); cursor: pointer; text-align: left;
  transition: background-color 0.16s ease;
}
.rank-row:hover, .loved-item:hover { background: var(--ml-surface-2); }
.rank-no { width: 20px; font-size: 11px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; }
.rank-meta, .loved-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.rank-meta strong, .loved-meta strong { font-size: 13px; font-weight: 650; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rank-meta small, .loved-meta small { font-size: 11px; color: var(--ml-text-3); }
.rank-count { font-size: 11px; color: var(--ml-text-3); white-space: nowrap; }
.loved-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px; }

@media (max-width: 1080px) {
  .quick-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 768px) {
  .discover { gap: 18px; }
  .hero { grid-template-columns: 110px minmax(0, 1fr); gap: 14px; padding: 14px; }
  .hero-body h2 { font-size: 19px; }
  .hero-stats { gap: 10px; margin-top: 8px; }
  .hero-stats span:nth-child(3) { display: none; }
  .hero-actions .ml-btn:nth-child(3) { display: none; }
  .hero-play { height: 36px; padding: 0 13px; font-size: 12px; }
  .quick-grid { gap: 8px; }
  .quick { padding: 9px; }
  .quick-arrow { display: none; }
  .shelf-card { flex-basis: 116px; }
  .dual { grid-template-columns: 1fr; }
}
</style>
