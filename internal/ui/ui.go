// Package ui 提供 CJK(中文/emoji)宽度感知的表格输出, 解决 tabwriter 中英文不对齐问题。
package ui

import (
	"fmt"
	"io"
	"strings"
)

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

// Colorpad 带颜色的延迟单元格 (考虑 ANSI 码不计宽度)
func ColorDelay(ms int) string {
	if ms <= 0 {
		return DelayColor(ms) + "timeout" + ColorReset
	}
	return DelayColor(ms) + fmt.Sprintf("%d ms", ms) + ColorReset
}

func pad(s string, w int) string {
	d := w - Width(s)
	if d < 1 {
		d = 1
	}
	return s + strings.Repeat(" ", d)
}

// Table 按显示宽度对齐打印表格 (rows 含表头; gap 为列间最少空格)
func Table(w io.Writer, rows [][]string, gap int) {
	if gap < 2 {
		gap = 2
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if w := Width(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for ri, r := range rows {
		var b strings.Builder
		for i := 0; i < len(r); i++ {
			if i == len(r)-1 {
				b.WriteString(r[i])
				break
			}
			b.WriteString(pad(r[i], widths[i]+gap-1))
		}
		fmt.Fprintln(w, b.String())
		if ri == 0 {
			// 表头下分隔线
			var line strings.Builder
			for i := 0; i < cols; i++ {
				if i > 0 {
					line.WriteString(strings.Repeat(" ", gap-1))
				}
				line.WriteString(strings.Repeat("-", widths[i]))
			}
			fmt.Fprintln(w, line.String())
		}
	}
}
