package console

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	closeIcon = icons.NewSvgIcon(icons.X)
	clearIcon = icons.NewSvgIcon(icons.BrushCleaning)
)

type Console struct {
	state           *ConsoleState
	colorScheme     syntax.ColorScheme
	closeConsoleBtn widget.Clickable
	clearConsoleBtn widget.Clickable
	yScroll         widget.Scrollbar

	ShowConsole bool
}

func NewConsolePanel(cs *ConsoleState) *Console {
	c := &Console{
		state: cs,
	}

	return c
}

func (c *Console) update(gtx C, th *theme.Theme) {
	if c.closeConsoleBtn.Clicked(gtx) {
		c.ShowConsole = false
	}

	if !c.ShowConsole {
		return
	}

	if c.clearConsoleBtn.Clicked(gtx) {
		c.state.Clear()
	}

	if c.colorScheme.Foreground.NRGBA() != th.Fg || c.colorScheme.Background.NRGBA() != th.Bg {
		c.colorScheme.Foreground = color.MakeColor(th.Fg)
		c.colorScheme.Background = color.MakeColor(th.Bg) // overwrite with global palette color.
		c.colorScheme.SelectColor = color.MakeColor(th.ContrastBg).MulAlpha(0x60)
		c.colorScheme.LineColor = color.MakeColor(th.ContrastBg).MulAlpha(0x30)
		c.colorScheme.LineNumberColor = c.colorScheme.Foreground.MulAlpha(0xb6)
		c.state.State.WithOptions(gvcode.WithColorScheme(c.colorScheme))
	}

	c.state.State.WithOptions(
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithFont(font.Font{Typeface: th.Face, Weight: font.Medium}),
		gvcode.WithDefaultGutters(),
		gvcode.WithGutterGap(unit.Dp(8)),
	)

	c.state.Update()

	yScrollDist := c.yScroll.ScrollDistance()
	if yScrollDist != 0.0 {
		c.state.State.Scroll(gtx, 0, yScrollDist)
	}

}

func (c *Console) Layout(gtx C, th *theme.Theme) D {
	c.update(gtx, th)

	if !c.ShowConsole {
		return D{}
	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return c.layoutBorder(gtx, th)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{
				Top:   unit.Dp(4),
				Left:  unit.Dp(24),
				Right: unit.Dp(0),
			}.Layout(gtx, func(gtx C) D {
				return c.layoutBody(gtx, th)
			})
		}),
	)
}

func (c *Console) layoutBody(gtx C, th *theme.Theme) D {
	if c.state.State.Len() <= 0 {
		lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("No messages."))
		lb.Font.Typeface = th.Face

		return lb.Layout(gtx)
	}

	return layout.Flex{
		Axis: layout.Horizontal,
	}.Layout(gtx,
		layout.Flexed(1.0, func(gtx layout.Context) layout.Dimensions {
			return c.state.State.Layout(gtx, th.Shaper)
		}),

		layout.Rigid(func(gtx C) D {
			_, _, minY, maxY := c.state.State.ScrollRatio()
			scrollIndicatorColor := misc.WithAlpha(th.Fg, 0x30)

			bar := utils.MakeScrollbar(th.Theme, &c.yScroll, scrollIndicatorColor)
			return bar.Layout(gtx, layout.Vertical, minY, maxY)
		}),
	)

}

func (c *Console) layoutBorder(gtx C, th *theme.Theme) D {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				cap := material.Caption(th.Theme, "Output")
				cap.Color = th.Fg
				return cap.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &c.clearConsoleBtn, func(gtx C) D {
					return clearIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &c.closeConsoleBtn, func(gtx C) D {
					return closeIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			}),
		)
	})
	callOp := macro.Stop()

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, misc.WithAlpha(th.Bg2, 0xb6))
	callOp.Add(gtx.Ops)

	return dims
}
