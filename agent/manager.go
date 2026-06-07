package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"

	"github.com/coder/acp-go-sdk"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/version"
)

type AgentConfig struct {
	Name string
	Cmd  string
	Args []string
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
	// conn maintains a reference for connection between the currently running agent
	// and the ACP client.
	conn *AgentConn
	// The registered agent config.
	agentConfig AgentConfig
	// Active ACP sessions which are either newly created, loaded or resumed from Agents.
	activeSessions []*ACPSession
	mu             sync.Mutex
}

// Start runs a agent through ACP client. The config is used to specify
// which agent to be started.
func (sm *SessionManager) Start(ctx context.Context, agentConfig AgentConfig, client *ACPClient, enableDebug bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.conn != nil {
		return nil
	}

	sm.agentConfig = agentConfig
	// Use a clean context other than the incoming ctx, to prevent the command
	// from being canceled accidentally.
	cmd := utils.BuildCmd(context.Background(), agentConfig.Cmd, agentConfig.Args...)
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

	in, out := stdin, stdout
	if enableDebug {
		in, out = duplicatedIO(stdin, stdout)
	}

	conn := acp.NewClientSideConnection(client, in, out)
	conn.SetLogger(slog.Default()) // TODO: redirect to app console.

	// Initialize
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "Typstify",
			Version: version.BinVersion,
		},
		ClientCapabilities: acp.ClientCapabilities{
			Meta: client.ExtensionCapabilities(),
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

	sm.conn = &AgentConn{
		cmd:               cmd,
		Conn:              conn,
		AgentInfo:         *initResp.AgentInfo,
		AgentCapabilities: initResp.AgentCapabilities,
	}

	log.Printf("Connected to %s (ACP version %v)", initResp.AgentInfo.Name, initResp.ProtocolVersion)
	return nil
}

func (sm *SessionManager) NewSession(ctx context.Context, cwd string) (*ACPSession, error) {
	sm.mu.Lock()
	conn := sm.conn
	sm.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected to agent")
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
	session.SetMode(*resp.Modes)
	session.SetConn(conn)
	session.SetConfigOptions(resp.ConfigOptions)

	sm.mu.Lock()
	sm.activeSessions = append(sm.activeSessions, session)
	sm.mu.Unlock()
	log.Println("created new session: ", session.SessionID)
	return session, nil
}

