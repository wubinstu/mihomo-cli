#!/usr/bin/env bash
# 补全自检: 逐路径调用 cobra 的 __complete, 断言:
#   1) 返回值不含任何文件系统条目 (mihomo-cli 不与文件打交道)
#   2) directive 恒为 :4 (ShellCompDirectiveNoFileComp)
#   3) 关键路径能给出候选值
# 用法: scripts/completion-check.sh [path-to-binary]
set -uo pipefail

BIN="${1:-./mihomo-cli}"
[[ -x $BIN ]] || BIN="$(command -v mihomo-cli)"
fail=0
ok=0

check() { # check <args...>  —— 断言 NoFileComp 且不出现文件名
  local out
  out=$("$BIN" __complete "$@" 2>/dev/null)
  local dir
  dir=$(tail -1 <<<"$out")
  if [[ "$dir" != *":4" ]]; then
    echo "✘ [$*] directive=$dir (期望 :4)" >&2
    fail=$((fail+1)); return
  fi
  # 文件补全的特征: 末尾带 / 或以 cwd 下的文件名出现
  if grep -qE '(^| )([^ /]*/|config\.toml|overrides\.yaml|mihomo-cli|\.go|\.md)$' <<<"$out"; then
    echo "✘ [$*] 出现文件系统补全" >&2
    fail=$((fail+1)); return
  fi
  ok=$((ok+1))
}

expect() { # expect <期望子串> <args...>
  local want="$1"; shift
  local out
  out=$("$BIN" __complete "$@" 2>/dev/null)
  if ! grep -q "$want" <<<"$out"; then
    echo "✘ [$*] 期望包含 $want, 实际: $(head -2 <<<"$out" | tr '\n' ' ')" >&2
    fail=$((fail+1)); return
  fi
  ok=$((ok+1))
}

# --- 空命令行 (v1.2 及更早在这里漏出 config.toml 的根因) ---
check ""
# --- 两层 key 补全 ---
check "config" "set" ""
expect "allow-lan"    "config" "set" ""
expect "dns.enable"   "config" "set" "dns."
expect "tun.stack"    "config" "set" "tun."
expect "sub-auto-update-enabled" "config" "set" ""
# --- 每个 key 的值补全都必须有候选 ---
for k in allow-lan mixed-port socks-port http-port proxy-mode ipv6-enabled log-level \
         tcp-concurrent unified-delay keep-alive-interval cli-language github-mirror \
         test-url test-timeout sub-auto-update-enabled sub-auto-update-interval \
         node-auto-select-enabled node-auto-select-interval \
         resource-auto-update-enabled resource-auto-update-interval; do
  expect "." "config" "set" "$k" ""
done
# --- 其余命令树: 一律不落文件 ---
for p in "sub" "sub add" "sub use " "sub rm " "group" "group use " "node" "node use " \
         "rule" "rule add" "rule enable " "top" "top kill " "dns" "dns use " "tun" \
         "install" "install --core" "install --resource" "install --completion" \
         "uninstall" "uninstall --completion" "resource" "resource core" \
         "resource core upgrade" "resource mmdb" "config get" "config reset-default" \
         "config update-file" "config update-service" "config unset"; do
  check $p
done

echo "补全自检: $ok 项通过, $fail 项失败"
[[ $fail -eq 0 ]]
