package filetree

import (
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets/menu"
)

func FileTreeMenuOptions(vm view.ViewManager, tree *TreeView) MenuOptionFunc {
	rootDir := filepath.Clean(tree.root.Path)

	return func(node *FileNode) [][]menu.MenuOption {
		// copy & paste files or folders
		revealInExplorerOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				openInFsExplorer(node)
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				name := i18n.Translate("Open File Location")
				if node.IsDir() {
					name = i18n.Translate("Open Folder Location")
				}

				return material.Label(th.Theme, th.TextSize, name).Layout(gtx)
			},
		}

		// copy & paste files or folders
		copyOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				tree.OnCopyOrCut(gtx, node, false)
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Copy").Layout(gtx)
			},
		}

		copyPathOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(node.Path))})
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Copy Path").Layout(gtx)
			},
		}

		copyRelativePathOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				relPath, err := filepath.Rel(rootDir, node.Path)
				if err != nil {
					return err
				}
				gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(relPath))})
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Copy Relative Path").Layout(gtx)
			},
		}

		cutOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				tree.OnCopyOrCut(gtx, node, true)
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Cut").Layout(gtx)
			},
		}

		pasteOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				tree.OnPasteByContextMenu(gtx)
				gtx.Execute(op.InvalidateCmd{})

				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Paste").Layout(gtx)
			},
		}

		renameOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				tree.StartEditing(gtx, node)
				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Rename").Layout(gtx)
			},
		}

		deleteOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				// go func() {
				// 	destPath := filepath.Clean(node.Path)
				// 	relPath, err := filepath.Rel(rootDir, destPath)
				// 	if err == nil {
				// 		destPath = relPath
				// 	}

				// 	caller := dialog.NewDialogChooser[bool](vm)
				// 	result, err := caller.Call(dialog.DeleteFileDialogViewID, map[string]any{"destination": destPath})
				// 	if err != nil {
				// 		log.Println("delete file error: ", err)
				// 	}

				// 	if result.Params {
				// 		tree.Remove(node); err != nil {
				// 			log.Println("delete file error: ", err)
				// 		}
				// 	}
				// }()
				tree.Remove(node)

				return nil
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "Delete").Layout(gtx)
			},
		}

		// create new file in current folder
		newFileOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				err := tree.CreateChild(gtx, node, explorer.FileNode)
				if err != nil {
					log.Println("create file failed: ", err)
				}

				return err
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "New File").Layout(gtx)
			},
		}

		// create subfolder
		newFolderOpt := menu.MenuOption{
			OnClicked: func(gtx layout.Context) error {
				err := tree.CreateChild(gtx, node, explorer.FolderNode)
				if err != nil {
					log.Println("create folder failed: ", err)
				}

				return err
			},

			Layout: func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return material.Label(th.Theme, th.TextSize, "New Folder").Layout(gtx)
			},
		}

		// root node options
		if node.Path == rootDir {
			return [][]menu.MenuOption{
				{newFileOpt, newFolderOpt},
				{revealInExplorerOpt, copyPathOpt, copyRelativePathOpt, pasteOpt},
			}
		}

		if node.IsDir() {
			// more options to create subfolder, files, remove files, rename files
			return [][]menu.MenuOption{
				{newFileOpt, newFolderOpt},
				{copyOpt, cutOpt, pasteOpt},
				{revealInExplorerOpt, copyPathOpt, copyRelativePathOpt, renameOpt, deleteOpt},
			}
		}

		// Menu options for file node
		return [][]menu.MenuOption{
			{copyOpt, cutOpt},
			{revealInExplorerOpt, copyPathOpt, copyRelativePathOpt, renameOpt, deleteOpt},
		}
	}
}

// open a file or folder in the file manager and have it selected.
func openInFsExplorer(node *FileNode) error {
	switch runtime.GOOS {
	case "darwin", "ios":
		return runCmd("open", "-R", node.Path)
	case "windows":
		return runCmd("explorer", "/select,"+node.Path)
	default:
		// linux, unix flavors.
		path := node.Path
		if !node.IsDir() {
			path = filepath.Dir(path)
		}
		return runCmd("xdg-open", path)
	}
}

func runCmd(cmdName string, arg ...string) error {
	cmd := exec.Command(cmdName, arg...)
	return cmd.Run()
}
