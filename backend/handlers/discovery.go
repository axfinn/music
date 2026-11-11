package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 歌词发现（Discovery）──
//
// 发现的定义：**本地库里已有的** 与 **还没拿到的**，混在一条流里推给用户。
//
// 本地侧：随机一首歌、随机一句摘句、艺人榜、高频词，以及"收了歌却还没歌词"的缺口；
// 在线侧：后台定时去上游抓一批候选（种子歌单），只保留本地库里没有的那些。
//
// 在线部分刻意做成"后台刷新 + 内存池"而不是"请求时现抓"：
// 抓一次要起 Python 进程、等上游网络，2~10 秒起步，
// 放在请求里会让首屏卡死；放后台则首屏永远是本地素材，秒回。
//
// 池是进程内状态：重启后重新抓即可，不值得为它引入一张表。

const (
	discoveryDefaultPoolSize = 24  // 在线候选池上限
	discoveryDefaultRefresh  = 30  // 池刷新间隔（分钟）
	discoveryDefaultBatch    = 4   // 每次刷新抓几首
	discoveryPreviewChars    = 160 // 在线结果在流里展示的正文预览长度
	discoveryMaxSeeds        = 200 // 种子歌单上限（防止配置写太长）
	discoveryStaleAfter      = 3   // 池超过几倍刷新间隔就算"该刷新了"
)

// discoverySeeds 默认种子歌单。
// 作用有两个：给空库的新站提供"在线发现"的起点；给已有库补充"你可能还没收录"的热门曲目。
// 格式与歌词抓取脚本一致："歌名 - 艺人"。
var discoveryDefaultSeeds = []string{
	"晴天 - 周杰伦", "稻香 - 周杰伦", "七里香 - 周杰伦", "夜曲 - 周杰伦",
	"十年 - 陈奕迅", "红豆 - 陈奕迅", "K歌之王 - 陈奕迅", "浮夸 - 陈奕迅",
	"海阔天空 - Beyond", "光辉岁月 - Beyond", "真的爱你 - Beyond",
	"小幸运 - 田馥甄", "突然好想你 - 五月天", "倔强 - 五月天", "温柔 - 五月天",
	"平凡之路 - 朴树", "那些花儿 - 朴树", "生如夏花 - 朴树",
	"青花瓷 - 周杰伦", "发如雪 - 周杰伦", "传奇 - 王菲", "匆匆那年 - 王菲",
	"喜欢你 - Beyond", "海阔天空 - 邓紫棋", "光年之外 - 邓紫棋",
}

// discoveryOnlineItem 池里的一条在线候选。
type discoveryOnlineItem struct {
	Query     string    // 抓取时的查询词
	Song      string    `json:"song"`
	Artist    string    `json:"artist"`
	Album     string    `json:"album"`
	Engine    string    `json:"engine"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	Lyrics    string    `json:"lyrics"`
	LineCount int       `json:"line_count"`
	FetchedAt time.Time `json:"fetched_at"`
}

// discoveryPool 在线候选池：进程内、带时间戳、只保留"本地库里没有"的。
type discoveryPool struct {
	mu         sync.RWMutex
	items      []discoveryOnlineItem
	updatedAt  time.Time
	refreshing atomic.Bool
	lastErr    string
}

func (p *discoveryPool) snapshot() ([]discoveryOnlineItem, time.Time, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]discoveryOnlineItem, len(p.items))
	copy(out, p.items)
	return out, p.updatedAt, p.lastErr
}

func (p *discoveryPool) replace(items []discoveryOnlineItem, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = items
	p.updatedAt = time.Now()
	p.lastErr = errMsg
}

// DiscoveryItem 发现流里的一条。所有类型共用一个结构，前端按 kind 分派渲染。
type DiscoveryItem struct {
	Kind    string `json:"kind"` // local_song | fragment | artist | word | needs_lyrics | online
	ID      string `json:"id"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Text    string `json:"text"` // 正文预览（歌词片段 / 摘句）
	Meta    string `json:"meta"` // 一行补充说明
	Source  string `json:"source"`
	URL     string `json:"url"`   // 在线来源链接
	Saved   bool   `json:"saved"` // 本地库是否已有
	TrackID string `json:"track_id,omitempty"`
}

// discoveryFeedQuery 发现流的查询参数。
type discoveryFeedQuery struct {
	limit         int
	includeOnline bool
	includeLocal  bool
}

