package cmd

import (
	"fmt"
	"net"
	"os"

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

// presetNamesText 预设名的文字形式 (帮助信息用)

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

// resolveDNSServers 解析 DNS 值: 预设名 | subN | 裸 IP (dns.nameserver 的值域)
func resolveDNSServers(s *app.Settings, args []string) ([]string, error) {
	var servers []string
	if len(args) == 1 { // 单个参数: 预设名 / subN
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

// dnsCmd 的裸命令 = config get core.dns (只打参数表; 预设表移到 config set core.dns.nameserver -h)
// 写操作全部走 config set; 这里只保留 on/off 两个最常用的糖。
func init() {
	for _, c := range []*cobra.Command{dnsOnCmd, dnsOffCmd} {
		markMutating(c)
	}
	// 预设名 / subN 的解析与补全交给 cfg (dns.nameserver 的值域),
	// 这样 config set core.dns.nameserver cloudflare 也能用, 且只有一处实现
	cfg.SetDNSPresetNames(dnsPresetNameList(), func(s *app.Settings, v string) ([]string, error) {
		return resolveDNSServers(s, []string{v})
	})
}

// dnsPresetNameList 预设名列表 (补全用)
func dnsPresetNameList() []string {
	out := make([]string, 0, len(dnsPresets))
	for _, p := range dnsPresets {
		out = append(out, p.Name)
	}
	return out
}
