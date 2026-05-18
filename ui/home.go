package ui

import (
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/assistant"
	"looz.ws/typstify/ui/console"
	"looz.ws/typstify/ui/navpanel"
	"looz.ws/typstify/ui/preview"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// previewable is implemented by views that support an inline preview panel.
type previewable interface {
	IsVisible() bool
	SetPreviewer(previewer *preview.Previewer)
	LayoutPreview(gtx C, th *theme.Theme) D
}

type HomeView struct {
	view.ViewManager
	srv          *service.ServiceFacade
	sidebar      *navpanel.NavDrawer
	tabbar       *navpanel.Tabbar
	statusBar    *statusbar.StatusBar
	consolePanel *console.Console
	menuPanel    *navpanel.MenuPanel

	// horizontal resizer
	resizer         widgets.Resize
	lastResizeWidth int
	lastResizeRatio float32
	bar             *widgets.ResizeBar

	// console and main view resizer
	yresizer   *widgets.Resize
	lastYRatio float32
	hbar       *widgets.ResizeBar

	// preview resizer (view | preview split)
	previewResizer *widgets.Resize
	previewBar     *widgets.ResizeBar
	previewer      *preview.Previewer

	welcome WelcomeView
}

func (hv *HomeView) ID() string {
	return "Home"
}

func (hv *HomeView) toggleConsole() {
	hv.srv.Console().ShowConsole = !hv.srv.Console().ShowConsole

	if !hv.srv.Console().ShowConsole {
		hv.lastYRatio = hv.yresizer.Ratio
		hv.yresizer.Ratio = 1.0
	} else {
		hv.yresizer.Ratio = hv.lastYRatio

	}
}

func (hv *HomeView) toggleChat() {
	projectDir := hv.srv.CurrentProjectDir()
	if projectDir == "" {
		return
	}

	intent := view.Intent{
		Target:      assistant.AgentChatViewID,
		ShowAsModal: false,
		RequireNew:  false,
	}
	hv.srv.RequestSwitch(intent)
}

func (hv *HomeView) update(gtx C) {
	// handle events and states update
	showConsoleClicked, showChatClicked := hv.statusBar.Update(gtx)
	if showConsoleClicked {
		hv.toggleConsole()
	}
	if showChatClicked {
		hv.toggleChat()
	}

	// global key handler, without a focused target.
	for {
		e, ok := gtx.Event(
			key.Filter{Name: "D", Required: key.ModShortcut}, // toggle hide/show of drawer.
			key.Filter{Name: "K", Required: key.ModShortcut}, // toggle hide/show of console.
			key.Filter{Name: "L", Required: key.ModShortcut}, // toggle hide/show of chat.
		)
		if !ok {
			break
		}

		switch event := e.(type) {
		case key.Event:
			if event.State != key.Press {
				continue
			}

			if event.Name == "D" && event.Modifiers.Contain(key.ModShortcut) {
				hv.menuPanel.IsDrawerHidden = !hv.menuPanel.IsDrawerHidden
			}

			if event.Name == "K" && event.Modifiers.Contain(key.ModShortcut) {
				hv.toggleConsole()
			}

			if event.Name == "L" && event.Modifiers.Contain(key.ModShortcut) {
				hv.toggleChat()
			}
		}
	}
}

func (hv *HomeView) Layout(gtx C, th *theme.Theme, deco *widget.Decorations, title string) layout.Dimensions {
	hv.update(gtx)

	dims := layout.Flex{
		Axis:      layout.Vertical,
		Alignment: layout.Start,
	}.Layout(gtx,
		// layout.Rigid(func(gtx C) D {
		// 	d := decoration.Decorations(th, deco, ^system.Action(0), title)
		// 	d.Background = th.Bg2
		// 	d.Foreground = th.Fg
		// 	d.Title.Color = th.Fg
		// 	return d.Layout(gtx)
		// }),
		layout.Flexed(1, func(gtx C) D {
			// Store window layout metrics for native webview positioning.
			hv.srv.WindowContentWidth = gtx.Constraints.Max.X

			if hv.resizer == (widgets.Resize{}) {
				hv.resizer.Axis = layout.Horizontal
				hv.resizer.Ratio = float32(gtx.Dp(unit.Dp(280))) / float32(gtx.Constraints.Max.X)
				hv.lastResizeWidth = gtx.Constraints.Max.X
				hv.lastResizeRatio = hv.resizer.Ratio
			}

			if hv.lastResizeWidth != gtx.Constraints.Max.X {
				hv.resizer.Ratio = (float32(hv.lastResizeWidth) * hv.lastResizeRatio) / float32(gtx.Constraints.Max.X)
				hv.lastResizeWidth = gtx.Constraints.Max.X
				hv.lastResizeRatio = hv.resizer.Ratio
			} else if hv.lastResizeRatio != hv.resizer.Ratio {
				hv.lastResizeWidth = gtx.Constraints.Max.X
				hv.lastResizeRatio = hv.resizer.Ratio
			}

			if hv.menuPanel.IsDrawerHidden {
				return hv.layoutMain(gtx, th)
			}

			return hv.resizer.Layout(gtx,
				// navdrawer
				func(gtx C) D {
					return navpanel.NaviDrawerStyle{
						NavDrawer: hv.sidebar,
						Bg:        th.Bg2,
					}.Layout(gtx, th)

				},
				// switchable view
				func(gtx C) D {
					return hv.layoutMain(gtx, th)
				},

				func(gtx C) D {
					if hv.bar == nil {
						hv.bar = widgets.NewResizeBar(layout.Vertical)
					}

					return hv.bar.Layout(gtx, th)
				},
			)
		}),
		layout.Rigid(func(gtx C) D {
			rect := clip.Rect{Max: gtx.Constraints.Max}
			paint.FillShape(gtx.Ops, th.Bg2, rect.Op())
			return layout.Flex{
				Gap:     gtx.Dp(unit.Dp(4)),
				Spacing: layout.SpaceBetween,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return hv.menuPanel.Layout(gtx, th)
				}),
				layout.Flexed(1, func(gtx C) D {
					return hv.statusBar.Layout(gtx, th)

				}),
			)
		}),
	)

	modalIter := hv.ModalViews()

	var allModals []*view.ModalView
	for modal := range modalIter {
		modal.Halted = true
		modal.MaxWidth = unit.Dp(960)
		modal.MaxHeight = 0.8
		modal.Radius = unit.Dp(8)
		modal.Padding = layout.Inset{
			Top:    unit.Dp(24),
			Bottom: unit.Dp(24),
			Left:   unit.Dp(20),
			Right:  unit.Dp(20),
		}

		allModals = append(allModals, modal)

	}

	for i, modal := range allModals {
		modal.ShowUp(gtx)

		if i == len(allModals)-1 {
			modal.Halted = false
		}

		// closing modal view
		if modal.IsClosed(gtx) {
			// should be the top most view.
			hv.FinishModalView()
			gtx.Execute(op.InvalidateCmd{})
		} else {
			modal.Layout(gtx, th)
		}

	}

	return dims
}

