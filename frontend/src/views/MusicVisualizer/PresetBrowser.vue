<template>
  <div class="pz-panel">
    <header class="pz-head">
      <div>
        <p class="pz-eyebrow">OPEN SOURCE · MILKDROP</p>
        <h2>预设库 <span>{{ entries.length || '—' }}</span></h2>
      </div>
      <button class="pz-close" title="收起预设库 (B)" @click="$emit('close')">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round">
          <path d="M6 6l12 12M18 6L6 18" />
        </svg>
      </button>
    </header>
    <p class="pz-source">Butterchurn · MIT · Winamp MilkDrop 社区预设</p>

    <div v-if="error" class="pz-note error">
      开源引擎载入失败，当前显示内置实验场景。
      <button @click="$emit('retry')">重试</button>
    </div>
    <div v-else-if="loading" class="pz-note">
      <i class="pz-spinner" /> 正在载入开源可视化引擎…
    </div>

    <template v-else>
      <label class="pz-search">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
          <circle cx="11" cy="11" r="6.5" /><path d="M16 16l4.5 4.5" />
        </svg>
        <input v-model="query" type="search" placeholder="搜索预设或作者" aria-label="搜索预设" />
        <button v-if="query" class="pz-clear" title="清空" @click="query = ''">×</button>
      </label>

      <div class="pz-filters">
        <button
          v-for="f in FILTERS" :key="f.id"
          :class="{ active: filter === f.id }" @click="filter = f.id"
        >{{ f.label }}<em>{{ f.count }}</em></button>
      </div>

      <div ref="listRef" class="pz-list">
        <button
          v-for="(e, i) in visible" :key="e.key"
          class="pz-item" :class="{ active: isOpenSource && e.key === currentKey }"
          :title="e.key" @click="$emit('select', e.key)"
        >
          <span class="pz-idx">{{ String(i + 1).padStart(2, '0') }}</span>
          <span class="pz-text">
            <strong>{{ e.title }}</strong>
            <em>{{ e.author }}</em>
          </span>
          <i v-if="e.verified" class="pz-verified" title="已验证可正常渲染" />
          <span
            class="pz-star" :class="{ on: favorites.includes(e.key) }"
            :title="favorites.includes(e.key) ? '取消收藏' : '收藏'"
            @click.stop="$emit('favorite', e.key)"
          >★</span>
        </button>
        <p v-if="!visible.length" class="pz-empty">
          {{ filter === 'favorites' ? '还没有收藏的预设，点右侧 ★ 收一个' : '没有匹配的预设' }}
        </p>
      </div>
    </template>

    <div class="pz-tools">
      <button title="上一个预设 ([)" @click="step(-1)">‹</button>
      <button class="pz-random" title="随机抽一个已验证预设 (R)" @click="$emit('random')">随机</button>
      <button title="下一个预设 (])" @click="step(1)">›</button>
      <button
        class="pz-auto" :class="{ on: autoMode !== 'off' }"
        :title="autoHint" @click="cycleAuto"
      >{{ autoLabel }}</button>
    </div>

    <div class="pz-fx-row">
      <button
        :class="{ on: lyricFxOn }"
        :title="lyricFxOn ? '关闭自建 3D 歌词字效 (L)' : '开启自建 3D 歌词字效 (L)'"
        @click="$emit('toggle-lyric')"
      ><span>3D 歌词字效</span><em>{{ lyricFxOn ? 'ON' : 'OFF' }}</em></button>
      <small>L · 自建效果，默认关闭</small>
    </div>

    <details class="pz-fold" :open="toneOpen">
      <summary @click.prevent="toneOpen = !toneOpen">
        画面调节 <span>{{ isOpenSource ? '对当前预设生效' : '仅开源预设态生效' }}</span>
      </summary>
      <div class="pz-sliders">
        <label v-for="s in TONES" :key="s.key">
          <span>{{ s.label }}</span>
          <input
            type="range" :min="s.min" :max="s.max" :step="s.step"
            :value="tone[s.key]" @input="onTone(s.key, $event.target.value)"
          />
          <em>{{ Number(tone[s.key] ?? 1).toFixed(2) }}</em>
        </label>
      </div>
    </details>

    <details class="pz-fold pz-lab" :open="labOpen">
      <summary @click.prevent="labOpen = !labOpen">
        LAB · 自建场景 <span>实验 / 回退</span>
      </summary>
      <p class="pz-lab-note">
        自研 three.js 场景，观感不及社区预设，仅作实验与 WebGL2 不可用时的回退。
      </p>
      <div class="pz-lab-grid">
        <button
          v-for="(s, i) in scenes" :key="s.id"
          :class="{ active: !isOpenSource && labSceneId === s.id }"
          :title="s.hint" @click="$emit('select-lab', s.id)"
        ><em>{{ String(i + 1).padStart(2, '0') }}</em>{{ s.label }}</button>
      </div>
      <div v-if="!isOpenSource" class="pz-palettes">
        <span>MOOD</span>
        <button
          v-for="p in palettes" :key="p.id"
          :class="{ active: paletteId === p.id }"
          :style="{ '--sw-a': p.a, '--sw-b': p.b, '--sw-c': p.c }"
          :title="`${p.label} · ${p.hint}`" @click="$emit('palette', p.id)"
        />
      </div>
      <button class="pz-back" @click="$emit('back-open')">← 回到开源预设</button>
    </details>

    <footer class="pz-foot">
      <a :href="OPEN_SOURCE.engineUrl" target="_blank" rel="noopener noreferrer">Butterchurn</a>
      <span>· MIT ·</span>
      <a :href="OPEN_SOURCE.presetsUrl" target="_blank" rel="noopener noreferrer">预设来源</a>
    </footer>
  </div>
