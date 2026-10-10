// Package cfg 配置注册表: 一个 key 一条记录, 驱动 get/set/补全/校验/单键帮助/同步。
//
// 设计动机 (见 .stepcode/plans): v1.2 及更早, 同一个配置项要在 configValue()/configDefaults/
// configKeys/set switch/configValueComp 五处平行维护, socks-port 就是漏改出过编译错, 补全也只覆盖 6/22 键。
// 这里把"键的元数据"收敛到唯一一张表, 其余全部推导。
//
// 两类键:
//   - 注册键 (Keys): 扁平键 (mixed-port) 与点号路径键 (tun.enable / dns.fake-ip-range)。
//     扁平键存 Settings 的字段; 点号路径键存 Settings.Overrides。
//   - 通用键 (未注册): config set <任意yaml路径> <值> 直接写进 Overrides, 值按字面推断类型。
//     这是"覆盖内核绝大多数常用功能"的兜底 —— 内核任何配置项都能读写, 不必为每个功能造子命令。
package cfg

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

// Kind 值的形态; 决定校验规则、补全候选与报错文案
type Kind int

const (
	KindBool     Kind = iota // true|false
	KindTri                  // true|false|sub (sub=跟随订阅)
	KindPort                 // off|sub|1..65535
	KindEnum                 // Enum 白名单
	KindDur                  // sub|>=30s
	KindInt                  // sub|<Min..Max>
	KindText                 // sub|任意非空串
	KindURL                  // http(s)://...
	KindMirror               // auto|http(s)://host
	KindNameList             // 逗号分隔的 IP 或 URL 列表
	KindCIDRList             // 逗号分隔的 CIDR/IP 列表
	KindList                 // 逗号分隔的任意 token 列表 (不校验内容)
	KindState                // 只读状态展示
)

// ValueDoc 一个取值的含义 (详版帮助用)
type ValueDoc struct {
	Name string // "rule" / "off" / "sub"
	Desc string // 中文说明 (en 由 i18n 查表)
}

// Key 一个配置项的完整描述
type Key struct {
	Name     string // "core.mixed-port" / "core.tun.enable"
	Section  string // core | cli | timer | dns | tun
	Kind     Kind
	Def      string                                      // 默认值 (展示与 reset-default); "" = 无默认值
	Enum     []string                                    // KindEnum/KindTri/KindPort 的合法值 (sub/off 由 Kind 决定)
	Usage    string                                      // 一句话说明 (简版帮助/列表)
	Detail   string                                      // 详细说明 (详版帮助; 空则用 Usage)
	Values   []ValueDoc                                  // 每个取值的含义 (详版帮助; 空则按 Kind 自动生成)
	Example  string                                      // 用法示例 (详版帮助)
	Resolve  func(*app.Settings, string) (string, error) // 值域扩展 (预设名→真实值), 在 Parse 之前跑
	Comp     []string                                    // 显式补全候选 (覆盖 Kind 的通用候选)
	CompFn   func() []string                             // 动态补全候选 (预设名由 cmd 包延迟注入)
	Min, Max int                                         // KindInt/KindDur 的取值范围 (Dur 用秒)
	Restart  bool                                        // true = 改动必须重启服务; false = reload/PATCH 即可
	Get      func(*app.Settings) string
	Set      func(*app.Settings, string) error
}

// Desc 本地化后的一句话说明
func (k Key) Desc() string { return i18n.T(k.Usage) }

// LongDesc 本地化后的详细说明 (没有单独写就用一句话说明)
func (k Key) LongDesc() string {
	if k.Detail != "" {
		return i18n.T(k.Detail)
	}
	return i18n.T(k.Usage)
}

