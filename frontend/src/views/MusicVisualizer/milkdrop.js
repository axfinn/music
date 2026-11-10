// MilkDrop 引擎 —— 直接跑开源项目 Butterchurn(jberg/butterchurn,MIT)。
// 它把 Winamp MilkDrop 的 2D/3D 预设翻译成 WebGL,自带几百个社区预设,
// 这是"极致炫酷"最省力也最靠谱的一条路(自己写 shader 很难追上这些预设的密度)。
//
// 和自研 three.js 场景的分工:
//   自研(engine.js + scenes.js)= 简约、克制、和页面 UI 同一套视觉语言;
//   MilkDrop(milkdrop.js)      = 迷幻、高密度、纯正的老牌可视化味道。
// 两套互斥播放,同一时间只有一个在渲染,切换时停另一个,省 GPU。
//
// 两个坑:
//   1. 预设必须 `await import()` 懒加载 —— butterchurn + 预设包接近 400KB,
//      不能进首屏 chunk;
//   2. 预设名是社区字符串,版本升级可能改名,所以 CURATED 里存全名,
//      匹配不到就按关键词模糊兜底,再不行才随机。

// 精选预设 —— 不是随手挑的名字:这 6 个是跑了一遍全量体检(100 个预设逐个渲染 +
// 采样亮度)之后人工看画面留下的,要求是"有内容、不过曝、不像坏掉"。
// 换预设请先跑一次体检测亮度,别只看名字好听。
export const CURATED_MILK = [
  { key: 'Eo.S. + Zylot - skylight (Stained Glass Majesty mix)', label: '彩窗', hint: '彩绘玻璃,冷光很干净' },
  { key: 'flexi + fishbrain - neon mindblob grafitti', label: '霓虹', hint: '霓虹 blob,饱和度拉满' },
  { key: 'suksma - vector exp 1 - couldn\u2032t not', label: '迷幻', hint: '经典迷幻色块,密度高' },
  { key: 'Unchained - Rewop', label: '线框', hint: '紫色线框结构,最"设计感"' },
  { key: 'cope + martin - mother-of-pearl', label: '珠光', hint: '珠光缎面,慢速流动' },
  { key: 'Flexi + Martin - cascading decay swing', label: '流淌', hint: '彩色颜料往下淌' },
]

// 随机池:体检里"有画面、不过曝"的预设。随机只从这里抽,
// 免得转到 shader 编译失败的黑屏预设(100 个里大约 1/5 在弱 GPU 上是黑的)。
export const RANDOM_POOL = [
  'Eo.S. + Zylot - skylight (Stained Glass Majesty mix)',
  'Eo.S. + Phat - cubetrace - v2',
  'flexi - what is the matrix',
  'flexi + fishbrain - neon mindblob grafitti',
  'suksma - vector exp 1 - couldn\u2032t not',
  'Unchained - Rewop',
  'Unchained - Unified Drag 2',
  'cope + martin - mother-of-pearl',
  'Flexi + Martin - cascading decay swing',
  'Flexi - infused with the spiral',
  'Flexi, fishbrain, Geiss + Martin - tokamak witchery',
  'Geiss - Thumb Drum',
  'Geiss - Reaction Diffusion 2',
  'Aderrasi - Songflower (Moss Posy)',
  'martin - fruit machine',
  'martin - mandelbox explorer - high speed demo version',
  'TonyMilkdrop - Magellan\'s Nebula [Flexi - you enter first + multiverse]',
  'Phat+fiShbRaiN+Eo.S_Mandala_Chasers_remix',
  'Rovastar - Oozing Resistance',
  'Zylot - Star Ornament',
  'Goody - The Wild Vort',
  'fiShbRaiN + Flexi - witchcraft 2.0',
]

let modulePromise = null

// 懒加载 + 单例:第二次进入直接复用已经解析好的模块
function loadModules() {
  if (!modulePromise) {
    modulePromise = Promise.all([
      import('butterchurn'),
      import('butterchurn-presets'),
    ]).then(([bc, presetMod]) => {
      const butterchurn = bc.default || bc
      const presets = presetMod.default || presetMod
      const all = presets.getPresets()
      return { butterchurn, all, keys: Object.keys(all) }
    })
  }
  return modulePromise
}

function resolveKey(keys, key) {
  if (!key) return keys[0]
  if (keys.includes(key)) return key
  // 精确名对不上(版本改名/空格差异)→ 用前两段关键词模糊匹配
  const tokens = key.toLowerCase().split(/[\s,+]+/).filter(t => t.length > 3).slice(0, 3)
  const hit = keys.find(k => {
    const low = k.toLowerCase()
    return tokens.every(t => low.includes(t))
  })
  return hit || keys[0]
}

function supported() {
  try {
    const canvas = document.createElement('canvas')
    const gl = canvas.getContext('webgl2') || canvas.getContext('webgl')
    return !!gl
  } catch {
    return false
  }
}

/**
 * 创建 MilkDrop 渲染器。首次调用会下载约 400KB 的 chunk。
 * 返回 null 表示环境不支持(无 WebGL / 加载失败),调用方应回退到自研场景。
 */
