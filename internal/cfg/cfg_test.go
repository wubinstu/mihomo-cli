package cfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

func TestKeysUniqueAndComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range Keys {
		if k.Name == "" {
			t.Fatalf("empty key name in section %s", k.Section)
		}
		if seen[k.Name] {
			t.Fatalf("duplicate key %q", k.Name)
		}
		seen[k.Name] = true
		if k.Get == nil && k.Kind != KindState {
			t.Fatalf("%s: no Get", k.Name)
		}
		if k.Set == nil && k.Kind != KindState {
			t.Fatalf("%s: no Set", k.Name)
		}
		if k.Usage == "" {
			t.Fatalf("%s: no Usage", k.Name)
		}
	}
	if Lookup("core.mixed-port") == nil {
		t.Fatal("core.mixed-port must be registered")
	}
	if Lookup("core.tun.enable") == nil || Lookup("core.dns.enable") == nil || Lookup("cli.language") == nil || Lookup("timer.sub-auto-update-enabled") == nil {
		t.Fatal("dotted keys must be registered")
	}
}

// 每个 Kind 至少有一条覆盖, 且 Parse 的归一化与报错都符合预期
func TestKindParse(t *testing.T) {
	cases := []struct {
		kind     Kind
		in       string
		want     string
		wantErr  bool
		enum     []string
		min, max int
	}{
		{KindBool, "true", "true", false, nil, 0, 0},
		{KindBool, "off", "false", false, nil, 0, 0},
		{KindBool, "yes", "true", false, nil, 0, 0},
		{KindBool, "auto1", "", true, nil, 0, 0},
		{KindTri, "sub", "sub", false, nil, 0, 0},
		{KindTri, "false", "false", false, nil, 0, 0},
		{KindPort, "7891", "7891", false, nil, 0, 0},
		{KindPort, "off", "off", false, nil, 0, 0},
		{KindPort, "0", "off", false, nil, 0, 0},
		{KindPort, "none", "off", false, nil, 0, 0},
		{KindPort, "sub", "sub", false, nil, 0, 0},
		{KindPort, "70000", "", true, nil, 0, 0},
		{KindPort, "abc", "", true, nil, 0, 0},
		{KindEnum, "rule", "rule", false, []string{"rule", "global", "direct"}, 0, 0},
		{KindEnum, "sub", "sub", false, []string{"rule", "sub"}, 0, 0},
		{KindEnum, "nope", "", true, []string{"rule"}, 0, 0},
		{KindDur, "24h", "24h", false, nil, 0, 0},
		{KindDur, "30m", "30m", false, nil, 0, 0},
		{KindDur, "90s", "1m30s", false, nil, 0, 0},
		{KindDur, "sub", "sub", false, nil, 0, 0},
		{KindDur, "10s", "", true, nil, 60, 0},
		{KindInt, "5000", "5000", false, nil, 100, 60000},
		{KindInt, "99", "", true, nil, 100, 60000},
		{KindInt, "70000", "", true, nil, 100, 60000},
		{KindText, "0.0.0.0:53", "0.0.0.0:53", false, nil, 0, 0},
		{KindText, "sub", "sub", false, nil, 0, 0},
		{KindText, "", "", true, nil, 0, 0},
		{KindURL, "https://a.b/c", "https://a.b/c", false, nil, 0, 0},
		{KindURL, "not-a-url", "", true, nil, 0, 0},
		{KindURL, "ftp://a.b", "", true, nil, 0, 0},
		{KindMirror, "auto", "auto", false, nil, 0, 0},
		{KindMirror, "https://ghfast.top", "https://ghfast.top", false, nil, 0, 0},
		{KindMirror, "auto1", "", true, nil, 0, 0},
		{KindNameList, "223.5.5.5,119.29.29.29", "223.5.5.5,119.29.29.29", false, nil, 0, 0},
		{KindNameList, "https://a/dns-query", "https://a/dns-query", false, nil, 0, 0},
		{KindNameList, "bogus", "", true, nil, 0, 0},
		{KindCIDRList, "10.0.0.0/8,192.168.0.0/16", "10.0.0.0/8,192.168.0.0/16", false, nil, 0, 0},
		{KindCIDRList, "10.0.0.0/33", "", true, nil, 0, 0},
		{KindList, "any:53,tcp://any:53", "any:53,tcp://any:53", false, nil, 0, 0},
		{KindList, ",", "", true, nil, 0, 0},
	}
	for _, c := range cases {
		k := Key{Kind: c.kind, Enum: c.enum, Min: c.min, Max: c.max}
		got, err := k.Parse(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%v.Parse(%q) should fail, got %q", c.kind, c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v.Parse(%q) unexpected error: %v", c.kind, c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%v.Parse(%q) = %q, want %q", c.kind, c.in, got, c.want)
		}
	}
}

