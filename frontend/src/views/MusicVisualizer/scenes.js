// 视觉场景集合 —— 6 个风格互相独立的音律场景。
//
// 每个场景工厂返回:
//   { group, camera, update(dt, t, d, pal), dispose() }
//   d   = drive(见 drive.js)   pal = { a, b, c } 三个 THREE.Color
// 场景只读这三个色,换配色 = 换整页气质。
//
// 设计原则(改之前请守住):
//   1. 简约优先 —— 元素少、留白多、颜色只用 3 个,靠绽放(bloom)出光;
//   2. 没有频谱也必须好看 —— 用 drive.beat / drive.levels 的合成节奏驱动;
//   3. 顶点级别的计算都在 GPU,JS 每帧只写 uniform / 少量 instanceMatrix。

import * as THREE from 'three'

const TAU = Math.PI * 2

// 便宜的 3D 值噪声(比 simplex 短,视觉上够用)
const NOISE_GLSL = `
float hash31(vec3 p) {
  p = fract(p * 0.3183099 + vec3(0.1, 0.2, 0.3));
  p *= 17.0;
  return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}
float vnoise(vec3 x) {
  vec3 i = floor(x);
  vec3 f = fract(x);
  f = f * f * (3.0 - 2.0 * f);
  return mix(
    mix(mix(hash31(i), hash31(i + vec3(1.0, 0.0, 0.0)), f.x),
        mix(hash31(i + vec3(0.0, 1.0, 0.0)), hash31(i + vec3(1.0, 1.0, 0.0)), f.x), f.y),
    mix(mix(hash31(i + vec3(0.0, 0.0, 1.0)), hash31(i + vec3(1.0, 0.0, 1.0)), f.x),
        mix(hash31(i + vec3(0.0, 1.0, 1.0)), hash31(i + vec3(1.0, 1.0, 1.0)), f.x), f.y),
    f.z);
}
float fbm3(vec3 p) {
  float v = 0.0;
  float a = 0.5;
  for (int i = 0; i < 5; i++) {
    v += a * vnoise(p);
    p *= 2.03;
    a *= 0.5;
  }
  return v;
}
`

// 频谱 → 纹理:场景在 GPU 里按需采样某个频段,免掉每帧上传顶点属性
export function createFreqTexture(size = 256) {
  const data = new Uint8Array(size * 4)
  const texture = new THREE.DataTexture(data, size, 1, THREE.RGBAFormat)
  texture.minFilter = THREE.LinearFilter
  texture.magFilter = THREE.LinearFilter
  texture.wrapS = THREE.ClampToEdgeWrapping
  texture.wrapT = THREE.ClampToEdgeWrapping
  texture.needsUpdate = true

  function update(freq) {
    const n = freq.length
    for (let i = 0; i < size; i++) {
      // 对数取样:低频给更多格子(和听感、视觉兴奋点都对齐)
      const a = Math.min(n - 1, Math.floor(Math.pow(i / size, 1.7) * n))
      const b = Math.max(a + 1, Math.min(n, Math.floor(Math.pow((i + 1) / size, 1.7) * n)))
      let sum = 0
      for (let k = a; k < b; k++) sum += freq[k]
      const v = Math.round(sum / (b - a))
      const o = i * 4
      data[o] = v
      data[o + 1] = v
      data[o + 2] = v
      data[o + 3] = 255
    }
    texture.needsUpdate = true
  }

  return { texture, update }
}

// 径向发光贴图(隧道尽头 / 球的背光)
function createGlowTexture() {
  const size = 128
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const g = canvas.getContext('2d')
  const grad = g.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  grad.addColorStop(0, 'rgba(255,255,255,1)')
  grad.addColorStop(0.25, 'rgba(255,255,255,0.55)')
  grad.addColorStop(0.6, 'rgba(255,255,255,0.12)')
  grad.addColorStop(1, 'rgba(255,255,255,0)')
  g.fillStyle = grad
  g.fillRect(0, 0, size, size)
  const tex = new THREE.CanvasTexture(canvas)
  tex.needsUpdate = true
  return tex
}

function sampleFreq(freq, band) {
  const n = freq.length
  const idx = Math.min(n - 1, Math.max(0, Math.floor(band * n)))
  return freq[idx] / 255
}

