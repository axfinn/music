<script setup>
import { computed, h, onMounted, ref, watch } from 'vue'
import { ElNotification } from 'element-plus'
import { useRoute } from 'vue-router'
import { useAdmin } from './composables/useAdmin.js'
import { useAuth } from './composables/useAuth.js'
import { loadSite, onSiteChange, setDocumentTitle, siteInfo } from './site.js'
import SupportDialog from './components/SupportDialog.vue'
import { useSupportNudge, supportSnooze, supportDisable } from './composables/useSupportNudge.js'

const route = useRoute()
const { isAdmin, enabled, refreshAdmin, logout } = useAdmin()
const { isMember, available: authAvailable, user: authUser, refreshAuth } = useAuth()

// 站名/标语来自 /api/site（后端 config.site），不写死在前端产物里。
const site = ref(siteInfo())
onSiteChange(info => { site.value = info })

// 曲库是站点主体，访客也能进；歌词库与歌词发现属于后台，仅管理可见。
const navs = computed(() => {
  const list = [
    { path: '/music', label: '曲库' },
    { path: '/playlists', label: '歌单' },
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
  Playlists: '我的歌单',
  MusicVisualizer: '沉浸播放',
  LyricsLibrary: '歌词库',
  Discover: '歌词发现',
  AdminLogin: '管理入口',
}
watch(() => route.name, name => {
  const title = name === 'Auth' ? (isMember.value ? '我的账号' : '会员登录') : (SECTION_TITLES[name] || '')
  setDocumentTitle(title)
}, { immediate: true })

// 沉浸播放是全屏页，不显示顶部导航。
const immersive = computed(() => route.meta?.immersive === true)
// 只有沉浸播放需要锁死视口；管理登录虽然也不显示顶栏，但键盘弹起时
// 必须能滚动，否则手机上输入框会被软键盘盖住。
const fullscreen = computed(() => route.meta?.fullscreen === true)

function isActive(path) {
  return route.path === path
}

// ── 赞助入口 ──
// 常驻一个「☕ 支持」按钮（顶栏右侧，移动端只留图标），再把「听满三首歌」
// 的温柔提醒挂到播放器上：非阻塞通知，点到才弹对话框，且只提醒一次。
const supportVisible = ref(false)

function showSupportNudge() {
  const btn = (bg, border, label, onClick) =>
    h(
      'button',
      {
        onClick: (e) => {
          e.stopPropagation() // 别让通知整体的 onClick 也把对话框弹出来
          onClick()
        },
        style: {
          padding: '5px 12px',
          borderRadius: '8px',
          border: `1px solid ${border}`,
          background: bg,
          color: '#fff',
          fontSize: '12.5px',
          fontWeight: '600',
          cursor: 'pointer',
        },
      },
      label,
    )

  const instance = ElNotification({
    title: '陪你听了 3 首啦 🎧',
    duration: 12000,
    position: 'bottom-right',
    showClose: true,
    message: h('div', { style: 'font-size:13px;line-height:1.6;color:#c3c9d6;' }, [
      h(
        'div',
        { style: 'margin-bottom:10px;' },
        '这个站纯属作者用爱发电，服务器带宽要花钱。要是觉得还不错，请作者喝杯咖啡？☕',
      ),
      h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap;' }, [
        btn('linear-gradient(135deg,#f2592a,#ffb055)', 'transparent', '☕ 去加电', () => {
          supportVisible.value = true
          instance.close()
        }),
        btn('#2a2f3a', 'rgba(255,255,255,0.14)', '7天内不提醒', () => {
          supportSnooze(7)
          instance.close()
        }),
        btn('#2a2f3a', 'rgba(255,255,255,0.14)', '不再提醒', () => {
          supportDisable()
          instance.close()
        }),
      ]),
    ]),
    // 点通知正文（非按钮）也能直接打开赞助页
    onClick: () => {
      supportVisible.value = true
    },
  })
}

onMounted(() => {
  loadSite()
  refreshAdmin()
  refreshAuth()
  // 注册一次即可：内部用模块级标记保证只触发一回。
  useSupportNudge(showSupportNudge)
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
        <button
          class="app-support"
          type="button"
          title="支持作者，请喝杯咖啡 ☕"
          @click="supportVisible = true"
        >
          <span class="app-support-ico">☕</span>
          <span class="app-support-txt">支持</span>
        </button>

        <!-- 邮箱会员入口：与管理员隔离，未开放邮件服务时不显示 -->
        <template v-if="authAvailable">
          <RouterLink v-if="isMember" class="app-member" to="/login" :title="authUser.email">
            <span class="app-member-ico">◉</span>
            <span class="app-member-name">{{ authUser.nickname || authUser.email.split('@')[0] }}</span>
          </RouterLink>
          <RouterLink v-else class="app-member" to="/login">
            <span class="app-member-ico">○</span>
            <span class="app-member-name">登录</span>
          </RouterLink>
        </template>

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

    <SupportDialog v-model="supportVisible" />
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
  height: var(--app-bar-h, 48px);
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

/* 会员入口：访客/会员都能看到，与管理员入口分开放置。 */
.app-member {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 10px;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  color: #c9d1e0;
  font-size: 13px;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease;
}
.app-member:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}
.app-member-ico { color: #ff8b5c; font-size: 12px; line-height: 1; }
.app-member-name { max-width: 90px; overflow: hidden; text-overflow: ellipsis; }

/* 常驻的「☕ 支持」入口：胶囊按钮，悬停微亮，不抢视觉。 */
.app-support {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-left: 6px;
  padding: 5px 12px;
  border: 1px solid rgba(242, 89, 42, 0.5);
  border-radius: 999px;
  background: rgba(242, 89, 42, 0.12);
  color: #ffb055;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, filter 0.15s ease;
}
.app-support:hover {
  background: rgba(242, 89, 42, 0.22);
  border-color: rgba(242, 89, 42, 0.85);
  filter: brightness(1.05);
}
.app-support:active {
  filter: brightness(0.95);
}
.app-support-ico {
  font-size: 14px;
  line-height: 1;
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
  /* 会员入口窄屏只留图标，把宽度让给主导航；点图标依旧进会员页。 */
  .app-member { padding: 5px 8px; margin-left: 2px; }
  .app-member-name { display: none; }
  .app-exit { font-size: 12px; }
  /* 窄屏只留 ☕ 图标，把宽度让给导航。 */
  .app-support { padding: 5px 9px; margin-left: 2px; }
  .app-support-txt { display: none; }
}

/* 沉浸播放页铺满整个视口，不留导航与滚动条。 */
.app--immersive {
  height: 100dvh;
  min-height: 0;
  padding-bottom: 0;
  overflow: hidden;
}

/* ── 桌面宽屏：把全站顶栏放大一号，别像后台工具栏 ──
   只改尺寸/排版，不改配色：后台的歌词发现页是深色，深色顶栏在两边都成立。
   断点取 1241px 是为了避开曲库里 max-width:1240px 的旧规则，不产生重叠。 */
@media (min-width: 1241px) {
  .app-bar {
    gap: 26px;
    padding: 0 30px;
    background: linear-gradient(180deg, #161a24 0%, #11141c 100%);
  }
  .app-brand { gap: 10px; font-size: 15px; }
  .app-brand-mark { width: 28px; height: 28px; border-radius: 9px; font-size: 15px; }
  .app-brand-tag { font-size: 13px; }
  .app-nav { gap: 6px; }
  .app-nav-link { padding: 7px 14px; font-size: 14px; }
  .app-support { padding: 7px 15px; font-size: 14px; }
  .app-exit { font-size: 14px; }
}
</style>
