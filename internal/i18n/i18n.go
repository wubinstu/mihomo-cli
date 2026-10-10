// Package i18n 提供中/英文输出。语言来源: config.toml 的 lang 项 > $LANG 环境变量 > 中文。
// T() 以中文原文为 key, 英文表未收录时原样返回中文。
package i18n

import (
	"os"
	"sort"
	"strings"

	"github.com/wubinstu/mihomo-cli/internal/app"
)

var lang = "zh"

// init 只留中文默认值: 命令的 Short/Long 是在包初始化时 T() 的, 那时还不能读配置
// (读出来也不是最终语言)。真正的语言判定推迟到 main → cmd.Execute() → Reset(),
// 之后再由 applyLanguage() 把整棵命令树按最终语言重新翻一遍。
func init() { lang = "zh" }

// resolve 按 config.toml 的语言设置重新定语言; Reset 在进程启动和改 cli.language 后调用。
// 优先级: config.toml 的 [cli] language > $LANG > 中文。
func resolve() {
	switch l := app.ConfigLanguage(); l {
	case "zh", "en":
		lang = l
	default:
		l := os.Getenv("LANG")
		switch {
		case strings.Contains(l, "zh"):
			lang = "zh"
		case l != "":
			lang = "en"
		}
	}
}

// Reset 重新判定语言 (启动时 / config set cli.language 之后)
func Reset() { resolve() }

// Lang 当前语言 ("zh"/"en")
func Lang() string { return lang }

// Set 直接指定语言 (测试用)
func Set(l string) {
	switch strings.ToLower(l) {
	case "zh", "en":
		lang = strings.ToLower(l)
	}
}

// TranslateComposite 把"由多个翻译 key 拼成的句子"整体翻过来。
// cobra 命令的 Long 是 T(a)+"\n"+T(b) 拼的, 整串不是表里的 key, 只能按片段替换:
// 包初始化时语言还是中文, 各 T() 原样返回 key, 所以拼出来的串就是"key + 字面量 + key",
// 按 key 长度降序替换即可无损还原英文。
func TranslateComposite(s string) string {
	if lang != "en" || s == "" {
		return s
	}
	for _, p := range compositePairs() {
		if strings.Contains(s, p.key) {
			s = strings.ReplaceAll(s, p.key, p.val)
		}
	}
	return s
}

type kv struct{ key, val string }

var compositeCache []kv

// compositePairs en 表的 key→译文, 按 key 长度降序 (长的先替换, 防止短键截断长键)
func compositePairs() []kv {
	if compositeCache != nil {
		return compositeCache
	}
	out := make([]kv, 0, len(en))
	for k, v := range en {
		if k == v || k == "" {
			continue
		}
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].key) > len(out[j].key) })
	compositeCache = out
	return out
}

