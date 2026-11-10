<template>
  <div class="lyrics-app">
    <div class="bg-orb bg-orb-1" />
    <div class="bg-orb bg-orb-2" />

    <!-- Hero -->
    <header class="ly-hero">
      <div class="ly-badge">LYRICS INDEX</div>
      <h1 class="ly-title">歌<span class="accent">词</span>库</h1>
      <p class="ly-sub">把我读过的词收进来 — 按句检索、摘金句、看意象，写歌时随手取用</p>
      <div class="ly-stats" v-if="stats">
        <div class="stat"><span class="stat-num">{{ stats.total_songs }}</span><span class="stat-label">首</span></div>
        <div class="stat"><span class="stat-num">{{ stats.total_artists }}</span><span class="stat-label">位艺人</span></div>
        <div class="stat"><span class="stat-num">{{ stats.total_lines }}</span><span class="stat-label">行词</span></div>
        <div class="stat"><span class="stat-num">{{ stats.fragment_count }}</span><span class="stat-label">条摘句</span></div>
      </div>
    </header>

    <!-- Tabs -->
    <nav class="ly-tabs">
      <button
        v-for="t in tabs" :key="t.key"
        class="ly-tab" :class="{ active: tab === t.key }"
        @click="switchTab(t.key)"
      >
        <el-icon :size="15"><component :is="t.icon" /></el-icon>
        {{ t.label }}
        <span v-if="t.badge" class="tab-badge">{{ t.badge }}</span>
      </button>
    </nav>

    <!-- ── 曲库 ── -->
    <section v-show="tab === 'songs'" class="ly-body">
      <div class="ly-toolbar">
        <div class="toolbar-left">
          <el-input
            v-model="keyword" placeholder="搜歌名 / 艺人 / 歌词 / 标签..."
            :prefix-icon="Search" clearable class="search-input" @input="onKeywordInput"
          />
          <el-select v-model="artistFilter" class="narrow-select" placeholder="艺人" clearable filterable @change="loadSongs">
            <el-option v-for="a in artists" :key="a.name" :label="`${a.name} (${a.count})`" :value="a.name" />
          </el-select>
          <el-select v-model="langFilter" class="narrow-select" placeholder="语种" clearable @change="loadSongs">
            <el-option v-for="l in languageOptions" :key="l.value" :label="l.label" :value="l.value" />
          </el-select>
          <el-select v-model="genreFilter" class="narrow-select" placeholder="风格" clearable @change="loadSongs">
            <el-option v-for="g in genreOptions" :key="g.value" :label="g.label" :value="g.value" />
          </el-select>
          <button class="pill" :class="{ active: favOnly }" @click="favOnly = !favOnly; loadSongs()">
            <el-icon :size="13"><StarFilled /></el-icon> 只看收藏
          </button>
        </div>
        <div class="toolbar-right">
          <button class="ghost-pill" @click="openRandomSong"><el-icon><Refresh /></el-icon> 随机一首</button>
          <button class="ghost-pill" @click="openFetch"><el-icon><Download /></el-icon> 在线抓取</button>
          <button class="ghost-pill" @click="openImport"><el-icon><Files /></el-icon> 批量粘贴</button>
          <button class="upload-pill" @click="openCreate"><el-icon><Plus /></el-icon> 收录歌词</button>
        </div>
      </div>

      <div v-if="loading" class="ly-skeletons">
        <div v-for="i in 6" :key="i" class="skel-row" />
      </div>

      <div v-else-if="!songs.length" class="ly-empty">
        <el-icon :size="46"><Reading /></el-icon>
        <h3>{{ keyword || artistFilter || favOnly ? '没有匹配的歌词' : '库还是空的' }}</h3>
        <p>{{ keyword || artistFilter || favOnly ? '换个关键词试试，或者清掉筛选条件' : '把记得的歌词一段段存进来，之后就能按句检索了' }}</p>
        <el-button type="primary" round @click="openCreate">收录第一首</el-button>
      </div>

      <div v-else class="song-list">
        <article
          v-for="s in songs" :key="s.id"
          class="song-row" :class="{ open: expandedId === s.id }"
        >
          <div class="song-main" @click="toggleExpand(s)">
            <div class="song-mark">{{ (s.title || '?')[0] }}</div>
            <div class="song-meta">
              <div class="song-title-line">
                <span class="song-title">{{ s.title }}</span>
                <span v-if="s.favorite" class="fav-star"><el-icon :size="12"><StarFilled /></el-icon></span>
                <span v-if="s.language" class="chip chip-lang">{{ langLabel(s.language) }}</span>
                <span v-if="s.genre" class="chip">{{ genreLabel(s.genre) }}</span>
                <span v-if="s.era" class="chip chip-plain">{{ s.era }}</span>
                <span v-if="musicLinks[s.id]" class="chip chip-music" title="已在音乐库">♪ 音乐库</span>
              </div>
              <div class="song-sub">
                <span class="song-artist">{{ s.artist || '未知艺人' }}</span>
                <template v-if="s.album"> · {{ s.album }}</template>
                <span class="song-dims"> · {{ s.line_count }} 行 / {{ s.char_count }} 字</span>
              </div>
              <div v-if="s.tags && s.tags.length" class="song-tags">
                <button v-for="t in s.tags" :key="t" class="mini-tag" @click.stop="filterByTag(t)">#{{ t }}</button>
              </div>
            </div>
              <div class="song-actions" @click.stop>
                <button class="row-btn" :class="{ faved: s.favorite }" title="收藏" @click="toggleFav(s)">
                  <el-icon :size="15"><component :is="s.favorite ? 'StarFilled' : 'Star'" /></el-icon>
                </button>
                <button class="row-btn" title="生成音乐 prompt" @click="openPrompt(s)">
                  <el-icon :size="15"><MagicStick /></el-icon>
                </button>
                <button v-if="musicLinks[s.id]" class="row-btn" title="在音乐库播放" @click="playInMusicLibrary(s)">
                  <el-icon :size="15"><Headset /></el-icon>
                </button>
              <button class="row-btn" title="编辑" @click="openEdit(s)">
                <el-icon :size="15"><Edit /></el-icon>
              </button>
              <button class="row-btn danger" title="删除" @click="removeSong(s)">
                <el-icon :size="15"><Delete /></el-icon>
              </button>
            </div>
          </div>

          <div v-if="expandedId === s.id" class="song-lyrics">
            <div v-for="(line, i) in splitLines(s.lyrics)" :key="i" class="lyric-line">
              <span class="line-no">{{ i + 1 }}</span>
              <span class="line-text">{{ line }}</span>
              <button class="line-pick" title="摘这句" @click="quickFragment(s, line)">
                <el-icon :size="13"><Collection /></el-icon>
              </button>
            </div>
            <div v-if="s.note" class="song-note"><b>笔记</b>{{ s.note }}</div>
            <div class="song-footer">
              <el-button size="small" round @click="copyText(s.lyrics, '歌词已复制')">
                <el-icon><CopyDocument /></el-icon> 复制全文
              </el-button>
              <el-button size="small" round @click="openPrompt(s)">
                <el-icon><MagicStick /></el-icon> 拼成生成 prompt
              </el-button>
              <el-button size="small" round @click="openFragmentDialog(s)">
                <el-icon><Collection /></el-icon> 添加摘句
              </el-button>
            </div>
          </div>
        </article>
      </div>

      <div v-if="songs.length < total" class="load-more">
        <el-button :loading="loadingMore" round @click="loadMore">
          还有 {{ total - songs.length }} 首，加载更多
        </el-button>
      </div>
    </section>

    <!-- ── 句检索 ── -->
    <section v-show="tab === 'lines'" class="ly-body">
      <div class="line-search-box">
        <el-input
          v-model="lineKeyword" size="large" clearable
          placeholder="输入一个词或一句话，找回它出现在哪首歌的哪一行…"
          :prefix-icon="Search"
          @keyup.enter="searchLines"
        >
          <template #append>
            <el-button :loading="lineLoading" @click="searchLines">检索</el-button>
          </template>
        </el-input>
        <p class="hint">多个词用空格隔开表示「同时包含」，例如：<code>城市 雨</code></p>
      </div>

      <div v-if="lineLoading" class="ly-skeletons">
        <div v-for="i in 5" :key="i" class="skel-row" />
      </div>
      <div v-else-if="!lineHits.length" class="ly-empty">
        <el-icon :size="44"><Search /></el-icon>
        <h3>{{ lineSearched ? '没有命中的句子' : '按句检索' }}</h3>
        <p>{{ lineSearched ? '换个说法，或者只留关键词再试' : '这里会告诉每个词出在哪首歌的第几行' }}</p>
      </div>
      <div v-else class="hit-list">
        <div class="hit-head">命中 <b>{{ lineTotal }}</b> 行{{ lineTotal > lineHits.length ? `（显示前 ${lineHits.length} 行）` : '' }}</div>
        <div v-for="(h, i) in lineHits" :key="i" class="hit-row">
          <div class="hit-text" v-html="highlight(h.line_text)" />
          <div class="hit-meta">
            <span class="hit-song" @click="jumpToSong(h)">{{ h.title }}</span>
            <span class="hit-artist">{{ h.artist || '未知艺人' }}</span>
            <span class="hit-lineno">第 {{ h.line_no }} 行</span>
            <button class="row-btn" title="复制这句" @click="copyText(h.line_text, '已复制')">
              <el-icon :size="14"><CopyDocument /></el-icon>
            </button>
            <button class="row-btn" title="存为摘句" @click="quickFragment({ id: h.song_id, title: h.title, artist: h.artist }, h.line_text)">
              <el-icon :size="14"><Collection /></el-icon>
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ── 摘句 ── -->
    <section v-show="tab === 'fragments'" class="ly-body">
      <div class="ly-toolbar">
        <div class="toolbar-left">
          <el-input v-model="fragKeyword" placeholder="搜摘句内容 / 标签..." :prefix-icon="Search" clearable class="search-input" @input="onFragKeywordInput" />
          <button class="pill" :class="{ active: fragFavOnly }" @click="fragFavOnly = !fragFavOnly; loadFragments()">
            <el-icon :size="13"><StarFilled /></el-icon> 只看收藏
          </button>
        </div>
        <div class="toolbar-right">
          <button class="ghost-pill" @click="randomFragment"><el-icon><Refresh /></el-icon> 灵感一下</button>
          <button class="upload-pill" @click="openFragmentDialog(null)"><el-icon><Plus /></el-icon> 记一句</button>
        </div>
      </div>

      <div v-if="!fragments.length" class="ly-empty">
        <el-icon :size="44"><Collection /></el-icon>
        <h3>还没有摘句</h3>
        <p>读到戳你的那句，顺手记下来；写词卡住时就能从这里翻</p>
      </div>
      <div v-else class="frag-grid">
        <div v-for="f in fragments" :key="f.id" class="frag-card">
          <div class="frag-text" @click="copyText(f.text, '已复制')">{{ f.text }}</div>
          <div class="frag-meta">
            <span v-if="f.song_title" class="frag-src">{{ f.song_title }}<template v-if="f.artist"> · {{ f.artist }}</template></span>
            <span v-else class="frag-src dim">自由摘句</span>
            <span class="frag-kind">{{ kindLabel(f.kind) }}</span>
          </div>
          <div v-if="f.tags && f.tags.length" class="frag-tags">
            <span v-for="t in f.tags" :key="t" class="mini-tag">#{{ t }}</span>
          </div>
          <div class="frag-actions">
            <button class="mini-btn" @click="copyText(f.text, '已复制')"><el-icon :size="13"><CopyDocument /></el-icon></button>
            <button class="mini-btn" :class="{ faved: f.favorite }" @click="toggleFragFav(f)"><el-icon :size="13"><component :is="f.favorite ? 'StarFilled' : 'Star'" /></el-icon></button>
            <button class="mini-btn" title="编辑" @click="openFragmentDialog(null, f)"><el-icon :size="13"><Edit /></el-icon></button>
            <button class="mini-btn danger" @click="removeFragment(f)"><el-icon :size="13"><Delete /></el-icon></button>
            <span class="frag-use">用过 {{ f.use_count }} 次</span>
          </div>
        </div>
      </div>
    </section>

    <!-- ── 意象 / 高频词 ── -->
    <section v-show="tab === 'words'" class="ly-body">
      <div class="words-panel">
        <div class="panel-head">
          <h3>高频字词</h3>
          <div class="panel-actions">
            <el-radio-group v-model="wordType" size="small">
              <el-radio-button value="all">全部</el-radio-button>
              <el-radio-button value="cjk2">词组</el-radio-button>
              <el-radio-button value="cjk1">单字</el-radio-button>
              <el-radio-button value="word">英文</el-radio-button>
            </el-radio-group>
            <el-button size="small" text :loading="wordLoading" @click="loadWords"><el-icon><Refresh /></el-icon></el-button>
          </div>
        </div>
        <p class="panel-hint">从库里所有歌词里统计（已过滤 LRC 时间戳、括号注记和常见虚词）。点击任意词直接去检索它出现在哪些句子。</p>

        <div v-if="!filteredWords.length" class="ly-empty compact">
          <el-icon :size="36"><DataAnalysis /></el-icon>
          <h3>还没有足够的数据</h3>
          <p>收录几首歌词后，这里会给出你的用词偏好</p>
        </div>
        <div v-else class="word-bars">
          <button v-for="w in filteredWords" :key="w.type + w.word" class="word-bar" @click="searchWordInLines(w.word)">
            <span class="wb-word">{{ w.word }}</span>
            <span class="wb-track"><i :style="{ width: barWidth(w.count) }" /></span>
            <span class="wb-count">{{ w.count }}</span>
          </button>
        </div>
      </div>

      <div class="words-panel" v-if="stats && stats.top_tags && stats.top_tags.length">
        <div class="panel-head"><h3>标签分布</h3></div>
        <div class="tag-cloud">
          <button
            v-for="t in stats.top_tags" :key="t.name" class="cloud-tag"
            :style="cloudStyle(t.count)" @click="filterByTag(t.name)"
          >
            {{ t.name }} <span class="cloud-count">{{ t.count }}</span>
          </button>
        </div>
      </div>
    </section>

    <!-- ── 对外接口 ── -->
    <section v-show="tab === 'api'" class="ly-body">
      <div class="api-grid">
        <div class="api-card">
          <div class="panel-head"><h3>接口地址</h3><span class="panel-sub">给别的系统调这个歌词库</span></div>
          <div class="api-base">
            <code>{{ apiBase }}</code>
            <button class="mini-copy" @click="copyText(apiBase, '已复制接口地址')">复制</button>
          </div>
          <p class="api-hint">
            鉴权二选一：<code>X-API-Key: &lt;密钥&gt;</code> 或 <code>Authorization: Bearer &lt;密钥&gt;</code>。
            密钥在右边签发，撤销后立即失效；同一把密钥多台机器共用一份额度。
          </p>
          <div class="api-list">
            <div v-for="e in apiEndpoints" :key="e.method + e.path" class="api-row">
              <span class="api-method" :class="e.method.toLowerCase()">{{ e.method }}</span>
              <code class="api-path">{{ e.path }}</code>
              <span class="api-desc">{{ e.desc }}</span>
            </div>
          </div>
        </div>

        <div class="api-card">
          <div class="panel-head">
            <h3>密钥</h3>
            <button v-if="apiUnlocked" class="mini-copy" @click="loadApiKeys">刷新</button>
          </div>
          <div v-if="!apiUnlocked" class="api-lock">
            <el-button type="primary" round :loading="apiLoading" @click="loadApiKeys">加载密钥</el-button>
            <p class="api-hint small">用当前管理员身份读取密钥，无需再输密码。</p>
          </div>
          <template v-else>
            <div class="key-create">
              <el-input v-model="newKeyName" placeholder="给这把 key 起个名，如「博客脚本」" @keyup.enter="createApiKey" />
              <el-button type="primary" round :loading="apiLoading" @click="createApiKey">签发</el-button>
            </div>
            <div v-if="freshKey" class="key-fresh">
              <div class="kf-label">新密钥（只显示这一次，复制后存好）</div>
              <div class="kf-value">
                <code>{{ freshKey }}</code>
                <button class="mini-copy" @click="copyText(freshKey, '密钥已复制')">复制</button>
              </div>
            </div>
            <div v-if="!apiKeys.length" class="api-empty">还没有密钥，签发一把就能开始调用。</div>
            <div v-else class="key-list">
              <div v-for="k in apiKeys" :key="k.id" class="key-row" :class="{ off: !k.enabled }">
                <div class="key-main">
                  <span class="key-name">{{ k.name }}</span>
                  <code class="key-prefix">{{ k.prefix }}…</code>
                  <span class="key-usage">{{ k.call_count }} 次</span>
                  <span v-if="!k.enabled" class="key-off">已撤销</span>
                </div>
                <div class="key-actions">
                  <button class="mini-copy" @click="toggleKey(k)">{{ k.enabled ? '撤销' : '恢复' }}</button>
                  <button class="mini-copy danger" @click="deleteKey(k)">删除</button>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>

      <div class="api-card wide">
        <div class="panel-head">
          <h3>调用示例</h3>
          <button class="mini-copy" @click="copyText(curlSample, '示例已复制')">复制</button>
        </div>
        <pre class="api-pre">{{ curlSample }}</pre>
      </div>
    </section>

    <!-- ── 收录 / 编辑弹窗 ── -->
    <el-dialog
      v-model="formVisible" :title="form.id ? '编辑歌词' : '收录歌词'" width="720px"
      :close-on-click-modal="false" top="6vh"
    >
      <div class="form-grid">
        <el-input v-model="form.title" placeholder="歌名（必填）" class="span-2" />
        <el-input v-model="form.artist" placeholder="艺人（建议填，便于去重和聚合）" />
        <el-input v-model="form.album" placeholder="专辑 / 出处（可选）" />
        <el-select v-model="form.language" placeholder="语种" clearable filterable>
          <el-option v-for="l in languageOptions" :key="l.value" :label="l.label" :value="l.value" />
        </el-select>
        <el-select v-model="form.genre" placeholder="风格" clearable filterable>
          <el-option v-for="g in genreOptions" :key="g.value" :label="g.label" :value="g.value" />
        </el-select>
        <el-input v-model="form.era" placeholder="年代，如 2000s" />
        <el-input v-model="tagsText" placeholder="标签，逗号分隔：城市,雨,长句" />
        <el-input v-model="form.source" placeholder="来源（可选）" />
        <el-input v-model="form.lyrics" type="textarea" :rows="14" placeholder="歌词正文，一行一句；带 [00:12.34] 时间戳也能直接粘" class="span-2" />
        <el-input v-model="form.note" type="textarea" :rows="3" placeholder="创作笔记：为什么收它、句式上有什么值得学" class="span-2" />
      </div>
      <template #footer>
        <el-button @click="formVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveSong">{{ form.id ? '保存' : '收录' }}</el-button>
      </template>
    </el-dialog>

    <!-- ── 批量粘贴弹窗 ── -->
    <el-dialog v-model="importVisible" title="批量粘贴收录" width="720px" :close-on-click-modal="false" top="6vh">
      <p class="import-hint">
        用 <code>=== 艺人 | 歌名</code> 分块，每块下面是歌词正文。没写 <code>|</code> 时整行当作歌名，艺人取下方默认值。
      </p>
      <pre class="import-sample">=== 陈奕迅 | 富士山下
