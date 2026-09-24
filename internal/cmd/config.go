package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// configCmd 配置管理: config.toml 由程序管理, 禁止手动编辑 (文件头有警告)
var configCmd = &cobra.Command{
	Use:   "config",
	Short: T("配置管理: get/set/reset-default/sync"),
	Long: T("配置文件 /etc/mihomo-cli/config.toml 由 mihomo-cli 管理与运行时回写, 请勿手动编辑;") + "\n" +
		T("手动改动后请执行 config sync update-service 以配置文件覆盖运行中的服务。"),
}

// ---- get ----

type cfgEntry struct{ cat, key string }

// configKeys 分类键表 (get 展示 / set 校验 / 补全)
// configDefaults 有默认值的配置项
var configDefaults = map[string]string{
	"allow-lan": "false", "mixed-port": "7890", "proxy-mode": "rule",
	"ipv6-enabled": "false", "log-level": "info",
	"cli-language": "auto", "github-mirror": "auto",
	"test-url": "gstatic-204", "test-timeout": "5000ms",
	"sub-auto-update-enabled": "true", "sub-auto-update-interval": "24h",
	"node-auto-select-enabled": "false", "node-auto-select-interval": "30m",
	"resource-auto-update-enabled": "false", "resource-auto-update-interval": "24h",
}

var configKeys = []cfgEntry{
	{"core", "allow-lan"}, {"core", "mixed-port"}, {"core", "proxy-mode"},
	{"core", "ipv6-enabled"}, {"core", "log-level"},
	{"core", "tcp-concurrent"}, {"core", "unified-delay"}, {"core", "keep-alive-interval"},
	{"cli", "cli-language"}, {"cli", "github-mirror"},
	{"cli", "test-url"}, {"cli", "test-timeout"},
	{"cli", "current-profile"}, {"cli", "current-group"},
	{"timer", "sub-auto-update-enabled"}, {"timer", "sub-auto-update-interval"},
	{"timer", "node-auto-select-enabled"}, {"timer", "node-auto-select-interval"},
	{"timer", "resource-auto-update-enabled"}, {"timer", "resource-auto-update-interval"},
}

func configValue(s *app.Settings, key string) string {
	switch key {
	case "allow-lan":
		return fmt.Sprintf("%v", s.AllowLan)
	case "mixed-port":
		return fmt.Sprintf("%d", s.MixedPort)
	case "proxy-mode":
		if s.ProxyMode == "" {
			return "rule (sub default)"
		}
		return s.ProxyMode
	case "ipv6-enabled":
		return fmt.Sprintf("%v", s.IPV6Enabled)
	case "log-level":
		if s.LogLevel == "" {
			return "info (default)"
		}
		return s.LogLevel
	case "cli-language":
		if s.CLILanguage == "" {
			return i18n.Lang() + " (auto)"
		}
		return s.CLILanguage
	case "github-mirror":
		return orDash2(s.GithubMirror)
	case "test-url":
		return s.TestURL
	case "test-timeout":
		return fmt.Sprintf("%dms", s.TestTimeout)
	case "current-profile":
		return orDash2(s.CurrentProfile)
	case "current-group":
		return orDash2(s.CurrentGroup)
	case "tcp-concurrent":
		return triBool(s.TCPConcurrent)
	case "unified-delay":
		return triBool(s.UnifiedDelay)
	case "keep-alive-interval":
		if s.KeepAliveInterval == nil {
			return T("跟随订阅")
		}
		return fmt.Sprintf("%ds", *s.KeepAliveInterval)
	case "sub-auto-update-enabled":
		return fmt.Sprintf("%v", s.SubAutoUpdateEnabled)
	case "sub-auto-update-interval":
		return s.SubAutoUpdateInterval.String()
	case "node-auto-select-enabled":
		return fmt.Sprintf("%v", s.NodeAutoSelectEnabled)
	case "node-auto-select-interval":
		return s.NodeAutoSelectInterval.String()
	case "resource-auto-update-enabled":
		return fmt.Sprintf("%v", s.ResourceAutoUpdateEnabled)
	case "resource-auto-update-interval":
		return s.ResourceAutoUpdateInterval.String()
	}
	return "?"
}

