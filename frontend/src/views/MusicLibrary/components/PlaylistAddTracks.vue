<script setup>
/** 给指定歌单批量添加曲目：从已加载的全量曲库里搜索、勾选。 */
import { computed, ref, watch } from 'vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import { fmtTime } from '../utils/format.js'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  playlist: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue'])
const { store, playlists } = useMusicContext()

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})
const keyword = ref('')
const picked = ref(new Set())
const busy = ref(false)

watch(() => props.modelValue, open => {
  if (!open) return
  keyword.value = ''
  picked.value = new Set()
})

const candidates = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  const list = store.tracks.value.filter(t =>
    !q || [t.title, t.artist, t.album].some(v => String(v || '').toLowerCase().includes(q)))
  return list.slice(0, 300)
})

function isPicked(id) { return picked.value.has(id) }
function toggle(id) {
  const next = new Set(picked.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  picked.value = next
}

async function submit() {
  if (!picked.value.size || busy.value || !props.playlist) return
  busy.value = true
  // 服务端会去重；这里只提交新勾的，歌单里已有的曲目不用再传一遍。
  const ids = [...picked.value].filter(id => !props.playlist.track_ids.includes(id))
  const ok = await playlists.run(() => playlists.addTracks(props.playlist, ids), `已添加 ${ids.length} 首`)
  busy.value = false
  if (ok) visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible" width="560px" class="ml-dialog" align-center
    :title="`添加歌曲 · ${playlist?.name || ''}`"
  >
    <el-input v-model="keyword" placeholder="搜索歌名 / 歌手 / 专辑" clearable class="search" />
    <div class="picked-bar">
      <span>已选 <strong>{{ picked.size }}</strong> 首</span>
      <button class="ml-btn ghost" :disabled="!picked.size" @click="picked = new Set()">清空</button>
    </div>
    <div v-if="!candidates.length" class="empty">没有匹配的曲目</div>
    <div v-else class="list ml-scroll">
      <button
        v-for="t in candidates" :key="t.id" class="row" :class="{ on: isPicked(t.id) }"
        type="button" @click="toggle(t.id)"
      >
        <span class="check" :class="{ on: isPicked(t.id) }">{{ isPicked(t.id) ? '✓' : '' }}</span>
        <CoverArt :track="t" size="xs" />
        <span class="row-main">
          <strong>{{ t.title }}</strong>
          <small>{{ t.artist || '未知歌手' }}<template v-if="t.album"> · {{ t.album }}</template></small>
        </span>
        <span class="dur">{{ fmtTime(t.duration_sec) }}</span>
      </button>
    </div>
    <small class="note">列表最多显示 300 条结果，可用关键词缩小范围。</small>

    <template #footer>
      <button class="ml-btn" @click="visible = false">取消</button>
      <button class="ml-btn primary" :disabled="!picked.size || busy" @click="submit">
        {{ busy ? '添加中…' : `添加${picked.size ? ` (${picked.size})` : ''}` }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.search { margin-bottom: 10px; }
.picked-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 12px; color: var(--ml-text-2); }
.picked-bar strong { color: var(--ml-accent); }
.picked-bar .ml-btn { height: 28px; padding: 0 10px; font-size: 11px; }
.list { display: flex; flex-direction: column; gap: 3px; max-height: 360px; overflow-y: auto; }
.row {
  display: flex; align-items: center; gap: 9px;
  width: 100%; min-height: 48px; padding: 6px 8px;
  border: 1px solid transparent; border-radius: 10px;
  background: transparent; color: var(--ml-text);
  font-family: inherit; text-align: left; cursor: pointer;
}
.row:hover { background: var(--ml-surface-2); }
.row.on { background: var(--ml-accent-soft); border-color: var(--ml-accent); }
.check {
  width: 18px; height: 18px; flex-shrink: 0;
  display: grid; place-items: center; font-size: 11px;
  border: 1.5px solid var(--ml-line-strong); border-radius: 6px; color: transparent;
}
.check.on { background: var(--ml-accent); border-color: var(--ml-accent); color: #fff; }
.row-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.row-main strong { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-main small { font-size: 11px; color: var(--ml-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.dur { font-size: 11px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; }
.empty { padding: 30px 8px; text-align: center; font-size: 13px; color: var(--ml-text-3); }
.note { display: block; margin-top: 8px; font-size: 11px; color: var(--ml-text-3); }
</style>
