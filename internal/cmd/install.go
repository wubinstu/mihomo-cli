package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

// reloadIfActive 服务运行中则热重载配置, 否则提示
func reloadIfActive(s *app.Settings) {
	if !sysd.IsActive() {
		fmt.Println(T("服务未运行, 已跳过热重载 (mihomo-cli start)"))
		return
	}
	if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v %s\n", T("警告: 热重载失败"), err, T("(可执行 mihomo-cli restart)"))
		return
	}
	fmt.Println(T("已热重载配置"))
}

var (
	installProxy       string
	installMirrorFlag  string
	installCoreSpec    string
	installArch        string
	installResource    string
	installSystemd     bool
	installCompletionS string
)

// completionTargets 补全安装目标 (bash/zsh/fish → 系统级目录)
func completionTargets() []struct {
	shell string
	dir   string
	file  string
	gen   func(w *bytes.Buffer) error
} {
	return []struct {
		shell string
		dir   string
		file  string
		gen   func(w *bytes.Buffer) error
	}{
		{"bash", "/usr/share/bash-completion/completions", "mihomo-cli", func(w *bytes.Buffer) error { return rootCmd.GenBashCompletionV2(w, false) }},
		{"zsh", "/usr/share/zsh/site-functions", "_mihomo-cli", func(w *bytes.Buffer) error { return rootCmd.GenZshCompletion(w) }},
		{"fish", "/usr/share/fish/completions", "mihomo-cli.fish", func(w *bytes.Buffer) error { return rootCmd.GenFishCompletion(w, false) }},
	}
}

// completionPaths 全部可能的补全文件路径 (uninstall --completion 用)
func completionPaths(shell string) []string {
	var out []string
	for _, t := range completionTargets() {
		if shell != "" && t.shell != shell {
			continue
		}
		out = append(out, filepath.Join(t.dir, t.file),
			filepath.Join("/usr/local", strings.TrimPrefix(t.dir, "/usr"), t.file))
	}
	return out
}

// installCmd 安装器: 只负责下载/写入 (幂等); 订阅用 sub add, 参数用 config set
var installCmd = &cobra.Command{
	Use:   "install",
	Short: T("安装: --core / --resource / --systemd / --completion (可组合; 无参数显示帮助)"),
	Long: T("安装动作幂等: 文件不存在则下载创建, 存在则更新刷新。订阅请用 sub add, 参数请用 config set。") + `

mihomo-cli install --core auto --resource all --systemd --completion bash   # ` + T("全新安装") + `
mihomo-cli install --core compatible:v1.19.19   # ` + T("指定风味与版本") + `
mihomo-cli install --resource mmdb              # ` + T("安装/更新单个资源 (mmdb/asn/geoip/geosite/all)") + `
mihomo-cli install --systemd                    # ` + T("按 config.toml 重新生成全部 systemd 单元并 daemon-reload") + `
mihomo-cli install --completion bash            # ` + T("安装/更新 shell 补全 (bash/zsh/fish)") + `
mihomo-cli install [--proxy URL] [--mirror URL] # ` + T("一次性下载代理/镜像 (默认失败时自动尝试内置镜像)") + `

` + T("--core 的规格是「风味:版本」两个正交轴, 平台(系统/架构)自动探测:") + `
  auto                       ` + T("自动探测风味, 装最新版") + `
  v1.19.31                   ` + T("指定版本, 风味自动探测") + `
  compatible                 ` + T("旧 CPU/旧系统兼容构建, 版本取最新") + `
  compatible:v1.19.19        ` + T("风味与版本都指定") + `
  v2-go120                   ` + T("微架构档位+工具链组合, 版本取最新") + `

` + T("结束时若内核与 systemd 就绪而服务未运行, 将以最小配置自动拉起服务。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli install)", T("安装需要 root 权限: 配置目录 /etc/mihomo-cli 与 systemd 单元"))
		}
		if installCoreSpec == "" && installResource == "" && !installSystemd && installCompletionS == "" {
			return cmd.Help()
		}
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if err := app.EnsureDirs(); err != nil {
			return err
		}
		if err := s.Save(); err != nil { // 兼作配置格式迁移 (旧版 config.toml 会重排成新格式)
			return err
		}
		mirror := installMirrorFlag
		if mirror == "" {
			mirror = s.GithubMirror
		}

		// --core <风味>[:<版本>]
		if installCoreSpec != "" {
			flavor, version, err := core.ParseCoreSpec(installCoreSpec)
			if err != nil {
				return err
			}
			res, err := core.InstallSpec(s, flavor, version, installProxy, mirror, installArch)
			if err != nil {
				return err
			}
			fmt.Printf("%s %s [%s] %s\n", T("已安装内核"), res.Version,
				res.Platform+" / "+flavorText(res.Flavor), archText(res.Arch))
			if res.Degraded {
				fmt.Printf("%s\n", T("提示: 已自动降级到可在本机运行的内核构建"))
			}
			// 内核换了, TUN 能力要跟着判断
			if err := sysd.EnsureCapabilities(tunOn(s)); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 更新 systemd 单元失败"), err)
			}
		}

		// --resource <name|all>
		if installResource != "" {
			names := []string{installResource}
			if installResource == "all" {
				names = core.SortResources()
			}
			for _, n := range names {
				if err := core.UpdateResource(n, installProxy, mirror); err != nil {
					return err
				}
			}
			fmt.Println(T("资源已下载"))
		}

		// --systemd: 按 config.toml 重新生成全部单元
		if installSystemd {
			fmt.Println(T("注册 systemd 服务") + " ...")
			if err := sysd.InstallService(tunOn(s)); err != nil {
				return err
			}
			if err := sysd.InstallTimers(s); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 定时任务安装失败"), err)
			}
		}

		// --completion <shell>
		if cmd.Flags().Changed("completion") {
			if err := installCompletions(installCompletionS); err != nil {
				return err
			}
		}

		app.HardenPerms()
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

