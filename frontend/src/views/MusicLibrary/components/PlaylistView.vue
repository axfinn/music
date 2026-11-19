<script setup>
/**
 * 歌单视图：曲库侧栏 / 顶部「歌单」入口共用的内容区。
 * 列表 → 详情两级，不新增路由页；公开歌单与本机歌单在这里统一呈现。
 */
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowLeft, Plus, VideoPlay, Delete, Edit, CirclePlus, Rank, Lock,
} from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import CoverArt from './CoverArt.vue'
import PlaylistAddTracks from './PlaylistAddTracks.vue'
import { fmtTime } from '../utils/format.js'

const { playback, playlists, ui } = useMusicContext()
const { items, loading, selected } = playlists

const addVisible = ref(false)

const tracks = computed(() => playlists.tracksOf(selected.value))
const duration = computed(() => tracks.value.reduce((s, t) => s + (Number(t.duration_sec) || 0), 0))
const canEdit = computed(() => playlists.canEdit(selected.value))

function firstTrack(p) { return playlists.tracksOf(p)[0] || null }
function durationOf(p) {
  return playlists.tracksOf(p).reduce((s, t) => s + (Number(t.duration_sec) || 0), 0)
}

function openCreate() { ui.openPlaylistDialog() }
function openEdit() { ui.openPlaylistDialog(selected.value) }
function openAdd() { addVisible.value = true }

function play(p) {
  const list = playlists.tracksOf(p)
  if (!list.length) return ElMessage.warning('这个歌单还没有曲目')
  playback.playTracks(list)
}
function playSelected() { if (selected.value) play(selected.value) }

function enqueue(p) {
  const list = playlists.tracksOf(p)
  if (!list.length) return ElMessage.warning('这个歌单还没有曲目')
  list.forEach(t => playback.player.addToPlaylist(playback.toPlayerTrack(t)))
  ElMessage.success(`已把 ${list.length} 首加入播放队列`)
}

async function removePlaylist(p) {
  try {
    await ElMessageBox.confirm(`确定删除歌单「${p.name}」？曲目不会从曲库删除。`, '删除歌单', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消',
    })
  } catch { return }
  await playlists.run(() => playlists.remove(p), '歌单已删除')
}

async function removeTrack(track) {
  if (!selected.value) return
  await playlists.run(() => playlists.removeTrack(selected.value, track.id), '已从歌单移除')
}
</script>

<template>
  <div class="pl-view">
    <!-- ── 歌单详情 ── -->
    <template v-if="selected">
      <section class="detail ml-card">
        <button class="ml-icon-btn back" aria-label="返回歌单列表" @click="playlists.select('')">
          <el-icon :size="16"><ArrowLeft /></el-icon>
        </button>
        <div class="detail-cover">
          <CoverArt :track="tracks[0] || null" :seed="selected.id" size="xl" />
        </div>
        <div class="detail-meta">
          <span class="ml-eyebrow">{{ selected.source === 'shared' ? '公开歌单' : '本机歌单' }}</span>
          <h2>{{ selected.name }}</h2>
          <p v-if="selected.description">{{ selected.description }}</p>
          <p class="detail-count">{{ tracks.length }} 首 · {{ fmtTime(duration) }}</p>
          <div class="detail-actions">
            <button class="ml-btn primary" :disabled="!tracks.length" @click="playSelected">
              <el-icon :size="14"><VideoPlay /></el-icon> 播放全部
            </button>
            <button class="ml-btn" :disabled="!tracks.length" @click="enqueue(selected)">
              <el-icon :size="14"><Rank /></el-icon> 加入队列
            </button>
            <button v-if="canEdit" class="ml-btn ghost" @click="openAdd">
              <el-icon :size="14"><CirclePlus /></el-icon> 添加歌曲
            </button>
            <button v-if="canEdit" class="ml-icon-btn" title="编辑歌单" aria-label="编辑歌单" @click="openEdit">
              <el-icon :size="15"><Edit /></el-icon>
            </button>
            <button v-if="canEdit" class="ml-icon-btn danger" title="删除歌单" aria-label="删除歌单" @click="removePlaylist(selected)">
              <el-icon :size="15"><Delete /></el-icon>
            </button>
          </div>
        </div>
      </section>

      <div class="track-list ml-card">
        <div v-if="!tracks.length" class="empty">
          <el-icon :size="28"><CirclePlus /></el-icon>
          <strong>这个歌单还没有曲目</strong>
          <button v-if="canEdit" class="ml-btn primary" @click="openAdd">添加歌曲</button>
          <span v-else>等管理员往公开歌单里加歌</span>
        </div>
        <div v-for="(t, i) in tracks" :key="t.id" class="track">
          <span class="idx">{{ String(i + 1).padStart(2, '0') }}</span>
          <CoverArt :track="t" size="xs" />
          <span class="t-main">
            <strong>{{ t.title }}</strong>
            <small>{{ t.artist || '未知歌手' }}<template v-if="t.album"> · {{ t.album }}</template></small>
          </span>
          <span class="t-dur">{{ fmtTime(t.duration_sec) }}</span>
          <button
            v-if="canEdit" class="ml-icon-btn" title="从歌单移除" aria-label="从歌单移除"
            @click="removeTrack(t)"
          ><el-icon :size="14"><Delete /></el-icon></button>
        </div>
      </div>
    </template>

    <!-- ── 歌单列表 ── -->
    <template v-else>
      <div class="list-head">
        <div>
          <h2>我的歌单</h2>
          <p>公开的歌单大家都能听，本机歌单只属于这台设备。</p>
        </div>
        <button class="ml-btn primary" @click="openCreate">
          <el-icon :size="14"><Plus /></el-icon> 新建歌单
        </button>
      </div>

      <div v-if="loading" class="skeleton-grid">
        <div v-for="n in 4" :key="n" class="skel ml-skeleton" />
      </div>
      <div v-else-if="!items.length" class="empty ml-card">
        <el-icon :size="30"><CirclePlus /></el-icon>
        <strong>还没有歌单</strong>
        <span>建一个歌单，把喜欢的歌装进去</span>
        <button class="ml-btn primary" @click="openCreate">新建第一个歌单</button>
      </div>
      <div v-else class="pl-grid">
        <article
          v-for="p in items" :key="p.id" class="pl-card ml-card"
          role="button" tabindex="0"
          :aria-label="`打开歌单 ${p.name}`"
          @click="playlists.select(p.id)" @keydown.enter="playlists.select(p.id)"
        >
          <div class="pl-cover">
            <CoverArt
              :track="firstTrack(p)" :seed="p.id" size="xl" overlay
              :playing="false" @play="play(p)"
            />
          </div>
          <div class="pl-info">
            <strong :title="p.name">{{ p.name }}</strong>
            <span class="pl-badge">
              <el-icon :size="11"><Lock v-if="p.source === 'local'" /></el-icon>
              {{ p.source === 'shared' ? '公开' : '本机' }} · {{ playlists.countOf(p) }} 首
            </span>
          </div>
          <span class="pl-duration">{{ fmtTime(durationOf(p)) }}</span>
          <button v-if="playlists.canEdit(p)" class="ml-icon-btn pl-edit" title="编辑歌单" aria-label="编辑歌单" @click.stop="playlists.select(p.id); openEdit()">
            <el-icon :size="14"><Edit /></el-icon>
          </button>
        </article>
      </div>
    </template>

    <PlaylistAddTracks v-model="addVisible" :playlist="selected" />
  </div>
