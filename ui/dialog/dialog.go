package dialog

import (
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
)

const errorShowDuration = time.Second * 2

type (
	C = layout.Context
	D = layout.Dimensions
)

type Dialog interface {
	OnInit(intent view.Intent) error
	OnConfirm() error
	LayoutBody(gtx layout.Context, th *theme.Theme) layout.Dimensions
}

type DialogResult[T any] struct {
	Params T
	Err    error
}

type DialogChooser[T any] struct {
	vm         view.ViewManager
	resultChan chan DialogResult[T]
}

type DialogModal struct {
	*view.BaseView
	confirmBtn widget.Clickable
	cancelBtn  widget.Clickable
	err        error
	errShowAt  time.Time
	id         view.ViewID
	list       widget.List

	//OnInit       func(intent view.Intent) error
	// OnConfirm    func() error
	name         string
	confirmLabel string
	Dialog
	//Body         view.Widget
}

type formItem struct {
	Axis      layout.Axis
	Alignment layout.Alignment
}

func NewDialogModal(viewID view.ViewID, name string, confirmLabel string) *DialogModal {
	return &DialogModal{
		BaseView:     &view.BaseView{},
		id:           viewID,
		name:         name,
		confirmLabel: confirmLabel,
	}
}

func (d *DialogModal) ID() view.ViewID { return d.id }

func (d *DialogModal) Title() string { return d.name }

func (d *DialogModal) OnNavTo(intent view.Intent) error {
	d.BaseView.OnNavTo(intent)
	return d.OnInit(intent)
}

func (d *DialogModal) OnFinish() {
	d.BaseView.OnFinish()
}

func (d *DialogModal) Layout(gtx C, th *theme.Theme) D {
	if d.confirmBtn.Clicked(gtx) {
		d.err = d.OnConfirm()
		if d.err == nil {
			d.OnFinish()
		}
	}

	if d.cancelBtn.Clicked(gtx) {
		d.OnFinish()
	}

	d.list.Axis = layout.Vertical

	return layout.Inset{
		Top: unit.Dp(16),
	}.Layout(gtx, func(gtx C) D {
		return material.List(th.Theme, &d.list).Layout(gtx, 1, func(gtx C, index int) D {

			return layout.Flex{
				Axis: layout.Vertical,
				Gap:  gtx.Dp(unit.Dp(32)),
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return d.LayoutBody(gtx, th)
				}),
				layout.Rigid(func(gtx C) D {
					return d.layoutBtnArea(gtx, th)
				}),
			)
		})
	})

}

func (d *DialogModal) layoutBtnArea(gtx C, th *theme.Theme) D {
	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
		Spacing:   layout.SpaceBetween,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return d.layoutError(gtx, th)
		}),

		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
				Spacing:   layout.SpaceStart,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					btn := material.Button(th.Theme, &d.cancelBtn, i18n.Translate("Cancel"))
					btn.Inset = layout.UniformInset(unit.Dp(6))
					btn.Background = th.Bg
					btn.Color = th.Fg
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

				layout.Rigid(func(gtx C) D {
					if d.confirmLabel == "" {
						d.confirmLabel = "OK"
					}
					btn := material.Button(th.Theme, &d.confirmBtn, d.confirmLabel)
					btn.Inset = layout.UniformInset(unit.Dp(6))
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

func (d *DialogModal) layoutError(gtx C, th *theme.Theme) D {
	if d.err == nil {
		return D{}
	}

	if d.errShowAt.IsZero() {
		d.errShowAt = gtx.Now
		gtx.Execute(op.InvalidateCmd{At: d.errShowAt.Add(errorShowDuration)})
	} else if d.errShowAt.Add(errorShowDuration).Before(gtx.Now) {
		defer func() {
			d.errShowAt = time.Time{}
			d.err = nil
		}()
	}

	return layout.Inset{Right: unit.Dp(36)}.Layout(gtx, func(gtx C) D {
		label := material.Label(th.Theme, th.TextSize, d.err.Error())
		label.Color = color.NRGBA{R: 255, A: 255}
		label.Alignment = text.Middle
		return label.Layout(gtx)
	})
}

func (item formItem) Layout(gtx C, th *theme.Theme, title, labelDesc string, w layout.Widget) D {
	return layout.Inset{
		Bottom: unit.Dp(24),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      item.Axis,
			Alignment: item.Alignment,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lb := material.Subtitle2(th.Theme, title)
				lb.Font.Weight = font.SemiBold
				return lb.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				if labelDesc == "" {
					return D{}
				}
				return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				if labelDesc == "" {
					return D{}
				}

				lb := material.Label(th.Theme, th.TextSize*0.9, labelDesc)
				// lb.Color = misc.WithAlpha(th.Fg, 0xb6)
				return lb.Layout(gtx)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				return layout.UniformInset(unit.Dp(3)).Layout(gtx, w)
			}),
		)
	})

}

func NewDialogChooser[T any](vm view.ViewManager) *DialogChooser[T] {
	return &DialogChooser[T]{
		vm:         vm,
		resultChan: make(chan DialogResult[T]),
	}
}

func (d *DialogChooser[T]) show(id view.ViewID, params map[string]interface{}) {
	intentParams := map[string]interface{}{"resultChan": d.resultChan}
	for k, v := range params {
		intentParams[k] = v
	}

	d.vm.RequestSwitch(view.Intent{
		Target:      id,
		ShowAsModal: true,
		Params:      intentParams,
	})
}

// It's a blocking call, you should call it on a separated goroutine.
func (fc *DialogChooser[T]) Call(id view.ViewID, params map[string]interface{}) (DialogResult[T], error) {
	fc.show(id, params)

	resp := <-fc.resultChan
	return resp, nil
}
