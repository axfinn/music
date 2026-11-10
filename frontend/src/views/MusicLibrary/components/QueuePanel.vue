<script setup>
import { computed } from 'vue'
import { Close } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'

const { playback } = useMusicContext()
const { state } = playback

const shuffleTag = computed(() => state.shuffleMode && state.shuffleOrder)
</script>

<template>
  <div class="queue ml-scroll">
    <div class="queue-head">
      <span>
        播放队列 · {{ state.playlist.length }} 首
        <span v-if="shuffleTag" class="shuffle-badge">随机列表</span>
      </span>
      <slot name="close"><button class="ml-icon-btn" aria-label="关闭队列"><el-icon :size="14"><Close /></el-icon></button></slot>
    </div>
    <div v-if="!state.playlist.length" class="queue-empty">队列是空的，点一首歌开始</div>
    <button
      v-for="(q, i) in state.playlist" :key="`${q.id}-${i}`"
      class="queue-item" :class="{ current: i === state.currentIndex }"
      @click="playback.player.play(i)"
    >
      <span class="q-idx">
        <span v-if="i === state.currentIndex && state.isPlaying" class="ml-eq"><i /><i /><i /></span>
        <template v-else>{{ playback.queuePosLabel(i) }}</template>
      </span>
      <span class="q-meta"><strong>{{ q.title }}</strong><small>{{ q.artist || '未知歌手' }}</small></span>
      <span v-if="i === state.currentIndex" class="q-dot" />
    </button>
  </div>
</template>

<style scoped>
.queue { flex: 1; min-height: 0; overflow-y: auto; padding: 0 4px 8px; }
.queue-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 2px 4px 10px; font-size: 12px; font-weight: 700;
  color: var(--ml-text); position: sticky; top: 0;
  background: var(--ml-surface); z-index: 1;
}
.shuffle-badge {
  margin-left: 6px; padding: 1px 7px; border-radius: 8px;
  font-size: 10px; color: var(--ml-accent); background: var(--ml-accent-soft);
}
.queue-empty { padding: 30px 0; text-align: center; font-size: 12px; color: var(--ml-text-3); }
.queue-item {
  display: flex; align-items: center; gap: 10px;
  width: 100%; padding: 8px 9px; border: none; border-radius: 10px;
  background: transparent; color: var(--ml-text-2); cursor: pointer; text-align: left;
  transition: background-color 0.16s ease;
}
.queue-item:hover { background: var(--ml-surface-2); }
.queue-item.current { background: var(--ml-accent-soft); color: var(--ml-accent); }
.q-idx { width: 24px; flex-shrink: 0; text-align: center; font-size: 11px; font-variant-numeric: tabular-nums; }
.q-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.q-meta strong { font-size: 12px; font-weight: 650; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.q-meta small { font-size: 10px; color: var(--ml-text-3); }
.q-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--ml-accent); flex-shrink: 0; }
</style>
