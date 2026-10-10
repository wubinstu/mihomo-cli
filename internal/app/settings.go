package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
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

// ---- 磁盘上的段 (TOML 表) ----
// 用「导出的内嵌结构体 + toml tag」: BurntSushi 会写成 [cli]/[core]/[timer] 表,
// 而 Go 的内嵌让内存里仍是 s.AllowLan 这种扁平访问 —— 调用点一行都不用改,
// 文件的段落名却和 CLI 的 <段>.<键> 一一对应。

// CLI cli 自身的设置
type CLI struct {
	Language       string `toml:"language"` // zh | en; 空则按 $LANG
	CurrentProfile string `toml:"current_profile"`
	CurrentGroup   string `toml:"current_group"`
	GithubMirror   string `toml:"github_mirror,omitempty"` // GitHub 镜像站前缀, 空=auto
}

// CoreSpec 内核二进制的规格 (探测结果, 被 resource core upgrade 沿用)
type CoreSpec struct {
	Platform string    `toml:"platform,omitempty"` // linux/amd64
	Flavor   string    `toml:"flavor,omitempty"`   // v1|v2|v3|compatible|go120…
	Version  string    `toml:"version,omitempty"`  // v1.19.31
	History  []CoreVer `toml:"history,omitempty"`  // 版本栈 (新→旧)
}

// Core 注入内核 config.yaml 的参数
type Core struct {
	AllowLan          bool   `toml:"allow_lan"`
	MixedPort         string `toml:"mixed_port"` // 端口|off|sub; 默认 7890
	SocksPort         string `toml:"socks_port"` // 默认 off
	HTTPPort          string `toml:"http_port"`  // 默认 off
	ProxyMode         string `toml:"proxy_mode"` // rule/global/direct/sub
	IPV6Enabled       bool   `toml:"ipv6_enabled,omitempty"`
	LogLevel          string `toml:"log_level,omitempty"`           // debug/info/warning/error/silent/sub
	TCPConcurrent     string `toml:"tcp_concurrent,omitempty"`      // true|false|sub
	UnifiedDelay      string `toml:"unified_delay,omitempty"`       // true|false|sub
	KeepAliveInterval string `toml:"keep_alive_interval,omitempty"` // 秒 | sub
}

// ControlAPI 外部控制 API
type ControlAPI struct {
	Base   string `toml:"base"`
	Secret string `toml:"secret"`
}

// Timer systemd 定时任务
type Timer struct {
	SubAutoUpdateEnabled       bool          `toml:"sub_auto_update_enabled"`
	SubAutoUpdateInterval      time.Duration `toml:"sub_auto_update_interval"`
	NodeAutoSelectEnabled      bool          `toml:"node_auto_select_enabled"`
	NodeAutoSelectInterval     time.Duration `toml:"node_auto_select_interval"`
	AutoSelectLastRun          time.Time     `toml:"node_auto_select_last_run,omitempty"`
	ResourceAutoUpdateEnabled  bool          `toml:"resource_auto_update_enabled"`
	ResourceAutoUpdateInterval time.Duration `toml:"resource_auto_update_interval"`
	ResourceLastRun            time.Time     `toml:"resource_auto_update_last_run,omitempty"`
}

// Misc 其它 cli 设置
type Misc struct {
	TestURL     string `toml:"test_url"`
	TestTimeout int    `toml:"test_timeout_ms"`
}

// Settings cli 自身配置 (config.toml, 由程序管理, 请勿手动编辑)
type Settings struct {
	CLI        `toml:"cli"`       // cli.*
	CoreSpec   `toml:"core-spec"` // 内核规格
	Core       `toml:"core"`      // core.*
	ControlAPI `toml:"control-api"`
	Timer      `toml:"timer"` // timer.*
	Misc       `toml:"misc"`
	// 点号路径写入的配置段: [overrides.dns] / [overrides.tun] …
	Overrides map[string]any `toml:"overrides,omitempty"`
	UserRules []UserRule     `toml:"user_rules,omitempty"`
	Profiles  []Profile      `toml:"profiles"`
}

