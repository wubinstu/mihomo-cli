package cfg

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"gopkg.in/yaml.v3"
)

// TestURLPresets 测速端点预设 (config set test-url 的补全与默认值同形)
var TestURLPresets = []string{
	"https://www.gstatic.com/generate_204",
	"http://www.gstatic.com/generate_204",
	"http://1.1.1.1/generate_204",
	"http://www.qualcomm.cn/generate_204",
	"http://cp.cloudflare.com/generate_204",
	"http://connect.rom.miui.com/generate_204",
	"http://www.apple.com/library/test/success.html",
	"http://connectivitycheck.platform.hicloud.com/generate_204",
}

// dnsPresetNames DNS 预设名 (config set dns.nameserver 的补全)。
// 预设表本身在 internal/cmd (要输出中文说明), 这里只放名字, 由 cmd 包注册进来,
// 避免 cfg → cmd 的依赖倒置。
var dnsPresetNames = func() []string { return nil }

// SetDNSPresetNames 由 cmd 包注入预设名与解析函数
func SetDNSPresetNames(names []string, resolve func(s *app.Settings, v string) ([]string, error)) {
	dnsPresetNames = func() []string { return names }
	dnsResolve = resolve
}

var dnsResolve func(s *app.Settings, v string) ([]string, error)

// resolveDNSValue 把 dns.nameserver 的值扩展成逗号分隔的真实值:
// 预设名 (cloudflare) / subN (订阅自带 DNS) → IP 或 URL 列表; 其它原样返回。
func resolveDNSValue(s *app.Settings, v string) (string, error) {
	if dnsResolve != nil {
		if out, err := dnsResolve(s, v); err == nil && len(out) > 0 {
			return strings.Join(out, ","), nil
		}
	}
	return v, nil
}

// GithubMirrorPresets GitHub 镜像站预设 (config set cli.github-mirror 的补全)
var GithubMirrorPresets = []string{
	"https://ghfast.top",
	"https://gh-proxy.com",
	"https://mirror.ghproxy.com",
}

// LangPresets cli-language 可选值
var LangPresets = []string{"auto", "zh", "en"}

