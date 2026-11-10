<template>
  <div class="dj-console" @pointerdown="$emit('bump')">
    <header class="dj-head">
      <div class="dj-brand">
        <span class="dj-dot" :class="{ on: isPlaying }" />
        <strong>DJ CONSOLE</strong>
        <em>效果器真实作用于音频 · 键盘勿冲突</em>
      </div>
      <div class="dj-actions">
        <button class="dj-play" type="button" :title="isPlaying ? '暂停' : '播放'" @click="$emit('toggle')">
          <svg v-if="isPlaying" viewBox="0 0 24 24" width="15" height="15" fill="currentColor">
            <path d="M8 5h3v14H8zM13 5h3v14h-3z" />
          </svg>
          <svg v-else viewBox="0 0 24 24" width="15" height="15" fill="currentColor">
            <path d="M8 4l13 8-13 8z" />
          </svg>
        </button>
        <button class="dj-close" type="button" @click="$emit('close')">收起</button>
      </div>
    </header>

    <div class="dj-grid">
      <!-- 唱盘 -->
      <div class="dj-deck">
        <div
          ref="platterEl"
          class="platter"
          :class="{ grabbing: dragging }"
          @pointerdown="onPlatterDown"
          @pointermove="onPlatterMove"
          @pointerup="onPlatterUp"
          @pointercancel="onPlatterUp"
        >
          <div ref="rotEl" class="platter-rot">
            <div class="platter-grooves" />
            <div class="platter-label">
              <span>{{ trackInitial }}</span>
              <em>33⅓</em>
            </div>
          </div>
          <div class="platter-arm" />
          <div class="platter-hub" />
        </div>
        <p class="deck-hint">{{ dragging ? `搓盘中 · ${rate.toFixed(2)}×` : '按住转盘左右拖 = 搓碟' }}</p>
      </div>

      <!-- 旋钮 -->
      <div class="dj-knobs">
        <div class="knob-row">
          <div
            v-for="k in eqKnobs" :key="k.band"
            class="knob" :class="{ killed: eq[k.band] <= -25 }"
            @pointerdown="startKnob($event, 'eq', k.band)"
            @dblclick="resetEq(k.band)"
          >
            <svg viewBox="0 0 60 60">
              <circle class="k-track" cx="30" cy="30" r="24" />
              <circle
                class="k-arc" cx="30" cy="30" r="24"
                :stroke-dasharray="`${arcLen * eqPos(k.band)} 999`"
              />
              <line class="k-needle" x1="30" y1="11" x2="30" y2="30" :transform="`rotate(${needleDeg(eqPos(k.band))} 30 30)`" />
            </svg>
            <span class="k-name">{{ k.label }}</span>
            <span class="k-val">{{ eqText(k.band) }}</span>
          </div>
        </div>

        <div class="knob-row lower">
          <div
            class="knob wide"
            @pointerdown="startKnob($event, 'filter')"
            @dblclick="resetFilter"
          >
            <svg viewBox="0 0 60 60">
              <circle class="k-track" cx="30" cy="30" r="24" />
              <circle class="k-arc" cx="30" cy="30" r="24" :stroke-dasharray="`${arcLen * filterPos} 999`" />
              <line class="k-needle" x1="30" y1="11" x2="30" y2="30" :transform="`rotate(${needleDeg(filterPos)} 30 30)`" />
            </svg>
            <span class="k-name">FILTER</span>
            <span class="k-val">{{ filterText }}</span>
          </div>

          <button class="tap" type="button" @click="tapTempo">
            <strong>{{ bpm || '—' }}</strong>
            <em>TAP BPM</em>
          </button>
        </div>
      </div>

      <!-- FX + 鼓垫 -->
      <div class="dj-fx">
        <div class="fx-pads">
          <button
            v-for="f in fxList" :key="f.id"
            type="button" class="fx-pad" :class="{ on: fx[f.id] }"
            @click="toggleFx(f.id)"
          >{{ f.label }}</button>
        </div>
        <label class="fx-mix">
          <span>MIX</span>
          <input v-model.number="fxMix" type="range" min="0" max="1" step="0.01" @input="onMix" />
          <em>{{ Math.round(fxMix * 100) }}</em>
        </label>

        <div class="drum-pads">
          <button
            v-for="p in drumPads" :key="p.id"
            type="button" class="drum-pad" :class="{ hit: hitPad === p.id }"
            @pointerdown="hit(p.id)"
          >
            <span>{{ p.label }}</span>
            <em>{{ p.hint }}</em>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'

