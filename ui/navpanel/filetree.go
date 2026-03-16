package navpanel

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/editors"
	"looz.ws/typstify/ui/viewer"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/filetree"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	moreIcon, _       = widget.NewIcon(icons.NavigationMoreHoriz)
	createFolder, _   = widget.NewIcon(icons.FileCreateNewFolder)
	createFile, _     = widget.NewIcon(icons.ContentCreate)
)

type FileTreeNav struct {
	title string
	tree  *filetree.TreeView
	srv   *service.ServiceFacade
	vm    view.ViewManager

	rootSwitched bool
	newRoot      string
}

// Construct a FileTreeNav object that loads files and folders from rootDir. The skipFolders
// parameter allows you to specify folder name prefixes to exclude from the navigation.
func NewFileTreeNav(title string, srv *service.ServiceFacade, vm view.ViewManager) *FileTreeNav {
	ftn := &FileTreeNav{
		title: title,
		srv:   srv,
		vm:    vm,
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
	tn.title = filepath.Base(newRoot)
	tn.srv.RecentProjects().AddRecent(newRoot)

	// Restore the workplace.
	states := tn.srv.RecentProjects().Current.TreeState
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
	newTree.OnFileCreatedFunc = func(node *filetree.FileNode) {
		tn.vm.RequestSwitch(onFileSelected(node))
	}
	newTree.OnFileRemoveFunc = tn.onFileDeleted
	newTree.OnErrorFunc = func(err error) {
		log.Println("file tree error: ", err)
	}

	tn.tree = newTree

	for _, file := range tn.srv.RecentProjects().Current.OpenedFiles {
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

	tn.srv.RecentProjects().SaveSnapshot(states, openedFiles)
}

func (tn *FileTreeNav) OnClose() {
	tn.saveLastWorkplace()
	tn.tree.Close()
}

func (tn *FileTreeNav) Title() string {
	return tn.title
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
			lb := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("No open projects."))
			lb.Font.Style = font.Italic
			lb.Color = misc.WithAlpha(th.Fg, 0xb6)
			return lb.Layout(gtx)
		})
	}

	return tn.tree.Layout(gtx, th)
}

func (tn *FileTreeNav) onFileSelected(node *filetree.FileNode) {
	if node == nil {
		return
	}

	intent := onFileSelected(node)
	// An empty also refresh the UI so do not drop it.
	if err := tn.vm.RequestSwitch(intent); err != nil {
		log.Printf("switching to view %s error: %v", intent.Target, err)
	}
}

