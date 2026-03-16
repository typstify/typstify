package filetree

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"slices"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
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
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/menu"
)

var (
	IconSize          = unit.Dp(14)
	FileIcon, _       = widget.NewIcon(icons.ActionDescription)
	FolderIcon, _     = widget.NewIcon(icons.NavigationChevronRight)
	FolderOpenIcon, _ = widget.NewIcon(icons.NavigationExpandMore)
)

type OnDropConfirmFunc func(srcPath string, dest *FileNode, onConfirmed func())
type MenuOptionFunc func(node *FileNode) [][]menu.MenuOption

// TreeView is the view controller of file nodes.
type TreeView struct {
	root *FileNode
	vm   view.ViewManager

	// states maps a file path to its persistent UI state.
	states map[string]*NodeState

	// The visible, flattened list updated only when expansion changes.
	visibleNodes []FlatNode

	// View components managed by the controller
	list widget.List

	// The selected node which is determined by a left-click.
	// Keyboard shortcuts operates on selected node.
	selectedNode *FileNode
	// The context node which is determined by a right-click.
	// Context menu operates on context node.
	contextNode *FileNode
	// Global context menu state
	contextMenu    *menu.ContextMenu
	contextMenuPos f32.Point

	// node currently being dropped to
	currentDropTarget *FileNode

	pendingRebuild bool
	pointerEntered bool
	dndInited      bool

	OnFileCreatedFunc func(node *FileNode)
	OnFileRemoveFunc  func(node *FileNode) bool
	OnDropConfirmFunc OnDropConfirmFunc
	OnErrorFunc       func(err error)
}

type TreeState struct {
	Path     string
	Children []NodeState
}

func NewTreeView(rootNode *FileNode) *TreeView {
	return &TreeView{
		root:           rootNode,
		states:         make(map[string]*NodeState),
		visibleNodes:   make([]FlatNode, 0),
		pendingRebuild: true,
		contextMenu:    menu.NewContextMenu(),
	}
}

// GetState retrieves or initializes the UI state for a specific path.
func (t *TreeView) GetState(path string) *NodeState {
	if state, exists := t.states[path]; exists {
		return state
	}
	newState := &NodeState{}
	t.states[path] = newState
	return newState
}

func (t *TreeView) deleteState(path string) {
	delete(t.states, path)
}

// Rebuild flattens the tree. Call this ONLY when a node expands/collapses,
// not on every single frame.
func (t *TreeView) Rebuild() {
	t.visibleNodes = t.visibleNodes[:0]
	t.flatten(t.root, 0)
}

func (t *TreeView) flatten(node *FileNode, depth int) {
	state := t.GetState(node.Path)

	if node != t.root {
		flatNode := FlatNode{
			Node:            node,
			Depth:           depth,
			VerticalPadding: unit.Dp(3),
			IndentUnit:      unit.Dp(16),
		}
		if !node.IsDir() {
			flatNode.Icon = FileIcon
		}
		flatNode.State = state
		t.visibleNodes = append(t.visibleNodes, flatNode)
	}

	if node == t.root || (state.Expanded && node.IsDir()) {
		for _, child := range node.Children() {
			t.flatten(child, depth+1)
		}
	}
}

func (t *TreeView) droppable() bool {
	return t.pointerEntered && t.dndInited
}

func (t *TreeView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	t.update(gtx)

	if t.pendingRebuild {
		t.Rebuild()
		t.pendingRebuild = false
	}

	// drop target
	dropTarget := t.currentDropTarget

	macro := op.Record(gtx.Ops)
	dims := func(gtx layout.Context) layout.Dimensions {
		if t.root == t.contextNode {
			return widget.Border{
				Color:        th.ContrastBg,
				CornerRadius: 0,
				Width:        unit.Dp(1),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return t.layout(gtx, th, dropTarget)
			})
		}

		return t.layout(gtx, th, dropTarget)
	}(gtx)
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	// draw a highlighted background for the potential drop target.
	if t.root == dropTarget {
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	event.Op(gtx.Ops, t)
	callOp.Add(gtx.Ops)

	if t.contextMenu != nil {
		t.contextMenu.Layout(gtx, th)
	}

	return dims
}

func (t *TreeView) layout(gtx layout.Context, th *theme.Theme, dropTarget *FileNode) layout.Dimensions {
	t.list.Axis = layout.Vertical
	list := material.List(th.Theme, &t.list)
	list.AnchorStrategy = material.Overlay
	list.ScrollbarStyle = utils.MakeScrollbar(th.Theme, list.Scrollbar, misc.WithAlpha(th.Fg, 0x30))

	return list.Layout(gtx, len(t.visibleNodes), func(gtx layout.Context, index int) layout.Dimensions {
		flatNode := t.visibleNodes[index]
		state := t.GetState(flatNode.Node.Path)

		// skip root as root needs to paint the entire wiget with the background color.
		highlightRow := dropTarget != t.root && shouldHighlight(dropTarget, flatNode.Node)

		// Render the actual row
		return t.layoutRow(gtx, th, flatNode, state, highlightRow)
	})
}

