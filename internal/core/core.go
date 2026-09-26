package core

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

const repoAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"

// MaxHistory 本地保留的旧内核版本数 (rollback 出栈用)
const MaxHistory = 3

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

// ---- 平台与风味 (内核安装包规格的两个正交轴) ----

// ArchName 返回 mihomo release 资产中的架构名; arch 为空则按当前系统检测
func ArchName() string { return archName(runtime.GOARCH) }

func archName(goarch string) string {
	switch goarch {
	case "arm":
		return "armv7"
	case "":
		return archName(runtime.GOARCH)
	default:
		return goarch
	}
}

// PlatformName 平台字符串 (linux/amd64); arch 为空则自动检测
func PlatformName(arch string) string {
	if arch == "" {
		arch = runtime.GOARCH
	}
	return "linux/" + archName(arch)
}

// FlavorCandidates 风味候选链 (最佳优先; 下载或试跑失败则降级)。
// 上游资产名形如 mihomo-linux-<arch>[-<flavor>]-<version>.gz, 风味即中括号里的内容:
//
//	amd64: plain | v1 | v2 | v3 | compatible | v1-go120 | v2-go120 | v3-go120 …
//	386  : plain | go120 | go123 | softfloat
//	arm64/armv7: 只有 plain
//
// v1/v2/v3 = GOAMD64 微架构档位 (v3 需 AVX2); go120/go123 = 编译用 Go 工具链(影响最低 glibc);
// compatible = 旧 CPU + 旧 glibc 的保守构建 (仅 amd64 有)。
func FlavorCandidates(flavor, arch string) []string {
	if flavor != "" && flavor != "auto" && flavor != "latest" {
		return []string{flavor}
	}
	switch archName(arch) {
	case "amd64":
		return []string{"", "v3", "v2", "v1", "compatible", "v2-go120", "v1-go120"}
	case "386":
		return []string{"", "go120", "softfloat", "go123"}
	default:
		return []string{""}
	}
}

// ParseCoreSpec 解析 --core <spec>: [flavor][:version]
//
//	"auto"                → 风味自动 + 最新版本
//	"latest"              → 同上 (兼容 v1.2)
//	"compatible"          → 指定风味, 最新版本
//	"v2-go120"            → 指定风味组合, 最新版本
//	"v1.19.31"            → 指定版本, 风味自动
//	"compatible:v1.19.19" → 风味 + 版本
func ParseCoreSpec(spec string) (flavor, version string, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", nil
	}
	parts := strings.SplitN(spec, ":", 2)
	flavor = strings.TrimSpace(parts[0])
	if flavor == "latest" {
		flavor = "auto"
	}
	if len(parts) == 2 {
		version = strings.TrimSpace(parts[1])
		if !looksLikeVersion(version) {
			return "", "", fmt.Errorf("%s: %q (%s: vX.Y.Z)", i18n.T("无效版本号"), version, i18n.T("期望"))
		}
	}
	if flavor != "" && flavor != "auto" && !validFlavor(flavor) {
		return "", "", fmt.Errorf("%s: %q (%s)", i18n.T("无效内核风味"), flavor, flavorsHint())
	}
	return flavor, version, nil
}

// looksLikeVersion vX.Y.Z / vX.Y.Z-rcN
func looksLikeVersion(v string) bool {
	if len(v) < 4 || v[0] != 'v' {
		return false
	}
	for _, r := range v[1:] {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-') {
			return false
		}
	}
	segs := strings.Split(v[1:], ".")
	for _, s := range segs {
		if s == "" {
			return false
		}
	}
	return len(segs) >= 2 && len(segs) <= 3
}

