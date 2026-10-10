package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// captureHelp 捕获 configKeyHelp 的输出
func captureHelp(t *testing.T, name string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	err = configKeyHelp(name)
	w.Close()
	os.Stdout = saved
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// 每个注册键的详版帮助都必须能渲染出来, 且包含全部小节
// (固定用 en, 标签才是稳定的字面量; 顺便验证 en 翻译齐全)
func TestDetailedHelpRendersForEveryKey(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	for _, k := range cfg.Keys {
		var buf bytes.Buffer
		old := stdoutWriter()
		_ = old
		out := captureHelp(t, k.Name)
		if out == "" {
			t.Errorf("%s: detailed help is empty", k.Name)
			continue
		}
		for _, want := range []string{"DESCRIPTION", "CURRENT", "DEFAULT", "YAML KEY", "HOW IT APPLIES", "USAGE", "SHOW"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: detailed help missing %q section", k.Name, want)
			}
		}
		// 取值信息: 要么有逐值说明, 要么有可选值/期望
		if !strings.Contains(out, "VALUES") && !strings.Contains(out, "ALLOWED VALUES") && !strings.Contains(out, "EXPECTED") {
			t.Errorf("%s: detailed help has no value information", k.Name)
		}
		_ = buf
	}
}

// headOf 简版帮助里的 "键 <取值>" 部分
func headOf(k cfg.Key) string {
	h := k.Name
	if spec := k.Spec(); spec != "" {
		h += " <" + spec + ">"
	}
	return h
}

// 简版帮助: 每个键一行的 "键 <取值>  说明  [默认 x]", 说明列必须对齐。
// 直接按 ui.Pad 的规则重建每一行再比对, 比"猜列边界"可靠。
func TestBriefHelpAligned(t *testing.T) {
	i18n.Set("zh")
	defer i18n.Set("en")
	secs := []string{"core", "cli", "timer", "dns", "tun"}
	out := buildSetHelp()
	for _, sec := range secs {
		// 每个段独立算列宽 (简版帮助是一段一块)
		w := 0
		for _, k := range cfg.KeysOf(sec) {
			h := headOf(k)
			if x := ui.Width(h); x > w {
				w = x
			}
		}
		for _, k := range cfg.KeysOf(sec) {
			h := headOf(k)
			desc := k.Desc()
			if k.Def != "" {
				desc += "  [" + T("默认") + " " + k.Def + "]"
			}
			want := ui.Pad(h, w+2) + desc
			if !strings.Contains(out, want) {
				t.Errorf("%s: brief help line not found/aligned:\n  want %q", k.Name, want)
			}
		}
	}
}

// allCmds 整棵命令树 (含 root)
func allCmds() []*cobra.Command {
	var out []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		out = append(out, c)
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(rootCmd)
	return out
}

// renderHelp 渲染某个命令的帮助 (含 cobra 用法骨架)
func renderHelp(c *cobra.Command) string {
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.HelpFunc()(c, nil)
	return buf.String()
}

// TestZhHelpHasNoEnglish 中文模式下帮助不许漏英文句子。
// 用户 1.4.2 反馈"中文模式下很多帮助信息显示的不正常" —— 根因是语言探测读了旧键名,
// 外加 cobra 自带的骨架没本地化。这里把整棵树渲染一遍盯住回归。
func TestZhHelpHasNoEnglish(t *testing.T) {
	i18n.Set("zh")
	defer i18n.Set("zh")
	applyLanguage(rootCmd) // 命令树文案按当前语言重翻
	localizeCobra(rootCmd) // 用法骨架也按当前语言重建
	for _, c := range allCmds() {
		for _, line := range strings.Split(renderHelp(c), "\n") {
			checkNoEnglish(t, c.Name(), line)
		}
	}
}

