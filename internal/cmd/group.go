package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// groupType 是否为代理分组类型
func groupType(p api.Proxy) bool {
	switch p.Type {
	case "Selector", "URLTest", "Fallback", "LoadBalance":
		return true
	}
	return false
}

// isDirectish 判断是否为直连/拒绝类虚拟节点
func isDirectish(name string) bool {
	switch name {
	case "DIRECT", "REJECT", "REJECT-DROP", "COMPATIBLE", "PASS":
		return true
	}
	return false
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
		if strings.HasPrefix(ln, lw) {
			if hit != "" {
				return want // 多义
			}
			hit = n
		}
		if strings.Contains(ln, lw) {
			if sub != "" {
				sub = "\x00" // 多义标记
			} else {
				sub = n
			}
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

// orderedGroups 返回按名称排序的分组列表 (含 GLOBAL)
func orderedGroups(ps *api.ProxiesResp) []api.Proxy {
	names := make([]string, 0)
	for n, p := range ps.Proxies {
		if groupType(p) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]api.Proxy, 0, len(names))
	for _, n := range names {
		p := ps.Proxies[n]
		p.Name = n
		out = append(out, p)
	}
	return out
}

// resolveGroupArg 解析分组参数: 索引(1..n) | 精确 | 前缀/子串唯一匹配
func resolveGroupArg(ps *api.ProxiesResp, arg string) *api.Proxy {
	groups := orderedGroups(ps)
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(groups) {
		return &groups[n-1]
	}
	if p, ok := ps.Proxies[arg]; ok && groupType(p) {
		p.Name = arg
		return &p
	}
	if m := matchName(ps.Proxies, arg, true); m != "" && m != arg {
		p := ps.Proxies[m]
		p.Name = m
		return &p
	}
	return nil
}

// groupID 返回分组在列表中的编号, 如 "group8"; 未找到返回 "group:-"
func groupID(ps *api.ProxiesResp, name string) string {
	for i, g := range orderedGroups(ps) {
		if g.Name == name {
			return fmt.Sprintf("group%d", i+1)
		}
	}
	return "group:-"
}

// nodeID 返回节点在分组 All 中的编号, 如 "node6"; 未找到返回 "node:-"
func nodeID(g *api.Proxy, name string) string {
	for i, n := range g.All {
		if n == name {
			return fmt.Sprintf("node%d", i+1)
		}
	}
	return "node:-"
}

// printChain 打印当前 sub->group->node 链路
func printChain() {
	s := mustSettings()
	if s.Current() == nil {
		fmt.Printf("%s\n", T("当前无生效订阅(sub 悬空), 代理未生效"))
		return
	}
	sub := fmt.Sprintf("sub%d:%s", subIndex(s, s.Current().Name), s.Current().Name)
	g, n := "group:-", "node:-"
	if c := api.New(s); s.CurrentGroup != "" {
		if ps, err := c.Proxies(); err == nil {
			if gp := resolveGroupArg(ps, s.CurrentGroup); gp != nil {
				g = groupID(ps, gp.Name) + ":" + gp.Name
				if gp.Now != "" {
					n = nodeID(gp, gp.Now) + ":" + gp.Now
				}
			}
		}
	}
	fmt.Printf("%s %s → %s → %s\n", T("当前链路"), sub, g, n)
	if s.CurrentGroup == "" {
		fmt.Println(T("提示: 分组未选择, 可执行 mihomo-cli group use <id|名称>"))
	}
}

func subIndex(s *app.Settings, name string) int {
	for i := range s.Profiles {
		if s.Profiles[i].Name == name {
			return i + 1
		}
	}
	return 1
}

var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: T("查看代理分组列表 (索引别名 #1..#n)"),
	RunE:  groupListRun,
}

func groupListRun(cmd *cobra.Command, args []string) error {
	s := mustSettings()
	if s.Current() == nil {
		return fmt.Errorf("%s", T("没有可用订阅, 请先 mihomo-cli sub use <id|名称>"))
	}
	ps, err := api.New(s).Proxies()
	if err != nil {
		return err
	}
	rows := [][]string{{"", "#", T("分组"), T("类型"), T("当前节点"), T("节点数")}}
	for i, g := range orderedGroups(ps) {
		cur := ""
		if g.Name == s.CurrentGroup {
			cur = "*"
		}
		rows = append(rows, []string{
			cur, strconv.Itoa(i + 1), g.Name, g.Type, g.Now, strconv.Itoa(len(g.All)),
		})
	}
	ui.Table(os.Stdout, rows, 2)
	if s.CurrentGroup == "" {
		fmt.Println(T("未设置当前分组, 请先 mihomo-cli group use <id|名称>"))
	}
	return nil
}

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: T("查看代理分组列表 (索引别名 #1..#n)"),
	RunE:  groupListRun,
}

var groupUseCmd = &cobra.Command{
	Use:   "use <id|name>",
	Short: T("设置当前操作分组"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if s.Current() == nil {
			return fmt.Errorf("%s", T("没有可用订阅, 请先 mihomo-cli sub use <id|名称>"))
		}
		ps, err := api.New(s).Proxies()
		if err != nil {
			return err
		}
		g := resolveGroupArg(ps, args[0])
		if g == nil {
			return fmt.Errorf("%s %q", T("找不到分组"), args[0])
		}
		s.CurrentGroup = g.Name
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s: [%s] (%s)\n", T("当前操作分组已切换为"), g.Name, groupID(ps, g.Name))
		fmt.Printf("mihomo-cli node          # %s (%d)\n", T("节点"), len(g.All))
		return nil
	},
}

// groupUnuseCmd 取消当前分组: 分组切换为 DIRECT 直连, node 上下文悬空
var groupUnuseCmd = &cobra.Command{
	Use:   "unuse",
	Short: T("取消当前分组选择: 该分组流量走 DIRECT 直连"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if s.CurrentGroup == "" {
			fmt.Println(T("当前分组已是悬空状态"))
			return nil
		}
		if c := api.New(s); sysd.IsActive() {
			if ps, err := c.Proxies(); err == nil {
				if g := resolveGroupArg(ps, s.CurrentGroup); g != nil {
					for _, n := range g.All {
						if n == "DIRECT" {
							_ = c.SetProxy(g.Name, "DIRECT")
							break
						}
					}
				}
			}
		}
		s.CurrentGroup = ""
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Println(T("已取消, 该分组流量走 DIRECT 直连"))
		return nil
	},
}

func init() {
	groupCmd.AddCommand(groupListCmd, groupUseCmd, groupUnuseCmd)
	rootCmd.AddCommand(groupCmd)
}
