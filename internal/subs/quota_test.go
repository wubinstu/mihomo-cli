package subs

import "testing"

func TestParseQuota(t *testing.T) {
	q := ParseQuota("upload=0; download=0; total=171798691840; expire=1777732477")
	if q == nil {
		t.Fatal("valid header must parse")
	}
	if q.Total != 171798691840 || q.Expire != 1777732477 {
		t.Errorf("total/expire = %d/%d", q.Total, q.Expire)
	}
	if q.Used() != 0 || q.Percent() != 0 {
		t.Errorf("used/percent = %d/%.2f", q.Used(), q.Percent())
	}
	// 缺字段/畸形字段都不能挂
	for _, h := range []string{
		"", "   ", "garbage", "upload=abc; total=100",
		"total=1024", "expire=1777732477", "upload=1; download=2",
	} {
		if got := ParseQuota(h); got == nil && h != "" && h != "   " && h != "garbage" {
			t.Errorf("%q should still parse", h)
		}
	}
	if ParseQuota("") != nil || ParseQuota("garbage") != nil {
		t.Error("empty/garbage must be nil")
	}
}

func TestQuotaShort(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"upload=0; download=0; total=171798691840; expire=1777732477", "0B/160.00GB (0.00%)"},
		{"upload=1073741824; download=2147483648; total=10737418240", "3.00GB/10.00GB (30.00%)"},
		{"upload=0; download=0", "长期有效"},
	}
	for _, c := range cases {
		got := ParseQuota(c.header).Short()
		if got[:len(c.want)] != c.want {
			t.Errorf("%q → %q, want prefix %q", c.header, got, c.want)
		}
	}
}

func TestQuotaState(t *testing.T) {
	// 已过期
	if s := ParseQuota("total=100; download=1; expire=1000000000").State(); s != "expired" {
		t.Errorf("expired state = %q", s)
	}
	// 用超了
	if s := ParseQuota("total=100; download=100").State(); s != "out" {
		t.Errorf("out state = %q", s)
	}
	// 快用完了
	if s := ParseQuota("total=100; download=85").State(); s != "low" {
		t.Errorf("low state = %q", s)
	}
	// 正常
	if s := ParseQuota("total=100; download=10").State(); s != "ok" {
		t.Errorf("ok state = %q", s)
	}
	// 不限量
	if s := ParseQuota("download=10").State(); s != "ok" {
		t.Errorf("unlimited state = %q", s)
	}
}

func TestBytesHuman(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0B", 1023: "1023B", 1024: "1.00KB",
		171798691840: "160.00GB", 1099511627776: "1.00TB",
	} {
		if got := BytesHuman(in); got != want {
			t.Errorf("BytesHuman(%d) = %q, want %q", in, got, want)
		}
	}
}
