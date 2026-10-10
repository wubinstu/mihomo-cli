package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/cfg"
	"github.com/wubinstu/mihomo-cli/internal/i18n"
	"github.com/wubinstu/mihomo-cli/internal/render"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// configCmd 配置管理。config.toml 由程序管理, 禁止手动编辑 (文件头有警告);
// 它是唯一权威, render 时合成 runtime/config.yaml, 再由内核加载。
var configCmd = &cobra.Command{
	Use:   "config",
	Short: T("配置管理: get/set/reset-default/unset/apply/adopt"),
	Long: T("配置文件 /etc/mihomo-cli/config.toml 由 mihomo-cli 管理与运行时回写, 请勿手动编辑;") + "\n" +
		T("手动改动后执行 config apply 以配置文件覆盖运行中的服务, config adopt 反之;") + "\n" +
		T("撤销某个键的接管用 config unset <key>。") + "\n\n" +
		T("点号路径可读写内核任意配置段:") + "\n" +
		"  mihomo-cli config set tun.enable true\n" +
		"  mihomo-cli config set dns.fake-ip-range 28.0.0.1/8\n" +
		"  mihomo-cli config set dns.nameserver 223.5.5.5,119.29.29.29",
}

// ---- get ----

var configGetCmd = &cobra.Command{
	Use:   "get [key|section]",
	Short: T("查看配置 (无参数 = 全部, 一张表显示 默认值/设置值/运行值)"),
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		live := cfg.Fetch(s)
		if len(args) == 1 {
			arg := args[0]
			if k := cfg.Lookup(arg); k != nil {
				fmt.Println(k.Get(s))
				return nil
			}
			// 段
			if isSection(arg) {
				printConfigSections(s, live, []string{arg})
				return nil
			}
			// 未注册的点号路径 (通用键)
			if v, ok := cfg.GetGeneric(s, arg); ok {
				fmt.Println(v)
				return nil
			}
			return fmt.Errorf("%s %q (mihomo-cli config get --help)", T("未知配置项"), arg)
		}
		sections := []string{"core", "cli", "timer"}
		for _, sec := range cfg.ShownSections(s) {
			if !contains(sections, sec) {
				sections = append(sections, sec)
			}
		}
		printConfigSections(s, live, sections)
		// 设了值但总开关没开的段: 不摆出来, 但要告诉用户"你设过, 只是没启用",
		// 否则用户会以为没保存 (显式 config get <段> 永远能看)
		if hidden := cfg.HiddenSections(s); len(hidden) > 0 {
			fmt.Printf("%s: %s\n", T("已设置但未启用的段"), strings.Join(hidden, ", "))
			fmt.Printf("  %s\n", T("查看: mihomo-cli config get <段>; 启用: mihomo-cli config set <段>.enable true"))
		}
		return nil
	},
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isSection(name string) bool {
	for _, k := range cfg.Keys {
		if k.Section == name {
			return true
		}
	}
	return false
}

// printConfigSections 一张表打印多个配置段 (列宽跨段统一)
func printConfigSections(s *app.Settings, live *cfg.Live, sections []string) {
	secs := []ui.Section{}
	truncated := false
	for _, sec := range sections {
		rows := [][]string{}
		for _, k := range cfg.KeysOf(sec) {
			setting := k.Effective(s)
			if setting == "" && !k.Managed() {
				setting = "-" // 未接管: 不写进内核 yaml, 订阅/内核原样保留
			}
			def := k.Def
			if def == "" {
				def = "/"
			}
			run := live.Value(k)
			state := "-"
			if run != "-" && run != "" && live.Compared(k) {
				if live.Same(k, setting) {
					state = ui.Paint("\x1b[32m", okWord(true))
				} else {
					state = ui.Paint("\x1b[31m", okWord(false))
				}
			}
			// 长值截断, 完整值用 config get <key> (单键输出不截断)
			setting = ui.Truncate(setting, ui.MaxCellWidth, &truncated)
			def = ui.Truncate(def, ui.MaxCellWidth, &truncated)
			run = ui.Truncate(run, ui.MaxCellWidth, &truncated)
			rows = append(rows, []string{k.Name, run, setting, def, state})
		}
		if len(rows) == 0 {
			continue
		}
		secs = append(secs, ui.Section{Title: "[" + cfg.SectionTitles(sec) + "]", Rows: rows})
	}
	if len(secs) == 0 {
		fmt.Println(T("无配置项"))
		return
	}
	ui.TableSections(os.Stdout, []string{"KEY", T("运行值"), T("设置值"), T("默认值"), T("状态")}, secs, 2)
	if truncated {
		fmt.Printf("(%s)\n", T("长值已截断, 完整值: mihomo-cli config get <key>"))
	}
	if !live.Running() {
		fmt.Printf("%s\n", T("服务未运行, 运行值与状态列不可用 (mihomo-cli start)"))
	}
}

