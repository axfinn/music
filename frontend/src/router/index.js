import { createRouter, createWebHistory } from 'vue-router'
import { refreshAdmin, useAdmin } from '../composables/useAdmin.js'

// 前端路由表。
//
// 三个页面各自独立：
//   /music             音乐库 —— 访客可听歌
//   /lyrics            歌词库 —— 后台
//   /discover          发现流 —— 后台
//   /music/visualizer  沉浸播放 —— 访客可听歌
//
// 关于 useMediaPlayer：
//   它是模块级单例（state / audioEl 都在模块作用域），所以音乐库页放的歌，
//   切到沉浸播放页还在放——这正是两个页面共用同一个 player 的原因。
//
// 关于后台守卫：
//   真正的权限判定在后端（Cookie 会话），前端守卫只是避免访客点进后台页看到一屏报错。
//   守卫放行与否都不影响安全性，后端才是唯一裁判。

const routes = [
  { path: '/', redirect: '/music' },
  {
    path: '/music',
    name: 'MusicLibrary',
    component: () => import('../views/MusicLibraryTool.vue'),
  },
  {
    path: '/music/visualizer',
    name: 'MusicVisualizer',
    component: () => import('../views/MusicVisualizerTool.vue'),
    // 沉浸播放是全屏页，不显示顶部导航。
    meta: { immersive: true },
  },
  {
    path: '/lyrics',
    name: 'LyricsLibrary',
    component: () => import('../views/LyricsLibraryTool.vue'),
    meta: { admin: true },
  },
  {
    path: '/discover',
    name: 'Discover',
    component: () => import('../views/DiscoverTool.vue'),
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
