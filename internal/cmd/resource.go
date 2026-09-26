package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

var resProxy string

// resourceCmd 资源管理: 内核二进制 + geo 数据
var resourceCmd = &cobra.Command{
	Use:   "resource",
	Short: T("资源管理: core/mmdb/asn/geoip/geosite"),
	Long: T("管理内核与 geo 数据资源; 数据资源支持自动更新 (resource-auto-update-*)。") + `

mihomo-cli resource core version                 # ` + T("已安装内核版本") + `
mihomo-cli resource core upgrade [<spec>]        # ` + T("切换内核版本 (规格同 install --core, 可新可旧)") + `
mihomo-cli resource core rollback                # ` + T("回滚到上一个装过的版本 (本地版本栈, 不联网)") + `
mihomo-cli resource core history                 # ` + T("查看本地版本栈") + `
mihomo-cli resource mmdb|asn|geoip|geosite info|update
mihomo-cli resource update-all                   # ` + T("更新全部数据资源 (定时任务复用)") + `

` + T("内核规格 <spec> = [风味][:版本], 平台自动探测并被记住:") + `
  auto                       ` + T("自动探测风味, 装最新版") + `
  v1.19.31                   ` + T("指定版本, 沿用已记住的平台与风味") + `
  compatible:v1.19.19        ` + T("风味与版本都指定") + `
  ` + T("省略参数") + `                   ` + T("沿用已记住的平台与风味 + 最新版") + `
`,
}

// ---- core ----

var resourceCoreCmd = &cobra.Command{
	Use:   "core",
	Short: "mihomo " + T("内核二进制"),
}

var resCoreVersionCmd = &cobra.Command{
	Use:   "version",
	Short: T("已安装内核版本"),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := core.Version()
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	},
}

var resCoreUpgradeCmd = &cobra.Command{
	Use:   "upgrade [<spec>]",
	Short: T("切换内核版本 (规格同 install --core, 可新可旧)"),
	Long: T("广义的更新: 接受任意版本号, 哪怕比当前旧, 只要规格与当前不同就换。") + "\n" +
		T("平台与风味沿用 config.toml 里记住的值 (install 时探测写入), 也可用 <spec> 覆盖。"),
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli resource core upgrade)", T("需要 root 权限"))
		}
		s, err := app.LoadSettings()
		if err != nil {
			return err
		}
		spec := ""
		if len(args) == 1 {
			spec = args[0]
		}
		flavor, version, err := core.ParseCoreSpec(spec)
		if err != nil {
			return err
		}
		res, err := core.Upgrade(s, flavor, version, resProxy, mirrorOf(s))
		if err != nil {
			return err
		}
		fmt.Printf("%s %s [%s / %s]\n", T("已切换内核到"), res.Version,
			res.Platform, flavorText(res.Flavor))
		if res.Degraded {
			fmt.Printf("%s\n", T("提示: 已自动降级到可在本机运行的内核构建"))
		}
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		fmt.Println(T("服务未运行 (mihomo-cli start)"))
		return nil
	},
}

var resCoreRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: T("回滚到上一个装过的版本 (本地版本栈, 不联网)"),
	Long: T("纯本地行为: 从版本栈出栈一个版本并装回, 不需要网络。") + "\n" +
		T("新内核把代理链路搞坏时, 网络可能正走在坏掉的内核上, 离线才能可靠回退。"),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("%s (sudo mihomo-cli resource core rollback)", T("需要 root 权限"))
		}
		v, err := core.Rollback()
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", T("已回滚内核到"), v)
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		fmt.Println(T("服务未运行 (mihomo-cli start)"))
		return nil
	},
}

var resCoreHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: T("查看本地内核版本栈"),
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		h := core.History()
		cur := core.VersionShort()
		rows := [][]string{{"#", T("版本"), T("风味"), T("安装时间"), T("位置")}}
		if cur != "" {
			rows = append(rows, []string{"*", cur, currentFlavorText(), "-", T("当前")})
		}
		for i, v := range h {
			rows = append(rows, []string{fmt.Sprintf("%d", i+1), v.Version,
				orFlavor(v.Flavor), v.InstalledAt.Format("2006-01-02 15:04"), T("回滚可用")})
		}
		if len(rows) == 1 {
			fmt.Println(T("版本栈为空 (至少经历一次升级/切换后才可回滚)"))
			return nil
		}
		ui.Table(os.Stdout, rows, 2)
		return nil
	},
}

