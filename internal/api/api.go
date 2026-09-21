package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
)

type Client struct {
	base   string
	secret string
	hc     *http.Client
}

func New(s *app.Settings) *Client {
	return &Client{
		base:   s.APIBase,
		secret: s.APISecret,
		hc:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.hc.Do(req)
}

func (c *Client) GetJSON(path string, out any) error {
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("无法连接 mihomo API(服务是否已启动?)"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("API %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Proxy 单个代理(节点或分组)
type Proxy struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Now  string `json:"now"`
	All  []string `json:"all"`
	UDP  bool   `json:"udp"`
}

type ProxiesResp struct {
	Proxies map[string]Proxy `json:"proxies"`
}

func (c *Client) Proxies() (*ProxiesResp, error) {
	var r ProxiesResp
	if err := c.GetJSON("/proxies", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) SetProxy(group, node string) error {
	resp, err := c.do("PUT", "/proxies/"+url.PathEscape(group), map[string]string{"name": node})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %d: %s", i18n.T("切换失败"), resp.StatusCode, string(b))
	}
	return nil
}

// GroupDelay 对分组内所有节点测延迟, 返回 节点->延迟ms
func (c *Client) GroupDelay(group, testURL string, timeoutMs int) (map[string]int, error) {
	var m map[string]int
	path := fmt.Sprintf("/group/%s/delay?url=%s&timeout=%d",
		url.PathEscape(group), url.QueryEscape(testURL), timeoutMs)
	if err := c.GetJSON(path, &m); err != nil {
		return nil, err
	}
	return m, nil
}

type ConnInfo struct {
	ID      string `json:"id"`
	Metadata struct {
		Network     string `json:"network"`
		Type        string `json:"type"`
		SourceIP    string `json:"sourceIP"`
		Destination string `json:"destinationIP"`
		Host        string `json:"host"`
		Process     string `json:"process"`
		ProcessPath string `json:"processPath"`
	} `json:"metadata"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Start    string `json:"start"`
	Chains   []string `json:"chains"`
	Rule     string `json:"rule"`
}

type ConnsResp struct {
	DownloadTotal int64     `json:"downloadTotal"`
	UploadTotal   int64     `json:"uploadTotal"`
	Connections   []ConnInfo `json:"connections"`
}

func (c *Client) Connections() (*ConnsResp, error) {
	var r ConnsResp
	if err := c.GetJSON("/connections", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) CloseConns() error {
	resp, err := c.do("DELETE", "/connections", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// CloseConn 关闭指定连接
func (c *Client) CloseConn(id string) error {
	req, err := http.NewRequest("DELETE", c.base+"/connections/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("close %d", resp.StatusCode)
	}
	return nil
}

// Reload 热重载配置文件
func (c *Client) Reload(path string) error {
	resp, err := c.do("PUT", "/configs?force=true", map[string]string{"path": path})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %d: %s", i18n.T("警告: 热重载失败"), resp.StatusCode, string(b))
	}
	return nil
}

// SetMode 热切换代理模式 (rule/global/direct)
func (c *Client) SetMode(mode string) error {
	resp, err := c.do("PATCH", "/configs", map[string]string{"mode": mode})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %d: %s", i18n.T("切换失败"), resp.StatusCode, string(b))
	}
	return nil
}

// LastDelay 取节点最近一次历史延迟 (ms), 无历史返回 -1
func (c *Client) LastDelay(name string) int {
	var p struct {
		Extra struct {
			History []struct {
				Delay int `json:"delay"`
			} `json:"history"`
		} `json:"extra"`
	}
	if err := c.GetJSON("/proxies/"+url.PathEscape(name), &p); err != nil || len(p.Extra.History) == 0 {
		return -1
	}
	return p.Extra.History[len(p.Extra.History)-1].Delay
}

// ConfigMode 读取内核当前代理模式
func (c *Client) ConfigMode() (string, error) {
	var cfg struct {
		Mode string `json:"mode"`
	}
	if err := c.GetJSON("/configs", &cfg); err != nil {
		return "", err
	}
	return cfg.Mode, nil
}

// Stream 返回流式响应(用于 /traffic)
func (c *Client) Stream(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}

func (c *Client) Version() (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := c.GetJSON("/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}
