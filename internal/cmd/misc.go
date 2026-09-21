package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// ---- log ----

var logFollow bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: T("查看内核日志 (-f 跟随)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(app.LogFile); err != nil {
			return fmt.Errorf("%s (%s)", T("暂无日志"), app.LogFile)
		}
		c := exec.Command("tail", "-n", "100", app.LogFile)
		if logFollow {
			c = exec.Command("tail", "-n", "100", "-f", app.LogFile)
		}
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

// ---- doctor ----

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: T("体检: 内核/服务/端口/API/订阅/定时器"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		ok := func(good bool) string {
			if good {
				return "✔"
			}
			return "✘"
		}
		err := app.EnsureDirs()
		fmt.Printf("%s %-16s %s\n", ok(err == nil), T("数据目录"), app.BaseDir)
		v, err := core.Version()
		fmt.Printf("%s %-16s %s\n", ok(err == nil), T("内核"), orDash(v, err))
		var pinfo string
		if p := s.Current(); p != nil {
			pinfo = fmt.Sprintf("%s (%d %s, %s)", p.Name, p.Nodes, T("节点"), humanTime(p.UpdatedAt))
		}
		fmt.Printf("%s %-16s %s\n", ok(s.Current() != nil), T("订阅"), orDash(pinfo, nil))
		_, err = os.Stat(app.RuntimeConfig)
		fmt.Printf("%s %-16s %s\n", ok(err == nil), T("运行配置"), app.RuntimeConfig)
		// geo 数据 (缺失会导致内核启动 fatal 循环)
		geoOK := false
		if _, err := os.Stat(app.RuntimeDir + "/geoip.metadb"); err == nil {
			geoOK = true
		}
		fmt.Printf("%s %-16s %s\n", ok(geoOK), "geo " + T("数据"), map[bool]string{
			true: T("已下载"), false: T("缺失 (mihomo-cli core geo)")}[geoOK])
		active := sysd.IsActive()
		svc := T("未运行 (mihomo-cli start)")
		if active {
			svc = T("运行中")
		}
		fmt.Printf("%s %-16s %s\n", ok(active), T("服务"), svc)
		var apiInfo string
		ver, err := api.New(s).Version()
		if err == nil {
			apiInfo = T("API 正常, 内核") + " " + ver
		}
		fmt.Printf("%s %-16s %s\n", ok(err == nil), T("控制API"), orDash(apiInfo, err))
		live := portOpen(fmt.Sprintf("127.0.0.1:%d", s.MixedPort))
		fmt.Printf("%s %-16s 127.0.0.1:%d %s\n", ok(live), T("代理端口"), s.MixedPort, listenWord(live))
		if s.AllowLan {
			live2 := portOpen(fmt.Sprintf("0.0.0.0:%d", s.MixedPort))
			fmt.Printf("%s %-16s 0.0.0.0:%d %s (LAN: http://%s:%d)\n",
				ok(live2), T("局域网"), s.MixedPort, listenWord(live2), lanIP(), s.MixedPort)
		}
		subOn := sysd.TimerEnabled("mihomo-cli-sub.timer")
		fmt.Printf("%s %-16s %s (%s %s)\n", ok(subOn == s.SubAutoUpdateEnabled),
			T("订阅自动更新"), onOff(subOn), T("周期"), s.SubAutoUpdateInterval)
		autoOn := sysd.TimerEnabled("mihomo-cli-auto.timer")
		fmt.Printf("%s %-16s %s (%s %s)\n", ok(autoOn == s.NodeAutoSelectEnabled),
			T("节点自动择优"), onOff(autoOn), T("周期"), s.NodeAutoSelectInterval)
		fmt.Println(T("提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli proxy on))"))
		return nil
	},
}

func onOff(b bool) string {
	if b {
		return T("已启用")
	}
	return T("已停用")
}

func listenWord(live bool) string {
	if live {
		return T("监听中")
	}
	return T("未监听")
}