func triBool(p *bool) string {
	if p == nil {
		return T("跟随订阅")
	}
	return fmt.Sprintf("%v", *p)
}

func orDash2(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: T("查看配置 (无参数 = 全部, 按类别分组)"),
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if len(args) == 1 {
			for _, e := range configKeys {
				if e.key == args[0] {
					fmt.Println(configValue(s, e.key))
					return nil
				}
			}
			return fmt.Errorf("%s %q", T("未知配置项"), args[0])
		}
		// 每类一张表 (各自对齐), 含 DEFAULT 列
		cat := ""
		var rows [][]string
		flush := func() {
			if len(rows) > 1 {
				ui.Table(os.Stdout, rows, 2)
			}
		}
		for _, e := range configKeys {
			if e.cat != cat {
				flush()
				cat = e.cat
				rows = [][]string{{"KEY (" + configCatName(cat) + ")", "VALUE", "DEFAULT"}}
			}
			def := configDefaults[e.key]
			if def == "" {
				def = "/"
			}
			rows = append(rows, []string{e.key, configValue(s, e.key), def})
		}
		flush()
		return nil
	},
}

func configCatName(c string) string {
	switch c {
	case "core":
		return T("内核 (config.yaml)")
	case "cli":
		return T("cli 自身")
	case "timer":
		return T("定时任务 (systemd)")
	}
	return c
}

