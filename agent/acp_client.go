package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
)

var _ acp.Client = (*ACPClient)(nil)

// ACPClient implements a ACP client.
type ACPClient struct {
	sm *SessionManager
	// ongoing terminals
	terminals []*acpTernimal
	tmMu      sync.Mutex
}

func NewACPClient(sm *SessionManager) *ACPClient {
	return &ACPClient{
		sm: sm,
	}
}

// ReadTextFile implements [acp.Client].
func (a *ACPClient) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	emptyResp := acp.ReadTextFileResponse{}

	if err := params.Validate(); err != nil {
		return emptyResp, err
	}

	session := a.sm.GetActiveSession(string(params.SessionId))
	if session == nil {
		return emptyResp, fmt.Errorf("no active session: %s", params.SessionId)
	}

	absPath, err := resolvePath(session.Cwd, params.Path)
	if err != nil {
		return emptyResp, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return emptyResp, fmt.Errorf("read file %s: %w", absPath, err)
	}
	defer f.Close()

	startLine := 1
	if params.Line != nil && *params.Line > 0 {
		startLine = *params.Line
	}
	limit := 0
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}

	scanner := bufio.NewScanner(f)
	var out strings.Builder
	lineNum := 0
	written := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if written > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(scanner.Text())
		written++
		if limit > 0 && written >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return emptyResp, fmt.Errorf("read file %s: %w", absPath, err)
	}

	return acp.ReadTextFileResponse{
		Content: out.String(),
	}, nil
}

// WriteTextFile implements [acp.Client].
func (a *ACPClient) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	emptyResp := acp.WriteTextFileResponse{}

	if err := params.Validate(); err != nil {
		return emptyResp, err
	}

	session := a.sm.GetActiveSession(string(params.SessionId))
	if session == nil {
		return emptyResp, fmt.Errorf("no active session: %s", params.SessionId)
	}

	absPath, err := resolvePath(session.Cwd, params.Path)
	if err != nil {
		return emptyResp, err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return emptyResp, fmt.Errorf("write file %s: %w", absPath, err)
	}

	if err := os.WriteFile(absPath, []byte(params.Content), 0644); err != nil {
		return emptyResp, fmt.Errorf("write file %s: %w", absPath, err)
	}

	return emptyResp, nil
}

// RequestPermission implements [acp.Client].
func (a *ACPClient) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	emptyResp := acp.RequestPermissionResponse{}

	if err := params.Validate(); err != nil {
		log.Println("RequestPermission error: ", err)
		return emptyResp, err
	}

	//log.Printf("Agent request permission for toolcall: %v", params.ToolCall.ToolCallId)
	session := a.sm.GetActiveSession(string(params.SessionId))
	if session == nil {
		log.Printf("No active ACP session found: %s", params.SessionId)
		return emptyResp, fmt.Errorf("No active ACP session found: %s", params.SessionId)
	}

	// send and wait for user grants
	permissionGrantChan := make(chan acp.PermissionOptionId)
	session.RequestPermission(params, permissionGrantChan)

	// respond with the result. If prompt turn is canceled, the ongoing tool call grants
	// should also be canceled.
	if session.CurrentTurn == nil {
		optionID := <-permissionGrantChan
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(optionID),
		}, nil

	} else {
		select {
		case optionID := <-permissionGrantChan:
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(optionID),
			}, nil
		case toolCallID := <-session.CurrentTurn.CancelChan():
			if toolCallID != params.ToolCall.ToolCallId {
				break
			}
			// do not check session.hasOngoingTurn here, as this should be called AFTER the turn is canceled.
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeCancelled(),
			}, nil

		}
	}

	return emptyResp, nil

}

// SessionUpdate implements [acp.Client].
func (a *ACPClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	if err := params.Validate(); err != nil {
		log.Println("SessionUpdate error: ", err)
		return err
	}

	if err := params.Update.Validate(); err != nil {
		log.Println("SessionUpdate error: ", err)
		return err
	}

	update := params.Update
	session := a.sm.GetActiveSession(string(params.SessionId))
	if session == nil {
		log.Printf("No active ACP session found: %s", params.SessionId)
		return fmt.Errorf("No active ACP session found: %s", params.SessionId)
	}

	if update.UserMessageChunk != nil {
		session.PublishUpdate(*update.UserMessageChunk)
	}

	if update.AgentMessageChunk != nil {
		session.PublishUpdate(*update.AgentMessageChunk)
	}

	if update.AgentThoughtChunk != nil {
		session.PublishUpdate(*update.AgentThoughtChunk)
	}

	if update.Plan != nil {
		session.PublishUpdate(*update.Plan)
	}

	if update.ToolCall != nil {
		session.PublishUpdate(*update.ToolCall)
	}

	if update.ToolCallUpdate != nil {
		session.PublishUpdate(*update.ToolCallUpdate)
	}

	if update.CurrentModeUpdate != nil {
		session.PublishUpdate(*update.CurrentModeUpdate)
	}

	if update.ConfigOptionUpdate != nil {
		session.PublishUpdate(*update.ConfigOptionUpdate)
	}

	if update.SessionInfoUpdate != nil {
		session.PublishUpdate(*update.SessionInfoUpdate)
	}

	if update.AvailableCommandsUpdate != nil {
		session.PublishUpdate(*update.AvailableCommandsUpdate)
	}

	if update.UsageUpdate != nil {
		session.PublishUpdate(*update.UsageUpdate)
	}

	return nil
}

