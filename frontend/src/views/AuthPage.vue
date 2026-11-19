<script setup>
/**
 * 邮箱会员认证页：登录 / 注册 / 找回密码 三态。
 *
 * 混合模式：
 *   - 注册：邮箱 → 收验证码 → 设密码（一步完成，成功后即登录）
 *   - 登录：邮箱 + 密码；也可切「验证码登录」收码免密登入
 *   - 找回：邮箱 → 收验证码 → 设新密码
 *
 * 会员与管理员是两套身份：这里只签会员 Cookie，拿不到后台权限；
 * 后台入口在 /admin/login，与本页互不相干。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  login, loginByCode, register, resetPassword, sendCode, useAuth, logout,
} from '../composables/useAuth.js'

const route = useRoute()
const router = useRouter()
const { refreshAuth, user } = useAuth()

// 三个主模式：login / register / reset；登录下再分密码 / 验证码两种凭证。
const mode = ref('login')
const credential = ref('password') // password | code

const email = ref('')
const password = ref('')
const code = ref('')
const nickname = ref('')
const busy = ref(false)
const sending = ref(false)
const countdown = ref(0)
let timer = null

// 路由守卫/顶栏塞进来的 ?redirect= 只接受站内绝对路径。
function target() {
  const to = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  return to.startsWith('/') && !to.startsWith('//') ? to : '/music'
}

const title = computed(() => ({
  login: '会员登录', register: '注册会员', reset: '找回密码',
}[mode.value]))
const codeLabel = computed(() => (mode.value === 'register' ? '验证码' : '邮箱验证码'))

function switchMode(next) {
  mode.value = next
  password.value = ''
  code.value = ''
  stopCountdown()
}

function startCountdown() {
  countdown.value = 60
  stopCountdown()
  timer = setInterval(() => {
    countdown.value = Math.max(0, countdown.value - 1)
    if (countdown.value === 0) stopCountdown()
  }, 1000)
}
function stopCountdown() {
  if (timer) clearInterval(timer)
  timer = null
}
onUnmounted(stopCountdown)

async function doSendCode() {
  if (!email.value.trim()) return ElMessage.warning('先填邮箱')
  sending.value = true
  try {
    await sendCode(email.value.trim(), mode.value)
    ElMessage.success('验证码已发送，注意查收（含垃圾邮件箱）')
    startCountdown()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    sending.value = false
  }
}

async function submit() {
  if (!email.value.trim()) return ElMessage.warning('先填邮箱')
  if (mode.value === 'login' && credential.value === 'password' && !password.value) {
    return ElMessage.warning('先填密码')
  }
  if (!code.value.trim()) return ElMessage.warning('先填验证码')
  if ((mode.value === 'register' || mode.value === 'reset') && !password.value) {
    return ElMessage.warning('先填密码')
  }
  busy.value = true
  try {
    const mail = email.value.trim()
    if (mode.value === 'register') {
      await register(mail, code.value.trim(), password.value, nickname.value.trim())
      ElMessage.success('注册成功，欢迎回来')
    } else if (mode.value === 'reset') {
      await resetPassword(mail, code.value.trim(), password.value)
      ElMessage.success('密码已重置')
    } else if (credential.value === 'code') {
      await loginByCode(mail, code.value.trim())
      ElMessage.success('登录成功')
    } else {
      await login(mail, password.value)
      ElMessage.success('登录成功')
    }
    router.replace(target())
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  refreshAuth()
})

async function doLogout() {
  await logout()
  ElMessage.success('已退出登录')
  mode.value = 'login'
  email.value = ''
}
</script>

<template>
  <div class="auth">
    <!-- 已登录：展示会员信息，提供退出；不再强制跳转 -->
    <div v-if="user" class="auth-card">
      <div class="auth-badge">MEMBER</div>
      <h1 class="auth-title">我的账号</h1>
      <p class="auth-sub">会员身份用来记住你的偏好；不登录也能照常听歌。</p>
      <div class="auth-profile">
        <div><span>邮箱</span><strong>{{ user.email }}</strong></div>
        <div><span>昵称</span><strong>{{ user.nickname || '—' }}</strong></div>
      </div>
      <el-button size="large" @click="doLogout">退出登录</el-button>
      <RouterLink class="auth-back" to="/music">← 回到音乐库</RouterLink>
    </div>

    <form v-else class="auth-card" @submit.prevent="submit">
      <div class="auth-badge">MEMBER</div>
      <h1 class="auth-title">{{ title }}</h1>
      <p class="auth-sub">
        会员身份用来记住你的偏好；不登录也能照常听歌。
      </p>

      <!-- 模式切换 -->
      <div class="auth-tabs">
        <button type="button" :class="{ on: mode === 'login' }" @click="switchMode('login')">登录</button>
        <button type="button" :class="{ on: mode === 'register' }" @click="switchMode('register')">注册</button>
        <button type="button" :class="{ on: mode === 'reset' }" @click="switchMode('reset')">找回密码</button>
      </div>

      <el-input
        v-model="email"
        type="email"
        placeholder="邮箱地址"
        size="large"
        autofocus
      />

      <!-- 登录：密码 / 验证码 二选一 -->
      <template v-if="mode === 'login'">
        <div class="auth-cred-switch">
          <button type="button" :class="{ on: credential === 'password' }" @click="credential = 'password'">密码登录</button>
          <button type="button" :class="{ on: credential === 'code' }" @click="credential = 'code'">验证码登录</button>
        </div>
        <el-input
          v-if="credential === 'password'"
          v-model="password"
          type="password"
          show-password
          placeholder="密码"
          size="large"
          @keyup.enter="submit"
        />
        <template v-else>
          <div class="auth-code-row">
            <el-input v-model="code" placeholder="邮箱验证码" size="large" @keyup.enter="submit" />
            <el-button size="large" :disabled="countdown > 0" :loading="sending" @click="doSendCode">
              {{ countdown > 0 ? `${countdown}s` : '获取验证码' }}
            </el-button>
          </div>
        </template>
      </template>

      <!-- 注册：验证码 + 密码 + 可选昵称 -->
      <template v-else-if="mode === 'register'">
        <el-input v-model="nickname" placeholder="昵称（可选）" size="large" maxlength="32" />
        <div class="auth-code-row">
          <el-input v-model="code" placeholder="邮箱验证码" size="large" @keyup.enter="submit" />
          <el-button size="large" :disabled="countdown > 0" :loading="sending" @click="doSendCode">
            {{ countdown > 0 ? `${countdown}s` : '获取验证码' }}
          </el-button>
        </div>
        <el-input
          v-model="password"
          type="password"
          show-password
          placeholder="设置密码（至少 6 位）"
          size="large"
          @keyup.enter="submit"
        />
      </template>

      <!-- 找回：验证码 + 新密码 -->
      <template v-else>
        <div class="auth-code-row">
          <el-input v-model="code" placeholder="邮箱验证码" size="large" @keyup.enter="submit" />
          <el-button size="large" :disabled="countdown > 0" :loading="sending" @click="doSendCode">
            {{ countdown > 0 ? `${countdown}s` : '获取验证码' }}
          </el-button>
        </div>
        <el-input
          v-model="password"
          type="password"
          show-password
          placeholder="新密码（至少 6 位）"
          size="large"
          @keyup.enter="submit"
        />
      </template>

      <el-button
        type="primary"
        size="large"
        class="auth-submit"
        :loading="busy"
        native-type="submit"
      >
        {{ mode === 'register' ? '注册并登录' : mode === 'reset' ? '重置密码' : '登录' }}
      </el-button>

      <RouterLink class="auth-back" to="/music">← 回到音乐库</RouterLink>
    </form>
  </div>
</template>

<style scoped>
.auth {
  min-height: 100dvh;
  display: grid;
  place-items: center;
  padding: 24px;
  background: #0b0d12;
}

.auth-card {
  width: min(420px, 100%);
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 30px 28px 26px;
  border-radius: 18px;
  background: #151921;
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.45);
}

.auth-badge {
  align-self: flex-start;
  padding: 3px 10px;
  border-radius: 99px;
  border: 1px solid rgba(242, 89, 42, 0.4);
  color: #ff8b5c;
  font-size: 11px;
  letter-spacing: 0.16em;
}

.auth-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: #f2f4f9;
}

.auth-sub {
  margin: 0 0 2px;
  font-size: 13px;
  line-height: 1.6;
  color: #8a93a5;
}

.auth-tabs,
.auth-cred-switch {
  display: flex;
  gap: 6px;
}

.auth-tabs button,
.auth-cred-switch button {
  flex: 1;
  padding: 7px 0;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  background: transparent;
  color: #9aa3b5;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.auth-tabs button.on,
.auth-cred-switch button.on {
  background: rgba(242, 89, 42, 0.16);
  border-color: rgba(242, 89, 42, 0.55);
  color: #ffb055;
}

.auth-profile {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.07);
}
.auth-profile > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}
.auth-profile span { color: #8a93a5; }
.auth-profile strong { color: #e8eaf0; font-weight: 500; word-break: break-all; }

.auth-code-row {
  display: flex;
  gap: 8px;
}

.auth-code-row :deep(.el-button) {
  flex: none;
  white-space: nowrap;
}

.auth-submit {
  width: 100%;
  margin-top: 4px;
}

.auth-back {
  align-self: center;
  font-size: 12px;
  color: #6f7889;
}

.auth-back:hover {
  color: #a9b2c2;
}

@media (max-width: 768px) {
  .auth { place-items: start center; padding: 16px; }
  .auth-card { padding: 22px 18px; gap: 12px; }
  .auth-title { font-size: 20px; }
}
</style>
