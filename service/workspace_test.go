package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"looz.ws/typstify/service/bus"
)

func newTestWorkspace(t *testing.T) *WorkspaceService {
	t.Helper()

	ws := NewWorkspaceService(t.TempDir(), bus.NewEventBus(context.Background(), true))
	t.Cleanup(ws.Close)

	return ws
}

func writeTypFile(t *testing.T, dir string, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("= Test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestSetEntryFile(t *testing.T) {
	projectDir := t.TempDir()
	entryFile := writeTypFile(t, projectDir, "main.typ")

	ws := newTestWorkspace(t)
	ws.SwitchWorkspace(projectDir)

	if got := ws.EntryFile(); got != "" {
		t.Fatalf("expected no entry file initially, got %q", got)
	}

	if err := ws.SetEntryFile("main.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}

	if got := ws.EntryFile(); got != entryFile {
		t.Fatalf("expected entry file %q, got %q", entryFile, got)
	}

	// The pin must be persisted in the workspace settings file.
	if got := ws.LoadWorkspaceSettings().EntryFile; got != "main.typ" {
		t.Fatalf("expected persisted entry file %q, got %q", "main.typ", got)
	}

	if err := ws.SetEntryFile(""); err != nil {
		t.Fatalf("unpinning the entry file failed: %v", err)
	}

	if got := ws.EntryFile(); got != "" {
		t.Fatalf("expected no entry file after unpinning, got %q", got)
	}
}

func TestSetEntryFileValidation(t *testing.T) {
	projectDir := t.TempDir()
	writeTypFile(t, projectDir, "chapters/chapter1.typ")

	ws := newTestWorkspace(t)
	ws.SwitchWorkspace(projectDir)

	for _, relPath := range []string{
		"missing.typ",    // does not exist
		"..",             // not a file
		"../outside.typ", // escapes the project root
		filepath.Join(projectDir, "chapters/chapter1.typ"), // not a local path
	} {
		if err := ws.SetEntryFile(relPath); err == nil {
			t.Fatalf("expected an error when pinning %q", relPath)
		}
	}

	// Nested files are fine.
	if err := ws.SetEntryFile("chapters/chapter1.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}
}

func TestEntryFileIsPerWorkspace(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	entryA := writeTypFile(t, dirA, "main.typ")
	entryB := writeTypFile(t, dirB, "thesis.typ")

	ws := newTestWorkspace(t)

	ws.SwitchWorkspace(dirA)
	if err := ws.SetEntryFile("main.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}

	ws.SwitchWorkspace(dirB)
	if got := ws.EntryFile(); got != "" {
		t.Fatalf("expected workspace B to have no entry file, got %q", got)
	}
	if err := ws.SetEntryFile("thesis.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}

	ws.SwitchWorkspace(dirA)
	if got := ws.EntryFile(); got != entryA {
		t.Fatalf("expected entry file %q, got %q", entryA, got)
	}

	ws.SwitchWorkspace(dirB)
	if got := ws.EntryFile(); got != entryB {
		t.Fatalf("expected entry file %q, got %q", entryB, got)
	}
}

func TestEntryFileRemovedFromDisk(t *testing.T) {
	projectDir := t.TempDir()
	entryFile := writeTypFile(t, projectDir, "main.typ")

	ws := newTestWorkspace(t)
	ws.SwitchWorkspace(projectDir)
	if err := ws.SetEntryFile("main.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}

	// Unpinning clears the entry file.
	if err := ws.SetEntryFile(""); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}
	if got := ws.EntryFile(); got != "" {
		t.Fatalf("expected no entry file, got %q", got)
	}

	// A pin pointing to a file that disappeared is resolved to nothing the next
	// time the workspace is opened.
	if err := ws.SetEntryFile("main.typ"); err != nil {
		t.Fatalf("SetEntryFile failed: %v", err)
	}
	if err := os.Remove(entryFile); err != nil {
		t.Fatal(err)
	}

	ws.SwitchWorkspace(projectDir)
	if got := ws.EntryFile(); got != "" {
		t.Fatalf("expected a deleted entry file to be ignored, got %q", got)
	}
}
