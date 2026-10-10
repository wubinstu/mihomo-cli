package subs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

// Quota 订阅用量信息, 来源是订阅响应的 subscription-userinfo 头:
//
//	upload=0; download=0; total=171798691840; expire=1777732477
//
// 三个量都是字节, expire 是 unix 秒。绝大多数机场都发这个头, 但字段可缺可坏,
// 所以每个字段都要能"没有", 解析失败就整体当没有 (绝不让一个畸形头把列表打挂)。
type Quota struct {
	Upload   int64 // 已上传 (字节)
	Download int64 // 已下载 (字节)
	Total    int64 // 总量 (字节); <=0 表示不限量
	Expire   int64 // 到期 unix 秒; <=0 表示长期有效
}

// ParseQuota 解析 subscription-userinfo 头; 空/畸形返回 nil
func ParseQuota(header string) *Quota {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	q := &Quota{}
	seen := false
	for _, part := range strings.Split(header, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue // 单个字段坏掉就跳过, 不影响其它字段
		}
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "upload":
			q.Upload, seen = n, true
		case "download":
			q.Download, seen = n, true
		case "total":
			q.Total, seen = n, true
		case "expire":
			q.Expire, seen = n, true
		}
	}
	if !seen {
		return nil
	}
	return q
}

// Used 已用量 (上传+下载)
func (q *Quota) Used() int64 { return q.Upload + q.Download }

// Percent 已用百分比; 不限量返回 -1
func (q *Quota) Percent() float64 {
	if q.Total <= 0 {
		return -1
	}
	return float64(q.Used()) / float64(q.Total) * 100
}

// ExpireTime 到期时间; 长期有效返回零值
func (q *Quota) ExpireTime() time.Time {
	if q.Expire <= 0 {
		return time.Time{}
	}
	return time.Unix(q.Expire, 0)
}

// DaysLeft 距到期天数 (向下取整); 长期有效返回 -1
func (q *Quota) DaysLeft() int {
	if q.Expire <= 0 {
		return -1
	}
	d := time.Until(q.ExpireTime()).Hours() / 24
	if d < 0 {
		return -2 // 已过期
	}
	return int(d)
}

// State 用量状态: ok / low / out / expired (用于上色)
func (q *Quota) State() string {
	switch {
	case q.DaysLeft() == -2:
		return "expired"
	case q.Total > 0 && q.Percent() >= 100:
		return "out"
	case q.Total > 0 && q.Percent() >= 80:
		return "low"
	}
	return "ok"
}

// Short 一行紧凑摘要, 例如 "1.2GB/160GB · 30天后到期"
func (q *Quota) Short() string {
	var b strings.Builder
	if q.Total > 0 {
		fmt.Fprintf(&b, "%s/%s", bytesHuman(q.Used()), bytesHuman(q.Total))
		if p := q.Percent(); p >= 0 {
			fmt.Fprintf(&b, " (%.2f%%)", p)
		}
	} else if q.Used() > 0 {
		b.WriteString(bytesHuman(q.Used()))
	}
	sep := func() {
		if b.Len() > 0 {
			b.WriteString(" · ")
		}
	}
	switch q.DaysLeft() {
	case -1:
		sep()
		b.WriteString(i18n.T("长期有效"))
	case -2:
		sep()
		fmt.Fprintf(&b, i18n.T("已过期 %s"), q.ExpireTime().Format("2006-01-02"))
	default:
		sep()
		fmt.Fprintf(&b, i18n.T("%d 天后到期 (%s)"), q.DaysLeft(), q.ExpireTime().Format("2006-01-02"))
	}
	if b.Len() == 0 {
		return i18n.T("不限量")
	}
	return b.String()
}

// bytesHuman 字节数转可读 (二进制单位, 和机场后台一致)
func bytesHuman(n int64) string {
	if n < 0 {
		return "-"
	}
	const unit = 1024.0
	f := float64(n)
	for _, s := range []string{"B", "KB", "MB", "GB", "TB", "PB"} {
		if f < unit || s == "PB" {
			if s == "B" {
				return fmt.Sprintf("%d%s", int64(f), s)
			}
			return fmt.Sprintf("%.2f%s", f, s)
		}
		f /= unit
	}
	return fmt.Sprintf("%dB", n)
}
