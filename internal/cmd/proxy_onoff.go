package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

// proxyOnCmd 开启代理: 确保服务运行, 输出可 eval 的环境变量
// 推荐 alias: alias proxy_on='eval $(mihomo-cli proxy on)'
var proxyOnCmd = &cobra.Command{
	Use:   "on",
	Short: T("开启代理: 启动服务并输出代理环境变量 (eval $(mihomo-cli proxy on))"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if !sysd.IsActive() {
			if s.Current() == nil {
				return fmt.Errorf("%s", T("没有订阅, 请先 mihomo-cli init"))
			}
			if err := render.Generate(s); err != nil {
				return err
			}
			if err := sysd.Service("start"); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, T("服务未运行, 已自动启动"))
		}
		addr := fmt.Sprintf("127.0.0.1:%d", s.ProxyPort())
		if s.AllowLan {
			addr = fmt.Sprintf("%s:%d", lanIP(), s.ProxyPort())
		}
		fmt.Printf(`export http_proxy=http://%s
export https_proxy=http://%s
export all_proxy=socks5://%s
export HTTP_PROXY=http://%s
export HTTPS_PROXY=http://%s
export ALL_PROXY=socks5://%s
export no_proxy=localhost,127.0.0.1,::1
export NO_PROXY=localhost,127.0.0.1,::1
`, addr, addr, addr, addr, addr, addr)
		return nil
	},
}

// proxyOffCmd 关闭当前 shell 的代理环境变量 (服务保持运行)
// 推荐 alias: alias proxy_off='eval $(mihomo-cli proxy off)'
var proxyOffCmd = &cobra.Command{
	Use:   "off",
	Short: T("关闭当前 shell 的代理环境变量 (服务保持运行)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(`unset http_proxy https_proxy all_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY no_proxy NO_PROXY`)
		return nil
	},
}

func init() {
	var proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: T("开/关当前 shell 代理(配合 alias)"),
	}
	proxyCmd.AddCommand(proxyOnCmd, proxyOffCmd)
	rootCmd.AddCommand(proxyCmd)
}
