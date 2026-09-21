// Package geo 基于 geoip.metadb 将节点服务器地址解析为地区名 (带本地缓存)
package geo

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"github.com/wubinstu/mihomo-cli/internal/app"
)

type cacheEntry struct {
	ISO string    `json:"iso"`
	TS  time.Time `json:"ts"`
}

var (
	mu    sync.Mutex
	cache = map[string]cacheEntry{}
	once  sync.Once
)

func cachePath() string { return filepath.Join(app.RuntimeDir, "node-region.json") }

func loadCache() {
	once.Do(func() {
		if data, err := os.ReadFile(cachePath()); err == nil {
			_ = json.Unmarshal(data, &cache)
		}
	})
}

func saveCache() {
	if data, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(cachePath(), data, 0o644)
	}
}

// Region 返回服务器地址对应地区 (ISO 国家码); 解析失败返回 ""
func Region(server string) string {
	if server == "" || net.ParseIP(server) == nil && !isDomainish(server) {
		return ""
	}
	loadCache()
	mu.Lock()
	if e, ok := cache[server]; ok && time.Since(e.TS) < 24*time.Hour {
		mu.Unlock()
		return e.ISO
	}
	mu.Unlock()

	ip := server
	if net.ParseIP(server) == nil {
		resolved := resolve(server)
		if resolved == "" {
			return ""
		}
		ip = resolved
	}
	iso := lookupISO(ip)
	mu.Lock()
	cache[server] = cacheEntry{iso, time.Now()}
	saveCache()
	mu.Unlock()
	return iso
}

func isDomainish(s string) bool {
	for _, r := range s {
		if r == '.' {
			return true
		}
	}
	return false
}

var resolver = &net.Resolver{}

func context2() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func resolve(host string) string {
	ctx, cancel := context2()
	defer cancel()
	addrs, err := resolver.LookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

func lookupISO(ipStr string) string {
	db, err := maxminddb.Open(filepath.Join(app.RuntimeDir, "geoip.metadb"))
	if err != nil {
		return ""
	}
	defer db.Close()
	var rec any
	if err := db.Lookup(net.ParseIP(ipStr), &rec); err != nil {
		return ""
	}
	// v2ray 风格记录: ["iso", "org"] / ["org"] / 裸字符串
	switch v := rec.(type) {
	case string:
		return strings.ToUpper(v)
	case []any:
		iso := ""
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				continue
			}
			if len(s) == 2 {
				iso = strings.ToUpper(s)
				break
			}
			if iso == "" {
				iso = strings.ToUpper(s) // 无两字母码时用 org 名 (如 CLOUDFLARE)
			}
		}
		return iso
	}
	return ""
}
