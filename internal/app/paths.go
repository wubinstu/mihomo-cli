package app

import (
	"os"
	"path/filepath"
)

// 目录与文件路径。CLI 本体建议安装为 /usr/local/bin/mihomo-cli,
// 内核二进制存放在私有目录、不进入 PATH, 避免命名歧义。
var (
	BaseDir       string // ~/.config/mihomo-cli
	BinDir        string
	ProfileDir    string
	RuntimeDir    string
	LogDir        string
	CoreBin       string // 当前内核
	CoreBinOld    string // 升级前备份
	SettingsFile  string // config.toml
	OverridesFile string // overrides.yaml
	RuntimeConfig string // runtime/config.yaml (生成物)
	LogFile       string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	BaseDir = filepath.Join(home, ".config", "mihomo-cli")
	BinDir = filepath.Join(BaseDir, "bin")
	ProfileDir = filepath.Join(BaseDir, "profiles")
	RuntimeDir = filepath.Join(BaseDir, "runtime")
	LogDir = filepath.Join(BaseDir, "logs")
	CoreBin = filepath.Join(BinDir, "mihomo")
	CoreBinOld = filepath.Join(BinDir, "mihomo.old")
	SettingsFile = filepath.Join(BaseDir, "config.toml")
	OverridesFile = filepath.Join(BaseDir, "overrides.yaml")
	RuntimeConfig = filepath.Join(RuntimeDir, "config.yaml")
	LogFile = filepath.Join(LogDir, "mihomo.log")
}

// EnsureDirs 创建全部数据目录
func EnsureDirs() error {
	for _, d := range []string{BaseDir, BinDir, ProfileDir, RuntimeDir, LogDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
