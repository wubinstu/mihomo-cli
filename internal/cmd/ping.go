package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/i18n"

	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// pingSite 检测站点: 延迟 + 可达性/解锁启发式判定
type pingSite struct {
	// Name 是翻译 key (包初始化时语言还是中文), 渲染时用 siteName 再翻一次
	Name string
	URL  string
	// OK 自定义"这个状态码算可用"的判定 (nil = 2xx/3xx 都算)。
	// Docker Hub 的 registry 匿名访问就是回 401, 那是正常的鉴权挑战, 不是被墙。
	OK func(code int) bool
}

var pingSites = []pingSite{
	{"GitHub", "https://api.github.com", nil},
	{"Docker Hub", "https://registry-1.docker.io/v2/", func(code int) bool {
		return code == 200 || code == 401
	}},
	{"Apple", "https://www.apple.com", nil},
	{"Google", "https://www.google.com/generate_204", nil},
	{"YouTube", "https://www.youtube.com/generate_204", nil},
	{T("哔哩哔哩大陆"), "https://www.bilibili.com", nil},
	{T("哔哩哔哩港澳台"), "https://www.bilibili.com", func(code int) bool {
		// 大陆 IP 会被重定向到大陆版, 拿不到港澳台内容; 这里只能报"通不通"
		return code == 200
	}},
	{"ChatGPT Web", "https://ios.chat.openai.com/public-api/mobile/server_status/v1", nil},
	{"Claude", "https://claude.ai", nil},
	{"Gemini", "https://gemini.google.com", nil},
	{"Netflix", "https://www.netflix.com/title/81280792", nil},
	{"Disney+", "https://www.disneyplus.com", nil},
	{"Prime Video", "https://www.primevideo.com", nil},
	{"Spotify", "https://www.spotify.com", nil},
	{"TikTok", "https://www.tiktok.com", nil},
}

var pingTimeout = 8 * time.Second

var pingCmd = &cobra.Command{
	Use:   "ping [site...]",
	Short: T("站点延迟与可用性检测 (经当前代理; 解锁判定为启发式)"),
	Long: T("经本机代理端口访问站点, 输出 HTTP 状态与延迟; 2xx/3xx 视为可用, 403/451 视为地区受限。") + "\n" +
		T("解锁判定为启发式 (仅看 HTTP 状态码), 仅供参考。"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		sites := pingSites
		if len(args) > 0 {
			var sel []pingSite
			for _, a := range args {
				for _, st := range pingSites {
					if strings.EqualFold(st.Name, a) || strings.Contains(strings.ToLower(st.Name), strings.ToLower(a)) {
						sel = append(sel, st)
					}
				}
			}
			if len(sel) == 0 {
				return fmt.Errorf("%s: %s", T("未知站点"), strings.Join(args, ","))
			}
			sites = sel
		}
		proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", s.ProxyPort())
		tr := &http.Transport{
			Proxy:           http.ProxyURL(mustParseURL(proxyAddr)),
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		client := &http.Client{Timeout: pingTimeout, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		}}

		type result struct {
			name string
			code int
			ms   int64
			err  string
		}
		results := make([]result, len(sites))
		var wg sync.WaitGroup
		for i, st := range sites {
			wg.Add(1)
			go func(i int, st pingSite) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
				req, _ := http.NewRequestWithContext(ctx, "GET", st.URL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (mihomo-cli ping)")
				start := time.Now()
				resp, err := client.Do(req)
				ms := time.Since(start).Milliseconds()
				cancel()
				if err != nil {
					results[i] = result{siteName(&st), 0, ms, "timeout"}
					return
				}
				io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				resp.Body.Close()
				results[i] = result{siteName(&st), resp.StatusCode, ms, ""}
			}(i, st)
		}
		wg.Wait()

		rows := [][]string{{T("站点"), "HTTP", T("延迟"), T("状态")}}
		for i, r := range results {
			status, delay, color := T("超时"), ui.ColorDelay(-1), "\x1b[90m"
			switch {
			case r.err != "":
			case r.code == 403 || r.code == 451:
				status = T("受限")
				delay = ui.ColorDelay(4000)
				color = "\x1b[31m"
			case siteOK(&sites[i], r.code):
				status = T("可用")
				delay = ui.ColorDelay(int(r.ms))
				color = "\x1b[32m"
			default:
				status = fmt.Sprintf("HTTP %d", r.code)
				delay = ui.ColorDelay(3200)
				color = "\x1b[33m"
			}
			status = color + status + ui.ColorReset
			rows = append(rows, []string{r.name, itoa(r.code), delay, status})
		}
		ui.Table(os.Stdout, rows, 2)
		fmt.Println(T("说明: 可用=HTTP 2xx/3xx (Docker Hub 的 401 是匿名鉴权挑战, 也算通), 受限=403/451(地区限制), 判定为启发式仅供参考"))
		return nil
	},
}

// siteName 站点显示名 (跟随当前语言)
func siteName(st *pingSite) string { return i18n.T(st.Name) }

// siteOK 该站点的这个状态码算不算"可用"
func siteOK(st *pingSite, code int) bool {
	if st.OK != nil {
		return st.OK(code)
	}
	return code >= 200 && code < 400
}

func itoa(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func init() {
	rootCmd.AddCommand(pingCmd)
}

func mustParseURL(u string) *url.URL {
	p, _ := url.Parse(u)
	return p
}
