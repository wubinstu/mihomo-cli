package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

// reloadIfActive 服务运行中则热重载配置, 否则提示
func reloadIfActive(s *app.Settings) {
	if sysd.IsActive() {
		if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v %s\n", T("警告: 热重载失败"), err, T("(可执行 mihomo-cli restart)"))
		} else {
			fmt.Println(T("已热重载配置"))
		}
	} else {
		fmt.Println(T("服务未运行, 已跳过热重载 (mihomo-cli start)"))
	}
}

var (
	installProxy       string
	installMirrorFlag  string
	installCore        string
	installResource    string
	installSystemd     bool
	installCompletionS string
)

// installCmd 安装器: 只负责下载/写入 (幂等); 订阅用 sub add, 参数用 config set
var installCmd = &cobra.Command{
	Use:   "install",
	Short: T("安装: --core / --resource / --systemd / --completion (可组合; 无参数显示帮助)"),
	Long: T("安装动作幂等: 文件不存在则下载创建, 存在则更新刷新。订阅请用 sub add, 参数请用 config set。") + `

mihomo-cli install --core latest --resource all --systemd --completion bash   # ` + T("全新安装") + `
mihomo-cli install --core compatible    # ` + T("安装旧 CPU 兼容内核") + `
mihomo-cli install --resource mmdb      # ` + T("安装/更新单个资源 (mmdb/asn/geoip/geosite/all)") + `
mihomo-cli install --systemd            # ` + T("按 config.toml 重新生成全部 systemd 单元并 daemon-reload") + `
mihomo-cli install --completion bash    # ` + T("安装/更新 shell 补全 (bash/zsh/fish, 空=交互)") + `
mihomo-cli install [--proxy URL] [--mirror URL]   # ` + T("一次性下载代理/镜像 (默认失败时自动尝试内置镜像)") + `

` + T("结束时若内核与 systemd 就绪而服务未运行, 将以最小配置自动拉起服务。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli install)", T("安装需要 root 权限: 配置目录 /etc/mihomo-cli 与 systemd 单元"))
		}
		noFlags := installCore == "" && installResource == "" && !installSystemd && installCompletionS == ""
		if noFlags {
			return cmd.Help()
		}
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if err := app.EnsureDirs(); err != nil {
			return err
		}
		if err := s.Save(); err != nil { // 无 config.toml 时按默认值创建
			return err
		}
		mirror := installMirrorFlag
		if mirror == "" {
			mirror = s.GithubMirror
		}
		any := false

		// --core latest|compatible
		if installCore != "" {
			compatible := installCore == "compatible"
			if !compatible && installCore != "latest" {
				return fmt.Errorf("--core: latest|compatible")
			}
			if _, err := os.Stat(app.CoreBin); os.IsNotExist(err) {
				rel, err := core.Latest(installProxy)
				if err != nil {
					return err
				}
				fmt.Printf("%s: %s (arch=%s)\n", T("最新内核版本"), rel.TagName, core.ArchName())
				if err := core.DownloadInstall(rel.TagName, installProxy, mirror, compatible); err != nil {
					return err
				}
			} else {
				if compatible {
					rel, err := core.Latest(installProxy)
					if err != nil {
						return err
					}
					if err := core.DownloadInstall(rel.TagName, installProxy, mirror, true); err != nil {
						return err
					}
				} else {
					fmt.Println(T("内核已安装") + " (" + T("如需更新") + ": mihomo-cli resource core upgrade)")
				}
			}
			any = true
		}

		// --resource <name|all>
		if installResource != "" {
			names := []string{installResource}
			if installResource == "all" {
				names = []string{"mmdb", "geosite", "asn", "geoip"}
			}
			for _, n := range names {
				if err := core.UpdateResource(n, installProxy, mirror); err != nil {
					return err
				}
			}
			any = true
		}

		// --systemd: 重新生成全部单元
		if installSystemd {
			fmt.Println(T("注册 systemd 服务") + " ...")
			if err := sysd.InstallService(); err != nil {
				return err
			}
			if err := sysd.InstallTimers(s); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 定时任务安装失败"), err)
			}
			any = true
		}

		// --completion <shell|空=交互>
		if cmd.Flags().Changed("completion") {
			installCompletions(installCompletionS)
			any = true
		}

		_ = any
		// 其他用户权限与属主一致 (任何用户可读写配置; 安全敏感环境请自行收紧)
		_ = exec.Command("chmod", "-R", "o=rwX", app.BaseDir).Run()
		// 结束检查: 内核+systemd 就绪则确保服务运行 (空配置即全 DIRECT)
		if _, err := os.Stat(app.CoreBin); err == nil {
			if _, err := os.Stat("/etc/systemd/system/mihomo-cli.service"); err == nil {
				if err := render.Generate(s); err == nil && !sysd.IsActive() {
					if err := sysd.Service("start"); err == nil {
						fmt.Println(T("服务已自动拉起") + " (" + T("无订阅时为最小配置, 全部流量 DIRECT") + ")")
					}
				}
			}
		}
		return nil
	},
}

