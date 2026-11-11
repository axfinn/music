<script setup>
import { computed } from 'vue'
import {
  House, Headset, Clock, Star, FolderOpened, User,
  UploadFilled, MagicStick, Connection, QuestionFilled,
} from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import { fmtDurationHuman, fmtCount, humanSize } from '../utils/format.js'

const { store, playback, lyrics, upload, isAdmin } = useMusicContext()
const { section, insights, loading } = store
const { linkBusy } = lyrics

const browse = [
  // section key 沿用 'discover'，避免让已保存的偏好失效；
  // 但 UI 上叫「曲库首页」，和后台的「歌词发现」区分开。
  { key: 'discover', label: '曲库首页', icon: House },
  { key: 'all', label: '全部曲目', icon: Headset },
  { key: 'recent', label: '最近添加', icon: Clock },
]
const mine = [
  { key: 'favorites', label: '我喜欢的', icon: Star },
  { key: 'albums', label: '专辑', icon: FolderOpened },
  { key: 'artists', label: '歌手', icon: User },
]

const shortcuts = computed(() => [
  ['空格', '播放 / 暂停'],
  ['← →', '快退 / 快进 5 秒'],
  ['J / K', '下一首 / 上一首'],
  ['M', '静音切换'],
  ['/', '聚焦搜索'],
])

const statsRows = computed(() => [
  { label: '曲目', value: fmtCount(insights.value.total) },
  { label: '总时长', value: fmtDurationHuman(insights.value.totalDuration) },
  { label: '累计播放', value: fmtCount(insights.value.totalPlays) },
  { label: '占用空间', value: humanSize(insights.value.totalBytes) },
])
</script>

<template>
  <aside class="sidebar ml-card">
    <div class="brand">
      <span class="brand-mark"><el-icon :size="20"><Headset /></el-icon></span>
      <div class="brand-text">
        <strong>音乐库</strong>
        <span class="ml-eyebrow">MY LIBRARY</span>
      </div>
    </div>

    <nav class="nav" aria-label="曲库导航">
      <span class="nav-label">浏览</span>
      <button
        v-for="item in browse" :key="item.key"
        class="nav-item" :class="{ active: section === item.key }"
        :aria-current="section === item.key ? 'page' : undefined"
        @click="store.setSection(item.key)"
      >
        <el-icon :size="17"><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </button>

      <span class="nav-label">我的音乐</span>
      <button
        v-for="item in mine" :key="item.key"
        class="nav-item" :class="{ active: section === item.key }"
        :aria-current="section === item.key ? 'page' : undefined"
        @click="store.setSection(item.key)"
      >
        <el-icon :size="17"><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
        <span v-if="item.key === 'favorites'" class="nav-count">{{ insights.favorites }}</span>
      </button>
    </nav>

    <div class="side-actions">
      <button v-if="isAdmin" class="ml-btn primary upload-btn" @click="upload.open()">
        <el-icon :size="15"><UploadFilled /></el-icon> 上传音乐
      </button>
      <button class="ml-btn viz-btn" @click="playback.openVisualizer()">
        <el-icon :size="15"><MagicStick /></el-icon> 沉浸播放
      </button>
      <button
        v-if="isAdmin"
        class="ml-btn ghost match-btn"
        :disabled="linkBusy"
        @click="lyrics.autoMatchAll()"
      >
        <el-icon :size="15"><Connection /></el-icon>
        {{ linkBusy ? '匹配中…' : '按歌名匹配歌词' }}
      </button>
    </div>

    <div class="side-spacer" />

    <div class="stats-card">
      <div class="stats-head">
        <span class="ml-eyebrow">曲库概览</span>
        <span v-if="loading" class="stats-pulse" />
      </div>
      <div v-for="row in statsRows" :key="row.label" class="stats-row">
        <span>{{ row.label }}</span><strong>{{ row.value }}</strong>
      </div>
    </div>

    <el-popover placement="right-start" :width="230" trigger="click" popper-class="ml-pop">
      <template #reference>
        <button class="help-btn" aria-label="查看键盘快捷键">
          <el-icon :size="14"><QuestionFilled /></el-icon> 快捷键
          <span class="ml-kbd">?</span>
        </button>
      </template>
      <div class="shortcut-list">
        <strong>键盘快捷键</strong>
        <div v-for="[k, desc] in shortcuts" :key="k" class="shortcut-row">
          <span>{{ desc }}</span><span class="ml-kbd">{{ k }}</span>
        </div>
      </div>
    </el-popover>
  </aside>
</template>

