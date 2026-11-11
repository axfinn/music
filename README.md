<div align="center">

# music

**独立音乐站** — 私人曲库、歌词档案与发现流，一个自带前后端的开源项目。

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![SQLite](https://img.shields.io/badge/SQLite-pure%20Go-003B57?logo=sqlite&logoColor=white)](https://modernc.org/sqlite)

[在线站点](https://music.jaxiu.cn) · [接口文档](docs/api.md) · [参与贡献](CONTRIBUTING.md) · [许可证](LICENSE)

</div>

---

## 这是什么

一个**可以单独部署的独立音乐站**：曲库是主体，访客打开就能听；歌词档案与歌词发现是站点的后台能力，登录后才能管理。

- **曲库** — 上传本地音频，浏览器流式播放（Range）；管元数据、封面、歌词关联、收藏、播放次数，另有全屏沉浸播放页。
- **歌词库** — 歌词档案：收录、检索（含**行级**检索：能回答"这句词出自哪首歌第几行"）、艺人聚合、高频词、摘句本、在线抓取。
- **歌词发现** — 把本地库里的内容与在线抓取结果**融成一条探索流**，是"随便逛逛"的地方，而不是一个搜索框。在线抓取在后台跑、结果进内存候选池，首屏永远秒回。
- **开放接口** — `/api/v1/music/*` 与 `/api/v1/lyrics/*` 共用一把 API Key，任何系统都能对接曲库与歌词库。

站点品牌是可配置的：站名、标语、描述、域名写在 `config.yaml` 的 `site` 段（或 `SITE_*` 环境变量），
前端启动后从 [`GET /api/site`](docs/api.md#站点信息) 读取，用来渲染顶栏、页面标题与分享卡片。
默认域名 `https://music.jaxiu.cn` 用于 canonical / og:url，**不参与路由与鉴权**；HTTPS 由外层反向代理负责。

技术上刻意做得很轻：

| | |
|---|---|
| 后端 | Go 1.25+ / Gin，**单二进制**同时提供 API 与静态前端 |
| 数据库 | SQLite via [modernc.org/sqlite](https://modernc.org/sqlite) — **纯 Go，零 CGO**，交叉编译一行搞定 |
| 前端 | Vue 3 + Element Plus + Vite，路由页 `*Page.vue`，网络请求统一走 `src/api/client.js` |
| 部署 | `docker compose up -d` 或 `go run .` 均可，无外部依赖 |

## 快速开始

### 用 Docker（推荐）

```bash
git clone https://github.com/axfinn/music.git
cd music
cp .env.example .env                   # 设 ADMIN_PASSWORD 与端口（可选）
docker compose up -d --build
```

打开 <http://localhost:8083>。数据落在 `./data`（SQLite + 音频 + 封面），已挂载为持久卷。

> 默认映射宿主机 **8083** → 容器 8080（`HOST_PORT` 可改）。不设 `ADMIN_PASSWORD` 时站点是
> **纯访客模式**：曲库能听，歌词库 / 歌词发现 / 上传全部关闭 —— 第一次部署建议先设好口令。

### 本地开发

要求 **Go 1.25+**（无需 CGO、无需装 SQLite）。

```bash
cp config.example.yaml config.yaml
go run .

curl http://localhost:8080/api/health   # {"status":"ok","service":"music"}
curl http://localhost:8080/api/site     # 站点身份 + 接口地址
```

前端开发模式（Vite 单独跑，代理 `/api` 到后端）：

```bash
cd frontend
npm install
npm run dev      # http://localhost:5173
```

### 在线歌词抓取（可选）

"在线抓取"依赖仓库内置的 `scripts/lyrics_finder.py`，需要机器上有 **Python 3**（仅标准库）：

```bash
python3 scripts/lyrics_finder.py 晴天 -a 周杰伦
```

脚本会被后端自动探测到；也可以在 `config.yaml` 里用 `lyrics.python_bin` / `lyrics.script_path`
显式指定，或用环境变量 `MUSIC_LYRICS_PYTHON` / `MUSIC_LYRICS_SCRIPT`（旧名 `DEVTOOLS_LYRICS_*` 仍兼容）。
**不装 Python 其余功能照常可用**，只是抓取接口会返回"抓取组件未就绪"。

抓取引擎：`auto`（默认，依次尝试百度卡片 → 网易云 → QQ音乐 → 百度结果页）、
`baidu`、`card`、`page`、`netease`、`qq`。

## 身份与权限

站点只有两种身份，没有注册、没有用户表：

| | 访客 | 管理 |
|---|---|---|
| 进入方式 | 直接打开站点 | 右上角「管理入口」输入口令 |
| 曲库 | 浏览、搜索、播放、沉浸播放、收藏、看当前这首的歌词 | 同上 |
| 上传 / 改曲目 / 删曲目 / 换封面 / 关联歌词 | ✗ | ✓ |
| 歌词库、歌词发现、密钥管理 | ✗ | ✓ |
| 对外接口 `/api/v1/*` | 凭 API Key，与站内身份无关 | 同左 |

口令在 `admin.password`（环境变量 `ADMIN_PASSWORD`）。登录成功后服务端下发 7 天有效的
**HttpOnly 会话 Cookie**（`music_admin_session`，HMAC 签名、无需服务端会话表）；改口令等于换密钥，
所有旧会话立即失效。

留空 = **纯访客模式**：后台接口一律 403，前端也不显示任何管理入口。

## 反爬与数据保护

私人曲库默认**不对搜索引擎和自动化程序开放**。实现集中在
[`backend/middleware/anticrawl.go`](backend/middleware/anticrawl.go)，分三层：

| 层 | 做法 | 挡住什么 |
|---|---|---|
| 声明 | `/robots.txt` 全站 `Disallow: /`；所有响应带 `X-Robots-Tag: noindex, nofollow, noarchive, nosnippet, noimageindex`；`index.html` 里写等价 meta | 守规矩的爬虫与搜索引擎：不抓、不收录、不生成摘要 |
| 识别 | 拦脚本 / 爬虫 / 无头浏览器 UA 与空 UA；匿名数据接口要求浏览器来源信号（`X-Music-Client: web` 或 `Sec-Fetch-*`）；拒绝 `Sec-Fetch-Site: cross-site` | `curl` / `wget` / `python-requests` / 通用扫描器 / 第三方站点跨域取数，直接 403 |
| 限流 | 匿名读按 IP 分三档：普通 180、全量与导出 40、音频封面 300（次/分钟），另有全局兜底额度 | 即使伪装成浏览器，也无法在短时间内分页拖走整库 |

全站 fetch 由 [`src/api/client.js`](frontend/src/api/client.js) 统一带 `X-Music-Client: web`；
`<audio>` / `<img>` 直发请求走浏览器 UA 通道，播放与封面不受影响。

**刻意不拦的部分**：`/api/health`（Docker 健康检查用 wget）、`/api/site`（前端启动拉品牌信息）
与 OPTIONS 预检全部豁免；`/api/v1/*` 是管理员主动签发 API Key 允许的**程序化访问**，
只受密钥鉴权与单密钥额度管控 —— 要停用请到后台「密钥管理」撤销密钥，而不是拦它的 UA；
后台接口另有一层管理员会话门。

配置写在 `security` 段，见 [`config.example.yaml`](config.example.yaml)。想让自家维护脚本
继续访问又不想整体关掉，把脚本 UA 加进 `allow_user_agents` 即可。

> **边界与部署注意**
> - 代理链（EdgeOne → nginx → nps → 容器）必须把真实客户端 IP 透传为 `X-Forwarded-For`，
>   否则所有访客共享同一份额度、正常用户也可能 429；建议在边缘用
>   `proxy_set_header X-Forwarded-For $remote_addr;` 覆盖客户端自带的伪造值。
> - EdgeOne 会缓存 HTML 且可能剥掉响应头，`noindex` 才同时写进 HTML meta。**发版后需在
>   EdgeOne 控制台刷新 `/`、`/robots.txt`、`/index.html`、`/favicon.svg`**，否则边缘节点仍在发旧页面。
> - 这套措施挡得住通用爬虫与批量脚本；伪造完整浏览器指纹的定向抓取仍能通过。
>   要彻底禁止匿名取数，只能把曲库改成「登录后可见」——那会改变「访客可听歌」的定位。
> - 想再加一层，可在 nginx / EdgeOne 配 UA 黑名单与速率限制，对 `/api/music/`、`/api/v1/`
>   分别设阈值。

## 配置

优先级：**环境变量 > `config.yaml` > 默认值**。完整示例见 [`config.example.yaml`](config.example.yaml)。

```yaml
server:
  host: "0.0.0.0"
  port: 8080

data_dir: "./data"          # 音频 / 封面 / SQLite 根目录

site:
  name: "music"             # 站名（顶栏 / 标题 / 媒体通知）
  tagline: "独立音乐站"
  description: "私人曲库与歌词档案"
  base_url: "https://music.jaxiu.cn"   # canonical / og:url

admin:
  password: ""              # 留空 = 纯访客模式

api:                        # 两套 v1 接口共用
  key_rate_limit: 60        # 单密钥每分钟上限
  allow_origins: []         # 空 = 允许任意来源

security:                   # 反爬 / 防批量拖库，详见上一节
  anti_crawl: true
  anonymous_read_rate: 180  # 次/分钟/IP
  bulk_read_rate: 40        # snapshot / full=1 / 大分页 / export
  stream_rate: 300          # 音频与封面
  block_user_agents: []
  allow_user_agents: []

music:
  audio_max_bytes: 209715200
  cover_max_bytes: 15728640

lyrics:
  python_bin: ""
  script_path: ""
  timeout_seconds: 45
  max_concurrent: 3
  discovery_seeds: []       # 歌词发现的在线候选池
  discovery_pool_size: 24
  discovery_batch_size: 4
  discovery_refresh_minutes: 30
```

| 环境变量 | 作用 |
|---|---|
| `PORT` / `DATA_DIR` | 监听端口 / 数据目录 |
| `ADMIN_PASSWORD` | 管理口令（旧名 `LYRICS_ADMIN_PASSWORD` 兼容） |
| `SITE_NAME` / `SITE_TAGLINE` / `SITE_DESCRIPTION` / `SITE_BASE_URL` | 站点身份 |
| `API_KEY_RATE_LIMIT` / `API_ALLOW_ORIGINS` | 对外接口限流与 CORS 白名单（逗号分隔） |
| `ANTI_CRAWL` / `ANTI_CRAWL_READ_RATE` / `ANTI_CRAWL_BULK_RATE` / `ANTI_CRAWL_STREAM_RATE` | 反爬开关与三档匿名读额度 |
| `ANTI_CRAWL_BLOCK_UA` / `ANTI_CRAWL_ALLOW_UA` | 追加拦截 / 放行的 UA 片段（逗号分隔） |

> 旧配置里的 `lyrics.admin_password` / `lyrics.api_key_rate_limit` / `lyrics.allow_origins`
> 会自动迁移到新位置，升级不需要改文件。`config.yaml` 与 `.env` 都已被 `.gitignore` 排除，
> **仓库只包含代码**。

## 部署与运维

`scripts/deploy.sh` 把常用的 Docker Compose 动作收成一条命令。先准备 `.env`（含口令，已 gitignore）：

```bash
cp .env.example .env
./scripts/deploy.sh up      # 构建并启动，随后等健康检查通过
```

| 命令 | 作用 |
|---|---|
| `up` | 构建 + 后台启动，等到 `/api/health` 通过才返回 |
| `update` | 备份数据库 → 拉代码 → 用 git 短 sha 打标签重建 → 重启（发布主流程） |
| `rollback <tag>` | 回滚到上一个镜像标签（会先备份当前库） |
| `status` / `logs -f` / `health` | 看容器、跟日志、探健康 |
| `backup [--full]` | 备份 SQLite；加 `--full` 连音频与封面一起打包 |
| `restore <file>` | 停服 → 再留一份安全备份 → 覆盖数据库 → 启动 |
| `down` / `restart` / `shell` / `clean` | 停服（保留数据）/ 重启 / 进容器 / 清构建缓存 |

镜像构建是多阶段的：Vite 打包前端 → Go 编译二进制 → Alpine 运行期（带 `python3`），
最终只有一个进程、一个数据目录。公网部署时把 `HOST_PORT` 交给外层反向代理做 TLS 终止即可。

## 对外接口（v1）

站内接口在 `/api/music/*` 与 `/api/lyrics/*`；对外接口另开两套，**签名、限流、返回信封都不同**：

| | 基地址 | 能力 |
|---|---|---|
| 曲库 v1 | `/api/v1/music` | 曲目列表 / 详情 / 快照 / 检索 / 音频流 |
| 歌词 v1 | `/api/v1/lyrics` | 歌词收录 / 行级检索 / 抓取 / 统计 |

两套共用 `middleware.V1API` 这一个实例——**同一把密钥、同一套 CORS 白名单、同一份额度、同一份信封**：

- 鉴权：`X-API-Key: <key>` 或 `Authorization: Bearer <key>`（也支持 `?api_key=`，仅本地调试用）
- 信封：成功 `{"ok":true,"data":{...},"meta":{...}}`，失败 `{"ok":false,"error":"...","code":4xx}`
- 限流：按密钥 ID 计数，默认 60 次/分钟（单实例内共享额度）
- 自检：`/ping` 可选鉴权，不带密钥可探活，带密钥则附带用量

```bash
curl -H "X-API-Key: lly_xxx" https://music.jaxiu.cn/api/v1/music/tracks?limit=10
```

密钥在「歌词库 → 接口」页签发（需要管理员身份），页面上并列展示两套 v1 的端点与 curl 示例。
**明文只在签发那一刻返回一次**，库里只存 sha256。完整清单见 **[docs/api.md](docs/api.md)**。

## 目录结构

```
.
├── main.go                     # 后端入口（API + 静态前端 + SPA 回落）
├── backend/
│   ├── config/                 # 配置加载（site / api / 旧键迁移）
│   ├── models/                 # SQLite 底座 + 曲目 / 歌词 / 密钥表
│   ├── handlers/               # 曲库 / 歌词库 / 抓取 / 发现流 / v1 数据端点
│   ├── routes/                 # 路由注册（含站点信息与 /api/v1）
│   └── middleware/             # 管理会话、限流、v1 公共设施（CORS / 鉴权 / 信封）
├── frontend/src/
│   ├── api/client.js           # 全站唯一 HTTP 出入口
│   ├── site.js                 # 站点身份（从 /api/site 读取）
│   ├── composables/            # 播放器单例 / 管理身份
│   ├── router/                 # 路由表（曲库 / 沉浸播放 / 歌词库 / 歌词发现）
│   └── views/                  # *Page.vue 路由页 + MusicLibrary/ 组件与 composables
├── scripts/
│   ├── lyrics_finder.py        # 在线歌词抓取
│   └── deploy.sh               # Docker Compose 一键发布 / 备份 / 回滚
├── docs/                       # api.md / migration.md
├── data/                       # 运行时数据（gitignore）
└── Dockerfile / docker-compose.yml / config.example.yaml
```

## 数据迁移

仓库保留了一次性迁移工具 `cmd/migrate` 与 [docs/migration.md](docs/migration.md)，
可从旧库把 `music_tracks` / `lyric_songs` / `lyric_fragments` / `lyric_api_keys` 与音频文件迁到本站。
迁移是**只读导出**，不破坏原库；日常使用完全不需要它。

## 参与贡献

欢迎提 issue 和 PR。开始之前请读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

[MIT](LICENSE) © axfinn
