// Package i18n 提供中/英文输出。语言来源: config.toml 的 lang 项 > $LANG 环境变量 > 中文。
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

func Set(l string) {
	switch strings.ToLower(l) {
	case "zh", "en":
		lang = strings.ToLower(l)
	}
}

var zh = map[string]string{
	// root / 服务
	"svc.start":   "服务已启动",
	"svc.stop":    "服务已停止",
	"svc.restart": "服务已重启",
	"svc.running": "运行中",
	"svc.stopped": "未运行",
	"svc.start.hint": "未运行 (mihomo-cli start)",
	// status
	"st.service":  "服务状态",
	"st.corever":  "内核版本",
	"st.mode":     "代理模式",
	"st.port":     "混合端口",
	"st.lan":      "局域网",
	"st.sub":      "当前订阅",
	"st.lan.on":   "允许 (0.0.0.0)",
	"st.lan.off":  "仅本机 (127.0.0.1)",
	"st.port.hint": "(http+socks5)",
	// proxy
	"proxy.hdr":   "分组\t类型\t当前节点\t节点数",
	"proxy.set.ok": "[%s] %s -> %s",
	"proxy.on": "服务未运行, 已自动启动",
	// sub
	"sub.hdr":     "当前\t名称\t节点数\t更新时间\t流量信息\tURL",
	"sub.updated": "已热重载配置",
	// doctor
	"dr.dir":    "数据目录",
	"dr.core":   "内核",
	"dr.sub":    "订阅",
	"dr.rt":     "运行配置",
	"dr.svc":    "服务",
	"dr.api":    "控制API",
	"dr.port":   "代理端口",
	"dr.lan":    "局域网",
	"dr.subau":  "订阅自动更新",
	"dr.auto":   "自动择优节点",
	"dr.timer.on":  "已启用",
	"dr.timer.off": "已停用",
	"dr.period":    "周期",
	"dr.groups":    "分组",
	"dr.hint":   "提示: 使用 curl -I https://www.google.com 验证代理是否生效 (先 eval $(mihomo-cli env))",
	"dr.api.ok": "API 正常, 内核 ",
	// conn / traffic / log
	"conn.hdr":    "网络\t目标\t代理链\t↑\t↓",
	"conn.active": "活动连接",
	"conn.acc":    "累计",
	"tr.title":    "实时流量 (每秒):",
	// set / get
	"set.saved": "(已保存)",
	"set.regen": "已热重载配置",
}

var en = map[string]string{
	"svc.start":   "service started",
	"svc.stop":    "service stopped",
	"svc.restart": "service restarted",
	"svc.running": "running",
	"svc.stopped": "stopped",
	"svc.start.hint": "not running (mihomo-cli start)",
	"st.service":  "Service",
	"st.corever":  "Core",
	"st.mode":     "Mode",
	"st.port":     "Mixed port",
	"st.lan":      "LAN",
	"st.sub":      "Profile",
	"st.lan.on":   "allowed (0.0.0.0)",
	"st.lan.off":  "localhost only (127.0.0.1)",
	"st.port.hint": "(http+socks5)",
	"proxy.hdr":   "GROUP\tTYPE\tNOW\tNODES",
	"proxy.set.ok": "[%s] %s -> %s",
	"proxy.on": "service was not running, started automatically",
	"sub.hdr":     "*\tNAME\tNODES\tUPDATED\tQUOTA\tURL",
	"sub.updated": "config reloaded",
	"dr.dir":    "Data dir",
	"dr.core":   "Core",
	"dr.sub":    "Profile",
	"dr.rt":     "Runtime cfg",
	"dr.svc":    "Service",
	"dr.api":    "Ctrl API",
	"dr.port":   "Proxy port",
	"dr.lan":    "LAN",
	"dr.subau":  "Sub auto-update",
	"dr.auto":   "Auto select",
	"dr.timer.on":  "enabled",
	"dr.timer.off": "disabled",
	"dr.period":    "interval",
	"dr.groups":    "groups",
	"dr.hint":   "Tip: verify with curl -I https://www.google.com (after eval $(mihomo-cli env))",
	"dr.api.ok": "API ok, core ",
	"conn.hdr":    "NET\tHOST\tCHAIN\tUP\tDOWN",
	"conn.active": "Active conns",
	"conn.acc":    "Total",
	"tr.title":    "Live traffic (/s):",
	"set.saved": "(saved)",
	"set.regen": "config reloaded",
}

// T 取当前语言的文案; 未收录时回退中文
func T(key string) string {
	if lang == "en" {
		if s, ok := en[key]; ok {
			return s
		}
	}
	if s, ok := zh[key]; ok {
		return s
	}
	return key
}
