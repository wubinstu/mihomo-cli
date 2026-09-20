package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

var subAddName string

var subCmd = &cobra.Command{
	Use:   "sub",
	Short: T("订阅管理: add/rm/list/update/use"),
}

var subAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: T("添加订阅"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if err := subs.Add(s, subAddName, args[0]); err != nil {
			return err
		}
		fmt.Printf("%s [%s] %s\n", T("订阅"), subs.Sanitize(subAddName), T("已添加并生效"))
		return render.Generate(s)
	},
}

var subRmCmd = &cobra.Command{
	Use:   "rm <name|#id>",
	Short: T("删除订阅"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		name, err := resolveSubArg(s, args[0])
		if err != nil {
			return err
		}
		if err := subs.Remove(s, name); err != nil {
			return err
		}
		if s.Current() != nil {
			if err := render.Generate(s); err != nil {
				return err
			}
			reloadIfActive(s)
		}
		fmt.Println(T("已删除"))
		return nil
	},
}

var subListCmd = &cobra.Command{
	Use:   "list",
	Short: T("列出全部订阅"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		rows := [][]string{{"#", "*", T("名称"), T("节点数"), T("更新时间"), "QUOTA", "URL"}}
		for i := range s.Profiles {
			p := &s.Profiles[i]
			cur := ""
			if p.Name == s.CurrentProfile {
				cur = "*"
			}
			rows = append(rows, []string{
				strconv.Itoa(i + 1), cur, p.Name, strconv.Itoa(p.Nodes),
				humanTime(p.UpdatedAt),
				shorten(p.UserInfo, 32), shorten(p.URL, 40),
			})
		}
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

var subUpdateCmd = &cobra.Command{
	Use:   "update [name|#id|all]",
	Short: T("更新订阅 (默认当前; all = 全部), 完成后热重载"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		name := ""
		if len(args) > 0 {
			name = args[0]
			if name != "all" {
				n, err := resolveSubArg(s, name)
				if err != nil {
					return err
				}
				name = n
			}
		}
		if err := subs.Update(s, name); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		reloadIfActive(s)
		return nil
	},
}

var subUseCmd = &cobra.Command{
	Use:   "use <name|#id>",
	Short: T("切换当前生效的订阅"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		name, err := resolveSubArg(s, args[0])
		if err != nil {
			return err
		}
		p := s.FindProfile(name)
		s.CurrentProfile = p.Name
		s.CurrentGroup = "" // 分组上下文随订阅失效
		if err := s.Save(); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		reloadIfActive(s)
		fmt.Printf("%s [%s]\n", T("当前订阅已切换为"), p.Name)
		return nil
	},
}

// resolveSubArg 订阅参数解析: 索引(1..n) | 名称
func resolveSubArg(s *app.Settings, arg string) (string, error) {
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(s.Profiles) {
		return s.Profiles[n-1].Name, nil
	}
	if s.FindProfile(subs.Sanitize(arg)) != nil {
		return subs.Sanitize(arg), nil
	}
	return "", fmt.Errorf("%s %q%s", T("订阅"), arg, T("不存在 (mihomo-cli sub list 查看)"))
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func init() {
	subAddCmd.Flags().StringVarP(&subAddName, "name", "n", "default", T("订阅名称"))
	subCmd.AddCommand(subAddCmd, subRmCmd, subListCmd, subUpdateCmd, subUseCmd)
	rootCmd.AddCommand(subCmd)
}
