// 3D 歌词/歌名字效层 —— 叠在沉浸播放画面之上的独立 three.js 层。
//
// 为什么单独一层,不塞进 scenes.js:
//   scenes 的场景会被 mountScene 整个换掉,而且 MilkDrop 模式干脆把 engine 停掉
//   (它用自己的 canvas)。字效要"换任何场景都在、切 MilkDrop 也在",
//   只能自己一块透明画布叠上去。
//
// 核心玩法(不是"歌词 + 背景歌名"两块静态字):
//   只有**一套粒子**。它们的 A 位置是当前这句歌词,B 位置是这首歌的歌名。
//   平时粒子待在歌词上、逐字随拍点跳动(按字序错峰,形成从左到右传的浪);
//   每到副歌/固定拍数,粒子按拍点**逐批起飞**,飞过去把这句歌词重组成歌名,
//   摆几拍再散回歌词。所以歌名是"歌词的字拼出来的",不是另画一层。
//
// 字还是点云:文字画进 canvas → 采样 alpha → 每像素一点。中文字体不用加载
// typeface JSON;天然能炸开/重组;一行字一次 draw call。
//
// 节奏来源:直接读 useMediaPlayer 的 AnalyserNode(只读,不影响 engine 的 drive),
// 没频谱时退化成 BPM 合成拍子,和 drive.js 同样的兜底思路。

import * as THREE from 'three'

const TAU = Math.PI * 2
const FONT_STACK = '"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", system-ui, sans-serif'
const FOV = 46

// 粒子预算。真正决定观感的是采样步长:步长大了笔画会碎成沙,
// 点数只是上限,没那么多像素时不会硬凑。
const CAP = 14000
const CAP_MOBILE = 6200

// 歌词块 / 歌名块在画面里的高度(世界单位,1 单位 = 一个字高)
const LYRIC_Y = 0.44
const TITLE_Y = 0.28

// 拼歌名的节拍编排
const STEPS_UP = 4        // 4 拍拼齐
const STEPS_DOWN = 2      // 2 拍散回歌词(歌词回来了要赶紧能看)
const HOLD_BEATS = 3      // 拼齐后摆 3 拍
const IDLE_BEATS = 10     // 散回后歇 10 拍再来一轮

