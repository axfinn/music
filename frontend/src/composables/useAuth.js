/**
 * 邮箱会员身份（与管理员 useAdmin 完全独立）。
 *
 * 会员靠邮箱验证码注册/登录，用于识别「你是谁」；管理员靠口令进后台。
 * 两者 Cookie 不同、后端密钥不同，互不授权。
 *
 * 身份靠 HttpOnly Cookie 证明，前端只缓存「当前会员是谁」，供顶栏与页面渲染用；
 * 真正的权限判定永远在后端。访客不需要登录就能听歌，这里只影响会员专属入口。
 */
import { ref, computed } from 'vue'
import { request, json } from '../api/client.js'

// user         : 当前会员公开信息（id/email/nickname/created_at），未登录为 null
// available    : 服务端是否开放了邮箱注册/登录（取决于是否配置邮件服务）
// checked      : 是否已经查过一次会话
const user = ref(null)
const available = ref(false)
const checked = ref(false)

/** 查一次当前会员身份。应用启动时调一次即可。 */
export async function refreshAuth() {
  try {
    const data = await request('/api/auth/session')
    available.value = !!data.available
    user.value = data.authenticated && data.user ? data.user : null
  } catch {
    available.value = false
    user.value = null
  } finally {
    checked.value = true
  }
  return user.value
}

/** 发送验证码。purpose: register / login / reset。 */
export async function sendCode(email, purpose) {
  return request('/api/auth/code', json({ email, purpose }))
}

/** 注册：验证码 + 密码，成功即登录。 */
export async function register(email, code, password, nickname = '') {
  const data = await request('/api/auth/register', json({ email, code, password, nickname }))
  user.value = data.user || null
  return data
}

/** 密码登录。 */
export async function login(email, password) {
  const data = await request('/api/auth/login', json({ email, password }))
  user.value = data.user || null
  return data
}

/** 验证码登录（免密）。 */
export async function loginByCode(email, code) {
  const data = await request('/api/auth/login/code', json({ email, code }))
  user.value = data.user || null
  return data
}

/** 找回密码：验证码 + 新密码，成功即登录。 */
export async function resetPassword(email, code, password) {
  const data = await request('/api/auth/reset', json({ email, code, password }))
  user.value = data.user || null
  return data
}

/** 改昵称。 */
export async function updateProfile(nickname) {
  const data = await request('/api/auth/profile', json({ nickname }))
  if (data.user) user.value = data.user
  return data
}

/** 登出：清掉会员 Cookie。 */
export async function logout() {
  try {
    await request('/api/auth/logout', { method: 'POST' })
  } finally {
    user.value = null
  }
}

export function useAuth() {
  return {
    user,
    available,
    checked,
    isMember: computed(() => !!user.value),
    refreshAuth,
    sendCode,
    register,
    login,
    loginByCode,
    resetPassword,
    updateProfile,
    logout,
  }
}
