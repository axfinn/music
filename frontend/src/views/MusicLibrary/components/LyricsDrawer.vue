<script setup>
import { computed, onBeforeUnmount, watch } from 'vue'
import { Close, Edit, Connection } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import LyricsView from './LyricsView.vue'
import QueuePanel from './QueuePanel.vue'

const { lyrics, ui, playback, isAdmin } = useMusicContext()
const { drawerOpen, drawerTab, drawerTrack, drawerSource, drawerLines } = lyrics
const { state } = playback

const subtitle = computed(() => {
  const t = drawerTrack.value
  if (!t) return ''
  return `${t.artist || '未知歌手'}${t.album ? ` · ${t.album}` : ''}`
})

// 抽屉挂着时让宠物浮窗让位（它层级在 9999+，会盖住歌词）
watch(drawerOpen, (open) => {
  document.documentElement.classList.toggle('ml-drawer-open', !!open)
}, { immediate: true })
onBeforeUnmount(() => document.documentElement.classList.remove('ml-drawer-open'))
</script>

<template>
  <el-drawer v-model="drawerOpen" size="460px" class="ml-drawer" :with-header="false" direction="rtl">
    <div class="pane">
      <header class="pane-head">
        <CoverArt v-if="drawerTrack" :track="drawerTrack" size="lg" />
        <div class="pane-meta">
          <span class="ml-eyebrow">{{ drawerTab === 'queue' ? 'PLAY QUEUE' : 'LYRICS' }}</span>
          <h2>{{ drawerTab === 'queue' ? '播放队列' : (drawerTrack?.title || '歌词') }}</h2>
          <p v-if="drawerTab === 'queue'">共 {{ state.playlist.length }} 首 · 点任意一首直接跳播</p>
          <p v-else>{{ subtitle }}</p>
          <span v-if="drawerTab === 'lyrics' && drawerSource === 'library'" class="ml-tag accent">来自歌词库</span>
        </div>
        <button class="ml-icon-btn" aria-label="关闭" @click="drawerOpen = false"><el-icon :size="16"><Close /></el-icon></button>
      </header>

      <div class="pane-tabs" role="tablist">
        <button
          role="tab" :aria-selected="drawerTab === 'lyrics'"
          :class="{ active: drawerTab === 'lyrics' }" @click="drawerTab = 'lyrics'"
        >歌词</button>
        <button
          role="tab" :aria-selected="drawerTab === 'queue'"
          :class="{ active: drawerTab === 'queue' }" @click="drawerTab = 'queue'"
        >队列 <span class="tab-count">{{ state.playlist.length }}</span></button>
      </div>

      <LyricsView
        v-if="drawerTab === 'lyrics'"
        :lines="drawerLines"
        empty-text="这首歌还没有歌词，点下方「编辑歌词」补上吧"
      />
      <QueuePanel v-else>
        <template #close><span /></template>
      </QueuePanel>

      <footer v-if="drawerTab === 'lyrics'" class="pane-foot">
        <template v-if="isAdmin">
          <button class="ml-btn" @click="ui.openEdit(drawerTrack); drawerOpen = false">
            <el-icon :size="14"><Edit /></el-icon> 编辑歌词
          </button>
          <button class="ml-btn" @click="lyrics.openLink(drawerTrack); drawerOpen = false">
            <el-icon :size="14"><Connection /></el-icon> 关联歌词库
          </button>
        </template>
        <span class="foot-tip">歌词按原文展示，不跟随播放进度滚动</span>
      </footer>
    </div>
  </el-drawer>
</template>

<style scoped>
.pane { display: flex; flex-direction: column; height: 100%; gap: 10px; }
.pane-head { display: flex; align-items: center; gap: 14px; padding: 4px 0 8px; border-bottom: 1px solid var(--ml-line); }
.pane-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.pane-meta h2 { margin: 0; font-size: 18px; font-weight: 750; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pane-meta p { margin: 0; font-size: 12px; color: var(--ml-text-3); }
.pane-tabs { display: flex; gap: 4px; }
.pane-tabs button {
  border: none; background: transparent; color: var(--ml-text-3);
  font-size: 12px; font-weight: 650; font-family: inherit;
  padding: 6px 12px; border-radius: 9px; cursor: pointer;
}
.pane-tabs button:hover { color: var(--ml-text); }
.pane-tabs button.active { color: var(--ml-accent); background: var(--ml-accent-soft); }
.tab-count { font-size: 10px; opacity: 0.7; }
.pane-foot { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding-top: 10px; border-top: 1px solid var(--ml-line); }
.foot-tip { flex-basis: 100%; font-size: 11px; color: var(--ml-text-3); }
:deep(.lyrics-view) { flex: 1; min-height: 0; }
</style>
