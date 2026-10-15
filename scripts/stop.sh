#!/usr/bin/env bash
# ============================================================================
# DNSLogHUB - Linux 停止脚本 (stop.sh)
#
# 功能：优雅停止 dnsloghub（SIGTERM -> 超时 SIGKILL），随后恢复
#       systemd-resolved 与 /etc/resolv.conf 到启动前状态。幂等。
#
# 退出码：
#   0  成功（含“未在运行”的幂等场景）
#   1  通用错误
#   2  参数错误
#   3  权限不足
#
# 用法：
#   sudo ./stop.sh
#   sudo ./stop.sh --help
#
# 环境变量（与 start.sh 一致）：
#   DNSLOGHUB_BIN / DNSLOGHUB_CONFIG / DNSLOGHUB_RUN_DIR
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -x "${SCRIPT_DIR}/dnsloghub" ]]; then
  PROJECT_DIR="${SCRIPT_DIR}"
else
  PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

RUN_DIR="${DNSLOGHUB_RUN_DIR:-${PROJECT_DIR}/run}"
PID_FILE="${RUN_DIR}/dnsloghub.pid"
STATE_FILE="${RUN_DIR}/start.state"
RESOLV_BACKUP="${RUN_DIR}/resolv.conf.bak"
RESOLV_CONF="/etc/resolv.conf"
RESOLVED_UNIT="systemd-resolved"
STUB_RESOLV="/run/systemd/resolve/stub-resolv.conf"

TERM_TIMEOUT="${DNSLOGHUB_STOP_TIMEOUT:-10}"   # SIGTERM 后等待秒数

log()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO]  $*"; }
warn() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [WARN]  $*" >&2; }
err()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*" >&2; }

usage() {
  cat <<EOF
用法: sudo $0 [选项]

选项:
  --help        显示此帮助
  --skip-dns    不恢复 systemd-resolved 与 /etc/resolv.conf（仅停止进程）

说明:
  - 需要 root 权限
  - 幂等：未在运行时不报错；重复执行安全
  - 优先 SIGTERM 优雅停止，超时后 SIGKILL
  - 恢复 systemd-resolved：仅当 start.sh 记录其原本为 enabled/active 时才恢复
EOF
}

SKIP_DNS=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --skip-dns) SKIP_DNS=1; shift ;;
    *) err "未知参数: $1"; usage >&2; exit 2 ;;
  esac
done

# 权限检查
if [[ $EUID -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    log "当前非 root，使用 sudo 重新执行..."
    exec sudo "$0" "$@"
  fi
  err "需要 root 权限，请使用：sudo $0"
  exit 3
fi

# ---------------------------------------------------------------------------
# 定位并停止进程
# ---------------------------------------------------------------------------
stop_pid() {
  local pid="$1"
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  log "发送 SIGTERM 到 PID $pid ..."
  kill -TERM "$pid" 2>/dev/null || true

  local waited=0
  while kill -0 "$pid" 2>/dev/null; do
    if (( waited >= TERM_TIMEOUT )); then
      warn "SIGTERM 后 ${TERM_TIMEOUT}s 仍未退出，发送 SIGKILL"
      kill -KILL "$pid" 2>/dev/null || true
      break
    fi
    sleep 0.5
    waited=$(( waited + 1 ))
  done

  if kill -0 "$pid" 2>/dev/null; then
    err "进程 $pid 无法停止"
    return 1
  fi
  log "进程 $pid 已退出"
  return 0
}

stopped=0
if [[ -f "$PID_FILE" ]]; then
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    stop_pid "$pid" || exit 1
    stopped=1
  else
    warn "PID 文件中的进程 ($pid) 已不存在"
  fi
  rm -f "$PID_FILE"
fi

# 按精确进程名兜底（避免误杀同名无关进程）
mapfile -t pids < <(pgrep -x dnsloghub 2>/dev/null || true)
for pid in "${pids[@]}"; do
  stop_pid "$pid" || exit 1
  stopped=1
done

if [[ $stopped -eq 0 ]]; then
  log "dnsloghub 未在运行，无需停止"
else
  log "dnsloghub 已停止"
fi

# ---------------------------------------------------------------------------
# 恢复 systemd-resolved 与 resolv.conf
# ---------------------------------------------------------------------------
if [[ $SKIP_DNS -eq 1 ]]; then
  log "--skip-dns：跳过 DNS 相关恢复"
  rm -f "$STATE_FILE"
  exit 0
fi

resolved_present=0
was_enabled="absent"
was_active="absent"
if [[ -f "$STATE_FILE" ]]; then
  # shellcheck disable=SC1090
  source "$STATE_FILE"
fi

# 恢复 systemd-resolved（仅恢复到启动前状态，不擅自改变系统）
if systemctl list-unit-files "$RESOLVED_UNIT.service" >/dev/null 2>&1; then
  if [[ "$resolved_present" == "1" ]]; then
    if [[ "$was_enabled" == "enabled" ]]; then
      log "恢复 systemd-resolved 开机自启"
      systemctl enable "$RESOLVED_UNIT" 2>/dev/null || warn "enable 失败"
    else
      log "systemd-resolved 原本未启用，保持现状（不擅自 enable）"
    fi
    if [[ "$was_active" == "active" ]]; then
      log "启动 systemd-resolved"
      systemctl start "$RESOLVED_UNIT" 2>/dev/null || warn "start 失败"
    else
      log "systemd-resolved 原本未运行，保持现状（不擅自 start）"
    fi
  else
    log "start.sh 未记录到 systemd-resolved（不存在），跳过恢复"
  fi
else
  log "当前系统不存在 systemd-resolved，跳过"
fi

# 恢复 resolv.conf
if [[ -e "$RESOLV_BACKUP" || -L "$RESOLV_BACKUP" ]]; then
  log "从备份恢复 $RESOLV_CONF"
  rm -f "$RESOLV_CONF"
  cp -a "$RESOLV_BACKUP" "$RESOLV_CONF"
elif systemctl list-unit-files "$RESOLVED_UNIT.service" >/dev/null 2>&1; then
  log "无备份，恢复 systemd-resolved 软链接 -> $STUB_RESOLV"
  rm -f "$RESOLV_CONF"
  ln -sf "$STUB_RESOLV" "$RESOLV_CONF"
else
  warn "既无备份也无 systemd-resolved，无法自动恢复 $RESOLV_CONF，请手动检查"
fi

# ---------------------------------------------------------------------------
# 验证
# ---------------------------------------------------------------------------
log "验证 DNS 解析与端口状态 ..."
if command -v getent >/dev/null 2>&1; then
  getent hosts example.com >/dev/null 2>&1 && log "DNS 解析正常" || warn "DNS 解析验证失败（请检查 $RESOLV_CONF）"
fi
if ss -lunp 2>/dev/null | grep -E ':53([[:space:]]|$)' | grep -q 'dnsloghub'; then
  warn "仍检测到 dnsloghub 监听 53 端口"
else
  log "53 端口已释放"
fi
if systemctl list-unit-files "$RESOLVED_UNIT.service" >/dev/null 2>&1; then
  log "systemd-resolved 当前状态: $(systemctl is-active "$RESOLVED_UNIT" 2>/dev/null || echo unknown)"
fi

# 清理状态文件
rm -f "$STATE_FILE"
log "已清理状态文件与 PID 文件"
log "停止完成"
exit 0