// ---- set ----

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: T("修改配置并生效 (有校验, 非法值拒绝写入)"),
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		if len(args) == 1 {
			return configKeyHelp(args[0])
		}
		s := mustSettings()
		return configSet(s, args[0], args[1])
	},
}

// configSet 校验 → 写入 → 生效
func configSet(s *app.Settings, key, value string) error {
	k := cfg.Lookup(key)
	if k == nil {
		// 通用键: 未注册的任意 yaml 路径
		if err := cfg.SetGeneric(s, key, value); err != nil {
			return badValue(key, value, T("任意 yaml 路径"), T("值按字面推断类型: true/false/数字/逗号列表/字符串"))
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s %s\n", key, value, T("已保存"))
		return applyConfig(s, nil)
	}
	// 值域扩展 (dns.nameserver 的预设名/subN) 先跑, 再走形态校验
	if k.Resolve != nil {
		rv, rerr := k.Resolve(s, value)
		if rerr != nil {
			return badValue(key, value, k.Expected(), legalValues(k))
		}
		value = rv
	}
	canon, err := k.Parse(value)
	if err != nil {
		return badValue(key, value, k.Expected(), legalValues(k))
	}
	if err := k.Set(s, canon); err != nil {
		return badValue(key, value, k.Expected(), legalValues(k))
	}
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Printf("%s = %s %s\n", key, canon, T("已保存"))
	warnSectionOff(s, k)
	if k.Name == "cli.language" {
		// 让当次输出立刻用上新语言 (auto = 回到按 $LANG 判断); 命令树的文案也要重翻
		i18n.Reset()
		applyLanguage(rootCmd)
	}
	if k.Name == "cli.github-mirror" {
		_ = render.Generate(s)
	}
	return applyConfig(s, k)
}

// warnSectionOff 设了 dns/tun 的某个键, 但该段总开关没开: 存得下但根本不生效。
// 段可见性改为"只看 enable"之后 (v1.4.2), 这种设置在 config get 里默认看不见,
// 所以必须在这里说清楚, 否则用户会以为设了没生效。
func warnSectionOff(s *app.Settings, k *cfg.Key) {
	switch k.Section {
	case "dns", "tun":
	default:
		return
	}
	if cfg.SectionShown(s, k.Section) {
		return
	}
	fmt.Printf("%s: %s %s\n", T("注意"), k.Section, T("段未启用, 此设置暂不生效"))
	fmt.Printf("  %s: mihomo-cli config set %s.enable true\n", T("启用"), k.Section)
}

// badValue L1 校验失败: 拒绝写入, 并把"改哪个键/填什么"说清楚
func badValue(key, value, expected, legal string) error {
	return fmt.Errorf("%s\n  %s: %s\n  %s: %q\n  %s: %s\n  %s: %s",
		T("无效值, 已拒绝写入"),
		T("配置项"), key,
		T("填入"), value,
		T("期望"), expected,
		T("合法值"), legal)
}

func legalValues(k *cfg.Key) string {
	if len(k.Enum) > 0 {
		return strings.Join(k.Enum, " | ")
	}
	return k.Expected()
}

// applyConfig 让运行态跟上 config.toml
func applyConfig(s *app.Settings, k *cfg.Key) error {
	if k != nil {
		if k.Top() == "tun" {
			if err := ensureTunReady(s, k); err != nil {
				return err
			}
		}
		if k.Section == "timer" {
			if err := sysd.InstallTimers(s); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 更新定时器失败"), err)
			} else {
				fmt.Println(T("定时任务已更新"))
			}
			return nil
		}
	}
	active := sysd.IsActive()
	if k != nil && cfg.HotPatchable(*k) && active {
		if err := api.New(s).PatchConfig(map[string]any{cfg.YAMLPath(k.Name): canonForPatch(s, k)}); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 热切换失败"), err)
			return renderAndApply(s, k)
		}
		fmt.Println(T("已热切换"))
		return verifyApplied(s, k)
	}
	return renderAndApply(s, k)
}

