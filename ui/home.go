package ui

import (
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/assistant"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/navpanel"
	"looz.ws/typstify/ui/preview"
	"looz.ws/typstify/ui/settings"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/console"
	appIcons "looz.ws/typstify/widgets/icons"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	previewIcon = appIcons.NewSvgIcon(appIcons.Columns2)
	exportIcon  = appIcons.NewSvgIcon(appIcons.FileDown)
)

// previewable is implemented by views that render preview related controls in
// their own chrome, e.g. the preview mode toggle in the status bar. The preview
// panel itself is project level and drawn by the home view.
type previewable interface {
	SetPreviewer(previewer *preview.Previewer)
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

	// project level actions rendered in the tab line.
	previewBtn widget.Clickable
	exportBtn  widget.Clickable

	welcome      WelcomeView
	accountClick widget.Clickable
}

func (hv *HomeView) ID() string {
	return "Home"
}

func (hv *HomeView) toggleConsole() {
	hv.consolePanel.ShowConsole = !hv.consolePanel.ShowConsole

	if !hv.consolePanel.ShowConsole {
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

	if hv.accountClick.Clicked(gtx) {
		hv.RequestSwitch(view.Intent{
			Target: settings.SettingViewID,
			Params: map[string]any{
				"tabIdx": 4, // hardcoded tpix tab index in setting page.
			},
		})
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
			return layout.Flex{
				Axis:      layout.Horizontal,
				Spacing:   layout.SpaceBetween,
				Alignment: layout.Middle,
				Gap:       gtx.Dp(unit.Dp(4)),
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return hv.tabbar.Layout(gtx, th)
				}),

				// The project actions are grouped with the account button so that
				// they stay next to it instead of spreading across the spare space
				// of the tab line.
				layout.Rigid(func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Middle,
						Gap:       gtx.Dp(unit.Dp(6)),
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return hv.layoutTopActions(gtx, th)
						}),
						layout.Rigid(func(gtx C) D {
							return hv.layoutAccountInfo(gtx, th)
						}),
					)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Spacer{Height: unit.Dp(1)}.Layout(gtx)
		}),

		layout.Flexed(1, func(gtx C) D {
			// Top offset = tabbar + spacer = rightPanelH - this child's height.
			hv.srv.ViewAreaTopOffset = rightPanelH - gtx.Constraints.Max.Y
			if !hv.consolePanel.ShowConsole {
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

	if pv, ok := cv.(previewable); ok {
		pv.SetPreviewer(hv.previewer)
	}

	// The preview panel belongs to the project instead of the active view, so it
	// stays visible while switching tabs.
	if !hv.previewer.IsVisible() {
		return cv.Layout(gtx, th)
	}

	if hv.previewResizer == nil {
		hv.previewResizer = &widgets.Resize{Axis: layout.Horizontal, Ratio: 0.7}
	}

	return hv.previewResizer.Layout(gtx,
		func(gtx C) D {
			return cv.Layout(gtx, th)
		},
		func(gtx C) D {
			return hv.previewer.Layout(gtx, th)
		},
		func(gtx C) D {
			if hv.previewBar == nil {
				hv.previewBar = widgets.NewResizeBar(layout.Vertical)
			}
			return hv.previewBar.Layout(gtx, th)
		},
	)
}

// layoutTopActions renders the project level actions, e.g. the preview toggle
// and the export dialog. They live in the tab line because they do not belong
// to any particular editor.
func (hv *HomeView) layoutTopActions(gtx C, th *theme.Theme) D {
	previewVisible := hv.previewer.IsVisible()

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
		Gap:       gtx.Dp(unit.Dp(8)),
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if hv.previewBtn.Clicked(gtx) {
				hv.previewer.Toggle()
				gtx.Execute(op.InvalidateCmd{})
			}

			return layoutActionIcon(gtx, th, &hv.previewBtn, previewIcon, previewVisible)
		}),
		layout.Rigid(func(gtx C) D {
			if hv.exportBtn.Clicked(gtx) {
				// The export dialog resolves the pinned entry file or the active
				// document by itself.
				hv.RequestSwitch(view.Intent{
					Target:      dialog.ExportDialogViewID,
					ShowAsModal: true,
				})
			}

			return layoutActionIcon(gtx, th, &hv.exportBtn, exportIcon, false)
		}),
	)
}

// layoutActionIcon renders an icon button used in the tab line. An active
// button is drawn with the accent color to reflect a toggled state.
func layoutActionIcon(gtx C, th *theme.Theme, btn *widget.Clickable, icon *appIcons.SvgIcon, active bool) D {
	return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			iconColor := th.Fg
			if btn.Hovered() || active {
				iconColor = th.ContrastBg
			}

			return icon.Layout(gtx, iconColor, th.TextSize)
		})
	})
}

func (hv *HomeView) layoutAccountInfo(gtx C, th *theme.Theme) D {
	userAuthed := hv.srv.Authenticated()
	username := hv.srv.Settings().Tpix().Username

	return hv.accountClick.Layout(gtx, func(gtx C) D {
		paintColor := th.Fg
		if hv.accountClick.Hovered() {
			paintColor = th.ContrastBg
		}

		return layout.Inset{
			Top:    unit.Dp(2),
			Bottom: unit.Dp(2),
			Left:   unit.Dp(4),
			Right:  unit.Dp(12),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
				Gap:       gtx.Dp(unit.Dp(4)),
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return userIcon.Layout(gtx, paintColor, th.TextSize*0.8)
				}),
				layout.Rigid(func(gtx C) D {
					if userAuthed {
						lb := material.Label(th.Theme, th.TextSize, username)
						lb.Color = paintColor
						return lb.Layout(gtx)
					}

					lb := material.Label(th.Theme, th.TextSize, i18n.Translate("Sign In"))
					lb.Color = paintColor
					return lb.Layout(gtx)

				}),
			)
		})
	})

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
