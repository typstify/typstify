package view

import (
	"crypto/md5"
	"encoding/hex"
	"image/color"
	"log"
	"math"
	"strings"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/unit"
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
	err := mb.parse(th, content)
	if err != nil {
		return D{}
	}

	if len(mb.spans) == 0 {
		return D{}
	}

	return richtext.Text(&mb.label, th.Shaper, mb.spans...).Layout(gtx)
}

func contentDigest(content []byte) string {
	hasher := md5.New()
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}
