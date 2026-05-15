package view

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/addons/completion"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"

	"looz.ws/typstify/agent"
)

type InputBox struct {
	*gvcode.Editor
	colorScheme     *syntax.ColorScheme
	completionPopup *completion.CompletionPopup
	submit          bool
}

func newInputBox() *InputBox {
	ed := &gvcode.Editor{}

	ed.WithOptions(
		gvcode.WrapLine(true),
		gvcode.WithSoftTab(true),
		gvcode.WithCornerRadius(unit.Dp(4)),
		gvcode.WithTabWidth(2),
	)

	cm := &completion.DefaultCompletion{Editor: ed}
	cmdCompletor := &commandCompletor{}
	rsCompletor := &resourceCompletor{}

	popup := completion.NewCompletionPopup(ed, cm)
	cm.AddCompletor(cmdCompletor, popup)
	cm.AddCompletor(rsCompletor, popup)
	// ed.WithOptions(gvcode.WithAutoCompletion(cm))

	b := &InputBox{
		Editor:          ed,
		completionPopup: popup,
	}

	ed.RegisterCommand("input-box",
		key.Filter{Name: key.NameEnter, Required: key.ModShift},
		func(gtx layout.Context, evt key.Event) gvcode.EditorEvent {
			b.submit = true
			return nil
		})

	ed.RegisterCommand("input-box",
		key.Filter{Name: key.NameReturn, Required: key.ModShift},
		func(gtx layout.Context, evt key.Event) gvcode.EditorEvent {
			b.submit = true
			return nil
		})

	return b
}

func (b *InputBox) Update(gtx C) bool {
	for {
		_, ok := b.Editor.Update(gtx)
		if !ok {
			break
		}
	}

	submitted := b.submit
	b.submit = false
	return submitted
}

func (b *InputBox) Layout(gtx C, th *theme.Theme) D {
	cs := syntax.ColorScheme{}
	cs.Background = gvcolor.MakeColor(th.Bg)
	cs.Foreground = gvcolor.MakeColor(th.Fg)
	cs.SelectColor = gvcolor.MakeColor(th.ContrastBg).MulAlpha(th.SelectedAlpha)
	b.Editor.WithOptions(
		gvcode.WithFont(font.Font{Typeface: th.Face}),
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithTextAlignment(text.Start),
		gvcode.WithLineHeight(0, 1.5),
		gvcode.WithColorScheme(cs),
	)

	gtx.Constraints.Max.Y = min(gtx.Dp(unit.Dp(100)), gtx.Constraints.Max.Y)

	return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
		b.completionPopup.Size = image.Point{
			X: min(gtx.Dp(unit.Dp(350)), gtx.Constraints.Max.X),
			Y: min(gtx.Dp(unit.Dp(250)), gtx.Constraints.Max.Y),
		}
		return b.Editor.Layout(gtx, th.Shaper)
	})

}

var _ gvcode.Completor = (*commandCompletor)(nil)
var _ gvcode.Completor = (*resourceCompletor)(nil)

type commandCompletor struct {
	session *agent.ACPSession
}

// FilterAndRank implements [gvcode.Completor].
func (c *commandCompletor) FilterAndRank(pattern string, candidates []gvcode.CompletionCandidate) []gvcode.CompletionCandidate {
	return []gvcode.CompletionCandidate{}
}

// Suggest implements [gvcode.Completor].
func (c *commandCompletor) Suggest(ctx gvcode.CompletionContext) []gvcode.CompletionCandidate {
	return []gvcode.CompletionCandidate{}

}

// Trigger implements [gvcode.Completor].
func (c *commandCompletor) Trigger() gvcode.Trigger {
	return gvcode.Trigger{
		Characters: []string{"/"},
	}
}

// suggest project resources, such as files and folders.
type resourceCompletor struct {
}

// FilterAndRank implements [gvcode.Completor].
func (r *resourceCompletor) FilterAndRank(pattern string, candidates []gvcode.CompletionCandidate) []gvcode.CompletionCandidate {
	return []gvcode.CompletionCandidate{}

}

// Suggest implements [gvcode.Completor].
func (r *resourceCompletor) Suggest(ctx gvcode.CompletionContext) []gvcode.CompletionCandidate {
	return []gvcode.CompletionCandidate{}

}

// Trigger implements [gvcode.Completor].
func (r *resourceCompletor) Trigger() gvcode.Trigger {
	return gvcode.Trigger{
		Characters: []string{"@"},
	}
}
