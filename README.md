# mihomo-cli

面向 Linux 服务器的纯命令行代理工具 —— [mihomo](https://github.com/MetaCubeX/mihomo) 内核的 CLI 外壳。
无 GUI、单二进制、systemd 托管，为没有图形界面的服务器设计。

```
/usr/bin/mihomo-cli       CLI 本体
/etc/mihomo-cli/          全局配置 (唯一权威: config.toml, 勿手动编辑)
/etc/systemd/system/      mihomo-core.service + 三个 timer (sub / node / resource)
```

- 配置模型：`config.toml` 是唯一权威，`config set` 声明并立即生效，`config get` 的 `STATE` 列就是漂移视图
- 双语输出：`config set cli.language auto|zh|en`（默认按 locale，无中文环境回退英文）
- 权限回归系统惯例：只读命令任何用户可用，变更类命令自动 `sudo` 提权

---

## 快速开始

```bash
# 1) 装 CLI 本体 (只装二进制到 /usr/bin; 大陆网络可指定镜像)
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | sudo bash
#   或: ... | sudo bash -s -- --mirror https://ghfast.top

# 2) 全新安装 (幂等可组合; 结束自动拉起服务)
sudo mihomo-cli install --core auto --resource all --systemd --completion bash

# 3) 加订阅 (第一个自动激活)
sudo mihomo-cli sub add mysub <订阅URL>

mihomo-cli doctor          # 体检
mihomo-cli status          # 一眼看当前状态
```

`install` 只做安装：`--core` / `--resource` / `--systemd` / `--completion` 可任意组合，重复执行就是刷新。
订阅用 `sub add`，参数用 `config set`，互不越界。

---

## 三层结构

```
订阅 sub ──use──> 生效订阅 ──> 分组 group ──use──> 当前分组 ──> 节点 node ──use──> 选中节点
```

全部支持 `#id` 索引别名与 `use/unuse`。`unuse` 的语义：sub = 内核空配置运行（全部 DIRECT），
group/node = 该组走 DIRECT，**服务永不停止**。规则模式下多分组同时生效。

---

## 命令一览

```
install / uninstall / update(自更新) / run(前台调试)

start|stop|restart|status                      服务
sub    add|rm|rename|update|use|unuse [list]   订阅 (索引)
group  use|unuse [list]                        分组 (#1..#n)
node   use|unuse|test|auto [list]              节点 (地区列/彩色测速/择优)  [-g 分组]
proxy  on/off                                  当前 shell 开关代理 (alias)
rule   list/add/enable/disable/rm              用户规则 (优先于订阅, 内核校验回滚)
dns    on/off/list                             DNS 覆写开关 (其余参数走 config set core.dns.*)
tun    on/off/list                             TUN 透明代理 (带安全护栏; 其余走 config set core.tun.*)
top    [watch N] [kill <id..>]                 流量/速度/连接总览
ping   [站点...]                               站点延迟/受限检测 (含 GitHub、Docker Hub)
config get|set|reset-default|unset|apply|adopt 配置管理
resource core version|upgrade|rollback|history 内核版本与本地版本栈
         mmdb|asn|geoip|geosite info|update    geo 数据资源
         update-all
log [-f]                                       日志 (journalctl)
doctor / version                               体检 / 版本
```

---

## 配置管理

`config.toml` 是唯一权威，按段写成 TOML 表。**`core.` 前缀就是"内核 config.yaml"**，
后面的路径和 yaml 一一对应：

```toml
[cli]                                  # cli.*      —— CLI 自身
language = "zh"
current_profile = "EDT"

[core-spec]                            # 内核规格 (探测结果, upgrade 沿用)
platform = "linux/amd64"
flavor = "v3"
version = "v1.19.31"

[core]                                 # core.* → 内核 config.yaml 的顶层键
allow_lan = false                      # 托管键一律写有效值, 和 config get 完全一致
mixed_port = "7890"
socks_port = "off"

[control-api]                          # external-controller (CLI 自己的管道)
base = "http://127.0.0.1:9090"

[timer]                                # timer.* → systemd
sub_auto_update_interval = "24h0m0s"

[misc]
test_url = "https://www.gstatic.com/generate_204"

[overrides.dns]                        # core.dns.* → 内核的 dns 段 (设过才写)
enable = true
nameserver = ["1.1.1.1", "1.0.0.1"]

[[user_rules]]                         # rule 命令的结构化存储
type = "DOMAIN"

[[profiles]]                           # sub 命令的订阅
name = "EDT"
```

| 前缀 | 含义 | 例 |
|---|---|---|
| `core` | 内核 config.yaml 的顶层键 | `core.allow-lan` → yaml `allow-lan` |
| `core.<段>` | 内核 config.yaml 的配置段 | `core.dns.enable` → yaml `dns.enable` |
| `cli` | mihomo-cli 自身 | `cli.language` |
| `timer` | systemd 定时器 | `timer.sub-auto-update-interval` |

异名键只有三个（内核原名太含糊）：`proxy-mode→mode`、`ipv6-enabled→ipv6`、`http-port→port`。
`config get core` 因此是**内核配置的全集**（顶层键 + dns/tun 一次看完）。

### config get：一张表说清四件事

```
$ mihomo-cli config get
[core config]
KEY                          RUNNING SETTING DEFAULT STATE
---------------------------- ------- ------- ------- -----
core.allow-lan               true   true    false   ✔
core.mixed-port              7890   7890    7890    ✔
core.proxy-mode              rule   rule    rule    ✔
core.log-level               info   info    info    ✔
...

[core.dns]
core.dns.enable              -      true    false   -
core.dns.nameserver          -      1.1.1.1,1.0.0.1  sub  -

[cli]
cli.language                 -      zh      auto    -

[systemd timers]
timer.sub-auto-update-enabled  enabled enabled enabled ✔
...

(长值已截断, 完整值: mihomo-cli config get <key>)
```

- `KEY` 参数名 · `RUNNING` 内核/timer 实际在用的值 · `SETTING` config.toml 里的值 ·
  `DEFAULT` 内置默认值 · `STATE` 两者是否一致（绿✔/红✘；未接管或服务未运行时显示 `-`）
- 段顺序：内核相关（`core` / `core.dns` / `core.tun`）在前，CLI 自身（`cli` / `timer`）在后
- `core.dns` / `core.tun` 默认**不露面**，只看总开关（`core.dns.enable` / `core.tun.enable`）：
  开关没开时这段配置根本不参与渲染，摆出来只会误导。设了值但没开会给一行提示，
  显式 `config get core.dns` 永远能看
- 长值按 32 列宽截断加 `…`，完整值用 `config get <key>`（单键输出不截断）
- `SETTING` 为 `-` 表示**未接管**：不写进内核 yaml，订阅/内核原样保留

### config set

```bash
mihomo-cli config set core.proxy-mode global
mihomo-cli config set core.mixed-port 7891
mihomo-cli config set core.dns.nameserver cloudflare     # 预设名, TAB 可补全
mihomo-cli config set core.tun.stack gvisor
mihomo-cli config set cli.language en                    # 当次立即生效
```

- 写入 config.toml 并**立刻让运行态跟上**：`proxy-mode`/`log-level` 走 PATCH 热切换，
  端口等需要重建监听的先试热重载、失败才重启，timer 键重写 unit 并启停
- 切换后会校验是否真的生效，跟不上时给出确切的下一步命令
- **非法值直接拒绝写入**，并告诉你是哪个键、填了什么、期望什么、合法值有哪些：
  ```
  $ mihomo-cli config set cli.github-mirror auto1
  Error: 无效值, 已拒绝写入
    配置项: cli.github-mirror
    填入: "auto1"
    期望: auto|http(s)://host
    合法值: auto | https://ghfast.top | https://gh-proxy.com | https://mirror.ghproxy.com
  ```
- 帮助分两级：`config set -h` 给全部键的一行简介；`config set <key> -h` 给该键的详细介绍
  （说明 / 每个取值的含义 / 当前值 / 默认值 / 内核键名 / 生效方式 / 用法 / 恢复默认 / 撤销接管）

### 内核任意配置项都能设：点号路径

```bash
mihomo-cli config set core.sniffer.enable true        # 未注册的 yaml 路径也行
mihomo-cli config set core.dns.fake-ip-range 28.0.0.1/8
mihomo-cli config get core.tun                        # 只看某一段
mihomo-cli config unset core.dns.listen               # 撤销接管, 回到"跟随订阅"
```

点号路径键存在 `[overrides]` 表里，render 时对订阅 yaml 深合并；**没用过的段完全不写**，订阅原样保留。

### 端口语义

```
mixed-port  7890 | off | sub | 1..65535     默认 7890 (http+socks5 混合)
socks-port  off  | sub | 1..65535          默认 off
http-port   off  | sub | 1..65535           默认 off
```

端口号本身就是开关：`off` = 不监听（输入侧也接受 `0`/`none`/`-`，显示永远是 `off`），
`sub` = 跟随订阅。三个端口两两不得相同。

### 配置模型（一段话）

`config.toml` 是唯一权威；`config set` = 声明并立即生效；`config get` 的 `STATE` 列就是漂移视图
（相当于 `terraform plan`）；`config apply` / `config adopt` 是两个方向的手动 reconcile，
只在"别人改过 systemd / 你手改过文件"时才需要。`runtime/config.yaml` 是自动生成的产物，不要手改。

```bash
mihomo-cli config apply [KEY]            # 用 config.toml 覆盖运行态
mihomo-cli config adopt [KEY]            # 用运行态覆盖 config.toml
mihomo-cli config reset-default [KEY]    # 恢复内置默认值
mihomo-cli config unset <key...>         # 撤销接管 (点号路径键回到跟随订阅)
```

### 定时器作用域

| 定时器 | 作用域 | 理由 |
|---|---|---|
| `timer.sub-auto-update-*` | **全部订阅** | 订阅才几百 KB，全量更新保证切到哪个订阅都不会拿到过期节点 |
| `timer.node-auto-select-*` | **仅当前订阅 + 当前分组** | 它改的是活的代理链，对没在用的订阅择优没有意义 |
| `timer.resource-auto-update-*` | 全局 | geo 数据只有一份 |

### 配置项速查

| 类别 | key | 默认 |
|---|---|---|
| 内核 | `core.allow-lan` `core.mixed-port` `core.socks-port` `core.http-port` `core.proxy-mode` `core.ipv6-enabled` `core.log-level` `core.tcp-concurrent` `core.unified-delay` `core.keep-alive-interval` | false / 7890 / off / off / rule / false / info / true / true / 30 |
| cli | `cli.language` `cli.github-mirror` `cli.test-url` `cli.test-timeout-ms` | auto / auto / gstatic-204 / 5000 |
| 定时器 | `timer.sub-auto-update-*` `timer.node-auto-select-*` `timer.resource-auto-update-*` | 24h / 30m / 24h (node-auto-select-enabled 默认 false) |
| DNS 段 | `core.dns.enable` `core.dns.listen` `core.dns.enhanced-mode` `core.dns.fake-ip*` `core.dns.nameserver` `core.dns.default-nameserver` `core.dns.fallback` `core.dns.use-system-hosts` `core.dns.ipv6` | 设过才写入; `config get core.dns` |
| TUN 段 | `core.tun.enable` `core.tun.stack` `core.tun.device` `core.tun.mtu` `core.tun.dns-hijack` `core.tun.auto-route` `core.tun.strict-route` `core.tun.route-exclude-address` … | 同上; `config get core.tun` |

---

## 内核安装包规格（`--core`）

上游资产命名是**两个正交的轴**：`mihomo-linux-<arch>[-<flavor>]-<version>.gz`

- **版本轴**：`v1.19.31`
- **风味轴**：`v1`/`v2`/`v3`（GOAMD64 微架构档位，v3 需要 AVX2）、`go120`/`go123`（编译用 Go 工具链，
  影响最低 glibc）、`compatible`（旧 CPU + 旧 glibc 的保守构建，仅 amd64 有）

平台（系统/架构）自动探测并被记住，日常只需要写版本：

```bash
sudo mihomo-cli install --core auto                  # 自动探测风味, 装最新版
sudo mihomo-cli install --core v1.19.31              # 指定版本, 风味自动
sudo mihomo-cli install --core compatible:v1.19.19   # 风味+版本都指定
mihomo-cli resource core upgrade                     # 沿用记住的平台风味, 装最新
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

---

## TUN 透明代理

```bash
mihomo-cli tun on     # 开启前自动做安全检查
mihomo-cli tun list   # 查看 core.tun 段全部配置项
```

开启前的三道护栏（任一不满足则拒绝并解释原因）：

1. `/dev/net/tun` 必须存在（容器常缺，提示改用 mixed-port 端口模式）
2. 自动给内核申请 `CAP_NET_ADMIN`（写进 systemd 单元 `AmbientCapabilities`）
3. **SSH 自保**：`auto-route` 会把默认路由指进 tun，极易把自己踢下线；
   当前 SSH 对端网段必须已在 `core.tun.route-exclude-address` 里

`core.tun.stack` 有四个值：`system`（内核协议栈，性能最好）/ `gvisor`（用户态，兼容最好）/
`mixed`（TCP 走 system，其余走 gvisor）/ `mips`（内核自研用户态栈，默认）。

---

## DNS

```bash
mihomo-cli dns                                          # = config get core.dns (只显示参数表)
mihomo-cli dns on / off                                 # = config set core.dns.enable true|false
mihomo-cli config set core.dns.nameserver cloudflare    # 预设名 (ali/114/google/cloudflare/adguard/quad9/dnspod)
mihomo-cli config set core.dns.nameserver 223.5.5.5,119.29.29.29   # 自定义 IP
mihomo-cli config set core.dns.nameserver sub1          # 用某订阅自带的 DNS
mihomo-cli config set core.dns.nameserver sub           # 恢复跟随订阅
```

预设名的完整列表与 IP：`mihomo-cli config set core.dns.nameserver -h`。
`dns`/`tun` 是这两个段的快捷命令组：**写操作统一走 `config set`**，这里只留 `on/off` 两个糖。

---

## 订阅用量

订阅的 `subscription-userinfo` 响应头会被解析成用量与到期时间，三处给出结论：

```
$ mihomo-cli sub list
*  #  名称  节点数  更新时间          用量                                     URL
   1  EDT   16      2026-10-09 17:29  -                                        https://proxy.wubinstu.com/sub?token=...
   2  dog   4       2026-10-10 10:16  0B/160.00GB (0.00%) · 已过期 2026-05-02   https://...
```

- `status` / `doctor` 对**当前订阅**各给一行；到期或超额算 ✘，用量 ≥80% 标黄

---

## 安装/更新方式

```bash
install.sh (自动 sudo/镜像) / deb / rpm (Releases) / sudo mihomo-cli update
```

镜像：`install.sh --mirror <url>`（管道形式 `sudo bash -s -- --mirror <url>`）；
CLI 内下载内核/geo/自更新走"指定代理 → 自身代理 → 镜像站 → 直连"的自动尝试链，
可用 `config set cli.github-mirror <url>` 固定。

---

## 代码结构

```
main.go                 入口: 兜住 panic, 转交 cmd.Execute()
internal/app           路径与配置 (config.toml 的唯一读写方; 不含任何业务)
internal/i18n          中英文案 (中文原文即 key; 静态审计保证 en 模式零中文)
internal/sysd          systemd 单元读写 (服务 + 三个 timer)
internal/api           内核 REST API 客户端 (127.0.0.1 + secret)
internal/core          内核二进制: 探测/下载/安装/版本栈
internal/geo           IP 地域判定 (国家码/旗帜)
internal/subs          订阅: 下载/解析/节点计数/用量
internal/cfg           配置注册表: 一张表驱动 get/set/补全/校验/帮助 + 运行态读取
internal/render        把 config.toml + 订阅合成内核的 config.yaml
internal/ui            终端表格/对齐/上色 (宽度感知, 不依赖其它 internal 包)
internal/cmd           cobra 命令层 (薄, 只做编排)
```

依赖方向是严格的 DAG：`app` 是叶子，`cmd` 在顶端，无环。
`internal/cfg` 的注册表是单一事实来源——新增一个配置键只需要改那一张表，
`config get`/`set`/补全/校验/详版帮助/`doctor` 全部自动跟着变。

### 自检

```bash
scripts/completion-check.sh      # 67 项补全断言 (禁止文件补全 + 每个键都有值补全)
go test ./...                    # 单元测试 + i18n 静态审计 + 中英双向帮助渲染
```

---

## License

[MIT](LICENSE)