func (t *TreeView) layoutRow(gtx layout.Context, th *theme.Theme, flatNode FlatNode, state *NodeState, highlight bool) layout.Dimensions {
	// Process click of the node.
	if state.Label.Update(gtx) {
		t.OnSelect(flatNode.Node)
		gtx.Execute(op.InvalidateCmd{})
	}

	macro := op.Record(gtx.Ops)
	dims := flatNode.Layout(gtx, th, th.Fg, t)
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	if state.Cutted {
		defer paint.PushOpacity(gtx.Ops, 0.6).Pop()
	}

	// draw a highlighted background for the potential drop target.
	if highlight {
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	callOp.Add(gtx.Ops)

	return dims
}

func (t *TreeView) update(gtx layout.Context) {
	// Lifting the event processing of context menu first, so dismiss event can be handled first
	// and won't overwrite t.contextMenu.Show in OnContextNodeChange.
	t.contextMenu.Update(gtx)

	err := t.processKeyEvents(gtx)
	if t.OnErrorFunc != nil {
		log.Println("filetree error: ", err)
		t.OnErrorFunc(err)
	}
}

func (t *TreeView) processKeyEvents(gtx layout.Context) error {
	filters := []event.Filter{
		key.FocusFilter{Target: t},
		key.Filter{Focus: t, Name: "C", Required: key.ModShortcut},
		key.Filter{Focus: t, Name: "V", Required: key.ModShortcut},
		key.Filter{Focus: t, Name: "X", Required: key.ModShortcut},
		transfer.TargetFilter{Target: t, Type: mimeText},
		transfer.TargetFilter{Target: t, Type: mimeDnd},
		// Detect if pointer is inside of the dir item, so we can highlight it when dropping items to it.
		pointer.Filter{Target: t, Kinds: pointer.Enter | pointer.Leave | pointer.Press},
	}

	for {
		ke, ok := gtx.Event(filters...)
		if !ok {
			break
		}

		switch event := ke.(type) {
		case key.Event:
			if !event.Modifiers.Contain(key.ModShortcut) {
				break
			}

			switch event.Name {
			// Initiate a paste operation, by requesting the clipboard contents; other
			// half is in DataEvent.
			case "V":
				t.onPasteByShortcut(gtx)
			// Copy or Cut selection -- ignored if nothing selected.
			case "C", "X":
				t.OnCopyOrCut(gtx, t.selectedNode, event.Name == "X")
			}

		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				t.pointerEntered = true
			case pointer.Leave:
				t.pointerEntered = false
			case pointer.Press:
				// let treeView to grab the focus, so we can do copy/cut/paste
				gtx.Execute(key.FocusCmd{Tag: t})
				// also update context node
				if event.Buttons == pointer.ButtonSecondary {
					t.contextMenuPos = event.Position
					t.OnContextNodeChange(t.root)

				} else {
					// left clicking in the empty area, will clear the context node
					// and clear the node selection.
					t.OnContextNodeChange(nil)
					// clear node selection
					t.OnSelect(nil)

				}
			}
		case key.FocusEvent:
			// no-op
		case transfer.InitiateEvent:
			t.dndInited = true
		case transfer.CancelEvent:
			t.dndInited = false
			t.pointerEntered = false

		case transfer.DataEvent:
			// read the clipboard content:
			reader := event.Open()
			defer reader.Close()
			content, err := io.ReadAll(reader)
			if err != nil {
				return err
			}

			defer gtx.Execute(op.InvalidateCmd{})

			switch event.Type {
			case mimeText: // when using MacOS, path paste is handled directly in key.Event 'cmd+V'
				paths := parseClipboardPaths(string(content))
				// Guess which kind of node we should operating on.
				targetNode := t.contextNode
				if targetNode == nil {
					targetNode = t.selectedNode
				}
				if err := t.OnPaste(paths, targetNode); err != nil {
					return err
				}
			case mimeDnd:
				t.OnDropped(t.root, string(content))
			}
		}
	}

	return nil
}

