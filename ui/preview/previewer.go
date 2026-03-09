package preview

import (
	"context"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/gioui-plugins/gio-plugins/plugin/gioplugins"
	"github.com/oligo/gioview/theme"

	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
)

type Previewer struct {
	srv            *service.ServiceFacade
	targetFile     string
	err            error
	popupCancel    context.CancelFunc
	windowChan     chan struct{}
	isPopup        bool
	webview        *WebView
	destroyPending bool // true when webview should be destroyed on next layout
}

func NewPreviewer(targetFile string, srv *service.ServiceFacade) *Previewer {
	return &Previewer{
		srv:        srv,
		targetFile: targetFile,
		webview:    NewWebView(),
	}
}

func (p *Previewer) Navigate(url string) {
	p.webview.Navigate(url)
}

// HideWebView makes the native webview invisible and restores keyboard focus
// to the Gio view. The plugin auto-hides unseen webviews, but on macOS the
// WKWebView retains first-responder status even when hidden, so we must
// explicitly restore focus to the Gio NSView.
func (p *Previewer) HideWebView(gtx layout.Context) {
	//p.webview.Hide(gtx)
}

func (p *Previewer) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Handle pending destroy request
	if p.destroyPending {
		p.webview.Destroy(gtx)
		p.webview = nil
		p.destroyPending = false
		return layout.Dimensions{}
	}

	// If webview was destroyed and not recreated, return empty dimensions
	if p.webview == nil {
		return layout.Dimensions{}
	}

	// Left inset so the native webview doesn't cover the resize drag handle.
	return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		absOffset := f32.Point{
			X: float32(p.srv.WindowContentWidth - gtx.Constraints.Max.X),
			Y: float32(p.srv.ViewAreaTopOffset),
		}
		return p.webview.Layout(gtx, th, absOffset)
	})
}

func (p *Previewer) LayoutPopupIndicator(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Label(th.Theme, th.TextSize, i18n.Translate("Previewing is switched to the pop-up window.")).Layout(gtx)
	})
}

func (p *Previewer) IsPopup() bool {
	return p.isPopup && p.popupCancel != nil && p.windowChan != nil
}

func (p *Previewer) Popup() bool {
	// popup previewer.
	ctx, cancel := context.WithCancel(context.Background())
	p.popupCancel = cancel
	p.windowChan = make(chan struct{})
	p.srv.WindowService().NewWindow(ctx, i18n.Translate("%s preview", filepath.Base(p.targetFile)), p)
	return true
}

// Run displays the widget in a dedicated window.
func (p *Previewer) Run(ctx context.Context, w *service.Window) error {
	var ops op.Ops
	th := w.Service.LoadTheme()

	go func() {
		for {
			select {
			case <-w.Service.Context.Done():
				w.Perform(system.ActionClose)
				return
			case <-ctx.Done():
				w.Perform(system.ActionClose)
				return
			case <-p.windowChan:
				w.Invalidate()
			}
		}
	}()

	for {
		evt := gioplugins.Hijack(w.Window)

		switch e := evt.(type) {
		case app.DestroyEvent:
			p.CancelPopup()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			p.Layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

func (p *Previewer) Close() {
	p.CancelPopup()
}

// Destroy permanently destroys the native webview.
// Called when switching to OpenInBrowser mode to release resources.
func (p *Previewer) Destroy() {
	// We need a valid context to execute the destroy command.
	// Since this is called outside of layout, we defer it to the next layout.
	// The webview will be destroyed on the next frame.
	// Actually, we need to handle this differently - the Destroy needs to be
	// called during a frame. We'll use a flag that gets processed in Layout.
	p.destroyPending = true
}

func (p *Previewer) CancelPopup() {
	if !p.isPopup {
		return
	}

	if p.popupCancel != nil {
		// close popup window
		p.popupCancel()
	}

	if p.windowChan != nil {
		close(p.windowChan)
	}

	p.isPopup = false
}
