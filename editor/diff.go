package editor

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/gutter/providers"
	"github.com/rogpeppe/go-internal/diff"
	"looz.ws/typstify/utils"
)

var (
	defaultDiffDebounce    = time.Millisecond * 100
	defaultMaxDiffInternal = time.Millisecond * 200
)

type DiffBuffer struct {
	baseline []byte
	staged   bool
}

// GitDiff is a helper that can be used to diff git HEAD and the live editor buffer.
type GitDiff struct {
	filePath string
	dir      string
	filename string
	buf      DiffBuffer
	mu       sync.Mutex

	debounce     time.Duration
	maxInterval  time.Duration
	timer        *time.Timer
	lastRun      time.Time
	pending      atomic.Bool
	triggerMu    sync.Mutex
	pendingHunks atomic.Pointer[[]*providers.DiffHunk]
}

func NewGitDiff(filePath string) *GitDiff {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		log.Printf("Failed to get absolute path: %v", err)
		return nil
	}
	dir := filepath.Dir(absPath)
	filename := filepath.Base(absPath)

	return &GitDiff{
		filePath:    absPath,
		dir:         dir,
		filename:    filename,
		debounce:    defaultDiffDebounce,
		maxInterval: defaultMaxDiffInternal,
	}
}

// ReloadBaseline re-reads the content of the file from git HEAD, or staged version.
func (d *GitDiff) ReloadBaseline(staged bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// reload as a result of branch switching:
	if !staged {
		// Get the HEAD version of the file.
		d.loadUnstagedContent()
		return
	}

	// reload as a result of staging
	d.loadStagedContent()
}

func (d *GitDiff) loadUnstagedContent() {
	cmd := utils.BuildCmd(context.Background(), "git", "show", "HEAD:./"+d.filename)
	cmd.Dir = d.dir
	original, err := cmd.Output()
	if err != nil {
		// File might not be committed yet (new file). Treat as empty base.
		return
	}

	d.buf = DiffBuffer{
		baseline: original,
		staged:   false,
	}
}

func (d *GitDiff) loadStagedContent() {
	cmd := utils.BuildCmd(context.Background(), "git", "show", ":./"+d.filename)
	cmd.Dir = d.dir
	indexContent, err := cmd.Output()
	if err != nil {
		return
	}

	d.buf = DiffBuffer{
		baseline: indexContent,
		staged:   true,
	}
}

func (d *GitDiff) Trigger(editor *gvcode.Editor) {
	d.triggerMu.Lock()
	defer d.triggerMu.Unlock()

	now := time.Now()

	// If too long since last run, run immediately
	if now.Sub(d.lastRun) >= d.maxInterval {
		d.lastRun = now
		if d.timer != nil {
			d.timer.Stop()
		}
		d.pending.Store(false)
		go d.ParseDiff(editor)
		return
	}

	// Otherwise debounce
	d.pending.Store(true)

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.debounce, func() {
		if d.pending.CompareAndSwap(true, false) {
			d.lastRun = time.Now()

			go d.ParseDiff(editor)
		}

	})
}

func (d *GitDiff) PendingHunks() []*providers.DiffHunk {
	lastHunks := d.pendingHunks.Swap(nil)
	if lastHunks == nil {
		return nil
	}

	return *lastHunks
}

// ParseDiff diffs the given buffer content against baseline.
// All hunks are marked Staged if the buffer matches the index (staged) version,
// meaning the user hasn't made further edits beyond what's staged.
func (d *GitDiff) ParseDiff(editor *gvcode.Editor) {
	d.pendingHunks.Store(nil)

	if d == nil || editor == nil {
		return
	}

	reader := editor.GetReader()
	reader.Seek(0, io.SeekStart)

	editorBuf, err := io.ReadAll(reader)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.buf.baseline) == 0 {
		// first time to parse the file.
		status := utils.GitFileStatus(d.filePath)
		switch status {
		case utils.StatusModified:
			d.loadUnstagedContent()

		case utils.StatusStaged:
			d.loadStagedContent()
		case utils.StatusStagedModified:
			d.loadUnstagedContent()
		case utils.StatusAdded:
			d.loadUnstagedContent()
		default:
			d.buf = DiffBuffer{
				baseline: editorBuf,
				staged:   false,
			}
		}
	}

	hunks := d.parseBufferDiff(editorBuf)
	// Distinguish nil and empty slice of DiffHunks, so editors can decide whether to
	// clear the hunks or not update it at all.
	if hunks == nil {
		hunks = []*providers.DiffHunk{}
	}

	d.pendingHunks.Store(&hunks)
}

