package render

import (
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/subs"
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

// minimalConfig 无生效订阅时的最小配置: mode direct, 全部流量直连, 服务保持运行
func minimalConfig(s *app.Settings) map[string]any {
	m := map[string]any{
		"allow-lan":           s.AllowLan,
		"mode":                "direct",
		"log-level":           "info",
		"external-controller": "127.0.0.1:9090",
		"secret":              s.APISecret,
	}
	injectPorts(m, s)
	if s.AllowLan {
		m["bind-address"] = "*"
	}
	return m
}

// injectPorts 写入三个代理端口; off/sub 一律不写 (交由订阅或内核默认)
func injectPorts(cfgMap map[string]any, s *app.Settings) {
	if n, ok := app.PortNum(s.MixedPort); ok {
		cfgMap["mixed-port"] = n
	} else {
		delete(cfgMap, "mixed-port")
	}
	if n, ok := app.PortNum(s.SocksPort); ok {
		cfgMap["socks-port"] = n
	} else {
		delete(cfgMap, "socks-port")
	}
	if n, ok := app.PortNum(s.HTTPPort); ok {
		cfgMap["port"] = n
	} else {
		delete(cfgMap, "port")
	}
	delete(cfgMap, "redir-port")
}

// Generate 将「当前订阅 + [overrides] + 运行时注入项」合成为 runtime/config.yaml
// 无生效订阅时生成空配置(mode: direct): 服务保持运行, 全部流量直连
func Generate(s *app.Settings) error {
	if err := app.EnsureDirs(); err != nil {
		return err
	}
	cfgMap := map[string]any{}
	if s.Current() == nil {
		out, err := yaml.Marshal(minimalConfig(s))
		if err != nil {
			return err
		}
		return os.WriteFile(app.RuntimeConfig, out, 0o640)
	}
	p := s.Current()
	data, err := os.ReadFile(subs.Path(p.Name))
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("读取订阅文件失败"), err)
	}
	if err := yaml.Unmarshal(data, &cfgMap); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("订阅配置解析失败"), err)
	}
	if cfgMap == nil {
		cfgMap = map[string]any{}
	}

	// 点号路径写入的配置段: 深合并进订阅 (只用过的段才出现, 未用过的完全不碰订阅)
	for _, sec := range []string{"dns", "tun"} {
		if m := cfg.OverrideSection(s, sec); m != nil {
			DeepMerge(cfgMap, map[string]any{sec: m})
		}
	}

	// geo 数据镜像 (内核更新 geodata 时不再直连 GitHub; 用户 overrides 可覆盖)
	if _, ok := cfgMap["geox-url"]; !ok {
		if _, need := cfgMap["rules"]; need {
			cfgMap["geox-url"] = map[string]any{
				"geoip":   "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
				"geosite": "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
				"mmdb":    "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
				"asn":     "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
			}
		}
	}

	// cli 运行时注入层(强制)
	cfgMap["external-controller"] = "127.0.0.1:9090"
	cfgMap["secret"] = s.APISecret
	injectPorts(cfgMap, s)
	cfgMap["allow-lan"] = s.AllowLan
	if s.AllowLan {
		cfgMap["bind-address"] = "*"
	} else {
		delete(cfgMap, "bind-address")
	}
	// 三态键: "sub" = 不注入(跟随订阅); 空值 = 默认值 (CLI 权威, 照样注入)
	for _, name := range []string{"proxy-mode", "log-level", "tcp-concurrent", "unified-delay", "keep-alive-interval"} {
		k := cfg.Lookup(name)
		if k == nil {
			continue
		}
		v := k.Effective(s)
		if v == "" || v == "sub" {
			continue
		}
		if k.Kind == cfg.KindInt {
			if n, ok := app.PortNum(v); ok && n > 0 {
				cfgMap[cfg.YAMLPath(name)] = n
			}
			continue
		}
		cfgMap[cfg.YAMLPath(name)] = triValue(k, v)
	}
	// 用户规则优先: 置于订阅规则之前 (mihomo 首条匹配即生效)
	if enabled := s.EnabledRules(); len(enabled) > 0 {
		subRules := []string{}
		if raw, ok := cfgMap["rules"].([]any); ok {
			for _, r := range raw {
				if rs, ok := r.(string); ok {
					subRules = append(subRules, rs)
				}
			}
		}
		merged := append(append([]string{}, enabled...), subRules...)
		cfgMap["rules"] = merged
	}
	// 自定义 DNS: 覆盖 nameserver; default-nameserver 必须为纯 IP (内核要求), DoH 时用内置 IP
	if len(s.DNSServers) > 0 || dnsSectionActive(s) {
		injectDNS(cfgMap, s)
	}

	out, err := yaml.Marshal(cfgMap)
	if err != nil {
		return err
	}
	return os.WriteFile(app.RuntimeConfig, out, 0o640)
}

// dnsSectionActive dns 段是否被显式开启 (config set dns.enable true)
func dnsSectionActive(s *app.Settings) bool {
	m := cfg.OverrideSection(s, "dns")
	if m == nil {
		return false
	}
	on, _ := m["enable"].(bool)
	return on
}

// injectDNS 组装 dns 段: 顶层键由注册表提供, cli 侧的 nameserver 优先
func injectDNS(cfgMap map[string]any, s *app.Settings) {
	dns, _ := cfgMap["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	dns["enable"] = true
	if len(s.DNSServers) > 0 {
		dns["nameserver"] = toAnyList(s.DNSServers)
	}
	// default-nameserver 必须纯 IP: 自定义 nameserver 全是 DoH/DoT 时用内置 IP
	if _, ok := dns["default-nameserver"]; !ok {
		ns := s.DNSServers
		hasIP := false
		for _, n := range ns {
			if net.ParseIP(strings.Split(strings.Split(n, "//")[len(strings.Split(n, "//"))-1], ":")[0]) != nil && !strings.Contains(n, "://") {
				hasIP = true
				break
			}
		}
		if hasIP && len(ns) > 0 {
			dns["default-nameserver"] = toAnyList(ns)
		} else {
			dns["default-nameserver"] = toAnyList([]string{"223.5.5.5", "119.29.29.29"})
		}
	}
	cfgMap["dns"] = dns
}

func toAnyList(items []string) []any {
	out := make([]any, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

// triValue 布尔三态键的类型转换
func triValue(k *cfg.Key, v string) any {
	if k.Kind == cfg.KindBool || k.Kind == cfg.KindTri {
		return v == "true"
	}
	return v
}
