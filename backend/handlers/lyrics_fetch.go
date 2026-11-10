package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/axfinn/music/backend/config"
)

// ── 歌词在线抓取（内置 lyrics_finder.py） ──
// 资料库里的 lyrics_finder.py 是纯标准库脚本，链路：百度音乐卡片 → 网易云 → QQ音乐 → 百度结果页。
// 这里做成薄封装：不重写抓取逻辑，只负责「起进程 / 限并发 / 卡超时 / 清洗文本 / 可选落库」，
// 让脚本以后单独升级也不会牵连后端。

const (
	lyricFetchMaxSongs      = 20      // 单次批量抓取上限
	lyricFetchMinTimeout    = 10      // 秒
	lyricFetchMaxTimeout    = 90      // 秒，留足上游 100s 网关上限
	lyricFetchDefaultTime   = 45      // 秒
	lyricFetchMaxQueryChars = 120     // 歌名/艺人长度上限
	lyricFetchMaxOutputSize = 8 << 20 // 脚本 stdout 上限 8MB，防意外输出撑爆内存
	lyricFetchStderrKeep    = 600     // 报错时回带给用户的 stderr 尾部长度
)

var (
	// ErrLyricFetchNotFound 上游确实没有这首词的歌词（不是故障，别当 5xx 报）。
	ErrLyricFetchNotFound = errors.New("没有找到这首歌的歌词")
	// ErrLyricFetchTimeout 抓取超时。
	ErrLyricFetchTimeout = errors.New("抓取超时，上游站点响应太慢")
	// ErrLyricFetchUnavailable 运行环境不可用（找不到 python 或脚本）。
	ErrLyricFetchUnavailable = errors.New("歌词抓取组件不可用")
)

// lyricFetchEngines 允许的抓取引擎（与脚本 --engine 的取值保持一致）。
var lyricFetchEngines = map[string]bool{
	"auto": true, "baidu": true, "card": true, "page": true, "netease": true, "qq": true,
}

// LyricsFetcher 负责真正去跑脚本。
type LyricsFetcher struct {
	pythonBin  string
	scriptPath string
	timeout    time.Duration
	sem        chan struct{}

	mu       sync.RWMutex
	resolved bool
}

// NewLyricsFetcher 依据配置构造；路径留空则运行时自动探测。
func NewLyricsFetcher(cfg config.LyricsConfig) *LyricsFetcher {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds <= 0 {
		timeout = lyricFetchDefaultTime * time.Second
	}
	if timeout < lyricFetchMinTimeout*time.Second {
		timeout = lyricFetchMinTimeout * time.Second
	}
	if timeout > lyricFetchMaxTimeout*time.Second {
		timeout = lyricFetchMaxTimeout * time.Second
	}
	concurrency := cfg.MaxConcurrent
	if concurrency <= 0 {
		concurrency = 3
	}
	return &LyricsFetcher{
		pythonBin:  strings.TrimSpace(cfg.PythonBin),
		scriptPath: strings.TrimSpace(cfg.ScriptPath),
		timeout:    timeout,
		sem:        make(chan struct{}, concurrency),
	}
}

// Ready 组件是否可用（前端据此决定显示"在线抓取"还是提示未安装）。
func (f *LyricsFetcher) Ready() bool {
	if f == nil {
		return false
	}
	_, _, err := f.resolve()
	return err == nil
}

// ScriptInfo 供接口自检时展示，不用暴露完整路径给外部。
func (f *LyricsFetcher) ScriptInfo() string {
	if f == nil {
		return ""
	}
	_, script, err := f.resolve()
	if err != nil {
		return ""
	}
	return filepath.Base(script)
}

