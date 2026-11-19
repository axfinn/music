// 歌词特效的文字层 —— 「跳动的字」「段落」两种非粒子效果。
//
// 为什么不用 WebGL/canvas:
//   粒子层已经占了一块 context,MilkDrop 再占一块;文字效果用 DOM + CSS
//   transform/opacity 动画走合成层,不新增 context、不新增 draw call,
//   而且中文字直接由系统字体渲染,不用把字形采样成点云。
//
// 性能约定:
//   · 动画全交给 CSS(animation / transition),JS 不逐帧写 DOM;
//   · 只有歌词/风格真的变了才重建节点(一行一次,几十个 span);
//   · 低频响应用一个 120ms 的定时器读一次 analyser,写一次 CSS 变量,
//     stop() 立刻清掉;关闭字效时这个定时器根本不存在。
//
// 与粒子层互斥:页面同一时刻只 start 一个渲染器。

import { getLyricStyle } from './lyricStyles.js'

let styleInjected = false
function injectStyle() {
  if (styleInjected) return
  styleInjected = true
  const el = document.createElement('style')
  el.dataset.lyricTextFx = '1'
  el.textContent = `
.ly-text-fx {
  position: absolute; inset: 0; pointer-events: none;
  display: flex; align-items: center; justify-content: center;
  --ly-bass: 0;
  --ly-a: var(--ly-accent, #7dd3fc);
  --ly-b: color-mix(in srgb, var(--ly-accent, #7dd3fc) 62%, #ffffff);
  --ly-c: color-mix(in srgb, var(--ly-accent, #7dd3fc) 68%, #b07cff);
}
.ly-text-stage {
  width: min(84vw, 900px); max-height: 100%;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 10px; transform: translateY(-4vh); text-align: center;
  font-family: 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Noto Sans SC', system-ui, sans-serif;
  user-select: none;
}
/* 弹幕要铺满全屏:盖掉 stage 的居中/位移/尺寸,否则 top:% 会相对一个塌陷的盒子 */
.ly-text-stage.as-marquee {
  position: absolute; inset: 0; width: auto; max-height: none;
  display: block; transform: none;
}
.ly-text-title {
  max-width: 100%; font-size: clamp(10px, 1.2vw, 13px); font-weight: 400;
  letter-spacing: 0.36em; text-indent: 0.36em; line-height: 1.6;
  color: color-mix(in srgb, var(--ly-a) 72%, transparent);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.ly-text-current {
  max-width: 100%; font-size: clamp(28px, 5.8vw, 74px); font-weight: 700;
  line-height: 1.32; letter-spacing: 0.02em;
  filter: drop-shadow(0 3px 18px rgba(0, 0, 0, 0.45));
}
.ly-text-ch {
  display: inline-block; white-space: pre; transform-origin: 50% 78%;
  background: linear-gradient(100deg, var(--ly-a), var(--ly-b) 46%, var(--ly-c));
  -webkit-background-clip: text; background-clip: text;
  -webkit-text-fill-color: transparent; color: transparent;
  animation: ly-bounce 0.62s ease-in-out infinite;
  animation-delay: calc(var(--i, 0) * -0.055s);
  animation-play-state: var(--ly-play, paused);
  will-change: transform;
}
@keyframes ly-bounce {
  0%, 100% { transform: translate3d(0, 0, 0) scale(1, 1); }
  22% { transform: translate3d(0, -0.15em, 0) scale(1.08, 0.93); }
  48% { transform: translate3d(0, 0.025em, 0) scale(0.98, 1.03); }
  72% { transform: translate3d(0, -0.04em, 0) scale(1.02, 0.99); }
}

/* 段落:当前句放大聚焦,前后各两句淡出,像歌词本在滚 */
.ly-text-para {
  width: 100%; max-width: min(62vw, 720px);
  display: flex; flex-direction: column; align-items: center; gap: 2px;
  max-height: 54vh; overflow: hidden;
  mask-image: linear-gradient(to bottom, transparent, #000 24%, #000 76%, transparent);
  -webkit-mask-image: linear-gradient(to bottom, transparent, #000 24%, #000 76%, transparent);
}
.ly-text-line {
  max-width: 100%; padding: 1px 8px;
  font-size: clamp(15px, 2.1vw, 26px); font-weight: 500; line-height: 1.75;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  color: rgba(255, 255, 255, 0.42);
  opacity: var(--ly-op, 1);
  transition: opacity 0.42s ease, transform 0.42s ease, color 0.42s ease;
}
.ly-text-line.active {
  color: #fff;
  transform: scale(calc(1 + var(--ly-bass) * 0.045));
  filter: drop-shadow(0 2px 14px rgba(0, 0, 0, 0.5));
}
.ly-text-line.active .ly-text-ch { animation: none; -webkit-text-fill-color: #fff; color: #fff; }
.ly-text-line.active.ly-focus { animation: ly-focus 0.5s cubic-bezier(0.2, 0.8, 0.2, 1); }
@keyframes ly-focus {
  from { opacity: 0; transform: translateY(14px) scale(0.96); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
.ly-text-idle {
  font-size: clamp(24px, 4.6vw, 60px); font-weight: 700; line-height: 1.3;
  max-width: 100%; padding: 0 12px;
  background: linear-gradient(100deg, var(--ly-a), var(--ly-b) 50%, var(--ly-c));
  -webkit-background-clip: text; background-clip: text;
  -webkit-text-fill-color: transparent; color: transparent;
}

/* 弹幕:固定几条轨道,各自 CSS 无限横飞;JS 只在一轮结束(animationiteration)换句 */
.ly-text-marquee {
  position: absolute; inset: 0; width: 100%; height: 100%;
  overflow: hidden; pointer-events: none;
}
.ly-marquee-lane {
  position: absolute; left: 0; top: var(--ly-top, 20%);
  white-space: nowrap; will-change: transform;
  font-family: 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Noto Sans SC', system-ui, sans-serif;
  font-size: clamp(16px, 2.6vw, 34px); font-weight: 600; line-height: 1.2;
  color: color-mix(in srgb, var(--ly-a) 62%, transparent);
  text-shadow: 0 2px 14px rgba(0, 0, 0, 0.55);
  transform: translateX(100vw);
  animation: ly-marquee-run var(--ly-dur, 16s) linear var(--ly-delay, 0s) infinite;
  animation-play-state: var(--ly-play, paused);
}
.ly-marquee-lane.active {
  color: #fff; font-weight: 800;
  text-shadow: 0 0 16px var(--ly-a), 0 2px 18px rgba(0, 0, 0, 0.6);
}
@keyframes ly-marquee-run {
  from { transform: translateX(100vw); }
  to { transform: translateX(calc(-100% - 24px)); }
}
@media (max-width: 768px) {
  .ly-marquee-lane { font-size: clamp(14px, 4vw, 22px); }
}
@media (prefers-reduced-motion: reduce) {
  .ly-marquee-lane { animation: none; transform: none; }
  .ly-text-marquee { display: flex; flex-direction: column; align-items: center; justify-content: center; }
  .ly-marquee-lane { position: relative; top: auto; }
}
@media (max-width: 768px) {
  .ly-text-stage { width: 90vw; gap: 8px; transform: translateY(-7vh); }
  .ly-text-para { max-width: 88vw; max-height: 46vh; }
  .ly-text-line { font-size: clamp(14px, 4vw, 20px); }
}
@media (prefers-reduced-motion: reduce) {
  .ly-text-ch { animation: none; }
  .ly-text-line, .ly-text-line.active.ly-focus { transition: none; animation: none; }
}
`
  document.head.appendChild(el)
}