const VERT = /* glsl */ `
  attribute vec3 aTitle;     // 歌名里的落点
  attribute vec3 aCenter;    // 歌词里这个字的中心(做逐字刚体运动用)
  attribute vec3 aCTitle;    // 歌名里这个字的中心
  attribute float aCharL;    // 在歌词里的顺序 0..1 → 浪从左往右传
  attribute float aCharT;    // 在歌名里的顺序
  attribute float aDelay;    // 起飞顺序 0..1 → 拼字时逐批出发
  attribute float aRand;
  attribute float aTint;
  attribute float aWL;       // 亮度权重:某侧点被复制了几份,就按份数摊薄
  attribute float aWT;

  uniform float uTime;
  uniform float uBeatTime;   // 上一次拍点的时间戳
  uniform float uBeat;       // 0→1 拍子冲量
  uniform float uMorph;      // 0 = 歌词,1 = 歌名
  uniform float uFresh;      // 0 = 刚换行(从四周聚拢),1 = 落位
  uniform float uSize;
  uniform float uScaleL;     // 两套字各自的缩放,点大小要跟着它走
  uniform float uScaleT;
  uniform float uPixelScale;
  uniform float uBass;
  uniform float uHigh;

  varying float vTint;
  varying float vGlow;

  void main() {
    vTint = aTint;

    // 每颗粒子的起飞窗口:错峰之后,"歌名"是被一片片凑出来的
    float w0 = aDelay * 0.58;
    float m = smoothstep(w0, w0 + 0.30, uMorph);

    vec3 p = mix(position, aTitle, m);
    vec3 c = mix(aCenter, aCTitle, m);
    float ch = mix(aCharL, aCharT, m);

    // 飞行弧:中段抬起来 + 侧向散开,才像"飞过去"而不是线性插值
    float arc = sin(m * 3.14159265);
    // 纵深拉开:飞行中的粒子分在前后不同深度,点大小不同,才不会糊成一团白
    p.y += arc * (0.14 + aRand * 0.55) + sin(arc * 3.4 + aRand * 6.28) * arc * 0.34;
    p.x += arc * (aRand - 0.5) * 0.5;
    p.z += arc * (fract(aRand * 7.31) - 0.5) * 2.2;

    // 换新一行歌词时,字从四周收拢落位(不是"啪"一下换掉)
    float pop = (1.0 - uFresh) * (1.0 - m);
    p += (vec3(fract(aRand * 13.1), fract(aRand * 7.7), fract(aRand * 3.3)) - 0.5) * pop * 3.4;

    vec3 local = p - c;

    // ── 每个字在拍点上跳一下 ──
    // 衰减振荡当弹簧:先往上窜,再回落,不会像逐拍阻尼脉冲那样抖成筛子。
    // 错峰 0.085s/字,一拍的浪从行首扫到行尾。
    float dtb = uTime - uBeatTime - ch * 0.085;
    float k = dtb > 0.0 ? exp(-dtb * 6.5) * sin(dtb * 12.0) : 0.0;
    float amp = k * (0.2 + uBeat * 0.62);

    // 刚体扰动:绕自己中心歪一下 + 纵向拉长 + 横向压窄,才是"跳"不是"整体平移"
    float flip = mix(-1.0, 1.0, step(0.5, fract(ch * 4.0 + aRand * 0.37)));
    float ang = amp * 0.20 * flip;
    float cs = cos(ang);
    float sn = sin(ang);
    local.xy = mat2(cs, -sn, sn, cs) * local.xy;
    local.y *= 1.0 + amp * 0.34;
    local.x *= 1.0 - amp * 0.14;
    local.y += amp * 0.62;

    // 常时呼吸:幅度跟低频走,静止画面也有颗粒在动
    float ph = aRand * 6.2831;
    local.x += sin(uTime * 1.2 + ph) * (0.004 + uBass * 0.012);
    local.y += cos(uTime * 0.95 + ph * 1.7) * (0.005 + uBass * 0.014);
    local.z += sin(uTime * 0.8 + ph * 0.6) * (0.01 + uBass * 0.03);

    vec3 pos = c + local;

    vec4 mv = modelViewMatrix * vec4(pos, 1.0);
    gl_Position = projectionMatrix * mv;

    float size = uSize * mix(uScaleL, uScaleT, m) * (0.7 + aRand * 0.85) * (1.0 + uBass * 0.42);
    // 飞行中收小:一堆点挤在航线上时会叠成一整块亮斑
    gl_PointSize = size * (1.0 - 0.45 * arc) * uPixelScale / max(0.6, -mv.z);

    // 亮度:高频闪烁;复制出来的点按份数摊薄,免得短句被叠成一团白
    float w = mix(aWL, aWT, m);
    vGlow = (0.52 + aRand * 0.8) * (0.72 + uHigh * 1.0) * w * (1.0 - 0.62 * arc);
  }
`

// GLSL 没有函数返回值简写,单独写一个 inline 表达式更省事
const VERT_FIX = VERT

const FRAG = /* glsl */ `
  uniform vec3 uA;
  uniform vec3 uB;
  uniform vec3 uC;
  uniform float uOpacity;

  varying float vTint;
  varying float vGlow;

  void main() {
    // 圆形柔光点:核心实、边缘化开
    float d = length(gl_PointCoord - 0.5);
    if (d > 0.5) discard;
    float a = smoothstep(0.5, 0.06, d);
    a = a * a * 0.8 + smoothstep(0.26, 0.0, d) * 0.65;

    vec3 col = mix(uA, uB, vTint);
    col = mix(col, uC, smoothstep(0.58, 1.0, vTint));

    // 霓虹灯管:点中心推向白,边缘留纯色 —— 从"点阵字"变成"通电的灯管"
    col = mix(col, vec3(1.0), smoothstep(0.34, 0.0, d) * 0.32);

    // additive 叠色:笔画密集处自己会过曝成亮芯,这就是没有 bloom 也有光的来源
    gl_FragColor = vec4(col * (0.38 + vGlow * 0.58), a * uOpacity * clamp(vGlow * 1.1, 0.0, 1.2));
  }
`

// ─────────────────────────────────────────────────────────────
// 文字 → 点云
// ─────────────────────────────────────────────────────────────
/**
 * 把一行字采样成点云。perLine > 0 且超长时自动折行 ——
 * 手机竖屏一行塞不下 12 个字,硬塞的话字高只剩 30px,糊成一条虚线。
 *
 * @returns {null | {pos, center, charT, tint, count, width, height}}
 */
