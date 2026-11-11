<script setup>
/**
 * 后台登录。
 *
 * 只有配了 admin.password 才进得来；没配的话后端会直接拒绝，
 * 这里只是提前把话说清楚，别让人对着一个永远失败的表单发呆。
 */
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { login, useAdmin } from '../composables/useAdmin.js'

const route = useRoute()
const router = useRouter()
const { enabled, refreshAdmin } = useAdmin()

const password = ref('')
const busy = ref(false)

// 路由守卫把来路塞进 ?redirect=（例如直接输 /discover 被拦下）。
// 只接受站内绝对路径：`//evil.com` 这种协议相对写法会被浏览器当外链，必须挡掉。
function target() {
  const to = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  return to.startsWith('/') && !to.startsWith('//') ? to : '/lyrics'
}

onMounted(async () => {
  const authed = await refreshAdmin()
  if (authed) router.replace(target())
})

async function submit() {
  if (!password.value.trim()) return ElMessage.warning('先填口令')
  busy.value = true
  try {
    await login(password.value)
    ElMessage.success('已进入后台')
    router.replace(target())
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login">
    <form class="login-card" @submit.prevent="submit">
      <div class="login-badge">ADMIN</div>
      <h1 class="login-title">后台管理</h1>
      <p class="login-sub">
        歌词库、歌词发现与曲库增删改都在后台。
        访客只需要能听歌，这里是给管理用的。
      </p>

      <template v-if="enabled === false">
        <p class="login-warn">
          服务端没有配置 <code>admin.password</code>，后台未启用。
        </p>
        <RouterLink class="login-back" to="/music">← 回到音乐库</RouterLink>
      </template>

      <template v-else>
        <el-input
          v-model="password"
          type="password"
          show-password
          placeholder="管理口令"
          size="large"
          autofocus
          @keyup.enter="submit"
        />
        <el-button
          type="primary"
          size="large"
          class="login-submit"
          :loading="busy"
          native-type="submit"
        >
          进入后台
        </el-button>
        <RouterLink class="login-back" to="/music">← 回到音乐库</RouterLink>
      </template>
    </form>
  </div>
</template>

<style scoped>
.login {
  min-height: 100dvh;
  display: grid;
  place-items: center;
  padding: 24px;
  background: #0b0d12;
}

.login-card {
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

.login-badge {
  align-self: flex-start;
  padding: 3px 10px;
  border-radius: 99px;
  border: 1px solid rgba(242, 89, 42, 0.4);
  color: #ff8b5c;
  font-size: 11px;
  letter-spacing: 0.16em;
}

.login-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: #f2f4f9;
}

.login-sub {
  margin: 0 0 4px;
  font-size: 13px;
  line-height: 1.65;
  color: #8a93a5;
}

.login-warn {
  margin: 0;
  padding: 12px 14px;
  border-radius: 10px;
  background: rgba(255, 179, 71, 0.1);
  border: 1px solid rgba(255, 179, 71, 0.25);
  color: #ffc46b;
  font-size: 13px;
  line-height: 1.6;
}

.login-warn code {
  font-family: 'SF Mono', Menlo, monospace;
  font-size: 12px;
}

.login-submit {
  width: 100%;
  margin-top: 4px;
}

.login-back {
  align-self: center;
  font-size: 12px;
  color: #6f7889;
}

.login-back:hover {
  color: #a9b2c2;
}

@media (max-width: 768px) {
  /* 手机键盘弹起时视口变矮，卡片靠上放，避免输入框被顶到屏幕外 */
  .login { place-items: start center; padding: 16px; }
  .login-card { padding: 22px 18px; gap: 12px; }
  .login-title { font-size: 20px; }
}
</style>
