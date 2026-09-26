package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 系统级目录: 服务/内核/配置全机共享, 管理命令需 root (sudo mihomo-cli ...)
// MIHOMO_CLI_HOME 可整体改名目录 (仅用于测试/容器, 不属于用户接口的一部分)
var (
	BaseDir       = envOr("MIHOMO_CLI_HOME", "/etc/mihomo-cli")
	BinDir        = filepath.Join(BaseDir, "bin")
	ProfileDir    = filepath.Join(BaseDir, "profiles")
	RuntimeDir    = filepath.Join(BaseDir, "runtime")
	VersionsDir   = filepath.Join(BinDir, "versions")
	LogDir        = filepath.Join(BaseDir, "logs")
	CoreBin       = filepath.Join(BinDir, "mihomo") // 当前内核
	CoreBinOld    = filepath.Join(BinDir, "mihomo.old")
	SettingsFile  = filepath.Join(BaseDir, "config.toml")
	OverridesFile = filepath.Join(BaseDir, "overrides.yaml")
	RuntimeConfig = filepath.Join(RuntimeDir, "config.yaml")
	LogFile       = filepath.Join(LogDir, "mihomo.log")
)

// envOr 返回环境变量值, 未设置时返回默认值 (空串视为未设置)
func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func init() {
	// 一次性迁移: 旧版用户目录 ~/.config/mihomo-cli -> /etc/mihomo-cli (需 root)
	legacy := legacyDir()
	if os.Geteuid() == 0 && legacy != "" {
		if _, err := os.Stat(BaseDir); os.IsNotExist(err) {
			if _, err := os.Stat(legacy); err == nil {
				_ = os.Rename(legacy, BaseDir)
			}
		}
	}
}

func legacyDir() string {
	// sudo 调用时取原用户家目录 (SUDO_USER)
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		if home, err := homeOf(u); err == nil {
			return filepath.Join(home, ".config", "mihomo-cli")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "mihomo-cli")
}

// homeOf 返回用户家目录 (查 /etc/passwd)
func homeOf(user string) (string, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) >= 6 && parts[0] == user {
			return parts[5], nil
		}
	}
	return "", fmt.Errorf("user %s not found", user)
}

// EnsureDirs 创建全部数据目录 (需要 root)
func EnsureDirs() error {
	// logs/ 已停用: 内核日志由 journald 接管 (unit StandardOutput=journal)
	for _, d := range []string{BaseDir, BinDir, ProfileDir, RuntimeDir, VersionsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// HardenPerms 把数据目录收敛回系统惯例 (root:root, 目录 0755 / 文件 0644, 内核 0755);
// v1.2 及更早用过 chmod -R o=rwX, 这里一次性纠正历史残留
func HardenPerms() {
	_ = filepath.Walk(BaseDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if p == CoreBin || strings.HasPrefix(p, VersionsDir+string(os.PathSeparator)) {
			_ = os.Chmod(p, 0o755)
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(p, 0o755)
			return nil
		}
		if strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".toml") ||
			strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".mmdb") || strings.HasSuffix(p, ".dat") {
			_ = os.Chmod(p, 0o644)
		}
		return nil
	})
}
