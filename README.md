# mihomo-cli

面向 Linux 服务器的纯命令行 Clash 代理工具 —— [mihomo](https://github.com/MetaCubeX/mihomo) 内核的 CLI 外壳。无 GUI、单二进制、systemd 托管，专为无图形界面的服务器设计。

## 架构与作用范围

```
/usr/local/bin/mihomo-cli   CLI 客户端 (本软件)
/etc/mihomo-cli/            全局配置目录 (内核/订阅/设置, 系统级共享)
/etc/systemd/system/        mihomo-cli.service (后台服务) + 定时器
```

- **系统级部署**: 服务端(内核)/客户端(CLI)/配置(`/etc/mihomo-cli`)全机共享;
  多用户共用同一套代理, 无需重复设置。`install`/`uninstall` 需要 root
  (install 后配置目录属主自动交给发起安装的 sudo 用户, 该用户日常管理无需再 sudo);
  查询/切换命令所有用户可用。
- CLI 通过 systemd 托管内核子进程, 经 `external-controller` API (仅 127.0.0.1 + 随机 secret) 控制。
- 旧版用户目录 `~/.config/mihomo-cli` 会在 root 运行时自动迁移到 `/etc/mihomo-cli`。

## 快速开始

```bash
# 安装 (中国大陆服务器建议 --proxy 走代理下载内核)
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | sudo bash -s -- --proxy http://192.168.1.1:7890

sudo mihomo-cli init            # 粘贴订阅链接
sudo mihomo-cli start           # 启动
mihomo-cli doctor               # 体检 (所有用户可查)
```

## 三层结构与命令

```
订阅 sub ──use──> 生效订阅 ──> 分组 group ──use──> 当前分组 ──> 节点 node ──use──> 分组选中的节点
```

- 三层均支持 `#id` 索引别名 (`sub use 1` / `group use 8` / `node use 5`), emoji 组名免输入。
- `unuse` 悬空语义(三层统一, **服务永不停止**): 内核切换到空配置(`mode: direct`),
  所有流量直连, 代理端口仍可用; `sub rm` 删掉唯一订阅时同样处理。
- 概念说明: 规则模式下不同流量按规则走不同分组(国内直连、国外走代理), 多个分组同时生效;
  `group/node use` 设置的是「当前操作的分组上下文」及其选中节点。

## 常用命令

```
mihomo-cli start|stop|restart|status    服务生命周期
mihomo-cli run                          前台运行(调试)
mihomo-cli update                       更新 mihomo-cli 自身 (别名 upgrade)

mihomo-cli sub                          订阅列表 (= sub list)
mihomo-cli sub add|rm|rename|update     订阅管理 (支持 #id)
mihomo-cli sub use|unuse                选择/悬空订阅

mihomo-cli group                        分组列表 (= group list)
mihomo-cli group use|unuse              选择/取消当前操作分组

mihomo-cli node                         当前分组的节点列表 (= node list)
mihomo-cli node use|unuse               选择/取消节点 (unuse=直连)
mihomo-cli node test                    节点测速 (彩色: 绿<200 蓝<500 黄<3000 红, 灰=超时)
mihomo-cli node auto                    对当前分组择优一次 (定时任务复用)

mihomo-cli proxy on/off                 开/关当前 shell 代理(配合 alias)
mihomo-cli top [watch N] [kill <编号..>]  流量/速度/连接总览 (PID 式编号可 kill)
mihomo-cli dns [use|unuse]              DNS 查看/预设(ali/114/google/cloudflare/adguard/quad9/dnspod)/自定义 IP
mihomo-cli log [-f]                     内核日志
mihomo-cli core upgrade|rollback|geo    内核升级/回滚/geo 数据
mihomo-cli doctor                       体检
mihomo-cli get/set [key]                查看/修改设置 (set <key> 单独执行显示该参数说明)
mihomo-cli uninstall [--purge]          卸载
```

下载代理不再持久化: `install --proxy` / `core upgrade --proxy` / `core geo --proxy` / `update --proxy` 临时使用,
或直接走 `https_proxy` 环境变量。

sub/group/node 三层列表均以第一列 `*` 标记当前 use 选中项。

推荐 alias（加到 `~/.bashrc`）:

```bash
alias proxy_on='eval $(mihomo-cli proxy on)'
alias proxy_off='eval $(mihomo-cli proxy off)'
```

bash/zsh/fish 补全脚本随 `install` 自动安装、随 `uninstall` 清理。
所有命令输出/帮助双语: `set lang zh|en`（默认按系统 locale, 回退中文）。

## 配置 (`mihomo-cli get [key]` / `sudo mihomo-cli set key value`)

| key | 说明 | 默认 |
|---|---|---|
| `lang` | 输出语言 `zh`/`en` | 按系统 locale, 回退中文 |
| `allow-lan` | 允许局域网设备使用代理 (0.0.0.0) | `false` |
| `mixed-port` | 混合代理端口 (http+socks5) | `7890` |
| `proxy-mode` | 代理模式 `rule`/`global`/`direct` (热切换) | 跟随订阅 |
| `sub-auto-update-enabled` | 订阅定时自动更新 | `true` |
| `sub-auto-update-interval` | 订阅更新周期 (如 `12h`) | `24h` |
| `node-auto-select-enabled` | 定时对当前分组自动择优 | `false` |
| `node-auto-select-interval` | 自动择优周期 (如 `15m`) | `30m` |
| `test-url` / `test-timeout` | 测速 URL / 超时 ms | gstatic 204 / 5000 |

自动择优 (`node-auto-select-*`) 只作用于当前 `group use` 的分组; 手动 `node use` 时若自动择优开启会有黄色提示。
自动任务通过 systemd timer (`mihomo-cli-sub.timer` / `mihomo-cli-auto.timer`) 实现, `set` 修改后立即生效。

## 目录结构 (/etc/mihomo-cli)

```
/etc/mihomo-cli/
├── config.toml        # 设置 (644)
├── overrides.yaml     # 对订阅配置的深度覆盖 (dns/tun/rules 等)
├── profiles/          # 订阅原文
├── runtime/config.yaml# 内核实际配置(自动生成, 勿手改)
├── bin/mihomo         # 内核 (mihomo.old 为上一版本)
└── logs/mihomo.log
```

## 安装方式

```bash
# 一键脚本
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | sudo bash

# deb / rpm (GitHub Releases)
sudo dpkg -i mihomo-cli_linux_x86_64.deb   # 或 rpm -i

# 自更新到最新版
sudo mihomo-cli update
```

## License

MIT