// verifyApplied set 之后核对运行态是否真的跟上, 并明确告诉用户下一步该做什么。
// v1.4.1 之前只打印"已热重载配置", 用户无法判断到底生效没有 (ipv6 那个 bug
// 就是这样被藏了很久)。现在: 跟不上就说清楚是"需要重启"还是"内核不接受"。
func verifyApplied(s *app.Settings, k *cfg.Key) error {
	if k == nil || k.Section == "timer" || k.Section == "cli" || k.Kind == cfg.KindState {
		return nil // 这些键没有可核对的内核运行态
	}
	if !sysd.IsActive() {
		return nil // 服务没起, 没什么可核对
	}
	live := cfg.Fetch(s)
	if !live.Compared(*k) {
		return nil
	}
	if live.Same(*k, k.Effective(s)) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\n%s: %s\n", T("注意"), T("该值已写入配置, 但内核还没用它"))
	if k.Restart {
		fmt.Fprintf(os.Stderr, "  %s\n", T("它需要重建监听, 请执行: mihomo-cli restart"))
	} else {
		fmt.Fprintf(os.Stderr, "  %s\n", T("请执行: mihomo-cli config apply "+k.Name))
	}
	fmt.Fprintf(os.Stderr, "  %s: mihomo-cli config get %s\n", T("核对"), k.Name)
	return nil
}

// canonForPatch PATCH 用的值 (sub = 不下发)
func canonForPatch(s *app.Settings, k *cfg.Key) any {
	v := k.Get(s)
	if v == "sub" || v == "" {
		return nil
	}
	if k.Kind == cfg.KindBool {
		return v == "true"
	}
	return v
}

// renderAndApply 重渲染运行配置并 reload/restart; 渲染失败只警告不覆盖"已保存"的事实
func renderAndApply(s *app.Settings, k *cfg.Key) error {
	if err := render.Generate(s); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 已保存但暂时无法生效"), err)
		return nil
	}
	if !sysd.IsActive() {
		fmt.Println(T("服务未运行, 已跳过热重载 (mihomo-cli start)"))
		return nil
	}
	if k != nil && k.Restart {
		// 端口/TUN 这类需要重启的键: 先试 reload —— reload 不会打断已有连接,
		// 也不会因为「上一个实例还没释放端口」而 fatal 崩溃循环。
		if reloadAndVerify(s, k) {
			fmt.Println(T("已热重载配置"))
			return nil
		}
		if err := sysd.Service("restart"); err != nil {
			return err
		}
		if !serviceUpSoon() {
			fmt.Fprintf(os.Stderr, "%s\n  %s\n  %s\n",
				T("警告: 服务重启后未就绪, 请执行 mihomo-cli log 查看原因"),
				T("如需恢复原值: mihomo-cli config apply <key>"),
				T("原配置备份在 /etc/mihomo-cli/config.toml.pre-1.3.bak"))
			return nil
		}
		fmt.Println(T("已重启服务使配置生效"))
		return verifyApplied(s, k)
	}
	if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v %s\n", T("警告: 热重载失败"), err, T("(可执行 mihomo-cli restart)"))
		return nil
	}
	fmt.Println(T("已热重载配置"))
	return verifyApplied(s, k)
}

// reloadAndVerify reload 后验证运行态是否真的跟上了设置值
func reloadAndVerify(s *app.Settings, k *cfg.Key) bool {
	if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
		return false
	}
	for i := 0; i < 10; i++ {
		live := cfg.Fetch(s)
		if live.Running() && live.Same(*k, k.Effective(s)) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// serviceUpSoon 等待服务进入 running (最多 ~5s)
func serviceUpSoon() bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sysd.IsActive() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return sysd.IsActive()
}

