package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	"strings"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

const repoAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func HTTPClient(proxy string) *http.Client {
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr}
}

// ArchName 返回 mihomo release 资产中的架构名
func ArchName() string {
	a := runtime.GOARCH
	if a == "arm" {
		a = "armv7"
	}
	return a
}

// Latest 获取最新 release 信息; API 限流时降级为解析 releases/latest 重定向
func Latest(proxy string) (*Release, error) {
	req, _ := http.NewRequest("GET", repoAPI, nil)
	req.Header.Set("User-Agent", "mihomo-cli")
	req.Header.Set("Accept", "application/vnd.github+json")
	hc := HTTPClient(proxy)
	resp, err := hc.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			r := &Release{}
			if err := json.NewDecoder(resp.Body).Decode(r); err == nil {
				return r, nil
			}
		}
	}
	// fallback: /releases/latest 302 -> /tag/<tag>
	if tag := tagFromRedirect(hc); tag != "" {
		return &Release{TagName: tag}, nil
	}
	if resp != nil {
		return nil, fmt.Errorf("%s %d (%s)", i18n.T("GitHub API 返回"), resp.StatusCode, i18n.T("可能被限流, 可稍后重试"))
	}
	return nil, fmt.Errorf("%s GitHub: %w", i18n.T("访问失败"), err)
}

// LatestTag 通过 releases/latest 重定向解析仓库最新 tag (不依赖 API 限额)
func LatestTag(hc *http.Client, repo string) string {
	client := *hc
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	req, _ := http.NewRequest("GET", "https://github.com/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", "mihomo-cli")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		loc = resp.Request.URL.String()
	}
	if i := strings.LastIndex(loc, "/tag/"); i >= 0 {
		return loc[i+5:]
	}
	return ""
}

func tagFromRedirect(hc *http.Client) string {
	return LatestTag(hc, "MetaCubeX/mihomo")
}

// DownloadInstall 下载指定 tag 的内核并安装到 CoreBin, 旧版本备份为 mihomo.old
func DownloadInstall(tag, proxy, mirrorPref string, compatible bool) error {
	arch := ArchName()
	if compatible && arch == "amd64" {
		arch = "amd64-compatible" // 旧 CPU 无 AVX 时使用
	}
	assetName := fmt.Sprintf("mihomo-linux-%s-%s.gz", arch, tag)

	rel := &Release{TagName: tag}
	assetURL := ""
	if tag == "" {
		r, err := Latest(proxy)
		if err != nil {
			return err
		}
		rel = r
		assetName = fmt.Sprintf("mihomo-linux-%s-%s.gz", arch, r.TagName)
	}
	for _, a := range rel.Assets {
		if a.Name == assetName {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		// API 未返回资产列表时直接构造下载 URL
		assetURL = fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", rel.TagName, assetName)
	}

	fmt.Printf("%s: %s\n", i18n.T("下载内核"), assetURL)
	if err := downloadGunzip(assetURL, proxy, app.CoreBin+".tmp"); err != nil {
		return err
	}
	if _, err := os.Stat(app.CoreBin); err == nil {
		_ = os.Remove(app.CoreBinOld)
		_ = os.Rename(app.CoreBin, app.CoreBinOld)
	}
	if err := os.Rename(app.CoreBin+".tmp", app.CoreBin); err != nil {
		return err
	}
	return os.Chmod(app.CoreBin, 0o755)
}

func downloadGunzip(u, proxy, dst string) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "mihomo-cli")
	resp, err := HTTPClient(proxy).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s %d", i18n.T("下载失败"), resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, gz)
	if err != nil {
		return err
	}
	fmt.Printf("%s %.1f MiB\n", i18n.T("已下载"), float64(n)/1024/1024)
	return nil
}

// Version 获取已安装内核的版本描述
func Version() (string, error) {
	if _, err := os.Stat(app.CoreBin); os.IsNotExist(err) {
		return "", fmt.Errorf("%s", i18n.T("内核未安装, 请先执行 mihomo-cli install"))
	}
	out, err := exec.Command(app.CoreBin, "-v").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
}

