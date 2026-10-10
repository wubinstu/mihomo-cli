package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
	"time"
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
		if ns := cfg.SecList(s, "core.dns.nameserver"); len(ns) == 0 {
			rows = append(rows, checkRow{true, T("DNS"), T("跟随订阅")})
		} else {
			rows = append(rows, checkRow{true, T("DNS"), strings.Join(ns, ", ")})
		}
		if k := cfgLookup("core.dns.enable"); k != nil && k.Get(s) == "true" {
			rows = append(rows, checkRow{true, T("DNS override"), T("已开启")})
		}
		if k := cfgLookup("core.tun.enable"); k != nil && k.Get(s) == "true" {
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
			// 当前订阅带用量限制时直接给结论 (用户 1.5.0 要求)
			if q := subs.ParseQuota(p.UserInfo); q != nil {
				rows = append(rows, checkRow{true, T("订阅用量"), q.Short()})
			}
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
			checkRow{true, T("Res auto-update"),
				timerText(sysd.ResourceTimerName(), s.ResourceAutoUpdateEnabled, s.ResourceAutoUpdateInterval, s.ResourceLastRun)},
			checkRow{true, T("Sub auto-update"),
				timerText(sysd.SubTimerName(), s.SubAutoUpdateEnabled, s.SubAutoUpdateInterval, subLast(s))},
			checkRow{true, T("Node auto-select"),
				timerText(sysd.NodeTimerName(), s.NodeAutoSelectEnabled, s.NodeAutoSelectInterval, s.AutoSelectLastRun)},
		)

		ui.TableSections(stdoutWriter(), []string{"", T("项目"), T("值")}, []ui.Section{{Rows: plainToRows(rows)}}, 2)
		return nil
	},
}

// subLast 订阅更新的上次时间: 优先 systemd, 回退订阅自己的更新时间
func subLast(s *app.Settings) time.Time {
	if t := sysd.TimerLast(sysd.SubTimerName()); !t.IsZero() {
		return t
	}
	if p := s.Current(); p != nil {
		return p.UpdatedAt
	}
	return time.Time{}
}

func onOff2(b bool) string {
	if b {
		return T("已启用")
	}
	return T("已停用")
}

func init() {
	for _, c := range []*cobra.Command{startCmd, stopCmd, restartCmd} {
		markMutating(c)
	}
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
