package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
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

// checkRow 体检行: 状态标记 + 项目 + 详情
type checkRow struct {
	ok    bool
	item  string
	ctext string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: T("体检: 内核/服务/端口/资源/配置一致性"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		var rows []checkRow

		rows = append(rows, checkRow{app.EnsureDirs() == nil, T("Data dir"), app.BaseDir})
		v, err := core.Version()
		rows = append(rows, checkRow{err == nil, T("Core"), dashIf(v, err)})
		rows = append(rows, checkRow{true, T("Core version"), coreAssetText(s)})

		var pinfo string
		if p := s.Current(); p != nil {
			pinfo = fmt.Sprintf("%s (%d %s, %s)", p.Name, p.Nodes, T("节点"), humanTime(p.UpdatedAt))
		} else {
			pinfo = T("悬空 (sub unuse)")
		}
		rows = append(rows, checkRow{s.Current() != nil, T("Profile"), dashIf(pinfo, nil)})

		_, rerr := os.Stat(app.RuntimeConfig)
		rows = append(rows, checkRow{rerr == nil, T("Runtime config"), app.RuntimeConfig})

		// 四个资源分别一行: 任一缺失都可能让内核 fatal 循环启动 (v0.6 线上事故)
		for _, name := range core.SortResources() {
			var r core.Resource
			for _, x := range core.Resources() {
				if x.Name == name {
					r = x
				}
			}
			path, ok, mt, size := core.ResourceInfo(r.Name)
			text := ""
			if ok {
				text = fmt.Sprintf("%s  %s  %s", r.File, humanBytes(size), mt.Format("2006-01-02 15:04"))
			} else {
				text = fmt.Sprintf("%s (%s)", T("缺失"), r.File)
			}
			rows = append(rows, checkRow{ok, T("Resource") + " " + r.Name, text})
			_ = path
		}

		active := sysd.IsActive()
		svcText := T("运行中")
		if !active {
			svcText = T("未运行 (mihomo-cli start)")
		}
		rows = append(rows, checkRow{active, T("Service"), svcText})

		var apiInfo string
		ver, aerr := api.New(s).Version()
		if aerr == nil {
			apiInfo = T("API 正常, 内核") + " " + ver
		} else {
			apiInfo = aerr.Error()
		}
		rows = append(rows, checkRow{aerr == nil, T("Ctrl API"), apiInfo})

		port := s.ProxyPort()
		live := portOpen(fmt.Sprintf("127.0.0.1:%d", port))
		rows = append(rows, checkRow{live, T("Proxy port"), fmt.Sprintf("127.0.0.1:%d %s", port, listenWord(live))})
		if s.AllowLan {
			live2 := portOpen(fmt.Sprintf("0.0.0.0:%d", port))
			rows = append(rows, checkRow{live2, T("LAN"),
				fmt.Sprintf("0.0.0.0:%d %s (LAN: http://%s:%d)", port, listenWord(live2), lanIP(), port)})
		}

		// 配置一致性 (config.toml ↔ 运行态)
		rows = append(rows, configCheckRow(s)...)

		// 顺序固定为 Res → Sub → Node (与 status 一致)
		rows = append(rows,
			checkRow{true, T("Res auto-update"), timerText(s.ResourceAutoUpdateEnabled, s.ResourceAutoUpdateInterval)},
			checkRow{sysd.TimerEnabled("mihomo-cli-sub.timer") == s.SubAutoUpdateEnabled, T("Sub auto-update"),
				timerText(s.SubAutoUpdateEnabled, s.SubAutoUpdateInterval)},
			checkRow{sysd.TimerEnabled("mihomo-cli-auto.timer") == s.NodeAutoSelectEnabled, T("Node auto-select"),
				timerText(s.NodeAutoSelectEnabled, s.NodeAutoSelectInterval)},
		)

		ui.TableSections(os.Stdout, []string{"", T("检查项"), T("详情")},
			[]ui.Section{{Rows: checkToRows(rows)}}, 2)

		fmt.Println(T("提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli proxy on))"))
		return nil
	},
}

