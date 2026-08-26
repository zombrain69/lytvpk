package main

import (
	"embed"
	"os"

	backend "vpk-manager/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// AppVersion 和 UpdateRepo 会在正式 Fork 发布时通过 -ldflags 注入。
// 源码/本地构建保持开发版本且不配置更新源，避免误更新到上游项目。
var (
	AppVersion = "0.0.0-dev"
	UpdateRepo string
)

func main() {
	backend.AppVersion = AppVersion
	backend.UpdateRepo = UpdateRepo

	// 确保单例运行
	// 如果已有实例运行，会将参数传递给已有实例并退出
	ensureSingleInstance(os.Args)

	// Create an instance of the app structure
	app := backend.NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "LytVPK Community Fork — MOD管理器",
		Width:     1400,
		Height:    900,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.Startup,
		OnBeforeClose:    app.BeforeClose,
		Bind: []any{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
