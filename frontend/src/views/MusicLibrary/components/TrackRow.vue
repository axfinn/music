<script setup>
import { computed } from 'vue'
import {
  MoreFilled, Star, Document, Edit, Connection, Download, Delete, VideoPlay,
} from '@element-plus/icons-vue'
import CoverArt from './CoverArt.vue'
import { streamUrl } from '../api.js'
import { fmtTime, fmtCount } from '../utils/format.js'
import { useAdmin } from '../../../composables/useAdmin.js'

// 访客只能听歌：编辑信息 / 关联歌词库 / 删除曲目是管理动作。
const { isAdmin } = useAdmin()

const props = defineProps({
  track: { type: Object, required: true },
  index: { type: Number, required: true },
  current: { type: Boolean, default: false },
  playing: { type: Boolean, default: false },
  selected: { type: Boolean, default: false },
  selectionMode: { type: Boolean, default: false },
})
const emit = defineEmits(['play', 'toggle-select', 'favorite', 'lyrics', 'edit', 'link', 'remove'])

const sub = computed(() => {
  const bits = [props.track.artist || '未知歌手']
  if (props.track.album) bits.push(props.track.album)
  return bits.join(' · ')
})

function onRowClick() {
  if (props.selectionMode) emit('toggle-select')
  else emit('play')
}

function onCommand(cmd) {
  if (cmd === 'download') {
    const a = document.createElement('a')
    a.href = streamUrl(props.track.id)
    a.download = props.track.original_name || props.track.title || 'track'
    document.body.appendChild(a)
    a.click()
    a.remove()
    return
  }
  const map = { play: 'play', lyrics: 'lyrics', edit: 'edit', link: 'link', remove: 'remove' }
  if (map[cmd]) emit(map[cmd])
}
</script>

<template>
  <div
    class="track-row"
    :class="{ current, selected, 'select-mode': selectionMode, broken: track.playable === false }"
    :role="selectionMode ? 'checkbox' : 'button'"
    :aria-checked="selectionMode ? selected : undefined"
    :aria-label="`${track.title}${selectionMode ? '' : '，点击播放'}`"
    :tabindex="0"
    @click="onRowClick"
    @keydown.enter="onRowClick"
    @dblclick="!selectionMode && emit('play')"
  >
    <span class="cell-index">
      <el-checkbox
        v-if="selectionMode"
        :model-value="selected"
        :aria-label="`选择 ${track.title}`"
        @click.stop
        @change="emit('toggle-select')"
      />
      <span v-else-if="current && playing" class="ml-eq" aria-label="正在播放"><i /><i /><i /></span>
      <span v-else class="idx" :class="{ active: current }">{{ String(index + 1).padStart(2, '0') }}</span>
    </span>

    <CoverArt
      :track="track" size="sm" :overlay="!selectionMode"
      :playing="current && playing"
      @play="emit('play')"
    />

    <div class="cell-main">
      <div class="title-line">
        <span class="title" :class="{ active: current }">{{ track.title }}</span>
        <span v-if="track.lyrics || track.has_lyrics" class="ml-tag" title="自带歌词">词</span>
        <span v-if="track.lyric_song_id" class="ml-tag accent" title="已关联歌词库">库</span>
        <span v-if="track.playable === false" class="ml-tag warn" title="音频文件缺失">文件缺失</span>
      </div>
      <span class="sub">{{ sub }}<span class="sub-dur"> · {{ fmtTime(track.duration_sec) }}</span></span>
    </div>

    <span class="cell-plays" :title="`播放 ${track.play_count} 次`">{{ fmtCount(track.play_count) }}</span>
    <span class="cell-duration">{{ fmtTime(track.duration_sec) }}</span>

    <div class="cell-actions" @click.stop>
      <button
        class="ml-icon-btn fav" :class="{ active: track.favorite }"
        :aria-label="track.favorite ? '取消收藏' : '收藏'" title="收藏"
        @click="emit('favorite')"
      >
        <el-icon :size="16"><Star /></el-icon>
      </button>
      <el-dropdown trigger="click" placement="bottom-end" @command="onCommand">
        <button class="ml-icon-btn" aria-label="更多操作" title="更多操作"><el-icon :size="16"><MoreFilled /></el-icon></button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="play"><el-icon><VideoPlay /></el-icon>立即播放</el-dropdown-item>
            <el-dropdown-item command="lyrics"><el-icon><Document /></el-icon>查看歌词</el-dropdown-item>
            <el-dropdown-item v-if="isAdmin" command="edit"><el-icon><Edit /></el-icon>编辑信息</el-dropdown-item>
            <el-dropdown-item v-if="isAdmin" command="link"><el-icon><Connection /></el-icon>关联歌词库</el-dropdown-item>
            <el-dropdown-item command="download" divided><el-icon><Download /></el-icon>下载音频</el-dropdown-item>
            <el-dropdown-item v-if="isAdmin" command="remove"><el-icon><Delete /></el-icon>删除曲目</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<script>
// el-dropdown 的 'play' 命令需要转成 emit('play')，在 setup 里已用 emit 映射，
// 这里保留空壳只为让组件名可读（不影响 <script setup> 的编译）
export default { name: 'TrackRow' }
</script>

<style scoped>
.track-row {
  display: grid;
  grid-template-columns: 34px 48px minmax(0, 1fr) 62px 52px 78px;
  align-items: center;
  gap: 12px;
  min-height: 62px;
  padding: 7px 12px 7px 6px;
  border-radius: 12px;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease;
}
.track-row:hover { background: var(--ml-surface-2); }
.track-row.current { background: var(--ml-accent-soft); border-color: color-mix(in srgb, var(--ml-accent) 35%, transparent); }
.track-row.selected { background: var(--ml-accent-soft); }
.track-row.broken { opacity: 0.62; }

.cell-index { display: flex; align-items: center; justify-content: center; }
.idx { font-size: 12px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; }
.idx.active { color: var(--ml-accent); font-weight: 700; }

.cell-main { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.title-line { display: flex; align-items: center; gap: 7px; min-width: 0; }
.title {
  font-size: 14px; font-weight: 650; color: var(--ml-text);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.title.active { color: var(--ml-accent); }
.sub {
  font-size: 12px; color: var(--ml-text-3);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}

.cell-plays, .cell-duration {
  font-size: 12px; color: var(--ml-text-3); text-align: right;
  font-variant-numeric: tabular-nums;
}
.sub-dur { display: none; }
.cell-actions { display: flex; align-items: center; justify-content: flex-end; gap: 2px; opacity: 0; transition: opacity 0.16s ease; }
.track-row:hover .cell-actions,
.track-row:focus-within .cell-actions,
.track-row.current .cell-actions { opacity: 1; }
.fav.active { color: var(--ml-accent); }
.fav.active :deep(svg) { fill: currentColor; }

@media (max-width: 768px) {
  /* 手机上列宽极紧：砍掉「播放次数/时长」两列，把时长并进副行，
     否则标题只剩 ~90px，几乎每首都显示成「1.」「J.」 */
  .track-row {
    grid-template-columns: 24px 42px minmax(0, 1fr) 62px;
    gap: 8px; padding: 8px;
  }
  .cell-plays, .cell-duration { display: none; }
  .sub-dur { display: inline; }
  .cell-actions { opacity: 1; gap: 0; }
  .cell-actions .ml-icon-btn { width: 30px; height: 30px; }
}
</style>
