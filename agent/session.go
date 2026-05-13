package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/coder/acp-go-sdk"
)

type (
	PromptResponse = acp.PromptResponse

	UserMessageChunk        = acp.SessionUpdateUserMessageChunk
	AgentMessageChunk       = acp.SessionUpdateAgentMessageChunk
	AgentThoughtChunk       = acp.SessionUpdateAgentThoughtChunk
	ToolCall                = acp.SessionUpdateToolCall
	ToolCallUpdate          = acp.SessionToolCallUpdate
	Plan                    = acp.SessionUpdatePlan
	AvailableCommandsUpdate = acp.SessionAvailableCommandsUpdate
	CurrentModeUpdate       = acp.SessionCurrentModeUpdate
	ConfigOptionUpdate      = acp.SessionConfigOptionUpdate
	SessionInfoUpdate       = acp.SessionSessionInfoUpdate
	UsageUpdate             = acp.SessionUsageUpdate
)

type PermissionGrantRequest struct {
	Req          acp.RequestPermissionRequest
	ResponseChan chan acp.PermissionOptionId
}

type SessionUpdateSubsciber interface {
	OnUserMessage(chunk UserMessageChunk)

	OnAgentMessage(chunk AgentMessageChunk)

	OnAgentThought(chunk AgentThoughtChunk)

	OnToolCallInit(toolCall ToolCall)

	OnToolCallUpdate(toolCallUpdate ToolCallUpdate)

	OnPlan(plan Plan)

	OnRequestPermission(params PermissionGrantRequest)
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
	commands      []acp.AvailableCommand
	configOptions []acp.SessionConfigOption
	usage         UsageUpdate
	mu            sync.Mutex

	// ongoing prompt turn info
	hasOngoingTurn atomic.Bool
	CurrentTurn    *PromptTurn

	// channel used to exchange session/update data.
	updateChan chan any
	grantChan  chan PermissionGrantRequest
	// bound to a view or not. A session has to be bound to
	// a view implementing a SessionUpdateSubsciber to work.
	bound bool
}

func NewACPSession(sessionID string, cwd string) *ACPSession {
	return &ACPSession{
		SessionID:  sessionID,
		Cwd:        cwd,
		updateChan: make(chan any, 1),
		grantChan:  make(chan PermissionGrantRequest),
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
	return sn.commands
}

func (sn *ACPSession) ConfigOptions() []acp.SessionConfigOption {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.configOptions
}

func (sn *ACPSession) UpdateInfo(title string, updatedAt string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.title = title
	sn.updatedAt = updatedAt
}

func (sn *ACPSession) SetMode(modeState acp.SessionModeState) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.modeState = modeState
}

func (sn *ACPSession) UpdateMode(modeID string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.modeState.CurrentModeId = acp.SessionModeId(modeID)
}

func (sn *ACPSession) SetCommands(commands []acp.AvailableCommand) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.commands = commands
}

func (sn *ACPSession) SetConfigOptions(options []acp.SessionConfigOption) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.configOptions = options
}

func (sn *ACPSession) UpdateUsage(usage UsageUpdate) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.usage = usage
}

