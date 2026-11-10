# 数据迁移指南

把旧一体化工具箱（devtools）里的**音乐库**与**歌词库**数据迁到本项目。

迁移由仓库自带的 `cmd/migrate` 完成，**源库全程只读**：数据库用 SQLite URI 的 `mode=ro` 打开，
任何写操作都会被驱动直接拒绝（`attempt to write a readonly database`），不是靠代码自觉。

---

## 会迁什么

| 类别 | 内容 |
|---|---|
| 数据库表 | `lyric_songs`、`lyric_fragments`、`lyric_api_keys`、`music_tracks` |
| 音频文件 | `<源数据目录>/music/*.mp3`（以及其它音频格式） |
| 封面图片 | `<源数据目录>/music/covers/*.jpg` |

> 两边的表结构是**一致的**（本项目就是从那边搬过来的），所以不需要字段映射。
> 就算旧库以后加了新字段，迁移工具也只会搬目标库**已有**的列，多出来的列自动跳过并在输出里提示。

---

## 准备：先把数据从容器里取出来

旧系统的数据在 Docker 命名卷里，先拷到宿主机。

```bash
# 旧系统的容器名与卷名以你的实际部署为准
docker cp <旧容器>:/app/data/paste.db      ./old/paste.db
docker cp <旧容器>:/app/data/music          ./old/music
```

得到这样的目录结构：

```
old/
├── paste.db      # 共享 SQLite，音乐/歌词表都在里面
└── music/        # 音频
    └── covers/   # 封面
```

> 也可以直接对着容器里的路径跑迁移（工具会自己打开文件），但先拷出来更安全 ——
> 万一中途要重来，源数据还在。

---

## 更省事的做法：直接对拷 Docker 卷

旧数据在命名卷里、动辄几十 GB 时，`docker cp` 等于白拷两遍。可以直接把旧卷
**只读**挂进一个临时容器，让迁移工具在里面跑：

```bash
# 1) 编一份 linux 版迁移工具（纯 Go，免 CGO，几秒）
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o /tmp/migrate ./cmd/migrate

# 2) 先停服务，避免迁移与在线请求抢 SQLite 写锁
docker stop music

# 3) 旧卷 :ro 挂载，新数据目录挂到 /target，直接对拷
docker run --rm \
  -v devtools_devtools-data:/source:ro \
  -v /tmp/migrate:/migrate:ro \
  -v "$PWD/data:/target" \
  alpine:3.20 /migrate \
    -source-db /source/paste.db \
    -source-data-dir /source \
    -target-db /target/music.db \
    -target-data-dir /target

# 4) 起服务
docker start music
```

源卷只读挂载 + 工具内部 `mode=ro` 打开，是双保险；已存在的同名同大小文件自动跳过，
所以中途中断重跑只补差异，不会重复占用空间。

---

## 试运行

**先加 `-dry-run`**：只统计要迁多少，不写任何东西。

```bash
go run ./cmd/migrate \
  -source-db ./old/paste.db \
  -source-data-dir ./old \
  -target-db ./data/music.db \
  -target-data-dir ./data \
  -dry-run
```

输出示例：

```
源库: ./old/paste.db（只读）
目标: ./data/music.db

  lyric_songs        128 行（dry-run，未写入）
  lyric_fragments     36 行（dry-run，未写入）
  lyric_api_keys       1 行（dry-run，未写入）
  music_tracks      2597 行（dry-run，未写入）

数据库共 2762 行（dry-run，未写入）

音频/封面文件：拷贝 2597 个（1024.0 MB），跳过 0 个已存在，失败 0 个（dry-run，未写入）
```

数字对得上，就去掉 `-dry-run` 再跑一次。

---

## 正式迁移

```bash
go run ./cmd/migrate \
  -source-db ./old/paste.db \
  -source-data-dir ./old \
  -target-db ./data/music.db \
  -target-data-dir ./data
```

要点：

- **目标库会自动建表** —— 工具内部用 `models.NewDB` 打开目标库，顺带把本项目的表结构初始化好，
  你不需要先跑一遍服务。