// checkNoEnglish 一行里不该出现"纯英文句子"; 命令路径/标志/取值清单本来就该是英文
func checkNoEnglish(t *testing.T, cmd, line string) {
	t.Helper()
	s := strings.TrimSpace(line)
	if s == "" || cjkRe.MatchString(s) {
		return
	}
	switch {
	case strings.HasPrefix(s, "mihomo-cli"), strings.HasPrefix(s, "eval "),
		strings.HasPrefix(s, "sudo "), strings.HasPrefix(s, "curl "),
		strings.HasPrefix(s, "-"), strings.HasPrefix(s, "#"),
		strings.HasPrefix(s, "Usage:"), strings.HasPrefix(s, "Available Commands:"),
		strings.HasPrefix(s, "Flags:"), strings.HasPrefix(s, "Global Flags:"),
		strings.HasPrefix(s, "Aliases:"), strings.HasPrefix(s, "Examples:"),
		strings.HasPrefix(s, "Additional"), strings.HasPrefix(s, "Use \""):
		return
	}
	if !englishSentence(s) {
		return
	}
	t.Errorf("zh mode: %q help leaks English: %q", cmd, s)
}

// englishSentence 像一句英文文案 (多个英文单词, 且不是"键 说明"这种表格行)
func englishSentence(s string) bool {
	if strings.Contains(s, "=") || strings.Contains(s, "|") {
		return false
	}
	return len(regexp.MustCompile(`[A-Za-z]{2,}`).FindAllString(s, -1)) >= 3
}

var cjkRe = regexp.MustCompile(`[\x{4e00}-\x{9fff}]`)

// TestEnHelpHasNoChinese 英文模式下帮助不许漏中文 (用户反复要求的硬约束)
func TestEnHelpHasNoChinese(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	applyLanguage(rootCmd)
	localizeCobra(rootCmd)
	for _, c := range allCmds() {
		for _, line := range strings.Split(renderHelp(c), "\n") {
			if cjkRe.MatchString(line) {
				t.Errorf("en mode: %q help leaks Chinese: %q", c.Name(), strings.TrimSpace(line))
			}
		}
	}
}

