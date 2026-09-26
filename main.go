package main

import (
	"os"

	"github.com/wubinstu/mihomo-cli/internal/cmd"
)

func main() {
	// cobra 的空命令行补全 shim:
	// bash/zsh 补全脚本在"刚敲完命令名还没敲空格"时调用的是
	//     mihomo-cli __complete          (0 个参数)
	// 而 cobra 的 __complete 要求至少 1 个参数, 于是报错 → 输出为空 → directive 落到
	// ShellCompDirectiveDefault → shell 回退成"文件名补全"。这就是
	// "在 /etc/mihomo-cli 下敲 mihomo-cli <TAB> 会补出 config.toml" 的真正根因。
	// 补一个空参数, 让它正常走子命令补全 (NoFileComp)。
	if len(os.Args) > 1 && (os.Args[1] == "__complete" || os.Args[1] == "__completeNoDesc") && len(os.Args) < 3 {
		os.Args = append(os.Args, "")
	}
	cmd.Execute()
}
