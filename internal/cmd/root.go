package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mihomo-cli",
	Short: "mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)",
	Long: `mihomo-cli - 面向 Linux 服务器的 Clash/mihomo 代理管理工具

内核(mihomo)存放于 ~/.config/mihomo-cli/bin/, 不与 PATH 冲突;
服务以 systemd 托管, 控制走 external-controller API。

快速上手:
  mihomo-cli install          安装内核 + 注册服务
  mihomo-cli init             添加订阅
  mihomo-cli start            启动代理
  eval $(mihomo-cli env)      当前 shell 开启代理`,
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// fail 打印错误并退出
func fail(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