func currentFlavorText() string {
	s, err := app.LoadSettings()
	if err != nil {
		return "-"
	}
	return orFlavor(s.CoreFlavor)
}

func orFlavor(f string) string {
	if f == "" {
		return T("官方默认")
	}
	return f
}

// ---- 数据资源 ----

func resourceRun(name string) *cobra.Command {
	var info = &cobra.Command{
		Use:   "info",
		Short: T("查看资源状态"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ok, mt, size := core.ResourceInfo(name)
			rows := [][]string{{"RESOURCE", T("已安装"), T("更新时间"), T("大小")}}
			inst, ts, sz := T("未安装"), "-", "-"
			if ok {
				inst, ts, sz = T("已安装"), mt.Format("2006-01-02 15:04"), humanBytes(size)
			}
			rows = append(rows, []string{name, inst, ts, sz})
			ui.Table(os.Stdout, rows, 2)
			fmt.Println(path)
			return nil
		},
	}
	var update = &cobra.Command{
		Use:   "update",
		Short: T("下载/更新资源"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 {
				return fmt.Errorf("%s (sudo mihomo-cli resource %s update)", T("需要 root 权限"), name)
			}
			s := mustSettings()
			if err := core.UpdateResource(name, resProxy, mirrorOf(s)); err != nil {
				return err
			}
			s.ResourceLastRun = time.Now()
			_ = s.Save()
			fmt.Println(T("已下载"))
			return nil
		},
	}
	c := &cobra.Command{
		Use:   name,
		Short: name + " " + T("数据资源"),
		Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}
	markMutating(update)
	c.AddCommand(info, update)
	return c
}

// ---- update-all (定时任务) ----

var resourceUpdateAllCmd = &cobra.Command{
	Use:   "update-all",
	Short: T("更新全部数据资源 (mmdb/asn/geoip/geosite)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if err := core.UpdateAllResources(resProxy, mirrorOf(s)); err != nil {
			return err
		}
		s.ResourceLastRun = time.Now()
		_ = s.Save()
		fmt.Println(T("已下载"))
		return nil
	},
}

// mirrorOf 本次下载用的镜像: 命令行指定优先, 否则 config.toml
func mirrorOf(s *app.Settings) string {
	if resMirrorFlag != "" {
		return resMirrorFlag
	}
	return s.GithubMirror
}

var resMirrorFlag string

func init() {
	for _, c := range []*cobra.Command{resCoreUpgradeCmd, resCoreRollbackCmd, resourceUpdateAllCmd} {
		c.Flags().StringVar(&resProxy, "proxy", "", T("下载使用的代理 (空=按环境变量/直连)"))
	}
	for _, c := range []*cobra.Command{resCoreUpgradeCmd, resourceUpdateAllCmd} {
		c.Flags().StringVar(&resMirrorFlag, "mirror", "", T("GitHub 镜像站前缀 (空=按 config.toml/内置列表)"))
	}
	resCoreUpgradeCmd.RegisterFlagCompletionFunc("proxy", noFlagComp)
	for _, c := range []*cobra.Command{resCoreUpgradeCmd, resCoreRollbackCmd} {
		markMutating(c)
	}
	resourceCoreCmd.AddCommand(resCoreVersionCmd, resCoreUpgradeCmd, resCoreRollbackCmd, resCoreHistoryCmd)
	for _, c := range []*cobra.Command{resourceUpdateAllCmd} {
		markMutating(c)
	}
	resourceCmd.AddCommand(resourceCoreCmd, resourceUpdateAllCmd)
	for _, r := range core.Resources() {
		resourceCmd.AddCommand(resourceRun(r.Name))
	}
	rootCmd.AddCommand(resourceCmd)
}

// noFlagComp 不补全的 flag 值 (避免落到文件名)
func noFlagComp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, noFileComp()
}