func (hv *HomeView) layoutMain(gtx C, th *theme.Theme) D {
	// draw the background
	gtx.Constraints.Min = gtx.Constraints.Max
	rect := clip.Rect{Max: gtx.Constraints.Max}
	paint.FillShape(gtx.Ops, th.Bg, rect.Op())

	rightPanelH := gtx.Constraints.Max.Y

	return layout.Flex{
		Axis:      layout.Vertical,
		Alignment: layout.Middle,
	}.Layout(gtx,
		// horizontal navbar
		layout.Rigid(func(gtx C) D {
			return hv.tabbar.Layout(gtx, th)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Spacer{Height: unit.Dp(1)}.Layout(gtx)
		}),

		layout.Flexed(1, func(gtx C) D {
			// Top offset = tabbar + spacer = rightPanelH - this child's height.
			hv.srv.ViewAreaTopOffset = rightPanelH - gtx.Constraints.Max.Y
			if !hv.srv.Console().ShowConsole {
				return hv.layoutView(gtx, th)
			}

			return hv.yresizer.Layout(gtx,
				func(gtx C) D {
					return hv.layoutView(gtx, th)
				},
				func(gtx C) D {
					return hv.consolePanel.Layout(gtx, th)
				},
				func(gtx C) D {
					if hv.hbar == nil {
						hv.hbar = widgets.NewResizeBar(layout.Horizontal)
					}

					return hv.hbar.Layout(gtx, th)
				},
			)
		}),
	)

}

func (hv *HomeView) layoutView(gtx C, th *theme.Theme) D {
	cv := hv.CurrentView()
	if cv == nil {
		return hv.welcome.Layout(gtx, th)
	}

	pv, ok := cv.(previewable)
	if ok {
		pv.SetPreviewer(hv.previewer)
	}

	showPreview := ok && pv.IsVisible()

	if !showPreview {
		return cv.Layout(gtx, th)
	}

	// Preview is visible.
	if hv.previewResizer == nil {
		hv.previewResizer = &widgets.Resize{Axis: layout.Horizontal, Ratio: 0.7}
	}

	return hv.previewResizer.Layout(gtx,
		func(gtx C) D {
			return cv.Layout(gtx, th)
		},
		func(gtx C) D {
			return pv.LayoutPreview(gtx, th)
		},
		func(gtx C) D {
			if hv.previewBar == nil {
				hv.previewBar = widgets.NewResizeBar(layout.Vertical)
			}
			return hv.previewBar.Layout(gtx, th)
		},
	)
}

func (hv *HomeView) OnClose() {
	hv.sidebar.Close()
	if hv.previewer != nil {
		hv.previewer.Destroy()
	}
}

func newHome(window *app.Window, srv *service.ServiceFacade) *HomeView {
	vm := view.DefaultViewManager(window)

	return &HomeView{
		ViewManager:  vm,
		srv:          srv,
		tabbar:       navpanel.NewTabbar(vm, nil),
		sidebar:      navpanel.NewNavDrawer(vm, srv),
		statusBar:    statusbar.NewStatusBar(srv, vm),
		consolePanel: console.NewConsolePanel(srv.Console()),
		menuPanel:    navpanel.NewMenuPanel(vm, srv),
		yresizer:     &widgets.Resize{Axis: layout.Vertical, Ratio: 1.0},
		lastYRatio:   0.7,
		welcome:      WelcomeView{vm: vm, srv: srv},
		previewer:    preview.NewPreviewer(srv),
	}
}
