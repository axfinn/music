<script setup>
/**
 * 歌词视图：只做「可查看」。
 *
 * 之前按 LRC 时间戳逐行高亮 + 自动滚动，但曲库里的歌词大多没有可靠时间轴
 * （或与音频对不齐），同步时反而比原文更碍事。所以这里彻底去掉时间绑定的
 * 高亮、滚动和点击跳转，只把歌词完整、安静地排出来给人看。
 */
defineProps({
  lines: { type: Array, default: () => [] },
  emptyText: { type: String, default: '这首歌还没有歌词' },
  compact: { type: Boolean, default: false },
})
</script>

<template>
  <div class="lyrics-view ml-scroll" :class="{ compact }">
    <template v-if="lines.length">
      <p v-for="(line, i) in lines" :key="i" class="lyric-line">{{ line.text }}</p>
    </template>
    <div v-else class="lyrics-empty">{{ emptyText }}</div>
  </div>
</template>

<style scoped>
.lyrics-view {
  flex: 1; min-height: 0;
  overflow-y: auto;
  padding: 12px 10px 36px;
  mask-image: linear-gradient(180deg, transparent 0, #000 14px, #000 calc(100% - 28px), transparent 100%);
  -webkit-mask-image: linear-gradient(180deg, transparent 0, #000 14px, #000 calc(100% - 28px), transparent 100%);
  scrollbar-width: none;
}
.lyrics-view::-webkit-scrollbar { display: none; }
.lyric-line {
  margin: 0;
  padding: 5px 0;
  font-size: 15px;
  line-height: 1.85;
  color: var(--ml-text-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.compact .lyric-line { font-size: 13px; line-height: 1.75; padding: 3px 0; }
.lyrics-empty { padding: 36px 8px; text-align: center; font-size: 12px; color: var(--ml-text-3); }
</style>
