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
var configKeys = []cfgEntry{
	{"core", "allow-lan"}, {"core", "mixed-port"}, {"core", "proxy-mode"},
	{"core", "ipv6-enabled"}, {"core", "log-level"},
	{"cli", "cli-language"}, {"cli", "install-mirror"},
	{"cli", "test-url"}, {"cli", "test-timeout"},
	{"cli", "current-profile"}, {"cli", "current-group"},
	{"timer", "sub-auto-update-enabled"}, {"timer", "sub-auto-update-interval"},
	{"timer", "node-auto-select-enabled"}, {"timer", "node-auto-select-interval"},
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
	case "install-mirror":
		return orDash2(s.InstallMirror)
	case "test-url":
		return s.TestURL
	case "test-timeout":
		return fmt.Sprintf("%dms", s.TestTimeout)
	case "current-profile":
		return orDash2(s.CurrentProfile)
	case "current-group":
		return orDash2(s.CurrentGroup)
	case "sub-auto-update-enabled":
		return fmt.Sprintf("%v", s.SubAutoUpdateEnabled)
	case "sub-auto-update-interval":
		return s.SubAutoUpdateInterval.String()
	case "node-auto-select-enabled":
		return fmt.Sprintf("%v", s.NodeAutoSelectEnabled)
	case "node-auto-select-interval":
		return s.NodeAutoSelectInterval.String()
	}
	return "?"
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
		cat := ""
		rows := [][]string{{"KEY", "VALUE"}}
		flush := func() {
			if len(rows) > 1 {
				ui.Table(os.Stdout, rows, 3)
			}
		}
		for _, e := range configKeys {
			if e.cat != cat {
				flush()
				cat = e.cat
				fmt.Printf("\n== %s ==\n", configCatName(cat))
				rows = [][]string{{"KEY", "VALUE"}}
			}
			rows = append(rows, []string{e.key, configValue(s, e.key)})
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
	Long: `cli-language <zh|en>             ` + T("输出语言 (默认按系统 locale, 回退中文)") + `
allow-lan <true|false>           ` + T("允许局域网设备使用代理 (0.0.0.0)") + `
mixed-port <port>                ` + T("混合代理端口 (http+socks5)") + ", " + T("默认 7890") + `
proxy-mode <rule|global|direct>  ` + T("代理模式 (热切换)") + `
ipv6-enabled <true|false>        ` + T("启用 IPv6") + `
log-level <debug|info|warning|error|silent>   ` + T("内核日志等级") + `
install-mirror <url>             ` + T("GitHub 镜像站前缀 (空=自动尝试)") + `
sub-auto-update-enabled <bool>   ` + T("订阅定时自动更新") + `
sub-auto-update-interval <dur>   ` + T("订阅自动更新周期") + ", " + T("如 12h / 30m") + `
node-auto-select-enabled <bool>  ` + T("定时对当前分组自动择优") + `
node-auto-select-interval <dur>  ` + T("自动择优周期") + ", " + T("如 15m") + `
test-url <url>                   ` + T("测速 URL") + `
test-timeout <ms>                ` + T("测速超时(毫秒)"),
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
		case "install-mirror":
			s.InstallMirror = v
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
	Use:   "sync [update-file|update-service]",
	Short: T("配置一致性: 检查/以文件覆盖服务/以运行状态覆盖文件"),
	Long: T("配置以 config.toml 为唯一权威; sync 检测文件(渲染后的 runtime 配置)与内核运行状态的差异。") + `
mihomo-cli config sync                # ` + T("仅检查并显示差异") + `
mihomo-cli config sync update-file    # ` + T("用内核运行状态覆盖配置文件") + `
mihomo-cli config sync update-service # ` + T("用配置文件覆盖服务 (重新渲染并重启)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		mode := ""
		if len(args) == 1 {
			mode = args[0]
		}
		if mode == "update-service" {
			if err := render.Generate(s); err != nil {
				return err
			}
			if err := sysd.Service("restart"); err != nil {
				return err
			}
			fmt.Println(T("已用配置文件重启服务"))
			return nil
		}
		// 读取内核运行状态
		c := api.New(s)
		var live struct {
			Mode     string `json:"mode"`
			MixedPort int  `json:"mixed-port"`
			AllowLan bool  `json:"allow-lan"`
			IPV6     bool  `json:"ipv6"`
			LogLevel string `json:"log-level"`
		}
		if err := c.GetJSONStruct("/configs", &live); err != nil {
			return err
		}
		if mode == "update-file" {
			// 内核运行状态回写文件
			if live.Mode != "" {
				s.ProxyMode = live.Mode
			}
			if live.LogLevel != "" {
				s.LogLevel = live.LogLevel
			}
			s.AllowLan = live.AllowLan
			s.IPV6Enabled = live.IPV6
			if live.MixedPort > 0 {
				s.MixedPort = live.MixedPort
			}
			if err := s.Save(); err != nil {
				return err
			}
			fmt.Println(T("已用内核运行状态覆盖配置文件"))
			return nil
		}
		// 比对
		wantMode := s.ProxyMode
		if wantMode == "" {
			wantMode = runtimeYAMLKey("mode")
		}
		wantLevel := s.LogLevel
		if wantLevel == "" {
			wantLevel = "info"
		}
		type diff struct{ k, want, got string }
		var diffs []diff
		if wantMode != live.Mode {
			diffs = append(diffs, diff{"proxy-mode", wantMode, live.Mode})
		}
		if s.MixedPort != live.MixedPort {
			diffs = append(diffs, diff{"mixed-port", strconv.Itoa(s.MixedPort), strconv.Itoa(live.MixedPort)})
		}
		if s.AllowLan != live.AllowLan {
			diffs = append(diffs, diff{"allow-lan", fmt.Sprintf("%v", s.AllowLan), fmt.Sprintf("%v", live.AllowLan)})
		}
		if s.IPV6Enabled != live.IPV6 {
			diffs = append(diffs, diff{"ipv6-enabled", fmt.Sprintf("%v", s.IPV6Enabled), fmt.Sprintf("%v", live.IPV6)})
		}
		if wantLevel != live.LogLevel {
			diffs = append(diffs, diff{"log-level", wantLevel, live.LogLevel})
		}
		if len(diffs) == 0 {
			fmt.Println(T("配置一致"))
			return nil
		}
		rows := [][]string{{"KEY", T("配置文件"), T("内核运行")}}
		for _, d := range diffs {
			rows = append(rows, []string{d.k, d.want, d.got})
		}
		ui.Table(os.Stdout, rows, 3)
		fmt.Println(T("提示: config sync update-service 以文件覆盖服务; config sync update-file 以运行状态覆盖文件"))
		return nil
	},
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
	configGetCmd.ValidArgsFunction = configKeyComp
	configSetCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if out, d := configKeyComp(cmd, args, toComplete); len(args) == 0 {
			return out, d
		}
		return configValueComp(cmd, args, toComplete)
	}
	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd, configSyncCmd)
	rootCmd.AddCommand(configCmd)
}
