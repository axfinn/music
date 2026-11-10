<script setup>
import { UploadFilled } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'

const { upload, isAdmin } = useMusicContext()
const { dragActive } = upload
</script>

<template>
  <transition name="drop">
    <div v-if="dragActive" class="drop-overlay" aria-live="polite">
      <div class="drop-card ml-card">
        <span class="drop-icon"><el-icon :size="34"><UploadFilled /></el-icon></span>
        <strong>松开即可加入曲库</strong>
        <span>支持批量音频文件，单曲最大 200MB</span>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.drop-overlay {
  position: fixed; inset: 0; z-index: 2500;
  display: flex; align-items: center; justify-content: center;
  background: color-mix(in srgb, var(--ml-bg) 72%, transparent);
  backdrop-filter: blur(8px);
  pointer-events: none; /* 不抢拖放事件，否则 dragenter/leave 会抖动 */
}
.drop-card {
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  padding: 42px 60px;
  border: 2px dashed var(--ml-accent);
  border-radius: 24px;
  background: var(--ml-surface);
  box-shadow: var(--ml-shadow-lg);
  animation: drop-in 0.25s ease both;
}
.drop-icon {
  width: 66px; height: 66px; border-radius: 20px;
  display: flex; align-items: center; justify-content: center;
  background: var(--ml-accent-soft); color: var(--ml-accent);
}
.drop-card strong { font-size: 18px; }
.drop-card span:last-child { font-size: 12px; color: var(--ml-text-3); }
@keyframes drop-in { from { opacity: 0; transform: scale(0.94); } to { opacity: 1; transform: none; } }
.drop-enter-active, .drop-leave-active { transition: opacity 0.2s ease; }
.drop-enter-from, .drop-leave-to { opacity: 0; }
</style>
