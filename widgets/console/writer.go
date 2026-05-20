package console

import (
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/oligo/gvcode"
)

var _ io.Writer = (*ConsoleState)(nil)

type ConsoleState struct {
	lines       []string
	partialLine string
	maxLines    int
	textUpated  atomic.Bool
	mu          sync.Mutex
	State       *gvcode.Editor
}

// Create a console.
func NewConsoleState(maxLines int) *ConsoleState {
	state := &gvcode.Editor{}
	state.WithOptions(
		gvcode.WithLineHeight(0, 1.5),
		gvcode.ReadOnlyMode(true),
		gvcode.WrapLine(true),
	)
	c := &ConsoleState{
		maxLines: maxLines,
		State:    state,
	}

	return c
}

func (c *ConsoleState) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	msg := stripAnsi(string(data))
	c.appendText(msg)
	c.textUpated.Store(true)

	return len(data), nil
}

func (c *ConsoleState) HasMore() bool {
	return c.textUpated.Load()
}

func (c *ConsoleState) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = c.lines[:0]
	c.partialLine = ""
	c.State.SetText("")
}

func (c *ConsoleState) readBuffered() {
	c.mu.Lock()
	text := c.visibleText()
	c.State.SetText(text)
	textLen := c.State.Len()
	c.State.SetCaret(textLen, textLen)
	c.mu.Unlock()
}

func (c *ConsoleState) Update() {
	if c.textUpated.CompareAndSwap(true, false) {
		c.readBuffered()
		c.truncate()
	}
}

// truncate keeps only the newest maxLines of completed/visible console output.
// It runs after new text is appended, so older lines are dropped as soon as the
// buffered line store grows past the configured cap.
func (c *ConsoleState) truncate() {
	if c.maxLines <= 0 {
		c.lines = c.lines[:0]
		c.partialLine = ""
		return
	}

	lineCount := len(c.lines)
	if c.partialLine != "" {
		lineCount++
	}
	if lineCount <= c.maxLines {
		return
	}

	overflow := lineCount - c.maxLines
	if overflow >= len(c.lines) {
		c.lines = c.lines[:0]
		return
	}

	c.lines = append(c.lines[:0], c.lines[overflow:]...)
}

func (c *ConsoleState) appendText(msg string) {
	if msg == "" {
		return
	}

	combined := c.partialLine + msg
	c.partialLine = ""

	parts := strings.SplitAfter(combined, "\n")
	if !strings.HasSuffix(combined, "\n") {
		c.partialLine = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	c.lines = append(c.lines, parts...)
	c.truncate()
}

func (c *ConsoleState) visibleText() string {
	var b strings.Builder
	total := 0
	for _, line := range c.lines {
		total += len(line)
	}
	total += len(c.partialLine)
	b.Grow(total)

	for _, line := range c.lines {
		b.WriteString(line)
	}
	b.WriteString(c.partialLine)

	return b.String()
}

// ansi matches terminal escape/control sequences such as colors, cursor moves,
// and other CSI/OSC-style commands so GUI console output stays plain text.
const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

// Strip removes ANSI terminal escape sequences from streamed console output.
func stripAnsi(str string) string {
	return re.ReplaceAllString(str, "")
}
