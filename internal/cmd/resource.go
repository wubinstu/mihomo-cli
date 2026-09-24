package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/core"
	"github.com/wubinstu/mihomo-cli/internal/sysd"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

var resProxy string

// resourceCmd 资源管理: 内核二进制 + geo 数据 (原 core/geo 命令合并)
var resourceCmd = &cobra.Command{
	Use:   "resource",
	Short: T("资源管理: core/mmdb/asn/geoip/geosite"),
	Long: T("管理内核与 geo 数据资源; 数据资源支持自动更新 (resource-auto-update-*)。") + `

mihomo-cli resource core version|upgrade|rollback
mihomo-cli resource mmdb|asn|geoip|geosite info|update
mihomo-cli resource update-all    # ` + T("更新全部数据资源 (定时任务复用)"),
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
	Use:   "upgrade",
	Short: T("升级内核 (从 GitHub Releases)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := core.Upgrade(resProxy, false); err != nil {
			return err
		}
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		return nil
	},
}

var resCoreRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: T("回滚到上一版本"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := core.Rollback(); err != nil {
			return err
		}
		if sysd.IsActive() {
			return sysd.Service("restart")
		}
		return nil
	},
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
			s := mustSettings()
			if err := core.UpdateResource(name, resProxy, s.GithubMirror); err != nil {
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
	c.AddCommand(info, update)
	return c
}

// ---- update-all (定时任务) ----

var resourceUpdateAllCmd = &cobra.Command{
	Use:   "update-all",
	Short: T("更新全部数据资源 (mmdb/asn/geoip/geosite)"),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		if err := core.UpdateAllResources(resProxy, s.GithubMirror); err != nil {
			return err
		}
		s.ResourceLastRun = time.Now()
		_ = s.Save()
		fmt.Println(T("已下载"))
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{resCoreUpgradeCmd, resCoreRollbackCmd, resourceUpdateAllCmd} {
		c.Flags().StringVar(&resProxy, "proxy", "", T("下载使用的代理 (空=按环境变量/直连)"))
	}
	for _, sub := range []*cobra.Command{resCoreUpgradeCmd, resCoreRollbackCmd, resourceUpdateAllCmd} {
		_ = sub
	}
	resourceCoreCmd.AddCommand(resCoreVersionCmd, resCoreUpgradeCmd, resCoreRollbackCmd)
	resourceCmd.AddCommand(resourceCoreCmd, resourceUpdateAllCmd)
	for _, r := range core.Resources() {
		resourceCmd.AddCommand(resourceRun(r.Name))
	}
	rootCmd.AddCommand(resourceCmd)
}

var _ = app.BaseDir
var _ = strconv.Itoa