// Keys 全部注册的配置项 (顺序即 config get 的展示顺序)
var Keys = []Key{
	// ---- core config: 注入内核 config.yaml (sub=不注入, 跟随订阅) ----
	{Name: "core.allow-lan", Section: "core", Kind: KindBool, Def: "false",
		Usage:   "允许局域网设备使用代理 (监听 0.0.0.0)",
		Detail:  "关时只监听 127.0.0.1 (只有本机能用); 开时监听 0.0.0.0, 局域网设备可用。\n注意: 开放后同网段任何人都能通过这台机器上网, 请在受信任网络里使用。",
		Example: "config set core.allow-lan true",
		Get:     func(s *app.Settings) string { return boolStr(s.AllowLan) },
		Set:     func(s *app.Settings, v string) error { s.AllowLan = v == "true"; return nil }},
	{Name: "core.mixed-port", Section: "core", Kind: KindPort, Def: "7890", Restart: true,
		Usage:   "混合代理端口 (http + socks5); off=关闭",
		Detail:  "一个端口同时提供 HTTP 和 SOCKS5 代理, 推荐只开这一个。\n改成 off 后本机与局域网都无法通过端口用代理 (TUN 模式除外)。",
		Example: "config set core.mixed-port 7890",
		Get:     func(s *app.Settings) string { return portStr(s.MixedPort) },
		Set:     func(s *app.Settings, v string) error { s.MixedPort = v; return nil }},
	{Name: "core.socks-port", Section: "core", Kind: KindPort, Def: "off", Restart: true,
		Usage:   "独立 SOCKS5 端口; off=关闭 (mixed-port 已含 socks5)",
		Detail:  "只有当某个客户端必须用独立 SOCKS5 端口时才需要开; mixed-port 已经能同时提供 socks5。\n三个端口 (mixed/socks/http) 不能填成同一个号。",
		Example: "config set core.socks-port 7891",
		Get:     func(s *app.Settings) string { return portStr(s.SocksPort) },
		Set:     func(s *app.Settings, v string) error { s.SocksPort = v; return nil }},
	{Name: "core.http-port", Section: "core", Kind: KindPort, Def: "off", Restart: true,
		Usage:   "独立 HTTP(S) 代理端口; off=关闭 (mixed-port 已含 http)",
		Detail:  "只有当某个客户端必须用独立 HTTP 端口时才需要开; mixed-port 已经能同时提供 http。\n三个端口 (mixed/socks/http) 不能填成同一个号。",
		Example: "config set core.http-port 7892",
		Get:     func(s *app.Settings) string { return portStr(s.HTTPPort) },
		Set:     func(s *app.Settings, v string) error { s.HTTPPort = v; return nil }},
	{Name: "core.proxy-mode", Section: "core", Kind: KindEnum, Def: "rule",
		Enum:   []string{"rule", "global", "direct", "sub"},
		Usage:  "代理模式; sub=跟随订阅 (热切换)",
		Detail: "决定流量如何被分流。rule 是日常推荐; global/direct 用于临时全量代理或全量直连。",
		Values: []ValueDoc{
			{"rule", "按规则分流: 国内直连/国外代理 (推荐)"},
			{"global", "所有流量都走代理 (调试用; 正常上网会很慢)"},
			{"direct", "所有流量都直连, 不代理 (相当于临时关掉代理)"},
			{"sub", "跟随订阅: 用订阅里的 mode, CLI 不注入"},
		},
		Example: "config set core.proxy-mode global",
		Get:     func(s *app.Settings) string { return orDef(s.ProxyMode, "rule") },
		Set: func(s *app.Settings, v string) error {
			if v == "sub" {
				s.ProxyMode = "sub"
			} else {
				s.ProxyMode = v
			}
			return nil
		}},
	{Name: "core.ipv6-enabled", Section: "core", Kind: KindBool, Def: "false",
		Usage:   "启用 IPv6 转发",
		Detail:  "开时内核会处理 AAAA 记录并通过代理访问 IPv6 目标。\n服务器没有 IPv6 出口时建议关闭, 否则可能反而连不上。",
		Example: "config set core.ipv6-enabled false",
		Get:     func(s *app.Settings) string { return boolStr(s.IPV6Enabled) },
		Set:     func(s *app.Settings, v string) error { s.IPV6Enabled = v == "true"; return nil }},
	{Name: "core.log-level", Section: "core", Kind: KindEnum, Def: "info",
		Enum:   []string{"debug", "info", "warning", "error", "silent", "sub"},
		Usage:  "内核日志等级; sub=跟随订阅 (热切换)",
		Detail: "内核写进 journal 的日志量。排查节点/规则问题时用 debug, 日常用 info。",
		Values: []ValueDoc{
			{"debug", "最详细: 每条连接匹配了哪条规则都记 (日志量很大)"},
			{"info", "常规信息 (推荐)"},
			{"warning", "只记警告"},
			{"error", "只记错误"},
			{"silent", "完全静音"},
			{"sub", "跟随订阅: 用订阅里的 log-level"},
		},
		Example: "config set core.log-level debug",
		Get:     func(s *app.Settings) string { return orDef(s.LogLevel, "info") },
		Set:     func(s *app.Settings, v string) error { s.LogLevel = v; return nil }},
	{Name: "core.tcp-concurrent", Section: "core", Kind: KindTri, Def: "true",
		Usage:   "TCP 并发连接 (多路复用, 提速); sub=跟随订阅",
		Detail:  "开时同一目的地的多个请求会复用同一条 TCP 连接, 明显降低握手开销。\n极少数对连接复用敏感的服务可能异常, 那时关掉试试。",
		Example: "config set core.tcp-concurrent true",
		Get:     func(s *app.Settings) string { return orDef(s.TCPConcurrent, "true") },
		Set:     func(s *app.Settings, v string) error { s.TCPConcurrent = v; return nil }},
	{Name: "core.unified-delay", Section: "core", Kind: KindTri, Def: "true",
		Usage:   "统一延迟计算 (URL-Test 更精准); sub=跟随订阅",
		Detail:  "开时延迟测试会把「建连+首字节」合并计算, URL-Test 择优更准。\n只影响测速显示和自动择优, 不影响实际转发。",
		Example: "config set core.unified-delay false",
		Get:     func(s *app.Settings) string { return orDef(s.UnifiedDelay, "true") },
		Set:     func(s *app.Settings, v string) error { s.UnifiedDelay = v; return nil }},
	{Name: "core.keep-alive-interval", Section: "core", Kind: KindInt, Def: "30", Min: 1, Max: 600,
		Comp:    []string{"sub", "15", "30", "60", "120", "300"},
		Usage:   "长连接保活间隔 (秒); sub=跟随订阅",
		Detail:  "空闲连接多久发一次保活包。太小浪费流量, 太大可能被中间设备掐断。\n30 秒是常见选择; 移动网络下可改 15。",
		Example: "config set core.keep-alive-interval 15",
		Get:     func(s *app.Settings) string { return orDef(s.KeepAliveInterval, "30") },
		Set:     func(s *app.Settings, v string) error { s.KeepAliveInterval = v; return nil }},

	// ---- cli: 只影响 mihomo-cli 自身 ----
	{Name: "cli.language", Section: "cli", Kind: KindEnum, Def: "auto", Enum: LangPresets,
		Usage:  "输出语言; auto=按系统 locale",
		Detail: "影响所有命令的提示/帮助/表格表头。节点名和订阅内容始终原样显示。",
		Values: []ValueDoc{
			{"auto", "按系统 locale 判断, 中文环境用中文, 其它用英文"},
			{"zh", "中文"},
			{"en", "English"},
		},
		Example: "config set cli.language en",
		Get: func(s *app.Settings) string {
			if s.Language == "" {
				return "auto"
			}
			return s.Language
		},
		Set: func(s *app.Settings, v string) error {
			if v == "auto" {
				s.Language = ""
			} else {
				s.Language = v
			}
			return nil
		}},
	{Name: "cli.github-mirror", Section: "cli", Kind: KindMirror, Def: "auto", Enum: append([]string{"auto"}, GithubMirrorPresets...),
		Usage:   "GitHub 镜像站前缀 (下载内核/资源失败时依次尝试); auto=内置列表",
		Detail:  "下载内核/geo 资源/CLI 自更新时, GitHub 直连失败会自动改走镜像站。\nauto = 依次尝试内置的 3 个镜像; 填了具体地址就固定用它。",
		Example: "config set cli.github-mirror https://ghfast.top",
		Get: func(s *app.Settings) string {
			if s.GithubMirror == "" {
				return "auto"
			}
			return s.GithubMirror
		},
		Set: func(s *app.Settings, v string) error {
			if v == "auto" {
				s.GithubMirror = ""
			} else {
				s.GithubMirror = v
			}
			return nil
		}},
	{Name: "cli.test-url", Section: "cli", Kind: KindURL, Def: TestURLPresets[0], Enum: TestURLPresets,
		Usage:   "测速/连通性检测 URL (204 端点)",
		Detail:  "节点测速 (node test / node auto) 和 doctor 都请求这个 URL, 用返回 204 的时间当延迟。\n换一个离服务器近、且被代理允许的端点, 测速结果会更真实。",
		Example: "config set cli.test-url http://cp.cloudflare.com/generate_204",
		Get:     func(s *app.Settings) string { return s.TestURL },
		Set:     func(s *app.Settings, v string) error { s.TestURL = v; return nil }},
	{Name: "cli.test-timeout-ms", Section: "cli", Kind: KindInt, Def: "5000", Min: 100, Max: 60000,
		Comp:  []string{"1000", "3000", "5000", "10000"},
		Usage: "测速超时 (毫秒)",
		Get:   func(s *app.Settings) string { return strconv.Itoa(s.TestTimeout) },
		Set: func(s *app.Settings, v string) error {
			n, _ := strconv.Atoi(v)
			s.TestTimeout = n
			return nil
		}},
	{Name: "cli.current-profile", Section: "cli", Kind: KindState,
		Usage: "当前生效订阅 (只读; 用 sub use 切换)",
		Get:   func(s *app.Settings) string { return orDash(s.CurrentProfile) }},
	{Name: "cli.current-group", Section: "cli", Kind: KindState,
		Usage: "当前操作分组 (只读; 用 group use 切换)",
		Get:   func(s *app.Settings) string { return orDash(s.CurrentGroup) }},

	// ---- systemd timers ----
	{Name: "timer.sub-auto-update-enabled", Section: "timer", Kind: KindBool, Def: "true",
		Usage: "定时更新全部订阅 (作用域: 所有订阅)",
		Get:   func(s *app.Settings) string { return boolStr(s.SubAutoUpdateEnabled) },
		Set:   func(s *app.Settings, v string) error { s.SubAutoUpdateEnabled = v == "true"; return nil }},
	{Name: "timer.sub-auto-update-interval", Section: "timer", Kind: KindDur, Def: "24h", Min: 60,
		Usage: "订阅自动更新周期",
		Get:   func(s *app.Settings) string { return durStr(s.SubAutoUpdateInterval) },
		Set: func(s *app.Settings, v string) error {
			d, err := parseDur(v)
			if err == nil {
				s.SubAutoUpdateInterval = d
			}
			return err
		}},
	{Name: "timer.node-auto-select-enabled", Section: "timer", Kind: KindBool, Def: "false",
		Usage: "定时对当前订阅的当前分组自动择优 (作用域: 仅当前)",
		Get:   func(s *app.Settings) string { return boolStr(s.NodeAutoSelectEnabled) },
		Set:   func(s *app.Settings, v string) error { s.NodeAutoSelectEnabled = v == "true"; return nil }},
	{Name: "timer.node-auto-select-interval", Section: "timer", Kind: KindDur, Def: "30m", Min: 60,
		Usage: "自动择优周期",
		Get:   func(s *app.Settings) string { return durStr(s.NodeAutoSelectInterval) },
		Set: func(s *app.Settings, v string) error {
			d, err := parseDur(v)
			if err == nil {
				s.NodeAutoSelectInterval = d
			}
			return err
		}},
	{Name: "timer.resource-auto-update-enabled", Section: "timer", Kind: KindBool, Def: "false",
		Usage: "定时更新 geo 资源文件 (作用域: 全局)",
		Get:   func(s *app.Settings) string { return boolStr(s.ResourceAutoUpdateEnabled) },
		Set:   func(s *app.Settings, v string) error { s.ResourceAutoUpdateEnabled = v == "true"; return nil }},
	{Name: "timer.resource-auto-update-interval", Section: "timer", Kind: KindDur, Def: "24h", Min: 3600,
		Usage: "geo 资源更新周期",
		Get:   func(s *app.Settings) string { return durStr(s.ResourceAutoUpdateInterval) },
		Set: func(s *app.Settings, v string) error {
			d, err := parseDur(v)
			if err == nil {
				s.ResourceAutoUpdateInterval = d
			}
			return err
		}},

	// ---- dns 段 (点号路径; 只用过一次才写进内核 yaml) ----
	dnsKey("core.dns.enable", KindBool, "false", "启用 DNS 覆写 (覆盖订阅的 dns 配置)"),
	dnsKey("core.dns.ipv6", KindBool, "false", "DNS 解析 IPv6 结果 (AAAA)"),
	func() Key {
		k := dnsKey("core.dns.enhanced-mode", KindEnum, "fake-ip", "DNS 增强模式")
		k.Detail = "决定 DNS 对代理域名返回什么地址。fake-ip 兼容性最好; redir-host 返回真实 IP, 便于排查。"
		k.Values = []ValueDoc{
			{"fake-ip", "返回虚拟 IP (推荐: 兼容最好, 支持按域名分流)"},
			{"redir-host", "返回真实 IP (便于抓包排查, 部分客户端会绕过分流)"},
			{"normal", "只转发不返回映射 (兼容性最差)"},
		}
		k.Example = "config set dns.enhanced-mode redir-host"
		return k
	}(),
	dnsKey("core.dns.fake-ip", KindBool, "true", "启用 Fake-IP (enhanced-mode=fake-ip 时生效)"),
	dnsKey("core.dns.fake-ip-range", KindText, "28.0.0.1/8", "Fake-IP 地址段"),
	dnsKey("core.dns.use-system-hosts", KindBool, "true", "使用 /etc/hosts"),
	dnsKey("core.dns.listen", KindText, "0.0.0.0:53", "DNS 监听地址"),
	dnsKey("core.dns.fallback", KindNameList, "sub", "备用 DNS (解析国内域名)"),
	dnsKey("core.dns.default-nameserver", KindNameList, "223.5.5.5,119.29.29.29", "DNS 引导解析器 (必须是纯 IP)"),
	dnsKey("core.dns.nameserver", KindNameList, "sub", "DNS 服务器列表 (IP 或 DoH/DoT URL)"),

	// ---- tun 段 (点号路径; 开启需要 CAP_NET_ADMIN, 见 cmd 的 TUN 安全护栏) ----
	tunKey("core.tun.enable", KindBool, "false", "启用 TUN 透明代理 (需要 CAP_NET_ADMIN; 默认关闭)", true),
	func() Key {
		k := tunKey("core.tun.stack", KindEnum, "mips", "TUN 网络栈", true)
		k.Detail = "TUN 网卡用哪种协议栈转发。mips 是内核自研的用户态栈 (默认); system 性能最好但依赖内核特性; gvisor 兼容性最好。"
		k.Values = []ValueDoc{
			{"system", "内核协议栈 (性能最好, 需要较新内核; 开了防火墙的平台可能不可用)"},
			{"gvisor", "用户态协议栈 (兼容性最好, 性能一般)"},
			{"mixed", "TCP 走 system、其余走 gvisor (两者混合)"},
			{"mips", "内核自研用户态协议栈 (mipstack, 默认)"},
		}
		k.Example = "config set tun.stack gvisor"
		return k
	}(),
	tunKey("core.tun.device", KindText, "Meta", "TUN 网卡名", true),
	tunKey("core.tun.mtu", KindInt, "9000", "TUN MTU", true),
	tunKey("core.tun.dns-hijack", KindList, "0.0.0.0:53", "DNS 劫持规则", true),
	tunKey("core.tun.auto-route", KindBool, "true", "自动配置路由表 (iptables/nftables)", true),
	tunKey("core.tun.auto-detect-interface", KindBool, "true", "自动检测出口网卡", true),
	tunKey("core.tun.strict-route", KindBool, "false", "严格路由 (防止流量绕过; android 生效)", true),
	tunKey("core.tun.route-exclude-address", KindCIDRList, "", "不进入 TUN 的网段 (务必包含 SSH 对端)", true),
	tunKey("core.tun.endpoint-independent-nat", KindBool, "true", "端点无关 NAT (提升 UDP 兼容性)", true),
	tunKey("core.tun.udp-timeout", KindInt, "300", "UDP 会话超时 (秒)", true),
	tunKey("core.tun.auto-redirect", KindBool, "true", "自动配置 iptables redirect (Linux)", true),
	tunKey("core.tun.iproute2-table-index", KindInt, "2022", "iproute2 路由表编号", true),
}

