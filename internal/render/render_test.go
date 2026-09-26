package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/subs"
)

// setup 准备一个临时数据目录 (不碰 /etc/mihomo-cli)
func setup(t *testing.T) *app.Settings {
	t.Helper()
	dir := t.TempDir()
	app.BaseDir = dir
	app.BinDir = filepath.Join(dir, "bin")
	app.ProfileDir = filepath.Join(dir, "profiles")
	app.RuntimeDir = filepath.Join(dir, "runtime")
	app.VersionsDir = filepath.Join(dir, "bin", "versions")
	app.LogDir = filepath.Join(dir, "logs")
	app.CoreBin = filepath.Join(app.BinDir, "mihomo")
	app.CoreBinOld = filepath.Join(app.BinDir, "mihomo.old")
	app.SettingsFile = filepath.Join(dir, "config.toml")
	app.OverridesFile = filepath.Join(dir, "overrides.yaml")
	app.RuntimeConfig = filepath.Join(app.RuntimeDir, "config.yaml")
	app.LogFile = filepath.Join(app.LogDir, "mihomo.log")

	if err := os.MkdirAll(app.ProfileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := `proxies:
  - name: "A"
    type: ss
    server: 1.2.3.4
    port: 8388
rules:
  - MATCH,DIRECT
dns:
  enable: false
  nameserver:
    - 8.8.8.8
`
	if err := os.WriteFile(subs.Path("t"), []byte(sub), 0o644); err != nil {
		t.Fatal(err)
	}
	s := app.DefaultSettings()
	s.APISecret = "test-secret"
	s.Profiles = []app.Profile{{Name: "t", URL: "https://x.example/sub", Nodes: 1}}
	s.CurrentProfile = "t"
	return s
}

// read 读回 runtime/config.yaml
func read(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(app.RuntimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// 十个核心键: 默认值必须真的写进内核 yaml (用户要求"配置里没有就写入默认值")
func TestGenerateWritesDefaults(t *testing.T) {
	s := setup(t)
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	m := read(t)
	want := map[string]any{
		"mixed-port":          7890,
		"allow-lan":           false,
		"mode":                "rule",
		"log-level":           "info",
		"tcp-concurrent":      true,
		"unified-delay":       true,
		"keep-alive-interval": 30,
		"external-controller": "127.0.0.1:9090",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %#v, want %#v", k, m[k], v)
		}
	}
	// socks/http 默认 off → 不得出现
	if _, ok := m["socks-port"]; ok {
		t.Error("socks-port must be absent when off")
	}
	if _, ok := m["port"]; ok {
		t.Error("port must be absent when off")
	}
	if _, ok := m["redir-port"]; ok {
		t.Error("redir-port must be removed")
	}
	// secret 由 cli 注入
	if m["secret"] == nil || m["secret"] == "" {
		t.Error("secret must be injected")
	}
}

// sub 值 → 不注入 (跟随订阅)
func TestGenerateSubFollowsSubscription(t *testing.T) {
	s := setup(t)
	s.ProxyMode = "sub"
	s.TCPConcurrent = "sub"
	s.LogLevel = "sub"
	s.KeepAliveInterval = "sub"
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	m := read(t)
	if _, ok := m["mode"]; ok {
		t.Error("mode must not be injected when sub")
	}
	if _, ok := m["tcp-concurrent"]; ok {
		t.Error("tcp-concurrent must not be injected when sub")
	}
	if _, ok := m["keep-alive-interval"]; ok {
		t.Error("keep-alive-interval must not be injected when sub")
	}
	if _, ok := m["socks-port"]; ok {
		t.Error("socks-port must not be injected when sub")
	}
}

// 端口开启时要会写进去; 三个端口写哪个键
func TestGeneratePorts(t *testing.T) {
	s := setup(t)
	s.SocksPort = "7891"
	s.HTTPPort = "7892"
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	m := read(t)
	if m["socks-port"] != 7891 {
		t.Errorf("socks-port = %#v", m["socks-port"])
	}
	if m["port"] != 7892 { // http-port 在内核里叫 port
		t.Errorf("port = %#v", m["port"])
	}
}

// 点号路径: 只用过的段才写, 没用过就一点都不碰订阅
func TestGenerateDottedSectionsOnlyWhenUsed(t *testing.T) {
	s := setup(t)
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	if _, ok := read(t)["tun"]; ok {
		t.Error("tun must be absent when unused")
	}
	if err := cfg.SetGeneric(s, "tun.enable", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetGeneric(s, "tun.mtu", "1500"); err != nil {
		t.Fatal(err)
	}
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	tun, ok := read(t)["tun"].(map[string]any)
	if !ok {
		t.Fatal("tun section missing after set")
	}
	if tun["enable"] != true || tun["mtu"] != 1500 {
		t.Errorf("tun = %#v", tun)
	}
}

// 用户规则要插在订阅规则之前 (首条匹配即生效)
func TestGenerateUserRulesFirst(t *testing.T) {
	s := setup(t)
	s.UserRules = []app.UserRule{{Type: "DOMAIN-SUFFIX", Condition: "openai.com", Strategy: "DIRECT", Enabled: true}}
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	rules, _ := read(t)["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("rules = %#v", rules)
	}
	if rules[0] != "DOMAIN-SUFFIX,openai.com,DIRECT" {
		t.Errorf("user rule must come first, got %#v", rules[0])
	}
	// 停用的规则不注入
	s.UserRules[0].Enabled = false
	_ = Generate(s)
	rules, _ = read(t)["rules"].([]any)
	if len(rules) != 1 {
		t.Errorf("disabled rule must not be injected: %#v", rules)
	}
}

// 无生效订阅: 最小配置 (mode direct), 服务保持运行
func TestGenerateMinimalWhenDangling(t *testing.T) {
	s := setup(t)
	s.CurrentProfile = ""
	if err := Generate(s); err != nil {
		t.Fatal(err)
	}
	m := read(t)
	if m["mode"] != "direct" {
		t.Errorf("mode = %#v, want direct", m["mode"])
	}
	if _, ok := m["proxies"]; ok {
		t.Error("minimal config must not carry proxies")
	}
	if m["mixed-port"] != 7890 {
		t.Errorf("mixed-port = %#v", m["mixed-port"])
	}
}

// DeepMerge 递归合并 map, 其余覆盖
func TestDeepMerge(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"x": 1, "y": 2}, "b": 1}
	DeepMerge(dst, map[string]any{"a": map[string]any{"y": 3, "z": 4}, "c": 5})
	a := dst["a"].(map[string]any)
	if a["x"] != 1 || a["y"] != 3 || a["z"] != 4 {
		t.Errorf("deep merge lost keys: %#v", a)
	}
	if dst["b"] != 1 || dst["c"] != 5 {
		t.Errorf("top-level merge wrong: %#v", dst)
	}
}

// overrides.yaml 收编进 config.toml [overrides]
func TestLegacyOverridesMigrated(t *testing.T) {
	s := setup(t)
	if err := s.Save(); err != nil { // 先造出 config.toml, 否则 LoadSettings 走"文件不存在"分支
		t.Fatal(err)
	}
	if err := os.WriteFile(app.OverridesFile, []byte("tun:\n  enable: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := app.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Overrides) == 0 {
		t.Fatal("overrides.yaml should be migrated into config.toml")
	}
	if _, statErr := os.Stat(app.OverridesFile); statErr == nil {
		t.Error("legacy overrides.yaml should be renamed away after migration")
	}
}

// 端口字符串别名: off/0/none/- 都等于不监听
func TestPortNumAliases(t *testing.T) {
	for _, v := range []string{"off", "0", "none", "-", "", "sub"} {
		if n, ok := app.PortNum(v); ok || n != 0 {
			t.Errorf("PortNum(%q) = %d,%v", v, n, ok)
		}
	}
	for _, v := range []string{"7890", "1", "65535"} {
		if n, ok := app.PortNum(v); !ok || n <= 0 {
			t.Errorf("PortNum(%q) = %d,%v", v, n, ok)
		}
	}
	if n, ok := app.PortNum("70000"); ok || n != 0 {
		t.Error("out-of-range port must be rejected")
	}
}

var _ = strings.TrimSpace
