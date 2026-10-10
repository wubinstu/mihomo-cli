package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wubinstu/mihomo-cli/internal/ui"

	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

// T 输出文案(跟随 lang 设置)
func T(key string) string { return i18n.T(key) }

var rootCmd = &cobra.Command{
	Use:   "mihomo-cli",
	Short: T("mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)"),
	// 示例行整行作为一个 T() key: 连 <订阅URL> 这种占位符一起翻译,
	// 否则英文模式下会留下"中英混排"的一行 (v1.4.2 用户反馈)。
	// 两列各自整段翻译 (命令列连 <订阅URL> 一起), 再用 Align2 对齐 ——
	// 早期把整行当一个 key, 对齐只能靠手敲空格, 英文模式下必然歪。
	Long: T("面向 Linux 服务器的 Clash/mihomo 代理管理工具") + "\n\n" +
		ui.ExampleLines([][2]string{
			{T("mihomo-cli install --core auto --resource all --systemd --completion bash"), T("全新安装")},
			{T("mihomo-cli sub add <name> <订阅URL>"), T("添加订阅")},
			{T("mihomo-cli start"), T("启动代理服务")},
			{T("eval $(mihomo-cli proxy on)"), T("当前 shell 开启代理")},
			{T("mihomo-cli config get"), T("查看/修改全部配置")},
		}, 2) + "\n" +
		T("三层结构: 订阅 sub → 分组 group → 节点 node; 裸命令等于各自的 list。"),
	SilenceUsage:  true,
	SilenceErrors: true,
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

func Execute() {
	// 语言到这才最终定下来 (读 config.toml / $LANG), 随后把整棵命令树按它重翻一遍 ——
	// 包初始化时 T() 出来的还是中文原文, 见 i18n.init 的说明。
	i18n.Reset()
	applyLanguage(rootCmd)
	// 所有子命令禁用文件路径补全 (mihomo-cli 不与文件打交道; root 自身也要挂)
	disableFileComp(rootCmd)
	// cobra 的 ExecuteC 会无条件调用 InitDefaultCompletionCmd(); 唯一的抑制办法是
	// 抢先注册一个同名命令 (它见到已有 completion 就直接返回)。
	// `mihomo-cli completion` 和 `mihomo-cli install --completion` 功能完全重合
	// (都是往系统目录写补全脚本), 留一个入口就够 (v1.5.0 用户反馈)。
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.CompletionOptions.DisableDescriptions = true
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.InitDefaultHelpCmd()
	localizeBuiltins(rootCmd)
	// cobra 写死的帮助骨架 ("Usage:"/"Available Commands:"/"Flags:"/"help for x") 本地化。
	// 放在内置命令创建之后: 它们也是命令树的一部分, 漏掉就还是英文。
	localizeCobra(rootCmd)
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
		// 全命令树都设了 SilenceErrors, 由这里统一按当前语言打印
		fmt.Fprintln(os.Stderr, T("错误")+": "+err.Error())
		fmt.Fprintln(os.Stderr, T("用法")+": "+T("mihomo-cli <命令> --help"))
		os.Exit(1)
	}
}

// cmdText 一条命令的全部可翻译文案 (中文原文快照)
type cmdText struct {
	short, long, example string
	flags                map[*pflag.Flag]string
}

// cmdRaw 命令树文案的中文原文快照 (第一次 applyLanguage 时留下)。
// 包初始化时语言固定是中文 (见 i18n.init), 所以那一刻字段里就是原文, 之后可以无损重翻。
var cmdRaw = map[*cobra.Command]*cmdText{}

// applyLanguage 按当前语言重新翻译整棵命令树: Short/Long/Example + 每个标志的说明。
// Long 常是多个 key 拼的, 走 TranslateComposite (按片段替换)。
func applyLanguage(c *cobra.Command) {
	for _, sub := range allCommands(c) {
		raw, ok := cmdRaw[sub]
		if !ok {
			raw = &cmdText{short: sub.Short, long: sub.Long, example: sub.Example, flags: map[*pflag.Flag]string{}}
			sub.Flags().VisitAll(func(f *pflag.Flag) { raw.flags[f] = f.Usage })
			sub.PersistentFlags().VisitAll(func(f *pflag.Flag) { raw.flags[f] = f.Usage })
			cmdRaw[sub] = raw
		}
		// Short 也走 composite: 有些是 "T(前缀) + 命令名" 拼的 (旧名别名)
		sub.Short = i18n.TranslateComposite(raw.short)
		sub.Long = ui.RealignExamples(i18n.TranslateComposite(raw.long))
		if raw.example != "" {
			sub.Example = i18n.TranslateComposite(raw.example)
		}
		for f, usage := range raw.flags {
			f.Usage = i18n.TranslateComposite(usage)
		}
	}
}

// localizeCobra 把 cobra 写死的帮助骨架翻过来: 用法模板 + -h 标志的说明 + 错误输出。
// cobra 的模板是英文硬编码的, 只能整块替换; -h 的标志说明则由 InitDefaultHelpFlag
// 生成 "help for <cmd>", 我们抢先注册同名标志, cobra 见到已存在就不再覆盖。
func localizeCobra(c *cobra.Command) {
	c.SetUsageTemplate(usageTemplate())
	for _, sub := range allCommands(c) {
		sub.SilenceErrors = true
		sub.SilenceUsage = true
		// 抢先注册同名标志: cobra 的 InitDefaultHelpFlag 见到已存在就不会覆盖成
		// "help for <cmd>"。已经注册过的 (重复调用/测试) 也要刷新文案。
		if f := sub.Flags().Lookup("help"); f != nil {
			f.Usage = T("显示帮助")
		} else {
			sub.Flags().BoolP("help", "h", false, T("显示帮助"))
		}
	}
}

// allCommands 整棵命令树 (含 root)
func allCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, sub := range c.Commands() {
		out = append(out, allCommands(sub)...)
	}
	return out
}

// usageTemplate cobra 用法模板的本地化版本 (字段名与 cobra 默认模板一致)
func usageTemplate() string {
	return T("用法:") + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

` + T("别名:") + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + T("示例:") + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + T("可用命令:") + `{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + T("其它命令:") + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + T("标志:") + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + T("全局标志:") + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + T("其它帮助主题:") + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

` + T("查看某命令的详细帮助: {{.CommandPath}} [command] --help") + `{{end}}
`
}

func localizeBuiltins(c *cobra.Command) {
	for _, sub := range c.Commands() {
		switch sub.Name() {
		case "help":
			sub.Short = T("任意命令的帮助信息")
			sub.Long = T("显示任意命令的帮助信息; 用法: mihomo-cli help [command]")
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