func flavorText(f string) string {
	if f == "" || f == "auto" {
		return T("官方默认")
	}
	return f
}

func archText(arch string) string {
	if arch == "" {
		arch = "auto"
	}
	return "arch=" + arch
}

// tunOn config.toml 里是否开了 TUN
func tunOn(s *app.Settings) bool {
	if k := cfgLookup("tun.enable"); k != nil {
		return k.Get(s) == "true"
	}
	return false
}

// ---- uninstall: 与 install 严格对称 ----

var (
	uninstallCore       bool
	uninstallResource   bool
	uninstallSystemd    bool
	uninstallCompletion string
	uninstallPurge      bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: T("卸载: --core / --resource / --systemd / --completion / --purge (无参数显示帮助)"),
	Long: T("卸载与 install 严格对称, 只删你指定的那一部分; --purge 才是彻底卸干净。") + `

mihomo-cli uninstall --core                    # ` + T("只卸载内核 (先停止服务), 配置与订阅保留") + `
mihomo-cli uninstall --resource                # ` + T("只删除 4 个 geo 资源文件") + `
mihomo-cli uninstall --systemd                 # ` + T("只关闭并删除全部 systemd service/timer") + `
mihomo-cli uninstall --completion bash         # ` + T("只删除指定 shell 补全 (不带值=三个全删)") + `
mihomo-cli uninstall --purge                   # ` + T("彻底卸载: 以上全部 + /etc/mihomo-cli + mihomo-cli 二进制自身") + `

` + T("相当于整套软件从未在这台机器上出现过。") + "\n" +
		T("不再使用的配置目录内容可先看 mihomo-cli config get。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli uninstall)", T("卸载需要 root 权限"))
		}
		if !uninstallCore && !uninstallResource && !uninstallSystemd &&
			!uninstallPurge && !cmd.Flags().Changed("completion") {
			return cmd.Help()
		}
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		wantCompletion := cmd.Flags().Changed("completion")

		if uninstallPurge {
			uninstallCore, uninstallResource, uninstallSystemd, wantCompletion = true, true, true, true
		}

		// systemd 先停服务 (卸载内核前必须先停, 否则 systemd 会拉起一个空二进制)
		if uninstallSystemd {
			fmt.Println(T("停止并移除 systemd 单元") + " ...")
			sysd.RemoveAll()
		} else if uninstallCore {
			if sysd.IsActive() {
				_ = sysd.Service("stop")
				fmt.Println(T("服务已停止"))
			}
		}

		if wantCompletion {
			shells := []string{}
			if uninstallCompletion != "" && !uninstallPurge {
				shells = []string{uninstallCompletion}
			}
			removed := 0
			for _, p := range completionPaths(shellOr(shells)) {
				if _, err := os.Stat(p); err == nil {
					if os.Remove(p) == nil {
						removed++
					}
				}
			}
			if removed > 0 {
				fmt.Printf("%s %d\n", T("已删除补全文件"), removed)
			}
		}

		if uninstallCore {
			removed := 0
			for _, p := range []string{app.CoreBin, app.CoreBinOld} {
				if _, err := os.Stat(p); err == nil && os.Remove(p) == nil {
					removed++
				}
			}
			if _, err := os.Stat(app.VersionsDir); err == nil {
				if os.RemoveAll(app.VersionsDir) == nil {
					removed++
				}
			}
			s.CoreHistory = nil
			s.CoreVersion = ""
			_ = s.Save()
			fmt.Printf("%s %d (%s)\n", T("内核已删除"), removed, app.BinDir)
			fmt.Println(T("服务已停止") + "; " + T("重新安装") + ": mihomo-cli install --core auto")
		}

		if uninstallResource {
			removed := 0
			for _, r := range core.Resources() {
				if core.RemoveResource(r.Name) {
					removed++
				}
			}
			fmt.Printf("%s %d\n", T("已删除资源文件"), removed)
		}

		if uninstallPurge {
			fmt.Printf("%s %s ...\n", T("删除"), app.BaseDir)
			if err := os.RemoveAll(app.BaseDir); err != nil {
				return err
			}
			// 二进制自身最后删 (Linux 允许 unlink 正在运行的文件)
			for _, bin := range []string{"/usr/bin/mihomo-cli", "/usr/local/bin/mihomo-cli", "/usr/bin/mihomoctl"} {
				_ = os.Remove(bin)
			}
			fmt.Println(T("已彻底卸载 mihomo-cli"))
			return nil
		}
		fmt.Printf("%s %s %s\n", T("已保留数据目录"), app.BaseDir, T("(--purge 可彻底删除)"))
		return nil
	},
}

