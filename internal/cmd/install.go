package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// reloadIfActive 服务运行中则热重载配置
func reloadIfActive(s *app.Settings) {
	if sysd.IsActive() {
		if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 热重载失败: %v (可执行 mihomo-cli restart)\n", err)
		} else {
			fmt.Println("已热重载配置")
		}
	}
}

var installProxy string
var installSub string
var installCompatible bool
var installAllowLan bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "安装 mihomo 内核并注册 systemd 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if installProxy != "" {
			s.DownloadProxy = installProxy
		}
		if installAllowLan {
			s.AllowLan = true
		}
		if err := app.EnsureDirs(); err != nil {
			return err
		}

		// 1. 内核
		if _, err := os.Stat(app.CoreBin); os.IsNotExist(err) {
			rel, err := core.Latest(s.DownloadProxy)
			if err != nil {
				return err
			}
			fmt.Printf("最新内核版本: %s (arch=%s)\n", rel.TagName, core.ArchName())
			if err := core.DownloadInstall(rel.TagName, s.DownloadProxy, installCompatible); err != nil {
				return err
			}
		} else {
			v, _ := core.Version()
			fmt.Printf("内核已安装: %s\n", v)
		}
		if err := s.Save(); err != nil {
			return err
		}

		// 2. systemd 服务
		fmt.Println("注册 systemd 服务 ...")
		if err := sysd.InstallService(); err != nil {
			return err
		}

		// 3. 订阅
		if installSub != "" {
			name := "default"
			if err := subs.Add(s, name, installSub); err != nil {
				return fmt.Errorf("添加订阅失败: %w", err)
			}
		}
		if s.Current() != nil {
			if err := render.Generate(s); err != nil {
				return err
			}
		}
		if err := sysd.InstallTimers(s); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 定时任务安装失败: %v\n", err)
		}
		installCompletions()

		fmt.Println(`
安装完成。后续步骤:
  mihomo-cli init                     # 交互式添加订阅(如果还没有)
  mihomo-cli start                    # 启动代理服务
  eval $(mihomo-cli env)              # 当前 shell 开启代理
  mihomo-cli doctor                   # 体检`)
		return nil
	},
}

// installCompletions 为已安装的 shell 安装补全脚本 (跟随 mihomo-cli install)
func installCompletions() {
	type target struct {
		dir  string
		file string
		gen  func(w io.Writer) error
	}
	self, _ := os.Executable()
	_ = self
	targets := []target{
		{"/usr/local/share/bash-completion/completions", "mihomo-cli", func(w io.Writer) error { return rootCmd.GenBashCompletionV2(w, false) }},
		{"/usr/local/share/zsh/site-functions", "_mihomo-cli", func(w io.Writer) error { return rootCmd.GenZshCompletion(w) }},
		{"/usr/local/share/fish/completions", "mihomo-cli.fish", func(w io.Writer) error { return rootCmd.GenFishCompletion(w, false) }},
	}
	home, _ := os.UserHomeDir()
	fallbacks := map[int]string{
		0: home + "/.local/share/bash-completion/completions",
		1: home + "/.local/share/zsh/site-functions",
		2: home + "/.config/fish/completions",
	}
	for i, t := range targets {
		dir := t.dir
		if _, err := os.Stat(t.dir); err != nil {
			dir = fallbacks[i]
			if err := os.MkdirAll(dir, 0o755); err != nil {
				continue
			}
		}
		if dir == t.dir && os.Geteuid() != 0 {
			// 系统目录需要 root: sudo tee
			var buf bytes.Buffer
			if t.gen(&buf) != nil {
				continue
			}
			c := exec.Command("sudo", "tee", filepath.Join(dir, t.file))
			c.Stdin = &buf
			c.Stdout = nil
			c.Stderr = os.Stderr
			if c.Run() == nil {
				fmt.Printf("补全已安装: %s\n", filepath.Join(dir, t.file))
			}
			continue
		}
		f, err := os.Create(filepath.Join(dir, t.file))
		if err != nil {
			continue
		}
		if err := t.gen(f); err == nil {
			fmt.Printf("补全已安装: %s\n", filepath.Join(dir, t.file))
		}
		f.Close()
	}
}

// removeCompletions 清理补全脚本 (跟随 uninstall)
func removeCompletions() {
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/usr/local/share/bash-completion/completions/mihomo-cli",
		"/usr/local/share/zsh/site-functions/_mihomo-cli",
		"/usr/local/share/fish/completions/mihomo-cli.fish",
		home + "/.local/share/bash-completion/completions/mihomo-cli",
		home + "/.local/share/zsh/site-functions/_mihomo-cli",
		home + "/.config/fish/completions/mihomo-cli.fish",
	} {
		if _, err := os.Stat(p); err == nil {
			if os.Geteuid() == 0 {
				_ = os.Remove(p)
			} else {
				_ = exec.Command("sudo", "rm", "-f", p).Run()
			}
		}
	}
}

var uninstallPurge bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "卸载服务与单元文件 (--purge 同时删除配置/订阅/内核)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("停止并移除 systemd 单元 ...")
		sysd.RemoveAll()
		removeCompletions()
		if uninstallPurge {
			home, _ := os.UserHomeDir()
			fmt.Printf("删除 %s ...\n", app.BaseDir)
			if err := os.RemoveAll(app.BaseDir); err != nil {
				return err
			}
			_ = home
		} else {
			fmt.Printf("已保留数据目录 %s (使用 --purge 彻底删除)\n", app.BaseDir)
		}
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "交互式初始化: 添加第一个订阅并启用服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if s.Current() != nil {
			fmt.Printf("已有订阅: %s\n", s.Current().Name)
		}
		fmt.Print("请输入订阅链接 (clash 订阅 URL): ")
		var rawurl string
		if _, err := fmt.Scanln(&rawurl); err != nil && strings.TrimSpace(rawurl) == "" {
			rawurl = ""
		}
		// Scanln 对含空格输入有问题, 换 bufio 读整行
		if rawurl == "" {
			line, _ := readLine()
			rawurl = strings.TrimSpace(line)
		}
		if rawurl == "" || !strings.Contains(rawurl, "://") {
			return fmt.Errorf("无效的订阅链接")
		}
		if err := subs.Add(s, "default", rawurl); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		fmt.Println("订阅已就绪。启动服务: mihomo-cli start")
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "前台运行内核(调试模式, Ctrl-C 退出)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		if s.Current() == nil {
			return fmt.Errorf("没有订阅, 请先 mihomo-cli init")
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		fmt.Println("前台启动内核, 日志输出到终端 ...")
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

func init() {
	installCmd.Flags().StringVar(&installProxy, "proxy", "", "下载内核使用的代理, 如 http://192.168.1.1:7890")
	installCmd.Flags().StringVar(&installSub, "sub", "", "订阅链接(跳过交互式 init)")
	installCmd.Flags().BoolVar(&installCompatible, "compatible", false, "使用 amd64-compatible 内核(老旧 CPU)")
	installCmd.Flags().BoolVar(&installAllowLan, "allow-lan", false, "允许局域网设备使用代理(监听 0.0.0.0)")
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, "同时删除配置/订阅/内核数据")
	rootCmd.AddCommand(installCmd, uninstallCmd, initCmd, runCmd)
}
