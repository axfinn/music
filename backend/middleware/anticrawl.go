// Package middleware 的 anticrawl.go 负责「禁止爬虫与批量拖库」。
//
// 分三层，任何一层都能单独关掉或调整：
//   - 声明层：/robots.txt 全站 Disallow，所有响应带 X-Robots-Tag: noindex；
//   - 识别层：拦截脚本 / 爬虫 UA、空 UA、没有浏览器来源信号的匿名数据请求；
//   - 限流层：匿名读接口按 IP 分级计数（普通 / 全量 / 流媒体），外加全局兜底额度。
//
// 刻意不动 /api/v1/*：那是管理员主动签发 API Key 允许的程序化访问，
// 由密钥鉴权 + 单密钥额度管控；要停用应撤销密钥，而不是在这里拦 UA。
// 后台接口另有管理员会话门，不受访客可见性影响。
//
// 边界：这能挡住守规矩的爬虫、通用扫描器和批量脚本；伪造完整浏览器指纹的
// 定向抓取仍可通过。真要彻底禁止匿名取数，只能把曲库改成登录后可见。
package middleware

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// BrowserClientHeader / BrowserClientValue 是自家前端在每个 fetch 上附带的标识。
	// 它不是安全凭据（可以伪造），只是补上老浏览器不发 Sec-Fetch-* 时的来源信号。
	BrowserClientHeader = "X-Music-Client"
	BrowserClientValue  = "web"

	noIndexHeader = "noindex, nofollow, noarchive, nosnippet, noimageindex"
)

// robotsTxt 明确拒绝一切抓取，并单独点名常见 AI / SEO 抓取器表明立场。
// User-agent: * 的 Disallow: / 已经覆盖全站（含 /api/、音频与封面）。
const robotsTxt = `# music 是私人曲库，不对搜索引擎与自动化程序开放。
# 请勿抓取页面、接口、音频、封面或歌词；内容也不用于模型训练。
User-agent: *
Disallow: /

User-agent: GPTBot
User-agent: ChatGPT-User
User-agent: OAI-SearchBot
User-agent: CCBot
User-agent: ClaudeBot
User-agent: anthropic-ai
User-agent: Google-Extended
User-agent: Bytespider
User-agent: PetalBot
User-agent: Baiduspider
User-agent: bingbot
User-agent: YisouSpider
User-agent: Sogou Spider
User-agent: 360Spider
User-agent: SemrushBot
User-agent: AhrefsBot
User-agent: DotBot
Disallow: /
`

// 内置的脚本 / 自动化 / 爬虫 UA 特征（全部按小写匹配）。
// 黑名单优先于「像不像浏览器」的判断；allow_user_agents 又优先于黑名单。
var builtinBlockedUA = []string{
	// 命令行与 HTTP 客户端库
	"curl/", "wget", "python-requests", "python-urllib", "urllib/", "aiohttp/",
	"httpx/", "http.client", "go-http-client", "java/", "apache-httpclient",
	"okhttp/", "libwww-perl", "node-fetch", "axios/", "postmanruntime", "insomnia/",
	"powershell/", "winhttp", "restsharp", "guzzle", "scrapy", "mechanize",
	"python/", "ruby/", "perl/", "php/", "lua-resty-http",
	// 无头浏览器与自动化框架
	"headlesschrome", "phantomjs", "selenium", "playwright", "puppeteer",
	"webdriver", "electron/", "cypress",
	// 通用爬虫、SEO 与漏洞扫描器
	"bot", "spider", "crawler", "slurp", "archiver", "scraper", "harvest",
	"semrush", "ahrefs", "mj12", "dotbot", "petalbot", "bytespider", "censys",
	"masscan", "nmap", "sqlmap", "nikto", "acunetix", "zgrab", "dirbuster",
	"gobuster", "fuzz", "http_request", "urlwatch",
}

// AntiCrawlOptions 由 main 从 config.SecurityConfig 适配后注入，
// middleware 包因此不反向依赖配置层（与 V1APIOptions 的做法一致）。
type AntiCrawlOptions struct {
	Enabled           bool
	AnonymousReadRate int
	BulkReadRate      int
	StreamRate        int
	BlockUserAgents   []string
	AllowUserAgents   []string
}

// AntiCrawl 持有三档限流器与 UA 规则，进程内复用同一个实例。
type AntiCrawl struct {
	enabled bool
	read    *RateLimiter
	bulk    *RateLimiter
	stream  *RateLimiter
	global  *RateLimiter // 防伪造 X-Forwarded-For 绕过单 IP 额度的兜底

	block []string
	allow []string

	blocked atomic.Uint64 // 拦截计数，用于日志采样
}

// NewAntiCrawl 创建反爬设施。额度非法时回落到保守默认值。
func NewAntiCrawl(opts AntiCrawlOptions) *AntiCrawl {
	read := opts.AnonymousReadRate
	if read <= 0 {
		read = 180
	}
	bulk := opts.BulkReadRate
	if bulk <= 0 {
		bulk = 40
	}
	stream := opts.StreamRate
	if stream <= 0 {
		stream = 300
	}
	a := &AntiCrawl{
		enabled: opts.Enabled,
		read:    NewRateLimiter(read, time.Minute),
		bulk:    NewRateLimiter(bulk, time.Minute),
		stream:  NewRateLimiter(stream, time.Minute),
		// 全局额度取普通读的 20 倍：正常访客量用不到，
		// 但即使有人靠伪造 XFF 换 IP，也拖不走整库。
		global: NewRateLimiter(read*20, time.Minute),
		block:  normalizePatterns(append(append([]string(nil), builtinBlockedUA...), opts.BlockUserAgents...)),
		allow:  normalizePatterns(opts.AllowUserAgents),
	}
	return a
}

