package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

func groupType(p api.Proxy) bool {
	switch p.Type {
	case "Selector", "URLTest", "Fallback", "LoadBalance":
		return true
	}
	return false
}

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "查看/切换代理分组与节点",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		ps, err := api.New(s).Proxies()
		if err != nil {
			return err
		}
		names := make([]string, 0)
		for n, p := range ps.Proxies {
			if groupType(p) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		rows := [][]string{strings.Split(T("proxy.hdr"), "\t")}
		for _, n := range names {
			p := ps.Proxies[n]
			rows = append(rows, []string{n, p.Type, p.Now, fmt.Sprintf("%d", len(p.All))})
		}
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

var proxySetCmd = &cobra.Command{
	Use:   "set <group> <node>",
	Short: "切换分组到指定节点 (支持唯一前缀模糊匹配)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		group := matchName(ps.Proxies, args[0], true)
		if group == "" {
			return fmt.Errorf("找不到分组 %q", args[0])
		}
		g := ps.Proxies[group]
		if g.Type != "Selector" {
			return fmt.Errorf("分组 %s 类型为 %s, 不支持手动切换", group, g.Type)
		}
		node := ""
		for _, n := range g.All {
			if n == args[1] {
				node = n
				break
			}
		}
		if node == "" {
			for _, n := range g.All {
				if strings.Contains(strings.ToLower(n), strings.ToLower(args[1])) {
					if node != "" {
						return fmt.Errorf("%q 匹配到多个节点, 请更精确", args[1])
					}
					node = n
				}
			}
		}
		if node == "" {
			return fmt.Errorf("节点 %q 不在分组 %s 中", args[1], group)
		}
		if err := c.SetProxy(group, node); err != nil {
			return err
		}
		fmt.Printf("[%s] %s -> %s\n", group, g.Now, node)
		return nil
	},
}

var proxyTestCmd = &cobra.Command{
	Use:   "test [group]",
	Short: "测试分组内节点延迟",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		group := ""
		if len(args) > 0 {
			group = args[0]
		}
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		if group == "" {
			if _, ok := ps.Proxies["GLOBAL"]; ok {
				group = "GLOBAL"
			} else {
				return fmt.Errorf("请指定分组")
			}
		}
		group = matchName(ps.Proxies, group, true)
		fmt.Printf("测试分组 [%s] (url=%s timeout=%dms) ...\n", group, s.TestURL, s.TestTimeout)
		delay, err := c.GroupDelay(group, s.TestURL, s.TestTimeout)
		if err != nil {
			return err
		}
		type kv struct {
			name string
			d    int
		}
		list := make([]kv, 0, len(delay))
		for n, d := range delay {
			list = append(list, kv{n, d})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].d < list[j].d })
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ms\tNODE")
		for _, e := range list {
			fmt.Fprintf(w, "%d\t%s\n", e.d, e.name)
		}
		if n := len(ps.Proxies[group].All) - len(list); n > 0 {
			fmt.Fprintf(w, "timeout\t(%d)\n", n)
		}
		return w.Flush()
	},
}

