package preview

import (
	"image"

	"gioui.org/f32"
	"gioui.org/op"
	"github.com/gioui-plugins/gio-plugins/plugin/gioplugins"
	"github.com/gioui-plugins/gio-plugins/webviewer/giowebview"
	"github.com/oligo/gioview/theme"
)

// Script injected into the webview to block all keyboard events.
// The preview panel is read-only; preventing key events stops the
// native webview from stealing input (e.g. F, H causing scrolling)
// that should go to the Gio editor.
const blockKeyboardJS = `document.addEventListener('keydown', function(e){ e.preventDefault(); e.stopPropagation(); }, true);
document.addEventListener('keyup', function(e){ e.preventDefault(); e.stopPropagation(); }, true);`

type WebView struct {
	tag         int
	currentURL  string
	pendingURL  string
	initialized bool // true after first frame with WebViewOp
	jsInstalled bool // true after keyboard-blocking JS is installed
}

func NewWebView() *WebView {
	return &WebView{}
}

func (wv *WebView) Navigate(url string) {
	wv.pendingURL = url
}

// Layout renders the webview at the given absolute offset within the window
// content area. The giowebview plugin does not track Gio's transform stack,
// so the caller must provide the absolute position.
func (wv *WebView) Layout(gtx C, th *theme.Theme, absOffset f32.Point) D {
	// Drain webview events.
	for {
		_, ok := gioplugins.Event(gtx, giowebview.Filter{Target: &wv.tag})
		if !ok {
			break
		}
	}

	w := float32(gtx.Constraints.Max.X)
	h := float32(gtx.Constraints.Max.Y)

	// Push WebViewOp to create the webview instance (processed at frame end).
	defer giowebview.WebViewOp{Tag: &wv.tag}.Push(gtx.Ops).Pop(gtx.Ops)
	giowebview.OffsetOp{Point: absOffset}.Add(gtx.Ops)
	giowebview.RectOp{Size: f32.Point{X: w, Y: h}}.Add(gtx.Ops)

	if wv.initialized {
		// Install keyboard-blocking JS once, before the first navigation.
		if !wv.jsInstalled {
			gioplugins.Execute(gtx, giowebview.InstallJavascriptCmd{
				View:   &wv.tag,
				Script: blockKeyboardJS,
			})
			wv.jsInstalled = true
		}

		if wv.pendingURL != "" {
			gioplugins.Execute(gtx, giowebview.NavigateCmd{
				View: &wv.tag,
				URL:  wv.pendingURL,
			})
			wv.currentURL = wv.pendingURL
			wv.pendingURL = ""
		}
	} else {
		wv.initialized = true
		gtx.Execute(op.InvalidateCmd{})
	}

	return D{Size: image.Point{X: int(w), Y: int(h)}}
}