function disposeTree(root) {
  root.traverse(obj => {
    if (obj.geometry) obj.geometry.dispose()
    const mats = Array.isArray(obj.material) ? obj.material : obj.material ? [obj.material] : []
    for (const m of mats) {
      for (const key of Object.keys(m)) {
        const val = m[key]
        if (val && val.isTexture) val.dispose()
      }
      m.dispose()
    }
  })
}

// ─────────────────────────────────────────────
// 1. 极光 —— 全屏噪声天际 + 星尘,最"贵"的一屏,也是默认场景
// ─────────────────────────────────────────────
function createAurora(ctx) {
  const group = new THREE.Group()
  const uniforms = {
    uTime: { value: 0 },
    uBass: { value: 0 },
    uMid: { value: 0 },
    uHigh: { value: 0 },
    uA: { value: new THREE.Color() },
    uB: { value: new THREE.Color() },
    uC: { value: new THREE.Color() },
  }

  const sky = new THREE.Mesh(
    new THREE.SphereGeometry(48, 64, 48),
    new THREE.ShaderMaterial({
      uniforms,
      side: THREE.BackSide,
      depthWrite: false,
      depthTest: false,
      vertexShader: `
        varying vec3 vDir;
        void main() {
          vDir = position;
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }
      `,
      fragmentShader: `
        varying vec3 vDir;
        uniform float uTime, uBass, uMid, uHigh;
        uniform vec3 uA, uB, uC;
        ${NOISE_GLSL}
        void main() {
          vec3 n = normalize(vDir);
          float t = uTime * 0.05;
          float f1 = fbm3(n * 2.2 + vec3(0.0, t * 1.5, t * 0.7));
          float f2 = fbm3(n * 5.4 - vec3(t * 1.1, 0.0, t * 1.3));
          float body = smoothstep(0.55, 1.12, f1 * 0.72 + f2 * 0.42 + uBass * 0.3);
          float lift = smoothstep(-0.95, 0.55, n.y);
          vec3 col = mix(uA, uB, smoothstep(0.22, 0.86, f1 + uMid * 0.22));
          col = mix(col, uC, smoothstep(0.40, 1.0, f2 + uHigh * 0.30));
          float streak = 0.55 + 0.45 * (0.5 + 0.5 * sin(n.y * 11.0 + f1 * 9.0 + uTime * 0.5));
          float glow = body * (0.26 + uBass * 1.0) * mix(0.14, 1.0, lift) * streak;
          vec3 base = vec3(0.004, 0.006, 0.014);
          gl_FragColor = vec4(base + col * glow * 1.05, 1.0);
        }
      `,
    })
  )
  sky.frustumCulled = false
  group.add(sky)

  // 星尘:远处的细点,靠缓慢自转 + 呼吸体现"空间在动"
  const count = ctx.isMobile ? 1400 : 3600
  const pos = new Float32Array(count * 3)
  for (let i = 0; i < count; i++) {
    const r = 6 + Math.pow(Math.random(), 0.65) * 24
    const theta = Math.random() * TAU
    const phi = Math.acos(2 * Math.random() - 1)
    pos[i * 3] = r * Math.sin(phi) * Math.cos(theta)
    pos[i * 3 + 1] = r * Math.cos(phi) * 0.65
    pos[i * 3 + 2] = r * Math.sin(phi) * Math.sin(theta)
  }
  const dustGeo = new THREE.BufferGeometry()
  dustGeo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
  const dustMat = new THREE.PointsMaterial({
    size: 0.5,
    sizeAttenuation: true,
    transparent: true,
    opacity: 0.5,
    depthWrite: false,
    // 圆点贴图:默认方块点在这种暗底上会露馅成"像素块"
    map: createGlowTexture(),
    blending: THREE.AdditiveBlending,
  })
  const dust = new THREE.Points(dustGeo, dustMat)
  group.add(dust)

  return {
    group,
    camera: { position: [0, 0, 0.4], target: [0, 0, -1], fov: 62, parallax: 0.7 },
    update(dt, t, d, pal) {
      uniforms.uTime.value = t
      uniforms.uBass.value = d.levels.bass
      uniforms.uMid.value = d.levels.mid
      uniforms.uHigh.value = d.levels.high
      uniforms.uA.value.copy(pal.a)
      uniforms.uB.value.copy(pal.b)
      uniforms.uC.value.copy(pal.c)
      dust.rotation.y += dt * (0.018 + d.energy * 0.05)
      dust.rotation.x = Math.sin(t * 0.06) * 0.1
      dustMat.opacity = 0.2 + d.energy * 0.75
      dustMat.color.copy(pal.b)
    },
    dispose() { disposeTree(group) },
  }
}

