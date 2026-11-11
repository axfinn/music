/**
 * 管理身份（后台）。
 *
 * 站点分两种身份：
 *   - 访客：只能浏览与播放音乐库；
 *   - 管理：额外能进后台 —— 歌词库、歌词发现、曲库增删改。
 *
 * 身份靠后端下发的会话 Cookie 证明（HttpOnly，前端读不到也改不了）。
 * 这里只缓存「当前是不是管理」这个布尔值，供导航与路由守卫用；
 * 真正的权限判定永远在后端，前端只是据此决定显示什么。
 */
import { ref, computed } from 'vue'
import { request, json } from '../api/client.js'

// enabled  : 后端有没有配置管理口令。没配就没有后台，登录页也不该出现。
// authenticated: 当前浏览器带的是不是有效会话。
const enabled = ref(false)
const authenticated = ref(false)
const checked = ref(false)

/** 查一次当前身份。应用启动时调一次即可。 */
export async function refreshAdmin() {
  try {
    const data = await request('/api/admin/session')
    enabled.value = !!data.enabled
    authenticated.value = !!data.authenticated
  } catch {
    enabled.value = false
    authenticated.value = false
  } finally {
    checked.value = true
  }
  return authenticated.value
}

/** 管理员登录。口令对了才发会话 Cookie。 */
export async function login(password) {
  const data = await request('/api/admin/login', json({ password }))
  enabled.value = true
  authenticated.value = !!data.authenticated
  return authenticated.value
}

/** 登出：清掉会话 Cookie。 */
export async function logout() {
  try {
    await request('/api/admin/logout', { method: 'POST' })
  } finally {
    authenticated.value = false
  }
}

export function useAdmin() {
  return {
    enabled,
    authenticated,
    checked,
    isAdmin: computed(() => enabled.value && authenticated.value),
    refreshAdmin,
    login,
    logout,
  }
}
