// Package i18n 提供中/英文输出。语言来源: config.toml 的 lang 项 > $LANG 环境变量 > 中文。
// T() 以中文原文为 key, 英文表未收录时原样返回中文。
package i18n

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

var lang = "zh"

func init() {
	var cfg struct {
		Lang string `toml:"lang"`
	}
	if data, err := os.ReadFile(settingsPath()); err == nil {
		_ = toml.Unmarshal(data, &cfg)
	}
	switch strings.ToLower(cfg.Lang) {
	case "zh", "en":
		lang = strings.ToLower(cfg.Lang)
	default:
		l := os.Getenv("LANG")
		switch {
		case strings.Contains(l, "zh"):
			lang = "zh"
		case l != "":
			lang = "en"
		}
	}
}

func settingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return home + "/.config/mihomo-cli/config.toml"
}

// Lang 当前语言 ("zh"/"en")
func Lang() string { return lang }

// Set 切换语言 (运行期)
func Set(l string) {
	switch strings.ToLower(l) {
	case "zh", "en":
		lang = strings.ToLower(l)
	}
}

// T 翻译: 英文模式下查表, 未收录/中文模式返回原文
func T(s string) string {
	if lang == "en" {
		if e, ok := en[s]; ok {
			return e
		}
	}
	return s
}