func (tn *FileTreeNav) onFileDeleted(node *filetree.FileNode) bool {
	rootDir := tn.tree.Root()
	var wg sync.WaitGroup
	var userConfirm bool

	wg.Go(func() {
		destPath := filepath.Clean(node.Path)
		relPath, err := filepath.Rel(rootDir, destPath)
		if err == nil {
			destPath = relPath
		}

		caller := dialog.NewDialogChooser[bool](tn.vm)
		result, err := caller.Call(dialog.DeleteFileDialogViewID, map[string]any{"destination": destPath})
		if err != nil {
			log.Println("delete file error: ", err)
		}

		userConfirm = result.Params
	})

	wg.Wait()
	return userConfirm
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
	if isTextFile(node) {
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

	utils.OpenInExternalApp(node.Path)
	return view.Intent{}
}

/*
func FileTreeMenuOptions(vm view.ViewManager, projectDir string) explorer.MenuOptionFunc {
	rootDir := filepath.Clean(projectDir)

	return func(gtx C, item *explorer.EntryNavItem) [][]menu.MenuOption {
		// copy & paste files or folders
		revealInExplorerOpt := menu.MenuOption{
			OnClicked: func() error {
				openInFsExplorer(item)
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				name := i18n.Translate("Open File Location")
				if item.IsDir() {
					name = i18n.Translate("Open Folder Location")
				}

				return material.Label(th.Theme, th.TextSize, name).Layout(gtx)
			},
		}

		// copy & paste files or folders
		copyOpt := menu.MenuOption{
			OnClicked: func() error {
				item.OnCopyOrCut(gtx, false)
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Copy").Layout(gtx)
			},
		}

		copyPathOpt := menu.MenuOption{
			OnClicked: func() error {
				gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(item.Path()))})
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Copy Path").Layout(gtx)
			},
		}

		copyRelativePathOpt := menu.MenuOption{
			OnClicked: func() error {
				relPath, err := filepath.Rel(projectDir, item.Path())
				if err != nil {
					return err
				}
				gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(relPath))})
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Copy Relative Path").Layout(gtx)
			},
		}

		cutOpt := menu.MenuOption{
			OnClicked: func() error {
				item.OnCopyOrCut(gtx, true)
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Cut").Layout(gtx)
			},
		}

		pasteOpt := menu.MenuOption{
			OnClicked: func() error {
				gtx.Execute(clipboard.ReadCmd{Tag: item})
				gtx.Execute(op.InvalidateCmd{})

				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Paste").Layout(gtx)
			},
		}

		renameOpt := menu.MenuOption{
			OnClicked: func() error {
				item.StartEditing(gtx)
				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Rename").Layout(gtx)
			},
		}

		deleteOpt := menu.MenuOption{
			OnClicked: func() error {
				go func() {
					destPath := filepath.Clean(item.Path())
					relPath, err := filepath.Rel(rootDir, destPath)
					if err == nil {
						destPath = relPath
					}

					caller := dialog.NewDialogChooser[bool](vm)
					result, err := caller.Call(dialog.DeleteFileDialogViewID, map[string]any{"destination": destPath})
					if err != nil {
						log.Println("delete file error: ", err)
					}

					if result.Params {
						if err := item.Remove(); err != nil {
							log.Println("delete file error: ", err)
						}
					}
				}()

				return nil
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "Delete").Layout(gtx)
			},
		}

		// create new file in current folder
		newFileOpt := menu.MenuOption{
			OnClicked: func() error {
				err := item.CreateChild(gtx, explorer.FileNode, func(node *explorer.EntryNode) {
					vm.RequestSwitch(onFileSelected(node))
				})
				if err != nil {
					log.Println("create file failed: ", err)
				}

				return err
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "New File").Layout(gtx)
			},
		}

		// create subfolder
		newFolderOpt := menu.MenuOption{
			OnClicked: func() error {
				err := item.CreateChild(gtx, explorer.FolderNode, nil)
				if err != nil {
					log.Println("create folder failed: ", err)
				}

				return err
			},

			Layout: func(gtx C, th *theme.Theme) D {
				return material.Label(th.Theme, th.TextSize, "New Folder").Layout(gtx)
			},
		}

		// root node options
		if item.Parent() == nil {
			return [][]menu.MenuOption{
				{newFileOpt, newFolderOpt},
				{revealInExplorerOpt, copyPathOpt, copyRelativePathOpt, pasteOpt},
			}
		}

		common := [][]menu.MenuOption{
			{copyOpt, cutOpt, pasteOpt},
			{revealInExplorerOpt, copyPathOpt, copyRelativePathOpt, renameOpt, deleteOpt},
		}

		if item.Kind() == explorer.FolderNode {
			// create subfolder, files, remove files, rename files
			dirOptions := []menu.MenuOption{newFileOpt, newFolderOpt}

			dirOptions = append(dirOptions, common[0]...)
			common[0] = dirOptions
		}

		return common
	}
}
*/

// ported from https://cs.opensource.google/go/x/tools/+/refs/tags/v0.26.0:godoc/util/util.go;l=69
func isTextFile(node *filetree.FileNode) bool {
	if lexer := lexers.Match(node.Path); lexer != nil {
		return true
	}

	// the extension is not known; read an initial chunk
	// of the file and check if it looks like text
	f, err := os.Open(node.Path)
	if err != nil {
		return false
	}
	defer f.Close()

	var buf [1024]byte
	n, err := f.Read(buf[0:])
	if err != nil {
		if err == io.EOF && n == 0 {
			return true
		}
		return false
	}

	// return IsText(buf[0:n])

	//  reports whether a significant prefix of buf looks like correct UTF-8;
	// that is, if it is likely that s is human-readable text.
	for i, c := range string(buf[0:n]) {
		if i+utf8.UTFMax > len(buf) {
			// last char may be incomplete - ignore
			break
		}
		if c == 0xFFFD || c < ' ' && c != '\n' && c != '\t' && c != '\f' {
			// decoding error or control character - not a text file
			return false
		}
	}
	return true
}
