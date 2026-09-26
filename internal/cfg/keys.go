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

// Key 一个配置项的完整描述
type Key struct {
	Name     string // "mixed-port" / "tun.enable"
	Section  string // core | cli | timer | dns | tun
	Kind     Kind
	Def      string   // 默认值 (展示与 reset-default); "" = 无默认值
	Enum     []string // KindEnum/KindTri/KindPort 的合法值 (sub/off 由 Kind 决定)
	Usage    string   // 中文说明 (en 由 i18n 查表)
	Comp     []string // 显式补全候选 (覆盖 Kind 的通用候选)
	Min, Max int      // KindInt/KindDur 的取值范围 (Dur 用秒)
	Restart  bool     // true = 改动必须重启服务; false = reload/PATCH 即可
	Get      func(*app.Settings) string
	Set      func(*app.Settings, string) error
}

// Desc 本地化后的说明
func (k Key) Desc() string { return i18n.T(k.Usage) }

// Managed 该键是否由 CLI 全权管理 (扁平键: 配置里没有就写入默认值, 永远注入内核 yaml)
// 点号路径段 (dns/tun) 相反: 没被 set 过就完全不写, 订阅/内核原样保留
func (k Key) Managed() bool {
	switch k.Section {
	case "dns", "tun":
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
var SectionOrder = []string{"core", "cli", "timer", "dns", "tun"}

// SectionTitles 段标题 (展示用)
func SectionTitles(section string) string {
	switch section {
	case "core":
		return "core config"
	case "cli":
		return "cli"
	case "timer":
		return "systemd timers"
	case "dns":
		return "dns"
	case "tun":
		return "tun"
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
	if len(k.Comp) > 0 {
		return k.Comp
	}
	switch k.Kind {
	case KindBool:
		return []string{"true", "false"}
	case KindTri:
		return []string{"true", "false", "sub"}
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

// DurHuman 紧凑时长: 不四舍五入, 只把整量的部分缩短
// (24h / 12h / 30m / 1m30s / 45s —— 绝不把 90s 显示成 1m 这种有损写法)
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