</template>

<script setup>
// 开源预设库面板 —— 沉浸播放页的一等公民。
// 数据来自 milkdrop.js 的 loadPresetLibrary(), 这里只负责「挑预设」和
// 「调节画面」, 不碰 WebGL: 引擎下载/渲染全在父页面里。
//
// 筛选默认停在「推荐」(精选 + 体检过的预设), 而不是 100 个全放出来:
// 社区预设里约 1/5 在弱 GPU 上会黑屏, 默认列表必须保证点哪个都有画面。
// 想翻全量就点「全部」—— 那是显式选择, 用户自己承担。

import { computed, nextTick, ref, watch } from 'vue'
import { OPEN_SOURCE } from './milkdrop.js'

const props = defineProps({
  entries: { type: Array, default: () => [] },
  currentKey: { type: String, default: '' },
  favorites: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  error: { type: Boolean, default: false },
  autoMode: { type: String, default: 'track' },
  tone: { type: Object, default: () => ({ brightness: 0.94, contrast: 1.06, saturate: 1.02 }) },
  isOpenSource: { type: Boolean, default: true },
  lyricFxOn: { type: Boolean, default: false },
  labSceneId: { type: String, default: '' },
  scenes: { type: Array, default: () => [] },
  palettes: { type: Array, default: () => [] },
  paletteId: { type: String, default: '' },
})

const emit = defineEmits([
  'select', 'favorite', 'random', 'step', 'auto', 'tone', 'toggle-lyric', 'select-lab',
  'back-open', 'palette', 'retry', 'close',
])

const query = ref('')
const filter = ref('featured')
const toneOpen = ref(false)
const labOpen = ref(false)

const recommended = computed(() => props.entries.filter(e => e.curated || e.verified))
const favoritesList = computed(() => props.entries.filter(e => props.favorites.includes(e.key)))
const FILTERS = computed(() => [
  { id: 'featured', label: '推荐', count: recommended.value.length },
  { id: 'all', label: '全部', count: props.entries.length },
  { id: 'favorites', label: '收藏', count: favoritesList.value.length },
])

const visible = computed(() => {
  let list = filter.value === 'featured' ? recommended.value
    : filter.value === 'favorites' ? favoritesList.value
      : props.entries
  const q = query.value.trim().toLowerCase()
  if (q) list = list.filter(e => e.key.toLowerCase().includes(q))
  return list
})

// 当前预设可能排在列表很后面(例如默认 Rewop 在推荐里第 20 位),
// 打开面板/切预设后要把它滚到视野中央, 否则高亮看不见, 像没选中一样。
const listRef = ref(null)
watch([() => props.currentKey, visible, () => props.loading], async () => {
  if (props.loading) return
  await nextTick()
  const list = listRef.value
  if (!list) return
  const active = list.querySelector('.pz-item.active')
  if (!active) return
  const lr = list.getBoundingClientRect()
  const ar = active.getBoundingClientRect()
  const top = ar.top - lr.top + list.scrollTop - list.clientHeight / 2 + ar.height / 2
  list.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
}, { flush: 'post' })

const TONES = [
  { key: 'brightness', label: '亮度', min: 0.5, max: 1.5, step: 0.02 },
  { key: 'contrast', label: '对比', min: 0.7, max: 1.6, step: 0.02 },
  { key: 'saturate', label: '饱和', min: 0, max: 1.8, step: 0.02 },
]