谁能凭爱意要富士山私有
曾沿着雪路浪游

=== 林夕 | 红豆
还没好好地感受
雪花绽放的气候</pre>

      <div class="import-file-row">
        <input
          ref="filePickerRef" type="file" multiple accept=".lrc,.txt,.md,.lyric" class="hidden-file"
          @change="onImportFiles"
        />
        <el-button size="small" round @click="filePickerRef?.click()">
          <el-icon><Files /></el-icon> 从 .lrc / .txt 文件导入
        </el-button>
        <span class="import-file-hint">文件名当作歌名，艺人取下方默认值；可一次选多份</span>
      </div>

      <div class="form-grid">
        <el-input v-model="importArtist" placeholder="默认艺人（可留空）" />
        <el-select v-model="importLanguage" placeholder="默认语种" clearable>
          <el-option v-for="l in languageOptions" :key="l.value" :label="l.label" :value="l.value" />
        </el-select>
        <el-input
          v-model="importText" type="textarea" :rows="12" class="span-2"
          placeholder="把上面那种格式的文本整段粘进来…"
        />
      </div>
      <div v-if="importPreview.length" class="import-preview">
        解析到 <b>{{ importPreview.length }}</b> 首：{{ importPreview.map(x => x.title).slice(0, 6).join('、') }}{{ importPreview.length > 6 ? ' …' : '' }}
      </div>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importPreview.length" @click="doImport">
          导入 {{ importPreview.length }} 首
        </el-button>
      </template>
    </el-dialog>

    <!-- ── 在线抓取弹窗 ── -->
    <el-dialog v-model="fetchVisible" title="在线抓取歌词" width="760px" :close-on-click-modal="false" top="6vh">
      <div class="fetch-mode-bar">
        <button class="mode-btn" :class="{ active: fetchMode === 'one' }" @click="fetchMode = 'one'">单首</button>
        <button class="mode-btn" :class="{ active: fetchMode === 'batch' }" @click="fetchMode = 'batch'">批量</button>
        <span class="mode-hint">
          {{ fetchMode === 'one' ? '按歌名 + 艺人检索，先看结果再决定收不收' : '一行一首，格式「歌名 - 艺人」' }}
        </span>
      </div>

      <div v-if="fetchReady === false" class="fetch-warn">
        抓取组件未就绪：服务端找不到 python 或 lyrics_finder.py，其余功能不受影响。
      </div>

      <template v-if="fetchMode === 'one'">
        <div class="form-grid">
          <el-input v-model="fetchForm.song" placeholder="歌名（必填）" @keyup.enter="doFetch" />
          <el-input v-model="fetchForm.artist" placeholder="艺人（强烈建议填，命中率高很多）" @keyup.enter="doFetch" />
          <el-select v-model="fetchForm.engine" placeholder="数据源">
            <el-option v-for="e in engineOptions" :key="e.value" :label="e.label" :value="e.value" />
          </el-select>
          <div class="fetch-flags">
            <el-checkbox v-model="fetchForm.raw_lrc">保留时间轴</el-checkbox>
            <el-checkbox v-model="fetchForm.strip_credits">剔除词曲署名行</el-checkbox>
            <el-checkbox v-model="fetchForm.save">抓完直接入库</el-checkbox>
          </div>
        </div>
      </template>

      <template v-else>
        <el-input
          v-model="fetchBatchText" type="textarea" :rows="7"
          placeholder="晴天 - 周杰伦&#10;倔强 - 五月天&#10;富士山下 - 陈奕迅"
        />
        <div class="fetch-flags">
          <el-checkbox v-model="fetchForm.strip_credits">剔除词曲署名行</el-checkbox>
          <span class="fetch-batch-hint">批量模式统一用「自动」数据源，一次最多 20 首</span>
        </div>
      </template>

      <!-- 单首结果 -->
      <div v-if="fetchMode === 'one' && fetchResult" class="fetch-result">
        <div class="fr-head">
          <b>{{ fetchResult.song }}</b>
          <span v-if="fetchResult.singer"> · {{ fetchResult.singer }}</span>
          <span class="fr-meta">
            {{ fetchResult.source || '未知来源' }} · {{ fetchResult.line_count }} 行 · {{ ((fetchResult.elapsed_ms || 0) / 1000).toFixed(1) }}s
          </span>
        </div>
        <el-input v-model="fetchResult.lyrics" type="textarea" :rows="11" class="fr-lyrics" />
        <div class="fr-actions">
          <el-button size="small" round @click="prefillFromFetch">收进库（可再加标签笔记）</el-button>
          <a v-if="fetchResult.url" class="fr-link" :href="fetchResult.url" target="_blank" rel="noreferrer">查看来源页</a>
        </div>
      </div>

      <!-- 批量结果 -->
      <div v-if="fetchMode === 'batch' && fetchBatchResults.length" class="fetch-result">
        <div class="fr-head">
          <b>{{ fetchBatchResults.filter(r => r.ok).length }}</b> 首成功
          <template v-if="fetchBatchResults.some(r => !r.ok)">
            ，{{ fetchBatchResults.filter(r => !r.ok).length }} 首失败
          </template>
        </div>
        <div class="fr-list">
          <div v-for="(r, i) in fetchBatchResults" :key="i" class="fr-item" :class="{ bad: !r.ok }">
            <span class="fr-q">{{ r.query }}</span>
            <span class="fr-meta">{{ r.ok ? `${r.item.line_count} 行 · ${r.item.engine}` : r.error }}</span>
          </div>
        </div>
        <el-button
          v-if="fetchBatchResults.some(r => r.ok)" size="small" round type="primary"
          :loading="savingBatch" @click="saveBatchFetched"
        >
          全部入库
        </el-button>
      </div>

      <template #footer>
        <el-button @click="fetchVisible = false">关闭</el-button>
        <el-button type="primary" :loading="fetching" :disabled="fetchReady === false" @click="doFetch">
          {{ fetchMode === 'one' ? '抓取' : '批量抓取' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- ── 生成 prompt 弹窗 ── -->
    <el-dialog v-model="promptVisible" title="生成音乐 prompt" width="760px" top="6vh">
      <div v-if="promptData" class="prompt-body">
        <div class="prompt-song">{{ promptData.item.title }}<template v-if="promptData.item.artist"> · {{ promptData.item.artist }}</template></div>

        <div class="prompt-block">
          <div class="pb-head"><span>风格描述（Style）</span><el-button size="small" text @click="copyText(promptData.style_prompt, '已复制风格')">复制</el-button></div>
          <div class="pb-body mono">{{ promptData.style_prompt }}</div>
        </div>

        <div class="prompt-block">
          <div class="pb-head"><span>结构化歌词（Lyrics）</span><el-button size="small" text @click="copyText(promptData.structured_lyrics, '已复制歌词')">复制</el-button></div>
          <pre class="pb-body lyrics-pre">{{ promptData.structured_lyrics }}</pre>
        </div>

        <el-input v-model="promptExtra" placeholder="追加风格描述（可选）：female vocal, lo-fi, 90 bpm…" class="prompt-extra" @change="reloadPrompt" />
      </div>
      <template #footer>
        <el-button @click="promptVisible = false">关闭</el-button>
        <el-button @click="copyText(promptData?.markdown || '', '完整 prompt 已复制')">复制完整 Markdown</el-button>
        <el-button type="primary" @click="copyPromptAndClose">复制完整 Prompt 并关闭</el-button>
      </template>
    </el-dialog>

    <!-- ── 摘句弹窗 ── -->
    <el-dialog v-model="fragVisible" :title="fragForm.id ? '编辑摘句' : '记一句'" width="560px">
      <div class="form-grid">
        <el-input v-model="fragForm.text" type="textarea" :rows="4" placeholder="要记下的那句（必填）" class="span-2" />
        <el-select v-model="fragForm.kind" placeholder="类型">
          <el-option v-for="k in kindOptions" :key="k.value" :label="k.label" :value="k.value" />
        </el-select>
        <el-input v-model="fragTagsText" placeholder="标签，逗号分隔" />
        <el-input v-model="fragForm.note" placeholder="为什么记它（可选）" class="span-2" />
        <el-select v-if="!fragForm.song_id" v-model="fragSongPick" placeholder="关联歌曲（可选）" clearable filterable class="span-2">
          <el-option v-for="s in songs" :key="s.id" :label="`${s.title} · ${s.artist || '未知艺人'}`" :value="s.id" />
        </el-select>
      </div>
      <template #footer>
        <el-button @click="fragVisible = false">取消</el-button>
        <el-button type="primary" :loading="fragSaving" @click="saveFragment">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script>
export default { name: 'LyricsLibraryTool' }
</script>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Search, Plus, Edit, Delete, Star, StarFilled, CopyDocument, MagicStick,
  Refresh, Reading, Collection, DataAnalysis, Files, Download, Connection,
} from '@element-plus/icons-vue'

const router = useRouter()

const languageOptions = [
  { value: 'zh', label: '国语' },
  { value: 'yue', label: '粤语' },
  { value: 'en', label: '英文' },
  { value: 'ja', label: '日语' },
  { value: 'ko', label: '韩语' },
  { value: 'other', label: '其他' },
]

const genreOptions = [
  { value: 'pop', label: '流行' },
  { value: 'rock', label: '摇滚' },
  { value: 'folk', label: '民谣' },
  { value: 'rap', label: '说唱' },
  { value: 'rnb', label: 'R&B' },
  { value: 'electronic', label: '电子' },
  { value: 'ballad', label: '抒情慢歌' },
  { value: 'other', label: '其他' },
]

const kindOptions = [
  { value: 'hook', label: 'Hooks / 记忆点' },
  { value: 'chorus', label: '副歌' },
  { value: 'verse', label: '主歌' },
  { value: 'bridge', label: '桥段' },
  { value: 'line', label: '单句' },
]

const langLabel = v => languageOptions.find(x => x.value === v)?.label || v
const genreLabel = v => genreOptions.find(x => x.value === v)?.label || v
const kindLabel = v => kindOptions.find(x => x.value === v)?.label || '单句'

// ── 通用请求 ──
// 管理员会话是 Cookie（HttpOnly），必须显式带上 credentials 才会随请求发送。
async function api(url, options = {}) {
  const res = await fetch(url, { credentials: 'same-origin', ...options })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `请求失败(${res.status})`)
  return data
}