// TestDetailedHelpTranslatesBothWays 每个键的详版帮助, 中英两边都不能串语言
func TestDetailedHelpTranslatesBothWays(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		i18n.Set(lang)
		for _, k := range cfg.Keys {
			out := captureHelp(t, k.Name)
			zh := lang == "zh"
			for _, line := range strings.Split(out, "\n") {
				s := strings.TrimSpace(line)
				// 首行是 "[段] 键名"; CURRENT 行是用户自己的数据 (分组名/节点名), 不算界面文案
				if strings.HasPrefix(s, "[") && strings.Contains(s, "] "+k.Name) {
					continue
				}
				if strings.HasPrefix(s, "CURRENT") || strings.HasPrefix(s, "当前值") {
					continue
				}
				// 取值表里带 IP/端口的行 (DNS 预设表), 专有名词中英同名
				if regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`).MatchString(s) {
					continue
				}
				hasCJK := cjkRe.MatchString(line)
				if zh && !hasCJK && englishSentence(s) {
					t.Errorf("%s mode: %s leaks English: %q", lang, k.Name, s)
				}
				if !zh && hasCJK {
					t.Errorf("en mode: %s leaks Chinese: %q", k.Name, s)
				}
			}
		}
	}
	i18n.Set("zh")
}

// TestSectionVisibilityFollowsEnable dns/tun 段默认露不露面, 只看总开关 (1.4.2 用户反馈:
// "改任意一个字段后整段都冒出来"不合理)
func TestSectionVisibilityFollowsEnable(t *testing.T) {
	i18n.Set("zh")
	defer i18n.Set("zh")
	s := app.DefaultSettings()
	if got := cfg.ShownSections(s); len(got) != 0 {
		t.Errorf("nothing configured: shown sections = %v, want none", got)
	}
	// 只设一个无关键: 段仍然不露面
	if err := cfg.SetGeneric(s, "core.dns.ipv6", "false"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ShownSections(s); len(got) != 0 {
		t.Errorf("dns.ipv6 set but dns.enable off: shown = %v, want none", got)
	}
	if got := cfg.HiddenSections(s); len(got) != 1 || got[0] != "core.dns" {
		t.Errorf("hidden = %v, want [core.dns]", got)
	}
	// 打开总开关才露面
	k := cfg.Lookup("core.dns.enable")
	if k == nil {
		t.Fatal("dns.enable not registered")
	}
	if err := k.Set(s, "true"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ShownSections(s); len(got) != 1 || got[0] != "core.dns" {
		t.Errorf("dns.enable on: shown = %v, want [core.dns]", got)
	}
}

// TestTunStackKnowsMips 内核 v1.19 新增 mips 协议栈, 值域和默认值必须跟上
func TestTunStackKnowsMips(t *testing.T) {
	k := cfg.Lookup("core.tun.stack")
	if k == nil {
		t.Fatal("tun.stack not registered")
	}
	if !containsStr(k.Enum, "mips") {
		t.Errorf("tun.stack enum = %v, want mips", k.Enum)
	}
	if k.Def != "mips" {
		t.Errorf("tun.stack default = %q, want mips", k.Def)
	}
}

// TestEnumRunningValueCaseInsensitive 内核回显大小写不统一 (Mips/Mixed/gVisor),
// RUNNING 列必须归一到我们的拼法, 否则和 SETTING 对不上
func TestEnumRunningValueCaseInsensitive(t *testing.T) {
	k := cfg.Lookup("core.tun.stack")
	if k == nil {
		t.Fatal("tun.stack not registered")
	}
	for _, raw := range []string{"Mips", "Mixed", "gVisor", "System"} {
		if got := cfg.NormRunForTest(*k, raw); got != strings.ToLower(raw) && got != "gvisor" {
			t.Errorf("NormRun(%q) = %q, want the canonical lowercase spelling", raw, got)
		}
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// sandbox 把 app 的全部路径指到临时目录 (测试不碰 /etc/mihomo-cli)
func sandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	app.BaseDir = dir
	app.BinDir = filepath.Join(dir, "bin")
	app.ProfileDir = filepath.Join(dir, "profiles")
	app.RuntimeDir = filepath.Join(dir, "runtime")
	app.VersionsDir = filepath.Join(dir, "bin", "versions")
	app.LogDir = filepath.Join(dir, "logs")
	app.CoreBin = filepath.Join(app.BinDir, "mihomo")
	app.CoreBinOld = filepath.Join(app.BinDir, "mihomo.old")
	app.SettingsFile = filepath.Join(dir, "config.toml")
	app.RuntimeConfig = filepath.Join(app.RuntimeDir, "config.yaml")
	app.LogFile = filepath.Join(app.LogDir, "mihomo.log")
	if err := os.MkdirAll(app.ProfileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestConfigFileMatchesConfigGet 托管键在 config.toml 里必须写**有效值**。
// v1.5.0 用户反馈: 文件里 log_level = "" 而 config get 显示 info, 两边对不上。
// 规则: core/cli/timer 的键一律落有效值; core.dns/core.tun 只在设过时才出现 (未托管)。
func TestConfigFileMatchesConfigGet(t *testing.T) {
	sandbox(t)
	s := app.DefaultSettings()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(app.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`log_level = ""`)) {
		t.Error("log_level must be written as its effective value, not an empty string")
	}
	back, err := app.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range cfg.Keys {
		if !k.Managed() {
			continue
		}
		want := k.Effective(s)
		got := k.Get(back)
		if got == "" && want != "" {
			continue // 空值键 (如 current-profile) 不落盘
		}
		if got != want {
			t.Errorf("%s: file has %q, config get shows %q", k.Name, got, want)
		}
	}
}

// TestCorePrefixMirrorsYAML core.<段>.<键> 必须和内核 yaml 的路径一致 (v1.5.0 统一命名)。
// 例外只有三个"为了好记起的名", 内核原名都太含糊 (port/mode/ipv6):
var yamlAliases = map[string]string{
	"core.http-port":    "port",
	"core.proxy-mode":   "mode",
	"core.ipv6-enabled": "ipv6",
}

func TestCorePrefixMirrorsYAML(t *testing.T) {
	for _, k := range cfg.Keys {
		if !strings.HasPrefix(k.Name, "core.") {
			continue
		}
		want := strings.TrimPrefix(k.Name, "core.")
		if a, ok := yamlAliases[k.Name]; ok {
			want = a
		}
		if got := cfg.YAMLPath(k.Name); got != want {
			t.Errorf("%s → yaml %q, want %q", k.Name, got, want)
		}
	}
	// dns/tun 的段名就是 yaml 段名, 不该再出现裸 dns.*/tun.* 键
	for _, k := range cfg.Keys {
		if strings.HasPrefix(k.Name, "dns.") || strings.HasPrefix(k.Name, "tun.") {
			t.Errorf("key %q must carry the core. prefix", k.Name)
		}
	}
}