// ---- set ----

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: T("修改配置并生效"),
	Long: `-- ` + T("内核 (config.yaml)") + ` --
allow-lan <true|false>                    ` + T("允许局域网设备使用代理 (0.0.0.0)") + `
mixed-port <port>                         ` + T("混合代理端口 (http+socks5)") + ", " + T("默认 7890") + `
proxy-mode <rule|global|direct>           ` + T("代理模式 (热切换)") + `
ipv6-enabled <true|false>                 ` + T("启用 IPv6") + `
log-level <debug|info|warning|error|silent>   ` + T("内核日志等级") + `
tcp-concurrent <true|false>               ` + T("TCP 并发连接") + `
unified-delay <true|false>                ` + T("统一延迟计算 (URL-Test 更精准)") + `
keep-alive-interval <1-600>               ` + T("长连接保活间隔 (秒)") + `

-- ` + T("cli 自身") + ` --
cli-language <zh|en>                      ` + T("输出语言 (默认按系统 locale, 回退中文)") + `
github-mirror <url>                       ` + T("GitHub 镜像站前缀 (空=自动尝试)") + `
test-url <url> / test-timeout <ms>        ` + T("测速 URL / 超时") + `

-- ` + T("定时任务 (systemd)") + ` --
sub-auto-update-enabled <bool>            ` + T("订阅定时自动更新") + `
sub-auto-update-interval <dur>            ` + T("订阅自动更新周期") + ", " + T("如 12h / 30m") + `
node-auto-select-enabled <bool>           ` + T("定时对当前分组自动择优") + `
node-auto-select-interval <dur>           ` + T("自动择优周期") + ", " + T("如 15m") + `
resource-auto-update-enabled <bool>       ` + T("定时更新 geo 资源 (mmdb/asn/geoip/geosite)") + `
resource-auto-update-interval <dur>       ` + T("资源更新周期") + ", " + T("如 12h"),
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		k := args[0]
		if len(args) == 1 {
			return configKeyHelp(k)
		}
		s := mustSettings()
		v := args[1]
		b := func() bool {
			return v == "true" || v == "on" || v == "yes" || v == "1"
		}
		switch k {
		case "cli-language":
			if v != "zh" && v != "en" {
				return fmt.Errorf("%s", T("cli-language 仅支持 zh / en"))
			}
			s.CLILanguage = v
			i18n.Set(v)
		case "allow-lan":
			s.AllowLan = b()
		case "mixed-port":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("%s", T("无效端口"))
			}
			s.MixedPort = n
		case "proxy-mode":
			if v != "rule" && v != "global" && v != "direct" {
				return fmt.Errorf("%s", T("proxy-mode 仅支持 rule / global / direct"))
			}
			s.ProxyMode = v
		case "ipv6-enabled":
			s.IPV6Enabled = b()
		case "log-level":
			switch v {
			case "debug", "info", "warning", "error", "silent":
			default:
				return fmt.Errorf("%s: debug/info/warning/error/silent", T("无效日志等级"))
			}
			s.LogLevel = v
		case "github-mirror":
			s.GithubMirror = v
		case "tcp-concurrent":
			bv := b()
			s.TCPConcurrent = &bv
		case "unified-delay":
			bv := b()
			s.UnifiedDelay = &bv
		case "keep-alive-interval":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 600 {
				return fmt.Errorf("%s (1-600)", T("无效保活间隔"))
			}
			s.KeepAliveInterval = &n
		case "resource-auto-update-enabled":
			s.ResourceAutoUpdateEnabled = b()
		case "resource-auto-update-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Hour {
				return fmt.Errorf("%s (>=1h)", T("无效周期"))
			}
			s.ResourceAutoUpdateInterval = d
		case "sub-auto-update-enabled":
			s.SubAutoUpdateEnabled = b()
		case "sub-auto-update-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("%s", T("无效周期 (>=1m), 如 12h"))
			}
			s.SubAutoUpdateInterval = d
		case "node-auto-select-enabled":
			s.NodeAutoSelectEnabled = b()
		case "node-auto-select-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("%s", T("无效周期 (>=1m), 如 15m"))
			}
			s.NodeAutoSelectInterval = d
		case "test-url":
			s.TestURL = v
		case "test-timeout":
			n, err := strconv.Atoi(v)
			if err != nil || n < 100 {
				return fmt.Errorf("%s", T("无效超时"))
			}
			s.TestTimeout = n
		default:
			return fmt.Errorf("%s %q (mihomo-cli config set --help)", T("未知配置项"), k)
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s %s\n", k, v, T("已保存"))
		// 轻量项仅 PATCH, 不整文件 reload
		if k == "proxy-mode" && sysd.IsActive() {
			if err := api.New(s).SetMode(v); err != nil {
				return err
			}
			fmt.Println(T("已热切换"))
			return nil
		}
		if k == "log-level" && sysd.IsActive() {
			if err := api.New(s).PatchConfig(map[string]any{"log-level": v}); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 热重载失败"), err)
			} else {
				fmt.Println(T("已热切换"))
				if s.Current() != nil {
					_ = render.Generate(s) // 同步 runtime 文件 (不再 reload)
				}
			}
			return nil
		}
		// 重建运行配置 + 定时器 + 热重载
		if s.Current() != nil {
			if err := render.Generate(s); err == nil {
				reloadIfActive(s)
			}
		}
		if err := sysd.InstallTimers(s); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 更新定时器失败"), err)
		}
		return nil
	},
}

// configKeyHelp 单键详情
func configKeyHelp(k string) error {
	for _, e := range configKeys {
		if e.key == k {
			s := mustSettings()
			fmt.Printf("[%s] %s\n  %s: %s\n  %s: %s\n",
				configCatName(e.cat), k,
				T("当前值"), configValue(s, k),
				T("用法"), "mihomo-cli config set "+k+" <"+T("值")+">")
			return nil
		}
	}
	return fmt.Errorf("%s %q", T("未知配置项"), k)
}

// ---- reset-default ----

