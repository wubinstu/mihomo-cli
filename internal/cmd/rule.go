package cmd

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// ruleTypeDef 规则类型定义
type ruleTypeDef struct {
	Name        string
	Desc        string // T() 翻译
	Example     string
	IPRelated   bool // 允许 no-resolve
	NoCondition bool // MATCH
}

var ruleTypes = []ruleTypeDef{
	{"DOMAIN", T("匹配完整域名"), "example.com", false, false},
	{"DOMAIN-SUFFIX", T("匹配域名后缀"), "example.com", false, false},
	{"DOMAIN-KEYWORD", T("匹配域名关键字"), "example", false, false},
	{"DOMAIN-REGEX", T("匹配域名正则表达式"), "example.*", false, false},
	{"GEOSITE", T("匹配 GeoSite 内的域名"), "youtube", false, false},
	{"GEOIP", T("匹配 IP 所属国家代码"), "CN", true, false},
	{"SRC-GEOIP", T("匹配来源 IP 所属国家代码"), "CN", true, false},
	{"IP-ASN", T("匹配 IP 所属 ASN"), "13335", true, false},
	{"SRC-IP-ASN", T("匹配来源 IP 所属 ASN"), "9808", true, false},
	{"IP-CIDR", T("匹配 IP 地址范围"), "127.0.0.0/8", true, false},
	{"IP-CIDR6", T("匹配 IPv6 地址范围"), "2620:0:2d0:200::7/32", true, false},
	{"SRC-IP-CIDR", T("匹配来源 IP 地址范围"), "192.168.1.201/32", true, false},
	{"IP-SUFFIX", T("匹配 IP 后缀范围"), "8.8.8.8/24", true, false},
	{"SRC-IP-SUFFIX", T("匹配来源 IP 后缀范围"), "192.168.1.201/8", true, false},
	{"SRC-PORT", T("匹配请求来源端口范围"), "7777", false, false},
	{"DST-PORT", T("匹配请求目标端口范围"), "80", false, false},
	{"IN-PORT", T("匹配入站端口"), "7897", false, false},
	{"DSCP", T("匹配 DSCP 标记"), "4", false, false},
	{"PROCESS-NAME", T("匹配进程名称"), "wget", false, false},
	{"PROCESS-PATH", T("匹配完整进程路径"), "/usr/bin/wget", false, false},
	{"PROCESS-NAME-REGEX", T("正则匹配进程名称"), ".*telegram.*", false, false},
	{"PROCESS-PATH-REGEX", T("正则匹配完整进程路径"), `(?i).*chrome.*`, false, false},
	{"NETWORK", T("匹配 TCP/UDP"), "udp", false, false},
	{"UID", T("匹配 Linux UserID"), "1001", false, false},
	{"IN-TYPE", T("匹配入站类型"), "SOCKS/HTTP", false, false},
	{"IN-USER", T("匹配入站用户"), "mihomo", false, false},
	{"IN-NAME", T("匹配入站名称"), "ss", false, false},
	{"AND", T("逻辑与"), "((DOMAIN,baidu.com),(NETWORK,TCP))", false, false},
	{"OR", T("逻辑或"), "((DOMAIN,baidu.com),(NETWORK,TCP))", false, false},
	{"NOT", T("逻辑非"), "((DOMAIN,baidu.com))", false, false},
	{"MATCH", T("匹配所有请求"), "<none>", false, true},
}

func ruleTypeDoc(name string) *ruleTypeDef {
	for i := range ruleTypes {
		if ruleTypes[i].Name == name {
			return &ruleTypes[i]
		}
	}
	return nil
}

// 内置策略
var builtinPolicies = []struct{ Name, Desc string }{
	{"DIRECT", T("直接连接")},
	{"REJECT", T("拦截请求")},
	{"REJECT-DROP", T("丢弃请求")},
	{"PASS", T("跳过此项")},
}

