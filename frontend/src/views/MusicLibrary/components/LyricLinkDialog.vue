<script setup>
import { Search, Plus, Connection } from '@element-plus/icons-vue'
import { useMusicContext } from '../context.js'
import { initialOf } from '../utils/format.js'

const { lyrics } = useMusicContext()
const { linkDialog, linkTarget, linkKeyword, linkCandidates, linkLoading, linkBusy } = lyrics
</script>

<template>
  <el-dialog
    v-model="linkDialog" title="关联歌词库" width="540px"
    class="ml-dialog" align-center :close-on-click-modal="false"
  >
    <p class="hint">
      默认按歌名匹配（AI 曲目的歌手名常对不上，只认歌名即可）。没自动命中就手动选一首。
    </p>
    <el-input
      v-model="linkKeyword" placeholder="按歌名匹配；也可以搜其它关键词…"
      :prefix-icon="Search" clearable class="search"
      @input="lyrics.searchLink" @clear="lyrics.searchLink"
    />
    <div v-if="linkLoading" class="state">检索中…</div>
    <div v-else-if="!linkCandidates.length" class="state">歌词库里没找到匹配的歌，换个词试试</div>
    <div v-else class="candidates ml-scroll">
      <button v-for="s in linkCandidates" :key="s.id" class="candidate" @click="lyrics.doLink(s)">
        <span class="c-mark">{{ initialOf(s.title) }}</span>
        <span class="c-meta"><strong>{{ s.title }}</strong><small>{{ s.artist || '未知艺人' }}</small></span>
        <el-icon :size="16"><Plus /></el-icon>
      </button>
    </div>

    <template #footer>
      <button class="ml-btn" @click="linkDialog = false">取消</button>
      <button v-if="linkTarget?.lyric_song_id" class="ml-btn danger-text" @click="lyrics.unlinkLyric(linkTarget)">解除当前关联</button>
      <button class="ml-btn primary" :disabled="linkBusy" @click="lyrics.autoMatchOne()">
        <el-icon :size="14"><Connection /></el-icon>
        {{ linkBusy ? '匹配中…' : '按歌名自动匹配' }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  margin: 0 0 12px; padding: 9px 11px; border-radius: 10px;
  font-size: 12px; line-height: 1.6;
  color: var(--ml-text-2); background: var(--ml-surface-2);
}
.search { margin-bottom: 12px; }
.state { padding: 30px 0; text-align: center; font-size: 13px; color: var(--ml-text-3); }
.candidates { display: flex; flex-direction: column; gap: 7px; max-height: 46vh; overflow-y: auto; }
.candidate {
  display: flex; align-items: center; gap: 11px;
  width: 100%; padding: 9px 11px;
  border: 1px solid var(--ml-line); border-radius: 11px;
  background: var(--ml-surface); color: var(--ml-text-2);
  cursor: pointer; text-align: left;
  transition: border-color 0.16s ease, background-color 0.16s ease;
}
.candidate:hover { border-color: var(--ml-accent); background: var(--ml-surface-2); }
.c-mark {
  width: 36px; height: 36px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--ml-accent-soft); color: var(--ml-accent);
  font-size: 16px; font-weight: 750;
}
.c-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.c-meta strong { font-size: 13px; color: var(--ml-text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.c-meta small { font-size: 11px; color: var(--ml-text-3); }
.danger-text { color: #e5484d; border-color: rgba(229, 72, 77, 0.4); }
:deep(.el-dialog__footer) { display: flex; gap: 8px; justify-content: flex-end; flex-wrap: wrap; }
</style>
