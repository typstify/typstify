package console

import (
	"bytes"
	"io"
	"regexp"
	"sync"
	"sync/atomic"

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
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/utils"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	closeIcon, _ = widget.NewIcon(icons.NavigationClose)
	clearIcon, _ = widget.NewIcon(icons.CommunicationClearAll)
)

var _ io.Writer = (*ConsoleState)(nil)

type ConsoleState struct {
	buf         bytes.Buffer
	state       *gvcode.Editor
	colorScheme syntax.ColorScheme
	yScroll     widget.Scrollbar
	maxLines    int
	err         error
	textUpated  atomic.Bool
	mu          sync.Mutex

	ShowConsole bool
}

// Create a console.
func NewConsoleState(maxLines int) *ConsoleState {
	state := &gvcode.Editor{}
	state.WithOptions(
		gvcode.WithLineHeight(0, 1.5),
		gvcode.ReadOnlyMode(true),
		gvcode.WrapLine(true),
	)
	c := &ConsoleState{
		maxLines: maxLines,
		state:    state,
	}

	return c
}

func (c *ConsoleState) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	n, err := c.buf.Write(data)
	c.textUpated.Store(true)

	return n, err
}

func (c *ConsoleState) HasMore() bool {
	return c.textUpated.Load()
}

func (c *ConsoleState) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.SetText("")
}

func (c *ConsoleState) readBuffered() {
	c.mu.Lock()
	msg := c.buf.String()
	c.state.SetCaret(c.state.Len(), c.state.Len())
	c.state.Insert(msg)
	c.buf.Reset()
	c.mu.Unlock()
}

func (c *ConsoleState) Layout(gtx C, th *theme.Theme) D {
	c.update(gtx, th)

	if c.err != nil {
		lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("Console has error:", c.err.Error()))
		lb.Font.Typeface = th.Face
		lb.Font.Weight = font.SemiBold

		return lb.Layout(gtx)
	}

	if c.state.Len() <= 0 {
		lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("No messages."))
		lb.Font.Typeface = th.Face

		return lb.Layout(gtx)
	}

	return layout.Flex{
		Axis: layout.Horizontal,
	}.Layout(gtx,
		layout.Flexed(1.0, func(gtx layout.Context) layout.Dimensions {
			return c.state.Layout(gtx, th.Shaper)
		}),

		layout.Rigid(func(gtx C) D {
			_, _, minY, maxY := c.state.ScrollRatio()
			scrollIndicatorColor := misc.WithAlpha(th.Fg, 0x30)

			bar := utils.MakeScrollbar(th.Theme, &c.yScroll, scrollIndicatorColor)
			return bar.Layout(gtx, layout.Vertical, minY, maxY)
		}),
	)

}

func (c *ConsoleState) update(gtx C, th *theme.Theme) {
	if c.colorScheme.Foreground.NRGBA() != th.Fg || c.colorScheme.Background.NRGBA() != th.Bg {
		c.colorScheme.Foreground = color.MakeColor(th.Fg)
		c.colorScheme.Background = color.MakeColor(th.Bg) // overwrite with global palette color.
		c.colorScheme.SelectColor = color.MakeColor(th.ContrastBg).MulAlpha(0x60)
		c.colorScheme.LineColor = color.MakeColor(th.ContrastBg).MulAlpha(0x30)
		c.colorScheme.LineNumberColor = c.colorScheme.Foreground.MulAlpha(0xb6)
		c.state.WithOptions(gvcode.WithColorScheme(c.colorScheme))
	}

	c.state.WithOptions(
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithFont(font.Font{Typeface: th.Face, Weight: font.Medium}),
		gvcode.WithDefaultGutters(),
		gvcode.WithGutterGap(unit.Dp(8)),
	)

	if c.textUpated.CompareAndSwap(true, false) {
		c.readBuffered()
		c.truncate()
	}

	yScrollDist := c.yScroll.ScrollDistance()
	if yScrollDist != 0.0 {
		c.state.Scroll(gtx, 0, yScrollDist)
	}

}

func (c *ConsoleState) truncate() {
	if c.state.Lines() <= c.maxLines {
		return
	}

	overflows := c.state.Lines() - c.maxLines
	c.state.SetCaret(0, 0)
	c.state.SelectLines(overflows, false)
	c.state.Delete(1)

	textLen := c.state.Len()
	c.state.SetCaret(textLen, textLen)
}

// ANSI escape sequence regexp
const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

func Strip(str string) string {
	return re.ReplaceAllString(str, "")
}

type Console struct {
	state           *ConsoleState
	closeConsoleBtn widget.Clickable
	clearConsoleBtn widget.Clickable
}

func NewConsolePanel(cs *ConsoleState) *Console {
	c := &Console{
		state: cs,
	}

	return c
}

func (c *Console) Layout(gtx C, th *theme.Theme) D {
	if c.closeConsoleBtn.Clicked(gtx) {
		c.state.ShowConsole = false
	}
	if c.clearConsoleBtn.Clicked(gtx) {
		c.state.Clear()
	}

	if !c.state.ShowConsole {
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
				return c.state.Layout(gtx, th)
			})
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
					return misc.Icon{Icon: clearIcon, Color: th.Fg}.Layout(gtx, th)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &c.closeConsoleBtn, func(gtx C) D {
					return misc.Icon{Icon: closeIcon, Color: th.Fg}.Layout(gtx, th)
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
