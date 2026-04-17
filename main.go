package main

import (
	"fmt"
	"os"

	"ssh-tools/src"
)

func main() {
	cfg, configPath, err := src.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	app := src.NewApp(cfg, configPath)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "程序异常退出: %v\n", err)
		os.Exit(1)
	}
}
