#!/usr/bin/env bash
#
# music 发布脚本 —— 基于 Docker Compose 的一键构建 / 发布 / 回滚 / 运维。
#
# 用法：
#   ./scripts/deploy.sh <命令> [参数]
#
#   build      构建镜像（打上版本标签，默认 latest）
#   up         构建并后台启动服务，随后等待健康检查通过
#   down       停止并移除容器（不动数据卷）
#   restart    重启服务
#   update     拉取最新代码 → 重建镜像 → 重启（发布主流程）
#   status     查看容器与健康状态
#   logs       查看日志（-f 跟随）
#   health     手动探测 /api/health
#   backup     备份 SQLite 数据库（--full 连同音频/封面一起备）
#   restore    从备份恢复数据库（备份文件名或路径）
#   shell      进入容器 shell
#   clean      清理悬空镜像与构建缓存
#   help       显示本帮助
#
# 配置：项目根目录的 .env（可选，勿提交到 git）
#   HOST_PORT=8083                 宿主机映射端口
#   TZ=Asia/Shanghai               容器时区
#   ADMIN_PASSWORD=xxxx            站内管理口令（留空则纯访客模式）
#   MUSIC_IMAGE=music              镜像名
#   MUSIC_TAG=latest               镜像标签（发布时建议用版本号或 git sha）
#   BACKUP_DIR=./backups           备份目录
#
set -Eeuo pipefail

# ─────────────────────────── 基础环境 ───────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

ENV_FILE="${PROJECT_ROOT}/.env"
BACKUP_DIR="${PROJECT_ROOT}/backups"
DATA_DIR="${PROJECT_ROOT}/data"
DB_FILE="${DATA_DIR}/music.db"
SERVICE="music"

# ─────────────────────────── 输出helper ───────────────────────────
if [[ -t 1 ]]; then
  C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YLW=$'\033[33m'
  C_BLU=$'\033[34m'; C_RST=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YLW=""; C_BLU=""; C_RST=""
fi

log()  { printf '%s[%s]%s %s\n' "${C_BLU}" "$(date '+%H:%M:%S')" "${C_RST}" "$*"; }
ok()   { printf '%s✓%s %s\n' "${C_GRN}" "${C_RST}" "$*"; }
warn() { printf '%s!%s %s\n' "${C_YLW}" "${C_RST}" "$*" >&2; }
err()  { printf '%s✗%s %s\n' "${C_RED}" "${C_RST}" "$*" >&2; }
die()  { err "$*"; exit 1; }

trap 'err "命令在第 ${LINENO} 行失败（退出码 $?）"' ERR

