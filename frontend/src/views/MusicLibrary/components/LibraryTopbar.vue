<script setup>
import { computed } from 'vue'
import { Search, Sort, Grid, List, Checked, Star, UploadFilled } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'

const { store, playback, upload, ui, isAdmin } = useMusicContext()
const { section, keyword, sortBy, viewMode, displayCount, selected, effectiveFav } = store
const { selectionMode, toggleSelectionMode } = ui

const titles = {
  discover: '曲库首页', all: '全部曲目', favorites: '我喜欢的',
  recent: '最近添加', albums: '专辑', artists: '歌手',
}
const title = computed(() => titles[section.value] || '音乐库')
const isBrowse = computed(() => ['all', 'recent', 'favorites'].includes(section.value))
const showGridToggle = computed(() => isBrowse.value)
</script>

<template>
  <header class="topbar">
    <div class="topbar-head">
      <div>
        <span class="ml-eyebrow">MY MUSIC LIBRARY</span>
        <h1>{{ title }}<span class="head-count">{{ displayCount }}</span></h1>
      </div>
      <div class="head-actions">
        <button v-if="isAdmin" class="ml-btn primary mobile-upload" @click="upload.open()">
          <el-icon :size="14"><UploadFilled /></el-icon> 上传
        </button>
      </div>
    </div>

    <div class="controls">
      <el-input
        id="ml-search"
        :model-value="keyword"
        placeholder="搜索标题、歌手、专辑、备注…"
        :prefix-icon="Search"
        clearable
        class="search"
        aria-label="搜索曲库"
        @input="store.setKeyword"
        @clear="store.setKeyword('')"
      >
        <template #suffix><span class="ml-kbd search-kbd">/</span></template>
      </el-input>

      <el-select
        :model-value="sortBy"
        class="sort"
        aria-label="排序方式"
        @change="store.setSortBy"
      >
        <template #prefix><el-icon><Sort /></el-icon></template>
        <el-option label="最新添加" value="created" />
        <el-option label="标题 A-Z" value="title" />
        <el-option label="最多播放" value="plays" />
      </el-select>

      <div v-if="showGridToggle" class="view-seg" role="group" aria-label="视图切换">
        <button :class="{ active: viewMode === 'list' }" title="列表视图" aria-label="列表视图" @click="store.setViewMode('list')">
          <el-icon :size="15"><List /></el-icon>
        </button>
        <button :class="{ active: viewMode === 'grid' }" title="封面墙视图" aria-label="封面墙视图" @click="store.setViewMode('grid')">
          <el-icon :size="15"><Grid /></el-icon>
        </button>
      </div>

      <button
        v-if="isBrowse"
        class="ml-btn" :class="{ active: selectionMode }"
        :title="selectionMode ? '退出多选' : '多选批量操作'"
        @click="toggleSelectionMode()"
      >
        <el-icon :size="14"><Checked /></el-icon>
        <span class="btn-text">多选</span>
        <span v-if="selected.size" class="sel-count">{{ selected.size }}</span>
      </button>

      <button
        v-if="section !== 'favorites'"
        class="ml-btn" :class="{ active: effectiveFav }"
        @click="store.toggleFavOnly()"
      >
        <el-icon :size="14"><Star /></el-icon>
        <span class="btn-text">只看收藏</span>
      </button>

      <button class="ml-btn shuffle-btn" title="把当前视图洗成随机列表播放" @click="playback.playAllVisible({ shuffle: true })">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <path d="M16 3h5v5M4 20L21 3M21 16v5h-5M15 15l6 6M4 4l5 5" />
        </svg>
        <span class="btn-text">随机播放</span>
      </button>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  position: sticky;
  top: var(--ml-sticky-top, 58px);
  z-index: 12;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px 18px 14px;
  margin-bottom: 20px;
  border-radius: var(--ml-radius);
  border: 1px solid var(--ml-line);
  background: color-mix(in srgb, var(--ml-surface) 88%, transparent);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow: var(--ml-shadow);
}
.topbar-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; }
.topbar-head h1 {
  display: flex; align-items: baseline; gap: 10px;
  margin: 3px 0 0;
  font-size: clamp(20px, 2.4vw, 28px);
  font-weight: 780; letter-spacing: -0.03em;
}
.head-count {
  font-size: 12px; font-weight: 600; letter-spacing: 0;
  color: var(--ml-text-3); font-variant-numeric: tabular-nums;
}
.mobile-upload { display: none; }

.controls { display: flex; align-items: center; gap: 9px; flex-wrap: wrap; }
.search { width: min(320px, 100%); }
.search-kbd { margin-right: 2px; }
.sort { width: 132px; }
.view-seg {
  display: inline-flex; gap: 2px; padding: 3px;
  border: 1px solid var(--ml-line); border-radius: 11px;
  background: var(--ml-surface-2);
}
.view-seg button {
  width: 32px; height: 28px;
  display: inline-flex; align-items: center; justify-content: center;
  border: none; border-radius: 8px;
  background: transparent; color: var(--ml-text-3); cursor: pointer;
  transition: all 0.16s ease;
}
.view-seg button:hover { color: var(--ml-text); }
.view-seg button.active { background: var(--ml-surface); color: var(--ml-accent); box-shadow: var(--ml-shadow); }
.sel-count {
  min-width: 17px; height: 17px; padding: 0 4px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: 9px; background: var(--ml-accent); color: #fff;
  font-size: 10px; font-weight: 700;
}

@media (max-width: 768px) {
  .topbar { position: static; padding: 12px; gap: 10px; margin-bottom: 14px; }
  .topbar-head h1 { font-size: 20px; }
  .mobile-upload { display: inline-flex; height: 32px; padding: 0 12px; }
  .search { width: 100%; }
  .sort { width: 108px; }
  .controls { gap: 7px; }
  .btn-text { display: none; }
  .controls .ml-btn { width: 36px; height: 36px; padding: 0; }
  .controls .ml-btn .sel-count { position: absolute; transform: translate(12px, -12px); }
  .shuffle-btn { display: none; }
}
</style>