func normalizePatterns(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// NoIndex 是全局中间件：连 404、静态文件与音频流都打上禁止索引标记。
// 光靠 robots.txt 不够，CDN 边缘可能缓存旧文件、也可能把响应头剥掉，
// 所以前端 index.html 里同时放了等价的 <meta name="robots">。
func NoIndex() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Robots-Tag", noIndexHeader)
		c.Next()
	}
}

// RobotsTxt 返回显式路由处理器，避免 SPA 回落把 /robots.txt 变成一张 HTML。
func RobotsTxt() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(robotsTxt))
	}
}

// Guard 挂在 /api 路由组上：站内匿名数据接口的入口检查。
func (a *AntiCrawl) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.enabled {
			c.Next()
			return
		}
		path := c.Request.URL.Path

		// 预检与健康检查不走浏览器校验：
		// docker healthcheck 用的是 wget，/api/site 则是前端启动的第一个请求，
		// 两者都不返回任何曲库数据。
		if c.Request.Method == http.MethodOptions || path == "/api/health" || path == "/api/site" {
			c.Next()
			return
		}

		// 授权通道：API Key 本身就是给程序用的，只保留全局兜底额度。
		if strings.HasPrefix(path, "/api/v1/") {
			if !a.global.Allow("v1") {
				a.reject(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
				return
			}
			c.Next()
			return
		}

		ua := strings.TrimSpace(c.GetHeader("User-Agent"))
		lowerUA := strings.ToLower(ua)
		if ua == "" || (a.matchAny(lowerUA, a.block) && !a.matchAny(lowerUA, a.allow)) {
			a.reject(c, http.StatusForbidden, "本站禁止自动化程序抓取数据")
			return
		}
		if !looksLikeBrowser(lowerUA) && c.GetHeader(BrowserClientHeader) != BrowserClientValue {
			a.reject(c, http.StatusForbidden, "本站禁止自动化程序抓取数据")
			return
		}

		// 第三方站点把接口当数据源跨域取用：明确发来 cross-site 就拒绝。
		// 例外：魔法宠物（跨站只读 companionRead）由 CORS 中间件在 /api/music、
		// /api/lyrics 两个公开只读前缀上显式放行，仍受下方限流约束。
		if !c.GetBool("companionRead") {
			if site := strings.ToLower(strings.TrimSpace(c.GetHeader("Sec-Fetch-Site"))); site != "" &&
				site != "same-origin" && site != "same-site" && site != "none" {
				a.reject(c, http.StatusForbidden, "不允许跨站取数")
				return
			}
		}

		class := classify(path, c.Request.URL.Query())
		// 数据接口必须有一个「来自本站浏览器」的信号；
		// 音频 / 封面是 <audio>、<img> 直接发的，老浏览器不带这些头，
		// 所以只对 UA 做校验，避免把播放功能弄坏。
		if class != classStream && !hasBrowserSignal(c) && !c.GetBool("companionRead") {
			a.reject(c, http.StatusForbidden, "本站禁止自动化程序抓取数据")
			return
		}

		limiter := a.read
		switch class {
		case classBulk:
			limiter = a.bulk
		case classStream:
			limiter = a.stream
		}
		ip := c.ClientIP()
		if !limiter.Allow(ip) || !a.global.Allow("site") {
			a.reject(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
		c.Next()
	}
}

// reject 统一返回 JSON，并低频记一条日志方便运维观察。
// 爬虫高频重试时如果每次都打日志，日志本身会先被写爆。
func (a *AntiCrawl) reject(c *gin.Context, status int, msg string) {
	n := a.blocked.Add(1)
	if n == 1 || n%50 == 0 {
		log.Printf("[anti-crawl] 累计拦截 %d 次；最近：%s %s ip=%s ua=%q status=%d",
			n, c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.GetHeader("User-Agent"), status)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": msg, "code": status})
}

func (a *AntiCrawl) matchAny(ua string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(ua, p) {
			return true
		}
	}
	return false
}

// looksLikeBrowser 只认现代浏览器的基本形态，不做精确 UA 解析。
func looksLikeBrowser(lowerUA string) bool {
	if !strings.HasPrefix(lowerUA, "mozilla/") {
		return false
	}
	return strings.Contains(lowerUA, "applewebkit") ||
		strings.Contains(lowerUA, "gecko") ||
		strings.Contains(lowerUA, "trident")
}

func hasBrowserSignal(c *gin.Context) bool {
	if c.GetHeader(BrowserClientHeader) == BrowserClientValue {
		return true
	}
	for _, h := range []string{"Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest"} {
		if strings.TrimSpace(c.GetHeader(h)) != "" {
			return true
		}
	}
	return false
}

type readClass int

const (
	classRead readClass = iota
	classBulk
	classStream
)

// classify 决定一次匿名读走哪一档额度。
// 批量接口是拖库的主要口子：snapshot 一次给全量、full=1 给完整正文、
// 大 limit 分页、export 直接导出，全部按更严的额度计。
func classify(path string, q url.Values) readClass {
	if strings.Contains(path, "/stream/") || strings.Contains(path, "/cover/") {
		return classStream
	}
	if strings.HasSuffix(path, "/snapshot") || strings.HasSuffix(path, "/export") {
		return classBulk
	}
	if strings.HasSuffix(path, "/tracks") {
		if q.Get("full") == "1" {
			return classBulk
		}
		if limit, err := strconv.Atoi(q.Get("limit")); err == nil && limit >= 500 {
			return classBulk
		}
	}
	return classRead
}
