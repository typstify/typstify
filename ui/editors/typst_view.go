package editors

import (
	//"image"

	"context"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"github.com/oligo/gvcode"
	"looz.ws/typstify/editor"
	"looz.ws/typstify/lsp"
	lspProtocol "looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/dialog"
	uipreview "looz.ws/typstify/ui/preview"
	"looz.ws/typstify/ui/viewer"
	"looz.ws/typstify/utils"
	appIcons "looz.ws/typstify/widgets/icons"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	TypstEditorViewID = view.NewViewID("TypstEditor")
	searchIcon        = appIcons.NewSvgIcon(appIcons.Search)
	previewIcon       = appIcons.NewSvgIcon(appIcons.ScanSearch)
	exportIcon        = appIcons.NewSvgIcon(appIcons.ArrowRightFromLine)
	presentationIcon  = appIcons.NewSvgIcon(appIcons.Presentation)
	tocIcon           = appIcons.NewSvgIcon(appIcons.TableOfContents)
)

type TypstEditor struct {
	*view.BaseView
	srv        *service.ServiceFacade
	srcEditor  *editor.TextEditor
	targetFile string
	header     *editorHeader
	lspReady   bool

	// Preview
	uiPreviewer    *uipreview.Previewer
	previewVisible bool
	toggleModeBtn  widget.Clickable

	// Outline
	cachedSymbols []lspProtocol.DocumentSymbol
	symbolsDirty  bool
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

	rootDir := te.srv.CurrentProjectDir()
	if rootDir == "" {
		rootDir = filepath.Dir(te.targetFile)
	}

	err := te.setupEditor(path)
	if err != nil {
		return err
	}

	te.header = newEditorHeader(rootDir, te.targetFile, te.headerActions())
	te.lspReady = false
	te.symbolsDirty = true

	return nil
}

func (te *TypstEditor) OnResume() {
	previewSrv := te.srv.PreviewService()
	if previewSrv == nil {
		return
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		return
	}

	openInBrowser := te.srv.Settings().General().OpenPreviewInBrowser != 0
	var isLinux = runtime.GOOS == "linux"
	if openInBrowser || isLinux {
		return
	}

	if !te.previewVisible {
		return
	}

	if serverAddr != "" && te.uiPreviewer != nil {
		te.srcEditor.FocusLsp()
		te.uiPreviewer.Navigate(serverAddr)
	}
}

func (te *TypstEditor) setupEditor(path string) error {
	showDiff := te.srv.Workspace().GitRepo().Branch != ""
	srcEditor, err := editor.NewTextEditor(path, showDiff, te.srv.Settings().Editor())
	if err != nil {
		return err
	}
	if err := srcEditor.BindWorkspaceWatcher(te.srv); err != nil {
		srcEditor.Close()
		return err
	}

	te.srcEditor = srcEditor

	te.srcEditor.OnSelectChange = func(p gvcode.Position) {
		previewSrv := te.srv.PreviewService()
		if previewSrv != nil {
			previewSrv.ScrollOnSelectionChange(context.Background())
		}
	}
	te.srcEditor.OnOpenLink = te.openLink
	te.srcEditor.OnTextChange = func() {
		te.symbolsDirty = true
	}

	return nil
}

func (te *TypstEditor) setupLsp(gtx layout.Context) {
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

	te.srcEditor.SetupLsp(gtx, client)
}

