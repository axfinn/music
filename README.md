# music-lyrics-archive

个人的**音乐库 + 歌词库 + 歌词发现**开源项目。从 devtools 的「音乐库 / 歌词库」模块拆分独立而来，
自带后端（Go + 纯 Go SQLite）与前端（Vue3 + Element Plus），单二进制 + 静态前端即可部署。

## 为什么独立

- 音乐/歌词是相对独立的领域模块，单独开源更易复用与部署；
- 提供**开放 API（v1）**，既能被原 devtools 通过 302 + 接口对接，也能给任何第三方系统调用；
- 后端改用 [modernc.org/sqlite](https://modernc.org/sqlite)（纯 Go，免 CGO），跨平台二进制一行编译。

## 模块构成

| 模块 | 说明 |
|------|------|
| **音乐库 music** | 上传本地音频 → Range 流式播放 → 元数据/歌词/封面/收藏/播放次数；`rev` 全量快照增量拉取。`/api/music/*` |
| **歌词库 lyrics** | 歌词档案：收录 / 检索（含行级）/ 艺人聚合 / 词频 / 摘句 / 在线抓取（lyrics_finder.py）。`/api/lyrics/*` + 对外 `/api/v1/lyrics/*` |
| **歌词发现 discovery** | 本地歌词库内容（随机/艺人/词频/摘句）与在线抓取结果融合的探索 / 推荐流（Phase 5） |

## 开放 API（v1）

- 歌词库对外接口已完整：`/api/v1/lyrics/*`（API Key 鉴权 + CORS）。
- 音乐库对外接口在 Phase 4 补齐：`/api/v1/music/*`（list / snapshot / 按 token 流式 / search）。
- 密钥在「歌词库 → API 密钥」页面签发（需 `lyrics.admin_password`），或见 `docs/api.md`。

## 快速开始

### 本地（Go 1.25+，无需 CGO）

```bash
cp config.example.yaml config.yaml   # 按需修改
go run .                             # 监听 :8080，数据落在 ./data
curl http://localhost:8080/api/health
```

### Docker

```bash
docker compose up -d --build
# 访问 http://localhost:8080
```

## 数据迁移（从 devtools 迁出）

一次性只读迁移脚本（Phase 7）会把 devtools 共享 SQLite 中的 4 张表导出到本项目的 `data/music.db`
，并把 `./data/music`（音频 + 封面）整体拷贝过来：

- `music_tracks` / `lyric_songs` / `lyric_fragments` / `lyric_api_keys`
- 文件：`data/music/`（音频）与 `data/music/covers/`（封面）

迁移为只读导出，**不破坏 devtools 原库**；完成后在 devtools 侧把旧 `/music`、`/lyrics` 及对应 API
改为 302 跳转到本服务（Phase 8）。

## 目录结构

```
music-lyrics-archive/
├── main.go              # 后端入口（API + 静态前端）
├── backend/
│   ├── config/          # 极简配置加载
│   ├── models/          # SQLite 底座 + 音乐/歌词表（含 TS 时间戳类型）
│   ├── handlers/        # 音乐库 / 歌词库 handler
│   ├── routes/          # 路由注册（含 /api/v1 对外接口）
│   └── middleware/      # 内存限流
├── scripts/
│   └── lyrics_finder.py # 在线歌词抓取（百度→网易云→QQ音乐→百度结果页）
├── frontend/            # Vue3 + Element Plus（Phase 6）
├── data/                # 运行时数据（gitignore）
├── Dockerfile / docker-compose.yml
└── config.example.yaml
```

## 许可证

待定（MIT 建议）。
