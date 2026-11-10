<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAdmin } from './composables/useAdmin.js'

const route = useRoute()
const { isAdmin, enabled, refreshAdmin, logout } = useAdmin()

// 沉浸播放是全屏页，不显示顶部导航。
const immersive = computed(() => route.meta?.immersive === true)

// 只有管理才看得到后台入口（歌词库 / 发现）。
const navs = computed(() => {
  const list = [{ path: '/music', label: '音乐库' }]
  if (isAdmin.value) {
    list.push({ path: '/lyrics', label: '歌词库' })
    list.push({ path: '/discover', label: '发现' })
  }
  return list
})

function isActive(path) {
  return route.path === path || (path === '/music' && route.path.startsWith('/music'))
}

onMounted(refreshAdmin)
</script>

<template>
  <div class="app" :class="{ 'app--immersive': immersive }">
    <header v-if="!immersive" class="app-bar">
      <RouterLink class="app-brand" to="/music">
        <span class="app-brand-mark">♪</span>
        <span>音乐库 · 歌词库</span>
      </RouterLink>

      <nav class="app-nav">
        <RouterLink
          v-for="n in navs"
          :key="n.path"
          class="app-nav-link"
          :class="{ active: isActive(n.path) }"
          :to="n.path"
        >
          {{ n.label }}
        </RouterLink>
      </nav>

      <div class="app-right">
        <template v-if="isAdmin">
          <RouterLink class="app-nav-link" to="/admin/login">切换身份</RouterLink>
          <el-button link class="app-exit" @click="logout">退出后台</el-button>
        </template>
        <RouterLink v-else-if="enabled" class="app-nav-link" to="/admin/login">管理入口</RouterLink>
      </div>
    </header>

    <!--
      页面用 keep-alive 缓存：音乐库页的 onActivated/onDeactivated 负责
      挂摘全局键盘监听（空格播放/暂停），没有 keep-alive 就不触发，
      快捷键会失效。key 用 route.fullPath，让 /music?play=xxx 也能命中
      新的页面实例（歌词库页点「去音乐库播放」就是这么跳的）。
    -->
    <RouterView v-slot="{ Component }">
      <keep-alive>
        <component :is="Component" :key="route.fullPath" />
      </keep-alive>
    </RouterView>
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100dvh;
  overflow: hidden;
}

.app-bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 18px;
  height: 48px;
  padding: 0 18px;
  background: #11141c;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.app-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 650;
  color: #e8eaf0;
  white-space: nowrap;
}

.app-brand-mark {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  background: linear-gradient(135deg, #f2592a, #ffb055);
  color: #fff;
  font-size: 13px;
}

.app-nav {
  display: flex;
  align-items: center;
  gap: 4px;
}

.app-nav-link {
  padding: 5px 12px;
  border-radius: 8px;
  font-size: 13px;
  color: #9aa3b5;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease;
}

.app-nav-link:hover {
  background: rgba(255, 255, 255, 0.06);
  color: #e8eaf0;
}

.app-nav-link.active {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}

.app-right {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}

.app-exit {
  color: #9aa3b5;
  font-size: 13px;
}

/* 沉浸播放页铺满整个视口，不留导航与滚动条。 */
.app--immersive {
  overflow: hidden;
}
</style>
