package cfg

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
)

// Live 一个配置项的运行态视图: 内核 GET /configs + systemd timer 实际状态
type Live struct {
	cfg     map[string]any    // /configs 原文
	timers  map[string]string // timer 键名 → 运行值
	active  map[string]bool   // 段是否被 CLI 管理 (dns/tun 只在使用过才算)
	running bool              // 服务是否在运行
}

// Fetch 读取运行态; 服务未运行时所有值都视为 "-"
func Fetch(s *app.Settings) *Live {
	l := &Live{timers: map[string]string{}, running: sysd.IsActive(), active: map[string]bool{}}
	// CLI 未接管过的段(tun/dns)没有运行态概念: 内核/订阅自带的默认值不算差异
	for _, sec := range []string{"dns", "tun"} {
		l.active[sec] = SectionUsed(s, sec)
	}
	if !l.running {
		return l
	}
	var m map[string]any
	if err := api.New(s).GetJSON("/configs", &m); err == nil {
		l.cfg = m
	}
	l.timers["sub-auto-update-enabled"] = enabledStr(sysd.TimerEnabled("mihomo-cli-sub.timer"))
	l.timers["sub-auto-update-interval"] = sysd.TimerInterval("mihomo-cli-sub.timer")
	l.timers["node-auto-select-enabled"] = enabledStr(sysd.TimerEnabled("mihomo-cli-auto.timer"))
	l.timers["node-auto-select-interval"] = sysd.TimerInterval("mihomo-cli-auto.timer")
	l.timers["resource-auto-update-enabled"] = enabledStr(sysd.TimerEnabled("mihomo-cli-resource.timer"))
	l.timers["resource-auto-update-interval"] = sysd.TimerInterval("mihomo-cli-resource.timer")
	return l
}

func enabledStr(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

// Running 服务是否在运行
func (l *Live) Running() bool { return l != nil && l.running }

// YAMLPath 注册键名 → 内核 yaml/API 里的键名 (点号路径逐段映射)
func YAMLPath(name string) string {
	if i := strings.Index(name, "."); i > 0 {
		return YAMLPath(name[:i]) + "." + name[i+1:]
	}
	switch name {
	case "proxy-mode":
		return "mode"
	case "ipv6-enabled":
		return "ipv6"
	case "http-port":
		return "port" // 内核里叫 port, 不叫 http-port
	}
	return name
}

// Value 某键的运行态值; "-" = 该项没有运行态概念(或服务未运行)
func (l *Live) Value(k Key) string {
	if !l.Running() {
		return "-"
	}
	switch k.Section {
	case "cli":
		return "-"
	case "dns", "tun":
		// 段未被 CLI 接管时不与运行态比较 (否则订阅/内核默认值会被误报成"不同")
		if !l.active[k.Section] {
			return "-"
		}
	case "timer":
		if v, ok := l.timers[k.Name]; ok {
			return v
		}
		return "-"
	}
	if l.cfg == nil {
		return "-"
	}
	// 点号路径: 逐层下钻
	parts := strings.Split(YAMLPath(k.Name), ".")
	var cur any = l.cfg
	for i, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return "-"
		}
		v, exists := m[p]
		if !exists {
			return "-"
		}
		if i == len(parts)-1 {
			return normRun(k, v)
		}
		cur = v
	}
	return "-"
}

// normRun 把 /configs 的值归一化成与 SETTING 同形的字符串
func normRun(k Key, v any) string {
	switch k.Kind {
	case KindPort:
		if n, ok := toInt(v); ok {
			if n == 0 {
				return "off"
			}
			return strconv.FormatInt(n, 10)
		}
	case KindBool:
		if b, ok := v.(bool); ok {
			return boolStr(b)
		}
	case KindInt:
		if n, ok := toInt(v); ok {
			return strconv.FormatInt(n, 10)
		}
		return "-"
	case KindDur:
		if n, ok := toInt(v); ok && n > 0 {
			return DurHuman(time.Duration(n) * time.Second)
		}
		return "-"
	case KindList, KindNameList, KindCIDRList:
		return scalarStr(v)
	}
	return scalarStr(v)
}

func toInt(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	}
	return 0, false
}

// Same 运行态与 SETTING 是否一致
func (l *Live) Same(k Key, setting string) bool {
	run := l.Value(k)
	if run == "-" {
		return true // 无运行态概念, 不算差异
	}
	return normCompare(k, run) == normCompare(k, setting)
}

// normCompare 归一化后比较 (端口 off/sub 视为等同; 周期 24h 与 24h0m0s 视为等同;
// timer 开关 enabled/disabled 与 true/false 视为等同)
func normCompare(k Key, v string) string {
	if k.Section == "timer" && strings.HasSuffix(k.Name, "-enabled") {
		switch v {
		case "enabled", "true":
			return "true"
		case "disabled", "false":
			return "false"
		}
	}
	switch k.Kind {
	case KindPort, KindTri, KindEnum:
		if v == "sub" || v == "" {
			return "sub"
		}
		if k.Kind == KindPort && (v == "off" || v == "0") {
			return "off"
		}
		return v
	case KindDur:
		d, err := time.ParseDuration(v)
		if err != nil {
			return v
		}
		return DurHuman(d)
	}
	return v
}

// HotPatchable 该键能否用 PATCH /configs 热切换(不 reload)
func HotPatchable(k Key) bool {
	return k.Name == "proxy-mode" || k.Name == "log-level"
}

// Pull 从运行态读回一个键的值 (config update-file 用); err = 该键不可回写
func (l *Live) Pull(k Key) (string, error) {
	if !l.Running() {
		return "", fmt.Errorf("%s", i18n.T("服务未运行"))
	}
	if k.Section == "timer" {
		var unit string
		switch {
		case strings.HasPrefix(k.Name, "sub-auto-update"):
			unit = "mihomo-cli-sub.timer"
		case strings.HasPrefix(k.Name, "node-auto-select"):
			unit = "mihomo-cli-auto.timer"
		case strings.HasPrefix(k.Name, "resource-auto-update"):
			unit = "mihomo-cli-resource.timer"
		}
		if strings.HasSuffix(k.Name, "-enabled") {
			return enabledStr(sysd.TimerEnabled(unit)), nil
		}
		return sysd.TimerInterval(unit), nil
	}
	v := l.Value(k)
	if v == "-" || v == "" {
		if l.cfg == nil {
			return "", fmt.Errorf("%s", i18n.T("内核 API 未就绪, 稍后重试"))
		}
		return "", fmt.Errorf("%s: %s", k.Name, i18n.T("内核未返回该值, 不可回写"))
	}
	return v, nil
}

// PushTimer 定时任务侧: 用 config.toml 的设置重写并启停 timer
func PushTimers(s *app.Settings) error {
	return sysd.InstallTimers(s)
}

// CoreVersionFromBinary 从已安装内核二进制读版本号 (v1.19.31)
func CoreVersionFromBinary(bin string) string {
	out, err := runQuiet(bin, "-v")
	if err != nil {
		return ""
	}
	if i := strings.Index(out, "v1."); i >= 0 {
		f := strings.Fields(out[i:])
		if len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

func runQuiet(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}