const props = defineProps({
  dj: { type: Object, default: null },
  isPlaying: Boolean,
  trackTitle: { type: String, default: '' },
})
defineEmits(['close', 'bump'])

const EQ_MIN = -26
const EQ_MAX = 9
const arcLen = 113.1 // 270° 弧长(半径 24)

const eq = reactive({ low: 0, mid: 0, high: 0 })
const filterV = ref(0)
const fxMix = ref(0.6)
const fx = reactive({ delay: false, reverb: false, crush: false })
const bpm = ref(0)
const rate = ref(1)
const dragging = ref(false)
const hitPad = ref('')

const eqKnobs = [
  { band: 'low', label: 'LOW' },
  { band: 'mid', label: 'MID' },
  { band: 'high', label: 'HIGH' },
]
const fxList = [
  { id: 'delay', label: 'DELAY' },
  { id: 'reverb', label: 'REVERB' },
  { id: 'crush', label: 'CRUSH' },
]
const drumPads = [
  { id: 'kick', label: 'KICK', hint: 'Q' },
  { id: 'snare', label: 'SNARE', hint: 'W' },
  { id: 'hat', label: 'HAT', hint: 'E' },
  { id: 'clap', label: 'CLAP', hint: 'R' },
]

const trackInitial = computed(() => (props.trackTitle || '?').trim().slice(0, 1).toUpperCase())

// ── 旋钮位置/文案 ──
const eqPos = band => (eq[band] - EQ_MIN) / (EQ_MAX - EQ_MIN)
const filterPos = computed(() => (filterV.value + 1) / 2)
const needleDeg = p => -135 + p * 270

function eqText(band) {
  const v = eq[band]
  if (v <= -25) return 'KILL'
  return `${v > 0 ? '+' : ''}${v.toFixed(0)}`
}
const filterText = computed(() => {
  const v = filterV.value
  if (Math.abs(v) < 0.04) return 'OFF'
  return `${v < 0 ? 'LP' : 'HP'} ${Math.round(Math.abs(v) * 100)}`
})

// ── 旋钮拖拽(垂直拖动改值,双击归零) ──
let knobDrag = null
function startKnob(e, kind, band) {
  e.preventDefault()
  const startY = e.clientY
  const from = kind === 'eq' ? eq[band] : filterV.value
  const span = kind === 'eq' ? EQ_MAX - EQ_MIN : 2
  const perPx = span / 180 // 180px 拖满量程
  knobDrag = { kind, band, startY, from, perPx }
  window.addEventListener('pointermove', onKnobMove)
  window.addEventListener('pointerup', onKnobEnd)
}
function onKnobMove(e) {
  if (!knobDrag) return
  const d = (knobDrag.startY - e.clientY) * knobDrag.perPx
  if (knobDrag.kind === 'eq') {
    const v = Math.max(EQ_MIN, Math.min(EQ_MAX, knobDrag.from + d))
    eq[knobDrag.band] = v
    props.dj?.setEq(knobDrag.band, v)
  } else {
    const v = Math.max(-1, Math.min(1, knobDrag.from + d))
    filterV.value = v
    props.dj?.setFilter(v)
  }
}
function onKnobEnd() {
  knobDrag = null
  window.removeEventListener('pointermove', onKnobMove)
  window.removeEventListener('pointerup', onKnobEnd)
}
function resetEq(band) {
  eq[band] = 0
  props.dj?.setEq(band, 0)
}
function resetFilter() {
  filterV.value = 0
  props.dj?.setFilter(0)
}

