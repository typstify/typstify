package filetree

import (
	"errors"
	"image"
	"io"
	"log"
	"slices"
	"strings"

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
	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gv "github.com/oligo/gioview/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/utils"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	IconSize    = unit.Dp(14)
	FileIcon, _ = widget.NewIcon(icons.ActionDescription)
)

type OnDropConfirmFunc func(srcPath string, dest *FileNode, onConfirmed func())

// TreeView is the view controller of file nodes.
type TreeView struct {
	root *FileNode

	// states maps a file path to its persistent UI state.
	states map[string]*NodeState

	// The visible, flattened list updated only when expansion changes.
	visibleNodes []FlatNode

	// View components managed by the controller
	list widget.List

	// The selected node
	selectedNode       *FileNode
	selectedNodeCutted bool

	// node currently being dropped to
	currentDropTarget *FileNode

	// Global context menu state
	contextMenu *menu.ContextMenu

	menuPos       image.Point
	isMenuVisible bool

	pendingRebuild bool
	pointerEntered bool
	dndInited      bool

	OnDropConfirmFunc
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

func (t *TreeView) Layout(gtx C, th *theme.Theme) D {
	t.update(gtx)

	if t.pendingRebuild {
		t.Rebuild()
		t.pendingRebuild = false
	}

	// drop target
	dropTarget := t.currentDropTarget

	macro := op.Record(gtx.Ops)
	dims := layout.Flex{
		Axis:      layout.Vertical,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return t.layout(gtx, th, dropTarget)
		}),
		layout.Flexed(1, func(gtx C) D {
			// setup an clip area for context menu and key, pointer events.
			//defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
			//event.Op(gtx.Ops, t)

			// top level menu.
			// if n.menu != nil {
			// 	n.menu.Layout(gtx, th)
			// }

			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)

	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	// draw a highlighted background for the potential drop target.
	if t.root == dropTarget {
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	event.Op(gtx.Ops, t)
	callOp.Add(gtx.Ops)

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

		// Highlight nodes if the target is a folder, or its parent folder.
		highlightRow := dropTarget == flatNode.Node || (dropTarget == flatNode.Node.Parent && dropTarget != t.root)

		// Render the actual row
		return t.layoutRow(gtx, th, flatNode, state, highlightRow)
	})
}

func (t *TreeView) layoutRow(gtx layout.Context, th *theme.Theme, flatNode FlatNode, state *NodeState, isDroppable bool) layout.Dimensions {
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
	if isDroppable {
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, th.HoverAlpha)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	callOp.Add(gtx.Ops)

	return dims
}

func (t *TreeView) update(gtx layout.Context) {
	t.processKeyEvents(gtx)
}

func (t *TreeView) processKeyEvents(gtx C) error {
	filters := []event.Filter{
		key.Filter{Focus: t, Name: "C", Required: key.ModShortcut},
		key.Filter{Focus: t, Name: "V", Required: key.ModShortcut},
		key.Filter{Focus: t, Name: "X", Required: key.ModShortcut},
		transfer.TargetFilter{Target: t, Type: mimeText},
		transfer.TargetFilter{Target: t, Type: mimeDnd},
		// Detect if pointer is inside of the dir item, so we can highlight it when dropping items to it.
		pointer.Filter{Target: t, Kinds: pointer.Enter | pointer.Leave},
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
				gtx.Execute(clipboard.ReadCmd{Tag: t})

			// Copy or Cut selection -- ignored if nothing selected.
			case "C", "X":
				log.Println("copy|cut", t.selectedNode.Path)
				t.OnCopyOrCut(gtx, t.selectedNode)
				if event.Name == "X" {
					t.selectedNodeCutted = true
				}
			}

		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				t.pointerEntered = true
			case pointer.Leave:
				t.pointerEntered = false
			}

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
			case mimeText:
				isCut := t.selectedNodeCutted == true
				if err := t.OnPaste(string(content), isCut, t.selectedNode); err != nil {
					return err
				}
			case mimeDnd:
				log.Println("root node received DnD drop! source: ", string(content))
				t.OnDropped(t.root, string(content))
			}
		}
	}

	return nil
}

// Create file or subfolder under the current folder.
func (t *TreeView) CreateChild(gtx C, kind explorer.NodeKind) error {
	if t.selectedNode == nil || !t.selectedNode.IsDir() {
		return nil
	}

	var err error
	if kind == explorer.FileNode {
		err = t.selectedNode.AddChild("new file", explorer.FileNode)
	} else {
		err = t.selectedNode.AddChild("new folder", explorer.FolderNode)
	}

	if err != nil {
		return err
	}

	childNode := t.selectedNode.Children()[0]

	childNodeState := t.GetState(childNode.Path)

	childNodeState.Editable = gv.EditableLabel(childNode.Name(), func(text string) {
		err := childNode.UpdateName(text)
		if err != nil {
			log.Println("update name err: ", err)
		}
		// Trigger a rebuild
		t.pendingRebuild = true
	})

	// focus the child input
	childNodeState.Editable.SetEditing(true)

	return nil
}

func (t *TreeView) Remove() error {
	if t.selectedNode == nil || !t.selectedNode.IsDir() {
		return nil
	}

	err := t.selectedNode.Delete()
	if err != nil {
		return err
	}

	return nil
}

// Move file to the current dir or the dir of the current file. Set removeOld to false to
// simulate a copy OP.
func (t *TreeView) OnPaste(data string, removeOld bool, dest *FileNode) error {
	// when paste destination is a normal file node, use its parent dir to ease the CUT/COPY operations.
	if dest == nil {
		return errors.New("no target node is selected")
	}

	if !dest.IsDir() && dest.Path != t.root.Path {
		dest = dest.Parent
	}

	pathes := strings.Split(string(data), "\n")
	if removeOld {
		for _, p := range pathes {
			err := dest.Move(p)
			if err != nil {
				return err
			}

			src := t.findVisibleNode(data)

			if src != nil && src.Node.Parent != nil {
				t.selectedNodeCutted = false // TODO: does not scale when multiple nodes selected
				// trigger a rebuild
				t.pendingRebuild = true
			}
		}
	} else {
		for _, p := range pathes {
			err := dest.Copy(p)
			if err != nil {
				return err
			}
			t.pendingRebuild = true
		}
	}

	return nil
}

func (t *TreeView) OnCopyOrCut(gtx C, srcNode *FileNode) error {
	if srcNode == nil {
		return errors.New("no source node is selected")
	}

	gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(strings.NewReader(srcNode.Path))})
	return nil
}

func (t *TreeView) OnSelect(fileNode *FileNode) {
	if t.selectedNode != nil {
		prevState := t.GetState(t.selectedNode.Path)
		prevState.Label.Unselect()
	}

	t.selectedNode = fileNode
	state := t.GetState(fileNode.Path)
	state.Expanded = fileNode.IsDir() && !state.Expanded

	if fileNode.IsDir() {
		t.pendingRebuild = true
	}
}

func (t *TreeView) UpdateDropTarget(destNode *FileNode) {
	if destNode == nil {
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
	srcNode := t.findVisibleNode(sourcePath)

	if sourcePath == destNode.Path || srcNode.Node.Parent.Path == destNode.Path {
		return
	}

	if t.OnDropConfirmFunc != nil {
		t.OnDropConfirmFunc(sourcePath, destNode, func() {
			t.OnPaste(sourcePath, true, destNode)
		})
	} else {
		t.OnPaste(sourcePath, true, destNode)
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
