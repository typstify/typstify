package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	//"github.com/pkg/profile"

	"looz.ws/typstify/logger"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui"
)

func main() {
	// log.SetFlags(log.Default().Flags() | log.Llongfile)
	//defer profile.Start(profile.CPUProfile).Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := service.NewService(ctx)
	// init logger
	logger.InitLogger(filepath.Join(srv.Settings().General().RootDir, "application.log"))
	defer logger.AppLogger.Close()

	ui := ui.NewUI(srv, false)

	go func() {
		err := ui.Loop(ctx)
		if err != nil {
			log.Println(err)
		}
		srv.Close(ctx)
		os.Exit(0)
	}()

	app.Main()
}