- **可以重复运行**。数据用 `INSERT OR REPLACE` 按主键覆盖，文件按「同名同大小」跳过，
  所以中断了直接重跑就行，不会产生重复。
- **单个文件拷贝失败不中断整体**，最后会汇总失败数。

### 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `-source-db` | 必填 | 旧系统的 SQLite 文件路径（只读打开） |
| `-source-data-dir` | — | 旧系统的数据目录，里面有 `music/` 子目录；留空则只迁数据库 |
| `-target-db` | `./data/music.db` | 本项目的 SQLite 路径 |
| `-target-data-dir` | `./data` | 本项目的数据目录，音频落在 `<dir>/music/` |
| `-dry-run` | false | 只统计不写入 |
| `-skip-files` | false | 只迁数据库，不拷音频/封面 |
| `-overwrite` | false | 目标已存在同名文件时也覆盖 |

---

## 校验

```bash
# 启动服务
go run .

# 看看数据在不在
curl http://localhost:8080/api/music/stats
curl http://localhost:8080/api/lyrics/stats
```

再打开网页，随手点一首曲子，确认**能播放**（音频文件路径对不对）、**封面显示正常**。

---

## 切换：旧地址 302 跳新站点

迁完之后，旧系统的 `/music`、`/lyrics` 页面就不该再独立存在了 —— 让它跳到新站点。

在旧系统的反向代理（Nginx）里加 302：

```nginx
location = /music {
    return 302 https://music.jaxiu.cn/music;
}
location = /lyrics {
    return 302 https://music.jaxiu.cn/lyrics;
}
```

> **注意**：只跳页面，**不要跳 API**。
> 旧系统内部若有模块还在调 `/api/music/*`、`/api/lyrics/*`，
> 应该改成调新服务的 `/api/v1/music/*`、`/api/v1/lyrics/*`（带 API Key），
> 而不是让浏览器重定向 —— 重定向会把 POST 变成 GET，接口语义会坏。

---

## 回滚

迁移**不修改源库**，所以回滚很简单：

```bash
rm -f ./data/music.db
rm -rf ./data/music
```

然后把旧系统的路由改回去即可。

---

## 常见问题

**Q：迁移后播放没声音？**
A：音频文件没拷过去。检查 `-source-data-dir` 指向的目录里是不是真的有 `music/` 子目录。
也可以只迁文件不迁数据库：加 `-skip-files` 反过来用即可。

**Q：提示「源库没有这张表，跳过」？**
A：源库里本来就没有该模块的数据，正常。

**Q：旧库的表比目标多几列？**
A：迁移工具会跳过目标库没有的列，并在输出里提示跳过了几列 —— 这是预期行为，不会丢已有数据。

**Q：密钥（`lyric_api_keys`）要一起迁吗？**
A：迁。库里只存 sha256 哈希，明文不可恢复，所以**迁完之后旧系统里的那把密钥仍然能用**
（哈希一样），但你没法从新库里看到明文。如果不想保留旧密钥，迁完在「歌词库 → API 密钥」页面撤销即可。

**Q：迁移明明「全部成功」，打开曲库却报 500 / `TS.parse: 无法解析时间 … +0800 +0800`？**
A：旧版迁移工具的问题。驱动会把 `DATETIME` 文本读成 `time.Time`，原样写回时序列化成了
Go 的 `time.String()`（例：`2026-10-30 17:42:40.294064882 +0800 +0800`），本项目解析不了。
已在 `e57c05c` 修复（迁移时统一格式化为 `2006-01-02 15:04:05.999999999-07:00`，时间解析也做了兼容）。
如果是旧工具迁出来的库，删掉 `data/music.db` 用新工具重跑即可 —— 音频文件会被跳过，几秒完成。

**Q：迁移要多久？**
A：数据库一万行左右是秒级；瓶颈在音频拷贝。本机实测 3149 个文件 / 20.5 GB 约 2.5 分钟
（外部卷 bind mount，约 150 MB/s）。
