package navpanel

import (
	"image"
	"image/color"
	"time"

	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"

	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	cmp "gioui.org/x/component"
)

type NaviDrawerStyle struct {
	*NavDrawer
	Bg color.NRGBA
}

type NavDrawer struct {
	srv          *service.ServiceFacade
	vm           view.ViewManager
	FileExplorer *FileTreeNav

	cmdPanel       *CommandPanel
	recentProjects *RecentProjects
	updateTips     *UpdateTips

	listState       *widget.List
	explorerSection *NavSection
	recentSection   *NavSection
}

type NavSection struct {
	click           gesture.Click
	anim            *cmp.VisibilityAnimation
	menu            *menu.ContextMenu
	expanded        bool
	collapseOnClick bool
	onClicked       func()
}

func NewNavDrawer(vm view.ViewManager, srv *service.ServiceFacade) *NavDrawer {
	return &NavDrawer{
		srv:            srv,
		vm:             vm,
		FileExplorer:   NewFileTreeNav(i18n.Translate("File Explorer"), srv, vm),
		cmdPanel:       NewCommandPanel(vm, srv),
		recentProjects: NewRecentProjects(vm, srv),
		updateTips:     &UpdateTips{srv: srv},
		listState: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		explorerSection: &NavSection{
			expanded:        true,
			collapseOnClick: false,
			anim: &cmp.VisibilityAnimation{
				State:    cmp.Visible,
				Duration: time.Millisecond * 150,
			},
		},
		recentSection: &NavSection{expanded: false, collapseOnClick: true},
	}
}

func (nv *NavDrawer) Layout(gtx C, th *theme.Theme) D {
	if nv.recentProjects.Update(gtx) {
		nv.explorerSection.expanded = true
	}

	return layout.Flex{
		Axis: layout.Horizontal,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{
				Top:   unit.Dp(8),
				Left:  unit.Dp(6),
				Right: unit.Dp(6),
			}.Layout(gtx, func(gtx C) D {
				return nv.cmdPanel.Layout(gtx, th)
			})
			callOp := macro.Stop()
			defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, th.Bg2)
			callOp.Add(gtx.Ops)
			return dims
		}),

		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx C) D {
			d := misc.Divider(layout.Vertical, unit.Dp(1))
			d.Fill = misc.WithAlpha(th.Fg, th.SelectedAlpha)
			return d.Layout(gtx, th)
		}),

		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,

				layout.Flexed(1, func(gtx C) D {
					return nv.explorerSection.Layout(gtx, th, viewListIcon, nv.FileExplorer.Title(), func(gtx C) D {
						return nv.FileExplorer.Layout(gtx, th)
					})
				}),

				layout.Rigid(layout.Spacer{Height: unit.Dp(1)}.Layout),

				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y/2, gtx.Dp(unit.Dp(400)))
					return nv.recentSection.Layout(gtx, th, historyIcon, nv.recentProjects.Title(), func(gtx C) D {
						return nv.recentProjects.Layout(gtx, th)
					})
				}),

				layout.Rigid(func(gtx C) D {
					return nv.updateTips.Layout(gtx, th)
				}),
			)
		}),
	)

}

func (ns *NavSection) Layout(gtx C, th *theme.Theme, icon *widget.Icon, title string, child layout.Widget) D {
	if ns.anim == nil {
		ns.anim = &cmp.VisibilityAnimation{
			State:    cmp.Invisible,
			Duration: time.Millisecond * 150,
		}
	}

	for {
		e, ok := ns.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			if ns.expanded {
				ns.anim.Disappear(gtx.Now)
			} else {
				ns.anim.Appear(gtx.Now)
			}
			ns.expanded = !ns.expanded
			if ns.expanded && ns.onClicked != nil {
				ns.onClicked()
			}

		}
	}

	if !ns.collapseOnClick {
		return ns.layout(gtx, th, icon, title, child)
	}

	macro := op.Record(gtx.Ops)
	dims := ns.layout(gtx, th, icon, title, child)
	mainOp := macro.Stop()

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	ns.click.Add(gtx.Ops)
	mainOp.Add(gtx.Ops)
	return dims
}

func (ns *NavSection) layout(gtx C, th *theme.Theme, icon *widget.Icon, title string, child layout.Widget) D {

	return layout.Flex{
		Axis:      layout.Vertical,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Background{}.Layout(gtx,
				func(gtx C) D {
					defer clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Min.Y}}.Push(gtx.Ops).Pop()
					// paint.LinearGradientOp{
					// 	Stop1: f32.Pt(0, 0), Color1: misc.WithAlpha(th.ContrastBg, th.HoverAlpha),
					// 	Stop2: f32.Pt(float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)), Color2: misc.HexColor(0x000851),
					// }.Add(gtx.Ops)

					//paint.PaintOp{}.Add(gtx.Ops)
					paint.Fill(gtx.Ops, th.Bg2)
					if !ns.collapseOnClick {
						ns.click.Add(gtx.Ops)
					}

					return D{Size: gtx.Constraints.Min}
				},
				func(gtx C) D {
					return widget.Border{Width: unit.Dp(0.5), Color: misc.WithAlpha(th.Fg, 0x30)}.Layout(gtx, func(gtx C) D {
						return layout.Inset{
							Left:   unit.Dp(8),
							Right:  unit.Dp(8),
							Top:    unit.Dp(6),
							Bottom: unit.Dp(7),
						}.Layout(gtx, func(gtx C) D {
							macro := op.Record(gtx.Ops)
							dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return misc.Icon{Icon: icon, Size: unit.Dp(th.TextSize) * 1.2}.Layout(gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
								layout.Flexed(1, func(gtx C) D {
									label := material.Subtitle2(th.Theme, title)
									return label.Layout(gtx)
								}),

								layout.Rigid(func(gtx C) D {
									arrow := arrowRightIcon
									if ns.expanded {
										arrow = arrowDownIcon
									}

									return misc.Icon{Icon: arrow, Color: th.Fg, Size: unit.Dp(th.TextSize) * 1.2}.Layout(gtx, th)

								}),
							)
							callOp := macro.Stop()
							defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
							callOp.Add(gtx.Ops)
							if ns.menu != nil {
								ns.menu.Layout(gtx, th)
							}

							return dims

						})
					})

				},
			)
		}),
		layout.Flexed(1, func(gtx C) D {
			if !ns.anim.Visible() {
				return D{}
			}

			dims := D{}
			callOp := func() op.CallOp {
				macro := op.Record(gtx.Ops)
				dims = layout.Inset{
					Top:    unit.Dp(2),
					Bottom: unit.Dp(2),
				}.Layout(gtx, child)
				return macro.Stop()
			}()

			if ns.anim.Animating() {
				dims.Size.Y = int(float32(dims.Size.Y) * ns.anim.Revealed(gtx))
			}

			defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
			callOp.Add(gtx.Ops)

			return dims
		}),
	)
}

func (ns NaviDrawerStyle) Layout(gtx C, th *theme.Theme) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	rect := clip.Rect{
		Max: gtx.Constraints.Max,
	}
	paint.FillShape(gtx.Ops, ns.Bg, rect.Op())

	return ns.NavDrawer.Layout(gtx, th)

}
