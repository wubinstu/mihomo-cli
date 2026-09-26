package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
)

// cfgLookup 注册键查找 (包内简写)
func cfgLookup(name string) *cfg.Key { return cfg.Lookup(name) }

// cfgMirrorPresets GitHub 镜像站预设
func cfgMirrorPresets() []string { return cfg.GithubMirrorPresets }

// execSyscall 前台运行内核 (替换当前进程)
func execSyscall() error {
	return syscall.Exec(app.CoreBin, []string{app.CoreBin, "-d", app.RuntimeDir}, os.Environ())
}

// stdoutWriter 输出到 stdout (包内统一入口)
func stdoutWriter() *os.File { return os.Stdout }

// mixedPortText 混合端口展示: off 时说明由哪个端口承担
func mixedPortText(s *app.Settings) string {
	if n, ok := app.PortNum(s.MixedPort); ok {
		return fmt.Sprintf("%d (http+socks5)", n)
	}
	if s.ProxyPort() > 0 {
		return fmt.Sprintf("%s: %d (%s)", T("未启用混合端口"), s.ProxyPort(), T("由其他端口承担"))
	}
	return T("未启用")
}

// mustSettingsQuiet 补全路径用: 读不到配置返回 nil 而不要退出进程
// (补全函数绝不能因环境问题把 shell 的 __complete 进程杀掉, 否则用户按 TAB 没反应)
func mustSettingsQuiet() *app.Settings {
	s, err := app.LoadSettings()
	if err != nil {
		return nil
	}
	return s
}