// ---- reset-default ----

var configResetCmd = &cobra.Command{
	Use:   "reset-default [key]",
	Short: T("恢复默认值 (仅对有默认值的配置项生效)"),
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		targets := cfg.Keys
		if len(args) == 1 {
			k := cfg.Lookup(args[0])
			if k == nil {
				return fmt.Errorf("%s %q", T("未知配置项"), args[0])
			}
			targets = []cfg.Key{*k}
		}
		var reset []string
		for _, k := range targets {
			if k.Def == "" {
				continue
			}
			if k.Kind == cfg.KindState {
				continue
			}
			if err := k.Set(s, k.Def); err != nil {
				continue
			}
			reset = append(reset, k.Name+" → "+k.Def)
		}
		if len(reset) == 0 {
			return fmt.Errorf("%s", T("没有可恢复默认值的配置项"))
		}
		if err := s.Save(); err != nil {
			return err
		}
		for _, r := range reset {
			fmt.Println(r)
		}
		if len(args) == 1 {
			return applyConfig(s, cfg.Lookup(args[0]))
		}
		// 全部重置: 端口/模式等都需要重新渲染
		_ = render.Generate(s)
		if sysd.IsActive() {
			_ = sysd.Service("restart")
			fmt.Println(T("已重启服务使配置生效"))
		}
		return sysd.InstallTimersQuiet(s)
	},
}

// ---- unset: 撤销对某个键的接管 (点号路径键回到"跟随订阅") ----

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key...>",
	Short: T("撤销接管: 该键回到跟随订阅/内核默认"),
	Long: T("点号路径键 (tun.* / dns.*) 用 unset 撤销, 该段就不再写进内核 yaml, 订阅原样保留;") + "\n" +
		T("扁平键 (mixed-port 等) 的 unset 等于设置为 sub (跟随订阅), 无默认值的键则清空。"),
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		for _, name := range args {
			k := cfg.Lookup(name)
			if k == nil {
				// 通用键: 直接从 overrides 删除
				if err := cfg.SetGeneric(s, name, "sub"); err != nil {
					return err
				}
				_ = cfg.DeleteGeneric(s, name)
				fmt.Printf("%s %s\n", name, T("已撤销"))
				continue
			}
			if k.Top() != "" {
				if err := cfg.DeleteSectionKey(s, name); err != nil {
					return err
				}
			} else if k.Def != "" {
				_ = k.Set(s, "sub")
			} else {
				_ = k.Set(s, "")
			}
			fmt.Printf("%s %s\n", name, T("已撤销"))
		}
		if err := s.Save(); err != nil {
			return err
		}
		for _, name := range args {
			if k := cfg.Lookup(name); k != nil {
				_ = applyConfig(s, k)
			}
		}
		return nil
	},
}

// ---- apply / adopt (旧名 update-service / update-file, v1.5 删除) ----

var legacyUpdateServiceCmd = &cobra.Command{
	Use:    "update-service [key...]",
	Short:  T("旧名, 等价于 config apply"),
	Hidden: true,
	RunE:   configApplyCmd.RunE,
}

var legacyUpdateFileCmd = &cobra.Command{
	Use:    "update-file [key...]",
	Short:  T("旧名, 等价于 config adopt"),
	Hidden: true,
	RunE:   configAdoptCmd.RunE,
}

