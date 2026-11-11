<script setup>
/**
 * 歌词发现（后台）。
 *
 * 发现流把「本地库里已有的」和「还没拿到的」混在一条流里推给用户：
 * 随机一首歌、随机一句摘句、艺人榜、高频词、收了却还没歌词的缺口，
 * 以及在线候选池里还没收录的歌。
 *
 * 在线部分由后端后台刷新（内存候选池），这里只负责取流 + 展示 + 收库。
 */
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useAdmin } from '../composables/useAdmin.js'
import { request, json } from '../api/client.js'

const { enabled } = useAdmin()

const items = ref([])
const stats = ref(null)
const pool = ref(null)
const loading = ref(false)
const savingId = ref('')

const query = ref('')
const onlineResult = ref(null)
const searching = ref(false)

const KIND_LABEL = {
  local_song: '本地歌曲',
  fragment: '摘句',
  artist: '艺人',
  word: '高频词',
  needs_lyrics: '缺歌词',
  online: '在线候选',
}

// 错误文案与 Cookie 携带统一交给 src/api/client.js。
const api = request

async function loadFeed() {
  loading.value = true
  try {
    const data = await api('/api/discovery/feed?limit=30')
    items.value = data.items || []
    stats.value = data.stats || null
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    loading.value = false
  }
}

async function loadPool() {
  try {
    pool.value = await api('/api/discovery/status')
  } catch { /* 池状态拿不到不影响主流程 */ }
}