// ─────────────────────────────────────────────
// 2. 频谱 —— 环形柱阵(瑞士风:只有柱、地面细圆、留白)
// ─────────────────────────────────────────────
function createBars(ctx) {
  const group = new THREE.Group()
  const COUNT = ctx.isMobile ? 60 : 112
  const R = 2.75
  const FLOOR = -1.2

  const barGeo = new THREE.BoxGeometry(0.05, 1, 0.05)
  barGeo.translate(0, 0.5, 0)
  const barMat = new THREE.MeshBasicMaterial({ transparent: true, opacity: 0.95 })
  const bars = new THREE.InstancedMesh(barGeo, barMat, COUNT)
  bars.instanceMatrix.setUsage(THREE.DynamicDrawUsage)
  bars.frustumCulled = false

  // 地面倒影:同一形状压扁反相,低透明度就够
  const mirrorMat = new THREE.MeshBasicMaterial({ transparent: true, opacity: 0.12, side: THREE.DoubleSide })
  const mirror = new THREE.InstancedMesh(barGeo, mirrorMat, COUNT)
  mirror.instanceMatrix.setUsage(THREE.DynamicDrawUsage)
  mirror.frustumCulled = false

  const capGeo = new THREE.BoxGeometry(0.075, 0.018, 0.075)
  const capMat = new THREE.MeshBasicMaterial({ transparent: true, opacity: 0.9 })
  const caps = new THREE.InstancedMesh(capGeo, capMat, COUNT)
  caps.instanceMatrix.setUsage(THREE.DynamicDrawUsage)
  caps.frustumCulled = false

  group.add(bars, mirror, caps)

  // 地面极细圆环 + 中心十字,画龙点睛的"设计感"
  const ringPts = []
  for (let i = 0; i <= 160; i++) {
    const a = (i / 160) * TAU
    ringPts.push(new THREE.Vector3(Math.cos(a) * R, FLOOR, Math.sin(a) * R))
  }
  const ringGeo = new THREE.BufferGeometry().setFromPoints(ringPts)
  const ringMat = new THREE.LineBasicMaterial({ transparent: true, opacity: 0.35 })
  const ring = new THREE.Line(ringGeo, ringMat)
  group.add(ring)

  const innerRingGeo = new THREE.RingGeometry(R * 0.98, R * 0.985, 128)
  const innerRingMat = new THREE.MeshBasicMaterial({
    transparent: true, opacity: 0.25, side: THREE.DoubleSide, depthWrite: false,
  })
  const innerRing = new THREE.Mesh(innerRingGeo, innerRingMat)
  innerRing.rotation.x = -Math.PI / 2
  innerRing.position.y = FLOOR
  group.add(innerRing)

  const dummy = new THREE.Object3D()
  const colorTmp = new THREE.Color()

  return {
    group,
    camera: { position: [0, 4.4, 11.4], target: [0, -0.1, 0], fov: 42, parallax: 0.25 },
    update(dt, t, d, pal) {
      group.rotation.y += dt * (0.06 + d.levels.bass * 0.12)
      const peak = pal.c
      for (let i = 0; i < COUNT; i++) {
        const angle = (i / COUNT) * TAU
        const band = Math.pow(i / COUNT, 1.45)
        const live = d.live ? sampleFreq(d.freq, band) : 0
        const synthetic = 0.16 + 0.55 * Math.abs(Math.sin(t * 1.7 + i * 0.42)) * (0.35 + d.levels.bass)
        const v = d.live ? live : synthetic
        const h = 0.1 + Math.pow(v, 1.28) * 4.4

        const x = Math.cos(angle) * R
        const z = Math.sin(angle) * R
        dummy.position.set(x, FLOOR, z)
        dummy.rotation.set(0, -angle, 0)
        dummy.scale.set(1, h, 1)
        dummy.updateMatrix()
        bars.setMatrixAt(i, dummy.matrix)

        dummy.position.set(x, FLOOR, z)
        dummy.rotation.set(0, -angle, 0)
        dummy.scale.set(1, -h * 0.55, 1)
        dummy.updateMatrix()
        mirror.setMatrixAt(i, dummy.matrix)

        dummy.position.set(x, FLOOR + h, z)
        dummy.scale.set(1, 1, 1)
        dummy.updateMatrix()
        caps.setMatrixAt(i, dummy.matrix)

        colorTmp.copy(pal.a).lerp(pal.b, band).lerp(peak, Math.pow(v, 2.2) * 0.85)
        bars.setColorAt(i, colorTmp)
        mirror.setColorAt(i, colorTmp)
        caps.setColorAt(i, colorTmp)
      }
      bars.instanceMatrix.needsUpdate = true
      mirror.instanceMatrix.needsUpdate = true
      caps.instanceMatrix.needsUpdate = true
      if (bars.instanceColor) bars.instanceColor.needsUpdate = true
      if (mirror.instanceColor) mirror.instanceColor.needsUpdate = true
      if (caps.instanceColor) caps.instanceColor.needsUpdate = true

      ringMat.color.copy(pal.c)
      ringMat.opacity = 0.18 + d.energy * 0.5
      innerRingMat.color.copy(pal.a)
      innerRingMat.opacity = 0.1 + d.levels.bass * 0.35
      capMat.opacity = 0.55 + d.levels.high * 0.45
    },
    dispose() { disposeTree(group) },
  }
}

