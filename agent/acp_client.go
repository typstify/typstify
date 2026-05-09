package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/coder/acp-go-sdk"
)

var _ acp.Client = (*ACPClient)(nil)

// ACPClient implements a ACP client.
type ACPClient struct {
	sm *SessionManager
}

func NewACPClient(sm *SessionManager) *ACPClient {
	return &ACPClient{
		sm: sm,
	}
}

// CreateTerminal implements [acp.Client].
func (a *ACPClient) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {

	panic("unimplemented")
}

// KillTerminal implements [acp.Client].
func (a *ACPClient) KillTerminal(ctx context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	panic("unimplemented")
}

// ReadTextFile implements [acp.Client].
func (a *ACPClient) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	panic("unimplemented")
}

// ReleaseTerminal implements [acp.Client].
func (a *ACPClient) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	panic("unimplemented")
}

// RequestPermission implements [acp.Client].
func (a *ACPClient) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	panic("unimplemented")
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
		log.Println("user message: ", update.UserMessageChunk.Content.Text)
	}

	if update.AgentMessageChunk != nil {
		log.Println("agent message: ", update.AgentMessageChunk.Content.Text)
	}

	if update.AvailableCommandsUpdate != nil {
		session.SetCommands(update.AvailableCommandsUpdate.AvailableCommands)
	}

	return nil
}

// TerminalOutput implements [acp.Client].
func (a *ACPClient) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	panic("unimplemented")
}

// WaitForTerminalExit implements [acp.Client].
func (a *ACPClient) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	panic("unimplemented")
}

// WriteTextFile implements [acp.Client].
func (a *ACPClient) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	panic("unimplemented")
}
