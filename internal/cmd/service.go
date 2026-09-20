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
	m := map[string]string{"start": T("svc.start"), "stop": T("svc.stop"), "restart": T("svc.restart")}
	fmt.Println(m[action])
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
		state := T("svc.stopped")
		if active {
			state = T("svc.running")
		}
		fmt.Printf("%-10s %s\n", T("st.service"), state)
		if active {
			c := api.New(s)
			if v, err := c.Version(); err == nil {
				fmt.Printf("%-10s %s\n", T("st.corever"), v)
			}
			if ps, err := c.Proxies(); err == nil {
				mode := "rule"
				if p, ok := ps.Proxies["GLOBAL"]; ok && p.Now != "" {
					mode = p.Now
				}
				fmt.Printf("%-10s %s\n", T("st.mode"), mode)
			}
		}
		fmt.Printf("%-10s %d %s\n", T("st.port"), s.MixedPort, T("st.port.hint"))
		lan := T("st.lan.off")
		if s.AllowLan {
			lan = T("st.lan.on")
		}
		fmt.Printf("%-10s %s\n", T("st.lan"), lan)
		if p := s.Current(); p != nil {
			fmt.Printf("%-10s %s (%d nodes, %s)\n", T("st.sub"), p.Name, p.Nodes, p.UpdatedAt.Format("2006-01-02 15:04"))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
}
