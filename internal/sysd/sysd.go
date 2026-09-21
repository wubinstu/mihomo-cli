package sysd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

const unitDir = "/etc/systemd/system"

const serviceName = "mihomo-cli.service"

// runRoot 以 root 权限执行命令(需要时自动加 sudo)
func runRoot(name string, args ...string) (string, error) {
	if os.Geteuid() != 0 {
		args = append([]string{name}, args...)
		name = "sudo"
	}
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func writeUnit(name, content string) error {
	if os.Geteuid() == 0 {
		return os.WriteFile(filepath.Join(unitDir, name), []byte(content), 0o644)
	}
	cmd := exec.Command("sudo", "tee", filepath.Join(unitDir, name))
	cmd.Stdin = bytes.NewBufferString(content)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func removeUnit(name string) {
	_, _ = runRoot("rm", "-f", filepath.Join(unitDir, name))
}

func daemonReload() error {
	_, err := runRoot("systemctl", "daemon-reload")
	return err
}

// ServiceUnit 生成主服务单元
func ServiceUnit() string {
	return fmt.Sprintf(`[Unit]
Description=mihomo proxy core (managed by mihomo-cli)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s -d %s
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
StandardOutput=append:%s
StandardError=inherit

[Install]
WantedBy=multi-user.target
`, app.CoreBin, app.RuntimeDir, app.LogFile)
}

// InstallService 安装并 enable 主服务
func InstallService() error {
	if err := writeUnit(serviceName, ServiceUnit()); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("写入 systemd 单元失败(需要 root)"), err)
	}
	if err := daemonReload(); err != nil {
		return err
	}
	_, err := runRoot("systemctl", "enable", serviceName)
	return err
}

// InstallTimers 安装订阅自动更新与自动选节点的 systemd timer
func InstallTimers(s *app.Settings) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cli, _ := filepath.Abs(self)

	subUnit := fmt.Sprintf(`[Unit]
Description=mihomo-cli: auto update subscriptions

[Service]
Type=oneshot
ExecStart=%s sub update
`, cli)
	subTimer := fmt.Sprintf(`[Unit]
Description=mihomo-cli: subscription update timer

[Timer]
OnBootSec=3min
OnUnitActiveSec=%s
Unit=mihomo-cli-sub.service

[Install]
WantedBy=timers.target
`, systemdDur(s.SubAutoUpdateInterval))

	autoUnit := fmt.Sprintf(`[Unit]
Description=mihomo-cli: auto select lowest-latency proxy

[Service]
Type=oneshot
ExecStart=%s node auto
`, cli)
	autoTimer := fmt.Sprintf(`[Unit]
Description=mihomo-cli: auto select timer

[Timer]
OnBootSec=2min
OnUnitActiveSec=%s
Unit=mihomo-cli-auto.service

[Install]
WantedBy=timers.target
`, systemdDur(s.NodeAutoSelectInterval))

	if err := writeUnit("mihomo-cli-sub.service", subUnit); err != nil {
		return err
	}
	if err := writeUnit("mihomo-cli-sub.timer", subTimer); err != nil {
		return err
	}
	if err := writeUnit("mihomo-cli-auto.service", autoUnit); err != nil {
		return err
	}
	if err := writeUnit("mihomo-cli-auto.timer", autoTimer); err != nil {
		return err
	}
	if err := daemonReload(); err != nil {
		return err
	}
	// 按设置启停
	if s.SubAutoUpdateEnabled {
		_, err = runRoot("systemctl", "enable", "--now", "mihomo-cli-sub.timer")
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", "mihomo-cli-sub.timer")
	}
	if err != nil {
		return err
	}
	if s.NodeAutoSelectEnabled {
		_, err = runRoot("systemctl", "enable", "--now", "mihomo-cli-auto.timer")
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", "mihomo-cli-auto.timer")
	}
	return err
}

func systemdDur(d time.Duration) string {
	return strconv.FormatInt(int64(d.Seconds()), 10) + "s"
}

// RemoveAll 卸载全部单元文件
func RemoveAll() {
	_, _ = runRoot("systemctl", "disable", "--now", serviceName, "mihomo-cli-sub.timer", "mihomo-cli-auto.timer")
	removeUnit(serviceName)
	removeUnit("mihomo-cli-sub.service")
	removeUnit("mihomo-cli-sub.timer")
	removeUnit("mihomo-cli-auto.service")
	removeUnit("mihomo-cli-auto.timer")
	_ = daemonReload()
}

func Service(action string) error {
	_, err := runRoot("systemctl", action, serviceName)
	return err
}

// IsActive 服务是否正在运行
func IsActive() bool {
	out, err := runRoot("systemctl", "is-active", "--quiet", serviceName)
	_ = out
	return err == nil
}

func TimerEnabled(name string) bool {
	cmd := exec.Command("systemctl", "is-enabled", name)
	if os.Geteuid() != 0 {
		cmd = exec.Command("sudo", "-n", "systemctl", "is-enabled", name)
	}
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "enabled"
}
