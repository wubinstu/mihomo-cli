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

// GithubMirrorPresets GitHub 镜像站预设 (config set github-mirror 的补全)
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
	{Name: "allow-lan", Section: "core", Kind: KindBool, Def: "false",
		Usage: "允许局域网设备使用代理 (监听 0.0.0.0)",
		Get:   func(s *app.Settings) string { return boolStr(s.AllowLan) },
		Set:   func(s *app.Settings, v string) error { s.AllowLan = v == "true"; return nil }},
	{Name: "mixed-port", Section: "core", Kind: KindPort, Def: "7890", Restart: true,
		Usage: "混合代理端口 (http + socks5); off=关闭",
		Get:   func(s *app.Settings) string { return portStr(s.MixedPort) },
		Set:   func(s *app.Settings, v string) error { s.MixedPort = v; return nil }},
	{Name: "socks-port", Section: "core", Kind: KindPort, Def: "off", Restart: true,
		Usage: "独立 SOCKS5 端口; off=关闭 (mixed-port 已含 socks5)",
		Get:   func(s *app.Settings) string { return portStr(s.SocksPort) },
		Set:   func(s *app.Settings, v string) error { s.SocksPort = v; return nil }},
	{Name: "http-port", Section: "core", Kind: KindPort, Def: "off", Restart: true,
		Usage: "独立 HTTP(S) 代理端口; off=关闭 (mixed-port 已含 http)",
		Get:   func(s *app.Settings) string { return portStr(s.HTTPPort) },
		Set:   func(s *app.Settings, v string) error { s.HTTPPort = v; return nil }},
	{Name: "proxy-mode", Section: "core", Kind: KindEnum, Def: "rule",
		Enum:  []string{"rule", "global", "direct", "sub"},
		Usage: "代理模式; sub=跟随订阅 (热切换)",
		Get:   func(s *app.Settings) string { return orDef(s.ProxyMode, "rule") },
		Set: func(s *app.Settings, v string) error {
			if v == "sub" {
				s.ProxyMode = "sub"
			} else {
				s.ProxyMode = v
			}
			return nil
		}},
	{Name: "ipv6-enabled", Section: "core", Kind: KindBool, Def: "false",
		Usage: "启用 IPv6 转发",
		Get:   func(s *app.Settings) string { return boolStr(s.IPV6Enabled) },
		Set:   func(s *app.Settings, v string) error { s.IPV6Enabled = v == "true"; return nil }},
	{Name: "log-level", Section: "core", Kind: KindEnum, Def: "info",
		Enum:  []string{"debug", "info", "warning", "error", "silent", "sub"},
		Usage: "内核日志等级; sub=跟随订阅 (热切换)",
		Get:   func(s *app.Settings) string { return orDef(s.LogLevel, "info") },
		Set:   func(s *app.Settings, v string) error { s.LogLevel = v; return nil }},
	{Name: "tcp-concurrent", Section: "core", Kind: KindTri, Def: "true",
		Usage: "TCP 并发连接 (多路复用, 提速); sub=跟随订阅",
		Get:   func(s *app.Settings) string { return orDef(s.TCPConcurrent, "true") },
		Set:   func(s *app.Settings, v string) error { s.TCPConcurrent = v; return nil }},
	{Name: "unified-delay", Section: "core", Kind: KindTri, Def: "true",
		Usage: "统一延迟计算 (URL-Test 更精准); sub=跟随订阅",
		Get:   func(s *app.Settings) string { return orDef(s.UnifiedDelay, "true") },
		Set:   func(s *app.Settings, v string) error { s.UnifiedDelay = v; return nil }},
	{Name: "keep-alive-interval", Section: "core", Kind: KindInt, Def: "30", Min: 1, Max: 600,
		Comp:  []string{"sub", "15", "30", "60", "120", "300"},
		Usage: "长连接保活间隔 (秒); sub=跟随订阅",
		Get:   func(s *app.Settings) string { return orDef(s.KeepAliveInterval, "30") },
		Set:   func(s *app.Settings, v string) error { s.KeepAliveInterval = v; return nil }},

	// ---- cli: 只影响 mihomo-cli 自身 ----
	{Name: "cli-language", Section: "cli", Kind: KindEnum, Def: "auto", Enum: LangPresets,
		Usage: "输出语言; auto=按系统 locale",
		Get: func(s *app.Settings) string {
			if s.CLILanguage == "" {
				return "auto"
			}
			return s.CLILanguage
		},
		Set: func(s *app.Settings, v string) error {
			if v == "auto" {
				s.CLILanguage = ""
			} else {
				s.CLILanguage = v
			}
			return nil
		}},
	{Name: "github-mirror", Section: "cli", Kind: KindMirror, Def: "auto", Enum: append([]string{"auto"}, GithubMirrorPresets...),
		Usage: "GitHub 镜像站前缀 (下载内核/资源失败时依次尝试); auto=内置列表",
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
	{Name: "test-url", Section: "cli", Kind: KindURL, Def: TestURLPresets[0], Enum: TestURLPresets,
		Usage: "测速/连通性检测 URL (204 端点)",
		Get:   func(s *app.Settings) string { return s.TestURL },
		Set:   func(s *app.Settings, v string) error { s.TestURL = v; return nil }},
	{Name: "test-timeout", Section: "cli", Kind: KindInt, Def: "5000", Min: 100, Max: 60000,
		Comp:  []string{"1000", "3000", "5000", "10000"},
		Usage: "测速超时 (毫秒)",
		Get:   func(s *app.Settings) string { return strconv.Itoa(s.TestTimeout) },
		Set: func(s *app.Settings, v string) error {
			n, _ := strconv.Atoi(v)
			s.TestTimeout = n
			return nil
		}},
	{Name: "current-profile", Section: "cli", Kind: KindState,
		Usage: "当前生效订阅 (只读; 用 sub use 切换)",
		Get:   func(s *app.Settings) string { return orDash(s.CurrentProfile) }},
	{Name: "current-group", Section: "cli", Kind: KindState,
		Usage: "当前操作分组 (只读; 用 group use 切换)",
		Get:   func(s *app.Settings) string { return orDash(s.CurrentGroup) }},

	// ---- systemd timers ----
	{Name: "sub-auto-update-enabled", Section: "timer", Kind: KindBool, Def: "true",
		Usage: "订阅定时自动更新",
		Get:   func(s *app.Settings) string { return boolStr(s.SubAutoUpdateEnabled) },
		Set:   func(s *app.Settings, v string) error { s.SubAutoUpdateEnabled = v == "true"; return nil }},
	{Name: "sub-auto-update-interval", Section: "timer", Kind: KindDur, Def: "24h", Min: 60,
		Usage: "订阅自动更新周期",
		Get:   func(s *app.Settings) string { return durStr(s.SubAutoUpdateInterval) },
		Set: func(s *app.Settings, v string) error {
			d, err := parseDur(v)
			if err == nil {
				s.SubAutoUpdateInterval = d
			}
			return err
		}},
	{Name: "node-auto-select-enabled", Section: "timer", Kind: KindBool, Def: "false",
		Usage: "定时对当前分组自动择优",
		Get:   func(s *app.Settings) string { return boolStr(s.NodeAutoSelectEnabled) },
		Set:   func(s *app.Settings, v string) error { s.NodeAutoSelectEnabled = v == "true"; return nil }},
	{Name: "node-auto-select-interval", Section: "timer", Kind: KindDur, Def: "30m", Min: 60,
		Usage: "自动择优周期",
		Get:   func(s *app.Settings) string { return durStr(s.NodeAutoSelectInterval) },
		Set: func(s *app.Settings, v string) error {
			d, err := parseDur(v)
			if err == nil {
				s.NodeAutoSelectInterval = d
			}
			return err
		}},
	{Name: "resource-auto-update-enabled", Section: "timer", Kind: KindBool, Def: "false",
		Usage: "定时自动更新 geo 资源文件",
		Get:   func(s *app.Settings) string { return boolStr(s.ResourceAutoUpdateEnabled) },
		Set:   func(s *app.Settings, v string) error { s.ResourceAutoUpdateEnabled = v == "true"; return nil }},
	{Name: "resource-auto-update-interval", Section: "timer", Kind: KindDur, Def: "24h", Min: 3600,
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
	dnsKey("dns.enable", KindBool, "false", "启用 DNS 覆写 (覆盖订阅的 dns 配置)"),
	dnsKey("dns.ipv6", KindBool, "false", "DNS 解析 IPv6 结果 (AAAA)"),
	dnsKey("dns.enhanced-mode", KindEnum, "fake-ip", "DNS 增强模式"),
	dnsKey("dns.fake-ip", KindBool, "true", "启用 Fake-IP (enhanced-mode=fake-ip 时生效)"),
	dnsKey("dns.fake-ip-range", KindText, "28.0.0.1/8", "Fake-IP 地址段"),
	dnsKey("dns.use-system-hosts", KindBool, "true", "使用 /etc/hosts"),
	dnsKey("dns.listen", KindText, "0.0.0.0:53", "DNS 监听地址"),
	dnsKey("dns.fallback", KindNameList, "sub", "备用 DNS (解析国内域名)"),
	dnsKey("dns.default-nameserver", KindNameList, "223.5.5.5,119.29.29.29", "DNS 引导解析器 (必须是纯 IP)"),
	dnsKey("dns.nameserver", KindNameList, "sub", "DNS 服务器列表 (IP 或 DoH/DoT URL)"),

	// ---- tun 段 (点号路径; 开启需要 CAP_NET_ADMIN, 见 cmd 的 TUN 安全护栏) ----
	tunKey("tun.enable", KindBool, "false", "启用 TUN 透明代理 (需要 CAP_NET_ADMIN; 默认关闭)", true),
	tunKey("tun.stack", KindEnum, "mixed", "TUN 网络栈", true),
	tunKey("tun.device", KindText, "mihomo", "TUN 网卡名", true),
	tunKey("tun.mtu", KindInt, "9000", "TUN MTU", true),
	tunKey("tun.dns-hijack", KindList, "any:53,tcp://any:53", "DNS 劫持规则", true),
	tunKey("tun.auto-route", KindBool, "true", "自动配置路由表 (iptables/nftables)", true),
	tunKey("tun.auto-detect-interface", KindBool, "true", "自动检测出口网卡", true),
	tunKey("tun.strict-route", KindBool, "false", "严格路由 (防止流量绕过; android 生效)", true),
	tunKey("tun.route-exclude-address", KindCIDRList, "", "不进入 TUN 的网段 (务必包含 SSH 对端)", true),
	tunKey("tun.endpoint-independent-nat", KindBool, "true", "端点无关 NAT (提升 UDP 兼容性)", true),
	tunKey("tun.udp-timeout", KindInt, "300", "UDP 会话超时 (秒)", true),
	tunKey("tun.auto-redirect", KindBool, "true", "自动配置 iptables redirect (Linux)", true),
	tunKey("tun.iproute2-table-index", KindInt, "2022", "iproute2 路由表编号", true),
}

// tunEnum tun.stack 的可选值
var tunEnums = map[string][]string{
	"tun.stack": {"system", "gvisor", "mixed"},
}

// dnsEnum dns.enhanced-mode 的可选值
var dnsEnums = map[string][]string{
	"dns.enhanced-mode": {"fake-ip", "redir-host", "normal"},
}

// tunKey 构造 tun 段键 (存 [overrides])
func tunKey(name string, kind Kind, def, usage string, restart bool) Key {
	return Key{
		Name: name, Section: "tun", Kind: kind, Def: def, Enum: tunEnums[name],
		Usage: usage, Restart: restart,
		Get: func(s *app.Settings) string { return secGet(s, name) },
		Set: func(s *app.Settings, v string) error { return secSet(s, name, v, kind, tunEnums[name]) },
	}
}

// dnsKey 构造 dns 段键。dns.enable 与 dns.nameserver 额外挂到 DNSServers 字段,
// 使 dns use / dns on 与 config set dns.* 是同一套存储。
func dnsKey(name string, kind Kind, def, usage string) Key {
	base := Key{
		Name: name, Section: "dns", Kind: kind, Def: def, Enum: dnsEnums[name],
		Usage: usage,
		Get:   func(s *app.Settings) string { return secGet(s, name) },
		Set:   func(s *app.Settings, v string) error { return secSet(s, name, v, kind, dnsEnums[name]) },
	}
	switch name {
	case "dns.enable":
		base.Get = func(s *app.Settings) string {
			if len(s.DNSServers) > 0 || secGet(s, "dns.enable") == "true" {
				return "true"
			}
			if secGet(s, "dns.enable") == "false" {
				return "false"
			}
			return "false"
		}
		base.Set = func(s *app.Settings, v string) error {
			if v == "false" {
				s.DNSServers = nil
				return secDel(s, "dns.enable")
			}
			return secSet(s, "dns.enable", "true", KindBool, nil)
		}
	case "dns.nameserver":
		base.Get = func(s *app.Settings) string {
			if len(s.DNSServers) == 0 {
				return "sub"
			}
			return strings.Join(s.DNSServers, ",")
		}
		base.Set = func(s *app.Settings, v string) error {
			if v == "sub" {
				s.DNSServers = nil
				return nil
			}
			items := []string{}
			for _, it := range strings.Split(v, ",") {
				if it = strings.TrimSpace(it); it != "" {
					items = append(items, it)
				}
			}
			if len(items) == 0 {
				return fmt.Errorf("%s: %s", v, "ip|url,ip|url")
			}
			s.DNSServers = items
			return nil
		}
	}
	return base
}

// ---- Overrides (点号路径) 访问器 ----

// secGet 读 [overrides] 里的点号路径值; 不存在返回 ""
func secGet(s *app.Settings, dotted string) string {
	if s.Overrides == nil {
		return ""
	}
	parts := strings.Split(dotted, ".")
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
	parts := strings.Split(dotted, ".")
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
	parts := strings.Split(dotted, ".")
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
	m, ok := s.Overrides[section].(map[string]any)
	return ok && len(m) > 0
}

// SectionUsed 段是否被使用过 (对外)
func SectionUsed(s *app.Settings, section string) bool {
	if section == "dns" {
		return len(s.DNSServers) > 0 || secUsed(s, "dns")
	}
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
	parts := strings.Split(dotted, ".")
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

// UsedSections 已使用(非 core/cli/timer)的段名
func UsedSections(s *app.Settings) []string {
	var out []string
	for _, sec := range []string{"dns", "tun"} {
		if SectionUsed(s, sec) {
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
	m, _ := s.Overrides[section].(map[string]any)
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