// tunEnum tun.stack 的可选值 (与内核一致: system/gvisor/mixed/mips)
var tunEnums = map[string][]string{
	"core.tun.stack": {"system", "gvisor", "mixed", "mips"},
}

// dnsEnum dns.enhanced-mode 的可选值
var dnsEnums = map[string][]string{
	"core.dns.enhanced-mode": {"fake-ip", "redir-host", "normal"},
}

// SecList 读出段里的一个列表键 (逗号分隔的展示形式 → 切片); 没有返回 nil
func SecList(s *app.Settings, dotted string) []string {
	v := secGet(s, dotted)
	if v == "" || v == "sub" {
		return nil
	}
	var out []string
	for _, it := range strings.Split(v, ",") {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, it)
		}
	}
	return out
}

// DNSSectionActive dns 段是否注入内核 yaml。
// 显式 false = 一律不注入 (段里别的键也忽略); true = 注入;
// 没设开关 = 只要设了 nameserver 就算要用。
func DNSSectionActive(s *app.Settings) bool {
	switch secGet(s, "core.dns.enable") {
	case "false":
		return false
	case "true":
		return true
	}
	return secGet(s, "core.dns.nameserver") != ""
}

// SecStoreName 段名 → 内核 yaml / Overrides 里的段名: core.dns → dns (导出给 render 用)
func SecStoreName(section string) string { return secStore(section) }

