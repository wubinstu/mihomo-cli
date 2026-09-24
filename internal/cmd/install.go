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
	"github.com/wubinstu/mihomo-cli/internal/subs"
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

var installProxy string
var installSub string
var installCompatible bool
var installAllowLan bool
var installCompletion string

var installCmd = &cobra.Command{
	Use:   "install",
	Short: T("安装 mihomo 内核并注册 systemd 服务"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli install)", T("安装需要 root 权限: 配置目录 /etc/mihomo-cli 与 systemd 单元"))
		}
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if installAllowLan {
			s.AllowLan = true
		}
		if err := app.EnsureDirs(); err != nil {
			return err
		}

		// 1. 内核
		if _, err := os.Stat(app.CoreBin); os.IsNotExist(err) {
			rel, err := core.Latest(installProxy)
			if err != nil {
				return err
			}
			fmt.Printf("%s: %s (arch=%s)\n", T("最新内核版本"), rel.TagName, core.ArchName())
			if err := core.DownloadInstall(rel.TagName, installProxy, s.GithubMirror, installCompatible); err != nil {
				return err
			}
		} else {
			v, _ := core.Version()
			fmt.Printf("%s: %s\n", T("内核已安装"), v)
		}
		if err := s.Save(); err != nil {
			return err
		}

		// 2. geo 数据预下载 (避免内核首次启动直连 GitHub 下载 MMDB 失败导致 fatal 循环)
		fmt.Println(T("预下载 geo 数据 (geoip/geosite) ..."))
		if err := core.DownloadGeo(installProxy, s.GithubMirror); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: geo 数据下载失败"), err)
		}

		// 3. systemd 服务
		fmt.Println(T("注册 systemd 服务 ..."))
		if err := sysd.InstallService(); err != nil {
			return err
		}

		// 4. 订阅
		if installSub != "" {
			if err := subs.Add(s, "default", installSub, installProxy); err != nil {
				return fmt.Errorf("%s: %w", T("添加订阅失败"), err)
			}
		}
		if s.Current() != nil {
			if err := render.Generate(s); err != nil {
				return err
			}
		}
		if err := sysd.InstallTimers(s); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 定时任务安装失败"), err)
		}
		installCompletions(installCompletion)

		// 配置目录属主交给发起安装的用户 (sudo 调用), 该用户后续无需 sudo 即可管理
		if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
			_ = exec.Command("chown", "-R", u+":", app.BaseDir).Run()
		}

		fmt.Printf("\n%s\n", T("安装完成。后续步骤:"))
		steps := []string{
			"mihomo-cli init                # " + T("添加订阅"),
			"mihomo-cli start               # " + T("启动代理服务"),
			"eval $(mihomo-cli proxy on)    # " + T("当前 shell 开启代理"),
			"mihomo-cli doctor              # " + T("体检: 内核/服务/端口/API/订阅/定时器"),
		}
		for _, l := range steps {
			fmt.Println("  " + l)
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
		fmt.Println(T("停止并移除 systemd 单元 ..."))
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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: T("交互式初始化: 添加第一个订阅并启用服务"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if s.Current() != nil {
			fmt.Printf("%s %s\n", T("已有订阅"), s.Current().Name)
		}
		fmt.Print(T("请输入订阅链接 (clash 订阅 URL): "))
		line, _ := readLine()
		rawurl := strings.TrimSpace(line)
		if rawurl == "" || !strings.Contains(rawurl, "://") {
			return fmt.Errorf("%s", T("无效的订阅链接"))
		}
		if err := subs.Add(s, "default", rawurl, ""); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		fmt.Printf("%s mihomo-cli start\n", T("订阅已就绪。启动服务:"))
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
			return fmt.Errorf("%s", T("没有订阅, 请先 mihomo-cli init"))
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		fmt.Println(T("前台启动内核, 日志输出到终端 ..."))
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
			fmt.Printf("  %d) %-5s (%s)\\n", j+1, t.shell, t.dir)
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
	installCmd.Flags().StringVar(&installProxy, "proxy", "", T("下载内核使用的代理")+", "+T("如")+" http://192.168.1.1:7890")
	installCmd.Flags().StringVar(&installSub, "sub", "", T("订阅链接")+"("+T("跳过交互式 init")+")")
	installCmd.Flags().BoolVar(&installCompatible, "compatible", false, T("使用 amd64-compatible 内核(老旧 CPU)"))
	installCmd.Flags().BoolVar(&installAllowLan, "allow-lan", false, T("允许局域网设备使用代理 (0.0.0.0)"))
	installCmd.Flags().StringVar(&installCompletion, "completion", "", T("安装指定 shell 的补全 (bash/zsh/fish; 空=交互选择)"))
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, T("同时删除配置/订阅/内核数据"))
	rootCmd.AddCommand(installCmd, uninstallCmd, initCmd, runCmd)
}
