package subs

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"gopkg.in/yaml.v3"
)

var nameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// Sanitize 规范化订阅名
func Sanitize(name string) string {
	n := nameRe.ReplaceAllString(strings.TrimSpace(name), "-")
	if n == "" {
		n = "default"
	}
	return n
}

func Path(name string) string {
	return filepath.Join(app.ProfileDir, Sanitize(name)+".yaml")
}

// Download 下载订阅内容, 返回 yaml 字节与 subscription-userinfo 头
func Download(rawurl, proxy string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", rawurl, nil)
	if err != nil {
		return nil, "", err
	}
	// 部分机场按 UA 返回格式
	req.Header.Set("User-Agent", "clash.meta/mihomo-cli")
	resp, err := core.HTTPClient(proxy).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", i18n.T("下载订阅失败"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("%s %d", i18n.T("订阅服务器返回"), resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", err
	}
	info := resp.Header.Get("subscription-userinfo")
	if !looksLikeYAML(data) {
		return nil, info, fmt.Errorf("%s (%s)", i18n.T("订阅内容不是 clash yaml 格式"), i18n.T("可能需要 UA 或链接失效"))
	}
	return data, info, nil
}

func looksLikeYAML(data []byte) bool {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return false
	}
	_, hasProxies := m["proxies"]
	_, hasProxyGroups := m["proxy-groups"]
	return hasProxies || hasProxyGroups
}

// CountNodes 统计节点数
func CountNodes(data []byte) int {
	var m struct {
		Proxies []any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return 0
	}
	return len(m.Proxies)
}

// Add 新增订阅: 下载 + 保存 + 生效
func Add(s *app.Settings, name, rawurl, proxy string) error {
	name = Sanitize(name)
	if s.FindProfile(name) != nil {
		return fmt.Errorf("%s %q %s", i18n.T("订阅"), name, i18n.T("已存在"))
	}
	fmt.Printf("%s %s ...\n", i18n.T("下载订阅"), rawurl)
	data, info, err := Download(rawurl, proxy)
	if err != nil {
		return err
	}
	if err := app.EnsureDirs(); err != nil {
		return err
	}
	if err := os.WriteFile(Path(name), data, 0o644); err != nil {
		return err
	}
	s.Profiles = append(s.Profiles, app.Profile{
		Name: name, URL: rawurl, UpdatedAt: time.Now(),
		Nodes: CountNodes(data), UserInfo: info,
	})
	if s.CurrentProfile == "" {
		s.CurrentProfile = name
	}
	return s.Save()
}

// Update 更新订阅(空名 = 当前订阅; all = 全部)
func Update(s *app.Settings, name, proxy string) error {
	targets := []*app.Profile{}
	if name == "all" {
		for i := range s.Profiles {
			targets = append(targets, &s.Profiles[i])
		}
	} else {
		n := name
		if n == "" {
			if s.Current() == nil {
				return fmt.Errorf("%s", i18n.T("没有可用订阅"))
			}
			n = s.Current().Name
		}
		p := s.FindProfile(n)
		if p == nil {
			return fmt.Errorf("%s %q %s", i18n.T("订阅"), n, i18n.T("不存在"))
		}
		targets = append(targets, p)
	}
	var errs []string
	for _, p := range targets {
		fmt.Printf("%s [%s] ...\n", i18n.T("更新订阅"), p.Name)
		data, info, err := Download(p.URL, proxy)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		if err := os.WriteFile(Path(p.Name), data, 0o600); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		p.UpdatedAt = time.Now()
		p.Nodes = CountNodes(data)
		p.UserInfo = info
		fmt.Printf("  %s: %d\n", i18n.T("节点数"), p.Nodes)
	}
	if err := s.Save(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// Remove 删除订阅
func Remove(s *app.Settings, name string) error {
	idx := -1
	for i := range s.Profiles {
		if s.Profiles[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("%s %q %s", i18n.T("订阅"), name, i18n.T("不存在"))
	}
	s.Profiles = append(s.Profiles[:idx], s.Profiles[idx+1:]...)
	_ = os.Remove(Path(name))
	if s.CurrentProfile == name {
		s.CurrentProfile = ""
		if len(s.Profiles) > 0 {
			s.CurrentProfile = s.Profiles[0].Name
		}
	}
	return s.Save()
}
