// 配色方案 —— 视觉播放页唯一的"风格开关"。
// 每个方案 3 个渐变端色(a → b → c),场景只认这三个色,不再各自硬编码颜色,
// 换方案 = 整页气质全换(极简白 / 霓虹 / 日落 ...)。
export const PALETTES = [
  {
    id: 'aurora',
    label: '极光',
    hint: '青蓝紫的冷调渐变,最耐看',
    a: '#6ee7d7',
    b: '#4d9dff',
    c: '#b07cff',
  },
  {
    id: 'mono',
    label: '极简',
    hint: '近乎黑白的低饱和,克制、时尚',
    a: '#ffffff',
    b: '#c7d0e0',
    c: '#7c879b',
  },
  {
    id: 'neon',
    label: '霓虹',
    hint: '酸性青 + 品红,像赛博招牌',
    a: '#00ffc6',
    b: '#00a3ff',
    c: '#ff2fb3',
  },
  {
    id: 'sunset',
    label: '日落',
    hint: '暖橘到玫瑰红,情绪饱满',
    a: '#ffd166',
    b: '#ff7a59',
    c: '#ff2e73',
  },
  {
    id: 'ice',
    label: '冰川',
    hint: '冷白与深蓝,安静通透',
    a: '#e8f6ff',
    b: '#7cc6ff',
    c: '#2b5cff',
  },
  {
    id: 'ember',
    label: '余烬',
    hint: '金铜色,像暗房里的暖灯',
    a: '#ffe9b0',
    b: '#ffa63d',
    c: '#c2410c',
  },
]

export function getPalette(id) {
  return PALETTES.find(p => p.id === id) || PALETTES[0]
}