var configAdoptCmd = &cobra.Command{
	Use:   "adopt [key...]",
	Short: T("用运行态覆盖 config.toml (可指定键, 兜底手段)"),
	Long: T("把内核当前运行值/定时器实际状态写回 config.toml, 然后重渲染运行配置。") + "\n" +
		T("适用于: 被别人的 systemctl 改过 timer、或想让文件追上现实时。") + "\n" +
		T("日常不需要: config set 已经会立即生效, config get 的 STATE 列就是漂移视图。"),
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		live := cfg.Fetch(s)
		if !live.Running() {
			return fmt.Errorf("%s", T("服务未运行, 没有运行态可读取"))
		}
		keys, err := resolveKeys(args)
		if err != nil {
			return err
		}
		changed, failed := 0, 0
		for _, k := range keys {
			v, err := live.Pull(k)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", k.Name, err)
				failed++
				continue
			}
			if k.Kind == cfg.KindBool && k.Section == "timer" {
				// enabled/disabled → true/false
				v = strings.TrimSuffix(v, "d") // "enabled"→"enable"? 不行, 显式映射
				if v == "enable" {
					v = "true"
				} else {
					v = "false"
				}
			}
			if canon, perr := k.Parse(v); perr == nil {
				v = canon
			}
			old := k.Effective(s)
			if old == v {
				continue
			}
			if err := k.Set(s, v); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", k.Name, err)
				continue
			}
			fmt.Printf("%s: %s → %s\n", k.Name, orDash(old), v)
			changed++
		}
		if changed == 0 {
			if failed > 0 {
				return fmt.Errorf("%s", T("没有可回写的运行值(内核 API 未就绪?)"))
			}
			fmt.Println(T("配置文件与运行态一致, 无需更新"))
			return nil
		}
		if err := s.Save(); err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		return sysd.InstallTimersQuiet(s)
	},
}

var configApplyCmd = &cobra.Command{
	Use:   "apply [key...]",
	Short: T("用 config.toml 覆盖运行态 (可指定键, 兜底手段)"),
	Long: T("按 config.toml 重渲染运行配置并让内核/定时器跟上; 端口等需要重建监听的键会自动重启。") + "\n" +
		T("适用于: 手动编辑过 config.toml, 或改完没生效。") + "\n" +
		T("日常不需要: config set 已经会立即生效。"),
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		keys, err := resolveKeys(args)
		if err != nil {
			return err
		}
		if err := render.Generate(s); err != nil {
			return err
		}
		needRestart := false
		for _, k := range keys {
			if k.Restart || k.Section == "timer" {
				needRestart = true
			}
		}
		if needRestart {
			if err := sysd.InstallTimers(s); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", T("警告: 更新定时器失败"), err)
			}
			if sysd.IsActive() {
				if err := sysd.Service("restart"); err != nil {
					return err
				}
				fmt.Println(T("已重启服务使配置生效"))
			} else {
				fmt.Println(T("服务未运行, 已写入配置文件 (mihomo-cli start)"))
			}
		} else if sysd.IsActive() {
			if err := api.New(s).Reload(app.RuntimeConfig); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v %s\n", T("警告: 热重载失败"), err, T("(可执行 mihomo-cli restart)"))
			} else {
				fmt.Println(T("已热重载配置"))
			}
		} else {
			fmt.Println(T("服务未运行, 已写入配置文件 (mihomo-cli start)"))
		}
		return nil
	},
}

