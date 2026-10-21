package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/chuccp/auto-chess/internal/api"
	"github.com/chuccp/auto-chess/internal/registry"
	"github.com/chuccp/auto-chess/internal/webui"
	wf "github.com/chuccp/go-web-frame"
)

func main() {
	cfg, err := wf.LoadConfig("application.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 application.yml 失败，使用默认配置：%v\n", err)
		cfg = wf.LoadAutoConfig()
		cfg.Put("web.server.port", 8090)
		cfg.Put("web.log.level", "info")
	}
	if raw := os.Getenv("PORT"); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port < 65536 {
			cfg.Put("web.server.port", port)
		}
	}
	if raw := os.Getenv("CONTEXT_PATH"); raw != "" {
		cfg.Put("web.server.context_path", raw)
	}

	reg := registry.New()
	renderer, err := webui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载页面模板失败：%v\n", err)
		os.Exit(1)
	}
	app := wf.NewBuilder(cfg).
		Filter(&api.CORSFilter{}).
		Filter(&api.RecoveryFilter{}).
		Rest(&api.Controller{Registry: reg, Renderer: renderer, StartedAt: time.Now()}).
		Build()

	port := cfg.GetIntOrDefault("web.server.port", 19009)
	fmt.Printf("auto-chess AI 服务已启动\n")
	fmt.Printf("  首页:   http://127.0.0.1:%d/\n", port)
	fmt.Printf("  文档:   http://127.0.0.1:%d/docs\n", port)
	fmt.Printf("  走子:   POST http://127.0.0.1:%d/api/v1/move\n", port)
	fmt.Printf("  流式:   GET/POST http://127.0.0.1:%d/api/v1/move/stream\n", port)
	fmt.Printf("  引擎:   chess=chess-3, xiangqi=godogpaw（纯 Go）\n")
	fmt.Printf("  棋类:   %d 种（%v）\n", len(reg.List()), reg.IDs())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "服务退出：%v\n", err)
		os.Exit(1)
	}
}
