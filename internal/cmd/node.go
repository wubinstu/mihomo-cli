package cmd

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/geo"
	"github.com/wubinstu/mihomo-cli/internal/subs"
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
	RunE:  nodeListRun,
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: T("列出当前分组的节点 (索引别名 #1..#n)"),
	RunE:  nodeListRun,
}

func nodeListRun(cmd *cobra.Command, args []string) error {
	s := mustSettings()
	if s.Current() == nil {
		return fmt.Errorf("%s", T("没有可用订阅, 请先 mihomo-cli sub use <id|名称>"))
	}
	c := api.New(s)
	ps, err := c.Proxies()
	if err != nil {
		return err
	}
	g, err := workingGroup(c, nodeGroupFlag)
	if err != nil {
		return err
	}
	// 节点服务器地址 -> 地区 (并发, 结果进 region map)
	servers := nodeServers(s)
	regions := make(map[string]string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, n := range g.All {
		if p, ok := ps.Proxies[n]; ok && groupType(p) {
			continue // 子分组
		}
		if isDirectish(n) {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			iso := geo.Region(servers[name])
			if iso == "" {
				return
			}
			mu.Lock()
			regions[name] = geo.Name(iso)
			mu.Unlock()
		}(n)
	}
	wg.Wait()

	rows := [][]string{{"*", "#", T("节点"), T("类型"), T("地区")}}
	for i, n := range g.All {
		typ := "node"
		mark := ""
		region := "-"
		if p, ok := ps.Proxies[n]; ok && groupType(p) {
			typ = p.Type
		} else if isDirectish(n) {
			typ = "policy"
		} else if r, ok := regions[n]; ok {
			region = r
		}
		if n == g.Now {
			mark = "*"
		}
		rows = append(rows, []string{mark, strconv.Itoa(i + 1), n, typ, region})
	}
	fmt.Printf("[%s] %d %s\n", g.Name, len(g.All), T("节点"))
	ui.Table(os.Stdout, rows, 2)
	return nil
}

// nodeServers 从订阅文件提取 节点名 -> server 映射
func nodeServers(s *app.Settings) map[string]string {
	out := map[string]string{}
	p := s.Current()
	if p == nil {
		return out
	}
	data, err := os.ReadFile(subs.Path(p.Name))
	if err != nil {
		return out
	}
	var cfg struct {
		Proxies []struct {
			Name   string `yaml:"name"`
			Server string `yaml:"server"`
		} `yaml:"proxies"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return out
	}
	for _, px := range cfg.Proxies {
		out[px.Name] = px.Server
	}
	return out
}

var nodeUseCmd = &cobra.Command{
	Use:   "use <id|name>",
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
		if s.NodeAutoSelectEnabled {
			fmt.Fprintf(os.Stderr, "\x1b[33m%s\x1b[0m\n",
				T("注意: 自动择优已开启, 下次定时任务可能覆盖此设置"))
		}
		return nil
	},
}

// nodeUnuseCmd 取消当前节点: 分组切换为 DIRECT 直连
var nodeUnuseCmd = &cobra.Command{
	Use:   "unuse",
	Short: T("取消当前节点选择: 分组流量走 DIRECT 直连"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		g, err := workingGroup(api.New(s), nodeGroupFlag)
		if err != nil {
			return err
		}
		found := false
		for _, n := range g.All {
			if n == "DIRECT" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: %s (DIRECT %s)", T("取消失败"), g.Name, T("不在该分组中"))
		}
		c := api.New(s)
		if err := c.SetProxy(g.Name, "DIRECT"); err != nil {
			return err
		}
		fmt.Println(T("已取消, 分组流量走 DIRECT 直连"))
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

// nodeAutoCmd 仅作用于当前操作分组 (sub->group 链路) 的择优; 定时任务复用此命令
var nodeAutoCmd = &cobra.Command{
	Use:   "auto",
	Short: T("对当前分组测速并切换到延迟最低的节点"),
	Long: T("仅在当前 use 的分组内, 于真实节点(排除子分组/DIRECT/REJECT)中选择延迟最低者切换;") + "\n" +
		T("定时任务 (node-auto-select-enabled) 周期性执行本命令; 未设置当前分组时跳过。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		g, err := workingGroup(c, nodeGroupFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", T("无当前分组, 跳过自动择优 (mihomo-cli group use <id|名称>)"))
			return nil
		}
		if g.Type != "Selector" || countRealNodes(nil2map(c), *g) == 0 {
			fmt.Fprintf(os.Stderr, "%s [%s] (%s)\n", T("跳过"), g.Name, T("无真实节点, 策略组"))
			return nil
		}
		delay, err := c.GroupDelay(g.Name, s.TestURL, s.TestTimeout)
		if err != nil {
			return fmt.Errorf("%s: %w", T("测速失败"), err)
		}
		ps, _ := c.Proxies()
		best, bestD := "", math.MaxInt
		for n, d := range delay {
			if d <= 0 || d >= bestD {
				continue
			}
			if ps != nil {
				if p, ok := ps.Proxies[n]; ok && groupType(p) {
					continue // 子分组不参与择优
				}
			}
			if isDirectish(n) {
				continue
			}
			best, bestD = n, d
		}
		if best == "" {
			return fmt.Errorf("[%s] %s", g.Name, T("全部节点不可用"))
		}
		if err := c.SetProxy(g.Name, best); err != nil {
			return err
		}
		s.AutoSelectLastRun = time.Now()
		_ = s.Save()
		fmt.Printf(ui.ColorDelay(bestD)+" [%s] -> %s (%s)\n", g.Name, best, nodeID(g, best))
		return nil
	},
}

func nil2map(c *api.Client) map[string]api.Proxy {
	if ps, err := c.Proxies(); err == nil {
		return ps.Proxies
	}
	return map[string]api.Proxy{}
}

func init() {
	for _, sub := range []*cobra.Command{nodeCmd, nodeUseCmd, nodeUnuseCmd, nodeTestCmd, nodeAutoCmd} {
		sub.Flags().StringVarP(&nodeGroupFlag, "group", "g", "", T("分组")+" (id|name)")
	}
	for _, c := range []*cobra.Command{nodeUseCmd, nodeUnuseCmd, nodeAutoCmd} {
		markMutating(c)
	}
	for _, c := range []*cobra.Command{nodeUseCmd, nodeUnuseCmd} {
		c.ValidArgsFunction = nodeArgComp
	}
	for _, c := range []*cobra.Command{nodeCmd, nodeUseCmd, nodeUnuseCmd, nodeTestCmd, nodeAutoCmd} {
		c.RegisterFlagCompletionFunc("group", groupFlagComp)
	}
	nodeCmd.AddCommand(nodeListCmd, nodeUseCmd, nodeUnuseCmd, nodeTestCmd, nodeAutoCmd)
	rootCmd.AddCommand(nodeCmd)
}
