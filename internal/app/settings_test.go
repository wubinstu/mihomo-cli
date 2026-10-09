package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setup 临时数据目录 (不碰 /etc/mihomo-cli)
func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	BaseDir = dir
	BinDir = filepath.Join(dir, "bin")
	ProfileDir = filepath.Join(dir, "profiles")
	RuntimeDir = filepath.Join(dir, "runtime")
	VersionsDir = filepath.Join(dir, "bin", "versions")
	LogDir = filepath.Join(dir, "logs")
	CoreBin = filepath.Join(BinDir, "mihomo")
	CoreBinOld = filepath.Join(BinDir, "mihomo.old")
	SettingsFile = filepath.Join(dir, "config.toml")
	OverridesFile = filepath.Join(dir, "overrides.yaml")
	RuntimeConfig = filepath.Join(RuntimeDir, "config.yaml")
	LogFile = filepath.Join(LogDir, "mihomo.log")
}

// 新格式 round-trip: Save 写出来的必须能原样读回
func TestSettingsRoundTrip(t *testing.T) {
	setup(t)
	s := DefaultSettings()
	s.Language = "zh"
	s.CurrentProfile = "EDT"
	s.CurrentGroup = "🚀 节点选择"
	s.AllowLan = true
	s.MixedPort = "7890"
	s.SocksPort = "7891"
	s.HTTPPort = "7892"
	s.IPV6Enabled = true
	s.DNSServers = []string{"https://a/dns-query", "https://b/dns-query"}
	s.TCPConcurrent = "true"
	s.Platform = "linux/amd64"
	s.Flavor = "v3"
	s.Version = "v1.19.31"
	s.History = []CoreVer{{Version: "v1.19.30", Flavor: "v3", Platform: "linux/amd64", InstalledAt: time.Now()}}
	s.Overrides = map[string]any{"dns": map[string]any{"enable": true}}
	s.UserRules = []UserRule{{Type: "DOMAIN", Condition: "x.com", Strategy: "DIRECT", Enabled: true}}
	s.Profiles = []Profile{{Name: "EDT", URL: "https://x/sub", Nodes: 16}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if back.Language != "zh" || back.CurrentProfile != "EDT" || back.CurrentGroup != "🚀 节点选择" {
		t.Errorf("cli section lost: %+v", back.CLI)
	}
	if !back.AllowLan || back.MixedPort != "7890" || back.SocksPort != "7891" || back.HTTPPort != "7892" || !back.IPV6Enabled {
		t.Errorf("core section lost: %+v", back.Core)
	}
	if len(back.DNSServers) != 2 || back.TCPConcurrent != "true" {
		t.Errorf("core lists lost: %v %q", back.DNSServers, back.TCPConcurrent)
	}
	if back.Platform != "linux/amd64" || back.Flavor != "v3" || back.Version != "v1.19.31" || len(back.History) != 1 {
		t.Errorf("core-spec lost: %+v %+v", back.CoreSpec, back.History)
	}
	if dns, ok := back.Overrides["dns"].(map[string]any); !ok || dns["enable"] != true {
		t.Errorf("overrides lost: %#v", back.Overrides)
	}
	if len(back.UserRules) != 1 || len(back.Profiles) != 1 {
		t.Errorf("tables lost: %d %d", len(back.UserRules), len(back.Profiles))
	}
}

// 旧格式 (<=v1.4.0 扁平键) 必须能迁移, 且一个值都不能丢
func TestLegacyFlatFormatMigrates(t *testing.T) {
	setup(t)
	legacy := `cli_language = "zh"
current_profile = "EDT"
current_group = "节点选择"
github_mirror = "auto1"
core_platform = "linux/amd64"
core_flavor = "v3"
core_version = "v1.19.31"
allow_lan = true
mixed_port = 7890
socks_port = 7891
http_port = 7892
proxy_mode = "rule"
ipv6_enabled = true
log_level = "info"
dns_servers = ["https://a/dns-query"]
tcp_concurrent = true
unified_delay = false
keep_alive_interval = 30
api_base = "http://127.0.0.1:9090"
api_secret = "sec"
sub_auto_update_enabled = true
sub_auto_update_interval = "24h0m0s"
node_auto_select_enabled = true
node_auto_select_interval = "30m0s"
resource_auto_update_enabled = true
resource_auto_update_interval = "24h0m0s"
test_url = "https://www.gstatic.com/generate_204"
test_timeout_ms = 5000

[[user_rules]]
type = "DOMAIN"
condition = "x.com"
strategy = "DIRECT"
enabled = true

[[profiles]]
name = "EDT"
url = "https://x/sub"
updated_at = 2026-09-26T09:52:36+08:00
nodes = 16
`
	if err := os.WriteFile(SettingsFile, []byte(legacy), 0o640); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("legacy migrate failed: %v", err)
	}
	// 每个字段逐一核对 (丢一个就是这个测试存在的意义)
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"language", s.Language, "zh"},
		{"current_profile", s.CurrentProfile, "EDT"},
		{"current_group", s.CurrentGroup, "节点选择"},
		{"github_mirror", s.GithubMirror, ""}, // auto1 非法 → 置空
		{"platform", s.Platform, "linux/amd64"},
		{"flavor", s.Flavor, "v3"},
		{"version", s.Version, "v1.19.31"},
		{"allow_lan", s.AllowLan, true},
		{"mixed_port", s.MixedPort, "7890"},
		{"socks_port", s.SocksPort, "7891"},
		{"http_port", s.HTTPPort, "7892"},
		{"proxy_mode", s.ProxyMode, "rule"},
		{"ipv6_enabled", s.IPV6Enabled, true},
		{"log_level", s.LogLevel, "info"},
		{"tcp_concurrent", s.TCPConcurrent, "true"},
		{"unified_delay", s.UnifiedDelay, "false"},
		{"keep_alive_interval", s.KeepAliveInterval, "30"},
		{"base", s.Base, "http://127.0.0.1:9090"},
		{"secret", s.Secret, "sec"},
		{"sub_auto_update_enabled", s.SubAutoUpdateEnabled, true},
		{"sub_auto_update_interval", s.SubAutoUpdateInterval, 24 * time.Hour},
		{"node_auto_select_enabled", s.NodeAutoSelectEnabled, true},
		{"node_auto_select_interval", s.NodeAutoSelectInterval, 30 * time.Minute},
		{"resource_auto_update_enabled", s.ResourceAutoUpdateEnabled, true},
		{"test_url", s.TestURL, "https://www.gstatic.com/generate_204"},
		{"test_timeout_ms", s.TestTimeout, 5000},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %#v, want %#v", c.name, c.got, c.want)
		}
	}
	if len(s.DNSServers) != 1 || s.DNSServers[0] != "https://a/dns-query" {
		t.Errorf("dns_servers lost: %v", s.DNSServers)
	}
	if len(s.UserRules) != 1 || len(s.Profiles) != 1 {
		t.Errorf("tables lost: %d %d", len(s.UserRules), len(s.Profiles))
	}
	// 迁移要留备份, 并把文件重排成新格式
	if _, err := os.Stat(SettingsFile + ".pre-1.5.bak"); err != nil {
		t.Error("legacy config should be backed up")
	}
	data, _ := os.ReadFile(SettingsFile)
	if !contains(string(data), "[core]") || !contains(string(data), "[cli]") {
		t.Errorf("file not rewritten to the new format:\n%s", data)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// 认不出来的键必须报错, 不能静默忽略
func TestUnknownKeysAreAnError(t *testing.T) {
	setup(t)
	if err := os.WriteFile(SettingsFile, []byte("bogus_key = 1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSettings(); err == nil {
		t.Fatal("unknown keys must be an error, not silently ignored")
	} else {
		t.Logf("correctly rejected: %v", err)
	}
}
