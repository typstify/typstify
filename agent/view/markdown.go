package view

import (
	"crypto/md5"
	"encoding/hex"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"strings"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/markdown"
	"gioui.org/x/richtext"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/theme"
)

type markdownBlock struct {
	spans   []richtext.SpanStyle
	label   richtext.InteractiveText
	config  markdown.Config
	digest  string
	refresh bool

	copyClick widget.Clickable
	hovering  bool

	Color    color.NRGBA
	TextSize unit.Sp
}

func (mb *markdownBlock) parse(th *theme.Theme, content []byte) error {
	digest := contentDigest(content)
	contentUpdated := digest != mb.digest

	var textColor color.NRGBA
	var textSize unit.Sp

	if mb.Color == (color.NRGBA{}) {
		textColor = th.Fg
	}

	if mb.TextSize == 0 {
		textSize = th.TextSize
	}

	config := markdown.Config{
		DefaultFont: font.Font{
			Typeface: th.Face,
		},
		// TODO: use editor fonts?
		MonospaceFont:    font.Font{Typeface: "monospace"},
		DefaultSize:      textSize,
		DefaultColor:     textColor,
		InteractiveColor: th.ContrastBg,
		H6Size:           textSize,
		H5Size:           unit.Sp(math.Round(1.2 * float64(textSize))),
		H4Size:           unit.Sp(math.Round(1.3 * float64(textSize))),
		H3Size:           unit.Sp(math.Round(1.4 * float64(textSize))),
		H2Size:           unit.Sp(math.Round(1.5 * float64(textSize))),
		H1Size:           unit.Sp(math.Round(1.6 * float64(textSize))),
	}

	if !contentUpdated && mb.config == config {
		return nil
	}

	mb.digest = digest
	mb.config = config
	mdRenderer := markdown.NewRenderer()
	mdRenderer.Config = mb.config

	spans, err := mdRenderer.Render(content)
	if err != nil {
		return err
	}
	mb.spans = mb.spans[:0]
	mb.spans = append(mb.spans, spans...)
	return nil
}

func (mb *markdownBlock) update(gtx C) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: mb,
			Kinds:  pointer.Enter | pointer.Leave | pointer.Cancel,
		})
		if !ok {
			break
		}

		if ev, ok := ev.(pointer.Event); ok {
			switch ev.Kind {
			case pointer.Enter:
				mb.hovering = true
			case pointer.Leave, pointer.Cancel:
				mb.hovering = false
			}
		}
	}

	for {
		span, event, ok := mb.label.Update(gtx)
		if !ok {
			break
		}
		url := span.Get(markdown.MetadataURL)
		switch event.Type {
		case richtext.Click:
			if event.ClickData.Kind == gesture.KindClick {
				link := url.(string)
				isHttpLink := strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://")
				if isHttpLink {
					if err := giohyperlink.Open(link); err != nil {
						log.Printf("error: opening hyperlink: %v, url: %s", err, link)
					}
				} else {
					// TODO: handle local files
				}

			}
		}
	}

}

func (mb *markdownBlock) Layout(gtx C, th *theme.Theme, content []byte) D {
	mb.update(gtx)
	if mb.copyClick.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(string(content)))})
	}
	err := mb.parse(th, content)
	if err != nil {
		return D{}
	}

	if len(mb.spans) == 0 {
		return D{}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return richtext.Text(&mb.label, th.Shaper, mb.spans...).Layout(gtx)
		}),
		layout.Expanded(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			dims := D{Size: gtx.Constraints.Min}

			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, mb)

			if !mb.hovering {
				return dims
			}

			layout.NE.Layout(gtx, func(gtx C) D {
				btn := material.Button(th.Theme, &mb.copyClick, "\u29C9")
				btn.Color = th.Fg
				btn.Background = th.Bg
				btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4)}
				return btn.Layout(gtx)
			})

			return dims
		}),
	)
}

func contentDigest(content []byte) string {
	hasher := md5.New()
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}
