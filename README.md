# mihomo-cli

面向 Linux 服务器的纯命令行 Clash 代理工具 —— [mihomo](https://github.com/MetaCubeX/mihomo) 内核的 CLI 外壳。无 GUI、单二进制、systemd 托管，专为无图形界面的服务器设计。

## 特性

- **一键安装**: 自动从 GitHub Releases 下载 mihomo 内核并注册 systemd 服务
- **订阅管理**: 添加/更新/切换多个订阅, 定时自动更新并热重载
- **自动择优**: 定时测速, 自动把分组切换到延迟最低的节点 (可配置分组/周期/测速 URL)
- **节点切换**: `proxy set` 支持模糊匹配 (emoji 组名友好)
- **观测**: 活动连接、实时流量、内核日志、体检 (`doctor`)
- **局域网共享**: `set allow-lan true` 监听 `0.0.0.0`, 供局域网设备使用
- **环境变量**: `eval $(mihomo-cli env)` 一键给当前 shell 挂代理

CLI 本体为 `mihomo-cli`, 内核存放在 `~/.config/mihomo-cli/bin/mihomo` (不进入 PATH), 二者互不冲突。

## 快速开始

```bash
# 安装 (中国大陆服务器建议 --proxy 走代理下载内核)
curl -fsSL https://raw.githubusercontent.com/wubinstu/mihomo-cli/main/install.sh | bash -s -- --proxy http://192.168.1.1:7890

mihomo-cli init                # 粘贴订阅链接
mihomo-cli start               # 启动
eval $(mihomo-cli env)         # 当前 shell 开启代理
mihomo-cli doctor              # 体检
```

或手动安装:

```bash
mihomo-cli install [--proxy http://host:port] [--sub <订阅URL>] [--allow-lan]
```

## 常用命令

```
mihomo-cli start|stop|restart|status    服务生命周期
mihomo-cli run                          前台运行(调试)
mihomo-cli sub add|rm|list|update|use   订阅管理
mihomo-cli proxy                        查看分组 (CJK 宽度对齐)
mihomo-cli proxy set <组> <节点>         切换节点(模糊匹配)
mihomo-cli proxy test [组]              节点测速
mihomo-cli proxy auto                   立即择优一次
mihomo-cli proxy on/off                 开/关当前 shell 代理(配合 alias)
mihomo-cli conn [--watch]               活动连接
mihomo-cli traffic                      实时流量
mihomo-cli log [-f]                     内核日志
mihomo-cli core upgrade|rollback        内核升级/回滚
mihomo-cli env [--unset]                代理环境变量
mihomo-cli doctor                       体检
mihomo-cli get/set [key]                查看/修改设置
mihomo-cli uninstall [--purge]          卸载
```

推荐 alias（加到 `~/.bashrc`）:

```bash
alias proxy_on='eval $(mihomo-cli proxy on)'
alias proxy_off='eval $(mihomo-cli proxy off)'
```

bash/zsh/fish 补全脚本随 `install` 自动安装、随 `uninstall` 清理。

## 配置 (`mihomo-cli get [key]` / `mihomo-cli set key value`)

| key | 说明 | 默认 |
|---|---|---|
| `lang` | 输出语言 `zh`/`en` | 按系统 locale, 回退中文 |
| `allow-lan` | 允许局域网设备使用代理 (0.0.0.0) | `false` |
| `mixed-port` | 混合代理端口 (http+socks5) | `7890` |
| `sub-auto-update-enabled` | 订阅定时自动更新 | `true` |
| `sub-auto-update-interval` | 订阅更新周期 (如 `12h`) | `24h` |
| `auto-select-enabled` | 自动切换到最低延迟节点 | `false` |
| `auto-select-interval` | 自动择优周期 (如 `15m`) | `30m` |
| `auto-groups` | 择优作用的分组, 逗号分隔 (空=全部含真实节点的分组) | 空 |
| `test-url` / `test-timeout` | 测速 URL / 超时 ms | gstatic 204 / 5000 |
| `download-proxy` | 下载内核/订阅使用的代理 | 直连 |

自动任务通过 systemd timer (`mihomo-cli-sub.timer` / `mihomo-cli-auto.timer`) 实现, `set` 修改后立即生效。

### 高级自定义

`~/.config/mihomo-cli/overrides.yaml` 中的任意键会深度合并覆盖订阅配置 (如 `dns`、`tun`、`rules`), 订阅更新不会冲掉自定义。

## 目录结构

```
~/.config/mihomo-cli/
├── config.toml        # cli 设置
├── overrides.yaml     # 对订阅配置的深度覆盖
├── profiles/          # 订阅原文
├── runtime/config.yaml# 内核实际配置(自动生成, 勿手改)
├── bin/mihomo         # 内核 (mihomo.old 为上一版本)
└── logs/mihomo.log
```

## 架构

见 [软件设计说明.md](软件设计说明.md)。CLI 通过 systemd 托管内核子进程, 并经 `external-controller` API (仅 127.0.0.1 + 随机 secret) 控制内核。

## License

MIT