// DiscoveryFeed GET /api/discovery/feed?limit=20
// 一条混着本地与在线的发现流。
func (h *LyricsHandler) DiscoveryFeed(c *gin.Context) {
	q := discoveryFeedQuery{
		limit:         parseIntDefault(c.Query("limit"), 20),
		includeOnline: c.Query("online") != "0",
		includeLocal:  c.Query("local") != "0",
	}
	if q.limit <= 0 {
		q.limit = 20
	}
	if q.limit > 60 {
		q.limit = 60
	}
	if !q.includeOnline && !q.includeLocal {
		q.includeLocal = true
	}

	items, err := h.buildDiscoveryFeed(c.Request.Context(), q)
	if err != nil && len(items) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成发现流失败: " + err.Error()})
		return
	}

	// 池空了就顺手触发一次后台刷新（不等它，本次响应先用本地内容）。
	h.maybeRefreshDiscoveryPool(c.Request.Context())

	missing, _ := h.db.CountMusicTracksMissingLyrics()
	stats, _ := h.db.LyricStats()
	pool, poolAt, poolErr := h.discovery.snapshot()

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"stats": gin.H{
			"local_songs":       stats.TotalSongs,
			"total_songs":       stats.TotalSongs,
			"missing_lyrics":    missing,
			"online_candidates": len(pool),
			"online_ready":      h.fetcher != nil && h.fetcher.Ready(),
			"pool_updated_at":   poolAt,
			"pool_error":        poolErr,
		},
	})
}

// buildDiscoveryFeed 组装发现流：本地素材与在线候选交错排列。
// 交错而不是"本地在前、在线在后"，是因为发现流的乐趣在于不知道下一张是什么。
func (h *LyricsHandler) buildDiscoveryFeed(ctx context.Context, q discoveryFeedQuery) ([]DiscoveryItem, error) {
	var local []DiscoveryItem
	var online []DiscoveryItem

	if q.includeLocal {
		local = h.buildLocalDiscovery(q.limit)
	}
	if q.includeOnline {
		online = h.buildOnlineDiscovery(q.limit / 3)
	}

	// 交错合并：local[i] 与 online[j] 按 2:1 的比例穿插。
	merged := make([]DiscoveryItem, 0, len(local)+len(online))
	li, oi := 0, 0
	for len(merged) < q.limit && (li < len(local) || oi < len(online)) {
		for n := 0; n < 2 && li < len(local) && len(merged) < q.limit; n++ {
			merged = append(merged, local[li])
			li++
		}
		if oi < len(online) && len(merged) < q.limit {
			merged = append(merged, online[oi])
			oi++
		}
	}
	return merged, nil
}

// buildLocalDiscovery 采样本地素材。
// 任何一项取不到都只是少一块，不该让整条流失败——本地库可能只有歌没有摘句。
func (h *LyricsHandler) buildLocalDiscovery(limit int) []DiscoveryItem {
	items := make([]DiscoveryItem, 0, limit)

	// 1) 随机歌曲
	if songs, err := h.db.RandomLyricSongs(min(limit/3+1, 6)); err == nil {
		for _, s := range songs {
			items = append(items, DiscoveryItem{
				Kind:   "local_song",
				ID:     s.ID,
				Title:  s.Title,
				Artist: s.Artist,
				Text:   previewLyricText(s.Lyrics),
				Meta:   lyricSongMeta(s),
				Source: s.Source,
				Saved:  true,
			})
		}
	}

	// 2) 随机摘句
	if frags, err := h.db.RandomLyricFragments(min(limit/4+1, 4)); err == nil {
		for _, f := range frags {
			items = append(items, DiscoveryItem{
				Kind:   "fragment",
				ID:     f.ID,
				Title:  f.SongTitle,
				Artist: f.Artist,
				Text:   f.Text,
				Meta:   fragmentMeta(f),
				Saved:  true,
			})
		}
	}

	// 3) 艺人榜
	if artists, err := h.db.ListLyricArtists(); err == nil {
		for i, a := range artists {
			if i >= 5 {
				break
			}
			items = append(items, DiscoveryItem{
				Kind:  "artist",
				ID:    a.Name,
				Title: a.Name,
				Meta:  strconv.Itoa(a.Count) + " 首歌",
				Saved: true,
			})
		}
	}

	// 4) 高频词
	if words, err := h.db.LyricWordFrequency(6); err == nil {
		for _, w := range words {
			items = append(items, DiscoveryItem{
				Kind:  "word",
				ID:    w.Word,
				Title: w.Word,
				Meta:  "出现 " + strconv.Itoa(w.Count) + " 次",
				Saved: true,
			})
		}
	}

	// 5) 缺口：有歌没歌词 —— 最该被看见的一类
	if gaps, err := h.db.ListMusicTracksMissingLyrics(3); err == nil {
		for _, g := range gaps {
			items = append(items, DiscoveryItem{
				Kind:    "needs_lyrics",
				ID:      g.ID,
				Title:   g.Title,
				Artist:  g.Artist,
				Meta:    "这首歌还没有歌词",
				TrackID: g.ID,
			})
		}
	}

	return items
}

