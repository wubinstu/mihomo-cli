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
