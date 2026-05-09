package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"

	"github.com/coder/acp-go-sdk"
	"looz.ws/typstify/utils"
)

type PromptResponse = acp.PromptResponse

type AgentConfig struct {
	Name string
	Cmd  string
	Args []string
}

type ACPSession struct {
	SessionID string
	Cwd       string
	conn      *AgentConn
	// Human-readable title for the session
	title string
	// ISO 8601 timestamp of last activity
	updatedAt string
	modeState acp.SessionModeState
	// Available slash commands supported by the Agent.
	availableCommands []acp.AvailableCommand
	mu                sync.Mutex
}

type AgentConn struct {
	cmd               *exec.Cmd
	Conn              *acp.ClientSideConnection
	AgentInfo         acp.Implementation
	AgentCapabilities acp.AgentCapabilities
}

func (c *AgentConn) Close() error {
	return c.cmd.Process.Kill()
}

type SessionManager struct {
	// conns maintains a pool for connections between all running agents
	// and the ACP client
	conns map[string]*AgentConn
	// All registered agents in the app. Maps name to its config.
	agents map[string]AgentConfig
	// Active ACP sessions which are either newly created, loaded or resumed from Agents.
	activeSessions []*ACPSession
	mu             sync.Mutex
}

// Start runs a agent through ACP client. The config is used to specify
// which agent to be started.
func (sm *SessionManager) Start(ctx context.Context, agentConfig AgentConfig, client acp.Client) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.conns[agentConfig.Name]; exists {
		return nil
	}

	sm.agents[agentConfig.Name] = agentConfig

	cmd := utils.BuildCmd(ctx, agentConfig.Cmd, agentConfig.Args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe error: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe error: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(slog.Default()) // TODO: redirect to app console.

	// Initialize
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	})

	err = checkACPErr(err)
	if err != nil {
		_ = cmd.Process.Kill()

		return err
	}

	sm.conns[agentConfig.Name] = &AgentConn{
		cmd:               cmd,
		Conn:              conn,
		AgentInfo:         *initResp.AgentInfo,
		AgentCapabilities: initResp.AgentCapabilities,
	}

	log.Printf("Connected to %s (ACP version %v)", initResp.AgentInfo.Name, initResp.ProtocolVersion)
	return nil
}

func (sm *SessionManager) NewSession(ctx context.Context, agentName string, cwd string) (*ACPSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	conn := sm.conns[agentName]
	if conn == nil {
		return nil, fmt.Errorf("not connected to agent: %s", agentName)
	}

	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}

	resp, err := conn.Conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})

	err = checkACPErr(err)
	if err != nil {
		return nil, err
	}
	session := NewACPSession(string(resp.SessionId), cwd)
	session.UpdateMode(*resp.Modes)
	session.SetConn(conn)

	sm.activeSessions = append(sm.activeSessions, session)

	return session, nil
}

// ListSessions from Agent. Sessions returned are not *active*, callers need to call LoadSession to get an active one.
func (sm *SessionManager) ListSessions(ctx context.Context, agentName string, filterCwd string) ([]*ACPSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	conn := sm.conns[agentName]
	if conn == nil {
		return nil, fmt.Errorf("not connected to agent: %s", agentName)
	}

	// If the agent does not support loading sessions, return without error.
	listCap := conn.AgentCapabilities.SessionCapabilities.List
	if listCap != nil {
		return nil, nil
	}

	cwd, err := filepath.Abs(filterCwd)
	if err != nil {
		return nil, err
	}

	allAgentSessions := make([]*ACPSession, 0)

	cursor := ""
	first := true
	for cursor != "" || first {
		resp, err := conn.Conn.ListSessions(ctx, acp.ListSessionsRequest{
			Cwd:    &cwd,
			Cursor: &cursor,
		})

		err = checkACPErr(err)
		if err != nil {
			return nil, err
		}

		for _, sn := range resp.Sessions {
			// some fields is not populated, needs to call LoadSession to fill them.
			session := NewACPSession(string(sn.SessionId), cwd)
			session.UpdateInfo(*sn.Title, *sn.UpdatedAt)

			allAgentSessions = append(allAgentSessions, session)

		}

		if resp.NextCursor == nil {
			cursor = ""
		} else {
			cursor = *resp.NextCursor
		}
	}

	return allAgentSessions, nil
}

