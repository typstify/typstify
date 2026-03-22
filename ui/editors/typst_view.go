package editors

import (
	//"image"

	"context"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"github.com/oligo/gvcode"
	"looz.ws/typstify/editor"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/preview"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/dialog"
	uipreview "looz.ws/typstify/ui/preview"
	"looz.ws/typstify/ui/viewer"
	"looz.ws/typstify/utils"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	TypstEditorViewID = view.NewViewID("TypstEditor")
	previewIcon, _    = widget.NewIcon(icons.ActionPageview)
	exportIcon, _     = widget.NewIcon(icons.CommunicationImportExport)
)

type TypstEditor struct {
	*view.BaseView
	srv           *service.ServiceFacade
	previewClient *preview.PreviewClient
	srcEditor     *editor.TextEditor
	targetFile    string // the main file
	currentFile   string // switched temp file
	breadcrums    *fileBreadcrums
	lspReady      bool

	// Preview
	uiPreviewer    *uipreview.Previewer
	previewVisible bool
}

func (te *TypstEditor) ID() view.ViewID {
	return TypstEditorViewID
}

func (te *TypstEditor) Title() string {
	if te.targetFile == "" {
		return "Typst Editor"
	} else {
		return filepath.Base(te.targetFile)
	}
}

func (te *TypstEditor) OnNavTo(intent view.Intent) error {
	te.BaseView.OnNavTo(intent)
	path, ok := intent.Params["path"].(string)
	if !ok {
		return errors.New("missing parameters")
	}

	te.targetFile = path
	te.currentFile = te.targetFile

	rootDir := te.srv.CurrentProjectDir()
	if rootDir == "" {
		rootDir = filepath.Dir(te.targetFile)
	}

	err := te.setupEditor(path, false, false)
	if err != nil {
		return err
	}

	te.breadcrums = newBreadcrums(rootDir, te.targetFile, te.onSelectFile)
	te.lspReady = false

	client := lsp.GetLspClient(rootDir, te.srv.Settings())
	if client != nil {
		te.previewClient = preview.NewPreviwClient(client, te.targetFile)
	}

	return nil
}

func (te *TypstEditor) setupEditor(path string, createOnMissing bool, readonly bool) error {
	srcEditor, err := editor.NewTextEditor(path, createOnMissing, readonly, te.srv.Settings().Editor())
	if err != nil {
		return err
	}

	te.srcEditor = srcEditor

	te.srcEditor.OnSelectChange = func(p gvcode.Position) {
		if te.previewClient != nil {
			te.previewClient.ScrollOnSelectionChange(context.Background(), p)
		}
	}
	te.srcEditor.OnOpenLink = te.openLink
	return nil
}

func (te *TypstEditor) setupLsp(gtx layout.Context, th *theme.Theme) {
	if te.lspReady {
		return
	}
	defer func() {
		te.lspReady = true
	}()

	client := lsp.GetLspClient(te.srv.CurrentProjectDir(), te.srv.Settings())
	if client == nil {
		log.Println("LSP client is not initialized!")
		return
	}

	te.srcEditor.SetupLsp(gtx, th, client)
}

func (te *TypstEditor) isExternalFile(file string) bool {
	if te.srv.CurrentProjectDir() == "" {
		return false
	}

	return !strings.HasPrefix(file, te.srv.CurrentProjectDir())
}

func (te *TypstEditor) onSelectFile(path string) {
	te.currentFile = path
	log.Println("open editor: ", path)
}

// func (te *TypstEditor) IsReadOnly() bool {
// 	return te.srcEditor.state.Mode() == gvcode.ModeReadOnly
// }

