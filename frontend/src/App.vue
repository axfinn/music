<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAdmin } from './composables/useAdmin.js'
import { loadSite, onSiteChange, setDocumentTitle, siteInfo } from './site.js'

const route = useRoute()
const { isAdmin, enabled, refreshAdmin, logout } = useAdmin()

// 站名/标语来自 /api/site（后端 config.site），不写死在前端产物里。
const site = ref(siteInfo())
onSiteChange(info => { site.value = info })

// 曲库是站点主体，访客也能进；歌词库与歌词发现属于后台，仅管理可见。
const navs = computed(() => {
  const list = [
    { path: '/music', label: '曲库' },
    { path: '/music/visualizer', label: '沉浸播放' },
  ]
  if (isAdmin.value) {
    list.push({ path: '/lyrics', label: '歌词库' })
    list.push({ path: '/discover', label: '歌词发现' })
  }
  return list
})

const SECTION_TITLES = {
  MusicLibrary: '曲库',
  MusicVisualizer: '沉浸播放',
  LyricsLibrary: '歌词库',
  Discover: '歌词发现',
  AdminLogin: '管理入口',
}
watch(() => route.name, name => setDocumentTitle(SECTION_TITLES[name] || ''), { immediate: true })

// 沉浸播放是全屏页，不显示顶部导航。
const immersive = computed(() => route.meta?.immersive === true)
// 只有沉浸播放需要锁死视口；管理登录虽然也不显示顶栏，但键盘弹起时
// 必须能滚动，否则手机上输入框会被软键盘盖住。
const fullscreen = computed(() => route.meta?.fullscreen === true)

function isActive(path) {
  return route.path === path
}

onMounted(() => {
  loadSite()
  refreshAdmin()
})
</script>

<template>
  <div class="app" :class="{ 'app--immersive': fullscreen }">
    <header v-if="!immersive" class="app-bar">
      <RouterLink class="app-brand" to="/music" :title="site.tagline">
        <span class="app-brand-mark">♪</span>
        <span class="app-brand-name">{{ site.name }}</span>
        <span class="app-brand-tag">{{ site.tagline }}</span>
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
  /* 之前用 height:100dvh + overflow:hidden 把整页锁死在一屏：内容超出后
     既没有滚动条、也无法用手指滑动，手机上连曲目列表和播放控制都够不着。
     改成 min-height，由 body 承担页面滚动；只有沉浸播放页仍锁死全屏。 */
  min-height: 100dvh;
  overflow: visible;
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.app-bar {
  /* 曲库页有 position:fixed 的氛围背景层，会盖在普通文档流之上；
     顶栏必须显式抬高层级，否则跨页导航/管理入口在任何视口下都看不见。 */
  position: sticky;
  top: 0;
  z-index: 30;
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

.app-brand-tag {
  padding-left: 9px;
  border-left: 1px solid rgba(255, 255, 255, 0.14);
  font-size: 12px;
  font-weight: 400;
  color: #8a93a5;
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

@media (max-width: 720px) {
  .app-bar { gap: 8px; padding: 0 8px; }
  .app-brand { gap: 6px; }
  /* 窄屏只留 ♪ 标记，把宽度让给导航；管理员会有 4 个入口，靠横滑保证都可达 */
  .app-brand-name, .app-brand-tag { display: none; }
  .app-nav {
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
  }
  .app-nav::-webkit-scrollbar { display: none; }
  .app-nav-link { padding: 5px 8px; font-size: 12px; }
  .app-right { flex: none; gap: 2px; }
  /* 管理员态右侧的「切换身份」在手机上会挤掉「歌词发现」，
     退出后台后再从管理入口登录即可，这里把宽度让给主导航。 */
  .app-right .app-nav-link { display: none; }
  .app-exit { font-size: 12px; }
}

/* 沉浸播放页铺满整个视口，不留导航与滚动条。 */
.app--immersive {
  height: 100dvh;
  min-height: 0;
  padding-bottom: 0;
  overflow: hidden;
}
</style>
