# 多阶段构建（纯 Go 后端，无 CGO，跨平台一行编译）
FROM golang:1.25-alpine AS gobuild
WORKDIR /src
# 现代 SQLite 驱动（modernc）为纯 Go，关闭 CGO 即可静态交叉编译。
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod ./
# 若处于无网络环境，可预先 go mod download 后再构建；此处按标准流程执行。
RUN go mod download || true
COPY . .
RUN go build -o /out/app .

# 运行期：前端构建产物在 Phase 6 接入后由 /frontend/dist 提供；
# 当前阶段回退到仓库内的占位 index.html。
FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=gobuild /out/app /app/app
COPY scripts /app/scripts
COPY config.example.yaml /app/config.example.yaml
COPY frontend/index.html /app/frontend/index.html
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/app"]