var proxyAutoCmd = &cobra.Command{
	Use:   "auto [group...]",
	Short: "对分组测速并切换到延迟最低的节点 (供定时任务/手动调用)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		groups := args
		if len(groups) == 0 {
			groups = s.AutoGroups
		}
		if len(groups) == 0 {
			for n, p := range ps.Proxies {
				if p.Type == "Selector" && n != "GLOBAL" {
					groups = append(groups, n)
				}
			}
			sort.Strings(groups)
		}
		if len(groups) == 0 {
			return fmt.Errorf("没有 Selector 分组")
		}
		failed := 0
		for _, g := range groups {
			orig := g
			g = matchName(ps.Proxies, g, true)
			if g == "" {
				fmt.Fprintf(os.Stderr, "跳过: 找不到分组 %q\n", orig)
				failed++
				continue
			}
			gp := ps.Proxies[g]
			if gp.Type != "Selector" {
				fmt.Fprintf(os.Stderr, "跳过: %s 类型为 %s, 非手动分组\n", g, gp.Type)
				continue
			}
			// 仅在真实节点(非子组/非 DIRECT/REJECT)中择优; 无真实节点的策略组(如全球直连)跳过
			if countRealNodes(ps.Proxies, gp) == 0 {
				fmt.Printf("跳过 [%s] (无真实节点, 策略组)\n", g)
				continue
			}
			delay, err := c.GroupDelay(g, s.TestURL, s.TestTimeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] 测速失败: %v\n", g, err)
				failed++
				continue
			}
			best, bestD := "", 1<<62
			for n, d := range delay {
				if d <= 0 || d >= bestD {
					continue
				}
				if p, ok := ps.Proxies[n]; ok && groupType(p) {
					continue // 子分组不参与择优
				}
				if isDirectish(n) {
					continue
				}
				best, bestD = n, d
			}
			if best == "" {
				fmt.Fprintf(os.Stderr, "[%s] 没有可用节点\n", g)
				failed++
				continue
			}
			if err := c.SetProxy(g, best); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] 切换失败: %v\n", g, err)
				failed++
				continue
			}
			fmt.Printf("[%s] -> %s (%d ms)\n", g, best, bestD)
		}
		if failed > 0 {
			return fmt.Errorf("%d 个分组处理失败", failed)
		}
		return nil
	},
}

// isDirectish 判断是否为直连/拒绝类虚拟节点
func isDirectish(name string) bool {
	switch name {
	case "DIRECT", "REJECT", "REJECT-DROP", "COMPATIBLE", "PASS":
		return true
	}
	return strings.Contains(name, "直连") || strings.Contains(name, "拦截")
}

// countRealNodes 统计组内真实节点数(非代理组、非直连/拒绝)
func countRealNodes(proxies map[string]api.Proxy, g api.Proxy) int {
	n := 0
	for _, name := range g.All {
		if p, ok := proxies[name]; ok && groupType(p) {
			continue // 子分组
		}
		if isDirectish(name) {
			continue
		}
		n++
	}
	return n
}

// matchName 精确匹配, 否则唯一前缀/子串匹配; onlyGroup 限定分组
func matchName(proxies map[string]api.Proxy, want string, onlyGroup bool) string {
	if _, ok := proxies[want]; ok && (!onlyGroup || groupType(proxies[want])) {
		return want
	}
	hit, sub := "", ""
	for n, p := range proxies {
		if onlyGroup && !groupType(p) {
			continue
		}
		ln, lw := strings.ToLower(n), strings.ToLower(want)
		if strings.HasPrefix(ln, lw) && hit == "" {
			hit = n
		} else if strings.HasPrefix(ln, lw) {
			return want // 多义
		}
		if strings.Contains(ln, lw) && sub == "" {
			sub = n
		} else if strings.Contains(ln, lw) {
			sub = "\x00" // 多义标记
		}
	}
	if hit != "" {
		return hit
	}
	if sub != "" && sub != "\x00" {
		return sub
	}
	return want // 未命中, 返回原值让调用方报错
}

// proxyOnCmd 开启代理: 确保服务运行, 输出可 eval 的环境变量
// 推荐 alias: alias proxy_on='eval $(mihomo-cli proxy on)'
var proxyOnCmd = &cobra.Command{
	Use:   "on",
	Short: "开启代理: 启动服务并输出代理环境变量 (eval $(mihomo-cli proxy on))",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if !sysd.IsActive() {
			if s.Current() == nil {
				return fmt.Errorf("没有订阅, 请先 mihomo-cli init")
			}
			if err := render.Generate(s); err != nil {
				return err
			}
			if err := sysd.Service("start"); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, T("proxy.on"))
		}
		return envCmd.RunE(envCmd, args)
	},
}

// proxyOffCmd 关闭当前 shell 的代理环境变量 (服务保持运行)
// 推荐 alias: alias proxy_off='eval $(mihomo-cli proxy off)'
var proxyOffCmd = &cobra.Command{
	Use:   "off",
	Short: "关闭当前 shell 的代理环境变量 (服务保持运行)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(`unset http_proxy https_proxy all_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY`)
		return nil
	},
}

func init() {
	proxyCmd.AddCommand(proxySetCmd, proxyTestCmd, proxyAutoCmd, proxyOnCmd, proxyOffCmd)
	rootCmd.AddCommand(proxyCmd)
}