/** 把一行字拆成 span,每个字带固定序号做错峰波浪(--i) */
function charsNode(text) {
  const frag = document.createDocumentFragment()
  const chars = [...String(text || '')]
  chars.forEach((ch, i) => {
    const span = document.createElement('span')
    span.className = 'ly-text-ch'
    span.style.setProperty('--i', String(i))
    span.textContent = ch
    frag.appendChild(span)
  })
  return frag
}

/** 没有歌词时,歌名就是主角 */
function idleNode(title) {
  const el = document.createElement('div')
  el.className = 'ly-text-idle'
  el.textContent = title || ''
  return el
}

export function createLyricTextFx({ container, getAnalyser, styleId = 'bounce', isMobile = false }) {
  if (!container || typeof document === 'undefined') return null
  injectStyle()

  const root = document.createElement('div')
  root.className = 'ly-text-fx'
  const stage = document.createElement('div')
  stage.className = 'ly-text-stage'
  root.appendChild(stage)
  container.appendChild(root)

  let cfg = getLyricStyle(styleId)
  let currentStyleId = cfg.id
  let running = false
  let timer = 0
  let bins = null
  let lineText = ''
  let titleText = ''
  let paraLines = []
  let paraActive = -1
  let lastBass = -1
  // 弹幕:固定几条轨道,每条有自己的 DOM/字幕指针/动画序号。
  // DOM 只在切风格或换歌(force)时重建;换句靠 animationiteration 事件驱动。
  const MARQUEE_LANES = isMobile ? 5 : 7
  let lanes = []   // { el, index, cycle }
  let marqueeSeed = 0

  function setEffect(id) {
    const s = getLyricStyle(id)
    cfg = s
    currentStyleId = s.id
    root.dataset.effect = s.effect || 'bounce'
    stage.classList.toggle('as-marquee', s.effect === 'marquee')
    if (s.effect !== 'marquee') {
      delete root.dataset.marquee
      lanes = []
    } else {
      marqueeSeed = Math.random() * 4
    }
    render(true)
    return s.id
  }

  function fill(node) {
    stage.replaceChildren(node)
  }

  /** 弹幕:把整段歌词铺到固定几条轨道上,错峰横飞。换句只改 textContent。 */
  function renderMarquee(force) {
    const lines = paraLines.length ? paraLines : (titleText ? [titleText] : [])
    if (force || lanes.length !== MARQUEE_LANES || root.dataset.marquee !== '1') {
      // 必须包一层 .ly-text-marquee(absolute inset:0):否则 lane 直接挂在 flex
      // 居中的 stage 下,top:% 相对一个高度塌陷的盒子,所有轨道会挤在中间一行。
      const wrap = document.createElement('div')
      wrap.className = 'ly-text-marquee'
      lanes = []
      for (let i = 0; i < MARQUEE_LANES; i++) {
        const el = document.createElement('div')
        el.className = 'ly-marquee-lane'
        // 轨道从上到下均匀分布,时长/延迟错峰,不会排成一堵墙
        el.style.setProperty('--ly-top', `${6 + (i / MARQUEE_LANES) * 88}%`)
        el.style.setProperty('--ly-dur', `${13 + (i % 5) * 2.6}s`)
        el.style.setProperty('--ly-delay', `${-(i * 2.9 + marqueeSeed).toFixed(2)}s`)
        // 一轮飞完(出屏)才换下一句:JS 几秒写一次 DOM,不逐帧
        el.addEventListener('animationiteration', () => {
          const cur = paraLines.length ? paraLines : (titleText ? [titleText] : [])
          if (!cur.length) return
          const lane = lanes[i]
          if (!lane) return
          lane.index = (lane.index + MARQUEE_LANES + 1) % cur.length
          el.textContent = cur[lane.index]
          el.classList.toggle('active', lane.index === paraActive)
        })
        lanes.push({ el, index: 0 })
        wrap.appendChild(el)
      }
      root.dataset.marquee = '1'
      fill(wrap)
    }
    // 分配句子:每条轨道从当前句往后错开,保证一开局就能看到当前句
    lanes.forEach((lane, i) => {
      const cur = paraLines.length ? paraLines : (titleText ? [titleText] : [])
      if (!cur.length) { lane.el.textContent = ''; return }
      let idx
      if (force) {
        idx = ((paraActive < 0 ? 0 : paraActive) + i) % cur.length
        lane.index = idx
      } else {
        idx = lane.index % cur.length
      }
      const text = cur[idx]
      if (lane.el.textContent !== text) lane.el.textContent = text
      lane.el.classList.toggle('active', idx === paraActive)
    })
  }

  function render(force = false) {
    const effect = cfg.effect || 'bounce'
    if (effect === 'marquee') {
      renderMarquee(force)
      return
    }
    if (effect === 'paragraph') {
      if (paraLines.length) {
        const wrap = document.createElement('div')
        wrap.className = 'ly-text-para'
        // 只渲染当前句前后各两句:DOM 数量恒定,长歌词也不会堆几百个节点
        const from = Math.max(0, paraActive - 2)
        const to = Math.min(paraLines.length, paraActive + 3)
        for (let i = from; i < to; i++) {
          const row = document.createElement('div')
          const active = i === paraActive
          row.className = active ? 'ly-text-line active' : 'ly-text-line'
          row.style.setProperty('--ly-op', active ? '1' : String(Math.max(0.18, 0.72 - Math.abs(paraActive - i) * 0.22)))
          if (active && !force) row.classList.add('ly-focus')
          if (active) row.appendChild(charsNode(paraLines[i]))
          else row.textContent = paraLines[i]
          wrap.appendChild(row)
        }
        const title = document.createElement('div')
        title.className = 'ly-text-title'
        title.textContent = titleText
        const frag = document.createDocumentFragment()
        frag.appendChild(title)
        frag.appendChild(wrap)
        fill(frag)
      } else {
        fill(idleNode(titleText))
      }
      return
    }

    // bounce:当前句 + 上面一行歌名
    const frag = document.createDocumentFragment()
    if (lineText) {
      const title = document.createElement('div')
      title.className = 'ly-text-title'
      title.textContent = titleText
      frag.appendChild(title)
      const cur = document.createElement('div')
      cur.className = 'ly-text-current'
      cur.appendChild(charsNode(lineText))
      frag.appendChild(cur)
    } else {
      frag.appendChild(idleNode(titleText))
    }
    fill(frag)
  }

  function sampleBass() {
    const an = getAnalyser?.()
    if (!an || !running) return
    if (!bins || bins.length !== an.frequencyBinCount) bins = new Uint8Array(an.frequencyBinCount)
    an.getByteFrequencyData(bins)
    let sum = 0
    const end = Math.min(bins.length, 24)
    for (let i = 1; i < end; i++) sum += bins[i]
    const bass = Math.min(1, sum / Math.max(1, end - 1) / 255)
    // 只在变化时写;静音段 120ms 一次的写入也省掉
    if (Math.abs(bass - lastBass) > 0.012) {
      lastBass = bass
      root.style.setProperty('--ly-bass', bass.toFixed(3))
    }
  }

  function start() {
    if (running) return
    running = true
    root.style.display = ''
    root.style.setProperty('--ly-play', 'running')
    if (getAnalyser?.()) {
      sampleBass()
      timer = window.setInterval(sampleBass, 120)
    }
  }

  function stop() {
    running = false
    // 切回粒子风格时文字层要彻底藏掉,不能只暂停动画留在画面上
    root.style.display = 'none'
    root.style.setProperty('--ly-play', 'paused')
    if (timer) { clearInterval(timer); timer = 0 }
  }

  function dispose() {
    stop()
    if (root.parentNode === container) container.removeChild(root)
  }

  setEffect(styleId)

  return {
    start,
    stop,
    dispose,
    get styleId() { return currentStyleId },
    setStyle: setEffect,
    /** 当前行(跳动模式用) */
    setLine(text) {
      const next = text || ''
      if (next === lineText) return
      lineText = next
      // 弹幕走整段歌词(setContext),段落走上下文;只有跳动需要当前行重建
      const effect = cfg.effect || 'bounce'
      if (effect === 'bounce') render(true)
    },
    setTitle(title) {
      const next = String(title || '')
      if (next === titleText) return
      titleText = next
      render(true)
    },
    /** 段落模式:整段歌词 + 当前句下标。只在换行时由页面调用。 */
    setContext(lines, activeIndex) {
      paraLines = Array.isArray(lines) ? lines.map(l => l?.text ?? String(l ?? '')) : []
      paraActive = activeIndex | 0
      const effect = cfg.effect || 'bounce'
      if (effect === 'paragraph') render(false)
      else if (effect === 'marquee') render(false)
    },
    /** 站点配色 / 画面采样主色,只需要一个强调色,CSS 里派生明暗 */
    setPalette(p) {
      const c = typeof p === 'string' ? p : p?.a
      if (c) root.style.setProperty('--ly-accent', c)
    },
    // 调试/测试探针
    _debug: { root, stage, get effect() { return cfg.effect }, get line() { return lineText } },
  }
}