// validFlavor 风味必须是上游资产名的中括段 (字母数字与连字符)
func validFlavor(f string) bool {
	for _, r := range f {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return len(f) > 0
}

func flavorsHint() string {
	return "auto | compatible | v1 | v2 | v3 | go120 | go123 | v2-go120 …"
}

// AssetName 资产文件名
func AssetName(arch, flavor, version string) string {
	mid := ""
	if flavor != "" && flavor != "auto" {
		mid = "-" + flavor
	}
	return fmt.Sprintf("mihomo-%s-%s%s-%s.gz", "linux", archName(arch), mid, version)
}

// ---- 版本查询 ----

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

// Version 获取已安装内核的版本描述
func Version() (string, error) {
	if _, err := os.Stat(app.CoreBin); os.IsNotExist(err) {
		return "", fmt.Errorf("%s", i18n.T("内核未安装, 请先执行 mihomo-cli install --core auto"))
	}
	out, err := exec.Command(app.CoreBin, "-v").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
}

// VersionShort 已安装内核的版本号 (v1.19.31)
func VersionShort() string {
	v, err := Version()
	if err != nil {
		return ""
	}
	if i := strings.Index(v, "v1."); i >= 0 {
		f := strings.Fields(v[i:])
		if len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

// ---- 安装 / 候选链降级 ----

// InstallResult 一次内核安装的结果
type InstallResult struct {
	Version  string // v1.19.31
	Flavor   string // 实际使用的风味 (""=plain)
	Platform string // linux/amd64
	Arch     string // amd64
	Degraded bool   // 是否发生了降级 (自动探测时)
}

// InstallSpec 按规格安装内核: 风味走候选链, 每下载一个都试跑 <bin> -v,
// 跑不起来(老 CPU 缺指令集 / glibc 太低)就降级下一个候选。
// 安装前把当前版本推入本地版本栈 (供 rollback 离线回滚)。
func InstallSpec(s *app.Settings, flavor, version, proxy, mirror, arch string) (*InstallResult, error) {
	if arch == "" {
		arch = runtime.GOARCH
	}
	if version == "" {
		rel, err := Latest(proxy)
		if err != nil {
			return nil, err
		}
		version = rel.TagName
	}
	cands := FlavorCandidates(flavor, arch)
	var lastErr error
	for i, f := range cands {
		if i > 0 {
			fmt.Printf("%s: %s\n", i18n.T("降级尝试下一个内核构建"), orPlain(f))
		}
		tmp := app.CoreBin + ".cand"
		_ = os.Remove(tmp)
		ok, err := downloadAsset(arch, f, version, proxy, mirror, tmp)
		if err != nil {
			lastErr = err
			continue
		}
		if !ok {
			continue
		}
		// 试跑: 老平台会 exec 失败 (GLIBC/指令集)
		if out, err := exec.Command(tmp, "-v").CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", firstLine(string(out)), err)
			lastErr = fmt.Errorf("%s %s", i18n.T("该构建无法在当前系统运行"), orPlain(f))
			_ = os.Remove(tmp)
			continue
		}
		// 装填: 旧版本入栈 → 新二进制就位
		if err := swapIn(s, tmp, version, f, arch); err != nil {
			return nil, err
		}
		return &InstallResult{
			Version: version, Flavor: f, Platform: PlatformName(arch), Arch: arch,
			Degraded: i > 0,
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s", i18n.T("没有可用的内核构建"))
	}
	return nil, lastErr
}

func orPlain(f string) string {
	if f == "" {
		return i18n.T("官方默认")
	}
	return f
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// downloadAsset 下载(解压)一个候选构建到 dst; ok=false 表示该构建不存在(404)
func downloadAsset(arch, flavor, version, proxy, mirror, dst string) (bool, error) {
	name := AssetName(arch, flavor, version)
	rel, err := Latest(proxy)
	if err == nil {
		for _, a := range rel.Assets {
			if a.Name == name {
				if err := fetchGunzip(a.BrowserDownloadURL, proxy, mirror, dst); err != nil {
					return true, err
				}
				return true, nil
			}
		}
	}
	url := fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", version, name)
	if err := fetchGunzip(url, proxy, mirror, dst); err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return true, err
	}
	return true, nil
}

// swapIn 旧版本入栈, 新二进制就位, chmod 0755
func swapIn(s *app.Settings, tmp, version, flavor, arch string) error {
	if err := app.EnsureDirs(); err != nil {
		return err
	}
	if err := pushHistory(s, version, flavor, arch); err != nil {
		return err
	}
	_ = os.Remove(app.CoreBin)
	if err := os.Rename(tmp, app.CoreBin); err != nil {
		// 跨文件系统时回退拷贝
		if err2 := copyFile(tmp, app.CoreBin); err2 != nil {
			return err
		}
	}
	return os.Chmod(app.CoreBin, 0o755)
}

// pushHistory 把当前已安装的版本推入版本栈, 并把二进制存档到 bin/versions/<ver>/
func pushHistory(s *app.Settings, newVersion, flavor, arch string) error {
	cur := VersionShort()
	if cur == "" || cur == newVersion {
		return nil
	}
	if _, err := os.Stat(app.CoreBin); err != nil {
		return nil
	}
	dir := filepath.Join(app.VersionsDir, cur)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, "mihomo")
	if _, err := os.Stat(dst); err != nil {
		if err := copyFile(app.CoreBin, dst); err != nil {
			return err
		}
		_ = os.Chmod(dst, 0o755)
	}
	// 入栈 (栈顶=最近一个)
	s.CoreHistory = append([]app.CoreVer{{
		Version: cur, Flavor: flavorOf(s), Platform: PlatformName(arch), InstalledAt: time.Now(),
	}}, s.CoreHistory...)
	if len(s.CoreHistory) > MaxHistory {
		dropped := s.CoreHistory[MaxHistory:]
		s.CoreHistory = s.CoreHistory[:MaxHistory]
		for _, d := range dropped {
			_ = os.RemoveAll(filepath.Join(app.VersionsDir, d.Version))
		}
	}
	s.CoreVersion = newVersion
	s.CoreFlavor = flavor
	s.CorePlatform = PlatformName(arch)
	return s.Save()
}

// flavorOf 已记录的当前风味 (新版本换风味时不能污染旧记录)
func flavorOf(s *app.Settings) string { return s.CoreFlavor }

// Rollback 出栈: 把上一个装过的版本装回 (纯本地, 不需要网络)
func Rollback() (string, error) {
	s, err := app.LoadSettings()
	if err != nil {
		return "", err
	}
	if len(s.CoreHistory) == 0 {
		// v1.2 遗留的 bin/mihomo.old
		if _, err := os.Stat(app.CoreBinOld); err == nil {
			_ = os.Remove(app.CoreBin)
			if err := os.Rename(app.CoreBinOld, app.CoreBin); err != nil {
				return "", err
			}
			return "bin/mihomo.old", nil
		}
		return "", fmt.Errorf("%s", i18n.T("没有可回滚的旧版本 (版本栈为空)"))
	}
	top := s.CoreHistory[0]
	src := filepath.Join(app.VersionsDir, top.Version, "mihomo")
	if _, err := os.Stat(src); err != nil {
		// 存档丢失: 弹出这一项看下一个
		s.CoreHistory = s.CoreHistory[1:]
		_ = s.Save()
		return Rollback()
	}
	cur := VersionShort()
	// 当前版本放回被回滚版本的位置, 栈顶弹出
	if cur != "" {
		curDir := filepath.Join(app.VersionsDir, cur)
		_ = os.MkdirAll(curDir, 0o755)
		if _, err := os.Stat(app.CoreBin); err == nil {
			_ = copyFile(app.CoreBin, filepath.Join(curDir, "mihomo"))
			_ = os.Chmod(filepath.Join(curDir, "mihomo"), 0o755)
		}
	}
	_ = os.Remove(app.CoreBin)
	if err := copyFile(src, app.CoreBin); err != nil {
		return "", err
	}
	_ = os.Chmod(app.CoreBin, 0o755)
	s.CoreHistory = s.CoreHistory[1:]
	s.CoreVersion = top.Version
	s.CoreFlavor = top.Flavor
	_ = s.Save()
	return top.Version, nil
}

// History 本地版本栈 (新→旧)
func History() []app.CoreVer {
	s, err := app.LoadSettings()
	if err != nil {
		return nil
	}
	return s.CoreHistory
}

// Upgrade 通用"切换版本": 接受任意版本号 (可新可旧), 只要与当前不同就换。
// spec 为空 = 沿用已记住的平台+风味 + 最新版本。
func Upgrade(s *app.Settings, flavor, version, proxy, mirror string) (*InstallResult, error) {
	arch := ""
	if s.CorePlatform != "" {
		arch = strings.TrimPrefix(s.CorePlatform, "linux/")
	}
	if flavor == "" {
		flavor = s.CoreFlavor // 沿用已记住的风味
	}
	cur := VersionShort()
	res, err := InstallSpec(s, flavor, version, proxy, mirror, arch)
	if err != nil {
		return nil, err
	}
	if cur != "" && cur == res.Version && s.CoreFlavor == res.Flavor {
		fmt.Printf("%s %s\n", i18n.T("内核已是该版本"), res.Version)
	}
	return res, nil
}

// ---- 下载链 ----

var ghMirrors = []string{
	"https://ghfast.top",
	"https://gh-proxy.com",
	"https://mirror.ghproxy.com",
}

// MirrorURLs 返回 GitHub 资产的候选 URL: 镜像站格式为 <mirror>/<原始URL>
func MirrorURLs(u, preferred string) []string {
	out := []string{u}
	mirrors := ghMirrors
	if preferred != "" && preferred != "auto" {
		mirrors = append([]string{preferred}, ghMirrors...)
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
	addr := fmt.Sprintf("http://127.0.0.1:%d", s.ProxyPort())
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
	return fetch(u, proxy, mirror, dst, 0o644, func(r io.Reader, f *os.File) (int64, error) { return io.Copy(f, r) })
}

// FetchGunzip 同 FetchURL 但做 gunzip 解压 (内核二进制)
func FetchGunzip(u, proxy, mirror, dst string) error {
	return fetch(u, proxy, mirror, dst, 0o755, func(r io.Reader, f *os.File) (int64, error) {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return 0, err
		}
		defer gz.Close()
		return io.Copy(f, gz)
	})
}

// fetchGunzip 同 FetchGunzip (内部简称)
func fetchGunzip(u, proxy, mirror, dst string) error { return FetchGunzip(u, proxy, mirror, dst) }

func fetch(u, proxy, mirror, dst string, mode os.FileMode, process func(io.Reader, *os.File) (int64, error)) error {
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
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
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
				if resp.StatusCode == 404 {
					return fmt.Errorf("HTTP 404")
				}
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

// ---- geo 资源 ----

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
	{"geoip", "geoip.dat", "GeoIP (dat, for geodata mode)", []string{
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat",
		"https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geoip.dat",
	}},
}

// Resources 返回数据资源列表 (不含 core)
func Resources() []Resource { return resources }

// ResourcePath 资源文件路径
func ResourcePath(name string) string {
	for _, r := range resources {
		if r.Name == name {
			return filepath.Join(app.RuntimeDir, r.File)
		}
	}
	return ""
}

// RemoveResource 删除一个数据资源
func RemoveResource(name string) bool {
	p := ResourcePath(name)
	if p == "" {
		return false
	}
	if _, err := os.Stat(p); err != nil {
		return false
	}
	return os.Remove(p) == nil
}

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
		if _, ok, _, _ := ResourceInfo(name); ok {
			continue
		}
		if err := UpdateResource(name, proxy, mirrorPref); err != nil {
			return err
		}
	}
	return nil
}

// SHA256 文件摘要 (测试/校验用)
func SHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
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

// SortResources 资源名排序 (展示用)
func SortResources() []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}
