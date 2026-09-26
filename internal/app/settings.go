package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
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

// CoreVer 一个装过的内核版本 (本地版本栈, rollback 出栈)
type CoreVer struct {
	Version     string    `toml:"version"`
	Flavor      string    `toml:"flavor,omitempty"`   // amd64: v1/v2/v3/compatible 等
	Platform    string    `toml:"platform,omitempty"` // linux/amd64
	InstalledAt time.Time `toml:"installed_at"`
}

// Settings cli 自身配置 (config.toml, 由程序管理, 请勿手动编辑)
type Settings struct {
	// ---- cli 自身 ----
	CLILanguage    string `toml:"cli_language"` // zh | en; 空则按 $LANG
	CurrentProfile string `toml:"current_profile"`
	CurrentGroup   string `toml:"current_group"`
	GithubMirror   string `toml:"github_mirror,omitempty"` // GitHub 镜像站前缀, 空=auto

	// ---- 内核规格 (探测结果, 由 install/resource core 写入并被沿用) ----
	CorePlatform string    `toml:"core_platform,omitempty"` // linux/amd64
	CoreFlavor   string    `toml:"core_flavor,omitempty"`   // v1|v2|v3|compatible|go120…; 空=官方默认包
	CoreVersion  string    `toml:"core_version,omitempty"`  // 当前安装的内核版本
	CoreHistory  []CoreVer `toml:"core_history,omitempty"`  // 版本栈 (新→旧)

	// ---- 内核 config.yaml (render 注入) ----
	AllowLan    bool     `toml:"allow_lan"`
	MixedPort   string   `toml:"mixed_port"` // 端口|off|sub; 默认 7890
	SocksPort   string   `toml:"socks_port"` // 默认 off
	HTTPPort    string   `toml:"http_port"`  // 默认 off
	ProxyMode   string   `toml:"proxy_mode"` // rule/global/direct/sub; sub=跟随订阅
	IPV6Enabled bool     `toml:"ipv6_enabled,omitempty"`
	LogLevel    string   `toml:"log_level,omitempty"` // debug/info/warning/error/silent/sub
	DNSServers  []string `toml:"dns_servers,omitempty"`
	// 高级内核参数: 具体数值 / "sub"(跟随订阅) / ""(=默认值, 由 cfg 补全)
	TCPConcurrent     string `toml:"tcp_concurrent,omitempty"`
	UnifiedDelay      string `toml:"unified_delay,omitempty"`
	KeepAliveInterval string `toml:"keep_alive_interval,omitempty"`

	// ---- 嵌套配置段 (点号路径写入; render 时对订阅 yaml 深合并) ----
	// 形如 [overrides.tun] enable = true / [overrides.dns] enable = true
	Overrides map[string]any `toml:"overrides,omitempty"`

	// ---- 外部控制 API ----
	APIBase   string `toml:"api_base"`
	APISecret string `toml:"api_secret"`

	// ---- systemd timer 自动任务 ----
	SubAutoUpdateEnabled   bool          `toml:"sub_auto_update_enabled"`
	SubAutoUpdateInterval  time.Duration `toml:"sub_auto_update_interval"`
	NodeAutoSelectEnabled  bool          `toml:"node_auto_select_enabled"`
	NodeAutoSelectInterval time.Duration `toml:"node_auto_select_interval"`
	AutoSelectLastRun      time.Time     `toml:"auto_select_last_run,omitempty"`

	// ---- 资源自动更新 (mmdb/asn/geoip/geosite 数据, systemd timer) ----
	ResourceAutoUpdateEnabled  bool          `toml:"resource_auto_update_enabled"`
	ResourceAutoUpdateInterval time.Duration `toml:"resource_auto_update_interval"`
	ResourceLastRun            time.Time     `toml:"resource_last_run,omitempty"`

	TestURL     string `toml:"test_url"`
	TestTimeout int    `toml:"test_timeout_ms"`

	// ---- 用户规则 (独立于订阅, 优先匹配) ----
	UserRules []UserRule `toml:"user_rules,omitempty"`

	Profiles []Profile `toml:"profiles"`
}

func DefaultSettings() *Settings {
	return &Settings{
		MixedPort:                  "7890",
		SocksPort:                  "off",
		HTTPPort:                   "off",
		ProxyMode:                  "rule",
		SubAutoUpdateEnabled:       true,
		SubAutoUpdateInterval:      24 * time.Hour,
		NodeAutoSelectEnabled:      false,
		NodeAutoSelectInterval:     30 * time.Minute,
		ResourceAutoUpdateEnabled:  false,
		ResourceAutoUpdateInterval: 24 * time.Hour,
		TestURL:                    "https://www.gstatic.com/generate_204",
		TestTimeout:                5000,
	}
}

