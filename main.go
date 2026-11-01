package main

import (
	"embed"
	"log"
	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/windowlife"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == sourceruntime.WorkerFlag {
		os.Exit(sourceruntime.WorkerMain())
	}
	if len(os.Args) > 1 && os.Args[1] == sourceruntime.ResolveWorkerFlag {
		os.Exit(sourceruntime.ResolveWorkerMain())
	}
	app, err := openApp()
	if err != nil {
		log.Fatal("无法打开本地数据库: ", err)
	}
	defer func() {
		app.closeDownloads()
		app.closePlayback()
		app.sources.Close()
		app.playDiagnostics.Close()
		if err := app.store.Close(); err != nil {
			log.Printf("关闭本地数据库失败: %v", err)
		}
	}()
	appOptions := &options.App{
		Title:     "妙音娘子",
		Bind:      []interface{}{app},
		OnStartup: app.startup,

		Width: 1180, Height: 800,
		MinWidth: 720, MinHeight: 600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 255},
	}
	windowlife.Configure(appOptions, app.windowContext, app.beforeQuit)
	if err := wails.Run(appOptions); err != nil {
		log.Print(err)
	}
}
