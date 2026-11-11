/**
 * 站点身份（站名 / 标语 / 域名）。
 *
 * 独立站的品牌由后端 config 的 site 段决定，运行时从 /api/site 拉一次。
 * 这是一个纯 JS 模块，不依赖 Vue：顶栏、document meta、系统媒体通知都能读，
 * 换站名或域名不用重新构建前端。index.html 里另有静态兜底值，
 * 保证首屏与接口失败时分享卡片依然正确。
 */
import { request } from './api/client.js'

export const SITE_FALLBACK = {
  name: 'music',
  tagline: '独立音乐站',
  description: '私人曲库与歌词档案：上传、流式播放、行级歌词检索与发现流。',
  base_url: 'https://music.jaxiu.cn',
}

let state = { ...SITE_FALLBACK }
const listeners = new Set()
let pending = null

export function siteInfo() {
  return { ...state }
}

/** 订阅站点信息变化；返回取消订阅函数。 */
export function onSiteChange(fn) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

/** 系统媒体通知里的「专辑」位，用来标站点身份。 */
export function siteAlbum() {
  return `${state.name} · ${state.tagline}`
}

/** 设置页面标题：有分区时 "曲库 · music"，首页则 "music · 独立音乐站"。 */
export function setDocumentTitle(section = '') {
  if (typeof document === 'undefined') return
  document.title = section
    ? `${section} · ${state.name}`
    : `${state.name} · ${state.tagline}`
}

function setMeta(selector, attr, value) {
  if (typeof document === 'undefined' || !value) return
  let el = document.head.querySelector(selector)
  if (!el) {
    el = document.createElement('meta')
    const match = selector.match(/^meta\[([^=]+)="([^"]+)"\]$/)
    if (match) el.setAttribute(match[1], match[2])
    document.head.appendChild(el)
  }
  el.setAttribute(attr, value)
}

function applyDocumentMeta() {
  if (typeof document === 'undefined') return
  const base = state.base_url.replace(/\/+$/, '')
  setMeta('meta[name="description"]', 'content', state.description)
  setMeta('meta[property="og:site_name"]', 'content', state.name)
  setMeta('meta[property="og:description"]', 'content', state.description)
  setMeta('meta[property="og:url"]', 'content', `${base}/`)
  setMeta('meta[property="og:title"]', 'content', `${state.name} · ${state.tagline}`)
  const canonical = document.head.querySelector('link[rel="canonical"]')
  if (canonical) canonical.setAttribute('href', `${base}/`)
}

/** 应用启动时调用一次即可；重复调用共用同一个 Promise。 */
export function loadSite() {
  if (pending) return pending
  pending = request('/api/site')
    .then(data => {
      state = {
        name: data.name || state.name,
        tagline: data.tagline || state.tagline,
        description: data.description || state.description,
        base_url: data.base_url || state.base_url,
      }
      applyDocumentMeta()
      setDocumentTitle()
      const snapshot = siteInfo()
      listeners.forEach(fn => fn(snapshot))
      return snapshot
    })
    .catch(() => siteInfo()) // 静态兜底已足够，站点信息不该阻塞界面
  return pending
}
