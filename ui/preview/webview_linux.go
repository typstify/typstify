package preview

import (
	"gioui.org/f32"
	"gioui.org/layout"
	"github.com/oligo/gioview/theme"
)

// WebView in linux is not supported by gioplugins for now.
// This is just a placeholder struct.
type WebView struct {
}

func NewWebView() *WebView {
	return &WebView{}
}

func (wv *WebView) Navigate(url string) {
}

func (wv *WebView) Destroy(gtx layout.Context) {
}

func (wv *WebView) Layout(gtx layout.Context, th *theme.Theme, absOffset f32.Point) layout.Dimensions {
	return layout.Dimensions{}
}