// validateCondition 简单本地校验; 完整校验交给内核 reload
func validateCondition(td *ruleTypeDef, cond string) string {
	if td.NoCondition {
		return ""
	}
	if cond == "" {
		return T("缺少匹配条件")
	}
	switch td.Name {
	case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR":
		if _, _, err := net.ParseCIDR(cond); err != nil {
			return T("无效 CIDR")
		}
	case "IP-SUFFIX", "SRC-IP-SUFFIX":
		if !strings.Contains(cond, "/") {
			return T("无效 IP 后缀 (格式 8.8.8.8/24)")
		}
	case "GEOIP", "SRC-GEOIP":
		if len(cond) != 2 {
			return T("无效国家代码 (如 CN)")
		}
	case "IP-ASN", "SRC-IP-ASN":
		if _, err := strconv.Atoi(cond); err != nil {
			return T("无效 ASN 编号")
		}
	case "DST-PORT", "SRC-PORT", "IN-PORT", "DSCP", "UID":
		if _, err := strconv.Atoi(cond); err != nil {
			return T("无效数字")
		}
	}
	return ""
}

// strategyDesc 策略中文描述
func strategyDesc(name string) string {
	for _, p := range builtinPolicies {
		if p.Name == name {
			return p.Desc
		}
	}
	return T("走代理组") + " " + name
}

// resolveStrategy 解析策略: 内置 | PROXY=当前分组 | 模糊匹配分组名
func resolveStrategy(s *app.Settings, raw string) (string, error) {
	for _, p := range builtinPolicies {
		if strings.EqualFold(raw, p.Name) {
			return p.Name, nil
		}
	}
	c := api.New(s)
	ps, err := c.Proxies()
	if err != nil {
		return raw, nil // 内核不可达时原样写入
	}
	if strings.EqualFold(raw, "PROXY") {
		if s.CurrentGroup == "" {
			return "", fmt.Errorf("%s", T("PROXY 需要先 group use 选定分组"))
		}
		return s.CurrentGroup, nil
	}
	if g := resolveGroupArg(ps, raw); g != nil {
		return g.Name, nil
	}
	return "", fmt.Errorf("%s %q (%s)", T("未知策略"), raw, "DIRECT/REJECT/分组名")
}

// ---- apply + 内核校验回滚 ----

func applyRules(s *app.Settings) error {
	if err := s.Save(); err != nil {
		return err
	}
	if s.Current() == nil {
		return nil
	}
	if err := render.Generate(s); err != nil {
		return err
	}
	if sysd.IsActive() {
		c := api.New(s)
		if err := c.Reload(app.RuntimeConfig); err != nil {
			// 内核校验失败: 回滚配置文件 (内核仍在旧配置上运行)
			fmt.Fprintf(os.Stderr, "\x1b[33m%s: %v\x1b[0m\n", T("内核校验失败, 已回滚"), err)
			_ = s.Save()
			_ = render.Generate(s)
			return fmt.Errorf("%s", T("规则未生效"))
		}
		fmt.Println(T("已热重载配置"))
	}
	if m, err := api.New(s).ConfigMode(); err == nil && m != "rule" {
		fmt.Fprintf(os.Stderr, "\x1b[33m%s\x1b[0m\n",
			T("注意: 当前代理模式不是 rule, 规则暂不生效 (set proxy-mode rule)"))
	}
	return nil
}

// ---- list ----

var (
	ruleWithSub   bool
	filterType    string
	filterCond    string
	filterStrat   string
	filterNoRes   bool
)

func filterMatch(typ, cond, strat string, noRes bool) bool {
	if filterType != "" && !strings.EqualFold(typ, filterType) {
		return false
	}
	if filterCond != "" && cond != filterCond {
		return false
	}
	if filterStrat != "" && !strings.EqualFold(strat, filterStrat) {
		return false
	}
	if filterNoRes && !noRes {
		return false
	}
	return true
}

