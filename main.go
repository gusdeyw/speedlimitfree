package main

import (
	"context"
	"embed"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"log"
	"os"
	"path/filepath"
	"slices"
	"speedlimitfree/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

func main() {
	app := desktop.New(trayIcon)
	exe, _ := os.Executable()
	userData, err := desktop.UserDataPath()
	if err != nil {
		log.Fatal(err)
	}
	startInTray := slices.Contains(os.Args[1:], "--tray")
	if err := wails.Run(&options.App{
		Title: "SpeedLimitFree", Width: 1280, Height: 820, MinWidth: 920, MinHeight: 640,
		BackgroundColour: desktop.BackgroundColour(), AssetServer: &assetserver.Options{Assets: assets},
		OnStartup: app.Startup, OnShutdown: app.Shutdown, OnBeforeClose: app.BeforeClose, Bind: []interface{}{app},
		StartHidden: startInTray,
		OnDomReady: func(ctx context.Context) {
			if startInTray && !app.BeforeClose(ctx) {
				app.Show()
			}
		},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "speedlimitfree-desktop-" + filepath.Base(exe), OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
			if !slices.Contains(data.Args, "--tray") {
				app.Show()
			}
		}},
		Windows: &windows.Options{WebviewIsTransparent: false, WindowIsTranslucent: false, Theme: windows.SystemDefault, WebviewUserDataPath: userData},
	}); err != nil {
		log.Fatal(err)
	}
}
