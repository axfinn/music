# 参与贡献

感谢你愿意为 **music** 花时间。这个项目很小，流程也尽量简单。

## 开发环境

| 依赖 | 版本 | 必需？ |
|---|---|---|
| Go | 1.25+ | ✅ 后端必需 |
| Node.js | 18+ | 前端开发必需 |
| Python 3 | 3.8+ | 可选，仅在线歌词抓取需要 |

SQLite **不需要**安装 —— 走 [modernc.org/sqlite](https://modernc.org/sqlite) 纯 Go 驱动，**无需 CGO**。

```bash
git clone https://github.com/axfinn/music.git
cd music

# 后端
go run .

# 前端（另开一个终端）
cd frontend && npm install && npm run dev
```

## 提交前检查

```bash
go build ./...
go vet ./...
cd frontend && npm run build
```

仓库没有引入 lint 工具链，请在 PR 里说明你改了什么、为什么。

## 代码约定

**后端（Go）**

- 包路径形如 `github.com/axfinn/music/backend/xxx`，不要用相对导入。
- 注释用中文，解释**为什么**这么做，而不是复述代码在做什么。
- 时间戳统一用 `models.TS` 类型（`sql.Scanner` + `driver.Valuer` + JSON RFC3339），
  不要直接用 `time.Time` 扫 SQLite 文本时间。
- 站内接口与 `/api/v1/*` 对外接口**保持隔离**：对外接口用 `v1OK` / `v1Fail` 信封，
  站内接口直接返回业务对象。不要把两套混着写。
- 对外接口新增端点时，同步更新 `docs/api.md`。

**前端（Vue 3）**

- 一律用 `<script setup>` 写 `.vue` SFC，**不要**用 `defineComponent({ template })` —— 运行时构建不支持。
- 设计系统用 `frontend/src/styles/theme.css` 里的 `--ml-*` 变量，**不要**引入宿主应用的全局样式。
- 新状态变量放在 `<script setup>` 末尾，避免 TDZ 导致白屏。
- `el-form` 里的 `<button>` 必须写 `type="button"`，否则会误提交表单。

## 提交信息

用中文描述做了什么，格式保持简洁：

```
功能：歌词发现融合本地与在线结果
修复：快照 rev 在筛选条件变化后未重算
文档：补充 v1 接口的限流说明
```

## 不要提交的东西

`.gitignore` 已经挡住了大部分，但请特别留意：

- ❌ `config.yaml`（可能含管理员密码）
- ❌ `data/` 目录（SQLite 数据库、音频、封面）
- ❌ `frontend/dist/`、`frontend/node_modules/`
- ❌ 任何 API Key 明文

**本仓库只包含代码。** 如果你误提交了数据或密钥，请立即在 PR 里说明，并联系维护者处理历史记录。

## 提 PR

1. 从 `main` 切出分支：`git checkout -b feat/xxx`
2. 提交代码，确保上面的检查命令都通过
3. 推送到你的 fork，向上游 `main` 提 PR
4. PR 描述里写清楚**改了什么**和**为什么**

## 报告问题

提 issue 时请附上：

- 你的系统（macOS / Linux / Docker）
- Go / Node 版本
- 复现步骤
- 相关日志（**注意删掉密钥、密码和隐私数据**）

## 许可

提交即表示你同意你的贡献以 [MIT](LICENSE) 协议发布。
