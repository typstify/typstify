package editor

import (
	"strings"
	"testing"

	"github.com/oligo/gvcode/gutter/providers"
	"github.com/rogpeppe/go-internal/diff"
)

// diffLines joins lines with newlines and appends a trailing newline.
func diffLines(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

func verifyHunk(t *testing.T, h *providers.DiffHunk, wantType providers.DiffType, wantStart, wantEnd int, wantOld, wantNew []string) {
	t.Helper()
	if h == nil {
		t.Fatal("expected hunk, got nil")
	}
	if h.Type != wantType {
		t.Errorf("Type: got %v, want %v", h.Type, wantType)
	}
	if h.StartLine != wantStart {
		t.Errorf("StartLine: got %d, want %d", h.StartLine, wantStart)
	}
	if h.EndLine != wantEnd {
		t.Errorf("EndLine: got %d, want %d", h.EndLine, wantEnd)
	}
	if len(h.OldLines) != len(wantOld) || len(h.NewLines) != len(wantNew) {
		t.Errorf("OldLines/NewLines: got %d/%d, want %d/%d",
			len(h.OldLines), len(h.NewLines), len(wantOld), len(wantNew))
		return
	}
	for i := range h.OldLines {
		if h.OldLines[i] != wantOld[i] {
			t.Errorf("OldLines[%d]: got %q, want %q", i, h.OldLines[i], wantOld[i])
		}
	}
	for i := range h.NewLines {
		if h.NewLines[i] != wantNew[i] {
			t.Errorf("NewLines[%d]: got %q, want %q", i, h.NewLines[i], wantNew[i])
		}
	}
}

// ---------------------------------------------------------------------------
// parseDiffOutput tests
// ---------------------------------------------------------------------------

func TestParseDiffOutput_NoChanges(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := diffLines("line1", "line2", "line3")
	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks for identical content, got %d", len(hunks))
	}
}

func TestParseDiffOutput_AddSingleLine(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := diffLines("line1", "line2", "line3", "line4")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffAdded, 3, 3, nil, []string{"line4"})
}

func TestParseDiffOutput_AddMultipleLines(t *testing.T) {
	old := diffLines("line1", "line5")
	new := diffLines("line1", "line2", "line3", "line4", "line5")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffAdded, 1, 3, nil, []string{"line2", "line3", "line4"})
}

func TestParseDiffOutput_DeleteSingleLine(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := diffLines("line1", "line3")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffDeleted, 1, 1, []string{"line2"}, nil)
}

func TestParseDiffOutput_DeleteMultipleLines(t *testing.T) {
	old := diffLines("line1", "line2", "line3", "line4", "line5")
	new := diffLines("line1", "line5")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffDeleted, 1, 1,
		[]string{"line2", "line3", "line4"}, nil)
}

func TestParseDiffOutput_ModifyLine(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := diffLines("line1", "line2-modified", "line3")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffModified, 1, 1,
		[]string{"line2"}, []string{"line2-modified"})
}

func TestParseDiffOutput_MultipleHunks(t *testing.T) {
	// Two changes far apart produce separate hunks.
	oldLines := make([]string, 30)
	newLines := make([]string, 30)
	for i := range 30 {
		s := "line" + strings.Repeat("x", 10) + "-" + string(rune('A'+i%26))
		oldLines[i] = s
		newLines[i] = s
	}
	newLines[5] = "CHANGED-AT-5"
	newLines[22] = "CHANGED-AT-22"

	old := []byte(strings.Join(oldLines, "\n") + "\n")
	new := []byte(strings.Join(newLines, "\n") + "\n")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}
	if hunks[0].StartLine != 5 || hunks[0].EndLine != 5 {
		t.Errorf("hunk 0: got StartLine=%d EndLine=%d, want 5,5", hunks[0].StartLine, hunks[0].EndLine)
	}
	if hunks[1].StartLine != 22 || hunks[1].EndLine != 22 {
		t.Errorf("hunk 1: got StartLine=%d EndLine=%d, want 22,22", hunks[1].StartLine, hunks[1].EndLine)
	}
}

