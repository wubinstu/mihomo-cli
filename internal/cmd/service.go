package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
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
		return fmt.Errorf("%s (%s)", T("没有可用订阅"), T("mihomo-cli sub use <id|名称>"))
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

// statusCmd 服务与代理状态: 与 doctor 共用同一套标签与列宽, 顺序固定 Res → Sub → Node
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: T("查看服务与代理状态"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		active := sysd.IsActive()
		var rows []checkRow

		state := T("未运行")
		if active {
			state = T("运行中")
		}
		rows = append(rows, checkRow{true, T("Service"), state})

		c := api.New(s)
		if active {
			if v, err := c.Version(); err == nil {
				rows = append(rows, checkRow{true, T("Core"), v})
			}
			if m, err := c.ConfigMode(); err == nil {
				rows = append(rows, checkRow{true, T("Mode"), m})
			}
		}
		rows = append(rows, checkRow{true, T("Proxy port"), mixedPortText(s)})
		if s.AllowLan {
			rows = append(rows, checkRow{true, T("LAN"),
				fmt.Sprintf("allowed (http://%s:%d)", lanIP(), s.ProxyPort())})
		} else {
			rows = append(rows, checkRow{true, T("LAN"), T("仅本机 (127.0.0.1)")})
		}
		if len(s.DNSServers) == 0 {
			rows = append(rows, checkRow{true, T("DNS"), T("跟随订阅")})
		} else {
			rows = append(rows, checkRow{true, T("DNS"), strings.Join(s.DNSServers, ", ")})
		}
		if k := cfgLookup("dns.enable"); k != nil && k.Get(s) == "true" {
			rows = append(rows, checkRow{true, T("DNS override"), T("已开启")})
		}
		if k := cfgLookup("tun.enable"); k != nil && k.Get(s) == "true" {
			rows = append(rows, checkRow{true, T("TUN"), T("已开启")})
		}

		if active {
			if r, err := c.Connections(); err == nil {
				rows = append(rows, checkRow{true, T("Traffic"),
					fmt.Sprintf("↑ %s  ↓ %s  (%d %s)", humanBytes(r.UploadTotal), humanBytes(r.DownloadTotal),
						len(r.Connections), T("活动连接"))})
			}
		}
		if p := s.Current(); p != nil {
			rows = append(rows, checkRow{true, T("Profile"),
				fmt.Sprintf("sub%d:%s (%d %s, %s)", subIndex(s, p.Name), p.Name, p.Nodes, T("节点"),
					humanTime(p.UpdatedAt))})
			if s.CurrentGroup != "" && active {
				if ps, err := c.Proxies(); err == nil {
					if g := resolveGroupArg(ps, s.CurrentGroup); g != nil {
						gid := groupID(ps, g.Name)
						if g.Now != "" {
							d := c.LastDelay(g.Now)
							dTxt := "-"
							if d > 0 {
								dTxt = fmt.Sprintf("%d ms", d)
							}
							rows = append(rows, checkRow{true, T("Chain"),
								fmt.Sprintf("%s:%s → %s:%s (%s)", gid, g.Name, nodeID(g, g.Now), g.Now, dTxt)})
						} else {
							rows = append(rows, checkRow{true, T("Chain"), fmt.Sprintf("%s:%s", gid, g.Name)})
						}
					}
				}
			}
		} else {
			rows = append(rows, checkRow{true, T("Profile"), T("悬空 (sub unuse)")})
			if active {
				rows = append(rows, checkRow{true, T("Core traffic"), T("DIRECT (空配置)")})
			}
		}

		// 顺序固定 Res → Sub → Node (与 doctor 完全一致)
		rows = append(rows,
			checkRow{true, T("Res auto-update"), timerText(s.ResourceAutoUpdateEnabled, s.ResourceAutoUpdateInterval)},
			checkRow{true, T("Sub auto-update"), subTimerText(s)},
			checkRow{true, T("Node auto-select"), nodeTimerText(s)},
		)

		ui.TableSections(stdoutWriter(), []string{"", T("项目"), T("值")}, []ui.Section{{Rows: plainToRows(rows)}}, 2)
		return nil
	},
}

// subTimerText 订阅自动更新: 悬空(sub unuse)时如实说明
func subTimerText(s *app.Settings) string {
	if s.Current() == nil {
		return T("悬空 (sub unuse)")
	}
	last := humanTime(s.Current().UpdatedAt)
	if s.SubAutoUpdateEnabled {
		return fmt.Sprintf("%s | %s %s | %s %s", T("已启用"), T("上次"), last,
			T("下次"), remaining(s.Current().UpdatedAt, s.SubAutoUpdateInterval))
	}
	return fmt.Sprintf("%s | %s %s", T("已停用"), T("上次"), last)
}

// nodeTimerText 自动择优: group 未选时显示悬空
func nodeTimerText(s *app.Settings) string {
	if s.Current() == nil {
		return T("悬空 (sub unuse)")
	}
	if s.CurrentGroup == "" {
		return T("悬空 (group 未选)")
	}
	if s.NodeAutoSelectEnabled {
		return fmt.Sprintf("%s | %s %s | %s %s", T("已启用"), T("上次"), humanTime(s.AutoSelectLastRun),
			T("下次"), remaining(s.AutoSelectLastRun, s.NodeAutoSelectInterval))
	}
	return fmt.Sprintf("%s | %s %s", T("已停用"), T("上次"), humanTime(s.AutoSelectLastRun))
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
	return cfg.DurHuman(time.Duration(int(left.Minutes())) * time.Minute) // 剩余时间截断到分钟
}

func init() {
	for _, c := range []*cobra.Command{startCmd, stopCmd, restartCmd} {
		markMutating(c)
	}
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
