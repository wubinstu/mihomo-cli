# mihomo-cli

面向 Linux 服务器的纯命令行 Clash 代理工具 —— [mihomo](https://github.com/MetaCubeX/mihomo) 内核的 CLI 外壳。
无 GUI、单二进制、systemd 托管，专为无图形界面的服务器设计。

## 架构与作用范围

```
/usr/bin/mihomo-cli            CLI 客户端
/etc/mihomo-cli/              全局配置 (唯一权威: config.toml; 文件头有管理警告, 勿手动编辑)
/etc/systemd/system/          mihomo-cli.service (journal 日志) + 三个 timer
```

- `config.toml` 是**唯一权威**：内核参数、定时器、点号路径写的配置段（tun/dns/任意 yaml 键）全在这里；
  `render` 合成 `/etc/mihomo-cli/runtime/config.yaml` 给内核，`config update-service` / `update-file`
  是两个方向的兜底同步
- CLI 经 systemd 托管内核，通过 `external-controller` API (127.0.0.1 + secret) 控制
- 权限回归系统惯例：目录 `755` / 文件 `644`（root:root）。只读命令全用户可用，
  变更类命令自动 `sudo` 提权（TTY 下输一次密码；非 TTY 直接报错并给出确切命令）
- GitHub 下载 (CLI/内核/geo) 自动尝试: 指定代理 → 自身代理(链路有效时) → 镜像站 → 直连

## 快速开始

```bash
# 1) 安装 CLI 本体 (install.sh 只装二进制到 /usr/bin; 大陆网络可指定镜像)
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | sudo bash
#   或: ... | sudo bash -s -- --mirror https://ghfast.top

# 2) 全新安装 (幂等可组合; 结束时自动拉起服务, 无订阅则为最小配置全 DIRECT)
sudo mihomo-cli install --core auto --resource all --systemd --completion bash

sudo mihomo-cli sub add mysub <订阅URL>   # 3) 添加订阅 (第一个自动激活)
mihomo-cli doctor
```

`install` 只做安装：`--core` / `--resource` / `--systemd` / `--completion` 可任意组合，重复执行就是刷新。
订阅用 `sub add`，参数用 `config set`，互不越界。

## 三层结构

```
订阅 sub ──use──> 生效订阅 ──> 分组 group ──use──> 当前分组 ──> 节点 node ──use──> 选中节点
```

- 全部支持 `#id` 索引别名与 `use/unuse`；unuse 语义: sub=内核空配置(全 DIRECT)、group/node=该组 DIRECT，**服务永不停止**
- 规则模式下多分组同时生效（国内直连/国外代理）；`group/node use` 设定的是操作上下文

## 命令一览

```
install / uninstall / update(自更新) / run(前台调试)

start|stop|restart|status                       服务
sub    add|rm|rename|update|use|unuse [list]     订阅 (索引)
group  use|unuse [list]                          分组 (#1..#n)
node   use|unuse|test|auto [list]                节点 (地区列/彩色测速/择优)  [-g 分组]
proxy  on/off                                    当前 shell 开关代理 (alias)
rule   list/add/enable/disable/rm                用户规则 (结构化, 优先于订阅, 内核校验回滚)
dns    on/off/use/unuse/list                     DNS (预设 + subN + 自定义 IP; 点号路径可配全段)
tun    on/off/list                               TUN 透明代理 (带安全护栏)
top    [watch N] [kill <id..>]                   流量/速度/连接总览 (PID 式编号)
ping   [站点...]                                 站点延迟/受限检测 (启发式)
config get|set|reset-default|unset|              配置管理: 一张表显示 默认值/设置值/运行值/状态
       update-file|update-service [KEY]          两个方向的兜底同步
resource core version|upgrade|rollback|history   内核版本与本地版本栈
         mmdb|asn|geoip|geosite info|update      geo 数据资源
         update-all
log [-f]                                         日志 (journalctl)
doctor / version / completion                    体检/版本/补全
```

全部输出双语：`config set cli-language auto|zh|en`（默认按 locale 回退中文）。
部分子命令（`config set`、`sub add`、`install` 等）会改变系统状态，非 root 执行时自动提权。

## 配置管理（`config`）

`config.toml` 是唯一权威。`config get` 用**一张表**把四件事一次说清楚：

```
$ mihomo-cli config get
[core config]
KEY                            RUNNING  SETTING   default   STATUS
----------------------------- ------- ---------- --------- ------
allow-lan                      true     true      false     ✔
mixed-port                     7890     7890      7890      ✔
socks-port                     7891     7891      off       ✔
http-port                      7892     7892      off       ✔
proxy-mode                     rule     rule      rule      ✔
ipv6-enabled                   true     true      false     ✔
log-level                      info     info      info      ✔
tcp-concurrent                true     true      true      ✔
unified-delay                 true     true      true      ✔
keep-alive-interval           30       30        30        ✔

[cli]
cli-language                   -        auto      auto      -
github-mirror                  -        auto      auto      -
test-url                       -        https://www.gstatic.com/generate_204 https://www.gstatic.com/generate_204 -
test-timeout                   -        5000ms    5000ms    -
current-profile                -        EDT       -         -
current-group                  -        🚀 节点选择 -        -

[systemd timers]
sub-auto-update-enabled        enabled  enabled   enabled   ✔
sub-auto-update-interval       24h      24h       24h       ✔
node-auto-select-enabled       enabled  enabled   disabled  ✔
node-auto-select-interval      30m      30m       30m       ✔
resource-auto-update-enabled   enabled  enabled   disabled  ✔
resource-auto-update-interval  24h      24h       24h       ✔
```

- `KEY` 参数名 · `RUNNING` 内核/timer 实际在用的值 · `SETTING` config.toml 里的值 ·
  `DEFAULT` 内置默认值 · `STATE` 两者是否一致（绿✔/红✘；服务未运行或该项无运行态时显示 `-`）
- `config set <key> <value>`：写入 config.toml 并立刻让运行态跟上（`proxy-mode`/`log-level` 走 PATCH 热切换；
  端口等需要重建监听的先试热重载，失败才重启；timer 键重写 unit 并启停）
- `config set <key> <TAB>` 补全全部合法值；`config set <key> -h` 显示单键详情（说明/当前值/默认值/可选值/生效方式）
- **非法值直接拒绝写入**，并告诉你是哪个键、填了什么、期望什么、合法值有哪些：
  ```
  $ mihomo-cli config set github-mirror auto1
  Error: 无效值, 已拒绝写入
    配置项: github-mirror
    填入: "auto1"
    期望: auto|http(s)://host
    合法值: auto | https://ghfast.top | https://gh-proxy.com | https://mirror.ghproxy.com
  ```

### 内核全部配置项都能设：点号路径

```bash
mihomo-cli config set dns.enable true
mihomo-cli config set dns.fake-ip-range 28.0.0.1/8
mihomo-cli config set tun.stack mixed
mihomo-cli config set tun.route-exclude-address 192.168.0.0/16,10.0.0.0/8
mihomo-cli config get tun              # 只看某一段
mihomo-cli config unset dns.listen     # 撤销接管, 该段回到"跟随订阅"
```

- 已知段 `dns`/`tun` 有完整的默认值/补全/校验；**未注册的任意 yaml 路径**也可以写
  （`config set sniffer.enable true`、`config set geodata-mode false`），值按字面推断类型
- 点号路径键存在 `[overrides]` 表里，render 时对订阅 yaml 深合并；**没用过的段完全不写**，
  订阅原样保留。原来的 `overrides.yaml` 已收编进 config.toml
- `dns on/off`、`tun on/off` 是 `config set dns.enable` / `tun.enable` 的简写

### 端口语义

```
mixed-port  7890 | off | sub | 1..65535     默认 7890 (http+socks5 混合)
socks-port  off  | sub | 1..65535          默认 off
http-port   off  | sub | 1..65535           默认 off
```
端口号本身就是开关：`off`=不监听（输入侧也接受 `0`/`none`/`-`，但显示永远是 `off`），
`sub`=跟随订阅。默认只开一个混合端口，另两个默认关闭；三个端口两两不得相同。

### 配置与运行态不一致时

```bash
mihomo-cli config update-service [KEY]   # 用 config.toml 覆盖运行态 (手动改过文件/改完没生效)
mihomo-cli config update-file [KEY]      # 用运行态覆盖 config.toml (别人 systemctl 改过 timer 之后)
mihomo-cli config reset-default [KEY]    # 恢复内置默认值
mihomo-cli config unset <key...>         # 撤销接管 (点号路径键回到跟随订阅)
```

### 配置项速查

| 类别 | key | 默认 |
|---|---|---|
| 内核 | `allow-lan` `mixed-port` `socks-port` `http-port` `proxy-mode` `ipv6-enabled` `log-level` `tcp-concurrent` `unified-delay` `keep-alive-interval` | false / 7890 / off / off / rule / false / info / true / true / 30 |
| cli | `cli-language` `github-mirror` `test-url` `test-timeout` | auto / auto / gstatic-204 / 5000ms |
| 定时器 | `sub-auto-update-*` `node-auto-select-*` `resource-auto-update-*` | 24h / 30m / 24h (除 node-auto-select-enabled 默认 false) |
| DNS 段 | `dns.enable` `dns.listen` `dns.enhanced-mode` `dns.fake-ip*` `dns.nameserver` `dns.default-nameserver` `dns.fallback` `dns.use-system-hosts` `dns.ipv6` | 只用过才写入; 见 `config get dns` |
| TUN 段 | `tun.enable` `tun.stack` `tun.device` `tun.mtu` `tun.dns-hijack` `tun.auto-route` `tun.strict-route` `tun.route-exclude-address` … | 同上; 见 `config get tun` |

## 内核安装包规格（`--core`）

上游资产的命名是**两个正交的轴**：`mihomo-linux-<arch>[-<flavor>]-<version>.gz`

- **版本轴**：`v1.19.31`
- **风味轴**：`v1`/`v2`/`v3`（GOAMD64 微架构档位，v3 需要 AVX2）、`go120`/`go123`（编译用 Go 工具链，影响最低 glibc）、
  `compatible`（旧 CPU + 旧 glibc 的保守构建，仅 amd64 有）

平台（系统/架构）自动探测并被记住，所以日常只需要写版本：

```bash
sudo mihomo-cli install --core auto                  # 自动探测风味, 装最新版
sudo mihomo-cli install --core v1.19.31              # 指定版本, 风味自动
sudo mihomo-cli install --core compatible            # 旧 CPU/旧系统, 版本取最新
sudo mihomo-cli install --core compatible:v1.19.19   # 风味+版本都指定
mihomo-cli resource core upgrade                     # 沿用已记住的平台风味, 装最新
mihomo-cli resource core upgrade v1.19.5             # 同上前缀, 只变版本 (可新可旧)
mihomo-cli resource core rollback                    # 本地版本栈出栈 (离线可用)
mihomo-cli resource core history                     # 看版本栈
```

自动探测是**候选链 + 试跑**：按 `plain → v3 → v2 → v1 → compatible → *-go120` 依次下载，
每个都跑一次 `<bin> -v`，跑不起来（缺指令集/glibc 太低）就自动降级，不靠猜。

### 卸载与安装严格对称

| install | uninstall |
|---|---|
| `install --core auto` | `uninstall --core`（先停服务，删内核与版本栈） |
| `install --resource all` | `uninstall --resource`（删 4 个 geo 文件） |
| `install --systemd` | `uninstall --systemd`（关闭并删除 service/timer） |
| `install --completion bash` | `uninstall --completion bash`（不带值=三个全删） |
| — | `uninstall --purge`（以上全部 + `/etc/mihomo-cli` + 二进制自身） |
| 无参数 = 帮助 | 无参数 = 帮助 |

## TUN 透明代理

```bash
mihomo-cli tun on     # 开启前自动做安全检查
mihomo-cli tun list   # 查看 tun 段全部配置项
```

开启前的三道护栏（任一不满足则拒绝并解释原因）：

1. `/dev/net/tun` 必须存在（容器常缺，提示改用 mixed-port 端口模式）
2. 自动给内核申请 `CAP_NET_ADMIN`（写进 systemd 单元 `AmbientCapabilities`）
3. **SSH 自保**：`auto-route` 会把默认路由指进 tun，极易把自己踢下线；
   当前 SSH 对端网段必须已在 `tun.route-exclude-address` 里

## DNS

```bash
mihomo-cli dns on                 # = config set dns.enable true
mihomo-cli dns use cloudflare     # = config set dns.nameserver 1.1.1.1,1.0.0.1
mihomo-cli dns use 223.5.5.5      # 自定义 IP
mihomo-cli dns use sub1           # 用某订阅自带的 DNS
mihomo-cli dns list               # = config get dns + 预设表
mihomo-cli dns off                # 恢复跟随订阅
```

## 安装/更新方式

```bash
install.sh (自动 sudo/镜像) / deb / rpm (Releases) / sudo mihomo-cli update
```

镜像：`install.sh --mirror <url>`（管道形式 `sudo bash -s -- --mirror <url>`）；
CLI 内下载内核/geo/自更新走 镜像自动尝试链，可用 `config set github-mirror <url>` 固定。

## 自检

```bash
scripts/completion-check.sh      # 57 项补全断言 (禁止文件补全 + 每个键都有值补全)
go test ./...                    # 单元测试 + i18n 静态审计 (en 模式零中文)
```

## License

MIT
