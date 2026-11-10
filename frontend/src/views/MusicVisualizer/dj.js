// DJ 打碟台 —— 真实插在音频链上的效果器,不是纯视觉。
//
// 链路(注册到 useMediaPlayer 的 fxChain 插槽后):
//   source → input → LOW → MID → HIGH → FILTER ─┬→ out(干声)
//                                               ├→ DELAY     → wet → out
//                                               ├→ REVERB    → wet → out
//                                               └→ CRUSH     → wet → out
//   out → analyser → destination
//
// 三个设计约束:
//   1. 所有参数变化都用 setTargetAtTime/linearRamp 平滑,直接赋值会「咔」一声;
//   2. 唯一能在浏览器里做的搓碟是改 playbackRate(没有时间拉伸,音调会跟着变),
//      所以搓碟时要关掉 preservesPitch —— 那正是黑胶的听感;
//   3. 鼓垫用 WebAudio 现场合成(kick/snare/hat/clap),不下载任何采样。

const EQ_BANDS = {
  low: { type: 'lowshelf', freq: 200 },
  mid: { type: 'peaking', freq: 1200, q: 0.9 },
  high: { type: 'highshelf', freq: 5000 },
}

const FX_AMOUNT = { delay: 0.62, reverb: 0.58, crush: 0.7 }