// Rollback 回滚到备份版本
func Rollback() error {
	if _, err := os.Stat(app.CoreBinOld); os.IsNotExist(err) {
		return fmt.Errorf("%s", i18n.T("没有可回滚的旧版本"))
	}
	_ = os.Remove(app.CoreBin)
	return os.Rename(app.CoreBinOld, app.CoreBin)
}

// Upgrade 检查并升级
func Upgrade(proxy string, pre bool) error {
	cur, _ := Version()
	curVer := ""
	if i := strings.Index(cur, "v1."); i >= 0 {
		f := strings.Fields(cur[i:])
		if len(f) > 0 {
			curVer = f[0]
		}
	}
	rel, err := Latest(proxy)
	if err != nil {
		return err
	}
	if curVer == rel.TagName {
		fmt.Printf("%s %s\n", i18n.T("内核已是最新版本"), rel.TagName)
		return nil
	}
	fmt.Printf("%s: %s -> %s\n", i18n.T("升级内核:"), curVer, rel.TagName)
	return DownloadInstall(rel.TagName, proxy, s_mirror(), false)
}

func s_mirror() string {
	s, err := app.LoadSettings()
	if err != nil {
		return ""
	}
	return s.GithubMirror
}

func CoreBinPath() string { return filepath.Clean(app.CoreBin) }

// GitHub 镜像站 (下载失败时依次尝试; 可通过 settings install-mirror 指定)
var ghMirrors = []string{
	"https://ghfast.top",
	"https://gh-proxy.com",
	"https://mirror.ghproxy.com",
}

// MirrorURLs 返回 GitHub 资产的候选 URL: 镜像站格式为 <mirror>/<原始URL>
func MirrorURLs(u, preferred string) []string {
	out := []string{u}
	mirrors := ghMirrors
	if preferred != "" {
		mirrors = append([]string{preferred}, mirrors...)
	}
	for _, m := range mirrors {
		m = strings.TrimRight(m, "/")
		if strings.Contains(u, m) {
			continue
		}
		out = append(out, m+"/"+u)
	}
	return out
}

// OwnProxy 若本机代理服务存活且链路可用, 返回自身代理地址 (下载优先走自己)
func OwnProxy() string {
	s, err := app.LoadSettings()
	if err != nil || s.Current() == nil || !chainReady(s) {
		return ""
	}
	addr := fmt.Sprintf("http://127.0.0.1:%d", s.MixedPort)
	c := http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(parseURL(addr))}}
	req, _ := http.NewRequest("GET", "https://www.gstatic.com/generate_204", nil)
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 204 {
		return ""
	}
	resp.Body.Close()
	return addr
}

func chainReady(s *app.Settings) bool {
	return s.CurrentGroup != "" // sub->group 已选即视为链路有效
}

func parseURL(u string) *url.URL {
	p, _ := url.Parse(u)
	return p
}

// FetchURL 依次尝试候选 URL: 指定代理 -> 自身代理 -> 各镜像 -> 直连
func FetchURL(u, proxy, mirror, dst string) error {
	return fetch(u, proxy, mirror, dst, func(r io.Reader, f *os.File) (int64, error) { return io.Copy(f, r) })
}

// FetchGunzip 同 FetchURL 但做 gunzip 解压 (内核二进制)
func FetchGunzip(u, proxy, mirror, dst string) error {
	return fetch(u, proxy, mirror, dst, func(r io.Reader, f *os.File) (int64, error) {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return 0, err
		}
		defer gz.Close()
		return io.Copy(f, gz)
	})
}