// buildOnlineDiscovery 从池里取在线候选，标出本地是否已有。
func (h *LyricsHandler) buildOnlineDiscovery(limit int) []DiscoveryItem {
	if limit <= 0 {
		return nil
	}
	pool, _, _ := h.discovery.snapshot()
	if len(pool) == 0 {
		return nil
	}
	index, err := h.db.LyricTitleIndex()
	if err != nil {
		index = map[string]bool{}
	}
	musicIndex, err := h.db.MusicTitleIndex()
	if err != nil {
		musicIndex = map[string]bool{}
	}

	out := make([]DiscoveryItem, 0, limit)
	for _, it := range pool {
		if len(out) >= limit {
			break
		}
		key := models.LyricTitleKey(it.Artist, it.Song)
		saved := index[key] || musicIndex[key]
		meta := "来自 " + it.Source
		if it.Source == "" {
			meta = "在线发现"
		}
		out = append(out, DiscoveryItem{
			Kind:   "online",
			ID:     it.Song,
			Title:  it.Song,
			Artist: it.Artist,
			Text:   previewLyricText(it.Lyrics),
			Meta:   meta,
			Source: it.Source,
			URL:    it.URL,
			Saved:  saved,
		})
	}
	return out
}

// DiscoveryOnlineSearch GET /api/discovery/online?q=&limit=5
// 按歌名/艺人在线搜，返回尚未收录的结果（前端可直接"收进库"）。
func (h *LyricsHandler) DiscoveryOnlineSearch(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "抓取组件未就绪：找不到 lyrics_finder.py 或 python 解释器"})
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		q = strings.TrimSpace(c.Query("keyword"))
	}
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请传 q"})
		return
	}
	limit := parseIntDefault(c.Query("limit"), 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	song, artist := splitSongLine(q)
	result, err := h.fetcher.Fetch(c.Request.Context(), lyricFetchQuery{Song: song, Artist: artist})
	if err != nil {
		writeLyricFetchError(c, err)
		return
	}
	applyLyricCleanup(result, true, song, artist)

	existing, _ := h.db.FindLyricSongByArtistTitle(result.Singer, result.Song)
	musicIndex, _ := h.db.MusicTitleIndex()
	inMusic := musicIndex[models.LyricTitleKey(result.Singer, result.Song)]

	c.JSON(http.StatusOK, gin.H{
		"item":    result,
		"preview": previewLyricText(result.Lyrics),
		"saved":   existing != nil,
		"existing_id": func() string {
			if existing != nil {
				return existing.ID
			}
			return ""
		}(),
		"in_music": inMusic,
	})
}

