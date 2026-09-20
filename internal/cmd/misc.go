package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	Short: "查看活动连接 (--watch 持续刷新)",
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
	fmt.Printf("%s: %d  %s ↑%s ↓%s\n", T("conn.active"), len(r.Connections), T("conn.acc"),
		humanBytes(r.UploadTotal), humanBytes(r.DownloadTotal))
	rows := [][]string{strings.Split(T("conn.hdr"), "\t")}
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
	Short: "实时上下行流量 (Ctrl-C 退出)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		resp, err := api.New(s).Stream("/traffic")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		fmt.Println(T("tr.title"))
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
	Short: "查看内核日志 (-f 跟随)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(app.LogFile); err != nil {
			return fmt.Errorf("暂无日志 (%s)", app.LogFile)
		}
		c := exec.Command("tail", "-n", "100")
		if logFollow {
			c = exec.Command("tail", "-n", "100", "-f", app.LogFile)
		} else {
			c = exec.Command("tail", "-n", "100", app.LogFile)
		}
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

// ---- env ----

var envUnset bool

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "输出代理环境变量 (eval $(mihomo-cli env) 生效)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if envUnset {
			fmt.Println(`unset http_proxy https_proxy all_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY`)
			return nil
		}
		addr := fmt.Sprintf("127.0.0.1:%d", s.MixedPort)
		if s.AllowLan {
			// 输出本机局域网地址方便其他设备引用
			addr = fmt.Sprintf("%s:%d", lanIP(), s.MixedPort)
		}
		fmt.Printf(`export http_proxy=http://%s
export https_proxy=http://%s
export all_proxy=socks5://%s
export HTTP_PROXY=http://%s
export HTTPS_PROXY=http://%s
export ALL_PROXY=socks5://%s
export no_proxy=localhost,127.0.0.1,::1
export NO_PROXY=localhost,127.0.0.1,::1
`, addr, addr, addr, addr, addr, addr)
		return nil
	},
}

func lanIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "0.0.0.0"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// ---- doctor ----

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "体检: 内核/服务/端口/API/订阅/定时器",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		ok := func(good bool) string {
			if good {
				return "✔"
			}
			return "✘"
		}
		// 目录
		err := app.EnsureDirs()
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("dr.dir"), app.BaseDir)
		// 内核
		v, err := core.Version()
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("dr.core"), orDash(v, err))
		// 订阅
		var pinfo string
		if p := s.Current(); p != nil {
			pinfo = fmt.Sprintf("%s (%d nodes, %s)", p.Name, p.Nodes, humanTime(p.UpdatedAt))
		}
		fmt.Printf("%s %-14s %s\n", ok(s.Current() != nil), T("dr.sub"), orDash(pinfo, nil))
		// 运行配置
		_, err = os.Stat(app.RuntimeConfig)
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("dr.rt"), app.RuntimeConfig)
		// 服务
		active := sysd.IsActive()
		svc := T("svc.running")
		if !active {
			svc = T("svc.start.hint")
		}
		fmt.Printf("%s %-14s %s\n", ok(active), T("dr.svc"), svc)
		// API
		var apiInfo string
		ver, err := api.New(s).Version()
		if err == nil {
			apiInfo = T("dr.api.ok") + ver
		}
		fmt.Printf("%s %-14s %s\n", ok(err == nil), T("dr.api"), orDash(apiInfo, err))
		// 代理端口
		live := portOpen(fmt.Sprintf("127.0.0.1:%d", s.MixedPort))
		fmt.Printf("%s %-14s 127.0.0.1:%d %s\n", ok(live), T("dr.port"), s.MixedPort, listenWord(live))
		if s.AllowLan {
			live2 := portOpen(fmt.Sprintf("0.0.0.0:%d", s.MixedPort))
			fmt.Printf("%s %-14s 0.0.0.0:%d %s (LAN: http://%s:%d)\n",
				ok(live2), T("dr.lan"), s.MixedPort, listenWord(live2), lanIP(), s.MixedPort)
		}
		// 定时器
		subOn := sysd.TimerEnabled("mihomo-cli-sub.timer")
		fmt.Printf("%s %-14s %s (%s %s)\n", ok(subOn == s.SubAutoUpdateEnabled),
			T("dr.subau"), onOff(subOn), T("dr.period"), s.SubAutoUpdateInterval)
		autoOn := sysd.TimerEnabled("mihomo-cli-auto.timer")
		fmt.Printf("%s %-14s %s (%s %s, %s %s)\n", ok(autoOn == s.AutoSelectEnabled),
			T("dr.auto"), onOff(autoOn), T("dr.period"), s.AutoSelectInterval,
			T("dr.groups"), orDash(strings.Join(s.AutoGroups, ","), nil))
		// 直连测试
		fmt.Println(T("dr.hint"))
		return nil
	},
}

func onOff(b bool) string {
	if b {
		return T("dr.timer.on")
	}
	return T("dr.timer.off")
}

