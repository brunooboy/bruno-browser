package main

import (
	"embed"
	"log"

	appcore "bruno-browser/internal/app"
	"bruno-browser/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	configuration, err := config.Default()
	if err != nil {
		log.Fatal(err)
	}
	core, err := appcore.New(configuration)
	if err != nil {
		log.Fatal(err)
	}
	desktop := NewDesktop(core, configuration.SettingsPath(), core.Account.OAuthConfigured())
	if err := wails.Run(&options.App{
		Title:            "Bruno Browser",
		Width:            1480,
		Height:           920,
		MinWidth:         980,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 5, G: 8, B: 10, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        desktop.startup,
		OnDomReady:       desktop.domReady,
		Bind:             []interface{}{desktop},
	}); err != nil {
		log.Fatal(err)
	}
}