func ruleListRun(cmd *cobra.Command, args []string) error {
	s := mustSettings()
	rows := [][]string{{"*", "#", "TYPE", T("匹配条件"), T("代理策略"), "no-resolve"}}
	for i, r := range s.UserRules {
		if !filterMatch(r.Type, r.Condition, r.Strategy, r.NoResolve) {
			continue
		}
		star := ""
		if r.Enabled {
			star = "*"
		}
		rows = append(rows, []string{star, strconv.Itoa(i + 1), r.Type, r.Condition, r.Strategy, boolMark(r.NoResolve)})
	}
	fmt.Printf("%s: %d\n", T("用户规则"), len(rows)-1)
	if len(rows) > 1 {
		ui.Table(os.Stdout, rows, 2)
	}
	if ruleWithSub {
		sr := subRules(s)
		shown := 0
		rows2 := [][]string{{"*", "#", "TYPE", T("匹配条件"), T("代理策略"), "no-resolve"}}
		for _, raw := range sr {
			parts := strings.Split(raw, ",")
			if len(parts) < 3 {
				continue
			}
			typ, cond, strat := parts[0], parts[1], parts[2]
			noRes := len(parts) >= 4 && parts[3] == "no-resolve"
			if !filterMatch(typ, cond, strat, noRes) {
				continue
			}
			shown++
			rows2 = append(rows2, []string{"", strconv.Itoa(shown), typ, cond, strat, boolMark(noRes)})
		}
		fmt.Printf("%s: %d/%d\n", T("订阅规则"), shown, len(sr))
		if shown > 0 {
			ui.Table(os.Stdout, rows2, 2)
		}
	} else {
		fmt.Println(T("提示: rule list --sub 查看订阅规则"))
	}
	return nil
}

func boolMark(b bool) string {
	if b {
		return "√"
	}
	return "-"
}

// subRules 读取当前订阅的规则列表
func subRules(s *app.Settings) []string {
	p := s.Current()
	if p == nil {
		return nil
	}
	data, err := os.ReadFile(subs.Path(p.Name))
	if err != nil {
		return nil
	}
	var cfg struct {
		Rules []string `yaml:"rules"`
	}
	_ = yaml.Unmarshal(data, &cfg)
	return cfg.Rules
}

var ruleCmd = &cobra.Command{
	Use:   "rule",
	Short: T("用户自定义规则 (独立于订阅, 优先匹配; list --sub 同时显示订阅规则)"),
	Long: T("用户规则独立于订阅保存, 不会因订阅更新/删除/更换而丢失; 匹配优先级高于订阅规则。") + `

mihomo-cli rule                       # ` + T("显示用户规则") + `
mihomo-cli rule list --sub            # ` + T("同时显示订阅规则") + `
mihomo-cli rule list --type DOMAIN [--condition ..] [--strategy ..] [--no-resolve]   # ` + T("过滤") + `
mihomo-cli rule add --type <类型> --condition <条件> --strategy <策略> [--no-resolve]
mihomo-cli rule enable|disable <#id>
mihomo-cli rule rm <#id>

` + T("规则由内核执行; CLI 负责拼接写入并经内核 reload 校验, 不合法会被拒绝并回滚。"),
	RunE: ruleListRun,
}

var ruleListCmd = &cobra.Command{
	Use:   "list",
	Short: T("显示用户规则 (--sub 同时显示订阅规则)"),
	RunE:  ruleListRun,
}

// ---- add ----

var (
	addType, addCond, addStrategy string
	addNoResolve                  bool
)