// secStore 段名 → Overrides 里的存储键: core.dns → dns。
// CLI 键名统一带 core. 前缀 (core.dns.enable 就是内核 yaml 的 dns.enable),
// 但磁盘上的 [overrides] 段名仍和 yaml 段名一致, 少一层嵌套。
func secStore(section string) string { return strings.TrimPrefix(section, "core.") }

// tunKey 构造 tun 段键 (存 [overrides.tun])
func tunKey(name string, kind Kind, def, usage string, restart bool) Key {
	return Key{
		Name: name, Section: "core.tun", Kind: kind, Def: def, Enum: tunEnums[name],
		Usage: usage, Restart: restart,
		Get: func(s *app.Settings) string { return secGet(s, name) },
		Set: func(s *app.Settings, v string) error { return secSet(s, name, v, kind, tunEnums[name]) },
	}
}

// dnsKey 构造 dns 段键 (存 [overrides.dns])
func dnsKey(name string, kind Kind, def, usage string) Key {
	base := Key{
		Name: name, Section: "core.dns", Kind: kind, Def: def, Enum: dnsEnums[name],
		Usage: usage,
		Get:   func(s *app.Settings) string { return secGet(s, name) },
		Set:   func(s *app.Settings, v string) error { return secSet(s, name, v, kind, dnsEnums[name]) },
	}
	if name == "core.dns.enable" {
		// 开关的语义: 显式 false = 整段不注入; true = 注入; 没设 = 只要设了 nameserver 就算要用
		// (老 `dns use` 的语义, 现在 nameserver 和段里其它键同一个存储, 特权字段没了)
		base.Get = func(s *app.Settings) string {
			if DNSSectionActive(s) {
				return "true"
			}
			return "false"
		}
		base.Set = func(s *app.Settings, v string) error {
			if v == "false" {
				// 显式写 false: 盖住可能已存在的 nameserver, 整段不注入
				return secSet(s, "core.dns.enable", "false", KindBool, nil)
			}
			return secSet(s, "core.dns.enable", "true", KindBool, nil)
		}
	}
	if name == "core.dns.nameserver" {
		// 没设过就报 "sub" (跟随订阅), 和默认值一致; 否则 SETTING 列会显示 "-"
		base.Get = func(s *app.Settings) string {
			if v := secGet(s, "core.dns.nameserver"); v != "" {
				return v
			}
			return "sub"
		}
		base.Resolve = resolveDNSValue // 预设名 / subN → 逗号分隔的真实值
		// 值域: 预设名 / subN / IP / DoH-DoT URL / sub
		base.CompFn = func() []string { return dnsPresetNames() } // 间接一层, 让 cmd 包后注入的预设名生效
		base.Detail = "DNS 服务器列表。可以填预设名 (TAB 可补全)、订阅编号 subN、裸 IP 或 DoH/DoT URL;\n" +
			"sub = 跟随订阅, 不注入。"
		base.Values = []ValueDoc{
			{"<预设名>", "ali / 114 / google / cloudflare / adguard / quad9 / dnspod"},
			{"subN", "用第 N 个订阅自带的 dns.nameserver"},
			{"<ip>", "一个或多个 IP, 逗号分隔"},
			{"<url>", "DoH/DoT 地址, 如 https://dns.alidns.com/dns-query"},
			{"sub", "跟随订阅, CLI 不注入"},
		}
		base.Example = "config set core.dns.nameserver cloudflare"
	}
	return base
}