func (f *LyricsFetcher) resolve() (string, string, error) {
	f.mu.RLock()
	if f.resolved && f.pythonBin != "" && f.scriptPath != "" {
		py, script := f.pythonBin, f.scriptPath
		f.mu.RUnlock()
		return py, script, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	python := f.pythonBin
	if python == "" {
		python = os.Getenv("DEVTOOLS_LYRICS_PYTHON")
	}
	if python == "" {
		for _, candidate := range []string{"python3", "python"} {
			if p, err := exec.LookPath(candidate); err == nil {
				python = p
				break
			}
		}
	}
	if python == "" {
		for _, candidate := range []string{"/app/uv-venv/bin/python3", "/usr/local/bin/python3", "/usr/bin/python3"} {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				python = candidate
				break
			}
		}
	}
	if python == "" {
		return "", "", fmt.Errorf("%w: 找不到 python 解释器", ErrLyricFetchUnavailable)
	}

	script := f.scriptPath
	if script == "" {
		script = os.Getenv("DEVTOOLS_LYRICS_FINDER")
	}
	if script == "" {
		script = discoverLyricScript()
	}
	if script == "" {
		return "", "", fmt.Errorf("%w: 找不到 lyrics_finder.py", ErrLyricFetchUnavailable)
	}
	if st, err := os.Stat(script); err != nil || st.IsDir() {
		return "", "", fmt.Errorf("%w: 脚本路径不可读 %s", ErrLyricFetchUnavailable, script)
	}

	f.pythonBin, f.scriptPath, f.resolved = python, script, true
	return python, script, nil
}

// discoverLyricScript 依次在「当前工作目录 / 可执行文件目录 / 源码目录」找脚本，兼容容器与本地跑。
func discoverLyricScript() string {
	candidates := make([]string, 0, 8)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "scripts", "lyrics_finder.py"),
			filepath.Join(dir, "..", "scripts", "lyrics_finder.py"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "scripts", "lyrics_finder.py"),
			filepath.Join(cwd, "backend", "scripts", "lyrics_finder.py"),
			filepath.Join(cwd, "..", "scripts", "lyrics_finder.py"),
		)
	}
	candidates = append(candidates, "/app/scripts/lyrics_finder.py")
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if abs, absErr := filepath.Abs(c); absErr == nil {
				return abs
			}
			return c
		}
	}
	return ""
}

// ── 请求/结果结构 ──

// lyricFetchQuery 一次抓取的输入。
type lyricFetchQuery struct {
	Song   string
	Artist string
	Engine string
	RawLRC bool
	Page   int
}

// lyricFetchResult 脚本单首输出的镜像（外加耗时）。
type lyricFetchResult struct {
	Song      string `json:"song"`
	Singer    string `json:"singer"`
	Album     string `json:"album"`
	Engine    string `json:"engine"`
	Source    string `json:"source"`
	URL       string `json:"url"`
	LineCount int    `json:"line_count"`
	Lyrics    string `json:"lyrics"`
	ElapsedMS int64  `json:"elapsed_ms"`
	// Origin 覆盖入库时的 source 文案（脚本不会产出这个字段）。
	// 发现页把"用户自己带正文收进来的"也算作在线来源，但不该写成"在线抓取"。
	Origin string `json:"origin,omitempty"`
}

// validate 把用户输入收敛成脚本能安全吃下的形态。
func (q *lyricFetchQuery) validate() error {
	q.Song = strings.TrimSpace(q.Song)
	q.Artist = strings.TrimSpace(q.Artist)
	q.Engine = strings.ToLower(strings.TrimSpace(q.Engine))
	if q.Song == "" {
		return errors.New("歌名不能为空")
	}
	if len([]rune(q.Song)) > lyricFetchMaxQueryChars || len([]rune(q.Artist)) > lyricFetchMaxQueryChars {
		return fmt.Errorf("歌名/艺人过长（上限 %d 字）", lyricFetchMaxQueryChars)
	}
	if strings.ContainsAny(q.Song, "\r\n\t") || strings.ContainsAny(q.Artist, "\r\n\t") {
		return errors.New("歌名/艺人不能包含换行或制表符")
	}
	if q.Engine == "" {
		q.Engine = "auto"
	}
	if !lyricFetchEngines[q.Engine] {
		return fmt.Errorf("不支持的引擎 %q（可选 auto/baidu/card/page/netease/qq）", q.Engine)
	}
	if q.Page < 0 || q.Page > 5 {
		q.Page = 0
	}
	return nil
}

