package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// ruleValidMin 最小合法性: 至少两段 (类型,值[,目标...])
func ruleValid(r string) bool {
	parts := strings.Split(strings.TrimSpace(r), ",")
	return len(parts) >= 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

var ruleWithSub bool

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

// applyRules 保存 + 重新渲染 + 热重载
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
	reloadIfActive(s)
	if m, err := apiMode(); err == nil && m != "rule" {
		fmt.Fprintf(os.Stderr, "\x1b[33m%s\x1b[0m\n",
			T("注意: 当前代理模式不是 rule, 规则暂不生效 (set proxy-mode rule)"))
	}
	return nil
}

// apiMode 读取内核当前代理模式
func apiMode() (string, error) {
	s := mustSettings()
	return api.New(s).ConfigMode()
}

var ruleCmd = &cobra.Command{
	Use:   "rule",
	Short: T("用户自定义规则 (独立于订阅, 优先匹配; list --sub 同时显示订阅规则)"),
	Long: T("用户规则独立于订阅保存, 不会因订阅更新/删除/更换而丢失; 匹配优先级高于订阅规则。") + `

mihomo-cli rule                       # ` + T("显示用户规则") + `
mihomo-cli rule list --sub            # ` + T("同时显示订阅规则") + `
mihomo-cli rule add <rule>            # ` + T("添加规则") + `, ` + T("如") + ` DOMAIN-SUFFIX,example.com,DIRECT
mihomo-cli rule rm <#id|keyword>        # ` + T("删除规则") + `
mihomo-cli rule test <host>        # ` + T("查询匹配结果 (仅参考, 实际以内核为准)") + `
` + T("规则格式: TYPE,VALUE[,TARGET]  TARGET 可为 分组名/节点名/DIRECT/REJECT") + `
TYPE: DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD / IP-CIDR / GEOIP / PROCESS-NAME ...`,
	RunE: ruleListRun,
}

var ruleListCmd = &cobra.Command{
	Use:   "list",
	Short: T("显示用户规则 (--sub 同时显示订阅规则)"),
	RunE:  ruleListRun,
}

func ruleListRun(cmd *cobra.Command, args []string) error {
	s := mustSettings()
	rows := [][]string{{"*", "#", T("规则")}}
	for i, r := range s.UserRules {
		rows = append(rows, []string{"*", strconv.Itoa(i + 1), r})
	}
	fmt.Printf("%s: %d\n", T("用户规则"), len(s.UserRules))
	if len(s.UserRules) > 0 {
		ui.Table(os.Stdout, rows, 2)
	}
	if ruleWithSub {
		sr := subRules(s)
		fmt.Printf("%s: %d\n", T("订阅规则"), len(sr))
		if len(sr) > 0 {
			rows2 := [][]string{{"*", "#", T("规则")}}
			for i, r := range sr {
				rows2 = append(rows2, []string{"", strconv.Itoa(i + 1), r})
			}
			ui.Table(os.Stdout, rows2, 2)
		}
	} else {
		fmt.Println(T("提示: rule list --sub 查看订阅规则"))
	}
	return nil
}

var ruleAddCmd = &cobra.Command{
	Use:   "add <rule>",
	Short: T("添加规则") + ", " + T("如") + " DOMAIN-SUFFIX,example.com,DIRECT",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := strings.Join(args, " ")
		if !ruleValid(r) {
			return fmt.Errorf("%s: %q (%s)", T("无效规则"), r, "TYPE,VALUE[,TARGET]")
		}
		s := mustSettings()
		s.UserRules = append(s.UserRules, r)
		fmt.Printf("#%d %s\n", len(s.UserRules), r)
		return applyRules(s)
	},
}

var ruleRmCmd = &cobra.Command{
	Use:   "rm <#id|keyword>",
	Short: T("删除规则"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		idx := -1
		if n, err := strconv.Atoi(args[0]); err == nil && n >= 1 && n <= len(s.UserRules) {
			idx = n - 1
		} else {
			for i, r := range s.UserRules {
				if strings.Contains(r, args[0]) {
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
		removed := s.UserRules[idx]
		s.UserRules = append(s.UserRules[:idx], s.UserRules[idx+1:]...)
		fmt.Printf("%s: %s\n", T("已删除"), removed)
		return applyRules(s)
	},
}

func init() {
	ruleListCmd.Flags().BoolVar(&ruleWithSub, "sub", false, T("同时显示订阅规则"))
	ruleCmd.Flags().BoolVar(&ruleWithSub, "sub", false, T("同时显示订阅规则"))
	ruleCmd.AddCommand(ruleListCmd, ruleAddCmd, ruleRmCmd)
	rootCmd.AddCommand(ruleCmd)
}
