package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wubinstu/mihomo-cli/internal/api"
	"github.com/wubinstu/mihomo-cli/internal/app"
	"github.com/wubinstu/mihomo-cli/internal/ui"
)

// topCmd = conn + traffic 合并: 流量统计 + 速度 + 连接列表 (编号可 kill)
var topInterval = 1

var topCmd = &cobra.Command{
	Use:   "top [watch] [sec]",
	Short: T("流量与连接总览 (top watch 持续刷新, top kill <编号> 关连接)"),
	Long: T("流量与连接总览") + `:
  mihomo-cli top              # ` + T("单次输出: 总流量/速度/连接数/连接列表") + `
  mihomo-cli top watch [N]    # ` + T("每 N 秒刷新 (默认 1s), Ctrl-C 退出") + `
  mihomo-cli top kill <id..>  # ` + T("关闭指定编号的连接"),
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		watch := false
		if len(args) > 0 && args[0] == "watch" {
			watch = true
			if len(args) > 1 {
				n, err := strconv.Atoi(args[1])
				if err != nil || n < 1 || n > 60 {
					return fmt.Errorf("%s (1-60)", T("无效刷新间隔"))
				}
				topInterval = n
			}
		}
		for {
			if err := printTop(c); err != nil {
				return err
			}
			if !watch {
				return nil
			}
			time.Sleep(time.Duration(topInterval) * time.Second)
			fmt.Print("\033[H\033[2J")
		}
	},
}

// printTop 两次采样 (间隔 topInterval 秒) 计算速度并输出
func printTop(c *api.Client) error {
	r1, err := c.Connections()
	if err != nil {
		return err
	}
	time.Sleep(time.Duration(topInterval) * time.Second)
	r2, err := c.Connections()
	if err != nil {
		return err
	}
	dt := float64(topInterval)

	// 单次输出也保持 1s 采样; 汇总
	fmt.Printf("%s: %d   %s ↑%s ↓%s   %s ↑%s/s ↓%s/s\n",
		T("活动连接"), len(r2.Connections),
		T("累计"), humanBytes(r2.UploadTotal), humanBytes(r2.DownloadTotal),
		T("速度"),
		humanBytes(rate(r2.UploadTotal, r1.UploadTotal, dt)),
		humanBytes(rate(r2.DownloadTotal, r1.DownloadTotal, dt)))
	if len(r2.Connections) == 0 {
		return nil
	}
	// 单连接速度 = 两次采样差
	up1 := map[string]int64{}
	down1 := map[string]int64{}
	for _, cn := range r1.Connections {
		up1[cn.ID] = cn.Upload
		down1[cn.ID] = cn.Download
	}
	st := loadConnSeq().assign(r2.Connections)
	rows := [][]string{{"#", T("网络"), T("进程"), T("目标"), T("时长"), T("代理链"), "↑", "↓", "↑/s", "↓/s"}}
	list := append([]api.ConnInfo(nil), r2.Connections...)
	sort.Slice(list, func(i, j int) bool { return list[i].Download > list[j].Download })
	for _, cn := range list {
		host := cn.Metadata.Host
		if host == "" {
			host = cn.Metadata.Destination
		}
		proc := cn.Metadata.Process
		if proc == "" {
			proc = "-"
		}
		upPrev, downPrev := up1[cn.ID], down1[cn.ID]
		rows = append(rows, []string{
			strconv.Itoa(st[cn.ID]),
			cn.Metadata.Network,
			proc,
			host,
			connAge(cn.Start),
			strings.Join(cn.Chains, "/"),
			humanBytes(cn.Upload), humanBytes(cn.Download),
			humanBytes((cn.Upload - upPrev) / int64(dt)),
			humanBytes((cn.Download - downPrev) / int64(dt)),
		})
	}
	ui.Table(os.Stdout, rows, 1)
	return nil
}

// rate 计算速率, 采样回退(内核重启/重载导致计数清零)时返回 0
func rate(cur, prev int64, dt float64) int64 {
	if cur < prev {
		return 0
	}
	return int64(float64(cur-prev) / dt)
}

func connAge(start string) string {
	t, err := time.Parse(time.RFC3339Nano, start)
	if err != nil {
		return "-"
	}
	d := time.Since(t).Round(time.Second)
	if d > time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return d.String()
}

// connSeqStore 连接编号持久化 (PID 风格: 递增分配, 断开的编号保留一段时间后回收)
type connSeqStore struct {
	Seq  int              `json:"seq"`
	Maps map[string]int   `json:"maps"` // conn uuid -> 编号
	Seen map[string]int64 `json:"seen"` // conn uuid -> 最后见到的时间戳
}

func loadConnSeq() *connSeqStore {
	st := &connSeqStore{Maps: map[string]int{}, Seen: map[string]int64{}}
	if data, err := os.ReadFile(app.RuntimeDir + "/connseq.json"); err == nil {
		_ = json.Unmarshal(data, st)
	}
	if st.Maps == nil {
		st.Maps = map[string]int{}
	}
	if st.Seen == nil {
		st.Seen = map[string]int64{}
	}
	return st
}

func (st *connSeqStore) save() {
	data, _ := json.Marshal(st)
	_ = os.WriteFile(app.RuntimeDir+"/connseq.json", data, 0o644)
}

// assign 为当前连接集合分配编号: 新连接取新号; 10 分钟未见到的旧编号回收
func (st *connSeqStore) assign(conns []api.ConnInfo) map[string]int {
	now := time.Now().Unix()
	for _, cn := range conns {
		if _, ok := st.Maps[cn.ID]; !ok {
			st.Seq++
			st.Maps[cn.ID] = st.Seq
		}
		st.Seen[cn.ID] = now
	}
	for id, ts := range st.Seen {
		if now-ts > 600 {
			delete(st.Maps, id)
			delete(st.Seen, id)
		}
	}
	st.save()
	return st.Maps
}

var topKillCmd = &cobra.Command{
	Use:   "kill <id..>",
	Short: T("关闭指定编号的活动连接"),
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mustSettings()
		c := api.New(s)
		r, err := c.Connections()
		if err != nil {
			return err
		}
		st := loadConnSeq().assign(r.Connections)
		bySeq := map[int]string{}
		for id, n := range st {
			bySeq[n] = id
		}
		killed := 0
		for _, a := range args {
			n, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %q\n", T("无效编号"), a)
				continue
			}
			id, ok := bySeq[n]
			if !ok {
				fmt.Fprintf(os.Stderr, "#%d: %s\n", n, T("连接不存在或已关闭"))
				continue
			}
			if err := c.CloseConn(id); err != nil {
				fmt.Fprintf(os.Stderr, "#%d: %v\n", n, err)
				continue
			}
			fmt.Printf("#%d %s\n", n, T("已关闭"))
			killed++
		}
		if killed == 0 {
			return fmt.Errorf("%s", T("没有连接被关闭"))
		}
		return nil
	},
}

func init() {
	topKillCmd.ValidArgsFunction = connIDComp
	topCmd.AddCommand(topKillCmd)
	rootCmd.AddCommand(topCmd)
}
