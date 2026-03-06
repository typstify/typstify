package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	//"github.com/pkg/profile"

	"looz.ws/typstify/logger"
	"looz.ws/typstify/preview"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui"
)

func main() {
	// log.SetFlags(log.Default().Flags() | log.Llongfile)
	//defer profile.Start(profile.CPUProfile).Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	previewCmd := flag.NewFlagSet("preview", flag.ExitOnError)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "preview":
			previewCmd.Parse(os.Args[2:])
			previewServer := preview.NewPreviewServer()
			defer previewServer.Close()
			// Run starts the server and blocks the thread.
			previewServer.Run()
		}
	} else {
		// And then the main UI loop.
		startMain(ctx)
	}

}

func startMain(ctx context.Context) {
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

	// Proxy Gio events to gioplugins
	//go gioplugins.ProxyEvents(app.Events)

	app.Main()
}