// parseBufferDiff returns the diff between baseline and the given buffer content,
// using a pure Go diff implementation.
func (d *GitDiff) parseBufferDiff(content []byte) []*providers.DiffHunk {
	staged := d.buf.staged
	oldName := "HEAD"
	if staged {
		oldName = "STAGED"
	}

	output := diff.Diff(oldName, d.buf.baseline, "buffer", content)
	if len(output) == 0 {
		return nil
	}

	hunks := parseDiffOutput(output)

	if staged {
		for _, h := range hunks {
			h.Staged = true
		}
	}

	return hunks
}

func (d *GitDiff) Stop() {
	d.triggerMu.Lock()
	defer d.triggerMu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
}

var (
	// Regex to match hunk headers like @@ -10,3 +10,5 @@
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

// finalizeHunkType determines the hunk type based on the actual content
func finalizeHunkType(hunk *providers.DiffHunk) {
	if hunk == nil {
		return
	}

	hasOldLines := len(hunk.OldLines) > 0
	hasNewLines := len(hunk.NewLines) > 0

	if !hasOldLines && hasNewLines {
		hunk.Type = providers.DiffAdded
	} else if hasOldLines && !hasNewLines {
		hunk.Type = providers.DiffDeleted
		// For deleted hunks, the line number is where the deletion occurred
		hunk.EndLine = hunk.StartLine
	} else if hasOldLines && hasNewLines {
		hunk.Type = providers.DiffModified
	}
}

// parseDiffOutput parses unified diff output into DiffHunks.
// Since diff.Diff uses 3 lines of context (like -U3), the hunk header line
// numbers span context + changes. This parser tracks the actual line position
// of each +/- line to compute precise StartLine/EndLine for gutter markers.
func parseDiffOutput(output []byte) []*providers.DiffHunk {
	var hunks []*providers.DiffHunk

	scanner := bufio.NewScanner(bytes.NewReader(output))
	var currentHunk *providers.DiffHunk
	var inHunk bool

	// currentNewLine tracks the 0-based line number in the new file as we
	// walk through the diff content. Context lines and + lines advance it;
	// - lines (from the old file) do not.
	var currentNewLine int

	for scanner.Scan() {
		line := scanner.Text()

		// Check for hunk header
		if matches := hunkHeaderRe.FindStringSubmatch(line); matches != nil {
			// Save previous hunk if exists
			if currentHunk != nil {
				finalizeHunkType(currentHunk)
				hunks = append(hunks, currentHunk)
			}

			newStart, _ := strconv.Atoi(matches[3])
			currentNewLine = newStart - 1 // convert to 0-based
			if currentNewLine < 0 {
				currentNewLine = 0 // e.g. empty new file produces +0,0
			}

			// StartLine/EndLine will be set when the first +/- line is seen
			currentHunk = &providers.DiffHunk{
				Type:      providers.DiffModified,
				StartLine: -1,
				EndLine:   -1,
			}

			inHunk = true
			continue
		}

		// Skip diff headers
		if strings.HasPrefix(line, "diff ") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") {
			continue
		}

		// Process hunk content
		if inHunk && currentHunk != nil {
			if strings.HasPrefix(line, "-") {
				currentHunk.OldLines = append(currentHunk.OldLines, strings.TrimPrefix(line, "-"))
				// Deletion is anchored at currentNewLine (does not advance it)
				if currentHunk.StartLine < 0 || currentNewLine < currentHunk.StartLine {
					currentHunk.StartLine = currentNewLine
				}
				if currentNewLine > currentHunk.EndLine {
					currentHunk.EndLine = currentNewLine
				}
			} else if strings.HasPrefix(line, "+") {
				currentHunk.NewLines = append(currentHunk.NewLines, strings.TrimPrefix(line, "+"))
				if currentHunk.StartLine < 0 || currentNewLine < currentHunk.StartLine {
					currentHunk.StartLine = currentNewLine
				}
				if currentNewLine > currentHunk.EndLine {
					currentHunk.EndLine = currentNewLine
				}
				currentNewLine++
			} else {
				// Context line (starts with " ") or other (e.g. \ No newline)
				currentNewLine++
			}
		}
	}

	// Don't forget the last hunk
	if currentHunk != nil {
		finalizeHunkType(currentHunk)
		hunks = append(hunks, currentHunk)
	}

	return hunks
}
