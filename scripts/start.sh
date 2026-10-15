#!/usr/bin/env bash
# ============================================================================
# DNSLogHUB - Linux 部署/启动脚本 (start.sh)
#
# 功能：在 systemd 发行版（Ubuntu/Debian 等）上以后台方式启动 dnsloghub，
#       自动处理 systemd-resolved 占用 53 端口的问题，并具备幂等、可回滚、
#       错误处理与清晰日志。
#
# 退出码：
#   0  成功
#   1  通用错误
#   2  参数错误
#   3  权限不足
#   4  依赖缺失
#   5  启动失败（已尝试回滚）
#   6  服务已在运行（幂等）
#
# 用法：
#   sudo ./start.sh            # 启动
#   sudo ./start.sh --help     # 查看帮助
#
# 环境变量（可选覆盖）：
#   DNSLOGHUB_BIN        dnsloghub 二进制路径
#   DNSLOGHUB_CONFIG     config.yaml 路径
#   DNSLOGHUB_RUN_DIR    运行目录（PID/状态/日志/备份）
#   DNSLOGHUB_DNS1        主 DNS（默认 223.5.5.5）
#   DNSLOGHUB_DNS2        备 DNS（默认 114.114.114.114）
# ============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# 基本配置
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -x "${SCRIPT_DIR}/dnsloghub" ]]; then
  PROJECT_DIR="${SCRIPT_DIR}"
else
  PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

BIN="${DNSLOGHUB_BIN:-${PROJECT_DIR}/dnsloghub}"
CONFIG="${DNSLOGHUB_CONFIG:-${PROJECT_DIR}/config.yaml}"
RUN_DIR="${DNSLOGHUB_RUN_DIR:-${PROJECT_DIR}/run}"
PID_FILE="${RUN_DIR}/dnsloghub.pid"
STATE_FILE="${RUN_DIR}/start.state"
LOG_FILE="${RUN_DIR}/dnsloghub.log"
RESOLV_BACKUP="${RUN_DIR}/resolv.conf.bak"
DNS1="${DNSLOGHUB_DNS1:-223.5.5.5}"
DNS2="${DNSLOGHUB_DNS2:-114.114.114.114}"

# systemd-resolved 相关
RESOLVED_UNIT="systemd-resolved"
STUB_RESOLV="/run/systemd/resolve/stub-resolv.conf"
RESOLV_CONF="/etc/resolv.conf"

# ---------------------------------------------------------------------------
# 日志函数
# ---------------------------------------------------------------------------
log()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO]  $*"; }
warn() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [WARN]  $*" >&2; }
err()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*" >&2; }

usage() {
  cat <<EOF
用法: sudo $0 [选项]

选项:
  --help        显示此帮助
  --skip-dns    不处理 systemd-resolved 与 /etc/resolv.conf（仅启动二进制）
  --dns1 <IP>   主 DNS（默认 ${DNS1}）
  --dns2 <IP>   备 DNS（默认 ${DNS2}）

说明:
  - 需要 root 权限（绑定 53 端口、写 /etc/resolv.conf、操作 systemd-resolved）
  - 幂等：检测到已在运行时不会重复启动
  - 启动失败会自动回滚 systemd-resolved 与 /etc/resolv.conf

NetworkManager / resolvconf 用户注意：
  若 DNS 由 NetworkManager 或 resolvconf 管理，直接改写 /etc/resolv.conf
  可能在重启后被覆盖。推荐改用 systemd-resolved 的 stub 监听关闭方案：
      sudo mkdir -p /etc/systemd/resolved.conf.d
      printf '[Resolve]\nDNSStubListener=no\n' \\
        | sudo tee /etc/systemd/resolved.conf.d/dnsloghub.conf
      sudo systemctl restart systemd-resolved
  之后 53 端口将不再被占用，无需修改 /etc/resolv.conf。
EOF
}

# 解析参数
SKIP_DNS=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --skip-dns) SKIP_DNS=1; shift ;;
    --dns1) DNS1="${2:-}"; shift 2 ;;
    --dns2) DNS2="${2:-}"; shift 2 ;;
    *) err "未知参数: $1"; usage >&2; exit 2 ;;
  esac
done

