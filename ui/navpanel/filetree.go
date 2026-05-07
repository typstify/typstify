package navpanel

import (
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/editors"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/ui/viewer"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/filetree"
	"looz.ws/typstify/widgets/icons"
	"looz.ws/typstify/widgets/menu"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

type FileTreeNav struct {
	title string
	tree  *filetree.TreeView
	srv   *service.ServiceFacade
	vm    view.ViewManager

	rootSwitched bool
	newRoot      string

	historyBtn      widget.Clickable
	historyProjects *RecentProjects
}

// Construct a FileTreeNav object that loads files and folders from rootDir. The skipFolders
// parameter allows you to specify folder name prefixes to exclude from the navigation.
func NewFileTreeNav(title string, srv *service.ServiceFacade, vm view.ViewManager) *FileTreeNav {
	ftn := &FileTreeNav{
		title:           title,
		srv:             srv,
		vm:              vm,
		historyProjects: NewRecentProjects(srv),
	}

	srv.EventBus().Subscribe(ftn, "filetree", `project\.(switched|create)$`, func(topic string, data interface{}) {
		path, ok := data.(string)
		if !ok {
			panic("not a path")
		}

		if ftn.tree != nil && path == ftn.tree.Root() {
			return
		}

		ftn.saveLastWorkplace()
		ftn.newRoot = path
	})

	return ftn
}

func (tn *FileTreeNav) switchRoot() {
	if tn.newRoot == "" {
		return
	}

	newRoot, err := filepath.Abs(tn.newRoot)
	if err != nil {
		log.Println("convert dir to abs dir error: ", err)
		return
	}

	tn.srv.SetProjectDir(newRoot)

	// Restore the workplace.
	states := tn.srv.Workspace().Current().TreeState
	var newTree *filetree.TreeView
	if states != nil {
		restoredTree, err := filetree.RestoreTree(states)
		if err != nil {
			log.Println("Restore file tree error: ", err)
		} else {
			newTree = restoredTree
		}
	}

	if newTree == nil {
		root, err := explorer.NewFileTree(newRoot)
		if err != nil {
			log.Println("open explorer failed: ", err)
			return
		}

		newTree = filetree.NewTreeView(root)
	}

	// set callbacks for file operations
	newTree.OnFileSelectedFunc = tn.onFileSelected
	newTree.OnDropConfirmFunc = onDropConfirmFunc(tn.vm, newTree.Root())
	newTree.OnFileUpdatedFunc = tn.onFileUpdated
	newTree.OnFileRemoveFunc = tn.onFileDeleted
	newTree.OnErrorFunc = func(err error) {
		log.Println("file tree error: ", err)
		tn.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: err.Error(), Level: 1})
	}

	newTree.ExtraMenuOptionProvider = tn.extraMenuOptions
	newTree.NodeMarkerProvider = tn.nodeMarker

	tn.tree = newTree

	for _, file := range tn.srv.Workspace().Current().OpenedFiles {
		node, err := explorer.NewFileTree(file)
		if err != nil {
			log.Println("open file failed: ", err)
			continue
		}
		tn.onFileSelected(node)
	}

}

func (tn *FileTreeNav) saveLastWorkplace() {
	if tn.tree == nil {
		return
	}

	defer tn.vm.Reset()

	states := tn.tree.Snapshot()
	openedFiles := make([]string, 0)
	views := tn.vm.OpenedViews()
	for _, vw := range views {
		location := vw.Location()
		switch vw.ID() {
		case editors.GenericTextEditorViewID, editors.TypstEditorViewID, viewer.ImgViewerViewID:
			filePath := location.Query().Get("path")
			if filePath != "" {
				openedFiles = append(openedFiles, filePath)
			}
		}
	}

	tn.srv.Workspace().SaveSnapshot(states, openedFiles)
}

func (tn *FileTreeNav) OnClose() {
	tn.saveLastWorkplace()
	if tn.tree != nil {
		tn.tree.Close()
	}
}

func (tn *FileTreeNav) Title() string {
	return tn.title
}

func (tn *FileTreeNav) Icon() *icons.SvgIcon {
	return explorerIcon
}

func (tn *FileTreeNav) LayoutHeader(gtx C, th *theme.Theme) D {
	if tn.historyBtn.Clicked(gtx) {
		tn.historyProjects.Show()
	}

	title := i18n.Translate("No project")
	if tn.tree != nil {
		root := filepath.Base(tn.tree.Root())
		if root != "" {
			title = root
		}
	}

	title = strings.ToUpper(title)

	return tn.historyProjects.Layout(gtx, th,
		func(gtx C) D {
			return tn.historyBtn.Layout(gtx, func(gtx C) D {
				macro := op.Record(gtx.Ops)
				dims := layout.Inset{
					Top:    unit.Dp(2),
					Bottom: unit.Dp(2),
					Left:   unit.Dp(4),
					Right:  unit.Dp(4),
				}.Layout(gtx, func(gtx C) D {
					return material.Subtitle2(th.Theme, title).Layout(gtx)
				})
				callOp := macro.Stop()

				defer clip.UniformRRect(
					image.Rectangle{
						Max: dims.Size,
					},
					gtx.Dp(unit.Dp(4)),
				).Push(gtx.Ops).Pop()

				if tn.historyBtn.Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
					paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)

				}

				callOp.Add(gtx.Ops)

				return dims
			})
		},
	)

}

