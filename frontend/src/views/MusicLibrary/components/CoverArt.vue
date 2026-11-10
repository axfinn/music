<script setup>
/**
 * 统一封面。没有封面时用曲目 id 生成确定性渐变艺术封面 —— 每首歌都拥有
 * 稳定且不同的视觉身份，而不是清一色灰方块。
 */
import { computed, ref, watch } from 'vue'
import { VideoPlay, VideoPause } from '@element-plus/icons-vue'
import { coverUrl } from '../api.js'
import { initialOf, toneFor } from '../utils/format.js'

const props = defineProps({
  track: { type: Object, default: null },
  seed: { type: String, default: '' },
  size: { type: String, default: 'md' }, // xs | sm | md | lg | xl
  overlay: { type: Boolean, default: false },
  playing: { type: Boolean, default: false },
  vinyl: { type: Boolean, default: false },
})
const emit = defineEmits(['play'])

const broken = ref(false)
watch(() => props.track?.id || props.seed, () => { broken.value = false })
const src = computed(() => (broken.value ? '' : coverUrl(props.track)))
const tone = computed(() => toneFor(props.seed || props.track?.id || props.track?.title || 'music'))
const label = computed(() => initialOf(props.track?.title))
const style = computed(() => ({ background: tone.value.gradient }))

function onImgError() { broken.value = true }

// 只有作为「播放按钮」时才拦截冒泡；多选模式下行点击必须能穿到父级去勾选
function onRootActivate(e) {
  if (!props.overlay) return
  e.stopPropagation()
  emit('play')
}
</script>

<template>
  <div
    class="cover-art"
    :class="[`size-${size}`, { vinyl, playing }]"
    :style="style"
    :role="overlay ? 'button' : 'img'"
    :aria-label="overlay ? `播放 ${track?.title || ''}` : `${track?.title || ''} 封面`"
    :tabindex="overlay ? 0 : -1"
    @click="onRootActivate"
    @keydown.enter="onRootActivate"
  >
    <img v-if="src" :src="src" :alt="track?.title || '封面'" loading="lazy" @error="onImgError" />
    <template v-else>
      <span class="art-grain" />
      <span class="art-disc" />
      <span class="art-letter">{{ label }}</span>
      <span class="art-note">♪</span>
    </template>
    <button v-if="overlay" class="art-play" type="button" :aria-label="playing ? '暂停' : '播放'" @click.stop="emit('play')">
      <el-icon :size="size === 'xl' || size === 'lg' ? 26 : 18">
        <component :is="playing ? VideoPause : VideoPlay" />
      </el-icon>
    </button>
  </div>
</template>

<style scoped>
.cover-art {
  position: relative;
  flex-shrink: 0;
  overflow: hidden;
  border-radius: 12px;
  background: var(--ml-surface-2);
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.16);
  aspect-ratio: 1 / 1;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
}
.size-xs { width: 36px; border-radius: 8px; }
.size-sm { width: 48px; }
.size-md { width: 56px; }
.size-lg { width: 96px; border-radius: 16px; }
.size-xl { width: 100%; border-radius: 18px; box-shadow: 0 22px 50px rgba(0, 0, 0, 0.34); }

.cover-art img {
  width: 100%; height: 100%;
  object-fit: cover; display: block;
}

.art-grain {
  position: absolute; inset: 0;
  background:
    radial-gradient(circle at 22% 18%, rgba(255, 255, 255, 0.42), transparent 52%),
    radial-gradient(circle at 82% 88%, rgba(0, 0, 0, 0.28), transparent 58%);
}
.art-disc {
  position: absolute;
  width: 74%; height: 74%;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.24);
  box-shadow: inset 0 0 0 8px rgba(255, 255, 255, 0.07), inset 0 0 0 9px rgba(255, 255, 255, 0.16);
  transform: translate(32%, -30%);
}
.art-letter {
  position: relative;
  font-weight: 800;
  line-height: 1;
  color: rgba(255, 255, 255, 0.94);
  text-shadow: 0 3px 18px rgba(0, 0, 0, 0.28);
  font-size: 1.5em;
}
.size-xs .art-letter, .size-sm .art-letter { font-size: 17px; }
.size-md .art-letter { font-size: 22px; }
.size-lg .art-letter { font-size: 38px; }
.size-xl .art-letter { font-size: 92px; }
.art-note {
  position: absolute; right: 9%; bottom: 7%;
  font-size: 13px; color: rgba(255, 255, 255, 0.6);
}
.size-xl .art-note { font-size: 22px; }

.art-play {
  position: absolute; inset: 0;
  display: flex; align-items: center; justify-content: center;
  border: none; cursor: pointer;
  background: rgba(8, 10, 16, 0.48);
  color: #fff;
  opacity: 0;
  transition: opacity 0.2s ease;
}
.cover-art:hover .art-play,
.cover-art:focus-visible .art-play,
.cover-art.playing .art-play { opacity: 1; }
.vinyl img, .vinyl .art-disc { border-radius: 50%; }
.vinyl.playing img { animation: cover-spin 18s linear infinite; }
@keyframes cover-spin { to { transform: rotate(360deg); } }

@media (prefers-reduced-motion: reduce) {
  .vinyl.playing img { animation: none; }
}
</style>