// ─────────────────────────────────────────────
// 3. 星云 —— 两万多颗粒子,按频段呼吸、按节拍外推
// ─────────────────────────────────────────────
function createNebula(ctx) {
  const group = new THREE.Group()
  const COUNT = ctx.isMobile ? 9000 : 26000

  const base = new Float32Array(COUNT * 3)
  const band = new Float32Array(COUNT)
  const seed = new Float32Array(COUNT)
  for (let i = 0; i < COUNT; i++) {
    const r = 3.6 + Math.pow(Math.random(), 0.55) * 13
    const theta = Math.random() * TAU
    const phi = Math.acos(2 * Math.random() - 1)
    base[i * 3] = r * Math.sin(phi) * Math.cos(theta)
    base[i * 3 + 1] = r * Math.cos(phi) * 0.7
    base[i * 3 + 2] = r * Math.sin(phi) * Math.sin(theta)
    band[i] = Math.pow(Math.random(), 1.5) * 0.92
    seed[i] = Math.random()
  }

  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.BufferAttribute(base, 3))
  geo.setAttribute('aBand', new THREE.BufferAttribute(band, 1))
  geo.setAttribute('aSeed', new THREE.BufferAttribute(seed, 1))

  const uniforms = {
    uTime: { value: 0 },
    uBass: { value: 0 },
    uBeat: { value: 0 },
    uFreq: { value: ctx.freqTex.texture },
    uA: { value: new THREE.Color() },
    uB: { value: new THREE.Color() },
    uC: { value: new THREE.Color() },
    uPixelRatio: { value: ctx.pixelRatio || 1 },
  }

  const mat = new THREE.ShaderMaterial({
    uniforms,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    vertexShader: `
      attribute float aBand;
      attribute float aSeed;
      uniform sampler2D uFreq;
      uniform float uTime, uBass, uBeat, uPixelRatio;
      uniform vec3 uA, uB, uC;
      varying vec3 vColor;
      varying float vAlpha;
      void main() {
        float f = texture2D(uFreq, vec2(aBand, 0.5)).r;
        vec3 p = position;
        float swirl = uTime * (0.045 + aSeed * 0.13);
        float s = sin(swirl);
        float c = cos(swirl);
        p.xz = mat2(c, -s, s, c) * p.xz;
        float push = 1.0 + (f - 0.26) * 0.5 + uBass * 0.1 + uBeat * 0.06;
        p *= push;
        p.y += sin(uTime * 0.55 + aSeed * 20.0) * 0.45;
        vec4 mv = modelViewMatrix * vec4(p, 1.0);
        gl_Position = projectionMatrix * mv;
        vColor = mix(mix(uA, uB, aBand), uC, smoothstep(0.12, 0.95, f));
        vAlpha = 0.1 + f * 0.85;
        gl_PointSize = clamp((1.0 + f * 3.6) * uPixelRatio * (30.0 / max(-mv.z, 0.001)), 0.6, 30.0);
      }
    `,
    fragmentShader: `
      varying vec3 vColor;
      varying float vAlpha;
      void main() {
        float d = length(gl_PointCoord - 0.5);
        if (d > 0.5) discard;
        float a = smoothstep(0.5, 0.04, d) * vAlpha;
        gl_FragColor = vec4(vColor, a);
      }
    `,
  })

  group.add(new THREE.Points(geo, mat))

  return {
    group,
    camera: { position: [0, 0.6, 8.6], target: [0, 0, 0], fov: 58, parallax: 0.55 },
    update(dt, t, d, pal) {
      uniforms.uTime.value = t
      uniforms.uBass.value = d.levels.bass
      uniforms.uBeat.value = d.beat
      uniforms.uA.value.copy(pal.a)
      uniforms.uB.value.copy(pal.b)
      uniforms.uC.value.copy(pal.c)
      group.rotation.y += dt * 0.035
    },
    dispose() { disposeTree(group) },
  }
}