// Fetch 抓一首。
func (f *LyricsFetcher) Fetch(ctx context.Context, q lyricFetchQuery) (*lyricFetchResult, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	args := []string{
		"-e", q.Engine,
		"--json",
		"--timeout", "15",
		"--page", strconv.Itoa(q.Page),
	}
	if q.RawLRC {
		args = append(args, "--raw-lrc")
	}
	if q.Artist != "" {
		args = append(args, "-a", q.Artist)
	}
	// 选项在前、"--" 收尾，歌名以 "-" 开头也不会被 argparse 当成参数。
	args = append(args, "--", q.Song)

	started := time.Now()
	out, err := f.run(ctx, args)
	if err != nil {
		return nil, err
	}
	var result lyricFetchResult
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		return nil, fmt.Errorf("解析抓取结果失败: %w", jsonErr)
	}
	if strings.TrimSpace(result.Lyrics) == "" {
		return nil, ErrLyricFetchNotFound
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	return &result, nil
}

// lyricFetchBatchItem 批量模式下的单条：成功带 lyrics，失败只有 error。
type lyricFetchBatchItem struct {
	Query     string `json:"query"`
	Song      string `json:"song"`
	Singer    string `json:"singer"`
	Album     string `json:"album"`
	Engine    string `json:"engine"`
	Source    string `json:"source"`
	URL       string `json:"url"`
	LineCount int    `json:"line_count"`
	Lyrics    string `json:"lyrics"`
	Error     string `json:"error"`
}

func (it lyricFetchBatchItem) toResult() *lyricFetchResult {
	return &lyricFetchResult{
		Song: it.Song, Singer: it.Singer, Album: it.Album,
		Engine: it.Engine, Source: it.Source, URL: it.URL,
		LineCount: it.LineCount, Lyrics: it.Lyrics,
	}
}

// lyricFetchBatchEnvelope 批量模式（脚本 --file --json）的输出。
type lyricFetchBatchEnvelope struct {
	OK     int                   `json:"ok"`
	Failed int                   `json:"failed"`
	Items  []lyricFetchBatchItem `json:"items"`
}

// FetchBatch 批量抓取：一次进程搞定多首，省掉重复预热百度。
func (f *LyricsFetcher) FetchBatch(ctx context.Context, queries []lyricFetchQuery) ([]lyricFetchBatchItem, error) {
	if len(queries) == 0 {
		return nil, errors.New("没有要抓取的歌曲")
	}
	if len(queries) > lyricFetchMaxSongs {
		return nil, fmt.Errorf("单次最多抓取 %d 首", lyricFetchMaxSongs)
	}
	engine := "auto"
	lines := make([]string, 0, len(queries))
	for i := range queries {
		if err := queries[i].validate(); err != nil {
			return nil, fmt.Errorf("第 %d 首: %w", i+1, err)
		}
		if i == 0 {
			engine = queries[i].Engine
		}
		line := queries[i].Song
		if queries[i].Artist != "" {
			line += " - " + queries[i].Artist
		}
		lines = append(lines, line)
	}

	tmp, err := os.CreateTemp("", "lyrics-batch-*.txt")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("写临时文件失败: %w", err)
	}
	tmp.Close()

	args := []string{
		"--file", tmp.Name(),
		"-e", engine,
		"--json",
		"--timeout", "15",
		"--delay", "0.5",
	}
	out, err := f.run(ctx, args)
	if err != nil {
		return nil, err
	}
	var env lyricFetchBatchEnvelope
	if jsonErr := json.Unmarshal(out, &env); jsonErr != nil {
		return nil, fmt.Errorf("解析批量抓取结果失败: %w", jsonErr)
	}
	return env.Items, nil
}

