<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import { coverUrl } from '../api.js'

const { store, ui, lyrics } = useMusicContext()
const { editDialog, editForm } = ui

const saving = ref(false)
const coverFile = ref(null)
const coverPreview = ref('')
let previewUrl = ''

const existingCover = computed(() => editForm.value?.cover_filename ? coverUrl(editForm.value) : '')
const shownCover = computed(() => coverPreview.value || existingCover.value)

function releasePreview() {
  if (previewUrl) { URL.revokeObjectURL(previewUrl); previewUrl = '' }
  coverPreview.value = ''
}

function onCoverPick(uploadFile) {
  const raw = uploadFile?.raw
  if (!raw) return
  coverFile.value = raw
  releasePreview()
  previewUrl = URL.createObjectURL(raw)
  coverPreview.value = previewUrl
}

watch(editDialog, (open) => {
  if (!open) { coverFile.value = null; releasePreview() }
})
onBeforeUnmount(releasePreview)

async function save() {
  const f = editForm.value
  if (!f?.title?.trim()) return ElMessage.warning('标题不能为空')
  saving.value = true
  try {
    // 详情没取到时表单上就没有 lyrics/note；此时不要提交空串把库里的正文洗掉
    await store.saveTrack(f.id, {
      title: f.title.trim(),
      artist: f.artist || '',
      album: f.album || '',
      ...(f.note !== undefined ? { note: f.note || '' } : {}),
      ...(f.lyrics !== undefined ? { lyrics: f.lyrics || '' } : {}),
      duration_sec: Number(f.duration_sec) > 0 ? Number(f.duration_sec) : undefined,
    })
    if (coverFile.value) {
      await store.saveCover(f.id, coverFile.value)
      coverFile.value = null
      releasePreview()
    }
    editDialog.value = false
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="editDialog" title="编辑曲目" width="560px"
    class="ml-dialog" align-center :close-on-click-modal="false"
  >
    <div v-if="editForm" class="form">
      <div class="cover-row">
        <div class="cover-box">
          <img v-if="shownCover" :src="shownCover" alt="封面预览" />
          <CoverArt v-else :track="editForm" size="xl" />
        </div>
        <div class="cover-actions">
          <el-upload :auto-upload="false" :show-file-list="false" accept="image/*" :on-change="onCoverPick">
            <button class="ml-btn">{{ shownCover ? '更换封面' : '上传封面' }}</button>
          </el-upload>
          <span class="hint">方形图片效果最佳，最大 15MB</span>
          <span class="hint">支持 jpg / png / webp / gif / avif</span>
        </div>
      </div>

      <div class="grid-2">
        <el-input v-model="editForm.title" placeholder="标题（必填）" />
        <el-input v-model="editForm.artist" placeholder="歌手 / 来源" />
        <el-input v-model="editForm.album" placeholder="专辑 / 合集" />
        <el-input v-model="editForm.note" placeholder="备注（参与搜索）" />
      </div>

      <label class="field-label">歌词</label>
      <el-input
        v-model="editForm.lyrics" type="textarea" :rows="7"
        placeholder="支持纯文本或 LRC 时间戳格式，LRC 会逐句同步高亮"
      />
      <div class="form-foot">
        <span v-if="editForm.lyric_song_id" class="ml-tag accent">已关联歌词库</span>
        <button class="ml-btn ghost" @click="lyrics.openLink(editForm); editDialog = false">
          关联 / 更换歌词库
        </button>
      </div>
    </div>

    <template #footer>
      <button class="ml-btn" @click="editDialog = false">取消</button>
      <button class="ml-btn primary" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form { display: flex; flex-direction: column; gap: 12px; }
.cover-row { display: flex; align-items: center; gap: 16px; }
.cover-box {
  width: 92px; height: 92px; flex-shrink: 0;
  border-radius: 14px; overflow: hidden; background: var(--ml-surface-2);
}
.cover-box img { width: 100%; height: 100%; object-fit: cover; display: block; }
.cover-actions { display: flex; flex-direction: column; align-items: flex-start; gap: 5px; }
.hint { font-size: 11px; color: var(--ml-text-3); }
.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.field-label { font-size: 12px; font-weight: 650; color: var(--ml-text-2); }
.form-foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.form-foot .ml-btn { margin-left: auto; font-size: 12px; }
:deep(.el-dialog__footer) { display: flex; gap: 8px; justify-content: flex-end; }

@media (max-width: 768px) {
  .grid-2 { grid-template-columns: 1fr; }
  .cover-box { width: 72px; height: 72px; }
}
</style>