var en = map[string]string{
	// ---- root / help ----
	"mihomo 内核的纯 CLI 管理外壳 (Linux 服务器代理工具)": "Pure-CLI manager for the mihomo proxy core (Linux server proxy tool)",
	"面向 Linux 服务器的 Clash/mihomo 代理管理工具":       "Clash/mihomo proxy manager for Linux servers",
	"安装 mihomo 内核并注册 systemd 服务":              "install mihomo core and register the systemd service",
	"卸载服务与单元文件 (--purge 同时删除配置/订阅/内核)":     "uninstall service and unit files (--purge also removes data)",
	"交互式初始化: 添加第一个订阅并启用服务":               "interactive init: add first subscription and enable service",
	"启动代理服务":                                      "start proxy service",
	"停止代理服务":                                      "stop proxy service",
	"重启代理服务":                                      "restart proxy service",
	"查看服务与代理状态":                                   "show service and proxy status",
	"前台运行内核(调试模式, Ctrl-C 退出)":                 "run core in foreground (debug, Ctrl-C to quit)",
	"订阅管理: add/rm/list/update/use":               "subscription management: add/rm/list/update/use",
	"查看/切换代理分组与节点":                                "view/switch groups and nodes",
	"开/关当前 shell 代理(配合 alias)":                   "toggle proxy env for current shell (use with alias)",
	"查看活动连接 (--watch 持续刷新)":                      "show active connections (--watch to refresh)",
	"实时上下行流量 (Ctrl-C 退出)":                        "live upload/download traffic (Ctrl-C to quit)",
	"查看内核日志 (-f 跟随)":                              "show core logs (-f follow)",
	"体检: 内核/服务/端口/API/订阅/定时器":                    "health check: core/service/port/API/subs/timers",
	"查看设置 (无参数 = 全部)":                            "show settings (no arg = all)",
	"修改设置并生效":                                     "change a setting and apply",
	"内核管理: version/upgrade/rollback":              "core management: version/upgrade/rollback",
	"已安装内核版本":                                     "installed core version",
	"升级内核 (从 GitHub Releases)":                    "upgrade core (from GitHub Releases)",
	"回滚到上一版本":                                     "rollback to previous core version",
	"mihomo-cli 版本":                                "mihomo-cli version",
	"查看代理分组列表 (索引别名 #1..#n)":                      "list proxy groups (index aliases #1..#n)",
	"设置当前操作分组":                                    "set the current working group",
	"列出当前分组的节点 (索引别名 #1..#n)":                     "list nodes of current group (index aliases)",
	"切换当前分组到指定节点":                                 "switch current group to a node",
	"测试当前分组节点延迟 (彩色)":                            "test node delays of current group (colored)",
	"对分组测速并切换到延迟最低的节点":                           "test and switch to the lowest-latency node",

	// ---- install / uninstall ----
	"最新内核版本":                 "latest core version",
	"内核已安装":                  "core already installed",
	"注册 systemd 服务 ...":      "registering systemd service ...",
	"安装完成。后续步骤:":             "install done. Next steps:",
	"停止并移除 systemd 单元 ...":   "stopping and removing systemd units ...",
	"已保留数据目录":               "data directory kept",
	"(使用 --purge 彻底删除)":      "(use --purge to remove everything)",
	"删除":                     "removing",
	"补全已安装":                  "completion installed:",
	"下载内核":                   "downloading core:",
	"已下载":                    "downloaded",

	// ---- init ----
	"请输入订阅链接 (clash 订阅 URL): ": "enter subscription URL (clash sub): ",
	"无效的订阅链接":                 "invalid subscription URL",
	"订阅已就绪。启动服务:":             "subscription ready. Start with:",
	"已有订阅":                    "existing profile:",

	// ---- service ----
	"服务已启动":                    "service started",
	"服务已停止":                    "service stopped",
	"服务已重启":                    "service restarted",
	"运行中":                      "running",
	"未运行":                      "not running",
	"服务状态":                     "Service",
	"内核版本":                     "Core",
	"代理模式":                     "Mode",
	"混合端口":                     "Mixed port",
	"局域网":                      "LAN",
	"当前订阅":                     "Profile",
	"当前分组":                     "Group",
	"允许 (0.0.0.0)":              "allowed (0.0.0.0)",
	"仅本机 (127.0.0.1)":           "localhost only (127.0.0.1)",
	"规则":                       "rule",
	"未运行 (mihomo-cli start)":   "not running (mihomo-cli start)",
	"没有订阅, 请先 mihomo-cli init": "no subscription, run mihomo-cli init first",

	// ---- sub ----
	"下载订阅":                    "downloading subscription",
	"订阅":                      "profile",
	"已添加并生效":                  "added and activated",
	"已删除":                     "removed",
	"更新订阅":                    "updating profile",
	"节点数":                      "nodes",
	"已热重载配置":                  "config reloaded",
	"当前订阅已切换为":                "current profile switched to",
	"已存在":                     "already exists",
	"不存在 (mihomo-cli sub list 查看)": "not found (see mihomo-cli sub list)",
	"没有可用订阅":                  "no subscription available",

	// ---- group / node ----
	"分组":                       "GROUP",
	"类型":                       "TYPE",
	"当前节点":                     "NOW",
	"节点":                       "NODE",
	"当前操作分组已切换为":              "current working group set to",
	"不在该分组中":                  "not in this group",
	"匹配到多个, 请更精确":             "matches multiple, be more specific",
	"手动切换成功。注意: 自动择优已开启, 下次定时任务可能覆盖此设置": "switched. NOTE: auto-select is enabled and may override this at the next interval",
	"超时":                       "timeout",
	"个节点超时/失败":                "nodes timed out/failed",
	"测试分组":                    "testing group",
	"没有 Selector 分组":           "no Selector group found",
	"全部节点不可用":                 "all nodes unavailable",
	"测速失败":                     "delay test failed",
	"切换失败":                     "switch failed",
	"跳过":                      "skip",
	"无真实节点, 策略组":              "no real nodes (policy group)",
	"未设置当前分组, 请先 mihomo-cli group use <id|名称>": "no current group, run mihomo-cli group use <id|name>",
	"找不到分组":                   "group not found:",

	// ---- doctor ----
	"数据目录":                     "Data dir",
	"内核":                       "Core",
	"服务":                       "Service",
	"控制API":                    "Ctrl API",
	"代理端口":                     "Proxy port",
	"订阅自动更新":                  "Sub auto-update",
	"自动择优节点":                  "Auto select",
	"已启用":                      "enabled",
	"已停用":                      "disabled",
	"周期":                       "interval",
	"监听中":                      "LISTEN",
	"未监听":                      "-",
	"API 正常, 内核":               "API ok, core",
	"提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli proxy on))": "Tip: verify with curl -I https://www.google.com (after eval $(mihomo-cli proxy on))",

	// ---- conn / traffic / log ----
	"活动连接":                     "Active conns",
	"累计":                       "total",
	"网络":                       "NET",
	"目标":                       "HOST",
	"代理链":                      "CHAIN",
	"实时流量 (每秒):":               "Live traffic (/s):",
	"暂无日志":                    "no logs yet:",

	// ---- set / get ----
	"已保存":                      "(saved)",
	"无效端口":                     "invalid port",
	"无效周期 (>=1m), 如 12h":       "invalid interval (>=1m), e.g. 12h",
	"无效超时":                     "invalid timeout",
	"未知配置项":                    "unknown setting key:",
	"lang 仅支持 zh / en":         "lang must be zh or en",
	"proxy-mode 仅支持 rule / global / direct": "proxy-mode must be rule/global/direct",
	"警告: 更新定时器失败":             "warn: timer update failed:",
	"警告: 热重载失败":               "warn: hot reload failed:",
	"(可执行 mihomo-cli restart)": "(try mihomo-cli restart)",
	"警告: 定时任务安装失败":            "warn: timer install failed:",

	// ---- core ----
	"没有可回滚的旧版本":               "no old core to rollback to",
	"内核已是最新版本":                "core is already the latest:",
	"升级内核:":                    "upgrading core:",
	"内核未安装, 请先执行 mihomo-cli install": "core not installed, run mihomo-cli install",

	// ---- update / unuse / chain ----
	"更新 mihomo-cli 自身 (从 GitHub Releases)": "update mihomo-cli itself (from GitHub Releases)",
	"已是最新版本":                   "already the latest version",
	"升级":                        "upgrading",
	"已安装":                       "installed",
	"安装包中未找到二进制":               "binary not found in package",
	"访问失败":                      "failed to reach",
	"当前无生效订阅(sub 悬空), 代理未生效":    "no active profile (sub unused), proxy inactive",
	"当前链路":                      "Chain",
	"提示: 分组未选择, 可执行 mihomo-cli group use <id|名称>": "hint: no group selected, run mihomo-cli group use <id|name>",
	"没有可用订阅, 请先 mihomo-cli sub use <id|名称>": "no subscription, run mihomo-cli sub use <id|name> first",
	"当前分组已是悬空状态":               "current group is already unused",
	"已取消, 该分组流量走 DIRECT 直连":     "unused; traffic of this group goes DIRECT",
	"取消当前分组选择: 该分组流量走 DIRECT 直连": "unset current group: its traffic goes DIRECT",
	"取消当前节点选择: 分组流量走 DIRECT 直连":  "unset current node: group traffic goes DIRECT",
	"已取消, 分组流量走 DIRECT 直连":      "unused; group traffic goes DIRECT",
	"取消失败":                      "unuse failed",
	"取消当前订阅: 停止代理服务, group/node 级联悬空": "unset current sub: stops service, cascades group/node to unused",
	"当前订阅已是悬空状态":               "current profile is already unused",
	"代理服务已停止, 网络不再被代理":          "proxy service stopped; traffic is no longer proxied",
	"悬空 (sub unuse)":            "unused (sub unuse)",
	"节点自动择优":                    "Node auto-select",
	"上次":                        "last",
	"下次":                        "next",
	"无当前分组, 跳过自动择优 (mihomo-cli group use <id|名称>)": "no current group, auto-select skipped (mihomo-cli group use <id|name>)",
	"对当前分组测速并切换到延迟最低的节点":         "test and switch to the lowest-latency node of the current group",

	// ---- proxy on/off ----
	"服务未运行, 已自动启动":             "service was not running, started automatically",

	// ---- 内部包 ----
	"访问 GitHub API 失败":         "failed to reach GitHub API",
	"GitHub API 返回":            "GitHub API returned",
	"(可用 --proxy 指定代理)":        "(use --proxy to specify a proxy)",
	"可能被限流, 可稍后重试":             "rate-limited, retry later",
	"release 中未找到资产":           "asset not found in release",
	"读取订阅文件失败":                "failed to read profile file",
	"订阅配置解析失败":                "failed to parse subscription yaml",
	"没有可用订阅, 请先执行 mihomo-cli init 或 mihomo-cli sub add": "no subscription, run mihomo-cli init first",
	"订阅服务器返回":                  "subscription server returned",
	"订阅内容不是 clash yaml 格式":     "subscription is not clash yaml",
	"无法连接 mihomo API(服务是否已启动?)": "cannot reach mihomo API (is the service running?)",
	"写入 systemd 单元失败(需要 root)":  "failed to write systemd unit (root required)",
	"下载失败":                     "download failed",
}
