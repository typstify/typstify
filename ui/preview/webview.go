//go:build !linux

package preview

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"log"
	"sync"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/gioui-plugins/gio-plugins/plugin/gioplugins"
	"github.com/gioui-plugins/gio-plugins/webviewer/giowebview"
	"github.com/oligo/gioview/theme"
)

type WebView struct {
	tag         int
	currentURL  string
	initialized bool // true after first frame with WebViewOp
	once        sync.Once
}

func NewWebView() *WebView {
	if runtime.GOOS == "windows" {
		ensureWebView2UserDataFolder()
	}

	return &WebView{}
}

// ensureWebView2UserDataFolder set the UDF for WebView2 on Windows.
// For UDF in WebView2, see https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder?tabs=win32.
//
// If the app is installed in folder like 'Program Files', we may have no
// privileges to write to the installation folder.
func ensureWebView2UserDataFolder() {
	candidates := []string{}

	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		candidates = append(candidates, filepath.Join(cacheDir, "Typstify", "WebView2"))
	}
	candidates = append(candidates,
		filepath.Join(os.TempDir(), "Typstify", "WebView2"),
		os.TempDir(),
	)

	for _, dataDir := range candidates {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			continue
		}

		// WebView2 needs a writable user-data folder. When the app is run elevated
		// from Program Files, the default sibling folder is not writable.
		if err := os.Setenv("WEBVIEW2_USER_DATA_FOLDER", dataDir); err == nil {
			return
		}
	}

	log.Println("WebView2: could not set a writable user-data folder; preview may fail when run elevated")
}

func (wv *WebView) Navigate(url string) {
	wv.currentURL = url
	wv.initialized = false
	wv.once = sync.Once{}
}

func (wv *WebView) initialize(gtx layout.Context) {
	if wv.currentURL == "" {
		return
	}

	wv.once.Do(func() {
		gioplugins.Execute(gtx, giowebview.NavigateCmd{
			View: &wv.tag,
			URL:  wv.currentURL,
		})
	})

}

// Destroy sends a DestroyCmd to giowebview to permanently destroy the native webview.
func (wv *WebView) Destroy(gtx layout.Context) {
	if !wv.initialized {
		return
	}

	// DestroyCmd does not work for now, as it's just a empty cmd in giowebview.
	// We place it here to wait for it to be implemented in the future.
	gioplugins.Execute(gtx, giowebview.DestroyCmd{View: &wv.tag})
	// Reset state so a new webview can be created if needed
	wv.initialized = false
	wv.currentURL = ""
}

// Layout renders the webview at the given absolute offset within the window
// content area. The giowebview plugin does not track Gio's transform stack,
// so the caller must provide the absolute position.
func (wv *WebView) Layout(gtx layout.Context, th *theme.Theme, absOffset f32.Point) layout.Dimensions {
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

	if !wv.initialized {
		wv.initialized = true
		gtx.Execute(op.InvalidateCmd{})
	} else {
		wv.initialize(gtx)
	}

	return layout.Dimensions{Size: image.Point{X: int(w), Y: int(h)}}
}
