package filetree

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/fonts"
	"looz.ws/typstify/widgets"
)

func testTheme() *theme.Theme {
	return theme.NewTheme("", [][]byte{fonts.HackRegular}, true)
}

func testNode(t *testing.T, name string) *FileNode {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("= Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	return &FileNode{Path: path, FileInfo: info}
}

func testContext(width int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(width, 400)},
		Now:         time.Now(),
	}
}

// layoutNode lays out a node row and returns the resulting dimensions together
// with the editable label state the row ended up with.
func layoutNode(t *testing.T, th *theme.Theme, gtx layout.Context, name string, marker *NodeMarker) (image.Point, *widgets.Editable) {
	t.Helper()

	editable := widgets.EditableLabel(name)
	fn := &FlatNode{
		Node: testNode(t, name),
		State: &NodeState{
			Editable: editable,
			Marker:   marker,
		},
		IndentUnit: unit.Dp(12),
	}

	gtx.Ops = new(op.Ops)

	return fn.Layout(gtx, th).Size, editable
}

func TestNodeRowSpansFullWidth(t *testing.T) {
	th := testTheme()

	// The name must fill the row, otherwise the hover highlight, the click area
	// and the inline rename editor would shrink to the text width.
	plain, _ := layoutNode(t, th, testContext(300), "chapter.typ", nil)
	if plain.X != 300 {
		t.Fatalf("expected a row spanning 300px, got %d", plain.X)
	}

	marked, _ := layoutNode(t, th, testContext(300), "chapter.typ", &NodeMarker{Emphasize: true})
	if marked.X != 300 {
		t.Fatalf("expected a marked row spanning 300px, got %d", marked.X)
	}

	if marked.Y != plain.Y {
		t.Fatalf("marker changed the row height: %d without marker, %d with marker", plain.Y, marked.Y)
	}
}

func TestNodeMarkerEmphasizesName(t *testing.T) {
	th := testTheme()

	_, plain := layoutNode(t, th, testContext(300), "chapter.typ", nil)
	if plain.Bold {
		t.Fatal("expected the name of an unmarked node to be regular")
	}

	markerColor := color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}
	_, marked := layoutNode(t, th, testContext(300), "chapter.typ", &NodeMarker{
		Emphasize: true,
		Color:     func(color.NRGBA) color.NRGBA { return markerColor },
	})
	if !marked.Bold {
		t.Fatal("expected the name of a marked node to be bold")
	}
	if marked.Color != markerColor {
		t.Fatalf("expected the marker color %v, got %v", markerColor, marked.Color)
	}
}

// A marker can disappear at runtime, e.g. when the entry file is unpinned. The
// emphasis must not stick to the node in that case.
func TestNodeMarkerEmphasisIsNotSticky(t *testing.T) {
	th := testTheme()

	editable := widgets.EditableLabel("chapter.typ")
	fn := &FlatNode{
		Node: testNode(t, "chapter.typ"),
		State: &NodeState{
			Editable: editable,
			Marker:   &NodeMarker{Emphasize: true},
		},
		IndentUnit: unit.Dp(12),
	}

	gtx := testContext(300)
	fn.Layout(gtx, th)
	if !editable.Bold {
		t.Fatal("expected the name to be bold while the node is marked")
	}

	fn.State.Marker = nil
	gtx.Ops = new(op.Ops)
	fn.Layout(gtx, th)
	if editable.Bold {
		t.Fatal("expected the emphasis to be dropped once the marker is gone")
	}
}
