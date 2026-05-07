package filetree

import (
	"image/color"

	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/oligo/gioview/explorer"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"
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
	Marker    *NodeMarker
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
	Icon            *icons.SvgIcon
	VerticalPadding unit.Dp
	IndentUnit      unit.Dp
}

type TreeState struct {
	Path          string
	ExpandedNodes []string
}

// NodeMarker decorate the file node and also can change
// its behavior based on its kind and meta data.
type NodeMarker struct {
	Kind  string
	Color func(baseColor color.NRGBA) color.NRGBA
	Meta  map[string]any
}

func isAncestor(ancestor, childNode *FileNode) bool {
	if ancestor == nil || childNode == nil {
		return false
	}

	for parent := childNode.Parent; parent != nil; parent = parent.Parent {
		if parent == ancestor {
			return true
		}
	}
	return false
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
