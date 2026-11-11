// 开源可视化引擎 —— Butterchurn (jberg/butterchurn, MIT) + butterchurn-presets
// (Winamp MilkDrop 社区预设, 100 个)。这是本页**默认且首选**的画面来源。
//
// 为什么不再自己写 shader:
//   MilkDrop 预设是二十多年社区积累的方程/波形/形变, 密度与变化远超手搓的
//   three.js 场景。自研场景(scenes.js)只保留为「LAB 实验」与 WebGL2 不可用时的
//   回退, 不再和开源预设并列抢主位。
//
// 本文件只做三件事:
//   1. loadPresetLibrary()  —— 只读预设包, 让 UI 能在引擎下载完之前就列出全部预设;
//   2. createMilkdropEngine() —— 建 WebGL 画布并把 analyser 接进 Butterchurn;
//   3. 预设切换 / 生命周期管理(start/stop/resize/dispose)。
//
// 踩过的坑, 改之前别踩回去:
//   * 预设包 + 引擎近 400KB, 必须动态 import, 不能进首屏 chunk;
//   * butterchurn 的 gl.viewport 直接按传入 width/height 设, 内部不会乘 pixelRatio,
//     所以要把「设备像素」当尺寸传、pixelRatio 恒为 1, 否则 DPR>1 屏幕上画面只占
//     左下角, 留出 L 形黑边;
//   * render() 不要传 elapsedTime, 它内部自己累计 lastTime;
//   * 预设名是社区字符串, 版本升级会改名: 存全名 + 模糊兜底, 别按下标存;
//   * 一部分预设在弱 GPU 上 shader 编译失败(黑屏), 默认随机只从体检过的池子抽。

export const OPEN_SOURCE = {
  engine: 'Butterchurn',
  engineLicense: 'MIT',
  engineUrl: 'https://github.com/jberg/butterchurn',
  presets: 'MilkDrop 社区预设',
  presetsUrl: 'https://github.com/jberg/butterchurn-presets',
}

// 首屏默认预设。挑它的理由很具体: Butterchurn 靠 analyser 驱动, 没播放时
// 大部分预设几乎全黑(例如 skylight 静音时只剩几根暗线), 而 Rewop 的几何线框
// 不依赖音频也有完整画面 —— 用户刚进页面还没放歌时不能像坏了。
export const DEFAULT_PRESET = 'Unchained - Rewop'

// 精选预设 —— 跑过一遍全量体检(逐个渲染 + 采样亮度)之后人工看画面留下的,
// 标准是「有内容、不过曝、不像坏掉」。换预设请先体检测亮度, 别只看名字好听。
// 顺序同时用于旧版 milk:序号 的迁移, 不要随意重排。
export const CURATED_MILK = [
  { key: 'Eo.S. + Zylot - skylight (Stained Glass Majesty mix)', label: '彩窗', hint: '彩绘玻璃,冷光很干净' },
  { key: 'flexi + fishbrain - neon mindblob grafitti', label: '霓虹', hint: '霓虹 blob,饱和度拉满' },
  { key: 'suksma - vector exp 1 - couldn\u2032t not', label: '迷幻', hint: '经典迷幻色块,密度高' },
  { key: 'Unchained - Rewop', label: '线框', hint: '紫色线框结构,最"设计感"' },
  { key: 'cope + martin - mother-of-pearl', label: '珠光', hint: '珠光缎面,慢速流动' },
  { key: 'Flexi + Martin - cascading decay swing', label: '流淌', hint: '彩色颜料往下淌' },
]

// 随机池: 体检里「有画面、不过曝」的预设。随机只从这里抽,
// 免得转到 shader 编译失败的黑屏预设(全量里大约 1/5 在弱 GPU 上是黑的)。
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

// ── 预设名解析 ────────────────────────────────────────────────
// 社区命名大致是 "作者A + 作者B - 作品名", 也有一批是 "_作者 - 作品" 或纯标题。
// 拆成作者/标题两栏后, 列表才读得下去, 也才能按作者搜索。
export function parsePresetName(key) {
  const s = String(key).replace(/^_+/, '').trim()
  const i = s.indexOf(' - ')
  if (i > 0 && i < 70) {
    return {
      author: s.slice(0, i).replace(/\s*[_+]\s*/g, ' + ').trim() || 'Community',
      title: s.slice(i + 3).trim() || s,
    }
  }
  return { author: 'Community', title: s }
}

export function resolveKey(keys, key) {
  if (!key || !keys.length) return ''
  if (keys.includes(key)) return key
  // 精确名对不上(版本改名/空格差异)→ 用前几段关键词模糊匹配
  const tokens = key.toLowerCase().split(/[\s,+_]+/).filter(t => t.length > 3).slice(0, 3)
  if (tokens.length) {
    const hit = keys.find(k => {
      const low = k.toLowerCase()
      return tokens.every(t => low.includes(t))
    })
    if (hit) return hit
  }
  return ''
}

let libraryPromise = null

