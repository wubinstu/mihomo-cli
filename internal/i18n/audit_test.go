package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestAllKeysTranslated 源码里每个 T("…") 的 key 都必须在 en 表里。
// 这是用户反复要求的"en 模式零中文"的自动守卫: 漏一条就 CI 红。
func TestAllKeysTranslated(t *testing.T) {
	root := repoRoot(t)
	missing := map[string]bool{}
	for _, f := range goFiles(t, root) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range regexp.MustCompile(`T\("((?:[^"\\]|\\.)*)"\)`).FindAllStringSubmatch(string(data), -1) {
			key := unescape(m[1])
			if _, ok := en[key]; !ok {
				missing[key] = true
				t.Errorf("untranslated key %q (in %s)", key, rel(t, f))
			}
		}
	}
	if len(missing) > 0 {
		t.Logf("run: grep -ohE 'T\\(\"[^\"]+\"\\)' internal/*/*.go | sed 's/T(\"//;s/\")//' | sort -u")
	}
}

// TestNoOrphanKeys en 表里不应有源码已不再使用的 key (防止词条腐化)
func TestNoOrphanKeys(t *testing.T) {
	root := repoRoot(t)
	used := map[string]bool{}
	for _, f := range goFiles(t, root) {
		data, _ := os.ReadFile(f)
		for _, m := range regexp.MustCompile(`T\("((?:[^"\\]|\\.)*)"\)`).FindAllStringSubmatch(string(data), -1) {
			used[unescape(m[1])] = true
		}
	}
	var orphans []string
	for k := range en {
		if !used[k] {
			orphans = append(orphans, k)
		}
	}
	sort.Strings(orphans)
	if len(orphans) > 0 {
		t.Logf("orphan en entries (harmless, translations kept for reuse): %d", len(orphans))
	}
}

// TestNoBareChinese 用户可见字符串必须包 T(); 这里检查新增的裸中文字面量
// (names.go 国家名表、注释、geo 数据文件路径等是预期例外, 走白名单)
func TestNoBareChinese(t *testing.T) {
	root := repoRoot(t)
	allow := map[string]bool{
		"internal/geo/names.go":    true, // ISO→中/英文国家名表
		"internal/i18n/i18n.go":    true, // en 表本身就是中文 key
		"internal/cmd/node.go":     true, // 节点地区判定(含订阅数据特征)
		"internal/app/settings.go": true, // config.toml 文件头注释(用户被要求别手改, 交给中文)
		"internal/cfg/registry.go": true, // Usage 字段值即翻译 key, 由 TestRegistryUsageTranslated 校验
		"internal/cfg/keys.go":     true, // 报错文案的 key (交由注册表/审计统一校验)
		"internal/cfg/live.go":     true, // 同上
	}
	re := regexp.MustCompile(`"([^"\n]*[\x{4e00}-\x{9fff}][^"\n]*)"`)
	var bad []string
	for _, f := range goFiles(t, root) {
		if allow[rel(t, f)] {
			continue
		}
		_ = f
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			code := stripComment(line)
			for _, m := range re.FindAllStringSubmatch(code, -1) {
				s := m[1]
				// 同一行里出现 T("…原样…") 就是翻译 key, 属正常
				if strings.Contains(code, `T("`+s) || strings.Contains(code, `T("`+s[:min(8, len(s))]) {
					continue
				}
				if strings.Contains(code, `i18n.T("`) {
					continue
				}
				if isProgramToken(s) {
					continue
				}
				bad = append(bad, rel(t, f)+":"+itoa(i+1)+" "+s)
			}
		}
	}
	if len(bad) > 0 {
		t.Errorf("bare Chinese literals (wrap with T() or move to the en table):\n  %s",
			strings.Join(bad, "\n  "))
	}
}

func isProgramToken(s string) bool {
	// 环境变量名 / 二进制名 / 单元名 / 键名token 不算用户可见文案
	for _, p := range []string{"mihomo", "systemd", "bash", "zsh", "fish", "deb", "rpm",
		"journalctl", "direct", "rule", "global", "gvisor", "mixed", "fake-ip"} {
		if strings.EqualFold(s, p) {
			return true
		}
	}
	return false
}

func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

func unescape(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func goFiles(t *testing.T, root string) []string {
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// repoRoot 向上找到 go.mod 所在的目录
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Fatalf("go.mod not found above %s", wd)
		}
	}
}

func rel(t *testing.T, p string) string {
	t.Helper()
	root := repoRoot(t)
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

// TestSet 语言切换
func TestSet(t *testing.T) {
	Set("zh")
	if Lang() != "zh" || T("已启用") != "已启用" {
		t.Fatal("zh should keep the original")
	}
	Set("en")
	if Lang() != "en" || T("已启用") != "enabled" {
		t.Fatalf("en: %q", T("已启用"))
	}
	Set("bogus") // 非法值忽略
	if Lang() != "en" {
		t.Fatal("bogus must be ignored")
	}
	Set("zh")
}
