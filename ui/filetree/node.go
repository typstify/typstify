package filetree

import (
	"strings"

	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/oligo/gioview/explorer"
	gv "github.com/oligo/gioview/widget"
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
	Editable  *gv.Editable
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

// Implelments io.ReadCloser for widget.Draggable.
type fileNodeReader struct {
	pathReader strings.Reader
}

func newFileNodeReader(node *FileNode) *fileNodeReader {
	return &fileNodeReader{pathReader: *strings.NewReader(node.Path)}
}

func (f *fileNodeReader) Read(p []byte) (n int, err error) {
	return f.pathReader.Read(p)
}

func (f *fileNodeReader) Close() error {
	return nil
}