// Create file or subfolder under the specified folder.
func (t *TreeView) CreateChild(gtx layout.Context, parent *FileNode, kind explorer.NodeKind) error {
	if parent == nil || !parent.IsDir() {
		return nil
	}

	var err error
	if kind == explorer.FileNode {
		err = parent.AddChild("new file", explorer.FileNode)
	} else {
		err = parent.AddChild("new folder", explorer.FolderNode)
	}

	if err != nil {
		return err
	}

	childNode := parent.Children()[0]

	childNodeState := t.GetState(childNode.Path)

	if t.OnFileCreatedFunc != nil {
		t.OnFileCreatedFunc(childNode)
	}

	childNodeState.Editable = widgets.EditableLabel(childNode.Name(), func(text string) {
		err := childNode.UpdateName(text)
		if err != nil {
			log.Println("update name err: ", err)
			return
		}
	})

	// focus the child input
	childNodeState.Editable.SetEditing(true)

	// Expand parent folder
	nodeState := t.GetState(parent.Path)
	nodeState.Expanded = true
	// Trigger a rebuild
	t.pendingRebuild = true
	return nil
}

func (t *TreeView) Remove(node *FileNode) {
	if node == nil {
		return
	}

	removeNode := func(n *FileNode) {
		err := n.Delete()
		if err != nil {
			if t.OnErrorFunc != nil {
				t.OnErrorFunc(err)
			} else {
				log.Println("remove file/folder error: ", err)
			}
		}
		t.deleteState(n.Path)
		t.pendingRebuild = true
	}

	if t.OnFileRemoveFunc != nil {
		if t.OnFileRemoveFunc(node) {
			removeNode(node)
			return
		}
	} else {
		removeNode(node)
	}
}

// onPasteInit init the paste process by executing a clipboard.ReadCmd command,
// or read from the OS clipboard and process the file urls directly. This method
// is the keyboard shortcuts event handler.
func (t *TreeView) onPasteByShortcut(gtx layout.Context) {
	t.OnPasteByTarget(gtx, t.selectedNode)
}

// OnPasteByContextMenu works the same way like onPasteByShortcut, except that
// it works on the contextNode instead of the selected node.
func (t *TreeView) OnPasteByContextMenu(gtx layout.Context) {
	t.OnPasteByTarget(gtx, t.contextNode)
}

func (t *TreeView) OnPasteByTarget(gtx layout.Context, targetNode *FileNode) {
	paths := ReadClipboardFiles()
	if len(paths) == 0 {
		gtx.Execute(clipboard.ReadCmd{Tag: t})
		return
	}

	// else process the paste directly here
	if err := t.OnPaste(paths, targetNode); err != nil {
		log.Println("paste error: ", err)
		if t.OnErrorFunc != nil {
			t.OnErrorFunc(err)
		}
	}
}

// Move file to the current dir or the dir of the current file.
// pathStr can be space seperated multiple paths
func (t *TreeView) OnPaste(paths []string, dest *FileNode) error {
	// when paste destination is a normal file node, use its parent dir to ease the CUT/COPY operations.
	if dest == nil {
		dest = t.root
	}

	if !dest.IsDir() && dest.Path != t.root.Path {
		dest = dest.Parent
	}

	for _, p := range paths {
		if !isValidFilePath(p) {
			// no op if path is invalid
			return nil
		}
	}

	for _, p := range paths {
		nodeState := t.GetState(p)
		var opErr error
		if nodeState.Cutted {
			opErr = dest.Move(p)
			// No need to check if the path is external of the root dir.
			t.deleteState(p)
		} else {
			opErr = dest.Copy(p)
		}
		if opErr != nil {
			return opErr
		}

		// trigger a rebuild
		t.pendingRebuild = true
	}

	return nil
}

func (t *TreeView) OnCopyOrCut(gtx layout.Context, srcNode *FileNode, isCut bool) error {
	if srcNode == nil {
		return errors.New("no source node is selected")
	}

	gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(srcNode.Path))})

	if isCut {
		nodeState := t.GetState(srcNode.Path)
		nodeState.Cutted = true
	}

	return nil
}

// OnContextNodeChange is called when the node is right-clicked.
// This should be distinguished from [OnSelect] that the latter is triggered
// by left clicking and the node is highlighted with background color. And the
// former will only be highlighted with a border.
// Both treeView and flatNode will receive the same event and root is always
// the first to process the event, so we can get the right context node here.
func (t *TreeView) OnContextNodeChange(fileNode *FileNode) {
	lastContextNode := t.contextNode
	if lastContextNode != nil {
		nodeState := t.GetState(lastContextNode.Path)
		nodeState.Label.SetActivated(false)
	}

	t.contextNode = fileNode
	if fileNode != nil {
		nodeState := t.GetState(fileNode.Path)
		nodeState.Label.SetActivated(true)
	}

	// update context menu options
	if t.contextNode == nil {
		t.contextMenu.Dismiss()
	} else {
		menuOpts := t.getContextMenuOptions(t.contextNode)
		t.contextMenu.Show(t.contextMenuPos, menuOpts)
	}
}