var ruleAddCmd = &cobra.Command{
	Use:   "add",
	Short: T("添加规则 (结构化参数)"),
	Long: T("添加规则") + `: --type <` + T("规则类型") + `> --condition <` + T("匹配条件") + `> --strategy <` + T("代理策略") + `> [--no-resolve]
` + T("策略: DIRECT/REJECT/REJECT-DROP/PASS/PROXY(当前分组)/分组名; 语法由内核校验, 失败回滚。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if addType == "" && addCond == "" && addStrategy == "" {
			return cmd.Help()
		}
		s := mustSettings()
		td := ruleTypeDoc(addType)
		if td == nil {
			return fmt.Errorf("%s %q (%s)", T("未知规则类型"), addType, "rule add --type -h")
		}
		if msg := validateCondition(td, addCond); msg != "" {
			return fmt.Errorf("%s: %s", msg, addCond)
		}
		strategy := addStrategy
		if strategy == "" {
			strategy = "DIRECT"
		}
		resolved, err := resolveStrategy(s, strategy)
		if err != nil {
			return err
		}
		if addNoResolve && !td.IPRelated {
			fmt.Fprintf(os.Stderr, "\x1b[33m%s\x1b[0m\n", T("注意: no-resolve 一般仅用于 IP 类规则, 已忽略"))
			addNoResolve = false
		}
		r := app.UserRule{
			Type: td.Name, Condition: addCond, Strategy: resolved,
			NoResolve: addNoResolve, Enabled: true,
		}
		s.UserRules = append(s.UserRules, r)
		td2 := ruleTypeDoc(addType)
		fmt.Printf("%s %s %s %s: %s\n",
			T("该规则选中了所有"), td2.Desc, condText(td2, addCond), T("的流量"), strategyDesc(resolved))
		fmt.Printf("  -> %s\n", r.String())
		return applyRules(s)
	},
}

func condText(td *ruleTypeDef, cond string) string {
	if td.NoCondition {
		return ""
	}
	return cond
}

// addHelp 实现 rule add -h 的上下文帮助
func addHelp(cmd *cobra.Command, args []string) {
	typ, _ := cmd.Flags().GetString("type")
	cond, _ := cmd.Flags().GetString("condition")
	strat, _ := cmd.Flags().GetString("strategy")
	noRes, _ := cmd.Flags().GetBool("no-resolve")

	fmt.Println("mihomo-cli rule add --type <type> --condition <cond> --strategy <strategy> [--no-resolve]")
	switch {
	case typ == "":
		// 类型总览
		fmt.Println("\n" + T("规则类型") + " (--type):")
		rows := [][]string{{"TYPE", T("说明"), T("示例")}}
		for _, rt := range ruleTypes {
			rows = append(rows, []string{rt.Name, rt.Desc, rt.Example})
		}
		ui.Table(os.Stdout, rows, 2)
		fmt.Println(T("策略") + " (--strategy):")
		for _, p := range builtinPolicies {
			fmt.Printf("  %-12s %s\n", p.Name, p.Desc)
		}
		fmt.Printf("  %-12s %s\n", "PROXY", T("当前 group use 的分组"))
		fmt.Printf("  %-12s %s\n", "<"+T("分组名")+">", T("模糊匹配代理分组, 如")+" '节点选择')")
	case ruleTypeDoc(typ) == nil:
		fmt.Printf("%s %q\n", T("未知规则类型"), typ)
	case cond == "":
		td := ruleTypeDoc(typ)
		fmt.Printf("\n%s\n  %s: %s\n  %s: %s\n", typ, T("说明"), td.Desc, T("示例"), "--condition \""+td.Example+"\"")
		if td.IPRelated {
			fmt.Printf("  --no-resolve: %s\n", T("跳过域名解析 (仅 IP 类规则)"))
		}
	case strat == "":
		td := ruleTypeDoc(typ)
		fmt.Printf("\n%s: %s %q %s\n", T("预览"), td.Desc, cond, T("的流量")+" — "+T("未指定策略 (--strategy)"))
	default:
		s := mustSettings()
		resolved, err := resolveStrategy(s, strat)
		if err != nil {
			fmt.Printf("\n%s\n", err)
			return
		}
		td := ruleTypeDoc(typ)
		fmt.Printf("\n%s: %s %s %s: %s\n", T("预览"), td.Desc, condText(td, cond), T("的流量"), strategyDesc(resolved))
		fmt.Println(T("语法将由内核校验, 非法规则会被拒绝并回滚"))
		_ = noRes
	}
}

