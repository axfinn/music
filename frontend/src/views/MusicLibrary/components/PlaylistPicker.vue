<script setup>
/** 把一首/一批曲目加入（或移出）歌单的轻量弹窗。 */
import { computed, ref } from 'vue'
import { Check, Lock } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  tracks: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:modelValue'])
const { playlists } = useMusicContext()

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})
const busyId = ref('')
const ids = computed(() => props.tracks.map(t => t.id).filter(Boolean))
const firstTrack = computed(() => props.tracks[0] || null)

function allIn(p) {
  return ids.value.length > 0 && ids.value.every(id => p.track_ids.includes(id))
}

async function toggle(p) {
  if (!playlists.canEdit(p) || busyId.value) return
  busyId.value = p.id
  if (allIn(p)) {
    await playlists.run(async () => {
      // 每次都取最新的歌单对象：removeTrack 会替换 items 里的条目，
      // 用旧对象连续删多首只会留下最后一次的结果。
      for (const id of ids.value) {
        const fresh = playlists.items.value.find(x => x.id === p.id)
        if (fresh?.track_ids.includes(id)) await playlists.removeTrack(fresh, id)
      }
    }, '已从歌单移除')
  } else {
    await playlists.run(() => playlists.addTracks(p, ids.value), '已加入歌单')
  }
  busyId.value = ''
}
</script>

<template>
  <el-dialog
    v-model="visible" width="460px" class="ml-dialog" align-center
    :title="tracks.length > 1 ? `加入歌单（${tracks.length} 首）` : '加入歌单'"
  >
    <div v-if="firstTrack" class="subject">
      <CoverArt :track="firstTrack" size="sm" />
      <div class="subject-text">
        <strong>{{ firstTrack.title }}</strong>
        <span>{{ tracks.length > 1 ? `等 ${tracks.length} 首` : firstTrack.artist || '未知歌手' }}</span>
      </div>
    </div>

    <div v-if="!playlists.items.value.length" class="empty">还没有歌单，先在「我的歌单」里建一个吧。</div>
    <div v-else class="list ml-scroll">
      <button
        v-for="p in playlists.items.value" :key="p.id"
        class="row" :class="{ on: allIn(p), disabled: !playlists.canEdit(p) }"
        :disabled="!playlists.canEdit(p) || !!busyId"
        @click="toggle(p)"
      >
        <span class="check" :class="{ on: allIn(p) }">
          <el-icon v-if="allIn(p)" :size="12"><Check /></el-icon>
        </span>
        <span class="row-main">
          <strong>{{ p.name }}</strong>
          <small>{{ p.source === 'shared' ? '公开歌单' : '本机歌单' }} · {{ p.track_ids.length }} 首</small>
        </span>
        <el-icon v-if="!playlists.canEdit(p)" :size="14"><Lock /></el-icon>
        <span v-else-if="busyId === p.id" class="busy">…</span>
      </button>
    </div>

    <template #footer>
      <button class="ml-btn primary" @click="visible = false">完成</button>
    </template>
  </el-dialog>
</template>

<style scoped>
.subject { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.subject-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.subject-text strong { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.subject-text span { font-size: 11px; color: var(--ml-text-3); }
.empty { padding: 24px 8px; text-align: center; font-size: 13px; color: var(--ml-text-3); }
.list { display: flex; flex-direction: column; gap: 4px; max-height: 320px; overflow-y: auto; }
.row {
  display: flex; align-items: center; gap: 10px;
  width: 100%; min-height: 48px; padding: 7px 10px;
  border: 1px solid var(--ml-line); border-radius: 11px;
  background: transparent; color: var(--ml-text-2);
  font-family: inherit; text-align: left; cursor: pointer;
}
.row:hover { background: var(--ml-surface-2); }
.row.on { border-color: var(--ml-accent); background: var(--ml-accent-soft); }
.row.disabled { opacity: 0.6; cursor: not-allowed; }
.check {
  width: 18px; height: 18px; flex-shrink: 0;
  display: grid; place-items: center;
  border: 1.5px solid var(--ml-line-strong); border-radius: 6px;
}
.check.on { background: var(--ml-accent); border-color: var(--ml-accent); color: #fff; }
.row-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.row-main strong { font-size: 13px; color: var(--ml-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-main small { font-size: 11px; color: var(--ml-text-3); }
.busy { font-size: 15px; }
</style>
