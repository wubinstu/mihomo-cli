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

const serviceName = "mihomo-core.service"

const (
	subTimerName      = "mihomo-cli-sub.timer"
	subUnitName       = "mihomo-cli-sub.service"
	nodeTimerName     = "mihomo-cli-node.timer"
	nodeUnitName      = "mihomo-cli-node.service"
	resourceTimerName = "mihomo-cli-resource.timer"
	resourceUnitName  = "mihomo-cli-resource.service"
)

// cliPath timer 里 ExecStart 用的固定路径: install/uninstall 都是 root-only 且就装在这里,
// 绝不能用 os.Executable() —— 用户从 /tmp 跑一次二进制就会把 /tmp/mihomo-cli 写进 timer。
const cliPath = "/usr/bin/mihomo-cli"

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
	cli := CLIPath()

	subUnit := fmt.Sprintf(`[Unit]
Description=mihomo-cli: subscription update timer service

[Service]
Type=oneshot
ExecStart=%s sub update all
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
Description=mihomo-cli: node auto-select service

[Service]
Type=oneshot
ExecStart=%s node auto
`, cli)
	resUnit := fmt.Sprintf(`[Unit]
Description=mihomo-cli: geo resource update service

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
Description=mihomo-cli: node auto-select timer

[Timer]
OnBootSec=2min
OnUnitActiveSec=%s
Unit=mihomo-cli-node.service

[Install]
WantedBy=timers.target
`, systemdDur(s.NodeAutoSelectInterval))

	if err := writeUnit(subUnitName, subUnit); err != nil {
		return err
	}
	if err := writeUnit(subTimerName, subTimer); err != nil {
		return err
	}
	if err := writeUnit(nodeUnitName, autoUnit); err != nil {
		return err
	}
	if err := writeUnit(nodeTimerName, autoTimer); err != nil {
		return err
	}
	if err := writeUnit(resourceUnitName, resUnit); err != nil {
		return err
	}
	if err := writeUnit(resourceTimerName, resTimer); err != nil {
		return err
	}
	if err := daemonReload(); err != nil {
		return err
	}
	if s.ResourceAutoUpdateEnabled {
		if _, err := runRoot("systemctl", "enable", "--now", resourceTimerName); err != nil {
			return err
		}
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", resourceTimerName)
	}
	// 按设置启停
	if s.SubAutoUpdateEnabled {
		if _, err := runRoot("systemctl", "enable", "--now", subTimerName); err != nil {
			return err
		}
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", subTimerName)
	}
	if s.NodeAutoSelectEnabled {
		if _, err := runRoot("systemctl", "enable", "--now", nodeTimerName); err != nil {
			return err
		}
	} else {
		_, _ = runRoot("systemctl", "disable", "--now", nodeTimerName)
	}
	return nil
}

func systemdDur(d time.Duration) string {
	return strconv.FormatInt(int64(d.Seconds()), 10) + "s"
}

// InstallTimersQuiet 同 InstallTimers, 失败仅返回错误 (不打印)
func InstallTimersQuiet(s *app.Settings) error { return InstallTimers(s) }

// RemoveAll 卸载全部单元文件 (服务 + 三个 timer 的 service/timer)
func RemoveAll() {
	_, _ = runRoot("systemctl", "disable", "--now", serviceName, subTimerName, nodeTimerName, resourceTimerName)
	for _, u := range []string{serviceName, subUnitName, subTimerName,
		nodeUnitName, nodeTimerName, resourceUnitName, resourceTimerName} {
		removeUnit(u)
	}
	_ = daemonReload()
}

// ServiceName 主服务单元名 (外部引用用)
func ServiceName() string { return serviceName }

// SubTimerName / NodeTimerName / ResourceTimerName 三个定时器单元名
func SubTimerName() string      { return subTimerName }
func NodeTimerName() string     { return nodeTimerName }
func ResourceTimerName() string { return resourceTimerName }

// CLIPath timer 里 ExecStart 的路径: 固定 /usr/bin/mihomo-cli,
// 该文件不存在(开发/沙箱)时才回退到当前可执行文件并告警。
func CLIPath() string {
	if _, err := os.Stat(cliPath); err == nil {
		return cliPath
	}
	self, err := os.Executable()
	if err != nil {
		return cliPath
	}
	abs, _ := filepath.Abs(self)
	fmt.Fprintf(os.Stderr, i18n.T("警告: 未找到 %s, timer 将使用 %s (安装后请重跑 install --systemd)\n"), cliPath, abs)
	return abs
}

func Service(action string) error {
	_, err := runRoot("systemctl", action, serviceName)
	return err
}

// IsActive 服务是否正在运行
func IsActive() bool {
	_, err := runRoot("systemctl", "is-active", "--quiet", serviceName)
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

// TimerLast 读取 timer 上一次实际触发时间 (systemd 权威源)。
// oneshot 成功或失败 systemd 都会更新它, 所以它比"我们自己存在 config.toml 里的
// last_run"更可靠 —— 后者在任务失败时不写回, 会让"下次剩余时间"一直停在 0m。
// 读不到(未安装/被禁用/从未触发)返回零值。
func TimerLast(name string) time.Time {
	cmd := exec.Command("systemctl", "show", name, "-p", "LastTriggerUSec", "--value")
	if os.Geteuid() != 0 {
		cmd = exec.Command("sudo", "-n", "systemctl", "show", name, "-p", "LastTriggerUSec", "--value")
	}
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}
	}
	return parseSystemdTime(strings.TrimSpace(string(out)))
}

// parseSystemdTime 解析 systemctl show 的时间输出: "Thu 2026-10-08 09:54:51 CST"
// (也可能是裸微秒数 / "n/a" / 空)
func parseSystemdTime(v string) time.Time {
	if v == "" || v == "n/a" || v == "0" {
		return time.Time{}
	}
	if us, err := strconv.ParseInt(v, 10, 64); err == nil && us > 0 {
		return time.UnixMicro(us)
	}
	for _, layout := range []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 MST",
		"Mon Jan _2 15:04:05 2006",
	} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
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
