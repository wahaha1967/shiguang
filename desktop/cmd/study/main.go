package main

import (
	"flag"
	"os"

	"shiguang-study/desktop/internal/app"
)

func main() {
	serve := flag.Bool("serve", false, "只启动本地 HTTP 服务，不打开窗口（调试用）")
	port := flag.Int("port", 0, "监听端口，0 表示随机端口")
	open := flag.Bool("open", false, "在 --serve 模式下自动打开浏览器")
	flag.Parse()

	if err := app.Run(app.Options{Serve: *serve, Port: *port, Open: *open}); err != nil {
		app.ReportError(err)
		os.Exit(1)
	}
}