<style scoped>
.sidebar {
  position: sticky;
  /* 顶栏是 sticky 且 48px 高，sticky 侧栏必须避开它，否则滚动时上沿被压住 */
  top: var(--ml-sticky-top, 58px);
  align-self: start;
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 20px 16px 16px;
  max-height: calc(100dvh - var(--app-bar-h, 48px) - 32px - var(--player-bar-h, 0px));
  overflow-y: auto;
  scrollbar-width: thin;
}

.brand { display: flex; align-items: center; gap: 11px; padding: 0 4px; }
.brand-mark {
  width: 38px; height: 38px; border-radius: 12px;
  display: flex; align-items: center; justify-content: center;
  background: var(--ml-grad); color: var(--ml-on-accent);
  box-shadow: 0 8px 20px var(--ml-accent-soft);
}
.brand-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.brand-text strong { font-size: 16px; font-weight: 750; letter-spacing: -0.01em; }

.nav { display: flex; flex-direction: column; gap: 3px; }
.nav-label {
  padding: 8px 12px 4px;
  font-size: 10px; font-weight: 700; letter-spacing: 0.16em;
  text-transform: uppercase; color: var(--ml-text-3);
}
.nav-item {
  display: flex; align-items: center; gap: 11px;
  width: 100%; height: 38px; padding: 0 12px;
  border: none; border-radius: 11px;
  background: transparent; color: var(--ml-text-2);
  font-size: 13px; font-weight: 600; font-family: inherit;
  cursor: pointer; text-align: left;
  transition: background-color 0.16s ease, color 0.16s ease;
}
.nav-item:hover { background: var(--ml-surface-2); color: var(--ml-text); }
.nav-item.active { background: var(--ml-accent-soft); color: var(--ml-accent); }
.nav-count {
  margin-left: auto; font-size: 11px; font-weight: 700;
  color: var(--ml-text-3); font-variant-numeric: tabular-nums;
}
.nav-item.active .nav-count { color: var(--ml-accent); }

.side-actions { display: flex; flex-direction: column; gap: 8px; }
.side-actions .ml-btn { width: 100%; height: 38px; }
.upload-btn { box-shadow: 0 8px 20px var(--ml-accent-soft); }
.viz-btn { border-color: var(--ml-line-strong); }
.match-btn { font-size: 12px; }

.side-spacer { flex: 1; min-height: 8px; }

.stats-card {
  padding: 14px; border-radius: 14px;
  background: var(--ml-surface-2);
  border: 1px solid var(--ml-line);
}
.stats-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.stats-pulse {
  width: 7px; height: 7px; border-radius: 50%;
  background: var(--ml-accent); animation: ml-fade-up 1s ease-in-out infinite alternate;
}
.stats-row {
  display: flex; align-items: baseline; justify-content: space-between;
  padding: 3px 0; font-size: 12px; color: var(--ml-text-2);
}
.stats-row strong {
  font-size: 13px; color: var(--ml-text); font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.help-btn {
  display: flex; align-items: center; gap: 7px;
  padding: 6px 8px; border: none; border-radius: 9px;
  background: transparent; color: var(--ml-text-3);
  font-size: 11px; font-family: inherit; cursor: pointer;
}
.help-btn:hover { color: var(--ml-text); background: var(--ml-surface-2); }
.help-btn .ml-kbd { margin-left: auto; }

.shortcut-list strong { display: block; margin-bottom: 10px; font-size: 13px; color: var(--ml-text); }
.shortcut-row {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  padding: 5px 0; font-size: 12px; color: var(--ml-text-2);
}

@media (max-width: 1080px) {
  .sidebar { gap: 12px; padding: 14px 12px; }
  .stats-card, .help-btn, .match-btn, .side-spacer { display: none; }
  .brand-text .ml-eyebrow { display: none; }
  .viz-btn { display: none; }
}
@media (max-width: 768px) {
  .sidebar {
    position: static;
    width: 100%; max-width: 100%; min-width: 0;
    max-height: none;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding: 8px;
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: none;
  }
  .sidebar::-webkit-scrollbar { display: none; }
  .brand { display: none; }
  .nav { flex-direction: row; gap: 6px; }
  .nav-label { display: none; }
  .nav-item { width: auto; height: 34px; padding: 0 12px; gap: 6px; white-space: nowrap; flex-shrink: 0; }
  .nav-item .el-icon { display: none; }
  .nav-count { margin-left: 2px; }
  .side-actions { flex-direction: row; margin-left: auto; }
  .side-actions .ml-btn { width: auto; height: 34px; padding: 0 12px; }
  .side-actions .viz-btn { display: inline-flex; }
  .upload-btn { box-shadow: none; }
}
</style>
