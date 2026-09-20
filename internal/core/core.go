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
func DownloadInstall(tag, proxy string, compatible bool) error {
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
	return DownloadInstall(rel.TagName, proxy, false)
}

func CoreBinPath() string { return filepath.Clean(app.CoreBin) }