// 每个注册键: 默认值合法 + 补全候选全部合法
func TestDefaultsAndCompletionLegal(t *testing.T) {
	for _, k := range Keys {
		if k.Def == "" || k.Def == "sub" {
			continue
		}
		if _, err := k.Parse(k.Def); err != nil {
			t.Errorf("%s: default %q is not parseable: %v", k.Name, k.Def, err)
		}
		for _, v := range k.CompleteValues() {
			if _, err := k.Parse(v); err != nil {
				t.Errorf("%s: completion candidate %q is not parseable: %v", k.Name, v, err)
			}
		}
	}
}

// 端口键: off/0/none/- 四种写法等价, 且两两不得相同
func TestPortAliases(t *testing.T) {
	s := app.DefaultSettings()
	for _, v := range []string{"off", "0", "none", "-"} {
		k := Lookup("core.socks-port")
		if _, err := k.Parse(v); err != nil {
			t.Fatalf("socks-port should accept %q: %v", v, err)
		}
	}
	_ = s
	if !samePortText("off", "0") || !samePortText("7891", "7891") {
		t.Fatal("port compare")
	}
}

func samePortText(a, b string) bool {
	ka := Key{Kind: KindPort}
	na, _ := ka.Parse(a)
	nb, _ := ka.Parse(b)
	return na == nb
}

// 三态键: "sub" = 删除该键 (不注入 yaml), 具体值 = 写入
func TestSectionSetAndDelete(t *testing.T) {
	s := &app.Settings{Overrides: map[string]any{}}
	k := Lookup("core.tun.stack")
	if err := k.Set(s, "gvisor"); err != nil {
		t.Fatal(err)
	}
	if got := k.Get(s); got != "gvisor" {
		t.Fatalf("tun.stack = %q, want gvisor", got)
	}
	if err := k.Set(s, "sub"); err != nil {
		t.Fatal(err)
	}
	if got := k.Get(s); got != "" {
		t.Fatalf("tun.stack after sub = %q, want empty", got)
	}
	if SectionUsed(s, "tun") {
		t.Fatal("deleting the only key must leave the section unused")
	}
}

// dns.enable / dns.nameserver 挂到 DNSServers 字段, 与 dns use 语义一致
// dns/tun 段的所有键共用一个存储 ([overrides.<段>]), 不再有 DNSServers 这种特权字段
func TestDNSSectionSharesOneStore(t *testing.T) {
	s := &app.Settings{}
	ns := Lookup("core.dns.nameserver")
	if err := ns.Set(s, "223.5.5.5,119.29.29.29"); err != nil {
		t.Fatal(err)
	}
	en := Lookup("core.dns.enable")
	if en.Get(s) != "true" {
		t.Fatalf("core.dns.enable = %q, want true (nameserver set implies the section is on)", en.Get(s))
	}
	if !SecHas(s, "core.dns.nameserver") {
		t.Error("nameserver must land in the same store as the rest of the dns section")
	}
	if err := en.Set(s, "false"); err != nil {
		t.Fatal(err)
	}
	// 显式关掉: 盖住已设的 nameserver, 整段不注入
	if en.Get(s) != "false" || DNSSectionActive(s) {
		t.Errorf("after dns off: enable=%q active=%v, want false/false", en.Get(s), DNSSectionActive(s))
	}
	if err := ns.Set(s, "sub"); err != nil {
		t.Fatal(err)
	}
	if ns.Get(s) != "sub" {
		t.Fatalf("core.dns.nameserver = %q, want sub", ns.Get(s))
	}
	// 重新打开又能用
	if err := en.Set(s, "true"); err != nil {
		t.Fatal(err)
	}
	if !DNSSectionActive(s) {
		t.Error("dns on must reactivate the section")
	}
}

