package view

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets/icons"
)

var (
	thoughtIcon = icons.NewSvgIcon(icons.LightBulb)
	planIcon    = icons.NewSvgIcon(icons.SquareCheck)
)

type UserMsgStyle struct {
	msg       *chatMessage
	selection widget.Selectable
}

func (u *UserMsgStyle) Layout(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		// Measure the label first so we know the background size.
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: unit.Dp(6), Bottom: unit.Dp(6),
			Left: unit.Dp(8), Right: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, u.msg.Content)
			label.Color = th.Fg
			label.State = &u.selection
			return label.Layout(gtx)
		})
		call := macro.Stop()

		// Draw background behind the label.
		rect := clip.RRect{
			Rect: image.Rectangle{Max: dims.Size},
			SE:   0, SW: gtx.Dp(unit.Dp(6)),
			NE: 0, NW: gtx.Dp(unit.Dp(6)),
		}
		defer rect.Push(gtx.Ops).Pop()
		paint.FillShape(gtx.Ops, misc.WithAlpha(th.ContrastBg, 0x20), rect.Op(gtx.Ops))

		call.Add(gtx.Ops)
		return dims
	})
}

type AgentMsgStyle struct {
	msg       *chatMessage
	mdBock    markdownBlock
	selection widget.Selectable
}

func (a *AgentMsgStyle) Layout(gtx C, th *theme.Theme) D {
	if a.msg.Content == "" {
		return D{}
	}
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return a.mdBock.Layout(gtx, th, []byte(a.msg.Content))
	})
}

type ThoughtMsgStyle struct {
	msg       *chatMessage
	mdBock    markdownBlock
	selection widget.Selectable
	Collapsed bool
}

func (t ThoughtMsgStyle) Layout(gtx C, th *theme.Theme) D {
	if t.msg.Kind != msgThought || t.msg.Content == "" {
		return D{}
	}

	return CardStyle{Icon: thoughtIcon}.Layout(gtx, th,
		func(gtx C) D {
			return material.Label(th.Theme, th.TextSize, i18n.Translate("Thinking")).Layout(gtx)
		},
		func(gtx C) D {
			return t.mdBock.Layout(gtx, th, []byte(t.msg.Content))
		},
	)
}

type PlanMsgStyle struct {
	msg       *chatMessage
	selection widget.Selectable
}

func (p *PlanMsgStyle) Layout(gtx C, th *theme.Theme) D {
	return CardStyle{Icon: planIcon}.Layout(gtx, th,
		func(gtx C) D {
			return material.Label(th.Theme, th.TextSize, i18n.Translate("Plan")).Layout(gtx)
		},
		func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, p.msg.Content)
			label.Color = misc.WithAlpha(th.Fg, 0x80)
			return label.Layout(gtx)
		},
	)

}
