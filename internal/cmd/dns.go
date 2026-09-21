package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// dnsPresets 常用公共 DNS 预设
var dnsPresets = []struct {
	Name string
	Desc string
	IPs  []string
}{
	{"ali", T("阿里 DNS"), []string{"223.5.5.5", "223.6.6.6"}},
	{"114", T("114 DNS"), []string{"114.114.114.114", "114.114.115.115"}},
	{"google", T("谷歌 DNS"), []string{"8.8.8.8", "8.8.4.4"}},
	{"cloudflare", T("Cloudflare DNS"), []string{"1.1.1.1", "1.0.0.1"}},
	{"adguard", T("AdGuard DNS (拦截广告)"), []string{"94.140.14.14", "94.140.15.15"}},
	{"quad9", T("Quad9 DNS (安全拦截)"), []string{"9.9.9.9", "149.112.112.112"}},
	{"dnspod", T("腾讯 DNSPod"), []string{"119.29.29.29", "182.252.116.116"}},
}

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: T("查看/设置 DNS (预设或自定义 IP)"),
	Long: T("自定义 DNS 会覆盖订阅中的 dns.nameserver; unuse 恢复跟随订阅。") + `
mihomo-cli dns               # ` + T("查看当前 DNS") + `
mihomo-cli dns use <预设>     # ` + T("使用预设") + `: ` + presetNames() + `
mihomo-cli dns use <ip...>    # ` + T("自定义 DNS 服务器 IP"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if len(s.DNSServers) == 0 {
			fmt.Printf("%s: %s\n", T("当前DNS"), T("跟随订阅"))
		} else {
			fmt.Printf("%s: %s\n", T("当前DNS"), strings.Join(s.DNSServers, ", "))
		}
		rows := [][]string{{T("预设"), T("说明"), "IP"}}
		for _, p := range dnsPresets {
			mark := ""
			if strings.Join(p.IPs, ",") == strings.Join(s.DNSServers, ",") {
				mark = " *"
			}
			rows = append(rows, []string{p.Name + mark, p.Desc, strings.Join(p.IPs, ", ")})
		}
		ui.Table(os.Stdout, rows, 2)
		fmt.Println(T("用法: mihomo-cli dns use <预设|ip...> | mihomo-cli dns unuse"))
		return nil
	},
}

func presetNames() string {
	var names []string
	for _, p := range dnsPresets {
		names = append(names, p.Name)
	}
	return strings.Join(names, "/")
}

var dnsUseCmd = &cobra.Command{
	Use:   "use <预设|ip...>",
	Short: T("设置 DNS (预设名或 1-3 个 IP)"),
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var servers []string
		if len(args) == 1 {
			for _, p := range dnsPresets {
				if p.Name == args[0] {
					servers = p.IPs
					break
				}
			}
		}
		if servers == nil {
			for _, a := range args {
				if net.ParseIP(a) == nil {
					return fmt.Errorf("%q: %s (%s)", a, T("无效 IP"), T("或未知预设"))
				}
				servers = append(servers, a)
			}
			if len(servers) > 3 {
				servers = servers[:3]
			}
		}
		s := mustSettings()
		s.DNSServers = servers
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", T("当前DNS"), strings.Join(servers, ", "))
		if err := render.Generate(s); err != nil {
			return err
		}
		reloadIfActive(s)
		return nil
	},
}

var dnsUnuseCmd = &cobra.Command{
	Use:   "unuse",
	Short: T("取消自定义 DNS (恢复跟随订阅)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		s.DNSServers = nil
		if err := s.Save(); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		reloadIfActive(s)
		fmt.Println(T("已恢复跟随订阅"))
		return nil
	},
}

func init() {
	dnsUseCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			var out []string
			for _, p := range dnsPresets {
				if strings.HasPrefix(p.Name, toComplete) {
					out = append(out, p.Name)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dnsCmd.AddCommand(dnsUseCmd, dnsUnuseCmd)
	rootCmd.AddCommand(dnsCmd)
}