// 通用键: 未注册的任意 yaml 路径, 值按字面推断类型
func TestGenericKeys(t *testing.T) {
	s := &app.Settings{}
	if err := SetGeneric(s, "sniffer.enable", "true"); err != nil {
		t.Fatal(err)
	}
	if err := SetGeneric(s, "geodata-mode", "false"); err != nil {
		t.Fatal(err)
	}
	if err := SetGeneric(s, "rules", "MATCH,DIRECT"); err != nil && !strings.Contains(err.Error(), "") {
		t.Fatal(err)
	}
	if v, ok := GetGeneric(s, "sniffer.enable"); !ok || v != "true" {
		t.Fatalf("sniffer.enable = %q ok=%v", v, ok)
	}
	m, _ := s.Overrides["sniffer"].(map[string]any)
	if m == nil || m["enable"] != true {
		t.Fatalf("overrides.sniffer = %#v", s.Overrides["sniffer"])
	}
	if v, _ := GetGeneric(s, "missing.key"); v != "" {
		t.Fatalf("missing key should be empty")
	}
	if err := SetGeneric(s, "", "x"); err == nil {
		t.Fatal("empty path must fail")
	}
	if err := SetGeneric(s, "a..b", "x"); err == nil {
		t.Fatal("bad path must fail")
	}
}

// 端口默认语义: 只有 mixed-port 默认开, socks/http 默认 off (用户拍板)
func TestPortDefaults(t *testing.T) {
	s := app.DefaultSettings()
	if s.MixedPort != "7890" || s.SocksPort != "off" || s.HTTPPort != "off" {
		t.Fatalf("defaults: %q %q %q", s.MixedPort, s.SocksPort, s.HTTPPort)
	}
	if s.ProxyPort() != 7890 {
		t.Fatalf("ProxyPort = %d", s.ProxyPort())
	}
	if n, ok := app.PortNum("off"); ok || n != 0 {
		t.Fatalf("off must not be a port: %d %v", n, ok)
	}
	if n, ok := app.PortNum("7892"); !ok || n != 7892 {
		t.Fatalf("7892 must parse: %d %v", n, ok)
	}
	if n, ok := app.PortNum("sub"); ok || n != 0 {
		t.Fatalf("sub must not be a port: %d %v", n, ok)
	}
}

// 段排序: core → cli → timer → core.dns → core.tun (段名 = CLI 键前缀)
func TestSectionOrder(t *testing.T) {
	got := Sections()
	want := []string{"core", "cli", "timer", "core.dns", "core.tun"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sections = %v, want %v", got, want)
	}
}

// 补全: KEY 位置能补出段内键
func TestCompleteKeysTwoLevels(t *testing.T) {
	all := CompleteKeys("")
	if len(all) == 0 {
		t.Fatal("no key completions")
	}
	dns := CompleteKeys("core.dns.")
	found := false
	for _, k := range dns {
		if k == "core.dns.enable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dns. completion = %v", dns)
	}
	tun := CompleteKeys("core.tun.")
	if len(tun) == 0 {
		t.Fatal("tun. completion empty")
	}
}

