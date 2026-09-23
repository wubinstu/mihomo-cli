#!/usr/bin/env bash
# mihomo-cli 一键安装脚本
# 用法: curl -fsSL .../install.sh | bash -s -- [--proxy http://host:port]
set -euo pipefail

REPO="wubinstu/mihomo-cli"
PROXY="${PROXY:-}"
MIRROR="${MIRROR:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --proxy) PROXY="$2"; shift 2;;
    --proxy=*) PROXY="${1#--proxy=}"; shift;;
    --mirror) MIRROR="$2"; shift 2;;
    --mirror=*) MIRROR="${1#--mirror=}"; shift;;
    *) echo "未知参数: $1"; shift;;
  esac
done

# 系统级配置 (/etc/mihomo-cli) 与 systemd 单元需要 root
if [ "$(id -u)" != 0 ]; then
  echo ">> 需要 root, 尝试 sudo 重新执行 ..."
  exec sudo bash "$0" "$@"
fi

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH2="x86_64";;
  aarch64) ARCH2="aarch64";;
  armv7l|armhf) ARCH2="arm";;
  i686|i386) ARCH2="386";;
  *) echo "不支持的架构: $ARCH"; exit 1;;
esac

echo ">> 安装 mihomo-cli (linux/$ARCH2)"
URL="https://github.com/${REPO}/releases/latest/download/mihomo-cli_linux_${ARCH2}.tar.gz"

# 依次尝试: 直连 -> 常用镜像站 (可用 MIRROR=... 覆盖)
MIRRORS=("$MIRROR" "https://ghfast.top" "https://gh-proxy.com" "https://mirror.ghproxy.com")

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if [ -n "$PROXY" ]; then
  export https_proxy="$PROXY" http_proxy="$PROXY"
  echo ">> 使用代理: $PROXY"
fi

try_download() {
  local u
  for u in "$URL" "${MIRRORS[@]}"; do
    [ -z "$u" ] && continue
    echo ">> 下载: $u"
    if curl -fsSL --retry 2 --connect-timeout 15 -o "$TMP/mihomo-cli.tar.gz" "$u"; then
      return 0
    fi
  done
  return 1
}

if ! try_download; then
  echo "<< 下载失败, 请检查网络/镜像 (MIRROR=https://... bash install.sh)"; exit 1
fi
tar xzf "$TMP/mihomo-cli.tar.gz" -C "$TMP"

SUDO=""
[ "$(id -u)" != 0 ] && SUDO="sudo"
$SUDO install -m 0755 "$TMP/mihomo-cli" /usr/local/bin/mihomo-cli

echo ">> 完成: $(mihomo-cli version)"
echo ">> 下一步: mihomo-cli install [--proxy ...] && mihomo-cli init && mihomo-cli start"
