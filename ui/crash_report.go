package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/logger"
	"looz.ws/typstify/widgets/icons"
)

var (
	errIcon = icons.NewSvgIcon(icons.CircleAlert)
)

type CrashReport struct {
	crashView  *gvcode.Editor
	cancelBtn  widget.Clickable
	restartBtn widget.Clickable
}

func (cr *CrashReport) logCrashReport(r interface{}, stack []byte) {
	recoverFromPanics(r, stack)

	if cr.crashView == nil {
		cr.crashView = &gvcode.Editor{}
		cr.crashView.WithOptions(gvcode.ReadOnlyMode(true))
	}

	cr.crashView.SetText(fmt.Sprintf("panics: %v, stack trace: \n%s", r, string(stack)))
}

func (cr *CrashReport) Layout(gtx C, th *theme.Theme) D {
	if cr.crashView == nil {
		return D{}
	}

	if cr.cancelBtn.Clicked(gtx) {
		os.Exit(1)
	}
	if cr.restartBtn.Clicked(gtx) {
		restartApp()
	}

	// draw the background
	gtx.Constraints.Min = gtx.Constraints.Max
	rect := clip.Rect{Max: gtx.Constraints.Max}
	paint.FillShape(gtx.Ops, th.Bg, rect.Op())

	return layout.UniformInset(48).Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return errIcon.Layout(gtx, th.Fg, th.TextSize*2)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D {
						header := material.H5(th.Theme, "Typstify Crashed!")
						header.Alignment = text.Middle
						return header.Layout(gtx)
					}),
				)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
					Spacing:   layout.SpaceStart,
				}.Layout(gtx,

					layout.Rigid(func(gtx C) D {
						btn := material.Button(th.Theme, &cr.cancelBtn, i18n.Translate("Exit"))
						btn.Inset = layout.UniformInset(unit.Dp(6))
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

					layout.Rigid(func(gtx C) D {
						btn := material.Button(th.Theme, &cr.restartBtn, "Restart")
						btn.Inset = layout.UniformInset(unit.Dp(6))
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx C) D {
				return widget.Border{Width: unit.Dp(1), Color: th.ContrastBg, CornerRadius: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
						gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(400))
						configureEditor(cr.crashView, th.Theme)
						return cr.crashView.Layout(gtx, th.Shaper)
					})
				})
			}),
		)
	})

}

func configureEditor(editor *gvcode.Editor, th *material.Theme) {
	colorScheme := syntax.ColorScheme{}
	colorScheme.Foreground = gvcolor.MakeColor(th.Fg)
	colorScheme.Background = gvcolor.MakeColor(th.Bg)
	colorScheme.SelectColor = gvcolor.MakeColor(th.ContrastBg).MulAlpha(0x60)
	colorScheme.LineColor = gvcolor.MakeColor(th.ContrastBg).MulAlpha(0x30)
	colorScheme.LineNumberColor = gvcolor.MakeColor(th.Fg).MulAlpha(0xb6)

	editor.WithOptions(
		gvcode.WrapLine(false),
		gvcode.WithFont(font.Font{Typeface: th.Face}),
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithTextAlignment(text.Start),
		gvcode.WithLineHeight(0, 1.2),
		gvcode.WithTabWidth(4),
		gvcode.WithDefaultGutters(),
		gvcode.WithGutterGap(unit.Dp(24)),
		gvcode.WithColorScheme(colorScheme),
	)

}

func recoverFromPanics(r interface{}, stack []byte) {
	path, err := os.UserConfigDir()
	if err != nil {
		path, _ = os.UserHomeDir()
	}

	log := logger.NewFileLogger(filepath.Join(path, "/typstify/crash.log"))
	log.Printf("typstify panics: %v, stack trace: \n%s", r, string(stack))
	log.Close()
}

func restartApp() {
	fmt.Println("Restarting process...")

	// Construct command to restart the program
	cmd := exec.Command(os.Args[0], os.Args[1:]...)

	// Start the new process
	err := cmd.Start()
	if err != nil {
		fmt.Println("Error starting process:", err)
		os.Exit(1)
	}

	// Detach from the new process
	if err := cmd.Process.Release(); err != nil {
		fmt.Println("Error releasing process:", err)
		os.Exit(1)
	}

	// Exit the current process
	os.Exit(0)
}