// ---- 端口语义 ----
// 端口键的值是三态的: 具体数字 = 强制监听该端口; "off" = 不监听(输入侧接受 0/none/-);
// "sub" = 跟随订阅(订阅没写就不监听); "" = 未设置(视为默认值, 由 cfg 补全)

// PortNum 解析端口字符串; ok=false 表示"不监听"(off/sub/未设置), ok=true 返回端口号
func PortNum(v string) (n int, ok bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "sub", "off", "none", "-", "0":
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// PortNumOr 端口值; 不监听/未设置时回退 def
func PortNumOr(v string, def int) int {
	if n, ok := PortNum(v); ok {
		return n
	}
	return def
}

// ProxyPort 本机代理端口(环境变量/API/体检都看它): mixed-port 优先, 否则 http, 否则 socks, 再否则 7890
func (s *Settings) ProxyPort() int {
	if n, ok := PortNum(s.MixedPort); ok {
		return n
	}
	if n, ok := PortNum(s.HTTPPort); ok {
		return n
	}
	if n, ok := PortNum(s.SocksPort); ok {
		return n
	}
	return 7890
}

// LoadSettings 读取配置; 文件不存在时返回默认值
func LoadSettings() (*Settings, error) {
	s := DefaultSettings()
	data, err := os.ReadFile(SettingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			s.fixup()
			return s, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, s); err != nil {
		// 旧版 (<=v1.2) 键类型不同: 端口是 int, tcp-concurrent/unified-delay 是 bool,
		// keep-alive-interval 是 int。先规范化再解码。
		if norm, nerr := legacyNormalize(data); nerr == nil {
			if err2 := toml.Unmarshal(norm, s); err2 == nil {
				// 迁移前先备份, 任何时候都能找回原值
				backupLegacyConfig()
				s.fixup()
				return s, nil
			}
		}
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
	// v1.2 之前的 overrides.yaml 收编进 config.toml [overrides]
	if err := s.MigrateLegacy(); err == nil && len(s.Overrides) > 0 {
		_ = s.Save()
		DropLegacyOverrides()
	}
	return s, nil
}

// legacyNormalize 把 <=v1.2 的 config.toml 规范成当前键类型:
// 端口 int→string(0=off)、三态 bool→"true"/"false"、keep-alive-interval int→string
func legacyNormalize(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	port := func(k string) {
		if v, ok := raw[k]; ok {
			if n, isInt := v.(int64); isInt {
				if n > 0 {
					raw[k] = strconv.FormatInt(n, 10)
				} else {
					raw[k] = "off"
				}
			}
		}
	}
	tri := func(k string) {
		if v, ok := raw[k]; ok {
			if b, isBool := v.(bool); isBool {
				if b {
					raw[k] = "true"
				} else {
					raw[k] = "false"
				}
			}
		}
	}
	num := func(k string) {
		if v, ok := raw[k]; ok {
			if n, isInt := v.(int64); isInt {
				raw[k] = strconv.FormatInt(n, 10)
			}
		}
	}
	port("mixed_port")
	port("socks_port")
	port("http_port")
	tri("tcp_concurrent")
	tri("unified_delay")
	num("keep_alive_interval")
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// fixup 兜底修正与旧键迁移
func (s *Settings) fixup() {
	if s.MixedPort == "" {
		s.MixedPort = "7890"
	}
	if s.SocksPort == "" {
		s.SocksPort = "off"
	}
	if s.HTTPPort == "" {
		s.HTTPPort = "off"
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
	if s.ResourceAutoUpdateInterval <= 0 {
		s.ResourceAutoUpdateInterval = 24 * time.Hour
	}
	if s.TestURL == "" {
		s.TestURL = "https://www.gstatic.com/generate_204"
	}
	if s.TestTimeout <= 0 {
		s.TestTimeout = 5000
	}
	switch s.ProxyMode {
	case "rule", "global", "direct", "sub", "":
	default:
		s.ProxyMode = ""
	}
	switch s.LogLevel {
	case "debug", "info", "warning", "error", "silent", "sub", "":
	default:
		s.LogLevel = ""
	}
	switch s.TCPConcurrent {
	case "true", "false", "sub", "":
	default:
		s.TCPConcurrent = ""
	}
	switch s.UnifiedDelay {
	case "true", "false", "sub", "":
	default:
		s.UnifiedDelay = ""
	}
	if s.KeepAliveInterval != "" {
		if _, ok := PortNum(s.KeepAliveInterval); s.KeepAliveInterval != "sub" && !ok {
			s.KeepAliveInterval = ""
		}
	}
	if s.CLILanguage != "zh" && s.CLILanguage != "en" {
		s.CLILanguage = ""
	}
	// github-mirror: auto / http(s)://host ; 旧版遗留的非法值(auto1 等)立即置空回退 auto
	if s.GithubMirror != "" && !validMirror(s.GithubMirror) {
		s.GithubMirror = ""
	}
	// 丢弃无效规则 (空类型/迁移残留)
	valid := s.UserRules[:0]
	for _, r := range s.UserRules {
		if r.Type != "" {
			valid = append(valid, r)
		}
	}
	s.UserRules = valid
	if s.Overrides == nil {
		s.Overrides = map[string]any{}
	}
}

// validMirror 镜像站前缀形态校验 (L1): auto 或 http(s)://host
func validMirror(v string) bool {
	if v == "auto" || v == "" {
		return true
	}
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}

// MigrateLegacy 把 v1.2 之前的 overrides.yaml 收编进 config.toml 的 [overrides];
// 成功写回后删除旧文件。只做一次。
func (s *Settings) MigrateLegacy() error {
	if len(s.Overrides) > 0 {
		return nil
	}
	data, err := os.ReadFile(OverridesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil || len(m) == 0 {
		return nil // 空文件或不可解析: 不迁移, 保持原样
	}
	s.Overrides = m
	return nil
}

// DropLegacyOverrides 删除已被收编的 overrides.yaml (写回成功后调用)
func DropLegacyOverrides() {
	if _, err := os.Stat(OverridesFile); err == nil {
		_ = os.Rename(OverridesFile, OverridesFile+".migrated")
	}
}

// Save 写回配置: 分组 + 注释, 美观且程序可读
func (s *Settings) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# ============================================================\n")
	w("#  mihomo-cli 配置 (由程序管理, 请勿手动编辑)\n")
	w("#    查看: mihomo-cli config get    修改: mihomo-cli config set <key> <value>\n")
	w("#    手动改动后请执行: mihomo-cli config update-service\n")
	w("# ============================================================\n")

	w("\n# ---------- cli ----------\n")
	w("cli_language = %s\n", tomlStr(orDefault(s.CLILanguage, "")))
	w("current_profile = %s\n", tomlStr(s.CurrentProfile))
	w("current_group = %s\n", tomlStr(s.CurrentGroup))
	if s.GithubMirror != "" {
		w("github_mirror = %s\n", tomlStr(s.GithubMirror))
	}

	w("\n# ---------- core (内核二进制规格) ----------\n")
	if s.CorePlatform != "" {
		w("core_platform = %s\n", tomlStr(s.CorePlatform))
	}
	if s.CoreFlavor != "" {
		w("core_flavor = %s\n", tomlStr(s.CoreFlavor))
	}
	if s.CoreVersion != "" {
		w("core_version = %s\n", tomlStr(s.CoreVersion))
	}

	w("\n# ---------- core config (注入内核 config.yaml) ----------\n")
	w("allow_lan = %v\n", s.AllowLan)
	w("mixed_port = %s\n", tomlStr(s.MixedPort))
	w("socks_port = %s\n", tomlStr(s.SocksPort))
	w("http_port = %s\n", tomlStr(s.HTTPPort))
	w("proxy_mode = %s\n", tomlStr(s.ProxyMode))
	w("ipv6_enabled = %v\n", s.IPV6Enabled)
	w("log_level = %s\n", tomlStr(s.LogLevel))
	if len(s.DNSServers) > 0 {
		w("dns_servers = [%s]\n", tomlArr(s.DNSServers))
	}
	if s.TCPConcurrent != "" && s.TCPConcurrent != "sub" {
		w("tcp_concurrent = %s\n", s.TCPConcurrent)
	}
	if s.UnifiedDelay != "" && s.UnifiedDelay != "sub" {
		w("unified_delay = %s\n", s.UnifiedDelay)
	}
	if s.KeepAliveInterval != "" && s.KeepAliveInterval != "sub" {
		w("keep_alive_interval = %s\n", s.KeepAliveInterval)
	}

	w("\n# ---------- control api ----------\n")
	w("api_base = %s\n", tomlStr(s.APIBase))
	w("api_secret = %s\n", tomlStr(s.APISecret))

	w("\n# ---------- timers (systemd) ----------\n")
	w("sub_auto_update_enabled = %v\n", s.SubAutoUpdateEnabled)
	w("sub_auto_update_interval = %s\n", tomlStr(s.SubAutoUpdateInterval.String()))
	w("node_auto_select_enabled = %v\n", s.NodeAutoSelectEnabled)
	w("node_auto_select_interval = %s\n", tomlStr(s.NodeAutoSelectInterval.String()))
	w("resource_auto_update_enabled = %v\n", s.ResourceAutoUpdateEnabled)
	w("resource_auto_update_interval = %s\n", tomlStr(s.ResourceAutoUpdateInterval.String()))
	if !s.ResourceLastRun.IsZero() {
		w("resource_last_run = %s\n", s.ResourceLastRun.Format("2006-01-02T15:04:05Z07:00"))
	}
	if !s.AutoSelectLastRun.IsZero() {
		w("auto_select_last_run = %s\n", s.AutoSelectLastRun.Format("2006-01-02T15:04:05Z07:00"))
	}

	w("\n# ---------- misc ----------\n")
	w("test_url = %s\n", tomlStr(s.TestURL))
	w("test_timeout_ms = %d\n", s.TestTimeout)

	if len(s.UserRules) > 0 {
		w("\n# ---------- user rules (优先于订阅规则) ----------\n")
		for _, r := range s.UserRules {
			w("[[user_rules]]\n")
			w("type = %s\n", tomlStr(r.Type))
			w("condition = %s\n", tomlStr(r.Condition))
			w("strategy = %s\n", tomlStr(r.Strategy))
			if r.NoResolve {
				w("no_resolve = true\n")
			}
			w("enabled = %v\n\n", r.Enabled)
		}
	}

	// 版本栈 (rollback 出栈, 旧→新)
	if len(s.CoreHistory) > 0 {
		w("\n# ---------- core history (版本栈, rollback 出栈) ----------\n")
		for _, v := range s.CoreHistory {
			w("[[core_history]]\n")
			w("version = %s\n", tomlStr(v.Version))
			if v.Flavor != "" {
				w("flavor = %s\n", tomlStr(v.Flavor))
			}
			if v.Platform != "" {
				w("platform = %s\n", tomlStr(v.Platform))
			}
			w("installed_at = %s\n\n", v.InstalledAt.Format("2006-01-02T15:04:05Z07:00"))
		}
	}

	// 嵌套配置段: [overrides.tun] / [overrides.dns] / …(render 时深合并进订阅 yaml)
	if len(s.Overrides) > 0 {
		var ov strings.Builder
		if err := toml.NewEncoder(&ov).Encode(map[string]any{"overrides": s.Overrides}); err == nil {
			w("\n# ---------- overrides (点号路径写入的配置段, 深合并进内核 yaml) ----------\n")
			b.WriteString(ov.String())
		}
	}

	if len(s.Profiles) > 0 {
		w("# ---------- profiles (订阅) ----------\n")
		for _, p := range s.Profiles {
			w("[[profiles]]\n")
			w("name = %s\n", tomlStr(p.Name))
			w("url = %s\n", tomlStr(p.URL))
			w("updated_at = %s\n", p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
			w("nodes = %d\n", p.Nodes)
			if p.UserInfo != "" {
				w("userinfo = %s\n", tomlStr(p.UserInfo))
			}
			w("\n")
		}
	}
	f, err := os.OpenFile(SettingsFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}

func tomlStr(s string) string {
	q := fmt.Sprintf("%q", s)
	return q
}

func tomlArr(items []string) string {
	var ps []string
	for _, it := range items {
		ps = append(ps, tomlStr(it))
	}
	return strings.Join(ps, ", ")
}

func orDefault(s, d string) string { return s }

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

// LoadSettingsQuiet 同 LoadSettings; 权限不足/损坏时返回 nil 而非报错
// (补全函数调用它, 绝不能因环境问题把 shell 的 __complete 进程杀掉)
func LoadSettingsQuiet() (*Settings, error) {
	s, err := LoadSettings()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// backupLegacyConfig 把旧格式 config.toml 备份为 config.toml.pre-1.3.bak
// (只做一次: 目标已存在就跳过)。格式迁移是会改变文件内容的动作, 必须留后路。
func backupLegacyConfig() {
	bak := SettingsFile + ".pre-1.3.bak"
	if _, err := os.Stat(bak); err == nil {
		return
	}
	data, err := os.ReadFile(SettingsFile)
	if err != nil {
		return
	}
	_ = os.WriteFile(bak, data, 0o640)
}