// Spec 简版帮助里的紧凑取值形态 (长枚举截断, 完整列表只在详版)
func (k Key) Spec() string {
	switch k.Kind {
	case KindBool:
		return "true|false"
	case KindTri:
		return "true|false|sub"
	case KindPort:
		return "off|sub|port"
	case KindEnum:
		if len(k.Enum) <= 3 {
			return strings.Join(k.Enum, "|")
		}
		return strings.Join(k.Enum[:3], "|") + "|…"
	case KindDur:
		return fmt.Sprintf("%d-24h", k.Min/60)
	case KindInt:
		if k.Max > 0 {
			return fmt.Sprintf("%d-%d", k.Min, k.Max)
		}
		return "number"
	case KindURL:
		return "url"
	case KindMirror:
		return "auto|url"
	case KindNameList:
		return "ip|url,ip|url"
	case KindCIDRList:
		return "cidr,cidr"
	case KindList:
		return "item,item"
	case KindState:
		return ""
	}
	return "value"
}

// ValueDocs 取值的含义; 通用取值(true/false/off/sub/auto)按 Kind 自动生成,
// 只有"取值含义差异大"的键才手写 (proxy-mode/log-level/enhanced-mode/stack…)
func (k Key) ValueDocs() []ValueDoc {
	if len(k.Values) > 0 {
		out := make([]ValueDoc, len(k.Values))
		for i, v := range k.Values {
			out[i] = ValueDoc{Name: i18n.T(v.Name), Desc: i18n.T(v.Desc)}
		}
		return out
	}
	var out []ValueDoc
	switch k.Kind {
	case KindBool:
		out = []ValueDoc{{"true", i18n.T("开启")}, {"false", i18n.T("关闭")}}
	case KindTri:
		out = []ValueDoc{{"true", i18n.T("开启")}, {"false", i18n.T("关闭")},
			{"sub", i18n.T("跟随订阅: 不注入, 用订阅/内核的值")}}
	case KindPort:
		out = []ValueDoc{{"off", i18n.T("关闭该端口, 不监听 (输入 0/none/- 也可以)")},
			{"sub", i18n.T("跟随订阅: 订阅写了才监听")}, {i18n.T("<端口号>"), i18n.T("监听指定端口 (1-65535)")}}
	case KindMirror:
		out = []ValueDoc{{"auto", i18n.T("自动: 依次尝试内置镜像列表")},
			{i18n.T("<url>"), i18n.T("固定使用该镜像站前缀, 如 https://ghfast.top")}}
	}
	return out
}

// Managed 该键是否由 CLI 全权管理 (扁平键: 配置里没有就写入默认值, 永远注入内核 yaml)
// 点号路径段 (dns/tun) 相反: 没被 set 过就完全不写, 订阅/内核原样保留
func (k Key) Managed() bool {
	switch k.Section {
	case "core.dns", "core.tun":
		return false
	}
	return true
}

// Effective 有效值: Managed 键的空值回退默认值; 段键的空值就是"未接管"
func (k Key) Effective(s *app.Settings) string {
	v := k.Get(s)
	if v == "" && k.Managed() && k.Def != "" {
		return k.Def
	}
	return v
}

// Top 段名 (点号路径的第一段); 无点号返回 ""
func (k Key) Top() string {
	if i := strings.Index(k.Name, "."); i > 0 {
		return k.Name[:i]
	}
	return ""
}

// SectionOrder 段展示顺序
var SectionOrder = []string{"core", "cli", "timer", "core.dns", "core.tun"}

// SectionTitles 段标题 (展示用; 跟随语言)
func SectionTitles(section string) string {
	switch section {
	case "core":
		return i18n.T("core config")
	case "cli":
		return i18n.T("cli")
	case "timer":
		return i18n.T("systemd timers")
	case "core.dns":
		return i18n.T("dns")
	case "core.tun":
		return i18n.T("tun")
	}
	return section
}

// Sections 返回已注册的所有段名 (按展示顺序)
func Sections() []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range Keys {
		if !seen[k.Section] {
			seen[k.Section] = true
			out = append(out, k.Section)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return idx(SectionOrder, out[i]) < idx(SectionOrder, out[j])
	})
	return out
}

func idx(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return len(list)
}

// KeysOf 某一段的键 (含顺序)
func KeysOf(section string) []Key {
	var out []Key
	for _, k := range Keys {
		if k.Section == section {
			out = append(out, k)
		}
	}
	return out
}

