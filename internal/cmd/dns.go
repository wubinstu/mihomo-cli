package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/subs"
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

// presetNames 预设名列表 (帮助信息用)
func presetNames() string {
	var names []string
	for _, p := range dnsPresets {
		names = append(names, p.Name)
	}
	return strings.Join(names, "/")
}

// subDNS 从订阅文件提取 dns.nameserver
func subDNS(name string) []string {
	data, err := os.ReadFile(subs.Path(name))
	if err != nil {
		return nil
	}
	var cfgYAML struct {
		DNS struct {
			Nameserver []string `yaml:"nameserver"`
		} `yaml:"dns"`
	}
	if yaml.Unmarshal(data, &cfgYAML) != nil {
		return nil
	}
	return cfgYAML.DNS.Nameserver
}

func sameIPs(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// resolveDNSServers 解析 dns use 的参数: 预设名 | sub# | 裸 IP
func resolveDNSServers(s *app.Settings, args []string) ([]string, error) {
	var servers []string
	if len(args) == 1 {
		for _, p := range dnsPresets {
			if p.Name == args[0] {
				return p.IPs, nil
			}
		}
		for i := range s.Profiles {
			if args[0] == fmt.Sprintf("sub%d", i+1) {
				ips := subDNS(s.Profiles[i].Name)
				if len(ips) == 0 {
					return nil, fmt.Errorf("%s: %s", args[0], T("该订阅无 dns.nameserver 配置"))
				}
				return ips, nil
			}
		}
	}
	for _, a := range args {
		if net.ParseIP(a) == nil {
			return nil, fmt.Errorf("%q: %s (%s)", a, T("无效 IP"), T("或未知预设"))
		}
		servers = append(servers, a)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("%s", T("未指定 DNS 服务器"))
	}
	if len(servers) > 3 {
		servers = servers[:3]
	}
	return servers, nil
}

var dnsUseCmd = &cobra.Command{
	Use:   "use <preset|sub#|ip...>",
	Short: T("设置 DNS 服务器 (预设名/订阅编号/1-3 个 IP)"),
	Long: T("等于 config set dns.nameserver <值>; unuse 恢复跟随订阅。") + "\n" +
		"mihomo-cli dns use <preset>   # " + T("预设") + ": " + presetNames() + "\n" +
		"mihomo-cli dns use sub#       # " + T("使用某订阅自带的 DNS") + "\n" +
		"mihomo-cli dns use <ip...>    # " + T("自定义 DNS 服务器 IP"),
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		servers, err := resolveDNSServers(s, args)
		if err != nil {
			return err
		}
		k := cfg.Lookup("dns.nameserver")
		if k == nil {
			return fmt.Errorf("%s", T("内部错误: dns.nameserver 未注册"))
		}
		canon, err := k.Parse(strings.Join(servers, ","))
		if err != nil {
			return err
		}
		if err := k.Set(s, canon); err != nil {
			return err
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("dns.nameserver = %s %s\n", canon, T("已保存"))
		return applyConfig(s, k)
	},
}

var dnsUnuseCmd = &cobra.Command{
	Use:   "unuse",
	Short: T("取消自定义 DNS (恢复跟随订阅)"),
	RunE:  func(cmd *cobra.Command, args []string) error { return dnsSet(false) },
}

func init() {
	dnsUseCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, noFileComp()
		}
		s := mustSettingsQuiet()
		var out []string
		for _, p := range dnsPresets {
			if strings.HasPrefix(p.Name, toComplete) {
				out = append(out, p.Name)
			}
		}
		if s != nil {
			for i := range s.Profiles {
				n := fmt.Sprintf("sub%d", i+1)
				if strings.HasPrefix(n, toComplete) {
					out = append(out, n)
				}
			}
		}
		return out, noFileComp()
	}
	for _, c := range []*cobra.Command{dnsUseCmd, dnsUnuseCmd, dnsOnCmd, dnsOffCmd} {
		markMutating(c)
	}
	dnsCmd.AddCommand(dnsUseCmd, dnsUnuseCmd)
}