// ---- enable / disable / rm ----

func ruleToggle(idxArg string, enable bool) error {
	s := mustSettings()
	n, err := strconv.Atoi(idxArg)
	if err != nil || n < 1 || n > len(s.UserRules) {
		return fmt.Errorf("%s: %q (1-%d)", T("无效编号"), idxArg, len(s.UserRules))
	}
	s.UserRules[n-1].Enabled = enable
	word := T("已停用")
	if enable {
		word = T("已启用")
	}
	fmt.Printf("#%d %s\n", n, word)
	return applyRules(s)
}

var ruleEnableCmd = &cobra.Command{
	Use:   "enable <#id>",
	Short: T("启用用户规则"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return ruleToggle(args[0], true) },
}

var ruleDisableCmd = &cobra.Command{
	Use:   "disable <#id>",
	Short: T("禁用用户规则"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return ruleToggle(args[0], false) },
}

var ruleRmCmd = &cobra.Command{
	Use:   "rm <#id|关键词>",
	Short: T("删除规则"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		idx := -1
		if n, err := strconv.Atoi(args[0]); err == nil && n >= 1 && n <= len(s.UserRules) {
			idx = n - 1
		} else {
			for i, r := range s.UserRules {
				if strings.Contains(r.String(), args[0]) {
					if idx >= 0 {
						return fmt.Errorf("%q %s", args[0], T("匹配到多个, 请更精确"))
					}
					idx = i
				}
			}
		}
		if idx < 0 {
			return fmt.Errorf("%s", T("规则不存在"))
		}
		removed := s.UserRules[idx].String()
		s.UserRules = append(s.UserRules[:idx], s.UserRules[idx+1:]...)
		fmt.Printf("%s: %s\n", T("已删除"), removed)
		return applyRules(s)
	},
}

func init() {
	f := ruleListCmd.Flags()
	f.BoolVar(&ruleWithSub, "sub", false, T("同时显示订阅规则"))
	f.StringVar(&filterType, "type", "", T("按规则类型过滤"))
	f.StringVar(&filterCond, "condition", "", T("按匹配条件过滤"))
	f.StringVar(&filterStrat, "strategy", "", T("按代理策略过滤"))
	f.BoolVar(&filterNoRes, "no-resolve", false, T("仅显示带 no-resolve 的规则"))
	rf := ruleCmd.Flags()
	rf.AddFlagSet(f)

	af := ruleAddCmd.Flags()
	af.StringVar(&addType, "type", "", T("规则类型"))
	af.StringVar(&addCond, "condition", "", T("匹配条件"))
	af.StringVar(&addStrategy, "strategy", "", T("代理策略"))
	af.BoolVar(&addNoResolve, "no-resolve", false, T("跳过域名解析 (仅 IP 类规则)"))
	ruleAddCmd.SetHelpFunc(addHelp)
	// 补全
	af.Lookup("type").Annotations = nil
	ruleAddCmd.RegisterFlagCompletionFunc("type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var out []string
		for _, rt := range ruleTypes {
			if strings.HasPrefix(rt.Name, toComplete) {
				out = append(out, rt.Name+"\t"+rt.Desc)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
	ruleAddCmd.RegisterFlagCompletionFunc("strategy", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		s := mustSettings()
		var out []string
		for _, p := range builtinPolicies {
			out = append(out, p.Name+"\t"+p.Desc)
		}
		if ps, err := api.New(s).Proxies(); err == nil {
			var groups []string
			for n, p := range ps.Proxies {
				if groupType(p) {
					groups = append(groups, n)
				}
			}
			sort.Strings(groups)
			for _, g := range groups {
				out = append(out, g)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})

	ruleCmd.AddCommand(ruleListCmd, ruleAddCmd, ruleRmCmd, ruleEnableCmd, ruleDisableCmd)
	rootCmd.AddCommand(ruleCmd)
}