func (te *TypstEditor) headerActions() []editorHeaderAction {
	return []editorHeaderAction{

		{
			Name: "Preview",
			Icon: previewIcon,
			OnClicked: func(gtx C) {
				te.togglePreview(gtx)
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
		// {
		// 	Name: "Outline",
		// 	Icon: tocIcon,
		// 	OnClicked: func(gtx C) {
		// 	},
		// },
		{
			Name: "Search & Replace",
			Icon: searchIcon,
			OnClicked: func(gtx C) {
				te.srcEditor.ToggleSearchBar(gtx)
			},
		},
	}
}

func (te *TypstEditor) update(gtx C) {
	te.setupLsp(gtx)

	// global key handler.
	for {
		e, ok := gtx.Event(
			key.Filter{Name: "P", Required: key.ModShortcut}, // toggle hide/show of previewer.
		)
		if !ok {
			break
		}

		switch event := e.(type) {
		case key.Event:
			if event.State != key.Press {
				continue
			}

			if event.Name == "P" && event.Modifiers.Contain(key.ModShortcut) {
				te.togglePreview(gtx)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
}

func (te *TypstEditor) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	te.update(gtx)

	return layout.Inset{
		Left:  unit.Dp(1),
		Right: unit.Dp(1),
		Top:   unit.Dp(1),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return te.header.Layout(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return te.srcEditor.Layout(gtx, th, te.srv.Settings().Editor())
			}),
		)
	})
}

func (te *TypstEditor) SetPreviewer(previewer *uipreview.Previewer) {
	te.uiPreviewer = previewer
}

// IsPreviewVisible returns whether the inline preview panel should be shown.
func (te *TypstEditor) IsVisible() bool {
	return te.previewVisible
}

// LayoutPreview renders the preview panel. Called by home.go when preview is active.
func (te *TypstEditor) LayoutPreview(gtx C, th *theme.Theme) D {
	return te.uiPreviewer.Layout(gtx, th)
}

func (te *TypstEditor) togglePreview(gtx C) {
	previewSrv := te.srv.PreviewService()
	if previewSrv == nil {
		return
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		log.Println("preview ERR: no preview server address")
		return
	}

	// focus LSP triggers a refresh of the preview server.
	te.srcEditor.FocusLsp()

	openInBrowser := te.srv.Settings().General().OpenPreviewInBrowser != 0
	var isLinux = runtime.GOOS == "linux"
	if (openInBrowser || isLinux) && serverAddr != "" {
		utils.OpenInExternalApp(serverAddr)
		te.previewVisible = false
		return
	}

	// built-in previewer
	te.previewVisible = !te.previewVisible

	if !te.previewVisible {
		return
	}

	if serverAddr != "" && te.uiPreviewer != nil {
		te.uiPreviewer.Navigate(serverAddr)
	}
}

// Implements StatusIndicator to let statusbar render it.
func (te *TypstEditor) LayoutStatus(gtx C, th *theme.Theme) D {
	if !te.previewVisible {
		return te.srcEditor.LayoutStatus(gtx, th, te.srv)
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return te.srcEditor.LayoutStatus(gtx, th, te.srv)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx C) D {
			if te.toggleModeBtn.Clicked(gtx) {
				te.uiPreviewer.ToggleMode()
			}

			iconColor := utils.DisableColor(th.Fg)
			if te.uiPreviewer.Mode() == lsp.SlidePreviewMode {
				iconColor = th.ContrastBg
			}

			return te.toggleModeBtn.Layout(gtx, func(gtx C) D {
				return presentationIcon.Layout(gtx, iconColor, th.TextSize)
			})
		}),
	)
}

// OutlineSymbols implements navpanel.OutlineProvider.
func (te *TypstEditor) OutlineSymbols() []lspProtocol.DocumentSymbol {
	if te.symbolsDirty {
		te.symbolsDirty = false
		client := lsp.GetLspClient(te.srv.CurrentProjectDir(), te.srv.Settings())
		if client != nil && client.IsReady() {
			symbols, err := client.DocumentSymbols(context.Background(), te.targetFile)
			if err == nil {
				te.cachedSymbols = symbols
			} else {
				log.Println("fetch symbol error: ", err)
			}
		}
	}
	return te.cachedSymbols
}

// OnOutlineSymbolSelected implements navpanel.OutlineProvider.
func (te *TypstEditor) OnOutlineSymbolSelected(symbol lspProtocol.DocumentSymbol) {
	line := int(symbol.SelectionRange.Start.Line)
	col := int(symbol.SelectionRange.Start.Character)
	te.srcEditor.NavigateToLine(line, col)
}

func (te *TypstEditor) OnFinish() {
	te.BaseView.OnFinish()
	if te.srcEditor != nil {
		te.srcEditor.Close()
	}
}

func (te *TypstEditor) openLink(link string, external bool) {
	isHttpLink := strings.HasPrefix(link, "https://") && strings.HasPrefix(link, "http://")
	if isHttpLink {
		if err := giohyperlink.Open(link); err != nil {
			log.Printf("error: opening hyperlink: %v, url: %s", err, link)
		}
		return
	}

	if external {
		utils.OpenInExternalApp(link)
		return
	}

	// Open using internal views.

	pattern := `(\.png|\.jpg|\.jpeg|\.gif|\.PNG|\.JPG|\.JPEG|\.GIF)$`
	matched, err := regexp.MatchString(pattern, link)

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
		return

	}

	if utils.IsTextFile(link) {
		target := GenericTextEditorViewID
		if strings.HasSuffix(link, ".typ") {
			target = TypstEditorViewID
		}
		// open as text
		intent := view.Intent{
			Target:      target,
			ShowAsModal: false,
			RequireNew:  true,
			Params: map[string]interface{}{
				"path": link,
			},
		}
		te.srv.RequestSwitch(intent)
		return
	} else {
		utils.OpenInExternalApp(link)
	}
}

func NewTypstEditor(srv *service.ServiceFacade) view.View {
	return &TypstEditor{
		BaseView: &view.BaseView{},
		srv:      srv,
	}
}