function buildGlyph(raw, { fontSize = 132, gap = 3, tracking = 0.1, maxChars = 30, maxPoints = 4000, perLine = 0 } = {}) {
  const text = String(raw || '').replace(/\s+/g, ' ').trim()
  if (!text) return null

  const all = [...text].slice(0, maxChars)
  const fs = fontSize
  const track = fs * tracking

  const base = document.createElement('canvas').getContext('2d')
  base.font = `700 ${fs}px ${FONT_STACK}`

  // 折行。中文按字数均分(12 字 3 行 = 4/4/4,不会出现 5/5/2);
  // 带空格的(英文)必须按单词断,不然会出现 "Let's go part / y party all n" 这种断词。
  let lines
  if (perLine > 0 && all.length > perLine) {
    if (all.includes(' ')) {
      // 按空格切成"词 + 尾随空格",贪心塞行,超宽就换行
      const words = []
      let w = ''
      for (const ch of all) {
        w += ch
        if (ch === ' ') { words.push(w); w = '' }
      }
      if (w) words.push(w)
      lines = []
      let cur = ''
      for (const word of words) {
        if (cur && cur.length + word.length > perLine) {
          lines.push([...cur.replace(/\s+$/, '')])
          cur = word
        } else {
          cur += word
        }
      }
      if (cur.replace(/\s+$/, '')) lines.push([...cur.replace(/\s+$/, '')])
    } else {
      const rows = Math.ceil(all.length / perLine)
      const per = Math.ceil(all.length / rows)
      lines = []
      for (let i = 0; i < all.length; i += per) lines.push(all.slice(i, i + per))
    }
  } else {
    lines = [all]
  }

  const offsets = []
  let acc = 0
  for (const l of lines) {
    offsets.push(acc)
    acc += l.length
  }

  // 逐行量字宽 —— 折行会吃掉行尾空格,不能再用全局字数下标去查宽度
  const lw = lines.map(l => l.map(ch => base.measureText(ch).width))

  const lineW = li => {
    let w = 0
    for (let i = 0; i < lines[li].length; i++) w += lw[li][i] + track
    return Math.max(0, w - track)
  }

  const LH = fs * 1.28
  const padX = Math.ceil(fs * 0.16)
  const padY = Math.ceil(fs * 0.14)
  const maxLineW = Math.max(...lines.map((_, i) => lineW(i)))

  const cw = Math.min(5600, Math.ceil(maxLineW) + padX * 2)
  const ch = Math.ceil(LH * lines.length + padY * 2)

  const canvas = document.createElement('canvas')
  canvas.width = cw
  canvas.height = ch
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  ctx.font = `700 ${fs}px ${FONT_STACK}`
  ctx.textBaseline = 'middle'
  ctx.textAlign = 'left'
  ctx.fillStyle = '#fff'

  // 每个字的中心(做逐字刚体运动要用),先把位置画清楚再采样
  const charCX = new Float32Array(all.length)
  const charCY = new Float32Array(all.length)
  for (let li = 0; li < lines.length; li++) {
    let pen = (cw - lineW(li)) / 2
    const baseline = padY + LH * li + LH * 0.5
    for (let i = 0; i < lines[li].length; i++) {
      const gi = offsets[li] + i
      const c = lines[li][i]
      const w = lw[li][i]
      charCX[gi] = pen + w / 2
      charCY[gi] = baseline
      if (c !== ' ') ctx.fillText(c, pen, baseline)
      pen += w + track
    }
  }

  const data = ctx.getImageData(0, 0, cw, ch).data
  const step = Math.max(3, Math.round(gap))
  const px = []
  const py = []
  const pi = []
  const charOfX = new Int16Array(cw)

  for (let li = 0; li < lines.length; li++) {
    // 该行像素列 → 全局字序号(逐行重建,采样时 O(1) 查)
    charOfX.fill(-1)
    let pen = (cw - lineW(li)) / 2
    for (let i = 0; i < lines[li].length; i++) {
      const gi = offsets[li] + i
      const w = lw[li][i]
      const a = Math.max(0, Math.floor(pen) - 1)
      const b = Math.min(cw, Math.ceil(pen + w + track) + 1)
      for (let k = a; k < b; k++) charOfX[k] = gi
      pen += w + track
    }
    const yTop = Math.round(padY + LH * li)
    const yBot = Math.min(ch, Math.round(yTop + LH))
    for (let y = yTop; y < yBot; y += step) {
      const row = y * cw
      for (let x = 0; x < cw; x += step) {
        if (data[(row + x) * 4 + 3] < 118) continue
        const ci = charOfX[x]
        if (ci < 0) continue
        px.push(x)
        py.push(y)
        pi.push(ci)
      }
    }
  }
  if (!px.length) return null

  // 超上限就等距抽稀 —— 字形轮廓不会塌,只是点稀一点
  const stride = Math.max(1, Math.ceil(px.length / maxPoints))
  const n = Math.ceil(px.length / stride)
  const pos = new Float32Array(n * 3)
  const center = new Float32Array(n * 3)
  const charT = new Float32Array(n)
  const tint = new Float32Array(n)

  const s = 1 / fs
  const last = Math.max(1, all.length - 1)
  const cx = cw / 2
  const cy = ch / 2

  for (let i = 0, k = 0; k < n; i += stride, k++) {
    const ci = pi[i]
    const o3 = k * 3
    // 世界坐标:一个字高 = 1 单位,整块居中
    pos[o3] = (px[i] - cx) * s
    pos[o3 + 1] = -(py[i] - cy) * s
    pos[o3 + 2] = (Math.random() - 0.5) * 0.04

    center[o3] = (charCX[ci] - cx) * s
    center[o3 + 1] = -(charCY[ci] - cy) * s
    center[o3 + 2] = 0

    const byChar = ci / last
    charT[k] = byChar
    tint[k] = byChar
  }

  return { pos, center, charT, tint, count: n, width: maxLineW * s, height: ch * s }
}

