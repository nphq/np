package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := app.NewApp()

	wailsApp := application.New(application.Options{
		Name: "Nomad Panel",
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Nomad Panel",
		Width:            1024,
		Height:           768,
		BackgroundColour: application.NewRGBA(27, 38, 54, 255),
		URL:              "/",
	})

	if err := wailsApp.Run(); err != nil {
		log.Printf("wails run: %v", err)
		os.Exit(1)
	}
}
