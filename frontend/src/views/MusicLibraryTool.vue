<script>
// 音乐库顶层壳：只做「编排」——创建一次页面级状态并注入，布局 + 生命周期。
// 业务分层见 ./MusicLibrary/：
//   api.js / utils/        → 网络与纯函数（可单测）
//   composables/           → 曲库状态、播放器桥接、歌词、上传
//   components/ + theme.css → 视图与设计系统
export default { name: 'MusicLibraryTool' }
</script>

<script setup>
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref } from 'vue'
import { ArrowLeft, VideoPlay, UploadFilled } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import './MusicLibrary/theme.css'
import { provideMusicContext } from './MusicLibrary/context.js'
import { musicApi } from './MusicLibrary/api.js'
import { useLibraryStore } from './MusicLibrary/composables/useLibraryStore.js'
import { useMusicPlayback } from './MusicLibrary/composables/useMusicPlayback.js'
import { useMusicLyrics } from './MusicLibrary/composables/useMusicLyrics.js'
import { useMusicUpload } from './MusicLibrary/composables/useMusicUpload.js'
import { fmtTime } from './MusicLibrary/utils/format.js'
import { useAdmin } from '../composables/useAdmin.js'

import LibrarySidebar from './MusicLibrary/components/LibrarySidebar.vue'
import LibraryTopbar from './MusicLibrary/components/LibraryTopbar.vue'
import DiscoverHome from './MusicLibrary/components/DiscoverHome.vue'
import CollectionGrid from './MusicLibrary/components/CollectionGrid.vue'
import TrackTable from './MusicLibrary/components/TrackTable.vue'
import NowPlayingPanel from './MusicLibrary/components/NowPlayingPanel.vue'
import UploadDialog from './MusicLibrary/components/UploadDialog.vue'
import TrackEditDialog from './MusicLibrary/components/TrackEditDialog.vue'
import LyricLinkDialog from './MusicLibrary/components/LyricLinkDialog.vue'
import LyricsDrawer from './MusicLibrary/components/LyricsDrawer.vue'
import DropOverlay from './MusicLibrary/components/DropOverlay.vue'

const route = useRoute()
const router = useRouter()

const editDialog = ref(false)
const editForm = ref(null)
const selectionMode = ref(false)

function focusSearch() {
  document.getElementById('ml-search')?.focus?.()
}

const store = useLibraryStore()
const playback = useMusicPlayback(store, { route, router, focusSearch })
const lyrics = useMusicLyrics(store, playback)
const upload = useMusicUpload(store)

// 访客只能听歌：上传/编辑/删除/关联歌词这些管理动作只对管理员显示。
// 真正的拦截在后端（没有管理会话就是 403），前端隐藏只是不给访客看入口。
const { isAdmin } = useAdmin()

const ui = {
  editDialog, editForm, selectionMode, focusSearch,
  async openEdit(track) {
    if (!track) return
    // 列表只带 has_lyrics/has_note 标记：先取详情再开弹窗，
    // 避免异步响应覆盖用户已经敲进去的内容。
    let full = track
    if (track.lyrics === undefined || track.note === undefined) {
      try {
        const detail = await musicApi.detail(track.id)
        if (detail) full = { ...track, ...detail }
      } catch { /* 详情失败仍允许编辑其它字段，未加载的字段保存时不会提交 */ }
    }
    editForm.value = { ...full }
    editDialog.value = true
  },
  toggleSelectionMode() {
    selectionMode.value = !selectionMode.value
    if (!selectionMode.value) store.clearSelection()
  },
}

provideMusicContext({ store, playback, lyrics, upload, ui, isAdmin })

const { section, facetFilter, facets, groups, visibleTracks, dimension, displayCount, displayDuration } = store
const { ambient } = playback

const isCollection = computed(() => section.value === 'albums' || section.value === 'artists')
const shownCount = displayCount
const totalDuration = displayDuration

// 只让「氛围光」跟着封面走；品牌色 / 按钮渐变保持固定，
// 否则整站按钮会随着歌曲在橙/绿/紫之间跳，品牌识别度就没了。
const ambientVars = computed(() => ({
  '--ml-glow-a': ambient.value.glowA,
  '--ml-glow-b': ambient.value.glowB,
}))

function openGroup(g) { store.setFacet(g.name) }
function playGroup(g) { playback.playGroup(g) }

