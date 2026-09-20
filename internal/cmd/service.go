package cmd

import (
	"fmt"

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
		if p := s.Current(); p != nil {
			fmt.Printf("%-12s %s (%d %s, %s)\n", T("当前订阅"), p.Name, p.Nodes, T("节点"), p.UpdatedAt.Format("2006-01-02 15:04"))
		}
		if g := s.CurrentGroup; g != "" {
			fmt.Printf("%-12s %s\n", T("当前分组"), g)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