func (t *TreeView) OnSelect(fileNode *FileNode) {
	if t.selectedNode != nil {
		prevState := t.GetState(t.selectedNode.Path)
		prevState.Label.Unselect()
	}

	t.selectedNode = fileNode
	if fileNode != nil {
		state := t.GetState(fileNode.Path)
		state.Expanded = fileNode.IsDir() && !state.Expanded

		if fileNode.IsDir() {
			t.pendingRebuild = true
		}
	}
}

func (t *TreeView) UpdateDropTarget(destNode *FileNode, isLeave bool) {
	if destNode == nil {
		return
	}

	if isLeave {
		previousTarget := t.currentDropTarget
		// enter and leave may not happen sequentially, usually it is like:
		// A-enter, B-enter, A-leave,...
		// So we have to check if we are reseting the right node.
		if previousTarget != nil && previousTarget != destNode {
			return
		}

		if t.droppable() {
			t.currentDropTarget = t.root
			return
		}
		t.currentDropTarget = nil
		return
	}

	if destNode == t.root {
		if t.droppable() {
			t.currentDropTarget = t.root
			return
		}

		t.currentDropTarget = nil
		return
	}

	destNodeState := t.GetState(destNode.Path)

	if destNodeState.Droppable() {
		if destNode.IsDir() {
			t.currentDropTarget = destNode
		} else {
			t.currentDropTarget = destNode.Parent
		}

		return
	}

	t.currentDropTarget = nil

}

func (t *TreeView) OnDropped(destNode *FileNode, sourcePath string) {
	moveNode := func(srcNodePath string, dest *FileNode) error {
		if dest == nil {
			return errors.New("no target node is selected")
		}

		if !dest.IsDir() && dest.Path != t.root.Path {
			dest = dest.Parent
		}

		err := dest.Move(srcNodePath)
		if err != nil {
			if t.OnErrorFunc != nil {
				t.OnErrorFunc(fmt.Errorf("move file error: %w", err))
			}
			return err
		}

		return nil
	}

	defer func() {
		t.currentDropTarget = nil
	}()

	srcNode := t.findVisibleNode(sourcePath)

	if sourcePath == destNode.Path || srcNode.Node.Parent.Path == destNode.Path {
		return
	}

	if t.OnDropConfirmFunc != nil {
		t.OnDropConfirmFunc(sourcePath, destNode, func() {
			err := moveNode(sourcePath, destNode)
			if err == nil {
				t.pendingRebuild = true
				t.deleteState(sourcePath)
			}
		})
	} else {
		err := moveNode(sourcePath, destNode)
		if err == nil {
			t.pendingRebuild = true
			t.deleteState(sourcePath)
		}
	}
}

func (t *TreeView) findVisibleNode(path string) *FlatNode {
	idx := slices.IndexFunc(t.visibleNodes, func(flatNode FlatNode) bool {
		return flatNode.Node.Path == path
	})

	if idx < 0 {
		return nil
	}

	return &t.visibleNodes[idx]
}

// StartEditing turn the node into a editable state to edit its name.
func (t *TreeView) StartEditing(gtx layout.Context, node *FileNode) {
	nodeState := t.GetState(node.Path)
	nodeState.Editable.SetEditing(true)
	gtx.Execute(op.InvalidateCmd{})
}

/*
// Restore restores the tree states by applying state to the current node and its children.
func (t *TreeView) Restore(state *TreeState) error {
	root, err := explorer.NewFileTree(state.Path)
	if err != nil {
		return err
	}
	t.root = root

	stateMap := make(map[string]*TreeState, len(state.Children))
	for _, st := range state.Children {
		stateMap[st.Path] = st
	}

	for _, child := range eitem.children {
		child := child.(*EntryNavItem)
		if !child.state.IsDir() {
			continue
		}

		if st, exists := stateMap[child.Path()]; exists {
			child.Restore(st)
		}
	}
}

// Snapshot saves states of the expanded [EntryNavItem] node, and the states of its children.
func (t *TreeView) Snapshot() *TreeState {
	if t.root == nil {
		return nil
	}

	state := &TreeState{Path: t.root.Path, Expanded: eitem.expanded}

	for _, child := range eitem.children {
		child := child.(*EntryNavItem)
		if !child.state.IsDir() {
			continue
		}

		if childState := child.Snapshot(); childState != nil {
			state.Children = append(state.Children, childState)
		}
	}

	return state
}
*/

func (t *TreeView) getContextMenuOptions(node *FileNode) [][]menu.MenuOption {
	if node == nil {
		return nil
	}

	menuOptionFunc := FileTreeMenuOptions(t.vm, t)
	return menuOptionFunc(node)
}
