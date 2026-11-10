# 接口文档

本文档描述 **music** 项目的 HTTP 接口。

- **对外接口**（`/api/v1/*`）— 给别的系统调用，需要 API Key。
- **站内接口**（`/api/music/*`、`/api/lyrics/*`）— 给本站前端用，不做密钥鉴权。

两套接口刻意分开：**签名、限流、返回信封都不同**，避免"站内随便调"的习惯泄漏到对外契约里。

在线站点：<https://music.jaxiu.cn>

---

## 目录

- [通用约定](#通用约定)
- [鉴权](#鉴权)
- [管理员会话](#管理员会话)
- [密钥管理](#密钥管理)
- [对外接口 · 歌词库](#对外接口--歌词库-apiv1lyrics)
- [对外接口 · 音乐库](#对外接口--音乐库-apiv1music)
- [站内接口 · 音乐库](#站内接口--音乐库-apimusic)
- [站内接口 · 歌词库](#站内接口--歌词库-apilyrics)
- [站内接口 · 歌词发现](#站内接口--歌词发现-apidiscovery)
- [错误码](#错误码)

---

## 通用约定

### 基础地址

| 环境 | 地址 |
|---|---|
| 在线 | `https://music.jaxiu.cn` |
| 本地（`go run .`） | `http://localhost:8080` |
| 本地（`docker compose up`） | `http://localhost:8083` |

### 返回信封

对外接口（`/api/v1/*`）统一用信封包起来：

**成功**

```json
{
  "ok": true,
  "data": { "...": "业务数据" },
  "meta": { "total": 128, "elapsed_ms": 32 }
}
```

`meta` 是可选的，只有对调用方有意义的附加信息才会出现（如分页总数、耗时、引擎名）。

**失败**

```json
{
  "ok": false,
  "error": "密钥无效或已撤销",
  "code": 401
}
```

`code` 与 HTTP 状态码一致。

> 例外：**音视频流**接口（`/api/v1/music/stream/:id`）不套信封，直接返回二进制流，
> 因为 Range 请求本身不是 JSON 语义。

### 站内接口

站内接口直接返回业务对象或 `{"error": "..."}`，**不套 `ok/data` 信封**。

### CORS

对外接口默认放开任意来源（凭据是密钥而非 Cookie，无 CSRF 风险）。要收紧就在配置里设白名单：

```yaml
lyrics:
  allow_origins: ["https://example.com", "https://app.example.com"]
```

带 `Origin` 的请求会回 `Access-Control-Allow-Origin`；白名单外的来源不会回该头，浏览器即拦截。

预检请求（`OPTIONS`）在鉴权**之前**就地返回 `204`，不会因为没带密钥而失败。

---

## 鉴权

对外接口需要 API Key，三种传递方式任选：

```http
X-API-Key: lly_xxxxxxxx
```

```http
Authorization: Bearer lly_xxxxxxxx
```

```
GET /api/v1/lyrics/songs?api_key=lly_xxxxxxxx
```

> `?api_key=` 方式会把密钥写进访问日志，**仅建议在本地调试时使用**。

### 限流

按**密钥**计数，同一把 key 被多台机器共用时共享同一份额度。默认 60 次/分钟，可在配置里调整：

```yaml
lyrics:
  api_key_rate_limit: 60
```

超限返回 `429`：

```json
{ "ok": false, "error": "该密钥请求过于频繁，请稍后再试", "code": 429 }
```

### 公开自检接口

`/api/v1/lyrics/ping` 与 `/api/v1/music/ping` **不需要密钥**（只挂 CORS），方便外部探活。

> 歌词库的 `/ping` 返回当前密钥的用量信息（`key_name` / `call_count` / `rate_limit_per_minute`），
> 所以它**只在带了密钥时**才带这些字段；音乐库的 `/ping` 始终返回 `{"ready": true}`。

---

## 管理员会话

后台（歌词库 / 歌词发现 / 曲库增删改）需要管理员身份，凭证是**会话 Cookie**，不是 API Key。

口令在 `admin.password`（环境变量 `ADMIN_PASSWORD`）；留空 = 纯访客模式，后台接口全部返回 403。

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/admin/login` | 用口令换会话 Cookie（`{"password":"..."}`） |
| `POST` | `/api/admin/logout` | 清掉会话 Cookie |
| `GET` | `/api/admin/session` | 当前身份：`{"enabled":bool,"authenticated":bool}` |

Cookie 名 `music_admin_session`，HttpOnly，有效期 7 天。改口令等于换密钥，所有旧会话立即失效。

---

## 密钥管理

密钥的增删改走**站内接口**，由管理员会话保护。

先登录换会话 Cookie（口令在 `admin.password`；留空则整个后台禁用）：

```http
POST /api/admin/login
Content-Type: application/json

{"password": "<你的管理员口令>"}
```

成功后服务端下发 `music_admin_session` Cookie（HttpOnly），后续请求自动带上。

> 管理员会话和 API Key 是**两套东西**：会话管"进后台"，密钥管"调对外接口"。
> 对外的 `/api/v1/*` 仍然只认密钥，不认管理员会话。

### 列出密钥

```http
GET /api/lyrics/api-keys
```

```json
{
  "items": [
    {
      "id": "lak_a1b2c3d4e5f6",
      "name": "devtools",
      "prefix": "lly_1a2b",
      "enabled": true,
      "call_count": 128,
      "created_at": "2026-11-01T10:00:00+08:00",
      "last_used_at": "2026-11-10T09:12:33+08:00"
    }
  ],
  "base_url": "/api/v1/lyrics"
}
```

`prefix` 只是密钥前 8 位，用于辨认，**不构成凭据**。

### 签发密钥

```http
POST /api/lyrics/api-keys
Content-Type: application/json

{ "name": "devtools" }
```

```json
{
  "item": { "id": "lak_...", "name": "devtools", "prefix": "lly_1a2b", "enabled": true, "call_count": 0 },
  "plain": "lly_1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4"
}
```

> ⚠️ **`plain` 明文只在这里返回这一次**，之后任何接口都拿不到。库里只存 sha256，泄露数据库也无法还原密钥。
> 请立刻保存。

### 撤销 / 恢复密钥

```http
PATCH /api/lyrics/api-keys/:id
Content-Type: application/json

{ "enabled": false }
```

撤销后调用立即返回 401，但**记录与调用次数保留**，方便排查"是谁在调"。

### 删除密钥

```http
DELETE /api/lyrics/api-keys/:id
```

物理删除，仅用于清理误建的密钥。

---

## 对外接口 · 歌词库 (`/api/v1/lyrics`)

### `GET /api/v1/lyrics/ping`

公开自检。返回抓取组件是否就绪；带密钥时附带该密钥的用量。

```json
{
  "ok": true,
  "data": {
    "ready": true,
    "key_name": "devtools",
    "key_prefix": "lly_1a2b",
    "call_count": 128,
    "rate_limit_per_minute": 60
  }
}
```

### `GET /api/v1/lyrics/songs`

列出歌词档案。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `keyword` | string | — | 标题/艺人模糊匹配 |
| `artist` | string | — | 精确匹配艺人 |
| `language` | string | — | 语种 |
| `genre` | string | — | 风格 |
| `tag` | string | — | 标签 |
| `era` | string | — | 年代 |
| `sort` | string | — | 排序字段 |
| `limit` | int | 50 | 每页条数 |
| `offset` | int | 0 | 偏移 |

```json
{
  "ok": true,
  "data": { "items": [ { "id": "ls_...", "title": "...", "artist": "...", "...": "..." } ] },
  "meta": { "total": 128 }
}
```

### `GET /api/v1/lyrics/songs/:id`

单条歌词详情（含完整歌词正文与备注）。

```json
{ "ok": true, "data": { "id": "ls_...", "title": "...", "artist": "...", "lyrics": "[00:00.00]..." } }
```

### `POST /api/v1/lyrics/songs`

外部系统把自己的歌词直接推进库（不经过在线抓取）。重复的「艺人 - 标题」返回 `409`。

```json
{
  "title": "晴天",
  "artist": "周杰伦",
  "lyrics": "[00:00.00]故事的小黄花...",
  "source": "外部接口"
}
```

成功返回 `201`：

```json
{ "ok": true, "data": { "id": "ls_...", "title": "晴天", "artist": "周杰伦", "...": "..." } }
```

### `GET /api/v1/lyrics/search`

**行级**检索 —— 回答"这句词出自哪首歌第几行"。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `q` | string | 必填 | 关键词（也接受 `keyword`） |
| `limit` | int | 50 | 返回条数 |

```json
{
  "ok": true,
  "data": {
    "keyword": "小黄花",
    "items": [
      { "song_id": "ls_...", "title": "晴天", "artist": "周杰伦", "line_index": 3, "line": "故事的小黄花" }
    ]
  },
  "meta": { "total": 1 }
}
```

### `GET /api/v1/lyrics/stats`

歌词库统计（总数、艺人数、收藏数等）。

### `POST /api/v1/lyrics/fetch`

在线抓取歌词。

```json
{
  "song": "晴天",
  "artist": "周杰伦",
  "engine": "",
  "raw_lrc": false,
  "strip_credits": true,
  "save": false
}
```

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `song` | string | 必填 | 歌名 |
| `artist` | string | — | 艺人 |
| `engine` | string | 自动 | 指定抓取引擎；留空自动选择。可选：`auto` / `baidu` / `card` / `page` / `netease` / `qq` |
| `raw_lrc` | bool | false | 是否返回未清洗的原始 LRC |
| `strip_credits` | bool | true | 是否剥离制作信息等噪音行 |
| `save` | bool | false | 是否直接入库 |

```json
{
  "ok": true,
  "data": {
    "item": { "song": "晴天", "singer": "周杰伦", "lyrics": "[00:00.00]...", "engine": "netease", "url": "..." },
    "saved": null,
    "duplicate": false,
    "existing_id": ""
  },
  "meta": { "engine": "netease", "elapsed_ms": 812 }
}
```

`save: true` 时 `saved` 为入库后的歌词对象；若命中已有记录则 `duplicate: true` 且 `existing_id` 指向原记录。

### `POST /api/v1/lyrics/fetch/batch`

批量抓取。单次最多 20 首。

```json
{
  "songs": ["歌名 - 艺人", "歌名|艺人", "歌名 艺人"],
  "engine": "",
  "raw_lrc": false,
  "strip_credits": true,
  "save": false,
  "language": "",
  "genre": "",
  "era": ""
}
```

`songs` 每行的分隔符支持 ` - `、` – `、`|`、`——`、Tab。

```json
{
  "ok": true,
  "data": {
    "items": [
      { "query": "晴天 - 周杰伦", "ok": true, "error": "", "item": { "...": "..." }, "saved": null, "duplicate": false }
    ]
  },
  "meta": { "ok_count": 1, "failed_count": 0, "saved_count": 0, "elapsed_ms": 934 }
}
```

批量接口**部分失败不影响整体**，每条自带 `ok` / `error`。

---

## 对外接口 · 音乐库 (`/api/v1/music`)

与歌词库对外接口对称：同一套 API Key 表、同一套 CORS 策略、同样的信封。

### `GET /api/v1/music/ping`

```json
{ "ok": true, "data": { "ready": true } }
```

### `GET /api/v1/music/tracks`

精简列表。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `keyword` | string | — | 标题/艺人模糊匹配 |
| `fav` | bool | false | 只看收藏 |
| `sort` | string | — | 排序字段 |
| `limit` | int | 100 | 每页条数 |
| `offset` | int | 0 | 偏移 |

```json
{
  "ok": true,
  "data": { "items": [ { "id": "mt_...", "title": "...", "artist": "...", "...": "..." } ] },
  "meta": { "total": 42 }
}
```

### `GET /api/v1/music/search`

按关键词检索曲目。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `keyword` | string | — | 关键词（也接受 `q`） |
| `limit` | int | 50 | 每页条数 |
| `offset` | int | 0 | 偏移 |

### `GET /api/v1/music/tracks/:id`

曲目详情（含歌词正文与备注）。

### `GET /api/v1/music/snapshot`

全量精简曲库 + 统计，带**增量语义**。

响应头 `X-Music-Revision` 是数据版本号（数据版本 + 筛选条件的哈希）。把它回传即可判断要不要重新拉：

```http
GET /api/v1/music/snapshot?rev=xxx
```

数据没变时：

```json
{ "ok": true, "data": { "not_modified": true, "revision": "xxx" } }
```

数据变了时：

```json
{
  "ok": true,
  "data": {
    "items": [ "..." ],
    "total": 42,
    "stats": { "...": "..." },
    "revision": "yyy",
    "truncated": false
  }
}
```

`truncated: true` 表示条目数达到快照上限，调用方应改用分页接口。

### `GET /api/v1/music/stream/:id`

按 token 流式播放音频。支持 HTTP Range（`206 Partial Content`），可直接喂给 `<audio src>`。

```http
GET /api/v1/music/stream/mt_xxx
X-API-Key: lly_xxx
```

> 浏览器 `<audio>` 标签**不能自定义请求头**，因此这个接口用 URL 参数传密钥：
> `<audio src="/api/v1/music/stream/mt_xxx?api_key=lly_xxx">`。
> 如果站点通过 HTTPS 对外提供，请优先用 Nginx 把该路径代理到后端并在代理层加鉴权，
> 或改用站内 `/api/music/stream/:id`。

---

## 站内接口 · 音乐库 (`/api/music`)

站内接口不做密钥鉴权（服务本身由反向代理保护），但**分公开与管理两档**：

- **公开**：访客用。浏览与听歌。
- **管理**：需要管理员会话（Cookie）。改曲库、关联歌词这些。

### 读取（公开）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/music/tracks` | 曲库列表（`keyword`/`fav`/`sort`/`limit`/`offset`） |
| `GET` | `/api/music/tracks/:id` | 曲目详情 |
| `GET` | `/api/music/tracks/:id/lyrics` | 这首歌的歌词（优先关联到歌词库的版本，没有则用自带正文） |
| `GET` | `/api/music/stats` | 统计 |
| `GET` | `/api/music/snapshot` | 全量快照（带 `rev` 增量语义） |
| `GET` | `/api/music/stream/:id` | 音频流（支持 Range） |
| `GET` | `/api/music/cover/:id` | 封面图 |

### 写入

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/music/tracks/:id/favorite` | 收藏 / 取消收藏（**公开**） |
| `POST` | `/api/music/tracks/:id/played` | 记一次播放（**公开**） |
| `POST` | `/api/music/tracks` | 上传音频（multipart，**管理**） |
| `PATCH` | `/api/music/tracks/:id` | 改元数据（**管理**） |
| `DELETE` | `/api/music/tracks/:id` | 删除曲目（**管理**） |
| `POST` | `/api/music/tracks/:id/cover` | 上传封面（**管理**） |

### 歌词联动（管理）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/music/tracks/:id/lyric-match` | 按歌名/艺人推荐候选歌词 |
| `POST` | `/api/music/tracks/:id/lyrics` | 关联指定歌词 |
| `POST` | `/api/music/tracks/:id/lyrics/auto` | 自动匹配并关联 |
| `DELETE` | `/api/music/tracks/:id/lyrics` | 解除关联 |
| `POST` | `/api/music/lyrics/auto-match` | 批量自动匹配所有未关联曲目 |

### 维护（管理）

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/music/maintenance/repair` | 音频文件体检：回填缺失 `filename`、报告无主文件（**不删**） |

---

## 站内接口 · 歌词库 (`/api/lyrics`)

**整组接口都需要管理员会话**（Cookie `music_admin_session`）。
歌词库是后台模块：收词、检索、摘句、拼提示词都是管理动作，访客只听歌。

### 读取

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/lyrics/songs` | 列表（`keyword`/`artist`/`language`/`genre`/`tag`/`era`/`sort`/`limit`/`offset`） |
| `GET` | `/api/lyrics/songs/:id` | 详情 |
| `GET` | `/api/lyrics/search/lines` | 行级检索 |
| `GET` | `/api/lyrics/stats` | 统计 |
| `GET` | `/api/lyrics/artists` | 艺人聚合 |
| `GET` | `/api/lyrics/words` | 高频词统计 |
| `GET` | `/api/lyrics/random` | 随机一条 |
| `GET` | `/api/lyrics/export` | 导出全部 |
| `GET` | `/api/lyrics/fragments` | 摘句列表 |
| `GET` | `/api/lyrics/fragments/random` | 随机摘句 |
| `GET` | `/api/lyrics/music-links` | 歌词 ↔ 曲目关联表 |
| `GET` | `/api/lyrics/songs/:id/music-match` | 关联的曲目 |
| `GET` | `/api/lyrics/songs/:id/prompt` | 生成该歌词的提示词 |
| `GET` | `/api/lyrics/fetch/status` | 抓取组件状态 |

### 写入

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/lyrics/songs` | 新建歌词 |
| `POST` | `/api/lyrics/import` | 批量导入 |
| `PATCH` | `/api/lyrics/songs/:id` | 修改 |
| `DELETE` | `/api/lyrics/songs/:id` | 删除 |
| `POST` | `/api/lyrics/songs/:id/favorite` | 收藏 / 取消 |
| `POST` | `/api/lyrics/fragments` | 新建摘句 |
| `PATCH` | `/api/lyrics/fragments/:id` | 修改摘句 |
| `DELETE` | `/api/lyrics/fragments/:id` | 删除摘句 |
| `POST` | `/api/lyrics/fragments/:id/use` | 标记摘句已使用 |
| `POST` | `/api/lyrics/fetch` | 在线抓取 |
| `POST` | `/api/lyrics/fetch/batch` | 批量抓取 |

---

## 站内接口 · 歌词发现 (`/api/discovery`)

**整组接口都需要管理员会话**（Cookie `music_admin_session`），同歌词库。

发现流把**本地库里已有的**和**还没拿到的**混在一条流里推给用户。

在线部分刻意做成「后台定时抓取 + 内存候选池」而不是「请求时现抓」：
抓一次要起 Python 进程、等上游网络，2~10 秒起步，放在请求里会让首屏卡死。

### `GET /api/discovery/feed`

一条混着本地与在线的发现流。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `limit` | int | 20 | 条数（上限 60） |
| `online` | string | — | `0` = 不含在线候选 |
| `local` | string | — | `0` = 不含本地素材 |

```json
{
  "items": [
    {
      "kind": "local_song",
      "id": "lyr_...",
      "title": "晴天",
      "artist": "周杰伦",
      "text": "故事的小黄花 从出生那年就飘着",
      "meta": "63 行 · 叶惠美 · 收藏",
      "source": "百度",
      "url": "",
      "saved": true
    },
    {
      "kind": "online",
      "id": "十年",
      "title": "十年",
      "artist": "陈奕迅",
      "text": "如果那两个字没有颤抖 我不会发现 我难受",
      "meta": "来自 网易云音乐",
      "source": "网易云音乐",
      "url": "https://...",
      "saved": false
    }
  ],
  "stats": {
    "local_songs": 128,
    "missing_lyrics": 17,
    "online_candidates": 8,
    "online_ready": true,
    "pool_updated_at": "2026-11-10T17:14:20+08:00",
    "pool_error": ""
  }
}
```

`kind` 有六种，前端按它分派渲染：

| `kind` | 含义 | 建议动作 |
|---|---|---|
| `local_song` | 本地库随机一首歌 | 打开详情 |
| `fragment` | 随机一句摘句 | 复制 / 标记已用 |
| `artist` | 艺人榜条目 | 看该艺人的所有歌 |
| `word` | 高频词 | 检索这个词 |
| `needs_lyrics` | 音乐库里「有歌没歌词」的曲目 | 在线补歌词（`track_id` 是曲目 ID） |
| `online` | 在线抓到、**本地还没有**的候选 | 收进歌词库 |

### `GET /api/discovery/status`

候选池状态与种子配置。

```json
{
  "ready": true,
  "pool_size": 8,
  "pool_updated_at": "2026-11-10T17:14:20+08:00",
  "pool_error": "",
  "refreshing": false,
  "seed_count": 25,
  "seeds": ["晴天 - 周杰伦", "..."]
}
```

### `GET /api/discovery/online`

按歌名/艺人在线搜一首。

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `q` | string | 必填 | 查询词，支持「歌名 - 艺人」格式 |
| `limit` | int | 5 | 上限 20 |

```json
{
  "item": { "song": "十年", "singer": "陈奕迅", "engine": "baidu-card", "lyrics": "..." },
  "preview": "如果那两个字没有颤抖 / 我不会发现 我难受 / ...",
  "saved": false,
  "existing_id": "",
  "in_music": false
}
```

`saved` 表示歌词库里已有；`in_music` 表示音乐库里已有同一首歌。

### `POST /api/discovery/save`

把一条在线结果收进歌词库。`lyrics` 可以带上正文（带上就不用再抓一次）。

```json
{
  "song": "十年",
  "artist": "陈奕迅",
  "lyrics": "[00:00.00]如果那两个字没有颤抖",
  "language": "zh",
  "genre": "pop",
  "era": "2000s",
  "tags": ["治愈"],
  "note": "一句顶一万句"
}
```

成功返回 `201`：

```json
{ "saved": { "id": "lyr_...", "title": "十年", "artist": "陈奕迅", "...": "..." } }
```

已存在同一首（艺人+歌名）时返回 `200`：

```json
{ "duplicate": true, "existing_id": "lyr_..." }
```

### `POST /api/discovery/refresh`

强制刷新候选池。**异步**执行，立即返回 `202`：

```json
{ "started": true }
```

刷新已在进行时返回 `200`：

```json
{ "started": false, "message": "刷新已在进行中" }
```

---

## 错误码

| 状态码 | 含义 | 常见场景 |
|---|---|---|
| `200` | 成功 | — |
| `201` | 已创建 | 签发密钥、上传曲目、创建歌词 |
| `204` | 无内容 | CORS 预检 |
| `400` | 参数错误 | 缺少必填参数、JSON 解析失败 |
| `401` | 未鉴权 | 缺密钥、密钥无效或已撤销 |
| `403` | 禁止 | 不是管理员身份，或未配置 `admin.password`（后台被禁用） |
| `404` | 不存在 | 曲目/歌词不存在、路由未命中 |
| `409` | 冲突 | 重复的「艺人 - 标题」歌词 |
| `429` | 限流 | 超出单密钥每分钟上限 |
| `500` | 服务器错误 | 数据库或文件系统异常 |
| `503` | 服务不可用 | 抓取组件未就绪（缺 Python 依赖） |
| `504` | 网关超时 | 在线抓取超时 |

---

## 健康检查

```http
GET /api/health
```

```json
{ "status": "ok", "service": "music" }
```

这是**公开**接口，用于负载均衡与容器探活，不返回任何敏感信息。