func listenWord(live bool) string {
	if live {
		return "LISTEN"
	}
	return "-"
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
	Short: "修改设置并生效",
	Long: `可配置项:
  lang <zh|en>                     输出语言 (默认按系统 locale, 回退中文)
  allow-lan <true|false>           允许局域网设备使用代理 (0.0.0.0)
  mixed-port <port>                混合代理端口 (默认 7890)
  sub-auto-update-enabled <bool>   订阅自动更新开关
  sub-auto-update-interval <dur>   订阅自动更新周期, 如 12h / 30m
  auto-select-enabled <bool>       自动切换到最低延迟节点
  auto-select-interval <dur>       自动择优周期, 如 15m
  auto-groups <g1,g2>              自动择优作用的分组 (空=全部含真实节点的分组)
  test-url <url>                   测速 URL
  test-timeout <ms>                测速超时(毫秒)
  download-proxy <url>             下载内核/订阅使用的代理 (空=直连)

旧键名 sub-auto-update / sub-interval / auto-select / auto-interval 仍被接受。`,
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
			k = "auto-select-enabled"
		case "auto-interval":
			k = "auto-select-interval"
		}
		b := func() bool {
			return v == "true" || v == "on" || v == "yes" || v == "1"
		}
		switch k {
		case "lang":
			if v != "zh" && v != "en" {
				return fmt.Errorf("lang 仅支持 zh / en")
			}
			s.Lang = v
			i18n.Set(v)
		case "allow-lan":
			s.AllowLan = b()
		case "mixed-port":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("无效端口")
			}
			s.MixedPort = n
		case "sub-auto-update-enabled":
			s.SubAutoUpdateEnabled = b()
		case "sub-auto-update-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("无效周期 (>=1m), 如 12h")
			}
			s.SubAutoUpdateInterval = d
		case "auto-select-enabled":
			s.AutoSelectEnabled = b()
		case "auto-select-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("无效周期 (>=1m), 如 15m")
			}
			s.AutoSelectInterval = d
		case "auto-groups":
			if v == "" {
				s.AutoGroups = nil
			} else {
				s.AutoGroups = strings.Split(v, ",")
			}
		case "test-url":
			s.TestURL = v
		case "test-timeout":
			n, err := strconv.Atoi(v)
			if err != nil || n < 100 {
				return fmt.Errorf("无效超时")
			}
			s.TestTimeout = n
		case "download-proxy":
			s.DownloadProxy = v
		default:
			return fmt.Errorf("未知配置项 %q (见 mihomo-cli set --help)", k)
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s %s\n", k, v, T("set.saved"))
		// 重建运行配置 + 定时器 + 热重载
		if s.Current() != nil {
			if err := render.Generate(s); err == nil {
				reloadIfActive(s)
			}
		}
		if err := sysd.InstallTimers(s); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 更新定时器失败: %v\n", err)
		}
		return nil
	},
}

var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "查看设置 (无参数 = 全部)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		lang := s.Lang
		if lang == "" {
			lang = i18n.Lang() + " (auto)"
		}
		all := [][2]string{
			{"lang", lang},
			{"allow-lan", fmt.Sprintf("%v", s.AllowLan)},
			{"mixed-port", fmt.Sprintf("%d", s.MixedPort)},
			{"sub-auto-update-enabled", fmt.Sprintf("%v", s.SubAutoUpdateEnabled)},
			{"sub-auto-update-interval", s.SubAutoUpdateInterval.String()},
			{"auto-select-enabled", fmt.Sprintf("%v", s.AutoSelectEnabled)},
			{"auto-select-interval", s.AutoSelectInterval.String()},
			{"auto-groups", strings.Join(s.AutoGroups, ",")},
			{"test-url", s.TestURL},
			{"test-timeout", fmt.Sprintf("%dms", s.TestTimeout)},
			{"download-proxy", s.DownloadProxy},
			{"current-profile", s.CurrentProfile},
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
		return fmt.Errorf("未知配置项 %q", args[0])
	},
}

// ---- core ----

var coreCmd = &cobra.Command{
	Use:   "core",
	Short: "内核管理: version/upgrade/rollback",
}

var coreVersionCmd = &cobra.Command{
	Use: "version",
	Short: "已安装内核版本",
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
	Short: "升级内核 (从 GitHub Releases)",
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
	Short: "回滚到上一版本",
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

var Version = "0.1.0"

var versionCmd = &cobra.Command{
	Use: "version",
	Short: "mihomo-cli 版本",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("mihomo-cli %s (%s/%s)\n", Version, runtimeGOOS(), runtimeGOARCH())
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
	connCmd.Flags().BoolVar(&connWatch, "watch", false, "持续刷新")
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "跟随日志")
	envCmd.Flags().BoolVar(&envUnset, "unset", false, "输出 unset 命令")
	coreCmd.AddCommand(coreVersionCmd, coreUpgradeCmd, coreRollbackCmd)
	rootCmd.AddCommand(connCmd, trafficCmd, logCmd, envCmd, doctorCmd, setCmd, getCmd, coreCmd, versionCmd)
}