// ── FX ──
function toggleFx(id) {
  fx[id] = !fx[id]
  props.dj?.setFx(id, fx[id])
}
function onMix() {
  props.dj?.setFxMix(fxMix.value)
}

// ── 鼓垫 ──
let hitTimer = 0
function hit(id) {
  props.dj?.trigger(id)
  hitPad.value = id
  clearTimeout(hitTimer)
  hitTimer = setTimeout(() => { hitPad.value = '' }, 130)
}

// ── TAP BPM ──
const taps = []
function tapTempo() {
  const now = performance.now()
  if (taps.length && now - taps[taps.length - 1] > 2000) taps.length = 0
  taps.push(now)
  if (taps.length > 6) taps.shift()
  if (taps.length >= 2) {
    const gaps = []
    for (let i = 1; i < taps.length; i++) gaps.push(taps[i] - taps[i - 1])
    gaps.sort((a, b) => a - b)
    bpm.value = Math.round(60000 / gaps[Math.floor(gaps.length / 2)])
  }
}

// ── 唱盘:自转 + 搓碟 ──
const platterEl = ref(null)
const rotEl = ref(null)
let spin = 0
let raf = 0
let lastT = 0
let drag = null

function angleAt(e) {
  const r = platterEl.value?.getBoundingClientRect()
  if (!r) return 0
  return (Math.atan2(e.clientY - (r.top + r.height / 2), e.clientX - (r.left + r.width / 2)) * 180) / Math.PI
}

function loop(t) {
  raf = requestAnimationFrame(loop)
  const dt = lastT ? Math.min(64, t - lastT) : 16
  lastT = t
  // 33⅓ rpm = 0.2 度/毫秒;搓碟时手指接管,不再自转
  if (!drag && props.isPlaying) spin = (spin + 0.2 * dt * rate.value) % 360
  if (rotEl.value) rotEl.value.style.transform = `rotate(${spin}deg)`
}

function onPlatterDown(e) {
  e.preventDefault()
  platterEl.value?.setPointerCapture?.(e.pointerId)
  dragging.value = true
  drag = { a: angleAt(e), t: performance.now(), vel: 0 }
}
function onPlatterMove(e) {
  if (!drag) return
  const a = angleAt(e)
  let d = a - drag.a
  if (d > 180) d -= 360
  if (d < -180) d += 360
  const now = performance.now()
  const dt = Math.max(1, now - drag.t)
  drag.vel = d / dt
  drag.a = a
  drag.t = now
  spin += d // 跟手
  const r = Math.max(0.2, Math.min(2.6, 1 + drag.vel * 0.9))
  rate.value = r
  props.dj?.setRate(r)
}
function onPlatterUp() {
  if (!drag) return
  drag = null
  dragging.value = false
  rate.value = 1
  props.dj?.setRate(null)
}

onMounted(() => {
  raf = requestAnimationFrame(loop)
  // 面板打开即挂上效果链(中性参数,听不出差别)
  props.dj?.attach?.()
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  clearTimeout(hitTimer)
  onKnobEnd()
  onPlatterUp()
})

// 键盘:Q/W/E/R 打鼓垫(仅在面板打开时由父级转发)
function playPad(id) { hit(id) }
defineExpose({ playPad })
</script>

<style scoped>
.dj-console {
  width: min(980px, calc(100vw - 32px));
  margin: 0 auto;
  padding: 14px 18px 18px;
  border-radius: 20px;
  background: rgba(14, 16, 22, 0.82);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.55), inset 0 1px 0 rgba(255, 255, 255, 0.06);
  backdrop-filter: blur(22px) saturate(1.15);
  color: #fff;
  user-select: none;
}

