<div align="center">

# music

**音乐库 · 歌词库 · 歌词发现** — 一个自带后端与前端的独立开源项目。

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![SQLite](https://img.shields.io/badge/SQLite-pure%20Go-003B57?logo=sqlite&logoColor=white)](https://modernc.org/sqlite)

[在线站点](https://music.jaxiu.cn) · [接口文档](docs/api.md) · [许可证](LICENSE)

</div>

---

## 这是什么

把「音乐库」和「歌词库」从一体化工具箱里拆出来，做成一个**可以单独部署、独立演进、被别的系统调用**的开源项目。

- **音乐库** — 上传本地音频，浏览器直接流式播放；管元数据、封面、歌词关联、收藏、播放次数。
- **歌词库** — 歌词档案库：收录、检索（含**行级**检索：能回答"这句词出自哪首歌第几行"）、艺人聚合、高频词统计、摘句本、在线抓取。
- **歌词发现** — 把本地库里的内容（随机曲、艺人榜、词频、摘句）与在线抓取结果**融成一条探索流**，是"随便逛逛"的地方，而不是一个搜索框。

技术上刻意做得很轻：

| | |
|---|---|
| 后端 | Go 1.25+ / Gin，**单二进制**同时提供 API 与静态前端 |
| 数据库 | SQLite via [modernc.org/sqlite](https://modernc.org/sqlite) — **纯 Go，零 CGO**，交叉编译一行搞定 |
| 前端 | Vue 3 + Element Plus + Vite，设计系统自包含，不依赖宿主应用 |
| 部署 | `docker compose up -d` 或 `go run .` 均可，无外部依赖 |

## 为什么独立出来

- 音乐和歌词是相对独立、边界清晰的领域模块，单独开源后更容易被复用。
- 提供 **v1 开放接口**（API Key 鉴权），任何系统都能对接：原工具箱通过 302 + 接口对接，第三方系统也可以直接调。
- 后端从"必须带一堆兄弟模块"变成一个**独立二进制**，扔进任何机器都能跑。

## 快速开始

### 用 Docker（推荐）

```bash
git clone https://github.com/axfinn/music.git
cd music
cp config.example.yaml config.yaml     # 按需修改（可选）
docker compose up -d --build
```

打开 <http://localhost:8080>。数据落在 `./data`（SQLite + 音频 + 封面），已挂载为持久卷。

### 本地开发

要求 **Go 1.25+**（无需 CGO、无需装 SQLite）。

```bash
git clone https://github.com/axfinn/music.git
cd music
cp config.example.yaml config.yaml
go run .
```

```bash
curl http://localhost:8080/api/health
# {"status":"ok","service":"music"}
```

前端开发模式（Vite 单独跑，代理 `/api` 到后端）：

```bash
cd frontend
npm install
npm run dev      # http://localhost:5173
```

### 在线歌词抓取（可选）

"在线抓取"依赖仓库内置的 `scripts/lyrics_finder.py`，需要机器上有 **Python 3**（仅标准库，无需 `pip install` 任何东西）：

```bash
python3 scripts/lyrics_finder.py 晴天 -a 周杰伦
```

脚本会被后端自动探测到；也可以在 `config.yaml` 里用 `lyrics.python_bin` / `lyrics.script_path` 显式指定。
**不装 Python 其余功能照常可用**，只是抓取接口会返回"抓取组件未就绪"。

抓取引擎：`auto`（默认，依次尝试百度卡片 → 网易云 → QQ音乐 → 百度结果页）、
`baidu`、`card`、`page`、`netease`、`qq`。

## 配置

优先级：**环境变量 > `config.yaml` > 默认值**。完整示例见 [`config.example.yaml`](config.example.yaml)。

```yaml
server:
  host: "0.0.0.0"
  port: 8080

data_dir: "./data"          # 音频 / 封面 / SQLite 根目录

music:
  audio_max_bytes: 209715200   # 单曲上限 200MB
  cover_max_bytes: 15728640    # 封面上限 15MB

lyrics:
  admin_password: ""           # 留空 = 禁用「签发/撤销 API 密钥」等管理动作
  python_bin: ""               # 留空自动探测 python3
  script_path: ""              # 留空自动探测 scripts/lyrics_finder.py
  timeout_seconds: 45
  max_concurrent: 3
  api_key_rate_limit: 60       # 单密钥每分钟上限
  allow_origins: []            # v1 接口 CORS 白名单；空 = 允许任意来源
```

对应环境变量：`PORT`、`DATA_DIR`、`LYRICS_ADMIN_PASSWORD`。

> `config.yaml` 已被 `.gitignore` 排除。**仓库只包含代码**，不含任何数据、密钥或个人配置。

## 对外接口（v1）

站内接口在 `/api/music/*` 与 `/api/lyrics/*`；对外接口另开一套 `/api/v1/music/*` 与 `/api/v1/lyrics/*`，
**签名、限流、返回信封都不同**，刻意与站内隔离：

- 鉴权：`X-API-Key: <key>` 或 `Authorization: Bearer <key>`（也支持 `?api_key=`）
- 信封：成功 `{"ok":true,"data":{...},"meta":{...}}`，失败 `{"ok":false,"error":"...","code":4xx}`
- 限流：按**密钥**计数（同一把 key 多台机器共享额度），默认 60 次/分钟
- CORS：默认放开来源，可用 `lyrics.allow_origins` 收紧

```bash
curl -H "X-API-Key: lly_xxx" https://music.jaxiu.cn/api/v1/lyrics/songs?limit=10
```

密钥在「歌词库 → API 密钥」页签发（需要 `lyrics.admin_password`）。**明文只在签发那一刻返回一次**，库里只存 sha256。

完整接口清单见 **[docs/api.md](docs/api.md)**。

## 目录结构

```
.
├── main.go                # 后端入口（API + 静态前端 + SPA 回落）
├── backend/
│   ├── config/            # 极简配置加载
│   ├── models/            # SQLite 底座 + 音乐/歌词/密钥表（含 TS 时间戳类型）
│   ├── handlers/          # 音乐库 / 歌词库 handler
│   ├── routes/            # 路由注册（含 /api/v1 对外接口）
│   └── middleware/        # 内存限流
├── scripts/
│   └── lyrics_finder.py   # 在线歌词抓取（百度→网易云→QQ音乐→百度结果页）
├── frontend/              # Vue 3 + Element Plus
├── docs/
│   ├── api.md             # 对外接口文档
│   └── migration.md       # 从旧工具箱迁移数据的说明
├── data/                  # 运行时数据（gitignore）
├── Dockerfile / docker-compose.yml
└── config.example.yaml
```

## 数据迁移

本项目可从原一体化工具箱迁移历史数据：把原库中 `music_tracks` / `lyric_songs` / `lyric_fragments` /
`lyric_api_keys` 四张表导出到本项目的 `data/music.db`，并把 `data/music/`（音频与封面）整体拷贝过来。
迁移是**只读导出**，不破坏原库；迁完后原侧的旧页面改为 302 跳转到本站。

详见 [`docs/migration.md`](docs/migration.md)。

## 参与贡献

欢迎提 issue 和 PR。开始之前请读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

[MIT](LICENSE) © axfinn
