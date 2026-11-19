// 歌词特效「重力」—— 2D canvas 刚体物理层。
//
// 为什么自研而不用 matter.js / planck:
//   这里只需要圆-圆碰撞 + 重力 + 墙壁反弹 + 低频冲量 + 鼠标推开,逻辑不到 250 行。
//   引一个物理引擎要几十 KB 和一套刚体/约束抽象,对「一堆字往下掉」是杀鸡用牛刀。
//
// 与另外两类渲染器的约定一致:
//   · 同一时刻只跑一个 —— 页面在 ensureActiveRenderer 里互斥 start/stop;
//   · 独立 canvas(不占 WebGL context),stop() 停 rAF 并藏掉 canvas;
//   · 颜色跟随画面主色(setFrameColor)或站点配色(setPalette)。
//
// 性能:
//   · 字块数量按屏宽算,手机上更少;半径/质量按字号定,碰撞 O(n²) 但 n 很小。
//   · 固定步长积分(1/60),掉帧时最多补 3 步,避免切后台回来物体穿墙。
//   · 低频/拍点读 analyser 的 0~24 bin,和 lyricTextFx 一样的轻量采样。

const FONT_STACK = '"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", system-ui, sans-serif'
const GRAVITY = 1180          // px/s²
const AIR = 0.998             // 每步空气阻尼(速度衰减)
const REST = 0.62             // 碰撞恢复系数(弹性)
const WALL_REST = 0.58        // 墙壁恢复系数
const FLOOR_FRICTION = 0.86   // 触地横向摩擦
const FIXED_DT = 1 / 60

function hexToRgb(hex) {
  const s = String(hex || '#7dd3fc').replace('#', '')
  const n = s.length === 3 ? s.split('').map(c => c + c).join('') : s
  const v = parseInt(n, 16)
  return { r: (v >> 16) & 255, g: (v >> 8) & 255, b: v & 255 }
}

