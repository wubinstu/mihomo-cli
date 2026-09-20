package render

import (
	"fmt"
	"os"

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
func Generate(s *app.Settings) error {
	p := s.Current()
	if p == nil {
		return fmt.Errorf("%s", i18n.T("没有可用订阅, 请先执行 mihomo-cli init 或 mihomo-cli sub add"))
	}
	data, err := os.ReadFile(subs.Path(p.Name))
	if err != nil {
		return fmt.Errorf("读取订阅文件失败: %w", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("订阅配置解析失败: %w", err)
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

	// cli 运行时注入层(强制)
	cfg["external-controller"] = "127.0.0.1:9090"
	cfg["secret"] = s.APISecret
	cfg["log-level"] = "info"
	cfg["mixed-port"] = s.MixedPort
	delete(cfg, "port")
	delete(cfg, "socks-port")
	delete(cfg, "redir-port")
	cfg["allow-lan"] = s.AllowLan
	if s.AllowLan {
		cfg["bind-address"] = "*"
	}
	if s.ProxyMode != "" {
		cfg["mode"] = s.ProxyMode
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := app.EnsureDirs(); err != nil {
		return err
	}
	return os.WriteFile(app.RuntimeConfig, out, 0o600)
}