async function refreshPool() {
  try {
    const data = await api('/api/discovery/refresh', { method: 'POST' })
    ElMessage.success(data.started ? '已开始刷新候选池' : data.message || '刷新已在进行中')
    setTimeout(loadPool, 800)
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function searchOnline() {
  if (!query.value.trim()) return ElMessage.warning('先填歌名或「歌名 - 艺人」')
  searching.value = true
  try {
    const data = await api(`/api/discovery/online?q=${encodeURIComponent(query.value.trim())}&limit=5`)
    onlineResult.value = data
    if (!data.item) ElMessage.info('没搜到')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    searching.value = false
  }
}

/** 把在线结果收进歌词库 */
async function saveOnline(item) {
  if (!item) return
  savingId.value = item.song
  try {
    const res = await api('/api/discovery/save', json({
      song: item.song,
      artist: item.singer,
      lyrics: item.lyrics || '',
      source: item.engine || '在线发现',
    }))
    if (res.duplicate) {
      ElMessage.info(`库里已经有《${item.song}》了`)
    } else {
      ElMessage.success(`已收进库：${res.saved?.title || item.song}`)
    }
    onlineResult.value = null
    loadFeed()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    savingId.value = ''
  }
}

async function saveFromFeed(it) {
  // 在线候选：按歌名再抓一次正文后入库
  await saveOnline({ song: it.title, artist: it.artist, engine: it.source, lyrics: '' })
}

const statCards = computed(() => {
  if (!stats.value) return []
  return [
    { label: '本地歌词', value: stats.value.local_songs ?? 0 },
    { label: '缺歌词的曲', value: stats.value.missing_lyrics ?? 0 },
    { label: '在线候选', value: stats.value.online_candidates ?? 0 },
  ]
})

onMounted(() => {
  loadFeed()
  loadPool()
})
</script>

<template>
  <div class="dc">
    <header class="dc-head">
      <div>
        <div class="dc-badge">DISCOVERY</div>
        <h1 class="dc-title">歌词发现</h1>
        <p class="dc-sub">
          本地已有的与还没拿到的混在一条流里。随便逛逛，顺手把想收的收进来。
        </p>
      </div>
      <div class="dc-head-actions">
        <el-button :loading="loading" @click="loadFeed">换一批</el-button>
        <el-button type="primary" :loading="pool?.refreshing" @click="refreshPool">刷新在线候选池</el-button>
      </div>
    </header>

    <section v-if="statCards.length" class="dc-stats">
      <div v-for="s in statCards" :key="s.label" class="dc-stat">
        <span class="dc-stat-num">{{ s.value }}</span>
        <span class="dc-stat-label">{{ s.label }}</span>
      </div>
      <div class="dc-pool">
        <span class="dc-pool-label">候选池</span>
        <span class="dc-pool-num">{{ pool?.pool_size ?? '—' }}</span>
        <span v-if="pool && !pool.ready" class="dc-pool-warn">抓取组件未就绪</span>
        <span v-else-if="pool?.pool_error" class="dc-pool-warn">{{ pool.pool_error }}</span>
      </div>
    </section>

    <section class="dc-search">
      <el-input
        v-model="query"
        placeholder="在线搜一首：歌名，或「歌名 - 艺人」"
        clearable
        @keyup.enter="searchOnline"
      >
        <template #append>
          <el-button :loading="searching" @click="searchOnline">搜索</el-button>
        </template>
      </el-input>
    </section>

    <section v-if="onlineResult" class="dc-online">
      <div class="dc-online-head">
        <strong>{{ onlineResult.item?.song }}</strong>
        <span class="dc-online-artist">{{ onlineResult.item?.singer }}</span>
        <span v-if="onlineResult.saved" class="dc-tag">已在库</span>
        <span v-else-if="onlineResult.in_music" class="dc-tag">音乐库已有</span>
      </div>
      <pre class="dc-online-text">{{ onlineResult.preview || '(无正文预览)' }}</pre>
      <div class="dc-online-actions">
        <el-button
          v-if="!onlineResult.saved"
          type="primary"
          :loading="savingId === onlineResult.item?.song"
          @click="saveOnline(onlineResult.item)"
        >
          收进歌词库
        </el-button>
        <el-button @click="onlineResult = null">关闭</el-button>
      </div>
    </section>

    <section v-if="items.length" class="dc-grid">
      <article v-for="it in items" :key="it.kind + it.id" class="dc-card">
        <div class="dc-card-head">
          <span class="dc-kind" :class="`dc-kind--${it.kind}`">{{ KIND_LABEL[it.kind] || it.kind }}</span>
          <span v-if="it.source" class="dc-source">{{ it.source }}</span>
        </div>

        <h3 class="dc-card-title">{{ it.title }}</h3>
        <p v-if="it.artist" class="dc-card-artist">{{ it.artist }}</p>
        <p v-if="it.text" class="dc-card-text">{{ it.text }}</p>
        <p v-if="it.meta" class="dc-card-meta">{{ it.meta }}</p>

        <div class="dc-card-actions">
          <template v-if="it.kind === 'online'">
            <el-button
              size="small"
              type="primary"
              :disabled="it.saved"
              :loading="savingId === it.title"
              @click="saveFromFeed(it)"
            >
              {{ it.saved ? '已在库' : '收进歌词库' }}
            </el-button>
            <a v-if="it.url" class="dc-link" :href="it.url" target="_blank" rel="noreferrer">来源</a>
          </template>
          <template v-else-if="it.kind === 'needs_lyrics'">
            <el-button size="small" type="primary" @click="query = `${it.title} - ${it.artist || ''}`.trim(); searchOnline()">
              在线补歌词
            </el-button>
          </template>
          <template v-else-if="it.kind === 'fragment'">
            <el-button size="small" @click="navigator.clipboard?.writeText(it.text)">复制</el-button>
          </template>
          <template v-else-if="it.kind === 'word'">
            <el-button size="small" @click="query = it.title; searchOnline()">按这个词找歌</el-button>
          </template>
          <template v-else-if="it.kind === 'artist'">
            <el-button size="small" @click="query = it.title; searchOnline()">搜这个艺人</el-button>
          </template>
        </div>
      </article>
    </section>

    <p v-else-if="!loading" class="dc-empty">
      发现流还没有素材 —— 先去歌词库收录几首，或者在上方搜一首收进来。
    </p>

    <p v-if="enabled === false" class="dc-warn">
      服务端没有配置 admin.password，发现接口已禁用。
    </p>
  </div>
</template>

<style scoped>
.dc {
  min-height: 100%;
  padding: 28px 24px 60px;
  background: #0b0d12;
  color: #e8eaf0;
}

.dc-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
  max-width: 1180px;
  margin: 0 auto 20px;
}

.dc-badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 99px;
  border: 1px solid rgba(139, 92, 246, 0.4);
  color: #a78bfa;
  font-size: 11px;
  letter-spacing: 0.16em;
}

.dc-title {
  margin: 10px 0 6px;
  font-size: 26px;
  font-weight: 750;
}

