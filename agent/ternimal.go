package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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

type acpTernimal struct {
	ID              string
	SessionId       string
	OutputByteLimit int
	cmd             *exec.Cmd
	outputBuf       bytes.Buffer
	bufMu           sync.Mutex
	killed          atomic.Bool
}

func newTerminal(req acp.CreateTerminalRequest) *acpTernimal {
	t := &acpTernimal{
		ID:        uuid.NewString(),
		SessionId: string(req.SessionId),
	}

	if req.OutputByteLimit != nil {
		t.OutputByteLimit = *req.OutputByteLimit
	}

	ctx := context.Background()
	cmd := utils.BuildCmd(ctx, req.Command, req.Args...)

	envs := make([]string, 0, len(req.Env))
	for _, env := range req.Env {
		envs = append(envs, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}
	cmd.Env = append(envs, cmd.Env...)

	if req.Cwd != nil && *req.Cwd != "" {
		cmd.Dir = *req.Cwd
	}

	cmd.Stdout = t.writer()
	cmd.Stderr = t.writer()
	t.cmd = cmd
	return t
}

func (t *acpTernimal) writer() *lockedWriter {
	return &lockedWriter{buf: &t.outputBuf, mu: &t.bufMu}
}

func (t *acpTernimal) Start() error {
	return t.cmd.Start()
}

func (t *acpTernimal) IsKilled() bool {
	return t.killed.Load()
}

func (t *acpTernimal) Kill() error {
	if t.killed.CompareAndSwap(false, true) {
		return t.cmd.Cancel()
	}

	return nil
}

// Output returns output of terminal, truncated if the content exceeds
// OutputByteLimit.
//
// Calling Output() twice without any intervening writes to the buffer
// will yield the exact same string.
func (t *acpTernimal) Output() (_ string, truncated bool) {
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

func (t *acpTernimal) ExitStatus() (exitCode int, signal syscall.Signal) {
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

func (t *acpTernimal) Wait() error {
	return t.cmd.Wait()
}