func TestParseDiffOutput_StartOfFile(t *testing.T) {
	old := diffLines("line1", "line2")
	new := diffLines("line0", "line1", "line2")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffAdded, 0, 0, nil, []string{"line0"})
}

func TestParseDiffOutput_DeleteAtStart(t *testing.T) {
	old := diffLines("line0", "line1", "line2")
	new := diffLines("line1", "line2")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffDeleted, 0, 0, []string{"line0"}, nil)
}

func TestParseDiffOutput_EmptyOldFile(t *testing.T) {
	old := []byte("")
	new := diffLines("line1", "line2", "line3")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffAdded, 0, 2, nil, []string{"line1", "line2", "line3"})
}

func TestParseDiffOutput_EmptyNewFile(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := []byte("")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffDeleted, 0, 0, []string{"line1", "line2", "line3"}, nil)
}

func TestParseDiffOutput_NoTrailingNewline(t *testing.T) {
	old := diffLines("line1", "line2")
	new := append(diffLines("line1", "line2"), []byte("line3")...) // no trailing \n

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffAdded, 2, 2, nil, []string{"line3"})
}

func TestParseDiffOutput_MixedAddDelete(t *testing.T) {
	old := diffLines("line1", "line2", "line3", "line4")
	new := diffLines("line1", "line2-new", "line3-new", "line4")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffModified, 1, 2,
		[]string{"line2", "line3"}, []string{"line2-new", "line3-new"})
}

func TestParseDiffOutput_ReplaceWithMoreLines(t *testing.T) {
	old := diffLines("line1", "line2", "line3")
	new := diffLines("line1", "line2a", "line2b", "line2c", "line3")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffModified, 1, 3,
		[]string{"line2"}, []string{"line2a", "line2b", "line2c"})
}

func TestParseDiffOutput_ReplaceWithFewerLines(t *testing.T) {
	old := diffLines("line1", "line2", "line3", "line4")
	new := diffLines("line1", "line2-replacement", "line4")

	output := diff.Diff("HEAD", old, "buffer", new)
	hunks := parseDiffOutput(output)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	verifyHunk(t, hunks[0], providers.DiffModified, 1, 1,
		[]string{"line2", "line3"}, []string{"line2-replacement"})
}

// ---------------------------------------------------------------------------
// finalizeHunkType tests
// ---------------------------------------------------------------------------

func TestFinalizeHunkType_Added(t *testing.T) {
	h := &providers.DiffHunk{
		Type:     providers.DiffModified,
		NewLines: []string{"new"},
	}
	finalizeHunkType(h)
	if h.Type != providers.DiffAdded {
		t.Errorf("Type: got %v, want DiffAdded", h.Type)
	}
}

func TestFinalizeHunkType_Deleted(t *testing.T) {
	h := &providers.DiffHunk{
		Type:      providers.DiffModified,
		StartLine: 5,
		EndLine:   7,
		OldLines:  []string{"old"},
	}
	finalizeHunkType(h)
	if h.Type != providers.DiffDeleted {
		t.Errorf("Type: got %v, want DiffDeleted", h.Type)
	}
	if h.EndLine != h.StartLine {
		t.Errorf("EndLine: got %d, want %d (should equal StartLine)", h.EndLine, h.StartLine)
	}
}

func TestFinalizeHunkType_Modified(t *testing.T) {
	h := &providers.DiffHunk{
		Type:     providers.DiffModified,
		OldLines: []string{"old"},
		NewLines: []string{"new"},
	}
	finalizeHunkType(h)
	if h.Type != providers.DiffModified {
		t.Errorf("Type: got %v, want DiffModified", h.Type)
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkParseDiffOutput(b *testing.B) {
	old := diffLines(
		"package main",
		"",
		"import \"fmt\"",
		"",
		"func main() {",
		"	fmt.Println(\"hello\")",
		"}",
	)
	new := diffLines(
		"package main",
		"",
		"import (",
		"	\"fmt\"",
		"	\"os\"",
		")",
		"",
		"func main() {",
		"	fmt.Println(\"hello world\")",
		"	os.Exit(0)",
		"}",
	)

	output := diff.Diff("HEAD", old, "buffer", new)

	
	for b.Loop() {
		parseDiffOutput(output)
	}
}