# ─────────────────────────── 加载 .env ───────────────────────────
# 只解析 KEY=VALUE，不执行任何内容，避免 .env 里的命令被运行。
load_env() {
  [[ -f "${ENV_FILE}" ]] || return 0
  local line key val
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    [[ -z "${line}" || "${line}" == \#* ]] && continue
    [[ "${line}" != *=* ]] && continue
    key="${line%%=*}"
    val="${line#*=}"
    key="${key%"${key##*[![:space:]]}"}"
    val="${val#"${val%%[![:space:]]*}"}"
    if [[ "${val}" == \"*\" || "${val}" == \'*\' ]]; then
      val="${val:1:${#val}-2}"
    fi
    export "${key}=${val}"
  done < "${ENV_FILE}"
}
load_env

HOST_PORT="${HOST_PORT:-8083}"
TZ_NAME="${TZ:-Asia/Shanghai}"
MUSIC_IMAGE="${MUSIC_IMAGE:-music}"
MUSIC_TAG="${MUSIC_TAG:-latest}"
export HOST_PORT TZ_NAME MUSIC_IMAGE MUSIC_TAG
# compose 变量名用 TZ
export TZ="${TZ_NAME}"

# ─────────────────────────── Docker 前置检查 ───────────────────────────
if ! command -v docker >/dev/null 2>&1; then
  die "未找到 docker，请先安装 Docker。"
fi
if ! docker info >/dev/null 2>&1; then
  die "docker daemon 未运行（可能需要启动 Docker Desktop 或 sudo）。"
fi

# 兼容 compose v2（docker compose）与 v1（docker-compose）
if docker compose version >/dev/null 2>&1; then
  DC=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  DC=(docker-compose)
else
  die "未找到 docker compose（v2: docker compose / v1: docker-compose）。"
fi

dc() { "${DC[@]}" "$@"; }

# ─────────────────────────── 通用helper ───────────────────────────

# 当前运行中容器的镜像标签（用于 update 前的回滚提示）
current_image() {
  docker inspect --format '{{.Config.Image}}' "${SERVICE}" 2>/dev/null || echo ""
}

# 等待健康检查通过
wait_healthy() {
  local timeout="${1:-60}" i=0
  log "等待服务就绪（最多 ${timeout}s）..."
  while (( i < timeout )); do
    if dc ps --status running --services 2>/dev/null | grep -qx "${SERVICE}"; then
      if probe_quiet; then
        ok "服务已就绪：http://localhost:${HOST_PORT}"
        return 0
      fi
    fi
    sleep 1
    (( i++ ))
  done
  warn "等待超时，下面是最近日志："
  dc logs --tail 30 "${SERVICE}" 2>&1 || true
  return 1
}

# 探测健康接口（宿主机）
probe_quiet() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsS --max-time 3 "http://127.0.0.1:${HOST_PORT}/api/health" >/dev/null 2>&1
  elif command -v wget >/dev/null 2>&1; then
    wget -q -T 3 -O /dev/null "http://127.0.0.1:${HOST_PORT}/api/health" >/dev/null 2>&1
  else
    # 宿主机没有 curl/wget 就退回容器内探测
    dc exec -T "${SERVICE}" wget -q -T 3 -O /dev/null http://127.0.0.1:8080/api/health >/dev/null 2>&1
  fi
}

ensure_data_dir() {
  mkdir -p "${DATA_DIR}" "${BACKUP_DIR}"
  # 数据目录权限放开给容器内的非 root 用户
  chmod 777 "${DATA_DIR}" 2>/dev/null || true
}

# ─────────────────────────── 命令实现 ───────────────────────────

cmd_build() {
  ensure_data_dir
  log "构建镜像 ${MUSIC_IMAGE}:${MUSIC_TAG} ..."
  # 标签来自 compose 的 image: ${MUSIC_IMAGE}:${MUSIC_TAG}，无需再传 -t。
  dc build --pull "${SERVICE}"
  ok "镜像构建完成：${MUSIC_IMAGE}:${MUSIC_TAG}"
}

cmd_up() {
  ensure_data_dir
  local args=(up -d --remove-orphans)
  [[ "${1:-}" == "--rebuild" || "${1:-}" == "-b" ]] && args+=(--build)
  log "启动服务 ..."
  dc "${args[@]}"
  wait_healthy 90
}

cmd_down() {
  log "停止服务 ..."
  dc down --remove-orphans
  ok "服务已停止（数据卷 ./data 保留）"
}

cmd_restart() {
  log "重启服务 ..."
  dc restart "${SERVICE}"
  wait_healthy 60
}

cmd_update() {
  local prev_tag prev_image
  prev_image="$(current_image)"
  prev_tag="${prev_image##*:}"
  [[ "${prev_image}" == "${MUSIC_IMAGE}:${MUSIC_TAG}" ]] && prev_tag=""

  # 1. 拉取最新代码（仅在是 git 仓库且工作区没有阻塞性改动时）
  if [[ -d .git ]]; then
    if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
      warn "工作区有未提交的改动，跳过 git pull（继续用本地代码构建）。"
    else
      log "拉取最新代码 ..."
      git pull --ff-only || warn "git pull 失败，继续使用本地代码构建。"
    fi
  fi

  # 2. 用 git 短 sha 打标签，方便回滚
  if [[ -d .git ]]; then
    local sha
    sha="$(git rev-parse --short HEAD 2>/dev/null || true)"
    if [[ -n "${sha}" ]]; then
      export MUSIC_TAG="${sha}"
      log "本次发布标签：${MUSIC_IMAGE}:${MUSIC_TAG}"
    fi
  fi

  # 3. 备份数据库（发布前自动留一份）
  cmd_backup || warn "发布前备份失败，继续发布。"

  # 4. 重建并重启
  cmd_build
  dc up -d --remove-orphans
  wait_healthy 90

  if [[ -n "${prev_tag}" ]]; then
    ok "发布完成。若需回滚：./scripts/deploy.sh rollback ${prev_tag}"
  else
    ok "发布完成。"
  fi
}

cmd_rollback() {
  local tag="${1:-}"
  [[ -n "${tag}" ]] || die "用法：$0 rollback <上一个标签>"
  docker image inspect "${MUSIC_IMAGE}:${tag}" >/dev/null 2>&1 \
    || die "本地不存在镜像 ${MUSIC_IMAGE}:${tag}，无法回滚。"
  log "回滚到 ${MUSIC_IMAGE}:${tag} ..."
  export MUSIC_TAG="${tag}"
  cmd_backup || warn "回滚前备份失败。"
  dc up -d --no-build --force-recreate "${SERVICE}"
  wait_healthy 60
  ok "已回滚到 ${MUSIC_IMAGE}:${tag}"
}

cmd_status() {
  dc ps
  echo
  local st
  st="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${SERVICE}" 2>/dev/null || echo "未运行")"
  if probe_quiet; then
    ok "健康检查通过 → http://localhost:${HOST_PORT}/api/health"
  else
    warn "健康检查未通过（容器状态：${st}）"
  fi
  echo
  log "数据目录：${DATA_DIR}"
  log "最近备份："
  if [[ -d "${BACKUP_DIR}" ]]; then
    find "${BACKUP_DIR}" -maxdepth 1 -name '*.db' -type f -print 2>/dev/null | sort -r | head -3 | sed 's/^/  /'
  fi
}

cmd_logs() {
  if [[ "${1:-}" == "-f" || "${1:-}" == "--follow" ]]; then
    dc logs -f --tail 200 "${SERVICE}"
  else
    dc logs --tail 200 "${SERVICE}"
  fi
}

cmd_health() {
  if probe_quiet; then
    ok "健康检查通过"
    if command -v curl >/dev/null 2>&1; then
      curl -fsS "http://127.0.0.1:${HOST_PORT}/api/health"; echo
    fi
  else
    err "健康检查失败（http://127.0.0.1:${HOST_PORT}/api/health）"
    return 1
  fi
}

cmd_backup() {
  ensure_data_dir
  [[ -f "${DB_FILE}" ]] || { warn "没有找到 ${DB_FILE}，跳过备份。"; return 0; }
  local stamp file
  stamp="$(date '+%Y%m%d-%H%M%S')"
  file="${BACKUP_DIR}/music-${stamp}.db"
  cp "${DB_FILE}" "${file}"
  ok "数据库已备份 → ${file}"

  if [[ "${1:-}" == "--full" ]]; then
    if [[ -d "${DATA_DIR}/music" ]]; then
      local media="${BACKUP_DIR}/media-${stamp}.tar.gz"
      tar -czf "${media}" -C "${DATA_DIR}" music
      ok "媒体文件已备份 → ${media}"
    else
      warn "没有 ${DATA_DIR}/music 目录，跳过媒体备份。"
    fi
  fi
}

cmd_restore() {
  local src="${1:-}"
  [[ -n "${src}" ]] || die "用法：$0 restore <备份文件路径>"
  [[ -f "${src}" ]] || die "备份文件不存在：${src}"
  ensure_data_dir
  log "停止服务 ..."
  dc stop "${SERVICE}" >/dev/null 2>&1 || true
  local safety
  safety="${BACKUP_DIR}/music-before-restore-$(date '+%Y%m%d-%H%M%S').db"
  [[ -f "${DB_FILE}" ]] && cp "${DB_FILE}" "${safety}"
  cp "${src}" "${DB_FILE}"
  ok "数据库已恢复（原库备份于 ${safety}）"
  log "启动服务 ..."
  dc up -d "${SERVICE}"
  wait_healthy 60
}

cmd_shell() {
  dc exec "${SERVICE}" /bin/sh
}

cmd_clean() {
  log "清理构建缓存 ..."
  docker builder prune -f >/dev/null
  log "清理悬空镜像 ..."
  docker image prune -f >/dev/null
  ok "清理完成"
}

usage() {
  cat <<'EOF'
music 发布脚本 —— 基于 Docker Compose

  build      构建镜像（打上版本标签，默认 latest）
  up         构建并后台启动服务，随后等待健康检查通过
  down       停止并移除容器（不动数据卷）
  stop       只停止容器
  restart    重启服务
  update     拉取最新代码 → 备份 → 重建镜像 → 重启（发布主流程）
  rollback   回滚到上一个标签：$0 rollback <标签>
  status     查看容器与健康状态
  logs       查看日志（-f 跟随）
  health     手动探测 /api/health
  backup     备份 SQLite 数据库（--full 连同音频/封面一起备）
  restore    从备份恢复数据库：$0 restore <备份文件路径>
  shell      进入容器 shell
  clean      清理悬空镜像与构建缓存
  help       显示本帮助

配置：项目根目录 .env（可选）
  HOST_PORT=8083  TZ=Asia/Shanghai  ADMIN_PASSWORD=xxxx
  MUSIC_IMAGE=music  MUSIC_TAG=latest  BACKUP_DIR=./backups
EOF
}

# ─────────────────────────── 入口 ───────────────────────────
main() {
  local cmd="${1:-help}"
  shift || true
  case "${cmd}" in
    build)   cmd_build "$@" ;;
    up)      cmd_up "$@" ;;
    down)    cmd_down "$@" ;;
    stop)    dc stop "${SERVICE}"; ok "服务已停止" ;;
    restart) cmd_restart "$@" ;;
    update|publish|release) cmd_update "$@" ;;
    rollback) cmd_rollback "$@" ;;
    status|ps) cmd_status "$@" ;;
    logs)    cmd_logs "$@" ;;
    health)  cmd_health "$@" ;;
    backup)  cmd_backup "$@" ;;
    restore) cmd_restore "$@" ;;
    shell)   cmd_shell "$@" ;;
    clean)   cmd_clean "$@" ;;
    help|-h|--help) usage ;;
    *) err "未知命令：${cmd}"; echo; usage; exit 1 ;;
  esac
}

main "$@"
