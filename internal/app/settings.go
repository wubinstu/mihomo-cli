package app

import (
	"crypto/rand"
	"encoding/hex"
	"os"
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

// Settings cli 自身配置 (config.toml)
type Settings struct {
	Lang           string `toml:"lang"` // "zh" | "en"; 空则按 $LANG 自动判断
	CurrentProfile string `toml:"current_profile"`
	CurrentGroup   string `toml:"current_group"` // group/node 命令的当前操作分组
	DownloadProxy  string `toml:"download_proxy"`

	// 代理监听与模式
	AllowLan  bool   `toml:"allow_lan"`  // 监听 0.0.0.0, 局域网可用
	MixedPort int    `toml:"mixed_port"` // http+socks5 混合端口
	ProxyMode string `toml:"proxy_mode"` // rule / global / direct; 空则跟随订阅

	// 外部控制 API
	APIBase   string `toml:"api_base"`
	APISecret string `toml:"api_secret"`

	// 订阅自动更新
	SubAutoUpdateEnabled  bool          `toml:"sub_auto_update_enabled"`
	SubAutoUpdateInterval time.Duration `toml:"sub_auto_update_interval"`

	// 自动测速并切换到最低延迟节点
	ProxyAutoSelectEnabled  bool          `toml:"proxy_auto_select_enabled"`
	ProxyAutoSelectInterval time.Duration `toml:"proxy_auto_select_interval"`
	ProxyAutoSelectGroup    string        `toml:"proxy_auto_select_group"` // 空 = 所有含真实节点的分组

	TestURL     string `toml:"test_url"`
	TestTimeout int    `toml:"test_timeout_ms"`

	Profiles []Profile `toml:"profiles"`
}

func DefaultSettings() *Settings {
	return &Settings{
		MixedPort:               7890,
		APIBase:                 "http://127.0.0.1:9090",
		SubAutoUpdateEnabled:    true,
		SubAutoUpdateInterval:   24 * time.Hour,
		ProxyAutoSelectEnabled:  false,
		ProxyAutoSelectInterval: 30 * time.Minute,
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
		return nil, err
	}
	// 兜底修正
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
	if s.ProxyAutoSelectInterval <= 0 {
		s.ProxyAutoSelectInterval = 30 * time.Minute
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
	return s, nil
}

// Save 写回配置
func (s *Settings) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	f, err := os.OpenFile(SettingsFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
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

// Current 返回当前生效的订阅, 不存在则返回 nil
func (s *Settings) Current() *Profile {
	if s.CurrentProfile == "" && len(s.Profiles) > 0 {
		return &s.Profiles[0]
	}
	return s.FindProfile(s.CurrentProfile)
}

func randSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