// run 起一次脚本进程。stdout 只认 JSON，stderr 用于错误诊断。
func (f *LyricsFetcher) run(parent context.Context, args []string) ([]byte, error) {
	python, script, err := f.resolve()
	if err != nil {
		return nil, err
	}

	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-parent.Done():
		return nil, parent.Err()
	}

	ctx, cancel := context.WithTimeout(parent, f.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, python, append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUNBUFFERED=1",
		"PYTHONDONTWRITEBYTECODE=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, keep: lyricFetchMaxOutputSize}
	cmd.Stderr = &limitedWriter{w: &stderr, keep: 32 << 10}

	runErr := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, ErrLyricFetchTimeout
	}
	if out := stdout.Bytes(); len(bytes.TrimSpace(out)) > 0 {
		// 有 JSON 就优先用；脚本找不到歌时也会非零退出。
		if json.Valid(out) {
			return out, nil
		}
		if runErr == nil {
			return out, nil
		}
	}
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return nil, ErrLyricFetchNotFound
		}
		return nil, fmt.Errorf("%w: %s", ErrLyricFetchNotFound, clipText(detail, lyricFetchStderrKeep))
	}
	return nil, ErrLyricFetchNotFound
}

// limitedWriter 只保留前 keep 字节，避免脚本抽风时把内存吃光。
type limitedWriter struct {
	w    *bytes.Buffer
	keep int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	room := l.keep - l.w.Len()
	if room > 0 {
		if len(p) <= room {
			l.w.Write(p)
		} else {
			l.w.Write(p[:room])
		}
	}
	return len(p), nil
}

func clipText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// ── 文本清洗 ──

// 词曲编配等署名行，进歌词库只会污染检索与意象统计。
// 兼容三种写法：全角冒号、半角冒号，以及网易云 LRC 里的反斜杠（"词 \ 黄家驹"）。
var lyricCreditLine = regexp.MustCompile(
	`^\s*(作词|作曲|编曲|制作人|制作|词|曲|编|合声编写|合声|和声编写|和声|吉他|贝斯|低音吉他|鼓手|鼓|键盘|弦乐|` +
		`录音助理|录音工程|录音师|录音|混音工程|混音|母带|母带后期|监制|出品|发行人|统筹|企划|OP|SP|演奏|配唱制作|人声|原唱|演唱|` +
		`钢琴|弦乐编写|弦乐监制|弦乐录音|弦乐手|人声编辑|人声录音|人声录音师|人声录音室|配唱|配唱制作人|和音|合唱|合唱团|合唱指挥|` +
		`指挥|管弦乐统筹|所有乐器|所有铜管|乐器|执行制作|制作行政|制作协力|制作执行|发行总监|发行监制|发行统筹|法律编辑|宣传|统筹|` +
		`童声|藏语童声|国乐协力|钢片琴|键盘乐器|合成器|特雷门|部分用典|混音/母带|` +
		`\s*\\?\s*[:：\\])`,
)

// lyricCreditLineStrict 与 lyricCreditLine 同义，但要求"关键词 + 分隔符"紧邻，
// 不允许关键词与分隔符之间夹空格（"作词 : X" 的空格写法由 lyricCreditLine 覆盖）。
var lyricCreditLineStrict = regexp.MustCompile(
	`^\s*(作词|作曲|编曲|制作人|制作|词|曲|编|主唱|演唱|和声|合声|合音|和音|演奏|配唱|混音|母带|录音|录音师|录音室|` +
		`人声|吉他|贝斯|鼓|键盘|弦乐|钢琴|所有乐器|乐器|指挥|统筹|发行|宣传|出品|监制|OP|SP|人声编辑|人声录音|` +
		`弦乐编写|弦乐监制|弦乐录音|母带后期|混音工程|录音工程|制作协力|制作行政|执行制作|发行总监|发行监制|发行统筹|` +
		`法律编辑|童声|合唱团|合唱指挥|配唱制作人|人声录音师|人声录音室|混音/母带|专辑编号|发行公司|产品形式|` +
		`演\s*唱\s*者|发行日期|专辑名称|唱片编号|出版公司)` +
		`\s*\\?\s*[:：\\]`,
)

