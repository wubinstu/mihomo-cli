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

const serviceName = "mihomo-cli.service"

// unitDir systemd 单元目录。生产环境固定 /etc/systemd/system;
// 测试环境 (MIHOMO_CLI_HOME 指向别处) 放沙箱目录, 避免污染真实 systemd 配置。
var unitDir = func() string {
	if app.BaseDir != "/etc/mihomo-cli" {
		return filepath.Join(app.BaseDir, "systemd")
	}
	return "/etc/systemd/system"
}()

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

// ServiceUnit 生成主服务单元; caps=true 时给内核申请 CAP_NET_ADMIN/CAP_NET_RAW (TUN 需要)
func ServiceUnit(caps bool) string {
	ambi := ""
	if caps {
		ambi = "AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW\nCapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW\n"
	}
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
StandardOutput=journal
StandardError=journal
%s
[Install]
WantedBy=multi-user.target
`, app.CoreBin, app.RuntimeDir, ambi)
}

// InstallService 安装并 enable 主服务; caps=true 时给内核申请 CAP_NET_ADMIN (TUN 需要)
func InstallService(caps bool) error {
	if err := writeUnit(serviceName, ServiceUnit(caps)); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("写入 systemd 单元失败(需要 root)"), err)
	}
	if err := daemonReload(); err != nil {
		return err
	}
	_, err := runRoot("systemctl", "enable", serviceName)
	return err
}

// EnsureCapabilities 按 config.toml 的 tun.enable 重写主服务单元 (TUN 需要 CAP_NET_ADMIN);
// 未安装服务单元时跳过。调用方负责随后的重启。
func EnsureCapabilities(caps bool) error {
	if _, err := os.Stat(filepath.Join(unitDir, serviceName)); err != nil {
		return nil
	}
	return InstallService(caps)
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
	resUnit := fmt.Sprintf(`[Unit]
Description=mihomo-cli: auto update geo resources

[Service]
Type=oneshot
ExecStart=%s resource update-all
`, cli)
	resTimer := fmt.Sprintf(`[Unit]
Description=mihomo-cli: resource update timer

[Timer]
OnBootSec=10min
OnUnitActiveSec=%s
Unit=mihomo-cli-resource.service

[Install]
WantedBy=timers.target
`, systemdDur(s.ResourceAutoUpdateInterval))
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
	if err := writeUnit("mihomo-cli-resource.service", resUnit); err != nil {
		return err
	}
	if err := writeUnit("mihomo-cli-resource.timer", resTimer); err != nil {
		return err
	}
	if err := daemonReload(); err != nil {
		return err
	}
	if s.ResourceAutoUpdateEnabled {
		_, err = runRoot("systemctl", "enable", "--now", "mihomo-cli-resource.timer")
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", "mihomo-cli-resource.timer")
	}
	if err != nil {
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

// InstallTimersQuiet 同 InstallTimers, 失败仅返回错误 (不打印)
func InstallTimersQuiet(s *app.Settings) error { return InstallTimers(s) }

// RemoveAll 卸载全部单元文件
func RemoveAll() {
	_, _ = runRoot("systemctl", "disable", "--now", serviceName, "mihomo-cli-sub.timer", "mihomo-cli-auto.timer", "mihomo-cli-resource.timer")
	removeUnit(serviceName)
	removeUnit("mihomo-cli-sub.service")
	removeUnit("mihomo-cli-sub.timer")
	removeUnit("mihomo-cli-auto.service")
	removeUnit("mihomo-cli-resource.timer")
	removeUnit("mihomo-cli-resource.service")
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

// TimerInterval 读取 timer 的实际执行周期 (OnUnitActiveSec), 读不到返回 "-"
func TimerInterval(name string) string {
	p := filepath.Join(unitDir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return "-"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "OnUnitActiveSec=") {
			v := strings.TrimPrefix(line, "OnUnitActiveSec=")
			if sec, err := strconv.Atoi(strings.TrimSuffix(v, "s")); err == nil && sec > 0 {
				d := time.Duration(sec) * time.Second
				if int(d.Hours())%24 == 0 && d >= 24*time.Hour {
					return fmt.Sprintf("%dh", int(d.Hours()))
				}
				if int(d.Minutes())%60 == 0 && d >= time.Hour {
					return fmt.Sprintf("%dh", int(d.Hours()))
				}
				if int(d.Seconds())%60 == 0 && d >= time.Minute {
					return fmt.Sprintf("%dm", int(d.Minutes()))
				}
				return d.String()
			}
		}
	}
	return "-"
}