export function createPhysicsFx({ container, getAnalyser, isPlaying = () => false, isMobile = false }) {
  if (!container || typeof document === 'undefined') return null

  const canvas = document.createElement('canvas')
  canvas.className = 'viz-canvas-physics'
  canvas.style.display = 'none'
  container.appendChild(canvas)
  const ctx = canvas.getContext('2d')

  let cssW = 1
  let cssH = 1
  let dpr = 1

  // 颜色:三色调,字块按序取一个渐变色;跟随画面主色
  let colors = ['#7dd3fc', '#a5b4fc', '#c4b5fd']
  function setPalette(p) {
    if (!p) return
    const arr = typeof p === 'string' ? [p] : [p.a, p.b, p.c].filter(Boolean)
    if (arr.length) colors = arr
  }
  function setFrameColor(hex) {
    if (!hex) return
    const { r, g, b } = hexToRgb(hex)
    // 主色 + 亮一档 + 偏冷邻色,都在同系
    const mix = (t, target) => Math.round(r + (target - r) * t)
    colors = [
      `rgb(${r},${g},${b})`,
      `rgb(${mix(0.45, 255)},${mix(0.45, 255)},${mix(0.45, 255)})`,
      `rgb(${mix(0.3, 168)},${mix(0.3, 130)},${mix(0.3, 255)})`,
    ]
  }
  const colorAt = i => colors[i % colors.length]

  // ── 刚体 ──
  let bodies = []
  let line = ''
  let title = ''
  let fontSize = 48

  function makeBodies(text) {
    const chars = [...String(text || '')].filter(c => c.trim()).slice(0, isMobile ? 14 : 22)
    fontSize = isMobile
      ? Math.max(26, Math.min(46, cssW / 9))
      : Math.max(36, Math.min(74, cssW / 18))
    const r = fontSize * 0.52
    const prev = bodies.length
    bodies = chars.map((ch, i) => {
      // 复用上一批同下标字块的位置,切行不"啪"一下全清;没有就撒在画面上方
      const seedX = cssW * (0.2 + 0.6 * ((i + 0.5) / Math.max(1, chars.length)))
      const old = prev ? bodies[Math.min(i, prev - 1)] : null
      return {
        ch,
        x: old ? old.x : seedX + (Math.random() - 0.5) * 18,
        y: old ? Math.min(old.y, cssH * 0.18) : -r - Math.random() * cssH * 0.3,
        vx: old ? old.vx * 0.5 : (Math.random() - 0.5) * 60,
        vy: old ? Math.min(old.vy, 0) : 0,
        angle: old ? old.angle : (Math.random() - 0.5) * 0.5,
        va: (Math.random() - 0.5) * 1.2,
        r,
        m: 1,
        ci: i,
      }
    })
  }

  function setLine(text) {
    const next = String(text || '')
    if (next === line) return
    line = next
    // 有歌词用歌词;前奏/间奏退回歌名,画面不会空着
    makeBodies(next || title)
  }
  function setTitle(t) {
    const next = String(t || '')
    if (next === title) return
    title = next
    if (!line) makeBodies(title)
  }

  // ── 频谱 / 拍点 ──
  let bins = null
  let lastBass = -1
  let beatCooldown = 0
  let synthPhase = 0
  function readBass(dt) {
    const an = getAnalyser?.()
    let bass = -1
    if (an) {
      if (!bins || bins.length !== an.frequencyBinCount) bins = new Uint8Array(an.frequencyBinCount)
      an.getByteFrequencyData(bins)
      const end = Math.min(bins.length, 24)
      let sum = 0
      for (let i = 1; i < end; i++) sum += bins[i]
      bass = sum / Math.max(1, end - 1) / 255
      if (bass <= 0.004) bass = -1
    }
    if (bass < 0) {
      synthPhase += dt * (96 / 60)
      bass = isPlaying() ? 0.24 + Math.pow(1 - (synthPhase % 1), 2.6) * 0.5 : 0.08
    }
    return Math.min(1, bass)
  }

  // ── 指针推开 ──
  const pointer = { x: -9999, y: -9999, on: false }
  function setPointer(nx, ny) {
    pointer.x = (nx * 0.5 + 0.5) * cssW
    pointer.y = (-ny * 0.5 + 0.5) * cssH
    pointer.on = true
  }
  function clearPointer() { pointer.on = false; pointer.x = -9999; pointer.y = -9999 }

  // ── 积分 ──
  function step(dt, bass, beat) {
    const floorY = cssH - 8
    for (const b of bodies) {
      b.vy += GRAVITY * dt
      b.vx *= AIR
      b.vy *= AIR
      b.x += b.vx * dt
      b.y += b.vy * dt
      b.angle += b.va * dt

      // 拍点:给下半场的字块一个向上的冲量,像被低音"顶"起来
      if (beat > 0.6 && b.y > cssH * 0.45 && Math.random() < 0.22) {
        b.vy -= (140 + bass * 260) * beat
        b.va += (Math.random() - 0.5) * 3
      }

      // 鼠标推开:半径内的字块受一个反向冲量
      if (pointer.on) {
        const dx = b.x - pointer.x
        const dy = b.y - pointer.y
        const d2 = dx * dx + dy * dy
        const R = 110
        if (d2 < R * R && d2 > 0.01) {
          const d = Math.sqrt(d2)
          const f = (1 - d / R) * 900
          b.vx += (dx / d) * f * dt
          b.vy += (dy / d) * f * dt
          b.va += (dx / d) * f * dt * 0.01
        }
      }

      // 墙壁
      if (b.x - b.r < 0) { b.x = b.r; b.vx = Math.abs(b.vx) * WALL_REST }
      else if (b.x + b.r > cssW) { b.x = cssW - b.r; b.vx = -Math.abs(b.vx) * WALL_REST }
      if (b.y - b.r < 0) { b.y = b.r; b.vy = Math.abs(b.vy) * WALL_REST }
      // 地面
      if (b.y + b.r > floorY) {
        b.y = floorY - b.r
        b.vy = -Math.abs(b.vy) * WALL_REST
        b.vx *= FLOOR_FRICTION
        b.va *= FLOOR_FRICTION
        // 躺平:低速时把角度慢慢收回 0,免得字一直歪着
        if (Math.abs(b.vy) < 40) b.angle += (0 - b.angle) * Math.min(1, dt * 4)
      }
    }
    resolveCollisions()
  }

  // 圆-圆碰撞:位置分离 + 弹性冲量。n 小(≤22),O(n²) 足够
  function resolveCollisions() {
    for (let i = 0; i < bodies.length; i++) {
      const a = bodies[i]
      for (let j = i + 1; j < bodies.length; j++) {
        const b = bodies[j]
        let dx = b.x - a.x
        let dy = b.y - a.y
        const min = a.r + b.r
        let d2 = dx * dx + dy * dy
        if (d2 >= min * min) continue
        if (d2 < 0.0001) { dx = Math.random() - 0.5; dy = -1; d2 = dx * dx + dy * dy }
        const d = Math.sqrt(d2)
        const nx = dx / d
        const ny = dy / d
        // 位置分离(各退一半),避免叠在一起
        const overlap = min - d
        a.x -= nx * overlap * 0.5
        a.y -= ny * overlap * 0.5
        b.x += nx * overlap * 0.5
        b.y += ny * overlap * 0.5
        // 相对速度沿法线分量
        const rvx = b.vx - a.vx
        const rvy = b.vy - a.vy
        const vn = rvx * nx + rvy * ny
        if (vn > 0) continue       // 正在分开,不处理
        const imp = -(1 + REST) * vn / 2
        a.vx -= imp * nx
        a.vy -= imp * ny
        b.vx += imp * nx
        b.vy += imp * ny
        a.va -= imp * 0.004
        b.va += imp * 0.004
      }
    }
  }

  function draw() {
    ctx.clearRect(0, 0, cssW, cssH)
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.font = `700 ${fontSize}px ${FONT_STACK}`
    for (const b of bodies) {
      ctx.save()
      ctx.translate(b.x, b.y)
      ctx.rotate(b.angle)
      ctx.fillStyle = colorAt(b.ci)
      ctx.shadowColor = colorAt(b.ci)
      ctx.shadowBlur = 14
      ctx.fillText(b.ch, 0, 0)
      ctx.restore()
    }
  }

  // ── 主循环 ──
  let raf = 0
  let running = false
  let last = 0
  let acc = 0

  function frame(now) {
    if (!running) return
    raf = requestAnimationFrame(frame)
    const dt = last ? Math.min((now - last) / 1000, 0.05) : FIXED_DT
    last = now
    const bass = readBass(dt)

    beatCooldown = Math.max(0, beatCooldown - dt)
    let beat = 0
    if (beatCooldown === 0 && bass > 0.34 && isPlaying()) {
      beat = bass
      beatCooldown = 0.28
    }

    // 固定步长积分,最多补 3 步,防止掉帧/切后台回来穿墙
    acc += dt
    let n = 0
    while (acc >= FIXED_DT && n < 3) {
      step(FIXED_DT, bass, beat)
      acc -= FIXED_DT
      n++
    }
    if (n === 3) acc = 0

    draw()
  }

  function resize() {
    cssW = Math.max(1, container.clientWidth)
    cssH = Math.max(1, container.clientHeight)
    dpr = Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2)
    canvas.width = Math.round(cssW * dpr)
    canvas.height = Math.round(cssH * dpr)
    canvas.style.width = '100%'
    canvas.style.height = '100%'
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    // 尺寸变了字号要重算;位置钳回画面内
    if (bodies.length) {
      const fs = isMobile
        ? Math.max(26, Math.min(46, cssW / 9))
        : Math.max(36, Math.min(74, cssW / 18))
      const scale = fs / fontSize
      fontSize = fs
      for (const b of bodies) {
        b.r *= scale
        b.x = Math.max(b.r, Math.min(cssW - b.r, b.x))
        b.y = Math.max(b.r, Math.min(cssH - b.r, b.y))
      }
    }
  }

  function start() {
    if (running) return
    running = true
    canvas.style.display = ''
    last = 0
    acc = 0
    raf = requestAnimationFrame(frame)
  }
  function stop() {
    running = false
    if (raf) cancelAnimationFrame(raf)
    raf = 0
    canvas.style.display = 'none'
    clearPointer()
  }
  function dispose() {
    stop()
    if (canvas.parentNode === container) container.removeChild(canvas)
  }

  resize()
  // 初始:用歌名铺一批,进入页面就有东西往下掉(有歌词后 setLine 会替换)
  makeBodies('')

  return {
    start,
    stop,
    resize,
    setPointer,
    clearPointer,
    setPalette,
    setFrameColor,
    setLine,
    setTitle,
    dispose,
    ...(import.meta.env?.DEV ? {
      _debug: { get bodies() { return bodies }, get count() { return bodies.length } },
    } : {}),
  }
}
