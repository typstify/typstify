package view

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/console"
)

type TerminalStyle struct {
	state       *console.ConsoleState
	colorScheme syntax.ColorScheme
	yScroll     widget.Scrollbar
	totalLen    int
}

func (t *TerminalStyle) update(gtx C, th *theme.Theme, source *agent.ACPTerminal) {
	if t.state == nil {
		t.state = console.NewConsoleState(1000)
	}

	if t.colorScheme.Foreground.NRGBA() != th.Fg || t.colorScheme.Background.NRGBA() != th.Bg {
		t.colorScheme.Foreground = color.MakeColor(th.Fg)
		t.colorScheme.Background = color.MakeColor(th.Bg)
		t.colorScheme.SelectColor = color.MakeColor(th.ContrastBg).MulAlpha(0x60)
		t.colorScheme.LineColor = color.MakeColor(th.ContrastBg).MulAlpha(0x30)
		t.colorScheme.LineNumberColor = t.colorScheme.Foreground.MulAlpha(0xb6)
		t.state.State.WithOptions(gvcode.WithColorScheme(t.colorScheme))
	}

	t.state.State.WithOptions(
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithFont(font.Font{Typeface: th.Face, Weight: font.Medium}),
		gvcode.WithDefaultGutters(),
		gvcode.WithGutterGap(unit.Dp(8)),
	)

	outputSize := source.OutputSize()
	if t.totalLen != outputSize {
		t.state.Clear()
		output, _ := source.Output()
		t.state.Write([]byte(output))
		t.state.Update()
		t.totalLen = outputSize
	}

	yScrollDist := t.yScroll.ScrollDistance()
	if yScrollDist != 0.0 {
		t.state.State.Scroll(gtx, 0, yScrollDist)
	}
}

func (t *TerminalStyle) Layout(gtx C, th *theme.Theme, source *agent.ACPTerminal) D {
	t.update(gtx, th, source)

	if t.state.State.Len() <= 0 {
		lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("No messages."))
		lb.Font.Typeface = th.Face

		return lb.Layout(gtx)
	}

	return layout.Flex{
		Axis: layout.Horizontal,
	}.Layout(gtx,
		layout.Flexed(1.0, func(gtx layout.Context) layout.Dimensions {
			return t.state.State.Layout(gtx, th.Shaper)
		}),

		layout.Rigid(func(gtx C) D {
			_, _, minY, maxY := t.state.State.ScrollRatio()
			scrollIndicatorColor := misc.WithAlpha(th.Fg, 0x30)

			bar := utils.MakeScrollbar(th.Theme, &t.yScroll, scrollIndicatorColor)
			return bar.Layout(gtx, layout.Vertical, minY, maxY)
		}),
	)

}
