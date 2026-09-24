package render

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/subs"
	"gopkg.in/yaml.v3"
)

// DeepMerge 将 src 合并覆盖到 dst (src 优先, 递归合并 map, 其余类型直接覆盖)
func DeepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok2 := dst[k].(map[string]any); ok2 {
				DeepMerge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

// Generate 将「当前订阅 + overrides.yaml + 运行时注入项」合成为 runtime/config.yaml
// 无生效订阅时生成空配置(mode: direct): 服务保持运行, 全部流量直连
func Generate(s *app.Settings) error {
	if err := app.EnsureDirs(); err != nil {
		return err
	}
	if s.Current() == nil {
		minimal := map[string]any{
			"mixed-port":           s.MixedPort,
			"allow-lan":            s.AllowLan,
			"mode":                 "direct",
			"log-level":            "info",
			"external-controller":  "127.0.0.1:9090",
			"secret":               s.APISecret,
		}
		if s.AllowLan {
			minimal["bind-address"] = "*"
		}
		out, err := yaml.Marshal(minimal)
		if err != nil {
			return err
		}
		return os.WriteFile(app.RuntimeConfig, out, 0o644)
	}
	p := s.Current()
	if p == nil {
		return fmt.Errorf("%s", i18n.T("没有可用订阅, 请先执行 mihomo-cli init 或 mihomo-cli sub add"))
	}
	data, err := os.ReadFile(subs.Path(p.Name))
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("读取订阅文件失败"), err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("订阅配置解析失败"), err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	// 用户覆盖层
	if ov, err := os.ReadFile(app.OverridesFile); err == nil {
		var m map[string]any
		if yaml.Unmarshal(ov, &m) == nil && m != nil {
			DeepMerge(cfg, m)
		}
	}

	// geo 数据镜像 (内核更新 geodata 时不再直连 GitHub; 用户 overrides 可覆盖)
	if _, ok := cfg["geox-url"]; !ok {
		if _, need := cfg["rules"]; need {
			cfg["geox-url"] = map[string]any{
				"geoip":   "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
				"geosite": "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
				"mmdb":    "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
				"asn":     "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
			}
		}
	}

	// cli 运行时注入层(强制)
	cfg["external-controller"] = "127.0.0.1:9090"
	cfg["secret"] = s.APISecret
	cfg["log-level"] = "info"
	cfg["mixed-port"] = s.MixedPort
	if s.SocksPort > 0 {
		cfg["socks-port"] = s.SocksPort
	} else {
		delete(cfg, "socks-port")
	}
	if s.HTTPPort > 0 {
		cfg["port"] = s.HTTPPort
	} else {
		delete(cfg, "port")
	}
	delete(cfg, "redir-port")
	cfg["allow-lan"] = s.AllowLan
	if s.AllowLan {
		cfg["bind-address"] = "*"
	}
	if s.ProxyMode != "" {
		cfg["mode"] = s.ProxyMode
	}
	if s.IPV6Enabled {
		cfg["ipv6"] = true
	}
	if s.LogLevel != "" {
		cfg["log-level"] = s.LogLevel
	}
	if s.TCPConcurrent != nil {
		cfg["tcp-concurrent"] = *s.TCPConcurrent
	}
	if s.UnifiedDelay != nil {
		cfg["unified-delay"] = *s.UnifiedDelay
	}
	if s.KeepAliveInterval != nil {
		cfg["keep-alive-interval"] = *s.KeepAliveInterval
	}
	// 用户规则优先: 置于订阅规则之前 (mihomo 首条匹配即生效)
	if enabled := s.EnabledRules(); len(enabled) > 0 {
		subRules := []string{}
		if raw, ok := cfg["rules"].([]any); ok {
			for _, r := range raw {
				if rs, ok := r.(string); ok {
					subRules = append(subRules, rs)
				}
			}
		}
		merged := append(append([]string{}, enabled...), subRules...)
		cfg["rules"] = merged
	}
	// 自定义 DNS: 覆盖 nameserver; default-nameserver 必须为纯 IP (内核要求), DoH 时用内置 IP
	if len(s.DNSServers) > 0 {
		dns, _ := cfg["dns"].(map[string]any)
		if dns == nil {
			dns = map[string]any{}
		}
		dns["enable"] = true
		dns["nameserver"] = s.DNSServers
		defaultNS := []string{"223.5.5.5", "119.29.29.29"}
		hasIP := false
		for _, ns := range s.DNSServers {
			if net.ParseIP(strings.Split(strings.Split(ns, "//")[len(strings.Split(ns, "//"))-1], ":")[0]) != nil && !strings.Contains(ns, "://") {
				hasIP = true
				break
			}
		}
		if hasIP {
			defaultNS = s.DNSServers
		}
		dns["default-nameserver"] = defaultNS
		cfg["dns"] = dns
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(app.RuntimeConfig, out, 0o644)
}
