package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Profile 一个订阅配置
type Profile struct {
	Name      string    `toml:"name"`
	URL       string    `toml:"url"`
	UpdatedAt time.Time `toml:"updated_at"`
	Nodes     int       `toml:"nodes"`
	UserInfo  string    `toml:"userinfo,omitempty"` // subscription-userinfo 响应头
}

// UserRule 用户自定义规则 (结构化存储)
type UserRule struct {
	Type      string `toml:"type"`
	Condition string `toml:"condition"`
	Strategy  string `toml:"strategy"`
	NoResolve bool   `toml:"no_resolve,omitempty"`
	Enabled   bool   `toml:"enabled"`
}

// String 拼接为内核规则行
func (r UserRule) String() string {
	parts := []string{r.Type}
	if r.Condition != "" {
		parts = append(parts, r.Condition)
	}
	parts = append(parts, r.Strategy)
	if r.NoResolve {
		parts = append(parts, "no-resolve")
	}
	return strings.Join(parts, ",")
}

// Settings cli 自身配置 (config.toml, 由程序管理, 请勿手动编辑)
type Settings struct {
	// ---- cli 自身 ----
	CLILanguage   string `toml:"cli_language"` // zh | en; 空则按 $LANG
	CurrentProfile string `toml:"current_profile"`
	CurrentGroup   string `toml:"current_group"`
	InstallMirror  string `toml:"install_mirror,omitempty"` // GitHub 镜像站前缀, 空=自动尝试

	// ---- 内核 config.yaml (render 注入) ----
	AllowLan   bool     `toml:"allow_lan"`
	MixedPort  int      `toml:"mixed_port"`
	ProxyMode  string   `toml:"proxy_mode"` // rule/global/direct; 空则跟随订阅
	IPV6Enabled bool    `toml:"ipv6_enabled,omitempty"`
	LogLevel   string   `toml:"log_level,omitempty"` // debug/info/warning/error/silent
	DNSServers []string `toml:"dns_servers,omitempty"`

	// ---- 外部控制 API ----
	APIBase   string `toml:"api_base"`
	APISecret string `toml:"api_secret"`

	// ---- systemd timer 自动任务 ----
	SubAutoUpdateEnabled  bool          `toml:"sub_auto_update_enabled"`
	SubAutoUpdateInterval time.Duration `toml:"sub_auto_update_interval"`
	NodeAutoSelectEnabled bool          `toml:"node_auto_select_enabled"`
	NodeAutoSelectInterval time.Duration `toml:"node_auto_select_interval"`
	AutoSelectLastRun     time.Time     `toml:"auto_select_last_run,omitempty"`

	TestURL     string `toml:"test_url"`
	TestTimeout int    `toml:"test_timeout_ms"`

	// ---- 用户规则 (独立于订阅, 优先匹配) ----
	UserRules []UserRule `toml:"user_rules,omitempty"`

	Profiles []Profile `toml:"profiles"`
}

func DefaultSettings() *Settings {
	return &Settings{
		MixedPort:               7890,
		APIBase:                 "http://127.0.0.1:9090",
		SubAutoUpdateEnabled:    true,
		SubAutoUpdateInterval:   24 * time.Hour,
		NodeAutoSelectEnabled:   false,
		NodeAutoSelectInterval:  30 * time.Minute,
		TestURL:                 "https://www.gstatic.com/generate_204",
		TestTimeout:             5000,
	}
}

// LoadSettings 读取配置; 文件不存在时返回默认值
func LoadSettings() (*Settings, error) {
	s := DefaultSettings()
	data, err := os.ReadFile(SettingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, s); err != nil {
		// 旧版 user_rules 为字符串数组, 类型不匹配: 以旧格式解码迁移
		old := struct {
			Settings
			LegacyRules []string `toml:"user_rules"`
		}{Settings: *s}
		if err2 := toml.Unmarshal(data, &old); err2 != nil {
			return nil, err
		}
		*s = old.Settings
		for _, raw := range old.LegacyRules {
			parts := strings.Split(raw, ",")
			if len(parts) < 3 {
				continue
			}
			s.UserRules = append(s.UserRules, UserRule{
				Type: parts[0], Condition: parts[1], Strategy: parts[2],
				NoResolve: len(parts) >= 4 && parts[3] == "no-resolve",
				Enabled:   true,
			})
		}
	}
	s.fixup()
	return s, nil
}

// fixup 兜底修正与旧键迁移
func (s *Settings) fixup() {
	if s.MixedPort == 0 {
		s.MixedPort = 7890
	}
	if s.APIBase == "" {
		s.APIBase = "http://127.0.0.1:9090"
	}
	if s.APISecret == "" {
		s.APISecret = randSecret()
	}
	if s.SubAutoUpdateInterval <= 0 {
		s.SubAutoUpdateInterval = 24 * time.Hour
	}
	if s.NodeAutoSelectInterval <= 0 {
		s.NodeAutoSelectInterval = 30 * time.Minute
	}
	if s.TestURL == "" {
		s.TestURL = "https://www.gstatic.com/generate_204"
	}
	if s.TestTimeout <= 0 {
		s.TestTimeout = 5000
	}
	switch s.ProxyMode {
	case "rule", "global", "direct", "":
	default:
		s.ProxyMode = ""
	}
	switch s.LogLevel {
	case "debug", "info", "warning", "error", "silent", "":
	default:
		s.LogLevel = ""
	}
	if s.CLILanguage != "zh" && s.CLILanguage != "en" {
		s.CLILanguage = ""
	}
	// 丢弃无效规则 (空类型/迁移残留)
	valid := s.UserRules[:0]
	for _, r := range s.UserRules {
		if r.Type != "" {
			valid = append(valid, r)
		}
	}
	s.UserRules = valid
}

// Save 写回配置 (文件头带程序管理警告)
func (s *Settings) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	f, err := os.OpenFile(SettingsFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# %s\n# %s\n", "Managed by mihomo-cli - DO NOT EDIT MANUALLY", "use: mihomo-cli config get/set")
	return toml.NewEncoder(f).Encode(s)
}

func (s *Settings) FindProfile(name string) *Profile {
	for i := range s.Profiles {
		if s.Profiles[i].Name == name {
			return &s.Profiles[i]
		}
	}
	return nil
}

// Current 返回当前生效的订阅 (current_profile 为空时悬空, 返回 nil)
func (s *Settings) Current() *Profile {
	return s.FindProfile(s.CurrentProfile)
}

// EnabledRules 仅启用的用户规则
func (s *Settings) EnabledRules() []string {
	var out []string
	for _, r := range s.UserRules {
		if r.Enabled {
			out = append(out, r.String())
		}
	}
	return out
}

func randSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
