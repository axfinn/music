// three.js 渲染引擎 —— 把 drive(音频)、scene(画面)、palette(配色)接起来。
// 只做三件事:搭渲染管线(含绽放)、每帧喂数据、切换场景/配色。
// 场景的具体画法都在 scenes.js,这里不出现任何"某个场景专属"的逻辑。

import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { createScene, createFreqTexture } from './scenes.js'

const BG = 0x05060a

export function createVisualEngine({ container, drive, isMobile = false }) {
  const width = Math.max(1, container.clientWidth)
  const height = Math.max(1, container.clientHeight)

  const renderer = new THREE.WebGLRenderer({
    antialias: false,
    alpha: false,
    powerPreference: 'high-performance',
    stencil: false,
  })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2))
  renderer.setSize(width, height)
  renderer.setClearColor(BG, 1)
  renderer.toneMapping = THREE.NoToneMapping
  renderer.domElement.className = 'viz-canvas'
  container.appendChild(renderer.domElement)

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(55, width / height, 0.1, 260)

  // 半浮点 + MSAA:线条不锯齿,绽放才有"霓虹"的干净边缘
  const target = new THREE.WebGLRenderTarget(width, height, {
    type: THREE.HalfFloatType,
    samples: isMobile ? 0 : 2,
  })
  const composer = new EffectComposer(renderer, target)
  composer.addPass(new RenderPass(scene, camera))
  const bloom = new UnrealBloomPass(
    new THREE.Vector2(width, height),
    isMobile ? 0.5 : 0.62,
    isMobile ? 0.5 : 0.6,
    isMobile ? 0.3 : 0.24
  )
  composer.addPass(bloom)
  composer.addPass(new OutputPass())

  const freqTex = createFreqTexture(256)
  const ctx = {
    freqTex,
    isMobile,
    pixelRatio: renderer.getPixelRatio(),
  }

  const pal = { a: new THREE.Color('#6ee7d7'), b: new THREE.Color('#4d9dff'), c: new THREE.Color('#b07cff') }
  let current = null
  let currentId = ''
  let base = { position: new THREE.Vector3(0, 0, 8), target: new THREE.Vector3(), fov: 55, parallax: 0.4 }
  let camPos = new THREE.Vector3(0, 0, 8)
  let camTarget = new THREE.Vector3()
  let camFov = 55
  const pointer = { x: 0, y: 0, tx: 0, ty: 0 }

  function mountScene(id) {
    if (current) {
      scene.remove(current.group)
      current.dispose?.()
    }
    current = createScene(id, ctx)
    currentId = current.id
    scene.add(current.group)
    const c = current.camera || {}
    base = {
      position: new THREE.Vector3().fromArray(c.position || [0, 0, 8]),
      target: new THREE.Vector3().fromArray(c.target || [0, 0, 0]),
      fov: c.fov || 55,
      parallax: c.parallax ?? 0.4,
    }
    camera.position.copy(base.position)
    camPos.copy(base.position)
    camTarget.copy(base.target)
    camFov = base.fov
    camera.fov = camFov
    camera.updateProjectionMatrix()
    camera.lookAt(base.target)
    return currentId
  }

  let raf = 0
  let last = 0
  let elapsed = 0
  let running = false

  function frame(now) {
    if (!running) return
    raf = requestAnimationFrame(frame)
    const dt = last ? Math.min((now - last) / 1000, 0.08) : 0.016
    last = now
    elapsed += dt

    drive.update(dt)
    freqTex.update(drive.freq)

    pointer.x += (pointer.tx - pointer.x) * Math.min(1, dt * 4)
    pointer.y += (pointer.ty - pointer.y) * Math.min(1, dt * 4)
    const amp = base.parallax
    camPos.set(
      base.position.x + pointer.x * amp,
      base.position.y - pointer.y * amp * 0.8,
      base.position.z
    )
    camTarget.lerp(base.target, Math.min(1, dt * 5))
    camera.position.lerp(camPos, Math.min(1, dt * 6))
    camera.lookAt(camTarget)
    // 节拍给一个极小的 FOV 冲击,像被人轻轻推了一下
    const fov = base.fov - drive.beat * 1.6
    if (Math.abs(fov - camFov) > 0.01) {
      camFov = fov
      camera.fov = fov
      camera.updateProjectionMatrix()
    }

    current?.update(dt, elapsed, drive, pal, camera)
    composer.render()
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

  function resize() {
    const w = Math.max(1, container.clientWidth)
    const h = Math.max(1, container.clientHeight)
    camera.aspect = w / h
    camera.updateProjectionMatrix()
    renderer.setSize(w, h)
    composer.setSize(w, h)
    bloom.setSize(w, h)
  }

  function setPointer(nx, ny) {
    pointer.tx = Math.max(-1, Math.min(1, nx))
    pointer.ty = Math.max(-1, Math.min(1, ny))
  }

  function setPalette(p) {
    pal.a.set(p.a)
    pal.b.set(p.b)
    pal.c.set(p.c)
  }

  function dispose() {
    stop()
    if (current) {
      scene.remove(current.group)
      current.dispose?.()
      current = null
    }
    freqTex.texture.dispose()
    composer.dispose?.()
    renderer.dispose()
    if (renderer.domElement.parentNode === container) container.removeChild(renderer.domElement)
  }

  mountScene('aurora')

  return {
    start,
    stop,
    resize,
    setPointer,
    setPalette,
    mountScene,
    getSceneId: () => currentId,
    get canvas() { return renderer.domElement },
    dispose,
  }
}
