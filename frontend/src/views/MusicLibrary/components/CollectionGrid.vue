<script setup>
import { VideoPlay, ArrowRight } from '@element-plus/icons-vue'
import CoverArt from './CoverArt.vue'
import { fmtTime } from '../utils/format.js'

defineProps({
  groups: { type: Array, default: () => [] },
  dimension: { type: String, default: 'artist' },
})
const emit = defineEmits(['open', 'play'])

const placeholder = (name) => ({ id: `group:${name}`, title: name })
</script>

<template>
  <div class="collection-grid">
    <div
      v-for="(g, i) in groups" :key="g.key"
      class="collection-card ml-card"
      role="button" tabindex="0"
      :aria-label="`查看${dimension === 'album' ? '专辑' : '歌手'} ${g.name}`"
      :style="{ animationDelay: `${Math.min(i, 12) * 35}ms` }"
      @click="emit('open', g)"
      @keydown.enter="emit('open', g)"
    >
      <div class="cc-art">
        <CoverArt :track="g.coverTrack || placeholder(g.name)" :seed="g.name" size="xl" />
        <button class="cc-play" type="button" :aria-label="`播放 ${g.name} 的全部曲目`" @click.stop="emit('play', g)">
          <el-icon :size="22"><VideoPlay /></el-icon>
        </button>
      </div>
      <div class="cc-info">
        <span class="cc-name">{{ g.name }}</span>
        <span class="cc-sub">{{ g.count }} 首 · {{ fmtTime(g.duration) }}</span>
        <span class="cc-meta">{{ g.subtitle }}</span>
      </div>
      <el-icon class="cc-arrow" :size="15"><ArrowRight /></el-icon>
    </div>
  </div>
</template>

<style scoped>
.collection-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
  gap: 16px;
}
.collection-card {
  position: relative;
  display: flex; flex-direction: column;
  padding: 12px; cursor: pointer;
  border: 1px solid var(--ml-line);
  transition: transform 0.22s ease, border-color 0.22s ease, box-shadow 0.22s ease;
  animation: ml-fade-up 0.45s ease both;
}
.collection-card:hover {
  transform: translateY(-5px);
  border-color: color-mix(in srgb, var(--ml-accent) 55%, transparent);
  box-shadow: var(--ml-shadow-lg);
}
.cc-art { position: relative; border-radius: 14px; overflow: hidden; }
.cc-play {
  position: absolute; right: 12px; bottom: 12px;
  width: 44px; height: 44px; border: none; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  background: var(--ml-grad); color: #fff; cursor: pointer;
  box-shadow: 0 8px 22px rgba(0, 0, 0, 0.3);
  opacity: 0; transform: translateY(8px);
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.collection-card:hover .cc-play,
.collection-card:focus-within .cc-play { opacity: 1; transform: none; }
.cc-info { display: flex; flex-direction: column; gap: 3px; margin-top: 12px; padding-right: 14px; }
.cc-name { font-size: 14px; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.cc-sub { font-size: 12px; color: var(--ml-text-2); }
.cc-meta { font-size: 11px; color: var(--ml-text-3); }
.cc-arrow { position: absolute; right: 12px; bottom: 14px; color: var(--ml-text-3); }

@media (max-width: 768px) {
  .collection-grid { grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 10px; }
  .cc-play { opacity: 1; transform: none; width: 36px; height: 36px; right: 8px; bottom: 8px; }
  .cc-meta { display: none; }
}
</style>
