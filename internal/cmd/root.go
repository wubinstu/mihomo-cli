package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

// T 输出文案(跟随 lang 设置)
func T(key string) string { return i18n.T(key) }

var rootCmd = &cobra.Command{
	Use:   "mihomo-cli",
	Short: T("mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)"),
	Long: `mihomo-cli - ` + T("面向 Linux 服务器的 Clash/mihomo 代理管理工具") + `

mihomo-cli install          ` + T("安装内核 + 注册服务") + `
mihomo-cli init             ` + T("添加订阅") + `
mihomo-cli start            ` + T("启动代理服务") + `
eval $(mihomo-cli proxy on) ` + T("当前 shell 开启代理"),
	SilenceUsage: true,
}

func Execute() {
	// 所有子命令禁用文件路径补全 (避免补全末端落到文件名)
	disableFileComp(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func disableFileComp(c *cobra.Command) {
	for _, sub := range c.Commands() {
		if sub.ValidArgsFunction == nil {
			sub.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
		disableFileComp(sub)
	}
}

// fail 打印错误并退出
func fail(err error) {
	fmt.Fprintln(os.Stderr, T("错误")+":", err)
	os.Exit(1)
}