function jsonBody(payload) {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }
}

// ── 基础状态 ──
const tab = ref('songs')
const stats = ref(null)
const tabs = computed(() => [
  { key: 'songs', label: '曲库', icon: Reading, badge: stats.value?.total_songs || 0 },
  { key: 'lines', label: '句检索', icon: Search },
  { key: 'fragments', label: '摘句', icon: Collection, badge: stats.value?.fragment_count || 0 },
  { key: 'words', label: '意象', icon: DataAnalysis },
  { key: 'api', label: '接口', icon: Connection },
])

const songs = ref([])
const total = ref(0)
const loading = ref(true)
const loadingMore = ref(false)
const keyword = ref('')
const artistFilter = ref('')
const langFilter = ref('')
const genreFilter = ref('')
const favOnly = ref(false)
const artists = ref([])
const expandedId = ref('')
const pageSize = 50

let keywordTimer = null
function onKeywordInput() {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(loadSongs, 320)
}

function songQuery(offset) {
  const p = new URLSearchParams({ limit: String(pageSize), offset: String(offset) })
  if (keyword.value.trim()) p.set('keyword', keyword.value.trim())
  if (artistFilter.value) p.set('artist', artistFilter.value)
  if (langFilter.value) p.set('language', langFilter.value)
  if (genreFilter.value) p.set('genre', genreFilter.value)
  if (favOnly.value) p.set('fav', '1')
  return `/api/lyrics/songs?${p}`
}

