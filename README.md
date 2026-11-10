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
- **歌词发现** — 把本地库里的内容（随机曲、艺人榜、词频、摘句、**收了却还没歌词的歌**）与在线抓取结果**融成一条探索流**，是"随便逛逛"的地方，而不是一个搜索框。

在线抓取在后台跑、结果进内存候选池，所以发现流首屏永远秒回，不会卡在等上游网络上。

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
cp .env.example .env                   # 设 ADMIN_PASSWORD 与端口（可选）
docker compose up -d --build
```

打开 <http://localhost:8083>。数据落在 `./data`（SQLite + 音频 + 封面），已挂载为持久卷。

> 默认映射宿主机 **8083** → 容器 8080（`HOST_PORT` 可改）。不设 `ADMIN_PASSWORD` 时站点是
> **纯访客模式**：音乐库能听，歌词库 / 发现 / 上传全部关闭 —— 第一次部署建议先设好口令。

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

## 身份与权限

站点只有两种身份，没有注册、没有用户表：

| | 访客 | 管理 |
|---|---|---|
| 进入方式 | 直接打开站点 | 右上角「管理入口」输入口令 |
| 音乐库 | 浏览、搜索、播放、收藏、看当前这首的歌词 | 同上 |
| 上传 / 改曲目 / 删曲目 / 换封面 / 关联歌词 | ✗ | ✓ |
| 歌词库、歌词发现 | ✗ | ✓ |
| 对外接口 `/api/v1/*` | 凭 API Key，与站内身份无关 | 同左 |

口令在 `admin.password`（环境变量 `ADMIN_PASSWORD`）。登录成功后服务端下发 7 天有效的
**HttpOnly 会话 Cookie**（`music_admin_session`，HMAC 签名、无需服务端会话表）；改口令等于换密钥，
所有旧会话立即失效。

留空 = **纯访客模式**：后台接口一律 403，前端也不显示任何管理入口。

## 配置

优先级：**环境变量 > `config.yaml` > 默认值**。完整示例见 [`config.example.yaml`](config.example.yaml)。

```yaml
server:
  host: "0.0.0.0"
  port: 8080

data_dir: "./data"          # 音频 / 封面 / SQLite 根目录

# 后台管理口令。留空 = 纯访客模式：只有音乐库能听，后台全部关闭。
admin:
  password: ""                 # 管理口令；留空 = 禁用后台

music:
  audio_max_bytes: 209715200   # 单曲上限 200MB
  cover_max_bytes: 15728640    # 封面上限 15MB

lyrics:
  python_bin: ""               # 留空自动探测 python3
  script_path: ""              # 留空自动探测 scripts/lyrics_finder.py
  timeout_seconds: 45
  max_concurrent: 3
  api_key_rate_limit: 60       # 单密钥每分钟上限
  allow_origins: []            # v1 接口 CORS 白名单；空 = 允许任意来源

  # 歌词发现的在线候选池（后台定时抓取，与本地内容混在发现流里）
  discovery_seeds: []            # 种子歌单（"歌名 - 艺人"）；留空 = 用内置默认
  discovery_pool_size: 24        # 候选池上限
  discovery_batch_size: 4        # 每次刷新抓几首
  discovery_refresh_minutes: 30  # 池刷新间隔（分钟）
```

对应环境变量：`PORT`、`DATA_DIR`、`ADMIN_PASSWORD`。

> `config.yaml` 已被 `.gitignore` 排除。**仓库只包含代码**，不含任何数据、密钥或个人配置。

## 部署与运维

`scripts/deploy.sh` 把常用的 Docker Compose 动作收成一条命令。先准备 `.env`（含口令，已 gitignore）：

```bash
cp .env.example .env        # HOST_PORT / ADMIN_PASSWORD / TZ / MUSIC_TAG
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

镜像构建是多阶段的：Vite 打包前端 → Go 编译二进制 → Alpine 运行期（带 `python3`，
在线抓取开箱可用），最终只有一个进程、一个数据目录。

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

密钥在「歌词库 → API 密钥」页签发（需要管理员身份）。**明文只在签发那一刻返回一次**，库里只存 sha256。

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
│   ├── lyrics_finder.py   # 在线歌词抓取（百度→网易云→QQ音乐→百度结果页）
│   └── deploy.sh          # Docker Compose 一键发布 / 备份 / 回滚
├── frontend/              # Vue 3 + Element Plus（npm run build → dist 由后端托管）
├── docs/
│   ├── api.md             # 对外接口文档
│   └── migration.md       # 从旧工具箱迁移数据的说明
├── data/                  # 运行时数据（gitignore）
├── Dockerfile / docker-compose.yml
├── .env.example           # 部署变量样例（复制成 .env）
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
