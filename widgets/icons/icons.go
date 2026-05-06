package icons

import (
	_ "embed"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/inkeliz/giosvg"
)

//go:embed lucide/chevron-right.svg
var ChevronRight []byte

//go:embed lucide/chevron-down.svg
var ChevronDown []byte

//go:embed lucide/chevron-up.svg
var ChevronUp []byte

//go:embed lucide/file-text.svg
var FileText []byte

//go:embed lucide/file-image.svg
var FileImage []byte

//go:embed lucide/file-type.svg
var FileType []byte

//go:embed lucide/file-code.svg
var FileCode []byte

//go:embed lucide/binary.svg
var FileBinary []byte

//go:embed lucide/x.svg
var X []byte

//go:embed lucide/trash.svg
var Trash []byte

//go:embed lucide/brush-cleaning.svg
var BrushCleaning []byte

//go:embed lucide/search.svg
var Search []byte

//go:embed lucide/arrow-up.svg
var ArrowUp []byte

//go:embed lucide/arrow-down.svg
var ArrowDown []byte

//go:embed lucide/pencil.svg
var Pencil []byte

//go:embed lucide/pencil-off.svg
var PencilOff []byte

//go:embed lucide/circle-alert.svg
var CircleAlert []byte

//go:embed lucide/circle-x.svg
var CircleX []byte

//go:embed lucide/triangle-alert.svg
var TriangleAlert []byte

//go:embed lucide/info.svg
var Info []byte

//go:embed lucide/scan-search.svg
var ScanSearch []byte

//go:embed lucide/arrow-left-right.svg
var ArrowLeftRight []byte

//go:embed lucide/arrow-right-from-line.svg
var ArrowRightFromLine []byte

//go:embed lucide/folder-open.svg
var FolderOpen []byte

//go:embed lucide/file-plus.svg
var FilePlus []byte

//go:embed lucide/list.svg
var List []byte

//go:embed lucide/folder-tree.svg
var FolderTree []byte

//go:embed lucide/settings.svg
var Settings []byte

//go:embed lucide/cog.svg
var Cog []byte

//go:embed lucide/history.svg
var History []byte

//go:embed lucide/refresh-ccw.svg
var RefreshCcw []byte

//go:embed lucide/external-link.svg
var ExternalLink []byte

//go:embed lucide/folder-plus.svg
var FolderPlus []byte

//go:embed lucide/arrow-right.svg
var ArrowRight []byte

//go:embed lucide/ellipsis.svg
var Ellipsis []byte

//go:embed lucide/package.svg
var Package []byte

//go:embed lucide/package-search.svg
var PackageSearch []byte

//go:embed lucide/package-open.svg
var PackageOpen []byte

//go:embed lucide/package-plus.svg
var PackagePlus []byte

//go:embed lucide/user.svg
var User []byte

//go:embed lucide/user-cog.svg
var UserCog []byte

//go:embed lucide/panel-left-close.svg
var PanelLeftClose []byte

//go:embed lucide/panel-right-close.svg
var PanelRightClose []byte

//go:embed lucide/presentation.svg
var Presentation []byte

//go:embed lucide/git-branch.svg
var GitBranch []byte

//go:embed lucide/git-branch-plus.svg
var GitBranchPlus []byte

//go:embed lucide/git-branch-minus.svg
var GitBranchMinus []byte

//go:embed lucide/tag.svg
var Tag []byte

//go:embed lucide/terminal.svg
var Terminal []byte

//go:embed lucide/table-of-contents.svg
var TableOfContents []byte

//go:embed lucide/square-function.svg
var SquareFunction []byte

//go:embed lucide/variable.svg
var Variable []byte

func newIcon(iconData []byte) *giosvg.Icon {
	vector, err := giosvg.NewVector(iconData)
	if err != nil {
		panic(err)
	}

	return giosvg.NewIcon(vector)
}

type SvgIcon struct {
	state *giosvg.Icon
}

// Layout renders the icon
func (i *SvgIcon) Layout(gtx layout.Context, fill color.NRGBA, size unit.Sp) layout.Dimensions {
	iconSize := image.Pt(gtx.Sp(size), gtx.Sp(size))
	gtx.Constraints = layout.Exact(iconSize)

	macro := op.Record(gtx.Ops)
	dims := i.state.Layout(gtx)
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: fill}.Add(gtx.Ops)
	callOp.Add(gtx.Ops)

	return dims
}

func NewSvgIcon(iconData []byte) *SvgIcon {
	return &SvgIcon{
		state: newIcon(iconData),
	}
}
