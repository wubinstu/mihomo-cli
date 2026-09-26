package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

// T 输出文案(跟随 lang 设置)
func T(key string) string { return i18n.T(key) }

var rootCmd = &cobra.Command{
	Use:   "mihomo-cli",
	Short: T("mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)"),
	Long: T("面向 Linux 服务器的 Clash/mihomo 代理管理工具") + `

mihomo-cli install --core auto --resource all --systemd --completion bash   ` + T("全新安装") + `
mihomo-cli sub add <name> <订阅URL>                                          ` + T("添加订阅") + `
mihomo-cli start                                                            ` + T("启动代理服务") + `
eval $(mihomo-cli proxy on)                                                 ` + T("当前 shell 开启代理") + `
mihomo-cli config get                                                       ` + T("查看/修改全部配置") + `

` + T("三层结构: 订阅 sub → 分组 group → 节点 node; 裸命令等于各自的 list。"),
	SilenceUsage: true,
}

// mutatingCmds 会改变系统状态的命令 (需要 root); 只读命令任何人可用
var mutatingCmds = map[*cobra.Command]bool{}

// markMutating 登记为"需要 root"
func markMutating(c *cobra.Command) { mutatingCmds[c] = true }

// requireRoot 非 root 执行变更类命令时自动提权:
//   - stdin 是 TTY: exec sudo 原样重跑 (体验 = 输一次密码)
//   - 非 TTY (脚本/管道/ssh 非交互): 立即报错并给出确切命令, 不悬挂等待密码
func requireRoot(cmd *cobra.Command, args []string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("%s\n  sudo mihomo-cli %s", T("需要 root 权限, 且本机没有 sudo"), cmd.CommandPath()[len("mihomo-cli"):])
	}
	if !isTTY() {
		return fmt.Errorf("%s\n  sudo mihomo-cli %s", T("需要 root 权限, 请执行"), cmd.CommandPath()[len("mihomo-cli"):])
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return err
	}
	argv := append([]string{"sudo"}, os.Args...)
	return syscall.Exec(sudo, argv, os.Environ())
}

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// currentUser 当前用户名 (日志/提示用)
func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	if n := os.Getenv("USER"); n != "" {
		return n
	}
	return "unknown"
}

func Execute() {
	// 所有子命令禁用文件路径补全 (mihomo-cli 不与文件打交道; root 自身也要挂)
	disableFileComp(rootCmd)
	// cobra 内置命令(help/completion)文案本地化 (需先触发默认命令创建)
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	localizeBuiltins(rootCmd)
	// 变更类命令非 root 时自动提权
	pre := rootCmd.PersistentPreRunE
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if pre != nil {
			if err := pre(cmd, args); err != nil {
				return err
			}
		}
		if mutatingCmds[cmd] {
			return requireRoot(cmd, args)
		}
		return nil
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func localizeBuiltins(c *cobra.Command) {
	for _, sub := range c.Commands() {
		switch sub.Name() {
		case "help":
			sub.Short = T("任意命令的帮助信息")
			sub.Long = T("显示任意命令的帮助信息; 用法: mihomo-cli help [command]")
		case "completion":
			sub.Short = T("生成指定 shell 的自动补全脚本")
			sub.Long = T("为指定的 shell 生成自动补全脚本 (bash/zsh/fish/powershell)。")
			for _, sh := range sub.Commands() {
				sh.Short = T("生成") + " " + sh.Name() + " " + T("补全脚本")
			}
		}
		localizeBuiltins(sub)
	}
}

// disableFileComp 全命令树(含 root 自身)禁止文件路径补全。
// 注意: cobra 的补全脚本在"空命令行"时会调用 __complete 且不带参数, 那样会落到
// ShellCompDirectiveDefault 从而触发文件名补全 —— 那个问题在 main.go 里补 shim。
func disableFileComp(c *cobra.Command) {
	if c.ValidArgsFunction == nil {
		c.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
	for _, sub := range c.Commands() {
		disableFileComp(sub)
	}
}

// fail 打印错误并退出
func fail(err error) {
	fmt.Fprintln(os.Stderr, T("错误")+":", err)
	os.Exit(1)
}