// ---- Overrides (点号路径) 访问器 ----

// secGet 读 [overrides] 里的点号路径值; 不存在返回 ""
// dotted 是完整键名 (core.dns.enable), 落到 Overrides 时去掉 core. 前缀 → dns.enable
func secGet(s *app.Settings, dotted string) string {
	if s.Overrides == nil {
		return ""
	}
	parts := strings.Split(stripCore(dotted), ".")
	var cur any = s.Overrides
	for i, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		v, ok := m[p]
		if !ok {
			return ""
		}
		if i == len(parts)-1 {
			return scalarStr(v)
		}
		cur = v
	}
	return ""
}

// secSet 写 [overrides]; 值按 Kind 解析为对应 Go 类型; "sub" = 删除该键(跟随订阅)
func secSet(s *app.Settings, dotted, v string, kind Kind, enum []string) error {
	parsed, err := (Key{Kind: kind, Enum: enum}).Parse(v)
	if err != nil {
		return err
	}
	if parsed == "sub" {
		return secDel(s, dotted)
	}
	parts := strings.Split(stripCore(dotted), ".")
	if s.Overrides == nil {
		s.Overrides = map[string]any{}
	}
	cur := s.Overrides
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = storeValue(parsed, kind)
	return nil
}

// secDel 删除 [overrides] 里的点号路径
func secDel(s *app.Settings, dotted string) error {
	if s.Overrides == nil {
		return nil
	}
	parts := strings.Split(stripCore(dotted), ".")
	cur := s.Overrides
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	delete(cur, parts[len(parts)-1])
	// 清理空段
	for i := len(parts) - 1; i > 0; i-- {
		parent := s.Overrides
		for _, p := range parts[:i-1] {
			parent, _ = parent[p].(map[string]any)
			if parent == nil {
				return nil
			}
		}
		if m, ok := parent[parts[i-1]].(map[string]any); ok && len(m) == 0 {
			delete(parent, parts[i-1])
		}
	}
	return nil
}