func (a *ACPClient) getTerminal(terminalID string) *acpTernimal {
	a.tmMu.Lock()
	defer a.tmMu.Unlock()

	idx := slices.IndexFunc(a.terminals, func(t *acpTernimal) bool {
		return t.ID == terminalID
	})
	if idx < 0 {
		return nil
	}

	return a.terminals[idx]
}

func (a *ACPClient) releaseTerminal(terminalID string) error {
	a.tmMu.Lock()
	defer a.tmMu.Unlock()

	idx := slices.IndexFunc(a.terminals, func(t *acpTernimal) bool {
		return t.ID == terminalID
	})
	if idx < 0 {
		return fmt.Errorf("terminal not found: %s", terminalID)
	}

	a.terminals = slices.Delete(a.terminals, idx, idx+1)
	return nil
}

// CreateTerminal implements [acp.Client].
func (a *ACPClient) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	emptyResp := acp.CreateTerminalResponse{}

	if err := params.Validate(); err != nil {
		log.Println("CreateTerminal error: ", err)
		return emptyResp, err
	}

	terminal := newTerminal(params)
	if err := terminal.Start(); err != nil {
		return emptyResp, err
	}

	a.tmMu.Lock()
	defer a.tmMu.Unlock()
	a.terminals = append(a.terminals, terminal)

	return acp.CreateTerminalResponse{
		TerminalId: terminal.ID,
	}, nil

}

// KillTerminal implements [acp.Client].
// The terminal/kill method terminates a command without releasing the terminal.
// After killing a command, the terminal remains valid and can be used with:
//
//	terminal/output to get the final output
//	terminal/wait_for_exit to get the exit status
//
// The Agent MUST still call terminal/release when it’s done using it.
func (a *ACPClient) KillTerminal(ctx context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	emptyResp := acp.KillTerminalResponse{}

	if err := params.Validate(); err != nil {
		log.Println("KillTerminal error: ", err)
		return emptyResp, err
	}

	terminal := a.getTerminal(params.TerminalId)
	if terminal == nil {
		return emptyResp, errors.New("terminal not found")
	}

	err := terminal.Kill()
	if err != nil {
		return emptyResp, err
	}

	return emptyResp, nil
}

// ReleaseTerminal implements [acp.Client].
// The terminal/release kills the command if still running and releases all resources.
// After release the terminal ID becomes invalid for all other terminal/* methods.
func (a *ACPClient) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	emptyResp := acp.ReleaseTerminalResponse{}

	if err := params.Validate(); err != nil {
		log.Println("ReleaseTerminal error: ", err)
		return emptyResp, err
	}

	terminal := a.getTerminal(params.TerminalId)
	if terminal == nil {
		return emptyResp, errors.New("terminal not found")
	}

	err := terminal.Kill()
	if err != nil {
		return emptyResp, err
	}

	err = a.releaseTerminal(params.TerminalId)
	if err != nil {
		return emptyResp, err
	}
	return emptyResp, nil
}

// TerminalOutput implements [acp.Client].
func (a *ACPClient) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	resp := acp.TerminalOutputResponse{}

	if err := params.Validate(); err != nil {
		log.Println("TerminalOutput error: ", err)
		return resp, err
	}

	terminal := a.getTerminal(params.TerminalId)
	if terminal == nil {
		return resp, errors.New("terminal not found")
	}

	output, truncated := terminal.Output()
	exitCode, signal := terminal.ExitStatus()

	var sig *string = nil
	if signal >= 0 {
		sigStr := signal.String()
		sig = &sigStr
	}
	resp.Output = output
	resp.Truncated = truncated
	resp.ExitStatus = &acp.TerminalExitStatus{
		ExitCode: &exitCode,
		Signal:   sig,
	}

	return resp, nil
}

// WaitForTerminalExit implements [acp.Client].
//
// The terminal/wait_for_exit method returns once the command completes.
func (a *ACPClient) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	resp := acp.WaitForTerminalExitResponse{}

	if err := params.Validate(); err != nil {
		log.Println("WaitForTerminalExit error: ", err)
		return resp, err
	}

	terminal := a.getTerminal(params.TerminalId)
	if terminal == nil {
		return resp, errors.New("terminal not found")
	}

	err := terminal.Wait()
	if err != nil {
		return resp, err
	}

	exitCode, signal := terminal.ExitStatus()
	var sig *string = nil
	if signal >= 0 {
		sigStr := signal.String()
		sig = &sigStr
	}
	resp.ExitCode = &exitCode
	resp.Signal = sig

	return resp, nil
}

// resolvePath resolves a file path relative to the session cwd, preventing
// directory traversal outside the project root.
func resolvePath(cwd, path string) (string, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(cwd, path)
	}

	clean := filepath.Clean(absPath)

	// Verify the resolved path stays within the project directory.
	rel, err := filepath.Rel(cwd, clean)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %s is outside project directory", path)
	}

	return clean, nil
}
