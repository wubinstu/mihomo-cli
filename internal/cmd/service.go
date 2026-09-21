package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

func mustSettings() *app.Settings {
	s, err := app.LoadSettings()
	if err != nil {
		fail(err)
	}
	return s
}

func serviceAction(action string) error {
	s := mustSettings()
	if s.Current() == nil {
		return fmt.Errorf("%s", T("没有订阅, 请先 mihomo-cli init"))
	}
	if action != "stop" {
		if err := render.Generate(s); err != nil {
			return err
		}
	}
	if err := sysd.Service(action); err != nil {
		return err
	}
	m := map[string]string{
		"start":   T("服务已启动"),
		"stop":    T("服务已停止"),
		"restart": T("服务已重启"),
	}
	fmt.Println(m[action])
	return nil
}

var startCmd = &cobra.Command{Use: "start", Short: T("启动代理服务"), RunE: func(*cobra.Command, []string) error { return serviceAction("start") }}
var stopCmd = &cobra.Command{Use: "stop", Short: T("停止代理服务"), RunE: func(*cobra.Command, []string) error { return serviceAction("stop") }}
var restartCmd = &cobra.Command{Use: "restart", Short: T("重启代理服务"), RunE: func(*cobra.Command, []string) error { return serviceAction("restart") }}

var statusCmd = &cobra.Command{
	Use: "status",
	Short: T("查看服务与代理状态"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		active := sysd.IsActive()
		state := T("未运行")
		if active {
			state = T("运行中")
		}
		fmt.Printf("%-12s %s\n", T("服务状态"), state)
		if active {
			c := api.New(s)
			if v, err := c.Version(); err == nil {
				fmt.Printf("%-12s %s\n", T("内核版本"), v)
			}
			if m, err := c.ConfigMode(); err == nil {
				fmt.Printf("%-12s %s\n", T("代理模式"), m)
			}
		}
		fmt.Printf("%-12s %d (http+socks5)\n", T("混合端口"), s.MixedPort)
		lan := T("仅本机 (127.0.0.1)")
		if s.AllowLan {
			lan = T("允许 (0.0.0.0)")
		}
		fmt.Printf("%-12s %s\n", T("局域网"), lan)
		if len(s.DNSServers) == 0 {
			fmt.Printf("%-12s %s\n", T("当前DNS"), T("跟随订阅"))
		} else {
			fmt.Printf("%-12s %s\n", T("当前DNS"), strings.Join(s.DNSServers, ", "))
		}
		if active {
			if r, err := api.New(s).Connections(); err == nil {
				fmt.Printf("%-12s ↑ %s  ↓ %s  (%d %s)\n", T("流量统计"),
					humanBytes(r.UploadTotal), humanBytes(r.DownloadTotal),
					len(r.Connections), T("活动连接"))
			}
		}
		if p := s.Current(); p != nil {
			fmt.Printf("%-12s sub%d:%s (%d %s, %s)\n", T("当前订阅"), subIndex(s, p.Name), p.Name, p.Nodes, T("节点"), p.UpdatedAt.Format("2006-01-02 15:04"))
		} else {
			fmt.Printf("%-12s %s\n", T("当前订阅"), T("悬空 (sub unuse)"))
			if active {
				fmt.Printf("%-12s %s\n", T("内核流量"), T("DIRECT (空配置)"))
			}
		}
		if s.Current() != nil && s.CurrentGroup != "" && active {
			c := api.New(s)
			if ps, err := c.Proxies(); err == nil {
				if g := resolveGroupArg(ps, s.CurrentGroup); g != nil {
					gid := groupID(ps, g.Name)
					if g.Now != "" {
						d := c.LastDelay(g.Now)
						dTxt := "-"
						if d > 0 {
							dTxt = fmt.Sprintf("%d ms", d)
						}
						fmt.Printf("%-12s %s:%s → %s:%s (%s)\n", T("当前链路"), gid, g.Name, nodeID(g, g.Now), g.Now, dTxt)
					} else {
						fmt.Printf("%-12s %s:%s\n", T("当前链路"), gid, g.Name)
					}
				}
			}
		}
		printTimers(s)
		return nil
	},
}

// printTimers 打印自动任务状态: sub 悬空时均显示悬空; group 悬空时择优显示悬空
func printTimers(s *app.Settings) {
	if s.Current() == nil {
		fmt.Printf("%-12s %s\n", T("订阅自动更新"), T("悬空 (sub unuse)"))
		fmt.Printf("%-12s %s\n", T("节点自动择优"), T("悬空 (sub unuse)"))
		return
	}
	last := humanTime(s.Current().UpdatedAt)
	next := remaining(s.Current().UpdatedAt, s.SubAutoUpdateInterval)
	fmt.Printf("%-12s %s | %s %s | %s %s\n", T("订阅自动更新"),
		onOff2(s.SubAutoUpdateEnabled), T("上次"), last, T("下次"), next)

	if s.CurrentGroup == "" {
		fmt.Printf("%-12s %s\n", T("节点自动择优"), T("悬空 (group 未选)"))
		return
	}
	next2 := remaining(s.AutoSelectLastRun, s.ProxyAutoSelectInterval)
	last2 := humanTime(s.AutoSelectLastRun)
	if s.ProxyAutoSelectEnabled {
		fmt.Printf("%-12s %s | %s %s | %s %s\n", T("节点自动择优"),
			onOff2(s.ProxyAutoSelectEnabled), T("上次"), last2, T("下次"), next2)
	} else {
		fmt.Printf("%-12s %s | %s %s\n", T("节点自动择优"),
			onOff2(s.ProxyAutoSelectEnabled), T("上次"), last2)
	}
}

func onOff2(b bool) string {
	if b {
		return T("已启用")
	}
	return T("已停用")
}

func remaining(last time.Time, ivl time.Duration) string {
	if last.IsZero() {
		return "?"
	}
	left := time.Until(last.Add(ivl))
	if left < 0 {
		left = 0
	}
	h := int(left.Hours())
	m := int(left.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func init() {
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
