package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
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

// subDNS 从订阅文件提取 dns.nameserver
func subDNS(name string) []string {
	data, err := os.ReadFile(subs.Path(name))
	if err != nil {
		return nil
	}
	var cfg struct {
		DNS struct {
			Nameserver []string `yaml:"nameserver"`
		} `yaml:"dns"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}
	return cfg.DNS.Nameserver
}

func sameIPs(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
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
		rows := [][]string{{"*", T("名称"), T("说明"), "IP"}}
		// 公共预设
		for _, p := range dnsPresets {
			mark := ""
			if sameIPs(p.IPs, s.DNSServers) {
				mark = "*"
			}
			rows = append(rows, []string{mark, p.Name, p.Desc, strings.Join(p.IPs, ", ")})
		}
		// 订阅自带 DNS
		for i := range s.Profiles {
			p := &s.Profiles[i]
			ips := subDNS(p.Name)
			if len(ips) == 0 {
				continue
			}
			mark := ""
			if p.Name == s.CurrentProfile && len(s.DNSServers) == 0 {
				mark = "*" // 跟随订阅
			}
			if len(s.DNSServers) > 0 && sameIPs(ips, s.DNSServers) {
				mark = "*"
			}
			rows = append(rows, []string{mark,
				fmt.Sprintf("sub%d", i+1), T("订阅") + " " + p.Name, strings.Join(ips, ", ")})
		}
		cur := T("跟随订阅")
		if len(s.DNSServers) > 0 {
			cur = strings.Join(s.DNSServers, ", ")
		} else if p := s.Current(); p != nil {
			if ips := subDNS(p.Name); len(ips) > 0 {
				cur = T("跟随订阅") + " (" + strings.Join(ips, ", ") + ")"
			}
		}
		fmt.Printf("%s: %s\n", T("当前DNS"), cur)
		ui.Table(os.Stdout, rows, 2)
		fmt.Println(T("用法: mihomo-cli dns use <预设|sub#|ip...> | mihomo-cli dns unuse"))
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
	Use:   "use <预设|sub#|ip...>",
	Short: T("设置 DNS (预设名/订阅编号/1-3 个 IP)"),
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
			// sub# 订阅自带 DNS
			s := mustSettings()
			for i := range s.Profiles {
				if args[0] == fmt.Sprintf("sub%d", i+1) {
					servers = subDNS(s.Profiles[i].Name)
					if len(servers) == 0 {
						return fmt.Errorf("%s: %s", args[0], T("该订阅无 dns.nameserver 配置"))
					}
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

var _ = app.BaseDir

func init() {
	dnsUseCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			s := mustSettings()
			var out []string
			for _, p := range dnsPresets {
				if strings.HasPrefix(p.Name, toComplete) {
					out = append(out, p.Name)
				}
			}
			for i := range s.Profiles {
				n := fmt.Sprintf("sub%d", i+1)
				if strings.HasPrefix(n, toComplete) {
					out = append(out, n)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dnsCmd.AddCommand(dnsUseCmd, dnsUnuseCmd)
	rootCmd.AddCommand(dnsCmd)
}