var uninstallPurge bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: T("卸载服务与单元文件 (--purge 同时删除配置/订阅/内核)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli uninstall)", T("安装需要 root 权限: 配置目录 /etc/mihomo-cli 与 systemd 单元"))
		}
		fmt.Println(T("停止并移除 systemd 单元") + " ...")
		sysd.RemoveAll()
		removeCompletions()
		for _, bin := range []string{"/usr/bin/mihomo-cli", "/usr/local/bin/mihomo-cli"} {
			_ = exec.Command("rm", "-f", bin).Run()
		}
		if uninstallPurge {
			fmt.Printf("%s %s ...\n", T("删除"), app.BaseDir)
			if err := os.RemoveAll(app.BaseDir); err != nil {
				return err
			}
		} else {
			fmt.Printf("%s %s %s\n", T("已保留数据目录"), app.BaseDir, T("(使用 --purge 彻底删除)"))
		}
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: T("前台运行内核(调试模式, Ctrl-C 退出)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if s.Current() == nil {
			return fmt.Errorf("%s", T("没有订阅, 请先 mihomo-cli sub use <id|名称>"))
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		fmt.Println(T("前台启动内核, 日志输出到终端") + " ...")
		return syscall.Exec(app.CoreBin, []string{app.CoreBin, "-d", app.RuntimeDir}, os.Environ())
	},
}

func readLine() (string, error) {
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

// installCompletions 安装 shell 补全到全局目录 (系统级, 全用户可用)
func installCompletions(shell string) {
	type target struct {
		shell string
		dir   string
		file  string
		gen   func(w io.Writer) error
	}
	targets := []target{
		{"bash", "/usr/share/bash-completion/completions", "mihomo-cli", func(w io.Writer) error { return rootCmd.GenBashCompletionV2(w, false) }},
		{"zsh", "/usr/share/zsh/site-functions", "_mihomo-cli", func(w io.Writer) error { return rootCmd.GenZshCompletion(w) }},
		{"fish", "/usr/share/fish/completions", "mihomo-cli.fish", func(w io.Writer) error { return rootCmd.GenFishCompletion(w, false) }},
	}
	var chosen []target
	if shell != "" {
		for _, t := range targets {
			if t.shell == shell {
				chosen = append(chosen, t)
			}
		}
		if len(chosen) == 0 {
			fmt.Printf("%s: %s\\n", T("未知 shell"), shell)
			return
		}
	} else {
		// 交互选择
		fmt.Println(T("请选择要安装补全的 shell:"))
		for j, t := range targets {
			fmt.Printf("  %d) %-5s (%s)\n", j+1, t.shell, t.dir)
		}
		fmt.Printf("%s [1-3]: ", T("输入编号"))
		line, _ := readLine()
		line = strings.TrimSpace(line)
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(targets) {
			fmt.Println(T("跳过补全安装"))
			return
		}
		chosen = append(chosen, targets[n-1])
	}
	for _, t := range chosen {
		if err := os.MkdirAll(t.dir, 0o755); err != nil {
			continue
		}
		var buf bytes.Buffer
		if t.gen(&buf) != nil {
			continue
		}
		c := exec.Command("tee", filepath.Join(t.dir, t.file))
		c.Stdin = &buf
		c.Stdout = nil
		c.Stderr = os.Stderr
		if c.Run() == nil {
			fmt.Printf("%s %s\n", T("补全已安装"), filepath.Join(t.dir, t.file))
		}
	}
}

// removeCompletions 清理补全脚本 (跟随 uninstall)
func removeCompletions() {
	for _, p := range []string{
		"/usr/share/bash-completion/completions/mihomo-cli",
		"/usr/share/zsh/site-functions/_mihomo-cli",
		"/usr/share/fish/completions/mihomo-cli.fish",
		"/usr/local/share/bash-completion/completions/mihomo-cli",
		"/usr/local/share/zsh/site-functions/_mihomo-cli",
		"/usr/local/share/fish/completions/mihomo-cli.fish",
	} {
		if _, err := os.Stat(p); err == nil {
			_ = exec.Command("rm", "-f", p).Run()
		}
	}
}

func init() {
	f := installCmd.Flags()
	f.StringVar(&installProxy, "proxy", "", T("下载内核使用的代理")+", "+T("如")+" http://192.168.1.1:7890")
	f.StringVar(&installMirrorFlag, "mirror", "", T("GitHub 镜像站前缀 (空=失败时自动尝试内置镜像)"))
	f.StringVar(&installCore, "core", "", "latest|compatible")
	f.StringVar(&installResource, "resource", "", "mmdb|asn|geoip|geosite|all")
	f.BoolVar(&installSystemd, "systemd", false, T("重新生成 systemd 单元"))
	f.StringVar(&installCompletionS, "completion", "", "bash|zsh|fish")
	installCmd.RegisterFlagCompletionFunc("core", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"latest", "compatible"}, cobra.ShellCompDirectiveNoFileComp
	})
	installCmd.RegisterFlagCompletionFunc("resource", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"mmdb", "asn", "geoip", "geosite", "all"}, cobra.ShellCompDirectiveNoFileComp
	})
	installCmd.RegisterFlagCompletionFunc("completion", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"bash", "zsh", "fish"}, cobra.ShellCompDirectiveNoFileComp
	})
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, T("同时删除配置/订阅/内核数据"))
	rootCmd.AddCommand(installCmd, uninstallCmd, runCmd)
}
