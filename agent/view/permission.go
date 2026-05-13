package view

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
)

type PermissionGrantPopup struct {
	Perm *permissionState
}

func (p PermissionGrantPopup) Layout(gtx C, th *theme.Theme) D {
	toolTitle := ""
	if p.Perm.request.Req.ToolCall.Title != nil {
		toolTitle = *p.Perm.request.Req.ToolCall.Title
	}
	if toolTitle == "" {
		toolTitle = string(p.Perm.request.Req.ToolCall.ToolCallId)
	}

	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(4), Right: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					prompt := material.Label(th.Theme, th.TextSize, "Allow "+toolTitle+"?")
					prompt.Color = th.Fg
					prompt.Font.Weight = font.Bold
					return prompt.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx C) D {
					return p.layoutPermissionOptions(gtx, th, p.Perm)
				}),
			)
		})
		callOp := macro.Stop()

		defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(4))).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		callOp.Add(gtx.Ops)

		return dims
	})
}

func (p *PermissionGrantPopup) layoutPermissionOptions(gtx C, th *theme.Theme, perm *permissionState) D {
	options := perm.request.Req.Options
	if len(options) == 0 {
		return D{}
	}

	children := make([]layout.FlexChild, 0, len(options))
	for i := range options {
		idx := i // capture
		opt := options[idx]
		children = append(children, layout.Rigid(func(gtx C) D {
			btn := material.Button(th.Theme, &perm.buttons[idx], permissionLabel(opt.Kind, opt.Name))
			btn.Inset = layout.Inset{
				Top: unit.Dp(4), Bottom: unit.Dp(4),
				Left: unit.Dp(8), Right: unit.Dp(8),
			}
			bg, fg := permissionColors(th, opt.Kind)
			if perm.buttons[idx].Hovered() {
				bg = misc.WithAlpha(bg, 0xcc)
			}
			btn.Background = bg
			btn.Color = fg

			return btn.Layout(gtx)

		}))
		if i < len(options)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
		}
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx, children...)
}

// --- Helpers ---

func permissionLabel(kind acp.PermissionOptionKind, name string) string {
	if name != "" {
		return name
	}
	switch kind {
	case acp.PermissionOptionKindAllowOnce:
		return "Allow Once"
	case acp.PermissionOptionKindAllowAlways:
		return "Allow Always"
	case acp.PermissionOptionKindRejectOnce:
		return "Reject Once"
	case acp.PermissionOptionKindRejectAlways:
		return "Reject Always"
	default:
		return string(kind)
	}
}

func permissionColors(th *theme.Theme, kind acp.PermissionOptionKind) (bg, fg color.NRGBA) {
	switch kind {
	case acp.PermissionOptionKindAllowOnce, acp.PermissionOptionKindAllowAlways:
		return misc.WithAlpha(color.NRGBA{R: 0, G: 200, B: 100, A: 255}, 0x18),
			color.NRGBA{R: 0, G: 180, B: 80, A: 255}
	case acp.PermissionOptionKindRejectOnce, acp.PermissionOptionKindRejectAlways:
		return misc.WithAlpha(color.NRGBA{R: 220, G: 60, B: 60, A: 255}, 0x18),
			color.NRGBA{R: 200, G: 50, B: 50, A: 255}
	default:
		return th.Bg2, th.Fg
	}
}