func (tn *FileTreeNav) Update(gtx C) bool {
	updated := tn.newRoot != ""
	if tn.newRoot != "" {
		tn.switchRoot()
	}

	tn.newRoot = ""
	return updated
}

func (tn *FileTreeNav) Layout(gtx C, th *theme.Theme) D {
	tn.Update(gtx)

	if tn.tree == nil {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
			lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("No open project."))
			lb.Font.Style = font.Italic
			lb.Color = misc.WithAlpha(th.Fg, 0xb6)
			return lb.Layout(gtx)
		})
	}

	return tn.tree.Layout(gtx, th)
}

// onFileUpdated close opened view, and then re-open the updated file.
func (tn *FileTreeNav) onFileUpdated(node *filetree.FileNode, oldPath string) {
	if node.IsDir() {
		return
	}

	views := tn.vm.OpenedViews()
	for idx, vw := range views {
		location := vw.Location()
		switch vw.ID() {
		case editors.GenericTextEditorViewID, editors.TypstEditorViewID, viewer.ImgViewerViewID:
			filePath := location.Query().Get("path")
			if filePath == oldPath {
				tn.vm.CloseTab(idx)
			}
		}
	}

	tn.vm.RequestSwitch(onFileSelected(node))
}

func (tn *FileTreeNav) onFileSelected(node *filetree.FileNode) {
	if node == nil {
		return
	}

	exists, isDir := utils.CheckFileExists(node.Path)
	if !exists || isDir {
		return
	}

	intent := onFileSelected(node)
	// An empty also refresh the UI so do not drop it.
	if err := tn.vm.RequestSwitch(intent); err != nil {
		log.Printf("switching to view %s error: %v", intent.Target, err)
		tn.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: err.Error(), Level: 1})
	}
}

func (tn *FileTreeNav) onFileDeleted(node *filetree.FileNode) {
	rootDir := tn.tree.Root()

	go func() {
		destPath := filepath.Clean(node.Path)
		relPath, err := filepath.Rel(rootDir, destPath)
		if err == nil {
			destPath = relPath
		}

		caller := dialog.NewDialogChooser[bool](tn.vm)
		// It's a blocking call, should call it on a separated goroutine.
		result, err := caller.Call(dialog.DeleteFileDialogViewID, map[string]any{"destination": destPath})
		if err != nil {
			log.Println("delete file error: ", err)
		}

		if result.Params {
			tn.tree.Remove(node)
		}
	}()
}

func (tn *FileTreeNav) nodeMarker(nodePath string) *filetree.NodeMarker {
	// First check if it's managed bibliography file.
	settings := tn.srv.Workspace().LoadWorkspaceSettings()
	if len(settings.BibFiles) > 0 {
		relPath, err := filepath.Rel(tn.tree.Root(), nodePath)
		if err != nil {
			log.Println("get relative path error: ", err)
			return nil
		}

		idx := slices.IndexFunc(settings.BibFiles, func(bib service.ManagedBibliography) bool {
			return bib.File == relPath
		})
		if idx >= 0 {
			return &filetree.NodeMarker{
				Kind:  "bib",
				Color: func(baseColor color.NRGBA) color.NRGBA { return utils.DisableColor(baseColor) },
				Meta: map[string]any{
					"meta": settings.BibFiles[idx],
				},
			}
		}
	}

	// Then check if its git managed and has changes made.

	// return &filetree.NodeMarker{
	// 	Kind:  "git",
	// 	Color: func(th *theme.Theme) color.NRGBA { return misc.WithAlpha(th.ContrastBg, 0x60) },
	// }

	// It's just regular node, do not set a marker.
	return nil
}