// plainToRows status 用: 没有"是否正常"的语义, 不加勾叉
func plainToRows(rows []checkRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{"", r.item, r.ctext})
	}
	return out
}

func checkToRows(rows []checkRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		mark := ui.OKMark()
		if !r.ok {
			mark = ui.ErrMark()
		}
		out = append(out, []string{mark, r.item, r.ctext})
	}
	return out
}

// coreAssetText 已安装内核的规格 (linux/amd64 + 风味 + 版本);
// 旧版本安装的内核没有记录规格, 就从二进制推断版本并标注"未记录"
func coreAssetText(s *app.Settings) string {
	platform := s.CorePlatform
	if platform == "" {
		platform = core.PlatformName("")
	}
	ver := s.CoreVersion
	note := ""
	if ver == "" {
		if v := core.VersionShort(); v != "" {
			ver = v
			note = T("(旧版安装, 规格未记录)")
		} else {
			ver = "-"
		}
	}
	flavor := s.CoreFlavor
	if flavor == "" {
		flavor = T("官方默认")
	}
	out := fmt.Sprintf("%s [%s] (%s)", platform, flavor, ver)
	if note != "" {
		out += " " + note
	}
	return out
}

// configCheckRow 汇总 config.toml 的形态校验 (L1) 与运行态一致性
func configCheckRow(s *app.Settings) []checkRow {
	bad := badConfigValues(s)
	if len(bad) > 0 {
		return []checkRow{{false, T("Config values"), strings.Join(bad, "; ")}}
	}
	valuesRow := checkRow{true, T("Config values"),
		fmt.Sprintf("%s (%d)", T("全部合法"), len(cfg.Keys))}
	live := cfg.Fetch(s)
	if !live.Running() {
		return []checkRow{valuesRow}
	}
	var diff []string
	for _, k := range cfg.Keys {
		if k.Kind == cfg.KindState {
			continue
		}
		if !live.Same(k, k.Effective(s)) {
			diff = append(diff, k.Name)
		}
	}
	if len(diff) > 0 {
		return []checkRow{valuesRow, {false, T("Config file"),
			fmt.Sprintf("%d %s: %s", len(diff), T("项与运行态不同"), strings.Join(diff, ", ")) +
				" (" + T("config update-service") + ")"}}
	}
	return []checkRow{valuesRow, {true, T("Config file"), T("与运行态完全一致")}}
}

// badConfigValues L1 形态校验: 返回所有不合法的键说明
func badConfigValues(s *app.Settings) []string {
	var out []string
	for _, k := range cfg.Keys {
		if k.Kind == cfg.KindState {
			continue
		}
		v := k.Effective(s)
		if v == "" || v == "sub" {
			continue
		}
		if _, err := k.Parse(v); err != nil {
			out = append(out, fmt.Sprintf("%s=%q (%s)", k.Name, v, k.Expected()))
		}
	}
	return out
}

func timerText(on bool, ivl time.Duration) string {
	if on {
		return fmt.Sprintf("%s (%s)", T("已启用"), cfg.DurHuman(ivl))
	}
	return fmt.Sprintf("%s (%s)", T("已停用"), cfg.DurHuman(ivl))
}

func dashIf(s string, err error) string {
	if s == "" {
		if err != nil {
			return err.Error()
		}
		return "-"
	}
	return s
}

func listenWord(live bool) string {
	if live {
		return T("监听中")
	}
	return T("未监听")
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// lanIP 取一个非回环 IPv4 (展示给用户拼 LAN 地址)
func lanIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}

// ---- version ----

var Version = "1.3.0"

var versionCmd = &cobra.Command{
	Use:   "version",
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
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, T("跟随日志"))
	rootCmd.AddCommand(logCmd, doctorCmd, versionCmd)
}