// Lookup 按名字找注册键; 找不到返回 nil
func Lookup(name string) *Key {
	for i := range Keys {
		if Keys[i].Name == name {
			return &Keys[i]
		}
	}
	return nil
}

// Names 全部注册键名 (补全用)
func Names() []string {
	out := make([]string, 0, len(Keys))
	for _, k := range Keys {
		out = append(out, k.Name)
	}
	return out
}

// SectionNames 段名 (补全用): 已注册的段 + 通用段提示
func SectionNames() []string { return Sections() }

// CompleteKeys key 位置的补全: 已注册键 + 段名 (前缀过滤)
func CompleteKeys(prefix string) []string {
	var out []string
	for _, k := range Keys {
		if strings.HasPrefix(k.Name, prefix) {
			out = append(out, k.Name)
		}
	}
	for _, s := range Sections() {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	// 段内键补全: "tun." → tun.enable / tun.stack …
	if i := strings.Index(prefix, "."); i > 0 {
		sec := prefix[:i+1]
		for _, k := range Keys {
			if strings.HasPrefix(k.Name, sec) && strings.HasPrefix(k.Name, prefix) {
				out = append(out, k.Name)
			}
		}
	}
	return dedup(out)
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ---- 值解析与校验 (L1 形态校验) ----

// Parse 校验并归一化一个值; 返回归一化后的值 (用于展示) 与错误
func (k Key) Parse(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s: %s", i18n.T("值不能为空"), k.Expected())
	}
	switch k.Kind {
	case KindBool:
		return canonBool(v, k)
	case KindTri:
		if v == "sub" {
			return "sub", nil
		}
		return canonBool(v, k)
	case KindPort:
		return canonPort(v, k)
	case KindEnum:
		if v == "sub" { // sub = 跟随订阅/内核默认, 与三态键同一语义
			return "sub", nil
		}
		if !k.hasEnum(v) {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		return v, nil
	case KindDur:
		if v == "sub" {
			return "sub", nil
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		sec := int(d.Seconds())
		if sec < k.Min || (k.Max > 0 && sec > k.Max) {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		return DurHuman(d), nil
	case KindInt:
		if v == "sub" {
			return "sub", nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		if n < k.Min || (k.Max > 0 && n > k.Max) {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		return strconv.Itoa(n), nil
	case KindText:
		if v == "sub" {
			return "sub", nil
		}
		return v, nil
	case KindURL:
		return canonURL(v, k)
	case KindMirror:
		if v == "auto" {
			return "auto", nil
		}
		return canonURL(v, k)
	case KindNameList, KindCIDRList:
		if v == "sub" { // 段键的"跟随订阅"
			return "sub", nil
		}
		return canonList(v, k)
	case KindList:
		items := []string{}
		for _, it := range strings.Split(v, ",") {
			if it = strings.TrimSpace(it); it != "" {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			return "", fmt.Errorf("%s: %s", v, k.Expected())
		}
		return strings.Join(items, ","), nil
	}
	return v, nil
}

func canonBool(v string, k Key) (string, error) {
	switch strings.ToLower(v) {
	case "true", "on", "yes", "1":
		return "true", nil
	case "false", "off", "no", "0":
		return "false", nil
	}
	return "", fmt.Errorf("%s: %s", v, k.Expected())
}

func canonPort(v string, k Key) (string, error) {
	switch strings.ToLower(v) {
	case "sub":
		return "sub", nil
	case "off", "none", "-", "0":
		return "off", nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("%s: %s", v, k.Expected())
	}
	return strconv.Itoa(n), nil
}

func canonURL(v string, k Key) (string, error) {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s: %s", v, k.Expected())
	}
	return v, nil
}

func canonList(v string, k Key) (string, error) {
	items := []string{}
	for _, it := range strings.Split(v, ",") {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if k.Kind == KindNameList {
			if net.ParseIP(it) == nil && !strings.HasPrefix(it, "http://") && !strings.HasPrefix(it, "https://") {
				return "", fmt.Errorf("%s: %s", v, k.Expected())
			}
		} else {
			if _, _, err := net.ParseCIDR(it); err != nil && net.ParseIP(it) == nil {
				return "", fmt.Errorf("%s: %s", v, k.Expected())
			}
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("%s: %s", v, k.Expected())
	}
	return strings.Join(items, ","), nil
}

func (k Key) hasEnum(v string) bool {
	for _, e := range k.Enum {
		if e == v {
			return true
		}
	}
	return false
}

// Expected 期望值的人类可读描述
func (k Key) Expected() string {
	switch k.Kind {
	case KindBool:
		return "true|false"
	case KindTri:
		return "true|false|sub"
	case KindPort:
		return "off|sub|1-65535"
	case KindEnum:
		return strings.Join(k.Enum, "|")
	case KindDur:
		return fmt.Sprintf("30s-24h (>=%ds)", k.Min)
	case KindInt:
		return fmt.Sprintf("%d-%d", k.Min, k.Max)
	case KindText:
		return "non-empty text"
	case KindURL:
		return "http(s)://host/path"
	case KindMirror:
		return "auto|http(s)://host"
	case KindNameList:
		return "ip|url,ip|url"
	case KindCIDRList:
		return "cidr,cidr"
	}
	return "value"
}

// CompleteValues 值位置补全 (TAB)
func (k Key) CompleteValues() []string {
	if k.CompFn != nil {
		return k.CompFn()
	}
	if len(k.Comp) > 0 {
		return k.Comp
	}
	switch k.Kind {
	case KindBool:
		return []string{"true", "false"}
	case KindTri:
		return []string{"true", "false", "sub"}
	case KindNameList, KindCIDRList, KindList:
		return []string{"sub"} // 段键: sub=跟随订阅; 具体值没有通用候选
	case KindPort:
		return []string{"off", "sub", "1080", "7890", "7891", "7892", "8080"}
	case KindEnum:
		return k.Enum
	case KindDur:
		return durCandidates(k)
	case KindInt:
		if k.Max > 2000 {
			return []string{"1000", "1500", "9000"}
		}
		return nil
	case KindURL:
		return TestURLPresets
	case KindMirror:
		return append([]string{"auto"}, GithubMirrorPresets...)
	}
	return nil
}

// durCandidates 时长键的补全候选, 过滤掉超出 Min/Max 的
func durCandidates(k Key) []string {
	var out []string
	for _, v := range []string{"1m", "5m", "10m", "30m", "1h", "2h", "6h", "12h", "24h"} {
		d, err := time.ParseDuration(v)
		if err != nil {
			continue
		}
		if int(d.Seconds()) < k.Min {
			continue
		}
		if k.Max > 0 && int(d.Seconds()) > k.Max {
			continue
		}
		out = append(out, v)
	}
	return out
}

// DurCountdown 倒计时显示: 一律截断到分钟 (用户 v0.6 明确要求"23h40m"而不是"23h40m0s");
// 与 DurHuman 的区别就在这里 —— 倒计时不需要秒级精度, 配置值需要。
func DurCountdown(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "<1m"
	}
	s := int(d.Minutes()) * 60
	switch {
	case s%3600 == 0:
		return fmt.Sprintf("%dh", s/3600)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	default:
		return fmt.Sprintf("%dh%dm", s/3600, (s%3600)/60)
	}
}

// DurHuman 紧凑时长: 不四舍五入, 只把整量的部分缩短
// (24h / 12h / 30m / 1m30s / 45s —— 绝不把 90s 显示成 1m 这种有损写法)
func DurHuman(d time.Duration) string {
	s := int(d.Seconds())
	switch {
	case s > 0 && s%3600 == 0:
		return fmt.Sprintf("%dh", s/3600)
	case s >= 60 && s%60 == 0:
		if h := s / 3600; h > 0 {
			return fmt.Sprintf("%dh%dm", h, (s%3600)/60)
		}
		return fmt.Sprintf("%dm", s/60)
	case s >= 0 && s < 60:
		return fmt.Sprintf("%ds", s)
	default:
		return d.String()
	}
}
