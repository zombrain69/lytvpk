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

// AppVersion 会在正式 Release 时通过 -ldflags 注入。默认值必须保持为当前
// Community Fork 正式版，避免直接构建的 EXE 被更新器误判为 0.0.0-dev。
// UpdateRepo 默认为本 Community Fork，且可由 -ldflags 覆盖为同一 Fork 的
// 迁移仓库；上游 LaoYutang/lytvpk 永远只用于署名，不会成为更新源。
var (
	AppVersion = "2.5.14-community.14"
	UpdateRepo = "zombrain69/lytvpk"
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
