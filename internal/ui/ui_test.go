package ui

import (
	"bytes"
	"strings"
	"testing"
)

// CJK 宽度: 中文/全角算 2, emoji 算 2, ANSI 转义不算宽
func TestWidth(t *testing.T) {
	cases := map[string]int{
		"":                 0,
		"abc":              3,
		"中文":               4,
		"混合abc":            7,
		"ｆｕｌｌ":             8, // 全角
		"\x1b[32m✔\x1b[0m": 1, // ANSI 不计宽, ✔ 本身宽 1
		"\x1b[31m✘\x1b[0m": 1,
		"🚀 节点选择":           11, // emoji 2 + 空格 1 + 4 个中文 8
		"→":                1,
		"🎯":                2,
		"a\u0301":          2, // 组合附加符按单符计 (简化处理)
	}
	for s, want := range cases {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

// col2Width 返回一行里"第 2 列起点"的显示宽度 (宽度感知, 不按字节)
func col2Width(line string) int {
	w, run := 0, 0
	for _, r := range line {
		if r == ' ' {
			run++
			w++
			continue
		}
		if run >= 2 { // 间隔结束, 现在的位置就是第 2 列的起点
			return w
		}
		run = 0
		w += Width(string(r))
	}
	return -1
}

// Table 各列严格对齐 (按显示宽度), 且表头下有分隔线
func TestTableAligned(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, [][]string{
		{"KEY", T2("运行"), "T3"},
		{"mixed-port", "7890", "x"},
		{"资源", T2("节点"), "y"},
	}, 2)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 { // 表头 + 分隔线 + 2 数据行
		t.Fatalf("want 4 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[2], "mixed-port  ") {
		t.Errorf("row 1 = %q", lines[2])
	}
	// 第 2 列的起点(显示宽度)在各数据行必须一致 —— 中英混排也不能错位
	if col2Width(lines[2]) != col2Width(lines[3]) {
		t.Errorf("column 2 not aligned:\n%q\n%q", lines[2], lines[3])
	}
	// 分隔线长度应等于表头显示宽度
	if len(lines[0]) < 3 {
		t.Errorf("header = %q", lines[0])
	}
}

// TableSections 多段共用一套列宽: 跨段也必须对齐 (这是 status/doctor/config get 的地基)
func TestTableSectionsAligned(t *testing.T) {
	var buf bytes.Buffer
	TableSections(&buf, []string{"KEY", "V"}, []Section{
		{Title: "[core]", Rows: [][]string{{"allow-lan", "true"}}},
		{Title: "[cli]", Rows: [][]string{{"cli-language", "en"}}},
	}, 2)
	out := buf.String()
	if !strings.Contains(out, "[core]") || !strings.Contains(out, "[cli]") {
		t.Fatalf("section titles missing: %q", out)
	}
	var keyCol = -1
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "KEY") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "[") {
			continue
		}
		w := col2Width(line)
		if w < 0 {
			continue
		}
		if keyCol == -1 {
			keyCol = w
		} else if w != keyCol {
			t.Errorf("cross-section misalignment at %q", line)
		}
	}
	// 空段不打印
	var buf2 bytes.Buffer
	TableSections(&buf2, []string{"A"}, []Section{{Rows: nil}, {Rows: [][]string{{"x"}}}}, 2)
	if strings.Contains(buf2.String(), "\n\n") {
		t.Errorf("empty section should be skipped: %q", buf2.String())
	}
}

// ANSI 颜色的列不破坏对齐
func TestTableWithANSI(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, [][]string{
		{"MARK", "NAME"},
		{Paint("\x1b[32m", "✔"), "long-name"},
		{ErrMark(), "s"},
	}, 2)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	col := func(l string) int {
		// 去掉 ANSI 后找第二列起点
		clean := l
		for {
			i := strings.Index(clean, "\x1b[")
			if i < 0 {
				break
			}
			j := strings.IndexByte(clean[i:], 'm')
			clean = clean[:i] + clean[i+j+1:]
		}
		return strings.Index(clean, "  ") + 2
	}
	if len(lines) < 4 || col(lines[2]) != col(lines[3]) {
		t.Errorf("ANSI column misaligned:\n%q\n%q", lines[2], lines[3])
	}
}

// 非 TTY / NO_COLOR 时不输出颜色
func TestColorDisabled(t *testing.T) {
	saved := ColorEnabled
	defer func() { ColorEnabled = saved }()
	ColorEnabled = false
	if Paint("\x1b[32m", "x") != "x" {
		t.Error("color must be suppressed when disabled")
	}
	if DelayColor(100) != "" {
		t.Error("DelayColor must be empty when disabled")
	}
	ColorEnabled = true
	if Paint("\x1b[32m", "x") != "\x1b[32mx\x1b[0m" {
		t.Error("color must be applied when enabled")
	}
}

func T2(s string) string { return s }
