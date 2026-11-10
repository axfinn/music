# 多阶段构建：前端(Vite) → 后端(纯 Go) → 运行期。
#
# 前端必须在这里构建一次：Go 后端直接托管 frontend/dist，
# 镜像里没有 dist 就只剩一张占位页（这正是「运行后没有前端」的根因）。

# ── 前端 ──
FROM node:22-alpine AS frontendbuild
WORKDIR /web
# 先只拷清单，让依赖层能被缓存：改业务代码不会触发重装依赖。
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── 后端 ──
FROM golang:1.25-alpine AS gobuild
WORKDIR /src
# 现代 SQLite 驱动（modernc）为纯 Go，关闭 CGO 即可静态交叉编译。
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod ./
# 若处于无网络环境，可预先 go mod download 后再构建；此处按标准流程执行。
RUN go mod download || true
COPY . .
RUN go build -o /out/app .

# ── 运行期 ──
FROM alpine:3.20
WORKDIR /app
# python3：歌词在线抓取依赖 scripts/lyrics_finder.py（纯标准库，无需 pip 依赖）。
RUN apk add --no-cache ca-certificates python3 tzdata
COPY --from=gobuild /out/app /app/app
COPY --from=frontendbuild /web/dist /app/frontend/dist
COPY scripts /app/scripts
COPY config.example.yaml /app/config.example.yaml
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/app"]
