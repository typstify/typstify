package editor

import (
	"image"
	"log"
	"math"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"gioui.org/x/markdown"
	"gioui.org/x/richtext"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/textstyle/syntax"
	"looz.ws/typstify/utils"
)

var (
	maxHoverTipY = unit.Dp(350)
)

type OnHoverAt func(gvcode.Position) (string, f32.Point)

type hoverResult struct {
	seq         uint64
	content     string
	pixelPos    f32.Point
	fallbackPos image.Point
}

type HoverTips struct {
	editor      *gvcode.Editor
	colorScheme *syntax.ColorScheme

	label      richtext.InteractiveText
	spanStyles []richtext.SpanStyle
	list       widget.List
	anim       *component.VisibilityAnimation

	lastHoverPos  image.Point
	lastContent   string
	needRebuild   bool
	cancelled     bool
	hoverSeq      uint64
	pendingResult atomic.Pointer[hoverResult]

	// Mouse tracking for tip area
	tipBounds      image.Rectangle
	tipHover       gesture.Hover
	hoverInTip     bool
	prevHoverInTip bool
}

func newHoverTips(editor *gvcode.Editor) *HoverTips {
	return &HoverTips{
		editor: editor,
		anim: &component.VisibilityAnimation{
			State:    component.Invisible,
			Duration: 150 * time.Millisecond,
		},
	}
}

func (h *HoverTips) Pos() image.Point {
	return h.lastHoverPos
}

func (h *HoverTips) OnHover(evt gvcode.HoverEvent, hoverCallbacks ...OnHoverAt) {
	if evt.IsCancel && h.anim.Visible() {
		h.cancelled = true
		return
	}

	// Increment sequence so stale goroutine results are discarded.
	h.hoverSeq++
	seq := h.hoverSeq
	fallbackPos := evt.PixelOff

	go func() {
		var content string
		var pixelPos f32.Point
		for _, cb := range hoverCallbacks {
			content, pixelPos = cb(evt.Pos)
			if content != "" {
				break
			}
		}

		h.pendingResult.Store(&hoverResult{
			seq:         seq,
			content:     content,
			pixelPos:    pixelPos,
			fallbackPos: fallbackPos,
		})
	}()
}

func (h *HoverTips) Clear(gtx layout.Context) {
	h.spanStyles = h.spanStyles[:0]
	h.list.ScrollTo(0)
	h.anim.Disappear(gtx.Now)
	h.cancelled = false
	h.needRebuild = false
	h.editor.RemoveCommands(h)
}

func (h *HoverTips) SetColorScheme(colorScheme string) {
	h.colorScheme = buildColorScheme(colorScheme)
}

func (h *HoverTips) buildSpans(th *theme.Theme) {
	if len(h.lastContent) == 0 {
		return
	}

	mdRenderer := markdown.NewRenderer()
	mdRenderer.Config = markdown.Config{
		DefaultFont: font.Font{
			Typeface: th.Face,
		},
		MonospaceFont: font.Font{Typeface: "monospace"},
		DefaultSize:   th.TextSize,
		DefaultColor:  h.colorScheme.Foreground.NRGBA(),
		H6Size:        th.TextSize,
		H5Size:        unit.Sp(math.Round(1.2 * float64(th.TextSize))),
		H4Size:        unit.Sp(math.Round(1.3 * float64(th.TextSize))),
		H3Size:        unit.Sp(math.Round(1.4 * float64(th.TextSize))),
		H2Size:        unit.Sp(math.Round(1.5 * float64(th.TextSize))),
		H1Size:        unit.Sp(math.Round(1.6 * float64(th.TextSize))),
	}

	spanStyles, err := mdRenderer.Render([]byte(h.lastContent))
	if err != nil {
		log.Println("render hover content failed", err)
		return
	}

	h.spanStyles = spanStyles
}

func (h *HoverTips) Update(gtx layout.Context, th *theme.Theme) (string, bool) {
	// press ESC to cancel and close the popup
	h.editor.RegisterCommand(h, key.Filter{Name: key.NameEscape},
		func(gtx layout.Context, evt key.Event) gvcode.EditorEvent {
			h.Clear(gtx)
			return nil
		},
	)

	for {
		span, event, ok := h.label.Update(gtx)
		if !ok {
			break
		}
		url := span.Get(markdown.MetadataURL)
		switch event.Type {
		case richtext.Click:
			if event.ClickData.Kind == gesture.KindClick {
				link, external, err := parseLink(url.(string))
				if err != nil {
					log.Printf("error opening link: %v, url: %s", err, url.(string))
					continue
				}
				return link, external

			}
		}
	}

	// Check if mouse is hovering over tip area or scrollbar is being interacted with
	scrollbarInteracted := false
	if h.tipBounds != (image.Rectangle{}) {
		// Check scrollbar state
		scrollbarInteracted = h.list.Scrollbar.Dragging() || h.list.Scrollbar.IndicatorHovered() || h.list.Scrollbar.TrackHovered()

		if h.tipHover.Update(gtx.Source) || scrollbarInteracted {
			h.hoverInTip = true
			// If mouse is in tip area, cancel any pending dismissal
			h.cancelled = false
		} else {
			h.hoverInTip = false
		}
	} else {
		h.hoverInTip = false
	}
	// If mouse leaves tip area while tip is visible, schedule dismissal
	if h.prevHoverInTip && !h.hoverInTip && h.anim.Visible() && !h.cancelled {
		h.cancelled = true
	}
	h.prevHoverInTip = h.hoverInTip

	if h.cancelled && h.anim.Visible() {
		h.anim.Disappear(gtx.Now.Add(300 * time.Millisecond))
		h.cancelled = false
	}

	if !h.anim.Visible() && len(h.spanStyles) != 0 {
		h.spanStyles = h.spanStyles[:0]
		h.list.ScrollTo(0)
		h.tipBounds = image.Rectangle{}
		h.hoverInTip = false
		h.prevHoverInTip = false
	}

	// Pick up async hover result from the goroutine.
	if result := h.pendingResult.Swap(nil); result != nil && result.seq == h.hoverSeq {
		h.cancelled = false
		h.lastContent = result.content
		if result.pixelPos != (f32.Point{}) {
			h.lastHoverPos = image.Point{X: result.pixelPos.Round().X, Y: result.pixelPos.Round().Y}
		} else {
			h.lastHoverPos = result.fallbackPos
		}
		h.needRebuild = true
	}

	if h.needRebuild {
		h.buildSpans(th)
		h.anim.Appear(gtx.Now.Add(350 * time.Millisecond))
		h.needRebuild = false
	}

	return "", false
}