export async function createMilkdropEngine({ container, getAudioContext, getAnalyser, isMobile = false }) {
  if (!supported()) return null
  let mod
  try {
    mod = await loadModules()
  } catch {
    return null
  }
  const { butterchurn, all, keys } = mod
  if (!keys.length) return null

  const canvas = document.createElement('canvas')
  canvas.className = 'viz-canvas viz-canvas-milk'
  container.appendChild(canvas)

  // ⚠️ butterchurn 的 gl.viewport 是按传进去的 width/height 设的,内部**不会**再乘
  // pixelRatio(它只拿 pixelRatio 算纹理 texsize)。所以如果按"CSS 尺寸 + 高 DPR 缓冲"
  // 传参,在 DPR>1 的屏幕(macBook 视网膜屏、所有手机)上画面只会铺满画布左下角,
  // 右边和上面留一条 L 形黑边。正确做法:直接把设备像素当尺寸传,pixelRatio 恒为 1。
  function measure() {
    const w = Math.max(1, container.clientWidth)
    const h = Math.max(1, container.clientHeight)
    const ratio = Math.min(window.devicePixelRatio || 1, 1.5) // 拖到另一块屏时 DPR 会变
    return { bw: Math.round(w * ratio), bh: Math.round(h * ratio) }
  }
  const { bw, bh } = measure()
  canvas.width = bw
  canvas.height = bh

  let viz = null
  try {
    const ctx = getAudioContext()
    if (!ctx) throw new Error('no audio context')
    viz = butterchurn.createVisualizer(ctx, canvas, {
      width: bw,
      height: bh,
      pixelRatio: 1,
    })
  } catch {
    canvas.remove()
    return null
  }

  let raf = 0
  let running = false
  let last = 0
  let currentKey = ''
  let warned = false

  function connectAudio() {
    const analyser = getAnalyser()
    if (analyser && viz) {
      try { viz.connectAudio(analyser) } catch {}
    }
  }
  connectAudio()

  function loadPreset(key, blend = 1.2) {
    const realKey = resolveKey(keys, key)
    const preset = all[realKey]
    if (!preset) return ''
    try {
      viz.loadPreset(preset, blend)
      currentKey = realKey
      return realKey
    } catch {
      return ''
    }
  }

  function frame(now) {
    if (!running) return
    raf = requestAnimationFrame(frame)
    last = now
    try {
      // 不要传 elapsedTime:butterchurn 内部要自己累计 lastTime,
      // 传了就不会更新,长时间播放会和真实时间脱节
      viz.render()
    } catch (err) {
      // 个别预设的 shader 在移动 GPU 上会编译失败:只喊一次,别刷屏
      if (!warned) {
        warned = true
        console.warn('[milkdrop] render failed:', err?.message || err)
      }
    }
  }

  // 开发环境探针:用来批量筛"在这台机器上真的渲染得出来"的预设。
  // 生产构建时 import.meta.env.DEV 为 false,整段会被摇掉。
  function sample() {
    try {
      viz.render()
      const gl = canvas.getContext('webgl2') || canvas.getContext('webgl')
      const px = new Uint8Array(4 * 64)
      gl.readPixels(0, 0, 8, 8, gl.RGBA, gl.UNSIGNED_BYTE, px)
      let sum = 0
      for (let i = 0; i < px.length; i += 4) sum += px[i] + px[i + 1] + px[i + 2]
      return sum / (64 * 3)
    } catch {
      return -1
    }
  }
  if (import.meta.env.DEV) {
    window.__milk = {
      keys,
      sample,
      loadPreset,
      loadIndex: (i, blend = 0) => loadPreset(keys[i], blend),
      // readPixels 必须在同一个任务里紧跟 render —— 默认缓冲合成后就读不到了
      render: () => { try { viz.render() } catch {} },
    }
  }

  return {
    canvas,
    get currentKey() { return currentKey },
    get presetKeys() { return keys },
    loadPreset,
    randomPreset() {
      // 只从体检过的池子里抽(100 个里约 1/5 在弱 GPU 上是黑屏的);
      // 同时排除当前这个 —— 否则 1/22 的概率会"点了没变化",看着像按钮失灵
      const pool = RANDOM_POOL.filter(k => keys.includes(k) && k !== currentKey)
      const list = pool.length ? pool : keys.filter(k => k !== currentKey)
      const key = list[Math.floor(Math.random() * list.length)]
      return loadPreset(key || keys[0], 2.2)
    },
    show() { canvas.style.display = 'block' },
    hide() { canvas.style.display = 'none' },
    start() {
      if (running) return
      connectAudio()
      running = true
      last = 0
      raf = requestAnimationFrame(frame)
    },
    stop() {
      running = false
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    },
    resize() {
      // 尺寸同样按设备像素传(见上面 createVisualizer 处的说明)
      const { bw: w, bh: h } = measure()
      canvas.width = w
      canvas.height = h
      try {
        viz.setRendererSize(w, h, { pixelRatio: 1 })
      } catch {}
    },
    dispose() {
      this.stop()
      try { viz.dispose?.() } catch {}
      viz = null
      canvas.remove()
    },
  }
}