// 一行字有多宽(1 单位 = 一个字高)。用来决定折几行,避免按字数瞎猜
let _measureCtx = null
function measureUnits(text) {
  if (!_measureCtx) {
    _measureCtx = document.createElement('canvas').getContext('2d')
    _measureCtx.font = `700 132px ${FONT_STACK}`
  }
  return _measureCtx.measureText(text).width / 132
}

// ─────────────────────────────────────────────────────────────
// 一套粒子:两个落点(A=歌词 / B=歌名)
// ─────────────────────────────────────────────────────────────
function createField({ cap, pointSize, opacity }) {
  const geo = new THREE.BufferGeometry()
  const buf = {
    position: new Float32Array(cap * 3),
    aTitle: new Float32Array(cap * 3),
    aCenter: new Float32Array(cap * 3),
    aCTitle: new Float32Array(cap * 3),
    aCharL: new Float32Array(cap),
    aCharT: new Float32Array(cap),
    aDelay: new Float32Array(cap),
    aRand: new Float32Array(cap),
    aTint: new Float32Array(cap),
    aWL: new Float32Array(cap),
    aWT: new Float32Array(cap),
  }
  for (const key of Object.keys(buf)) {
    const a = buf[key]
    geo.setAttribute(key, new THREE.BufferAttribute(a, key === 'position' || key === 'aTitle' || key === 'aCenter' || key === 'aCTitle' ? 3 : 1))
  }
  geo.setDrawRange(0, 0)

  // 每颗粒子一个固定随机数:整条命都不变,抖动/大小才不会逐帧闪
  const rand = new Float32Array(cap)
  for (let i = 0; i < cap; i++) rand[i] = Math.random()

  const shared = {
    uA: { value: new THREE.Color('#6ee7d7') },
    uB: { value: new THREE.Color('#4d9dff') },
    uC: { value: new THREE.Color('#b07cff') },
    uPixelScale: { value: 400 },
    uTime: { value: 0 },
    uBeatTime: { value: -10 },
    uBeat: { value: 0 },
    uMorph: { value: 0 },
    uFresh: { value: 1 },
    uScaleL: { value: 1 },
    uScaleT: { value: 1 },
    uBass: { value: 0 },
    uHigh: { value: 0 },
  }

  const material = new THREE.ShaderMaterial({
    vertexShader: VERT_FIX,
    fragmentShader: FRAG,
    uniforms: {
      ...shared,
      uSize: { value: pointSize },
      uOpacity: { value: opacity },
    },
    transparent: true,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  })

  const points = new THREE.Points(geo, material)
  points.frustumCulled = false // buffer 按上限预分配,包围球没法信

  // 柔光晕:同一份 geometry 再画一遍,点更大更淡 → additive 叠出霓虹感。
  // 两个 draw call 换一整套 bloom composer,怎么算都划算。
  const haloMaterial = new THREE.ShaderMaterial({
    vertexShader: VERT_FIX,
    fragmentShader: FRAG,
    uniforms: {
      ...material.uniforms,          // 浅拷贝:shared 里那几个仍是同一份引用
      uSize: { value: pointSize * 2.3 },
      uOpacity: { value: opacity * 0.34 },
    },
    transparent: true,
    depthTest: false,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  })
  const halo = new THREE.Points(geo, haloMaterial)
  halo.frustumCulled = false
  halo.renderOrder = -1

  const group = new THREE.Group()
  group.add(halo)
  group.add(points)

  // 源:两套字(归一化坐标,1 单位 = 一行字高)
  let srcL = null
  let srcT = null
  let nDraw = 0

  /** 把两套字铺到粒子上。scale/offset 在这儿烘进顶点,所以 morph 是纯世界空间插值 */
  function apply(scaleL, scaleT, yL, yT) {
    const nL = srcL ? srcL.count : 0
    const nT = srcT ? srcT.count : 0
    if (!nL && !nT) {
      geo.setDrawRange(0, 0)
      nDraw = 0
      return
    }
    const N = Math.min(cap, Math.max(nL, nT))
    // 复制了几份就摊薄几份:不然短句(点少)被复制 4 遍会叠成一团白
    const wL = nL ? Math.min(1, nL / N) : 1
    const wT = nT ? Math.min(1, nT / N) : 1
    const pxL = srcL ? srcL.pos : null
    const pxT = srcT ? srcT.pos : null
    const cxL = srcL ? srcL.center : null
    const cxT = srcT ? srcT.center : null

    for (let i = 0; i < N; i++) {
      const o3 = i * 3
      const li = nL ? Math.min(nL - 1, (i * nL / N) | 0) : -1
      const ti = nT ? Math.min(nT - 1, (i * nT / N) | 0) : -1
      // 一侧缺失时两边都用另一侧 → morph 变成原地不动,不会飞丢
      const sla = li >= 0 ? li : ti
      const sti = ti >= 0 ? ti : li
      const r = rand[i]
      const jx = (r - 0.5) * 0.012
      const jy = ((r * 7.13) % 1 - 0.5) * 0.012

      if (pxL) {
        buf.position[o3] = pxL[sla * 3] * scaleL + jx
        buf.position[o3 + 1] = pxL[sla * 3 + 1] * scaleL + jy + yL
        buf.position[o3 + 2] = pxL[sla * 3 + 2] * scaleL
        buf.aCenter[o3] = cxL[sla * 3] * scaleL
        buf.aCenter[o3 + 1] = cxL[sla * 3 + 1] * scaleL + yL
        buf.aCenter[o3 + 2] = 0
        buf.aCharL[i] = srcL.charT[sla]
      } else {
        buf.position[o3] = pxT[sti * 3] * scaleT + jx
        buf.position[o3 + 1] = pxT[sti * 3 + 1] * scaleT + jy + yT
        buf.position[o3 + 2] = pxT[sti * 3 + 2] * scaleT
        buf.aCenter[o3] = cxT[sti * 3] * scaleT
        buf.aCenter[o3 + 1] = cxT[sti * 3 + 1] * scaleT + yT
        buf.aCenter[o3 + 2] = 0
        buf.aCharL[i] = srcT.charT[sti]
      }

      if (pxT) {
        buf.aTitle[o3] = pxT[sti * 3] * scaleT + jx
        buf.aTitle[o3 + 1] = pxT[sti * 3 + 1] * scaleT + jy + yT
        buf.aTitle[o3 + 2] = pxT[sti * 3 + 2] * scaleT
        buf.aCTitle[o3] = cxT[sti * 3] * scaleT
        buf.aCTitle[o3 + 1] = cxT[sti * 3 + 1] * scaleT + yT
        buf.aCTitle[o3 + 2] = 0
        buf.aCharT[i] = srcT.charT[sti]
        // 起飞顺序跟着歌名的字序走,而且同一目标字只留一点随机 ——
        // 乱序随机会把"字一股股飞过来"打散成一团噪点云
        buf.aDelay[i] = Math.min(1, srcT.charT[sti] * 0.86 + rand[i] * 0.14)
      } else if (pxL) {
        buf.aTitle[o3] = buf.position[o3]
        buf.aTitle[o3 + 1] = buf.position[o3 + 1]
        buf.aTitle[o3 + 2] = buf.position[o3 + 2]
        buf.aCTitle[o3] = buf.aCenter[o3]
        buf.aCTitle[o3 + 1] = buf.aCenter[o3 + 1]
        buf.aCTitle[o3 + 2] = 0
        buf.aCharT[i] = buf.aCharL[i]
        buf.aDelay[i] = Math.min(1, buf.aCharL[i] * 0.62 + rand[i] * 0.38)
      }

      buf.aRand[i] = r
      buf.aTint[i] = (pxL ? srcL.tint[sla] : srcT.tint[sti])
      buf.aWL[i] = wL
      buf.aWT[i] = wT
    }

    for (const key of Object.keys(buf)) geo.attributes[key].needsUpdate = true
    geo.setDrawRange(0, N)
    nDraw = N
    shared.uScaleL.value = scaleL
    shared.uScaleT.value = scaleT
  }

  return {
    object: group,
    core: points,
    shared,
    get dataCount() { return nDraw },
    get lyric() { return srcL },
    get title() { return srcT },
    setLyric(glyph) { srcL = glyph },
    setTitle(glyph) { srcT = glyph },
    apply,
    setPixelScale(v) { shared.uPixelScale.value = v },
    dispose() {
      geo.dispose()
      material.dispose()
      haloMaterial.dispose()
    },
  }
}

