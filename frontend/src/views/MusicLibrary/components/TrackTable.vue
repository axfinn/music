<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Star, Delete, Close, UploadFilled, Check } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import TrackRow from './TrackRow.vue'
import CoverArt from './CoverArt.vue'

const { store, playback, lyrics, upload, ui, isAdmin } = useMusicContext()
const {
  visibleTracks, viewMode, loading, loadingMore, hasMore, keyword,
  selected, tracks,
} = store
const { selectionMode } = ui
const { state } = playback

const sentinel = ref(null)
let observer = null

// 曲库可能有近千首，DOM 只渲染前 N 条，其余用「显示更多」按需放出
const renderLimit = ref(120)
const shown = computed(() => visibleTracks.value.slice(0, renderLimit.value))
const hiddenCount = computed(() => Math.max(0, visibleTracks.value.length - shown.value.length))
const isEmpty = computed(() => !loading.value && visibleTracks.value.length === 0)
const emptyText = computed(() => keyword.value.trim()
  ? '没有匹配的曲目，换个关键词试试'
  : '这里还没有歌曲')

function isCurrent(t) { return playback.isCurrent(t) }
function isSelected(t) { return selected.value.has(t.id) }

function onPlay(t) { playback.playTrack(t) }
async function onFavorite(t) { await store.toggleFavorite(t) }
function onLyrics(t) { lyrics.openDrawer(t) }
function onEdit(t) { ui.openEdit(t) }
function onLink(t) { lyrics.openLink(t) }