// resolveKeys 全部键或指定键 (未指定 = 除只读状态外的全部)
func resolveKeys(args []string) ([]cfg.Key, error) {
	if len(args) == 0 {
		var out []cfg.Key
		for _, k := range cfg.Keys {
			if k.Kind == cfg.KindState {
				continue
			}
			out = append(out, k)
		}
		return out, nil
	}
	var out []cfg.Key
	for _, a := range args {
		k := cfg.Lookup(a)
		if k == nil {
			return nil, fmt.Errorf("%s %q", T("未知配置项"), a)
		}
		out = append(out, *k)
	}
	return out, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- 单键详情 (config set <key> -h) ----

func configKeyHelp(name string) error {
	k := cfg.Lookup(name)
	if k == nil {
		return genericKeyHelp(name)
	}
	s := mustSettings()
	cur := k.Effective(s)
	if cur == "" {
		cur = k.Def
	}

	fmt.Printf("[%s] %s\n", cfg.SectionTitles(k.Section), k.Name)
	rows := [][2]string{
		{T("说明"), k.LongDesc()},
		{T("当前值"), orDash(cur)},
		{T("默认值"), orDash(k.Def)},
		{T("内核键名"), cfg.YAMLPath(k.Name)},
	}
	// 每个取值的含义 (每个取值一行, 首列留空 -> 自动缩进到取值列)
	if docs := k.ValueDocs(); len(docs) > 0 {
		for i, d := range docs {
			if i == 0 {
				rows = append(rows, [2]string{T("取值"), d.Name + " = " + i18n.T(d.Desc)})
				continue
			}
			rows = append(rows, [2]string{"", d.Name + " = " + i18n.T(d.Desc)})
		}
	} else if len(k.Enum) > 0 {
		rows = append(rows, [2]string{T("可选值"), strings.Join(k.Enum, " | ")})
	} else {
		rows = append(rows, [2]string{T("期望"), k.Expected()})
	}
	// 生效方式
	how := T("热重载配置")
	if k.Restart {
		how = T("优先热重载, 失败才重启服务")
	} else if cfg.HotPatchable(*k) {
		how = T("热切换")
	}
	rows = append(rows,
		[2]string{T("生效方式"), how},
		[2]string{T("用法"), "mihomo-cli config set " + k.Name + " <" + k.Spec() + ">"},
		[2]string{T("恢复默认"), "mihomo-cli config reset-default " + k.Name},
		[2]string{T("撤销接管"), "mihomo-cli config unset " + k.Name},
		[2]string{T("查看"), "mihomo-cli config get " + k.Name},
	)
	fmt.Print(ui.Align2(rows, 2))
	// 段特有的补充表 (DNS 预设), 数据在 cmd 包, 在这里附加
	if extra := keyExtraDoc(k.Name); extra != "" {
		fmt.Println()
		fmt.Print(extra)
	}
	return nil
}

// genericKeyHelp 未注册键 (任意 yaml 路径) 的详版帮助, 排版和注册键一致
func genericKeyHelp(name string) error {
	s := mustSettings()
	cur := T("未设置")
	if v, ok := cfg.GetGeneric(s, name); ok && v != "" {
		cur = v
	}
	fmt.Printf("[extra] %s\n", name)
	rows := [][2]string{
		{T("说明"), T("未注册的内核配置项: 直接读写内核 config.yaml 的这个路径, 不受默认值/校验保护。")},
		{T("当前值"), cur},
		{T("内核键名"), name},
	}
	for i, d := range []struct{ name, desc string }{
		{"true / false", T("布尔")},
		{"123", T("整数")},
		{"1.5", T("小数")},
		{"a,b,c", T("列表 (逗号分隔)")},
		{T("其它"), T("字符串")},
	} {
		if i == 0 {
			rows = append(rows, [2]string{T("取值"), d.name + " = " + d.desc})
			continue
		}
		rows = append(rows, [2]string{"", d.name + " = " + d.desc})
	}
	rows = append(rows,
		[2]string{T("生效方式"), T("热重载配置")},
		[2]string{T("用法"), "mihomo-cli config set " + name + " <value>"},
		[2]string{T("撤销接管"), "mihomo-cli config unset " + name},
		[2]string{T("查看"), "mihomo-cli config get " + name},
	)
	fmt.Print(ui.Align2(rows, 2))
	return nil
}

// keyExtraDoc 某些键的附加说明表 (目前只有 DNS 预设)
func keyExtraDoc(name string) string {
	if name != "dns.nameserver" {
		return ""
	}
	rows := make([][2]string, 0, len(dnsPresets))
	for _, p := range dnsPresets {
		rows = append(rows, [2]string{p.Name, i18n.T(p.Desc) + "  " + strings.Join(p.IPs, ", ")})
	}
	return T("DNS 预设") + ":\n" + ui.Align2(rows, 2)
}

// ---- 补全 ----

func configKeyComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return keyCompletion(args, toComplete)
}

// noFileComp 统一 directive: 全命令树禁止文件补全 (mihomo-cli 不与文件打交道)
func noFileComp() cobra.ShellCompDirective { return cobra.ShellCompDirectiveNoFileComp }

func configValueComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 1 {
		return nil, noFileComp()
	}
	k := cfg.Lookup(args[0])
	if k == nil {
		return nil, noFileComp()
	}
	return prefixFilter(k.CompleteValues(), toComplete), noFileComp()
}