.dj-head {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 12px;
}
.dj-brand { display: flex; align-items: baseline; gap: 10px; }
.dj-brand strong { font-size: 11px; letter-spacing: 0.28em; font-weight: 600; }
.dj-brand em { font-size: 10px; font-style: normal; color: rgba(255, 255, 255, 0.32); letter-spacing: 0.04em; }
.dj-dot { width: 7px; height: 7px; border-radius: 50%; background: rgba(255, 255, 255, 0.2); align-self: center; }
.dj-dot.on { background: var(--accent, #62f3ff); box-shadow: 0 0 10px var(--accent, #62f3ff); }
.dj-actions { display: flex; align-items: center; gap: 8px; }
.dj-play {
  display: inline-flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; border-radius: 50%; cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.16); background: rgba(255, 255, 255, 0.07); color: #fff;
  transition: 0.16s;
}
.dj-play:hover { background: rgba(255, 255, 255, 0.16); }
.dj-close {
  border: 1px solid rgba(255, 255, 255, 0.14); background: transparent; color: rgba(255, 255, 255, 0.66);
  font-size: 11px; padding: 5px 12px; border-radius: 999px; cursor: pointer; letter-spacing: 0.06em;
  transition: 0.18s;
}
.dj-close:hover { color: #fff; border-color: rgba(255, 255, 255, 0.34); background: rgba(255, 255, 255, 0.06); }

.dj-grid { display: grid; grid-template-columns: 176px 1fr 244px; gap: 18px; align-items: center; }

/* ── 唱盘 ── */
.dj-deck { display: flex; flex-direction: column; align-items: center; gap: 8px; }
.platter {
  position: relative; width: 168px; height: 168px; border-radius: 50%;
  background: radial-gradient(circle at 34% 28%, #2a2d36, #0a0b0f 62%, #05060a);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08), 0 10px 30px rgba(0, 0, 0, 0.6);
  cursor: grab; touch-action: none;
}
.platter.grabbing { cursor: grabbing; box-shadow: inset 0 0 0 1px var(--accent, #62f3ff), 0 0 26px rgba(98, 243, 255, 0.28); }
.platter-rot { position: absolute; inset: 0; will-change: transform; }
.platter-grooves {
  position: absolute; inset: 8px; border-radius: 50%;
  background: repeating-radial-gradient(circle at 50% 50%, rgba(255, 255, 255, 0.055) 0 1px, transparent 1px 4px);
  opacity: 0.75;
}
.platter-label {
  position: absolute; inset: 52px; border-radius: 50%;
  display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 2px;
  background: radial-gradient(circle at 40% 32%, rgba(98, 243, 255, 0.22), rgba(20, 22, 30, 0.9));
  border: 1px solid rgba(255, 255, 255, 0.12);
}
.platter-label span { font-size: 25px; font-weight: 600; letter-spacing: 0.02em; }
.platter-label em { font-size: 8px; font-style: normal; letter-spacing: 0.22em; color: rgba(255, 255, 255, 0.42); }
.platter-arm {
  position: absolute; right: 4px; top: 16px; width: 3px; height: 62px; border-radius: 2px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.52), rgba(255, 255, 255, 0.1));
  transform-origin: top center; transform: rotate(14deg);
}
.platter-hub {
  position: absolute; left: 50%; top: 50%; width: 7px; height: 7px; margin: -3.5px 0 0 -3.5px;
  border-radius: 50%; background: #0c0d12; box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.25);
}
.deck-hint { margin: 0; font-size: 9px; letter-spacing: 0.08em; color: rgba(255, 255, 255, 0.34); }

/* ── 旋钮 ── */
.dj-knobs { display: flex; flex-direction: column; gap: 12px; }
.knob-row { display: flex; align-items: center; justify-content: space-around; gap: 10px; }
.knob {
  display: flex; flex-direction: column; align-items: center; gap: 2px;
  cursor: ns-resize; touch-action: none; width: 74px;
}
.knob svg { width: 62px; height: 62px; transform: rotate(-135deg); }
.k-track { fill: none; stroke: rgba(255, 255, 255, 0.1); stroke-width: 5; stroke-dasharray: 113.1 999; stroke-linecap: round; }
.k-arc { fill: none; stroke: var(--accent, #62f3ff); stroke-width: 5; stroke-linecap: round; transition: stroke-dasharray 0.05s linear; }
.k-needle { stroke: rgba(255, 255, 255, 0.9); stroke-width: 2.4; stroke-linecap: round; }
.knob.wide svg { width: 66px; height: 66px; }
.knob.killed .k-arc { stroke: #ff5470; }
.k-name { font-size: 9px; letter-spacing: 0.2em; color: rgba(255, 255, 255, 0.5); }
.k-val { font-size: 10px; font-variant-numeric: tabular-nums; color: rgba(255, 255, 255, 0.86); }

.tap {
  display: flex; flex-direction: column; align-items: center; gap: 2px;
  padding: 8px 14px; border-radius: 12px; cursor: pointer;
  background: rgba(255, 255, 255, 0.05); border: 1px solid rgba(255, 255, 255, 0.12); color: #fff;
  transition: 0.16s;
}
.tap:hover { background: rgba(255, 255, 255, 0.1); }
.tap:active { transform: scale(0.96); }
.tap strong { font-size: 17px; font-variant-numeric: tabular-nums; line-height: 1; }
.tap em { font-size: 8px; font-style: normal; letter-spacing: 0.16em; color: rgba(255, 255, 255, 0.42); }

/* ── FX ── */
.dj-fx { display: flex; flex-direction: column; gap: 10px; }
.fx-pads { display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; }
.fx-pad {
  padding: 9px 0; border-radius: 10px; cursor: pointer; font-size: 9.5px; letter-spacing: 0.14em;
  background: rgba(255, 255, 255, 0.045); border: 1px solid rgba(255, 255, 255, 0.11);
  color: rgba(255, 255, 255, 0.62); transition: 0.16s;
}
.fx-pad.on {
  color: #061018; background: var(--accent, #62f3ff); border-color: transparent;
  box-shadow: 0 0 18px rgba(98, 243, 255, 0.42);
}
.fx-mix { display: flex; align-items: center; gap: 8px; font-size: 9px; letter-spacing: 0.16em; color: rgba(255, 255, 255, 0.42); }
.fx-mix input { flex: 1; accent-color: var(--accent, #62f3ff); height: 3px; }
.fx-mix em { font-style: normal; font-variant-numeric: tabular-nums; color: rgba(255, 255, 255, 0.7); }

.drum-pads { display: grid; grid-template-columns: repeat(4, 1fr); gap: 7px; }
.drum-pad {
  display: flex; flex-direction: column; align-items: center; gap: 1px;
  padding: 11px 0 8px; border-radius: 10px; cursor: pointer; touch-action: none;
  background: rgba(255, 255, 255, 0.05); border: 1px solid rgba(255, 255, 255, 0.12);
  color: rgba(255, 255, 255, 0.78); transition: background 0.1s, transform 0.1s;
}
.drum-pad span { font-size: 9px; letter-spacing: 0.1em; }
.drum-pad em { font-size: 8px; font-style: normal; color: rgba(255, 255, 255, 0.3); }
.drum-pad.hit {
  background: rgba(98, 243, 255, 0.9); color: #05121a; transform: scale(0.95);
  border-color: transparent; box-shadow: 0 0 20px rgba(98, 243, 255, 0.55);
}
.drum-pad.hit em { color: rgba(5, 18, 26, 0.6); }

@media (max-width: 767px) {
  .dj-console { width: 100%; border-radius: 18px 18px 0 0; max-height: 64vh; overflow-y: auto; padding: 12px 14px 16px; }
  .dj-grid { grid-template-columns: 1fr; gap: 14px; }
  .dj-deck { flex-direction: row; gap: 14px; justify-content: center; }
  .platter { width: 118px; height: 118px; flex: none; }
  .platter-label { inset: 36px; }
  .platter-label span { font-size: 18px; }
  .deck-hint { display: none; }
  .dj-brand em { display: none; }
  .knob svg, .knob.wide svg { width: 54px; height: 54px; }
  .knob { width: 62px; }
}
</style>
