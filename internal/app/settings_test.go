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
	s.TCPConcurrent = "true"
	s.Platform = "linux/amd64"
	s.Flavor = "v3"
	s.Version = "v1.19.31"
	s.History = []CoreVer{{Version: "v1.19.30", Flavor: "v3", Platform: "linux/amd64", InstalledAt: time.Now()}}
	s.Overrides = map[string]any{"dns": map[string]any{
		"nameserver": []any{"https://a/dns-query", "https://b/dns-query"},
		"enable":     true,
	}}
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
	if back.TCPConcurrent != "true" {
		t.Errorf("core tri-state lost: %q", back.TCPConcurrent)
	}
	ns, _ := back.Overrides["dns"].(map[string]any)["nameserver"].([]any)
	if len(ns) != 2 || ns[0] != "https://a/dns-query" {
		t.Errorf("dns nameserver lost: %#v", back.Overrides)
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