</template>

<style scoped>
.pl-view { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.list-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 2px; }
.list-head h2 { margin: 0 0 4px; font-size: 19px; font-weight: 750; }
.list-head p { margin: 0; font-size: 12px; color: var(--ml-text-3); }
.list-head .ml-btn { height: 36px; flex-shrink: 0; }

.pl-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 14px; }
.pl-card {
  position: relative; display: flex; flex-direction: column; gap: 8px;
  padding: 10px; border: 1px solid var(--ml-line);
  cursor: pointer; transition: transform 0.18s ease, border-color 0.18s ease;
}
.pl-card:hover { transform: translateY(-3px); border-color: var(--ml-accent); }
.pl-cover { width: 100%; }
.pl-cover :deep(.cover-art) { width: 100%; }
.pl-info { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.pl-info strong { font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pl-badge { display: inline-flex; align-items: center; gap: 3px; font-size: 11px; color: var(--ml-text-3); }
.pl-duration { position: absolute; top: 18px; right: 18px; padding: 2px 7px; border-radius: 7px; background: rgba(8,10,16,0.6); color: #fff; font-size: 10px; }
.pl-edit { position: absolute; bottom: 12px; right: 12px; }
.skeleton-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 14px; }
.skel { height: 230px; }

.detail { position: relative; display: grid; grid-template-columns: 168px minmax(0, 1fr); gap: 20px; padding: 20px; }
.back { position: absolute; top: 12px; right: 12px; }
.detail-cover :deep(.cover-art) { width: 168px; }
.detail-meta { display: flex; flex-direction: column; gap: 8px; min-width: 0; justify-content: center; }
.detail-meta h2 { margin: 0; font-size: 24px; font-weight: 780; line-height: 1.25; }
.detail-meta p { margin: 0; font-size: 13px; color: var(--ml-text-2); }
.detail-meta .detail-count { color: var(--ml-text-3); font-size: 12px; }
.detail-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.detail-actions .ml-btn { height: 36px; }
.danger { color: #e5484d; }

.track-list { padding: 8px 12px; }
.track {
  display: flex; align-items: center; gap: 10px;
  min-height: 54px; padding: 6px 4px;
  border-bottom: 1px solid var(--ml-line);
}
.track:last-child { border-bottom: none; }
.idx { width: 24px; text-align: center; font-size: 11px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; }
.t-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.t-main strong { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.t-main small { font-size: 11px; color: var(--ml-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.t-dur { font-size: 11px; color: var(--ml-text-3); font-variant-numeric: tabular-nums; }
.empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 44px 16px; color: var(--ml-text-3); font-size: 12px; text-align: center; }
.empty strong { color: var(--ml-text); font-size: 14px; }
.empty .ml-btn { margin-top: 6px; }

@media (max-width: 768px) {
  .list-head { flex-wrap: wrap; }
  .pl-grid, .skeleton-grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
  .detail { grid-template-columns: 96px minmax(0, 1fr); gap: 14px; padding: 14px; }
  .detail-cover :deep(.cover-art) { width: 96px; }
  .detail-meta h2 { font-size: 18px; }
  .detail-actions .ml-btn { height: 32px; padding: 0 10px; font-size: 12px; }
}
@media (min-width: 1241px) {
  .list-head h2 { font-size: 22px; }
  .pl-grid, .skeleton-grid { grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 18px; }
  .detail { grid-template-columns: 200px minmax(0, 1fr); gap: 26px; padding: 26px; }
  .detail-cover :deep(.cover-art) { width: 200px; }
  .detail-meta h2 { font-size: 28px; }
}
</style>
