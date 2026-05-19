package view

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/widgets/icons"
)

var (
	chevronDownIcon  = icons.NewSvgIcon(icons.ChevronDown)
	chevronRightIcon = icons.NewSvgIcon(icons.ChevronRight)
)

type CardStyle struct {
	click    gesture.Click
	Icon     *icons.SvgIcon
	Expanded bool
}

func (t *CardStyle) Layout(gtx C, th *theme.Theme, header, body layout.Widget) D {
	for {
		e, ok := t.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			t.Expanded = !t.Expanded
		}
	}
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		macro := op.Record(gtx.Ops)
		dims := layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return t.layoutHeader(gtx, th, header)
			}),
			layout.Rigid(func(gtx C) D {
				if !t.Expanded {
					return D{}
				}
				return layout.Inset{
					Top: unit.Dp(8), Bottom: unit.Dp(8),
					Left: unit.Dp(12), Right: unit.Dp(12),
				}.Layout(gtx, body)
			}),
		)

		call := macro.Stop()

		// Draw card background and border.
		rr := gtx.Dp(unit.Dp(6))

		borderRect := image.Rectangle{Max: dims.Size}
		stack := clip.UniformRRect(borderRect, rr).Push(gtx.Ops)
		paint.ColorOp{Color: th.Bg}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		call.Add(gtx.Ops)
		stack.Pop()

		// Border stroke.
		paint.FillShape(gtx.Ops, misc.WithAlpha(th.Fg, 0x12),
			clip.Stroke{
				Path:  clip.UniformRRect(borderRect, rr).Path(gtx.Ops),
				Width: float32(gtx.Dp(unit.Dp(1))),
			}.Op(),
		)

		return dims
	})
}

func (c *CardStyle) layoutHeader(gtx C, th *theme.Theme, header layout.Widget) D {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{
		Top: unit.Dp(6), Bottom: unit.Dp(6),
		Left: unit.Dp(8), Right: unit.Dp(8),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						if c.Icon == nil {
							return D{}
						}
						return c.Icon.Layout(gtx, th.Fg, th.TextSize)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(header),
				)
			}),

			layout.Rigid(func(gtx C) D {
				if !c.Expanded {
					return chevronRightIcon.Layout(gtx, misc.WithAlpha(th.Fg, 0xb0), th.TextSize)
				}

				return chevronDownIcon.Layout(gtx, misc.WithAlpha(th.Fg, 0xb0), th.TextSize)
			}),
		)
	})
	call := macro.Stop()

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: th.Bg2}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	c.click.Add(gtx.Ops)

	call.Add(gtx.Ops)
	return dims
}