// ListSessions from Agent. Sessions returned are not *active*, callers need to call LoadSession to get an active one.
func (sm *SessionManager) ListSessions(ctx context.Context, filterCwd string) ([]*ACPSession, error) {
	if sm.conn == nil {
		return nil, fmt.Errorf("not connected to agent")
	}

	// If the agent does not support loading sessions, return without error.
	listCap := sm.conn.AgentCapabilities.SessionCapabilities.List
	if listCap == nil {
		return nil, fmt.Errorf("Agent does not support listing sessions")
	}

	cwd, err := filepath.Abs(filterCwd)
	if err != nil {
		return nil, err
	}

	allAgentSessions := make([]*ACPSession, 0)

	cursor := ""
	first := true
	for cursor != "" || first {
		first = false
		resp, err := sm.conn.Conn.ListSessions(ctx, acp.ListSessionsRequest{
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
func (sm *SessionManager) LoadSession(ctx context.Context, session *ACPSession) (*ACPSession, error) {
	if session.Active() {
		return session, nil
	}

	if sm.conn == nil {
		return nil, fmt.Errorf("not connected to agent")
	}

	// If the agent does not support loading session, return without error.
	if !sm.conn.AgentCapabilities.LoadSession {
		return nil, nil
	}

	// Agents will call 'session/update' before returing from LoadSession, so we have to make
	// it an active session before the rpc return.
	sm.mu.Lock()
	session.SetConn(sm.conn)
	sm.activeSessions = append(sm.activeSessions, session)
	sm.mu.Unlock()

	resp, err := sm.conn.Conn.LoadSession(ctx, acp.LoadSessionRequest{
		Cwd:        session.Cwd,
		McpServers: []acp.McpServer{},
		SessionId:  acp.SessionId(session.SessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return nil, err
	}

	session.SetMode(*resp.Modes)
	session.SetConfigOptions(resp.ConfigOptions)

	return session, nil
}

// ResumeSession loads a session from the Agent. Unlike LoadSession, the Agent will NOT replay prior
// conversation to the client.
func (sm *SessionManager) ResumeSession(ctx context.Context, session *ACPSession) (*ACPSession, error) {
	if session.Active() {
		return session, nil
	}

	if sm.conn == nil {
		return nil, fmt.Errorf("not connected to agent")
	}

	// If the agent does not support resume sessions, return without error.
	resumeCap := sm.conn.AgentCapabilities.SessionCapabilities.Resume
	if resumeCap == nil {
		return nil, fmt.Errorf("Agent does not support resuming session")
	}

	// Agents will call 'session/update' before returing from ResumeSession, so we have to make
	// it an active session before the rpc return.
	sm.mu.Lock()
	session.SetConn(sm.conn)
	sm.activeSessions = append(sm.activeSessions, session)
	sm.mu.Unlock()

	resp, err := sm.conn.Conn.ResumeSession(ctx, acp.ResumeSessionRequest{
		Cwd:        session.Cwd,
		McpServers: []acp.McpServer{},
		SessionId:  acp.SessionId(session.SessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return nil, err
	}

	session.SetMode(*resp.Modes)
	session.SetConfigOptions(resp.ConfigOptions)

	return session, nil
}

// CloseSession allow Clients to tell the Agent to cancel any ongoing work
// for a session and free any resources associated with that active session.
func (sm *SessionManager) CloseSession(ctx context.Context, sessionID string) error {
	sm.mu.Lock()
	session := sm.getActiveSession(sessionID)
	if session == nil {
		sm.mu.Unlock()
		return fmt.Errorf("no active session found: %s", sessionID)
	}
	sm.mu.Unlock()

	// If the agent does not support close active sessions, return without error.
	closeCap := session.Conn().AgentCapabilities.SessionCapabilities.Close
	if closeCap == nil {
		return nil
	}

	_, err := session.Conn().Conn.CloseSession(ctx, acp.CloseSessionRequest{
		SessionId: acp.SessionId(sessionID),
	})

	err = checkACPErr(err)
	if err != nil {
		return err
	}

	session.Close()

	sm.mu.Lock()
	sm.activeSessions = slices.DeleteFunc(sm.activeSessions, func(sn *ACPSession) bool {
		return sn.SessionID == sessionID
	})
	sm.mu.Unlock()

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
	if sm.conn == nil {
		sm.mu.Unlock()
		return nil
	}

	activeSessions := sm.activeSessions
	sm.mu.Unlock()

	var err error
	for _, sn := range activeSessions {
		closeErr := sm.CloseSession(ctx, sn.SessionID)
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	closeErr := sm.conn.Close()
	if closeErr != nil {
		err = errors.Join(err, closeErr)
	}

	return err
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

func duplicatedIO(stdin io.WriteCloser, stdout io.ReadCloser) (io.WriteCloser, io.ReadCloser) {
	// Duplicate what the Client sends TO the Agent (Client -> Agent)
	// Anything written to loggedStdin goes to both the agent's stdin AND the console
	loggedStdin := struct {
		io.Writer
		io.Closer
	}{
		Writer: io.MultiWriter(stdin, os.Stderr),
		Closer: stdin,
	}

	// Duplicate what the Agent sends BACK to the Client (Agent -> Client)
	// As the ACP client reads from loggedStdout, a copy is automatically piped to the console
	loggedStdout := struct {
		io.Reader
		io.Closer
	}{
		Reader: io.TeeReader(stdout, os.Stderr),
		Closer: stdout,
	}

	return loggedStdin, loggedStdout

}
