package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf8"

	"github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	"looz.ws/typstify/utils"
)

type lockedWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

type ACPTerminal struct {
	ID              string
	SessionId       string
	SessionCwd      string
	OutputByteLimit int
	cmd             *exec.Cmd
	outputBuf       bytes.Buffer
	bufMu           sync.Mutex
	exited          atomic.Bool
}

func newTerminal(cwd string, req acp.CreateTerminalRequest) (*ACPTerminal, error) {
	t := &ACPTerminal{
		ID:         uuid.NewString(),
		SessionId:  string(req.SessionId),
		SessionCwd: cwd,
	}

	if req.OutputByteLimit != nil {
		t.OutputByteLimit = *req.OutputByteLimit
	}

	ctx := context.Background()
	cmd := buildCmd(ctx, req.Command, req.Args)

	envs := make([]string, 0, len(req.Env))
	for _, env := range req.Env {
		envs = append(envs, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}
	cmd.Env = append(envs, cmd.Environ()...)

	if req.Cwd != nil && *req.Cwd != "" {
		resolvedCwd, err := resolvePath(t.SessionCwd, nil, *req.Cwd)
		if err != nil {
			return nil, err
		}
		cmd.Dir = resolvedCwd
	} else {
		cmd.Dir = t.SessionCwd
	}

	cmd.Stdout = t.writer()
	cmd.Stderr = t.writer()
	t.cmd = cmd
	return t, nil
}

func (t *ACPTerminal) writer() *lockedWriter {
	return &lockedWriter{buf: &t.outputBuf, mu: &t.bufMu}
}

func (t *ACPTerminal) Start() error {
	return t.cmd.Start()
}

func (t *ACPTerminal) IsKilled() bool {
	return t.exited.Load()
}

func (t *ACPTerminal) Kill() error {
	if t.exited.CompareAndSwap(false, true) {
		return t.cmd.Cancel()
	}

	return nil
}

// Output returns output of terminal, truncated if the content exceeds
// OutputByteLimit.
//
// Calling Output() twice without any intervening writes to the buffer
// will yield the exact same string.
func (t *ACPTerminal) Output() (_ string, truncated bool) {
	t.bufMu.Lock()
	defer t.bufMu.Unlock()

	data := t.outputBuf.Bytes()
	totalLen := len(data)

	if t.OutputByteLimit <= 0 || totalLen <= t.OutputByteLimit {
		return string(data), false
	}

	// Calculate the starting point for the truncation
	// We want to keep the LAST 'outputByteLimit' bytes
	start := totalLen - t.OutputByteLimit

	// ACP Requirement: Truncation MUST happen at a character boundary.
	// We move the start index forward until we find a valid UTF-8 start byte.
	for start < totalLen && !utf8.RuneStart(data[start]) {
		start++
	}

	return string(data[start:]), true
}

// OutputSize returns total buffered output size in bytes.
func (t *ACPTerminal) OutputSize() int {
	t.bufMu.Lock()
	defer t.bufMu.Unlock()

	return t.outputBuf.Len()
}

func (t *ACPTerminal) ExitStatus() (exitCode int, signal syscall.Signal) {
	if t.cmd.ProcessState == nil {
		return -1, -1
	}

	exitCode = t.cmd.ProcessState.ExitCode()

	sys := t.cmd.ProcessState.Sys()
	if sys != nil {
		status, ok := sys.(syscall.WaitStatus)
		if !ok {
			return
		}
		signal = status.Signal()
	}

	return
}

func (t *ACPTerminal) Wait() error {
	if t.exited.CompareAndSwap(false, true) {
		return t.cmd.Wait()
	}
	return nil
}

var (
	cmdNamePattern = regexp.MustCompile(`^[\w][\w.\-]*$`)
)

func isScript(cmd string) bool {
	return !cmdNamePattern.Match([]byte(cmd))
}

func getShell() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell.exe", []string{"-Command"}
	}
	// Unix-like
	return "bash", []string{"-c"}
}

func buildCmd(ctx context.Context, command string, args []string) *exec.Cmd {
	var cmd *exec.Cmd
	if isScript(command) {
		shell, args := getShell()
		fullArgs := append(args, command)
		cmd = utils.BuildCmd(ctx, shell, fullArgs...)
	} else {
		cmd = utils.BuildCmd(ctx, command, args...)
	}

	return cmd
}
