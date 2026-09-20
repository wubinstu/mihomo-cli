package cmd

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// workingGroup 解析当前操作分组: -g flag > 设置中的 current_group > 报错
func workingGroup(c *api.Client, flagGroup string) (*api.Proxy, error) {
	ps, err := c.Proxies()
	if err != nil {
		return nil, err
	}
	name := flagGroup
	s := mustSettings()
	if name == "" {
		name = s.CurrentGroup
	}
	if name == "" {
		return nil, fmt.Errorf("%s", T("未设置当前分组, 请先 mihomo-cli group use <id|名称>"))
	}
	g := resolveGroupArg(ps, name)
	if g == nil {
		return nil, fmt.Errorf("%s %q", T("找不到分组"), name)
	}
	return g, nil
}

// resolveNodeArg 在分组内解析节点: 索引(1..n, 按 All 顺序) | 精确 | 唯一包含
func resolveNodeArg(g *api.Proxy, ps *api.ProxiesResp, arg string) string {
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(g.All) {
		return g.All[n-1]
	}
	for _, n := range g.All {
		if n == arg {
			return n
		}
	}
	hit := ""
	for _, n := range g.All {
		if strings.Contains(strings.ToLower(n), strings.ToLower(arg)) {
			if hit != "" {
				return "" // 多义
			}
			hit = n
		}
	}
	return hit
}

var nodeGroupFlag string

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: T("列出当前分组的节点 (索引别名 #1..#n)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		g, err := workingGroup(c, nodeGroupFlag)
		if err != nil {
			return err
		}
		rows := [][]string{{"#", T("节点"), T("类型"), ""}}
		for i, n := range g.All {
			typ := "node"
			mark := ""
			if p, ok := ps.Proxies[n]; ok && groupType(p) {
				typ = p.Type
			} else if isDirectish(n) {
				typ = "policy"
			}
			if n == g.Now {
				mark = "<- now"
			}
			rows = append(rows, []string{strconv.Itoa(i + 1), n, typ, mark})
		}
		fmt.Printf("[%s] %d %s\n", g.Name, len(g.All), T("节点"))
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

var nodeSetCmd = &cobra.Command{
	Use:   "set <id|名称>",
	Short: T("切换当前分组到指定节点"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		g, err := workingGroup(c, nodeGroupFlag)
		if err != nil {
			return err
		}
		if g.Type != "Selector" {
			return fmt.Errorf("%s: %s (%s)", T("切换失败"), g.Name, g.Type)
		}
		node := resolveNodeArg(g, ps, args[0])
		if node == "" {
			return fmt.Errorf("%q %s", args[0], T("不在该分组中"))
		}
		if err := c.SetProxy(g.Name, node); err != nil {
			return err
		}
		fmt.Printf(T("[%s] %s -> %s")+"\n", g.Name, g.Now, node)
		if s.ProxyAutoSelectEnabled {
			fmt.Fprintln(os.Stderr, T("手动切换成功。注意: 自动择优已开启, 下次定时任务可能覆盖此设置"))
		}
		return nil
	},
}

var nodeTestCmd = &cobra.Command{
	Use:   "test",
	Short: T("测试当前分组节点延迟 (彩色)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		g, err := workingGroup(c, nodeGroupFlag)
		if err != nil {
			return err
		}
		fmt.Printf("%s [%s] (url=%s timeout=%dms) ...\n", T("测试分组"), g.Name, s.TestURL, s.TestTimeout)
		delay, err := c.GroupDelay(g.Name, s.TestURL, s.TestTimeout)
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
		rows := [][]string{{T("延迟"), T("节点")}}
		for _, e := range list {
			rows = append(rows, []string{ui.ColorDelay(e.d), e.name})
		}
		if n := len(g.All) - len(list); n > 0 {
			rows = append(rows, []string{ui.ColorDelay(-1), fmt.Sprintf("(%d %s)", n, T("超时"))})
		}
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

var nodeAutoCmd = &cobra.Command{
	Use:   "auto [id|名称]",
	Short: T("对分组测速并切换到延迟最低的节点"),
	Long: `定时任务调用形式 (mihomo-cli-auto.timer):
  未指定分组时: 使用 proxy-auto-select-group 设置; 也为空则对全部含真实节点的分组择优
仅在真实节点(排除子分组/DIRECT/REJECT)中择优, 策略组自动跳过。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		ps, err := c.Proxies()
		if err != nil {
			return err
		}
		// 目标分组
		var groups []string
		switch {
		case len(args) > 0:
			if g := resolveGroupArg(ps, args[0]); g != nil {
				groups = []string{g.Name}
			}
		case nodeGroupFlag != "":
			if g := resolveGroupArg(ps, nodeGroupFlag); g != nil {
				groups = []string{g.Name}
			}
		case s.ProxyAutoSelectGroup != "":
			if g := resolveGroupArg(ps, s.ProxyAutoSelectGroup); g != nil {
				groups = []string{g.Name}
			}
		default:
			for _, g := range orderedGroups(ps) {
				if g.Type == "Selector" && countRealNodes(ps.Proxies, g) > 0 {
					groups = append(groups, g.Name)
				}
			}
		}
		if len(groups) == 0 {
			return fmt.Errorf("%s", T("没有 Selector 分组"))
		}
		failed := 0
		for _, gname := range groups {
			gp := ps.Proxies[gname]
			if gp.Type != "Selector" || countRealNodes(ps.Proxies, gp) == 0 {
				fmt.Printf("%s [%s] (%s)\n", T("跳过"), gname, T("无真实节点, 策略组"))
				continue
			}
			delay, err := c.GroupDelay(gname, s.TestURL, s.TestTimeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] %s: %v\n", gname, T("测速失败"), err)
				failed++
				continue
			}
			best, bestD := "", math.MaxInt
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
				fmt.Fprintf(os.Stderr, "[%s] %s\n", gname, T("全部节点不可用"))
				failed++
				continue
			}
			if err := c.SetProxy(gname, best); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] %s: %v\n", gname, T("切换失败"), err)
				failed++
				continue
			}
			fmt.Printf(ui.ColorDelay(bestD)+" [%s] -> %s\n", gname, best)
		}
		if failed > 0 {
			return fmt.Errorf("%d %s", failed, T("切换失败"))
		}
		return nil
	},
}

func init() {
	for _, sub := range []*cobra.Command{nodeCmd, nodeSetCmd, nodeTestCmd, nodeAutoCmd} {
		sub.Flags().StringVarP(&nodeGroupFlag, "group", "g", "", T("分组")+" (id|名称)")
	}
	nodeCmd.AddCommand(nodeSetCmd, nodeTestCmd, nodeAutoCmd)
	rootCmd.AddCommand(nodeCmd)
}