// ─────────────────────────────────────────────
// 4. 隧道 —— 霓虹环高速掠过,速度跟着低频走
// ─────────────────────────────────────────────
function createTunnel(ctx) {
  const group = new THREE.Group()
  const RINGS = ctx.isMobile ? 34 : 54
  const GAP = 2.1
  const TOTAL = RINGS * GAP

  const geo = new THREE.TorusGeometry(1, 0.0075, 3, 140)
  const rings = []
  for (let i = 0; i < RINGS; i++) {
    const mat = new THREE.MeshBasicMaterial({
      transparent: true,
      opacity: 0.6,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    })
    const mesh = new THREE.Mesh(geo, mat)
    mesh.position.set(0, 0, 2 - i * GAP)
    mesh.scale.setScalar(1 + (i % 4) * 0.09)
    mesh.userData.spin = (i % 2 ? 1 : -1) * (0.1 + (i % 5) * 0.03)
    rings.push(mesh)
    group.add(mesh)
  }

  const glowTex = createGlowTexture()
  const coreMat = new THREE.SpriteMaterial({
    map: glowTex,
    transparent: true,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    opacity: 0.9,
  })
  const core = new THREE.Sprite(coreMat)
  core.position.set(0, 0, -TOTAL + 6)
  core.scale.setScalar(14)
  group.add(core)

  const colorTmp = new THREE.Color()

  return {
    group,
    camera: { position: [0, 0, 7.2], target: [0, 0, -12], fov: 68, parallax: 0.2 },
    update(dt, t, d, pal) {
      const speed = 6 + d.levels.bass * 34 + d.energy * 10
      for (let i = 0; i < rings.length; i++) {
        const r = rings[i]
        r.position.z += dt * speed
        if (r.position.z > 9) r.position.z -= TOTAL
        r.rotation.z += dt * r.userData.spin
        const s = (1 + (i % 4) * 0.09) * (1 + d.levels.bass * 0.42)
        r.scale.setScalar(s)
        const k = (i / rings.length)
        colorTmp.copy(pal.a).lerp(pal.b, k).lerp(pal.c, Math.pow(k, 2.4))
        r.material.color.copy(colorTmp)
        r.material.opacity = 0.28 + d.levels.mid * 0.7
      }
      coreMat.color.copy(pal.c)
      coreMat.opacity = 0.5 + d.levels.bass * 0.5
      core.scale.setScalar(11 + d.beat * 6 + d.levels.bass * 5)
      group.position.x = Math.sin(t * 0.35) * 0.35
      group.position.y = Math.cos(t * 0.28) * 0.3
    },
    dispose() {
      disposeTree(group)
      glowTex.dispose()
    },
  }
}

