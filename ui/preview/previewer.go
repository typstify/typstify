package preview

import (
	"context"
	"image"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"

	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

type Previewer struct {
	srv         *service.ServiceFacade
	targetFile  string
	err         error
	popupCancel context.CancelFunc
	windowChan  chan struct{}
	isPopup     bool
	webview     *WebView
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

func (p *Previewer) Layout(gtx C, th *theme.Theme) D {
	// Left inset so the native webview doesn't cover the resize drag handle.
	return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		absOffset := f32.Point{
			X: float32(p.srv.WindowContentWidth - gtx.Constraints.Max.X),
			Y: float32(p.srv.ViewAreaTopOffset),
		}

		macro := op.Record(gtx.Ops)
		dims := p.webview.Layout(gtx, th, absOffset)
		callOp := macro.Stop()

		defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
		callOp.Add(gtx.Ops)
		return dims
	})
}

func (p *Previewer) LayoutPopupIndicator(gtx C, th *theme.Theme) D {
	return layout.Center.Layout(gtx, func(gtx C) D {
		return material.Label(th.Theme, th.TextSize, i18n.Translate("Previewing is switched to the pop-up window.")).Layout(gtx)
	})
}

func (p *Previewer) IsPopup() bool {
	return p.isPopup && p.popupCancel != nil && p.windowChan != nil
}

func (p *Previewer) Popup() bool {
	if !p.isPopup {
		return false
	}

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
		switch e := w.Event().(type) {
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
