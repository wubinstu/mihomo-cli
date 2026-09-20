package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

var subAddName string

var subCmd = &cobra.Command{
	Use:   "sub",
	Short: "订阅管理: add/rm/list/update/use",
}

var subAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "添加订阅",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if err := subs.Add(s, subAddName, args[0]); err != nil {
			return err
		}
		fmt.Printf("订阅 [%s] 已添加并生效\n", subs.Sanitize(subAddName))
		return render.Generate(s)
	},
}

var subRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "删除订阅",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if err := subs.Remove(s, args[0]); err != nil {
			return err
		}
		if s.Current() != nil {
			if err := render.Generate(s); err != nil {
				return err
			}
			reloadIfActive(s)
		}
		fmt.Println("已删除")
		return nil
	},
}

var subListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部订阅",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		rows := [][]string{{"*", "NAME", "NODES", "UPDATED", "QUOTA", "URL"}}
		for i := range s.Profiles {
			p := &s.Profiles[i]
			cur := ""
			if p.Name == s.CurrentProfile {
				cur = "*"
			}
			rows = append(rows, []string{
				cur, p.Name, fmt.Sprintf("%d", p.Nodes),
				humanTime(p.UpdatedAt),
				shorten(p.UserInfo, 32), shorten(p.URL, 40),
			})
		}
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

var subUpdateCmd = &cobra.Command{
	Use:   "update [name|all]",
	Short: "更新订阅 (默认当前; all = 全部), 完成后热重载",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		name := ""
		if len(args) > 0 {
			name = args[0]
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
	Use:   "use <name>",
	Short: "切换当前生效的订阅",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		p := s.FindProfile(subs.Sanitize(args[0]))
		if p == nil {
			return fmt.Errorf("订阅 %q 不存在 (mihomo-cli sub list 查看)", args[0])
		}
		s.CurrentProfile = p.Name
		if err := s.Save(); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		reloadIfActive(s)
		fmt.Printf("当前订阅已切换为 [%s]\n", p.Name)
		return nil
	},
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
	subAddCmd.Flags().StringVarP(&subAddName, "name", "n", "default", "订阅名称")
	subCmd.AddCommand(subAddCmd, subRmCmd, subListCmd, subUpdateCmd, subUseCmd)
	rootCmd.AddCommand(subCmd)
}
