// 歌词特效风格表 —— 纯数据 + 纯函数,不碰 DOM / WebGL。
//
// 为什么单独一个模块:页面要持久化/循环/随机,渲染层要拿参数;
// 两个地方各写一份 id 迟早对不上。这里做唯一的真相来源。
//
// 三类渲染器:
//   'particles' —— 复用 lyricfx.js 里同一套 WebGL 粒子/同一块材质,切风格只改
//                  uniform 和几个 CPU 数字:不多开 context、不重建 buffer、不加 draw call。
//                  经典 / 涟漪 / 落沙(自由落体) / 星轨 / 爆炸 都属于这一类。
//   'text'      —— 轻量 DOM/CSS 文字层(跳动 / 段落 / 弹幕),完全不占 WebGL,
//                  动画交给 CSS transform/opacity 的合成层,JS 每帧零 DOM 写入。
//   'physics'   —— lyricPhysicsFx.js 的 2D canvas 刚体层:字块受重力、落地反弹、
//                  互相碰撞、随低频被顶起、鼠标可推开。自研轻量物理,不引第三方引擎。
// 三者同一时刻只跑一个,页面切风格不会两边同时烧 GPU/CPU。

export const RANDOM_STYLE_ID = 'random'

// particles 的 style: 传给 shader 的 uStyle,uniform 分支,所有顶点走同一条路径。
//   0 经典 / 1 涟漪 / 2 落沙(自由落体) / 3 星轨 / 4 爆炸
// pointSize/opacity/arc/sway: 手感差异,切换时直接写 uniform。
// idleBeats: 拼歌名点缀的间隔(拍)。落沙/爆炸有自己的编排,给 0 表示不拼歌名。
// text 的 effect: 文字层用的动画类型(bounce / paragraph / marquee)。
// physics 无额外参数:字块大小/重力在 lyricPhysicsFx.js 里按屏宽自适应。
export const LYRIC_STYLES = [
  {
    id: 'classic',
    label: '经典',
    hint: '粒子逐字轻跳，偶尔拼成歌名（默认）',
    renderer: 'particles',
    style: 0,
    pointSize: 0.05,
    opacity: 0.7,
    arc: 1,
    sway: 0.1,
    idleBeats: 40,
  },
  {
    id: 'ripple',
    label: '涟漪',
    hint: '粒子以字心为圆心，跟着低频一圈圈荡开',
    renderer: 'particles',
    style: 1,
    pointSize: 0.052,
    opacity: 0.72,
    arc: 1.15,
    sway: 0.07,
    idleBeats: 30,
  },
  {
    id: 'fall',
    label: '落沙',
    hint: '字化成沙自由落下、触底堆叠，随拍点扬起',
    renderer: 'particles',
    style: 2,
    pointSize: 0.051,
    opacity: 0.72,
    arc: 0.9,
    sway: 0.04,
    idleBeats: 0,          // 落沙自转,不拼歌名
  },
  {
    id: 'orbit',
    label: '星轨',
    hint: '粒子绕每个字缓慢旋转，像一小团星云',
    renderer: 'particles',
    style: 3,
    pointSize: 0.048,
    opacity: 0.7,
    arc: 0.8,
    sway: 0.14,
    idleBeats: 36,
  },
  {
    id: 'explode',
    label: '爆聚',
    hint: '鼓点把字炸成星屑，再被磁力吸回重组',
    renderer: 'particles',
    style: 4,
    pointSize: 0.05,
    opacity: 0.74,
    arc: 1.4,
    sway: 0.05,
    idleBeats: 0,          // 爆炸本身就是重组,不叠加拼歌名
  },
  {
    id: 'bounce',
    label: '跳动',
    hint: '整行字按拍点逐字错峰跳，像歌词在打拍子',
    renderer: 'text',
    effect: 'bounce',
  },
  {
    id: 'paragraph',
    label: '段落',
    hint: '整段歌词滚动聚焦，当前句放大高亮',
    renderer: 'text',
    effect: 'paragraph',
  },
  {
    id: 'marquee',
    label: '弹幕',
    hint: '整段歌词分多条轨道从右向左横飞，层层错峰',
    renderer: 'text',
    effect: 'marquee',
  },
  {
    id: 'gravity',
    label: '重力',
    hint: '字块受重力落地反弹、互相碰撞，鼠标能推开',
    renderer: 'physics',
  },
]

export function getLyricStyle(id) {
  return LYRIC_STYLES.find(s => s.id === id) || LYRIC_STYLES[0]
}

/** 存储里读回来的值也可能是旧版/手改的垃圾,统一收敛到合法 id */
export function normalizeLyricStyle(id) {
  if (id === RANDOM_STYLE_ID) return RANDOM_STYLE_ID
  return LYRIC_STYLES.some(s => s.id === id) ? id : LYRIC_STYLES[0].id
}

/**
 * 把用户选的 id 解析成真正要渲染的具体风格。
 * random 是「每曲一换」的模式:每次调用抽一个,尽量不重复上一首。
 * @param {string} id 用户选择(可能是 'random')
 * @param {{previous?: string, rng?: () => number}} options
 */
export function resolveLyricStyle(id, { previous = '', rng = Math.random } = {}) {
  if (id !== RANDOM_STYLE_ID) return getLyricStyle(id).id
  const pool = LYRIC_STYLES.filter(s => s.id !== previous)
  const list = pool.length ? pool : LYRIC_STYLES
  const raw = Number(rng())
  const r = Number.isFinite(raw) ? Math.min(0.999999, Math.max(0, raw)) : 0
  return list[Math.floor(r * list.length)].id
}

/** 移动端 chip / K 键用的顺序循环(不含 random,random 要显式点) */
export function nextLyricStyle(id) {
  const i = LYRIC_STYLES.findIndex(s => s.id === id)
  return LYRIC_STYLES[(i + 1 + LYRIC_STYLES.length) % LYRIC_STYLES.length].id
}