func fetch(u, proxy, mirror, dst string, process func(io.Reader, *os.File) (int64, error)) error {
	proxies := []string{}
	if proxy != "" {
		proxies = append(proxies, proxy)
	}
	if own := OwnProxy(); own != "" && own != proxy {
		proxies = append(proxies, own)
	}
	urls := MirrorURLs(u, mirror)
	var lastErr error
	for _, p := range append(proxies, "") {
		for _, uu := range urls {
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
			if err != nil {
				return err
			}
			req, _ := http.NewRequest("GET", uu, nil)
			req.Header.Set("User-Agent", "mihomo-cli")
			hc := HTTPClient(p)
			hc.Timeout = 10 * time.Minute
			resp, err := hc.Do(req)
			if err != nil {
				f.Close()
				os.Remove(dst)
				lastErr = err
				continue
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				f.Close()
				os.Remove(dst)
				lastErr = fmt.Errorf("HTTP %d (%s)", resp.StatusCode, shortURL(uu))
				continue
			}
			n, err := process(resp.Body, f)
			resp.Body.Close()
			f.Close()
			if err != nil || n == 0 {
				os.Remove(dst)
				lastErr = err
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("%s: %w", i18n.T("下载失败"), lastErr)
}

// geo 数据源: 镜像在前 (国内可达), GitHub 官方兜底
var geoMirrors = map[string][]string{
	"geoip.metadb": {
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
		"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geoip.metadb",
	},
	"GeoSite.dat": {
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
		"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geosite.dat",
	},
}

// Resource 可管理资源 (内核二进制 + geo 数据)
type Resource struct {
	Name string // core / mmdb / asn / geoip / geosite
	File string // 落盘路径
	Desc string
	URLs []string
}

var resources = []Resource{
	{"mmdb", "geoip.metadb", "GeoIP (metadb)", geoMirrors["geoip.metadb"]},
	{"geosite", "GeoSite.dat", "GeoSite", geoMirrors["GeoSite.dat"]},
	{"asn", "GeoLite2-ASN.mmdb", "GeoLite2 ASN", []string{
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
		"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/GeoLite2-ASN.mmdb",
	}},
	{"geoip", "geoip.dat", "GeoIP (dat, geodata 模式)", []string{
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat",
		"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geoip.dat",
	}},
}

// Resources 返回数据资源列表 (不含 core)
func Resources() []Resource { return resources }

// ResourceInfo 资源状态: 是否已安装 / 修改时间 / 大小
func ResourceInfo(name string) (path string, ok bool, modTime time.Time, size int64) {
	for _, r := range resources {
		if r.Name != name {
			continue
		}
		p := filepath.Join(app.RuntimeDir, r.File)
		st, err := os.Stat(p)
		if err != nil {
			return p, false, time.Time{}, 0
		}
		return p, true, st.ModTime(), st.Size()
	}
	return "", false, time.Time{}, 0
}

// UpdateResource 下载/更新指定数据资源
func UpdateResource(name, proxy, mirror string) error {
	for _, r := range resources {
		if r.Name != name {
			continue
		}
		dst := filepath.Join(app.RuntimeDir, r.File)
		fmt.Printf("%s %s (%s) ...\n", i18n.T("更新资源"), r.Name, r.Desc)
		var lastErr error
		for _, u := range r.URLs {
			if err := FetchURL(u, proxy, mirror, dst+".tmp"); err != nil {
				lastErr = err
				continue
			}
			if err := os.Rename(dst+".tmp", dst); err != nil {
				return err
			}
			return nil
		}
		return lastErr
	}
	return fmt.Errorf("unknown resource %q", name)
}

// UpdateAllResources 更新全部数据资源 (定时任务调用)
func UpdateAllResources(proxy, mirror string) error {
	var errs []string
	for _, r := range resources {
		if err := UpdateResource(r.Name, proxy, mirror); err != nil {
			errs = append(errs, r.Name+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// DownloadGeo 预下载 geo 数据到 runtime 目录 (install 调用: mmdb+geosite)
func DownloadGeo(proxy, mirrorPref string) error {
	for _, name := range []string{"mmdb", "geosite"} {
		p, ok, _, _ := ResourceInfo(name)
		if ok {
			continue
		}
		if err := UpdateResource(name, proxy, mirrorPref); err != nil {
			return err
		}
		_ = p
	}
	return nil
}

func downloadPlain(u, proxy, dst string) error {
	hc := HTTPClient(proxy)
	hc.Timeout = 10 * time.Minute
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "mihomo-cli")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func shortURL(u string) string {
    if i := strings.Index(u, "//"); i >= 0 {
        u = u[i+2:]
    }
    if len(u) > 40 {
        u = u[:40] + "..."
    }
    return u
}
