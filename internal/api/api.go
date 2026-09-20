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
		return fmt.Errorf("无法连接 mihomo API(服务是否已启动?): %w", err)
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
		return fmt.Errorf("切换失败 %d: %s", resp.StatusCode, string(b))
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

// Reload 热重载配置文件
func (c *Client) Reload(path string) error {
	resp, err := c.do("PUT", "/configs?force=true", map[string]string{"path": path})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("热重载失败 %d: %s", resp.StatusCode, string(b))
	}
	return nil
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