async function loadSongs() {
  loading.value = true
  expandedId.value = ''
  try {
    const data = await api(songQuery(0))
    songs.value = data.items || []
    total.value = Number(data.total || 0)
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    loading.value = false
  }
  refreshStats()
}

async function loadMore() {
  if (loadingMore.value) return
  loadingMore.value = true
  try {
    const data = await api(songQuery(songs.value.length))
    songs.value.push(...(data.items || []))
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    loadingMore.value = false
  }
}

async function refreshStats() {
  try {
    const data = await api('/api/lyrics/stats')
    stats.value = data.stats
  } catch {}
}

async function loadArtists() {
  try {
    const data = await api('/api/lyrics/artists')
    artists.value = data.items || []
  } catch {}
}

function switchTab(key) {
  tab.value = key
  if (key === 'fragments' && !fragments.value.length) loadFragments()
  if (key === 'words' && !words.value.length) loadWords()
}

// ── 在线抓取（服务端内置 lyrics_finder.py） ──
const fetchVisible = ref(false)
const fetchReady = ref(null)
const fetchMode = ref('one')
const fetching = ref(false)
const savingBatch = ref(false)
const fetchForm = reactive({ song: '', artist: '', engine: 'auto', raw_lrc: false, strip_credits: true, save: false })
const fetchBatchText = ref('')
const fetchResult = ref(null)
const fetchBatchResults = ref([])

const engineOptions = [
  { value: 'auto', label: '自动（卡片 › 网易云 › QQ › 结果页）' },
  { value: 'netease', label: '网易云音乐' },
  { value: 'qq', label: 'QQ 音乐' },
  { value: 'baidu', label: '百度（卡片 + 结果页）' },
  { value: 'card', label: '百度音乐卡片' },
  { value: 'page', label: '百度结果页' },
]

async function openFetch() {
  fetchVisible.value = true
  fetchResult.value = null
  fetchBatchResults.value = []
  if (fetchForm.artist === '' && artistFilter.value) fetchForm.artist = artistFilter.value
  if (fetchReady.value === null) {
    try {
      const data = await api('/api/lyrics/fetch/status')
      fetchReady.value = !!data.ready
    } catch {
      fetchReady.value = false
    }
  }
}

async function doFetch() {
  if (fetchMode.value === 'one') {
    if (!fetchForm.song.trim()) return ElMessage.warning('先填歌名')
    fetching.value = true
    try {
      const data = await api('/api/lyrics/fetch', jsonBody({ ...fetchForm }))
      if (data.saved) {
        ElMessage.success(`已收进库：${data.saved.title}`)
        fetchResult.value = null
        loadSongs()
        refreshStats()
        if (fetchForm.save) fetchVisible.value = false
      } else if (data.duplicate) {
        ElMessage.info(`库里已有「${data.item.song}」，没有重复收`)
        fetchResult.value = data.item
      } else {
        fetchResult.value = data.item
      }
    } catch (err) {
      ElMessage.error(err.message)
    } finally {
      fetching.value = false
    }
    return
  }

  const songs = fetchBatchText.value.split('\n').map(x => x.trim()).filter(Boolean)
  if (!songs.length) return ElMessage.warning('每行写一首，如「晴天 - 周杰伦」')
  if (songs.length > 20) return ElMessage.warning('一次最多 20 首，分批来吧')
  fetching.value = true
  try {
    const data = await api('/api/lyrics/fetch/batch', jsonBody({
      songs, engine: 'auto', strip_credits: fetchForm.strip_credits, save: false,
    }))
    fetchBatchResults.value = data.items || []
    const ok = fetchBatchResults.value.filter(r => r.ok).length
    if (ok) ElMessage.success(`抓到 ${ok} 首`)
    else ElMessage.warning('一首都没抓到，换个歌名/补上艺人再试')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    fetching.value = false
  }
}

function prefillFromFetch() {
  const r = fetchResult.value
  if (!r) return
  resetForm()
  Object.assign(form, {
    title: r.song || fetchForm.song,
    artist: r.singer || fetchForm.artist,
    album: r.album || '',
    language: '',
    genre: '',
    era: '',
    lyrics: r.lyrics || '',
    note: r.url ? `抓取自 ${r.url}` : '',
    source: r.source ? `在线抓取 · ${r.source}` : '在线抓取',
  })
  fetchVisible.value = false
  formVisible.value = true
}

async function saveBatchFetched() {
  const items = fetchBatchResults.value
    .filter(r => r.ok)
    .map(r => ({
      title: r.item.song,
      artist: r.item.singer,
      album: r.item.album,
      lyrics: r.item.lyrics,
      source: r.item.source ? `在线抓取 · ${r.item.source}` : '在线抓取',
      note: r.item.url ? `来源 ${r.item.url}` : '',
    }))
  if (!items.length) return
  savingBatch.value = true
  try {
    const data = await api('/api/lyrics/import', jsonBody({ items }))
    const skipped = data.skipped_count || 0
    ElMessage.success(`入库 ${data.created_count} 首${skipped ? `，跳过 ${skipped} 首（重复或缺歌名）` : ''}`)
    fetchBatchResults.value = []
    loadSongs()
    refreshStats()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    savingBatch.value = false
  }
}

// ── 对外接口 ──
const apiBase = computed(() => `${window.location.origin}/api/v1/lyrics`)
const apiEndpoints = [
  { method: 'GET', path: '/ping', desc: '鉴权自检，返回这把 key 的用量' },
  { method: 'POST', path: '/fetch', desc: '抓取一首歌词（save=true 顺带入库）' },
  { method: 'POST', path: '/fetch/batch', desc: '批量抓取，一次最多 20 首' },
  { method: 'GET', path: '/songs', desc: '列出库里歌词（keyword / artist / limit）' },
  { method: 'POST', path: '/songs', desc: '把自有歌词推进库（不经抓取）' },
  { method: 'GET', path: '/songs/:id', desc: '取单首' },
  { method: 'GET', path: '/search?q=', desc: '行级检索，返回命中行号' },
  { method: 'GET', path: '/stats', desc: '库的聚合统计' },
]

const apiUnlocked = ref(false)
const apiLoading = ref(false)
const apiKeys = ref([])
const newKeyName = ref('')
const freshKey = ref('')

const curlSample = computed(() => `curl -X POST '${apiBase.value}/fetch' \\
  -H 'X-API-Key: <你的密钥>' \\
  -H 'Content-Type: application/json' \\
  -d '{"song":"晴天","artist":"周杰伦","engine":"auto","save":true}'`)

// 密钥管理不再单独输密码：进到这一页就说明是管理员，
// 身份由会话 Cookie（HttpOnly）自动带上。
async function loadApiKeys() {
  apiLoading.value = true
  try {
    const data = await api('/api/lyrics/api-keys')
    apiKeys.value = data.items || []
    apiUnlocked.value = true
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    apiLoading.value = false
  }
}