// 网易云 LRC 里的反斜杠署名写法："主唱 \ 黄家驹." / "词 \ 罗弦武. 曲 \ 黄家驹."
// 特点是：关键词 + 反斜杠 + 名字（名字里可有点号），结尾不一定有冒号。
var lyricCreditBackslashLine = regexp.MustCompile(
	`^\s*(作词|作曲|编曲|制作人|制作|词|曲|编|主唱|演唱|和声|合声|合音|和音|演奏|配唱|混音|母带|录音|人声|` +
		`吉他|贝斯|鼓|键盘|弦乐|钢琴|所有乐器|乐器|指挥|统筹|发行|宣传)` +
		`\s*\\\s*[^\n]{1,30}$`,
)

// 专辑信息行："专辑名称：“惊喜”" —— 引号内容也算一行专辑元信息。
var lyricAlbumMetaLine = regexp.MustCompile(
	`^\s*(演\s*唱\s*者|发行日期|发行时间|专辑编号|专辑名称|唱片编号|发行公司|出版公司|产品形式|出品方|企划)\s*[:：]`,
)

// 制作信息行的"连字符"写法："制作人 - 林暐哲" / "演奏&合音&演唱 - 苏打绿"。
var lyricCreditDashLine = regexp.MustCompile(
	`^\s*(制作人|制作协力|制作行政|执行制作|混音|母带|录音|人声|配唱|和声|合声|和音|演奏|合音|演唱|` +
		`吉他|贝斯|鼓|键盘|弦乐|钢琴|所有乐器|乐器|编曲|指挥|统筹|发行|宣传|发行总监|发行监制|发行统筹|` +
		`合音编写|吉他手|贝斯手|鼓手|键盘手|弦乐手|部分用典|制作|混音师|混音室|母带工程师|母带工作室|` +
		`录音工程师|混音工程师|母带后期工程师|母带后期处理工程师|混音录音室|母带后期处理录音室|` +
		`人声编辑|人声录音师|人声录音室|弦乐监制|弦乐录音师|弦乐录音|弦乐编写)` +
		`[^:\n]{0,14}\s+[-–—]\s*\S`,
)

// 英文/繁中制作信息行（"Produced by"、"Mixed by Simon Li at nOiz"）。
var lyricMetaEnglishLine = regexp.MustCompile(
	`(?i)^\s*(produced by|mixed by|mastered by|recorded by|arranged by|composed by|lyrics by|` +
		`mixed at|recorded at|mastered at|all instruments|background vocals?|vocal producer|` +
		`mixing engineer|mastering engineer|recording engineer|recording studio|mixing studio|` +
		`mastering studio|conductor|orchestra administration|production assistant|legal editor|` +
		`sax solo|copyright|special thanks|daymaker|harbour heroes|central music|performed by)`,
)

// 制作信息 + 冒号 + 内容（英文制作行几乎都长这样）。
var lyricMetaColonLine = regexp.MustCompile(
	`^\s*[\p{Han}A-Za-z][\p{Han}A-Za-z\s/\.\-&·]{0,28}\s*[:：]\s*[^，。？！\n]{0,80}$`,
)

// 制作信息行里出现这些关键词时，即使格式不标准也要剔掉。
var lyricMetaKeyword = regexp.MustCompile(
	`(?i)(录音室|混音|母带|人声录音|配唱|和音|合唱团|弦乐编写|弦乐监制|弦乐录音|管弦乐|制作协力|制作行政|执行制作|` +
		`专辑编号|发行公司|产品形式|演\s*唱\s*者|发行日期|发行总监|发行监制|发行统筹|法律编辑|版权声明|统筹/|宣传/|` +
		`studio|engineer|mastering|mixing|recording|conductor|background vocal|all instruments|vocal producer|` +
		`produced by|mixed by|mastered by|recorded by|special thanks|midi pragramming)`,
)

