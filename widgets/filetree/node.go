package filetree

import (
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/oligo/gioview/explorer"
	"looz.ws/typstify/widgets"
)

// FileNode is the data representation of the file system node.
type FileNode = explorer.EntryNode

// NodeState holds the UI state for each of the FileNode.
type NodeState struct {
	Expanded  bool
	Cutted    bool
	Draggable widget.Draggable
	// entered and dnsInited are for Drag and Drop op.
	Entered   bool
	DndInited bool
	Editable  *widgets.Editable
	Label     widgets.InteractiveLabel
}

func (n *NodeState) Droppable() bool {
	return n.Entered && n.DndInited && !n.Draggable.Dragging()
}

// FlatNode is a transient struct used purely for rendering.
// It bridges the Model and the Store for the View layer.
type FlatNode struct {
	Node            *FileNode
	State           *NodeState
	Depth           int
	Icon            *widget.Icon
	VerticalPadding unit.Dp
	IndentUnit      unit.Dp
}

func isAncestor(ancestor, childNode *FileNode) bool {
	if ancestor == nil || childNode == nil {
		return false
	}

	parent := childNode.Parent
	for {
		if parent == ancestor {
			return true
		}

		parent = parent.Parent
		if parent == nil { // root node in explorer.EntryNode set parent to nil
			return false
		}
	}

}

// shouldHighlight checks if currentNode should be highlighted given the DnD
// target node. targetNode is always a dir or nil.
func shouldHighlight(targetNode, currentNode *FileNode) bool {
	if targetNode == nil {
		return false
	}

	if targetNode == currentNode {
		return true
	}

	if isAncestor(targetNode, currentNode) {
		return true
	}

	return false
}
