package main

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

func launchDesktop() error {
	assets, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("jide: load frontend assets: %w", err)
	}
	app := NewApp()
	if err := wails.Run(&options.App{
		Title:            "jide — Julia IDE",
		Width:            1440,
		Height:           920,
		DisableResize:    false,
		Frameless:        true,
		MinWidth:         1040,
		MinHeight:        680,
		WindowStartState: options.Normal,
		BackgroundColour: options.NewRGBA(12, 16, 24, 255),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	}); err != nil {
		return fmt.Errorf("jide: run desktop: %w", err)
	}
	return nil
}