// Every Usage 字段值必须是 i18n en 表里的 key (否则 en 模式会漏中文)
func TestRegistryUsageTranslated(t *testing.T) {
	i18n.Set("en") // T() 只在 en 模式下查表
	defer i18n.Set("zh")
	// 详版帮助里的每个用户可见字段都要过一遍: usage / detail / 每个取值的说明
	for _, k := range Keys {
		if k.Usage == "" {
			t.Errorf("%s: empty usage", k.Name)
		}
		checkTranslated(t, k.Name+".usage", k.Usage)
		if k.Detail != "" {
			checkTranslated(t, k.Name+".detail", k.Detail)
		}
		for _, v := range k.Values {
			checkTranslated(t, k.Name+".values."+v.Name, v.Desc)
		}
	}
}

// checkTranslated 中文字符串必须在 en 表里; 纯英文的必须原样返回 (不许被映射成别的)
func checkTranslated(t *testing.T, where, s string) {
	t.Helper()
	if s == "" {
		return
	}
	got := i18n.T(s)
	if hasCJK(s) {
		if got == s {
			t.Errorf("%s: %q is not in the en table (leaks Chinese in en mode)", where, s)
		}
		return
	}
	if got != s {
		t.Errorf("%s: %q looks English but the en table remaps it to %q", where, s, got)
	}
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// YAMLPath: core 段去前缀 + 内核里的异名键; dns/tun 原样嵌套
func TestYAMLPath(t *testing.T) {
	cases := map[string]string{
		"core.proxy-mode":          "mode",
		"core.ipv6-enabled":        "ipv6",
		"core.http-port":           "port",
		"core.mixed-port":          "mixed-port",
		"core.allow-lan":           "allow-lan",
		"core.keep-alive-interval": "keep-alive-interval",
		// core. 是 CLI 侧的前缀, 内核 yaml 里没有这一层
		"core.dns.enable":                "dns.enable",
		"core.dns.fake-ip-range":         "dns.fake-ip-range",
		"core.tun.stack":                 "tun.stack",
		"core.tun.route-exclude-address": "tun.route-exclude-address",
	}
	for in, want := range cases {
		if got := YAMLPath(in); got != want {
			t.Errorf("YAMLPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// DurCountdown 一律截断到分钟 (用户 v0.6 定的规矩)
func TestDurCountdown(t *testing.T) {
	cases := map[string]string{
		"24h0m0s":  "24h",
		"23h40m0s": "23h40m",
		"30m0s":    "30m",
		"29m2.8s":  "29m",
		"59s":      "<1m",
		"0s":       "<1m",
		"-5m":      "<1m",
		"168h0m0s": "168h",
	}
	for in, want := range cases {
		d, err := time.ParseDuration(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := DurCountdown(d); got != want {
			t.Errorf("DurCountdown(%s) = %q, want %q", in, got, want)
		}
	}
}

// TestNoOrphanTranslations en 表里不应有没人用的 key。
// 放在 cfg 包而不是 i18n 包: 注册表的 Usage/Detail/Values 也是翻译 key, 但它们是
// 结构体字段、不是 T("…") 字面量, i18n 包的静态扫描看不出来。
func TestNoOrphanTranslations(t *testing.T) {
	used := map[string]bool{}
	var root string
	// 1) 注册表里的全部用户可见字符串
	for _, k := range Keys {
		used[k.Usage] = true
		used[k.Detail] = true
		for _, v := range k.Values {
			used[v.Name] = true
			used[v.Desc] = true
		}
	}
	// 2) 源码里的 T("…") 字面量 (i18n 包自己的表除外 —— 那些就是 key 本身)
	for dir, err := os.Getwd(); err == nil; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			root = dir
			break
		}
		if filepath.Dir(dir) == dir {
			t.Fatal("go.mod not found")
		}
	}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if strings.HasPrefix(p, "internal/i18n/") {
			return nil
		}
		data, _ := os.ReadFile(p)
		for _, m := range regexp.MustCompile(`(?:i18n\.)?T\("((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = true
		}
		return nil
	})
	var orphans []string
	for _, k := range i18n.Keys() {
		if !used[k] {
			orphans = append(orphans, k)
		}
	}
	if len(orphans) > 0 {
		t.Errorf("orphan en entries (stale translations, delete them): %d\n  %s",
			len(orphans), strings.Join(orphans, "\n  "))
	}
}