// 演唱角色标注（"大合唱：" "伙伴合唱 Chorus："）是歌词的一部分，不能当元信息剔掉。
var lyricRoleMarker = regexp.MustCompile(
	`(?i)^\s*(大合唱|合唱|伙伴合唱|独唱|对唱|齐唱|群唱|和声|合声|Chorus|Vocal)(\s+[A-Za-z]+)?\s*[:：]?\s*$`,
)

// 纯分隔线（--- / === / ***）。
var lyricSeparatorLine = regexp.MustCompile(`^\s*[-=*_~—]{3,}\s*$`)

// isLyricMetaLine 判断该行是否为制作/版权/署名等元信息行（不是歌词正文）。
func isLyricMetaLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	if lyricSeparatorLine.MatchString(s) {
		return true
	}
	// 演唱角色标注先判：它是歌词的一部分。
	if lyricRoleMarker.MatchString(s) {
		return false
	}
	if lyricCreditLineStrict.MatchString(s) || lyricCreditDashLine.MatchString(s) {
		return true
	}
	if lyricAlbumMetaLine.MatchString(s) {
		return true
	}
	// 反斜杠署名：必须整行都是"关键词 + 名字"，不能是普通歌词里提到制作人。
	if lyricCreditBackslashLine.MatchString(s) && !strings.ContainsAny(s, "，。？！,") {
		return true
	}
	if lyricMetaEnglishLine.MatchString(s) {
		return true
	}
	if lyricMetaColonLine.MatchString(s) && lyricMetaKeyword.MatchString(s) {
		return true
	}
	return false
}

// CleanLyricLines 按行清洗：剔署名行与标题行、去分隔线、压掉多余空行。
func CleanLyricLines(lyrics, song, singer string) string {
	raw := strings.Split(strings.ReplaceAll(lyrics, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	seenContent := 0
	lastBlank := true
	for _, line := range raw {
		line = strings.TrimRight(strings.TrimSpace(line), " \t\u3000")
		if line == "" {
			if !lastBlank && len(out) > 0 {
				out = append(out, "")
				lastBlank = true
			}
			continue
		}
		if isLyricMetaLine(line) {
			continue
		}
		if seenContent < 2 && looksLikeLyricTitleLine(line, song, singer) {
			continue
		}
		seenContent++
		out = append(out, line)
		lastBlank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// looksLikeLyricTitleLine 判断是否是"歌名 - 艺人"这类标题行。
// 只在开头两行、且短于 40 字时启用，避免误杀正文里恰好提到歌名的句子。
func looksLikeLyricTitleLine(line, song, singer string) bool {
	if song == "" || len([]rune(line)) > 40 {
		return false
	}
	nLine := normalizeLyricText(line)
	nSong := normalizeLyricText(song)
	if nSong == "" || !strings.Contains(nLine, nSong) {
		return false
	}
	if singer == "" {
		return nLine == nSong
	}
	return strings.Contains(nLine, normalizeLyricText(singer))
}

func normalizeLyricText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || strings.ContainsRune("-–—_·、,，.。'\"“”‘’()（）[]【】", r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// DetectLyricLanguage 粗判语种：只做 zh/ja/ko/en/other，粤语没法从文本可靠区分，归到 zh 由用户改。
func DetectLyricLanguage(text string) string {
	var han, kana, hangul, latin int
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Han):
			han++
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.In(r, unicode.Hangul):
			hangul++
		case unicode.IsLetter(r):
			latin++
		}
	}
	switch {
	case kana > 3:
		return "ja"
	case hangul > 3:
		return "ko"
	case han > 0 && han >= latin:
		return "zh"
	case latin > 0:
		return "en"
	default:
		return "other"
	}
}