func (te *TypstEditor) Actions() []view.ViewAction {
	return []view.ViewAction{
		{
			Name: "Preview",
			Icon: previewIcon,
			OnClicked: func(gtx C) {
				te.previewVisible = !te.previewVisible

				if te.previewClient == nil {
					return
				}

				openInBrowser := te.srv.Settings().General().OpenPreviewInBrowser != 0
				if !te.previewVisible {
					te.previewClient.Close(context.Background())

					// Close existing preview webview if switching to browser mode
					if te.uiPreviewer != nil {
						te.uiPreviewer.CancelPopup()
						//te.uiPreviewer.Destroy()
						//te.uiPreviewer = nil
					}
					return
				}

				// create new
				serverAddr, err := te.previewClient.New(context.Background(),
					preview.PreviewOptions{
						PreviewMode:      "document",
						ProjectRoot:      te.srv.CurrentProjectDir(),
						FontPath:         te.srv.CurrentProjectDir(),
						PackagePath:      te.srv.Settings().Typst().PackageDir,
						PackageCachePath: te.srv.Settings().Typst().PackageCacheDir,
						InvertColor:      "never",
						PartialRender:    false,
						OpenInBrowser:    openInBrowser,
					})

				if err != nil {
					log.Println("preview ERR: ", err)
					return
				}

				// If OpenInBrowser is true, the LSP handles opening the browser
				if openInBrowser {
					return
				}

				if serverAddr != "" {
					if te.uiPreviewer == nil {
						te.uiPreviewer = uipreview.NewPreviewer(te.targetFile, te.srv)
					}
					te.uiPreviewer.Navigate(serverAddr)
					// te.uiPreviewer.Popup()
				}

			},
		},

		{
			Name: "Export",
			Icon: exportIcon,
			OnClicked: func(gtx C) {
				te.srv.RequestSwitch(view.Intent{
					Target:      dialog.ExportDialogViewID,
					ShowAsModal: true,
					Params: map[string]interface{}{
						"targetFile": te.targetFile,
					},
				})
			},
		},
	}
}

func (te *TypstEditor) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if te.currentFile != te.srcEditor.File() {
		isExternal := te.isExternalFile(te.currentFile)
		// close old one
		te.srcEditor.Close()
		// only project local file path can be created.
		te.setupEditor(te.currentFile, !isExternal, isExternal)
		te.lspReady = false
	}

	te.setupLsp(gtx, th)

	return layout.Inset{
		Left:  unit.Dp(1),
		Right: unit.Dp(1),
		Top:   unit.Dp(1),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return te.breadcrums.Layout(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return te.srcEditor.Layout(gtx, th, te.srv.Settings().Editor())
			}),
		)
	})
}

// IsPreviewVisible returns whether the inline preview panel should be shown.
func (te *TypstEditor) IsVisible() bool {
	return te.previewVisible && te.uiPreviewer != nil
}

// HidePreview hides the native webview so it stops intercepting keyboard events.
func (te *TypstEditor) HidePreview(gtx C) {
	if te.uiPreviewer != nil {
		te.uiPreviewer.HideWebView(gtx)
	}
}

// LayoutPreview renders the preview panel. Called by home.go when preview is active.
func (te *TypstEditor) LayoutPreview(gtx C, th *theme.Theme) D {
	return te.uiPreviewer.Layout(gtx, th)
}

// Implements StatusIndicator to let statusbar render it.
func (te *TypstEditor) LayoutStatus(gtx C, th *theme.Theme) D {
	return te.srcEditor.LayoutStatus(gtx, th, te.srv)
}

func (te *TypstEditor) OnFinish() {
	te.BaseView.OnFinish()
	if te.srcEditor != nil {
		te.srcEditor.Close()
	}

	if te.previewClient != nil {
		te.previewClient.Destroy(context.Background())
	}

	if te.uiPreviewer != nil {
		te.uiPreviewer.Destroy()
	}
}

func (te *TypstEditor) openLink(link string, external bool) {
	pattern := `(\.png|\.jpg|\.jpeg|\.gif|\.PNG|\.JPG|\.JPEG|\.GIF)$`
	matched, err := regexp.MatchString(pattern, link)

	if external {
		if matched {
			utils.OpenInExternalApp(link)
			return
		}
		// open doc in browser
		if err := giohyperlink.Open(link); err != nil {
			log.Printf("error: opening hyperlink: %v, url: %s", err, link)
		}
	} else {
		if err == nil && matched {
			openIntent := view.Intent{
				Target:      viewer.ImgViewerViewID,
				ShowAsModal: false,
				RequireNew:  true,
				Params: map[string]interface{}{
					"path": link,
				},
			}
			te.srv.RequestSwitch(openIntent)
		} else {
			utils.OpenInExternalApp(link)
		}
	}
	// gtx.Execute(op.InvalidateCmd{})
}

func NewTypstEditor(srv *service.ServiceFacade) view.View {
	return &TypstEditor{
		BaseView: &view.BaseView{},
		srv:      srv,
	}
}