const AUTO_LABELS = { off: '自动 · 关', track: '自动 · 每曲', time: '自动 · 定时' }
const AUTO_HINTS = {
  off: '自动换预设已关闭，点击开启「每曲一换」',
  track: '每换一首歌自动换一个预设，点击改为定时轮换',
  time: '每 45 秒自动换一个预设，点击关闭',
}
const autoLabel = computed(() => AUTO_LABELS[props.autoMode] || AUTO_LABELS.off)
const autoHint = computed(() => AUTO_HINTS[props.autoMode] || AUTO_HINTS.off)

function cycleAuto() {
  const order = ['off', 'track', 'time']
  emit('auto', order[(order.indexOf(props.autoMode) + 1) % order.length])
}

function onTone(key, value) {
  emit('tone', { ...props.tone, [key]: Number(value) })
}

function step(dir) {
  emit('step', { dir, keys: visible.value.map(e => e.key) })
}
</script>

<style scoped>
.pz-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  min-height: 0;
  padding: 16px 14px 12px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.11);
  background: linear-gradient(160deg, rgba(18, 20, 28, 0.82), rgba(8, 10, 16, 0.9));
  box-shadow: 0 22px 60px rgba(0, 0, 0, 0.48), 0 0 0 1px rgba(0, 0, 0, 0.24) inset;
  backdrop-filter: blur(22px);
  -webkit-backdrop-filter: blur(22px);
  color: #eef1f7;
  font-family: 'Inter', 'Helvetica Neue', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
  user-select: none;
}