// secUsed 段是否被使用过
func secUsed(s *app.Settings, section string) bool {
	if s.Overrides == nil {
		return false
	}
	m, ok := s.Overrides[secStore(section)].(map[string]any)
	return ok && len(m) > 0
}

// stripCore 去掉键名/段名的 core. 前缀, 得到 Overrides 里的存储路径
// (core.dns.enable → dns.enable; core.dns → dns)
func stripCore(dotted string) string { return strings.TrimPrefix(dotted, "core.") }

// SecHas 某个点号路径键是否真的被 set 过 (不是"段里有没有别的键")
func SecHas(s *app.Settings, dotted string) bool {
	if s.Overrides == nil {
		return false
	}
	parts := strings.Split(stripCore(dotted), ".")
	var cur any = s.Overrides
	for i, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		v, exists := m[p]
		if !exists {
			return false
		}
		if i == len(parts)-1 {
			return true
		}
		cur = v
	}
	return false
}

// SectionUsed 段是否被使用过 (对外)
func SectionUsed(s *app.Settings, section string) bool {
	return secUsed(s, section)
}

// storeValue 按 Kind 把归一化字符串转成可进 TOML/yaml 的 Go 值
func storeValue(parsed string, kind Kind) any {
	switch kind {
	case KindBool:
		return parsed == "true"
	case KindInt:
		n, _ := strconv.Atoi(parsed)
		return int64(n)
	case KindList, KindNameList, KindCIDRList:
		items := []any{}
		for _, it := range strings.Split(parsed, ",") {
			if it = strings.TrimSpace(it); it != "" {
				items = append(items, it)
			}
		}
		return items
	}
	return parsed
}

