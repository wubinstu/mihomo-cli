// Package ui 提供 CJK(中文/emoji)宽度感知的表格输出, 解决 tabwriter 中英文不对齐问题。
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ColorEnabled 是否输出 ANSI 颜色: 非 TTY 或设置 NO_COLOR 时关闭
var ColorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}()

// Width 计算字符串的终端显示宽度 (CJK/全角=2, 变体选择符=0, ANSI 转义序列不计)
func Width(s string) int {
	w := 0
	skip := false
	for _, r := range s {
		if r == 0x1b { // ESC: 进入 ANSI 序列, 直到字母结束
			skip = true
			continue
		}
		if skip {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				skip = false
			}
			continue
		}
		switch {
		case r == 0xFE0F || r == 0x200D: // emoji 变体选择符/零宽连接符
			continue
		case r >= 0x1100 && r <= 0x115F,
			r == 0x231A, r == 0x231B, r == 0x2329, r == 0x232A,
			r >= 0x2E80 && r <= 0x303E,
			r >= 0x3041 && r <= 0x33FF,
			r >= 0x3400 && r <= 0x4DBF,
			r >= 0x4E00 && r <= 0x9FFF,
			r >= 0xA000 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE30 && r <= 0xFE4F,
			r >= 0xFF00 && r <= 0xFF60,
			r >= 0xFFE0 && r <= 0xFFE6,
			r >= 0x1F300 && r <= 0x1FAFF,
			r >= 0x20000 && r <= 0x3FFFD:
			w += 2
		default:
			w++
		}
	}
	return w
}

// DelayColor 按延迟返回 ANSI 颜色码: <200 绿, 200-500 蓝, 500-3000 黄, >3000 红, <=0 灰(超时)
func DelayColor(ms int) string {
	if !ColorEnabled {
		return ""
	}
	switch {
	case ms <= 0:
		return "\x1b[90m" // 灰
	case ms < 200:
		return "\x1b[32m" // 绿
	case ms < 500:
		return "\x1b[34m" // 蓝
	case ms < 3000:
		return "\x1b[33m" // 黄
	default:
		return "\x1b[31m" // 红
	}
}

const ColorReset = "\x1b[0m"

// Paint 给文本上色 (颜色关闭时原样返回)
func Paint(color, s string) string {
	if !ColorEnabled || color == "" {
		return s
	}
	return color + s + ColorReset
}

// ColorDelay 带颜色的延迟单元格 (考虑 ANSI 码不计宽度)
func ColorDelay(ms int) string {
	if ms <= 0 {
		return Paint(DelayColor(ms), "timeout")
	}
	return Paint(DelayColor(ms), fmt.Sprintf("%d ms", ms))
}

// OK / ErrMark 体检标记: 绿勾 / 红叉 (T(颜色)关闭时为 ✔ / ✘)
func OKMark() string  { return Paint("\x1b[32m", "✔") }
func ErrMark() string { return Paint("\x1b[31m", "✘") }

func pad(s string, w int) string {
	d := w - Width(s)
	if d < 1 {
		d = 1
	}
	return s + strings.Repeat(" ", d)
}

// cols 统计并集行宽
func cols(rows [][]string) []int {
	n := 0
	for _, r := range rows {
		if len(r) > n {
			n = len(r)
		}
	}
	w := make([]int, n)
	for _, r := range rows {
		for i, c := range r {
			if x := Width(c); x > w[i] {
				w[i] = x
			}
		}
	}
	return w
}

// Table 按显示宽度对齐打印表格 (rows 含表头; gap 为列间最少空格)
func Table(w io.Writer, rows [][]string, gap int) {
	if gap < 2 {
		gap = 2
	}
	widths := cols(rows)
	for ri, r := range rows {
		var b strings.Builder
		for i := 0; i < len(r); i++ {
			if i == len(r)-1 {
				b.WriteString(r[i])
				break
			}
			b.WriteString(pad(r[i], widths[i]+gap)) // 至少 gap 个空格, 永不挤在一起
		}
		fmt.Fprintln(w, b.String())
		if ri == 0 {
			// 表头下分隔线
			var line strings.Builder
			for i := 0; i < len(widths); i++ {
				if i > 0 {
					line.WriteString(strings.Repeat(" ", gap))
				}
				line.WriteString(strings.Repeat("-", widths[i]))
			}
			fmt.Fprintln(w, line.String())
		}
	}
}

// Section 一组同类行 (标题 + 数据行), 用于把多段合成一张列宽对齐的表
type Section struct {
	Title string     // 段标题, 空则不打印
	Rows  [][]string // 数据行 (不含表头)
}

// TableSections 把多个 Section 打印成"一张表": 列宽在所有段之间统一计算,
// 因此跨段也严格对齐 (status/doctor/config get 三处共用, 不可能再出现各写各的宽度)
func TableSections(w io.Writer, header []string, secs []Section, gap int) {
	if gap < 2 {
		gap = 2
	}
	// 全局列宽
	all := append([][]string{header}, rowsOf(secs)...)
	widths := cols(all)
	line := func() {
		var b strings.Builder
		for i := range widths {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap-1))
			}
			b.WriteString(strings.Repeat("-", widths[i]))
		}
		fmt.Fprintln(w, b.String())
	}
	first := true
	for _, s := range secs {
		if len(s.Rows) == 0 {
			continue
		}
		if !first {
			fmt.Fprintln(w)
		}
		if s.Title != "" {
			fmt.Fprintln(w, s.Title)
		}
		if first {
			printRow(w, header, widths, gap)
			line()
		}
		for _, r := range s.Rows {
			printRow(w, r, widths, gap)
		}
		first = false
	}
}

func rowsOf(secs []Section) [][]string {
	var out [][]string
	for _, s := range secs {
		out = append(out, s.Rows...)
	}
	return out
}

func printRow(w io.Writer, r []string, widths []int, gap int) {
	var b strings.Builder
	for i := 0; i < len(r) && i < len(widths); i++ {
		if i == len(r)-1 || i == len(widths)-1 {
			b.WriteString(r[i])
			break
		}
		b.WriteString(pad(r[i], widths[i]+gap)) // 至少 gap 个空格
	}
	fmt.Fprintln(w, b.String())
}