func DefaultSettings() *Settings {
	return &Settings{
		Core: Core{
			MixedPort: "7890",
			SocksPort: "off",
			HTTPPort:  "off",
			ProxyMode: "rule",
			LogLevel:  "",
		},
		ControlAPI: ControlAPI{Base: "http://127.0.0.1:9090"},
		Timer: Timer{
			SubAutoUpdateEnabled:       true,
			SubAutoUpdateInterval:      24 * time.Hour,
			NodeAutoSelectEnabled:      false,
			NodeAutoSelectInterval:     30 * time.Minute,
			ResourceAutoUpdateEnabled:  false,
			ResourceAutoUpdateInterval: 24 * time.Hour,
		},
		Misc: Misc{
			TestURL:     "https://www.gstatic.com/generate_204",
			TestTimeout: 5000,
		},
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

// ConfigLanguage 只从 config.toml 里取语言设置 (i18n 在包初始化时就要用, 不能等 LoadSettings)。
// 新旧两种磁盘格式都认: v1.4.1+ 是 [cli] language, 更早是扁平键 cli_language。
func ConfigLanguage() string {
	data, err := os.ReadFile(SettingsFile)
	if err != nil {
		return ""
	}
	var v struct {
		Language string `toml:"cli_language"` // <=v1.4.0 扁平键
		CLI      struct {
			Language string `toml:"language"`
		} `toml:"cli"` // v1.4.1+ 表
	}
	if err := toml.Unmarshal(data, &v); err != nil {
		return ""
	}
	if l := strings.ToLower(strings.TrimSpace(v.CLI.Language)); l != "" {
		return l
	}
	return strings.ToLower(strings.TrimSpace(v.Language))
}

// LoadSettings 读取配置; 文件不存在时返回默认值
// 认不出来的键一律**报错**而不是静默忽略 —— 历史上两次弄丢用户配置都是"静默解码"造成的。
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
	md, err := toml.Decode(string(data), s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "config.toml 解析失败", err)
	}
	// BurntSushi 会把 map[string]any 字段(overrides)里的嵌套键也报成 undecoded,
	// 虽然它们其实解码成功了 —— 这里要过滤掉, 只留真正认不出的键。
	if undecoded := realUndecoded(md.Undecoded()); len(undecoded) > 0 {
		// 有认不出来的键: 要么是 <=v1.4.0 的扁平旧格式, 要么是手改错了。
		// 先试迁移; 迁移不了就明确报错, 绝不静默丢数据。
		if migrated, ok := tryMigrateLegacy(data); ok {
			backupLegacyConfig()
			*s = *migrated
			s.fixup()
			// 顺手把文件重排成新格式 (一次性; 备份已在上面做好)
			if err := s.Save(); err == nil {
				DropLegacyOverrides()
			}
			return s, nil
		}
		var keys []string
		for _, k := range undecoded {
			keys = append(keys, strings.Join(k, "."))
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("%s: %s", "config.toml 含有无法识别的键", strings.Join(keys, ", "))
	}
	s.fixup()
	// v1.2 之前的 overrides.yaml 收编进 config.toml [overrides]
	if err := s.MigrateLegacy(); err == nil && len(s.Overrides) > 0 {
		_ = s.Save()
		DropLegacyOverrides()
	}
	return s, nil
}

// realUndecoded 过滤掉 overrides 段内的嵌套键 (库的已知行为, 见上)
func realUndecoded(keys []toml.Key) []toml.Key {
	var out []toml.Key
	for _, k := range keys {
		if len(k) > 0 && k[0] == "overrides" {
			continue
		}
		out = append(out, k)
	}
	return out
}

// tryMigrateLegacy 试着把旧格式规范成新结构; 成功且没有认不出的键才返回 ok
func tryMigrateLegacy(data []byte) (*Settings, bool) {
	norm, err := legacyNormalize(data)
	if err != nil {
		return nil, false
	}
	fresh := DefaultSettings()
	md, err := toml.Decode(string(norm), fresh)
	if err != nil || len(realUndecoded(md.Undecoded())) > 0 {
		return nil, false
	}
	return fresh, true
}

// legacyNormalize 把 <=v1.4.0 的扁平 config.toml 规范成 v1.4.1 的表结构。
// 只做键名/类型的搬运, 不改语义。
func legacyNormalize(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// 旧键名 → (新表, 新键名)
	move := map[string][2]string{
		"cli_language":    {"cli", "language"},
		"current_profile": {"cli", "current_profile"},
		"current_group":   {"cli", "current_group"},
		"github_mirror":   {"cli", "github_mirror"},
		"core_platform":   {"core-spec", "platform"},
		"core_flavor":     {"core-spec", "flavor"},
		"core_version":    {"core-spec", "version"},
		"allow_lan":       {"core", "allow_lan"},
		"ipv6_enabled":    {"core", "ipv6_enabled"},
		"log_level":       {"core", "log_level"},

		"api_base":                      {"control-api", "base"},
		"api_secret":                    {"control-api", "secret"},
		"sub_auto_update_enabled":       {"timer", "sub_auto_update_enabled"},
		"sub_auto_update_interval":      {"timer", "sub_auto_update_interval"},
		"node_auto_select_enabled":      {"timer", "node_auto_select_enabled"},
		"node_auto_select_interval":     {"timer", "node_auto_select_interval"},
		"auto_select_last_run":          {"timer", "node_auto_select_last_run"},
		"resource_auto_update_enabled":  {"timer", "resource_auto_update_enabled"},
		"resource_auto_update_interval": {"timer", "resource_auto_update_interval"},
		"resource_last_run":             {"timer", "resource_auto_update_last_run"},
		"test_url":                      {"misc", "test_url"},
		"test_timeout_ms":               {"misc", "test_timeout_ms"},
	}
	out := map[string]any{}
	for k, v := range raw {
		if m, ok := move[k]; ok {
			sec, nk := m[0], m[1]
			tbl, _ := out[sec].(map[string]any)
			if tbl == nil {
				tbl = map[string]any{}
				out[sec] = tbl
			}
			tbl[nk] = legacyValue(k, v)
			continue
		}
		switch k {
		case "mixed_port", "socks_port", "http_port":
			// 端口: int → string (0=off)
			tbl, _ := out["core"].(map[string]any)
			if tbl == nil {
				tbl = map[string]any{}
				out["core"] = tbl
			}
			tbl[k] = legacyPort(v)
		case "dns_servers":
			// ≤v1.4.2 它躺在 [core] 里, 但语义上是 dns 段的 nameserver, 挪进 [overrides.dns]
			ov, _ := out["overrides"].(map[string]any)
			if ov == nil {
				ov = map[string]any{}
				out["overrides"] = ov
			}
			dns, _ := ov["dns"].(map[string]any)
			if dns == nil {
				dns = map[string]any{}
				ov["dns"] = dns
			}
			dns["nameserver"] = legacyStrList(v)
		case "proxy_mode":
			tbl, _ := out["core"].(map[string]any)
			if tbl == nil {
				tbl = map[string]any{}
				out["core"] = tbl
			}
			tbl[k] = v
		case "tcp_concurrent", "unified_delay":
			tbl, _ := out["core"].(map[string]any)
			if tbl == nil {
				tbl = map[string]any{}
				out["core"] = tbl
			}
			tbl[k] = legacyTri(v)
		case "keep_alive_interval":
			tbl, _ := out["core"].(map[string]any)
			if tbl == nil {
				tbl = map[string]any{}
				out["core"] = tbl
			}
			tbl[k] = legacyNum(v)
		case "user_rules", "profiles", "overrides":
			out[k] = v // 数组表/段原样保留
		case "core_history":
			// 旧版是顶层 [[core_history]], 新版是 [core-spec] 下的表
			spec, _ := out["core-spec"].(map[string]any)
			if spec == nil {
				spec = map[string]any{}
				out["core-spec"] = spec
			}
			spec["history"] = v
		default:
			// 认不出的键原样保留: 这样二次解码仍然会报 undecoded,
			// tryMigrateLegacy 就会失败 → 上层明确报错, 不会静默丢数据
			out[k] = v
		}
	}
	renameV15(out)
	return toml.Marshal(out)
}

// legacyPort int 端口 → "7891" / "off"
// legacyStrList 旧格式里的字符串列表 (interface{} → []any)
func legacyStrList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func legacyPort(v any) string {
	if n, ok := v.(int64); ok {
		if n > 0 {
			return strconv.FormatInt(n, 10)
		}
		return "off"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return "off"
}

// legacyTri bool → "true"/"false"
func legacyTri(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "true"
		}
		return "false"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return "sub"
}

// legacyNum int64 → 十进制字符串
func legacyNum(v any) string {
	if n, ok := v.(int64); ok {
		return strconv.FormatInt(n, 10)
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func legacyValue(key string, v any) any { return v }

// MigrateLegacy 把 v1.2 之前的 overrides.yaml 收编进 config.toml [overrides];
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

func randSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// backupLegacyConfig 把旧格式 config.toml 备份为 config.toml.pre-1.5.bak
// (只做一次: 目标已存在就跳过)。格式迁移是会改变文件内容的动作, 必须留后路。
func backupLegacyConfig() {
	// 备份名带版本号: 每一轮迁移各自留档, 不会把上一轮的备份覆盖掉
	bak := SettingsFile + ".pre-" + Version + ".bak"
	if _, err := os.Stat(bak); err == nil {
		return
	}
	data, err := os.ReadFile(SettingsFile)
	if err != nil {
		return
	}
	_ = os.WriteFile(bak, data, 0o640)
}

// Save 写回配置: 按段写成 TOML 表, 段落名和 CLI 的 <段>.<键> 一一对应
// ([cli] ↔ cli.*, [core] ↔ core.*, [timer] ↔ timer.*, [overrides.dns] ↔ dns.*)
func (s *Settings) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# ============================================================\n")
	w("#  mihomo-cli 配置 (由程序管理, 请勿手动编辑)\n")
	w("#    查看: mihomo-cli config get\n")
	w("#    修改: mihomo-cli config set <key> <value>\n")
	w("#    漂移: mihomo-cli config apply | adopt\n")
	w("#    段落名和命令的 <段>.<键> 对应: [core]↔core.* [cli]↔cli.* [timer]↔timer.*\n")
	w("# ============================================================\n")

	w("\n[cli]\n")
	w("language = %s\n", tomlStr(s.Language))
	w("current_profile = %s\n", tomlStr(s.CurrentProfile))
	w("current_group = %s\n", tomlStr(s.CurrentGroup))
	if s.GithubMirror != "" {
		w("github_mirror = %s\n", tomlStr(s.GithubMirror))
	}

	if s.Platform != "" || s.Flavor != "" || s.Version != "" || len(s.History) > 0 {
		w("\n[core-spec]\n")
		if s.Platform != "" {
			w("platform = %s\n", tomlStr(s.Platform))
		}
		if s.Flavor != "" {
			w("flavor = %s\n", tomlStr(s.Flavor))
		}
		if s.Version != "" {
			w("version = %s\n", tomlStr(s.Version))
		}
	}

	// 托管键一律写**有效值** (空值回退默认值), 不再出现 log_level = "" 这种
	// "文件说空、config get 说 info" 的错位 (v1.5.0 用户反馈)。
	w("\n[core]\n")
	w("allow_lan = %v\n", s.AllowLan)
	w("mixed_port = %s\n", tomlStr(orDef(s.MixedPort, "7890")))
	w("socks_port = %s\n", tomlStr(orDef(s.SocksPort, "off")))
	w("http_port = %s\n", tomlStr(orDef(s.HTTPPort, "off")))
	w("proxy_mode = %s\n", tomlStr(orDef(s.ProxyMode, "rule")))
	w("ipv6_enabled = %v\n", s.IPV6Enabled)
	w("log_level = %s\n", tomlStr(orDef(s.LogLevel, "info")))
	w("tcp_concurrent = %s\n", tomlStr(orDef(s.TCPConcurrent, "true")))
	w("unified_delay = %s\n", tomlStr(orDef(s.UnifiedDelay, "true")))
	w("keep_alive_interval = %s\n", tomlStr(orDef(s.KeepAliveInterval, "30")))

	w("\n[control-api]\n")
	w("base = %s\n", tomlStr(s.Base))
	w("secret = %s\n", tomlStr(s.Secret))

	w("\n[timer]\n")
	w("sub_auto_update_enabled = %v\n", s.SubAutoUpdateEnabled)
	w("sub_auto_update_interval = %s\n", tomlStr(s.SubAutoUpdateInterval.String()))
	w("node_auto_select_enabled = %v\n", s.NodeAutoSelectEnabled)
	w("node_auto_select_interval = %s\n", tomlStr(s.NodeAutoSelectInterval.String()))
	w("resource_auto_update_enabled = %v\n", s.ResourceAutoUpdateEnabled)
	w("resource_auto_update_interval = %s\n", tomlStr(s.ResourceAutoUpdateInterval.String()))
	if !s.ResourceLastRun.IsZero() {
		w("resource_auto_update_last_run = %s\n", s.ResourceLastRun.Format("2006-01-02T15:04:05Z07:00"))
	}
	if !s.AutoSelectLastRun.IsZero() {
		w("node_auto_select_last_run = %s\n", s.AutoSelectLastRun.Format("2006-01-02T15:04:05Z07:00"))
	}

	w("\n[misc]\n")
	w("test_url = %s\n", tomlStr(orDef(s.TestURL, "https://www.gstatic.com/generate_204")))
	w("test_timeout_ms = %d\n", orInt(s.TestTimeout, 5000))

	if len(s.History) > 0 {
		w("\n[[core-spec.history]]\n")
		for _, v := range s.History {
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

	if len(s.Overrides) > 0 {
		var ov strings.Builder
		if err := toml.NewEncoder(&ov).Encode(map[string]any{"overrides": s.Overrides}); err == nil {
			w("\n")
			b.WriteString(ov.String())
		}
	}

	for _, r := range s.UserRules {
		w("\n[[user_rules]]\n")
		w("type = %s\n", tomlStr(r.Type))
		w("condition = %s\n", tomlStr(r.Condition))
		w("strategy = %s\n", tomlStr(r.Strategy))
		if r.NoResolve {
			w("no_resolve = true\n")
		}
		w("enabled = %v\n", r.Enabled)
	}
	for _, p := range s.Profiles {
		w("\n[[profiles]]\n")
		w("name = %s\n", tomlStr(p.Name))
		w("url = %s\n", tomlStr(p.URL))
		w("updated_at = %s\n", p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
		w("nodes = %d\n", p.Nodes)
		if p.UserInfo != "" {
			w("userinfo = %s\n", tomlStr(p.UserInfo))
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
	if s.Base == "" {
		s.Base = "http://127.0.0.1:9090"
	}
	if s.Secret == "" {
		s.Secret = randSecret()
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
	if s.KeepAliveInterval != "" && s.KeepAliveInterval != "sub" {
		if _, ok := PortNum(s.KeepAliveInterval); !ok {
			s.KeepAliveInterval = ""
		}
	}
	if s.Language != "zh" && s.Language != "en" {
		s.Language = ""
	}
	// github-mirror: auto / http(s)://host ; 旧版遗留的非法值(auto1 等)置空回退 auto
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

// renameV15 v1.5.0 的键名/段落调整 (就地改 out):
//   - core.dns_servers → overrides.dns.nameserver (它本来就是 dns 段的东西)
//   - timer.auto_select_last_run → timer.node_auto_select_last_run
//   - timer.resource_last_run     → timer.resource_auto_update_last_run
func renameV15(out map[string]any) {
	if core, ok := out["core"].(map[string]any); ok {
		if ns, ok := core["dns_servers"]; ok {
			delete(core, "dns_servers")
			ov, _ := out["overrides"].(map[string]any)
			if ov == nil {
				ov = map[string]any{}
				out["overrides"] = ov
			}
			dns, _ := ov["dns"].(map[string]any)
			if dns == nil {
				dns = map[string]any{}
				ov["dns"] = dns
			}
			dns["nameserver"] = ns
		}
	}
	if tm, ok := out["timer"].(map[string]any); ok {
		for _, r := range [][2]string{
			{"auto_select_last_run", "node_auto_select_last_run"},
			{"resource_last_run", "resource_auto_update_last_run"},
		} {
			if v, ok := tm[r[0]]; ok {
				delete(tm, r[0])
				tm[r[1]] = v
			}
		}
	}
}

// orDef 空值回退默认值 (Save 写托管键时用, 保证文件和 config get 完全一致)
func orDef(v, def string) string {
	if v == "" || v == "sub" {
		return def
	}
	return v
}

func orInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func tomlStr(s string) string { return fmt.Sprintf("%q", s) }

func tomlArr(items []string) string {
	var ps []string
	for _, it := range items {
		ps = append(ps, tomlStr(it))
	}
	return strings.Join(ps, ", ")
}

// FindProfile 按名字找订阅
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

// CurrentUpdatedAt 当前订阅的更新时间 (没有订阅时给零值, 免得到处判空)
func (s *Settings) CurrentUpdatedAt() time.Time {
	if p := s.Current(); p != nil {
		return p.UpdatedAt
	}
	return time.Time{}
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