// scalarStr 把 Go 值渲染成展示字符串
func scalarStr(v any) string {
	switch t := v.(type) {
	case bool:
		return boolStr(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(t))
		for _, it := range t {
			parts = append(parts, scalarStr(it))
		}
		return strings.Join(parts, ",")
	case string:
		return t
	case nil:
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// ---- 通用键 (未注册的任意 yaml 路径) ----

// GetGeneric 读未注册的点号路径 (顶层键亦可); ok=false 表示未设置
func GetGeneric(s *app.Settings, dotted string) (string, bool) {
	v := secGet(s, dotted)
	if v == "" && !genericSet(s, dotted) {
		return "", false
	}
	return v, true
}

func genericSet(s *app.Settings, dotted string) bool {
	if s.Overrides == nil {
		return false
	}
	parts := strings.Split(dotted, ".")
	cur := s.Overrides
	for i, p := range parts {
		m, ok := cur[p].(map[string]any)
		if !ok {
			return false
		}
		if _, exists := m[p]; !exists {
			return false
		}
		if i == len(parts)-1 {
			return true
		}
		cur = m
	}
	return false
}

// validPath 配置路径校验: 非空, 不含空段/首尾点号/非法字符
func validPath(dotted string) error {
	if dotted == "" {
		return fmt.Errorf("%s: %q", i18n.T("非法配置路径"), dotted)
	}
	for _, seg := range strings.Split(dotted, ".") {
		if seg == "" {
			return fmt.Errorf("%s: %q", i18n.T("非法配置路径"), dotted)
		}
		for _, r := range seg {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return fmt.Errorf("%s: %q (%s)", i18n.T("非法配置路径"), dotted, i18n.T("只允许字母/数字/-/_"))
			}
		}
	}
	return nil
}

// SetGeneric 写未注册的点号路径; 值按字面推断类型 (bool/int/float/list/string)
func SetGeneric(s *app.Settings, dotted, v string) error {
	if err := validPath(dotted); err != nil {
		return err
	}
	parsed, err := (Key{Kind: KindText}).Parse(v)
	if err != nil {
		return err
	}
	parts := strings.Split(stripCore(dotted), ".")
	if s.Overrides == nil {
		s.Overrides = map[string]any{}
	}
	cur := s.Overrides
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = inferValue(parsed)
	return nil
}

// inferValue 字面量推断: true/false→bool, 整数→int64, 小数→float64, 含逗号→列表, 其余字符串
func inferValue(s string) any {
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if strings.Contains(s, ",") {
		items := []any{}
		for _, it := range strings.Split(s, ",") {
			if it = strings.TrimSpace(it); it != "" {
				items = append(items, it)
			}
		}
		if len(items) > 1 {
			return items
		}
	}
	return s
}

// ShownSections config get 默认要露面的段 (core.dns/core.tun): 只看总开关是否有效开启。
// 别的键设了但 <段>.enable 没开 = 这段压根不参与渲染, 摆出来只会误导 ——
// v1.4.2 用户反馈"改任意一个字段后整段都冒出来"不合理, 改成只看 enable。
func ShownSections(s *app.Settings) []string {
	var out []string
	for _, sec := range []string{"core.dns", "core.tun"} {
		if SectionShown(s, sec) {
			out = append(out, sec)
		}
	}
	return out
}

// SectionShown 段的总开关是否有效开启
func SectionShown(s *app.Settings, section string) bool {
	k := Lookup(section + ".enable")
	if k == nil {
		return SectionUsed(s, section)
	}
	return k.Effective(s) == "true"
}

// HiddenSections 段里有设置、但总开关没开的段 (config get 末尾给一行提示用)
func HiddenSections(s *app.Settings) []string {
	var out []string
	for _, sec := range []string{"core.dns", "core.tun"} {
		if SectionUsed(s, sec) && !SectionShown(s, sec) {
			out = append(out, sec)
		}
	}
	return out
}

// OverrideSection 取出某段用于渲染 (nil=未使用)
func OverrideSection(s *app.Settings, section string) map[string]any {
	if s.Overrides == nil {
		return nil
	}
	m, _ := s.Overrides[secStore(section)].(map[string]any)
	if len(m) == 0 {
		return nil
	}
	return m
}

// CloneSection 深拷贝一段 (测试/渲染用)
func CloneSection(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			out[k] = CloneSection(sub)
			continue
		}
		out[k] = v
	}
	return out
}

// YAMLOf 把一段转成 yaml (仅测试用)
func YAMLOf(m map[string]any) string {
	b, _ := yaml.Marshal(m)
	return string(b)
}

// ---- 小工具 ----

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// orDef 空值=未设置 → 回退默认值 (Managed 键永远有具体值);
// "sub" = 跟随订阅 (与原语义一致)
func orDef(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// portStr 空值(未设置)=显示默认; off=off
func portStr(s string) string {
	switch s {
	case "":
		return "off"
	case "off", "sub":
		return s
	}
	return s
}

func durStr(d time.Duration) string {
	return DurHuman(d)
}

func parseDur(v string) (time.Duration, error) {
	return time.ParseDuration(v)
}

// DeleteSectionKey 删除一个点号路径键 (config unset 用)
func DeleteSectionKey(s *app.Settings, dotted string) error { return secDel(s, dotted) }

// DeleteGeneric 删除一个未注册的点号路径键
func DeleteGeneric(s *app.Settings, dotted string) error { return secDel(s, dotted) }