var configResetCmd = &cobra.Command{
	Use:   "reset-default [key|all]",
	Short: T("恢复默认值 (仅对有默认值的配置项生效)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		target := "all"
		if len(args) == 1 {
			target = args[0]
		}
		reset := func(k string) bool {
			switch k {
			case "allow-lan":
				s.AllowLan = false
			case "mixed-port":
				s.MixedPort = 7890
			case "proxy-mode", "log-level":
				// 空 = 跟随订阅
			case "ipv6-enabled":
				s.IPV6Enabled = false
			case "sub-auto-update-enabled":
				s.SubAutoUpdateEnabled = true
			case "sub-auto-update-interval":
				s.SubAutoUpdateInterval = 24 * time.Hour
			case "node-auto-select-enabled":
				s.NodeAutoSelectEnabled = false
			case "node-auto-select-interval":
				s.NodeAutoSelectInterval = 30 * time.Minute
			case "test-url":
				s.TestURL = "https://www.gstatic.com/generate_204"
			case "test-timeout":
				s.TestTimeout = 5000
			default:
				return false
			}
			fmt.Printf("%s -> %s\n", k, T("默认值"))
			return true
		}
		if target == "all" {
			for _, e := range configKeys {
				reset(e.key)
			}
		} else if !reset(target) {
			return fmt.Errorf("%s %q (%s)", T("该项无默认值或不存在"), target, "reset-default all")
		}
		if err := s.Save(); err != nil {
			return err
		}
		if s.Current() != nil {
			if err := render.Generate(s); err == nil {
				reloadIfActive(s)
			}
		}
		return nil
	},
}

// ---- sync ----

var configSyncCmd = &cobra.Command{
	Use:   "sync [update-file|update-service] [key...]",
	Short: T("配置一致性: 检查/以文件覆盖服务/以运行状态覆盖文件"),
	Long: T("配置以 config.toml 为唯一权威; sync 检测文件(渲染后的 runtime 配置)与内核运行状态的差异。") + `
mihomo-cli config sync                # ` + T("三列对比: 文件值 / 运行值 / 一致性") + `
mihomo-cli config sync update-file [key...]    # ` + T("用内核运行状态覆盖配置文件 (可指定键)") + `
mihomo-cli config sync update-service [key...] # ` + T("用配置文件覆盖服务 (可指定键; 整体重启)"),
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		mode := ""
		var keys []string
		if len(args) > 0 && (args[0] == "update-file" || args[0] == "update-service") {
			mode = args[0]
			keys = args[1:]
		}
		live := liveCoreConfig(s)
		fileVals := syncFileValues(s)

		if mode == "update-service" {
			if len(keys) > 0 {
				return fmt.Errorf("%s", T("按键覆盖服务暂不支持, 请整体 update-service"))
			}
			if err := render.Generate(s); err != nil {
				return err
			}
			if err := sysd.Service("restart"); err != nil {
				return err
			}
			fmt.Println(T("已用配置文件重启服务"))
			return nil
		}
		if mode == "update-file" {
			// 仅把指定(或全部)可回写键从内核运行状态写回文件
			apply := func(k string) {
				switch k {
				case "proxy-mode":
					s.ProxyMode = live["proxy-mode"]
				case "log-level":
					s.LogLevel = live["log-level"]
				case "allow-lan":
					s.AllowLan = live["allow-lan"] == "true"
				case "ipv6-enabled":
					s.IPV6Enabled = live["ipv6-enabled"] == "true"
				case "mixed-port":
					if n, err := strconv.Atoi(live["mixed-port"]); err == nil && n > 0 {
						s.MixedPort = n
					}
				}
			}
			if len(keys) > 0 {
				for _, k := range keys {
					apply(k)
					fmt.Printf("%s <- %s\n", k, live[k])
				}
			} else {
				for k := range fileVals {
					apply(k)
				}
			}
			if err := s.Save(); err != nil {
				return err
			}
			fmt.Println(T("已用内核运行状态覆盖配置文件"))
			return nil
		}

		// 三列对比: 文件 / 运行 / 结果(绿同红异)
		rows := [][]string{{"KEY", T("配置文件"), T("内核运行"), T("比对")}}
		same, diff := 0, 0
		for _, k := range []string{"proxy-mode", "mixed-port", "allow-lan", "ipv6-enabled", "log-level"} {
			fv, lv := fileVals[k], live[k]
			mark := "\x1b[32m" + T("一致") + "\x1b[0m"
			if fv != lv {
				mark = "\x1b[31m" + T("不同") + "\x1b[0m"
				diff++
			} else {
				same++
			}
			rows = append(rows, []string{k, fv, lv, mark})
		}
		ui.Table(os.Stdout, rows, 2)
		if diff > 0 {
			fmt.Println(T("提示: config sync update-service 以文件覆盖服务; config sync update-file 以运行状态覆盖文件"))
		}
		_ = same
		return nil
	},
}