async function onRemove(t) {
  try {
    await ElMessageBox.confirm(
      `确定删除「${t.title}」？音频文件会一并删除，不可恢复。`,
      '删除曲目', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch { return }
  try { await store.remove(t) } catch (e) { ElMessage.error(e.message || '删除失败') }
}

async function onBatchRemove() {
  const ids = [...selected.value]
  if (!ids.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 首？音频文件会一并删除，不可恢复。`, '批量删除', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消',
    })
  } catch { return }
  await store.batchRemove(ids)
}

function onBatchFavorite(favorite) {
  store.batchFavorite([...selected.value], favorite)
}

onMounted(() => {
  if (typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(entries => {
    if (entries.some(e => e.isIntersecting) && hasMore.value && !loading.value && !loadingMore.value) {
      store.loadMore()
    }
  }, { rootMargin: '360px' })
  if (sentinel.value) observer.observe(sentinel.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <section class="table-wrap">
    <!-- 多选批量条 -->
    <transition name="batch">
      <div v-if="selectionMode" class="batch-bar ml-card">
        <button class="ml-icon-btn" aria-label="退出多选" @click="ui.toggleSelectionMode()"><el-icon :size="15"><Close /></el-icon></button>
        <strong>已选 {{ selected.size }} 首</strong>
        <span class="batch-sep" />
        <button class="ml-btn ghost" @click="store.selectAll(shown.map(t => t.id))">全选当前</button>
        <button class="ml-btn ghost" @click="store.clearSelection()">清空</button>
        <div class="batch-spacer" />
        <button class="ml-btn" :disabled="!selected.size" @click="onBatchFavorite(true)">
          <el-icon :size="14"><Star /></el-icon><span class="hide-sm">批量收藏</span>
        </button>
        <button class="ml-btn" :disabled="!selected.size" @click="onBatchFavorite(false)">
          <span class="hide-sm">取消收藏</span>
        </button>
        <button v-if="isAdmin" class="ml-btn danger-btn" :disabled="!selected.size" @click="onBatchRemove">
          <el-icon :size="14"><Delete /></el-icon><span class="hide-sm">删除</span>
        </button>
      </div>
    </transition>

    <!-- 骨架屏 -->
    <div v-if="loading" class="skeleton-list" aria-label="正在加载曲目">
      <div v-for="n in 7" :key="n" class="skel-row ml-skeleton" />
    </div>

    <!-- 空状态 -->
    <div v-else-if="isEmpty" class="empty ml-card">
      <div class="empty-art"><el-icon :size="34"><UploadFilled /></el-icon></div>
      <h3>{{ tracks.length === 0 ? '曲库还是空的' : emptyText }}</h3>
      <p>{{ tracks.length === 0 ? (isAdmin ? '把 AI 创作的 mp3 / flac / wav 拖进页面，或点「上传音乐」' : '等管理员上传，之后就能在线听') : '试试清空搜索词，或切换到其他分类' }}</p>
      <button v-if="tracks.length === 0 && isAdmin" class="ml-btn primary" @click="upload.open()">上传第一首歌</button>
    </div>

    <template v-else>
      <!-- 封面墙视图 -->
      <div v-if="viewMode === 'grid'" class="grid-view">
        <div
          v-for="(t, i) in shown" :key="t.id"
          class="grid-card ml-card"
          :class="{ current: isCurrent(t), selected: isSelected(t), 'select-mode': selectionMode }"
          role="button" tabindex="0"
          :aria-label="selectionMode ? `选择 ${t.title}` : `播放 ${t.title}`"
          :aria-pressed="selectionMode ? isSelected(t) : undefined"
          @click="selectionMode ? store.toggleSelect(t.id) : onPlay(t)"
          @keydown.enter="selectionMode ? store.toggleSelect(t.id) : onPlay(t)"
        >
          <CoverArt :track="t" size="xl" :overlay="!selectionMode" :playing="isCurrent(t) && state.isPlaying" @play="onPlay(t)" />
          <span v-if="selectionMode" class="gc-check" :class="{ on: isSelected(t) }" aria-hidden="true">
            <el-icon v-if="isSelected(t)" :size="12"><Check /></el-icon>
          </span>
          <span class="gc-title">{{ t.title }}</span>
          <span class="gc-sub">{{ t.artist || '未知歌手' }}</span>
          <span v-if="isCurrent(t) && state.isPlaying && !selectionMode" class="ml-eq gc-eq"><i /><i /><i /></span>
          <span class="gc-idx">{{ String(i + 1).padStart(2, '0') }}</span>
        </div>
      </div>

      <!-- 列表视图 -->
      <div v-else class="list-view ml-card">
        <div class="table-head" aria-hidden="true">
          <span>#</span><span /><span>标题</span><span>播放</span><span>时长</span><span />
        </div>
        <div class="rows">
          <TrackRow
            v-for="(t, i) in shown" :key="t.id"
            :track="t" :index="i"
            :current="isCurrent(t)" :playing="state.isPlaying"
            :selected="isSelected(t)" :selection-mode="selectionMode"
            @play="onPlay(t)" @toggle-select="store.toggleSelect(t.id)"
            @favorite="onFavorite(t)" @lyrics="onLyrics(t)"
            @edit="onEdit(t)" @link="onLink(t)" @remove="onRemove(t)"
          />
        </div>
      </div>

      <div ref="sentinel" class="sentinel" aria-hidden="true" />
      <div v-if="hiddenCount || hasMore" class="load-more">
        <button v-if="hiddenCount" class="ml-btn" @click="renderLimit += 120">
          显示更多 · 还有 {{ hiddenCount }} 首
        </button>
        <button v-if="hasMore" class="ml-btn" :disabled="loadingMore" @click="store.loadMore()">
          {{ loadingMore ? '加载中…' : '从服务器加载下一页' }}
        </button>
      </div>
    </template>
  </section>
</template>

<style scoped>
.table-wrap { display: flex; flex-direction: column; gap: 12px; }

.batch-bar {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 12px; border-radius: 14px;
  position: sticky; top: 0; z-index: 10;
}
.batch-bar strong { font-size: 13px; white-space: nowrap; }
.batch-sep { width: 1px; height: 20px; background: var(--ml-line-strong); margin: 0 4px; }
.batch-spacer { flex: 1; }
.batch-bar .ml-btn { height: 32px; padding: 0 12px; font-size: 12px; }
.danger-btn { color: #e5484d; border-color: rgba(229, 72, 77, 0.4); }
.batch-enter-active, .batch-leave-active { transition: opacity 0.2s ease, transform 0.2s ease; }
.batch-enter-from, .batch-leave-to { opacity: 0; transform: translateY(-8px); }

.skeleton-list { display: flex; flex-direction: column; gap: 8px; }
.skel-row { height: 62px; }

.empty { padding: 56px 24px; text-align: center; }
.empty-art {
  width: 68px; height: 68px; margin: 0 auto 16px;
  display: flex; align-items: center; justify-content: center;
  border-radius: 20px; background: var(--ml-accent-soft); color: var(--ml-accent);
}
.empty h3 { margin: 0 0 6px; font-size: 18px; }
.empty p { margin: 0 0 18px; font-size: 13px; color: var(--ml-text-3); }

.list-view { padding: 6px; }
.table-head {
  display: grid;
  grid-template-columns: 34px 48px minmax(0, 1fr) 62px 52px 78px;
  gap: 12px; align-items: center;
  padding: 8px 12px 8px 6px;
  font-size: 11px; font-weight: 700; letter-spacing: 0.06em;
  color: var(--ml-text-3); text-transform: uppercase;
  border-bottom: 1px solid var(--ml-line);
  margin-bottom: 4px;
}
.table-head span:nth-child(4), .table-head span:nth-child(5) { text-align: right; }
.rows { display: flex; flex-direction: column; gap: 2px; padding-top: 4px; }

.grid-view {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
}
.grid-card {
  position: relative;
  display: flex; flex-direction: column; gap: 4px;
  padding: 10px; border: 1px solid var(--ml-line);
  cursor: pointer; text-align: left;
  transition: transform 0.2s ease, border-color 0.2s ease;
  animation: ml-fade-up 0.4s ease both;
}
.grid-card:hover { transform: translateY(-4px); border-color: var(--ml-accent); }
.grid-card.current { border-color: var(--ml-accent); }
.grid-card.selected { border-color: var(--ml-accent); background: var(--ml-accent-soft); }
.gc-check {
  position: absolute; top: 16px; right: 16px; z-index: 2;
  width: 20px; height: 20px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  border: 1.5px solid rgba(255, 255, 255, 0.8);
  background: rgba(8, 10, 16, 0.42); color: #fff;
  backdrop-filter: blur(4px);
}
.gc-check.on { background: var(--ml-accent); border-color: var(--ml-accent); }
.gc-title {
  margin-top: 6px; font-size: 13px; font-weight: 700;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.gc-sub { font-size: 11px; color: var(--ml-text-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.gc-eq { position: absolute; top: 16px; right: 16px; }
.gc-idx {
  position: absolute; top: 16px; left: 16px;
  padding: 2px 7px; border-radius: 7px;
  background: rgba(8, 10, 16, 0.6); color: #fff;
  font-size: 10px; font-weight: 700;
}

.sentinel { height: 2px; }
.load-more { display: flex; justify-content: center; padding: 8px 0 18px; }

@media (max-width: 768px) {
  .table-head { display: none; }
  .grid-view { grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: 10px; }
  .hide-sm { display: none; }
  .batch-bar { gap: 3px; padding: 6px 8px; }
  .batch-bar .ml-btn { padding: 0 9px; }
  .empty { padding: 40px 16px; }
}
</style>
