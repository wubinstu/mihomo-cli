#!/usr/bin/env bash
# mihomo-cli 一键安装脚本
# 用法: curl -fsSL .../install.sh | bash -s -- [--proxy http://host:port]
set -euo pipefail

REPO="wubinstu/mihomo-cli"
PROXY="${PROXY:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --proxy) PROXY="$2"; shift 2;;
    --proxy=*) PROXY="${1#--proxy=}"; shift;;
    *) echo "未知参数: $1"; shift;;
  esac
done

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

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if [ -n "$PROXY" ]; then
  export https_proxy="$PROXY" http_proxy="$PROXY"
  echo ">> 使用代理: $PROXY"
fi

download() {
  local i
  for i in 1 2 3; do
    curl -fsSL --retry 2 -o "$2" "$1" && return 0
    echo ">> 重试 ($i/3) ..."
    sleep 2
  done
  return 1
}

if ! download "$URL" "$TMP/mihomo-cli.tar.gz"; then
  echo "<< 下载失败, 请检查网络或手动下载: $URL"; exit 1
fi
tar xzf "$TMP/mihomo-cli.tar.gz" -C "$TMP"

SUDO=""
[ "$(id -u)" != 0 ] && SUDO="sudo"
$SUDO install -m 0755 "$TMP/mihomo-cli" /usr/local/bin/mihomo-cli

echo ">> 完成: $(mihomo-cli version)"
echo ">> 下一步: mihomo-cli install [--proxy ...] && mihomo-cli init && mihomo-cli start"