async function createApiKey() {
  apiLoading.value = true
  try {
    const res = await fetch('/api/lyrics/api-keys', {
      ...jsonBody({ name: newKeyName.value.trim() || '未命名' }),
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(data.error || `签发失败(${res.status})`)
    freshKey.value = data.plain
    apiKeys.value.unshift(data.item)
    newKeyName.value = ''
    ElMessage.success('已签发，密钥只显示这一次')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    apiLoading.value = false
  }
}

async function toggleKey(k) {
  try {
    await api(`/api/lyrics/api-keys/${k.id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: !k.enabled }),
    })
    k.enabled = !k.enabled
    ElMessage.success(k.enabled ? '已恢复' : '已撤销，这把 key 立刻失效')
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function deleteKey(k) {
  try {
    await ElMessageBox.confirm(`删除「${k.name}」？之后用这把 key 的调用会全部失败。`, '删除密钥', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    })
  } catch {
    return
  }
  try {
    await api(`/api/lyrics/api-keys/${k.id}`, { method: 'DELETE' })
    apiKeys.value = apiKeys.value.filter(x => x.id !== k.id)
    ElMessage.success('已删除')
  } catch (err) {
    ElMessage.error(err.message)
  }
}

function toggleExpand(s) {
  expandedId.value = expandedId.value === s.id ? '' : s.id
}

// ── 与音乐库联动(反向) ──
// musicLinks: lyric_song_id → {id,title},用于给已关联的歌打标记并一键去音乐库播放。
const musicLinks = ref({})

async function loadMusicLinks() {
  try {
    const data = await api('/api/lyrics/music-links')
    const map = {}
    for (const it of (data.items || [])) {
      if (!map[it.lyric_song_id]) map[it.lyric_song_id] = { id: it.music_track_id, title: it.title }
    }
    musicLinks.value = map
  } catch {}
}

function playInMusicLibrary(s) {
  const link = musicLinks.value[s.id]
  if (!link) return
  router.push('/music?play=' + encodeURIComponent(link.id))
}

function splitLines(lyrics) {
  return String(lyrics || '').split('\n').map(l => l.trimEnd()).filter(l => l.trim() !== '')
}

function filterByTag(tag) {
  tab.value = 'songs'
  keyword.value = tag
  artistFilter.value = ''
  loadSongs()
}

// ── 收藏 / 删除 ──
async function toggleFav(s) {
  try {
    const data = await api(`/api/lyrics/songs/${s.id}/favorite`, jsonBody({ favorite: !s.favorite }))
    s.favorite = data.favorite
    refreshStats()
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function removeSong(s) {
  try {
    await ElMessageBox.confirm(`确定删除「${s.title}」？已摘的句子会保留。`, '删除歌词', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消',
    })
  } catch { return }
  try {
    await api(`/api/lyrics/songs/${s.id}`, { method: 'DELETE' })
    ElMessage.success('已删除')
    await loadSongs()
    loadArtists()
  } catch (err) {
    ElMessage.error(err.message)
  }
}

// ── 随机一首 ──
async function openRandomSong() {
  try {
    const data = await api('/api/lyrics/random')
    const song = data.item
    if (!songs.value.some(x => x.id === song.id)) {
      songs.value.unshift(song)
    }
    expandedId.value = song.id
    tab.value = 'songs'
    ElMessage.success(`随手翻到：${song.title}`)
  } catch (err) {
    ElMessage.warning(err.message)
  }
}

async function jumpToSong(hit) {
  try {
    const data = await api(`/api/lyrics/songs/${hit.song_id}`)
    const song = data.item
    tab.value = 'songs'
    if (!songs.value.some(x => x.id === song.id)) songs.value.unshift(song)
    expandedId.value = song.id
  } catch (err) {
    ElMessage.error(err.message)
  }
}

// ── 收录 / 编辑 ──
const formVisible = ref(false)
const saving = ref(false)
const tagsText = ref('')
const form = reactive({
  id: '', title: '', artist: '', album: '', language: '', genre: '', era: '',
  lyrics: '', note: '', source: '',
})

function resetForm() {
  Object.assign(form, {
    id: '', title: '', artist: '', album: '', language: '', genre: '', era: '',
    lyrics: '', note: '', source: '',
  })
  tagsText.value = ''
}

function openCreate() {
  resetForm()
  if (artistFilter.value) form.artist = artistFilter.value
  formVisible.value = true
}

function openEdit(s) {
  resetForm()
  Object.assign(form, {
    id: s.id, title: s.title, artist: s.artist || '', album: s.album || '',
    language: s.language || '', genre: s.genre || '', era: s.era || '',
    lyrics: s.lyrics || '', note: s.note || '', source: s.source || '',
  })
  tagsText.value = (s.tags || []).join(', ')
  formVisible.value = true
}

function parseTags(raw) {
  return String(raw || '')
    .split(/[,，;；/]/)
    .map(t => t.trim().toLowerCase())
    .filter((t, i, arr) => t && arr.indexOf(t) === i)
}

async function saveSong() {
  if (!form.title.trim()) {
    ElMessage.warning('歌名不能为空')
    return
  }
  saving.value = true
  const payload = {
    title: form.title.trim(),
    artist: form.artist.trim(),
    album: form.album.trim(),
    language: form.language || '',
    genre: form.genre || '',
    era: form.era.trim(),
    tags: parseTags(tagsText.value),
    lyrics: form.lyrics,
    note: form.note,
    source: form.source.trim(),
  }
  try {
    if (form.id) {
      await api(`/api/lyrics/songs/${form.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      ElMessage.success('已保存')
    } else {
      await api('/api/lyrics/songs', jsonBody(payload))
      ElMessage.success('已收录')
    }
    formVisible.value = false
    await loadSongs()
    loadArtists()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

// ── 批量粘贴 ──
const importVisible = ref(false)
const importText = ref('')
const importArtist = ref('')
const importLanguage = ref('')
const importing = ref(false)

function openImport() {
  importText.value = ''
  importArtist.value = artistFilter.value || ''
  importLanguage.value = langFilter.value || ''
  importVisible.value = true
}

// 解析规则：以 "===" 开头的行分块，块首行是 "艺人 | 歌名" 或 "歌名"。
function parseImport(text, defaultArtist, defaultLanguage) {
  const blocks = []
  let current = null
  for (const rawLine of String(text || '').split('\n')) {
    const line = rawLine.trimEnd()
    if (line.trim().startsWith('===')) {
      if (current) blocks.push(current)
      const head = line.trim().replace(/^=+\s*/, '').trim()
      const parts = head.split('|').map(x => x.trim())
      current = parts.length >= 2
        ? { artist: parts[0], title: parts[1], lines: [] }
        : { artist: defaultArtist, title: head, lines: [] }
      continue
    }
    if (!current) {
      if (line.trim()) {
        current = { artist: defaultArtist, title: '', lines: [line] }
      }
      continue
    }
    current.lines.push(line)
  }
  if (current) blocks.push(current)

  return blocks
    .map(b => ({
      title: (b.title || '').trim(),
      artist: (b.artist || '').trim() || defaultArtist,
      language: defaultLanguage,
      lyrics: b.lines.join('\n').trim(),
    }))
    .filter(b => b.title)
}

const importPreview = computed(() => parseImport(importText.value, importArtist.value.trim(), importLanguage.value))

// 从本地 .lrc / .txt 读入：文件名即歌名（去掉扩展名与「艺人-」前缀），正文原样落块。
const filePickerRef = ref(null)

async function onImportFiles(event) {
  const files = Array.from(event.target.files || [])
  if (!files.length) return
  const chunks = []
  for (const file of files) {
    try {
      const text = await file.text()
      // 文件名常见形态："陈奕迅 - 富士山下.lrc" / "富士山下.lrc"
      const base = file.name.replace(/\.[^.]+$/, '').trim()
      const m = base.match(/^(.+?)\s*[-–—_]\s*(.+)$/)
      const artist = m ? m[1].trim() : ''
      const title = m ? m[2].trim() : base
      chunks.push(`=== ${artist ? artist + ' | ' : ''}${title}\n${text.trim()}`)
    } catch (err) {
      ElMessage.error(`读取「${file.name}」失败：${err.message}`)
    }
  }
  if (chunks.length) {
    importText.value = (importText.value.trim() ? importText.value.trim() + '\n\n' : '') + chunks.join('\n\n')
    ElMessage.success(`已读入 ${chunks.length} 个文件`)
  }
  event.target.value = ''
}

async function doImport() {
  const items = importPreview.value
  if (!items.length) return
  importing.value = true
  try {
    const data = await api('/api/lyrics/import', jsonBody({ items }))
    const skipped = data.skipped_count || 0
    if (skipped) {
      ElMessage.success(`导入 ${data.created_count} 首，跳过 ${skipped} 首（重复或缺歌名）`)
    } else {
      ElMessage.success(`导入 ${data.created_count} 首`)
    }
    importVisible.value = false
    await loadSongs()
    loadArtists()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    importing.value = false
  }
}

// ── 句检索 ──
const lineKeyword = ref('')
const lineHits = ref([])
const lineTotal = ref(0)
const lineLoading = ref(false)
const lineSearched = ref(false)

async function searchLines() {
  const kw = lineKeyword.value.trim()
  if (!kw) {
    ElMessage.warning('先输入关键词')
    return
  }
  lineLoading.value = true
  lineSearched.value = true
  try {
    const data = await api(`/api/lyrics/search/lines?keyword=${encodeURIComponent(kw)}&limit=200`)
    lineHits.value = data.items || []
    lineTotal.value = Number(data.total || 0)
  } catch (err) {
    ElMessage.error(err.message)
    lineHits.value = []
  } finally {
    lineLoading.value = false
  }
}

function searchWordInLines(word) {
  tab.value = 'lines'
  lineKeyword.value = word
  searchLines()
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function highlight(text) {
  const words = lineKeyword.value.trim().split(/\s+/).filter(Boolean)
  let safe = escapeHtml(text)
  for (const w of words) {
    const esc = escapeHtml(w).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    safe = safe.replace(new RegExp(esc, 'gi'), m => `<mark>${m}</mark>`)
  }
  return safe
}

// ── 摘句 ──
const fragments = ref([])
const fragKeyword = ref('')
const fragFavOnly = ref(false)
const fragVisible = ref(false)
const fragSaving = ref(false)
const fragTagsText = ref('')
const fragSongPick = ref('')
const fragForm = reactive({ id: '', song_id: '', text: '', kind: 'hook', note: '' })

let fragTimer = null
function onFragKeywordInput() {
  clearTimeout(fragTimer)
  fragTimer = setTimeout(loadFragments, 320)
}

async function loadFragments() {
  try {
    const p = new URLSearchParams({ limit: '200' })
    if (fragKeyword.value.trim()) p.set('keyword', fragKeyword.value.trim())
    if (fragFavOnly.value) p.set('fav', '1')
    const data = await api(`/api/lyrics/fragments?${p}`)
    fragments.value = data.items || []
  } catch (err) {
    ElMessage.error(err.message)
  }
}

function openFragmentDialog(song, existing) {
  fragTagsText.value = ''
  fragSongPick.value = ''
  if (existing) {
    Object.assign(fragForm, {
      id: existing.id, song_id: existing.song_id || '', text: existing.text,
      kind: existing.kind || 'hook', note: existing.note || '',
    })
    fragTagsText.value = (existing.tags || []).join(', ')
  } else {
    Object.assign(fragForm, {
      id: '', song_id: song?.id || '', text: '', kind: 'hook', note: '',
    })
  }
  fragVisible.value = true
}

async function saveFragment() {
  if (!fragForm.text.trim()) {
    ElMessage.warning('摘句内容不能为空')
    return
  }
  fragSaving.value = true
  const payload = {
    song_id: fragForm.song_id || fragSongPick.value || '',
    text: fragForm.text.trim(),
    kind: fragForm.kind,
    tags: parseTags(fragTagsText.value),
    note: fragForm.note.trim(),
  }
  try {
    if (fragForm.id) {
      await api(`/api/lyrics/fragments/${fragForm.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      ElMessage.success('已保存')
    } else {
      await api('/api/lyrics/fragments', jsonBody(payload))
      ElMessage.success('已记下')
    }
    fragVisible.value = false
    await loadFragments()
    refreshStats()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    fragSaving.value = false
  }
}

// 列表里直接点某行 → 存为摘句
async function quickFragment(song, line) {
  const text = String(line || '').trim()
  if (!text) return
  try {
    await api('/api/lyrics/fragments', jsonBody({
      song_id: song.id, text, kind: 'hook', tags: [], note: '',
    }))
    ElMessage.success('已存为摘句')
    refreshStats()
    if (tab.value === 'fragments') loadFragments()
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function toggleFragFav(f) {
  try {
    const data = await api(`/api/lyrics/fragments/${f.id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ favorite: !f.favorite }),
    })
    f.favorite = data.item?.favorite ?? !f.favorite
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function removeFragment(f) {
  try {
    await api(`/api/lyrics/fragments/${f.id}`, { method: 'DELETE' })
    fragments.value = fragments.value.filter(x => x.id !== f.id)
    refreshStats()
    ElMessage.success('已删除')
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function randomFragment() {
  try {
    const data = await api('/api/lyrics/fragments/random')
    const f = data.item
    ElMessage({
      message: `「${f.text}」${f.song_title ? ' — ' + f.song_title : ''}`,
      duration: 5000,
      showClose: true,
    })
    if (!fragments.value.some(x => x.id === f.id)) fragments.value.unshift(f)
    // 记一次"被取用"，便于回看哪些句子最常被翻到
    fetch(`/api/lyrics/fragments/${f.id}/use`, { method: 'POST' }).catch(() => {})
  } catch (err) {
    ElMessage.warning(err.message)
  }
}

// ── 高频词 ──
const words = ref([])
const wordLoading = ref(false)
const wordType = ref('all')

const filteredWords = computed(() => {
  const list = wordType.value === 'all' ? words.value : words.value.filter(w => w.type === wordType.value)
  return list.slice(0, 60)
})

const maxWordCount = computed(() => filteredWords.value.reduce((m, w) => Math.max(m, w.count), 1))

function barWidth(count) {
  return `${Math.max(4, Math.round((count / maxWordCount.value) * 100))}%`
}

function cloudStyle(count) {
  const max = Math.max(1, ...(stats.value?.top_tags || []).map(t => t.count))
  const scale = 0.85 + (count / max) * 0.5
  return { fontSize: `${scale}rem`, opacity: 0.6 + (count / max) * 0.4 }
}

async function loadWords() {
  wordLoading.value = true
  try {
    const data = await api('/api/lyrics/words?limit=160')
    words.value = data.items || []
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    wordLoading.value = false
  }
}

// ── 生成 prompt ──
const promptVisible = ref(false)
const promptData = ref(null)
const promptExtra = ref('')
const promptSongId = ref('')

async function openPrompt(s) {
  promptSongId.value = s.id
  promptExtra.value = ''
  try {
    const data = await api(`/api/lyrics/songs/${s.id}/prompt`)
    promptData.value = data
    promptVisible.value = true
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function reloadPrompt() {
  if (!promptSongId.value) return
  try {
    const q = promptExtra.value.trim() ? `?style=${encodeURIComponent(promptExtra.value.trim())}` : ''
    promptData.value = await api(`/api/lyrics/songs/${promptSongId.value}/prompt${q}`)
  } catch (err) {
    ElMessage.error(err.message)
  }
}

function copyPromptAndClose() {
  if (!promptData.value) return
  // 本项目是独立的歌词/音乐库，不带 AI 作曲工作台。
  // 所以这里最有用的是"把 prompt 拿走"——复制完整 Markdown 后关掉弹窗，
  // 用户可以贴到任何自己常用的作曲工具里。
  copyText(promptData.value.markdown || '', '完整 prompt 已复制')
  promptVisible.value = false
}

// ── 复制 ──
async function copyText(text, okMsg = '已复制') {
  if (!text) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    ElMessage.success(okMsg)
  } catch {
    ElMessage.error('复制失败，请手动选中')
  }
}

// 「句检索」里点结果时会切 tab，这里保证切回来时保持检索词
watch(tab, () => {
  if (tab.value === 'songs') refreshStats()
})

onMounted(() => {
  loadSongs()
  loadArtists()
  loadMusicLinks()
})
</script>

<style scoped>
.lyrics-app {
  min-height: 100vh;
  padding: 0 24px 140px;
  background: #0b1120;
  color: #e2e8f0;
  position: relative;
  overflow-x: hidden;
}
.bg-orb { position: fixed; border-radius: 50%; filter: blur(120px); pointer-events: none; z-index: 0; }
.bg-orb-1 { width: 560px; height: 560px; top: -200px; left: -140px; background: rgba(139, 92, 246, 0.10); animation: orb1 22s ease-in-out infinite; }
.bg-orb-2 { width: 460px; height: 460px; bottom: -150px; right: -110px; background: rgba(56, 189, 248, 0.07); animation: orb2 26s ease-in-out infinite; }
@keyframes orb1 { 0%, 100% { transform: translate(0, 0); } 50% { transform: translate(70px, 80px); } }
@keyframes orb2 { 0%, 100% { transform: translate(0, 0); } 50% { transform: translate(-60px, -70px); } }

/* Hero */
.ly-hero { position: relative; z-index: 1; text-align: center; padding: 44px 0 18px; }
.ly-badge {
  display: inline-block; padding: 4px 14px; border: 1px solid rgba(139, 92, 246, 0.4);
  border-radius: 20px; font-size: 11px; letter-spacing: 0.2em; color: #a78bfa; margin-bottom: 14px;
}
.ly-title { margin: 0; font-size: clamp(36px, 6vw, 56px); font-weight: 800; letter-spacing: -0.03em; color: #f1f5f9; }
.ly-title .accent {
  background: linear-gradient(135deg, #a78bfa, #f472b6, #38bdf8);
  -webkit-background-clip: text; background-clip: text; -webkit-text-fill-color: transparent;
}
.ly-sub { margin: 12px 0 0; color: #64748b; font-size: 15px; }
.ly-stats { display: flex; justify-content: center; gap: 34px; margin-top: 20px; flex-wrap: wrap; }
.stat { text-align: center; }
.stat-num {
  display: block; font-size: 23px; font-weight: 700;
  background: linear-gradient(135deg, #a78bfa, #38bdf8);
  -webkit-background-clip: text; background-clip: text; -webkit-text-fill-color: transparent;
}
.stat-label { font-size: 12px; color: #64748b; letter-spacing: 0.06em; }

/* Tabs */
.ly-tabs {
  position: relative; z-index: 1;
  max-width: 1080px; margin: 18px auto 22px;
  display: flex; gap: 6px; flex-wrap: wrap; justify-content: center;
}
.ly-tab {
  display: inline-flex; align-items: center; gap: 7px;
  padding: 9px 18px; border-radius: 22px; cursor: pointer;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(30, 41, 59, 0.45); color: #94a3b8; font-size: 14px; transition: all 0.18s;
}
.ly-tab:hover { color: #e2e8f0; border-color: rgba(167, 139, 250, 0.35); }
.ly-tab.active {
  background: linear-gradient(135deg, #7c3aed, #6366f1);
  border-color: transparent; color: #fff; box-shadow: 0 6px 18px rgba(99, 102, 241, 0.28);
}
.tab-badge {
  font-size: 11px; padding: 0 6px; border-radius: 9px;
  background: rgba(255, 255, 255, 0.18); min-width: 18px; text-align: center;
}

/* Body / toolbar */
.ly-body { position: relative; z-index: 1; max-width: 1080px; margin: 0 auto; }
.ly-toolbar { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-bottom: 18px; }
.toolbar-left, .toolbar-right { display: flex; align-items: center; gap: 9px; flex-wrap: wrap; }
.search-input { width: 250px; }
.narrow-select { width: 128px; }
.pill {
  display: inline-flex; align-items: center; gap: 6px; padding: 7px 13px;
  border: 1px solid rgba(148, 163, 184, 0.2); border-radius: 20px;
  background: rgba(30, 41, 59, 0.5); color: #94a3b8; font-size: 13px; cursor: pointer; transition: all 0.18s;
}
.pill:hover { color: #e2e8f0; }
.pill.active { background: linear-gradient(135deg, #f59e0b, #ef4444); border-color: transparent; color: #fff; }
.ghost-pill {
  display: inline-flex; align-items: center; gap: 6px; padding: 8px 15px;
  border: 1px solid rgba(148, 163, 184, 0.22); border-radius: 22px;
  background: transparent; color: #94a3b8; font-size: 13px; cursor: pointer; transition: all 0.18s;
}
.ghost-pill:hover { color: #e2e8f0; border-color: rgba(167, 139, 250, 0.45); }
.upload-pill {
  display: inline-flex; align-items: center; gap: 7px; padding: 9px 20px;
  border: none; border-radius: 22px; cursor: pointer;
  background: linear-gradient(135deg, #7c3aed, #6366f1); color: #fff;
  font-size: 14px; font-weight: 600; transition: all 0.18s;
}
.upload-pill:hover { transform: translateY(-1px); box-shadow: 0 8px 20px rgba(99, 102, 241, 0.3); }

/* Skeleton / empty */
.ly-skeletons { display: flex; flex-direction: column; gap: 10px; }
.skel-row {
  height: 74px; border-radius: 16px;
  background: linear-gradient(110deg, rgba(30, 41, 59, 0.55) 30%, rgba(51, 65, 85, 0.35) 50%, rgba(30, 41, 59, 0.55) 70%);
  background-size: 200% 100%; animation: shimmer 1.5s ease-in-out infinite;
}
@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
.ly-empty { text-align: center; padding: 70px 0; color: #64748b; }
.ly-empty.compact { padding: 36px 0; }
.ly-empty h3 { margin: 14px 0 8px; color: #f1f5f9; }
.ly-empty p { margin: 0 0 18px; font-size: 13px; }

/* Song list */
.song-list { display: flex; flex-direction: column; gap: 8px; }
.song-row {
  border-radius: 16px; background: rgba(15, 23, 42, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.07); transition: border-color 0.2s, background 0.2s;
  animation: fadeUp 0.35s ease both;
}
.song-row:hover { border-color: rgba(167, 139, 250, 0.28); background: rgba(30, 41, 59, 0.6); }
.song-row.open { border-color: rgba(167, 139, 250, 0.45); background: rgba(23, 32, 51, 0.9); }
@keyframes fadeUp { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: none; } }
.song-main { display: flex; align-items: flex-start; gap: 14px; padding: 14px 16px; cursor: pointer; }
.song-mark {
  width: 42px; height: 42px; border-radius: 12px; flex-shrink: 0;
  background: linear-gradient(135deg, rgba(124, 58, 237, 0.35), rgba(56, 189, 248, 0.25));
  display: flex; align-items: center; justify-content: center;
  font-size: 19px; font-weight: 700; color: #ddd6fe;
}
.song-meta { flex: 1; min-width: 0; }
.song-title-line { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.song-title { font-size: 15.5px; font-weight: 650; color: #f1f5f9; }
.fav-star { color: #f59e0b; display: inline-flex; }
.chip {
  font-size: 10.5px; padding: 1px 7px; border-radius: 8px;
  color: #a5b4fc; border: 1px solid rgba(129, 140, 248, 0.35); background: rgba(99, 102, 241, 0.08);
}
.chip-lang { color: #7dd3fc; border-color: rgba(56, 189, 248, 0.35); background: rgba(56, 189, 248, 0.08); }
.chip-plain { color: #94a3b8; border-color: rgba(148, 163, 184, 0.25); background: transparent; }
.chip-music { color: #f59e0b; border-color: rgba(245, 158, 11, 0.4); background: rgba(245, 158, 11, 0.1); }
.song-sub { margin-top: 4px; font-size: 12.5px; color: #64748b; }
.song-artist { color: #cbd5e1; }
.song-dims { color: #475569; }
.song-tags { margin-top: 7px; display: flex; gap: 5px; flex-wrap: wrap; }
.mini-tag {
  font-size: 10.5px; color: #8b5cf6; background: rgba(139, 92, 246, 0.1);
  border: 1px solid rgba(139, 92, 246, 0.22); border-radius: 8px; padding: 1px 7px;
  cursor: pointer; transition: all 0.15s;
}
.mini-tag:hover { background: rgba(139, 92, 246, 0.22); color: #c4b5fd; }
.song-actions { display: flex; gap: 4px; opacity: 0; transition: opacity 0.18s; }
.song-row:hover .song-actions { opacity: 1; }
.row-btn {
  width: 30px; height: 30px; border: none; border-radius: 50%;
  background: rgba(148, 163, 184, 0.12); color: #94a3b8; cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center; transition: all 0.15s;
}
.row-btn:hover { background: rgba(139, 92, 246, 0.2); color: #c4b5fd; }
.row-btn.faved { color: #f59e0b; }
.row-btn.danger:hover { background: rgba(239, 68, 68, 0.18); color: #ef4444; }

.song-lyrics {
  border-top: 1px solid rgba(148, 163, 184, 0.1);
  padding: 12px 16px 16px 70px; max-height: 62vh; overflow-y: auto;
}
.lyric-line { display: flex; align-items: baseline; gap: 10px; padding: 3px 0; border-radius: 6px; }
.lyric-line:hover { background: rgba(148, 163, 184, 0.06); }
.line-no { width: 26px; text-align: right; font-size: 11px; color: #334155; flex-shrink: 0; font-variant-numeric: tabular-nums; }
.line-text { font-size: 14.5px; line-height: 1.75; color: #cbd5e1; flex: 1; white-space: pre-wrap; }
.line-pick {
  opacity: 0; border: none; background: transparent; color: #8b5cf6; cursor: pointer;
  padding: 2px 6px; border-radius: 6px; transition: opacity 0.15s;
}
.lyric-line:hover .line-pick { opacity: 1; }
.song-note {
  margin: 14px 0 0; padding: 10px 12px; border-left: 2px solid rgba(167, 139, 250, 0.5);
  background: rgba(139, 92, 246, 0.06); border-radius: 0 8px 8px 0;
  font-size: 13px; color: #94a3b8; white-space: pre-wrap;
}
.song-note b { color: #c4b5fd; margin-right: 8px; font-weight: 600; }
.song-footer { margin-top: 14px; display: flex; gap: 8px; flex-wrap: wrap; }
.load-more { text-align: center; margin-top: 26px; }

/* 句检索 */
.line-search-box { margin-bottom: 18px; }
.hint { margin: 8px 2px 0; font-size: 12px; color: #475569; }
.hint code, .import-hint code {
  background: rgba(148, 163, 184, 0.12); padding: 1px 5px; border-radius: 4px; color: #a5b4fc;
}
.hit-list { display: flex; flex-direction: column; gap: 6px; }
.hit-head { font-size: 12.5px; color: #64748b; margin-bottom: 4px; }
.hit-head b { color: #c4b5fd; }
.hit-row {
  display: flex; align-items: center; gap: 14px; padding: 11px 14px;
  border-radius: 12px; background: rgba(15, 23, 42, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.07); transition: all 0.16s;
}
.hit-row:hover { border-color: rgba(167, 139, 250, 0.3); background: rgba(30, 41, 59, 0.6); }
.hit-text { flex: 1; font-size: 14.5px; color: #e2e8f0; line-height: 1.6; }
.hit-text :deep(mark) { background: rgba(245, 158, 11, 0.28); color: #fde68a; border-radius: 3px; padding: 0 2px; }
.hit-meta { display: flex; align-items: center; gap: 8px; flex-shrink: 0; font-size: 12px; color: #64748b; }
.hit-song { color: #a5b4fc; cursor: pointer; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hit-song:hover { text-decoration: underline; }
.hit-artist { max-width: 100px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hit-lineno { color: #334155; font-variant-numeric: tabular-nums; }

/* 摘句 */
.frag-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.frag-card {
  padding: 14px 15px; border-radius: 14px; background: rgba(15, 23, 42, 0.75);
  border: 1px solid rgba(148, 163, 184, 0.08); display: flex; flex-direction: column; gap: 9px;
  transition: all 0.18s;
}
.frag-card:hover { border-color: rgba(167, 139, 250, 0.32); transform: translateY(-2px); }
.frag-text { font-size: 15px; line-height: 1.7; color: #e2e8f0; cursor: pointer; }
.frag-meta { display: flex; justify-content: space-between; gap: 8px; font-size: 11.5px; color: #64748b; }
.frag-src { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.frag-src.dim { color: #334155; }
.frag-kind { flex-shrink: 0; color: #8b5cf6; }
.frag-tags { display: flex; gap: 5px; flex-wrap: wrap; }
.frag-actions { display: flex; align-items: center; gap: 4px; margin-top: auto; }
.mini-btn {
  width: 26px; height: 26px; border: none; border-radius: 50%; cursor: pointer;
  background: rgba(148, 163, 184, 0.1); color: #94a3b8;
  display: inline-flex; align-items: center; justify-content: center; transition: all 0.15s;
}
.mini-btn:hover { background: rgba(139, 92, 246, 0.2); color: #c4b5fd; }
.mini-btn.faved { color: #f59e0b; }
.mini-btn.danger:hover { background: rgba(239, 68, 68, 0.18); color: #ef4444; }
.frag-use { margin-left: auto; font-size: 11px; color: #334155; }

/* 高频词 */
.words-panel {
  padding: 18px 20px; border-radius: 18px; margin-bottom: 16px;
  background: rgba(15, 23, 42, 0.7); border: 1px solid rgba(148, 163, 184, 0.07);
}
.panel-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.panel-head h3 { margin: 0; font-size: 15px; color: #f1f5f9; }
.panel-actions { display: flex; align-items: center; gap: 8px; }
.panel-hint { margin: 8px 0 16px; font-size: 12px; color: #475569; line-height: 1.6; }
.word-bars { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 6px 18px; }
.word-bar {
  display: flex; align-items: center; gap: 10px; padding: 5px 6px;
  border: none; background: transparent; cursor: pointer; border-radius: 8px;
  color: #cbd5e1; font-size: 13.5px; transition: background 0.15s;
}
.word-bar:hover { background: rgba(148, 163, 184, 0.08); }
.wb-word { width: 76px; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex-shrink: 0; }
.wb-track { flex: 1; height: 5px; border-radius: 3px; background: rgba(148, 163, 184, 0.1); overflow: hidden; }
.wb-track i { display: block; height: 100%; border-radius: 3px; background: linear-gradient(90deg, rgba(139, 92, 246, 0.85), rgba(56, 189, 248, 0.85)); }
.wb-count { width: 30px; text-align: right; font-size: 11.5px; color: #64748b; font-variant-numeric: tabular-nums; }

.tag-cloud { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 14px; }
.cloud-tag {
  border: 1px solid rgba(139, 92, 246, 0.25); background: rgba(139, 92, 246, 0.08);
  color: #c4b5fd; border-radius: 16px; padding: 4px 12px; cursor: pointer; transition: all 0.16s;
}
.cloud-tag:hover { background: rgba(139, 92, 246, 0.2); }
.cloud-count { font-size: 0.78em; color: #7c3aed; margin-left: 3px; }

/* 弹窗 */
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.form-grid .span-2 { grid-column: span 2; }
.import-hint { margin: 0 0 10px; font-size: 13px; color: #94a3b8; line-height: 1.7; }
.import-sample {
  margin: 0 0 14px; padding: 12px 14px; border-radius: 10px;
  background: rgba(2, 6, 23, 0.55); border: 1px solid rgba(148, 163, 184, 0.12);
  font-size: 12.5px; color: #a5b4fc; line-height: 1.7; overflow-x: auto;
  font-family: 'SF Mono', Menlo, monospace;
}
.import-preview {
  margin-top: 12px; padding: 9px 12px; border-radius: 10px;
  background: rgba(139, 92, 246, 0.1); font-size: 12.5px; color: #c4b5fd;
}
.import-file-row { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; flex-wrap: wrap; }
.import-file-hint { font-size: 11.5px; color: #475569; }
.hidden-file { display: none; }
.prompt-body { display: flex; flex-direction: column; gap: 14px; }
.prompt-song { font-size: 15px; font-weight: 600; color: #f1f5f9; }
.prompt-block { border: 1px solid rgba(148, 163, 184, 0.12); border-radius: 12px; overflow: hidden; }
.pb-head {
  display: flex; justify-content: space-between; align-items: center;
  padding: 7px 12px; background: rgba(148, 163, 184, 0.08); font-size: 12.5px; color: #94a3b8;
}
.pb-body {
  margin: 0; padding: 12px; font-size: 13px; line-height: 1.7; color: #e2e8f0;
  max-height: 220px; overflow: auto; white-space: pre-wrap; word-break: break-word;
  background: rgba(2, 6, 23, 0.4);
}
.pb-body.mono { font-family: 'SF Mono', Menlo, monospace; color: #a5b4fc; }
.prompt-extra { margin-top: 2px; }

/* ── 在线抓取 ── */
.fetch-mode-bar { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; }
.mode-btn {
  padding: 5px 14px; border-radius: 999px; cursor: pointer; font-size: 12.5px;
  border: 1px solid rgba(148, 163, 184, 0.18); background: transparent; color: #94a3b8;
  transition: all 0.16s;
}
.mode-btn.active { background: rgba(139, 92, 246, 0.16); border-color: rgba(139, 92, 246, 0.5); color: #c4b5fd; }
.mode-hint { font-size: 11.5px; color: #475569; }
.fetch-flags { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; grid-column: span 2; }
.fetch-batch-hint { font-size: 11.5px; color: #475569; }
.fetch-warn {
  margin-bottom: 12px; padding: 9px 12px; border-radius: 10px; font-size: 12.5px;
  background: rgba(248, 113, 113, 0.1); color: #fca5a5; border: 1px solid rgba(248, 113, 113, 0.2);
}
.fetch-result {
  margin-top: 16px; padding-top: 14px; border-top: 1px solid rgba(148, 163, 184, 0.12);
  display: flex; flex-direction: column; gap: 10px;
}
.fr-head { font-size: 13.5px; color: #e2e8f0; display: flex; align-items: baseline; gap: 4px; flex-wrap: wrap; }
.fr-meta { font-size: 11.5px; color: #64748b; margin-left: 6px; }
.fr-lyrics :deep(.el-textarea__inner) { font-size: 13px; line-height: 1.75; }
.fr-actions { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.fr-link { font-size: 12px; color: #818cf8; text-decoration: none; }
.fr-link:hover { text-decoration: underline; }
.fr-list { max-height: 220px; overflow: auto; display: flex; flex-direction: column; gap: 6px; }
.fr-item {
  display: flex; justify-content: space-between; gap: 10px; padding: 6px 10px;
  border-radius: 8px; background: rgba(148, 163, 184, 0.06); font-size: 12.5px; color: #cbd5e1;
}
.fr-item.bad { background: rgba(248, 113, 113, 0.08); color: #fca5a5; }
.fr-q { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* ── 对外接口 ── */
.api-grid { display: grid; grid-template-columns: 1.15fr 1fr; gap: 16px; }
.api-card {
  padding: 16px 18px; border-radius: 16px;
  background: rgba(15, 23, 42, 0.55); border: 1px solid rgba(148, 163, 184, 0.1);
}
.api-card.wide { margin-top: 16px; }
.panel-sub { font-size: 11.5px; color: #475569; margin-left: auto; }
.api-base { display: flex; align-items: center; gap: 10px; margin: 4px 0 10px; }
.api-base code {
  flex: 1; padding: 8px 11px; border-radius: 9px; font-size: 12.5px;
  background: rgba(2, 6, 23, 0.6); color: #a5b4fc; word-break: break-all;
}
.api-hint { font-size: 12px; line-height: 1.7; color: #64748b; margin: 0 0 12px; }
.api-hint.small { font-size: 11.5px; margin: 8px 0 0; }
.api-hint code { color: #a5b4fc; font-size: 11.5px; }
.api-list { display: flex; flex-direction: column; gap: 6px; }
.api-row { display: flex; align-items: baseline; gap: 9px; font-size: 12.5px; }
.api-method {
  flex: 0 0 42px; text-align: center; padding: 1px 0; border-radius: 5px; font-size: 10.5px;
  letter-spacing: 0.4px; font-weight: 600;
  background: rgba(148, 163, 184, 0.12); color: #94a3b8;
}
.api-method.get { background: rgba(52, 211, 153, 0.14); color: #6ee7b7; }
.api-method.post { background: rgba(139, 92, 246, 0.16); color: #c4b5fd; }
.api-path { color: #e2e8f0; font-size: 12px; }
.api-desc { color: #64748b; font-size: 11.5px; }
.api-lock { display: flex; flex-direction: column; gap: 10px; }
.api-empty { font-size: 12.5px; color: #475569; padding: 10px 0; }
.mini-copy {
  padding: 3px 10px; border-radius: 999px; cursor: pointer; font-size: 11.5px;
  border: 1px solid rgba(148, 163, 184, 0.2); background: transparent; color: #94a3b8;
  transition: all 0.16s; white-space: nowrap;
}
.mini-copy:hover { border-color: rgba(139, 92, 246, 0.5); color: #c4b5fd; }
.mini-copy.danger:hover { border-color: rgba(248, 113, 113, 0.5); color: #fca5a5; }
.key-create { display: flex; gap: 8px; }
.key-create .el-input { flex: 1; }
.key-fresh {
  margin-top: 12px; padding: 10px 12px; border-radius: 11px;
  background: rgba(52, 211, 153, 0.08); border: 1px solid rgba(52, 211, 153, 0.22);
}
.kf-label { font-size: 11.5px; color: #6ee7b7; margin-bottom: 6px; }
.kf-value { display: flex; align-items: center; gap: 10px; }
.kf-value code { flex: 1; font-size: 12px; color: #d1fae5; word-break: break-all; }
.key-list { margin-top: 12px; display: flex; flex-direction: column; gap: 8px; }
.key-row {
  display: flex; justify-content: space-between; align-items: center; gap: 10px;
  padding: 8px 12px; border-radius: 10px; background: rgba(148, 163, 184, 0.06);
}
.key-row.off { opacity: 0.55; }
.key-main { display: flex; align-items: baseline; gap: 9px; flex-wrap: wrap; min-width: 0; }
.key-name { font-size: 13px; color: #e2e8f0; }
.key-prefix { font-size: 11.5px; color: #818cf8; }
.key-usage { font-size: 11.5px; color: #64748b; }
.key-off { font-size: 11px; color: #fca5a5; }
.key-actions { display: flex; gap: 6px; }
.api-pre {
  margin: 6px 0 0; padding: 14px; border-radius: 11px; overflow: auto;
  background: rgba(2, 6, 23, 0.6); color: #a5b4fc; font-size: 12px; line-height: 1.8;
  font-family: 'SF Mono', Menlo, monospace; white-space: pre;
}

@media (max-width: 768px) {
  .lyrics-app { padding: 0 12px 140px; }
  .search-input { width: 100%; }
  .form-grid { grid-template-columns: 1fr; }
  .form-grid .span-2 { grid-column: span 1; }
  .fetch-flags { grid-column: span 1; }
  .song-lyrics { padding-left: 16px; }
  .song-actions { opacity: 1; }
  .hit-row { flex-direction: column; align-items: flex-start; gap: 6px; }
  .api-grid { grid-template-columns: 1fr; }
}
</style>
