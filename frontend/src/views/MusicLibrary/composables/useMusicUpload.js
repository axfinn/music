/**
 * 上传子系统：弹窗队列 + 整页拖放 + 并发控制 + 失败保留可重试。
 *
 * 后端 POST /music/tracks 挂了限流，所以并发固定为 2；失败的文件不会被
 * 静默清掉，队列里会标出原因，可以只重试失败的那几首。
 */
import { computed, onScopeDispose, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  AUDIO_ACCEPT, isAudioFile, MAX_AUDIO_BYTES, musicApi,
} from '../api.js'
import { humanSize } from '../utils/format.js'
import { useAdmin } from '../../../composables/useAdmin.js'

const CONCURRENCY = 2
const PROBE_TIMEOUT = 8000

function probeDuration(file) {
  return new Promise(resolve => {
    const url = URL.createObjectURL(file)
    const audio = new Audio()
    let done = false
    const finish = (value) => {
      if (done) return
      done = true
      clearTimeout(timer)
      audio.removeAttribute('src')
      URL.revokeObjectURL(url)
      resolve(value)
    }
    const timer = setTimeout(() => finish(0), PROBE_TIMEOUT)
    audio.preload = 'metadata'
    audio.onloadedmetadata = () => finish(isFinite(audio.duration) ? audio.duration : 0)
    audio.onerror = () => finish(0)
    audio.src = url
  })
}

export function useMusicUpload(store) {
  // 上传是管理动作，访客不开放。
  const { isAdmin } = useAdmin()
  const dialog = ref(false)
  const pending = ref([])
  const artist = ref('')
  const album = ref('')
  const uploading = ref(false)
  const dragActive = ref(false)
  let dragDepth = 0

  const readyCount = computed(() => pending.value.filter(p => p.status !== 'done').length)
  const hasFailures = computed(() => pending.value.some(p => p.status === 'error'))

  function addFiles(files) {
    const incoming = Array.from(files || []).filter(f => f instanceof File || f?.name)
    if (!incoming.length) return
    let rejected = 0
    const added = []
    for (const raw of incoming) {
      if (!isAudioFile(raw)) { rejected++; continue }
      if (raw.size > MAX_AUDIO_BYTES) { rejected++; continue }
      const base = raw.name.replace(/\.[^.]+$/, '')
      // reactive 包一层：时长是异步探测回来的，普通对象改字段不会触发视图更新
      const entry = reactive({
        key: `${raw.name}-${raw.size}-${Math.random().toString(36).slice(2, 8)}`,
        raw,
        title: base,
        duration: 0,
        humanSize: humanSize(raw.size),
        status: 'ready',
        error: '',
      })
      added.push(entry)
      probeDuration(raw).then(d => { entry.duration = d })
    }
    if (rejected) ElMessage.warning(`已跳过 ${rejected} 个文件（仅支持音频，单曲不超过 200MB）`)
    if (added.length) {
      pending.value = [...pending.value, ...added]
      dialog.value = true
    }
  }

  /** el-upload 的 on-change 回调（拿到的是 UploadFile，不是原生 File） */
  function onFileChange(uploadFile) {
    if (uploadFile?.raw) addFiles([uploadFile.raw])
  }

  function removeEntry(key) {
    pending.value = pending.value.filter(p => p.key !== key)
  }

  function clearDone() {
    pending.value = pending.value.filter(p => p.status !== 'done')
  }

  async function uploadAll() {
    if (uploading.value) return
    const targets = pending.value.filter(p => p.status !== 'done' && p.status !== 'uploading')
    if (!targets.length) return
    uploading.value = true
    let cursor = 0
    let ok = 0

    const worker = async () => {
      while (cursor < targets.length) {
        const p = targets[cursor++]
        p.status = 'uploading'
        p.error = ''
        try {
          await musicApi.upload(p.raw, {
            title: p.title.trim(),
            artist: artist.value.trim(),
            album: album.value.trim(),
            duration: p.duration,
          })
          p.status = 'done'
          ok++
        } catch (e) {
          p.status = 'error'
          p.error = e.message || '上传失败'
        }
      }
    }
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, targets.length) }, worker))
    uploading.value = false

    if (ok > 0) {
      ElMessage.success(`已入库 ${ok} 首`)
      await store.reload()
    }
    if (hasFailures.value) ElMessage.warning('部分文件上传失败，可点「重试失败项」')
    if (!hasFailures.value) {
      pending.value = []
      artist.value = ''
      album.value = ''
      dialog.value = false
    }
  }

  async function retryFailed() {
    pending.value = pending.value.map(p => (p.status === 'error' ? { ...p, status: 'ready', error: '' } : p))
    await uploadAll()
  }

  // ── 整页拖放：用计数器抵消子元素 dragenter/leave 冒泡 ──
  // 访客只能听歌，不能往曲库里加东西，所以拖放入口只对管理员开放。
  function onDragEnter(e) {
    if (!isAdmin.value) return
    if (!e.dataTransfer?.types?.includes('Files')) return
    e.preventDefault()
    dragDepth++
    dragActive.value = true
  }
  function onDragOver(e) {
    if (!isAdmin.value) return
    if (!e.dataTransfer?.types?.includes('Files')) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
  }
  function onDragLeave(e) {
    if (!isAdmin.value) return
    e.preventDefault()
    dragDepth = Math.max(0, dragDepth - 1)
    if (dragDepth === 0) dragActive.value = false
  }
  function onDrop(e) {
    if (!isAdmin.value) return
    e.preventDefault()
    dragDepth = 0
    dragActive.value = false
    addFiles(e.dataTransfer?.files)
  }

  onScopeDispose(() => { dragDepth = 0 })

  return {
    dialog, pending, artist, album, uploading, dragActive, readyCount, hasFailures,
    accept: AUDIO_ACCEPT,
    addFiles, onFileChange, removeEntry, clearDone, uploadAll, retryFailed,
    open: () => { dialog.value = true },
    onDragEnter, onDragOver, onDragLeave, onDrop,
  }
}