.dc-sub {
  margin: 0;
  color: #8a93a5;
  font-size: 13px;
  line-height: 1.6;
}

.dc-head-actions {
  display: flex;
  gap: 8px;
}

.dc-stats {
  display: flex;
  align-items: stretch;
  gap: 12px;
  flex-wrap: wrap;
  max-width: 1180px;
  margin: 0 auto 16px;
}

.dc-stat {
  flex: 1 1 140px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px 16px;
  border-radius: 12px;
  background: #151921;
  border: 1px solid rgba(255, 255, 255, 0.07);
}

.dc-stat-num {
  font-size: 22px;
  font-weight: 750;
}

.dc-stat-label {
  font-size: 11px;
  color: #6f7889;
}

.dc-pool {
  flex: 1 1 200px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 12px;
  background: #151921;
  border: 1px solid rgba(255, 255, 255, 0.07);
  font-size: 13px;
}

.dc-pool-label {
  color: #6f7889;
  font-size: 11px;
}

.dc-pool-num {
  font-weight: 700;
}

.dc-pool-warn {
  color: #ffc46b;
  font-size: 12px;
}

.dc-search {
  max-width: 1180px;
  margin: 0 auto 16px;
}

.dc-online {
  max-width: 1180px;
  margin: 0 auto 16px;
  padding: 14px 16px;
  border-radius: 12px;
  background: #151921;
  border: 1px solid rgba(167, 139, 250, 0.28);
}

.dc-online-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.dc-online-artist {
  color: #8a93a5;
  font-size: 13px;
}

.dc-online-text {
  margin: 0 0 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #0d1017;
  color: #c9d1de;
  font-family: 'SF Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  max-height: 190px;
  overflow-y: auto;
}

.dc-online-actions {
  display: flex;
  gap: 8px;
}

.dc-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
  max-width: 1180px;
  margin: 0 auto;
}

.dc-card {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 15px 16px;
  border-radius: 14px;
  background: #151921;
  border: 1px solid rgba(255, 255, 255, 0.07);
}

.dc-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.dc-kind {
  padding: 2px 8px;
  border-radius: 99px;
  font-size: 11px;
  background: rgba(255, 255, 255, 0.07);
  color: #9aa3b5;
}

.dc-kind--online {
  background: rgba(56, 189, 248, 0.16);
  color: #7dd3fc;
}

.dc-kind--needs_lyrics {
  background: rgba(255, 179, 71, 0.16);
  color: #ffc46b;
}

.dc-kind--fragment {
  background: rgba(167, 139, 250, 0.16);
  color: #c4b5fd;
}

.dc-source {
  font-size: 11px;
  color: #5f6878;
}

.dc-card-title {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
}

.dc-card-artist {
  margin: 0;
  font-size: 12px;
  color: #8a93a5;
}

.dc-card-text {
  margin: 0;
  color: #b9c1cf;
  font-size: 13px;
  line-height: 1.65;
}

.dc-card-meta {
  margin: 0;
  font-size: 11px;
  color: #6f7889;
}

.dc-card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: auto;
  padding-top: 4px;
}

.dc-link {
  font-size: 12px;
  color: #7dd3fc;
}

.dc-empty,
.dc-warn {
  max-width: 1180px;
  margin: 24px auto;
  color: #6f7889;
  font-size: 13px;
}

.dc-warn {
  color: #ffc46b;
}

/* 后台页原本按桌面写，窄屏下补齐：单列卡片、统计两列、按钮组换行 */
@media (max-width: 768px) {
  .dc { padding: 18px 12px calc(40px + env(safe-area-inset-bottom, 0px)); }
  .dc-head { gap: 12px; }
  .dc-title { font-size: 22px; }
  .dc-head-actions, .dc-online-actions, .dc-card-actions { flex-wrap: wrap; }
  .dc-stats { gap: 8px; }
  .dc-stat { flex: 1 1 calc(50% - 8px); padding: 10px 12px; }
  .dc-pool { flex-basis: 100%; flex-wrap: wrap; }
  .dc-grid { grid-template-columns: 1fr; }
  .dc-online-text { font-size: 11.5px; max-height: 160px; }
  .dc-online { padding: 12px; }
}
</style>
