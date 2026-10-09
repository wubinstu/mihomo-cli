package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

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
