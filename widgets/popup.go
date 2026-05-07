package widgets

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/x/component"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/widgets/menu"
)

type PopupWidget interface {
	OnClicked()
	Layout(gtx C, th *theme.Theme) D
}

type Popup struct {
	popupArea   menu.ContextArea
	pendingOpen bool
	list        layout.List
	labels      []*InteractiveLabel

	MaxHeight unit.Dp
	Width     unit.Dp
	Direction layout.Direction // only S and N are supported
}

func (p *Popup) SetOpen() {
	p.pendingOpen = true
}

func (p *Popup) Dismiss() {
	p.popupArea.Dismiss()
}

func (p *Popup) Layout(gtx C, th *theme.Theme, w layout.Widget, popItems ...PopupWidget) D {
	for {
		evt, ok := gtx.Event(
			key.FocusFilter{Target: p},
			key.Filter{Focus: p, Name: key.NameEscape},
		)
		if !ok {
			break
		}

		switch e := evt.(type) {
		case key.Event:
			if e.Name == key.NameEscape {
				p.Dismiss()
			}
		}
	}

	macro := op.Record(gtx.Ops)
	dims := p.layout(gtx, th, w, popItems...)
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, p)
	callOp.Add(gtx.Ops)

	return dims
}

func (p *Popup) layout(gtx C, th *theme.Theme, w layout.Widget, popItems ...PopupWidget) D {

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return w(gtx)
		}),

		layout.Expanded(func(gtx C) D {
			defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Push(gtx.Ops).Pop()

			//originalMin := gtx.Constraints.Min

			gtx.Constraints.Min = image.Point{}

			originalOps := gtx.Ops
			gtx.Ops = &op.Ops{}
			dims := p.layoutPopup(gtx, th, popItems...)
			gtx.Ops = originalOps

			var offset image.Point

			switch p.Direction {
			case layout.N:
				offset = image.Pt(0, -dims.Size.Y)
			case layout.S:
				offset = image.Pt(0, dims.Size.Y)
			default:
				offset = image.Pt(0, dims.Size.Y)
			}

			macro := op.Record(gtx.Ops)
			op.Offset(offset).Add(gtx.Ops)

			if p.pendingOpen {
				switch p.Direction {
				case layout.N:
					p.popupArea.PositionHint = layout.N
					p.popupArea.Show(f32.Pt(0, -float32(offset.Y)))
				default:
					p.popupArea.PositionHint = layout.S
					p.popupArea.Show(f32.Pt(0, 0))
				}

				p.pendingOpen = false
				gtx.Execute(key.FocusCmd{Tag: p})
			}

			p.popupArea.Layout(gtx, func(gtx C) D {
				return p.layoutPopup(gtx, th, popItems...)
			})

			call := macro.Stop()
			op.Defer(gtx.Ops, call)

			return D{}

		}),
	)

}

func (p *Popup) layoutPopup(gtx C, th *theme.Theme, items ...PopupWidget) D {
	if p.MaxHeight > 0 {
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(p.MaxHeight))
	}

	if p.Width > 0 {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(p.Width))
	}

	surface := component.Surface(th.Theme)
	surface.Fill = misc.WithAlpha(th.Bg, 0xff)
	surface.CornerRadius = unit.Dp(4)

	p.list.Axis = layout.Vertical
	return surface.Layout(gtx, func(gtx C) D {
		return p.list.Layout(gtx, len(items), func(gtx C, index int) D {
			if len(items) == 1 {
				return items[index].Layout(gtx, th)
			}

			if len(p.labels) <= index {
				p.labels = append(p.labels, &InteractiveLabel{})
			}
			return p.layoutItem(gtx, th, p.labels[index], items[index])
		})
	})
}

func (p *Popup) layoutItem(gtx C, th *theme.Theme, state *InteractiveLabel, item PopupWidget) D {
	if state.Update(gtx) {
		p.popupArea.Dismiss()
		item.OnClicked()
	} else if state.IsSelected() {
		state.Unselect()
	}

	gtx.Constraints.Min.X = gtx.Constraints.Max.X

	return state.Layout(gtx, th, func(gtx C, textColor color.NRGBA) D {
		return item.Layout(gtx, th)
	})
}
