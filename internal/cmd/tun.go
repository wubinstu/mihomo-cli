package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// tunCmd / dnsCmd 是 config set tun.* / config set dns.* 的极薄糖命令:
// 不重复实现 list/use/unuse 那套逻辑, 只保留 on/off/list 三个动作。
var tunCmd = &cobra.Command{
	Use:   "tun",
	Short: T("TUN 透明代理: on/off/list"),
	Long: T("TUN 会让默认流量进入虚拟网卡, 配置错误可能把自己踢出服务器;") + "\n" +
		T("开启前会自动做安全检查 (TUN 设备 / CAP_NET_ADMIN / SSH 对端网段排除)。") + "\n" +
		"mihomo-cli tun on        # " + T("等价于 config set tun.enable true") + "\n" +
		"mihomo-cli tun off       # " + T("等价于 config set tun.enable false") + "\n" +
		"mihomo-cli tun list      # " + T("查看 tun 段全部配置项"),
	RunE: func(cmd *cobra.Command, args []string) error { return tunList() },
}

var tunOnCmd = &cobra.Command{
	Use:   "on",
	Short: T("开启 TUN 透明代理"),
	RunE:  func(cmd *cobra.Command, args []string) error { return tunSet(true) },
}

var tunOffCmd = &cobra.Command{
	Use:   "off",
	Short: T("关闭 TUN 透明代理"),
	RunE:  func(cmd *cobra.Command, args []string) error { return tunSet(false) },
}

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: T("DNS 覆写: on/off/use/unuse/list"),
	Long: T("自定义 DNS 会覆盖订阅中的 dns 配置; off/unuse 恢复跟随订阅。") + "\n" +
		"mihomo-cli dns on|off|list      # " + T("等价于 config set dns.enable … / config get dns") + "\n" +
		"mihomo-cli dns use <preset|sub#|ip...>   # " + T("设置 DNS 服务器 (等于 config set dns.nameserver …)"),
	RunE: func(cmd *cobra.Command, args []string) error { return dnsList() },
}

var dnsOnCmd = &cobra.Command{
	Use:   "on",
	Short: T("开启 DNS 覆写"),
	RunE:  func(cmd *cobra.Command, args []string) error { return dnsSet(true) },
}

var dnsOffCmd = &cobra.Command{
	Use:   "off",
	Short: T("关闭 DNS 覆写 (恢复跟随订阅)"),
	RunE:  func(cmd *cobra.Command, args []string) error { return dnsSet(false) },
}

// ---- tun ----

func tunList() error {
	s := mustSettings()
	live := cfg.Fetch(s)
	printConfigSections(s, live, []string{"tun"})
	return nil
}

func tunSet(on bool) error {
	s := mustSettings()
	k := cfg.Lookup("tun.enable")
	if k == nil {
		return fmt.Errorf("%s", T("内部错误: tun.enable 未注册"))
	}
	canon, err := k.Parse(boolWord(on))
	if err != nil {
		return err
	}
	if on {
		if err := ensureTunReady(s, k); err != nil {
			return err
		}
	}
	if err := k.Set(s, canon); err != nil {
		return err
	}
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Printf("tun.enable = %s %s\n", canon, T("已保存"))
	return applyConfig(s, k)
}

// ensureTunReady 开启 TUN 前的三道安全护栏:
//  1. /dev/net/tun 必须存在 (容器常缺)
//  2. 内核需要 CAP_NET_ADMIN → 写进 systemd 单元 AmbientCapabilities
//  3. SSH 自保: auto-route 会把默认路由指进 tun, SSH 会话极易断线,
//     因此当前 SSH 对端网段必须在 tun.route-exclude-address 里
func ensureTunReady(s *app.Settings, k *cfg.Key) error {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return fmt.Errorf("%s: %s", T("无法开启 TUN"),
			T("未找到 /dev/net/tun (容器或内核未启用 TUN); 请改用 mixed-port 端口模式"))
	}
	if err := sysd.EnsureCapabilities(true); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 无法为内核申请 CAP_NET_ADMIN"), err)
	}
	if !sshSafe(s) {
		return fmt.Errorf("%s: %s\n  %s\n  %s",
			T("无法开启 TUN"),
			T("当前 SSH 会话的对端网段未在 tun.route-exclude-address 中, 开启后极可能被自己踢下线"),
			T("请先执行: mihomo-cli config set tun.route-exclude-address "+sshPeerCIDR(sshPeerIP())+",192.168.0.0/16,10.0.0.0/8"),
			T("确认无误后可再次执行 mihomo-cli tun on"))
	}
	return nil
}