// ─────────────────────────────────────────────────────────────
// 主入口
// ─────────────────────────────────────────────────────────────
export function createLyricFx({ container, getAnalyser, isPlaying = () => false, isMobile = false }) {
  if (!container) return null

  const renderer = new THREE.WebGLRenderer({
    alpha: true,
    antialias: false,
    powerPreference: 'high-performance',
    stencil: false,
  })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2))
  renderer.setClearColor(0x000000, 0)
  renderer.domElement.className = 'viz-canvas-lyricfx'
  container.appendChild(renderer.domElement)

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(FOV, 1, 0.1, 120)
  camera.position.set(0, 0, 8)
  camera.lookAt(0, 0, 0)

  // 手机上一个页面已经有两块 WebGL(自研场景 + MilkDrop),粒子砍半,别抢 GPU
  const cap = isMobile ? CAP_MOBILE : CAP
  const field = createField({ cap, pointSize: 0.075, opacity: 0.85 })
  const root = new THREE.Group()
  root.add(field.object)
  scene.add(root)

  // ── 频谱 ──
  let bins = new Uint8Array(512)
  const bassHistory = []
  let beatCooldown = 0
  let synthPhase = 0
  let beatPulse = 0
  let beatTime = -10
  let beatCount = 0

  // ── 拼歌名状态机 ──
  // idle:只有歌词 → up:逐拍拼歌名 → hold:摆几拍 → down:散回歌词
  let phase = 'idle'
  let morph = 0
  let morphTarget = 0
  let holdBeats = 0
  let idleBeats = 0
  let fresh = 1          // 0→1,换行时从四周聚拢
  let pendingFresh = false

  function onBeat() {
    beatCount++
    if (phase === 'idle') {
      if (idleBeats > 0) idleBeats--
      else if (field.title) {
        phase = 'up'
        morphTarget = 1 / STEPS_UP
      }
      return
    }
    if (phase === 'up') {
      morphTarget = Math.min(1, morphTarget + 1 / STEPS_UP)
      if (morphTarget >= 1) { phase = 'hold'; holdBeats = HOLD_BEATS }
      return
    }
    if (phase === 'hold') {
      if (--holdBeats <= 0) phase = 'down'
      return
    }
    // down
    morphTarget = Math.max(0, morphTarget - 1 / STEPS_DOWN)
    if (morphTarget <= 0) {
      phase = 'idle'
      idleBeats = IDLE_BEATS
    }
  }

  function readLevels(dt) {
    const an = getAnalyser?.()
    let bass = 0
    let high = 0
    let live = true

    if (an) {
      if (bins.length !== an.frequencyBinCount) bins = new Uint8Array(an.frequencyBinCount)
      an.getByteFrequencyData(bins)
      const n = bins.length
      const binHz = 22050 / n
      const at = hz => Math.max(2, Math.min(n - 1, Math.round(hz / binHz)))
      const b0 = at(160)
      const h0 = at(2400)
      const h1 = Math.max(h0 + 1, at(9000))
      let lo = 0
      for (let i = 1; i < b0; i++) lo += bins[i]
      bass = lo / Math.max(1, b0 - 1) / 255
      let hi = 0
      for (let i = h0; i < h1; i++) hi += bins[i]
      high = hi / Math.max(1, h1 - h0) / 255
      if (bass <= 0.004 && high <= 0.004) bass = -1 // 有分析器但没信号 → 走合成
    } else {
      bass = -1
    }

    if (bass < 0) {
      live = false
      // 兜底:96BPM 合成拍子,画面不会因为拿不到频谱就死掉
      synthPhase += dt * (96 / 60)
      if (isPlaying()) {
        const env = Math.pow(1 - (synthPhase % 1), 2.6)
        bass = 0.26 + env * 0.5
        high = 0.12 + env * 0.1
      } else {
        bass = 0.1
        high = 0.05
      }
    }

    // 瞬态检测:低频显著越过近 40 帧均值 → 一拍
    beatCooldown = Math.max(0, beatCooldown - dt)
    bassHistory.push(bass)
    if (bassHistory.length > 40) bassHistory.shift()
    let avg = 0
    for (const v of bassHistory) avg += v
    avg /= bassHistory.length
    if (beatCooldown === 0 && bass > 0.2 && bass > avg * 1.22 && bass - avg > 0.03) {
      // 合成拍子(无频谱)只给半个力度:兜底的鼓点本来就是匀速的
      beatPulse = Math.max(beatPulse, live ? 1 : 0.55)
      beatTime = elapsed
      beatCooldown = 0.24
      onBeat()
    }
    beatPulse = Math.max(0, beatPulse - dt * 1.9)

    return { bass: Math.min(1, bass), high: Math.min(1, high) }
  }

  // ── 指针视差 ──
  const pointer = { x: 0, y: 0, tx: 0, ty: 0 }

  // ── 主循环 ──
  let raf = 0
  let last = 0
  let elapsed = 0
  let running = false

  // 两套字的当前缩放(位置是烘进顶点的,缩放变了要重铺)
  let scaleL = 1
  let scaleT = 1

  function fitScales() {
    const visH = 2 * camera.position.z * Math.tan((FOV * Math.PI) / 360)
    const visW = visH * camera.aspect
    // 桌面右侧有一条 SCENE / MOOD 竖排面板(约 90px),字铺满整幅会钻到它底下
    const usableL = visW * (isMobile ? 0.9 : 0.74)
    const usableT = visW * (isMobile ? 0.88 : 0.84)
    const L = field.lyric
    const T = field.title
    const nextL = L ? Math.min(1, usableL / L.width) : scaleL
    const nextT = T ? Math.min(isMobile ? 1.25 : 1.65, usableT / T.width) : scaleT
    if (Math.abs(nextL - scaleL) < 0.004 && Math.abs(nextT - scaleT) < 0.004 && field.dataCount) return
    scaleL = nextL
    scaleT = nextT
    repaint()
  }

  function repaint() {
    field.apply(scaleL, scaleT, LYRIC_Y, TITLE_Y)
  }

  function frame(now) {
    if (!running) return
    raf = requestAnimationFrame(frame)
    const dt = last ? Math.min((now - last) / 1000, 0.08) : 0.016
    last = now
    elapsed += dt

    // 换行动画:等真的回到歌词态才开始聚拢(在歌名态换了行,玩家看不见)
    if (pendingFresh && morph < 0.25) {
      fresh = 0
      pendingFresh = false
    }
    fresh = Math.min(1, fresh + dt * 2.6)

    const { bass, high } = readLevels(dt)

    pointer.x += (pointer.tx - pointer.x) * Math.min(1, dt * 3.5)
    pointer.y += (pointer.ty - pointer.y) * Math.min(1, dt * 3.5)

    // 没有歌词(前奏/间奏)时,歌名就是主角,一直摆着
    if (!field.lyric && field.title) {
      phase = 'hold'
      morphTarget = 1
      holdBeats = Math.max(holdBeats, 1)
    }
    // 拍点断了(暂停 / 静音段 / 切歌空档)就别让粒子卡在航线中间 ——
    // 编排是逐拍推进的,没有拍子就会停在一个"半成品歌名"上,永远散着
    if (elapsed - beatTime > 2.2) {
      phase = 'idle'
      idleBeats = 0
      morphTarget = field.lyric ? 0 : 1
    }
    morph += (morphTarget - morph) * Math.min(1, dt * 9.5)

    root.rotation.y = pointer.x * 0.1 + Math.sin(elapsed * 0.25) * 0.022
    root.rotation.x = -pointer.y * 0.05 + Math.sin(elapsed * 0.19) * 0.012
    // 低频把整块往前推一点 —— 最便宜的冲击感
    root.position.z = bass * 0.3

    fitScales()

    const u = field.shared
    u.uTime.value = elapsed
    u.uBeatTime.value = beatTime
    u.uBeat.value = beatPulse
    u.uMorph.value = morph
    u.uFresh.value = fresh
    u.uBass.value = bass
    u.uHigh.value = high

    renderer.render(scene, camera)
  }

  // 屏幕上这块字能占多宽(世界单位)
  function availableWidth() {
    const visH = 2 * camera.position.z * Math.tan((FOV * Math.PI) / 360)
    return visH * camera.aspect * (isMobile ? 0.9 : 0.74)
  }

  // 一行放不下就折行:按"折行后每个字至少还有 0.55 单位高"反推每行字数。
  // 竖屏 390px 上 12 个字硬排一行,字高只剩 30px,实际看不清。
  // 中文每行最多几个字:横向排更多的话,整行缩放会把字压小(缩放上限是 1),
  // 12 个字挤一行只有 0.67 字高,不如折两行来得大。
  const MAX_CJK_PER_LINE = isMobile ? 4 : 7
  function perLineFor(text) {
    const chars = [...text]
    if (!chars.length) return 0
    // 按真实字宽估「至少几行才不超宽」,再按行数均分字数 ——
    // 只按字数猜会把英文(字宽只有中文一半)折成 "that" 单独一行这种孤儿行
    const rows = Math.max(1, Math.ceil(measureUnits(text) / availableWidth()))
    let per = Math.ceil(chars.length / rows)
    if (!text.includes(' ')) per = Math.min(per, MAX_CJK_PER_LINE)
    return per < chars.length ? per : 0 // 0 = 不折
  }

  let lastLine = ''
  let lastPerLine = -1

  function applyLine(text) {
    lastLine = text || ''
    lastPerLine = perLineFor(lastLine)
    field.setLyric(buildGlyph(lastLine, {
      gap: 3, tracking: 0.12, maxChars: 34, maxPoints: cap, perLine: lastPerLine,
    }))
    scaleL = 1
    repaint()
    // 正在歌名态换了行:聚拢动画留到散回歌词那一刻再放,否则看不见
    pendingFresh = morph > 0.25
    fresh = pendingFresh ? 1 : 0
    // 正在拼歌名就让它拼完(就差一两拍),已经摆起来了才立刻散回,别把新词压没
    if (phase === 'hold') phase = 'down'
  }

  function applyTitle(title) {
    // "T1213121（AI 新唱）"这种把括号里的版本说明砍掉,歌名才不会被撑成一条
    const t = String(title || '').replace(/\s+/g, ' ').split(/[（(【\[]/)[0].trim()
    // 手机竖屏一行塞不下长歌名,折成两行(桌面横屏没这问题)
    field.setTitle(buildGlyph(t, {
      gap: 3, tracking: 0.16, maxChars: 16, maxPoints: cap,
      perLine: isMobile ? 6 : 0,
    }))
    scaleT = 1
    repaint()
    // 换歌:让这些粒子逐拍把歌名拼出来亮个相,再散回歌词
    if (field.title) {
      phase = 'up'
      morphTarget = 1 / STEPS_UP
      idleBeats = 0
    }
  }

  function resize() {
    const w = Math.max(1, container.clientWidth)
    const h = Math.max(1, container.clientHeight)
    const heightPx = h * renderer.getPixelRatio()
    renderer.setSize(w, h, false)
    renderer.domElement.style.width = '100%'
    renderer.domElement.style.height = '100%'
    camera.aspect = w / h
    camera.updateProjectionMatrix()
    field.setPixelScale(heightPx / (2 * Math.tan((FOV * Math.PI) / 360)))
    // 宽度变了可能要重新折行(横竖屏切换),只在这件事真的变了时重建
    if (lastLine && perLineFor(lastLine) !== lastPerLine) applyLine(lastLine)
    else fitScales()
  }

  function setPalette(p) {
    const { uA, uB, uC } = field.shared
    uA.value.set(p.a)
    uB.value.set(p.b)
    uC.value.set(p.c)
  }

  function start() {
    if (running) return
    running = true
    last = 0
    raf = requestAnimationFrame(frame)
  }

  function stop() {
    running = false
    if (raf) cancelAnimationFrame(raf)
    raf = 0
  }

  function setPointer(nx, ny) {
    pointer.tx = Math.max(-1, Math.min(1, nx))
    pointer.ty = Math.max(-1, Math.min(1, ny))
  }

  function dispose() {
    stop()
    field.dispose()
    scene.clear()
    renderer.dispose()
    if (renderer.domElement.parentNode === container) container.removeChild(renderer.domElement)
  }

  resize()

  return {
    start,
    stop,
    resize,
    setPointer,
    setPalette,
    /** 换一行歌词;传空串 = 前奏/间奏,粒子散成歌名 */
    setLine: text => applyLine(text),
    /** 换歌名:粒子会把它拼出来亮个相 */
    setTitle: title => applyTitle(title),
    // 调试探针(生产构建会被摇掉)
    ...(import.meta.env?.DEV ? {
      _debug: { field, camera, get morph() { return morph }, get phase() { return phase }, get beatCount() { return beatCount } },
    } : {}),
    dispose,
  }
}
