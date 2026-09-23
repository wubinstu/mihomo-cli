package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

// ---- log ----

var logFollow bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: T("查看内核日志 (-f 跟随)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		jargs := []string{"-u", "mihomo-cli", "--no-pager", "-n", "100"}
		if logFollow {
			jargs = append(jargs, "-f")
		}
		if _, err := exec.LookPath("journalctl"); err == nil {
			bin := "journalctl"
			if os.Geteuid() != 0 {
				// system journal 需要 root/adm 组; 非 root 自动 sudo
				bin = "sudo"
				jargs = append([]string{"-n", "journalctl"}, jargs...)
			}
			c := exec.Command(bin, jargs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		}
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

var Version = "1.0.1"

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
		if err := core.DownloadGeo(dlProxy, mustSettings().InstallMirror); err != nil {
			return err
		}
		fmt.Println(T("已下载"))
		reloadIfActive(s)
		return nil
	},
}

func init() {
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, T("跟随日志"))
	coreCmd.AddCommand(coreVersionCmd, coreUpgradeCmd, coreRollbackCmd, coreGeoCmd)
	rootCmd.AddCommand(logCmd, doctorCmd, coreCmd, versionCmd)
}
