package editors

import (
	"errors"
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/unit"
	"looz.ws/typstify/editor"
	"looz.ws/typstify/service"

	"github.com/oligo/gioview/page"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	//"image"
)

var (
	GenericTextEditorViewID = view.NewViewID("GenericTextEditor")
)

type GenericTextEditor struct {
	*view.BaseView
	page.PageStyle
	srv *service.ServiceFacade

	srcEditor   *editor.TextEditor
	header      *editorHeader
	currentFile string
}

func (te *GenericTextEditor) ID() view.ViewID {
	return GenericTextEditorViewID
}

func (te *GenericTextEditor) Title() string {
	if te.currentFile == "" {
		return "Text Editor"
	} else {
		return filepath.Base(te.currentFile)
	}
}

func (te *GenericTextEditor) OnNavTo(intent view.Intent) error {
	te.BaseView.OnNavTo(intent)
	path, ok := intent.Params["path"].(string)
	if !ok {
		return errors.New("missing parameters")
	}

	rootDir := te.srv.CurrentProjectDir()

	showDiff := te.srv.Workspace().Current().GitBranch != ""

	te.currentFile = path
	srcEditor, err := editor.NewTextEditor(path, showDiff, te.srv.Settings().Editor())
	if err != nil {
		return err
	}
	if err := srcEditor.BindWorkspaceWatcher(te.srv); err != nil {
		srcEditor.Close()
		return err
	}

	te.srcEditor = srcEditor
	te.header = newEditorHeader(rootDir, te.currentFile, te.headerActions())
	return nil
}

func (te *GenericTextEditor) headerActions() []editorHeaderAction {
	return []editorHeaderAction{
		{
			Name: "Search & Replace",
			Icon: searchIcon,
			OnClicked: func(gtx C) {
				te.srcEditor.ToggleSearchBar(gtx)
			},
		},
	}
}

func (te *GenericTextEditor) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return te.layoutEditor(gtx, th)
}

func (te *GenericTextEditor) layoutEditor(gtx C, th *theme.Theme) D {

	return layout.Inset{
		Left:  unit.Dp(1),
		Right: unit.Dp(0),
		Top:   unit.Dp(1),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				te.header.SetCurrentPath(te.currentFile)
				return te.header.Layout(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx C) D {
				return te.srcEditor.Layout(gtx, th, te.srv.Settings().Editor())
			}),
		)
	})
}

// Implements StatusIndicator to let statusbar render it.
func (te *GenericTextEditor) LayoutStatus(gtx C, th *theme.Theme) D {
	return te.srcEditor.LayoutStatus(gtx, th, te.srv)
}

func (va *GenericTextEditor) OnFinish() {
	va.BaseView.OnFinish()
	// Put your cleanup code here.
	if va.srcEditor != nil {
		va.srcEditor.Close()
	}
}

func NewGenericTextEditor(srv *service.ServiceFacade) view.View {
	return &GenericTextEditor{
		BaseView: &view.BaseView{},
		srv:      srv,
	}
}
