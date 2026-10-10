package ui

import (
	"bytes"
	"strings"
	"testing"
)

// dashStarts 分隔线里每一组 "-" 的起始显示宽度
func dashStarts(line string) []int {
	var out []int
	w, inDash := 0, false
	for _, r := range line {
		if r == '-' {
			if !inDash {
				inDash = true
				out = append(out, w)
			}
			w++
			continue
		}
		inDash = false
		w += Width(string(r))
	}
	return out
}

// cellStarts 数据/表头行里每一列的起始显示宽度 (2 个以上空格才算列边界)
func cellStarts(line string) []int {
	var out []int
	w, spaces, started := 0, 0, false
	for _, r := range line {
		if r == ' ' {
			spaces++
			w++
			continue
		}
		if !started || spaces >= 2 {
			out = append(out, w)
		}
		started = true
		spaces = 0
		w += Width(string(r))
	}
	return out
}

func TestSeparatorAlignsWithHeader(t *testing.T) {
	var buf bytes.Buffer
	TableSections(&buf, []string{"KEY", "RUNNING", "SETTING", "DEFAULT", "STATUS"}, []Section{
		{Title: "[core]", Rows: [][]string{{"core.allow-lan", "true", "true", "false", "✔"}}},
		{Title: "[cli]", Rows: [][]string{{"cli.current-group", "-", "🚀 节点选择", "/", "-"}}},
	}, 2)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	var hdr, sep string
	for _, l := range lines {
		if strings.HasPrefix(l, "KEY") {
			hdr = l
		}
		if strings.HasPrefix(l, "---") {
			sep = l
			break
		}
	}
	hs, ss := cellStarts(hdr), dashStarts(sep)
	t.Logf("header starts=%v", hs)
	t.Logf("sep    starts=%v", ss)
	if len(hs) != len(ss) {
		t.Fatalf("column count differs: %v vs %v", hs, ss)
	}
	for i := range hs {
		if hs[i] != ss[i] {
			t.Errorf("column %d starts at %d (header) but %d (separator) — off by %d", i, hs[i], ss[i], hs[i]-ss[i])
		}
	}
}

// TestTableSeparatorAlignsWithHeader ui.Table 的分隔线也必须和表头逐列对齐
// (ping/status 用它; v1.4.2 发现它和 TableSections 有同一个 gap 漏算)
func TestTableSeparatorAlignsWithHeader(t *testing.T) {
	rows := [][]string{
		{"SITE", "HTTP", "DELAY", "STATUS"},
		{"GitHub", "200", "915 ms", "available"},
		{"哔哩哔哩大陆", "200", "386 ms", "available"},
		{"Docker Hub", "401", "1485 ms", "available"},
	}
	var buf bytes.Buffer
	Table(&buf, rows, 2)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header+separator+rows, got %d lines", len(lines))
	}
	hs := cellStarts(lines[0])
	sep := lines[1]
	// 分隔线现在是连续长划线, 没法按"划线组"数列; 改成验证每一列的开头位置都有划线
	for i, start := range hs {
		if start >= len(sep) || sep[start] != '-' {
			t.Errorf("column %d starts at %d but the separator has %q there", i, start,
				string(runeAt(sep, start)))
		}
	}
	// 分隔线总长 = 最后一列起点 + 该列内容宽度 (表头最后列之后可能有尾空格)
	want := hs[len(hs)-1] + Width(strings.TrimRight(lines[0], " ")[hs[len(hs)-1]:])
	if len(strings.TrimRight(sep, "-")) != 0 {
		t.Errorf("separator has non-dash chars: %q", sep)
	}
	if len(sep) < want {
		t.Errorf("separator is %d chars, want at least %d (last column start+width)", len(sep), want)
	}
}

func runeAt(s string, i int) rune {
	for _, r := range s {
		if i <= 0 {
			return r
		}
		i--
	}
	return ' '
}