func prefixFilter(list []string, prefix string) []string {
	var out []string
	for _, v := range list {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// okWord 勾/叉的文字 (颜色由调用方加)
func okWord(ok bool) string {
	if ok {
		return "✔"
	}
	return "✘"
}

// sortedKeys 键名排序 (补全/文档用)
func sortedKeys() []string {
	out := cfg.Names()
	sort.Strings(out)
	return out
}

func init() {
	configSetCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if pos := cmd.Flags().Args(); len(pos) > 0 {
			_ = configKeyHelp(pos[0])
			return
		}
		fmt.Println(configSetCmd.Long)
	})
	configGetCmd.ValidArgsFunction = configKeyComp
	configResetCmd.ValidArgsFunction = configKeyComp
	configSetCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// 上一个参数以 "." 结尾说明还在敲键名(半截点号路径), 不能走补值分支
		if len(args) == 0 || strings.HasSuffix(args[len(args)-1], ".") {
			return configKeyComp(cmd, args, toComplete)
		}
		return configValueComp(cmd, args, toComplete)
	}
	configUnsetCmd.ValidArgsFunction = configKeyComp
	for _, c := range []*cobra.Command{configAdoptCmd, configApplyCmd, legacyUpdateFileCmd, legacyUpdateServiceCmd} {
		c.ValidArgsFunction = configKeyComp
	}
	// 旧名 (v1.4.1 之前): 隐藏, 但还能用, 帮助里指向新名
	for old, new := range map[*cobra.Command]*cobra.Command{
		legacyUpdateFileCmd: configAdoptCmd, legacyUpdateServiceCmd: configApplyCmd,
	} {
		old.Hidden = true
		old.Short = T("旧名, 等价于") + " " + new.Name()
	}
	// set 的长帮助由注册表生成: 新增键只需改表格, 帮助/补全/校验自动跟着变
	configSetCmd.Long = buildSetHelp()
	for _, c := range []*cobra.Command{configSetCmd, configResetCmd, configApplyCmd, configAdoptCmd, legacyUpdateFileCmd, legacyUpdateServiceCmd} {
		markMutating(c)
	}
	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd, configUnsetCmd, configApplyCmd, configAdoptCmd,
		legacyUpdateFileCmd, legacyUpdateServiceCmd)
	rootCmd.AddCommand(configCmd)
}

// buildSetHelp 从注册表生成 config set 的分组帮助 (单一事实来源)。
// 简版: 一行一个键 = 键名 <紧凑取值> + 一句话说明 + [默认 x]; 取值列超长截断,
// 完整取值与每个值的含义只看详版 (config set <key> -h)。
func buildSetHelp() string {
	var b strings.Builder
	b.WriteString(T("修改配置并立即生效; 非法值拒绝写入。裸命令执行显示本帮助, <key> -h 显示单键详情。") + "\n")
	b.WriteString(T("键名统一为 <段>.<键>: core=内核顶层键 / cli=CLI 自身 / timer=systemd / dns,tun=内核同名段。") + "\n")
	for _, sec := range []string{"core", "cli", "timer", "dns", "tun"} {
		keys := cfg.KeysOf(sec)
		if len(keys) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n-- %s --\n", cfg.SectionTitles(sec))
		rows := make([][2]string, 0, len(keys))
		for _, k := range keys {
			head := k.Name
			if spec := k.Spec(); spec != "" {
				head += " <" + spec + ">"
			}
			desc := k.Desc()
			if k.Def != "" {
				desc += "  [" + T("默认") + " " + k.Def + "]"
			}
			rows = append(rows, [2]string{head, desc})
		}
		b.WriteString(ui.Align2(rows, 2))
	}
	b.WriteString("\n" + T("未注册的内核配置项(任意 yaml 路径)也可写, 值按字面推断类型:") + "\n")
	b.WriteString(ui.ExampleLines([][2]string{
		{"mihomo-cli config set sniffer.enable true", T("开启域名嗅探")},
		{"mihomo-cli config set geodata-mode false", T("用 metadb 而不是 dat")},
		{"mihomo-cli config set dns.fake-ip-range 28.0.0.1/8", T("改 Fake-IP 段")},
	}, 2))
	return b.String()
}

func defSuffix(k cfg.Key) string {
	if k.Def == "" {
		return ""
	}
	return " [" + T("默认") + " " + k.Def + "]"
}