export function createDjEngine({ player }) {
  let ctx = null
  let built = false
  let registered = false
  let nodes = null
  let noiseBuf = null
  const fxOn = { delay: false, reverb: false, crush: false }
  let fxMix = 0.6

  function ensure() {
    if (built) return true
    ctx = player.getAudioContext?.()
    if (!ctx) return false

    const input = ctx.createGain()
    const out = ctx.createGain()
    // 串联三段 EQ:每条都常驻,参数为 0 时是纯直通(相位影响可忽略)
    const eq = {}
    let prev = input
    for (const [name, cfg] of Object.entries(EQ_BANDS)) {
      const f = ctx.createBiquadFilter()
      f.type = cfg.type
      f.frequency.value = cfg.freq
      if (cfg.q) f.Q.value = cfg.q
      f.gain.value = 0
      prev.connect(f)
      prev = f
      eq[name] = f
    }

    const filter = ctx.createBiquadFilter()
    filter.type = 'lowpass'
    filter.frequency.value = 20000
    filter.Q.value = 0.7
    prev.connect(filter)

    // ── 干声 ──
    filter.connect(out)

    // ── DELAY ──
    const delay = ctx.createDelay(1.2)
    delay.delayTime.value = 0.3
    const fb = ctx.createGain()
    fb.gain.value = 0.34
    const delayWet = ctx.createGain()
    delayWet.gain.value = 0
    const delayTone = ctx.createBiquadFilter() // 反馈里削一点高频,免得越叠越刺
    delayTone.type = 'lowpass'
    delayTone.frequency.value = 3200
    filter.connect(delay)
    delay.connect(delayTone)
    delayTone.connect(fb)
    fb.connect(delay)
    delay.connect(delayWet)
    delayWet.connect(out)

    // ── REVERB(合成脉冲响应,不加载任何音频文件)──
    const conv = ctx.createConvolver()
    conv.buffer = makeImpulse(ctx, 2.2, 2.6)
    const reverbWet = ctx.createGain()
    reverbWet.gain.value = 0
    filter.connect(conv)
    conv.connect(reverbWet)
    reverbWet.connect(out)

    // ── CRUSH(阶梯量化 + 软削)──
    const shaper = ctx.createWaveShaper()
    shaper.curve = makeCrushCurve(12)
    shaper.oversample = 'none'
    const crushWet = ctx.createGain()
    crushWet.gain.value = 0
    filter.connect(shaper)
    shaper.connect(crushWet)
    crushWet.connect(out)

    const drumGain = ctx.createGain()
    drumGain.gain.value = 0.85
    drumGain.connect(input) // 鼓点也走 EQ/滤波/FX,才像同一个台子

    nodes = { input, out, eq, filter, delay, delayWet, conv, reverbWet, shaper, crushWet, drumGain }
    built = true
    return true
  }

  function attach() {
    if (!ensure()) return false
    if (registered) return true
    registered = !!player.registerFxChain?.({ input: nodes.input, output: nodes.out })
    return registered
  }

  function detach() {
    reset()
    if (registered) player.unregisterFxChain?.()
    registered = false
  }

  // ── 参数 ──
  function setEq(band, db) {
    if (!built || !nodes.eq[band]) return
    const v = Math.max(-26, Math.min(9, Number(db) || 0))
    nodes.eq[band].gain.setTargetAtTime(v, ctx.currentTime, 0.02)
  }

  /**
   * 滤波旋钮:-1 低通(闷)… 0 全开 … +1 高通(薄)
   * 用指数映射,耳朵听到的是均匀的扫频,而不是前半圈没反应。
   */
  function setFilter(v) {
    if (!built) return
    const x = Math.max(-1, Math.min(1, Number(v) || 0))
    const f = nodes.filter
    const now = ctx.currentTime
    if (x <= 0) {
      f.type = 'lowpass'
      f.frequency.setTargetAtTime(20000 * Math.pow(0.01, -x), now, 0.03)
      f.Q.setTargetAtTime(0.7 + -x * 6, now, 0.03)
    } else {
      f.type = 'highpass'
      f.frequency.setTargetAtTime(20 * Math.pow(1000, x), now, 0.03)
      f.Q.setTargetAtTime(0.7 + x * 6, now, 0.03)
    }
  }

  function wetFor(name) {
    return fxOn[name] ? fxMix * FX_AMOUNT[name] : 0
  }

  function applyFx() {
    if (!built) return
    const now = ctx.currentTime
    nodes.delayWet.gain.setTargetAtTime(wetFor('delay'), now, 0.05)
    nodes.reverbWet.gain.setTargetAtTime(wetFor('reverb'), now, 0.05)
    nodes.crushWet.gain.setTargetAtTime(wetFor('crush'), now, 0.05)
  }

  function setFx(name, on) {
    if (!(name in fxOn)) return
    fxOn[name] = !!on
    applyFx()
  }

  function setFxMix(v) {
    fxMix = Math.max(0, Math.min(1, Number(v) || 0))
    applyFx()
  }

  // ── 搓碟 ──
  let scratching = false
  function setRate(r) {
    const el = player.getAudioEl?.()
    if (!el) return
    if (r == null) {
      if (scratching) {
        el.preservesPitch = true
        if ('webkitPreservesPitch' in el) el.webkitPreservesPitch = true
        scratching = false
      }
      if (el.playbackRate !== 1) el.playbackRate = 1
      return
    }
    if (!scratching) {
      // 关掉音高保持:变速时音调跟着变,这才是黑胶被搓的听感
      el.preservesPitch = false
      if ('webkitPreservesPitch' in el) el.webkitPreservesPitch = false
      scratching = true
    }
    const rate = Math.max(0.15, Math.min(2.6, r))
    if (Math.abs(el.playbackRate - rate) > 0.02) el.playbackRate = rate
  }

  // ── 鼓垫(现场合成) ──
  function noiseSource(t, dur) {
    const src = ctx.createBufferSource()
    src.buffer = noiseBuf
    src.loop = true
    src.start(t)
    src.stop(t + dur)
    return src
  }

  function ensureNoise() {
    if (noiseBuf) return
    const n = Math.floor(ctx.sampleRate * 0.6)
    noiseBuf = ctx.createBuffer(1, n, ctx.sampleRate)
    const d = noiseBuf.getChannelData(0)
    for (let i = 0; i < n; i++) d[i] = Math.random() * 2 - 1
  }

  function trigger(pad) {
    if (!built) return false
    ensureNoise()
    const t = ctx.currentTime + 0.005
    const dest = nodes.drumGain

    if (pad === 'kick') {
      const o = ctx.createOscillator()
      const g = ctx.createGain()
      o.type = 'sine'
      o.frequency.setValueAtTime(165, t)
      o.frequency.exponentialRampToValueAtTime(44, t + 0.12)
      g.gain.setValueAtTime(0.0001, t)
      g.gain.exponentialRampToValueAtTime(0.95, t + 0.008)
      g.gain.exponentialRampToValueAtTime(0.0001, t + 0.38)
      o.connect(g).connect(dest)
      o.start(t)
      o.stop(t + 0.42)
      return true
    }

    if (pad === 'snare') {
      const src = noiseSource(t, 0.3)
      const bp = ctx.createBiquadFilter()
      bp.type = 'bandpass'
      bp.frequency.value = 1900
      bp.Q.value = 0.8
      const g = ctx.createGain()
      g.gain.setValueAtTime(0.55, t)
      g.gain.exponentialRampToValueAtTime(0.0001, t + 0.2)
      src.connect(bp).connect(g).connect(dest)
      // 加一点 190Hz 的鼓皮共鸣,单噪声太"纸"
      const o = ctx.createOscillator()
      const og = ctx.createGain()
      o.type = 'triangle'
      o.frequency.setValueAtTime(190, t)
      o.frequency.exponentialRampToValueAtTime(120, t + 0.09)
      og.gain.setValueAtTime(0.35, t)
      og.gain.exponentialRampToValueAtTime(0.0001, t + 0.1)
      o.connect(og).connect(dest)
      o.start(t)
      o.stop(t + 0.12)
      return true
    }

    if (pad === 'hat') {
      const src = noiseSource(t, 0.12)
      const hp = ctx.createBiquadFilter()
      hp.type = 'highpass'
      hp.frequency.value = 7200
      const g = ctx.createGain()
      g.gain.setValueAtTime(0.34, t)
      g.gain.exponentialRampToValueAtTime(0.0001, t + 0.055)
      src.connect(hp).connect(g).connect(dest)
      return true
    }

    if (pad === 'clap') {
      // 三连击的短包络,是"掌声"的关键
      const src = noiseSource(t, 0.4)
      const bp = ctx.createBiquadFilter()
      bp.type = 'bandpass'
      bp.frequency.value = 1500
      bp.Q.value = 1.4
      const g = ctx.createGain()
      g.gain.setValueAtTime(0.0001, t)
      for (let i = 0; i < 3; i++) {
        const at = t + i * 0.011
        g.gain.setValueAtTime(0.5, at)
        g.gain.exponentialRampToValueAtTime(0.06, at + 0.01)
      }
      g.gain.setValueAtTime(0.42, t + 0.033)
      g.gain.exponentialRampToValueAtTime(0.0001, t + 0.26)
      src.connect(bp).connect(g).connect(dest)
      return true
    }

    return false
  }

  // 回到中性:离开页面/关闭台子时用,避免把 EQ 的染色带出去
  function reset() {
    if (!built) return
    setRate(null)
    for (const b of Object.keys(EQ_BANDS)) setEq(b, 0)
    setFilter(0)
    for (const k of Object.keys(fxOn)) setFx(k, false)
  }

  function dispose() {
    detach()
    if (!built) return
    const list = [nodes.input, nodes.out, nodes.filter, nodes.delay, nodes.delayWet,
      nodes.conv, nodes.reverbWet, nodes.shaper, nodes.crushWet, nodes.drumGain,
      ...Object.values(nodes.eq)]
    for (const n of list) {
      try { n.disconnect() } catch {}
    }
    built = false
    nodes = null
    ctx = null
  }

  return {
    attach,
    detach,
    dispose,
    setEq,
    setFilter,
    setFx,
    setFxMix,
    setRate,
    trigger,
    reset,
    get ready() { return built },
  }
}

// 合成脉冲响应:指数衰减的噪声,2.2 秒足够做出"房间"感,又不会糊
function makeImpulse(ctx, seconds = 2.2, decay = 2.6) {
  const len = Math.max(1, Math.floor(ctx.sampleRate * seconds))
  const buf = ctx.createBuffer(2, len, ctx.sampleRate)
  for (let ch = 0; ch < 2; ch++) {
    const d = buf.getChannelData(ch)
    for (let i = 0; i < len; i++) {
      d[i] = (Math.random() * 2 - 1) * Math.pow(1 - i / len, decay)
    }
  }
  return buf
}

// 量化到 steps 档 → lo-fi 的"碎"
function makeCrushCurve(steps = 12) {
  const n = 2048
  const curve = new Float32Array(n)
  for (let i = 0; i < n; i++) {
    const x = (i / (n - 1)) * 2 - 1
    const q = Math.round(x * steps) / steps
    curve[i] = Math.tanh(q * 1.4) * 0.92
  }
  return curve
}