func orDash(s string, err error) string {
	if s == "" {
		if err != nil {
			return err.Error()
		}
		return "-"
	}
	return s
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// ---- set / get ----

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: T("修改设置并生效"),
	Long: `lang <zh|en>                     ` + T("输出语言 (默认按系统 locale, 回退中文)") + `
allow-lan <true|false>           ` + T("允许局域网设备使用代理 (0.0.0.0)") + `
mixed-port <port>                ` + T("混合代理端口 (http+socks5)") + ", " + T("默认 7890") + `
proxy-mode <rule|global|direct>  ` + T("代理模式 (热切换)") + `
sub-auto-update-enabled <bool>   ` + T("订阅定时自动更新") + `
sub-auto-update-interval <dur>   ` + T("订阅自动更新周期") + ", " + T("如 12h / 30m") + `
node-auto-select-enabled <bool>   ` + T("自动切换到最低延迟节点") + " (" + T("定时对当前分组自动择优") + ")" + `
node-auto-select-interval <dur>   ` + T("自动择优周期") + ", " + T("如 15m") + `
test-url <url>                   ` + T("测速 URL") + `
test-timeout <ms>                ` + T("测速超时(毫秒)"),
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		k := args[0]
		if len(args) == 1 {
			return setKeyHelp(k)
		}
		s := mustSettings()
		v := args[1]
		b := func() bool {
			return v == "true" || v == "on" || v == "yes" || v == "1"
		}
		switch k {
		case "lang":
			if v != "zh" && v != "en" {
				return fmt.Errorf("%s", T("lang 仅支持 zh / en"))
			}
			s.Lang = v
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
			return fmt.Errorf("%s %q (mihomo-cli set --help)", T("未知配置项"), k)
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s %s\n", k, v, T("已保存"))
		// proxy-mode 轻量热切换, 不做整配置 reload
		if k == "proxy-mode" && sysd.IsActive() {
			if err := api.New(s).SetMode(v); err != nil {
				return err
			}
			fmt.Println(T("已热重载配置"))
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

var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: T("查看设置 (无参数 = 全部)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		lang := s.Lang
		if lang == "" {
			lang = i18n.Lang() + " (auto)"
		}
		mode := s.ProxyMode
		if mode == "" {
			mode = "rule (sub default)"
		}
		all := [][2]string{
			{"lang", lang},
			{"allow-lan", fmt.Sprintf("%v", s.AllowLan)},
			{"mixed-port", fmt.Sprintf("%d", s.MixedPort)},
			{"proxy-mode", mode},
			{"sub-auto-update-enabled", fmt.Sprintf("%v", s.SubAutoUpdateEnabled)},
			{"sub-auto-update-interval", s.SubAutoUpdateInterval.String()},
			{"node-auto-select-enabled", fmt.Sprintf("%v", s.NodeAutoSelectEnabled)},
			{"node-auto-select-interval", s.NodeAutoSelectInterval.String()},
			{"test-url", s.TestURL},
			{"test-timeout", fmt.Sprintf("%dms", s.TestTimeout)},
			{"current-profile", s.CurrentProfile},
			{"current-group", s.CurrentGroup},
		}
		if len(args) == 0 {
			rows := [][]string{{"KEY", "VALUE"}}
			for _, e := range all {
				rows = append(rows, []string{e[0], e[1]})
			}
			ui.Table(os.Stdout, rows, 3)
			return nil
		}
		for _, e := range all {
			if e[0] == args[0] {
				fmt.Println(e[1])
				return nil
			}
		}
		return fmt.Errorf("%s %q", T("未知配置项"), args[0])
	},
}

// setKeyDocs 每个配置项的详细说明 (供 set <key> 单参数时显示)
var setKeyDocs = map[string][2]string{
	"lang":                        {T("输出语言"), "zh(" + T("中文") + ") | en(" + T("英文") + "); " + T("默认按系统 locale, 回退中文")},
	"allow-lan":                   {T("允许局域网设备使用代理"), "true | false; true " + T("时监听 0.0.0.0")},
	"mixed-port":                  {T("混合代理端口 (http+socks5)"), "1-65535; " + T("默认 7890")},
	"proxy-mode":                  {T("代理模式"), "rule(" + T("规则分流") + ") | global(" + T("全部走当前选中节点") + ") | direct(" + T("全部直连") + ")"},
	"sub-auto-update-enabled":     {T("订阅定时自动更新"), "true | false"},
	"sub-auto-update-interval":    {T("订阅自动更新周期"), "1m-720h; " + T("如 12h / 30m")},
	"node-auto-select-enabled":   {T("定时对当前分组自动择优"), "true | false; " + T("作用于当前 group use 的分组")},
	"node-auto-select-interval":  {T("自动择优周期"), "1m-720h; " + T("如 15m")},
	"test-url":                    {T("测速 URL"), "http(s)://...; " + T("建议 204 端点")},
	"test-timeout":                {T("测速超时(毫秒)"), "100-60000"},
}

func setKeyHelp(k string) error {
	d, ok := setKeyDocs[k]
	if !ok {
		return fmt.Errorf("%s %q (mihomo-cli set --help)", T("未知配置项"), k)
	}
	s := mustSettings()
	cur := func() string {
		switch k {
		case "lang":
			if s.Lang == "" {
				return i18n.Lang() + " (auto)"
			}
			return s.Lang
		case "allow-lan":
			return fmt.Sprintf("%v", s.AllowLan)
		case "mixed-port":
			return fmt.Sprintf("%d", s.MixedPort)
		case "proxy-mode":
			if s.ProxyMode == "" {
				return "rule (sub default)"
			}
			return s.ProxyMode
		case "sub-auto-update-enabled":
			return fmt.Sprintf("%v", s.SubAutoUpdateEnabled)
		case "sub-auto-update-interval":
			return s.SubAutoUpdateInterval.String()
		case "node-auto-select-enabled":
			return fmt.Sprintf("%v", s.NodeAutoSelectEnabled)
		case "node-auto-select-interval":
			return s.NodeAutoSelectInterval.String()
		case "test-url":
			return s.TestURL
		case "test-timeout":
			return fmt.Sprintf("%dms", s.TestTimeout)
		}
		return "-"
	}()
	fmt.Printf("%s\n  %s: %s\n  %s: %s\n  %s: %s\n  %s: mihomo-cli set %s <%s>\n",
		k, T("说明"), d[0], T("可选值"), d[1], T("当前值"), cur, T("用法"), k, T("值"))
	return nil
}

// settingKeys 有序 key 列表 (get/set 补全与展示)
func settingKeys() []string {
	return []string{
		"lang", "allow-lan", "mixed-port", "proxy-mode",
		"sub-auto-update-enabled", "sub-auto-update-interval",
		"node-auto-select-enabled", "node-auto-select-interval",
		"test-url", "test-timeout",
	}
}

func setValueCandidates(k string) []string {
	switch k {
	case "lang":
		return []string{"zh", "en"}
	case "allow-lan", "sub-auto-update-enabled", "node-auto-select-enabled":
		return []string{"true", "false"}
	case "proxy-mode":
		return []string{"rule", "global", "direct"}
	}
	return nil
}

func setValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		var out []string
		for _, k := range settingKeys() {
			if strings.HasPrefix(k, toComplete) {
				out = append(out, k)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 1 {
		return setValueCandidates(args[0]), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// ---- core ----

var coreCmd = &cobra.Command{
	Use:   "core",
	Short: T("内核管理: version/upgrade/rollback"),
}

var coreVersionCmd = &cobra.Command{
	Use: "version",
	Short: T("已安装内核版本"),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := core.Version()
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	},
}

var coreUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: T("升级内核 (从 GitHub Releases)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := core.Upgrade(dlProxy, false); err != nil {
			return err
		}
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		return nil
	},
}

var coreRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: T("回滚到上一版本"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := core.Rollback(); err != nil {
			return err
		}
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		return nil
	},
}

// ---- version ----

var Version = "0.8.1"

var versionCmd = &cobra.Command{
	Use: "version",
	Short: T("mihomo-cli 版本"),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("mihomo-cli %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	},
}

func humanBytes(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		if f < 1024 {
			return fmt.Sprintf("%.1f%s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1fPiB", f)
}

var dlProxy string

var coreGeoCmd = &cobra.Command{
	Use:   "geo",
	Short: T("下载/更新 geo 数据 (geoip/geosite, 内核规则依赖)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		// 强制刷新: 删除已有文件
		for _, f := range []string{"geoip.metadb", "GeoSite.dat"} {
			_ = os.Remove(filepath.Join(app.RuntimeDir, f))
		}
		if err := core.DownloadGeo(dlProxy); err != nil {
			return err
		}
		fmt.Println(T("已下载"))
		reloadIfActive(s)
		return nil
	},
}

func init() {
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, T("跟随日志"))
	setCmd.ValidArgsFunction = setValidArgs
	getCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			var out []string
			for _, k := range settingKeys() {
				if strings.HasPrefix(k, toComplete) {
					out = append(out, k)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	coreCmd.AddCommand(coreVersionCmd, coreUpgradeCmd, coreRollbackCmd, coreGeoCmd)
	rootCmd.AddCommand(logCmd, doctorCmd, setCmd, getCmd, coreCmd, versionCmd)
}
