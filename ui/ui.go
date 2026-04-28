package ui

import (
	"context"
	"log"
	"runtime/debug"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/gioui-plugins/gio-plugins/plugin/gioplugins"
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"

	"looz.ws/typstify/fonts"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/editors"
	"looz.ws/typstify/ui/palette"
	"looz.ws/typstify/ui/pkgmgmt"
	st "looz.ws/typstify/ui/settings"
	"looz.ws/typstify/ui/viewer"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

type UI struct {
	window      *app.Window
	theme       *theme.Theme
	vm          *HomeView
	srv         *service.ServiceFacade
	deco        widget.Decorations
	title       string
	crashReport *CrashReport
}

func (ui *UI) Loop(ctx context.Context) error {
	width, height := ui.getWindowSize()
	ui.window.Option(
		app.Title("Typstify"),
		app.Decorated(true),
		app.MinSize(unit.Dp(960), unit.Dp(640)),
		app.Size(width, height),
	)
	ui.window.Perform(system.ActionCenter)

	ui.registerViews()

	ui.srv.InitFileChooser(func() *explorer.FileChooser {
		// init file explorer
		exp, err := explorer.NewFileChooser(ui.vm.ViewManager)
		if err != nil {
			log.Println("cannot build file chooser", err)
			return nil
		}

		return exp
	})

	var ops op.Ops
	for {
		select {
		case <-ctx.Done():
			log.Println("quit UI loop")
			return nil
		default:
			// continue
		}

		evt := gioplugins.Hijack(ui.window)
		// evt := ui.window.Event()

		switch e := evt.(type) {
		case app.DestroyEvent:
			log.Println("exiting application")
			ui.vm.OnClose()
			ui.vm.Reset() // release resources.
			return e.Err
		case app.ConfigEvent:
			ui.deco.Maximized = e.Config.Mode == app.Maximized
			ui.title = e.Config.Title
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if ui.crashReport == nil {
				ui.Layout(gtx)
			} else {
				ui.crashReport.Layout(gtx, ui.theme)
			}

			e.Frame(gtx.Ops)
			// update window size
			ui.srv.Workspace().RememberWindowSize(int(gtx.Metric.PxToDp(e.Size.X)), int(gtx.Metric.PxToDp(e.Size.Y)))

		}
	}
}

func (ui *UI) Layout(gtx C) D {
	defer func() {
		if r := recover(); r != nil {
			ui.crashReport = &CrashReport{}
			ui.crashReport.logCrashReport(r, debug.Stack())
		}
	}()

	//ui.window.Perform(ui.deco.Update(gtx))
	return ui.vm.Layout(gtx, ui.theme, &ui.deco, ui.title)
}

func (ui *UI) registerViews() {
	vm := newHome(ui.window, ui.srv)

	vm.Register(editors.TypstEditorViewID, func() view.View { return editors.NewTypstEditor(ui.srv) })
	vm.Register(editors.GenericTextEditorViewID, func() view.View { return editors.NewGenericTextEditor(ui.srv) })
	vm.Register(viewer.ImgViewerViewID, viewer.NewImgViewerView)
	vm.Register(st.SettingViewID, func() view.View { return st.NewSettingsView(ui.srv) })
	vm.Register(pkgmgmt.PkgListViewID, func() view.View { return pkgmgmt.NewPkgListView(ui.srv, vm) })
	vm.Register(dialog.CreateProjectDialogViewID, func() view.View { return dialog.NewCreateProjectDialog(ui.srv) })
	vm.Register(dialog.ExportDialogViewID, func() view.View { return dialog.NewExportDialog(ui.srv) })
	vm.Register(dialog.DeleteFileDialogViewID, func() view.View { return dialog.NewDeleteFileDialog() })
	vm.Register(dialog.DndDropFileDialogViewID, func() view.View { return dialog.NewDndDropFileDialog() })
	vm.Register(dialog.ChangeIndentationDialogViewID, func() view.View { return dialog.NewChangeIndentationDialog() })
	vm.Register(dialog.OpenWithExternalAppDialogViewID, dialog.NewOpenWithExternalAppDialog)
	vm.Register(dialog.PublishPkgDialogViewID, func() view.View { return dialog.NewPublishPkgDialog(ui.srv) })
	vm.Register(dialog.SyncBibDialogViewID, func() view.View { return dialog.NewSyncBibDialog(ui.srv) })
	vm.Register(dialog.ViewBibInfoDialogViewID, func() view.View { return dialog.NewBibInfoDialog(ui.srv) })

	ui.vm = vm
	ui.srv.SetViewManager(vm.ViewManager)
}

func (ui *UI) getWindowSize() (width unit.Dp, height unit.Dp) {
	width = unit.Dp(960)
	height = unit.Dp(640)

	lastAppState := ui.srv.Workspace().GetAppState()
	if lastAppState == nil {
		return
	}

	if lastAppState.WindowSize[0] <= 0 || lastAppState.WindowSize[1] <= 0 {
		return
	}

	width = unit.Dp(lastAppState.WindowSize[0])
	height = unit.Dp(lastAppState.WindowSize[1])
	return
}

func NewUI(srv *service.ServiceFacade, enableProfiler bool) *UI {
	i18n.SetLocale(srv.Settings().General().Language)
	w := &app.Window{}

	appUI := &UI{
		window: w,
		srv:    srv,
	}

	// load from settings
	appUI.loadTheme(srv.Settings())

	srv.EventBus().Subscribe(appUI, "ui.onSettingsChanged", `settings\.updated`, func(topic string, data interface{}) {
		appUI.loadTheme(srv.Settings())
		i18n.SetLocale(srv.Settings().General().Language)
		appUI.window.Invalidate()
	})

	return appUI
}

func (ui *UI) loadTheme(s *settings.Settings) {
	if ui.theme == nil {
		ui.theme = theme.NewTheme("", fonts.Embedded, false)
	}

	themeName := s.General().Theme
	if themeName == "" {
		themeName = "Default Light"
	}

	cfg, err := palette.ThemeConfig(themeName)
	if err != nil {
		log.Println("Theme query failed: ", err)
		return
	}

	ui.theme.TextSize = unit.Sp(s.General().TextSize)
	ui.theme.Face = font.Typeface(s.General().TypeFace)
	ui.theme = ui.theme.WithPalette(cfg.Palette)
	ui.theme.Register("codeColorScheme", cfg.CodeColorScheme)
}
