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
		return fmt.Errorf("没有订阅, 请先 mihomo-cli init")
	}
	if action != "stop" {
		if err := render.Generate(s); err != nil {
			return err
		}
	}
	if err := sysd.Service(action); err != nil {
		return err
	}
	fmt.Printf("服务已 %s\n", action)
	return nil
}

var startCmd = &cobra.Command{Use: "start", Short: "启动代理服务", RunE: func(*cobra.Command, []string) error { return serviceAction("start") }}
var stopCmd = &cobra.Command{Use: "stop", Short: "停止代理服务", RunE: func(*cobra.Command, []string) error { return serviceAction("stop") }}
var restartCmd = &cobra.Command{Use: "restart", Short: "重启代理服务", RunE: func(*cobra.Command, []string) error { return serviceAction("restart") }}

var statusCmd = &cobra.Command{
	Use: "status",
	Short: "查看服务与代理状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		active := sysd.IsActive()
		fmt.Printf("服务状态 : %s\n", map[bool]string{true: "运行中", false: "未运行"}[active])
		if active {
			c := api.New(s)
			if v, err := c.Version(); err == nil {
				fmt.Printf("内核版本 : %s\n", v)
			}
			if ps, err := c.Proxies(); err == nil {
				mode := "规则"
				if p, ok := ps.Proxies["GLOBAL"]; ok && p.Now != "" {
					mode = p.Now
				}
				fmt.Printf("代理模式 : %s\n", mode)
			}
		}
		fmt.Printf("混合端口 : %d (http+socks5)\n", s.MixedPort)
		fmt.Printf("局域网   : %s\n", map[bool]string{true: "允许 (0.0.0.0)", false: "仅本机 (127.0.0.1)"}[s.AllowLan])
		if p := s.Current(); p != nil {
			fmt.Printf("当前订阅 : %s (%d 节点, 更新于 %s)\n", p.Name, p.Nodes, p.UpdatedAt.Format("2006-01-02 15:04"))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