func shellOr(shells []string) string {
	if len(shells) == 0 {
		return ""
	}
	return shells[0]
}

// installCompletions 安装 shell 补全到全局目录 (系统级, 全用户可用)
func installCompletions(shell string) error {
	if shell == "" {
		return fmt.Errorf("%s: bash|zsh|fish (mihomo-cli install --completion bash)", T("--completion 需要指定 shell"))
	}
	targets := completionTargets()
	var chosen []string
	for _, t := range targets {
		if t.shell == shell {
			chosen = append(chosen, t.shell)
		}
	}
	if len(chosen) == 0 {
		return fmt.Errorf("%s: %q (bash|zsh|fish)", T("未知 shell"), shell)
	}
	for _, t := range targets {
		if t.shell != shell {
			continue
		}
		if err := os.MkdirAll(t.dir, 0o755); err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.gen(&buf); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(t.dir, t.file), buf.Bytes(), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s %s\n", T("补全已安装"), filepath.Join(t.dir, t.file))
	}
	return nil
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: T("前台运行内核(调试模式, Ctrl-C 退出)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		if _, err := os.Stat(app.CoreBin); err != nil {
			return fmt.Errorf("%s (mihomo-cli install --core auto)", T("内核未安装"))
		}
		fmt.Println(T("前台启动内核, 日志输出到终端") + " ...")
		return execSyscall()
	},
}

func init() {
	f := installCmd.Flags()
	f.StringVar(&installProxy, "proxy", "", T("下载内核/资源使用的代理")+", "+T("如")+" http://192.168.1.1:7890")
	f.StringVar(&installMirrorFlag, "mirror", "", T("GitHub 镜像站前缀 (空=失败时自动尝试内置镜像)"))
	f.StringVar(&installCoreSpec, "core", "", "auto|latest|compatible|v1|v2|v3|go120|go123|vX.Y.Z|<flavor>:<version>")
	f.StringVar(&installArch, "arch", "", T("强制指定架构 (默认自动检测)")+": amd64|arm64|armv7|386")
	f.StringVar(&installResource, "resource", "", "mmdb|asn|geoip|geosite|all")
	f.BoolVar(&installSystemd, "systemd", false, T("重新生成 systemd 单元"))
	f.StringVar(&installCompletionS, "completion", "", "bash|zsh|fish")
	installCmd.RegisterFlagCompletionFunc("core", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter([]string{"auto", "latest", "compatible", "v1", "v2", "v3", "go120", "go123", "v2-go120"}, toComplete), noFileComp()
	})
	installCmd.RegisterFlagCompletionFunc("resource", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter(append(core.SortResources(), "all"), toComplete), noFileComp()
	})
	installCmd.RegisterFlagCompletionFunc("completion", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter([]string{"bash", "zsh", "fish"}, toComplete), noFileComp()
	})
	installCmd.RegisterFlagCompletionFunc("arch", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter([]string{"amd64", "arm64", "armv7", "386"}, toComplete), noFileComp()
	})
	installCmd.RegisterFlagCompletionFunc("mirror", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter(append([]string{"auto"}, cfgMirrorPresets()...), toComplete), noFileComp()
	})

	uninstallCmd.Flags().BoolVar(&uninstallCore, "core", false, T("只卸载内核 (先停止服务)"))
	uninstallCmd.Flags().BoolVar(&uninstallResource, "resource", false, T("只删除 geo 资源文件"))
	uninstallCmd.Flags().BoolVar(&uninstallSystemd, "systemd", false, T("只卸载 systemd service 与 timer"))
	uninstallCmd.Flags().StringVar(&uninstallCompletion, "completion", "", "bash|zsh|fish ("+T("不带值=全部")+")")
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, T("彻底卸载: 全部 + 配置目录 + 二进制自身"))
	uninstallCmd.RegisterFlagCompletionFunc("completion", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return prefixFilter([]string{"bash", "zsh", "fish"}, toComplete), noFileComp()
	})

	for _, c := range []*cobra.Command{installCmd, uninstallCmd} {
		markMutating(c)
	}
	rootCmd.AddCommand(installCmd, uninstallCmd, runCmd)
}
