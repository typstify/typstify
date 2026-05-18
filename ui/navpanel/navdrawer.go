package navpanel

import (
	"image"
	"image/color"

	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/assistant"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type NaviDrawerStyle struct {
	*NavDrawer
	Bg color.NRGBA
}

type NavSection interface {
	Title() string
	Icon() *icons.SvgIcon
	Layout(gtx C, th *theme.Theme) D
	LayoutHeader(gtx C, th *theme.Theme) D
	OnClose()
}

type NavDrawer struct {
	srv                *service.ServiceFacade
	vm                 view.ViewManager
	sections           []NavSection
	currentSectionIdx  int
	switchSectionBtn   widget.Clickable
	sectionSwitchPopup *widgets.Popup
	updateTips         *UpdateTips
}

func NewNavDrawer(vm view.ViewManager, srv *service.ServiceFacade) *NavDrawer {
	drawer := &NavDrawer{
		srv:        srv,
		vm:         vm,
		updateTips: &UpdateTips{srv: srv},
	}

	drawer.AddSection(NewFileTreeNav(i18n.Translate("File Explorer"), srv, vm))

	outlineNav := NewOutlineNav()
	outlineNav.SetProvider(func() OutlineProvider {
		// Update outline provider from current view.
		if cv := vm.CurrentView(); cv != nil {
			if op, ok := cv.(OutlineProvider); ok {
				return op
			} else {
				return nil
			}
		} else {
			return nil
		}
	})

	sessionsPanel := assistant.NewSessionHistory(srv)

	drawer.AddSection(outlineNav)
	drawer.AddSection(sessionsPanel)

	drawer.sectionSwitchPopup = &widgets.Popup{
		MaxHeight: unit.Dp(400),
		Width:     unit.Dp(250),
		Direction: layout.S,
	}

	return drawer
}

func (nv *NavDrawer) AddSection(section NavSection) {
	nv.sections = append(nv.sections, section)
}

func (nv *NavDrawer) Layout(gtx C, th *theme.Theme) D {
	section := nv.sections[nv.currentSectionIdx]

	if nv.switchSectionBtn.Clicked(gtx) {
		nv.sectionSwitchPopup.SetOpen()
	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return nv.layoutHeader(gtx, th, section)
		}),
		layout.Flexed(1, func(gtx C) D {
			if section == nil {
				return D{}
			}

			return layout.Inset{
				Top:    unit.Dp(2),
				Bottom: unit.Dp(2),
			}.Layout(gtx, func(gtx C) D {
				return section.Layout(gtx, th)
			})
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(1)}.Layout),

		layout.Rigid(func(gtx C) D {
			return nv.updateTips.Layout(gtx, th)
		}),
	)

}

func (nv *NavDrawer) layoutHeader(gtx C, th *theme.Theme, section NavSection) D {

	return layout.Background{}.Layout(gtx,
		func(gtx C) D {
			defer clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Min.Y}}.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, th.Bg2)

			return D{Size: gtx.Constraints.Min}
		},
		func(gtx C) D {
			return widget.Border{Width: unit.Dp(0.5), Color: misc.WithAlpha(th.Fg, 0x30)}.Layout(gtx, func(gtx C) D {
				return layout.Inset{
					Left:   unit.Dp(6),
					Right:  unit.Dp(6),
					Top:    unit.Dp(6),
					Bottom: unit.Dp(6),
				}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return section.Icon().Layout(gtx, th.Fg, th.TextSize)

						}),

						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Flexed(1, func(gtx C) D {
							if section == nil {
								return D{}
							}

							return section.LayoutHeader(gtx, th)
						}),

						layout.Rigid(func(gtx C) D {
							return nv.layoutButton(gtx, th)
						}),
					)

				})
			})

		},
	)
}

func (nv *NavDrawer) layoutButton(gtx C, th *theme.Theme) D {
	sectionItems := make([]widgets.PopupWidget, 0)
	for idx, s := range nv.sections {
		sectionItems = append(sectionItems, sectionItem{
			section:   s,
			isCurrent: nv.currentSectionIdx == idx,
			onClick: func() {
				nv.currentSectionIdx = idx
			},
		})
	}

	return nv.sectionSwitchPopup.Layout(gtx, th,
		func(gtx C) D {
			return nv.switchSectionBtn.Layout(gtx, func(gtx C) D {
				return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
					return ellipsisIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			})
		},

		sectionItems...,
	)

}

func (nv *NavDrawer) Close() {
	for _, s := range nv.sections {
		s.OnClose()
	}
}

func (ns NaviDrawerStyle) Layout(gtx C, th *theme.Theme) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	rect := clip.Rect{
		Max: gtx.Constraints.Max,
	}
	paint.FillShape(gtx.Ops, ns.Bg, rect.Op())

	return ns.NavDrawer.Layout(gtx, th)

}

type sectionItem struct {
	section   NavSection
	isCurrent bool
	onClick   func()
}

func (s sectionItem) OnClicked() {
	s.onClick()
}

func (s sectionItem) Layout(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Horizontal,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if s.isCurrent {
					return material.Label(th.Theme, th.TextSize, "\u2713").Layout(gtx)
				} else {
					return s.section.Icon().Layout(gtx, th.Fg, th.TextSize)
				}
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				return material.Label(th.Theme, th.TextSize, s.section.Title()).Layout(gtx)
			}),
		)
	})
}