func (tn *FileTreeNav) extraMenuOptions(node *filetree.FileNode) [][]menu.MenuOption {
	isPackage := isPackageProject(tn.tree.Root())
	isTpixLoggedIn := tn.srv.Settings().Tpix().LoginAt > 0
	publishPackageOpt := menu.MenuOption{
		OnClicked: func(gtx layout.Context) error {
			if !isPackage || !isTpixLoggedIn {
				return nil
			}

			// open the publish dialog
			tn.vm.RequestSwitch(view.Intent{
				Target:      dialog.PublishPkgDialogViewID,
				ShowAsModal: true,
				Params:      map[string]any{"projectDir": tn.tree.Root()},
			})

			return nil
		},

		Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
			name := i18n.Translate("Publish Package")
			label := material.Label(th.Theme, th.TextSize, name)
			if !isPackage || !isTpixLoggedIn {
				label.Color = utils.DisableColor(th.Fg)
			}
			return label.Layout(gtx)
		},
	}

	syncDependenciesOpt := menu.MenuOption{
		OnClicked: func(gtx layout.Context) error {
			if !isTpixLoggedIn {
				return nil
			}

			go func() {
				err := tn.srv.PkgService().PullDependencies(tn.tree.Root())
				if err != nil {
					tn.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("pull dependencies error: %s", err.Error()), Level: 2})
					return
				}
				tn.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("pull dependencies succeeded!")})

			}()
			return nil
		},

		Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
			name := i18n.Translate("Sync Dependencies")
			label := material.Label(th.Theme, th.TextSize, name)
			if !isTpixLoggedIn {
				label.Color = utils.DisableColor(th.Fg)
			}
			return label.Layout(gtx)
		},
	}

	syncBibOpt := menu.MenuOption{
		OnClicked: func(gtx layout.Context) error {
			if !isTpixLoggedIn {
				return nil
			}

			relPath, _ := filepath.Rel(tn.tree.Root(), node.Path)
			// open the publish dialog
			tn.vm.RequestSwitch(view.Intent{
				Target:      dialog.SyncBibDialogViewID,
				ShowAsModal: true,
				Params:      map[string]any{"parentDir": relPath},
			})

			return nil
		},

		Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
			name := i18n.Translate("Sync Bibliographies")
			label := material.Label(th.Theme, th.TextSize, name)
			if !isTpixLoggedIn {
				label.Color = utils.DisableColor(th.Fg)
			}
			return label.Layout(gtx)
		},
	}

	bibInfoOpt := menu.MenuOption{
		OnClicked: func(gtx layout.Context) error {
			if !isTpixLoggedIn {
				return nil
			}

			state := tn.tree.GetState(node.Path)
			if state.Marker == nil || state.Marker.Kind != "bib" {
				return nil
			}

			meta := state.Marker.Meta["meta"]

			// open the info dialog
			tn.vm.RequestSwitch(view.Intent{
				Target:      dialog.ViewBibInfoDialogViewID,
				ShowAsModal: true,
				Params:      map[string]any{"meta": meta},
			})

			return nil
		},

		Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
			name := i18n.Translate("View Bibliography Info")
			label := material.Label(th.Theme, th.TextSize, name)
			if !isTpixLoggedIn {
				label.Color = utils.DisableColor(th.Fg)
			}
			return label.Layout(gtx)
		},
	}

	options := [][]menu.MenuOption{}
	if tn.tree.Root() == node.Path {
		options = append(options, []menu.MenuOption{publishPackageOpt, syncDependenciesOpt})
	}

	if node.IsDir() {
		options = append(options, []menu.MenuOption{syncBibOpt})
	} else {
		state := tn.tree.GetState(node.Path)
		if state.Marker != nil && state.Marker.Kind == "bib" {
			options = append(options, []menu.MenuOption{bibInfoOpt})
		}
	}

	return options
}

func isPackageProject(projectDir string) bool {
	manifestPath := filepath.Join(projectDir, "typst.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return false
	}

	return true

}

func onDropConfirmFunc(vm view.ViewManager, rootDir string) filetree.OnDropConfirmFunc {
	return func(srcPath string, dest *filetree.FileNode, onConfirm func()) {
		go func() {
			caller := dialog.NewDialogChooser[bool](vm)
			srcPath = filepath.Clean(srcPath)
			relPath, err := filepath.Rel(rootDir, srcPath)
			if err != nil {
				log.Printf("Error calculating relative path: %v\n", err)
			} else {
				srcPath = relPath
			}

			result, err := caller.Call(dialog.DndDropFileDialogViewID, map[string]any{"source": srcPath, "destination": dest.Name()})
			if err != nil {
				log.Println("DnD dialog error: ", err)
				return
			}

			if result.Params {
				onConfirm()
			}
		}()
	}
}

func onFileSelected(node *filetree.FileNode) view.Intent {
	if slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".PNG", ".JPG", ".JPEG", ".GIF"}, node.FileType()) {
		return view.Intent{
			Target:      viewer.ImgViewerViewID,
			ShowAsModal: false,
			RequireNew:  true,
			Params: map[string]interface{}{
				"path": node.Path,
			},
		}
	}

	if node.FileType() == ".typ" {
		return view.Intent{
			Target:      editors.TypstEditorViewID,
			ShowAsModal: false,
			RequireNew:  true,
			Params: map[string]interface{}{
				"path": node.Path,
			},
		}
	}

	// detect its MIME type to see if it's a text file.
	if utils.IsTextFile(node.Path) {
		// open as plain text
		return view.Intent{
			Target:      editors.GenericTextEditorViewID,
			ShowAsModal: false,
			RequireNew:  true,
			Params: map[string]interface{}{
				"path": node.Path,
			},
		}
	}

	return view.Intent{
		Target:      dialog.OpenWithExternalAppDialogViewID,
		ShowAsModal: true,
		Params: map[string]interface{}{
			"path": node.Path,
		},
	}

}
