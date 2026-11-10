<script setup>
import { UploadFilled, Headset, Delete, Loading, CircleCheck, Warning, RefreshRight } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import { fmtTime } from '../utils/format.js'

const { upload } = useMusicContext()
const { dialog, pending, artist, album, uploading, readyCount, hasFailures, accept } = upload
</script>

<template>
  <el-dialog
    v-model="dialog" title="上传音乐" width="580px"
    class="ml-dialog" align-center :close-on-click-modal="false"
  >
    <el-upload
      class="uploader" drag multiple
      :auto-upload="false" :show-file-list="false"
      :accept="accept"
      :on-change="upload.onFileChange"
    >
      <el-icon :size="38" class="upload-icon"><UploadFilled /></el-icon>
      <div class="el-upload__text">把音频拖到这里，或 <em>点击选择</em></div>
      <template #tip>
        <div class="el-upload__tip">支持 mp3 / wav / flac / m4a / ogg / opus 等，单曲最大 200MB，也可以直接拖到页面任意位置</div>
      </template>
    </el-upload>

    <div v-if="pending.length" class="pending">
      <div class="pending-head">
        <span>待入库 <strong>{{ readyCount }}</strong> 个文件</span>
        <button class="ml-btn ghost" :disabled="uploading" @click="pending = []">清空</button>
      </div>
      <div class="pending-list ml-scroll">
        <div v-for="p in pending" :key="p.key" class="pending-item" :class="p.status">
          <span class="p-icon">
            <el-icon v-if="p.status === 'uploading'" class="is-loading" :size="16"><Loading /></el-icon>
            <el-icon v-else-if="p.status === 'done'" :size="16"><CircleCheck /></el-icon>
            <el-icon v-else-if="p.status === 'error'" :size="16"><Warning /></el-icon>
            <el-icon v-else :size="16"><Headset /></el-icon>
          </span>
          <span class="p-meta">
            <input v-model="p.title" class="p-title" :disabled="p.status === 'done' || uploading" aria-label="曲目标题" />
            <span class="p-sub">
              {{ p.humanSize }}<template v-if="p.duration"> · {{ fmtTime(p.duration) }}</template>
              <template v-if="p.error"> · <em>{{ p.error }}</em></template>
            </span>
          </span>
          <button v-if="p.status !== 'done'" class="ml-icon-btn" aria-label="移除文件" :disabled="uploading" @click="upload.removeEntry(p.key)">
            <el-icon :size="14"><Delete /></el-icon>
          </button>
        </div>
      </div>
    </div>

    <div class="fields">
      <el-input v-model="artist" placeholder="歌手 / 来源（可选，应用到全部）" clearable />
      <el-input v-model="album" placeholder="专辑 / 合集（可选，应用到全部）" clearable />
    </div>

    <template #footer>
      <button v-if="hasFailures" class="ml-btn" :disabled="uploading" @click="upload.retryFailed()">
        <el-icon :size="14"><RefreshRight /></el-icon> 重试失败项
      </button>
      <button class="ml-btn" @click="dialog = false">取消</button>
      <button class="ml-btn primary" :disabled="!readyCount || uploading" @click="upload.uploadAll()">
        {{ uploading ? '上传中…' : `开始上传${readyCount ? ` (${readyCount})` : ''}` }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.upload-icon { color: var(--ml-accent); }
.uploader :deep(.el-upload-dragger) { border-radius: 14px; padding: 22px 10px; }

.pending { margin-top: 14px; border: 1px solid var(--ml-line); border-radius: 12px; overflow: hidden; }
.pending-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 7px 12px; background: var(--ml-surface-2);
  font-size: 12px; color: var(--ml-text-2);
}
.pending-head strong { color: var(--ml-accent); }
.pending-list { max-height: 220px; overflow-y: auto; }
.pending-item {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 12px; border-top: 1px solid var(--ml-line);
}
.p-icon { color: var(--ml-text-3); display: flex; flex-shrink: 0; }
.pending-item.done .p-icon { color: #4caf7d; }
.pending-item.error .p-icon { color: #e5484d; }
.pending-item.uploading .p-icon { color: var(--ml-accent); }
.p-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.p-title {
  width: 100%; border: none; background: transparent;
  color: var(--ml-text); font-size: 13px; font-family: inherit;
  padding: 0; outline: none;
}
.p-title:focus { box-shadow: inset 0 -1px 0 var(--ml-accent); }
.p-title:disabled { opacity: 0.6; }
.p-sub { font-size: 11px; color: var(--ml-text-3); }
.p-sub em { font-style: normal; color: #e5484d; }

.fields { display: flex; gap: 10px; margin-top: 12px; }
:deep(.el-dialog__footer) { display: flex; gap: 8px; justify-content: flex-end; }

@media (max-width: 768px) {
  .fields { flex-direction: column; }
}
</style>