func (h *HoverTips) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Should be called for every frame.
	if h.anim.Animating() {
		_ = h.anim.Revealed(gtx)
	}

	if !h.anim.Visible() || len(h.spanStyles) == 0 {
		return layout.Dimensions{}
	}

	h.list.Axis = layout.Vertical

	scrollIndicatorColor := h.colorScheme.Foreground.MulAlpha(0x30)
	scrollbar := utils.MakeScrollbar(th.Theme, &h.list.Scrollbar, scrollIndicatorColor.NRGBA())
	listStyle := material.List(th.Theme, &h.list)
	listStyle.ScrollbarStyle = scrollbar
	listStyle.AnchorStrategy = material.Overlay

	macro := op.Record(gtx.Ops)
	dims := h.layoutContent(gtx, th, &listStyle)
	h.tipBounds = image.Rectangle{Max: dims.Size}
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	// Paint background (no animation)
	if h.colorScheme.Background.IsSet() {
		bgColor := h.colorScheme.Background.NRGBA()
		// Paint rounded rectangle background matching border radius
		rr := clip.RRect{
			Rect: image.Rectangle{Max: gtx.Constraints.Max},
			NE:   gtx.Dp(8), NW: gtx.Dp(8), SE: gtx.Dp(8), SW: gtx.Dp(8),
		}
		paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))
	}

	h.tipHover.Add(gtx.Ops)
	callOp.Add(gtx.Ops)

	return dims
}

func (h *HoverTips) layoutContent(gtx layout.Context, th *theme.Theme, listStyle *material.ListStyle) layout.Dimensions {
	maxSize := gtx.Constraints.Max
	maxSize.X = int(float32(maxSize.X) * 0.7)
	maxSize.Y = min(gtx.Dp(maxHoverTipY), int(float32(maxSize.Y)*0.8))
	gtx.Constraints.Max = maxSize
	gtx.Constraints.Min = image.Point{}

	return widget.Border{
		Color:        h.colorScheme.Foreground.MulAlpha(0x30).NRGBA(),
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(8),
	}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return listStyle.Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
				return layout.Inset{
					Left:   unit.Dp(12),
					Right:  unit.Dp(12),
					Top:    unit.Dp(12),
					Bottom: unit.Dp(12),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return richtext.Text(&h.label, th.Shaper, h.spanStyles...).Layout(gtx)
				})
			})
		})
}

// parseLink returns a possible link, a boolean indicating if it is to open in browser or not, and
// a possible error.
func parseLink(link string) (string, bool, error) {
	parsedUrl, err := url.Parse(link)
	if err != nil {
		return "", false, err
	}

	if parsedUrl.Scheme == "http" || parsedUrl.Scheme == "https" {
		log.Println("is http link")
		return link, true, nil
	}

	// tinymist uses command:tinymist.openInternal?["file:///Users/zjzhang/work/projects/typst-project/doc-test/figures/blue.png"]
	// or command:tinymist.openExternal?["file:///Users/zjzhang/work/projects/typst-project/doc-test/figures/blue.png"]
	// to tell LSP to open file, but we are not going to leverage LSP command here.

	extractPath := func(query string) string {
		finalLink, err := url.QueryUnescape(query)
		if err == nil {
			finalLink = strings.Trim(finalLink, "[\"]")
			return finalLink
		} else {
			return query
		}
	}

	var openExternal bool
	var finalLink string
	if strings.HasPrefix(link, "command:tinymist.openInternal?") {
		openExternal = false
		finalLink = extractPath(parsedUrl.RawQuery)
		finalLink = strings.TrimPrefix(finalLink, "file://")
	}
	if strings.HasPrefix(link, "command:tinymist.openExternal?") {
		openExternal = true
		finalLink = extractPath(parsedUrl.RawQuery)
	}

	// The file path is also escaped by TinyMist.
	finalLink, _ = url.QueryUnescape(finalLink)
	return finalLink, openExternal, nil
}