/** 只加载预设包。返回 { presets, keys, entries, verifiedKeys, curatedKeys }。 */
export function loadPresetLibrary() {
  if (!libraryPromise) {
    libraryPromise = import('butterchurn-presets')
      .then(mod => {
        const api = mod.default || mod
        const presets = api.getPresets ? api.getPresets() : api
        const keys = Object.keys(presets)
        const curatedKeys = CURATED_MILK.map(p => resolveKey(keys, p.key)).filter(Boolean)
        const verifiedKeys = RANDOM_POOL.map(k => resolveKey(keys, k)).filter(Boolean)
        const curatedSet = new Set(curatedKeys)
        const verifiedSet = new Set(verifiedKeys)
        const entries = keys.map(key => ({
          key,
          ...parsePresetName(key),
          curated: curatedSet.has(key),
          verified: verifiedSet.has(key),
        }))
        return { presets, keys, entries, verifiedKeys, curatedKeys }
      })
      .catch(err => {
        // 允许下一次进入重试, 别把失败缓存成永久状态
        libraryPromise = null
        throw err
      })
  }
  return libraryPromise
}

let butterchurnPromise = null
function loadButterchurn() {
  if (!butterchurnPromise) {
    butterchurnPromise = import('butterchurn')
      .then(mod => mod.default || mod)
      .catch(err => {
        butterchurnPromise = null
        throw err
      })
  }
  return butterchurnPromise
}

function supportsWebGL2() {
  try {
    const canvas = document.createElement('canvas')
    return !!canvas.getContext('webgl2')
  } catch {
    return false
  }
}

/**
 * 创建 MilkDrop 渲染器。首次调用会下载引擎 chunk(预设包可与 UI 并行预取)。
 * 返回 null 表示环境不支持(无 WebGL2 / 加载失败), 调用方应回退到 LAB 场景。
 */
export async function createMilkdropEngine({ container, getAudioContext, getAnalyser, isMobile = false }) {
  if (!supportsWebGL2()) return null

  let butterchurn, library
  try {
    ;[butterchurn, library] = await Promise.all([loadButterchurn(), loadPresetLibrary()])
  } catch {
    return null
  }
  const { presets, keys } = library
  if (!keys.length) return null

  const canvas = document.createElement('canvas')
  canvas.className = 'viz-canvas viz-canvas-milk'
  container.appendChild(canvas)

  // 见文件头: 尺寸按设备像素传, pixelRatio 恒为 1。
  function measure() {
    const w = Math.max(1, container.clientWidth)
    const h = Math.max(1, container.clientHeight)
    const cap = isMobile ? 1.25 : 1.5
    const ratio = Math.min(window.devicePixelRatio || 1, cap)
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
  let currentKey = ''
  let warned = false

  function connectAudio() {
    const analyser = getAnalyser()
    if (analyser && viz) {
      try { viz.connectAudio(analyser) } catch {}
    }
  }
  connectAudio()

  function loadPreset(key, blend = 1.6) {
    const realKey = resolveKey(keys, key)
    const preset = presets[realKey]
    if (!preset) return ''
    try {
      viz.loadPreset(preset, blend)
      currentKey = realKey
      warned = false
      return realKey
    } catch {
      return ''
    }
  }

  /** 从指定池子里随机抽一个(默认用体检过的池子), 排除当前这个。 */
  function randomPreset(pool) {
    const source = (pool?.length ? pool : library.verifiedKeys.length ? library.verifiedKeys : keys)
      .filter(k => keys.includes(k) && k !== currentKey)
    const list = source.length ? source : keys.filter(k => k !== currentKey)
    if (!list.length) return currentKey
    return loadPreset(list[Math.floor(Math.random() * list.length)], 2.2)
  }

  /** 按给定顺序上/下一个; 当前不在列表里就从第一项开始。 */
  function stepPreset(dir, pool) {
    const list = pool?.length ? pool : keys
    if (!list.length) return currentKey
    const i = list.indexOf(currentKey)
    const next = i < 0 ? 0 : (i + dir + list.length) % list.length
    return loadPreset(list[next], 1.4)
  }

  function frame() {
    if (!running) return
    raf = requestAnimationFrame(frame)
    try {
      // 不要传 elapsedTime: butterchurn 内部要自己累计 lastTime,
      // 传了就不会更新, 长时间播放会和真实时间脱节
      viz.render()
    } catch (err) {
      // 个别预设的 shader 在移动 GPU 上会编译失败: 只喊一次, 别刷屏
      if (!warned) {
        warned = true
        console.warn('[milkdrop] render failed:', err?.message || err)
      }
    }
  }

  // 开发环境探针: 用来批量筛「在这台机器上真的渲染得出来」的预设。
  // 生产构建时 import.meta.env.DEV 为 false, 整段会被摇掉。
  if (import.meta.env.DEV) {
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
    window.__milk = {
      keys,
      sample,
      loadPreset,
      loadIndex: (i, blend = 0) => loadPreset(keys[i], blend),
      render: () => { try { viz.render() } catch {} },
    }
  }

  return {
    canvas,
    library,
    get currentKey() { return currentKey },
    get presetKeys() { return keys },
    loadPreset,
    randomPreset,
    stepPreset,
    show() { canvas.style.display = 'block' },
    hide() { canvas.style.display = 'none' },
    start() {
      if (running) return
      connectAudio()
      running = true
      raf = requestAnimationFrame(frame)
    },
    stop() {
      running = false
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    },
    resize() {
      const { bw: w, bh: h } = measure()
      canvas.width = w
      canvas.height = h
      try { viz.setRendererSize(w, h, { pixelRatio: 1 }) } catch {}
    },
    dispose() {
      this.stop()
      try { viz.dispose?.() } catch {}
      viz = null
      canvas.remove()
    },
  }
}
