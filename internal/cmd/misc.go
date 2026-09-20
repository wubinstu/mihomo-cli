package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
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

// ---- conn ----

var connWatch bool

var connCmd = &cobra.Command{
	Use:   "conn",
	Short: T("查看活动连接 (--watch 持续刷新)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		for {
			if err := printConns(api.New(s)); err != nil {
				return err
			}
			if !connWatch {
				return nil
			}
			time.Sleep(2 * time.Second)
			fmt.Print("\033[H\033[2J")
		}
	},
}

func printConns(c *api.Client) error {
	r, err := c.Connections()
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d  %s ↑%s ↓%s\n", T("活动连接"), len(r.Connections), T("累计"),
		humanBytes(r.UploadTotal), humanBytes(r.DownloadTotal))
	if len(r.Connections) == 0 {
		return nil
	}
	rows := [][]string{{T("网络"), T("目标"), T("代理链"), "↑", "↓"}}
	for _, cn := range r.Connections {
		host := cn.Metadata.Host
		if host == "" {
			host = cn.Metadata.Destination
		}
		rows = append(rows, []string{
			cn.Metadata.Network, host,
			strings.Join(cn.Chains, "/"),
			humanBytes(cn.Upload), humanBytes(cn.Download),
		})
	}
	ui.Table(os.Stdout, rows, 2)
	return nil
}

// ---- traffic ----

var trafficCmd = &cobra.Command{
	Use:   "traffic",
	Short: T("实时上下行流量 (Ctrl-C 退出)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		resp, err := api.New(s).Stream("/traffic")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		fmt.Println(T("实时流量 (每秒):"))
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			var t struct {
				Up   int64 `json:"up"`
				Down int64 `json:"down"`
			}
			if json.Unmarshal(sc.Bytes(), &t) == nil {
				fmt.Printf("\r↑ %8s/s   ↓ %8s/s   ", humanBytes(t.Up), humanBytes(t.Down))
			}
		}
		return nil
	},
}

// ---- log ----

var logFollow bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: T("查看内核日志 (-f 跟随)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(app.LogFile); err != nil {
			return fmt.Errorf("%s (%s)", T("暂无日志"), app.LogFile)
		}
		c := exec.Command("tail", "-n", "100")
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
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("数据目录"), app.BaseDir)
		v, err := core.Version()
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("内核"), orDash(v, err))
		var pinfo string
		if p := s.Current(); p != nil {
			pinfo = fmt.Sprintf("%s (%d %s, %s)", p.Name, p.Nodes, T("节点"), humanTime(p.UpdatedAt))
		}
		fmt.Printf("%s %-14s %s\n", ok(s.Current() != nil), T("订阅"), orDash(pinfo, nil))
		_, err = os.Stat(app.RuntimeConfig)
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("运行配置"), app.RuntimeConfig)
		active := sysd.IsActive()
		svc := T("未运行 (mihomo-cli start)")
		if active {
			svc = T("运行中")
		}
		fmt.Printf("%s %-14s %s\n", ok(active), T("服务"), svc)
		var apiInfo string
		ver, err := api.New(s).Version()
		if err == nil {
			apiInfo = T("API 正常, 内核") + " " + ver
		}
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("控制API"), orDash(apiInfo, err))
		live := portOpen(fmt.Sprintf("127.0.0.1:%d", s.MixedPort))
		fmt.Printf("%s %-14s 127.0.0.1:%d %s\n", ok(live), T("代理端口"), s.MixedPort, listenWord(live))
		if s.AllowLan {
			live2 := portOpen(fmt.Sprintf("0.0.0.0:%d", s.MixedPort))
			fmt.Printf("%s %-14s 0.0.0.0:%d %s (LAN: http://%s:%d)\n",
				ok(live2), T("局域网"), s.MixedPort, listenWord(live2), lanIP(), s.MixedPort)
		}
		subOn := sysd.TimerEnabled("mihomo-cli-sub.timer")
		fmt.Printf("%s %-14s %s (%s %s)\n", ok(subOn == s.SubAutoUpdateEnabled),
			T("订阅自动更新"), onOff(subOn), T("周期"), s.SubAutoUpdateInterval)
		autoOn := sysd.TimerEnabled("mihomo-cli-auto.timer")
		fmt.Printf("%s %-14s %s (%s %s)\n", ok(autoOn == s.ProxyAutoSelectEnabled),
			T("自动择优节点"), onOff(autoOn), T("周期"), s.ProxyAutoSelectInterval)
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
mixed-port <port>                ` + T("混合代理端口 (默认 7890)") + `
proxy-mode <rule|global|direct>  ` + T("代理模式 (热切换)") + `
sub-auto-update-enabled <bool>   ` + T("订阅自动更新开关") + `
sub-auto-update-interval <dur>   ` + T("订阅自动更新周期") + `, 如 12h / 30m
proxy-auto-select-enabled <bool>   ` + T("自动切换到最低延迟节点") + ` (作用于当前分组)
proxy-auto-select-interval <dur>   ` + T("自动择优周期") + `, 如 15m
test-url <url>                   ` + T("测速 URL") + `
test-timeout <ms>                ` + T("测速超时(毫秒)") + `
download-proxy <url>             ` + T("下载内核/订阅使用的代理 (空=直连)") + `

` + T("旧键名 auto-select-* / sub-auto-update / sub-interval 仍被接受。"),
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		k, v := args[0], args[1]
		// 兼容旧键名
		switch k {
		case "sub-auto-update":
			k = "sub-auto-update-enabled"
		case "sub-interval":
			k = "sub-auto-update-interval"
		case "auto-select":
			k = "proxy-auto-select-enabled"
		case "auto-interval":
			k = "proxy-auto-select-interval"
		}
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
		case "proxy-auto-select-enabled":
			s.ProxyAutoSelectEnabled = b()
		case "proxy-auto-select-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("%s", T("无效周期 (>=1m), 如 15m"))
			}
			s.ProxyAutoSelectInterval = d
		case "test-url":
			s.TestURL = v
		case "test-timeout":
			n, err := strconv.Atoi(v)
			if err != nil || n < 100 {
				return fmt.Errorf("%s", T("无效超时"))
			}
			s.TestTimeout = n
		case "download-proxy":
			s.DownloadProxy = v
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
			{"proxy-auto-select-enabled", fmt.Sprintf("%v", s.ProxyAutoSelectEnabled)},
			{"proxy-auto-select-interval", s.ProxyAutoSelectInterval.String()},
			{"test-url", s.TestURL},
			{"test-timeout", fmt.Sprintf("%dms", s.TestTimeout)},
			{"download-proxy", s.DownloadProxy},
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
		s := mustSettings()
		if err := core.Upgrade(s.DownloadProxy, false); err != nil {
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

var Version = "0.2.0"

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

func init() {
	connCmd.Flags().BoolVar(&connWatch, "watch", false, T("持续刷新"))
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, T("跟随日志"))
	coreCmd.AddCommand(coreVersionCmd, coreUpgradeCmd, coreRollbackCmd)
	rootCmd.AddCommand(connCmd, trafficCmd, logCmd, doctorCmd, setCmd, getCmd, coreCmd, versionCmd)
}