// LoadSession loads a session from the Agent. The Agent will replay the entire
// conversation to the Client in the form of session/update notifications.
func (sm *SessionManager) LoadSession(ctx context.Context, agentName string, session *ACPSession) (*ACPSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session.Active() {
		return session, nil
	}

	conn := sm.conns[agentName]
	if conn == nil {
		return nil, fmt.Errorf("not connected to agent: %s", agentName)
	}

	// If the agent does not support loading session, return without error.
	if !conn.AgentCapabilities.LoadSession {
		return nil, nil
	}

	resp, err := conn.Conn.LoadSession(ctx, acp.LoadSessionRequest{
		Cwd:        session.Cwd,
		McpServers: []acp.McpServer{},
		SessionId:  acp.SessionId(session.SessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return nil, err
	}

	session.SetConn(conn)
	session.UpdateMode(*resp.Modes)

	sm.activeSessions = append(sm.activeSessions, session)

	return session, nil
}

// ResumeSession loads a session from the Agent. Unlike LoadSession, the Agent will NOT replay prior
// conversation to the client.
func (sm *SessionManager) ResumeSession(ctx context.Context, agentName string, session *ACPSession) (*ACPSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session.Active() {
		return session, nil
	}

	conn := sm.conns[agentName]
	if conn == nil {
		return nil, fmt.Errorf("not connected to agent: %s", agentName)
	}

	// If the agent does not support resume sessions, return without error.
	resumeCap := conn.AgentCapabilities.SessionCapabilities.Resume
	if resumeCap != nil {
		return nil, nil
	}

	resp, err := conn.Conn.ResumeSession(ctx, acp.ResumeSessionRequest{
		Cwd:        session.Cwd,
		McpServers: []acp.McpServer{},
		SessionId:  acp.SessionId(session.SessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return nil, err
	}

	session.SetConn(conn)
	session.UpdateMode(*resp.Modes)

	sm.activeSessions = append(sm.activeSessions, session)

	return session, nil
}

// CloseSession allow Clients to tell the Agent to cancel any ongoing work
// for a session and free any resources associated with that active session.
func (sm *SessionManager) CloseSession(ctx context.Context, sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := sm.getActiveSession(sessionID)
	if session == nil {
		return fmt.Errorf("no active session found: %s", sessionID)
	}

	// If the agent does not support close active sessions, return without error.
	closeCap := session.Conn().AgentCapabilities.SessionCapabilities.Close
	if closeCap != nil {
		return nil
	}

	_, err := session.Conn().Conn.CloseSession(ctx, acp.CloseSessionRequest{
		SessionId: acp.SessionId(sessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return err
	}

	sm.activeSessions = slices.DeleteFunc(sm.activeSessions, func(sn *ACPSession) bool {
		return sn.SessionID == sessionID
	})

	return nil
}

func (sm *SessionManager) getActiveSession(sessionID string) *ACPSession {
	idx := slices.IndexFunc(sm.activeSessions, func(sn *ACPSession) bool {
		return sn.SessionID == sessionID
	})

	if idx < 0 {
		return nil
	}

	return sm.activeSessions[idx]
}

func (sm *SessionManager) GetActiveSession(sessionID string) *ACPSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.getActiveSession(sessionID)
}

func (sm *SessionManager) Close(ctx context.Context) error {
	sm.mu.Lock()
	activeSessions := sm.activeSessions
	sm.mu.Unlock()

	for _, sn := range activeSessions {
		_ = sm.CloseSession(ctx, sn.SessionID)
	}

	return nil
}

func checkACPErr(err error) error {
	if err == nil {
		return nil
	}

	if re, ok := err.(*acp.RequestError); ok {
		return fmt.Errorf("ACP request error (%d): %s", re.Code, re.Message)
	} else {
		return fmt.Errorf("ACP request error: %w", err)
	}
}

func NewACPSession(sessionID string, cwd string) *ACPSession {
	return &ACPSession{
		SessionID: sessionID,
		Cwd:       cwd,
	}
}

func (sn *ACPSession) Active() bool {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.conn != nil
}

func (sn *ACPSession) SetConn(conn *AgentConn) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	if sn.conn != nil {
		panic("Cannot set connection of active session")
	}
	sn.conn = conn
}

func (sn *ACPSession) Conn() *AgentConn {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.conn
}

func (sn *ACPSession) Title() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.title
}

func (sn *ACPSession) UpdatedAt() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.updatedAt
}

func (sn *ACPSession) AvailableModes() []acp.SessionMode {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.modeState.AvailableModes
}

func (sn *ACPSession) CurrentModeID() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return string(sn.modeState.CurrentModeId)
}

func (sn *ACPSession) AvailableCommands() []acp.AvailableCommand {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.availableCommands
}

func (sn *ACPSession) UpdateInfo(title string, updatedAt string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.title = title
	sn.updatedAt = updatedAt
}

func (sn *ACPSession) UpdateMode(modeState acp.SessionModeState) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.modeState = modeState
}

func (sn *ACPSession) SetCommands(commands []acp.AvailableCommand) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.availableCommands = commands
}

// Prompt sends content blocks to Agent. If there are no pending tool calls,
// the turn ends and the Agent respond with a StopReason and optional Usage.
func (sn *ACPSession) Prompt(ctx context.Context, contents ...acp.ContentBlock) (PromptResponse, error) {
	if !sn.Active() {
		return PromptResponse{}, errors.New("invalid session")
	}

	// Validate the content structure and kind.
	//
	// ACP: As a baseline, all Agents MUST support ContentBlock::Text and
	// ContentBlock::ResourceLink in session/prompt requests.
	for _, content := range contents {
		if err := content.Validate(); err != nil {
			return PromptResponse{}, err
		}

		isAudio := content.Audio != nil
		isImage := content.Image != nil
		isEmbeddedContext := content.Resource != nil

		if !sn.conn.AgentCapabilities.PromptCapabilities.Audio && isAudio {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Audio.Type)
		}

		if !sn.conn.AgentCapabilities.PromptCapabilities.Image && isImage {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Image.Type)
		}

		if !sn.conn.AgentCapabilities.PromptCapabilities.EmbeddedContext && isEmbeddedContext {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Resource.Type)
		}
	}

	resp, err := sn.conn.Conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sn.SessionID),
		Prompt:    contents,
	})

	err = checkACPErr(err)
	if err != nil {
		return PromptResponse{}, err
	}

	return resp, nil
}