// DiscoverySave POST /api/discovery/save
// 把一条在线结果收进歌词库。
// body 与在线抓取接口大体同构，另外多一个 lyrics：前端手上已经有正文时不必再抓一次。
func (h *LyricsHandler) DiscoverySave(c *gin.Context) {
	var req struct {
		Song         string   `json:"song"`
		Artist       string   `json:"artist"`
		Lyrics       string   `json:"lyrics"`
		Source       string   `json:"source"`
		URL          string   `json:"url"`
		Language     string   `json:"language"`
		Genre        string   `json:"genre"`
		Era          string   `json:"era"`
		Tags         []string `json:"tags"`
		Note         string   `json:"note"`
		StripCredits *bool    `json:"strip_credits"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Song) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "歌名不能为空"})
		return
	}
	body := lyricFetchRequestBody{
		Song:     req.Song,
		Artist:   req.Artist,
		Save:     true,
		Language: req.Language,
		Genre:    req.Genre,
		Era:      req.Era,
		Tags:     req.Tags,
		Note:     req.Note,
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "在线发现"
	}
	result := &lyricFetchResult{
		Song:   req.Song,
		Singer: req.Artist,
		Source: source,
		Origin: source,
		URL:    req.URL,
		Lyrics: req.Lyrics,
	}
	if strings.TrimSpace(result.Lyrics) == "" {
		// 没带正文就现场抓一次。
		if h.fetcher == nil || !h.fetcher.Ready() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "抓取组件未就绪，且请求未带歌词正文"})
			return
		}
		fetched, err := h.fetcher.Fetch(c.Request.Context(), lyricFetchQuery{Song: req.Song, Artist: req.Artist})
		if err != nil {
			writeLyricFetchError(c, err)
			return
		}
		applyLyricCleanup(fetched, true, req.Song, req.Artist)
		result = fetched
	} else {
		strip := true
		if req.StripCredits != nil {
			strip = *req.StripCredits
		}
		applyLyricCleanup(result, strip, req.Song, req.Artist)
	}
	song, dup, err := h.saveFetchedSong(result, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "入库失败: " + err.Error()})
		return
	}
	if dup != "" {
		c.JSON(http.StatusOK, gin.H{"duplicate": true, "existing_id": dup})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"saved": song})
}

// DiscoveryRefresh POST /api/discovery/refresh — 强制刷新在线候选池（异步）。
func (h *LyricsHandler) DiscoveryRefresh(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "抓取组件未就绪：找不到 lyrics_finder.py 或 python 解释器"})
		return
	}
	if !h.discovery.refreshing.CompareAndSwap(false, true) {
		c.JSON(http.StatusOK, gin.H{"started": false, "message": "刷新已在进行中"})
		return
	}
	go func() {
		defer h.discovery.refreshing.Store(false)
		h.refreshDiscoveryPool(context.Background())
	}()
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// DiscoveryStatus GET /api/discovery/status — 池状态与种子配置。
func (h *LyricsHandler) DiscoveryStatus(c *gin.Context) {
	pool, updatedAt, poolErr := h.discovery.snapshot()
	seeds := h.discoverySeeds()
	c.JSON(http.StatusOK, gin.H{
		"ready":           h.fetcher != nil && h.fetcher.Ready(),
		"pool_size":       len(pool),
		"pool_updated_at": updatedAt,
		"pool_error":      poolErr,
		"refreshing":      h.discovery.refreshing.Load(),
		"seed_count":      len(seeds),
		"seeds":           seeds,
	})
}

// ── 在线候选池 ──

// maybeRefreshDiscoveryPool 池空或过期时触发一次后台刷新，不阻塞本次请求。
func (h *LyricsHandler) maybeRefreshDiscoveryPool(ctx context.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		return
	}
	pool, updatedAt, _ := h.discovery.snapshot()
	interval := h.discoveryRefreshInterval()
	stale := updatedAt.IsZero() || time.Since(updatedAt) > time.Duration(interval*discoveryStaleAfter)*time.Minute
	if len(pool) > 0 && !stale {
		return
	}
	if !h.discovery.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer h.discovery.refreshing.Store(false)
		h.refreshDiscoveryPool(context.Background())
	}()
}

// refreshDiscoveryPool 抓一批候选，滤掉本地已有的，写回池。
func (h *LyricsHandler) refreshDiscoveryPool(ctx context.Context) {
	seeds := h.discoverySeeds()
	if len(seeds) == 0 {
		h.discovery.replace(nil, "没有配置发现种子歌单")
		return
	}
	// 每次刷新换一批起点，避免总是抓同一首。
	start := time.Now().UnixNano() % int64(len(seeds))
	batch := h.discoveryBatchSize()
	if batch > len(seeds) {
		batch = len(seeds)
	}

	localIndex, err := h.db.LyricTitleIndex()
	if err != nil {
		localIndex = map[string]bool{}
	}
	musicIndex, err := h.db.MusicTitleIndex()
	if err != nil {
		musicIndex = map[string]bool{}
	}

	poolSize := h.discoveryPoolSize()
	items := make([]discoveryOnlineItem, 0, poolSize)
	seen := map[string]bool{}
	var lastErr string

	for i := 0; i < batch; i++ {
		seed := seeds[(start+int64(i))%int64(len(seeds))]
		song, artist := splitSongLine(seed)
		if song == "" {
			continue
		}
		result, err := h.fetcher.Fetch(ctx, lyricFetchQuery{Song: song, Artist: artist})
		if err != nil {
			lastErr = err.Error()
			continue
		}
		applyLyricCleanup(result, true, song, artist)
		key := models.LyricTitleKey(result.Singer, result.Song)
		if localIndex[key] || musicIndex[key] || seen[key] {
			continue // 本地已经有了，或这一轮重复
		}
		seen[key] = true
		items = append(items, discoveryOnlineItem{
			Query:     seed,
			Song:      result.Song,
			Artist:    result.Singer,
			Album:     result.Album,
			Engine:    result.Engine,
			Source:    result.Source,
			URL:       result.URL,
			Lyrics:    result.Lyrics,
			LineCount: result.LineCount,
			FetchedAt: time.Now(),
		})
		if len(items) >= poolSize {
			break
		}
	}

	// 抓到的不足就保留旧池里的内容，别把已有素材清空。
	if len(items) == 0 {
		old, _, _ := h.discovery.snapshot()
		if len(old) > 0 {
			h.discovery.replace(old, lastErr)
			return
		}
	}
	h.discovery.replace(items, lastErr)
}

// discoverySeeds 返回种子歌单：配置优先，否则用内置默认。
func (h *LyricsHandler) discoverySeeds() []string {
	var seeds []string
	if h.cfg != nil {
		for _, s := range h.cfg.Lyrics.DiscoverySeeds {
			if s = strings.TrimSpace(s); s != "" {
				seeds = append(seeds, s)
			}
		}
	}
	if len(seeds) == 0 {
		seeds = discoveryDefaultSeeds
	}
	if len(seeds) > discoveryMaxSeeds {
		seeds = seeds[:discoveryMaxSeeds]
	}
	return seeds
}

func (h *LyricsHandler) discoveryPoolSize() int {
	if h.cfg != nil && h.cfg.Lyrics.DiscoveryPoolSize > 0 {
		return h.cfg.Lyrics.DiscoveryPoolSize
	}
	return discoveryDefaultPoolSize
}

func (h *LyricsHandler) discoveryBatchSize() int {
	if h.cfg != nil && h.cfg.Lyrics.DiscoveryBatchSize > 0 {
		return h.cfg.Lyrics.DiscoveryBatchSize
	}
	return discoveryDefaultBatch
}

func (h *LyricsHandler) discoveryRefreshInterval() int {
	if h.cfg != nil && h.cfg.Lyrics.DiscoveryRefreshMinutes > 0 {
		return h.cfg.Lyrics.DiscoveryRefreshMinutes
	}
	return discoveryDefaultRefresh
}

// ── 小工具 ──

// previewLyricText 取正文前若干字符做预览，去掉时间轴标签。
func previewLyricText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 去掉 [00:12.34] 这类时间轴标签，纯文本预览更好读。
		for strings.HasPrefix(line, "[") {
			end := strings.Index(line, "]")
			if end < 0 {
				break
			}
			line = strings.TrimSpace(line[end+1:])
		}
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" / ")
		}
		b.WriteString(line)
		if len([]rune(b.String())) >= discoveryPreviewChars {
			break
		}
	}
	out := b.String()
	if r := []rune(out); len(r) > discoveryPreviewChars {
		out = string(r[:discoveryPreviewChars]) + "…"
	}
	return out
}

func lyricSongMeta(s *models.LyricSong) string {
	parts := make([]string, 0, 3)
	if s.LineCount > 0 {
		parts = append(parts, strconv.Itoa(s.LineCount)+" 行")
	}
	if s.Album != "" {
		parts = append(parts, s.Album)
	}
	if s.Favorite {
		parts = append(parts, "收藏")
	}
	if len(parts) == 0 {
		return "歌词库"
	}
	return strings.Join(parts, " · ")
}

func fragmentMeta(f *models.LyricFragment) string {
	parts := make([]string, 0, 3)
	if f.Kind != "" {
		parts = append(parts, f.Kind)
	}
	if len(f.Tags) > 0 {
		parts = append(parts, strings.Join(f.Tags, "/"))
	}
	if f.UseCount > 0 {
		parts = append(parts, "用过 "+strconv.Itoa(f.UseCount)+" 次")
	}
	if len(parts) == 0 {
		return "摘句"
	}
	return strings.Join(parts, " · ")
}