# ---------------------------------------------------------------------------
# 权限检查：非 root 时若可用 sudo 则提权重跑，否则报错退出
# ---------------------------------------------------------------------------
if [[ $EUID -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    log "当前非 root，使用 sudo 重新执行..."
    exec sudo "$0" "$@"
  fi
  err "需要 root 权限，请使用：sudo $0"
  exit 3
fi

# ---------------------------------------------------------------------------
# 依赖检查
# ---------------------------------------------------------------------------
require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "缺少依赖命令: $1"
    exit 4
  fi
}
require_cmd systemctl
require_cmd ss

if [[ ! -x "$BIN" ]]; then
  err "未找到 dnsloghub 二进制: $BIN（可用 DNSLOGHUB_BIN 指定）"
  exit 4
fi
if [[ ! -f "$CONFIG" ]]; then
  err "未找到配置文件: $CONFIG（可用 DNSLOGHUB_CONFIG 指定）"
  exit 4
fi

mkdir -p "$RUN_DIR"

# ---------------------------------------------------------------------------
# 幂等检查：已在运行则直接退出
# ---------------------------------------------------------------------------
if [[ -f "$PID_FILE" ]]; then
  old_pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [[ -n "$old_pid" ]] && kill -0 "$old_pid" 2>/dev/null; then
    log "dnsloghub 已在运行 (PID $old_pid)，跳过启动"
    exit 6
  fi
  warn "PID 文件存在但进程已不存在，清理后继续"
  rm -f "$PID_FILE"
fi
# 按精确进程名兜底检查（避免误判其它同名进程）
running_pid="$(pgrep -x dnsloghub 2>/dev/null | head -n1 || true)"
if [[ -n "$running_pid" ]]; then
  log "检测到已运行的 dnsloghub (PID $running_pid)，跳过启动"
  exit 6
fi

# ---------------------------------------------------------------------------
# 53 端口占用检查（只读，不盲目结束进程）
# ---------------------------------------------------------------------------
port53="$(ss -lunp 2>/dev/null | grep -E ':53([[:space:]]|$)' || true)"
if [[ -n "$port53" ]]; then
  log "53 端口当前被占用："
  log "$port53"
  if echo "$port53" | grep -qi 'systemd-resolve'; then
    log "占用者为 systemd-resolved，将按流程处理"
  elif [[ $SKIP_DNS -eq 1 ]]; then
    warn "--skip-dns 模式下不处理 53 端口占用，dnsloghub 可能无法绑定 53 端口"
  else
    err "53 端口被非 systemd-resolved 的进程占用，为避免误杀，请手动处理后再启动"
    err "提示：用 'sudo ss -lunp | grep :53' 查看占用者"
    exit 4
  fi
else
  log "53 端口当前空闲"
fi

# ---------------------------------------------------------------------------
# 处理 systemd-resolved（仅在未跳过且 systemd-resolved 存在时）
# ---------------------------------------------------------------------------
resolved_present=0
if systemctl list-unit-files "$RESOLVED_UNIT.service" >/dev/null 2>&1; then
  resolved_present=1
fi

# 记录当前状态（供 stop.sh 恢复）
if [[ $resolved_present -eq 1 ]]; then
  was_enabled="$(systemctl is-enabled "$RESOLVED_UNIT" 2>/dev/null || echo 'unknown')"
  was_active="$(systemctl is-active  "$RESOLVED_UNIT" 2>/dev/null || echo 'unknown')"
else
  was_enabled="absent"
  was_active="absent"
fi

# 备份 resolv.conf（cp -a 保留软链接属性）
if [[ -e "$RESOLV_CONF" || -L "$RESOLV_CONF" ]]; then
  cp -a "$RESOLV_CONF" "$RESOLV_BACKUP"
  log "已备份 $RESOLV_CONF -> $RESOLV_BACKUP"
else
  warn "$RESOLV_CONF 不存在，跳过备份"
fi

cat > "$STATE_FILE" <<EOF
resolved_present=$resolved_present
resolved_was_enabled=$was_enabled
resolved_was_active=$was_active
resolv_backup=$RESOLV_BACKUP
EOF
log "已记录 systemd-resolved 状态 -> $STATE_FILE"

if [[ $SKIP_DNS -eq 0 && $resolved_present -eq 1 ]]; then
  log "停止并禁用 systemd-resolved ..."
  systemctl stop "$RESOLVED_UNIT" 2>/dev/null || warn "停止 systemd-resolved 失败（可能已停止）"
  systemctl disable "$RESOLVED_UNIT" 2>/dev/null || warn "禁用 systemd-resolved 失败"

  log "删除旧软链接并写入公共 DNS ..."
  rm -f "$RESOLV_CONF"
  {
    printf 'nameserver %s\n' "$DNS1"
    printf 'nameserver %s\n' "$DNS2"
  } | tee "$RESOLV_CONF" >/dev/null
  log "已写入 $RESOLV_CONF: $DNS1, $DNS2"
elif [[ $SKIP_DNS -eq 0 && $resolved_present -eq 0 ]]; then
  log "未检测到 systemd-resolved，跳过其处理"
fi

# ---------------------------------------------------------------------------
# 后台启动
# ---------------------------------------------------------------------------
log "启动 dnsloghub: $BIN"
setsid "$BIN" >>"$LOG_FILE" 2>&1 < /dev/null &
pid=$!
echo "$pid" > "$PID_FILE"
log "已写入 PID $pid -> $PID_FILE"

# 等待启动并验证
sleep 2
rollback() {
  err "启动失败，开始回滚..."
  # 1) 停止刚启动的进程
  if kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    sleep 1
    kill -KILL "$pid" 2>/dev/null || true
  fi
  rm -f "$PID_FILE"
  # 2) 恢复 resolv.conf
  if [[ -e "$RESOLV_BACKUP" || -L "$RESOLV_BACKUP" ]]; then
    rm -f "$RESOLV_CONF"
    cp -a "$RESOLV_BACKUP" "$RESOLV_CONF"
    log "已恢复 $RESOLV_CONF"
  fi
  # 3) 恢复 systemd-resolved
  if [[ $resolved_present -eq 1 ]]; then
    if [[ "$was_enabled" == "enabled" ]]; then
      systemctl enable "$RESOLVED_UNIT" 2>/dev/null || true
    fi
    if [[ "$was_active" == "active" ]]; then
      systemctl start "$RESOLVED_UNIT" 2>/dev/null || true
    fi
    log "已恢复 systemd-resolved（enabled=$was_enabled active=$was_active）"
  fi
  exit 5
}

# 进程存活校验
if ! kill -0 "$pid" 2>/dev/null; then
  err "进程启动后立即退出，日志末尾："
  tail -n 20 "$LOG_FILE" >&2 || true
  rollback
fi

# 53 端口监听校验（仅在未 skip-dns 时强校验）
if [[ $SKIP_DNS -eq 0 ]]; then
  if ! ss -lunp 2>/dev/null | grep -E ':53([[:space:]]|$)' | grep -q 'dnsloghub'; then
    err "53 端口未由 dnsloghub 监听，日志末尾："
    tail -n 20 "$LOG_FILE" >&2 || true
    rollback
  fi
fi

log "启动成功：PID=$pid，53 端口已监听，日志 -> $LOG_FILE"
log "状态文件 -> $STATE_FILE"
exit 0