// ─────────────────────────────────────────────
// 5. 波纹 —— 线框水面,左声道低频在左、高频在右
// ─────────────────────────────────────────────
function createRipple(ctx) {
  const group = new THREE.Group()
  const seg = ctx.isMobile ? 90 : 150
  const uniforms = {
    uTime: { value: 0 },
    uBass: { value: 0 },
    uMid: { value: 0 },
    uHigh: { value: 0 },
    uFreq: { value: ctx.freqTex.texture },
    uA: { value: new THREE.Color() },
    uB: { value: new THREE.Color() },
    uC: { value: new THREE.Color() },
  }

  const mat = new THREE.ShaderMaterial({
    uniforms,
    wireframe: true,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    vertexShader: `
      uniform float uTime, uBass, uMid, uHigh;
      uniform sampler2D uFreq;
      varying float vH;
      varying float vD;
      void main() {
        vec3 p = position;
        float d = length(p.xy);
        float ring = sin(d * 1.15 - uTime * 2.1) * exp(-d * 0.06);
        float f = texture2D(uFreq, vec2(clamp(p.x * 0.055 + 0.5, 0.0, 1.0), 0.5)).r;
        float h = ring * (0.22 + uBass * 1.25)
                + f * (0.35 + uMid * 0.9)
                + sin(p.y * 1.7 + uTime * 1.25) * 0.05 * (0.4 + uHigh);
        p.z += h;
        vH = h;
        vD = d;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(p, 1.0);
      }
    `,
    fragmentShader: `
      uniform vec3 uA, uB, uC;
      varying float vH;
      varying float vD;
      void main() {
        vec3 col = mix(uA, uB, smoothstep(-0.55, 0.35, vH));
        col = mix(col, uC, smoothstep(0.25, 0.95, vH));
        float fade = 1.0 - smoothstep(9.0, 18.0, vD);
        gl_FragColor = vec4(col, (0.16 + 0.8 * smoothstep(-0.4, 0.8, vH)) * fade);
      }
    `,
  })

  const plane = new THREE.Mesh(new THREE.PlaneGeometry(34, 34, seg, seg), mat)
  plane.rotation.x = -Math.PI / 2
  plane.position.y = -0.9
  group.add(plane)

  return {
    group,
    camera: { position: [0, 2.6, 8.2], target: [0, -0.2, -2], fov: 52, parallax: 0.4 },
    update(dt, t, d, pal) {
      uniforms.uTime.value = t
      uniforms.uBass.value = d.levels.bass
      uniforms.uMid.value = d.levels.mid
      uniforms.uHigh.value = d.levels.high
      uniforms.uA.value.copy(pal.a)
      uniforms.uB.value.copy(pal.b)
      uniforms.uC.value.copy(pal.c)
    },
    dispose() { disposeTree(group) },
  }
}