// liveCoreConfig 读取内核运行配置关键字段
func liveCoreConfig(s *app.Settings) map[string]string {
	var live struct {
		Mode      string `json:"mode"`
		MixedPort int    `json:"mixed-port"`
		AllowLan  bool   `json:"allow-lan"`
		IPV6      bool   `json:"ipv6"`
		LogLevel  string `json:"log-level"`
	}
	m := map[string]string{}
	if err := api.New(s).GetJSONStruct("/configs", &live); err != nil {
		return m
	}
	m["proxy-mode"] = live.Mode
	m["mixed-port"] = strconv.Itoa(live.MixedPort)
	m["allow-lan"] = fmt.Sprintf("%v", live.AllowLan)
	m["ipv6-enabled"] = fmt.Sprintf("%v", live.IPV6)
	m["log-level"] = live.LogLevel
	return m
}

// syncFileValues 计算配置文件侧的关键字段期望值
func syncFileValues(s *app.Settings) map[string]string {
	mode := s.ProxyMode
	if mode == "" {
		mode = runtimeYAMLKey("mode")
	}
	if mode == "" {
		mode = "rule"
	}
	level := s.LogLevel
	if level == "" {
		level = "info"
	}
	return map[string]string{
		"proxy-mode":    mode,
		"mixed-port":    strconv.Itoa(s.MixedPort),
		"allow-lan":     fmt.Sprintf("%v", s.AllowLan),
		"ipv6-enabled":  fmt.Sprintf("%v", s.IPV6Enabled),
		"log-level":     level,
	}
}

// runtimeYAMLKey 读 runtime/config.yaml 顶层键
func runtimeYAMLKey(key string) string {
	data, err := os.ReadFile(app.RuntimeConfig)
	if err != nil {
		return ""
	}
	var m map[string]any
	_ = yaml.Unmarshal(data, &m)
	v, _ := m[key].(string)
	return v
}

var _ = http.StatusOK

func configKeyComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		var out []string
		for _, e := range configKeys {
			if strings.HasPrefix(e.key, toComplete) {
				out = append(out, e.key)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func configValueComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 1 {
		switch args[0] {
		case "cli-language":
			return []string{"zh", "en"}, cobra.ShellCompDirectiveNoFileComp
		case "allow-lan", "ipv6-enabled", "sub-auto-update-enabled", "node-auto-select-enabled":
			return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
		case "proxy-mode":
			return []string{"rule", "global", "direct"}, cobra.ShellCompDirectiveNoFileComp
		case "log-level":
			return []string{"debug", "info", "warning", "error", "silent"}, cobra.ShellCompDirectiveNoFileComp
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	configSetCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// config set <key> -h 只显示该键详情; 无 key 显示分组帮助
		if pos := cmd.Flags().Args(); len(pos) > 0 {
			_ = configKeyHelp(pos[0])
			return
		}
		fmt.Println(cmd.Long)
	})
	configGetCmd.ValidArgsFunction = configKeyComp
	configSetCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if out, d := configKeyComp(cmd, args, toComplete); len(args) == 0 {
			return out, d
		}
		return configValueComp(cmd, args, toComplete)
	}
	configSyncCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			var out []string
			for _, m := range []string{"update-file", "update-service"} {
				if strings.HasPrefix(m, toComplete) {
					out = append(out, m)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd, configSyncCmd)
	rootCmd.AddCommand(configCmd)
}
