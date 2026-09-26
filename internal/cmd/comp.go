package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/subs"
)

// ---- 补全数据源 (全部零副作用: 服务没起/配置读不到就返回空, 绝不退出进程) ----

// subCompletions 订阅名 + #id
func subCompletions(toComplete string) []string {
	s := mustSettingsQuiet()
	if s == nil {
		return nil
	}
	var out []string
	for i := range s.Profiles {
		p := &s.Profiles[i]
		id := strconv.Itoa(i + 1)
		if strings.HasPrefix(id, toComplete) {
			out = append(out, id)
		}
		if strings.HasPrefix(p.Name, toComplete) {
			out = append(out, p.Name)
		}
		if toComplete == "" {
			out = append(out, id)
		}
	}
	return dedupStr(out)
}

// groupCompletions 分组名 + #id (需要内核 API; 失败时静默返回空)
func groupCompletions(toComplete string) []string {
	s := mustSettingsQuiet()
	if s == nil {
		return nil
	}
	ps, err := api.New(s).Proxies()
	if err != nil || ps == nil {
		return nil
	}
	var out []string
	for i, g := range orderedGroups(ps) {
		id := strconv.Itoa(i + 1)
		if strings.HasPrefix(id, toComplete) {
			out = append(out, id)
		}
		if strings.HasPrefix(g.Name, toComplete) {
			out = append(out, g.Name)
		}
	}
	return dedupStr(out)
}

// nodeCompletions 当前分组的节点名 + #id; flagGroup 优先, 否则 current_group
func nodeCompletions(toComplete, flagGroup string) []string {
	s := mustSettingsQuiet()
	if s == nil {
		return nil
	}
	c := api.New(s)
	ps, err := c.Proxies()
	if err != nil || ps == nil {
		return nil
	}
	g := resolveGroupArg(ps, firstNon(flagGroup, s.CurrentGroup))
	if g == nil {
		return nil
	}
	var out []string
	for i, n := range g.All {
		id := strconv.Itoa(i + 1)
		if strings.HasPrefix(id, toComplete) {
			out = append(out, id)
		}
		if strings.HasPrefix(n, toComplete) {
			out = append(out, n)
		}
	}
	return dedupStr(out)
}

func firstNon(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func dedupStr(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// priorityCompletions 只保留前缀匹配的项 (TAB 输入了一部分时)
func priorityCompletions(list []string, toComplete string) []string {
	if toComplete == "" {
		return list
	}
	var out []string
	for _, v := range list {
		if strings.HasPrefix(strings.ToLower(v), strings.ToLower(toComplete)) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return out
	}
	return out
}

// withNoFile 包装补全结果, 统一禁止文件补全
func withNoFile(list []string) ([]string, cobra.ShellCompDirective) {
	return list, cobra.ShellCompDirectiveNoFileComp
}

// ---- sub 补全 ----

func subArgComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFileComp()
	}
	return withNoFile(priorityCompletions(subCompletions(toComplete), toComplete))
}

// ---- group 补全 ----

func groupArgComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFileComp()
	}
	return withNoFile(priorityCompletions(groupCompletions(toComplete), toComplete))
}

// groupFlagComp -g/--group 的值补全
func groupFlagComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return withNoFile(priorityCompletions(groupCompletions(toComplete), toComplete))
}

// ---- node 补全 ----

func nodeArgComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFileComp()
	}
	return withNoFile(priorityCompletions(nodeCompletions(toComplete, nodeGroupFlag), toComplete))
}

// ---- rule 补全: #id ----

func ruleIDComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFileComp()
	}
	s := mustSettingsQuiet()
	if s == nil {
		return nil, noFileComp()
	}
	var out []string
	for i := range s.UserRules {
		id := strconv.Itoa(i + 1)
		if strings.HasPrefix(id, toComplete) {
			out = append(out, id)
		}
	}
	return withNoFile(out)
}

// ---- top kill 补全: 连接编号 ----

func connIDComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	s := mustSettingsQuiet()
	if s == nil {
		return nil, noFileComp()
	}
	r, err := api.New(s).Connections()
	if err != nil || r == nil {
		return nil, noFileComp()
	}
	var out []string
	for i := range r.Connections {
		id := strconv.Itoa(i + 1)
		if strings.HasPrefix(id, toComplete) {
			out = append(out, id)
		}
	}
	return withNoFile(out)
}

// installCompletionsFor 把补全挂到命令 (统一入口)
func installArgComp(c *cobra.Command, fn func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	if c.ValidArgsFunction == nil {
		c.ValidArgsFunction = fn
	}
}

var (
	_ = fmt.Sprintf
	_ = subs.Sanitize
	_ = app.BaseDir
)