// ─────────────────────────────────────────────
// 6. 声球 —— 一颗被声音捏形的球 + 轨道环
// ─────────────────────────────────────────────
function createOrb(ctx) {
  const group = new THREE.Group()
  const uniforms = {
    uTime: { value: 0 },
    uBass: { value: 0 },
    uMid: { value: 0 },
    uCam: { value: new THREE.Vector3() },
    uA: { value: new THREE.Color() },
    uB: { value: new THREE.Color() },
    uC: { value: new THREE.Color() },
  }

  const orbMat = new THREE.ShaderMaterial({
    uniforms,
    vertexShader: `
      uniform float uTime, uBass, uMid;
      varying float vDisp;
      varying vec3 vN;
      varying vec3 vW;
      ${NOISE_GLSL}
      void main() {
        vec3 p = position;
        float n = fbm3(p * 1.05 + vec3(0.0, uTime * 0.2, 0.0));
        float spikes = fbm3(p * 2.7 - vec3(uTime * 0.34));
        float disp = (n - 0.5) * (0.4 + uBass * 1.15) + spikes * (0.14 + uMid * 0.4);
        p += normalize(position) * disp;
        vDisp = disp;
        vN = normalize(mat3(modelMatrix) * normal);
        vec4 wp = modelMatrix * vec4(p, 1.0);
        vW = wp.xyz;
        gl_Position = projectionMatrix * viewMatrix * wp;
      }
    `,
    fragmentShader: `
      uniform vec3 uA, uB, uC, uCam;
      varying float vDisp;
      varying vec3 vN;
      varying vec3 vW;
      void main() {
        vec3 v = normalize(uCam - vW);
        float fres = pow(1.0 - abs(dot(normalize(vN), v)), 2.4);
        vec3 col = mix(uA, uB, smoothstep(-0.35, 0.75, vDisp));
        col = mix(col, uC, fres * 0.85);
        col += uC * fres * 0.35;
        gl_FragColor = vec4(col * (0.2 + fres * 0.5), 1.0);
      }
    `,
  })

  const orb = new THREE.Mesh(
    new THREE.IcosahedronGeometry(2.05, ctx.isMobile ? 4 : 5),
    orbMat
  )
  group.add(orb)

  const shellMat = new THREE.MeshBasicMaterial({
    wireframe: true,
    transparent: true,
    opacity: 0.16,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
  })
  const shell = new THREE.Mesh(new THREE.IcosahedronGeometry(2.62, 2), shellMat)
  group.add(shell)

  // 轨道粒子环
  const COUNT = ctx.isMobile ? 500 : 1200
  const pos = new Float32Array(COUNT * 3)
  for (let i = 0; i < COUNT; i++) {
    const a = Math.random() * TAU
    const r = 3.0 + Math.random() * 1.6
    const tilt = (Math.random() - 0.5) * 0.5
    pos[i * 3] = Math.cos(a) * r
    pos[i * 3 + 1] = tilt * r * 0.4
    pos[i * 3 + 2] = Math.sin(a) * r
  }
  const orbitGeo = new THREE.BufferGeometry()
  orbitGeo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
  const orbitMat = new THREE.PointsMaterial({
    size: 0.055,
    transparent: true,
    opacity: 0.75,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    // 不吃贴图的话加点就是硬方块,近看很廉价;给一张径向渐变贴图变成柔光点
    map: createGlowTexture(),
  })
  const orbit = new THREE.Points(orbitGeo, orbitMat)
  group.add(orbit)

  return {
    group,
    camera: { position: [0, 0.5, 7.6], target: [0, 0, 0], fov: 48, parallax: 0.5 },
    update(dt, t, d, pal, camera) {
      uniforms.uTime.value = t
      uniforms.uBass.value = d.levels.bass
      uniforms.uMid.value = d.levels.mid
      uniforms.uA.value.copy(pal.a)
      uniforms.uB.value.copy(pal.b)
      uniforms.uC.value.copy(pal.c)
      if (camera) uniforms.uCam.value.copy(camera.position)
      orb.rotation.y += dt * (0.1 + d.levels.mid * 0.3)
      orb.rotation.x = Math.sin(t * 0.2) * 0.12
      shell.rotation.y -= dt * 0.16
      shell.rotation.z += dt * 0.05
      shellMat.color.copy(pal.c)
      shellMat.opacity = 0.08 + d.energy * 0.3
      orbit.rotation.y += dt * 0.35
      orbit.rotation.x = 0.35 + Math.sin(t * 0.3) * 0.12
      orbitMat.color.copy(pal.a)
      orbitMat.size = 0.04 + d.levels.high * 0.09
    },
    dispose() { disposeTree(group) },
  }
}

export const SCENE_LIST = [
  { id: 'aurora', label: '极光', en: 'AURORA', hint: '全屏流动的极光天际', create: createAurora },
  { id: 'bars', label: '频谱', en: 'SPECTRUM', hint: '环形柱阵,最直白的心跳', create: createBars },
  { id: 'nebula', label: '星云', en: 'NEBULA', hint: '两万颗粒子随频段呼吸', create: createNebula },
  { id: 'tunnel', label: '隧道', en: 'TUNNEL', hint: '霓虹环高速掠过', create: createTunnel },
  { id: 'ripple', label: '波纹', en: 'RIPPLE', hint: '线框水面,安静的高级感', create: createRipple },
  { id: 'orb', label: '声球', en: 'ORB', hint: '被声音捏形的球体', create: createOrb },
]

export function createScene(id, ctx) {
  const def = SCENE_LIST.find(s => s.id === id) || SCENE_LIST[0]
  const scene = def.create(ctx)
  scene.id = def.id
  scene.label = def.label
  return scene
}
