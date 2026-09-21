package cmd

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"archive/tar"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/core"
)

const cliRepoAPI = "https://api.github.com/repos/wubinstu/mihomo-cli/releases/latest"

var updProxy string

var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"upgrade"},
	Short:   T("更新 mihomo-cli 自身 (从 GitHub Releases)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		hc := core.HTTPClient(updProxy)

		tag, assetURL, err := latestCLI(hc)
		if err != nil {
			return err
		}
		if tag == "v"+Version {
			fmt.Printf("%s: mihomo-cli %s\n", T("已是最新版本"), Version)
			return nil
		}
		fmt.Printf("%s: %s -> %s\n", T("升级"), "v"+Version, tag)

		self, err := os.Executable()
		if err != nil {
			return err
		}
		self, _ = filepath.Abs(self)

		tmp, err := os.MkdirTemp("", "mihomo-cli-upd")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		tgz := filepath.Join(tmp, "pkg.tgz")

		req, _ := http.NewRequest("GET", assetURL, nil)
		req.Header.Set("User-Agent", "mihomo-cli")
		resp, err := hc.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", T("下载失败"), err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return fmt.Errorf("%s %d", T("下载失败"), resp.StatusCode)
		}
		f, _ := os.Create(tgz)
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			return err
		}

		// 解出二进制
		in, err := os.Open(tgz)
		if err != nil {
			return err
		}
		defer in.Close()
		gz, err := gzip.NewReader(in)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		binPath := filepath.Join(tmp, "mihomo-cli")
		out, err := os.OpenFile(binPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		found := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				out.Close()
				return err
			}
			if !hdr.FileInfo().Mode().IsRegular() {
				continue
			}
			if filepath.Base(hdr.Name) == "mihomo-cli" {
				_, _ = io.Copy(out, tr)
				found = true
				break
			}
		}
		out.Close()
		if !found {
			return fmt.Errorf("%s", T("安装包中未找到二进制"))
		}

		// 替换自身 (系统路径需要 root)
		install := []string{"install", "-m", "0755", binPath, self}
		if os.Geteuid() != 0 {
			install = append([]string{"sudo"}, install...)
		}
		if out, err := exec.Command(install[0], install[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("%s %s\n", T("已安装"), self)
		after, _ := os.Executable()
		_ = after
		return nil
	},
}

func latestCLI(hc *http.Client) (tag, assetURL string, err error) {
	req, _ := http.NewRequest("GET", cliRepoAPI, nil)
	req.Header.Set("User-Agent", "mihomo-cli")
	resp, err := hc.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var r struct {
				TagName string `json:"tag_name"`
				Assets  []struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				} `json:"assets"`
			}
			if json.NewDecoder(resp.Body).Decode(&r) == nil {
				want := "mihomo-cli_linux_" + cliArchAsset() + ".tar.gz"
				for _, a := range r.Assets {
					if a.Name == want {
						return r.TagName, a.BrowserDownloadURL, nil
					}
				}
			}
		}
	}
	// 降级: 解析 latest 重定向拿 tag, 直接构造资产 URL
	rel := core.LatestTag(hc, "wubinstu/mihomo-cli")
	if rel == "" {
		return "", "", fmt.Errorf("%s GitHub (wubinstu/mihomo-cli)", T("访问失败"))
	}
	return rel, fmt.Sprintf("https://github.com/wubinstu/mihomo-cli/releases/download/%s/mihomo-cli_linux_%s.tar.gz", rel, cliArchAsset()), nil
}

func cliArchAsset() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

func init() {
	updateCmd.Flags().StringVar(&updProxy, "proxy", "", T("下载使用的代理 (空=按环境变量/直连)"))
	coreUpgradeCmd.Flags().StringVar(&dlProxy, "proxy", "", T("下载使用的代理 (空=按环境变量/直连)"))
	coreGeoCmd.Flags().StringVar(&dlProxy, "proxy", "", T("下载使用的代理 (空=按环境变量/直连)"))
	rootCmd.AddCommand(updateCmd)
}