.pz-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.pz-eyebrow { margin: 0 0 5px; font-size: 8px; letter-spacing: 0.28em; color: rgba(255, 255, 255, 0.36); }
.pz-head h2 { margin: 0; font-size: 16px; font-weight: 400; letter-spacing: 0.06em; }
.pz-head h2 span { font-size: 10px; color: rgba(255, 255, 255, 0.35); margin-left: 5px; font-variant-numeric: tabular-nums; }
.pz-close {
  display: inline-flex; align-items: center; justify-content: center;
  width: 26px; height: 26px; border-radius: 50%; cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.12); background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.6); transition: all 0.2s;
}
.pz-close:hover { color: #fff; background: rgba(255, 255, 255, 0.12); }
.pz-source { margin: -4px 0 2px; font-size: 9px; letter-spacing: 0.04em; color: rgba(255, 255, 255, 0.34); }

.pz-note {
  display: flex; align-items: center; gap: 8px;
  padding: 18px 6px; font-size: 11px; color: rgba(255, 255, 255, 0.5);
  line-height: 1.7;
}
.pz-note.error { color: #ffc9a8; }
.pz-note button {
  border: 1px solid rgba(255, 255, 255, 0.2); background: rgba(255, 255, 255, 0.07);
  color: #fff; border-radius: 14px; padding: 3px 10px; font-size: 10px; cursor: pointer;
  font-family: inherit;
}
.pz-spinner {
  width: 11px; height: 11px; border-radius: 50%; flex: 0 0 auto;
  border: 1.5px solid rgba(255, 255, 255, 0.18); border-top-color: var(--accent, #7dd3fc);
  animation: pz-spin 0.8s linear infinite;
}
@keyframes pz-spin { to { transform: rotate(360deg); } }

.pz-search {
  display: flex; align-items: center; gap: 8px; flex: 0 0 auto;
  padding: 0 10px; height: 32px; border-radius: 9px;
  border: 1px solid rgba(255, 255, 255, 0.1); background: rgba(255, 255, 255, 0.045);
  color: rgba(255, 255, 255, 0.4);
}
.pz-search:focus-within { border-color: color-mix(in srgb, var(--accent, #7dd3fc) 55%, transparent); }
.pz-search input {
  flex: 1; min-width: 0; border: none; outline: none; background: transparent;
  color: #fff; font-size: 11px; font-family: inherit; user-select: text;
}
.pz-search input::placeholder { color: rgba(255, 255, 255, 0.28); }
.pz-search input::-webkit-search-cancel-button { display: none; }
.pz-clear { border: none; background: none; color: rgba(255, 255, 255, 0.45); cursor: pointer; font-size: 15px; line-height: 1; }

.pz-filters { display: flex; gap: 4px; flex: 0 0 auto; }
.pz-filters button {
  flex: 1; display: inline-flex; align-items: center; justify-content: center; gap: 5px;
  padding: 5px 4px; border-radius: 7px; cursor: pointer; font-family: inherit;
  border: 1px solid transparent; background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.45); font-size: 10px; letter-spacing: 0.05em;
  transition: all 0.18s;
}
.pz-filters button em { font-style: normal; font-size: 8px; opacity: 0.65; font-variant-numeric: tabular-nums; }
.pz-filters button:hover { color: rgba(255, 255, 255, 0.85); }
.pz-filters button.active {
  color: #fff; background: color-mix(in srgb, var(--accent, #7dd3fc) 17%, transparent);
  border-color: color-mix(in srgb, var(--accent, #7dd3fc) 42%, transparent);
}

.pz-list {
  flex: 1 1 auto; min-height: 90px; overflow-y: auto; overscroll-behavior: contain;
  display: flex; flex-direction: column; gap: 2px;
  scrollbar-width: thin; scrollbar-color: rgba(255, 255, 255, 0.18) transparent;
  mask-image: linear-gradient(to bottom, transparent 0, #000 8px, #000 calc(100% - 8px), transparent 100%);
}
.pz-list::-webkit-scrollbar { width: 4px; }
.pz-list::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.18); border-radius: 4px; }
.pz-item {
  display: flex; align-items: center; gap: 8px; width: 100%; text-align: left;
  padding: 7px 6px; border-radius: 8px; cursor: pointer; font-family: inherit;
  border: 1px solid transparent; background: transparent; color: rgba(255, 255, 255, 0.62);
  transition: background 0.16s, color 0.16s;
}
.pz-item:hover { background: rgba(255, 255, 255, 0.07); color: #fff; }
.pz-item.active {
  background: color-mix(in srgb, var(--accent, #7dd3fc) 14%, transparent);
  border-color: color-mix(in srgb, var(--accent, #7dd3fc) 34%, transparent);
  color: #fff;
}
.pz-idx { flex: 0 0 auto; font-size: 8px; letter-spacing: 0.08em; color: rgba(255, 255, 255, 0.26); font-variant-numeric: tabular-nums; }
.pz-item.active .pz-idx { color: var(--accent, #7dd3fc); }
.pz-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.pz-text strong { font-size: 11px; font-weight: 400; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pz-text em { font-style: normal; font-size: 9px; color: rgba(255, 255, 255, 0.32); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pz-verified {
  flex: 0 0 auto; width: 4px; height: 4px; border-radius: 50%;
  background: var(--accent, #7dd3fc); box-shadow: 0 0 7px var(--accent, #7dd3fc);
}
.pz-star {
  flex: 0 0 auto; width: 18px; text-align: center; font-size: 11px; line-height: 1;
  color: rgba(255, 255, 255, 0.2); transition: color 0.16s, transform 0.16s;
}
.pz-star:hover { color: rgba(255, 255, 255, 0.7); transform: scale(1.2); }
.pz-star.on { color: #ffd166; }
.pz-empty { padding: 22px 6px; text-align: center; font-size: 10px; color: rgba(255, 255, 255, 0.3); line-height: 1.7; }

.pz-tools { display: flex; gap: 5px; flex: 0 0 auto; }
.pz-tools button {
  flex: 1; min-width: 0; padding: 6px 2px; border-radius: 8px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.12); background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.7); font-size: 12px; line-height: 1.2; transition: all 0.18s;
}
.pz-tools button:hover { background: rgba(255, 255, 255, 0.14); color: #fff; }
.pz-tools .pz-random { flex: 1.4; font-size: 10px; letter-spacing: 0.1em; }
.pz-tools .pz-auto { flex: 2; font-size: 9px; letter-spacing: 0.02em; white-space: nowrap; }
.pz-tools .pz-auto.on {
  color: var(--accent, #7dd3fc);
  border-color: color-mix(in srgb, var(--accent, #7dd3fc) 45%, transparent);
  background: color-mix(in srgb, var(--accent, #7dd3fc) 12%, transparent);
}

.pz-fx-row { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; }
.pz-fx-row button {
  flex: 1; display: flex; align-items: center; justify-content: space-between;
  padding: 6px 9px; border-radius: 8px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.12); background: rgba(255, 255, 255, 0.045);
  color: rgba(255, 255, 255, 0.6); font-size: 10px; transition: all 0.18s;
}
.pz-fx-row button:hover { background: rgba(255, 255, 255, 0.12); color: #fff; }
.pz-fx-row button.on {
  color: var(--accent, #7dd3fc);
  border-color: color-mix(in srgb, var(--accent, #7dd3fc) 45%, transparent);
  background: color-mix(in srgb, var(--accent, #7dd3fc) 12%, transparent);
}
.pz-fx-row button em { font-style: normal; font-size: 8px; letter-spacing: 0.14em; opacity: 0.7; }
.pz-fx-row small { flex: 0 0 auto; font-size: 8px; color: rgba(255, 255, 255, 0.26); }

.pz-fold { flex: 0 0 auto; border-top: 1px solid rgba(255, 255, 255, 0.08); padding-top: 8px; }
.pz-fold summary {
  display: flex; align-items: center; justify-content: space-between; gap: 6px;
  list-style: none; cursor: pointer; font-size: 10px; letter-spacing: 0.08em;
  color: rgba(255, 255, 255, 0.5);
}
.pz-fold summary::-webkit-details-marker { display: none; }
.pz-fold summary span { font-size: 8px; letter-spacing: 0; color: rgba(255, 255, 255, 0.24); }
.pz-fold summary:hover { color: #fff; }
.pz-sliders { display: flex; flex-direction: column; gap: 6px; padding: 10px 2px 2px; }
.pz-sliders label { display: flex; align-items: center; gap: 8px; font-size: 9px; color: rgba(255, 255, 255, 0.45); }
.pz-sliders label span { width: 26px; flex: 0 0 auto; }
.pz-sliders label em { width: 26px; text-align: right; font-style: normal; font-variant-numeric: tabular-nums; color: rgba(255, 255, 255, 0.3); }
.pz-sliders input[type='range'] {
  flex: 1; min-width: 0; height: 2px; -webkit-appearance: none; appearance: none;
  border-radius: 2px; background: rgba(255, 255, 255, 0.18); outline: none; cursor: pointer;
}
.pz-sliders input[type='range']::-webkit-slider-thumb {
  -webkit-appearance: none; width: 9px; height: 9px; border-radius: 50%;
  background: #fff; box-shadow: 0 0 8px var(--accent, #7dd3fc);
}
.pz-sliders input[type='range']::-moz-range-thumb {
  width: 9px; height: 9px; border: none; border-radius: 50%; background: #fff;
}

.pz-lab-note { margin: 8px 0; font-size: 9px; line-height: 1.7; color: rgba(255, 255, 255, 0.32); }
.pz-lab-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 5px; }
.pz-lab-grid button {
  display: flex; align-items: center; justify-content: center; gap: 5px;
  padding: 6px 2px; border-radius: 7px; cursor: pointer; font-family: inherit;
  border: 1px solid rgba(255, 255, 255, 0.1); background: rgba(255, 255, 255, 0.035);
  color: rgba(255, 255, 255, 0.52); font-size: 10px; transition: all 0.18s;
}
.pz-lab-grid button em { font-style: normal; font-size: 8px; opacity: 0.45; }
.pz-lab-grid button:hover { color: #fff; background: rgba(255, 255, 255, 0.1); }
.pz-lab-grid button.active { color: #fff; border-color: rgba(255, 255, 255, 0.4); background: rgba(255, 255, 255, 0.14); }
.pz-palettes { display: flex; align-items: center; gap: 7px; margin-top: 10px; }
.pz-palettes span { font-size: 8px; letter-spacing: 0.2em; color: rgba(255, 255, 255, 0.28); margin-right: 2px; }
.pz-palettes button {
  width: 15px; height: 15px; padding: 0; border-radius: 50%; cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.25);
  background: linear-gradient(135deg, var(--sw-a), var(--sw-b) 55%, var(--sw-c));
  transition: transform 0.18s;
}
.pz-palettes button:hover { transform: scale(1.16); }
.pz-palettes button.active { transform: scale(1.12); box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.35); }
.pz-back {
  width: 100%; margin-top: 10px; padding: 6px; border-radius: 8px; cursor: pointer;
  font-family: inherit; font-size: 10px; letter-spacing: 0.06em;
  border: 1px solid color-mix(in srgb, var(--accent, #7dd3fc) 38%, transparent);
  background: color-mix(in srgb, var(--accent, #7dd3fc) 12%, transparent);
  color: var(--accent, #7dd3fc);
}
.pz-back:hover { background: color-mix(in srgb, var(--accent, #7dd3fc) 22%, transparent); }

.pz-foot {
  display: flex; align-items: center; justify-content: center; gap: 5px;
  flex: 0 0 auto; padding-top: 4px; font-size: 8px;
  color: rgba(255, 255, 255, 0.24); letter-spacing: 0.04em;
}
.pz-foot a { color: rgba(255, 255, 255, 0.42); text-decoration: none; border-bottom: 1px solid rgba(255, 255, 255, 0.16); }
.pz-foot a:hover { color: #fff; }
</style>
