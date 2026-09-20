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
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
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
	fmt.Printf("活动连接: %d  累计 ↑%s ↓%s\n", len(r.Connections), humanBytes(r.UploadTotal), humanBytes(r.DownloadTotal))
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "网络\t目标\t代理链\t↑\t↓")
	for _, cn := range r.Connections {
		host := cn.Metadata.Host
		if host == "" {
			host = cn.Metadata.Destination
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			cn.Metadata.Network, host,
			strings.Join(cn.Chains, "/"),
			humanBytes(cn.Upload), humanBytes(cn.Download))
	}
	return w.Flush()
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
		fmt.Println("实时流量 (每秒):")
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
		fmt.Printf("%s 数据目录 %s\n", ok(err == nil), app.BaseDir)
		// 内核
		v, err := core.Version()
		fmt.Printf("%s 内核     %s\n", ok(err == nil), orDash(v, err))
		// 订阅
		var pinfo string
		if p := s.Current(); p != nil {
			pinfo = fmt.Sprintf("%s (%d 节点, 更新于 %s)", p.Name, p.Nodes, humanTime(p.UpdatedAt))
		}
		fmt.Printf("%s 订阅     %s\n", ok(s.Current() != nil), orDash(pinfo, nil))
		// 运行配置
		_, err = os.Stat(app.RuntimeConfig)
		fmt.Printf("%s 运行配置 %s\n", ok(err == nil), app.RuntimeConfig)
		// 服务
		active := sysd.IsActive()
		fmt.Printf("%s 服务     %s\n", ok(active), map[bool]string{true: "运行中", false: "未运行 (mihomo-cli start)"}[active])
		// API
		var apiInfo string
		ver, err := api.New(s).Version()
		if err == nil {
			apiInfo = "API 正常, 内核 " + ver
		}
		fmt.Printf("%s 控制API  %s\n", ok(err == nil), orDash(apiInfo, err))
		// 代理端口
		live := portOpen(fmt.Sprintf("127.0.0.1:%d", s.MixedPort))
		fmt.Printf("%s 代理端口 127.0.0.1:%d %s\n", ok(live), s.MixedPort, map[bool]string{true: "监听中", false: "未监听"}[live])
		if s.AllowLan {
			live2 := portOpen(fmt.Sprintf("0.0.0.0:%d", s.MixedPort))
			fmt.Printf("%s 局域网   0.0.0.0:%d %s (其他设备代理地址 http://%s:%d)\n",
				ok(live2), s.MixedPort, map[bool]string{true: "监听中", false: "未监听"}[live2], lanIP(), s.MixedPort)
		}
		// 定时器
		subOn := sysd.TimerEnabled("mihomo-cli-sub.timer")
		fmt.Printf("%s 订阅自动更新 %s (周期 %s)\n", ok(subOn == s.SubAutoUpdate),
			onOff(subOn), s.SubInterval)
		autoOn := sysd.TimerEnabled("mihomo-cli-auto.timer")
		fmt.Printf("%s 自动择优节点 %s (周期 %s, 分组 %s)\n", ok(autoOn == s.AutoSelect),
			onOff(autoOn), s.AutoInterval, orDash(strings.Join(s.AutoGroups, ","), nil))
		// 直连测试
		fmt.Println("提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli env))")
		return nil
	},
}

func onOff(b bool) string {
	if b {
		return "已启用"
	}
	return "已停用"
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

// ---- set ----

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "修改设置并生效",
	Long: `可配置项:
  allow-lan <true|false>     允许局域网设备使用代理 (0.0.0.0)
  mixed-port <port>          混合代理端口 (默认 7890)
  sub-auto-update <bool>     订阅自动更新开关
  sub-interval <duration>    订阅自动更新周期, 如 12h / 30m
  auto-select <bool>         自动切换到最低延迟节点
  auto-interval <duration>   自动择优周期, 如 15m
  auto-groups <g1,g2>        自动择优作用的分组 (空=全部 Selector)
  test-url <url>             测速 URL
  test-timeout <ms>          测速超时(毫秒)
  download-proxy <url>       下载内核/订阅使用的代理 (空=直连)`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		k, v := args[0], args[1]
		b := func() bool {
			return v == "true" || v == "on" || v == "yes" || v == "1"
		}
		switch k {
		case "allow-lan":
			s.AllowLan = b()
		case "mixed-port":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("无效端口")
			}
			s.MixedPort = n
		case "sub-auto-update":
			s.SubAutoUpdate = b()
		case "sub-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("无效周期 (>=1m), 如 12h")
			}
			s.SubInterval = d
		case "auto-select":
			s.AutoSelect = b()
		case "auto-interval":
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Minute {
				return fmt.Errorf("无效周期 (>=1m), 如 15m")
			}
			s.AutoInterval = d
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
		fmt.Printf("%s = %s (已保存)\n", k, v)
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

var Version = "dev"

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
	rootCmd.AddCommand(connCmd, trafficCmd, logCmd, envCmd, doctorCmd, setCmd, coreCmd, versionCmd)
}
