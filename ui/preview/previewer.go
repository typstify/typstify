package preview

import (
	"context"
	"log"
	"runtime"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/oligo/gioview/theme"

	"looz.ws/typstify/lsp"
	"looz.ws/typstify/service"
	"looz.ws/typstify/utils"
)

type Previewer struct {
	srv *service.ServiceFacade
	err error
	// visible reports whether the preview panel is shown. The preview is a
	// project level feature, so this state is shared by all views instead of
	// being owned by the active editor.
	visible bool
	// loadedAddr is the preview server address the webview currently shows. It
	// is used to reload the panel when the preview server is restarted, e.g. on
	// a project switch or a preview mode change.
	loadedAddr     string
	webview        *WebView
	previewMode    lsp.PreviewMode
	destroyPending bool // true when webview should be destroyed on next layout
}

func NewPreviewer(srv *service.ServiceFacade) *Previewer {
	return &Previewer{
		srv:         srv,
		webview:     NewWebView(),
		previewMode: lsp.PreviewMode(srv.Workspace().LoadWorkspaceSettings().PreviewMode),
	}
}

func (p *Previewer) Navigate(url string) {
	p.loadedAddr = url
	p.webview.Navigate(url)
}

// IsVisible reports whether the preview panel should be shown.
func (p *Previewer) IsVisible() bool {
	return p.visible
}

// Toggle hides the preview panel when it is visible and shows it otherwise.
func (p *Previewer) Toggle() {
	if p.visible {
		p.Hide()
		return
	}

	p.Show()
}

// Hide hides the preview panel.
func (p *Previewer) Hide() {
	p.visible = false
}

// Show makes the preview panel visible. When the preview is configured to open
// in an external browser, the preview URL is opened instead and no panel is
// shown.
func (p *Previewer) Show() {
	previewSrv := p.srv.PreviewService()
	if previewSrv == nil {
		return
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		log.Println("preview ERR: no preview server address")
		return
	}

	openInBrowser := p.srv.Settings().Lsp().OpenPreviewInBrowser != 0
	if openInBrowser || runtime.GOOS == "linux" {
		utils.OpenInExternalApp(serverAddr)
		p.visible = false
		return
	}

	p.visible = true
	p.Navigate(serverAddr)
}

func (p *Previewer) Mode() lsp.PreviewMode {
	return p.previewMode
}

func (p *Previewer) ToggleMode() {
	p.previewMode = lsp.PreviewMode(p.srv.Workspace().LoadWorkspaceSettings().PreviewMode)

	switch p.previewMode {
	case lsp.DocumentPreviewMode:
		p.previewMode = lsp.SlidePreviewMode
	case lsp.SlidePreviewMode:
		p.previewMode = lsp.DocumentPreviewMode
	default:
		p.previewMode = lsp.DocumentPreviewMode
	}

	p.srv.Workspace().SetPreviewMode(string(p.previewMode))
	p.srv.RestartPreview(context.Background(), func() {
		serverAddr := p.srv.PreviewService().Address()
		if serverAddr != "" {
			p.Navigate(serverAddr)
		}
	})

}

func (p *Previewer) Restart() {
	p.srv.RestartPreview(context.Background(), func() {
		serverAddr := p.srv.PreviewService().Address()

		if serverAddr != "" {
			p.Navigate(serverAddr)
		}
	})
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

	// Load the preview of the current project when the preview server changed,
	// e.g. after switching projects. This keeps the panel from showing a stale
	// document.
	if p.visible {
		if previewSrv := p.srv.PreviewService(); previewSrv != nil {
			if serverAddr := previewSrv.Address(); serverAddr != "" && serverAddr != p.loadedAddr {
				p.Navigate(serverAddr)
			}
		}
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
