import { createRouter, createWebHistory } from 'vue-router'
import { refreshAdmin, useAdmin } from '../composables/useAdmin.js'

// 前端路由表。
//
// 曲库是站点主体，歌词库与歌词发现是后台能力：
//   /music             曲库 —— 访客可听歌
//   /music/visualizer  沉浸播放 —— 全屏页，访客可听歌
//   /lyrics            歌词库 —— 后台（管理）
//   /discover          歌词发现 —— 后台（管理）
//   /admin/login       管理入口
//
// 关于 useMediaPlayer：
//   它是模块级单例（state / audioEl 都在模块作用域），所以曲库页放的歌，
//   切到沉浸播放页还在放——这正是两个页面共用同一个 player 的原因。
//
// 关于后台守卫：
//   真正的权限判定在后端（Cookie 会话），前端守卫只是避免访客点进后台页看到一屏报错。
//   守卫放行与否都不影响安全性，后端才是唯一裁判。

const routes = [
  // 根路径直接渲染曲库，不再 301/跳转（消除 / → /music 的重定向）。
  {
    path: '/',
    name: 'Home',
    component: () => import('../views/MusicLibraryPage.vue'),
  },
  {
    path: '/music',
    name: 'MusicLibrary',
    component: () => import('../views/MusicLibraryPage.vue'),
  },
  {
    path: '/music/visualizer',
    name: 'MusicVisualizer',
    component: () => import('../views/VisualizerPage.vue'),
    // 沉浸播放是全屏页，不显示顶部导航，同时锁死视口（不需要页面滚动）。
    meta: { immersive: true, fullscreen: true },
  },
  {
    path: '/lyrics',
    name: 'LyricsLibrary',
    component: () => import('../views/LyricsLibraryPage.vue'),
    meta: { admin: true },
  },
  {
    path: '/discover',
    name: 'Discover',
    component: () => import('../views/LyricDiscoverPage.vue'),
    meta: { admin: true },
  },
  {
    path: '/admin/login',
    name: 'AdminLogin',
    component: () => import('../views/AdminLogin.vue'),
    meta: { immersive: true },
  },
  { path: '/:pathMatch(.*)*', redirect: '/music' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 后台页面的守卫：不是管理就送去登录页。
router.beforeEach(async (to) => {
  if (!to.meta?.admin) return true
  const { isAdmin } = useAdmin()
  if (isAdmin.value) return true
  // 身份还没查过（首屏直接输后台地址）就先查一次。
  const authed = await refreshAdmin()
  if (authed) return true
  return { name: 'AdminLogin', query: { redirect: to.fullPath } }
})

export default router