// Prompt sends content blocks to Agent. If there are no pending tool calls,
// the turn ends and the Agent respond with a StopReason and optional Usage.
func (sn *ACPSession) Prompt(ctx context.Context, contents ...acp.ContentBlock) (PromptResponse, error) {
	// Validate the content structure and kind.
	//
	// ACP: As a baseline, all Agents MUST support ContentBlock::Text and ContentBlock::ResourceLink in session/prompt requests.
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

	if !sn.Active() {
		return PromptResponse{}, errors.New("invalid session")
	}

	if !sn.hasOngoingTurn.CompareAndSwap(false, true) {
		return PromptResponse{}, errors.New("A prompt turn is ongoing, please wait for it to finish, or cancel it")
	}

	defer func() {
		sn.hasOngoingTurn.Store(false)
		sn.CurrentTurn = nil
	}()

	// start a new turn.
	sn.CurrentTurn = NewPromptTurn()

	// Prompt will not get a response until there is no pending tool calls, and the agent sends
	// the final response.
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

func (sn *ACPSession) HasOngoingTurn() bool {
	return sn.hasOngoingTurn.Load()
}

// Cancel cancels the ongoing prompt turn if there is one.
//
// According to ACP protocol:
//
//  1. The Client should mark all non-finished tool calls pertaining
//     to the current turn as cancelled as soon as it sends the session/cancel notification.
//
//  2. The client must respond to all pending session/request_permission requests
//     with the cancelled outcome.
func (sn *ACPSession) Cancel(ctx context.Context) error {
	if sn.hasOngoingTurn.CompareAndSwap(true, false) {
		defer func() {
			// the pending session/request_permission requests should check this to cancel
			// themselves. This should be called BEFORE session/request_permission responds.
			// so we should not set sn.CurrentTurn = nil.
			if sn.CurrentTurn != nil {
				sn.CurrentTurn.Cancel()
			}
			log.Println("prompt turn canceled")
		}()

		return sn.conn.Conn.Cancel(ctx, acp.CancelNotification{
			SessionId: acp.SessionId(sn.SessionID),
		})

	}

	return nil
}

func (sn *ACPSession) RequestPermission(req acp.RequestPermissionRequest, grantResponseChan chan acp.PermissionOptionId) {
	sn.mu.Lock()
	if sn.grantChan == nil {
		sn.grantChan = make(chan PermissionGrantRequest)
	}
	sn.mu.Unlock()

	sn.grantChan <- PermissionGrantRequest{
		Req:          req,
		ResponseChan: grantResponseChan,
	}
}

func (sn *ACPSession) PublishUpdate(update any) {
	sn.mu.Lock()
	if sn.updateChan == nil {
		sn.updateChan = make(chan any, 3)
	}
	sn.mu.Unlock()

	sn.updateChan <- update
}

func (sn *ACPSession) SubscribeUpdates(ctx context.Context, sub SessionUpdateSubsciber) {
	if sub == nil {
		return
	}

	sn.mu.Lock()
	if sn.bound {
		return
	}
	sn.bound = true
	sn.mu.Unlock()

	go func() {
		for {
			select {
			case update, ok := <-sn.updateChan:
				if !ok {
					return
				}
				switch update := update.(type) {
				case UserMessageChunk:
					sub.OnUserMessage(update)
				case AgentMessageChunk:
					sub.OnAgentMessage(update)
				case AgentThoughtChunk:
					sub.OnAgentThought(update)
				case ToolCall:
					if sn.CurrentTurn != nil {
						sn.CurrentTurn.UpdateToolCall(update)
					}
					sub.OnToolCallInit(update)
				case ToolCallUpdate:
					if sn.CurrentTurn != nil {
						sn.CurrentTurn.UpdateToolCall(update)
					}
					sub.OnToolCallUpdate(update)
				case Plan:
					sub.OnPlan(update)
				case AvailableCommandsUpdate:
					sn.SetCommands(update.AvailableCommands)
				case CurrentModeUpdate:
					sn.UpdateMode(string(update.CurrentModeId))
				case ConfigOptionUpdate:
					sn.SetConfigOptions(update.ConfigOptions)
				case SessionInfoUpdate:
					sn.UpdateInfo(*update.Title, *update.UpdatedAt)
				case UsageUpdate:
					sn.UpdateUsage(update)
				default:
					log.Panicf("unknown update object: %v", update)
				}
			case permissionReq, ok := <-sn.grantChan:
				if !ok {
					return
				}
				sub.OnRequestPermission(permissionReq)
			case <-ctx.Done():
				log.Println("session subscriber closed")
				return
			}
		}
	}()
}

func (sn *ACPSession) Close() {
	if sn.CurrentTurn != nil {
		sn.CurrentTurn.Close()
	}

	if sn.updateChan != nil {
		close(sn.updateChan)
	}

	if sn.grantChan != nil {
		close(sn.grantChan)
	}
}
