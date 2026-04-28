package utils

import (
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

func IsGitRepo(filePath string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		log.Println("No git found in PATH, diff will be disabled.")
		return false
	}

	// Get the absolute path and directory
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		log.Printf("Failed to get absolute path: %v", err)
		return false
	}
	dir := filepath.Dir(absPath)

	// Run git diff
	cmd := BuildCmd(context.Background(), "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		if err != nil {
			log.Println("Failed to detect git repo: ", err)
		}
		return false
	}

	return true

}

func CurrentGitBranch(projectPath string) (string, error) {
	cmd := BuildCmd(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = projectPath

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type Status int

const (
	StatusUnchanged Status = iota
	StatusUntracked
	StatusModified
	StatusStaged
	StatusStagedModified // Staged changes exist, AND further unstaged changes exist
	StatusAdded
	StatusDeleted
	StatusRenamed
	StatusConflict
)

// String representation for debugging or UI tooltips
func (s Status) String() string {
	return []string{
		"Unchanged", "Untracked", "Modified", "Staged",
		"Staged + Modified", "Added", "Deleted", "Renamed", "Conflict",
	}[s]
}

func mapCharsToStatus(x, y rune) Status {
	switch {
	// Untracked
	case x == '?' && y == '?':
		return StatusUntracked

	// Conflicts (U is common in merge conflicts)
	case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
		return StatusConflict

	// Added to index
	case x == 'A' && y == ' ':
		return StatusAdded

	// Renamed
	case x == 'R':
		return StatusRenamed

	case x == ' ' && y == 'M':
		return StatusModified
	case x == 'M' && y == ' ':
		return StatusStaged
	case x == 'M' && y == 'M':
		return StatusStagedModified

	// Deletions
	case x == 'D' || y == 'D':
		return StatusDeleted

	default:
		return StatusUnchanged
	}
}

// GitFileStatus returns the status of the file: 'M ' (staged), ' M' (unstaged), 'MM' (both)
func GitFileStatus(filePath string) Status {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return StatusUnchanged
	}

	dir := filepath.Dir(absPath)
	filename := filepath.Base(absPath)

	// --porcelain=v1 for stable output.
	cmd := BuildCmd(context.Background(), "git", "status", "--porcelain=v1", filename)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return StatusUnchanged
	}
	if len(out) < 2 {
		return StatusUnchanged
	}

	// X = Staged status, Y = Unstaged status
	x, y := rune(out[0]), rune(out[1])

	return mapCharsToStatus(x, y)
}
