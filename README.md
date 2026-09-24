# mihomo-cli

面向 Linux 服务器的纯命令行 Clash 代理工具 —— [mihomo](https://github.com/MetaCubeX/mihomo) 内核的 CLI 外壳。无 GUI、单二进制、systemd 托管，专为无图形界面的服务器设计。

## 架构与作用范围

```
/usr/bin/mihomo-cli            CLI 客户端
/etc/mihomo-cli/            全局配置 (内核/订阅/设置, 系统级共享; 文件头有管理警告, 勿手动编辑)
/etc/systemd/system/        mihomo-cli.service (journal 日志) + 定时器
```

- 配置以 `/etc/mihomo-cli/config.toml` 为**唯一权威**，运行时修改由 CLI 回写文件并热生效；
  手动改动后执行 `config sync update-service` 以文件覆盖服务（`sync update-file` 反向）
- CLI 经 systemd 托管内核，通过 `external-controller` API (127.0.0.1 + secret) 控制
- GitHub 下载 (CLI/内核/geo) 自动尝试: 指定代理 → 自身代理(链路有效时) → 镜像站 → 直连

## 快速开始

```bash
# 1) 安装 CLI 本体 (脚本自动 sudo; 大陆网络可指定镜像)
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | sudo bash
#   或: curl -fsSL .../install.sh | sudo bash -s -- --mirror https://ghfast.top

sudo mihomo-cli install      # 2) 下载内核 + 注册 systemd 服务 + geo 数据 + 补全
sudo mihomo-cli init         # 3) 粘贴订阅链接
sudo mihomo-cli start        # 4) 启动代理服务
mihomo-cli doctor            # 体检
```

## 三层结构

```
订阅 sub ──use──> 生效订阅 ──> 分组 group ──use──> 当前分组 ──> 节点 node ──use──> 选中节点
```

- 全部支持 `#id` 索引别名与 `use/unuse`；unuse 语义: sub=内核空配置(全 DIRECT)、group/node=该组 DIRECT，**服务永不停止**
- 规则模式下多分组同时生效（国内直连/国外代理）；`group/node use` 设定的是操作上下文

## 命令一览

```
install / uninstall / init / update(自更新)

start|stop|restart|status / run(前台)          服务
sub add|rm|rename|update|use|unuse [list]      订阅 (索引)
group use|unuse [list]                          分组 (#1..#n)
node use|unuse|test|auto [list]                 节点 (地区列/彩色测速/择优)
proxy on/off                                    当前 shell 开关代理 (alias)
rule list/add/enable/disable/rm                 用户规则 (结构化, 优先于订阅, 内核校验回滚)
dns use|unuse [list]                            DNS (7 预设 + subN + 自定义 IP)
top [watch N] [kill <id..>]                     流量/速度/连接总览 (PID 式编号)
ping [站点...]                                  站点延迟/受限检测 (启发式)
config get|set|reset-default|sync               配置管理 (分组展示/补全/单键详情)
resource core|mmdb|asn|geoip|geosite          资源管理 (info/update; geo 数据可自动更新)
log [-f]                                        日志 (journalctl)
doctor / version / completion                   体检/版本/补全 (install 自动装)
```

全部输出双语: `config set cli-language zh|en`（默认按 locale 回退中文）。

## 用户规则 (rule)

独立于订阅存储，订阅更新/更换不丢失；渲染时置于订阅规则之前（优先匹配）：

```bash
mihomo-cli rule add --type DOMAIN-SUFFIX --condition openai.com --strategy DIRECT
mihomo-cli rule add --type IP-CIDR --condition 10.0.0.0/8 --strategy DIRECT --no-resolve
mihomo-cli rule add --type GEOIP --condition CN --strategy PROXY     # PROXY=当前分组
mihomo-cli rule list --type DOMAIN-SUFFIX [--strategy DIRECT] [--no-resolve] [--sub]
mihomo-cli rule enable 1 / disable 1 / rm 1
```

- 31 种规则类型的说明/示例: `rule add -h`、`rule add --type DOMAIN -h` 等上下文帮助
- 语法由内核 reload 校验，非法规则自动回滚

## 配置 (`mihomo-cli config get` / `config set key value`)

| 类别 | key | 默认 |
|---|---|---|
| 内核 | `allow-lan` `mixed-port` `proxy-mode` `ipv6-enabled` `log-level` `tcp-concurrent` `unified-delay` `keep-alive-interval` | false / 7890 / 跟随订阅 / false / info / 跟随 / 跟随 / 跟随 |
| cli | `cli-language` / `install-mirror` / `test-url` / `test-timeout` | locale / 自动 / gstatic / 5000 |
| 定时器 | `sub-auto-update-*` / `node-auto-select-*` / `resource-auto-update-*` | true 24h / false 30m / false 24h |

`config reset-default [key|all]` 恢复默认; `config sync` 检测文件与内核运行差异，
`sync update-service` 文件覆盖服务(重启)，`sync update-file` 运行状态覆盖文件。

## 安装/更新方式

```bash
install.sh (自动 sudo/镜像) / deb / rpm (Releases) / sudo mihomo-cli update
```

镜像: `install.sh --mirror <url>` (管道形式 `sudo bash -s -- --mirror <url>`)；
CLI 内下载内核/geo/自更新走 镜像自动尝试链，可用 `config set install-mirror <url>` 固定。

## License

MIT