onMounted(async () => {
  await store.load()
  playback.handlePlayQuery()
})
// 整站 keep-alive：切页不触发 onBeforeUnmount，键盘监听必须随 activated 挂摘，
// 否则离开音乐库后空格键仍会被本页处理，和沉浸播放页的快捷键打架。
onActivated(() => {
  playback.handlePlayQuery()
  window.addEventListener('keydown', playback.handleKeydown)
})
onDeactivated(() => window.removeEventListener('keydown', playback.handleKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', playback.handleKeydown))
</script>

<template>
  <div
    class="ml-app"
    :style="ambientVars"
    @dragenter="upload.onDragEnter"
    @dragover="upload.onDragOver"
    @dragleave="upload.onDragLeave"
    @drop="upload.onDrop"
  >
    <div class="ml-ambient" aria-hidden="true">
      <div class="ml-glow ml-glow-a" />
      <div class="ml-glow ml-glow-b" />
    </div>

    <div class="ml-shell">
      <LibrarySidebar />

      <main class="ml-main">
        <LibraryTopbar />

        <DiscoverHome v-if="section === 'discover'" />

        <template v-else>
          <div class="section-bar ml-card">
            <button
              v-if="facetFilter" class="ml-icon-btn" aria-label="返回分组视图"
              @click="store.setFacet(facetFilter)"
            >
              <el-icon :size="16"><ArrowLeft /></el-icon>
            </button>
            <div class="sb-title">
              <!-- 未下钻时标题已在顶栏出现过，这里只留统计，避免两块卡片都写「全部曲目」 -->
              <strong v-if="facetFilter">{{ facetFilter }}</strong>
              <span>{{ shownCount }} 首 · {{ fmtTime(totalDuration) }}</span>
            </div>
            <div v-if="facets.length > 1 && isCollection && !facetFilter" class="sb-chips ml-scroll">
              <button class="ml-chip active" @click="store.setFacet('')">全部 {{ groups.length }}</button>
              <button
                v-for="f in facets" :key="f.name" class="ml-chip"
                @click="store.setFacet(f.name)"
              >{{ f.name }}<span class="count">{{ f.count }}</span></button>
            </div>
            <button class="ml-btn primary sb-play" @click="playback.playAllVisible()">
              <el-icon :size="14"><VideoPlay /></el-icon> 播放全部
            </button>
          </div>

          <CollectionGrid
            v-if="isCollection && !facetFilter"
            :groups="groups" :dimension="dimension"
            @open="openGroup" @play="playGroup"
          />
          <TrackTable v-else />
        </template>

        <footer class="ml-foot">
          <span><el-icon :size="13"><UploadFilled /></el-icon> 把音频文件拖到页面任意位置即可批量入库</span>
          <span>空格 播放 / 暂停 · <span class="ml-kbd">/</span> 搜索 · <span class="ml-kbd">J</span> <span class="ml-kbd">K</span> 切歌</span>
        </footer>
      </main>

      <NowPlayingPanel />
    </div>

    <!-- 管理弹窗只对管理员挂载，访客根本进不来 -->
    <template v-if="isAdmin">
      <UploadDialog />
      <TrackEditDialog />
      <LyricLinkDialog />
    </template>
    <LyricsDrawer />
    <DropOverlay />
  </div>
</template>

<style scoped>
.ml-shell {
  display: grid;
  grid-template-columns: 216px minmax(0, 1fr) 306px;
  gap: 18px;
  align-items: start;
  max-width: 1680px;
  margin: 0 auto;
  padding: 18px 20px 24px;
}
.ml-main { min-width: 0; display: flex; flex-direction: column; gap: 14px; }

.section-bar {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 14px;
}
.sb-title { display: flex; flex-direction: column; gap: 2px; min-width: 96px; }
.sb-title strong { font-size: 15px; font-weight: 750; }
.sb-title span { font-size: 11px; color: var(--ml-text-3); }
.sb-chips {
  display: flex; gap: 6px; overflow-x: auto; flex: 1;
  scrollbar-width: none; padding: 2px;
}
.sb-chips::-webkit-scrollbar { display: none; }
.sb-chips .ml-chip { flex-shrink: 0; height: 28px; }
.sb-play { flex-shrink: 0; height: 34px; }

.ml-foot {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  flex-wrap: wrap; padding: 12px 4px 0;
  font-size: 11px; color: var(--ml-text-3);
}
.ml-foot span { display: inline-flex; align-items: center; gap: 5px; }

@media (max-width: 1240px) {
  .ml-shell { grid-template-columns: 190px minmax(0, 1fr) 270px; gap: 12px; padding: 14px; }
}
@media (max-width: 1080px) {
  .ml-shell { grid-template-columns: 150px minmax(0, 1fr); }
  .ml-shell > :last-child { display: none; }
}
@media (max-width: 768px) {
  .ml-shell {
    display: flex; flex-direction: column;
    /* 列向 flex 里子项按 max-content 撑宽过（实测侧栏撑到 657px），显式压回视口 */
    width: 100%; max-width: 100%; min-width: 0;
    gap: 10px; padding: 10px 10px calc(20px + var(--player-bar-h, 0px));
  }
  .ml-main { width: 100%; min-width: 0; gap: 10px; }
  .section-bar { flex-wrap: wrap; padding: 9px 10px; }
  .sb-chips { order: 3; flex-basis: 100%; }
  .sb-play { margin-left: auto; }
  .ml-foot { display: none; }
  /* 移动端把 Now Playing 放到内容之后，避免首屏被播放器占满 */
  .ml-shell > :last-child { display: flex; order: 2; }
}
</style>
