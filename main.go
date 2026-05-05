// install-minions — sets up and launches the Dominion runtime.
// This is the Wails v2 entry point; the GUI replaces the previous CLI interface.
package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:         "The Dominion",
		Width:         620,
		Height:        600,
		MinWidth:      620,
		MinHeight:     600,
		MaxWidth:      620,
		MaxHeight:     600,
		DisableResize: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 15, B: 23, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisablePinchZoom:     true,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		// Wails failed to start (e.g. WebView2 missing). Write to a log file
		// since we have no UI at this point.
		logPath := filepath.Join(appDataDir(), "install-minions-error.log")
		_ = os.WriteFile(logPath, []byte(err.Error()+"\n"), 0644)
		os.Exit(1)
	}
}