// Keys en 表里收录的全部 key (供 orphan 检查: 有些 key 来自 cfg 注册表而不是 T() 字面量)
func Keys() []string {
	out := make([]string, 0, len(en))
	for k := range en {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// T 翻译: 英文模式下查表, 未收录/中文模式返回原文
func T(s string) string {
	if lang == "en" {
		if e, ok := en[s]; ok {
			return e
		}
	}
	return s
}

var en = map[string]string{
	// ---- cobra 帮助骨架 / 段标题 (键带冒号, 避免和 "示例"→example 这类短键撞车) ----
	"core config":    "core config",
	"cli":            "cli",
	"dns":            "dns",
	"tun":            "tun",
	"systemd timers": "systemd timers",
	"用法:":            "Usage:",
	"别名:":            "Aliases:",
	"示例:":            "Examples:",
	"可用命令:":          "Available Commands:",
	"其它命令:":          "Additional Commands:",
	"标志:":            "Flags:",
	"全局标志:":          "Global Flags:",
	"其它帮助主题:":        "Additional help topics:",
	"查看某命令的详细帮助: {{.CommandPath}} [command] --help": "Use \"{{.CommandPath}} [command] --help\" for more information about a command.",
	"显示帮助":                   "show help",
	"mihomo-cli <命令> --help": "mihomo-cli <command> --help",
	"mihomo-cli install --core auto --resource all --systemd --completion bash": "mihomo-cli install --core auto --resource all --systemd --completion bash",
	"mihomo-cli sub add <name> <订阅URL>":                                         "mihomo-cli sub add <name> <subscription URL>",
	"mihomo-cli start":                                                          "mihomo-cli start",
	"eval $(mihomo-cli proxy on)":                                               "eval $(mihomo-cli proxy on)",
	"mihomo-cli config get":                                                     "mihomo-cli config get",

	"请先执行: mihomo-cli config set tun.route-exclude-address ": "run first: mihomo-cli config set tun.route-exclude-address ",

	// ---- 详版帮助: Key.Detail / Key.Values (注册表字段, 不是 T() 字面量) ----
	"关时只监听 127.0.0.1 (只有本机能用); 开时监听 0.0.0.0, 局域网设备可用。\n注意: 开放后同网段任何人都能通过这台机器上网, 请在受信任网络里使用。":          "off = listen on 127.0.0.1 only (this machine alone); on = listen on 0.0.0.0 so LAN devices can use it.\nNote: once open, anyone on the same subnet can browse through this box; use it on trusted networks.",
	"一个端口同时提供 HTTP 和 SOCKS5 代理, 推荐只开这一个。\n改成 off 后本机与局域网都无法通过端口用代理 (TUN 模式除外)。":                       "one port serving both HTTP and SOCKS5; this is the one to keep.\nSetting it to off disables port proxying entirely (TUN still works).",
	"只有当某个客户端必须用独立 SOCKS5 端口时才需要开; mixed-port 已经能同时提供 socks5。\n三个端口 (mixed/socks/http) 不能填成同一个号。":     "enable only when a client demands a dedicated SOCKS5 port; mixed-port already serves socks5.\nThe three ports (mixed/socks/http) must not be the same number.",
	"只有当某个客户端必须用独立 HTTP 端口时才需要开; mixed-port 已经能同时提供 http。\n三个端口 (mixed/socks/http) 不能填成同一个号。":         "enable only when a client demands a dedicated HTTP port; mixed-port already serves http.\nThe three ports (mixed/socks/http) must not be the same number.",
	"开时内核会处理 AAAA 记录并通过代理访问 IPv6 目标。\n服务器没有 IPv6 出口时建议关闭, 否则可能反而连不上。":                                 "on = the core resolves AAAA records and reaches IPv6 targets through the proxy.\nTurn it off when the server has no IPv6 egress, otherwise connections may fail.",
	"开时同一目的地的多个请求会复用同一条 TCP 连接, 明显降低握手开销。\n极少数对连接复用敏感的服务可能异常, 那时关掉试试。":                                "on = requests to the same destination share one TCP connection, cutting handshake cost.\nA few services are sensitive to connection reuse; turn it off if they misbehave.",
	"开时延迟测试会把「建连+首字节」合并计算, URL-Test 择优更准。\n只影响测速显示和自动择优, 不影响实际转发。":                                    "on = latency tests count connect+first-byte together, so URL-Test picks better nodes.\nAffects latency display and auto-select only, not actual forwarding.",
	"空闲连接多久发一次保活包。太小浪费流量, 太大可能被中间设备掐断。\n30 秒是常见选择; 移动网络下可改 15。":                                       "how often idle connections send a keep-alive. Too small wastes traffic, too large gets killed by middleboxes.\n30s is a common choice; use 15 on mobile networks.",
	"下载内核/geo 资源/CLI 自更新时, GitHub 直连失败会自动改走镜像站。\nauto = 依次尝试内置的 3 个镜像; 填了具体地址就固定用它。":                  "when downloading core/geo resources or self-updating, a failed direct GitHub hit falls back to a mirror.\nauto = try the 3 built-in mirrors in order; a concrete URL pins one.",
	"节点测速 (node test / node auto) 和 doctor 都请求这个 URL, 用返回 204 的时间当延迟。\n换一个离服务器近、且被代理允许的端点, 测速结果会更真实。": "node test / node auto and doctor all request this URL and use the 204 round-trip as latency.\nPick an endpoint close to the server and allowed by the proxy for honest numbers.",
	"决定 DNS 对代理域名返回什么地址。fake-ip 兼容性最好; redir-host 返回真实 IP, 便于排查。":                                     "what DNS answers for proxied domains. fake-ip is the most compatible; redir-host returns real IPs, easier to debug.",
	"返回虚拟 IP (推荐: 兼容最好, 支持按域名分流)":                                                "return a virtual IP (recommended: best compatibility, supports per-domain routing)",
	"返回真实 IP (便于抓包排查, 部分客户端会绕过分流)":                                               "return the real IP (easy to packet-capture; some clients bypass routing)",
	"只转发不返回映射 (兼容性最差)":                                                           "forward only, no mapping (worst compatibility)",
	"DNS 服务器列表。可以填预设名 (TAB 可补全)、订阅编号 subN、裸 IP 或 DoH/DoT URL;\nsub = 跟随订阅, 不注入。": "DNS server list. Accepts a preset name (TAB completes it), a subscription index subN, bare IPs or DoH/DoT URLs;\nsub = follow the subscription, inject nothing.",
	"用第 N 个订阅自带的 dns.nameserver":                                                 "use dns.nameserver of the N-th subscription",
	"一个或多个 IP, 逗号分隔":                                                             "one or more IPs, comma separated",
	"DoH/DoT 地址, 如 https://dns.alidns.com/dns-query":                             "DoH/DoT URL, e.g. https://dns.alidns.com/dns-query",
	"跟随订阅, CLI 不注入":                                                              "follow the subscription; the CLI injects nothing",
	"TUN 网卡用哪种协议栈转发。mips 是内核自研的用户态栈 (默认); system 性能最好但依赖内核特性; gvisor 兼容性最好。": "which protocol stack forwards TUN traffic. mips is the core's own userspace stack (default); system is fastest but kernel-dependent; gvisor is the most compatible.",
	"内核协议栈 (性能最好, 需要较新内核; 开了防火墙的平台可能不可用)":                                    "kernel stack (fastest, needs a recent kernel; unavailable on platforms with a firewall)",
	"用户态协议栈 (兼容性最好, 性能一般)":                                                   "userspace stack (most compatible, average performance)",
	"TCP 走 system、其余走 gvisor (两者混合)":                                         "TCP via system, everything else via gvisor (hybrid)",
	"内核自研用户态协议栈 (mipstack, 默认)":                                              "the core's own userspace stack (mipstack, default)",
	"开启": "on",
	"关闭": "off",
	"跟随订阅: 不注入, 用订阅/内核的值":              "follow the subscription: inject nothing, use the subscription/kernel value",
	"关闭该端口, 不监听 (输入 0/none/- 也可以)":     "disable this port, do not listen (0/none/- are accepted too)",
	"跟随订阅: 订阅写了才监听":                    "follow the subscription: listen only if the subscription sets it",
	"<端口号>":                            "<port>",
	"监听指定端口 (1-65535)":                 "listen on the given port (1-65535)",
	"自动: 依次尝试内置镜像列表":                   "auto: try the built-in mirror list in order",
	"<url>":                            "<url>",
	"固定使用该镜像站前缀, 如 https://ghfast.top": "always use this mirror prefix, e.g. https://ghfast.top",
	"<预设名>":                            "<preset>",
	// ---- root / help ----
	"mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)": "Pure-CLI manager for the mihomo proxy core (Linux server proxy tool)",
	"面向 Linux 服务器的 Clash/mihomo 代理管理工具":    "Clash/mihomo proxy manager for Linux servers",
	"启动代理服务":                   "start proxy service",
	"停止代理服务":                   "stop proxy service",
	"重启代理服务":                   "restart proxy service",
	"查看服务与代理状态":                "show service and proxy status",
	"前台运行内核(调试模式, Ctrl-C 退出)":  "run core in foreground (debug, Ctrl-C to quit)",
	"开/关当前 shell 代理(配合 alias)": "toggle proxy env for current shell (use with alias)",
	"查看内核日志 (-f 跟随)":           "show core logs (-f follow)",
	"已安装内核版本":                  "installed core version",
	"mihomo-cli 版本":            "mihomo-cli version",
	"查看代理分组列表 (索引别名 #1..#n)":   "list proxy groups (index aliases #1..#n)",
	"设置当前操作分组":                 "set the current working group",
	"列出当前分组的节点 (索引别名 #1..#n)":  "list nodes of current group (index aliases)",
	"切换当前分组到指定节点":              "switch current group to a node",
	"测试当前分组节点延迟 (彩色)":          "test node delays of current group (colored)",

	// ---- install / uninstall ----
	"已保留数据目录": "data directory kept",
	"删除":      "removing",
	"补全已安装":   "completion installed:",
	"已下载":     "downloaded",

	// ---- init ----

	// ---- service ----
	"服务已启动":                    "service started",
	"服务已停止":                    "service stopped",
	"服务已重启":                    "service restarted",
	"运行中":                      "running",
	"未运行":                      "not running",
	"仅本机 (127.0.0.1)":          "localhost only (127.0.0.1)",
	"未运行 (mihomo-cli start)":   "not running (mihomo-cli start)",
	"没有订阅, 请先 mihomo-cli init": "no subscription, run mihomo-cli init first",

	// ---- sub ----
	"下载订阅":                         "downloading subscription",
	"订阅":                           "profile",
	"已添加并生效":                       "added and activated",
	"已删除":                          "removed",
	"更新订阅":                         "updating profile",
	"节点数":                          "nodes",
	"已热重载配置":                       "config reloaded",
	"当前订阅已切换为":                     "current profile switched to",
	"已存在":                          "already exists",
	"不存在 (mihomo-cli sub list 查看)": "not found (see mihomo-cli sub list)",
	"没有可用订阅":                       "no subscription available",

	// ---- group / node ----
	"分组":          "GROUP",
	"类型":          "TYPE",
	"当前节点":        "NOW",
	"节点":          "nodes",
	"当前操作分组已切换为":  "current working group set to",
	"不在该分组中":      "not in this group",
	"匹配到多个, 请更精确": "matches multiple, be more specific",
	"超时":          "timeout",
	"测试分组":        "testing group",
	"全部节点不可用":     "all nodes unavailable",
	"测速失败":        "delay test failed",
	"切换失败":        "switch failed",
	"跳过":          "skip",
	"无真实节点, 策略组":  "no real nodes (policy group)",
	"未设置当前分组, 请先 mihomo-cli group use <id|名称>": "no current group, run mihomo-cli group use <id|name>",
	"找不到分组": "group not found:",

	// ---- doctor ----
	"已启用":        "enabled",
	"已停用":        "disabled",
	"监听中":        "LISTEN",
	"未监听":        "-",
	"API 正常, 内核": "API ok, core",
	"提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli proxy on))": "Tip: verify with curl -I https://www.google.com (after eval $(mihomo-cli proxy on))",

	// ---- conn / traffic / log ----
	"活动连接": "Active conns",
	"累计":   "total",
	"网络":   "NET",
	"目标":   "HOST",
	"代理链":  "CHAIN",
	"暂无日志": "no logs yet:",

	// ---- set / get ----
	"已保存":                      "(saved)",
	"未知配置项":                    "unknown setting key:",
	"警告: 更新定时器失败":              "warn: timer update failed:",
	"警告: 热重载失败":                "warn: hot reload failed:",
	"(可执行 mihomo-cli restart)": "(try mihomo-cli restart)",
	"警告: 定时任务安装失败":             "warn: timer install failed:",

	// ---- core ----

	// ---- update / unuse / chain ----
	"更新 mihomo-cli 自身 (从 GitHub Releases)": "update mihomo-cli itself (from GitHub Releases)",
	"已是最新版本":                               "already the latest version",
	"升级":                                   "upgrading",
	"已安装":                                  "installed",
	"安装包中未找到二进制":                           "binary not found in package",
	"访问失败":                                 "failed to reach",
	"当前无生效订阅(sub 悬空), 代理未生效": "no active profile (sub unused), proxy inactive",
	"当前链路": "Chain",
	"提示: 分组未选择, 可执行 mihomo-cli group use <id|名称>": "hint: no group selected, run mihomo-cli group use <id|name>",
	"没有可用订阅, 请先 mihomo-cli sub use <id|名称>":       "no subscription, run mihomo-cli sub use <id|name> first",
	"当前分组已是悬空状态":                                  "current group is already unused",
	"已取消, 该分组流量走 DIRECT 直连":                       "unused; traffic of this group goes DIRECT",
	"取消当前分组选择: 该分组流量走 DIRECT 直连":                  "unset current group: its traffic goes DIRECT",
	"取消当前节点选择: 分组流量走 DIRECT 直连":                   "unset current node: group traffic goes DIRECT",
	"已取消, 分组流量走 DIRECT 直连":                        "unused; group traffic goes DIRECT",
	"取消失败":           "unuse failed",
	"当前订阅已是悬空状态":     "current profile is already unused",
	"悬空 (sub unuse)": "unused (sub unuse)",
	"上次":             "last",
	"下次":             "next",
	"无当前分组, 跳过自动择优 (mihomo-cli group use <id|名称>)": "no current group, auto-select skipped (mihomo-cli group use <id|name>)",
	"对当前分组测速并切换到延迟最低的节点":                           "test and switch to the lowest-latency node of the current group",

	// ---- proxy on/off ----
	"服务未运行, 已自动启动": "service was not running, started automatically",

	// ---- 内部包 ----
	"GitHub API 返回":             "GitHub API returned",
	"可能被限流, 可稍后重试":              "rate-limited, retry later",
	"读取订阅文件失败":                  "failed to read profile file",
	"订阅配置解析失败":                  "failed to parse subscription yaml",
	"订阅服务器返回":                   "subscription server returned",
	"订阅内容不是 clash yaml 格式":      "subscription is not clash yaml",
	"无法连接 mihomo API(服务是否已启动?)": "cannot reach mihomo API (is the service running?)",
	"写入 systemd 单元失败(需要 root)":  "failed to write systemd unit (root required)",
	"下载失败":                      "download failed",

	// ---- v0.4 补全 ----
	"错误":            "error",
	"当前 shell 开启代理": "enable proxy for current shell",
	"订阅管理: add/rm/list/update/use/unuse": "subscription: add/rm/list/update/use/unuse",
	"订阅自动更新周期":                           "subscription auto-update interval",
	"跟随日志":                               "follow logs",
	"更新订阅 (默认当前; all = 全部), 完成后热重载":      "update subscriptions (default: current; all), then hot-reload",
	"更新时间":                               "updated",
	"关闭当前 shell 的代理环境变量 (服务保持运行)":        "remove proxy env of current shell (service keeps running)",
	"开启代理: 启动服务并输出代理环境变量 (eval $(mihomo-cli proxy on))": "enable proxy: start service and print env (eval $(mihomo-cli proxy on))",
	"列出全部订阅":    "list all subscriptions",
	"名称":        "NAME",
	"切换当前生效的订阅": "switch the active subscription",
	"删除订阅":      "remove a subscription",
	"添加订阅":      "add a subscription",
	"延迟":        "DELAY",
	"重命名订阅":     "rename a subscription",
	"注意: 自动择优已开启, 下次定时任务可能覆盖此设置": "note: auto-select is enabled; the next timer run may override this",
	"自动择优周期":        "auto-select interval",
	"[%s] %s -> %s": "[%s] %s -> %s",
	"安装需要 root 权限: 配置目录 /etc/mihomo-cli 与 systemd 单元": "install requires root: /etc/mihomo-cli and systemd units",
	"DIRECT (空配置)": "DIRECT (empty config)",
	"取消当前订阅: 内核空配置运行(全部 DIRECT), 服务保持运行":    "unset current sub: core runs with empty config (all DIRECT), service keeps running",
	"已悬空: 内核以空配置运行, 全部流量 DIRECT (服务保持运行)":   "unused: core runs with empty config, all traffic DIRECT (service keeps running)",
	"提示: 可 mihomo-cli sub use <id|名称> 重新启用": "hint: re-enable with mihomo-cli sub use <id|name>",
	"服务未运行, 已跳过热重载 (mihomo-cli start)":      "service not running, hot-reload skipped (mihomo-cli start)",
	// ---- v0.7 set/dns/conn ----
	"说明":                 "DESCRIPTION",
	"可选值":                "ALLOWED VALUES",
	"当前值":                "CURRENT",
	"用法":                 "USAGE",
	"值":                  "VALUE",
	"中文":                 "Chinese",
	"跟随订阅":               "follow subscription",
	"阿里 DNS":             "AliDNS",
	"114 DNS":            "114 DNS",
	"谷歌 DNS":             "Google DNS",
	"Cloudflare DNS":     "Cloudflare DNS",
	"AdGuard DNS (拦截广告)": "AdGuard DNS (ad-blocking)",
	"Quad9 DNS (安全拦截)":   "Quad9 DNS (security)",
	"腾讯 DNSPod":          "Tencent DNSPod",
	"无效 IP":              "invalid IP",
	"或未知预设":              "or unknown preset",
	"关闭指定编号的活动连接":        "close connections by id",
	"无效编号":               "invalid id",
	"连接不存在或已关闭":          "connection not found or closed",
	"已关闭":                "closed",
	"没有连接被关闭":            "no connection closed",
	"下载使用的代理 (空=按环境变量/直连)": "proxy for downloading (empty = env/direct)",
	// ---- v0.8 ----
	"单次输出: 总流量/速度/连接数/连接列表":  "one-shot: totals/speeds/conn count/connection list",
	"定时对当前分组自动择优":            "auto-select best node of current group periodically",
	"订阅定时自动更新":               "subscription auto-update",
	"该订阅无 dns.nameserver 配置": "this profile has no dns.nameserver",
	"关闭指定编号的连接":              "close connections by id",
	"进程":                     "PROCESS",
	"流量与连接总览":                "traffic & connections overview",
	"流量与连接总览 (top watch 持续刷新, top kill <编号> 关连接)": "overview (top watch refreshes; top kill closes conns)",
	"每 N 秒刷新 (默认 1s), Ctrl-C 退出":                  "refresh every N seconds (default 1s), Ctrl-C to quit",
	"时长":     "AGE",
	"速度":     "speed",
	"无效刷新间隔": "invalid refresh interval",
	"仅在当前 use 的分组内, 于真实节点(排除子分组/DIRECT/REJECT)中选择延迟最低者切换;":  "pick the lowest-latency real node (excluding sub-groups/DIRECT/REJECT) within the current group;",
	"定时任务 (node-auto-select-enabled) 周期性执行本命令; 未设置当前分组时跳过。": "the timer (node-auto-select-enabled) runs this periodically; skipped when no current group is set.",
	"如": "e.g.",
	// ---- v0.9 rule/geo/builtins ----
	"地区": "REGION",
	"用户自定义规则 (独立于订阅, 优先匹配; list --sub 同时显示订阅规则)":  "user rules (independent of subscriptions, matched first; list --sub shows sub rules too)",
	"用户规则独立于订阅保存, 不会因订阅更新/删除/更换而丢失; 匹配优先级高于订阅规则。": "user rules are stored independently and survive subscription updates/removals; they take precedence over subscription rules.",
	"显示用户规则 (--sub 同时显示订阅规则)":                     "show user rules (--sub also shows subscription rules)",
	"显示用户规则":   "show user rules",
	"同时显示订阅规则": "also show subscription rules",
	"添加规则":     "add a rule",
	"删除规则":     "remove a rule",
	"用户规则":     "User rules",
	"订阅规则":     "Subscription rules",
	"提示: rule list --sub 查看订阅规则": "hint: rule list --sub shows subscription rules",
	"规则不存在": "rule not found",
	"注意: 当前代理模式不是 rule, 规则暂不生效 (set proxy-mode rule)": "note: current proxy mode is not rule; rules are inactive (set proxy-mode rule)",
	"任意命令的帮助信息":                                       "help about any command",
	"显示任意命令的帮助信息; 用法: mihomo-cli help [command]":      "show help for any command; usage: mihomo-cli help [command]",
	"生成指定 shell 的自动补全脚本":                              "generate the autocompletion script for the specified shell",
	"为指定的 shell 生成自动补全脚本 (bash/zsh/fish/powershell)。": "generate an autocompletion script for the specified shell (bash/zsh/fish/powershell).",
	"生成":            "generate",
	"补全脚本":          "completion script",
	"下载订阅失败":        "failed to download subscription",
	"可能需要 UA 或链接失效": "check UA or link validity",
	"不存在":           "not found",

	// ---- v1.0 ----
	"PROXY 需要先 group use 选定分组": "PROXY requires a current group (group use first)",
	"丢弃请求":                     "drop request",
	"仅显示带 no-resolve 的规则":      "only rules with no-resolve",
	"代理策略":                     "STRATEGY",
	"内核校验失败, 已回滚":              "core validation failed, rolled back",
	"分组名":                      "<group>",
	"匹配 DSCP 标记":               "DSCP mark",
	"匹配 GeoSite 内的域名":          "domains in GeoSite",
	"匹配 IP 后缀范围":               "IP suffix range",
	"匹配 IP 地址范围":               "IP range",
	"匹配 IP 所属 ASN":             "IP ASN",
	"匹配 IP 所属国家代码":             "IP country code",
	"匹配 IPv6 地址范围":             "IPv6 range",
	"匹配 Linux UserID":          "Linux UID",
	"匹配 TCP/UDP":               "TCP/UDP",
	"匹配入站名称":                   "inbound name",
	"匹配入站用户":                   "inbound user",
	"匹配入站端口":                   "inbound port",
	"匹配入站类型":                   "inbound type",
	"匹配域名关键字":                  "domain keyword",
	"匹配域名后缀":                   "domain suffix",
	"匹配域名正则表达式":                "domain regex",
	"匹配完整域名":                   "exact domain",
	"匹配完整进程路径":                 "full process path",
	"匹配所有请求":                   "all requests",
	"匹配条件":                     "CONDITION",
	"匹配来源 IP 后缀范围":             "source IP suffix range",
	"匹配来源 IP 地址范围":             "source IP range",
	"匹配来源 IP 所属 ASN":           "source IP ASN",
	"匹配来源 IP 所属国家代码":           "source IP country code",
	"匹配请求来源端口范围":               "source port",
	"匹配请求目标端口范围":               "destination port",
	"匹配进程名称":                   "process name",
	"受限":                       "restricted",
	"可用":                       "available",
	"启用用户规则":                   "enable a user rule",
	"已热切换":                     "hot-switched",
	"当前 group use 的分组":         "the current group",
	"恢复默认值 (仅对有默认值的配置项生效)": "restore defaults (only keys with defaults)",
	"拦截请求":                     "reject request",
	"按代理策略过滤":                  "filter by strategy",
	"按匹配条件过滤":                  "filter by condition",
	"按规则类型过滤":                  "filter by rule type",
	"无效 ASN 编号":                "invalid ASN number",
	"无效 CIDR":                  "invalid CIDR",
	"无效 IP 后缀 (格式 8.8.8.8/24)": "invalid IP suffix (e.g. 8.8.8.8/24)",
	"无效国家代码 (如 CN)":            "invalid country code (e.g. CN)",
	"无效数字":                     "invalid number",
	"未指定策略 (--strategy)":       "strategy not specified (--strategy)",
	"未知站点":                     "unknown site",
	"未知策略":                     "unknown strategy",
	"未知规则类型":                   "unknown rule type",
	"模糊匹配代理分组, 如":              "fuzzy-match a group, e.g.",
	"正则匹配完整进程路径":               "full process path regex",
	"正则匹配进程名称":                 "process name regex",
	"注意: no-resolve 一般仅用于 IP 类规则, 已忽略": "note: no-resolve applies to IP rules only; ignored",
	"添加规则 (结构化参数)":                     "add a rule (structured flags)",
	"状态":                               "STATUS",
	"的流量":                              " traffic",
	"直接连接":                             "direct connection",
	"示例":                               "example",
	"禁用用户规则":                           "disable a user rule",
	"站点":                               "SITE",
	"站点延迟与可用性检测 (经当前代理; 解锁判定为启发式)": "site latency & availability via current proxy (heuristic unlock check)",
	"策略": "strategy",
	"策略: DIRECT/REJECT/REJECT-DROP/PASS/PROXY(当前分组)/分组名; 语法由内核校验, 失败回滚。": "strategy: DIRECT/REJECT/REJECT-DROP/PASS/PROXY(current group)/group name; syntax is validated by the core, invalid rules are rolled back.",
	"经本机代理端口访问站点, 输出 HTTP 状态与延迟; 2xx/3xx 视为可用, 403/451 视为地区受限。":          "visit sites via the local proxy; 2xx/3xx = available, 403/451 = region restricted.",
	"缺少匹配条件": "missing condition",
	"规则未生效":  "rule not applied",
	"规则由内核执行; CLI 负责拼接写入并经内核 reload 校验, 不合法会被拒绝并回滚。": "rules are executed by the core; the CLI formats, writes and validates them via core reload; invalid ones are rejected and rolled back.",
	"规则类型": "rule type",
	"解锁判定为启发式 (仅看 HTTP 状态码), 仅供参考。": "unlock detection is heuristic (HTTP status only), for reference.",
	"该规则选中了所有":                                                                 "this rule selects all",
	"语法将由内核校验, 非法规则会被拒绝并回滚":                                                    "syntax will be validated by the core; invalid rules are rejected and rolled back",
	"已设置但未启用的段":                                                                "configured but not enabled sections",
	"查看: mihomo-cli config get <段>; 启用: mihomo-cli config set <段>.enable true": "view: mihomo-cli config get <section>; enable: mihomo-cli config set <section>.enable true",
	"段未启用, 此设置暂不生效":                                                            "section not enabled, this setting has no effect yet",
	"启用":                                                                       "enable",
	"说明: 可用=HTTP 2xx/3xx (Docker Hub 的 401 是匿名鉴权挑战, 也算通), 受限=403/451(地区限制), 判定为启发式仅供参考": "note: available = HTTP 2xx/3xx (Docker Hub's 401 is the anonymous auth challenge, also counts as reachable), restricted = 403/451 (geo-block); heuristic only",
	"走代理组":              "via group",
	"跳过域名解析 (仅 IP 类规则)": "skip DNS resolution (IP rules only)",
	"跳过此项":              "skip this rule",
	"过滤":                "filter",
	"逻辑与":               "AND (logical)",
	"逻辑或":               "OR (logical)",
	"逻辑非":               "NOT (logical)",
	"配置文件 /etc/mihomo-cli/config.toml 由 mihomo-cli 管理与运行时回写, 请勿手动编辑;": "/etc/mihomo-cli/config.toml is managed and rewritten by mihomo-cli; do not edit manually;",
	"预览":  "preview",
	"默认值": "DEFAULT",

	// ---- v1.0.1 ----
	"哔哩哔哩大陆":  "Bilibili CN",
	"哔哩哔哩港澳台": "Bilibili HK/MO/TW",

	// ---- v1.1 ----
	"下载/更新资源": "download/update a resource",
	"内核二进制":   "core binary",
	"大小":      "SIZE",
	"数据资源":    "data resource",
	"更新全部数据资源 (mmdb/asn/geoip/geosite)": "update all data resources (mmdb/asn/geoip/geosite)",
	"更新全部数据资源 (定时任务复用)":                 "update all data resources (timer entry)",
	"更新资源":     "updating resource",
	"未安装":      "not installed",
	"未知 shell": "unknown shell",
	"查看资源状态":   "show resource status",
	"管理内核与 geo 数据资源; 数据资源支持自动更新 (resource-auto-update-*)。": "manage core & geo resources; data resources support auto-update (resource-auto-update-*).",
	"资源管理: core/mmdb/asn/geoip/geosite":                    "resources: core/mmdb/asn/geoip/geosite",

	// ---- v1.2 ----
	"GitHub 镜像站前缀 (空=失败时自动尝试内置镜像)": "GitHub mirror prefix (empty = auto-fallback)",
	"Node auto-select": "Node auto-select",
	"Res auto-update":  "Res auto-update",
	"Sub auto-update":  "Sub auto-update",
	"一次性下载代理/镜像 (默认失败时自动尝试内置镜像)":                                        "one-shot proxy/mirror (auto-fallback to builtin mirrors)",
	"停止并移除 systemd 单元":                                                  "停止并移除 systemd 单元",
	"全新安装":                                                              "full fresh install",
	"前台启动内核, 日志输出到终端":                                                   "前台启动内核, 日志输出到终端",
	"安装/更新单个资源 (mmdb/asn/geoip/geosite/all)":                            "install/update a single resource",
	"安装: --core / --resource / --systemd / --completion (可组合; 无参数显示帮助)": "install: --core/--resource/--systemd/--completion (combinable; no flags shows help)",
	"安装动作幂等: 文件不存在则下载创建, 存在则更新刷新。订阅请用 sub add, 参数请用 config set。": "install is idempotent: downloads if missing, refreshes if present. Use sub add/config set for other concerns.",
	"已添加":          "added",
	"当前订阅不变, 如需切换": "current profile unchanged; to switch",
	"按 config.toml 重新生成全部 systemd 单元并 daemon-reload": "regenerate all systemd units from config.toml and daemon-reload",
	"无订阅时为最小配置, 全部流量 DIRECT":                         "minimal config without subscription; all traffic DIRECT",
	"服务已自动拉起":       "service started automatically",
	"未设置":           "unset",
	"注册 systemd 服务": "注册 systemd 服务",
	"结束时若内核与 systemd 就绪而服务未运行, 将以最小配置自动拉起服务。": "at the end, if core+systemd are ready but the service is down, it is started with a minimal config.",
	"重新生成 systemd 单元": "regenerate systemd units",

	// ---- v1.3.0 (config 注册表 / install-uninstall 对称 / tun / dns / 版本栈) ----
	"Chain":                   "Chain",
	"--completion 需要指定 shell": "--completion requires a shell (bash|zsh|fish)",
	"Config file":             "Config file",
	"Config values":           "Config values",
	"Core":                    "Core",
	"Core traffic":            "Core traffic",
	"Core version":            "Core version",
	"--core 的规格是「风味:版本」两个正交轴, 平台(系统/架构)自动探测:": "--core spec = <flavor>[:<version>], two orthogonal axes; the platform (OS/arch) is auto-detected:",
	"Ctrl API":            "Ctrl API",
	"Data dir":            "Data dir",
	"DNS":                 "DNS",
	"DNS override":        "DNS override",
	"DNS 覆写: on/off/list": "DNS override: on/off/list",
	"GitHub 镜像站前缀 (空=按 config.toml/内置列表)": "GitHub mirror prefix (empty = per config.toml / builtin list)",
	"LAN":             "LAN",
	"Mode":            "Mode",
	"Profile":         "Profile",
	"Proxy port":      "Proxy port",
	"(--purge 可彻底删除)": "(--purge removes it as well)",
	"Resource":        "Resource",
	"Runtime config":  "Runtime config",
	"Service":         "Service",
	"Traffic":         "Traffic",
	"TUN":             "TUN",
	"TUN 会让默认流量进入虚拟网卡, 配置错误可能把自己踢出服务器;": "TUN routes default traffic into a virtual interface; a wrong config can lock you out of your own server.",
	"TUN 透明代理: on/off/list": "TUN transparent proxy: on/off/list",
	"三层结构: 订阅 sub → 分组 group → 节点 node; 裸命令等于各自的 list。": "Three layers: subscription (sub) → group → node; a bare command lists its layer.",
	"下载内核/资源使用的代理":                          "Proxy used to download the core/resources",
	"不再使用的配置目录内容可先看 mihomo-cli config get。": "Review data dir contents with mihomo-cli config get before wiping them.",
	"不带值=全部":     "no value = all of them",
	"与运行态完全一致":   "fully consistent with the running state",
	"任意 yaml 路径": "any yaml path",
	"位置":         "Location",
	"体检: 内核/服务/端口/资源/配置一致性":                           "Health check: core/service/ports/resources/config consistency",
	"修改配置并生效 (有校验, 非法值拒绝写入)":                          "Change a setting and apply it (validated; bad values are rejected)",
	"修改配置并立即生效; 非法值拒绝写入。裸命令执行显示本帮助, <key> -h 显示单键详情。": "Changes take effect immediately; bad values are rejected. Bare command shows this help, <key> -h shows one key.",
	"值不能为空": "value must not be empty",
	"值按字面推断类型: true/false/数字/逗号列表/字符串": "Values are typed by literal: true/false/number/comma list/string",
	"全部合法":               "all values valid",
	"关闭 DNS 覆写 (恢复跟随订阅)": "Disable DNS override (follow the subscription again)",
	"关闭 TUN 透明代理":        "Disable TUN transparent proxy",
	"内核已删除":              "core removed",
	"内核已是该版本":            "core already on that version",
	"内核未安装":              "core not installed",
	"内核未安装, 请先执行 mihomo-cli install --core auto":                             "core not installed; run mihomo-cli install --core auto",
	"内核未返回该值, 不可回写":                                                          "the core did not return this value; cannot write back",
	"内核规格 <spec> = [风味][:版本], 平台自动探测并被记住:":                                   "Core spec <spec> = [flavor][:version]; the platform is auto-detected and remembered:",
	"内部错误: tun.enable 未注册":                                                   "internal error: tun.enable is not registered",
	"切换内核版本 (规格同 install --core, 可新可旧)":                                      "Switch core version (same spec as install --core; newer or older)",
	"卸载: --core / --resource / --systemd / --completion / --purge (无参数显示帮助)": "Uninstall: --core / --resource / --systemd / --completion / --purge (no args = help)",
	"卸载与 install 严格对称, 只删你指定的那一部分; --purge 才是彻底卸干净。":                         "Mirrors install exactly: remove only what you ask for; --purge is the full wipe.",
	"卸载需要 root 权限":                                                           "Uninstall requires root privileges",
	"只关闭并删除全部 systemd service/timer":                                         "Stop and remove all systemd services and timers only",
	"只删除 4 个 geo 资源文件":                                                       "Remove the 4 geo resource files only",
	"只删除 geo 资源文件":                                                           "Remove geo resource files only",
	"只删除指定 shell 补全 (不带值=三个全删)":                                              "Remove the given shell's completions only (no value = all three)",
	"只卸载 systemd service 与 timer":                                            "Remove systemd services and timers only",
	"只卸载内核 (先停止服务)":                                                          "Remove the core only (stops the service first)",
	"只卸载内核 (先停止服务), 配置与订阅保留":                                                 "Remove the core only (stops the service first); config and subscriptions are kept",
	"合法值": "legal values",
	"回滚到上一个装过的版本 (本地版本栈, 不联网)": "Roll back to the previously installed version (local version stack, offline)",
	"回滚可用": "available for rollback",
	"填入":   "value given",
	"安装时间": "Installed at",
	"安装/更新 shell 补全 (bash/zsh/fish)": "Install or update shell completions (bash/zsh/fish)",
	"官方默认":             "official default",
	"定时任务已更新":          "timers updated",
	"已切换内核到":           "switched core to",
	"已删除补全文件":          "completion files removed",
	"已删除资源文件":          "resource files removed",
	"已回滚内核到":           "rolled core back to",
	"已安装内核":            "installed core",
	"已开启":              "enabled",
	"已彻底卸载 mihomo-cli": "mihomo-cli fully removed",
	"已重启服务使配置生效":       "service restarted to apply the config",
	"平台与风味沿用 config.toml 里记住的值 (install 时探测写入), 也可用 <spec> 覆盖。": "The platform and flavor follow the values remembered in config.toml (written when install probed them); <spec> overrides both.",
	"广义的更新: 接受任意版本号, 哪怕比当前旧, 只要规格与当前不同就换。":                      "A general switch: any version is accepted, even older ones, as long as it differs from the current one.",
	"开启 DNS 覆写":   "Enable DNS override",
	"开启 TUN 透明代理": "Enable TUN transparent proxy",
	"开启前会自动做安全检查 (TUN 设备 / CAP_NET_ADMIN / SSH 对端网段排除)。": "Safety checks run first (TUN device / CAP_NET_ADMIN / SSH peer subnet exclusion).",
	"强制指定架构 (默认自动检测)":                                    "Force an architecture (default: auto-detect)",
	"当前": "current",
	"当前 SSH 会话的对端网段未在 tun.route-exclude-address 中, 开启后极可能被自己踢下线": "The SSH peer subnet is not in tun.route-exclude-address; enabling this will very likely lock you out",
	"彻底卸载: 以上全部 + /etc/mihomo-cli + mihomo-cli 二进制自身":            "Full uninstall: everything above + /etc/mihomo-cli + the mihomo-cli binary itself",
	"彻底卸载: 全部 + 配置目录 + 二进制自身":                                    "Full uninstall: everything + the data dir + the binary itself",
	"微架构档位+工具链组合, 版本取最新":                                         "Microarchitecture level + toolchain combination, latest version",
	"恢复默认": "RESET TO DEFAULT",
	"把内核当前运行值/定时器实际状态写回 config.toml, 然后重渲染运行配置。": "Writes the core's live values and the actual timer states back to config.toml, then re-renders the runtime config.",
	"指定版本, 沿用已记住的平台与风味":                          "Pin the version; keep the remembered platform and flavor",
	"指定版本, 风味自动探测":                               "Pin the version; the flavor is auto-detected",
	"指定风味与版本":                                    "Pin both flavor and version",
	"提示: 已自动降级到可在本机运行的内核构建":                      "Note: automatically downgraded to a core build that runs on this machine",
	"新内核把代理链路搞坏时, 网络可能正走在坏掉的内核上, 离线才能可靠回退。":      "When a new core breaks the proxy chain, the network may be riding that very core; only a local rollback is reliable.",
	"无效值, 已拒绝写入":                                 "invalid value, write rejected",
	"无效内核风味":                                     "invalid core flavor",
	"无效版本号":                                      "invalid version number",
	"无法开启 TUN":                                   "cannot enable TUN",
	"无配置项":                                       "no config items",
	"旧 CPU/旧系统兼容构建, 版本取最新":                       "Compatible build for old CPUs/systems, latest version",
	"(旧版安装, 规格未记录)":                              "(installed by an older version; spec not recorded)",
	"服务未运行":                                      "service not running",
	"服务未运行 (mihomo-cli start)":                   "service not running (mihomo-cli start)",
	"服务未运行, 已写入配置文件 (mihomo-cli start)":          "service not running; config file written (mihomo-cli start)",
	"服务未运行, 没有运行态可读取":                            "service not running; nothing to read from the running state",
	"服务未运行, 运行值与状态列不可用 (mihomo-cli start)":       "service not running; RUNNING and STATE columns are unavailable (mihomo-cli start)",
	"期望":      "EXPECTED",
	"未启用":     "disabled",
	"未启用混合端口": "mixed port disabled",
	"未找到 /dev/net/tun (容器或内核未启用 TUN); 请改用 mixed-port 端口模式": "/dev/net/tun not found (container or kernel without TUN); use the mixed-port mode instead",
	"未指定 DNS 服务器": "no DNS server given",
	"未注册的内核配置项(任意 yaml 路径)也可写, 值按字面推断类型:": "Unregistered core keys (any yaml path) can be written too; values are typed by literal:",
	"查看/修改全部配置": "View or change every setting",
	"查看本地内核版本栈": "Show the local core version stack",
	"查看本地版本栈":   "Show the local version stack",
	"查看配置 (无参数 = 全部, 一张表显示 默认值/设置值/运行值)": "Show settings (no args = all; one table with default / saved / running values)",
	"检查项": "CHECK",
	"没有可回滚的旧版本 (版本栈为空)": "no older version to roll back to (version stack is empty)",
	"没有可恢复默认值的配置项":      "no setting has a default to restore",
	"没有可用的内核构建":         "no usable core build",
	"沿用已记住的平台与风味 + 最新版": "Keep the remembered platform and flavor, latest version",
	"点号路径可读写内核任意配置段:":   "Dotted paths read and write any core config section:",
	"热切换":   "hot switch",
	"热重载配置": "reload the config",
	"版本":    "Version",
	"版本栈为空 (至少经历一次升级/切换后才可回滚)": "version stack is empty (rollback needs one upgrade/switch first)",
	"生效方式": "HOW IT APPLIES",
	"用 config.toml 覆盖运行态 (可指定键, 兜底手段)": "Let the config file override the running state (per key, fallback only)",
	"用运行态覆盖 config.toml (可指定键, 兜底手段)":  "Let the running state override config.toml (per key, fallback only)",
	"由其他端口承担": "covered by another port",
	"相当于整套软件从未在这台机器上出现过。":             "As if this software had never been on this machine.",
	"确认无误后可再次执行 mihomo-cli tun on":    "Run mihomo-cli tun on again once you are sure",
	"等价于 config set tun.enable false": "Same as config set tun.enable false",
	"等价于 config set tun.enable true":  "Same as config set tun.enable true",
	"纯本地行为: 从版本栈出栈一个版本并装回, 不需要网络。":    "Purely local: pops one version off the stack and reinstalls it, no network needed.",
	"缺失":           "missing",
	"自动探测风味, 装最新版": "Auto-detect the flavor, install the latest version",
	"警告: 无法为内核申请 CAP_NET_ADMIN": "Warning: could not grant CAP_NET_ADMIN to the core",
	"警告: 更新 systemd 单元失败":       "Warning: failed to update the systemd unit",
	"警告: 热切换失败":                 "Warning: hot switch failed",
	"设置值":                       "SETTING",
	"该构建无法在当前系统运行":              "this build does not run on this system",
	"详情":                        "DETAILS",
	"资源已下载":                     "resources downloaded",
	"运行值":                       "RUNNING",
	"适用于: 手动编辑过 config.toml, 或改完没生效。": "Use after a manual edit of config.toml, or when a change did not take effect.",
	"配置文件与运行态一致, 无需更新":                "config file already matches the running state",
	"配置项":                    "Key",
	"重新安装":                   "reinstall",
	"降级尝试下一个内核构建":            "falling back to the next core build",
	"需要 root 权限":             "root privileges required",
	"需要 root 权限, 且本机没有 sudo": "root privileges required and sudo is not available",
	"需要 root 权限, 请执行":        "root privileges required, run",
	"非法配置路径":                 "illegal config path",
	"项与运行态不同":                "items differ from the running state",
	"项目":                     "ITEM",
	"风味":                     "Flavor",
	"风味与版本都指定":               "Pin both flavor and version",
	"默认":                     "DEFAULT",

	// ---- v1.3.0 key descriptions (internal/cfg registry) ----
	"geo 资源更新周期": "Geo resource update interval",
	"GitHub 镜像站前缀 (下载内核/资源失败时依次尝试); auto=内置列表": "GitHub mirror prefix (tried in order when core/resource downloads fail); auto=builtin list",
	"TCP 并发连接 (多路复用, 提速); sub=跟随订阅":            "TCP concurrent connections (multiplexing, faster); sub=follow the subscription",
	"代理模式; sub=跟随订阅 (热切换)":                     "Proxy mode; sub=follow the subscription (hot switch)",
	"允许局域网设备使用代理 (监听 0.0.0.0)":                 "Allow LAN devices to use the proxy (listen on 0.0.0.0)",
	"内核日志等级; sub=跟随订阅 (热切换)":                   "Core log level; sub=follow the subscription (hot switch)",
	"启用 IPv6 转发":                                   "Enable IPv6 forwarding",
	"定时自动更新 geo 资源文件":                              "Periodically update geo resource files",
	"当前操作分组 (只读; 用 group use 切换)":                  "Current working group (read-only; switch with group use)",
	"当前生效订阅 (只读; 用 sub use 切换)":                    "Active subscription (read-only; switch with sub use)",
	"测速超时 (毫秒)":                                    "Latency test timeout (ms)",
	"测速/连通性检测 URL (204 端点)":                        "Latency/connectivity test URL (204 endpoint)",
	"混合代理端口 (http + socks5); off=关闭":               "Mixed proxy port (http + socks5); off=disabled",
	"独立 HTTP(S) 代理端口; off=关闭 (mixed-port 已含 http)": "Dedicated HTTP(S) proxy port; off=disabled (mixed-port already covers http)",
	"独立 SOCKS5 端口; off=关闭 (mixed-port 已含 socks5)":  "Dedicated SOCKS5 port; off=disabled (mixed-port already covers socks5)",
	"统一延迟计算 (URL-Test 更精准); sub=跟随订阅":              "Unified delay (more accurate URL-Test); sub=follow the subscription",
	"输出语言; auto=按系统 locale":                        "Output language; auto=follow the system locale",
	"长连接保活间隔 (秒); sub=跟随订阅":                        "Keep-alive interval for long connections (seconds); sub=follow the subscription",
	"DNS 劫持规则":                                     "DNS hijack rules",
	"DNS 增强模式":                                     "DNS enhanced mode",
	"DNS 引导解析器 (必须是纯 IP)":                          "DNS bootstrap resolver (must be pure IPs)",
	"DNS 服务器列表 (IP 或 DoH/DoT URL)":                 "DNS server list (IP or DoH/DoT URL)",
	"DNS 监听地址":                                     "DNS listen address",
	"DNS 解析 IPv6 结果 (AAAA)":                        "Resolve IPv6 (AAAA) records",
	"Fake-IP 地址段":                                  "Fake-IP range",
	"TUN MTU":                                      "TUN MTU",
	"TUN 网卡名":                                      "TUN device name",
	"TUN 网络栈":                                      "TUN network stack",
	"UDP 会话超时 (秒)":                                 "UDP session timeout (seconds)",
	"iproute2 路由表编号":                               "iproute2 routing table index",
	"不进入 TUN 的网段 (务必包含 SSH 对端)":                    "Subnets kept out of TUN (must include your SSH peer)",
	"严格路由 (防止流量绕过; android 生效)":                    "Strict route (prevents leaks; effective on Android)",
	"使用 /etc/hosts":                                "Use /etc/hosts",
	"启用 DNS 覆写 (覆盖订阅的 dns 配置)":                     "Enable DNS override (replaces the subscription's dns section)",
	"启用 Fake-IP (enhanced-mode=fake-ip 时生效)":       "Enable Fake-IP (effective when enhanced-mode=fake-ip)",
	"启用 TUN 透明代理 (需要 CAP_NET_ADMIN; 默认关闭)":         "Enable TUN transparent proxy (needs CAP_NET_ADMIN; off by default)",
	"备用 DNS (解析国内域名)":                              "Fallback DNS (resolves domestic domains)",
	"端点无关 NAT (提升 UDP 兼容性)":                        "Endpoint-independent NAT (better UDP compatibility)",
	"自动检测出口网卡":                                     "Auto-detect the outbound interface",
	"自动配置 iptables redirect (Linux)":               "Auto-configure iptables redirect (Linux)",
	"自动配置路由表 (iptables/nftables)":                  "Auto-configure routing tables (iptables/nftables)",

	// ---- v1.3.0 leftovers ----
	"省略参数": "no argument",

	// ---- v1.3.0 leftovers ----
	"只允许字母/数字/-/_":                                              "letters/digits/-/_ only",
	"撤销某个键的接管用 config unset <key>。":                             "use config unset <key> to stop managing a key",
	"警告: 已保存但暂时无法生效":                                            "Warning: saved but not applied yet",
	"警告: 服务重启后未就绪, 请执行 mihomo-cli log 查看原因":                     "Warning: the service is not ready after restart; run mihomo-cli log",
	"原配置备份在 /etc/mihomo-cli/config.toml.pre-1.3.bak":            "original config backed up at /etc/mihomo-cli/config.toml.pre-1.3.bak",
	"撤销接管: 该键回到跟随订阅/内核默认":                                       "Stop managing: the key follows the subscription/kernel default again",
	"点号路径键 (tun.* / dns.*) 用 unset 撤销, 该段就不再写进内核 yaml, 订阅原样保留;": "Dotted keys (tun.* / dns.*) are dropped with unset; that section is no longer written to the yaml, so the subscription is untouched",
	"扁平键 (mixed-port 等) 的 unset 等于设置为 sub (跟随订阅), 无默认值的键则清空。":   "For flat keys (mixed-port etc.) unset equals sub (follow the subscription); keys without a default are cleared",
	"已撤销":                        "reverted",
	"mihomo-cli sub use <id|名称>": "mihomo-cli sub use <id|name>",

	// ---- v1.3.0 leftovers ----
	"优先热重载, 失败才重启服务": "hot reload first, restart only if that fails",

	// ---- v1.3.0 leftovers ----
	"内核 API 未就绪, 稍后重试":       "the core API is not ready yet; retry in a moment",
	"没有可回写的运行值(内核 API 未就绪?)": "no running value could be read back (core API not ready?)",

	// ---- v1.4.0 ----
	"长值已截断, 完整值: mihomo-cli config get <key>":                "long values truncated; full value: mihomo-cli config get <key>",
	"警告: 未找到 %s, timer 将使用 %s (安装后请重跑 install --systemd)\\n": "Warning: %s not found; timers will use %s (re-run install --systemd after installing)\\n",

	// ---- v1.4.1 详版帮助 ----
	"内核键名": "YAML KEY",
	"取值":   "VALUES",
	"撤销接管": "STOP MANAGING",
	"查看":   "SHOW",
	"未注册的内核配置项: 直接读写内核 config.yaml 的这个路径, 不受默认值/校验保护。": "Unregistered core key: reads/writes this path of the core config.yaml directly, with no defaults or validation.",
	"布尔":        "boolean",
	"整数":        "integer",
	"小数":        "float",
	"列表 (逗号分隔)": "list (comma separated)",
	"字符串":       "string",
	"键名统一为 <段>.<键>: core=内核顶层键 / cli=CLI 自身 / timer=systemd / dns,tun=内核同名段。": "Keys are <section>.<key>: core=core top-level / cli=the CLI itself / timer=systemd / dns,tun=the core's sections.",
	"开启域名嗅探":           "enable the domain sniffer",
	"用 metadb 而不是 dat": "use metadb instead of dat",
	"改 Fake-IP 段":      "change the Fake-IP range",
	"查看/更新单个数据资源":      "show/update a single data resource",
	"决定流量如何被分流。rule 是日常推荐; global/direct 用于临时全量代理或全量直连。": "How traffic is split. rule is the daily recommendation; global/direct are for temporarily proxying or bypassing everything.",
	"按规则分流: 国内直连/国外代理 (推荐)":                              "by rule: domestic direct, foreign via proxy (recommended)",
	"所有流量都走代理 (调试用; 正常上网会很慢)":                            "proxy everything (debugging; normal browsing gets slow)",
	"所有流量都直连, 不代理 (相当于临时关掉代理)":                           "direct everything, no proxy (like turning the proxy off)",
	"跟随订阅: 用订阅里的 mode, CLI 不注入":                          "follow the subscription: use its mode, the CLI injects nothing",
	"内核写进 journal 的日志量。排查节点/规则问题时用 debug, 日常用 info。":     "How much the core logs to the journal. Use debug when diagnosing nodes/rules, info day to day.",
	"最详细: 每条连接匹配了哪条规则都记 (日志量很大)":                         "most verbose: which rule matched each connection (very noisy)",
	"常规信息 (推荐)": "routine information (recommended)",
	"只记警告":      "warnings only",
	"只记错误":      "errors only",
	"完全静音":      "completely silent",
	"跟随订阅: 用订阅里的 log-level":             "follow the subscription's log-level",
	"影响所有命令的提示/帮助/表格表头。节点名和订阅内容始终原样显示。": "Affects prompts, help and table headers. Node names and subscription content are always shown as-is.",
	"按系统 locale 判断, 中文环境用中文, 其它用英文":     "follow the system locale: Chinese on a Chinese locale, English otherwise",

	// ---- v1.4.1 leftovers ----
	"查看 tun 段全部配置项 (等于 config get tun)": "show every key of the tun section (same as config get tun)",
	"TUN 的其它参数都走 config set":            "every other TUN knob goes through config set",

	"其它": "anything else",

	// ---- v1.4.1 apply/adopt ----
	"配置管理: get/set/reset-default/unset/apply/adopt":          "Config management: get/set/reset-default/unset/apply/adopt",
	"手动改动后执行 config apply 以配置文件覆盖运行中的服务, config adopt 反之;":   "after a manual edit run config apply to let the file win, or config adopt to let the running state win",
	"如需恢复原值: mihomo-cli config apply <key>":                  "to restore: mihomo-cli config apply <key>",
	"旧名, 等价于 config apply":                                   "legacy name, same as config apply",
	"旧名, 等价于 config adopt":                                   "legacy name, same as config adopt",
	"适用于: 被别人的 systemctl 改过 timer、或想让文件追上现实时。":               "Use when a timer was changed by someone else's systemctl, or to bring the file back in line with reality.",
	"日常不需要: config set 已经会立即生效, config get 的 STATE 列就是漂移视图。": "Not needed day to day: config set applies immediately and config get's STATE column is the drift view.",
	"按 config.toml 重渲染运行配置并让内核/定时器跟上; 端口等需要重建监听的键会自动重启。":     "Re-renders the runtime config from config.toml and makes the core/timers follow; keys that need new listeners restart the service.",

	// ---- v1.4.1 leftovers ----
	"日常不需要: config set 已经会立即生效。": "Not needed day to day: config set applies immediately.",
	"旧名, 等价于":      "legacy name, same as",
	"config apply": "config apply",

	// ---- v1.4.1 leftovers ----
	"该值已写入配置, 但内核还没用它":                 "written to the config, but the core is not using it yet",
	"它需要重建监听, 请执行: mihomo-cli restart": "it needs new listeners; run: mihomo-cli restart",
	"请执行: mihomo-cli config apply ":    "run: mihomo-cli config apply ",
	"核对":                               "check",
	"注意":                               "note",

	// ---- v1.4.1 dns ----
	"自定义 DNS 会覆盖订阅中的 dns 配置; off 恢复跟随订阅。":                               "Custom DNS overrides the subscription's dns section; off follows the subscription again.",
	"裸命令等于 config get dns, 只显示参数表 (预设表见 config set dns.nameserver -h);": "The bare command equals config get dns and shows only the parameter table (presets: config set dns.nameserver -h);",
	"写操作统一走 config set, 这里只留 on/off 两个糖:":                               "All writes go through config set; only on/off remain here as sugar:",
	"开启 DNS 覆写 (等于 config set dns.enable true)":                         "enable DNS override (same as config set dns.enable true)",
	"关闭 DNS 覆写, 恢复跟随订阅":                                                 "disable DNS override, follow the subscription again",
	"设 DNS 服务器 (预设名, TAB 可补全)":                                          "set DNS servers (preset name, TAB completes)",
	"恢复跟随订阅": "follow the subscription again",
	"用法: mihomo-cli config set dns.nameserver <预设名|subN|ip...> | mihomo-cli dns off": "Usage: mihomo-cli config set dns.nameserver <preset|subN|ip...> | mihomo-cli dns off",
	"自定义 IP": "custom IP",
	"DNS 预设": "DNS presets",
	"预设名列表: mihomo-cli config set dns.nameserver -h": "preset list: mihomo-cli config set dns.nameserver -h",
}