// sshSafe SSH 对端是否已在 TUN 排除网段内
func sshSafe(s *app.Settings) bool {
	peer := sshPeerIP()
	if peer == "" {
		return true // 非 SSH 会话 (本地终端/容器), 不拦
	}
	for _, ex := range strings.Split(curExclude(s), ",") {
		if ex = strings.TrimSpace(ex); ex == "" {
			continue
		}
		if sameNet(ex, peer) {
			return true
		}
	}
	return false
}

// curExclude 当前 tun.route-exclude-address 的值
func curExclude(s *app.Settings) string {
	if k := cfg.Lookup("tun.route-exclude-address"); k != nil {
		return k.Get(s)
	}
	return ""
}

// sameNet 判断 CIDR 是否包含 IP (CIDR 解析失败时按字符串相等)
func sameNet(cidr, ip string) bool {
	if _, n, err := net.ParseCIDR(cidr); err == nil {
		return n.Contains(net.ParseIP(ip))
	}
	return cidr == ip
}

// sshPeerIP 从 SSH_CONNECTION 取对端 IP ("ip port ip port")
func sshPeerIP() string {
	v := os.Getenv("SSH_CONNECTION")
	if v == "" {
		v = os.Getenv("SSH_CLIENT")
	}
	f := strings.Fields(v)
	if len(f) == 0 {
		return ""
	}
	if net.ParseIP(f[0]) == nil {
		return ""
	}
	return f[0]
}

// sshPeerCIDR 对端 IP 所在网段 (IPv4 按 /24)
func sshPeerCIDR(ip string) string {
	p := net.ParseIP(ip)
	if p == nil {
		return ""
	}
	if v4 := p.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
	}
	return ip + "/128"
}

// ---- dns ----

func dnsList() error {
	s := mustSettings()
	live := cfg.Fetch(s)
	printConfigSections(s, live, []string{"dns"})
	rows := [][]string{{"*", T("名称"), T("说明"), "IP"}}
	for _, p := range dnsPresets {
		mark := ""
		if sameIPs(p.IPs, s.DNSServers) {
			mark = "*"
		}
		rows = append(rows, []string{mark, p.Name, p.Desc, strings.Join(p.IPs, ", ")})
	}
	for i := range s.Profiles {
		p := &s.Profiles[i]
		ips := subDNS(p.Name)
		if len(ips) == 0 {
			continue
		}
		mark := ""
		if p.Name == s.CurrentProfile && len(s.DNSServers) == 0 {
			mark = "*"
		}
		if len(s.DNSServers) > 0 && sameIPs(ips, s.DNSServers) {
			mark = "*"
		}
		rows = append(rows, []string{mark, fmt.Sprintf("sub%d", i+1), T("订阅") + " " + p.Name, strings.Join(ips, ", ")})
	}
	ui.Table(os.Stdout, rows, 2)
	fmt.Println(T("用法: mihomo-cli dns use <预设|sub#|ip...> | mihomo-cli dns off"))
	return nil
}

func dnsSet(on bool) error {
	return configSet(mustSettings(), "dns.enable", boolWord(on))
}

func boolWord(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

func init() {
	for _, c := range []*cobra.Command{tunOnCmd, tunOffCmd} {
		markMutating(c)
	}
	tunCmd.AddCommand(tunOnCmd, tunOffCmd)
	dnsCmd.AddCommand(dnsOnCmd, dnsOffCmd)
	rootCmd.AddCommand(tunCmd, dnsCmd)
}
